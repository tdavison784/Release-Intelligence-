package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRepo writes files below a fresh root and returns it.
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

func loadProduct(t *testing.T, root, product string, explicit ...string) *Environment {
	t.Helper()
	e, err := Load(Inputs{Repo: root, Product: product, ValuesFiles: explicit})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// effective maps path → value of ValuesKeys (failing on a path set twice).
func effective(t *testing.T, e *Environment) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, k := range e.ValuesKeys {
		if _, dup := out[k.Path]; dup {
			t.Fatalf("path %s appears twice in %+v", k.Path, e.ValuesKeys)
		}
		out[k.Path] = k.Value
	}
	return out
}

func keyOf(e *Environment, path string) *ValuesKey {
	for i := range e.ValuesKeys {
		if e.ValuesKeys[i].Path == path {
			return &e.ValuesKeys[i]
		}
	}
	return nil
}

func wantValues(t *testing.T, got map[string]string, want map[string]string) {
	t.Helper()
	for p, v := range want {
		if v == "<absent>" {
			if g, ok := got[p]; ok {
				t.Errorf("%s = %s, want absent", p, g)
			}
			continue
		}
		if got[p] != v {
			t.Errorf("%s = %q, want %q (all: %v)", p, got[p], v, got)
		}
	}
}

func warned(e *Environment, sub string) bool {
	for _, w := range e.Warnings {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

const argoApp = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: %NAME%
spec:
  sources:
    - repoURL: https://charts.jetstack.io
      chart: cert-manager
      targetRevision: v1.17.2
      helm:
%HELM%
    - repoURL: https://git.example.com/gitops.git
      targetRevision: main
      ref: values
`

func argo(name, helm string) string {
	return strings.NewReplacer("%NAME%", name, "%HELM%", helm).Replace(argoApp)
}

// Argo CD: valueFiles apply in DECLARED order (not alphabetically), then the
// inline values, then parameters (Argo CD "Helm Value Precedence":
// parameters > valuesObject > values > valueFiles).
func TestArgoStackOrder(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"apps/cm.yaml": argo("cert-manager", `        valueFiles:
          - $values/cm/z-base.yaml
          - $values/cm/a-prod.yaml
        values: |
          inline: from-values
          fromParam: from-values
        parameters:
          - name: fromParam
            value: from-parameters`),
		"cm/z-base.yaml": "order: z-base\nbaseOnly: 1\ninline: from-z\n",
		"cm/a-prod.yaml": "order: a-prod\n",
	})
	e := loadProduct(t, root, "cert-manager")
	wantValues(t, effective(t, e), map[string]string{
		"order":     `"a-prod"`, // the later-declared file wins although it sorts first
		"baseOnly":  "1",
		"inline":    `"from-values"`,
		"fromParam": `"from-parameters"`,
	})
	if h := e.Health(DimValues); h != HealthOK {
		t.Fatalf("health = %s (%v)", h, e.Warnings)
	}
	// per-key provenance: the file and line of the layer that set the value
	k := keyOf(e, "order")
	ev := evURI(*e, k.Evidence[0])
	if !strings.HasSuffix(ev.URI, "cm/a-prod.yaml") || k.Line != 1 || !strings.HasPrefix(k.Release, "argocd:") {
		t.Errorf("order provenance = %+v / %+v", k, ev)
	}
	k = keyOf(e, "fromParam")
	if ev := evURI(*e, k.Evidence[0]); !strings.Contains(ev.Locator, "parameters[fromParam]") {
		t.Errorf("parameter provenance = %+v", ev)
	}
	// the referenced files are discovered as values, citing the reference
	n := 0
	for _, d := range e.Discovered {
		if d.Kind == DiscValues && strings.Contains(d.Detail, "referenced by apps/cm.yaml") {
			n++
		}
	}
	if n != 2 {
		t.Errorf("referenced values discoveries = %d, want 2 (%+v)", n, e.Discovered)
	}
}

// valuesObject trumps values: values is ignored entirely, not merged under it.
func TestArgoValuesObjectIgnoresValues(t *testing.T) {
	root := writeRepo(t, map[string]string{"apps/cm.yaml": argo("cert-manager", `        values: |
          onlyInValues: 1
          both: values
        valuesObject:
          both: object`)})
	e := loadProduct(t, root, "cert-manager")
	wantValues(t, effective(t, e), map[string]string{"both": `"object"`, "onlyInValues": "<absent>"})
	if e.Health(DimValues) != HealthOK {
		t.Errorf("health = %s", e.Health(DimValues))
	}
}

// --set typing follows Helm's strvals: bools/null case-insensitively,
// integers without a leading zero; floats, leading zeros and forceString stay
// strings; \. escapes a dot; null deletes what files set.
func TestArgoParametersTyping(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"apps/cm.yaml": argo("cert-manager", `        valueFiles:
          - $values/v.yaml
        parameters:
          - {name: b, value: "TRUE"}
          - {name: i, value: "12"}
          - {name: z, value: "0123"}
          - {name: f, value: "1.5"}
          - {name: s, value: "12", forceString: true}
          - {name: podAnnotations.prometheus\.io/scrape, value: "true"}
          - {name: gone, value: "null"}`),
		"v.yaml": "gone:\n  deep: 1\n",
	})
	e := loadProduct(t, root, "cert-manager")
	wantValues(t, effective(t, e), map[string]string{
		"b": "true", "i": "12", "z": `"0123"`, "f": `"1.5"`, "s": `"12"`,
		`podAnnotations["prometheus.io/scrape"]`: "true",
		"gone":                                   "null", "gone.deep": "<absent>",
	})
}

// A parameter addressing a list element (or a {a,b} list) is not evaluated:
// the stack is incomplete.
func TestArgoParameterListSyntaxIsAGap(t *testing.T) {
	root := writeRepo(t, map[string]string{"apps/cm.yaml": argo("cert-manager", `        parameters:
          - {name: "tolerations[0].key", value: x}`)})
	e := loadProduct(t, root, "cert-manager")
	if e.Health(DimValues) != HealthPartial || !warned(e, "list syntax") {
		t.Fatalf("health = %s, warnings = %v", e.Health(DimValues), e.Warnings)
	}
}

// Value files Argo CD reads from somewhere this repository cannot show.
func TestArgoUnreadableValueFiles(t *testing.T) {
	for name, tc := range map[string]struct{ helm, want string }{
		"inside the chart": {`        valueFiles:
          - values-prod.yaml`, "inside the chart package"},
		"missing": {`        valueFiles:
          - $values/nope.yaml`, "is not present"},
		"ignoreMissing": {`        ignoreMissingValueFiles: true
        valueFiles:
          - $values/nope.yaml`, "ignoreMissingValueFiles"},
		"unknown ref": {`        valueFiles:
          - $other/x.yaml`, "no source of the Application declares as ref"},
		"remote": {`        valueFiles:
          - https://example.com/v.yaml`, "remote URL"},
		"escapes the repo": {`        valueFiles:
          - $values/../../etc/v.yaml`, "not readable from this repository"},
		"fileParameters": {`        fileParameters:
          - {name: config, path: files/config.json}`, "fileParameters"},
	} {
		t.Run(name, func(t *testing.T) {
			e := loadProduct(t, writeRepo(t, map[string]string{"apps/cm.yaml": argo("cert-manager", tc.helm)}), "cert-manager")
			if e.Health(DimValues) != HealthPartial || !warned(e, tc.want) {
				t.Fatalf("health = %s, warnings = %v", e.Health(DimValues), e.Warnings)
			}
		})
	}
}

// A plain valueFiles path of a git chart source resolves relative to the
// chart directory in this repository.
func TestArgoGitSourcePlainValueFile(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"apps/cm.yaml": `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
spec:
  source:
    repoURL: https://git.example.com/gitops.git
    path: charts/cert-manager
    targetRevision: main
    helm:
      valueFiles:
        - values-prod.yaml
`,
		"charts/cert-manager/Chart.yaml":       "name: cert-manager\nversion: 1.0.0\n",
		"charts/cert-manager/values.yaml":      "defaults: chart\n",
		"charts/cert-manager/values-prod.yaml": "env: prod\n",
	})
	e := loadProduct(t, root, "cert-manager")
	wantValues(t, effective(t, e), map[string]string{"env": `"prod"`, "defaults": "<absent>"})
	if e.Health(DimValues) != HealthOK {
		t.Fatalf("health = %s, %v", e.Health(DimValues), e.Warnings)
	}
}

// Helm merge semantics across layers: maps deep-merge, lists and scalars
// replace, an empty map {} deletes nothing, null deletes the key (and what
// earlier layers set below it); a later map replaces a scalar or null.
func TestHelmMergeSemantics(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"apps/cm.yaml": argo("cert-manager", `        valueFiles:
          - $values/1.yaml
          - $values/2.yaml
          - $values/3.yaml`),
		"1.yaml": `deep:
  a: 1
  nested: {x: 1, y: 1}
list: [1, 2]
keep:
  a: 1
drop:
  a: 1
scalarToMap: 5
nullThenMap: {old: 1}
emptyOnScalar: 5
`,
		"2.yaml": `deep:
  b: 2
  nested: {y: 2}
list: [3]
keep: {}
drop: null
scalarToMap: {k: v}
nullThenMap: null
emptyOnScalar: {}
fresh: {}
`,
		"3.yaml": `nullThenMap: {new: 1}
`,
	})
	e := loadProduct(t, root, "cert-manager")
	wantValues(t, effective(t, e), map[string]string{
		"deep.a": "1", "deep.b": "2", "deep.nested.x": "1", "deep.nested.y": "2", // deep merge
		"list":   "[3]", // a list replaces
		"keep.a": "1", "keep": "<absent>", // {} does not delete
		"drop": "null", "drop.a": "<absent>", // null deletes
		"scalarToMap": "<absent>", "scalarToMap.k": `"v"`,
		"nullThenMap.new": "1", "nullThenMap.old": "<absent>", "nullThenMap": "<absent>",
		"emptyOnScalar": "{}", "fresh": "{}",
	})
	if k := keyOf(e, "keep.a"); k == nil || !strings.HasSuffix(evURI(*e, k.Evidence[0]).URI, "1.yaml") {
		t.Errorf("keep.a must still cite 1.yaml: %+v", k)
	}
	if k := keyOf(e, "drop"); k == nil || !strings.HasSuffix(evURI(*e, k.Evidence[0]).URI, "2.yaml") || k.Line != 6 {
		t.Errorf("drop must cite the null in 2.yaml L6: %+v", k)
	}
}

const fluxHR = `apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: cert-manager
  namespace: cert-manager
spec:
  chart:
    spec:
      chart: cert-manager
      version: 1.17.2
  valuesFrom:
    - kind: ConfigMap
      name: cm-late
      valuesKey: v
      targetPath: fromTarget
    - kind: ConfigMap
      name: cm-base
    - kind: ConfigMap
      name: cm-over
  values:
    who: inline
    fromTarget: inline
`

// Flux: valuesFrom in order, then spec.values, then targetPath entries
// ("When targetPath is set, it will overwrite everything before, including
// inline values").
func TestFluxStackOrder(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"flux/hr.yaml": fluxHR,
		"flux/cm.yaml": `apiVersion: v1
kind: ConfigMap
metadata: {name: cm-base, namespace: cert-manager}
data:
  values.yaml: |
    who: base
    baseOnly: 1
    order: base
---
apiVersion: v1
kind: ConfigMap
metadata: {name: cm-over, namespace: cert-manager}
data:
  values.yaml: |
    order: over
---
apiVersion: v1
kind: ConfigMap
metadata: {name: cm-late, namespace: cert-manager}
data:
  v: "42"
`,
	})
	e := loadProduct(t, root, "cert-manager")
	wantValues(t, effective(t, e), map[string]string{
		"who": `"inline"`, "baseOnly": "1", "order": `"over"`, "fromTarget": "42",
	})
	if e.Health(DimValues) != HealthOK {
		t.Fatalf("health = %s %v", e.Health(DimValues), e.Warnings)
	}
}

// A valuesFrom object no input defines (or a Secret) leaves that release's
// values partial, with the reason.
func TestFluxValuesFromUnreadable(t *testing.T) {
	e := loadProduct(t, writeRepo(t, map[string]string{"flux/hr.yaml": fluxHR}), "cert-manager")
	if e.Health(DimValues) != HealthPartial || !warned(e, "spec.valuesFrom ConfigMap cm-base references a cluster object no input defines") {
		t.Fatalf("health = %s %v", e.Health(DimValues), e.Warnings)
	}
	e = loadProduct(t, writeRepo(t, map[string]string{"flux/hr.yaml": strings.Replace(fluxHR, "kind: ConfigMap\n      name: cm-base", "kind: Secret\n      name: cm-base", 1)}), "cert-manager")
	if !warned(e, "Secret values are never read") {
		t.Fatalf("warnings = %v", e.Warnings)
	}
}

// helmfile: values entries in order (files and inline maps), then set.
func TestHelmfileStackOrder(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"helmfile.yaml": `releases:
  - name: cm
    chart: jetstack/cert-manager
    version: v1.17.2
    values:
      - envs/b.yaml
      - who: inline
      - envs/a.yaml
    set:
      - name: replicas
        value: 3
`,
		"envs/b.yaml": "who: b\nreplicas: 1\nbOnly: true\n",
		"envs/a.yaml": "order: a\n",
	})
	e := loadProduct(t, root, "cert-manager")
	wantValues(t, effective(t, e), map[string]string{"who": `"inline"`, "order": `"a"`, "replicas": "3", "bOnly": "true"})
	if e.Health(DimValues) != HealthOK {
		t.Fatalf("health = %s %v", e.Health(DimValues), e.Warnings)
	}
	// a templated values entry is not evaluated
	root = writeRepo(t, map[string]string{"helmfile.yaml": `releases:
  - name: cm
    chart: jetstack/cert-manager
    values:
      - envs/prod.yaml.gotmpl
`})
	if e := loadProduct(t, root, "cert-manager"); e.Health(DimValues) != HealthPartial {
		t.Fatalf("template values entry: health = %s", e.Health(DimValues))
	}
}

// Explicit --values files are applied after everything the release applies
// — files, inline values, parameters — so an explicit file wins per key
// (Inputs.Repo / Inputs.ValuesFiles doc). The reverted change let inline
// values beat them.
func TestExplicitValuesWinOverTheRelease(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"apps/cm.yaml": argo("cert-manager", `        valueFiles:
          - $values/v.yaml
        valuesObject:
          a: inline
          b: inline
        parameters:
          - {name: c, value: param}`),
		"v.yaml": "a: file\nd: file\n",
	})
	extra := filepath.Join(t.TempDir(), "extra.yaml")
	if err := os.WriteFile(extra, []byte("a: explicit\nc: explicit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := loadProduct(t, root, "cert-manager", extra)
	wantValues(t, effective(t, e), map[string]string{"a": `"explicit"`, "b": `"inline"`, "c": `"explicit"`, "d": `"file"`})
	if e.Health(DimValues) != HealthOK {
		t.Fatalf("health = %s %v", e.Health(DimValues), e.Warnings)
	}
	// without any release, the explicit files are the release, in -f order
	e2, err := Load(Inputs{Product: "cert-manager", ValuesFiles: []string{filepath.Join(root, "v.yaml"), extra}})
	if err != nil {
		t.Fatal(err)
	}
	wantValues(t, effective(t, e2), map[string]string{"a": `"explicit"`, "d": `"file"`, "c": `"explicit"`})
	if e2.Health(DimValues) != HealthOK || !e2.Supplied.Values {
		t.Fatalf("explicit-only: health = %s supplied = %v", e2.Health(DimValues), e2.Supplied.Values)
	}
}

// The reverted change's counter-examples: per-cluster files and other
// releases' inline blocks must never be merged into one global set.
func TestReleasesAreNeverMergedAcrossEachOther(t *testing.T) {
	root := writeRepo(t, map[string]string{
		// two clusters deploy cert-manager with their own values
		"clusters/dev/cert-manager.yaml": argo("cert-manager-dev", `        valueFiles:
          - $values/clusters/dev/values.yaml`),
		"clusters/dev/values.yaml":        "replicaCount: 1\ndevOnly: true\n",
		"clusters/prod/cert-manager.yaml": argo("cert-manager-prod", `        valueFiles:
          - $values/clusters/prod/values.yaml`),
		"clusters/prod/values.yaml": "replicaCount: 3\n",
		// another product with inline values setting the same key
		"apps/ingress.yaml": `apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: ingress-nginx
spec:
  chart:
    spec:
      chart: ingress-nginx
  values:
    replicaCount: 9
    ingressOnly: true
`,
	})
	e := loadProduct(t, root, "cert-manager")
	// partial with the reason: two releases, different effective values
	if e.Health(DimValues) != HealthPartial || !warned(e, "2 releases deploy cert-manager with different effective values") {
		t.Fatalf("health = %s, warnings = %v", e.Health(DimValues), e.Warnings)
	}
	if e.ValuesScope == nil || !e.ValuesScope.Ambiguous || len(e.ValuesScope.Releases) != 2 {
		t.Fatalf("scope = %+v", e.ValuesScope)
	}
	// each release's keys stay its own: replicaCount appears once per release,
	// with its own value and provenance; nothing of the other product
	per := map[string]map[string]string{}
	for _, k := range e.ValuesKeys {
		if per[k.Release] == nil {
			per[k.Release] = map[string]string{}
		}
		per[k.Release][k.Path] = k.Value
		if k.Path == "ingressOnly" || k.Value == "9" {
			t.Errorf("another product's values leaked in: %+v", k)
		}
	}
	var dev, prod map[string]string
	for id, kv := range per {
		switch {
		case strings.Contains(id, "clusters/dev"):
			dev = kv
		case strings.Contains(id, "clusters/prod"):
			prod = kv
		}
	}
	if dev["replicaCount"] != "1" || dev["devOnly"] != "true" || prod["replicaCount"] != "3" || prod["devOnly"] != "" {
		t.Fatalf("per-release values = dev %v prod %v", dev, prod)
	}
	// scoped to the other product: only its own release, complete
	o := loadProduct(t, root, "ingress-nginx")
	wantValues(t, effective(t, o), map[string]string{"replicaCount": "9", "ingressOnly": "true", "devOnly": "<absent>"})
	if o.Health(DimValues) != HealthOK {
		t.Errorf("ingress-nginx: health = %s %v", o.Health(DimValues), o.Warnings)
	}
}

// Releases of the product whose effective values are identical are one
// configuration: not ambiguous.
func TestEquivalentReleasesAreNotAmbiguous(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"a/cm.yaml": argo("cm-a", `        valuesObject: {replicaCount: 2}`),
		"b/cm.yaml": argo("cm-b", `        values: "replicaCount: 2"`),
	})
	e := loadProduct(t, root, "cert-manager")
	if e.Health(DimValues) != HealthOK || len(e.ValuesKeys) != 1 || len(e.ValuesScope.Equivalent) != 1 {
		t.Fatalf("health = %s keys = %+v scope = %+v", e.Health(DimValues), e.ValuesKeys, e.ValuesScope)
	}
}

// A values file no release references keeps the old warning (partial)
// unless its name attributes it to exactly one release: to the product (its
// own values-files release, as the renderer renders it) or to another
// release (not the product's values).
func TestUnreferencedValuesFiles(t *testing.T) {
	base := map[string]string{
		"apps/cm.yaml": argo("cert-manager", `        valueFiles:
          - $values/platform/cert-manager/values.yaml`),
		"platform/cert-manager/values.yaml": "replicaCount: 2\n",
		"apps/ingress.yaml": `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: ingress-nginx
spec:
  source:
    repoURL: https://kubernetes.github.io/ingress-nginx
    chart: ingress-nginx
    targetRevision: 4.11.5
`,
	}
	with := func(extra map[string]string) map[string]string {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	// fully referenced: ok
	if e := loadProduct(t, writeRepo(t, base), "cert-manager"); e.Health(DimValues) != HealthOK {
		t.Fatalf("referenced only: %s %v", e.Health(DimValues), e.Warnings)
	}
	// a stray values file naming nothing: partial, its keys kept per file
	e := loadProduct(t, writeRepo(t, with(map[string]string{"values-prod.yaml": "stray: 1\n"})), "cert-manager")
	if e.Health(DimValues) != HealthPartial || !warned(e, "does not attribute it to exactly one release") ||
		len(e.ValuesScope.Unattributed) != 1 {
		t.Fatalf("stray file: %s %v %+v", e.Health(DimValues), e.Warnings, e.ValuesScope)
	}
	if k := keyOf(e, "stray"); k == nil || k.Release != "" {
		t.Errorf("stray key = %+v", k)
	}
	// a file naming another release: not this product's values
	e = loadProduct(t, writeRepo(t, with(map[string]string{"platform/ingress-nginx/values-staging.yaml": "x: 1\n"})), "cert-manager")
	if e.Health(DimValues) != HealthOK || keyOf(e, "x") != nil {
		t.Fatalf("other release's file: %s %v", e.Health(DimValues), e.Warnings)
	}
	// a file naming the product: its own release, different from the Argo one
	e = loadProduct(t, writeRepo(t, with(map[string]string{"staging/values-cert-manager.yaml": "replicaCount: 1\n"})), "cert-manager")
	if e.Health(DimValues) != HealthPartial || !e.ValuesScope.Ambiguous {
		t.Fatalf("product-named file: %s %v", e.Health(DimValues), e.Warnings)
	}
	// a file naming two releases: unattributable
	e = loadProduct(t, writeRepo(t, with(map[string]string{"shared/cert-manager-ingress-nginx-values.yaml": "x: 1\n"})), "cert-manager")
	if e.Health(DimValues) != HealthPartial || len(e.ValuesScope.Unattributed) != 1 {
		t.Fatalf("two-name file: %s %v", e.Health(DimValues), e.Warnings)
	}
}

// A product with no release and no values file: values are not supplied.
// Chart names from the catalog (ProductHint.Charts) identify its releases.
func TestProductScopeByChartHint(t *testing.T) {
	root := writeRepo(t, map[string]string{"apps/x.yaml": strings.Replace(argo("prom", `        valuesObject: {a: 1}`), "chart: cert-manager", "chart: kube-prometheus-stack", 1)})
	e := loadProduct(t, root, "cert-manager")
	if e.Supplied.Values || e.Health(DimValues) != HealthAbsent {
		t.Fatalf("no release of the product: supplied = %v health = %s", e.Supplied.Values, e.Health(DimValues))
	}
	e, err := Load(Inputs{Repo: root, Product: "prometheus", ProductHints: []ProductHint{{ID: "prometheus", Charts: []string{"kube-prometheus-stack"}}}})
	if err != nil {
		t.Fatal(err)
	}
	wantValues(t, effective(t, e), map[string]string{"a": "1"})
}
