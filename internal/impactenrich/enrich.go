package impactenrich

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// Producer is recorded in the provenance of every impact enrichment.
const Producer = "impact-enrich@v1"

// Options configure Run.
type Options struct {
	// Client answers the prompts (typically an llm.Cache around the API or an
	// llm.Exchange). nil: candidates only, no enrichment.
	Client llm.Client
	// Model is the requested model ("" = the client's default). It is part of
	// the prompt digest; the recorded model is the one that answered.
	Model string
	// Max bounds the number of prompts per report (default 30).
	Max int
	// MaxConsecutiveFailures stops asking after this many failed requests in
	// a row, e.g. a bad API key (default 3).
	MaxConsecutiveFailures int
	// Clock stamps answers whose client did not report a generation time.
	Clock func() time.Time
}

// Request statuses (same vocabulary as the edge enricher).
const (
	StatusAnswered = "answered"
	StatusPending  = "pending"
	StatusFailed   = "failed"
	StatusRejected = "rejected" // the whole answer was refused (not JSON, wrong schema)
	StatusSkipped  = "skipped"  // not asked: over Max or after repeated failures
)

// RequestRecord reports what happened to the prompt of one candidate.
type RequestRecord struct {
	Group        string `json:"group"`
	Type         string `json:"type"`
	PromptDigest string `json:"promptDigest,omitempty"`
	Status       string `json:"status"`
	Origin       string `json:"origin,omitempty"` // api, cache, exchange, fake
	Detail       string `json:"detail,omitempty"`
}

// Result is the outcome of Run. It does not modify the report; Apply does.
type Result struct {
	Candidates  []Candidate          `json:"candidates"`
	Requests    []RequestRecord      `json:"requests"`
	Enrichments []domain.Enrichment  `json:"enrichments"`
	Run         domain.EnrichmentRun `json:"run"`
}

// Run computes the candidates of rep and, when a client is configured, asks
// the model about each and validates the answers. The report is not
// modified; Apply attaches the result.
func Run(ctx context.Context, rep *domain.ImpactReport, edge *domain.UpgradeEdge, e *env.Environment, opts Options) (*Result, error) {
	if rep == nil || edge == nil {
		return nil, errors.New("impactenrich: nil report or edge")
	}
	if opts.Max <= 0 {
		opts.Max = 30
	}
	if opts.MaxConsecutiveFailures <= 0 {
		opts.MaxConsecutiveFailures = 3
	}
	now := opts.Clock
	if now == nil {
		now = time.Now
	}
	cands := Candidates(rep, edge, CandidateOptions{})
	res := &Result{Candidates: cands, Requests: []RequestRecord{}, Enrichments: []domain.Enrichment{}}
	res.Run = domain.EnrichmentRun{Producer: Producer, PromptVersion: PromptVersion, CandidateGroups: len(cands)}
	if opts.Client == nil {
		return res, nil
	}

	in := newInputs(edge, rep)
	findingsOf := func(cand Candidate) []domain.ImpactFinding {
		byID := map[string]domain.ImpactFinding{}
		for _, f := range rep.Findings {
			byID[f.ID] = f
		}
		fs := make([]domain.ImpactFinding, 0, len(cand.Findings))
		for _, id := range cand.Findings {
			fs = append(fs, byID[id])
		}
		return fs
	}
	clustered := map[string]string{} // finding id → cluster enrichment id
	usedIDs := map[string]bool{}
	failures := 0
	for i, cand := range cands {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if i >= opts.Max || failures >= opts.MaxConsecutiveFailures {
			detail := fmt.Sprintf("over the limit of %d prompts per report", opts.Max)
			if failures >= opts.MaxConsecutiveFailures {
				detail = fmt.Sprintf("not asked after %d failed requests in a row", failures)
			}
			res.Requests = append(res.Requests, RequestRecord{Group: cand.ID, Type: string(cand.Type), Status: StatusSkipped, Detail: detail})
			continue
		}
		p := buildPrompt(rep, cand, opts.Model, in, e)
		digest := llm.PromptDigest(p.req)
		res.Run.Requests++
		rec := RequestRecord{Group: cand.ID, Type: string(cand.Type), PromptDigest: digest}
		resp, err := opts.Client.Complete(ctx, p.req)
		switch {
		case err != nil && ctx.Err() != nil:
			return nil, ctx.Err()
		case errors.Is(err, llm.ErrPending), errors.Is(err, llm.ErrNotCached):
			rec.Status, rec.Detail = StatusPending, err.Error()
			res.Run.Pending++
			res.Requests = append(res.Requests, rec)
			continue
		case errors.Is(err, llm.ErrExchangeMismatch):
			// A supplied answer that belongs to another prompt: refused
			// model output, recorded like any other rejection.
			rec.Status, rec.Detail, rec.Origin = StatusRejected, err.Error(), llm.OriginExchange
			res.reject(cand, digest, rec.Detail)
			res.Requests = append(res.Requests, rec)
			continue
		case err != nil:
			rec.Status, rec.Detail = StatusFailed, err.Error()
			res.Run.Failed++
			failures++
			res.Requests = append(res.Requests, rec)
			continue
		}
		failures = 0
		rec.Origin = resp.Origin
		if resp.Model == "" || resp.ModelVersion == "" {
			rec.Status, rec.Detail = StatusRejected, "the answer does not report the model and model version that produced it"
			res.reject(cand, digest, rec.Detail)
			res.Requests = append(res.Requests, rec)
			continue
		}
		ans, err := decode(cand.Type, resp.Text)
		if err != nil {
			rec.Status, rec.Detail = StatusRejected, err.Error()
			res.reject(cand, digest, rec.Detail)
			res.Requests = append(res.Requests, rec)
			continue
		}
		rec.Status = StatusAnswered
		res.Requests = append(res.Requests, rec)

		gen := resp.GeneratedAt
		if gen.IsZero() {
			gen = now()
		}
		gen = gen.UTC()
		g := newAnswerContext(cand, findingsOf(cand), in, p.input)
		en, rejectReason := g.toEnrichment(ans, digest, p.input, resp, gen)
		if rejectReason != "" {
			res.reject(cand, digest, rejectReason)
			continue
		}
		if en == nil {
			continue // answered, but nothing to attach (e.g. "not duplicates")
		}
		if en.Kind == domain.EnrichmentCluster {
			dup := ""
			for _, id := range en.RelatesTo {
				if prev, ok := clustered[id]; ok && (dup == "" || prev != dup) {
					dup = prev
				}
			}
			if dup != "" {
				res.reject(cand, digest, fmt.Sprintf("a member is already consolidated by cluster %s", dup))
				continue
			}
			for _, id := range en.RelatesTo {
				clustered[id] = en.ID
			}
		}
		for usedIDs[en.ID] {
			en.ID = domain.EnrichmentIDPrefix + domain.ShortHash(en.ID)
		}
		usedIDs[en.ID] = true
		res.Enrichments = append(res.Enrichments, *en)
	}
	res.Run.Accepted = len(res.Enrichments)
	res.Run.Clusters, res.Run.ClusteredChanges = domain.ClusterMetrics(res.Enrichments)
	res.Run.DuplicatesConsolidated = res.Run.ClusteredChanges - res.Run.Clusters
	return res, nil
}

func (r *Result) reject(cand Candidate, digest, reason string) {
	r.Run.Rejected = append(r.Run.Rejected, domain.EnrichmentRejection{Group: cand.ID, PromptDigest: digest, Reason: reason})
}

// toEnrichment validates the decoded answer of one candidate and turns it
// into the Enrichment it may become. Returns (nil, "") for an answered
// question that yields no enrichment ("not duplicates"); (nil, reason) for a
// rejected proposal.
//
// The forbidden transitions are enforced by construction: the verdict's
// effect is looked up in verdictEffects (no input can name a different
// effect), citations are filtered to the evidence the prompt showed, and the
// confidence is capped at medium.
func (g *answerContext) toEnrichment(ans any, digest string, input []domain.EvidenceID, resp *llm.Response, gen time.Time) (*domain.Enrichment, string) {
	var kind domain.EnrichmentKind
	var title, content string
	var citations []string
	switch a := ans.(type) {
	case applicabilityAnswer:
		k, err := g.checkApplicability(a)
		if err != nil {
			return nil, err.Error()
		}
		kind, title, content, citations = k, a.Title, a.Content, a.Citations
	case clusterAnswer:
		same, err := g.checkCluster(a)
		if err != nil {
			return nil, err.Error()
		}
		if !same {
			return nil, ""
		}
		kind, title, content, citations = domain.EnrichmentCluster, a.Title, a.Content, a.Citations
	case migrationAnswer:
		steps, err := g.checkMigration(a)
		if err != nil {
			return nil, err.Error()
		}
		kind, title, content, citations = domain.EnrichmentMigrationSummary, a.Title, steps, a.Citations
	default:
		return nil, fmt.Sprintf("unknown answer type %T", ans)
	}

	cited := map[string]bool{}
	for _, id := range citations {
		cited[id] = true
	}
	var cites []domain.EvidenceID
	for _, id := range input {
		if cited[string(id)] {
			cites = append(cites, id)
		}
	}
	idParts := append([]string{digest, string(kind)}, g.cand.Findings...)
	return &domain.Enrichment{
		ID:        domain.EnrichmentIDPrefix + domain.ShortHash(idParts...),
		Kind:      kind,
		Title:     oneLine(title),
		Content:   strings.TrimSpace(content),
		RelatesTo: append([]string(nil), g.cand.Findings...),
		Citations: cites,
		Provenance: domain.Provenance{
			Method:        domain.MethodAI,
			Producer:      Producer,
			Rule:          "cand:" + g.cand.ID,
			Confidence:    impactConfidence(),
			Model:         resp.Model,
			ModelVersion:  resp.ModelVersion,
			PromptVersion: PromptVersion,
			PromptDigest:  digest,
			InputEvidence: append([]domain.EvidenceID(nil), input...),
			GeneratedAt:   &gen,
		},
	}, ""
}

// Apply attaches the result to the report: the enrichments, the run
// metadata, the review suggestions on the unknown findings (the ONLY report
// mutation an accepted verdict can produce: unknown → suggested
// review-required), and the environment evidence records the accepted
// enrichments cite (deterministic copies from the environment pool, so the
// citations resolve). The deterministic findings, their classifications and
// both evidence chains are not touched. The report is validated afterwards;
// on failure it is left unchanged.
func Apply(rep *domain.ImpactReport, r *Result, e *env.Environment) error {
	if rep == nil || r == nil {
		return errors.New("impactenrich: Apply without a report or result")
	}
	prevEns, prevRun := rep.Enrichments, rep.EnrichmentRun
	prevSummary := rep.Summary
	prevEvidenceN := len(rep.EnvironmentEvidence)
	prevSuggested := map[string]domain.ImpactClass{}
	for i := range rep.Findings {
		f := &rep.Findings[i]
		prevSuggested[f.ID] = f.SuggestedClassification
	}

	rep.Enrichments = append([]domain.Enrichment(nil), r.Enrichments...)
	run := r.Run
	rep.EnrichmentRun = &run

	for i := range rep.Findings {
		f := &rep.Findings[i]
		if f.Classification != domain.ImpactUnknown {
			continue
		}
		for _, en := range r.Enrichments {
			if en.Kind == domain.EnrichmentPlausiblyApplies && len(en.RelatesTo) == 1 && en.RelatesTo[0] == f.ID {
				// verdictEffects + suggestedClass: the ceiling of an AI
				// suggestion; there is no path here to action-required.
				f.SuggestedClassification = suggestedClass
			}
		}
	}

	// the prompt's environment entries join the report's local pool
	// (deterministic records from the environment, unchanged) so the
	// enrichments' input evidence — and not only their citations — resolves;
	// without -enrich the pool is exactly the findings' chain.
	if e != nil {
		pool := map[domain.EvidenceID]domain.Evidence{}
		for _, x := range e.Evidence {
			pool[x.ID] = x
		}
		have := map[domain.EvidenceID]bool{}
		for _, x := range rep.EnvironmentEvidence {
			have[x.ID] = true
		}
		for _, en := range r.Enrichments {
			for _, id := range en.Provenance.InputEvidence {
				if !have[id] {
					if x, ok := pool[id]; ok {
						rep.EnvironmentEvidence = append(rep.EnvironmentEvidence, x)
						have[id] = true
					}
				}
			}
		}
	}

	n := 0
	for i := range rep.Findings {
		if rep.Findings[i].SuggestedClassification != "" {
			n++
		}
	}
	rep.Summary.SuggestedReview = n

	if err := rep.Validate(); err != nil {
		rep.Enrichments, rep.EnrichmentRun = prevEns, prevRun
		rep.Summary = prevSummary
		rep.EnvironmentEvidence = rep.EnvironmentEvidence[:prevEvidenceN]
		for i := range rep.Findings {
			rep.Findings[i].SuggestedClassification = prevSuggested[rep.Findings[i].ID]
		}
		return fmt.Errorf("impactenrich: the enriched report is invalid: %w", err)
	}
	return nil
}
