package impact

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

const releaseApp = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: NAME
spec:
  sources:
    - repoURL: https://charts.example.com
      chart: example
      targetRevision: 1.0.0
      helm:
        valueFiles:
          - $values/FILE
    - repoURL: https://git.example.com/gitops.git
      ref: values
`

func releaseRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, c := range files {
		writeFile(t, dir, p, c)
	}
	return dir
}

func app(name, file string) string {
	return strings.NewReplacer("NAME", name, "FILE", file).Replace(releaseApp)
}

// No values key matches, but the product's values were only partially
// collected (here: a valueFiles entry that is not in the repository): the
// key may be set where nothing could be read — unknown, never not-affected.
func TestNoMatchWithPartialValuesIsUnknown(t *testing.T) {
	eb := newEdge()
	eb.change("values:removed", "Helm value `webhook.config` removed", "webhook.config")
	repo := releaseRepo(t, map[string]string{"apps/a.yaml": app("example", "missing.yaml")})
	en := loadEnv(t, env.Inputs{Repo: repo, Product: "example"})
	if en.Health(env.DimValues) != env.HealthPartial {
		t.Fatalf("health = %s", en.Health(env.DimValues))
	}
	r := buildReport(t, eb.edge, en)
	if fs := findingsByRule(r, RuleValuesUnset); len(fs) != 0 {
		t.Fatalf("partial values must not clear: %+v", fs)
	}
	fs := findingsByClass(r, domain.ImpactUnknown)
	if len(fs) != 1 || !strings.Contains(strings.Join(fs[0].NeededToDetermine, " "), "is not present in the repository") {
		t.Fatalf("unknown = %+v", fs)
	}
}

// A pin in one release never speaks for another: with two releases of the
// product, "pinned" needs the key set in both.
func TestPinnedNeedsEveryRelease(t *testing.T) {
	eb := newEdge()
	eb.change("values:default-changed", "Default of `replicaCount` changed: 1 → 3", "replicaCount")
	both := releaseRepo(t, map[string]string{
		"dev/app.yaml": app("example-dev", "dev/values.yaml"), "dev/values.yaml": "replicaCount: 1\n",
		"prod/app.yaml": app("example-prod", "prod/values.yaml"), "prod/values.yaml": "replicaCount: 5\n",
	})
	r := buildReport(t, eb.edge, loadEnv(t, env.Inputs{Repo: both, Product: "example"}))
	if fs := findingsByRule(r, RuleValuesPinned); len(fs) != 1 {
		t.Fatalf("pinned in every release: %+v", r.Findings)
	}
	one := releaseRepo(t, map[string]string{
		"dev/app.yaml": app("example-dev", "dev/values.yaml"), "dev/values.yaml": "other: 1\n",
		"prod/app.yaml": app("example-prod", "prod/values.yaml"), "prod/values.yaml": "replicaCount: 5\n",
	})
	r = buildReport(t, eb.edge, loadEnv(t, env.Inputs{Repo: one, Product: "example"}))
	if fs := findingsByRule(r, RuleValuesPinned); len(fs) != 0 {
		t.Fatalf("a pin in prod must not speak for dev: %+v", fs)
	}
	fs := findingsByClass(r, domain.ImpactUnknown)
	if len(fs) != 1 || !strings.Contains(fs[0].Title, "only some releases of the product pin") {
		t.Fatalf("unknown = %+v", r.Findings)
	}
}
