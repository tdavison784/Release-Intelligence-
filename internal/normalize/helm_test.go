package normalize

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestParseChartMetadata(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    ChartMetadata
		wantErr bool
	}{
		{name: "full", in: "apiVersion: v2\nname: argo-cd\nversion: 10.9.5\nappVersion: v3.5.3\nkubeVersion: \">=1.25.0-0\"\ndescription: x\n",
			want: ChartMetadata{Name: "argo-cd", Version: "10.9.5", AppVersion: "v3.5.3", KubeVersion: ">=1.25.0-0"}},
		{name: "numbers are strings", in: "name: c\nversion: 3\nappVersion: 1.16\n", want: ChartMetadata{Name: "c", Version: "3", AppVersion: "1.16"}},
		{name: "minimal", in: "name: c\n", want: ChartMetadata{Name: "c"}},
		{name: "invalid yaml", in: "name: [unclosed", wantErr: true},
		{name: "not a chart", in: "foo: bar\n", wantErr: true},
		{name: "empty", in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseChartMetadata([]byte(tt.in))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %+v", got)
				}
				return
			}
			if err != nil || *got != tt.want {
				t.Errorf("got %+v err=%v, want %+v", got, err, tt.want)
			}
		})
	}
	// the real cert-manager chart template has placeholder versions
	m, err := ParseChartMetadata(fixture(t, "certmanager/Chart.template.yaml"))
	if err != nil || m.Name != "cert-manager" || m.Version != "v0.0.0" || m.AppVersion != "v0.0.0" || m.KubeVersion != ">= 1.22.0-0" {
		t.Errorf("%+v %v", m, err)
	}
}

func TestValuesSnapshot(t *testing.T) {
	src := `# Top-level document comment

# -- Number of replicas
# @default -- 1
replicaCount: 1
image:
  # -- The image repository
  repository: quay.io/example/app
  tag: ""
  pullPolicy: IfNotPresent
  digest: null
flags: [--a, --b]
extraEnv:
  - name: X
    value: "1"
    extra: {b: 2, a: 1}
empty: {}
emptyList: []
ratio: 0.5
big: 9007199254740993
enabled: true
html: "<b>&</b>"
annotations:
  nginx.ingress.kubernetes.io/rewrite-target: /
  "with space": yes
  plain: v
  nested.key:
    leaf: 1
"a.b":
  c: 2
anchor: &anc
  one: 1
alias: *anc
merged:
  <<: *anc
  two: 2
# +docs:property
# @param bitnami Bitnami style description
bitnami: x
date: 2001-01-01
`
	snap, err := ValuesSnapshot("mychart", "1.0.0", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Chart != "mychart" || snap.Version != "1.0.0" {
		t.Errorf("%+v", snap)
	}
	want := map[string]string{
		"replicaCount":     "1",
		"image.repository": `"quay.io/example/app"`,
		"image.tag":        `""`,
		"image.pullPolicy": `"IfNotPresent"`,
		"image.digest":     "null",
		"flags":            `["--a","--b"]`,
		"extraEnv":         `[{"extra":{"a":1,"b":2},"name":"X","value":"1"}]`,
		"empty":            "{}",
		"emptyList":        "[]",
		"ratio":            "0.5",
		"big":              "9007199254740993",
		"enabled":          "true",
		"html":             `"<b>&</b>"`,
		`annotations["nginx.ingress.kubernetes.io/rewrite-target"]`: `"/"`,
		`annotations["with space"]`:                                 `"yes"`,
		"annotations.plain":                                         `"v"`,
		`annotations["nested.key"].leaf`:                            "1",
		`["a.b"].c`:                                                 "2",
		"anchor.one":                                                "1",
		"alias.one":                                                 "1",
		"merged.one":                                                "1",
		"merged.two":                                                "2",
		"bitnami":                                                   `"x"`,
		"date":                                                      `"2001-01-01"`,
	}
	if !reflect.DeepEqual(snap.Entries, want) {
		for k, v := range want {
			if snap.Entries[k] != v {
				t.Errorf("entry %q = %q, want %q", k, snap.Entries[k], v)
			}
		}
		for k, v := range snap.Entries {
			if _, ok := want[k]; !ok {
				t.Errorf("unexpected entry %q = %q", k, v)
			}
		}
	}
	// intermediate mappings are not entries
	for _, k := range []string{"image", "annotations", "merged"} {
		if _, ok := snap.Entries[k]; ok {
			t.Errorf("mapping %q should not be an entry", k)
		}
	}
	// every leaf value is valid JSON
	for k, v := range snap.Entries {
		if !json.Valid([]byte(v)) {
			t.Errorf("entry %q is not JSON: %q", k, v)
		}
	}
	wantComments := map[string]string{
		"replicaCount":     "Number of replicas",
		"image.repository": "The image repository",
		"bitnami":          "Bitnami style description",
	}
	if !reflect.DeepEqual(snap.Comments, wantComments) {
		t.Errorf("comments = %v, want %v", snap.Comments, wantComments)
	}
}

func TestValuesSnapshotEdgeCases(t *testing.T) {
	// empty document and comment-only document
	for _, src := range []string{"", "# only a comment\n", "---\n", "null\n"} {
		snap, err := ValuesSnapshot("c", "1", []byte(src))
		if err != nil || len(snap.Entries) != 0 || snap.Comments != nil {
			t.Errorf("%q: %+v %v", src, snap, err)
		}
	}
	for _, src := range []string{"a: [unclosed", "- a\n- b\n", "just a string"} {
		if _, err := ValuesSnapshot("c", "1", []byte(src)); err == nil {
			t.Errorf("%q should be an error", src)
		}
	}
	// long comments are bounded
	long := "# " + strings.Repeat("lorem ipsum ", 100) + "\nkey: v\n"
	snap, _ := ValuesSnapshot("c", "1", []byte(long))
	if c := snap.Comments["key"]; len(c) > maxCommentLen+4 || !strings.HasSuffix(c, "…") {
		t.Errorf("comment not bounded: %d %q", len(c), c[len(c)-8:])
	}
	// multi-line comment becomes one line, blank '#' lines and example indentation collapse
	snap, _ = ValuesSnapshot("c", "1", []byte("# Line one\n# line two\n#\n# For example:\n#  foo:\n#    - bar\nkey: v\n"))
	if got, want := snap.Comments["key"], "Line one line two For example: foo: - bar"; got != want {
		t.Errorf("comment = %q, want %q", got, want)
	}
	// a comment separated from the key by a blank line is not the key's comment
	snap, _ = ValuesSnapshot("c", "1", []byte("a: 1\n# detached\n\nb: 2\n"))
	if len(snap.Comments) != 0 {
		t.Errorf("detached comment attached: %v", snap.Comments)
	}
}

func TestValuesSnapshotCertManagerFixture(t *testing.T) {
	src := fixture(t, "certmanager/values.yaml")
	snap, err := ValuesSnapshot("cert-manager", "v1.18.0", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Entries) < 150 {
		t.Errorf("entries = %d", len(snap.Entries))
	}
	for k, want := range map[string]string{
		"replicaCount":                    "1",
		"image.repository":                `"quay.io/jetstack/cert-manager-controller"`,
		"crds.enabled":                    "false",
		"global.leaderElection.namespace": `"kube-system"`,
		"global.imagePullSecrets":         "[]",
		"resources":                       "{}",
		"extraArgs":                       "[]",
	} {
		if snap.Entries[k] != want {
			t.Errorf("%s = %q, want %q", k, snap.Entries[k], want)
		}
	}
	if c := snap.Comments["replicaCount"]; !strings.HasPrefix(c, "The number of replicas of the cert-manager controller to run.") {
		t.Errorf("replicaCount comment = %q", c)
	}
	if c := snap.Comments["global.leaderElection.namespace"]; c != "Override the namespace used for the leader election lease." {
		t.Errorf("comment = %q", c)
	}
	for k, c := range snap.Comments {
		if strings.Contains(c, "+docs:") || strings.HasPrefix(c, "#") || strings.HasPrefix(c, "--") || len(c) > maxCommentLen+4 {
			t.Errorf("comment of %s not cleaned: %q", k, c)
		}
	}
	// deterministic
	snap2, _ := ValuesSnapshot("cert-manager", "v1.18.0", src)
	if !reflect.DeepEqual(snap, snap2) {
		t.Error("not deterministic")
	}
}
