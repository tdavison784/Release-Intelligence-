package catalog

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// fieldDef parses a definition whose single artifact uses version.strategy
// "field" with the given version block.
func fieldDef(t *testing.T, versionYAML string) *ProductDefinition {
	t.Helper()
	d, err := Parse([]byte(`apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: x
name: X
versioning: {scheme: semver, tagPrefix: chart-}
sources:
  - {id: tags, roles: [versions], locator: {kind: git-tags, repository: example.org/x/x}}
artifacts:
  - id: dep
    type: helm-chart
    name: kube-state-metrics
    version:
` + versionYAML + `
    channels:
      - {kind: oci, repository: ghcr.io/prometheus-community/charts/kube-state-metrics}
`))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

const goodFieldVersion = `      strategy: field
      field: dependencies[name=kube-state-metrics].version
      from:
        kind: repo-file
        repository: example.org/x/x
        ref: "{{.Tag}}"
        path: charts/stack/Chart.yaml`

// version.strategy "field" reads the artifact version out of a YAML document
// at the release ref (the pinned sub-components of an aggregating chart).
func TestVersionFieldStrategyValidates(t *testing.T) {
	d := fieldDef(t, goodFieldVersion)
	if rep := Validate(d); !rep.OK() {
		t.Fatalf("field strategy must validate: %v", rep.Issues)
	}

	for name, c := range map[string]struct {
		version, path, want string
	}{
		"no field":     {"      strategy: field", "artifacts[0].version.field", "required for strategy field"},
		"no from":      {"      strategy: field\n      field: appVersion", "artifacts[0].version.from", "required for strategy field"},
		"from kind":    {"      strategy: field\n      field: appVersion\n      from: {kind: repo-dir, repository: example.org/x/x, path: charts}", "artifacts[0].version.from.kind", "one document"},
		"from missing path": {"      strategy: field\n      field: appVersion\n      from: {kind: repo-file, repository: example.org/x/x}", "artifacts[0].version.from.path", "required for kind"},
		"bad path":     {strings.Replace(goodFieldVersion, "dependencies[name=kube-state-metrics].version", "dependencies[name]", 1), "artifacts[0].version.field", "must be [field=value]"},
		"lookup leftovers": {goodFieldVersion + "\n      match: \"{{.Tag}}\"", "artifacts[0].version", "belong to other strategies"},
		"unknown strategy": {"      strategy: yaml", "artifacts[0].version.strategy", "must be template, lookup, field or independent"},
	} {
		t.Run(name, func(t *testing.T) {
			rep := Validate(fieldDef(t, c.version))
			if rep.OK() {
				t.Fatalf("must be rejected:\n%s", c.version)
			}
			found := false
			for _, i := range rep.Errors() {
				if i.Path == c.path && strings.Contains(i.Message, c.want) {
					found = true
				}
			}
			if !found {
				t.Fatalf("want an error at %s mentioning %q; got %v", c.path, c.want, rep.Errors())
			}
		})
	}
}

// the definition round-trips through the JSON Schema (raw documents, so a
// misspelled key can be handed to the schema before the strict YAML decode)
func TestVersionFieldStrategySchema(t *testing.T) {
	schema := compileSchema(t, filepath.Join("..", "..", "schemas", "product-definition.schema.json"))
	const doc = `apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: x
name: X
versioning: {scheme: semver, tagPrefix: chart-}
sources:
  - {id: tags, roles: [versions], locator: {kind: git-tags, repository: example.org/x/x}}
artifacts:
  - id: dep
    type: helm-chart
    name: kube-state-metrics
    version:
` + goodFieldVersion + `
    channels:
      - {kind: oci, repository: ghcr.io/prometheus-community/charts/kube-state-metrics}
`
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
		t.Fatalf("field strategy must satisfy the JSON Schema: %v", err)
	}
	bad := strings.Replace(doc, "      from:", "      frm:", 1)
	if err := check(bad); err == nil {
		t.Fatal("a misspelled from key must fail additionalProperties: false")
	}
}

func TestSplitYAMLPath(t *testing.T) {
	for _, ok := range []string{
		"appVersion",
		"dependencies[name=kube-state-metrics].version",
		"prometheus.prometheusSpec.image.tag",
		"a.b[name=x][other=y].c",
	} {
		if _, err := SplitYAMLPath(ok); err != nil {
			t.Errorf("SplitYAMLPath(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{
		"", " a", "a.", ".a", "dependencies[]", "dependencies[name]", "dependencies[name=]",
		"a[name=x]b.c", "dependencies[name=", "a]b",
	} {
		if _, err := SplitYAMLPath(bad); err == nil {
			t.Errorf("SplitYAMLPath(%q): must be rejected", bad)
		}
	}
	segs, err := SplitYAMLPath("dependencies[name=kube-state-metrics][alias=k].version")
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 2 || segs[0].Key != "dependencies" || len(segs[0].Selectors) != 2 ||
		segs[0].Selectors[0].Field != "name" || segs[0].Selectors[0].Value != "kube-state-metrics" ||
		segs[0].Selectors[1].Value != "k" || segs[1].Key != "version" {
		t.Fatalf("unexpected segments %+v", segs)
	}
}
