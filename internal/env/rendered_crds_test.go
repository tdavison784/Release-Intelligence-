package env

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// renderedCRDYAML is one rendered CustomResourceDefinition document, as
// RenderedCRDsOf would extract it from a FROM render.
const renderedCRDYAML = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: certificates.example.io
spec:
  group: example.io
  names:
    kind: Certificate
    plural: certificates
  versions:
    - name: v1
      served: true
      storage: true
    - name: v1alpha2
      served: true
      storage: false
`

func renderedSource(docs string) RenderedCRDSource {
	return RenderedCRDSource{Label: "chart example@1.0.0", Tool: "helm",
		ChartDigest: "sha256:art", ValuesDigest: "sha256:vals", Docs: []byte(docs)}
}

// PO-7a addendum 6: rendered CRDs populate the dimension as render evidence
// — parsed like any CRD, marked Rendered, cited by an EvidenceRendered record
// that names the render and carries the chart/values digests — while the
// dimension stays PARTIAL and never "supplied": render output is not observed
// state, so absence conclusions stay off.
func TestLoadRenderedCRDs(t *testing.T) {
	e, err := Load(Inputs{RenderedCRDs: []RenderedCRDSource{renderedSource(renderedCRDYAML)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.CRDs) != 1 {
		t.Fatalf("crds = %+v", e.CRDs)
	}
	c := e.CRDs[0]
	if c.Name != "certificates.example.io" || c.Group != "example.io" || c.Kind != "Certificate" || !c.Rendered {
		t.Fatalf("crd = %+v", c)
	}
	if len(c.Versions) != 2 || !c.Versions[0].Served || c.Versions[1].Storage {
		t.Errorf("versions = %+v", c.Versions)
	}
	if e.Supplied.CRDs {
		t.Error("rendered CRDs must never mark the dimension supplied (absence is not knowledge)")
	}
	if h := e.Health(DimCRDs); h != HealthPartial {
		t.Errorf("the CRDs dimension must stay partial, got %s", h)
	}
	ix := map[domain.EvidenceID]domain.Evidence{}
	for _, ev := range e.Evidence {
		ix[ev.ID] = ev
	}
	var rec domain.Evidence
	for _, id := range c.Evidence {
		if r, ok := ix[id]; ok {
			rec = r
		}
	}
	if rec.Kind != domain.EvidenceRendered || rec.URI != "render:chart example@1.0.0" || rec.Render == nil ||
		rec.Render.Scope != domain.RenderEnvironment || rec.Render.Tool != "helm" ||
		rec.Render.ChartDigest != "sha256:art" || rec.Render.ValuesDigest != "sha256:vals" {
		t.Fatalf("rendered CRD evidence = %+v (render %+v)", rec, rec.Render)
	}
	// the rendered CRD feeds the GVK inventory like an observed one
	var served bool
	for _, u := range e.GVKUsage {
		if u.Group == "example.io" && u.Version == "v1" && u.Kind == "Certificate" {
			served = true
		}
	}
	if !served {
		t.Error("the rendered CRD's served versions must reach the GVK inventory")
	}
	// a second source rendering the same CRD adds nothing
	e2, err := Load(Inputs{RenderedCRDs: []RenderedCRDSource{renderedSource(renderedCRDYAML), renderedSource(renderedCRDYAML)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(e2.CRDs) != 1 {
		t.Errorf("a CRD rendered twice loads once: %+v", e2.CRDs)
	}
}

// An observed --crds input wins: rendered CRDs are not loaded beside it, and
// a CRD an observed manifest already states keeps its observed provenance.
func TestRenderedCRDsNeverOverrideObserved(t *testing.T) {
	dir := t.TempDir()
	observed := write(t, dir, "crds/certificates.yaml", renderedCRDYAML)
	e, err := Load(Inputs{CRDs: []string{observed},
		RenderedCRDs: []RenderedCRDSource{renderedSource(renderedCRDYAML)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.CRDs) != 1 || e.CRDs[0].Rendered {
		t.Fatalf("an observed --crds input wins over rendered CRDs: %+v", e.CRDs)
	}
	if !e.Supplied.CRDs || e.Health(DimCRDs) != HealthOK {
		t.Errorf("the observed dimension stands: supplied=%v health=%s", e.Supplied.CRDs, e.Health(DimCRDs))
	}
	// the same CRD inside an observed manifest keeps the observed provenance
	// even though the rendered copy was offered first by input order
	manifest := write(t, dir, "manifests/certs.yaml", renderedCRDYAML)
	e2, err := Load(Inputs{Manifests: []string{manifest},
		RenderedCRDs: []RenderedCRDSource{renderedSource(renderedCRDYAML)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(e2.CRDs) != 1 || e2.CRDs[0].Rendered {
		t.Fatalf("a manifest-observed CRD wins over the rendered copy: %+v", e2.CRDs)
	}
}

// A render without CRD documents (a gate closed, or a separate install path)
// says nothing: nothing loads, the dimension stays absent, and no record
// claims "no CRDs installed". Non-CRD documents in a source are ignored.
func TestRenderedCRDsAbsenceIsNotKnowledge(t *testing.T) {
	e, err := Load(Inputs{RenderedCRDs: []RenderedCRDSource{renderedSource("")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.CRDs) != 0 || e.Health(DimCRDs) != HealthAbsent || e.Supplied.CRDs {
		t.Fatalf("an empty render leaves the dimension absent: %+v %s", e.CRDs, e.Health(DimCRDs))
	}
	noise := strings.Replace(renderedCRDYAML, "kind: CustomResourceDefinition", "kind: Deployment", 1)
	e2, err := Load(Inputs{RenderedCRDs: []RenderedCRDSource{renderedSource(noise)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(e2.CRDs) != 0 {
		t.Errorf("non-CRD documents are ignored: %+v", e2.CRDs)
	}
}
