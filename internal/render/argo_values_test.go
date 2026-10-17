package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, body := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// helmMerge is Helm's -f merge (pkg/cli/values mergeMaps), written out here
// independently of env's implementation so the test cross-checks it.
func helmMerge(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if bm, ok := v.(map[string]any); ok {
			if am, ok := out[k].(map[string]any); ok {
				out[k] = helmMerge(am, bm)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// helmSet applies one --set-json assignment (strvals key: \. escapes a dot).
func helmSet(t *testing.T, m map[string]any, name, value string) {
	t.Helper()
	var segs []string
	var cur strings.Builder
	for i := 0; i < len(name); i++ {
		switch {
		case name[i] == '\\' && i+1 < len(name):
			i++
			cur.WriteByte(name[i])
		case name[i] == '.':
			segs = append(segs, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(name[i])
		}
	}
	segs = append(segs, cur.String())
	var v any
	if err := json.Unmarshal([]byte(value), &v); err != nil {
		t.Fatalf("set %s=%s: %v", name, value, err)
	}
	for _, s := range segs[:len(segs)-1] {
		next, ok := m[s].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[s] = next
		}
		m = next
	}
	m[segs[len(segs)-1]] = v
}

// renderEffective is what helm receives for a target: -f layers in order,
// then --set-json, flattened like env's values keys.
func renderEffective(t *testing.T, tg Target) map[string]string {
	t.Helper()
	merged := map[string]any{}
	for _, l := range tg.Values {
		var m map[string]any
		if err := yaml.Unmarshal(l.Content, &m); err != nil {
			t.Fatalf("layer %s: %v", l.Origin, err)
		}
		merged = helmMerge(merged, m)
	}
	for _, s := range tg.Set {
		helmSet(t, merged, s.Path, s.Value)
	}
	b, err := yaml.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	flat, err := normalize.FlattenValues(b)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, kv := range flat {
		out[kv.Path] = kv.Value
	}
	return out
}

func envEffective(e *env.Environment, id string) (map[string]string, bool) {
	for _, r := range e.ValuesReleases {
		if r.ID != id {
			continue
		}
		out := map[string]string{}
		for _, k := range r.Keys {
			out[k.Path] = k.Value
		}
		return out, true
	}
	return nil, false
}

func sortedPairs(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k+"="+m[k])
	}
	sort.Strings(out)
	return out
}

// The renderer and the environment model layer an Argo CD Application's
// values identically: valueFiles in declared order ($ref and a plain path of
// a git source), valuesObject instead of values, parameters with Helm's --set
// typing — same effective values, same release id, same completeness.
func TestArgoRenderAndEnvStacksAgree(t *testing.T) {
	cases := map[string]map[string]string{
		"multi-source chart": {
			"apps/cm.yaml": `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
spec:
  destination: {namespace: cert-manager}
  sources:
    - repoURL: https://charts.jetstack.io
      chart: cert-manager
      targetRevision: v1.17.2
      helm:
        valueFiles:
          - $values/platform/z-base.yaml
          - $values/platform/a-prod.yaml
        values: |
          ignored: true
          replicaCount: 99
        valuesObject:
          webhook: {timeoutSeconds: 10}
          keep: {}
        parameters:
          - {name: f, value: "1.5"}
          - {name: b, value: "TRUE"}
          - {name: z, value: "0123"}
          - {name: i, value: "12"}
          - {name: s, value: "12", forceString: true}
          - {name: podAnnotations.prometheus\.io/scrape, value: "true"}
          - {name: gone, value: "null"}
    - repoURL: https://git.example.com/gitops.git
      targetRevision: main
      ref: values
`,
			"platform/z-base.yaml": "replicaCount: 1\norder: z\ngone: {deep: 1}\nkeep: {a: 1}\nlist: [1, 2]\n",
			"platform/a-prod.yaml": "order: a\nlist: [3]\nprometheus: {servicemonitor: {enabled: true}}\n",
		},
		"git path source": {
			"apps/cm.yaml": `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
spec:
  destination: {namespace: cert-manager}
  source:
    repoURL: https://git.example.com/gitops.git
    path: charts/cert-manager
    targetRevision: main
    helm:
      valueFiles:
        - values-prod.yaml
      values: |
        replicaCount: 2
`,
			"charts/cert-manager/Chart.yaml":       "name: cert-manager\nversion: 1.0.0\n",
			"charts/cert-manager/values.yaml":      "chartDefault: true\n",
			"charts/cert-manager/values-prod.yaml": "env: prod\nreplicaCount: 1\n",
		},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			repo := writeTree(t, files)
			e, err := env.Load(env.Inputs{Repo: repo, Product: "cert-manager"})
			if err != nil {
				t.Fatal(err)
			}
			ts, _ := DetectTargets(TargetOptions{Product: "cert-manager", Charts: []string{"cert-manager"}, Repo: repo, Files: RepoFilesFrom(e)})
			var argo []Target
			for _, tg := range ts {
				if tg.Kind == TargetArgoCD {
					argo = append(argo, tg)
				}
			}
			if len(argo) != 1 {
				t.Fatalf("argo targets = %+v", ts)
			}
			want, ok := envEffective(e, argo[0].ID)
			if !ok {
				t.Fatalf("no env release with the render target's id %s: %+v", argo[0].ID, e.ValuesReleases)
			}
			got := renderEffective(t, argo[0])
			if g, w := sortedPairs(got), sortedPairs(want); strings.Join(g, "\n") != strings.Join(w, "\n") {
				t.Fatalf("render and env disagree:\nrender: %v\nenv:    %v", g, w)
			}
			if !argo[0].ValuesComplete || e.Health(env.DimValues) != env.HealthOK {
				t.Fatalf("completeness: render %v (%s), env %s %v", argo[0].ValuesComplete, argo[0].IncompleteReason, e.Health(env.DimValues), e.Warnings)
			}
			if _, ignored := got["ignored"]; ignored {
				t.Error("helm.values applied although valuesObject is set")
			}
		})
	}
}

// Value files neither tool can read leave both incomplete: a plain path of a
// Helm-repository chart source resolves inside the chart package.
func TestArgoUnreadableValueFileAgrees(t *testing.T) {
	repo := writeTree(t, map[string]string{"apps/cm.yaml": `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
spec:
  source:
    repoURL: https://charts.jetstack.io
    chart: cert-manager
    targetRevision: v1.17.2
    helm:
      valueFiles:
        - values-prod.yaml
`})
	e, err := env.Load(env.Inputs{Repo: repo, Product: "cert-manager"})
	if err != nil {
		t.Fatal(err)
	}
	ts, _ := DetectTargets(TargetOptions{Product: "cert-manager", Repo: repo, Files: RepoFilesFrom(e)})
	if len(ts) != 1 || ts[0].ValuesComplete || !strings.Contains(ts[0].IncompleteReason, "inside the chart package") {
		t.Fatalf("render target = %+v", ts)
	}
	if e.Health(env.DimValues) != env.HealthPartial {
		t.Fatalf("env health = %s", e.Health(env.DimValues))
	}
}

// targetAndRelease detects the product's single target of a kind and the
// env release with the same id.
func targetAndRelease(t *testing.T, repo string, kind TargetKind) (Target, *env.Environment, map[string]string) {
	t.Helper()
	e, err := env.Load(env.Inputs{Repo: repo, Product: "cert-manager"})
	if err != nil {
		t.Fatal(err)
	}
	ts, _ := DetectTargets(TargetOptions{Product: "cert-manager", Charts: []string{"cert-manager"}, Repo: repo, Files: RepoFilesFrom(e)})
	var of []Target
	for _, tg := range ts {
		if tg.Kind == kind {
			of = append(of, tg)
		}
	}
	if len(of) != 1 {
		t.Fatalf("%s targets = %+v", kind, ts)
	}
	want, ok := envEffective(e, of[0].ID)
	if !ok {
		t.Fatalf("no env release with the render target's id %s: %+v", of[0].ID, e.ValuesReleases)
	}
	return of[0], e, want
}

// The renderer and the environment model layer a helmfile release's values
// identically: values entries in order (files, inline maps), then set — whose
// value is a string field in helmfile, so the scalar's text as written
// ("1.50", "0123", a quoted "12") reaches helm --set and is typed there.
func TestHelmfileRenderAndEnvStacksAgree(t *testing.T) {
	repo := writeTree(t, map[string]string{
		"helmfile.yaml": `releases:
  - name: cm
    namespace: cert-manager
    chart: jetstack/cert-manager
    version: v1.17.2
    values:
      - envs/z-base.yaml
      - who: inline
        keep: {}
      - envs/a-prod.yaml
    set:
      - {name: f, value: 1.50}
      - {name: z, value: 0123}
      - {name: q, value: "12"}
      - {name: b, value: TRUE}
      - {name: s, value: plain}
      - {name: podAnnotations.prometheus\.io/scrape, value: "true"}
      - {name: gone, value: null}
`,
		"envs/z-base.yaml": "who: base\norder: z\nkeep: {a: 1}\ngone: {deep: 1}\nlist: [1, 2]\n",
		"envs/a-prod.yaml": "order: a\nlist: [3]\n",
	})
	tg, e, want := targetAndRelease(t, repo, TargetHelmfile)
	got := renderEffective(t, tg)
	if g, w := sortedPairs(got), sortedPairs(want); strings.Join(g, "\n") != strings.Join(w, "\n") {
		t.Fatalf("render and env disagree:\nrender: %v\nenv:    %v", g, w)
	}
	for k, v := range map[string]string{"f": `"1.50"`, "z": `"0123"`, "q": "12", "b": "true", "s": `"plain"`, "order": `"a"`, "who": `"inline"`, "keep.a": "1", "list": "[3]"} {
		if got[k] != v {
			t.Errorf("%s = %s, want %s (all: %v)", k, got[k], v, got)
		}
	}
	if !tg.ValuesComplete || e.Health(env.DimValues) != env.HealthOK {
		t.Fatalf("completeness: render %v (%s), env %s %v", tg.ValuesComplete, tg.IncompleteReason, e.Health(env.DimValues), e.Warnings)
	}
}

// A set entry neither tool evaluates (a values list, --set-file, list syntax)
// leaves both incomplete.
func TestHelmfileUnsupportedSetAgrees(t *testing.T) {
	for name, entry := range map[string]string{
		"values list": "{name: hosts, values: [a, b]}",
		"set-file":    "{name: config, file: config.json}",
		"index":       `{name: "tolerations[0].key", value: x}`,
		"list value":  `{name: hosts, value: "{a,b}"}`,
	} {
		t.Run(name, func(t *testing.T) {
			repo := writeTree(t, map[string]string{"helmfile.yaml": "releases:\n  - name: cm\n    chart: jetstack/cert-manager\n    set:\n      - " + entry + "\n"})
			tg, e, _ := targetAndRelease(t, repo, TargetHelmfile)
			if tg.ValuesComplete || e.Health(env.DimValues) != env.HealthPartial {
				t.Fatalf("render complete=%v (%s), env %s", tg.ValuesComplete, tg.IncompleteReason, e.Health(env.DimValues))
			}
		})
	}
}
