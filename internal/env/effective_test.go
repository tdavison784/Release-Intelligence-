package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A base two overlays consume, one rendered and one not: the rendered
// overlay's objects are in the view, and so is the base's declaration,
// marked unrendered — the other overlay deploys a form of it nobody has
// seen. Files no render consumed are read as declared; the input
// environment is not modified.
func TestEffectiveSharedBase(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) string {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	base := write("base/w.yaml", "apiVersion: gizmo.example.io/v1\nkind: Widget\nmetadata:\n  name: w\nspec:\n  size: 2\n")
	write("other/g.yaml", "apiVersion: gizmo.example.io/v1\nkind: Gadget\nmetadata:\n  name: g\n")
	e, err := Load(Inputs{Manifests: []string{dir}})
	if err != nil {
		t.Fatal(err)
	}
	n := len(e.Resources)
	rendered := "apiVersion: gizmo.example.io/v1\nkind: Widget\nmetadata:\n  name: a-w\nspec:\n  size: 2\n"
	eff := Effective(e, []RenderedObjects{
		{Target: "kustomize:a", Consumed: map[string]int{base: 3}, Docs: []byte(rendered), Digest: "sha256:a"},
		{Target: "kustomize:b", Consumed: map[string]int{base: 3}, Unavailable: "render failed"},
	})
	var got []string
	for _, r := range eff.Resources {
		state := "declared"
		switch {
		case r.Unrendered != "":
			state = "unrendered"
		case r.Render != "":
			state = "rendered:" + r.Render
		}
		got = append(got, r.Kind+"/"+r.Name+" "+state)
	}
	want := "Widget/w unrendered,Gadget/g declared,Widget/a-w rendered:kustomize:a"
	if strings.Join(got, ",") != want {
		t.Fatalf("view %v, want %s", got, want)
	}
	if len(e.Resources) != n || e.EffectiveView || len(e.Evidence) == len(eff.Evidence) {
		t.Fatalf("the input changed, or the rendered records were not pooled")
	}
	// the rendered size cites the base's line; the name, which kustomize
	// rewrote, cites the render
	for _, r := range eff.Resources {
		if r.Render == "" {
			continue
		}
		for _, f := range r.Fields {
			if f.Path == "spec.size" && f.Line != 6 {
				t.Errorf("spec.size cites line %d, want the base's 6", f.Line)
			}
		}
		if res := eff.FieldValues(GVKSelector{Group: "gizmo.example.io", Kind: "Widget"}, "spec.size"); len(res) != 2 {
			t.Errorf("indexes not rebuilt: %+v", res)
		}
	}
}
