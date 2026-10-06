package render

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// crdgPairFor renders the crdg chart pair (CRDs behind the crds.enabled
// gate) with a named customer values file.
func crdgPairFor(t *testing.T, file string) *Pair {
	t.Helper()
	needTool(t, "helm")
	e := testEngine(t)
	content, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	tgt := Target{ID: "values:" + file, Kind: TargetValuesFiles, Origin: "testdata/" + file, Why: "test",
		ReleaseName: "crdg", Namespace: "crdg", ValuesComplete: true,
		Values: []ValuesLayer{{Origin: "testdata/" + file, Content: content}}}
	p := e.RenderPair(context.Background(), PairRequest{Product: "crdg", From: "1.0.0", To: "1.1.0", Target: tgt, Scope: domain.RenderEnvironment, KubeVersion: "1.31"})
	if p.Status != PairOK {
		t.Fatalf("pair: %s %+v", p.Status, p.Failure)
	}
	return p
}

// PO-7a addendum 6/7: a gated chart's CRDs reach the environment input only
// when the customer's own values open the gate; a render without CRD
// documents yields nothing (a gated-off CRD path says nothing).
func TestRenderedCRDsOfGatedChart(t *testing.T) {
	off := RenderedCRDsOf([]*Pair{crdgPairFor(t, "crdg-customer-off.yaml")})
	if len(off) != 0 {
		t.Fatalf("gate closed: %+v", off)
	}
	on := RenderedCRDsOf([]*Pair{crdgPairFor(t, "crdg-customer-on.yaml")})
	if len(on) != 1 || on[0].Empty() {
		t.Fatalf("gate open: %+v", on)
	}
	src := on[0]
	if !strings.Contains(string(src.Docs), "certificates.example.io") || !strings.Contains(string(src.Docs), "v1alpha2") {
		t.Errorf("the CRD document must carry name and versions: %s", src.Docs)
	}
	if src.Label != "chart crdg@1.0.0" || src.Tool != "helm" {
		t.Errorf("provenance label/tool: %q %q", src.Label, src.Tool)
	}
	if src.ChartDigest == "" || src.ValuesDigest == "" {
		t.Errorf("the render's digests are provenance: %q %q", src.ChartDigest, src.ValuesDigest)
	}
	if strings.Contains(string(src.Docs), "Deployment") {
		t.Errorf("only CustomResourceDefinition documents are extracted: %s", src.Docs)
	}
}

func crdObject(name, group, kind, version string) Object {
	return Object{ID: ObjectID{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition", Name: name},
		Body: map[string]any{
			"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
			"metadata": map[string]any{"name": name},
			"spec": map[string]any{
				"group":    group,
				"names":    map[string]any{"kind": kind, "plural": strings.SplitN(name, ".", 2)[0]},
				"versions": []any{map[string]any{"name": version, "served": true, "storage": true}},
			},
		}}
}

func envPair(label string, objs ...Object) *Pair {
	return &Pair{Scope: domain.RenderEnvironment, Status: PairOK, Diff: &DiffResult{},
		Target: Target{ID: label, ValuesComplete: true},
		FromResult: &Result{Status: StatusSucceeded, Objects: objs,
			Provenance: Provenance{Tool: ToolHelm, Chart: "c", ChartVersion: "1.0.0", ArtifactDigest: "sha256:art", ValuesDigest: "sha256:vals"}}}
}

// Scope, health and dedup rules of the extraction: release-scope pairs and
// failed FROM renders never contribute; a CRD rendered by several pairs is
// kept once, from the first pair.
func TestRenderedCRDsOfSelection(t *testing.T) {
	cert := crdObject("certificates.example.io", "example.io", "Certificate", "v1")
	issuer := crdObject("issuers.example.io", "example.io", "Issuer", "v1")
	// release scope: the chart-default pair is not the customer's install
	release := envPair("chart-default", cert)
	release.Scope = domain.RenderRelease
	// failed FROM render: an evidence gap, never "no CRDs"
	failed := envPair("values:failing", cert)
	failed.FromResult = &Result{Status: StatusFailed, Provenance: failed.FromResult.Provenance}
	// kustomize pair: labeled by its kustomization, not a chart; its copy of
	// the first pair's CRD is a duplicate
	kust := envPair("overlay", cert, issuer)
	kust.FromResult.Provenance = Provenance{Tool: ToolKustomize, KustomizeDir: "overlays/prod", ArtifactDigest: "sha256:tree"}

	got := RenderedCRDsOf([]*Pair{release, failed, envPair("values:a", cert), kust})
	if len(got) != 2 {
		t.Fatalf("two sources (helm then kustomize), got %d", len(got))
	}
	if got[0].Label != "chart c@1.0.0" || strings.Count(string(got[0].Docs), "certificates.example.io") != 1 ||
		strings.Contains(string(got[0].Docs), "issuers.example.io") {
		t.Errorf("the first environment pair carries its CRD once: %s (%s)", got[0].Docs, got[0].Label)
	}
	if got[1].Label != "kustomize overlays/prod" || got[1].Tool != "kustomize" ||
		strings.Contains(string(got[1].Docs), "certificates.example.io") || !strings.Contains(string(got[1].Docs), "issuers.example.io") {
		t.Errorf("the kustomize source carries only what the first pair did not: %s (%s)", got[1].Docs, got[1].Label)
	}
}

// The FROM→TO rendered CRD diff honors the customer's gates (addendum 6):
// the version the target drops appears in their rendered delta only when
// their values open the gate — with it closed, the CRD change does not
// reach their install and the delta says nothing about it.
func TestRenderedCRDDeltaHonorsGates(t *testing.T) {
	crdChanges := func(p *Pair) []Change {
		var out []Change
		for _, ch := range p.Diff.Changes {
			if ch.Object.Kind == "CustomResourceDefinition" {
				out = append(out, ch)
			}
		}
		return out
	}
	on := crdgPairFor(t, "crdg-customer-on.yaml")
	changes := crdChanges(on)
	if len(changes) == 0 {
		t.Fatal("gate open: the dropped v1alpha2 version must appear in the rendered CRD delta")
	}
	found := false
	for _, ch := range changes {
		if (ch.Class == FieldRemoved || ch.Class == FieldChanged || ch.Class == ResourceRemoved) &&
			(strings.Contains(ch.Path, "versions") || ch.Name != "") {
			found = true
		}
	}
	if !found {
		t.Errorf("the delta must carry the version change: %+v", changes)
	}
	if off := crdgPairFor(t, "crdg-customer-off.yaml"); len(crdChanges(off)) != 0 {
		t.Errorf("gate closed: the CRD change must not reach their render: %+v", crdChanges(off))
	}
}
