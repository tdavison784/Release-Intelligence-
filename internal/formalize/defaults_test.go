package formalize

import (
	"context"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/applicability"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// defaultAnswer settles the fixture's default uncertainty (spec.other, left
// to its default) with an evidenced default on the rule's value leaf,
// citing the upstream schema text the assessment established; the record
// keeps every other uncertainty.
func defaultAnswer(p *Precomputed, cites ...domain.EvidenceID) *Answer {
	a := validAnswer(p)
	var def string
	var keeps []string
	for _, id := range a.Hypotheses[1].Keeps {
		if u, _ := p.uncertainty(id); u.Kind == UncertaintyDefault {
			def = id
			continue
		}
		keeps = append(keeps, id)
	}
	a.Hypotheses[1].Keeps = keeps
	if len(cites) == 0 {
		cites = []domain.EvidenceID{"ev-default"}
	}
	ex := &a.Hypotheses[0].Rule.Assertion.Applicability.Exposure
	ex.Of = append(ex.Of, domain.Condition{Op: domain.OpField, Path: "spec.other", State: domain.StateEquals, Values: []string{`"off"`},
		Default: &domain.FieldDefault{Value: `"off"`, Source: domain.DefaultCRDSchema, Citations: cites}})
	a.Hypotheses[0].Discharges = []string{def}
	return a
}

func TestEvidencedDefault(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	p := precompute(t, f.input())
	if v := Floor(ctx, p, defaultAnswer(p)); len(v) != 0 {
		t.Fatalf("the evidenced default is rejected: %v", v)
	}
	cases := []struct {
		name string
		a    *Answer
		want string
	}{
		{"a default citing evidence the assessment never established", defaultAnswer(p, "ev-foreign"), "is not evidence the assessment established"},
		{"a default citing invented evidence", defaultAnswer(p, "ev-invented"), "is not evidence the assessment established"},
		{"a default citing the environment", defaultAnswer(p, "ev-env"), "is not upstream text"},
		{"a discharge without a default", func() *Answer {
			a := defaultAnswer(p)
			ex := &a.Hypotheses[0].Rule.Assertion.Applicability.Exposure
			ex.Of = ex.Of[:len(ex.Of)-1]
			return a
		}(), "states no default"},
		{"a discharge of a non-default uncertainty", func() *Answer {
			a := defaultAnswer(p)
			a.Hypotheses[0].Discharges = append(a.Hypotheses[0].Discharges, p.Uncertainties[0].ID)
			return a
		}(), "not a default uncertainty id"},
		{"a default neither stated nor kept", func() *Answer {
			a := defaultAnswer(p)
			a.Hypotheses[0].Discharges = nil
			return a
		}(), "is dropped"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := Floor(ctx, p, c.a)
			if !strings.Contains(strings.Join(v, "\n"), c.want) {
				t.Errorf("violations %q do not contain %q", v, c.want)
			}
		})
	}

	// the accepted rule carries its default's evidence; the settled default is
	// no longer an open uncertainty of the rule; the evaluators accept it
	r, err := (&Formalizer{Client: &scripted{texts: []string{answerJSON(t, defaultAnswer(p))}}}).Formalize(ctx, f.input())
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusComplete {
		t.Fatalf("record %s: %+v", r.Status, r.Failure)
	}
	var rule *domain.Hypothesis
	for i := range r.Hypotheses {
		if r.Hypotheses[i].Mitigation != nil {
			rule = &r.Hypotheses[i]
		}
	}
	if rule == nil {
		t.Fatal("no rule")
	}
	pool := map[domain.EvidenceID]domain.Evidence{}
	for _, id := range rule.Citations {
		pool[id] = p.pool[id]
	}
	if _, ok := pool["ev-default"]; !ok {
		t.Errorf("the default's evidence is not among the hypothesis' citations: %v", rule.Citations)
	}
	leaf := rule.Mitigation.ProposedRule.Assertion.Applicability.Exposure.Of[1]
	if d := applicability.RuleDefault(leaf, pool); d.State != applicability.Known || d.Value != `"off"` {
		t.Errorf("the evaluator does not accept the default: %+v", d)
	}
	for _, u := range rule.ProposedVerification.Uncertainties {
		if strings.Contains(u, "spec.other") {
			t.Errorf("the settled default is still an open uncertainty: %q", u)
		}
	}
	if !strings.Contains(rule.Mitigation.RequestedVerification, "evidenced defaults") {
		t.Errorf("requested verification = %q", rule.Mitigation.RequestedVerification)
	}
}
