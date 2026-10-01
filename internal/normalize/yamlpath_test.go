package normalize

import (
	"errors"
	"strings"
	"testing"
)

const chartYAMLFixture = `apiVersion: v2
name: kube-prometheus-stack
version: 91.8.2
appVersion: v0.94.1
kubeVersion: ">=1.25.0-0"
dependencies:
  - name: crds
    version: "0.0.0"
    condition: crds.enabled
  - name: kube-state-metrics
    version: "8.6.0"
    repository: oci://ghcr.io/prometheus-community/charts
  - name: grafana
    version: "13.2.7"
    repository: oci://ghcr.io/grafana-community/helm-charts
`

const valuesYAMLFixture = `defaultRules:
  create: true
prometheus:
  prometheusSpec:
    image:
      registry: quay.io
      repository: prometheus/prometheus
      tag: v3.15.0-distroless
      sha: ""
`

func TestReadYAMLPathScalars(t *testing.T) {
	cases := []struct {
		path  string
		want  string
		line  int
		fails bool
	}{
		{path: "appVersion", want: "v0.94.1", line: 4},
		{path: "version", want: "91.8.2", line: 3},
		{path: "kubeVersion", want: ">=1.25.0-0", line: 5},
		{path: "name", want: "kube-prometheus-stack", line: 2},
		// values-style nesting
		{path: "prometheus.prometheusSpec.image.tag", want: "", fails: true}, // different document
	}
	for _, c := range cases {
		got, line, err := ReadYAMLPath([]byte(chartYAMLFixture), c.path)
		if c.fails {
			if err == nil {
				t.Errorf("ReadYAMLPath(%q): expected an error, got %q", c.path, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ReadYAMLPath(%q): %v", c.path, err)
			continue
		}
		if got != c.want || line != c.line {
			t.Errorf("ReadYAMLPath(%q) = %q, line %d; want %q, line %d", c.path, got, line, c.want, c.line)
		}
	}
}

func TestReadYAMLPathNested(t *testing.T) {
	got, _, err := ReadYAMLPath([]byte(valuesYAMLFixture), "prometheus.prometheusSpec.image.tag")
	if err != nil {
		t.Fatalf("nested path: %v", err)
	}
	if got != "v3.15.0-distroless" {
		t.Fatalf("nested path = %q", got)
	}
	if _, _, err := ReadYAMLPath([]byte(valuesYAMLFixture), "prometheus.prometheusSpec.image.digest"); err == nil {
		t.Fatal("missing key: expected an error")
	}
}

func TestReadYAMLPathListSelector(t *testing.T) {
	for _, c := range []struct{ path, want string }{
		{"dependencies[name=kube-state-metrics].version", "8.6.0"},
		{"dependencies[name=grafana].version", "13.2.7"},
		{"dependencies[name=grafana].repository", "oci://ghcr.io/grafana-community/helm-charts"},
		{"dependencies[name=crds].condition", "crds.enabled"},
	} {
		got, _, err := ReadYAMLPath([]byte(chartYAMLFixture), c.path)
		if err != nil {
			t.Errorf("ReadYAMLPath(%q): %v", c.path, err)
			continue
		}
		if got != c.want {
			t.Errorf("ReadYAMLPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestReadYAMLPathErrors(t *testing.T) {
	doc := []byte(chartYAMLFixture)
	cases := []struct{ path, wantErr string }{
		{"", "empty"},
		{"appVersion.", "empty segment"},
		{"dependencies[name=kube-state-metrics]", "not a scalar"},
		{"dependencies[name=thanos].version", "no element with name=\"thanos\""},
		{"nope", "no key \"nope\""},
		{"dependencies[0]", "must be [field=value]"},
		{"dependencies[name=", "unterminated"},
		{"a[name=x]b.c", "misplaced bracket"},
	}
	for _, c := range cases {
		_, _, err := ReadYAMLPath(doc, c.path)
		if err == nil {
			t.Errorf("ReadYAMLPath(%q): expected error", c.path)
			continue
		}
		if !errors.Is(err, ErrNoMatch) && !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("ReadYAMLPath(%q) error %q does not mention %q", c.path, err, c.wantErr)
		}
	}
	// every "no match" error wraps ErrNoMatch
	for _, path := range []string{"nope", "dependencies[name=thanos].version", "dependencies[name=kube-state-metrics]"} {
		_, _, err := ReadYAMLPath(doc, path)
		if !errors.Is(err, ErrNoMatch) {
			t.Errorf("ReadYAMLPath(%q): error should wrap ErrNoMatch, got %v", path, err)
		}
	}
	// broken YAML is a parse error, not ErrNoMatch
	if _, _, err := ReadYAMLPath([]byte("a: [unclosed"), "a"); err == nil || errors.Is(err, ErrNoMatch) {
		t.Errorf("broken YAML: want parse error, got %v", err)
	}
	// quoted values lose their quotes: the value is the scalar's text
	if got, _, err := ReadYAMLPath([]byte("tag: \"8.6.0\"\n"), "tag"); err != nil || got != "8.6.0" {
		t.Errorf("quoted scalar = %q, err %v; want 8.6.0", got, err)
	}
}
