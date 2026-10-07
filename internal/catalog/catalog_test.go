package catalog

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestLoadAndValidateSample(t *testing.T) {
	d, err := LoadFile("testdata/sample.yaml")
	if err != nil {
		t.Fatal(err)
	}
	rep := Validate(d)
	if !rep.OK() {
		t.Fatalf("expected valid definition, got %v", rep.Issues)
	}
	if d.Digest() == "" {
		t.Fatal("digest not set")
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	_, err := Parse([]byte("apiVersion: ri.dev/v1alpha1\nkind: ProductDefinition\nid: x\nname: x\nbogus: 1\n"))
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestValidateCatchesErrors(t *testing.T) {
	d := &ProductDefinition{
		APIVersion: APIVersion, Kind: Kind, ID: "Bad_ID", Name: "x",
		Versioning: Versioning{Scheme: "calver"},
		Sources:    []Source{{ID: "s", Roles: []domain.SourceRole{"nope"}, Locator: Locator{Kind: "repo-file"}}},
		Artifacts: []Artifact{{
			ID: "img", Type: domain.ArtifactContainerImage, Name: "img",
			Version:    VersionRelation{Strategy: "template", Template: "{{.Nope}}"},
			Channels:   []Locator{{Kind: LocatorHTTP, URL: "https://x"}},
			References: []ArtifactReference{{Artifact: "missing", Pattern: "x"}},
		}},
	}
	rep := Validate(d)
	want := []string{"id", "versioning.scheme", "sources[0].roles[0]", "sources[0].locator.repository",
		"sources[0].locator.path", "sources", "artifacts[0].version.template", "artifacts[0].channels[0].kind",
		"artifacts[0].references[0].artifact"}
	got := map[string]bool{}
	for _, i := range rep.Errors() {
		got[i.Path] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("expected error at %s; issues: %v", w, rep.Issues)
		}
	}
}

func TestRenderContextPrevLine(t *testing.T) {
	known := []domain.Version{
		domain.MustVersion("v2.14.11", "2.14.11"),
		domain.MustVersion("v2.13.0", "2.13.0"),
		domain.MustVersion("v3.0.0-rc1", "3.0.0-rc1"),
		domain.MustVersion("v3.0.0", "3.0.0"),
	}
	rc := NewRenderContext("argo-cd", domain.MustVersion("v3.0.0", "3.0.0"), known)
	if rc.PrevLine != "2.14" || rc.Line != "3.0" {
		t.Fatalf("got prev=%q line=%q", rc.PrevLine, rc.Line)
	}
	s, err := Render("upgrading/{{.PrevLine}}-{{.Line}}.md", rc)
	if err != nil || s != "upgrading/2.14-3.0.md" {
		t.Fatalf("render: %q %v", s, err)
	}
	if rc.PrevTag != "" {
		t.Fatalf("v2.14.0 is unknown so PrevTag must be empty, got %q", rc.PrevTag)
	}
	p := NewRenderContext("argo-cd", domain.MustVersion("v2.14.11", "2.14.11"), append(known, domain.MustVersion("v2.14.10", "2.14.10")))
	if p.PrevTag != "v2.14.10" {
		t.Fatalf("patch prev tag: %q", p.PrevTag)
	}
	rc2 := NewRenderContext("x", domain.MustVersion("v1.18.0", "1.18.0"), nil)
	if rc2.PrevLine != "1.17" || rc2.PrevTag != "v1.17.0" {
		t.Fatalf("fallback prev line: %q", rc2.PrevLine)
	}
	h, _ := Render(`^v?{{regexQuote .Version}}$`, rc2)
	if h != `^v?1\.18\.0$` {
		t.Fatalf("regexQuote: %q", h)
	}
}

// The first release of a product has no previous line: it must not be
// invented when the known versions are given.
func TestRenderContextFirstReleaseHasNoPrevLine(t *testing.T) {
	known := []domain.Version{
		domain.MustVersion("v0.3.0", "0.3.0"),
		domain.MustVersion("v0.3.1", "0.3.1"),
		domain.MustVersion("v0.4.0", "0.4.0"),
	}
	first := NewRenderContext("x", known[0], known)
	if first.PrevLine != "" || first.PrevMajor != 0 || first.PrevMinor != 0 || first.PrevTag != "" || first.PrevVersion != "" {
		t.Fatalf("first release must have empty Prev* fields: %+v", first)
	}
	// a patch of the first line still has its previous patch
	if p := NewRenderContext("x", known[1], known); p.PrevLine != "" || p.PrevTag != "v0.3.0" {
		t.Fatalf("patch of the first line: line=%q tag=%q", p.PrevLine, p.PrevTag)
	}
	// later lines are unaffected
	if next := NewRenderContext("x", known[2], known); next.PrevLine != "0.3" || next.PrevTag != "v0.3.0" {
		t.Fatalf("second line: %+v", next)
	}
	// without any list the fallback to Minor-1 remains
	if fb := NewRenderContext("x", known[0], nil); fb.PrevLine != "0.2" || fb.PrevTag != "v0.2.0" {
		t.Fatalf("fallback without known versions: %+v", fb)
	}
}

func TestAppliesTo(t *testing.T) {
	v := domain.MustVersion("v1.18.2", "1.18.2")
	ok, _ := AppliesTo(v, ">= 1.15.0", []string{"minor"})
	if ok {
		t.Fatal("patch release should not match minor-only source")
	}
	ok, _ = AppliesTo(v, "< 1.18.0", nil)
	if ok {
		t.Fatal("availability should exclude")
	}
	ok, _ = AppliesTo(domain.MustVersion("v2.0.0", "2.0.0"), "", []string{"major"})
	if !ok {
		t.Fatal("major release should match")
	}
}

func TestAppliesToPrereleaseUsesReleaseVersion(t *testing.T) {
	alpha := domain.MustVersion("v1.19.0-alpha.0", "1.19.0-alpha.0")
	if ok, err := AppliesTo(alpha, ">= 1.5.0", nil); err != nil || !ok {
		t.Fatalf("1.19.0-alpha.0 must satisfy >= 1.5.0: %v %v", ok, err)
	}
	pre := domain.MustVersion("v1.21.0-alpha.1", "1.21.0-alpha.1")
	if ok, _ := AppliesTo(pre, ">= 1.21.0", nil); !ok {
		t.Error("1.21.0-alpha.1 must satisfy >= 1.21.0")
	}
	if ok, _ := AppliesTo(pre, "< 1.21.0", nil); ok {
		t.Error("1.21.0-alpha.1 must not satisfy < 1.21.0")
	}
}

// The column kind vocabulary is closed: supported/tested/minimum/maximum
// (maximum added for upper-bounded support matrices, e.g. Karpenter's
// maxK8sVersion; first used by products/karpenter.yaml).
func TestValidateColumnKindVocabulary(t *testing.T) {
	mk := func(kind string) *ProductDefinition {
		return &ProductDefinition{
			APIVersion: APIVersion, Kind: Kind, ID: "x", Name: "x",
			Versioning: Versioning{Scheme: "semver"},
			Sources: []Source{
				{ID: "t", Roles: []domain.SourceRole{domain.RoleVersions}, Locator: Locator{Kind: LocatorGitTags, Repository: "example.com/a/b"}},
				{ID: "c", Roles: []domain.SourceRole{domain.RoleCompatibility},
					Locator: Locator{Kind: LocatorRepoFile, Repository: "example.com/a/b", Ref: "{{.Tag}}", Path: "compat.yaml"},
					Extract: &Extract{Type: ExtractYAMLRecords, KeyColumns: []string{"appVersion"}, KeyMatch: ".",
						Columns: []ColumnSpec{{Platform: "kubernetes", Kind: kind, Headers: []string{"minK8sVersion"}}}}}},
		}
	}
	for _, kind := range []string{"", "supported", "tested", "minimum", "maximum"} {
		if rep := Validate(mk(kind)); len(rep.Errors()) != 0 {
			t.Errorf("kind %q: unexpected errors %v", kind, rep.Errors())
		}
	}
	if rep := Validate(mk("at-most")); len(rep.Errors()) == 0 {
		t.Errorf("kind \"at-most\": expected an error")
	}
}

// collect/where select operand-version sets from yaml-records (Strimzi's
// kafka-versions.yaml); collect needs no key, other modes still do.
func TestValidateCollectAndReduce(t *testing.T) {
	mk := func(ex Extract) *ProductDefinition {
		return &ProductDefinition{
			APIVersion: APIVersion, Kind: Kind, ID: "x", Name: "x",
			Versioning: Versioning{Scheme: "semver"},
			Sources: []Source{
				{ID: "t", Roles: []domain.SourceRole{domain.RoleVersions}, Locator: Locator{Kind: LocatorGitTags, Repository: "example.com/a/b"}},
				{ID: "c", Roles: []domain.SourceRole{domain.RoleCompatibility},
					Locator: Locator{Kind: LocatorRepoFile, Repository: "example.com/a/b", Ref: "{{.Tag}}", Path: "v.yaml"},
					Extract: &ex}},
		}
	}
	cols := func(reduce string) []ColumnSpec {
		return []ColumnSpec{{Platform: "kafka", Headers: []string{"version"}, Reduce: reduce}}
	}
	ok := Extract{Type: ExtractYAMLRecords, Collect: true, Where: map[string]string{"supported": "^true$"}, Columns: cols("minor")}
	if rep := Validate(mk(ok)); len(rep.Errors()) != 0 {
		t.Errorf("collect: unexpected errors %v", rep.Errors())
	}
	for name, ex := range map[string]Extract{
		"no key without collect": {Type: ExtractYAMLRecords, Columns: cols("")},
		"bad reduce":             {Type: ExtractYAMLRecords, Collect: true, Columns: cols("patch")},
		"bad where regex":        {Type: ExtractYAMLRecords, Collect: true, Where: map[string]string{"a": "("}, Columns: cols("")},
		"collect on a table":     {Type: ExtractMarkdownTable, Collect: true, Columns: cols("")},
	} {
		if rep := Validate(mk(ex)); len(rep.Errors()) == 0 {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestValidateLinesContent(t *testing.T) {
	mk := func(c Content) *ProductDefinition {
		return &ProductDefinition{
			APIVersion: APIVersion, Kind: Kind, ID: "x", Name: "x",
			Versioning: Versioning{Scheme: "semver"},
			Sources:    []Source{{ID: "t", Roles: []domain.SourceRole{domain.RoleVersions}, Locator: Locator{Kind: LocatorGitTags, Repository: "example.com/a/b"}}},
			Artifacts: []Artifact{{ID: "doc", Type: domain.ArtifactDocumentation, Name: "doc",
				Version:  VersionRelation{Strategy: VersionTemplate, Template: "{{.Tag}}"},
				Channels: []Locator{{Kind: LocatorRepoFile, Repository: "example.com/a/b", Ref: "{{.Tag}}", Path: "x.md"}},
				Contents: []Content{c}}},
		}
	}
	if rep := Validate(mk(Content{Kind: ContentLines, Pattern: `(?i)removed`, Label: "page"})); len(rep.Errors()) != 0 {
		t.Errorf("valid lines content: %v", rep.Errors())
	}
	for name, c := range map[string]Content{
		"no pattern":              Content{Kind: ContentLines},
		"invalid pattern":         Content{Kind: ContentLines, Pattern: "("},
		"pattern on another kind": Content{Kind: ContentCRDs, Pattern: "x"},
		"label on another kind":   Content{Kind: ContentImageRefs, Label: "x"},
		"stripPrefix on lines":    Content{Kind: ContentLines, Pattern: "x", StripPrefix: "a"},
	} {
		if rep := Validate(mk(c)); len(rep.Errors()) == 0 {
			t.Errorf("%s: expected an error", name)
		}
	}
}
