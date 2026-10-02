// Package semvalidate holds the deterministic validators of the learning loop
// (DESIGN.md §2.3, §9 `validate`): they check a proposed or corrected
// SemanticAssertion against the ingested release artifacts only — Helm values,
// CRD schemas, compatibility rows, image references and the edge's computed
// diffs — and never fetch anything.
//
// The contract every validator keeps:
//
//   - confirmed: the artifacts prove the aspect, and the result cites the
//     artifact evidence of BOTH sides it rests on;
//   - refuted: the artifacts contradict the aspect (a path that exists in
//     neither release, a default that is not the asserted one, a key that is
//     still present after being "removed");
//   - inconclusive: the artifacts carry too little to decide (no snapshot,
//     a schema that states no default, a deprecation, a behaviour).
//
// Absence of evidence is never a refutation unless the artifact is known to
// be complete for the claim (e.g. the group's CRDs are in the snapshot but
// the asserted path is not in any version's schema). A consequence is never
// confirmed, except the canonical `none` of an `added` subject.
package semvalidate

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// Producer strings (ValidationResult.Validator).
const (
	ProducerValues      = "semvalidate.values@v1"
	ProducerCRD         = "semvalidate.crd@v1"
	ProducerCompat      = "semvalidate.compat@v1"
	ProducerImage       = "semvalidate.image@v1"
	ProducerRestatement = "semvalidate.restatement@v1"
	ProducerCanonical   = "semvalidate.canonical@v1"
)

// Validators returns the deterministic validators in a stable order:
// helm-values, crd-schema, compatibility, image, restatement,
// canonical-applicability. rendered-diff belongs to the render lane.
//
// CONTRACT-CHANGE(validate): DESIGN §9 lists five validators; `restatement`
// (UNKNOWN-ANALYSIS §3.5-6, the cheapest tier-a win) is a sixth.
func Validators() []knowledge.Validator {
	return []knowledge.Validator{
		valuesValidator{}, crdValidator{}, compatValidator{}, imageValidator{},
		restatementValidator{}, newCanonical(),
	}
}

// verdict is one aspect conclusion before it becomes an AspectCheck.
type verdict struct {
	outcome domain.ValidationOutcome
	rule    string
	detail  string
}

func confirmed(rule, format string, args ...any) verdict {
	return verdict{domain.OutcomeConfirmed, rule, fmt.Sprintf(format, args...)}
}
func refuted(rule, format string, args ...any) verdict {
	return verdict{domain.OutcomeRefuted, rule, fmt.Sprintf(format, args...)}
}
func inconclusive(rule, format string, args ...any) verdict {
	return verdict{domain.OutcomeInconclusive, rule, fmt.Sprintf(format, args...)}
}

// evidenceSet collects the artifact evidence a result rests on.
type evidenceSet struct {
	seen map[domain.EvidenceID]bool
	list []domain.Evidence
}

func (s *evidenceSet) add(evs ...domain.Evidence) {
	if s.seen == nil {
		s.seen = map[domain.EvidenceID]bool{}
	}
	for _, e := range evs {
		if e.ID == "" || s.seen[e.ID] || e.Kind == domain.EvidenceLocalFile || e.Kind == domain.EvidenceInput {
			continue
		}
		s.seen[e.ID] = true
		s.list = append(s.list, e)
	}
}

// resolve returns the records of ids found in the pool.
func resolve(ids []domain.EvidenceID, pool ...[]domain.Evidence) []domain.Evidence {
	byID := map[domain.EvidenceID]domain.Evidence{}
	for _, p := range pool {
		for _, e := range p {
			if _, ok := byID[e.ID]; !ok {
				byID[e.ID] = e
			}
		}
	}
	var out []domain.Evidence
	for _, id := range ids {
		if e, ok := byID[id]; ok {
			out = append(out, e)
		}
	}
	return out
}

func releaseEvidence(r *domain.Release) []domain.Evidence {
	if r == nil {
		return nil
	}
	return r.Evidence
}

// build assembles a ValidationResult. Verdicts of aspects the assertion does
// not state are dropped; a result with no checks is not produced. A
// confirmation whose evidence could not be resolved is downgraded to
// inconclusive (a confirmation must cite what it rests on).
func build(in knowledge.ValidationInput, producer string, vs map[domain.Aspect]verdict, ev evidenceSet) []domain.ValidationResult {
	var checks []domain.AspectCheck
	for _, a := range domain.Aspects {
		v, ok := vs[a]
		if !ok || !in.Assertion.Has(a) {
			continue
		}
		if v.outcome == domain.OutcomeConfirmed && len(ev.list) == 0 {
			v = inconclusive(v.rule, "%s (artifact evidence could not be resolved, so the confirmation is withheld)", v.detail)
		}
		checks = append(checks, domain.AspectCheck{Aspect: a, Outcome: v.outcome, Rule: v.rule, Detail: v.detail})
	}
	if len(checks) == 0 {
		return nil
	}
	sort.SliceStable(ev.list, func(i, j int) bool { return ev.list[i].ID < ev.list[j].ID })
	return []domain.ValidationResult{{
		ID:          domain.ValidationID(in.Candidate.ID, producer, in.Assertion),
		CandidateID: in.Candidate.ID,
		ProposalID:  in.ProposalID,
		Validator:   producer,
		Assertion:   in.Assertion,
		Checks:      checks,
		Evidence:    ev.list,
		CheckedAt:   in.Now,
	}}
}

// applies reports whether the assertion states a subject and change of one of
// the families.
func applies(in knowledge.ValidationInput, families ...domain.SubjectFamily) bool {
	a := in.Assertion
	if a.Subject == nil || a.Change == nil {
		return false
	}
	for _, f := range families {
		if a.Subject.Family == f {
			return true
		}
	}
	return false
}

// canonJSON canonicalises a JSON-encoded scalar; text that is not valid JSON
// is taken as a bare string literal ("Always" → "\"Always\"").
func canonJSON(s string) string {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		b, _ := json.Marshal(strings.TrimSpace(s))
		return string(b)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return s
	}
	return string(b)
}

func ptr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

var _ = context.Background
