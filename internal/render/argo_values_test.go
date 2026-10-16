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
