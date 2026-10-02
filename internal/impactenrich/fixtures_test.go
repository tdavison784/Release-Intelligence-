package impactenrich

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
)

var testNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// fixture assembles a deterministic report over a small synthetic edge and a
// values+manifests environment, so every test works against the same shapes.
type fixture struct {
	edge    *domain.UpgradeEdge
	env     *env.Environment
	report  *domain.ImpactReport
	changes map[string]domain.Change
}

func declared(rule string) domain.Provenance {
	return domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.notes@v1", Rule: rule, Confidence: domain.ConfidenceHigh}
}

func computed(rule string) domain.Provenance {
	return domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade.values@v1", Rule: rule, Confidence: domain.ConfidenceHigh}
}

func writeEnvFiles(t *testing.T, dir string) env.Inputs {
	t.Helper()
	write := func(rel, content string) string {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	values := write("values/prod.yaml", `certManager:
  replicaCount: 3
  extraArgs:
  - --enable-profiling=true
  resources:
    requests:
      cpu: 100m
imagePullSecrets:
- name: registry-internal-pull
`)
	crd := write("crds/certs.yaml", `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: certificates.cert-manager.io
spec:
  group: cert-manager.io
  names:
    kind: Certificate
  versions:
  - name: v1
    served: true
    storage: true
`)
	manifest := write("manifests/app.yaml", `apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: api-cert
  namespace: platform
spec:
  secretName: api-cert-tls
  dnsNames: [api.internal]
---
apiVersion: v1
kind: Secret
metadata:
  name: registry-internal-pull
  namespace: platform
type: kubernetes.io/dockerconfigjson
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: cert-manager
  namespace: platform
spec:
  chart:
    spec:
      chart: cert-manager
      version: "1.17.x"
      sourceRef:
        kind: HelmRepository
        name: jetstack
`)
	return env.Inputs{
		KubernetesVersion: "1.28",
		ValuesFiles:       []string{values},
		Manifests:         []string{manifest},
		CRDs:              []string{crd},
	}
}

// newFixture builds an edge whose note-derived changes become unknown
// findings (plus one computed diff and one routine item), joins it with the
// synthetic environment and validates the report.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	edge := &domain.UpgradeEdge{
		SchemaVersion: domain.UpgradeEdgeSchemaVersion,
		Product:       domain.ProductRef{ID: "example", Name: "Example"},
		From:          domain.MustVersion("v1.0.0", "1.0.0"),
		To:            domain.MustVersion("v1.1.0", "1.1.0"),
		Path:          []domain.PathStep{{Version: domain.MustVersion("v1.1.0", "1.1.0"), Reason: "minor-release"}},
	}
	ev := func(uri, excerpt string) domain.EvidenceID {
		e := domain.NewEvidence(domain.EvidenceDocument, "notes", uri, "L1-L9", excerpt, domain.Digest([]byte(uri)), testNow)
		for _, x := range edge.Evidence {
			if x.ID == e.ID {
				return e.ID
			}
		}
		edge.Evidence = append(edge.Evidence, e)
		return e.ID
	}
	add := func(c domain.Change) domain.Change {
		if len(c.Evidence) == 0 {
			c.Evidence = []domain.EvidenceID{ev("https://example/notes/"+c.ID+".md", c.Title)}
		}
		c.ID = "chg-" + domain.ShortHash(c.Title)
		edge.Changes = append(edge.Changes, c)
		return c
	}
	// two note-derived unknowns: the second duplicates the first (cluster)
	add(domain.Change{
		Category: domain.CategoryConfiguration, Title: "The default value of `ExtraArgs.Profiling` changed",
		Detail:   "The profiling sidecar is now disabled by default.",
		Subjects: []string{"ExtraArgs.Profiling"}, Provenance: declared("section:/breaking/i"),
	})
	add(domain.Change{
		Category: domain.CategoryConfiguration, Title: "Default of `ExtraArgs.Profiling` changed",
		Detail: "Profiling defaults changed.", Subjects: []string{"ExtraArgs.Profiling"}, Provenance: declared("notes:v1.1.0"),
	})
	// a routine dependency bump (excluded from candidates)
	add(domain.Change{
		Category: domain.CategoryDependency, Title: "Bump github.com/foxy/dep from 1.2.3 to 1.2.4",
		Provenance: declared("notes:v1.1.0"), Routine: true, RoutineKind: "dependency",
	})
	// a computed diff with no join rule (excluded: machine-comparable subjects)
	add(domain.Change{
		Category: domain.CategoryCRDSchema, Title: "[diff] CRD field spec.newField added",
		Subjects: []string{"spec.newField"}, Provenance: computed("crd:fields-added"),
	})
	// an affected migration change (values-removed fires on the fixture env)
	add(domain.Change{
		Category: domain.CategoryMigration, Breaking: true,
		Title:    "The key `imagePullSecrets` was removed",
		Detail:   "imagePullSecrets is no longer supported; migrate to controllerImage.pullSecrets.",
		Subjects: []string{"imagePullSecrets"}, Provenance: computed("values:removed"),
	})

	e, err := env.Load(writeEnvFiles(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := impact.Build(impact.Input{Edge: edge, Env: e, Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	if err := rep.Validate(); err != nil {
		t.Fatalf("fixture report invalid: %v", err)
	}
	changes := map[string]domain.Change{}
	for _, c := range edge.Changes {
		changes[c.ID] = c
	}
	return &fixture{edge: edge, env: e, report: rep, changes: changes}
}

// unknownFindingByChange returns the unknown finding joined to a change.
func unknownFindingByChange(t *testing.T, f *fixture, changeID string) domain.ImpactFinding {
	t.Helper()
	for _, fd := range f.report.Findings {
		if fd.Classification == domain.ImpactUnknown && fd.ChangeID == changeID {
			return fd
		}
	}
	t.Fatalf("no unknown finding for change %s", changeID)
	return domain.ImpactFinding{}
}

// findingByID returns any finding by id.
func findingByID(t *testing.T, f *fixture, id string) domain.ImpactFinding {
	t.Helper()
	for _, fd := range f.report.Findings {
		if fd.ID == id {
			return fd
		}
	}
	t.Fatalf("no finding %s", id)
	return domain.ImpactFinding{}
}

// fakeAnswer builds an llm response body for scripted clients.
func fakeAnswer(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
