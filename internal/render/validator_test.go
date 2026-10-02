package render

import (
	"context"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

func sptr(s string) *string { return &s }

// goldenReleasePair is the release-level (chart-default) pair of the two
// golden streams, built exactly like Engine.RenderPair builds it.
func goldenReleasePair(t *testing.T) *Pair {
	t.Helper()
	from, to := goldenPair(t)
	d := Diff(from, to, DiffOptions{})
	res := func(version string, objs []Object) *Result {
		return &Result{Status: StatusSucceeded, Objects: objs, Provenance: Provenance{
			Scope: domain.RenderRelease, Tool: ToolHelm, ToolVersion: "v3.16.4",
			Command: []string{"helm", "template", "demo", "chart.tgz"},
			Chart:   "demo", ChartVersion: version, ChartURI: "testdata/charts/demo-" + version,
			ArtifactDigest: "sha256:chart-" + version, OutputDigest: "sha256:out-" + version, RenderedAt: fixedNow,
		}}
	}
	return &Pair{Product: "demo", From: "1.0.0", To: "1.1.0", Scope: domain.RenderRelease,
		Target: DefaultsTarget("demo"), Status: PairOK,
		FromResult: res("1.0.0", from), ToResult: res("1.1.0", to), Diff: &d}
}

// fixedPairs is a ReleasePairs source returning one pair.
type fixedPairs struct {
	p     *Pair
	calls int
}

func (f *fixedPairs) ReleasePair(context.Context, string, string, string) *Pair {
	f.calls++
	return f.p
}

func vInput(a domain.SemanticAssertion) knowledge.ValidationInput {
	return knowledge.ValidationInput{
		Candidate:  domain.SemanticCandidate{ID: domain.CandidateIDPrefix + "test", Product: "demo"},
		ProposalID: domain.ProposalIDPrefix + "p1", Assertion: a,
		From: &domain.Release{Product: "demo", Version: domain.MustVersion("1.0.0", "1.0.0")},
		To:   &domain.Release{Product: "demo", Version: domain.MustVersion("1.1.0", "1.1.0")},
		Now:  fixedNow,
	}
}

func assertOn(s *domain.Subject, c *domain.ChangeSpec) domain.SemanticAssertion {
	return domain.SemanticAssertion{Subject: s, Change: c}
}

// runValidate runs the rendered-diff validator and checks the result is
// well-formed (ValidationResult.Validate) before returning it.
func runValidate(t *testing.T, pairs ReleasePairs, in knowledge.ValidationInput) []domain.ValidationResult {
	t.Helper()
	rs, err := NewValidator(pairs).Validate(context.Background(), in)
	if err != nil {
		t.Fatalf("rendered-diff: %v", err)
	}
	for _, r := range rs {
		if err := r.Validate(); err != nil {
			t.Fatalf("rendered-diff produced an invalid result: %v", err)
		}
		if r.Validator != ValidatorName {
			t.Errorf("validator producer %q, want %q", r.Validator, ValidatorName)
		}
	}
	return rs
}

func TestRenderedDiffValidatorConfirms(t *testing.T) {
	pairs := &fixedPairs{p: goldenReleasePair(t)}
	cases := []struct {
		name      string
		assertion domain.SemanticAssertion
		relation  domain.RenderRelation
		subject   domain.ValidationOutcome
		change    domain.ValidationOutcome
		detailHas string
	}{
		{
			name: "image value changed",
			assertion: assertOn(
				&domain.Subject{Family: domain.SubjectImage, Product: "demo", Name: "example.org/demo/controller"},
				&domain.ChangeSpec{Type: domain.ChangeKindValueChanged, Before: sptr("v1.0.0"), After: sptr("v1.1.0")}),
			relation: domain.RenderConfirmed, subject: domain.OutcomeConfirmed, change: domain.OutcomeConfirmed,
		},
		{
			name: "rbac permission removed",
			assertion: assertOn(
				&domain.Subject{Family: domain.SubjectRBACPermission, Product: "demo", Group: "demo.example.org", Name: "widgets/update"},
				&domain.ChangeSpec{Type: domain.ChangeKindRemoved}),
			relation: domain.RenderConfirmed, subject: domain.OutcomeConfirmed, change: domain.OutcomeConfirmed,
		},
		{
			name: "env var added",
			assertion: assertOn(
				&domain.Subject{Family: domain.SubjectEnvVar, Product: "demo", Name: "GOMAXPROCS"},
				&domain.ChangeSpec{Type: domain.ChangeKindAdded}),
			relation: domain.RenderConfirmed, subject: domain.OutcomeConfirmed, change: domain.OutcomeConfirmed,
		},
		{
			name: "cli flag value changed",
			assertion: assertOn(
				&domain.Subject{Family: domain.SubjectCLIFlag, Product: "demo", Name: "leader-election-namespace"},
				&domain.ChangeSpec{Type: domain.ChangeKindValueChanged, Before: sptr("kube-system"), After: sptr("demo-system")}),
			relation: domain.RenderConfirmed, subject: domain.OutcomeConfirmed, change: domain.OutcomeConfirmed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := runValidate(t, pairs, vInput(tc.assertion))
			if len(rs) != 1 {
				t.Fatalf("%d results, want 1", len(rs))
			}
			r := rs[0]
			if r.RenderRelation != tc.relation {
				t.Errorf("relation %q, want %q (%s)", r.RenderRelation, tc.relation, detailOf(r))
			}
			for _, c := range r.Checks {
				want := tc.subject
				if c.Aspect == domain.AspectChange {
					want = tc.change
				}
				if c.Outcome != want {
					t.Errorf("%s = %s, want %s (%s)", c.Aspect, c.Outcome, want, c.Detail)
				}
			}
			if r.Confirms() {
				if len(r.Evidence) == 0 {
					t.Fatal("a confirmation must cite rendered evidence")
				}
				for _, e := range r.Evidence {
					if e.Render == nil || e.Render.Scope != domain.RenderRelease {
						t.Errorf("evidence %s is not release-level rendered evidence", e.ID)
					}
				}
			}
			if tc.detailHas != "" && !strings.Contains(detailOf(r), tc.detailHas) {
				t.Errorf("detail %q does not mention %q", detailOf(r), tc.detailHas)
			}
		})
	}
	if pairs.calls != len(cases) {
		t.Errorf("ReleasePair called %d times, want %d (memoization is the source's job)", pairs.calls, len(cases))
	}
}

func detailOf(r domain.ValidationResult) string {
	for _, c := range r.Checks {
		return c.Detail
	}
	return ""
}

func TestRenderedDiffValidatorRefutes(t *testing.T) {
	pairs := &fixedPairs{p: goldenReleasePair(t)}
	// POD_NAMESPACE exists unchanged in both renders: "added" is contradicted.
	in := vInput(assertOn(
		&domain.Subject{Family: domain.SubjectEnvVar, Product: "demo", Name: "POD_NAMESPACE"},
		&domain.ChangeSpec{Type: domain.ChangeKindAdded}))
	rs := runValidate(t, pairs, in)
	if len(rs) != 1 {
		t.Fatalf("%d results, want 1", len(rs))
	}
	r := rs[0]
	if r.RenderRelation != domain.RenderContradicted {
		t.Fatalf("relation %q, want contradicted-by-render", r.RenderRelation)
	}
	for _, c := range r.Checks {
		if c.Aspect == domain.AspectChange && c.Outcome != domain.OutcomeRefuted {
			t.Errorf("change = %s, want refuted (%s)", c.Outcome, c.Detail)
		}
	}

	// an asserted value the target does not render is contradicted too
	in = vInput(assertOn(
		&domain.Subject{Family: domain.SubjectImage, Product: "demo", Name: "example.org/demo/controller"},
		&domain.ChangeSpec{Type: domain.ChangeKindValueChanged, Before: sptr("v1.0.0"), After: sptr("v9.9.9")}))
	rs = runValidate(t, pairs, in)
	if rs[0].RenderRelation != domain.RenderContradicted || rs[0].Checks[1].Outcome != domain.OutcomeRefuted {
		t.Errorf("a wrong after-value must refute the change: %+v", rs[0].Checks)
	}
}

func TestRenderedDiffValidatorInconclusive(t *testing.T) {
	pairs := &fixedPairs{p: goldenReleasePair(t)}
	cases := []struct {
		name      string
		assertion domain.SemanticAssertion
		relation  domain.RenderRelation
	}{
		{
			name: "subject absent from both renders",
			assertion: assertOn(
				&domain.Subject{Family: domain.SubjectImage, Product: "demo", Name: "example.org/other/controller"},
				&domain.ChangeSpec{Type: domain.ChangeKindAdded}),
			relation: domain.RenderNotVisible,
		},
		{
			name: "behaviour change is not renderable",
			assertion: assertOn(
				&domain.Subject{Family: domain.SubjectCLIFlag, Product: "demo", Name: "anything"},
				&domain.ChangeSpec{Type: domain.ChangeKindBehaviorChanged}),
			relation: domain.RenderNotApplicable,
		},
		{
			name: "runtime family is never applicable",
			assertion: assertOn(
				&domain.Subject{Family: domain.SubjectProtocolBehavior, Product: "demo", Name: "handshake"},
				&domain.ChangeSpec{Type: domain.ChangeKindAdded}),
			relation: domain.RenderNotApplicable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := runValidate(t, pairs, vInput(tc.assertion))
			if len(rs) != 1 {
				t.Fatalf("%d results, want 1", len(rs))
			}
			r := rs[0]
			if r.RenderRelation != tc.relation {
				t.Fatalf("relation %q, want %q (%s)", r.RenderRelation, tc.relation, detailOf(r))
			}
			if r.Confirms() {
				t.Error("an inconclusive result must not confirm")
			}
			for _, c := range r.Checks {
				if c.Outcome != domain.OutcomeInconclusive {
					t.Errorf("%s = %s, want inconclusive", c.Aspect, c.Outcome)
				}
			}
		})
	}
}

func TestRenderedDiffValidatorFailedRender(t *testing.T) {
	// a failed pair is an evidence gap, never "no change"
	pairs := &fixedPairs{p: &Pair{Product: "demo", From: "1.0.0", To: "1.1.0", Status: PairFailed,
		Failure: &Failure{Reason: FailChartUnavailable, Detail: "no chart demo 1.1.0"}}}
	rs := runValidate(t, pairs, vInput(assertOn(
		&domain.Subject{Family: domain.SubjectImage, Product: "demo", Name: "example.org/demo/controller"},
		&domain.ChangeSpec{Type: domain.ChangeKindValueChanged, Before: sptr("v1.0.0"), After: sptr("v1.1.0")})))
	if len(rs) != 1 {
		t.Fatalf("%d results, want 1", len(rs))
	}
	r := rs[0]
	if r.RenderRelation != domain.RenderNotVisible || r.Confirms() {
		t.Fatalf("a failed render must stay inconclusive: %+v", r)
	}
	if !strings.Contains(detailOf(r), "render failed") && !strings.Contains(detailOf(r), "no chart") {
		t.Errorf("detail must name the failure, got %q", detailOf(r))
	}
}

func TestRenderedDiffValidatorNoVersions(t *testing.T) {
	pairs := &fixedPairs{p: goldenReleasePair(t)}
	in := knowledge.ValidationInput{
		Candidate: domain.SemanticCandidate{ID: domain.CandidateIDPrefix + "test", Product: "demo"},
		Assertion: assertOn(
			&domain.Subject{Family: domain.SubjectImage, Product: "demo", Name: "example.org/demo/controller"},
			&domain.ChangeSpec{Type: domain.ChangeKindAdded}),
		Now: fixedNow,
	}
	if _, err := NewValidator(pairs).Validate(context.Background(), in); err == nil {
		t.Fatal("no from/to versions must be an error, not a silent pass")
	}
	// and an assertion with no subject or change is skipped
	in.From, in.To = vInput(in.Assertion).From, vInput(in.Assertion).To
	in.Assertion = domain.SemanticAssertion{}
	rs, err := NewValidator(pairs).Validate(context.Background(), in)
	if err != nil || len(rs) != 0 {
		t.Fatalf("empty assertion: %d results, err %v; want 0, nil", len(rs), err)
	}
}

// The validator's evidence is only ever release-level: handed an environment
// pair (customer values), its results fail validation instead of entering
// knowledge — the R5 rule, tested from the validator side.
func TestRenderedDiffNeverCitesEnvironmentRenders(t *testing.T) {
	p := goldenReleasePair(t)
	p.Scope = domain.RenderEnvironment
	for _, r := range []*Result{p.FromResult, p.ToResult} {
		r.Provenance.Scope = domain.RenderEnvironment
		r.Provenance.ValuesDigest = "sha256:customer-values"
	}
	_, err := NewValidator(&fixedPairs{p: p}).Validate(context.Background(), vInput(assertOn(
		&domain.Subject{Family: domain.SubjectImage, Product: "demo", Name: "example.org/demo/controller"},
		&domain.ChangeSpec{Type: domain.ChangeKindValueChanged, Before: sptr("v1.0.0"), After: sptr("v1.1.0")})))
	if err == nil || !strings.Contains(err.Error(), "environment render") {
		t.Fatalf("environment-scope evidence must be rejected by the contract, got %v", err)
	}
}

func TestRenderabilityTable(t *testing.T) {
	verifiable := []domain.SubjectFamily{domain.SubjectRBACPermission, domain.SubjectImage, domain.SubjectCLIFlag,
		domain.SubjectEnvVar, domain.SubjectGVK}
	for _, f := range verifiable {
		for _, k := range []domain.ChangeKind{domain.ChangeKindAdded, domain.ChangeKindRemoved,
			domain.ChangeKindValueChanged, domain.ChangeKindDefaultChanged} {
			want := domain.RenderVerifiable
			if f == domain.SubjectGVK && (k == domain.ChangeKindValueChanged || k == domain.ChangeKindDefaultChanged) {
				// a GVK's value is its storage version: a CRD flag, not a
				// rendered value (VALIDATOR-AUDIT.md; TestRenderedDiffStorageVersionIsNotApplicable)
				want = domain.RenderNotVerifiable
			}
			if got := RenderabilityOf(f, k); got != want {
				t.Errorf("%s %s = %s, want %s", f, k, got, want)
			}
		}
		if got := RenderabilityOf(f, domain.ChangeKindBehaviorChanged); got != domain.RenderNotVerifiable {
			t.Errorf("%s behavior-changed = %s, want not-render-verifiable", f, got)
		}
	}
	partial := []domain.SubjectFamily{domain.SubjectFeatureGate, domain.SubjectHelmValue, domain.SubjectCRDField, domain.SubjectConfigKey}
	for _, f := range partial {
		if got := RenderabilityOf(f, domain.ChangeKindValueChanged); got != domain.RenderPartiallyVerifiable {
			t.Errorf("%s value-changed = %s, want partially-render-verifiable", f, got)
		}
	}
	runtime := []domain.SubjectFamily{domain.SubjectProtocolBehavior, domain.SubjectAPIEndpoint, domain.SubjectMigration,
		domain.SubjectCompatibilityBoundary, domain.SubjectProductRelationship}
	for _, f := range runtime {
		if got := RenderabilityOf(f, domain.ChangeKindAdded); got != domain.RenderNotVerifiable {
			t.Errorf("%s added = %s, want not-render-verifiable (runtime-only family)", f, got)
		}
	}
	a := assertOn(&domain.Subject{Family: domain.SubjectImage, Product: "demo", Name: "x"}, &domain.ChangeSpec{Type: domain.ChangeKindAdded})
	if AssessRenderability(a) != domain.RenderVerifiable {
		t.Error("AssessRenderability must classify a full assertion")
	}
	if AssessRenderability(domain.SemanticAssertion{}) != "" {
		t.Error(`AssessRenderability of an assertion without subject/change is ""`)
	}
	for class := range RenderedClasses {
		if !classIsStructural(class) {
			t.Errorf("auto-approval class %s is not one of the R4 structural classes", class)
		}
	}
}

func classIsStructural(c ChangeClass) bool {
	switch c {
	case ResourceAdded, ResourceRemoved, FieldAdded, FieldRemoved, FieldChanged, ImageChanged,
		RBACPermissionAdded, RBACPermissionRemoved, ContainerArgAdded, ContainerArgRemoved, ContainerArgChanged,
		EnvVarAdded, EnvVarRemoved, EnvVarChanged, ServicePortAdded, ServicePortRemoved, ServicePortChanged:
		return true
	}
	return false
}
