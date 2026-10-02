package impact

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/env"
)

// The product inventory travels into the report additively: absent without
// one, and — with a conflict between declared and detected versions — both
// entries are carried, the conflict is rendered, and every evidence id of the
// inventory resolves in the report's environment evidence.
func TestReportCarriesProductInventory(t *testing.T) {
	eb := newEdge()
	eb.change("values:removed", "Helm value `webhook.config` removed", "webhook.config")
	dir := t.TempDir()
	val := writeFile(t, dir, "values.yaml", "webhook:\n  config: true\n")

	plain := buildReport(t, eb.edge, loadEnv(t, env.Inputs{ValuesFiles: []string{val}, KubernetesVersion: "1.31"}))
	if len(plain.Environment.Products) != 0 || plain.Environment.ProductsHealth != "" {
		t.Errorf("no inventory supplied: report must not carry one, got %+v", plain.Environment)
	}

	inv := writeFile(t, dir, "inventory.yaml", "- product: ingress-nginx\n  version: v1.12.6\n")
	hints := []env.ProductHint{{ID: "ingress-nginx", Repositories: []string{"registry.k8s.io/ingress-nginx/controller"}}}
	rep := buildReport(t, eb.edge, loadEnv(t, env.Inputs{
		ValuesFiles: []string{val}, KubernetesVersion: "1.31", Inventory: inv, ProductHints: hints,
		Images: []string{"registry.k8s.io/ingress-nginx/controller:v1.12.1"},
	}))
	if rep.Environment.ProductsHealth != "partial" || len(rep.Environment.Products) != 3 {
		t.Fatalf("environment = %+v", rep.Environment)
	}
	resolves := map[string]bool{}
	for _, e := range rep.EnvironmentEvidence {
		resolves[string(e.ID)] = true
	}
	conflicts := 0
	for _, p := range rep.Environment.Products {
		if p.Conflict != "" {
			conflicts++
		}
		for _, id := range p.Evidence {
			if !resolves[string(id)] {
				t.Errorf("%s/%s: evidence %s does not resolve in environmentEvidence", p.Product, p.Source, id)
			}
		}
	}
	if conflicts != 2 {
		t.Errorf("both conflicting entries must be flagged, got %d", conflicts)
	}
	var buf bytes.Buffer
	if err := RenderText(&buf, rep, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"products: 3 inventory entries (partial)",
		"ingress-nginx 1.12.6 [declared] CONFLICT with detected-image 1.12.1",
		"ingress-nginx 1.12.1 [detected-image] CONFLICT with declared 1.12.6",
		"kubernetes 1.31 [declared]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
