package catalog

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func lifecycleDef(l ...Lifecycle) *ProductDefinition {
	return &ProductDefinition{
		APIVersion: APIVersion, Kind: Kind, ID: "demo", Name: "demo",
		Versioning: Versioning{Scheme: domain.SchemeSemver, TagPrefix: "v"},
		Sources: []Source{
			{ID: "tags", Roles: []domain.SourceRole{domain.RoleVersions}, Locator: Locator{Kind: LocatorGitTags, Repository: "github.com/o/r"}},
			{ID: "banner", Roles: []domain.SourceRole{domain.RoleReleaseNotes}, Locator: Locator{Kind: LocatorRepoFile, Repository: "github.com/o/r", Path: "README.md"}},
			{ID: "compat", Roles: []domain.SourceRole{domain.RoleCompatibility}, Locator: Locator{Kind: LocatorRepoFile, Repository: "github.com/o/r", Path: "COMPAT.md"}},
		},
		Lifecycle: l,
	}
}

func lifecycleErrors(d *ProductDefinition) map[string]bool {
	got := map[string]bool{}
	for _, i := range Validate(d).Errors() {
		got[i.Path] = true
	}
	return got
}

func TestLifecycleValid(t *testing.T) {
	for _, l := range []Lifecycle{
		{State: LifecycleEndOfLife, Since: "2026-03", Summary: "retired", Sources: []string{"banner"}},
		{State: LifecycleDeprecated, Since: "2026-03-24", Versions: "< 2.0.0", Summary: "old line", Sources: []string{"banner"}},
		{State: LifecycleEndOfLife, Summary: "no date, all versions", Sources: []string{"banner"}},
	} {
		if errs := lifecycleErrors(lifecycleDef(l)); len(errs) != 0 {
			t.Errorf("%+v: unexpected errors %v", l, errs)
		}
	}
}

func TestLifecycleValidationErrors(t *testing.T) {
	d := lifecycleDef(
		Lifecycle{State: "gone", Since: "March 2026", Versions: "not a constraint", Summary: " ", Sources: nil},
		Lifecycle{State: LifecycleEndOfLife, Since: "2026-13", Summary: "x", Sources: []string{"missing", "compat"}},
	)
	got := lifecycleErrors(d)
	for _, w := range []string{
		"lifecycle[0].state", "lifecycle[0].since", "lifecycle[0].versions", "lifecycle[0].summary", "lifecycle[0].sources",
		"lifecycle[1].since", "lifecycle[1].sources[0]", "lifecycle[1].sources[1]",
	} {
		if !got[w] {
			t.Errorf("expected an error at %s, got %v", w, got)
		}
	}
}

func TestLifecycleAffectsAndSources(t *testing.T) {
	l := Lifecycle{State: LifecycleEndOfLife, Versions: "< 1.13.0", Summary: "s", Sources: []string{"banner"}}
	for v, want := range map[string]bool{"1.12.8": true, "1.13.0": false, "1.13.0-beta.0": false, "1.12.0-beta.0": true} {
		got, err := l.Affects(domain.MustVersion("v"+v, v))
		if err != nil || got != want {
			t.Errorf("Affects(%s) = %v, %v; want %v", v, got, err, want)
		}
	}
	all := Lifecycle{State: LifecycleDeprecated, Summary: "s", Sources: []string{"banner"}}
	if ok, _ := all.Affects(domain.MustVersion("v0.0.1", "0.0.1")); !ok {
		t.Error("an empty versions constraint applies to every release")
	}
	d := lifecycleDef(l)
	if !d.IsLifecycleSource("banner") || d.IsLifecycleSource("tags") {
		t.Error("IsLifecycleSource")
	}
}

func TestLifecycleParseAndSchema(t *testing.T) {
	const doc = `
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: demo
name: demo
versioning: {scheme: semver, tagPrefix: v}
sources:
  - {id: tags, roles: [versions], locator: {kind: git-tags, repository: github.com/o/r}}
  - {id: banner, roles: [release-notes], locator: {kind: repo-file, repository: github.com/o/r, path: README.md}}
lifecycle:
  - state: end-of-life
    since: "2026-03"
    versions: "< 2.0.0"
    summary: retired
    sources: [banner]
artifacts: []
`
	d, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Lifecycle) != 1 || d.Lifecycle[0].State != LifecycleEndOfLife || d.Lifecycle[0].Versions != "< 2.0.0" {
		t.Fatalf("lifecycle not parsed: %+v", d.Lifecycle)
	}
	if !Validate(d).OK() {
		t.Fatalf("invalid: %v", Validate(d).Issues)
	}
	schema := compileSchema(t, filepath.Join("..", "..", "schemas", "product-definition.schema.json"))
	check := func(doc string) error {
		var v any
		if err := yaml.Unmarshal([]byte(doc), &v); err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(v)
		inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(b)))
		if err != nil {
			t.Fatal(err)
		}
		return schema.Validate(inst)
	}
	if err := check(doc); err != nil {
		t.Fatalf("schema rejects a valid lifecycle: %v", err)
	}
	for name, bad := range map[string]string{
		"state":   strings.Replace(doc, "state: end-of-life", "state: gone", 1),
		"since":   strings.Replace(doc, `since: "2026-03"`, `since: "March 2026"`, 1),
		"sources": strings.Replace(doc, "sources: [banner]", "sources: []", 1),
		"extra":   strings.Replace(doc, "summary: retired", "summary: retired\n    bogus: 1", 1),
	} {
		if check(bad) == nil {
			t.Errorf("schema accepts an invalid lifecycle (%s)", name)
		}
	}
}
