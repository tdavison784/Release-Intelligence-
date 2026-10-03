package catalog

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

var ref = []ConfigReference{{URL: "https://example.io/docs/config", Quote: "The server reads config.yaml."}}

func configDef(cs ...ConfigSource) *ProductDefinition {
	d := lifecycleDef()
	d.ConfigSources = cs
	return d
}

func TestConfigSourcesValid(t *testing.T) {
	d := configDef(
		ConfigSource{ID: "server-config", Channel: ChannelConfigMapFile, Component: "server", Summary: "main config", File: "config.yaml", Format: "yaml", ConfigMap: "demo-config", References: ref},
		ConfigSource{ID: "chart", Channel: ChannelHelmValues, Summary: "chart values", ValuesPath: ".", References: ref},
		ConfigSource{ID: "flags", Channel: ChannelCLIFlags, Component: "server", Summary: "flags", References: ref},
		ConfigSource{ID: "gates", Channel: ChannelFeatureGates, Summary: "gates", Flag: "--feature-gates", ValuesPath: "featureGates", References: ref},
		ConfigSource{ID: "global", Channel: ChannelCustomResource, Summary: "global CR", Resource: &ConfigResource{Group: "demo.io", Kind: "Config"}, References: ref},
	)
	if errs := lifecycleErrors(d); len(errs) != 0 {
		t.Fatalf("unexpected errors %v", errs)
	}
}

func TestConfigSourcesValidationErrors(t *testing.T) {
	d := configDef(
		ConfigSource{ID: "Bad_ID", Channel: "secrets", Summary: " ", References: nil},
		ConfigSource{ID: "cm", Channel: ChannelConfigMapFile, Summary: "x", Format: "xml", Flag: "--x", References: ref},
		ConfigSource{ID: "cm", Channel: ChannelCLIFlags, Summary: "x", References: []ConfigReference{
			{URL: "http://example.io/docs", Quote: "q"},
			{URL: "https://github.com/me/repo/blob/main/eval/cases/x/case.yaml", Quote: ""},
		}},
		ConfigSource{ID: "gates", Channel: ChannelFeatureGates, Summary: "x", Flag: "feature-gates", References: ref},
	)
	got := lifecycleErrors(d)
	for _, w := range []string{
		"configSources[0].id", "configSources[0].channel", "configSources[0].summary", "configSources[0].references",
		"configSources[1].file", "configSources[1].format", "configSources[1].flag",
		"configSources[2].id", "configSources[2].references[0].url", "configSources[2].references[1].url", "configSources[2].references[1].quote",
		"configSources[3].flag",
	} {
		if !got[w] {
			t.Errorf("expected an error at %s, got %v", w, got)
		}
	}
}

func TestConfigSourcesSchema(t *testing.T) {
	schema := compileSchema(t, filepath.Join("..", "..", "schemas", "product-definition.schema.json"))
	doc := `apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: demo
name: demo
versioning: {scheme: semver, tagPrefix: v}
sources:
  - id: tags
    roles: [versions]
    locator: {kind: git-tags, repository: github.com/o/r}
artifacts: []
configSources:
  - id: server-config
    channel: configmap-file
    summary: main config
    file: config.yaml
    format: yaml
    references:
      - url: https://example.io/docs/config
        quote: The server reads config.yaml.
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
		t.Fatalf("schema rejects a valid config source: %v", err)
	}
	d, err := Parse([]byte(doc))
	if err != nil || len(d.ConfigSources) != 1 || d.ConfigSources[0].File != "config.yaml" {
		t.Fatalf("parse: %v %+v", err, d)
	}
	for name, bad := range map[string]string{
		"channel":    strings.Replace(doc, "channel: configmap-file", "channel: secret", 1),
		"references": strings.Replace(doc, "    references:\n      - url: https://example.io/docs/config\n        quote: The server reads config.yaml.\n", "    references: []\n", 1),
		"http":       strings.Replace(doc, "https://example.io", "http://example.io", 1),
		"extra":      strings.Replace(doc, "summary: main config", "summary: main config\n    bogus: 1", 1),
	} {
		if check(bad) == nil {
			t.Errorf("schema accepts %s", name)
		}
	}
}
