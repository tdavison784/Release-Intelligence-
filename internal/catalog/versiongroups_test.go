package catalog

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func versioningDef(t *testing.T, pattern string) *ProductDefinition {
	t.Helper()
	d, err := Parse([]byte(`apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: x
name: X
versioning:
  scheme: semver
  tagPattern: '` + pattern + `'
sources:
  - id: tags
    roles: [versions]
    locator: {kind: git-tags, repository: example.org/x/x, tagPattern: '` + pattern + `'}
artifacts: []
`))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// Component groups (major/minor/patch/prerelease) are a valid way to read a
// version from a tag, for the product's pattern and for git-tags locators.
func TestVersionComponentGroupsValidate(t *testing.T) {
	d := versioningDef(t, `^REL_(?P<major>\d+)_(?P<patch>\d+)$`)
	if rep := Validate(d); !rep.OK() {
		t.Fatalf("component pattern must validate: %v", rep.Issues)
	}
	for _, i := range Validate(d).Issues {
		if i.Path == "versioning" {
			t.Fatalf("component pattern must not warn about a sample tag: %v", i)
		}
	}
	p, err := d.VersionParser()
	if err != nil {
		t.Fatal(err)
	}
	v, err := p.Parse("REL_17_2")
	if err != nil || v.Semver != "17.0.2" || v.Line() != "17.0" {
		t.Fatalf("parse REL_17_2: %+v %v", v, err)
	}

	bad := versioningDef(t, `^REL_(\d+)_(?P<patch>\d+)$`)
	rep := Validate(bad)
	if rep.OK() {
		t.Fatal("a pattern with neither version nor major group must be rejected")
	}
	found := false
	for _, i := range rep.Errors() {
		if i.Path == "versioning.tagPattern" && strings.Contains(i.Message, "major") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a versioning.tagPattern error mentioning major, got %v", rep.Issues)
	}
}

// Rendering derives line and previous-release context from the assembled
// semantic version, so lineage works for two-part versions (17.2 = 17.0.2).
func TestRenderContextForComponentVersions(t *testing.T) {
	d := versioningDef(t, `^REL_?(?P<major>[89]|\d{2})_(?:(?P<minor>\d+)_)?(?P<patch>\d+)$`)
	p, err := d.VersionParser()
	if err != nil {
		t.Fatal(err)
	}
	vs := knownVersions(t, p, []string{"REL9_6_24", "REL9_6_0", "REL_16_0", "REL_16_1", "REL_17_0", "REL_17_1", "REL_17_2"})
	parse := func(tag string) (rc RenderContext) {
		for _, v := range vs {
			if v.Tag == tag {
				return NewRenderContext("postgresql", v, vs)
			}
		}
		t.Fatalf("unknown %s", tag)
		return
	}
	if rc := parse("REL_17_2"); rc.PrevTag != "REL_17_1" || rc.Line != "17.0" || rc.PrevLine != "16.0" {
		t.Errorf("17.2: prev=%q line=%q prevLine=%q", rc.PrevTag, rc.Line, rc.PrevLine)
	}
	if rc := parse("REL_17_0"); rc.PrevTag != "REL_16_0" {
		t.Errorf("17.0 follows 16.0, got %q", rc.PrevTag)
	}
	if got := ReleaseKindOf(t, p, "REL_17_0"); got != "major" {
		t.Errorf("REL_17_0 kind = %s", got)
	}
	if got := ReleaseKindOf(t, p, "REL_17_2"); got != "patch" {
		t.Errorf("REL_17_2 kind = %s", got)
	}
	if got := ReleaseKindOf(t, p, "REL9_6_0"); got != "minor" {
		t.Errorf("REL9_6_0 kind = %s", got)
	}
	// published-form templates: Docker tags and release headings use "17.2".
	rc := parse("REL_17_2")
	s, err := Render(`{{if ge .Major 10}}{{.Major}}.{{.Patch}}{{else}}{{.Version}}{{end}}`, rc)
	if err != nil || s != "17.2" {
		t.Fatalf("published form of 17.0.2: %q %v", s, err)
	}
	rc96 := parse("REL9_6_24")
	s, err = Render(`{{if ge .Major 10}}{{.Major}}.{{.Patch}}{{else}}{{.Version}}{{end}}`, rc96)
	if err != nil || s != "9.6.24" {
		t.Fatalf("published form of 9.6.24: %q %v", s, err)
	}
}

func knownVersions(t *testing.T, p domain.VersionParser, tags []string) []domain.Version {
	t.Helper()
	var vs []domain.Version
	for _, tag := range tags {
		v, err := p.Parse(tag)
		if err != nil {
			t.Fatal(err)
		}
		vs = append(vs, v)
	}
	return vs
}

func ReleaseKindOf(t *testing.T, p domain.VersionParser, tag string) string {
	t.Helper()
	return ReleaseKind(knownVersions(t, p, []string{tag})[0])
}
