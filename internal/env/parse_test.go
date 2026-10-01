package env

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// The stream decoder must never let a "---" that lives INSIDE a scalar
// (here: a literal block) split a document — the failure mode of the
// previous line-splitting parser — and every extracted fact must keep its
// absolute line number.
func TestParseDocsBlockScalarSeparator(t *testing.T) {
	src := `apiVersion: v1
kind: ConfigMap
metadata:
  name: scripts
data:
  run.sh: |
    #!/bin/sh
    echo one
    # a fenced yaml block inside the script:
    # ---
    # kind: decoy
    echo two
---
apiVersion: v1
kind: Namespace
`
	docs, err := parseDocs([]byte(src), nil)
	if err != nil {
		t.Fatalf("parseDocs: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("documents = %d, want 2 (a --- inside a block scalar is not a separator)", len(docs))
	}
	if got := scalarOf(docs[0].node, "kind"); got != "ConfigMap" {
		t.Errorf("doc1 kind = %q", got)
	}
	script := fieldOf(docs[0].node, "data", "run.sh")
	if script == nil || !strings.Contains(script.Value, "decoy") {
		t.Errorf("block scalar content truncated: %v", script)
	}
	if script != nil && script.Line != 6 {
		t.Errorf("block scalar line = %d, want 6 (absolute)", script.Line)
	}
	if docs[0].startLine != 1 || docs[1].startLine != 14 {
		t.Errorf("start lines = %d, %d; want 1, 14", docs[0].startLine, docs[1].startLine)
	}
	if got := scalarOf(docs[1].node, "kind"); got != "Namespace" {
		t.Errorf("doc2 kind = %q", got)
	}
}

func TestParseDocsEdgeCases(t *testing.T) {
	t.Run("empty documents are skipped", func(t *testing.T) {
		docs, err := parseDocs([]byte("---\n---\n# only a comment\nkind: A\n---\n"), nil)
		if err != nil {
			t.Fatal(err)
		}
		var live int
		for _, d := range docs {
			if d.node != nil {
				live++
				if got := scalarOf(d.node, "kind"); got != "A" {
					t.Errorf("kind = %q", got)
				}
			}
		}
		if live != 1 {
			t.Errorf("live documents = %d, want 1 (have %d)", live, len(docs))
		}
	})
	t.Run("CRLF and BOM", func(t *testing.T) {
		docs, err := parseDocs([]byte("\xef\xbb\xbfapiVersion: v1\r\nkind: ConfigMap\r\n"), nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(docs) != 1 || scalarOf(docs[0].node, "kind") != "ConfigMap" {
			t.Fatalf("docs = %+v", docs)
		}
		if docs[0].startLine != 1 {
			t.Errorf("startLine = %d, want 1", docs[0].startLine)
		}
	})
	t.Run("JSON document", func(t *testing.T) {
		docs, err := parseDocs([]byte(`{"apiVersion": "v1", "kind": "ConfigMap"}`), nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(docs) != 1 || scalarOf(docs[0].node, "kind") != "ConfigMap" {
			t.Fatalf("docs = %+v", docs)
		}
	})
	t.Run("truncated stream keeps its prefix and reports the rest", func(t *testing.T) {
		// An explicit end marker ("...") must be followed by a new "---";
		// a bare document after it is invalid YAML. The decoder stops there.
		docs, err := parseDocs([]byte("kind: A\n...\nkind: B\n"), nil)
		if err == nil {
			t.Fatal("expected an error for the unparsable remainder")
		}
		if len(docs) != 1 || scalarOf(docs[0].node, "kind") != "A" {
			t.Errorf("prefix must survive: %+v", docs)
		}
	})
	t.Run("broken first document errors", func(t *testing.T) {
		docs, err := parseDocs([]byte("a: [unclosed\n"), nil)
		if err == nil {
			t.Fatal("expected an error")
		}
		if len(docs) != 0 {
			t.Errorf("no document can have been decoded: %+v", docs)
		}
	})
	t.Run("duplicate keys are reported and resolve to the last value", func(t *testing.T) {
		var hits []string
		docs, err := parseDocs([]byte("kind: A\nkind: B\nspec:\n  x: 1\n  x: 2\n"), func(key string, line int) {
			hits = append(hits, key+"@"+strconv.Itoa(line))
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 2 {
			t.Fatalf("duplicate hits = %v, want 2", hits)
		}
		if got := scalarOf(docs[0].node, "kind"); got != "B" {
			t.Errorf("duplicate kind resolved to %q, want last value B", got)
		}
		if got := scalarOf(docs[0].node, "spec", "x"); got != "2" {
			t.Errorf("duplicate spec.x resolved to %q, want last value 2", got)
		}
	})
	t.Run("deterministic decode", func(t *testing.T) {
		src := []byte("kind: A\n---\nkind: B\n")
		first, err := parseDocs(src, nil)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 5; i++ {
			again, err := parseDocs(src, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, again) {
				t.Fatalf("run %d differs", i)
			}
		}
	})
}

// End-to-end through Load: a manifest stream with a --- inside a block
// scalar, verifying document count, apiVersion inventory and the evidence
// locators of both documents (the contract the impact report renders).
func TestLoadBlockScalarProvenance(t *testing.T) {
	dir := t.TempDir()
	man := write(t, dir, "release.yaml", `apiVersion: v1
kind: Namespace
metadata:
  name: prod
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: scripts
data:
  run.sh: |
    #!/bin/sh
    migrate
    # ---
    # decoy: true
`)
	e, err := Load(Inputs{Manifests: []string{man}})
	if err != nil {
		t.Fatal(err)
	}
	if e.ManifestDocCount != 2 {
		t.Fatalf("docs = %d, want 2", e.ManifestDocCount)
	}

	var cm APIVersionUse
	for _, u := range e.APIVersions {
		if u.Kind == "ConfigMap" {
			cm = u
		}
	}
	if cm.Kind == "" {
		t.Fatalf("apiVersions = %+v", e.APIVersions)
	}
	// the ConfigMap document starts after the separator, at line 6
	if got := evURI(*e, cm.Evidence[0]).Locator; got != "L6" {
		t.Errorf("ConfigMap evidence locator = %q, want L6", got)
	}
	fields := map[string]ManifestField{}
	for _, f := range e.ManifestFields {
		fields[f.Path] = f
	}
	f, ok := fields[`data["run.sh"]`] // the key itself contains a dot
	if !ok {
		t.Fatalf("fields = %v", keys(e.ManifestFields, func(f ManifestField) string { return f.Path }))
	}
	if f.Line != 11 { // the "run.sh: |" block scalar starts at line 11
		t.Errorf("field line = %d, want 11", f.Line)
	}
	if got := evURI(*e, f.Evidence[0]).Locator; got != `$.data["run.sh"] (L11)` {
		t.Errorf("field evidence locator = %q", got)
	}
}

// Duplicate keys in a manifest: last value wins (what a YAML processor
// keeps) and a warning names the key and where the repeat is.
func TestLoadDuplicateKeyManifest(t *testing.T) {
	dir := t.TempDir()
	man := write(t, dir, "dup.yaml", `apiVersion: v1
kind: Deployment
kind: ConfigMap
`)
	e, err := Load(Inputs{Manifests: []string{man}})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, u := range e.APIVersions {
		if u.Kind == "ConfigMap" {
			found = true
		}
		if u.Kind == "Deployment" {
			t.Error("the first of a duplicated key must not be extracted")
		}
	}
	if !found {
		t.Errorf("kinds = %+v", e.APIVersions)
	}
	warned := false
	for _, w := range e.Warnings {
		if strings.Contains(w, "duplicate key") && strings.Contains(w, `"kind"`) && strings.Contains(w, "L3") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("no duplicate-key warning: %v", e.Warnings)
	}
	if e.Health(DimManifests) != HealthPartial {
		t.Errorf("a manifest warning must make the dimension partial, got %q", e.Health(DimManifests))
	}
}

// Values edge cases: JSON, CRLF/BOM, non-mapping roots, multi-document
// streams — facts are never silently dropped, failures are warnings and
// degrade the dimension.
func TestLoadValuesEdgeCases(t *testing.T) {
	t.Run("JSON values file", func(t *testing.T) {
		dir := t.TempDir()
		val := write(t, dir, "values.json", `{"replicaCount": 3, "image": {"tag": "v2"}}`)
		e, err := Load(Inputs{ValuesFiles: []string{val}})
		if err != nil {
			t.Fatal(err)
		}
		if len(e.ValuesKeys) != 2 {
			t.Errorf("keys = %+v", e.ValuesKeys)
		}
		if e.Health(DimValues) != HealthOK {
			t.Errorf("health = %q, want ok", e.Health(DimValues))
		}
	})
	t.Run("CRLF and BOM", func(t *testing.T) {
		dir := t.TempDir()
		val := write(t, dir, "values.yaml", "\xef\xbb\xbfreplicaCount: 2\r\n")
		e, err := Load(Inputs{ValuesFiles: []string{val}})
		if err != nil {
			t.Fatal(err)
		}
		if len(e.ValuesKeys) != 1 || e.ValuesKeys[0].Path != "replicaCount" {
			t.Errorf("keys = %+v", e.ValuesKeys)
		}
	})
	t.Run("non-mapping root is a warning, not a crash", func(t *testing.T) {
		dir := t.TempDir()
		val := write(t, dir, "values.yaml", "- just\n- a\n- list\n")
		e, err := Load(Inputs{ValuesFiles: []string{val}})
		if err != nil {
			t.Fatal(err)
		}
		if len(e.ValuesKeys) != 0 || len(e.Warnings) == 0 {
			t.Errorf("keys = %d, warnings = %v", len(e.ValuesKeys), e.Warnings)
		}
		if e.Health(DimValues) != HealthPartial {
			t.Errorf("health = %q, want partial", e.Health(DimValues))
		}
	})
	t.Run("multi-document stream is a warning", func(t *testing.T) {
		dir := t.TempDir()
		val := write(t, dir, "values.yaml", "a: 1\n---\nb: 2\n")
		e, err := Load(Inputs{ValuesFiles: []string{val}})
		if err != nil {
			t.Fatal(err)
		}
		if len(e.Warnings) == 0 {
			t.Errorf("a multi-document values file must warn: %v", e.Warnings)
		}
		if e.Health(DimValues) != HealthPartial {
			t.Errorf("health = %q, want partial", e.Health(DimValues))
		}
	})
	t.Run("empty file is skipped quietly", func(t *testing.T) {
		dir := t.TempDir()
		val := write(t, dir, "values.yaml", "\n \n")
		e, err := Load(Inputs{ValuesFiles: []string{val}})
		if err != nil {
			t.Fatal(err)
		}
		if len(e.ValuesKeys) != 0 || len(e.Warnings) != 0 || e.Health(DimValues) != HealthOK {
			t.Errorf("keys = %d, warnings = %v, health = %q", len(e.ValuesKeys), e.Warnings, e.Health(DimValues))
		}
	})
}

// The per-dimension health API: absent (not supplied — never knowledge),
// ok (supplied and clean), partial (supplied with warnings).
func TestHealthStatuses(t *testing.T) {
	dir := t.TempDir()
	val := write(t, dir, "values.yaml", "a: 1\n")
	man := write(t, dir, "good.yaml", "apiVersion: v1\nkind: Namespace\n")
	bad := write(t, dir, "bad.yaml", "a: [unclosed\n")

	e, err := Load(Inputs{KubernetesVersion: "1.31", ValuesFiles: []string{val}, Manifests: []string{man}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Health{
		DimKubernetes: HealthOK, DimValues: HealthOK, DimManifests: HealthOK,
		DimCRDs: HealthAbsent, DimImages: HealthAbsent,
	}
	for dim, h := range want {
		if got := e.Health(dim); got != h {
			t.Errorf("Health(%s) = %q, want %q", dim, got, h)
		}
	}
	statuses := e.Statuses()
	if len(statuses) != 5 || statuses[0].Dimension != DimKubernetes {
		t.Errorf("statuses = %+v", statuses)
	}
	// a zero Environment is absent everywhere, and never panics
	var zero Environment
	if zero.Health(DimValues) != HealthAbsent || zero.Health("nonsense") != HealthAbsent {
		t.Error("unknown/zero dimensions must report absent")
	}

	// one broken manifest file degrades only its dimension
	e2, err := Load(Inputs{Manifests: []string{dir}, ValuesFiles: []string{val}})
	if err != nil {
		t.Fatal(err)
	}
	if e2.Health(DimManifests) != HealthPartial || e2.Health(DimValues) != HealthOK {
		t.Errorf("health = manifests %q, values %q; want partial/ok", e2.Health(DimManifests), e2.Health(DimValues))
	}
	st := map[string]DimensionStatus{}
	for _, s := range e2.Statuses() {
		st[s.Dimension] = s
	}
	if len(st[DimManifests].Warnings) == 0 {
		t.Fatalf("partial dimension must carry its warnings: %+v", st[DimManifests])
	}
	if !strings.Contains(strings.Join(st[DimManifests].Warnings, " "), "not parsed") {
		t.Errorf("warnings = %v", st[DimManifests].Warnings)
	}
	if !strings.Contains(strings.Join(st[DimManifests].Warnings, " "), bad) {
		t.Errorf("the manifest warning must name the broken file %s: %v", bad, st[DimManifests].Warnings)
	}

	// a non-CRD document in a --crds input degrades the CRDs dimension
	crd := write(t, dir, "notacrd.yaml", "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: x\n")
	e3, err := Load(Inputs{CRDs: []string{crd}})
	if err != nil {
		t.Fatal(err)
	}
	if e3.Health(DimCRDs) != HealthPartial {
		t.Errorf("a non-CRD document in a --crds input must make CRDs partial, got %q", e3.Health(DimCRDs))
	}

	// an unparsable image in the list degrades the images dimension only
	e4, err := Load(Inputs{Images: []string{"not a ref at all!!", "docker.io/library/busybox:1.36"}})
	if err != nil {
		t.Fatal(err)
	}
	if e4.Health(DimImages) != HealthPartial || e4.Health(DimValues) != HealthAbsent {
		t.Errorf("health = images %q, values %q", e4.Health(DimImages), e4.Health(DimValues))
	}

	// a supplied directory with no yaml files degrades that dimension
	empty := filepath.Join(dir, "empty-manifests")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	e5, err := Load(Inputs{Manifests: []string{empty}})
	if err != nil {
		t.Fatal(err)
	}
	if e5.Health(DimManifests) != HealthPartial {
		t.Errorf("an input directory without yaml files must be partial, got %q (warnings %v)", e5.Health(DimManifests), e5.Warnings)
	}
}
