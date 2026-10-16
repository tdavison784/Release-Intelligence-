package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Layered Helm values files (Argo helm.valueFiles, Flux valuesFrom, helmfile,
// repeated -f): the loader merges them as Helm applies them instead of
// warning. Generic synthetic fixtures only.

func writeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, body := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func valuesKeyOf(e *Environment, path string) *ValuesKey {
	for i := range e.ValuesKeys {
		if e.ValuesKeys[i].Path == path {
			return &e.ValuesKeys[i]
		}
	}
	return nil
}

// Three layers over one release: later files override earlier ones per key,
// mappings deep-merge, and every effective key cites the file and line that
// set it. Layering itself is not a warning.
func TestLayeredValuesMerge(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"values-base.yaml": `replicaCount: 1
resources:
  limits:
    cpu: 100m
tolerations:
  - key: a
prometheus:
  servicemonitor:
    enabled: true
    targetPort: 9402
`,
		"values-overlay.yaml": `replicaCount: 3
resources: unlimited
tolerations:
  - key: b
prometheus:
  servicemonitor:
    targetPort: 1234
`,
	})
	e, err := Load(Inputs{Repo: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range e.Warnings {
		if strings.Contains(w, "values files") {
			t.Errorf("layered values files raised a warning: %q", w)
		}
	}
	// scalar override: the overlay's value, with the overlay's provenance
	k := valuesKeyOf(e, "replicaCount")
	if k == nil || k.Value != "3" {
		t.Fatalf("replicaCount = %+v, want the overlay's 3", k)
	}
	if ev := evURI(*e, k.Evidence[0]); !strings.HasSuffix(ev.URI, "values-overlay.yaml") || ev.Locator != "$.replicaCount (L1)" {
		t.Errorf("override provenance = %+v, want overlay.yaml L1", ev)
	}
	// a leaf replaces the subtree below its path (lists replace, so do
	// scalars over mappings)
	if k := valuesKeyOf(e, "resources"); k == nil || k.Value != `"unlimited"` {
		t.Errorf("resources = %+v, want the overlay's scalar", k)
	}
	if valuesKeyOf(e, "resources.limits.cpu") != nil {
		t.Error("a leaf at resources must clear the base file's subtree below it")
	}
	if k := valuesKeyOf(e, "tolerations"); k == nil || !strings.Contains(k.Value, `"key":"b"`) || strings.Contains(k.Value, `"key":"a"`) {
		t.Errorf("tolerations = %+v, want the overlay's list to replace the base's", k)
	}
	// mappings deep-merge: the untouched key survives, the overridden one
	// cites the overlay
	if k := valuesKeyOf(e, "prometheus.servicemonitor.enabled"); k == nil || k.Value != "true" {
		t.Errorf("deep-merge lost the base file's key: %+v", k)
	}
	if ev := evURI(*e, valuesKeyOf(e, "prometheus.servicemonitor.enabled").Evidence[0]); !strings.HasSuffix(ev.URI, "values-base.yaml") {
		t.Errorf("surviving key must keep the base file's provenance: %+v", ev)
	}
	if k := valuesKeyOf(e, "prometheus.servicemonitor.targetPort"); k == nil || k.Value != "1234" {
		t.Errorf("targetPort = %+v, want the overlay's", k)
	}
	// one effective key per path
	seen := map[string]int{}
	for _, k := range e.ValuesKeys {
		seen[k.Path]++
	}
	for p, n := range seen {
		if n != 1 {
			t.Errorf("path %q appears %d times in the effective values", p, n)
		}
	}
}

// A mapping replaces an earlier leaf at the same path (and vice versa): the
// merge tracks tree shape, not just exact paths.
func TestLayeredValuesMapReplacesLeaf(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"values-a.yaml": "affinity: none\nnodeSelector:\n  pool: default\n",
		"values-b.yaml": "affinity:\n  nodeAffinity: required\n",
	})
	e, err := Load(Inputs{Repo: root})
	if err != nil {
		t.Fatal(err)
	}
	if valuesKeyOf(e, "affinity") != nil {
		t.Error("a mapping at affinity must replace the earlier leaf at that path")
	}
	if k := valuesKeyOf(e, "affinity.nodeAffinity"); k == nil || k.Value != `"required"` {
		t.Errorf("affinity.nodeAffinity = %+v", k)
	}
	if k := valuesKeyOf(e, "nodeSelector.pool"); k == nil || k.Value != `"default"` {
		t.Errorf("untouched mapping key lost: %+v", k)
	}
}

// YAML merge keys ("<<") inside a layered file resolve the same way for the
// merge's tree shape as for its flattened leaves.
func TestLayeredValuesMergeKeys(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"values-a.yaml": "common:\n  labels:\n    app: x\n  replicas: 1\n",
		"values-b.yaml": "base: &base\n  replicas: 2\nover: &over\n  <<: *base\n  extra: true\ncommon:\n  <<: *over\n",
	})
	e, err := Load(Inputs{Repo: root})
	if err != nil {
		t.Fatal(err)
	}
	// the merged mapping under common: replicas from the merged key, the
	// base file's labels survive (deep merge), extra added
	if k := valuesKeyOf(e, "common.labels.app"); k == nil || k.Value != `"x"` {
		t.Errorf("common.labels.app = %+v, want the base file's", k)
	}
	if k := valuesKeyOf(e, "common.replicas"); k == nil || k.Value != "2" {
		t.Errorf("common.replicas = %+v, want the merge-key value 2", k)
	}
	if k := valuesKeyOf(e, "common.extra"); k == nil || k.Value != "true" {
		t.Errorf("common.extra = %+v", k)
	}
}

// Image references follow the merged values: repository and tag may come
// from different layers (Helm merges them the same way), and a repository an
// overlay replaced does not leak a stale image fact.
func TestLayeredValuesImages(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"values-a.yaml": "controller:\n  image:\n    repository: quay.io/jetstack/cert-manager-controller\n    tag: v1.16.0\n",
		"values-b.yaml": "controller:\n  image:\n    tag: v1.17.0\n",
	})
	e, err := Load(Inputs{Repo: root})
	if err != nil {
		t.Fatal(err)
	}
	var refs []string
	for _, im := range e.Images {
		if im.Source == "values" {
			refs = append(refs, im.Reference)
		}
	}
	if len(refs) != 1 || refs[0] != "quay.io/jetstack/cert-manager-controller:v1.17.0" {
		t.Fatalf("values image refs = %v, want exactly the merged v1.17.0", refs)
	}
	for _, im := range e.Images {
		if im.Source == "values" && im.Line != 3 {
			t.Errorf("merged image provenance line = %d, want the overlay's L3", im.Line)
		}
	}
}

// A values file a helmfile references but that is not present is a genuine
// gap: a warning with the reason, the values dimension partial.
func TestLayeredValuesMissingHelmfileReference(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"helmfile.yaml": "releases:\n  - name: cert-manager\n    chart: jetstack/cert-manager\n    version: v1.17.0\n    values:\n      - values/missing.yaml\n",
	})
	e, err := Load(Inputs{Repo: root})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, w := range e.Warnings {
		if strings.Contains(w, "values/missing.yaml") && strings.Contains(w, "not present") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing referenced values file not warned about: %v", e.Warnings)
	}
	if e.Health(DimValues) != HealthPartial {
		t.Errorf("values health = %q, want partial", e.Health(DimValues))
	}
}

// argoValuesFilesApp lists its values files in Helm's -f order: the list
// order applies them, not the alphabetical walk order (values-prod.yaml
// sorts before values.yaml). The last list entry also has a name the values
// convention would never discover — a reference must collect it anyway.
const argoValuesFilesApp = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
spec:
  project: default
  sources:
    - repoURL: https://charts.jetstack.io
      chart: cert-manager
      targetRevision: v1.17.0
      helm:
        valueFiles:
          - $values/platform/cert-manager/values.yaml
          - $values/platform/cert-manager/values-prod.yaml
          - $values/platform/cert-manager/extra.yaml
    - repoURL: https://git.example.com/platform/gitops.git
      ref: values
`

// Argo CD helm.valueFiles entries apply in list order — the later entry
// overrides the earlier one per key even when the reverse would hold under
// an alphabetical walk — and a referenced file is collected regardless of
// its name.
func TestLayeredValuesArgoListOrder(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"apps/cert-manager.yaml":                 argoValuesFilesApp,
		"platform/cert-manager/values.yaml":      "replicaCount: 2\n",
		"platform/cert-manager/values-prod.yaml": "replicaCount: 3\n",
		"platform/cert-manager/extra.yaml":       "extraKey: from-extra\n",
	})
	e, err := Load(Inputs{Repo: root})
	if err != nil {
		t.Fatal(err)
	}
	if k := valuesKeyOf(e, "replicaCount"); k == nil || k.Value != "3" {
		t.Fatalf("replicaCount = %+v, want the last list entry's 3", k)
	} else if ev := evURI(*e, k.Evidence[0]); !strings.HasSuffix(ev.URI, "values-prod.yaml") {
		t.Errorf("override provenance = %s, want values-prod.yaml", ev.URI)
	}
	if k := valuesKeyOf(e, "extraKey"); k == nil || k.Value != `"from-extra"` {
		t.Errorf("a referenced file outside the values name convention was not collected: extraKey = %+v", k)
	}
}

// A $values reference to a file that is not present is a genuine gap, the
// same as a helmfile reference: warning with the reason, values partial —
// not a silent skip and not a claim that values were seen.
func TestLayeredValuesMissingArgoReference(t *testing.T) {
	app := `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
spec:
  source:
    repoURL: https://charts.jetstack.io
    chart: cert-manager
    targetRevision: v1.17.0
    helm:
      valueFiles:
        - $values/platform/missing.yaml
`
	root := writeRepo(t, map[string]string{"apps/cert-manager.yaml": app})
	e, err := Load(Inputs{Repo: root})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, w := range e.Warnings {
		if strings.Contains(w, "platform/missing.yaml") && strings.Contains(w, "not present") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing $values reference not warned about: %v", e.Warnings)
	}
	if e.Health(DimValues) != HealthPartial {
		t.Errorf("values health = %q, want partial", e.Health(DimValues))
	}
	if e.Supplied.Values {
		t.Error("a values reference that could not be read must not claim values visibility")
	}
}
