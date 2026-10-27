package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// CheckMitigation is the session's spec check lifted out as a pure
// function (Milestone 4, lane formalize): it reads nothing but its env, so
// the formalization stage applies the same checks to a stored run.
func TestCheckMitigationReadsOnlyItsEnv(t *testing.T) {
	str := func(s string) *string { return &s }
	rule := domain.ProposedRule{Release: domain.ReleaseBounds{From: "1.0.0", To: "1.1.0"}, Assertion: domain.SemanticAssertion{
		Subject:       &domain.Subject{Family: domain.SubjectHelmValue, Product: "acme", Path: "controller.mode"},
		Change:        &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(`"a"`), After: str(`"b"`)},
		Applicability: &domain.Applicability{Exposure: domain.Condition{Op: domain.OpValuesKey, Path: "controller.mode", State: domain.StateUnset}},
		Consequence:   &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired},
		Statement:     "x"}}
	up := domain.ArtifactRef{ID: "art-up", Origin: domain.OriginUpstream}
	env := MitigationEnv{Product: "acme", From: "v1.0.0", To: "v1.1.0", Artifacts: []domain.ArtifactRef{up},
		Read: func(context.Context, domain.ArtifactRef) ([]byte, error) {
			return []byte("controller:\n  mode: a\n"), nil
		},
		Artifact: func(id string) (domain.ArtifactRef, bool) { return up, id == up.ID },
		Basis: func(_ context.Context, item, ref string) string {
			if ref == "clm-1" && item == "ci-1" {
				return ""
			}
			return "unknown"
		}}
	in := investigate.MitigationInput{ProposedRule: rule, EnvironmentBasis: []string{" clm-1 ", ""}, RequestedVerification: " verify "}
	cites := []domain.Evidence{{ID: "ev-1", SourceID: "art-up"}}
	spec, why := CheckMitigation(context.Background(), env, "ci-1", in, cites)
	if why != "" || spec == nil || len(spec.EnvironmentBasis) != 1 || spec.RequestedVerification != "verify" {
		t.Fatalf("spec %+v, why %q", spec, why)
	}
	cases := []struct {
		name string
		edit func(e *MitigationEnv, in *investigate.MitigationInput, cites *[]domain.Evidence)
		want string
	}{
		{"other product", func(e *MitigationEnv, _ *investigate.MitigationInput, _ *[]domain.Evidence) { e.Product = "other" }, "not this investigation's product"},
		{"subject absent", func(e *MitigationEnv, _ *investigate.MitigationInput, _ *[]domain.Evidence) {
			e.Read = func(context.Context, domain.ArtifactRef) ([]byte, error) { return []byte("nothing"), nil }
		}, "does not occur in the bundle"},
		{"bounds beyond the run", func(e *MitigationEnv, _ *investigate.MitigationInput, _ *[]domain.Evidence) { e.To = "v1.0.5" }, "not within this run's transition"},
		{"basis unresolved", func(_ *MitigationEnv, in *investigate.MitigationInput, _ *[]domain.Evidence) {
			in.EnvironmentBasis = []string{"clm-2"}
		}, `environmentBasis "clm-2": unknown`},
		{"no upstream citation", func(_ *MitigationEnv, _ *investigate.MitigationInput, c *[]domain.Evidence) {
			*c = []domain.Evidence{{ID: "ev-2", SourceID: "art-env"}}
		}, "no citation is upstream text"},
	}
	for _, c := range cases {
		e, i, cs := env, in, cites
		c.edit(&e, &i, &cs)
		if spec, why := CheckMitigation(context.Background(), e, "ci-1", i, cs); spec != nil || !strings.Contains(why, c.want) {
			t.Errorf("%s: spec %v, why %q", c.name, spec != nil, why)
		}
	}
}
