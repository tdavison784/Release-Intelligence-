package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// Lane join5 (D1), through a real kustomize render of a repository: the
// view's objects are the overlay's output, each field citing the file that
// produced it (the overlay patch for the patched field), and the files the
// render consumed are never read on their own.
func TestEffectiveObjectsOfARealOverlay(t *testing.T) {
	if _, err := exec.LookPath("kubectl"); err != nil {
		if _, err := exec.LookPath("kustomize"); err != nil {
			t.Skip("needs kustomize or kubectl (kubectl kustomize)")
		}
	}
	repo := t.TempDir()
	write := func(rel, body string) string {
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	base := write("apps/web/base/widget.yaml", "apiVersion: gizmo.example.io/v1\nkind: Widget\nmetadata:\n  name: w\nspec:\n  size: 2\n")
	write("apps/web/base/kustomization.yaml", "resources:\n- widget.yaml\n")
	patch := write("apps/web/overlays/prod/rotation.yaml", "apiVersion: gizmo.example.io/v1\nkind: Widget\nmetadata:\n  name: w\nspec:\n  rotation: Never\n")
	write("apps/web/overlays/prod/kustomization.yaml", "namespace: web\nnamePrefix: prod-\nresources:\n- ../../base\npatches:\n- path: rotation.yaml\n")

	e, err := env.Load(env.Inputs{Repo: repo})
	if err != nil {
		t.Fatal(err)
	}
	eng := &render.Engine{Kustomize: render.NewKustomize(render.NewCache(t.TempDir()))}
	dir := filepath.Join(repo, "apps/web/overlays/prod")
	p := eng.RenderPair(context.Background(), render.PairRequest{Product: "gizmo", From: "1.0.0", To: "1.1.0",
		Target: render.Target{ID: "kustomize:apps/web/overlays/prod", Kind: render.TargetKustomize, ValuesComplete: true,
			Kustomize: &render.KustomizeInput{Dir: dir, Root: repo}}})
	// the overlay does not pin the product: a version-independent overlay,
	// whose render is the same before and after
	if p.Status != render.PairNotApplicable || !p.FromResult.OK() {
		t.Fatalf("pair %s %+v", p.Status, p.Failure)
	}
	eff := env.Effective(e, []env.RenderedObjects{renderedObjectsOf(p)})

	unset := domain.Condition{Op: domain.OpResource, Group: "gizmo.example.io", Kind: "Widget",
		Of: []domain.Condition{{Op: domain.OpField, Path: "spec.rotation", State: domain.StateUnset}}}
	if r := impact.EvaluateCondition(unset, e, nil); r.Value != impact.True {
		t.Fatalf("files: %s, want the base file's (wrong) true", r.Value)
	}
	if r := impact.EvaluateCondition(unset, eff, nil); r.Value != impact.False {
		t.Fatalf("rendered: %s %v, want false", r.Value, r.Needed)
	}
	var widgets []*env.Resource
	for i := range eff.Resources {
		if eff.Resources[i].Kind == "Widget" {
			widgets = append(widgets, &eff.Resources[i])
		}
	}
	if len(widgets) != 1 || widgets[0].Name != "prod-w" || widgets[0].Namespace != "web" || widgets[0].Render == "" {
		t.Fatalf("view widgets %+v", widgets)
	}
	ix := map[domain.EvidenceID]domain.Evidence{}
	for _, x := range eff.Evidence {
		ix[x.ID] = x
	}
	cites := map[string]string{}
	for _, f := range widgets[0].Fields {
		if len(f.Evidence) > 0 {
			cites[f.Path] = ix[f.Evidence[0]].URI
		}
	}
	if !sameFile(cites["spec.rotation"], patch) || !sameFile(cites["spec.size"], base) {
		t.Errorf("cited %v, want the patch for rotation and the base for size", cites)
	}
	// transformer output (the namespace) is cited from the render itself
	if !strings.HasPrefix(cites["metadata.namespace"], "render:") {
		t.Errorf("metadata.namespace cited from %q, want the render", cites["metadata.namespace"])
	}
}

func sameFile(a, b string) bool {
	x, _ := filepath.Abs(a)
	y, _ := filepath.Abs(b)
	return x == y
}
