package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Provisional verdicts (prompt feature verdict; PROTOCOL §8). Every closing
// entry except not-examined carries the agent's own assessment of the item:
// one of the four classes, the mechanism, the environment-specific reasoning,
// what is verified and what is assumed, and what would overturn it. A
// verdict is measured against ground truth independently of the policy and
// is never a policy input: the classifier does not receive it
// (verdict_policy_test.go). The agent may say not-affected where the policy
// cannot. The verdict's shape is checked (a malformed one rejects the call so
// the model can fix it); its citations, verified facts and mitigations are
// resolved one by one, and whatever does not resolve is dropped and counted.

const (
	maxVerdictList  = 20
	maxDroppedNotes = 10
)

// StartFrom makes the focus item's session start from its dossier: the
// dossier is a run artifact, the item counts as examined (the dossier holds
// everything get_change_item returns), and the artifacts the dossier quotes
// count as retrieved in this session. The dossier is inlined into the item
// task, so the session's prompt digest pins it.
func (s *Session) StartFrom(d *Dossier, ref domain.ArtifactRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addRunArtifact(ref)
	s.dossiers[d.Item] = ref.ID
	s.examined[d.Item] = true
	s.inspections[d.Item]++
	if s.retrieved != nil {
		for _, a := range append(d.Artifacts(), ref.ID) {
			s.retrieved[a] = true
		}
	}
}

type getDossierArgs struct {
	ChangeItem string `json:"changeItem"`
}

// getDossier serves an item's dossier, built now from the run (the
// deterministic pass's validations plus those this run requested).
func (s *Session) getDossier(ctx context.Context, item string) (*outcome, error) {
	it, err := s.item(item)
	if err != nil {
		return nil, err
	}
	vals := append(append([]domain.ValidationRecord(nil), s.cfg.Validations...), s.vals...)
	d, err := BuildDossier(ctx, DossierInput{Bundle: s.cfg.Bundle, Graph: s.cfg.Graph, Traversal: s.cfg.Traversal, Report: s.cfg.Report,
		Validations: vals, Read: s.bytesOf}, it.ID)
	if err != nil {
		return nil, err
	}
	return &outcome{value: d, commit: s.retrieve(func() { s.examined[it.ID] = true; s.inspections[it.ID]++ }, d.Artifacts()...)}, nil
}

// GetDossier is the typed form of get_dossier.
func (s *Session) GetDossier(ctx context.Context, item string) (*Dossier, error) {
	return typed[*Dossier](s, ctx, investigate.ToolGetDossier, getDossierArgs{ChangeItem: item},
		func(ctx context.Context) (*outcome, error) { return s.getDossier(ctx, item) })
}

// verdictOf builds the verdict of a closing entry (caller holds s.mu, inside
// a tool implementation: nothing is committed here).
func (s *Session) verdictOf(ctx context.Context, it domain.ChangeItem, in investigate.VerdictInput) (domain.AgentVerdict, []domain.Evidence, error) {
	var v domain.AgentVerdict
	if !oneOfClass(in.Class) {
		return v, nil, rejectf("verdict.class %q: want affected, not-affected, potentially-affected or insufficient-evidence", in.Class)
	}
	switch in.Confidence {
	case domain.ConfidenceLow, domain.ConfidenceMedium, domain.ConfidenceHigh:
	default:
		return v, nil, rejectf("verdict.confidence %q: want low, medium or high", in.Confidence)
	}
	mech, err := s.statement("verdict.mechanism", in.Mechanism)
	if err != nil {
		return v, nil, err
	}
	reasoning, err := s.statement("verdict.environmentReasoning", in.EnvironmentReasoning)
	if err != nil {
		return v, nil, err
	}
	overturn, err := s.statement("verdict.wouldOverturn", in.WouldOverturn)
	if err != nil {
		return v, nil, err
	}
	if len(in.Uncertainty) > MaxStatement {
		return v, nil, rejectf("verdict.uncertainty is %d bytes (max %d)", len(in.Uncertainty), MaxStatement)
	}
	assumptions, err := textList("verdict.assumptions", in.Assumptions)
	if err != nil {
		return v, nil, err
	}
	validations, err := textList("verdict.proposedValidations", in.ProposedValidations)
	if err != nil {
		return v, nil, err
	}
	if len(in.Citations) > MaxCitations || len(in.VerifiedFacts) > maxVerdictList || len(in.Mitigations) > maxVerdictList {
		return v, nil, rejectf("verdict: at most %d citations and %d verified facts / mitigations", MaxCitations, maxVerdictList)
	}
	v = domain.AgentVerdict{ChangeItem: it.ID, Class: in.Class, Confidence: in.Confidence, Mechanism: mech, EnvironmentReasoning: reasoning,
		Assumptions: assumptions, Uncertainty: trimmed(in.Uncertainty), ProposedValidations: validations, WouldOverturn: overturn,
		Dossier: s.dossiers[it.ID], StatementKind: s.proseKind()}
	drop := func(format string, args ...any) {
		if len(v.Dropped) < maxDroppedNotes {
			v.Dropped = append(v.Dropped, fmt.Sprintf(format, args...))
		}
	}
	// Each citation on its own: one that does not resolve is dropped, not fatal.
	var evs []domain.Evidence
	for i, c := range in.Citations {
		e, err := s.citations(ctx, []investigate.Citation{c})
		if err != nil {
			v.DroppedCitations++
			drop("citation %d: %v", i+1, strings.TrimPrefix(err.Error(), investigate.ErrRejected.Error()+": "))
			continue
		}
		evs = append(evs, e...)
	}
	v.Citations = evidenceIDs(evs)
	for _, id := range uniqSorted(in.VerifiedFacts) {
		switch why := s.verifiedFact(it.ID, id); why {
		case "":
			v.VerifiedFacts = append(v.VerifiedFacts, id)
		default:
			v.DroppedVerifiedFacts++
			drop("verified fact %s: %s", id, why)
		}
	}
	for _, id := range uniqSorted(in.Mitigations) {
		h, ok := s.hypothesis(id)
		switch {
		case !ok:
			drop("mitigation %s: no hypothesis with that id in this run", id)
		case h.ChangeItem != it.ID:
			drop("mitigation %s: a hypothesis about %s, not %s", id, h.ChangeItem, it.ID)
		default:
			v.Mitigations = append(v.Mitigations, id)
		}
	}
	if v.Class != domain.FindingInsufficientEvidence && len(v.Citations) == 0 {
		return domain.AgentVerdict{}, nil, rejectf("verdict: a %s verdict needs at least one citation that resolves (%d dropped: %s)", v.Class, v.DroppedCitations, strings.Join(v.Dropped, "; "))
	}
	v.Provenance = s.aiProvenance(append(append([]domain.EvidenceID(nil), v.Citations...), it.Evidence...))
	v.ID = domain.AgentVerdictID(it.ID, v.Class, v.Mechanism, s.call.ID)
	if err := invalid(func() error { c := v; c.Provenance = placeholder(c.Provenance); return c.Validate() }()); err != nil {
		return domain.AgentVerdict{}, nil, err
	}
	return v, evs, nil
}

// verifiedFact says why id is not a verified fact of the item ("" = it is):
// a setting claim of this run about the item (or no item), or a validation
// record of this run for the item.
func (s *Session) verifiedFact(item, id string) string {
	switch {
	case strings.HasPrefix(id, domain.SettingClaimIDPrefix):
		for _, c := range s.claims {
			if c.ID == id {
				if c.ChangeItem != "" && c.ChangeItem != item {
					return "a claim about " + c.ChangeItem
				}
				return ""
			}
		}
		return "no verified setting claim with that id in this run"
	case strings.HasPrefix(id, domain.ValidationRecordIDPrefix):
		v, ok := s.valIDs[id]
		switch {
		case !ok:
			return "no validation record with that id in this run"
		case v.ChangeItem != item:
			return "a validation of " + v.ChangeItem
		}
		return ""
	}
	return "neither a setting claim (clm-…) nor a validation record (vr-…)"
}

func (s *Session) hypothesis(id string) (domain.Hypothesis, bool) {
	for _, h := range s.hyps {
		if h.ID == id {
			return h, true
		}
	}
	return domain.Hypothesis{}, false
}

func oneOfClass(c domain.FindingClass) bool {
	for _, x := range domain.FindingClasses {
		if x == c {
			return true
		}
	}
	return false
}

func textList(what string, in []string) ([]string, error) {
	if len(in) > maxVerdictList {
		return nil, rejectf("%s: at most %d entries", what, maxVerdictList)
	}
	var out []string
	for _, x := range in {
		x = trimmed(x)
		if len(x) > MaxStatement {
			return nil, rejectf("%s: an entry is %d bytes (max %d)", what, len(x), MaxStatement)
		}
		if x != "" {
			out = append(out, x)
		}
	}
	return out, nil
}
