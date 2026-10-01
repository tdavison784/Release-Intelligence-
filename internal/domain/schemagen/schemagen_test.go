package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

const regenerate = "run `go run ./internal/domain/schemagen` (or `go generate ./internal/domain/schemagen`) from the repository and commit the result"

// TestSchemasUpToDate fails when the checked-in schemas differ from what the
// generator derives from the current domain types.
func TestSchemasUpToDate(t *testing.T) {
	files, err := generate()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	for name, want := range files {
		path := filepath.Join(root, "schemas", name)
		have, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("schemas/%s cannot be read (%v); %s", name, err, regenerate)
			continue
		}
		if !bytes.Equal(have, want) {
			t.Errorf("schemas/%s is stale: it does not match the Go types in internal/domain.\n"+
				"To fix: %s.\nFirst difference: %s", name, regenerate, firstDiff(have, want))
		}
	}
}

func firstDiff(have, want []byte) string {
	h, w := strings.Split(string(have), "\n"), strings.Split(string(want), "\n")
	for i := 0; i < len(h) || i < len(w); i++ {
		var a, b string
		if i < len(h) {
			a = h[i]
		}
		if i < len(w) {
			b = w[i]
		}
		if a != b {
			return "line " + strconv.Itoa(i+1) + "\n  checked in: " + a + "\n  generated:  " + b
		}
	}
	return "(none)"
}

// TestGenerateIsDeterministic guards the byte-for-byte reproducibility the
// staleness check relies on.
func TestGenerateIsDeterministic(t *testing.T) {
	a, err := generate()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		b, err := generate()
		if err != nil {
			t.Fatal(err)
		}
		for name := range a {
			if !bytes.Equal(a[name], b[name]) {
				t.Fatalf("%s differs between runs", name)
			}
		}
	}
}

// TestSchemasCompile checks that the checked-in files are valid draft
// 2020-12 schemas (compiling validates them against the metaschema).
func TestSchemasCompile(t *testing.T) {
	for _, tg := range targets {
		s := compile(t, tg.file)
		if s == nil {
			t.Fatalf("%s: no schema", tg.file)
		}
	}
}

// TestEnumsMatchGoConstants parses the domain sources and compares the
// constants of every typed-string group with the enum lists in meta.go.
func TestEnumsMatchGoConstants(t *testing.T) {
	consts := domainConstants(t)
	registered := map[string][]string{}
	for _, e := range enums {
		registered[e.typ.Name()] = e.values
	}
	for typ, want := range consts {
		got, ok := registered[typ]
		if !ok {
			if !notSerialised[typ] {
				t.Errorf("domain.%s has typed constants %v but no enum in schemagen/meta.go: add it to `enums` (or to `notSerialised` if it never appears in JSON)", typ, want)
			}
			continue
		}
		g, w := append([]string(nil), got...), append([]string(nil), want...)
		sort.Strings(g)
		sort.Strings(w)
		if !reflect.DeepEqual(g, w) {
			t.Errorf("enum %s in schemagen/meta.go is out of date.\n  enum:      %v\n  constants: %v", typ, g, w)
		}
	}
	for typ := range registered {
		if _, ok := consts[typ]; !ok {
			t.Errorf("enum %s has no typed constants in package domain", typ)
		}
	}
}

// domainConstants returns the string values of every `const X T = "..."`
// declared in the (non-test) sources of package domain, grouped by type T.
func domainConstants(t *testing.T) map[string][]string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "internal", "domain")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				id, ok := vs.Type.(*ast.Ident)
				if !ok || len(vs.Values) != 1 {
					continue
				}
				lit, ok := vs.Values[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				v, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				out[id.Name] = append(out[id.Name], v)
			}
		}
	}
	return out
}

// TestUpgradeEdgeValidates marshals a realistic edge and validates it.
func TestUpgradeEdgeValidates(t *testing.T) {
	schema := compile(t, "upgrade-edge.schema.json")
	edge := sampleEdge()
	if err := edge.Validate(); err != nil {
		t.Fatalf("sample edge is not valid Go-side: %v", err)
	}
	if err := schema.Validate(toInstance(t, edge)); err != nil {
		t.Fatalf("a valid UpgradeEdge does not match the schema: %v", err)
	}
}

// TestUpgradeEdgeMinimal covers the smallest edge the builder can emit: only
// the required fields, empty collections, no optional pointers.
func TestUpgradeEdgeMinimal(t *testing.T) {
	schema := compile(t, "upgrade-edge.schema.json")
	edge := &domain.UpgradeEdge{
		SchemaVersion: domain.UpgradeEdgeSchemaVersion,
		Product:       domain.ProductRef{ID: "demo", Name: "Demo"},
		From:          domain.MustVersion("v1.0.0", "1.0.0"),
		To:            domain.MustVersion("v1.0.1", "1.0.1"),
		PathPolicy:    "all",
		Path:          []domain.PathStep{},
		Sources:       []domain.SourceStatus{},
		Changes:       []domain.Change{},
		Evidence:      []domain.Evidence{},
		GeneratedAt:   time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
	}
	if err := schema.Validate(toInstance(t, edge)); err != nil {
		t.Fatalf("minimal edge: %v", err)
	}
}

// TestProvenanceInvariantParity is the heart of the contract: each mutation
// is something a buggy producer could emit. The Go validator and the schema
// must both reject it.
func TestProvenanceInvariantParity(t *testing.T) {
	schema := compile(t, "upgrade-edge.schema.json")
	cases := []struct {
		name string
		edit func(e *domain.UpgradeEdge)
		at   string // instance location (JSON pointer prefix) that must be reported
	}{
		{"change with method ai", func(e *domain.UpgradeEdge) {
			p := &e.Changes[0].Provenance
			p.Method = domain.MethodAI
			p.Model, p.PromptDigest, p.InputEvidence = "claude-test", "sha256:abc", e.Changes[0].Evidence
		}, "/changes/0/provenance"},
		{"change with method ai and no ai fields", func(e *domain.UpgradeEdge) {
			e.Changes[1].Provenance.Method = domain.MethodAI
		}, "/changes/1/provenance"},
		{"deterministic change carrying a model", func(e *domain.UpgradeEdge) {
			e.Changes[0].Provenance.Model = "claude-test"
		}, "/changes/0/provenance"},
		{"deterministic change carrying a prompt digest", func(e *domain.UpgradeEdge) {
			e.Changes[0].Provenance.PromptDigest = "sha256:abc"
		}, "/changes/0/provenance"},
		{"unknown method on a change", func(e *domain.UpgradeEdge) {
			e.Changes[0].Provenance.Method = "guess"
		}, "/changes/0/provenance"},
		{"change without evidence", func(e *domain.UpgradeEdge) {
			e.Changes[0].Evidence = []domain.EvidenceID{}
		}, "/changes/0/evidence"},
		{"change without producer", func(e *domain.UpgradeEdge) {
			e.Changes[0].Provenance.Producer = ""
		}, "/changes/0/provenance"},
		{"change with invalid confidence", func(e *domain.UpgradeEdge) {
			e.Changes[0].Provenance.Confidence = "certain"
		}, "/changes/0/provenance"},
		{"enrichment with declared method", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].Provenance.Method = domain.MethodDeclared
		}, "/enrichments/0/provenance"},
		{"enrichment without model", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].Provenance.Model = ""
		}, "/enrichments/0/provenance"},
		{"enrichment without promptDigest", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].Provenance.PromptDigest = ""
		}, "/enrichments/0/provenance"},
		{"enrichment without inputEvidence", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].Provenance.InputEvidence = nil
		}, "/enrichments/0/provenance"},
		{"enrichment without modelVersion", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].Provenance.ModelVersion = ""
		}, "/enrichments/0/provenance"},
		{"enrichment without promptVersion", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].Provenance.PromptVersion = ""
		}, "/enrichments/0/provenance"},
		{"enrichment without generatedAt", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].Provenance.GeneratedAt = nil
		}, "/enrichments/0/provenance"},
		{"deterministic change carrying a model version", func(e *domain.UpgradeEdge) {
			e.Changes[0].Provenance.ModelVersion = "fake-model-v1"
		}, "/changes/0/provenance"},
		{"enrichment of unknown kind", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].Kind = "summary"
		}, "/enrichments/0/kind"},
		{"enrichment with blank content", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].Content = " \n "
		}, "/enrichments/0/content"},
		{"enrichment without citations", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].Citations = []domain.EvidenceID{}
		}, "/enrichments/0/citations"},
		{"enrichment about no change", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].RelatesTo = []string{}
		}, "/enrichments/0/relatesTo"},
		{"cluster of a single change", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].RelatesTo = e.Enrichments[0].RelatesTo[:1]
		}, "/enrichments/0/relatesTo"},
		{"related changes not marked unverified", func(e *domain.UpgradeEdge) {
			e.Enrichments[1].Unverified = false
		}, "/enrichments/1"},
		{"cluster marked unverified", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].Unverified = true
		}, "/enrichments/0/unverified"},
		{"enrichment id without the enrichment prefix", func(e *domain.UpgradeEdge) {
			e.Enrichments[0].ID = "chg-ai-1"
		}, "/enrichments/0/id"},
		{"change using the enrichment id prefix", func(e *domain.UpgradeEdge) {
			e.Changes[2].ID = "enr-smuggled"
		}, "/changes/2/id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := sampleEdge()
			tc.edit(e)
			if err := e.Validate(); err == nil {
				t.Fatal("test bug: the Go validator accepts this edge")
			}
			err := schema.Validate(toInstance(t, e))
			requireInvalidAt(t, err, tc.at)
		})
	}
}

// TestSchemaRejectsMalformedDocuments covers shape errors that have no Go
// validator counterpart.
func TestSchemaRejectsMalformedDocuments(t *testing.T) {
	schema := compile(t, "upgrade-edge.schema.json")
	cases := []struct {
		name string
		edit func(root map[string]any)
		at   string
	}{
		{"unknown top-level property", func(r map[string]any) { r["extra"] = true }, ""},
		{"unknown property on a change", func(r map[string]any) { change(r, 0)["severity"] = "high" }, "/changes/0"},
		{"category outside the enum", func(r map[string]any) { change(r, 0)["category"] = "chore" }, "/changes/0/category"},
		{"evidence kind outside the enum", func(r map[string]any) {
			r["evidence"].([]any)[0].(map[string]any)["kind"] = "screenshot"
		}, "/evidence/0/kind"},
		{"source state outside the enum", func(r map[string]any) {
			r["sources"].([]any)[0].(map[string]any)["state"] = "great"
		}, "/sources/0/state"},
		{"artifact status outside the enum", func(r map[string]any) {
			r["artifacts"].([]any)[0].(map[string]any)["to"].(map[string]any)["status"] = "maybe"
		}, "/artifacts/0/to/status"},
		{"wrong schemaVersion", func(r map[string]any) { r["schemaVersion"] = "ri.dev/upgrade-edge/v0" }, "/schemaVersion"},
		{"missing changes", func(r map[string]any) { delete(r, "changes") }, ""},
		{"generatedAt is not a date-time", func(r map[string]any) { r["generatedAt"] = "yesterday" }, "/generatedAt"},
		{"breaking is not a boolean", func(r map[string]any) { change(r, 0)["breaking"] = "yes" }, "/changes/0/breaking"},
		{"extra property in provenance", func(r map[string]any) { change(r, 0)["provenance"].(map[string]any)["by"] = "me" }, "/changes/0/provenance"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := toInstance(t, sampleEdge()).(map[string]any)
			tc.edit(root)
			requireInvalidAt(t, schema.Validate(root), tc.at)
		})
	}
}

// TestSampleUsesEveryBranch keeps the happy-path sample honest: it must
// exercise all methods, optional pointers and enum-typed fields, so that a
// regression in the schema for any of them shows up in TestUpgradeEdgeValidates.
func TestSampleUsesEveryBranch(t *testing.T) {
	e := sampleEdge()
	methods := map[domain.Method]bool{}
	for _, c := range e.Changes {
		methods[c.Provenance.Method] = true
	}
	for _, m := range []domain.Method{domain.MethodDeclared, domain.MethodComputed, domain.MethodHeuristic} {
		if !methods[m] {
			t.Errorf("sample edge has no %s change", m)
		}
	}
	if len(e.Enrichments) == 0 || e.Enrichments[0].Provenance.GeneratedAt == nil {
		t.Error("sample edge needs an enrichment with generatedAt")
	}
	if len(e.Compatibility) == 0 || len(e.Artifacts) == 0 || len(e.Facts) == 0 || len(e.SkippedReleases) == 0 {
		t.Error("sample edge needs compatibility, artifacts, facts and skipped releases")
	}
}

// TestReleaseValidates marshals a realistic release and validates it.
func TestReleaseValidates(t *testing.T) {
	schema := compile(t, "release.schema.json")
	rel := sampleRelease()
	if err := schema.Validate(toInstance(t, rel)); err != nil {
		t.Fatalf("a valid Release does not match the schema: %v", err)
	}

	cases := []struct {
		name string
		edit func(root map[string]any)
		at   string
	}{
		{"snapshot payload does not match kind", func(r map[string]any) {
			snapshot(r, 0)["kind"] = "crds"
		}, "/snapshots/0"},
		{"snapshot with two payloads", func(r map[string]any) {
			snapshot(r, 0)["images"] = map[string]any{"images": []any{}}
		}, "/snapshots/0"},
		{"snapshot without payload", func(r map[string]any) {
			delete(snapshot(r, 0), "values")
		}, "/snapshots/0"},
		{"note classified by ai", func(r map[string]any) {
			note := r["notes"].([]any)[0].(map[string]any)
			note["classification"].(map[string]any)["method"] = "ai"
		}, "/notes/0/classification"},
		{"compatibility provenance by ai", func(r map[string]any) {
			c := r["compatibility"].([]any)[0].(map[string]any)
			c["provenance"].(map[string]any)["method"] = "ai"
		}, "/compatibility/0/provenance"},
		{"artifact type outside the enum", func(r map[string]any) {
			r["artifacts"].([]any)[0].(map[string]any)["type"] = "tarball"
		}, "/artifacts/0/type"},
		{"missing ingestedAt", func(r map[string]any) { delete(r, "ingestedAt") }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := toInstance(t, sampleRelease()).(map[string]any)
			tc.edit(root)
			requireInvalidAt(t, schema.Validate(root), tc.at)
		})
	}
}

// TestPointerAndOmitSemantics pins how the generator reads json tags, using
// local types so the rules are tested independently of the domain model.
func TestPointerAndOmitSemantics(t *testing.T) {
	type inner struct {
		N int `json:"n"`
	}
	type sample struct {
		Plain      string            `json:"plain"`
		Omit       string            `json:"omit,omitempty"`
		OptPtr     *string           `json:"optPtr,omitempty"`
		NullPtr    *string           `json:"nullPtr"`
		When       time.Time         `json:"when,omitempty"` // omitempty is a no-op for structs
		Zero       time.Time         `json:"zero,omitzero"`  // omitzero is honoured
		OptTime    *time.Time        `json:"optTime,omitempty"`
		Inner      inner             `json:"inner,omitempty"` // no-op as well
		List       []string          `json:"list"`
		OptList    []inner           `json:"optList,omitempty"`
		Attrs      map[string]string `json:"attrs,omitempty"`
		Skipped    string            `json:"-"`
		unexported string
	}
	g := &gen{defs: map[string]obj{}, used: map[string]bool{}}
	g.ref(reflect.TypeOf(sample{}))
	def := g.defs["sample"]
	req, _ := def.get("required")
	got := req.([]string)
	want := []string{"plain", "nullPtr", "when", "inner", "list"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("required = %v, want %v", got, want)
	}
	props, _ := def.get("properties")
	p := props.(obj)
	find := func(k string) any { v, _ := p.get(k); return v }
	if _, ok := find("Skipped").(obj); ok {
		t.Error("json:\"-\" field must not be emitted")
	}
	if got := scalarString(find("nullPtr")); !strings.Contains(got, `"anyOf"`) || !strings.Contains(got, `"null"`) {
		t.Errorf("pointer without omitempty must be nullable, got %s", got)
	}
	if got := scalarString(find("optPtr")); strings.Contains(got, "null") {
		t.Errorf("pointer with omitempty is optional but not nullable, got %s", got)
	}
	if got := scalarString(find("when")); !strings.Contains(got, `"date-time"`) {
		t.Errorf("time.Time must be a date-time string, got %s", got)
	}
	if got := scalarString(find("attrs")); !strings.Contains(got, `"additionalProperties"`) {
		t.Errorf("maps must use additionalProperties, got %s", got)
	}
	if ap, _ := def.get("additionalProperties"); ap != false {
		t.Errorf("structs must forbid additional properties, got %v", ap)
	}
}

func scalarString(v any) string {
	s, _ := inline(v)
	return s
}

// --- helpers --------------------------------------------------------------

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// compile loads a checked-in schema, asserting formats such as date-time.
func compile(t *testing.T, file string) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	s, err := c.Compile(filepath.Join(repoRoot(t), "schemas", file))
	if err != nil {
		t.Fatalf("compile schemas/%s: %v", file, err)
	}
	return s
}

// toInstance marshals v with encoding/json (exactly what the CLI and the
// store emit) and decodes it the way the validator wants to see JSON.
func toInstance(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return inst
}

// requireInvalidAt asserts that validation failed and, when at is not empty,
// that some reported error sits at that instance location or below it.
func requireInvalidAt(t *testing.T, err error, at string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected the schema to reject the document, but it was accepted")
	}
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		t.Fatalf("unexpected error type %T: %v", err, err)
	}
	if at == "" {
		return
	}
	var locs []string
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		loc := "/" + strings.Join(e.InstanceLocation, "/")
		if len(e.InstanceLocation) == 0 {
			loc = ""
		}
		locs = append(locs, loc)
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	for _, l := range locs {
		if l == at || strings.HasPrefix(l, at+"/") {
			return
		}
	}
	t.Fatalf("schema rejected the document, but not at %s (errors at %v):\n%v", at, locs, err)
}

func change(root map[string]any, i int) map[string]any {
	return root["changes"].([]any)[i].(map[string]any)
}

func snapshot(root map[string]any, i int) map[string]any {
	return root["snapshots"].([]any)[i].(map[string]any)
}

// --- sample documents -----------------------------------------------------

var (
	t0      = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	vFrom   = domain.MustVersion("v1.17.0", "1.17.0")
	vMid    = domain.MustVersion("v1.18.0", "1.18.0")
	vTo     = domain.MustVersion("v1.19.0", "1.19.0")
	chart   = "https://charts.jetstack.io"
	notes   = "https://github.com/cert-manager/cert-manager/releases/tag/v1.18.0"
	rawVals = "https://raw.githubusercontent.com/cert-manager/cert-manager/v1.19.0/deploy/charts/cert-manager/values.yaml"
)

func declaredProv(rule string) domain.Provenance {
	return domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.notes@v1", Rule: rule, Confidence: domain.ConfidenceHigh}
}

// sampleEdge builds a realistic edge by hand: declared, computed and
// heuristic changes, compatibility and artifact deltas, an unavailable
// source, facts, evidence and one AI enrichment.
func sampleEdge() *domain.UpgradeEdge {
	pub := func(d int) *time.Time { x := t0.AddDate(0, 0, -d); return &x }

	evNotes := domain.NewEvidence(domain.EvidenceDocument, "release-notes", notes, "## Breaking Changes",
		"The default value of Certificate.spec.privateKey.rotationPolicy is now Always.", domain.Digest([]byte("notes-1.18.0")), t0)
	evValues := domain.NewEvidence(domain.EvidenceStructured, "chart-values", rawVals, "$.installCRDs",
		"installCRDs: false", domain.Digest([]byte("values-1.19.0")), t0)
	evBody := domain.NewEvidence(domain.EvidenceDocument, "release-notes", notes, "L88-L90",
		"Deprecated: the --enable-certificate-owner-ref flag will be removed in a future release.", domain.Digest([]byte("notes-1.18.0")), t0)
	evCompat := domain.NewEvidence(domain.EvidenceDocument, "supported-releases", "https://cert-manager.io/docs/releases/", "table:supported",
		"1.19 | Kubernetes 1.29 - 1.33", domain.Digest([]byte("compat")), t0)
	evChart := domain.NewEvidence(domain.EvidenceRegistry, "helm-repo", chart+"/index.yaml", "entries.cert-manager[v1.19.0]",
		"appVersion: v1.19.0", domain.Digest([]byte("index")), t0)
	evAdvisory := domain.NewEvidence(domain.EvidenceAdvisory, "advisories", "https://github.com/cert-manager/cert-manager/security/advisories/GHSA-xxxx-xxxx-xxxx", "",
		"Example advisory text", domain.Digest([]byte("advisory")), t0)
	evGuide := domain.NewEvidence(domain.EvidenceDocument, "upgrade-guide", "https://cert-manager.io/docs/releases/upgrading/upgrading-1.17-1.18", "L8-L10",
		"We have changed the default value of Certificate.Spec.PrivateKey.RotationPolicy from Never to Always.", domain.Digest([]byte("guide")), t0)

	fNotes := domain.NewFact(domain.FactDocumentRetrieved, "cert-manager@1.18.0", "1.18.0", "Release notes of v1.18.0 were retrieved",
		"normalize.notes@v1", map[string]string{"bytes": "4312"}, evNotes.ID)
	fChart := domain.NewFact(domain.FactArtifactPublished, "cert-manager chart", "1.19.0", "Chart v1.19.0 is published in the Helm repository",
		"helm.index@v1", nil, evChart.ID)

	generated := t0.Add(-time.Minute)
	e := &domain.UpgradeEdge{
		SchemaVersion: domain.UpgradeEdgeSchemaVersion,
		Product:       domain.ProductRef{ID: "cert-manager", Name: "cert-manager"},
		From:          vFrom,
		To:            vTo,
		PathPolicy:    "minor-lineage",
		Path: []domain.PathStep{
			{Version: vMid, PublishedAt: pub(120), Reason: "minor-release"},
			{Version: vTo, PublishedAt: pub(10), Reason: "minor-release"},
		},
		SkippedReleases: []domain.Version{domain.MustVersion("v1.17.1", "1.17.1")},
		Sources: []domain.SourceStatus{
			{SourceID: "gh-releases", Kind: "github-releases", Roles: []domain.SourceRole{domain.RoleVersions, domain.RoleReleaseNotes},
				State: domain.SourceOK, URI: "https://github.com/cert-manager/cert-manager/releases"},
			{SourceID: "images", Kind: "oci", Version: "1.19.0", State: domain.SourceUnavailable, Detail: "registry returned 403"},
		},
		Changes: []domain.Change{
			{
				ID: "chg-1", Category: domain.CategoryMigration, Breaking: true, ActionRequired: true,
				Title:   "Certificates are re-keyed on every renewal by default",
				Detail:  "The default of privateKey.rotationPolicy changed to Always; set it to Never to keep the old behaviour.",
				Release: "1.18.0", Subjects: []string{"Certificate.spec.privateKey.rotationPolicy"},
				References: []domain.Reference{{Type: "pull-request", ID: "#7000", URL: "https://github.com/cert-manager/cert-manager/pull/7000"}},
				Provenance: declaredProv("section:/breaking/i"),
				Facts:      []domain.FactID{fNotes.ID},
				Evidence:   []domain.EvidenceID{evNotes.ID},
			},
			{
				ID: "chg-2", Category: domain.CategoryHelmValues, Breaking: true,
				Title:      "Helm value installCRDs was removed",
				Subjects:   []string{"installCRDs"},
				Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: "values.removed", Confidence: domain.ConfidenceHigh},
				Evidence:   []domain.EvidenceID{evValues.ID},
			},
			{
				ID: "chg-3", Category: domain.CategoryDeprecation, Release: "1.18.0",
				Title:      "Flag --enable-certificate-owner-ref is deprecated",
				Provenance: domain.Provenance{Method: domain.MethodHeuristic, Producer: "normalize.notes@v1", Rule: "keyword:deprecated", Confidence: domain.ConfidenceMedium},
				Evidence:   []domain.EvidenceID{evBody.ID},
			},
			{
				ID: "chg-4", Category: domain.CategorySecurity,
				Title:      "Fixes an advisory present in 1.17.0",
				References: []domain.Reference{{Type: "ghsa", ID: "GHSA-xxxx-xxxx-xxxx"}, {Type: "cve", ID: "CVE-2026-0001"}},
				Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: "advisory.fixed", Confidence: domain.ConfidenceHigh},
				Evidence:   []domain.EvidenceID{evAdvisory.ID},
			},
			{
				ID: "chg-5", Category: domain.CategoryMigration, Breaking: true, Release: "1.18.0",
				Title:      "We have changed the default value of Certificate.Spec.PrivateKey.RotationPolicy from Never to Always.",
				Provenance: declaredProv("section:/upgrad/i"),
				Evidence:   []domain.EvidenceID{evGuide.ID},
			},
		},
		Compatibility: []domain.CompatibilityChange{{
			Platform: "kubernetes",
			From: &domain.CompatibilityConstraint{Platform: "kubernetes", Constraint: ">=1.25.0-0 <=1.32.x", Raw: "1.25 - 1.32", Kind: "supported",
				SourceID: "supported-releases", Provenance: declaredProv("table:supported"), Evidence: []domain.EvidenceID{evCompat.ID}},
			To: &domain.CompatibilityConstraint{Platform: "kubernetes", Constraint: ">=1.29.0-0 <=1.33.x", Raw: "1.29 - 1.33", Kind: "supported",
				SourceID: "supported-releases", Provenance: declaredProv("table:supported"), Evidence: []domain.EvidenceID{evCompat.ID}},
			Summary:  "Kubernetes 1.25 - 1.28 are no longer supported; 1.33 is now supported",
			Narrowed: true,
		}},
		Artifacts: []domain.ArtifactChange{
			{
				ArtifactID: "controller-image", Type: domain.ArtifactContainerImage, Name: "cert-manager-controller", Change: domain.ChangeUpdated,
				From: &domain.ArtifactInstance{ArtifactID: "controller-image", Type: domain.ArtifactContainerImage, Name: "cert-manager-controller",
					Coordinate: "quay.io/jetstack/cert-manager-controller:v1.17.0", Version: "v1.17.0", Status: domain.ArtifactReferenced},
				To: &domain.ArtifactInstance{ArtifactID: "controller-image", Type: domain.ArtifactContainerImage, Name: "cert-manager-controller",
					Coordinate: "quay.io/jetstack/cert-manager-controller:v1.19.0", Version: "v1.19.0", Status: domain.ArtifactExpected,
					Detail: "registry unreachable; predicted from the definition"},
			},
			{
				ArtifactID: "chart", Type: domain.ArtifactHelmChart, Name: "cert-manager", Change: domain.ChangeUnchanged,
				To: &domain.ArtifactInstance{ArtifactID: "chart", Type: domain.ArtifactHelmChart, Name: "cert-manager",
					Coordinate: chart + "/cert-manager", Version: "v1.19.0", Digest: "sha256:aaaa", Channel: "helm-repo",
					Status: domain.ArtifactVerified, Evidence: []domain.EvidenceID{evChart.ID}},
				Evidence: []domain.EvidenceID{evChart.ID},
			},
		},
		Enrichments: []domain.Enrichment{
			{
				ID: "enr-1", Kind: domain.EnrichmentCluster, Title: "Private keys are rotated on every renewal by default",
				Content:   "The release notes and the upgrade guide state the same change: rotationPolicy now defaults to Always; set it to Never to keep today's behaviour.",
				RelatesTo: []string{"chg-1", "chg-5"},
				Citations: []domain.EvidenceID{evNotes.ID, evGuide.ID},
				Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "enrich@v1", Rule: "group:cand-1", Confidence: domain.ConfidenceMedium,
					Model: "fake-model", ModelVersion: "fake-model-2026-01", PromptVersion: "enrich/v1", PromptDigest: "sha256:0123456789abcdef",
					InputEvidence: []domain.EvidenceID{evNotes.ID, evGuide.ID, evValues.ID}, GeneratedAt: &generated},
			},
			{
				ID: "enr-2", Kind: domain.EnrichmentRelated, Unverified: true,
				Content:   "Removing installCRDs and deprecating the owner-ref flag may both affect how CRDs are managed; verify.",
				RelatesTo: []string{"chg-2", "chg-3"},
				Citations: []domain.EvidenceID{evValues.ID},
				Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "enrich@v1", Rule: "group:cand-2", Confidence: domain.ConfidenceLow,
					Model: "fake-model", ModelVersion: "fake-model-2026-01", PromptVersion: "enrich/v1", PromptDigest: "sha256:fedcba9876543210",
					InputEvidence: []domain.EvidenceID{evValues.ID, evBody.ID}, GeneratedAt: &generated},
			},
		},
		EnrichmentRun: &domain.EnrichmentRun{
			Producer: "enrich@v1", PromptVersion: "enrich/v1", CandidateGroups: 3, Requests: 3, Pending: 1, Accepted: 2,
			Rejected: []domain.EnrichmentRejection{{Group: "cand-2", PromptDigest: "sha256:fedcba9876543210", Kind: domain.EnrichmentCluster,
				RelatesTo: []string{"chg-2", "chg-9"}, Reason: "unknown change id chg-9"}},
			Clusters: 1, ClusteredChanges: 2, DuplicatesConsolidated: 1,
		},
		Facts:            []domain.Fact{fNotes, fChart},
		Evidence:         []domain.Evidence{evNotes, evValues, evBody, evCompat, evChart, evAdvisory, evGuide},
		Warnings:         []string{"OCI images could not be probed (registry returned 403); image versions are expected, not verified"},
		GeneratedAt:      t0,
		DefinitionDigest: "sha256:def0",
	}
	return e
}

// sampleRelease builds a release carrying every snapshot kind, notes with
// classification, compatibility, artifacts, facts and a partial source.
func sampleRelease() *domain.Release {
	pub := t0.AddDate(0, 0, -10)
	evNotes := domain.NewEvidence(domain.EvidenceDocument, "release-notes", notes, "## Breaking Changes", "The default ...", domain.Digest([]byte("n")), t0)
	evValues := domain.NewEvidence(domain.EvidenceStructured, "chart-values", rawVals, "", "", domain.Digest([]byte("v")), t0)
	evCRD := domain.NewEvidence(domain.EvidenceStructured, "crds", "https://example.org/crds.yaml", "$.spec.versions[1]", "", domain.Digest([]byte("c")), t0)
	evImg := domain.NewEvidence(domain.EvidenceRepoFile, "manifest", "https://example.org/install.yaml", "L10", "image: quay.io/jetstack/cert-manager-controller:v1.19.0", domain.Digest([]byte("i")), t0)

	return &domain.Release{
		Product:     "cert-manager",
		Version:     vTo,
		PublishedAt: &pub,
		Artifacts: []domain.ArtifactInstance{
			{ArtifactID: "controller-image", Type: domain.ArtifactContainerImage, Name: "cert-manager-controller",
				Coordinate: "quay.io/jetstack/cert-manager-controller:v1.19.0", Version: "v1.19.0", Status: domain.ArtifactReferenced,
				Evidence: []domain.EvidenceID{evImg.ID}},
			{ArtifactID: "manifest", Type: domain.ArtifactManifest, Name: "install manifest", Coordinate: "https://example.org/install.yaml",
				Status: domain.ArtifactNotApplicable},
		},
		Notes: []domain.NoteItem{{
			ID: "note-1", Release: "1.19.0", SourceID: "release-notes", Role: domain.RoleReleaseNotes, Section: "Breaking Changes",
			Text: "The default value of privateKey.rotationPolicy is now Always.", Category: domain.CategoryMigration,
			Breaking: true, ActionRequired: true,
			References:     []domain.Reference{{Type: "pull-request", ID: "#7000"}},
			Classification: declaredProv("section:/breaking/i"),
			Evidence:       []domain.EvidenceID{evNotes.ID},
		}},
		Compat: []domain.CompatibilityConstraint{{
			Platform: "kubernetes", Constraint: ">=1.29.0-0 <=1.33.x", Versions: []string{"1.29", "1.30", "1.31", "1.32", "1.33"},
			Raw: "1.29 - 1.33", Kind: "supported", SourceID: "supported-releases", Provenance: declaredProv("table:supported"),
			Evidence: []domain.EvidenceID{evNotes.ID},
		}},
		Snapshots: []domain.Snapshot{
			{ArtifactID: "chart", Kind: domain.SnapshotHelmValues, Evidence: []domain.EvidenceID{evValues.ID},
				Values: &domain.ValuesSnapshot{Chart: "cert-manager", Version: "v1.19.0",
					Entries:  map[string]string{"replicaCount": "1", "image.tag": `""`},
					Comments: map[string]string{"replicaCount": "Number of controller replicas"}}},
			{ArtifactID: "crds", Kind: domain.SnapshotCRDs, Evidence: []domain.EvidenceID{evCRD.ID},
				CRDs: &domain.CRDSnapshot{CRDs: []domain.CRDSummary{{
					Name: "certificates.cert-manager.io", Group: "cert-manager.io", Kind: "Certificate", Scope: "Namespaced",
					Versions: []domain.CRDVersionInfo{
						{Name: "v1", Served: true, Storage: true, SchemaPaths: []string{"spec.secretTemplate.labels"}},
						{Name: "v1alpha2", Served: false, Storage: false, Deprecated: true, DeprecationWarning: "use v1"},
					}}}}},
			{ArtifactID: "manifest", Kind: domain.SnapshotImages, Evidence: []domain.EvidenceID{evImg.ID},
				Images: &domain.ImageRefsSnapshot{Images: []domain.ImageRef{
					{Repository: "quay.io/jetstack/cert-manager-controller", Tag: "v1.19.0"},
					{Repository: "quay.io/jetstack/cert-manager-cainjector", Digest: "sha256:bbbb"},
				}}},
		},
		Facts: []domain.Fact{domain.NewFact(domain.FactSnapshot, "cert-manager chart", "1.19.0", "Helm values were captured", "normalize.values@v1", nil, evValues.ID)},
		Sources: []domain.SourceStatus{
			{SourceID: "release-notes", Kind: "github-releases", Roles: []domain.SourceRole{domain.RoleReleaseNotes}, Version: "1.19.0", State: domain.SourceOK},
			{SourceID: "images", Kind: "oci", Version: "1.19.0", State: domain.SourcePartial, Detail: "2 of 3 tags found"},
		},
		Evidence:         []domain.Evidence{evNotes, evValues, evCRD, evImg},
		IngestedAt:       t0,
		DefinitionDigest: "sha256:def0",
	}
}
