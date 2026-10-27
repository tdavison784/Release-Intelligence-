package main

// REAL scope-refinement exercises on kind (lane verify2, D-0025): the
// declared cert-manager fixture (products/exercises/fixtures/cert-manager.yaml,
// consumer ca-trust with its role-absent observation), the real controller
// rendered from its chart at FROM and TO, throwaway clusters ri-verify-*.
// The rules are synthetic and generic in shape (a field default change of the
// subject kind, narrowed to the catalog's role); no evaluation case is read.
// Opt-in (each run creates one cluster and takes minutes; it pulls images):
//
//	RI_VERIFY2_KIND=1 RI_VERIFY2_STATE=<scratch clone of a warm state dir> \
//	  go test ./cmd/ri -run RefinementKind -v -timeout 60m
//
// RI_VERIFY2_CASES selects runs (comma list of confirmed, refuted,
// inconclusive; default all). It skips with a message when kind, kubectl,
// docker or the docker daemon is missing. Every cluster is deleted.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	rtval "github.com/tdavison784/release-intelligence/internal/investigate/runtime"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/investigate/verify"
)

func TestRefinementKind(t *testing.T) {
	if os.Getenv("RI_VERIFY2_KIND") == "" {
		t.Skip("opt-in: RI_VERIFY2_KIND=1 RI_VERIFY2_STATE=<scratch state dir> (creates kind clusters)")
	}
	for _, bin := range []string{"kind", "kubectl", "docker"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is not on PATH", bin)
		}
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("the docker daemon is not running")
	}
	state := os.Getenv("RI_VERIFY2_STATE")
	if state == "" {
		t.Fatal("RI_VERIFY2_STATE (a scratch state directory; never the shared one) is required")
	}
	products, _ := filepath.Abs("../../products")
	a, err := app.New(app.Config{ProductsDir: products, StateDir: state, Logf: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.New(state)
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := verify.LoadFixtures(filepath.Join(products, "exercises", "fixtures"))
	if err != nil {
		t.Fatal(err)
	}
	sets, err := catalog.LoadExercises(filepath.Join(products, "exercises"))
	if err != nil {
		t.Fatal(err)
	}
	exercises := rtval.FromSets(sets...)
	runner := rtval.New(rtval.WithStore(st), rtval.WithExercises(exercises...), rtval.WithTimeout(25*time.Minute), rtval.WithLogf(t.Logf))

	certs := func(of ...domain.Condition) domain.Condition {
		return domain.Condition{Op: domain.OpResource, Group: "cert-manager.io", Kind: "Certificate", Of: of}
	}
	leaf := func(path string, state domain.FieldState, values ...string) domain.Condition {
		return domain.Condition{Op: domain.OpField, Path: path, State: state, Values: values}
	}
	isCAEvidence := domain.NewEvidence(domain.EvidenceDocument, "types_certificate.go",
		"https://github.com/cert-manager/cert-manager/blob/v1.18.0/pkg/apis/certmanager/v1/types_certificate.go", "L258",
		"IsCA bool `json:\"isCA,omitempty\"`", "sha256:test", time.Time{})
	def := &domain.FieldDefault{Value: "false", Source: domain.DefaultController, Citations: []domain.EvidenceID{isCAEvidence.ID}}
	// rule narrows a rotation-policy default change to the CA role; shield
	// is the value the base rule claims preserves the previous behaviour
	rule := func(shield string) domain.SemanticAssertion {
		before, after := `"Never"`, `"Always"`
		role := func(v string) domain.Condition {
			c := leaf("spec.isCA", domain.StateEquals, v)
			d := *def
			c.Default = &d
			return c
		}
		return domain.SemanticAssertion{
			Subject: &domain.Subject{Family: domain.SubjectCRDField, Product: "cert-manager", Group: "cert-manager.io", Kind: "Certificate", Path: "spec.privateKey.rotationPolicy"},
			Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: &before, After: &after},
			Applicability: &domain.Applicability{
				// the shape formalize.RefineRule writes: one object scope each
				Exposure: certs(leaf("spec.privateKey.rotationPolicy", domain.StateUnset), role("true")),
				Overlap: &domain.Condition{Op: domain.OpResource, Group: "cert-manager.io", Kind: "Certificate", Of: []domain.Condition{{Op: domain.OpAny, Of: []domain.Condition{
					{Op: domain.OpAll, Of: []domain.Condition{leaf("spec.privateKey.rotationPolicy", domain.StateEquals, shield)}},
					{Op: domain.OpAll, Of: []domain.Condition{role("false")}}}}}}},
			Consequence: &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired},
		}
	}
	ref := &domain.ScopeRefinement{Refines: "hyp-000000000000", BaseRule: "kind-test", Consumer: "ca-trust", TrustKey: "ca.crt",
		Role: []domain.RoleField{{Path: "spec.isCA", Value: "true"}}, Outside: []domain.RoleField{{Path: "spec.isCA", Value: "false"}},
		Trace: []domain.TraceRead{{Graph: "art-test", Edge: "ge-test", Producer: "cert-manager.io/v1/Certificate/ns/c", Secret: "core/v1/Secret/ns/c-tls", Key: "ca.crt", Workload: "apps/v1/Deployment/ns/w"}}}

	cases := map[string]struct {
		rule     domain.SemanticAssertion
		from, to string
		want     domain.MethodOutcome
	}{
		// harm in the role only: a CA re-issued with a new key breaks its
		// consumer at TO; a non-CA certificate's consumer verifies through the CA
		"confirmed": {rule(`"Never"`), "1.17.2", "1.18.0", domain.MethodPass},
		// a refinement of a wrong behaviour rule (Always declared shielding):
		// the shielded CA's consumer breaks
		"refuted": {rule(`"Always"`), "1.17.2", "1.18.0", domain.MethodFail},
		// no default change between the versions: the change is not shown
		"inconclusive": {rule(`"Never"`), "1.17.1", "1.17.2", domain.MethodInconclusive},
	}
	only := map[string]bool{}
	for _, c := range strings.Split(os.Getenv("RI_VERIFY2_CASES"), ",") {
		if c = strings.TrimSpace(c); c != "" {
			only[c] = true
		}
	}
	for _, name := range []string{"confirmed", "refuted", "inconclusive"} {
		tc := cases[name]
		if len(only) > 0 && !only[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			m, lims := verify.ExerciseRule(context.Background(), verify.ExerciseInput{Product: "cert-manager", Rule: tc.rule,
				Bounds: &domain.ProofBounds{From: tc.from, To: tc.to}, Fixtures: fixtures["cert-manager"], Exercises: exercises,
				Relationships: catalogRelationships(a, "cert-manager"), Renderer: controllerRenderer{e: a.RenderEngine()}, Runner: runner,
				Evidence: []domain.Evidence{isCAEvidence}, Refinement: ref})
			t.Logf("%s: %s", m.Outcome, m.Detail)
			for _, c := range m.Checks {
				t.Logf("  %s: %s %s", c.Subject, c.Outcome, c.Detail)
			}
			for _, l := range lims {
				t.Logf("  limitation: %s", l)
			}
			if m.Outcome != tc.want {
				t.Errorf("outcome %s, want %s", m.Outcome, tc.want)
			}
		})
	}
	out, _ := exec.Command("kind", "get", "clusters").Output()
	for _, c := range strings.Fields(string(out)) {
		if strings.HasPrefix(c, rtval.FixtureClusterPrefix) {
			t.Errorf("cluster %s was not deleted", c)
		}
	}
}

func catalogRelationships(a *app.App, product string) []catalog.Relationship {
	if def, ok := a.Catalog.Get(product); ok {
		return def.Relationships
	}
	return nil
}
