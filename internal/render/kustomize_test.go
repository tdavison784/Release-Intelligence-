package render

import (
	"context"
	"path/filepath"
	"testing"
)

func TestKustomizeInputsAndSubstitutionPlan(t *testing.T) {
	root, _ := filepath.Abs("testdata/kustomize")
	files, remote, unsupported, err := KustomizeInputs(root, filepath.Join(root, "overlays/prod"))
	if err != nil || unsupported != "" || len(remote) != 0 {
		t.Fatalf("inputs: %v %q %v", err, unsupported, remote)
	}
	want := []string{"base/deployment.yaml", "base/kustomization.yaml", "overlays/prod/kustomization.yaml", "overlays/prod/replicas.yaml"}
	if len(files) != len(want) {
		t.Fatalf("files = %v", files)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Fatalf("files = %v", files)
		}
	}
	subs, err := PlanVersionSubstitution(root, files, "1.0.0", "1.1.0")
	if err != nil || len(subs) != 1 || subs[0].From != "v1.0.0" || subs[0].To != "v1.1.0" {
		t.Fatalf("plan = %+v %v", subs, err)
	}
	if s, ok := replaceVersion("https://github.com/x/y/releases/download/v1.0.0/install.yaml", "1.0.0", "1.1.0"); !ok || s != "https://github.com/x/y/releases/download/v1.1.0/install.yaml" {
		t.Errorf("remote ref: %s", s)
	}
	if _, ok := replaceVersion("v1.0.01", "1.0.0", "1.1.0"); ok {
		t.Error("partial version token replaced")
	}
}

func TestKustomizePairLive(t *testing.T) {
	needTool(t, "kubectl")
	root, _ := filepath.Abs("testdata/kustomize")
	e := testEngine(t)
	p := e.RenderPair(context.Background(), PairRequest{Product: "demo", From: "1.0.0", To: "1.1.0",
		Target: Target{ID: "k", Kind: TargetKustomize, ValuesComplete: true, Kustomize: &KustomizeInput{Dir: filepath.Join(root, "overlays/prod"), Root: root}}})
	if p.Status != PairOK {
		t.Fatalf("pair: %s %+v", p.Status, p.Failure)
	}
	if len(p.Diff.Changes) != 1 || p.Diff.Changes[0].Class != ImageChanged {
		for _, c := range p.Diff.Changes {
			t.Logf("%s", c.Summary(true))
		}
		t.Fatal("want exactly the image change")
	}
	if p.ToResult.Provenance.KustomizeDir != "overlays/prod" || len(p.ToResult.Provenance.Substitutions) != 1 || len(p.ToResult.Provenance.Inputs) != 4 {
		t.Errorf("provenance: %+v", p.ToResult.Provenance)
	}
	// an overlay that does not pin the version cannot produce a target render
	u := e.RenderPair(context.Background(), PairRequest{Product: "demo", From: "1.0.0", To: "1.1.0",
		Target: Target{ID: "u", Kind: TargetKustomize, ValuesComplete: true, Kustomize: &KustomizeInput{Dir: filepath.Join(root, "overlays/unpinned"), Root: root}}})
	if u.Status != PairNotApplicable || u.Diff != nil || u.Complete() {
		t.Fatalf("unpinned overlay: %s %+v", u.Status, u.Failure)
	}
}
