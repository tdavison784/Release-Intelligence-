package impact

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/env"
)

// Parse failures and truncation of the environment inputs must never be
// silently dropped: they are warnings on the Environment, the report copies
// them, and the rendered output shows them. They also never license a
// "not affected"-style conclusion — the per-dimension health statuses
// (env.Environment.Health/Statuses) let the join and the classifier tell
// "absent" from "supplied and healthy" from "supplied with warnings".
func TestEnvParseWarningsSurfaceInReport(t *testing.T) {
	dir := t.TempDir()
	man := writeFile(t, dir, "broken.yaml", "a: [unclosed\n")

	eb := newEdge()
	eb.change("values:removed", "removed a value", "replicaCount")
	rep := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Manifests: []string{man}}))

	if len(rep.Warnings) == 0 {
		t.Fatalf("env warnings must reach the report: %+v", rep.Warnings)
	}
	if !strings.Contains(strings.Join(rep.Warnings, " "), "not parsed") {
		t.Errorf("warnings = %v", rep.Warnings)
	}
	// the degraded dimension is explicit on the environment summary side of
	// the parsed environment
	if got := loadEnv(t, env.Inputs{Manifests: []string{man}}).Health(env.DimManifests); got != env.HealthPartial {
		t.Errorf("manifests health = %q, want partial", got)
	}

	var out bytes.Buffer
	if err := RenderText(&out, rep, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Warnings (") {
		t.Errorf("rendered output must show the warnings:\n%s", out.String())
	}
}

// An absent dimension must not read as a healthy one: the statuses an
// enrichment/classification layer can consult distinguish the two.
func TestEnvStatusesConsultable(t *testing.T) {
	e := loadEnv(t, env.Inputs{KubernetesVersion: "1.31"})
	var found bool
	for _, s := range e.Statuses() {
		if s.Dimension == env.DimKubernetes && s.Health == env.HealthOK {
			found = true
		}
		if s.Dimension == env.DimValues && s.Health != env.HealthAbsent {
			t.Errorf("unsupplied dimension reported %q, want absent", s.Health)
		}
	}
	if !found {
		t.Errorf("kubernetes status missing from %+v", e.Statuses())
	}
}
