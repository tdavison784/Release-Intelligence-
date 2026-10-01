package catalog

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// sampleWithIgnoreKeys returns the sample definition with ignoreKeys added to
// the helm-values content of the chart and, when onImages is not empty, to
// the image-refs content of the manifest.
func sampleWithIgnoreKeys(t *testing.T, onValues, onImages string) *ProductDefinition {
	t.Helper()
	d, err := LoadFile(filepath.Join("testdata", "sample.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	set := func(artifact, kind, yamlList string) {
		var keys []string
		if yamlList != "" {
			if err := yaml.Unmarshal([]byte(yamlList), &keys); err != nil {
				t.Fatal(err)
			}
		}
		for i := range d.Artifacts {
			for j := range d.Artifacts[i].Contents {
				if d.Artifacts[i].ID == artifact && d.Artifacts[i].Contents[j].Kind == kind {
					d.Artifacts[i].Contents[j].IgnoreKeys = keys
					return
				}
			}
		}
		t.Fatalf("no %s content in %s", kind, artifact)
	}
	set("chart", ContentHelmValues, onValues)
	if onImages != "" {
		set("manifest", ContentImageRefs, onImages)
	}
	return d
}

func TestIgnoreKeysParsesAndValidates(t *testing.T) {
	d, err := Parse([]byte(`apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: x
name: X
versioning: {scheme: semver, tagPrefix: v, lineage: minor}
sources:
  - {id: tags, roles: [versions], locator: {kind: git-tags, repository: github.com/example/x}}
artifacts:
  - id: chart
    type: helm-chart
    name: x
    version: {strategy: template, template: "{{.Tag}}"}
    channels: [{kind: helm-repo, url: "https://charts.example.com", chart: x}]
    contents:
      - kind: helm-values
        stripPrefix: _internal_defaults_do_not_set
        ignoreKeys: [global.hub, global.tag, "image.*"]
        locator: {kind: repo-file, repository: github.com/example/x, path: values.yaml}
`))
	if err != nil {
		t.Fatal(err)
	}
	got := d.Artifacts[0].Contents[0].IgnoreKeys
	if strings.Join(got, ",") != "global.hub,global.tag,image.*" {
		t.Fatalf("IgnoreKeys = %v", got)
	}
	if rep := Validate(d); !rep.OK() {
		t.Fatalf("valid definition rejected: %v", rep.Issues)
	}
	// the field survives a marshal round trip
	b, err := Marshal(d)
	if err != nil || !strings.Contains(string(b), "ignoreKeys:") {
		t.Fatalf("marshal: %v\n%s", err, b)
	}
}

func TestIgnoreKeysOnlyForHelmValues(t *testing.T) {
	d := sampleWithIgnoreKeys(t, "[a.b]", "[a.b]")
	rep := Validate(d)
	var paths []string
	for _, i := range rep.Errors() {
		paths = append(paths, i.Path)
	}
	if len(paths) != 1 || !strings.HasSuffix(paths[0], ".contents[0].ignoreKeys") || !strings.Contains(rep.Errors()[0].Message, "helm-values") {
		t.Fatalf("ignoreKeys on image-refs must be the only error: %v", rep.Issues)
	}
}

func TestIgnoreKeysEntriesAreChecked(t *testing.T) {
	for _, bad := range []string{`[""]`, `["  "]`, `[" a.b"]`, `["*"]`, `[".*"]`, `["a*b"]`, `["a.*.b"]`, `["a.**"]`} {
		rep := Validate(sampleWithIgnoreKeys(t, bad, ""))
		if rep.OK() {
			t.Errorf("ignoreKeys %s must be rejected", bad)
			continue
		}
		if p := rep.Errors()[0].Path; !strings.Contains(p, ".ignoreKeys[0]") {
			t.Errorf("ignoreKeys %s: error at %q", bad, p)
		}
	}
	for _, good := range []string{`[a]`, `[a.b, "a.c.*"]`, `['ingress.annotations["x.y"]']`, `[global.*]`} {
		if rep := Validate(sampleWithIgnoreKeys(t, good, "")); !rep.OK() {
			t.Errorf("ignoreKeys %s must be accepted: %v", good, rep.Issues)
		}
	}
}

func TestIgnoreKeysInJSONSchema(t *testing.T) {
	schema := compileSchema(t, filepath.Join("..", "..", "schemas", "product-definition.schema.json"))
	check := func(d *ProductDefinition) error {
		b, err := Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		var doc any
		if err := yaml.Unmarshal(b, &doc); err != nil {
			t.Fatal(err)
		}
		j, _ := json.Marshal(doc)
		inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(j)))
		if err != nil {
			t.Fatal(err)
		}
		return schema.Validate(inst)
	}
	if err := check(sampleWithIgnoreKeys(t, `[global.hub, "image.*"]`, "")); err != nil {
		t.Errorf("the schema must accept ignoreKeys: %v", err)
	}
	if err := check(sampleWithIgnoreKeys(t, `[""]`, "")); err == nil {
		t.Error("the schema must reject an empty ignoreKeys entry")
	}
}
