package eval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

// writeCase lays out <root>/cases/<id>/case.yaml plus fixture files.
func writeCase(t *testing.T, root, id, body string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(root, CasesDirName, id)
	for p, content := range files {
		full := filepath.Join(dir, EnvironmentDir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "case.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const labelledCase = `id: lab
product: cert-manager
from: v1.17.0
to: v1.18.0
researchedAt: "2026-10-01"
sources: [https://example/notes.md]
expected:
  - id: E1
    title: rotationPolicy default Never -> Always
    kind: behaviour-change
    importance: critical
    match: [{text: '(?i)rotationPolicy'}]
    evidence: [{url: https://example/notes.md, quote: q}]
    semantics:
      subject: {family: crd-field, product: cert-manager, group: cert-manager.io, kind: Certificate, path: spec.privateKey.rotationPolicy}
      change: {type: default-changed, before: '"Never"', after: '"Always"'}
      consequence: {kind: behavior-change, exposedClass: review-required, statement: keys rotate on re-issuance}
  - id: E2
    title: key renamed
    kind: helm-values
    importance: important
    match: [{text: '(?i)secretsBackend'}]
    evidence: [{url: https://example/notes.md}]
    semantics:
      - subject: {family: helm-value, product: cert-manager, path: tls.secretsBackend}
        change: {type: deprecated, replacedBy: {family: helm-value, product: cert-manager, path: tls.readSecretsOnlyFromSecretsNamespace}}
        consequence: {kind: deprecation, exposedClass: review-required}
      - subject: {family: helm-value, product: cert-manager, path: tls.other}
        change: {type: removed}
        consequence: {kind: setting-ignored, exposedClass: action-required, statement: the value stops being honoured}
environment:
  description: d
  kubernetes: "1.31"
  expectedImpact:
    - expected: E1
      relevance: review
      exposure:
        op: resource
        group: cert-manager.io
        kind: Certificate
        of: [{op: field, path: spec.privateKey.rotationPolicy, state: unset}]
      overlap: {op: resource, group: cert-manager.io, kind: Certificate, of: [{op: field, path: spec.privateKey.rotationPolicy, state: set}]}
      environmentEvidence: manifests/cert.yaml#L1-L3
  undecidedImpact:
    - expected: E2
      reason: environment-visibility-gap
      needed: the Helm values file
      exposure: {op: values-key, path: tls.secretsBackend, state: set}
      environmentEvidence: [environment.kubernetes]
`

var labelledFiles = map[string]string{"manifests/cert.yaml": "apiVersion: cert-manager.io/v1\nkind: Certificate\nmetadata: {name: a}\n"}

func TestLabelsDecodeStrictlyIntoDomainTypes(t *testing.T) {
	root := t.TempDir()
	dir := writeCase(t, root, "lab", labelledCase, labelledFiles)
	c, err := LoadCase(dir)
	if err != nil {
		t.Fatal(err)
	}
	e1 := c.Expected[0].Semantics
	if len(e1) != 1 || e1[0].Subject.Key() != "crd-field:cert-manager|group=cert-manager.io|kind=Certificate|path=spec.privateKey.rotationPolicy" {
		t.Fatalf("E1 semantics = %+v", e1)
	}
	if *e1[0].Change.Before != `"Never"` || e1[0].Consequence.ExposedClass != domain.ImpactReviewRequired {
		t.Errorf("E1 change/consequence = %+v %+v", e1[0].Change, e1[0].Consequence)
	}
	e2 := c.Expected[1].Semantics
	if len(e2) != 2 || e2[0].Change.ReplacedBy == nil || e2[0].Change.ReplacedBy.Path != "tls.readSecretsOnlyFromSecretsNamespace" {
		t.Fatalf("E2 semantics (list form, replacedBy) = %+v", e2)
	}
	l := c.Environment.ExpectedImpact[0]
	if l.Exposure == nil || l.Exposure.Op != domain.OpResource || l.Exposure.Of[0].State != domain.StateUnset || l.Overlap == nil {
		t.Fatalf("link exposure/overlap = %+v", l)
	}
	if len(l.EnvironmentEvidence) != 1 || l.EnvironmentEvidence[0] != "manifests/cert.yaml#L1-L3" {
		t.Errorf("environmentEvidence (scalar form) = %v", l.EnvironmentEvidence)
	}
	u := c.Environment.UndecidedImpact
	if len(u) != 1 || u[0].Reason != domain.UnknownEnvironmentVisibilityGap || u[0].Exposure.Op != domain.OpValuesKey {
		t.Errorf("undecided = %+v", u)
	}
	if a := e1[0].Assertion(); a.Subject == nil || a.Applicability != nil || a.Validate(false) != nil {
		t.Errorf("assertion = %+v (%v)", a, a.Validate(false))
	}
}

func TestLabelValidationFailsLoudly(t *testing.T) {
	cases := map[string]struct{ from, to, want string }{
		"typo in a domain field": {
			"change: {type: default-changed, before:", "change: {type: default-changed, befor:", "unknown field",
		},
		"typo in a link key": {
			"      environmentEvidence: manifests/cert.yaml#L1-L3", "      environmentEvidences: manifests/cert.yaml", "unknown key",
		},
		"class does not follow the kind": {
			"exposedClass: review-required, statement: keys", "exposedClass: action-required, statement: keys", "class follows the kind",
		},
		"subject of another product": {
			"{family: crd-field, product: cert-manager,", "{family: crd-field, product: istio,", "not the case product",
		},
		"incomplete label": {
			"      change: {type: default-changed, before: '\"Never\"', after: '\"Always\"'}\n", "", "all required",
		},
		"numeric before (must be JSON-encoded string)": {
			`before: '"Never"'`, "before: 30", "cannot unmarshal number",
		},
		"invalid condition": {
			"of: [{op: field, path: spec.privateKey.rotationPolicy, state: unset}]", "of: [{op: field, path: spec.privateKey.rotationPolicy, state: enabled}]", "not allowed",
		},
		"scoped leaf at top level": {
			"  undecidedImpact:\n    - expected: E2\n      reason: environment-visibility-gap\n      needed: the Helm values file\n      exposure: {op: values-key, path: tls.secretsBackend, state: set}",
			"  undecidedImpact:\n    - expected: E2\n      reason: environment-visibility-gap\n      needed: the Helm values file\n      exposure: {op: field, path: tls.secretsBackend, state: set}",
			"only valid inside",
		},
		"evidence file missing": {
			"manifests/cert.yaml#L1-L3", "manifests/nope.yaml", "no such fixture file",
		},
		"evidence line past the end": {
			"manifests/cert.yaml#L1-L3", "manifests/cert.yaml#L9", "past the end",
		},
		"evidence escapes the fixture": {
			"manifests/cert.yaml#L1-L3", "../case.yaml", "relative to environment",
		},
		"exposure without evidence": {
			"      environmentEvidence: manifests/cert.yaml#L1-L3\n", "", "environmentEvidence is required",
		},
		"unknown reason": {
			"reason: environment-visibility-gap", "reason: no-idea", "not an UNKNOWN reason",
		},
		"undecided without needed": {
			"needed: the Helm values file", "needed: ''", "needed",
		},
		"same item decided and undecided": {
			"  undecidedImpact:\n    - expected: E2", "  undecidedImpact:\n    - expected: E1", "already linked",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			body := strings.Replace(labelledCase, tc.from, tc.to, 1)
			if body == labelledCase {
				t.Fatalf("replacement %q did not apply", tc.from)
			}
			dir := writeCase(t, t.TempDir(), "lab", body, labelledFiles)
			_, err := LoadCase(dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestExposureNeedsItemSemantics(t *testing.T) {
	body := strings.Replace(labelledCase, "    semantics:\n      subject: {family: crd-field", "    xsemantics:\n      subject: {family: crd-field", 1)
	// drop the E1 semantics block entirely
	start := strings.Index(body, "    xsemantics:")
	end := strings.Index(body, "  - id: E2")
	body = body[:start] + body[end:]
	dir := writeCase(t, t.TempDir(), "lab", body, labelledFiles)
	if _, err := LoadCase(dir); err == nil || !strings.Contains(err.Error(), "no semantics block") {
		t.Fatalf("err = %v", err)
	}
}

const transferCase = `id: lab--b
transferOf: lab
researchedAt: "2026-10-01"
sources: [https://example/fixture-grounding]
environment:
  description: second cluster
  kubernetes: "1.30"
  expectedImpact:
    - expected: E1
      relevance: not-affected
      exposure:
        op: resource
        group: cert-manager.io
        kind: Certificate
        of: [{op: field, path: spec.privateKey.rotationPolicy, state: unset}]
      environmentEvidence: manifests/cert.yaml
`

func TestTransferCaseInheritsTheBase(t *testing.T) {
	root := t.TempDir()
	writeCase(t, root, "lab", labelledCase, labelledFiles)
	writeCase(t, root, "lab--b", transferCase, map[string]string{"manifests/cert.yaml": "kind: Certificate\n"})
	r := &Runner{CasesDir: root}
	cases, err := r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("cases = %d", len(cases))
	}
	tr := cases[1]
	if tr.ID != "lab--b" || tr.Product != "cert-manager" || tr.From != "v1.17.0" || len(tr.Expected) != 2 || len(tr.Expected[0].Semantics) != 1 {
		t.Fatalf("transfer case = %+v", tr)
	}
	if len(tr.Sources) != 2 || tr.Environment.Kubernetes != "1.30" {
		t.Errorf("sources/env = %v %+v", tr.Sources, tr.Environment)
	}

	for name, mutate := range map[string]func(string) string{
		"restating expected items": func(s string) string {
			return strings.Replace(s, "researchedAt:", "expected: [{id: E9, title: x, kind: removal, importance: minor, match: [{text: x}]}]\nresearchedAt:", 1)
		},
		"restating the product": func(s string) string {
			return strings.Replace(s, "researchedAt:", "product: cert-manager\nresearchedAt:", 1)
		},
		"missing base":                          func(s string) string { return strings.Replace(s, "transferOf: lab", "transferOf: nope", 1) },
		"no environment":                        func(s string) string { return s[:strings.Index(s, "environment:")] },
		"link to an unknown item":               func(s string) string { return strings.Replace(s, "expected: E1", "expected: E7", 1) },
		"chained transfer (base is a transfer)": nil,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeCase(t, root, "lab", labelledCase, labelledFiles)
			body := transferCase
			if mutate == nil {
				writeCase(t, root, "lab--b", transferCase, map[string]string{"manifests/cert.yaml": "kind: Certificate\n"})
				body = strings.NewReplacer("id: lab--b", "id: lab--c", "transferOf: lab", "transferOf: lab--b").Replace(transferCase)
				dir := writeCase(t, root, "lab--c", body, map[string]string{"manifests/cert.yaml": "kind: Certificate\n"})
				if _, err := LoadCase(dir); err == nil || !strings.Contains(err.Error(), "itself a transfer") {
					t.Fatalf("err = %v", err)
				}
				return
			}
			dir := writeCase(t, root, "lab--b", mutate(body), map[string]string{"manifests/cert.yaml": "kind: Certificate\n"})
			if _, err := LoadCase(dir); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

// labelPipeline: an edge with one change covering E1, and a report whose
// findings depend on the environment (an affected finding only in the base
// environment).
type labelPipeline struct{}

func (labelPipeline) Upgrade(ctx context.Context, product, from, to string) (*domain.UpgradeEdge, error) {
	return &domain.UpgradeEdge{
		Product: domain.ProductRef{ID: "cert-manager"},
		Changes: []domain.Change{
			ch("chg-a", "Default rotationPolicy changed to Always", "", "1.18.0", nil, domain.CategoryConfiguration, true, "ev-1"),
		},
		Evidence: []domain.Evidence{ev("ev-1", "https://example/notes.md")},
	}, nil
}

func (labelPipeline) Impact(ctx context.Context, product, from, to string, in env.Inputs) (*domain.ImpactReport, error) {
	cls := domain.ImpactReviewRequired
	if in.KubernetesVersion == "1.30" { // the transfer environment: checked and clear
		cls = domain.ImpactNotAffected
	}
	return &domain.ImpactReport{Findings: []domain.ImpactFinding{{
		ID: "f-1", Rule: "impact:knowledge-exposed", Classification: cls, ChangeID: "chg-a",
		Matches: []domain.ImpactMatch{{Subject: "Certificate/a"}},
	}}}, nil
}

func TestTransferEntryScoresOnlyItsEnvironment(t *testing.T) {
	root := t.TempDir()
	writeCase(t, root, "lab", labelledCase, labelledFiles)
	writeCase(t, root, "lab--b", transferCase, map[string]string{"manifests/cert.yaml": "kind: Certificate\n"})
	r := &Runner{Pipeline: labelPipeline{}, CasesDir: root}
	cases, err := r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	results := r.Run(context.Background(), cases)
	base, tr := results[0], results[1]
	if base.Metrics.Expected != 2 || base.Metrics.Found != 1 || base.Metrics.Changes != 1 {
		t.Fatalf("base metrics = %+v", base.Metrics)
	}
	if tr.Metrics.Expected != 0 || tr.Metrics.Found != 0 || tr.Metrics.Changes != 0 || len(tr.Matches) != 0 || len(tr.AllChangeIDs) != 0 {
		t.Fatalf("transfer entry must not rescore the edge: %+v", tr.Metrics)
	}
	if tr.Env == nil || tr.Env.NotAffectedLinks != 1 || tr.Env.NotAffectedViolations != 0 {
		t.Fatalf("transfer env = %+v", tr.Env)
	}
	agg := AggregateResults(results)
	if agg.Expected != 2 || agg.Changes != 1 || agg.EnvEntries != 2 {
		t.Errorf("aggregate double-counts the shared edge: %+v", agg)
	}
	// base: 1 affected link hit; transfer: 1 not-affected link clear → 2/2
	if agg.ApplicabilityAccuracy != 1 {
		t.Errorf("applicabilityAccuracy = %v", agg.ApplicabilityAccuracy)
	}
}
