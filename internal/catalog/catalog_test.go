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
