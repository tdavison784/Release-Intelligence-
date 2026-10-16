package render

import (
	"context"
	"os"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
)

const tokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// removedKey is a values:removed change as internal/upgrade writes it.
func removedKey(id, key, def string) domain.Change {
	return domain.Change{ID: id, Category: domain.CategoryHelmValues, ActionRequired: true,
		Title:      "Helm value `" + key + "` removed",
		Detail:     "Values set at this key no longer take effect after the upgrade (or are rejected when the chart validates values against a schema). Default in dflt-1.0.0: " + def + ".",
		Subjects:   []string{key},
		Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: "values:removed", Confidence: domain.ConfidenceHigh},
	}
}

func jsonPtr(v any) *string { s := encode(v); return &s }

func links(c Correlation) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, cc := range c.Changes {
		key := cc.Change.Object.Name + " " + cc.Change.Path
		for _, l := range cc.Links {
			if out[key] == nil {
				out[key] = map[string]string{}
			}
			out[key][l.ChangeID] = l.Rule
		}
	}
	return out
}

// The default a removed / default-changed key held disappears from a rendered
// change whose path is not the values path: linked by value and location
// (values-default), under the minimum-specificity and location guards.
func TestCorrelateFromDefaultGone(t *testing.T) {
	sm := func(name string, before, after any) Change {
		return Change{Class: FieldChanged, Object: ObjectID{Group: "monitoring.coreos.com", Version: "v1", Kind: "ServiceMonitor", Namespace: "m", Name: name},
			Path: "spec.endpoints", Pattern: "spec.endpoints", Before: jsonPtr(before), After: jsonPtr(after)}
	}
	ep := func(fields ...any) []any {
		m := map[string]any{"port": "metrics"}
		for i := 0; i+1 < len(fields); i += 2 {
			m[fields[i].(string)] = fields[i+1]
		}
		return []any{m}
	}
	auth := map[string]any{"credentials": map[string]any{"name": "t", "key": "token"}}
	cases := []struct {
		name   string
		rc     Change
		change domain.Change
		linked bool
	}{
		{"redacted default disappears (credential-named field)",
			sm("demo-exporter", ep("bearerTokenFile", redact(tokenPath)), ep("authorization", auth)),
			removedKey("chg-a", "exporter.serviceMonitor.bearerTokenFile", `"`+tokenPath+`"`), true},
		{"plain default disappears",
			sm("demo-exporter", ep("scheme", "https"), ep()),
			removedKey("chg-a", "exporter.serviceMonitor.scheme", `"https"`), true},
		{"the environment overrides the consumer path: its own value, not the default, disappears",
			sm("demo-exporter", ep("bearerTokenFile", redact("/etc/custom/token")), ep("authorization", auth)),
			removedKey("chg-a", "exporter.serviceMonitor.bearerTokenFile", `"`+tokenPath+`"`), false},
		{"the default stays in the after",
			sm("demo-exporter", ep("bearerTokenFile", redact(tokenPath)), ep("bearerTokenFile", redact(tokenPath), "interval", "30s")),
			removedKey("chg-a", "exporter.serviceMonitor.bearerTokenFile", `"`+tokenPath+`"`), false},
		{"another component's object holding the same value (location guard)",
			sm("demo-kubelet", ep("bearerTokenFile", redact(tokenPath)), ep("authorization", auth)),
			removedKey("chg-a", "exporter.serviceMonitor.bearerTokenFile", `"`+tokenPath+`"`), false},
		{"a field of another name holding the value",
			sm("demo-exporter", ep("tokenPath", tokenPath), ep()),
			removedKey("chg-a", "exporter.serviceMonitor.bearerTokenFile", `"`+tokenPath+`"`), false},
		{"trivial default true (specificity guard)",
			sm("demo-exporter", ep("honorLabels", true), ep()),
			removedKey("chg-a", "exporter.serviceMonitor.honorLabels", "true"), false},
		{"trivial default 1 (specificity guard)",
			sm("demo-exporter", ep("replicas", 1), ep()),
			removedKey("chg-a", "exporter.serviceMonitor.replicas", "1"), false},
		{`trivial default "" (specificity guard)`,
			sm("demo-exporter", ep("path", ""), ep()),
			removedKey("chg-a", "exporter.serviceMonitor.path", `""`), false},
		{"short string (specificity guard)",
			sm("demo-exporter", ep("interval", "30s"), ep()),
			removedKey("chg-a", "exporter.serviceMonitor.interval", `"30s"`), false},
		{"default-changed analogue: the old default leaves the render",
			sm("demo-exporter", ep("metricRelabelings", []any{map[string]any{"action": "drop", "regex": "go_.*"}}), ep()),
			domain.Change{ID: "chg-d", Category: domain.CategoryHelmValues, Title: "Default of Helm value `exporter.serviceMonitor.metricRelabelings` changed",
				Detail: `[{"action":"drop","regex":"go_.*"}] → []`, Subjects: []string{"exporter.serviceMonitor.metricRelabelings"},
				Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: "values:default-changed"}}, true},
		{"a hand-built change without a recorded default",
			sm("demo-exporter", ep("bearerTokenFile", redact(tokenPath)), ep()),
			domain.Change{ID: "chg-x", Category: domain.CategoryHelmValues, Title: "exporter.serviceMonitor.bearerTokenFile removed",
				Subjects: []string{"exporter.serviceMonitor.bearerTokenFile"}, Provenance: domain.Provenance{Method: domain.MethodComputed, Rule: "values:removed"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := links(Correlate(&DiffResult{Changes: []Change{tc.rc}}, &domain.UpgradeEdge{Changes: []domain.Change{tc.change}}))
			rule := got[tc.rc.Object.Name+" "+tc.rc.Path][tc.change.ID]
			if tc.linked != (rule == "values-default") {
				t.Errorf("linked=%v (rule %q), want %v", rule != "", rule, tc.linked)
			}
		})
	}
}

func dfltPair(t *testing.T, e *Engine, valuesFile string) *Pair {
	t.Helper()
	content, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatal(err)
	}
	tgt := Target{ID: "values:" + valuesFile, Kind: TargetValuesFiles, Origin: valuesFile, Why: "test",
		ReleaseName: "dflt", Namespace: "mon", ValuesComplete: true,
		Values: []ValuesLayer{{Origin: valuesFile, Content: content}}}
	p := e.RenderPair(context.Background(), PairRequest{Product: "dflt", From: "1.0.0", To: "1.1.0", Target: tgt, Scope: domain.RenderEnvironment, KubeVersion: "1.31"})
	if p.Status != PairOK {
		t.Fatalf("pair: %s %+v", p.Status, p.Failure)
	}
	return p
}

// Live helm, synthetic chart: dflt 1.1.0 removes
// exporter.serviceMonitor.bearerTokenFile (default: the service-account
// token path, rendered into the exporter's ServiceMonitor) and
// exporter.legacyNote (default ""). A customer who leaves the token key unset
// relied on its default: the join never says not-affected and the rendered
// change of the exporter's ServiceMonitor is correlated to the item; the
// kubelet ServiceMonitor, which hard-codes the same path, is not. A customer
// who sets the key keeps today's logic; the empty-default key stays
// not-affected.
func TestRemovedDefaultEndToEnd(t *testing.T) {
	needTool(t, "helm")
	e := testEngine(t)
	token := removedKey("chg-token", "exporter.serviceMonitor.bearerTokenFile", `"`+tokenPath+`"`)
	note := removedKey("chg-note", "exporter.legacyNote", `""`)
	ev := domain.NewEvidence(domain.EvidenceStructured, "chart", "testdata/charts/dflt-1.0.0/values.yaml", "", "# values", "sha256:dflt", fixedNow)
	token.Evidence, note.Evidence = []domain.EvidenceID{ev.ID}, []domain.EvidenceID{ev.ID}
	edge := &domain.UpgradeEdge{SchemaVersion: domain.UpgradeEdgeSchemaVersion, Product: domain.ProductRef{ID: "dflt", Name: "dflt"},
		From: domain.MustVersion("dflt-1.0.0", "1.0.0"), To: domain.MustVersion("dflt-1.1.0", "1.1.0"),
		Path:    []domain.PathStep{{Version: domain.MustVersion("dflt-1.1.0", "1.1.0"), Reason: "minor-release"}},
		Changes: []domain.Change{token, note}, Evidence: []domain.Evidence{ev}}

	join := func(valuesFile string) map[string]domain.ImpactFinding {
		en, err := env.Load(env.Inputs{ValuesFiles: []string{valuesFile}})
		if err != nil {
			t.Fatal(err)
		}
		rep, err := impact.Build(impact.Input{Edge: edge, Env: en, Now: fixedNow})
		if err != nil {
			t.Fatal(err)
		}
		if err := rep.Validate(); err != nil {
			t.Fatalf("report: %v", err)
		}
		out := map[string]domain.ImpactFinding{}
		for _, f := range rep.Findings {
			out[f.ChangeID] = f
		}
		return out
	}

	// unset: relied on the default
	p := dfltPair(t, e, "testdata/dflt-customer.yaml")
	got := links(Correlate(p.Diff, edge))
	if got["dflt-exporter spec.endpoints"]["chg-token"] != "values-default" {
		t.Errorf("exporter ServiceMonitor change not correlated to the removed key: %v (diff %+v)", got, p.Diff.Changes)
	}
	if r := got["dflt-kubelet spec.endpoints"]["chg-token"]; r != "" {
		t.Errorf("kubelet ServiceMonitor (hard-coded path) linked by %s", r)
	}
	for k, l := range got {
		if l["chg-note"] != "" {
			t.Errorf("empty-default key linked to %s", k)
		}
	}
	fs := join("testdata/dflt-customer.yaml")
	if f := fs["chg-token"]; f.Rule != impact.RuleValuesRemovedDefault || f.Classification != domain.ImpactReviewRequired {
		t.Errorf("unset token key: %s %s, want review-required values-removed-default", f.Classification, f.Rule)
	}
	if f := fs["chg-note"]; f.Rule != impact.RuleValuesUnset || f.Classification != domain.ImpactNotAffected {
		t.Errorf("empty-default key: %s %s, want not-affected values-unset", f.Classification, f.Rule)
	}

	// pinned: the customer's own value disappears, not the default — not
	// linked by the default; the join keeps the set-key verdict
	pp := dfltPair(t, e, "testdata/dflt-customer-pinned.yaml")
	if r := links(Correlate(pp.Diff, edge))["dflt-exporter spec.endpoints"]["chg-token"]; r == "values-default" {
		t.Error("an overridden key's value is not its default")
	}
	if f := join("testdata/dflt-customer-pinned.yaml")["chg-token"]; f.Rule != impact.RuleValuesRemoved {
		t.Errorf("pinned token key: %s %s, want today's values-removed", f.Classification, f.Rule)
	}
}
