package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/semantic"
)

// Export of mitigation hypotheses into the knowledge store (`ri investigate
// export-hypotheses`; PROTOCOL §6 step 4). Each well-formed mitigation
// becomes a SemanticProposal (AI provenance, rule hypothesis:<hyp>@<inv>)
// and a whole-fact fact-review item proposing exactly its rule, on the
// knowledge candidate of the item's upstream change. Never a fact: the rule
// becomes knowledge only when a human accepts the item, which the promotion
// gate allows only with a confirmed proof record (lane verify). Idempotent:
// every record is content-addressed, and an existing review item is left as
// it is (its status may have moved on).

// Export outcomes.
const (
	ExportExported = "exported"
	ExportExisting = "existing"
	ExportSkipped  = "skipped"
)

// ExportResult says what became of one hypothesis.
type ExportResult struct {
	Hypothesis string `json:"hypothesis"`
	ChangeItem string `json:"changeItem"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	Candidate  string `json:"candidate,omitempty"`
	Proposal   string `json:"proposal,omitempty"`
	ReviewItem string `json:"reviewItem,omitempty"`
	RuleDigest string `json:"ruleDigest,omitempty"`
}

// ExportHypotheses exports the run's mitigation hypotheses to the knowledge
// store. Hypotheses without a well-formed mitigation are reported skipped.
func ExportHypotheses(ctx context.Context, inv *domain.Investigation, b *domain.EvidenceBundle, st investigate.ArtifactStore, ks knowledge.Store, now time.Time) ([]ExportResult, error) {
	if inv == nil || b == nil || inv.Bundle != b.ID {
		return nil, errors.New("agent: export needs the investigation and its bundle")
	}
	var edge *domain.UpgradeEdge
	pool := b.EvidenceByID()
	for _, e := range inv.Evidence {
		if _, ok := pool[e.ID]; !ok {
			pool[e.ID] = e
		}
	}
	arts := map[string]domain.ArtifactRef{}
	for _, a := range b.Manifest.Artifacts {
		arts[a.ID] = a
	}
	proposals := map[string]string{} // proposal id -> hypothesis exported under it
	var out []ExportResult
	hyps := append([]domain.Hypothesis(nil), inv.Hypotheses...)
	sort.Slice(hyps, func(i, j int) bool { return hyps[i].ID < hyps[j].ID })
	for _, h := range hyps {
		r := ExportResult{Hypothesis: h.ID, ChangeItem: h.ChangeItem, Status: ExportSkipped}
		m := h.Mitigation
		if m == nil {
			r.Reason = "no well-formed mitigation"
			if h.MitigationRejected != "" {
				r.Reason += ": " + h.MitigationRejected
			}
			out = append(out, r)
			continue
		}
		r.RuleDigest = m.ProposedRule.Digest()
		if err := m.Validate(); err != nil {
			r.Reason = "the stored mitigation does not validate: " + err.Error()
			out = append(out, r)
			continue
		}
		it, ok := b.Item(h.ChangeItem)
		if !ok || it.ChangeID == "" {
			r.Reason = "the item has no upstream change to anchor knowledge on"
			out = append(out, r)
			continue
		}
		if edge == nil {
			var err error
			if edge, err = readEdge(ctx, st, b); err != nil {
				return nil, err
			}
		}
		cand, why, err := candidateFor(ctx, ks, edge, it.ChangeID, inv.CreatedAt)
		if err != nil {
			return nil, err
		}
		if cand == nil {
			r.Reason = why
			out = append(out, r)
			continue
		}
		r.Candidate = cand.ID
		if cand.Release == "" || !m.ProposedRule.Release.Contains(cand.Release) {
			r.Reason = fmt.Sprintf("the change's release %q is not within the rule's bounds (%s, %s]", cand.Release, m.ProposedRule.Release.From, m.ProposedRule.Release.To)
			out = append(out, r)
			continue
		}
		var cites []domain.Evidence
		for _, id := range h.Citations {
			e, ok := pool[id]
			if !ok || !upstreamEvidence(e, arts) {
				continue // environment evidence never enters knowledge
			}
			cites = append(cites, e)
		}
		if len(cites) == 0 {
			r.Reason = "no upstream citation: knowledge cites release-level evidence only"
			out = append(out, r)
			continue
		}
		if _, err := knowledge.ExtendCandidateEvidence(ctx, ks, cand.ID, cites); err != nil {
			return nil, fmt.Errorf("agent: export %s: extend candidate %s: %w", h.ID, cand.ID, err)
		}
		p := proposalOf(h, inv.ID, cand.ID, cites)
		if prev, dup := proposals[p.ID]; dup {
			r.Reason = fmt.Sprintf("hypothesis %s of the same model call was already exported as proposal %s (one proposal per call and candidate)", prev, p.ID)
			out = append(out, r)
			continue
		}
		if err := p.Validate(); err != nil {
			r.Reason = "the proposal does not validate: " + err.Error()
			out = append(out, r)
			continue
		}
		r.Proposal = p.ID
		existing, err := put(ctx, ks, p, p.ID)
		if err != nil {
			return nil, fmt.Errorf("agent: export %s: %w", h.ID, err)
		}
		proposals[p.ID] = h.ID
		item := reviewItemOf(h, inv.ID, *cand, p.ID, now)
		r.ReviewItem = item.ID
		itemExisting, err := put(ctx, ks, item, item.ID)
		if err != nil {
			return nil, fmt.Errorf("agent: export %s: %w", h.ID, err)
		}
		r.Status = ExportExported
		if existing && itemExisting {
			r.Status = ExportExisting
		}
		out = append(out, r)
	}
	return out, nil
}

func readEdge(ctx context.Context, st investigate.ArtifactStore, b *domain.EvidenceBundle) (*domain.UpgradeEdge, error) {
	for _, a := range b.Manifest.Artifacts {
		if a.ID != b.Change.Edge {
			continue
		}
		data, err := StoreReader(st)(ctx, a)
		if err != nil {
			return nil, err
		}
		var e domain.UpgradeEdge
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("agent: upgrade edge %s: %w", a.ID, err)
		}
		return &e, nil
	}
	return nil, fmt.Errorf("%w: upgrade-edge artifact %s", investigate.ErrNotFound, b.Change.Edge)
}

// candidateFor returns the knowledge candidate of a change: the store's own
// when it holds the candidate the edge yields (or any candidate naming the
// change), else the edge's, written to the store.
func candidateFor(ctx context.Context, ks knowledge.Store, edge *domain.UpgradeEdge, changeID string, at time.Time) (*domain.SemanticCandidate, string, error) {
	if at.IsZero() {
		at = time.Unix(0, 0).UTC()
	}
	for _, c := range semantic.Candidates(edge, at) {
		named := false
		for _, m := range c.Members {
			named = named || m.ChangeID == changeID
		}
		if !named {
			continue
		}
		rec, err := ks.Get(ctx, c.ID)
		switch {
		case err == nil && rec.Candidate != nil:
			return rec.Candidate, "", nil
		case err != nil && !errors.Is(err, knowledge.ErrNotFound):
			return nil, "", err
		}
		r, err := domain.NewRecord(c)
		if err != nil {
			return nil, "", err
		}
		if err := ks.Put(ctx, r); err != nil {
			return nil, "", fmt.Errorf("agent: write candidate %s: %w", c.ID, err)
		}
		return &c, "", nil
	}
	snap, err := ks.Load(ctx, knowledge.Query{Product: edge.Product.ID, Kinds: []domain.RecordKind{domain.RecordCandidate}})
	if err != nil {
		return nil, "", err
	}
	for i := range snap.Candidates {
		for _, m := range snap.Candidates[i].Members {
			if m.ChangeID == changeID {
				return &snap.Candidates[i], "", nil
			}
		}
	}
	return nil, "the change is not a knowledge candidate (the semantic pipeline does not propose rules for it)", nil
}

// upstreamEvidence reports whether evidence is release-level: not an
// environment file or input, not an environment render, and when it names
// an artifact of the bundle, an upstream one.
func upstreamEvidence(e domain.Evidence, arts map[string]domain.ArtifactRef) bool {
	if e.Kind == domain.EvidenceLocalFile || e.Kind == domain.EvidenceInput {
		return false
	}
	if e.Render != nil && e.Render.Scope != domain.RenderRelease {
		return false
	}
	if a, ok := arts[e.SourceID]; ok {
		return a.Origin == domain.OriginUpstream
	}
	return e.Render != nil
}

func proposalOf(h domain.Hypothesis, inv, cand string, cites []domain.Evidence) domain.SemanticProposal {
	prov := h.Provenance
	prov.Rule = domain.HypothesisRule(h.ID, inv)
	prov.Confidence = domain.ConfidenceLow
	prov.InputEvidence = nil
	for _, e := range cites {
		prov.InputEvidence = append(prov.InputEvidence, e.ID)
	}
	prov.InputEvidence = uniqEvidence(prov.InputEvidence)
	provider := prov.ProviderName()
	if provider == "" {
		provider = unreportedModelInfo
		prov.Provider = provider
	}
	p := domain.SemanticProposal{CandidateID: cand, Task: domain.TaskFull, Provider: provider, Assertion: h.Mitigation.ProposedRule.Assertion,
		Citations: prov.InputEvidence, Provenance: prov}
	p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	return p
}

func reviewItemOf(h domain.Hypothesis, inv string, c domain.SemanticCandidate, proposal string, now time.Time) domain.ReviewItem {
	m := h.Mitigation
	a := m.ProposedRule.Assertion
	q := fmt.Sprintf("Agent-proposed mitigation rule (AI, unverified; hypothesis %s of investigation %s): %s Release bounds (%s, %s]. "+
		"Is this whole rule correct — subject, change, applicability and consequence? Requested verification: %s "+
		"Accepting or correcting it needs a confirmed proof record for this exact rule.",
		h.ID, inv, strings.TrimSpace(a.Statement), m.ProposedRule.Release.From, m.ProposedRule.Release.To, m.RequestedVerification)
	return domain.ReviewItem{ID: domain.ReviewItemID(c.ID, domain.QuestionFactReview, a), CandidateID: c.ID, Product: c.Product, Release: c.Release,
		QuestionType: domain.QuestionFactReview, Question: q, Proposed: a, Proposals: []string{proposal},
		Routing: domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityNormal, Signals: []domain.RoutingSignal{domain.SignalSingleModel}},
		Status:  domain.ReviewPending, CreatedAt: now.UTC()}
}

// put writes a record unless one with its id exists (reported).
func put(ctx context.Context, ks knowledge.Store, entity any, id string) (bool, error) {
	if _, err := ks.Get(ctx, id); err == nil {
		return true, nil
	} else if !errors.Is(err, knowledge.ErrNotFound) {
		return false, err
	}
	rec, err := domain.NewRecord(entity)
	if err != nil {
		return false, err
	}
	return false, ks.Put(ctx, rec)
}
