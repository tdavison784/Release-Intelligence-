package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// --- fixtures: the rotationPolicy demonstration (DESIGN.md §10) ---------------

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func str(s string) *string { return &s }

func upEvidence() Evidence {
	return NewEvidence(EvidenceDocument, "notes", "https://example/upgrading-1.17-1.18.md", "L10-L12",
		"The default rotationPolicy is now Always", "sha256:doc", t0)
}

func crdEvidence() Evidence {
	return NewEvidence(EvidenceStructured, "crds", "https://example/crds.yaml", "$.spec.versions[0]",
		"rotationPolicy: default Always", "sha256:crd", t0)
}

func ptr[T any](v T) *T { return &v }

// certificates scopes predicates to one cert-manager Certificate.
func certificates(of ...Condition) Condition {
	return Condition{Op: OpResource, Group: "cert-manager.io", Kind: "Certificate", Of: of}
}

func rotationSubject() *Subject {
	return &Subject{Family: SubjectCRDField, Product: "cert-manager", Group: "cert-manager.io", Kind: "Certificate", Path: "spec.privateKey.rotationPolicy"}
}

func rotationAssertion() SemanticAssertion {
	return SemanticAssertion{
		Subject: rotationSubject(),
		Change:  &ChangeSpec{Type: ChangeKindDefaultChanged, Before: str(`"Never"`), After: str(`"Always"`)},
		Applicability: &Applicability{
			Exposure: certificates(Condition{Op: OpField, Path: "spec.privateKey.rotationPolicy", State: StateUnset}),
			Overlap:  ptr(certificates(Condition{Op: OpField, Path: "spec.privateKey.rotationPolicy", State: StateSet})),
		},
		// D11: the dataset labels this item review; the class is the reviewer's, carried by the fact.
		Consequence: &Consequence{Kind: ConsequenceBehaviorChange, ExposedClass: ImpactReviewRequired,
			Statement: "private keys are regenerated on every renewal", Severity: SeverityHigh},
		Statement: "Certificate.spec.privateKey.rotationPolicy default Never → Always",
	}
}

func anchor() ChangeAnchor {
	return ChangeAnchor{Release: "v1.18.0", EvidenceKeys: []string{EvidenceKey(upEvidence())},
		StatementKeys: []string{StatementKey(upEvidence())}, ChangeIDs: []string{"chg-rot"}}
}

func validCandidate() SemanticCandidate {
	a := anchor()
	members := []CandidateMember{{ChangeID: "chg-rot", Anchor: &a}}
	return SemanticCandidate{
		ID: CandidateID("cert-manager", a.Release, members), Product: "cert-manager", Release: a.Release,
		Members: members, Grouping: "single",
		Category: CategoryConfiguration, Title: "The default rotationPolicy is now Always",
		Evidence: []Evidence{upEvidence(), crdEvidence()}, Producer: "semantic.candidates@v1", CreatedAt: t0,
	}
}

func aiProvenance(model string, input ...EvidenceID) Provenance {
	at := t0
	return Provenance{Method: MethodAI, Producer: "semantic.propose@v1", Confidence: ConfidenceMedium,
		Model: model, ModelVersion: model, PromptVersion: "semantic/v1", PromptDigest: "sha256:" + model,
		InputEvidence: input, GeneratedAt: &at, CallID: "call-" + model}
}

func validProposal(c SemanticCandidate) SemanticProposal {
	a := rotationAssertion()
	p := SemanticProposal{
		CandidateID: c.ID, Task: TaskFull, Provider: "zai", Assertion: a,
		SuggestedClass: ImpactReviewRequired, Citations: []EvidenceID{upEvidence().ID},
		Provenance: aiProvenance("glm-5.3-flash", upEvidence().ID, crdEvidence().ID),
	}
	p.ID = ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	return p
}

func validValidation(c SemanticCandidate) ValidationResult {
	a := rotationAssertion()
	v := ValidationResult{
		CandidateID: c.ID, Validator: "semvalidate.crd@v1", Assertion: a,
		Checks: []AspectCheck{
			{Aspect: AspectSubject, Outcome: OutcomeConfirmed, Rule: "crd-schema:field"},
			{Aspect: AspectChange, Outcome: OutcomeConfirmed, Rule: "crd-schema:default"},
			{Aspect: AspectApplicability, Outcome: OutcomeConfirmed, Rule: "canonical:crd-field/default-changed"},
		},
		Evidence: []Evidence{crdEvidence()}, CheckedAt: t0,
	}
	v.ID = ValidationID(v.CandidateID, v.Validator, v.Assertion)
	return v
}

func validItem(c SemanticCandidate) ReviewItem {
	a := rotationAssertion()
	r := ReviewItem{
		CandidateID: c.ID, Product: c.Product, Release: c.Release, QuestionType: QuestionConsequence,
		Question: "If a Certificate with rotationPolicy unset does nothing, what happens?",
		Proposed: a, Routing: Routing{Route: RouteReview, Priority: PriorityHigh, Signals: []RoutingSignal{SignalModelsDisagree, SignalHighImpact}},
		Status: ReviewPending, CreatedAt: t0,
	}
	r.ID = ReviewItemID(r.CandidateID, r.QuestionType, r.Proposed)
	return r
}

func validDecision(item ReviewItem) ReviewDecision {
	a := item.Proposed
	d := ReviewDecision{
		ReviewItemID: item.ID, Action: ActionAccept, Labels: []FeedbackLabel{LabelAccepted},
		Original: &a, Reviewer: "engineer-1", ReviewerKind: ReviewerHuman,
		StartedAt: t0, DecidedAt: t0.Add(95 * time.Second),
	}
	d.ID = DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	return d
}

func validFact(c SemanticCandidate, v ValidationResult, d ReviewDecision) VerifiedFact {
	f := VerifiedFact{
		Product: c.Product, Release: c.Release, Anchors: c.Anchors(), Candidates: []string{c.ID},
		Assertion: rotationAssertion(),
		Verification: []AspectVerification{
			{Aspect: AspectSubject, Level: VerifiedDeterministic, Basis: []string{v.ID}},
			{Aspect: AspectChange, Level: VerifiedDeterministic, Basis: []string{v.ID}},
			{Aspect: AspectApplicability, Level: VerifiedDeterministic, Basis: []string{v.ID}},
			{Aspect: AspectConsequence, Level: VerifiedHuman, Basis: []string{d.ID}},
		},
		Evidence: []Evidence{upEvidence(), crdEvidence()}, Status: FactActive, CreatedAt: t0,
	}
	f.ID = VerifiedFactID(f.Product, f.Release, f.Assertion)
	return f
}

// expectErr asserts err is nil (want == "") or mentions want.
func expectErr(t *testing.T, err error, want string) {
	t.Helper()
	switch {
	case want == "" && err != nil:
		t.Fatalf("unexpected error: %v", err)
	case want != "" && err == nil:
		t.Fatalf("expected an error mentioning %q, got none", want)
	case want != "" && !strings.Contains(err.Error(), want):
		t.Fatalf("expected an error mentioning %q, got: %v", want, err)
	}
}

// --- subject / change / condition / consequence ------------------------------------

func TestSubjectValidate(t *testing.T) {
	cases := []struct {
		name string
		s    Subject
		want string
	}{
		{"crd field", *rotationSubject(), ""},
		{"core gvk without group", Subject{Family: SubjectGVK, Product: "k", Version: "v1", Kind: "Service"}, ""},
		{"unknown family", Subject{Family: "widget", Product: "p", Name: "x"}, "unknown family"},
		{"missing product", Subject{Family: SubjectImage, Name: "quay.io/x"}, "product is required"},
		{"missing required", Subject{Family: SubjectCRDField, Product: "p", Group: "g", Kind: "K"}, "path is required"},
		{"foreign field", Subject{Family: SubjectImage, Product: "p", Name: "quay.io/x", Path: "spec"}, "path is not part"},
		{"terraform", Subject{Family: SubjectTerraformAttribute, Product: "aws", Kind: "aws_s3_bucket", Path: "acl"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { expectErr(t, c.s.Validate(), c.want) })
	}
}

func TestSubjectKeyIsCanonical(t *testing.T) {
	a, b := *rotationSubject(), *rotationSubject()
	if a.Key() != b.Key() {
		t.Fatal("equal subjects must have equal keys")
	}
	b.Version = "v1"
	if a.Key() == b.Key() {
		t.Fatal("a version-pinned subject must differ")
	}
	want := "crd-field:cert-manager|group=cert-manager.io|kind=Certificate|path=spec.privateKey.rotationPolicy"
	if a.Key() != want {
		t.Fatalf("Key() = %q, want %q", a.Key(), want)
	}
}

func TestChangeSpecValidate(t *testing.T) {
	cases := []struct {
		name   string
		c      ChangeSpec
		family SubjectFamily
		want   string
	}{
		{"default changed", ChangeSpec{Type: ChangeKindDefaultChanged, Before: str(`"a"`), After: str(`"b"`)}, SubjectHelmValue, ""},
		{"default changed missing before", ChangeSpec{Type: ChangeKindDefaultChanged, After: str(`"b"`)}, SubjectHelmValue, "before and after are required"},
		{"default unchanged", ChangeSpec{Type: ChangeKindDefaultChanged, Before: str(`1`), After: str(`1`)}, SubjectHelmValue, "before equals after"},
		{"unknown type", ChangeSpec{Type: "mutated"}, SubjectHelmValue, "unknown type"},
		{"requirement ok", ChangeSpec{Type: ChangeKindRequirementChanged, After: str(">=1.12.6")}, SubjectProductRelationship, ""},
		{"requirement bad range", ChangeSpec{Type: ChangeKindRequirementChanged, After: str("newer please")}, SubjectProductRelationship, "not a version constraint"},
		{"requirement wrong family", ChangeSpec{Type: ChangeKindRequirementChanged, After: str(">=1")}, SubjectHelmValue, "applies to compatibility-boundary"},
		{"migration family mismatch", ChangeSpec{Type: ChangeKindMigrationRequired}, SubjectHelmValue, "go together"},
		{"migration ok", ChangeSpec{Type: ChangeKindMigrationRequired}, SubjectMigration, ""},
		{"renamed without target", ChangeSpec{Type: ChangeKindRenamed}, SubjectHelmValue, "replacedBy is required"},
		{"renamed across families", ChangeSpec{Type: ChangeKindRenamed, ReplacedBy: &Subject{Family: SubjectEnvVar, Product: "p", Name: "X"}}, SubjectHelmValue, "differs from subject family"},
		{"deprecated and replaced", ChangeSpec{Type: ChangeKindDeprecated, ReplacedBy: &Subject{Family: SubjectHelmValue, Product: "cilium", Path: "tls.readSecretsOnlyFromSecretsNamespace"}}, SubjectHelmValue, ""},
		{"replacedBy on a default change", ChangeSpec{Type: ChangeKindDefaultChanged, Before: str("1"), After: str("2"), ReplacedBy: &Subject{Family: SubjectHelmValue, Product: "p", Path: "a"}}, SubjectHelmValue, "renamed/deprecated/removed only"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { expectErr(t, c.c.Validate(c.family), c.want) })
	}
}

func TestConditionValidate(t *testing.T) {
	leaf := Condition{Op: OpValuesKey, Path: "webhook.timeoutSeconds", State: StateUnset}
	field := Condition{Op: OpField, Path: "spec.privateKey.rotationPolicy", State: StateUnset}
	deep := leaf
	for i := 0; i < MaxConditionDepth; i++ {
		deep = Condition{Op: OpAll, Of: []Condition{deep}}
	}
	cases := []struct {
		name string
		c    Condition
		want string
	}{
		{"values key unset", leaf, ""},
		{"values key equals/in", Condition{Op: OpValuesKey, Path: "a", State: StateEquals, Values: []string{`"x"`, `2`}}, ""},
		{"equals without values", Condition{Op: OpValuesKey, Path: "a", State: StateEquals}, "needs values"},
		{"values with unset", Condition{Op: OpValuesKey, Path: "a", State: StateUnset, Values: []string{`1`}}, "only used with"},
		{"value not json", Condition{Op: OpValuesKey, Path: "a", State: StateEquals, Values: []string{"Never"}}, "not JSON-encoded"},
		{"missing state", Condition{Op: OpValuesKey, Path: "a"}, "state is required"},
		{"gate state on values", Condition{Op: OpValuesKey, Path: "a", State: StateEnabled}, "not allowed"},
		{"foreign field", Condition{Op: OpValuesKey, Path: "a", State: StateSet, Kind: "Pod"}, "kind is not a field"},
		{"unknown op", Condition{Op: "regex"}, "unknown op"},
		{"empty combinator", Condition{Op: OpAny}, "at least one operand"},
		{"combinator with fields", Condition{Op: OpAll, Of: []Condition{leaf}, Path: "x"}, "path is not a field"},
		{"leaf with operands", Condition{Op: OpGVKInUse, Kind: "Ingress", Of: []Condition{leaf}}, "takes no operands"},
		{"nested error located", Condition{Op: OpAll, Of: []Condition{leaf, {Op: OpEnvVar}}}, "all[1]"},
		{"too deep", deep, "deeper than"},
		{"not takes one operand", Condition{Op: OpNot, Of: []Condition{leaf, leaf}}, "exactly one operand"},
		{"not", Condition{Op: OpNot, Of: []Condition{leaf}}, ""},
		{"scoped field", certificates(field), ""},
		{"field outside a scope", field, "only valid inside a resource"},
		{"environment-wide leaf inside a scope", certificates(leaf), "cannot appear inside"},
		{"resource without operands", Condition{Op: OpResource, Kind: "Certificate"}, "at least one operand"},
		{"regex on a field", certificates(Condition{Op: OpField, Path: "spec.metrics.overrides[].match.metric", State: StateMatches, Pattern: `filter_state\["wasm\.`}), ""},
		{"bad regex", certificates(Condition{Op: OpField, Path: "a", State: StateMatches, Pattern: "("}), "pattern"},
		{"pattern without matches", certificates(Condition{Op: OpField, Path: "a", State: StateSet, Pattern: "x"}), "exactly with state matches"},
		{"text-line needs a pattern", certificates(Condition{Op: OpTextLine, Path: "data", State: StateExists}), "pattern is required"},
		{"text-line state matches", certificates(Condition{Op: OpTextLine, Path: "data", State: StateMatches, Pattern: "x"}), "not allowed"},
		{"negated text-line", certificates(Condition{Op: OpNot, Of: []Condition{{Op: OpTextLine, Path: "data", State: StateExists, Pattern: "x"}}}), ""},
		{"token in a k=v list", Condition{Op: OpValuesKey, Path: "featureGates", State: StateHasTokenKey, Values: []string{"ValidateCAA"}}, ""},
		{"token without values", Condition{Op: OpCLIFlag, Name: "--feature-gates", State: StateHasToken}, "needs values"},
		{"separator without token state", Condition{Op: OpValuesKey, Path: "a", State: StateSet, Separator: ";"}, "separator is only used"},
		{"no line matches (negated text-line)", Condition{Op: OpResource, Kind: "ConfigMap", Name: "argocd-rbac-cm", Of: []Condition{
			{Op: OpTextLine, Path: `data["policy.csv"]`, State: StateNone, Pattern: `^p,.*,applications,update/\*`},
		}}, ""},
		{"cross-resource ref", certificates(Condition{Op: OpRef, Path: "spec.issuerRef", Kind: "Issuer", Group: "cert-manager.io",
			Of: []Condition{{Op: OpField, Path: "spec.ca", State: StateSet}}}), ""},
		{"ref outside a scope", Condition{Op: OpRef, Path: "spec.issuerRef", Of: []Condition{field}}, "only valid inside"},
		{"ref without path", certificates(Condition{Op: OpRef, Of: []Condition{field}}), "path is required"},
		{"gvk no state", Condition{Op: OpGVKInUse, Group: "networking.k8s.io", Version: "v1beta1", Kind: "Ingress"}, ""},
		{"gvk with state", Condition{Op: OpGVKInUse, Kind: "Ingress", State: StateSet}, "takes no state"},
		{"product version", Condition{Op: OpProductVersion, Name: "ingress-nginx", State: StateOutOfRange, Range: ">=1.12.6"}, ""},
		{"product version bad range", Condition{Op: OpProductVersion, Name: "ingress-nginx", State: StateInRange, Range: "recent"}, "range"},
		{"product version no range", Condition{Op: OpProductVersion, Name: "ingress-nginx", State: StateInRange}, "range is required"},
		{"rendered change", Condition{Op: OpRenderedChange, Group: "rbac.authorization.k8s.io", Kind: "ClusterRole", Path: "rules[].verbs", State: StateChanged}, ""},
		{"rendered change to a value", Condition{Op: OpRenderedChange, Group: "apps", Kind: "Deployment", Path: "spec.template.spec.containers[].args", State: StateAdded, Values: []string{`"--enable-gateway-api"`}}, ""},
		{"rendered change bad state", Condition{Op: OpRenderedChange, Kind: "Deployment", Path: "spec", State: StateSet}, "not allowed"},
		{"rendered change inside a scope", certificates(Condition{Op: OpRenderedChange, Kind: "Deployment", Path: "spec", State: StateRemoved}), "cannot appear inside"},
		{"edge from-version (precondition met)", Condition{Op: OpEdgeFromVersion, State: StateInRange, Range: ">=1.15.6"}, ""},
		{"image any tag", Condition{Op: OpImageInUse, Name: "quay.io/jetstack/cert-manager-controller"}, ""},
		{"image range without state", Condition{Op: OpImageInUse, Name: "quay.io/x", Range: "<1.18"}, "go together"},
		{"feature gate", Condition{Op: OpFeatureGate, Name: "ServerSideApply", State: StateEnabled, Path: "featureGates"}, ""},
		{"undecidable", Condition{Op: OpUndecidable, Reason: UnknownRuntimeBehaviorGap, Needed: "live ACME traffic"}, ""},
		{"undecidable bad reason", Condition{Op: OpUndecidable, Reason: "vibes", Needed: "x"}, "unknown reason"},
		{"undecidable without needed", Condition{Op: OpUndecidable, Reason: UnknownEvidenceGap}, "needed is required"},
		{"old set and new unset (replacedBy)", Condition{Op: OpAll, Of: []Condition{
			{Op: OpValuesKey, Path: "tls.secretsBackend", State: StateSet},
			{Op: OpValuesKey, Path: "tls.readSecretsOnlyFromSecretsNamespace", State: StateUnset},
		}}, ""},
		{"cross-product composite", Condition{Op: OpAll, Of: []Condition{
			{Op: OpProductVersion, Name: "ingress-nginx", State: StateOutOfRange, Range: "<1.12.0"},
			{Op: OpResource, Group: "acme.cert-manager.io", Kind: "Issuer", Of: []Condition{{Op: OpField, Path: "spec.acme.solvers[].http01.ingress", State: StateSet}}},
		}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { expectErr(t, c.c.Validate(), c.want) })
	}
}

func TestConsequence(t *testing.T) {
	cases := []struct {
		name string
		c    Consequence
		want string
	}{
		{"setting ignored is action", Consequence{Kind: ConsequenceSettingIgnored, ExposedClass: ImpactActionRequired, Statement: "the value stops taking effect"}, ""},
		{"action kind without statement", Consequence{Kind: ConsequenceSettingIgnored, ExposedClass: ImpactActionRequired}, "statement"},
		{"action kind softened by hand", Consequence{Kind: ConsequenceWorkloadFailure, ExposedClass: ImpactReviewRequired, Statement: "x"}, "class follows the kind"},
		{"behavior change is review (D11)", Consequence{Kind: ConsequenceBehaviorChange, ExposedClass: ImpactReviewRequired}, ""},
		{"behavior change promoted to action", Consequence{Kind: ConsequenceBehaviorChange, ExposedClass: ImpactActionRequired, Statement: "x"}, "must be review-required"},
		{"deprecation promoted to action", Consequence{Kind: ConsequenceDeprecation, ExposedClass: ImpactActionRequired, Statement: "x"}, "must be review-required"},
		{"none must be informational", Consequence{Kind: ConsequenceNone, ExposedClass: ImpactReviewRequired}, "must be informational"},
		{"missing exposed class", Consequence{Kind: ConsequenceDeprecation}, "exposedClass must be"},
		{"bad severity", Consequence{Kind: ConsequenceDeprecation, ExposedClass: ImpactReviewRequired, Severity: "apocalyptic"}, "unknown severity"},
		{"unknown kind", Consequence{Kind: "explodes", ExposedClass: ImpactReviewRequired}, "unknown kind"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { expectErr(t, c.c.Validate(), c.want) })
	}
	for _, k := range ConsequenceKinds {
		if (k.ExposedClass() == ImpactActionRequired) != k.ActionEligible() {
			t.Errorf("%s: action-required exactly for action-eligible kinds", k)
		}
		if !k.ExposedClass().Affected() {
			t.Errorf("%s: an exposed class is an affected class", k)
		}
	}
	if ConsequenceBehaviorChange.ActionEligible() {
		t.Error("behavior-change (works differently; verify) is never action-eligible")
	}
}

func TestAssertionDigests(t *testing.T) {
	a, b := rotationAssertion(), rotationAssertion()
	b.Statement = "reworded"
	b.Consequence.Statement = "reworded too"
	if a.Digest() != b.Digest() {
		t.Fatal("prose must not change digests (wording is not disagreement)")
	}
	b.Change.After = str(`"Sometimes"`)
	if a.AspectDigest(AspectChange) == b.AspectDigest(AspectChange) || a.Digest() == b.Digest() {
		t.Fatal("a different after-value must change the change digest")
	}
	if a.AspectDigest(AspectSubject) != b.AspectDigest(AspectSubject) {
		t.Fatal("subject digest must be independent of the change")
	}
	if (SemanticAssertion{}).AspectDigest(AspectSubject) != "" {
		t.Fatal("an absent aspect has an empty digest")
	}
	expectErr(t, SemanticAssertion{Subject: rotationSubject()}.Validate(true), "change is required")
	expectErr(t, rotationAssertion().Validate(true), "")
}

func TestConsensusLevelOrdering(t *testing.T) {
	if VerifiedConsensus.Trusted() {
		t.Fatal("consensus is never trusted")
	}
	for _, c := range []struct {
		l, min VerificationLevel
		want   bool
	}{
		{VerifiedConsensus, VerifiedConsensus, true}, {VerifiedHuman, VerifiedConsensus, true},
		{VerifiedProxy, VerifiedConsensus, false}, {VerifiedConsensus, VerifiedProxy, true},
		{VerifiedConsensus, VerifiedHuman, false}, {VerifiedConsensus, VerifiedDeterministic, false},
	} {
		if got := c.l.AtLeast(c.min); got != c.want {
			t.Errorf("%s.AtLeast(%s) = %v, want %v", c.l, c.min, got, c.want)
		}
	}
	for _, c := range []struct{ a, b, want VerificationLevel }{
		{VerifiedDeterministic, VerifiedDeterministic, VerifiedDeterministic},
		{VerifiedDeterministic, VerifiedHuman, VerifiedHuman},
		{VerifiedHuman, VerifiedConsensus, VerifiedConsensus},
		{VerifiedConsensus, VerifiedProxy, VerifiedProxy},
	} {
		if got := weaker(c.a, c.b); got != c.want {
			t.Errorf("weaker(%s, %s) = %s, want %s", c.a, c.b, got, c.want)
		}
	}
}

func TestSeparateCallsAndScope(t *testing.T) {
	fam := map[string]string{"claude-opus-5-5": "claude", "claude-sonnet-5-5": "claude", "glm-5.3-flash": "glm",
		"gpt-5": "gpt", "zai/glm-5.3": "glm", "Gemini-2.5-Pro": "gemini"}
	for m, want := range fam {
		if got := ModelFamily(m); got != want {
			t.Errorf("ModelFamily(%q) = %q, want %q", m, got, want)
		}
	}
	p := func(id, model, call string) SemanticProposal {
		return SemanticProposal{ID: id, Provenance: Provenance{Model: model, CallID: call}}
	}
	cases := []struct {
		name string
		a, b SemanticProposal
		want bool
	}{
		{"two Opus calls (PO-1)", p("sp-1", "claude-opus-5-5", "msg_1"), p("sp-2", "claude-opus-5-5", "msg_2"), true},
		{"GLM and Claude", p("sp-1", "glm-5.3-flash", "req_1"), p("sp-2", "claude-sonnet-5-5", "msg_1"), true},
		{"one call replayed", p("sp-1", "claude-opus-5-5", "msg_1"), p("sp-2", "claude-opus-5-5", "msg_1"), false},
		{"same proposal", p("sp-1", "claude-opus-5-5", "msg_1"), p("sp-1", "claude-opus-5-5", "msg_2"), false},
		{"missing call id", p("sp-1", "claude-opus-5-5", ""), p("sp-2", "claude-opus-5-5", "msg_2"), false},
	}
	for _, c := range cases {
		if got := SeparateCalls(c.a, c.b); got != c.want {
			t.Errorf("%s: SeparateCalls = %v, want %v", c.name, got, c.want)
		}
	}
	if s := ConsensusScopeOf([]SemanticProposal{p("a", "claude-opus-5-5", "1"), p("b", "claude-sonnet-5-5", "2")}); s != ConsensusSameModel {
		t.Errorf("two Claude models = %s, want same-model", s)
	}
	if s := ConsensusScopeOf([]SemanticProposal{p("a", "glm-5.3-flash", "1"), p("b", "claude-sonnet-5-5", "2")}); s != ConsensusCrossModel {
		t.Errorf("GLM + Claude = %s, want cross-model", s)
	}
}

func TestVerificationLevels(t *testing.T) {
	if !VerifiedHuman.AtLeast(VerifiedHuman) || !VerifiedDeterministic.AtLeast(VerifiedHuman) || VerifiedProxy.AtLeast(VerifiedHuman) {
		t.Error("human minimum admits deterministic and human, not proxy")
	}
	if VerifiedHuman.AtLeast(VerifiedDeterministic) || !VerifiedProxy.AtLeast(VerifiedProxy) {
		t.Error("deterministic minimum admits only deterministic; proxy minimum admits proxy")
	}
	if VerifiedProxy.Trusted() || !VerifiedHuman.Trusted() {
		t.Error("only deterministic and human are trusted")
	}
}

// --- anchor --------------------------------------------------------------------

func TestChangeAnchorMatches(t *testing.T) {
	ev := upEvidence()
	shifted := ev // the same statement after lines moved above it and markup changed
	shifted.Locator, shifted.Excerpt, shifted.ContentDigest = "L40-L42", "The default `rotationPolicy` is now **Always**.", "sha256:v2"
	shifted.ID = "ev-shifted"
	other := NewEvidence(EvidenceDocument, "notes", ev.URI, "L90", "ACME profiles are supported", "sha256:doc", t0)
	values := NewEvidence(EvidenceStructured, "chart", "https://example/values.yaml", "", "", "sha256:values", t0)
	pool := map[EvidenceID]Evidence{ev.ID: ev, shifted.ID: shifted, other.ID: other, values.ID: values}
	lookup := func(id EvidenceID) (Evidence, bool) { e, ok := pool[id]; return e, ok }
	declared := Provenance{Method: MethodDeclared, Producer: "normalize.notes@v1", Confidence: ConfidenceHigh}
	computed := Provenance{Method: MethodComputed, Producer: "upgrade@v1", Confidence: ConfidenceHigh}
	c := Change{ID: "chg-rot", Release: "v1.18.0", Evidence: []EvidenceID{ev.ID}, Provenance: declared}
	a := NewChangeAnchor(c, lookup)
	if len(a.StatementKeys) != 1 || a.ChangeIDs[0] != "chg-rot" {
		t.Fatalf("anchor = %+v", a)
	}
	cases := []struct {
		name string
		c    Change
		want bool
	}{
		{"same change", c, true},
		{"release unstated", Change{ID: "chg-rot", Evidence: []EvidenceID{ev.ID}, Provenance: declared}, true},
		{"re-ingested: new chg id, shifted lines, new markup", Change{ID: "chg-new", Release: "v1.18.0", Evidence: []EvidenceID{shifted.ID}, Provenance: declared}, true},
		{"same chg id, different statement (never keyed on chg ids)", Change{ID: "chg-rot", Release: "v1.18.0", Evidence: []EvidenceID{other.ID}, Provenance: declared}, false},
		{"same statement in another release", Change{ID: "chg-rot", Release: "v1.19.0", Evidence: []EvidenceID{ev.ID}, Provenance: declared}, false},
		{"computed change sharing structured evidence", Change{ID: "chg-diff", Release: "v1.18.0", Evidence: []EvidenceID{ev.ID}, Provenance: computed}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := a.Matches(tc.c, lookup); got != tc.want {
				t.Fatalf("Matches = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("umbrella", func(t *testing.T) {
		umbrella := Change{ID: "chg-u", Release: "v1.18.0", Evidence: []EvidenceID{ev.ID, other.ID}, Provenance: declared}
		if !IsUmbrella(umbrella, lookup) {
			t.Fatal("two distinct statements of one document make an umbrella")
		}
		listed := Change{ID: "chg-l", Release: "v1.18.0", Evidence: []EvidenceID{ev.ID}, Provenance: declared,
			Detail: "Feature gate changes:\n- NameConstraints is beta\n- UseDomainQualifiedFinalizer is beta"}
		if !IsUmbrella(listed, lookup) {
			t.Fatal("a detail listing two items makes an umbrella")
		}
		restated := Change{ID: "chg-r", Release: "v1.18.0", Evidence: []EvidenceID{ev.ID, shifted.ID}, Provenance: declared}
		if IsUmbrella(restated, lookup) {
			t.Fatal("the same statement quoted twice is a restatement, not an umbrella")
		}
		cand := validCandidate()
		v := validValidation(cand)
		f := validFact(cand, v, validDecision(validItem(cand)))
		if !f.AttachesByAnchor(c, lookup) || f.AttachesByAnchor(umbrella, lookup) {
			t.Fatal("a fact attaches to its restatements and never to an umbrella")
		}
		f.Status = FactRetracted
		if f.AttachesByAnchor(c, lookup) {
			t.Fatal("a retracted fact attaches nowhere")
		}
	})
}

// --- entities ------------------------------------------------------------------

func TestCandidateValidate(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*SemanticCandidate)
		want string
	}{
		{"valid", func(*SemanticCandidate) {}, ""},
		{"id not derived", func(c *SemanticCandidate) { c.ID = "sc-made-up" }, "not derived"},
		{"no evidence", func(c *SemanticCandidate) { c.Evidence = nil }, "no evidence"},
		{"environment evidence", func(c *SemanticCandidate) {
			c.Evidence = append(c.Evidence, NewEvidence(EvidenceLocalFile, "", "values.yaml", "L1", "x: 1", "sha256:v", time.Time{}))
		}, "release-level"},
		{"release mismatch", func(c *SemanticCandidate) { c.Release = "v9"; c.ID = CandidateID(c.Product, c.Release, c.Members) }, "anchor release"},
		{"anchor without statement keys", func(c *SemanticCandidate) {
			a := *c.Members[0].Anchor
			a.StatementKeys = nil
			c.Members[0].Anchor = &a
			c.ID = CandidateID(c.Product, c.Release, c.Members)
		}, "statement key is required"},
		{"restatement cluster: notes + guide + computed diff", func(c *SemanticCandidate) {
			guide := NewChangeAnchor(Change{ID: "chg-guide", Release: "v1.18.0", Evidence: []EvidenceID{crdEvidence().ID}},
				func(EvidenceID) (Evidence, bool) { return crdEvidence(), true })
			c.Members = append(c.Members, CandidateMember{ChangeID: "chg-guide", Anchor: &guide},
				CandidateMember{ChangeID: "chg-crd-default", Computed: true})
			c.Grouping = "subject-named"
			c.ID = CandidateID(c.Product, c.Release, c.Members)
		}, ""},
		{"computed member with an anchor", func(c *SemanticCandidate) {
			c.Members[0].Computed = true
			c.ID = CandidateID(c.Product, c.Release, c.Members)
		}, "a prose member carries an anchor"},
		{"member twice", func(c *SemanticCandidate) {
			c.Members = append(c.Members, c.Members[0])
			c.ID = CandidateID(c.Product, c.Release, c.Members)
		}, "listed twice"},
		{"no members", func(c *SemanticCandidate) { c.Members = nil; c.ID = CandidateID(c.Product, c.Release, nil) }, "no members"},
		{"no grouping", func(c *SemanticCandidate) { c.Grouping = "" }, "grouping"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validCandidate()
			tc.mut(&c)
			expectErr(t, c.Validate(), tc.want)
		})
	}
}

func TestProposalValidate(t *testing.T) {
	cand := validCandidate()
	rehash := func(p *SemanticProposal) { p.ID = ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance) }
	cases := []struct {
		name string
		mut  func(*SemanticProposal)
		want string
	}{
		{"valid", func(*SemanticProposal) {}, ""},
		{"deterministic provenance", func(p *SemanticProposal) {
			p.Provenance = Provenance{Method: MethodHeuristic, Producer: "x@v1", Confidence: ConfidenceLow}
			rehash(p)
		}, "must have ai provenance"},
		{"missing model version", func(p *SemanticProposal) { p.Provenance.ModelVersion = "" }, "modelVersion"},
		{"high confidence", func(p *SemanticProposal) { p.Provenance.Confidence = ConfidenceHigh }, "capped at medium"},
		{"no provider", func(p *SemanticProposal) { p.Provider = ""; rehash(p) }, "provider is required"},
		{"requests action on a behavior change", func(p *SemanticProposal) { p.SuggestedClass = ImpactActionRequired }, "must assert an action-eligible consequence"},
		{"requests action on a failure (PO-2)", func(p *SemanticProposal) {
			p.SuggestedClass = ImpactActionRequired
			p.Assertion.Consequence = &Consequence{Kind: ConsequenceSettingIgnored, ExposedClass: ImpactActionRequired, Statement: "keys stop rotating"}
		}, ""},
		{"no call id", func(p *SemanticProposal) { p.Provenance.CallID = ""; rehash(p) }, "callId"},
		{"suggests not-affected", func(p *SemanticProposal) { p.SuggestedClass = ImpactNotAffected }, "not \"not-affected\""},
		{"cites outside input", func(p *SemanticProposal) { p.Citations = []EvidenceID{"ev-invented"} }, "not part of its input"},
		{"asserts without citing", func(p *SemanticProposal) { p.Citations = nil }, "must cite evidence"},
		{"outside task scope", func(p *SemanticProposal) { p.Task = TaskConsequence; rehash(p) }, "outside task consequence"},
		{"task aspect unanswered", func(p *SemanticProposal) { p.Assertion.Consequence = nil }, "neither asserted nor undetermined"},
		{"abstains with reason", func(p *SemanticProposal) {
			p.Assertion = SemanticAssertion{}
			p.Undetermined = Aspects
			p.UndeterminedReason = "the note does not name a resource"
			p.Citations = nil
			p.SuggestedClass = ImpactUnknown
		}, ""},
		{"abstains without reason", func(p *SemanticProposal) {
			p.Assertion.Consequence = nil
			p.Undetermined = []Aspect{AspectConsequence}
		}, "undeterminedReason is required"},
		{"asserted and undetermined", func(p *SemanticProposal) {
			p.Undetermined = []Aspect{AspectSubject}
			p.UndeterminedReason = "x"
		}, "both asserted and undetermined"},
		{"duplicateOf outside duplicate task", func(p *SemanticProposal) { p.DuplicateOf = "vf-1" }, "duplicate task"},
		{"invalid assertion", func(p *SemanticProposal) { p.Assertion.Subject.Path = "" }, "path is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := validProposal(cand)
			tc.mut(&p)
			expectErr(t, p.Validate(), tc.want)
		})
	}
	t.Run("citation must be candidate evidence", func(t *testing.T) {
		p := validProposal(cand)
		other := NewEvidence(EvidenceDocument, "x", "https://elsewhere", "", "x", "", t0)
		p.Provenance.InputEvidence = append(p.Provenance.InputEvidence, other.ID)
		p.Citations = []EvidenceID{other.ID}
		rehash(&p)
		expectErr(t, p.Validate(), "")
		expectErr(t, p.ValidateAgainst(cand), "is not evidence of candidate")
	})
}

func TestValidationResultValidate(t *testing.T) {
	cand := validCandidate()
	cases := []struct {
		name string
		mut  func(*ValidationResult)
		want string
	}{
		{"valid", func(*ValidationResult) {}, ""},
		{"confirmation without evidence", func(v *ValidationResult) { v.Evidence = nil }, "must cite the artifact evidence"},
		{"validator not a producer", func(v *ValidationResult) {
			v.Validator = "crd"
			v.ID = ValidationID(v.CandidateID, v.Validator, v.Assertion)
		}, "component@vN"},
		{"validators do not judge consequences", func(v *ValidationResult) {
			v.Checks = append(v.Checks, AspectCheck{Aspect: AspectConsequence, Outcome: OutcomeConfirmed, Rule: "x"})
		}, "consequence is a judgement"},
		{"checks an unstated aspect", func(v *ValidationResult) {
			v.Assertion.Consequence = nil
			v.ID = ValidationID(v.CandidateID, v.Validator, v.Assertion)
			v.Checks = append(v.Checks, AspectCheck{Aspect: AspectConsequence, Outcome: OutcomeInconclusive, Rule: "x"})
		}, "does not state"},
		{"duplicate aspect", func(v *ValidationResult) { v.Checks = append(v.Checks, v.Checks[0]) }, "checked twice"},
		{"refutation needs no evidence", func(v *ValidationResult) {
			v.Checks = []AspectCheck{{Aspect: AspectChange, Outcome: OutcomeRefuted, Rule: "crd-schema:default"}}
			v.Evidence = nil
		}, ""},
		{"environment evidence", func(v *ValidationResult) {
			v.Evidence = []Evidence{NewEvidence(EvidenceInput, "", "--kubernetes", "", "1.28", "", time.Time{})}
		}, "release-level"},
		{"chart-default render backs a validation", func(v *ValidationResult) {
			e := NewEvidence(EvidenceStructured, "chart", "templates/clusterrole.yaml", "rules[0].verbs", "verbs: [get]", "sha256:r", t0)
			e.Render = &RenderProvenance{Scope: RenderRelease, Tool: "helm", ToolVersion: "v3.17.2", ChartDigest: "sha256:chart"}
			v.Evidence = []Evidence{e}
		}, ""},
		{"environment render never backs knowledge", func(v *ValidationResult) {
			e := NewEvidence(EvidenceStructured, "chart", "templates/clusterrole.yaml", "rules[0].verbs", "verbs: [get]", "sha256:r", t0)
			e.Render = &RenderProvenance{Scope: RenderEnvironment, Tool: "helm", ToolVersion: "v3.17.2", ChartDigest: "sha256:chart", ValuesDigest: "sha256:v"}
			v.Evidence = []Evidence{e}
		}, "environment render"},
		{"incomplete render provenance", func(v *ValidationResult) {
			e := NewEvidence(EvidenceStructured, "chart", "templates/x.yaml", "", "x", "sha256:r", t0)
			e.Render = &RenderProvenance{Scope: RenderRelease, Tool: "helm"}
			v.Evidence = []Evidence{e}
		}, "toolVersion and chartDigest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := validValidation(cand)
			tc.mut(&v)
			expectErr(t, v.Validate(), tc.want)
		})
	}
}

func TestReviewItemValidate(t *testing.T) {
	cand := validCandidate()
	cases := []struct {
		name string
		mut  func(*ReviewItem)
		want string
	}{
		{"valid", func(*ReviewItem) {}, ""},
		{"auto-verify is not a queue", func(r *ReviewItem) { r.Routing.Route = RouteAutoVerify }, "produce no review item"},
		{"agree and disagree", func(r *ReviewItem) { r.Routing.Signals = []RoutingSignal{SignalModelsAgree, SignalModelsDisagree} }, "both agree and disagree"},
		{"question without its aspect", func(r *ReviewItem) {
			r.Proposed.Consequence = nil
			r.ID = ReviewItemID(r.CandidateID, r.QuestionType, r.Proposed)
		}, "must propose the consequence aspect"},
		{"unknown question type", func(r *ReviewItem) {
			r.QuestionType = "vibe-check"
			r.ID = ReviewItemID(r.CandidateID, r.QuestionType, r.Proposed)
		}, "unknown question type"},
		{"empty question", func(r *ReviewItem) { r.Question = " " }, "question is required"},
		{"context without label", func(r *ReviewItem) { r.Context = &EnvironmentContext{Digest: "sha256:x"} }, "needs a label"},
		{"bad proposal ref", func(r *ReviewItem) { r.Proposals = []string{"rd-1"} }, "lacks the sp- prefix"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validItem(cand)
			tc.mut(&r)
			expectErr(t, r.Validate(), tc.want)
		})
	}
}

func TestReviewDecisionValidate(t *testing.T) {
	item := validItem(validCandidate())
	rehash := func(d *ReviewDecision) { d.ID = DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt) }
	corrected := func() *SemanticAssertion {
		a := rotationAssertion()
		a.Consequence = &Consequence{Kind: ConsequenceNone, ExposedClass: ImpactInformational}
		return &a
	}
	cases := []struct {
		name string
		mut  func(*ReviewDecision)
		want string
	}{
		{"accept", func(*ReviewDecision) {}, ""},
		{"accept with wrong label", func(d *ReviewDecision) { d.Labels = []FeedbackLabel{LabelAccepted, LabelWrongSubject} }, "exactly [accepted]"},
		{"correct keeps both values", func(d *ReviewDecision) {
			d.Action, d.Corrected, d.Reason = ActionCorrect, corrected(), "keys are reused by default for this issuer type"
			d.Labels = []FeedbackLabel{LabelCorrected, LabelWrongConsequence}
		}, ""},
		{"correct without wrong-* label", func(d *ReviewDecision) {
			d.Action, d.Corrected, d.Reason = ActionCorrect, corrected(), "x"
			d.Labels = []FeedbackLabel{LabelCorrected}
		}, "at least one wrong-*"},
		{"correct identical", func(d *ReviewDecision) {
			a := rotationAssertion()
			a.Statement = "only the wording differs"
			d.Action, d.Corrected, d.Reason = ActionCorrect, &a, "x"
			d.Labels = []FeedbackLabel{LabelCorrected, LabelWrongConsequence}
		}, "must change at least one aspect"},
		{"correct drops an aspect", func(d *ReviewDecision) {
			a := *corrected()
			a.Applicability = nil
			d.Action, d.Corrected, d.Reason = ActionCorrect, &a, "x"
			d.Labels = []FeedbackLabel{LabelCorrected, LabelWrongConsequence}
		}, "drops the applicability"},
		{"corrected on accept", func(d *ReviewDecision) { d.Corrected = corrected() }, "exactly for action correct"},
		{"accept without original", func(d *ReviewDecision) { d.Original = nil }, "requires the original"},
		{"reject needs reason", func(d *ReviewDecision) { d.Action, d.Labels = ActionReject, []FeedbackLabel{LabelRejected} }, "requires a reason"},
		{"reject as duplicate", func(d *ReviewDecision) {
			d.Action, d.Labels, d.Reason, d.DuplicateOf = ActionReject, []FeedbackLabel{LabelDuplicate}, "same as vf-1", "vf-1"
		}, ""},
		{"duplicate label without target", func(d *ReviewDecision) {
			d.Action, d.Labels, d.Reason = ActionReject, []FeedbackLabel{LabelDuplicate}, "dup"
		}, "duplicateOf is set exactly"},
		{"reject cannot produce a fact", func(d *ReviewDecision) {
			d.Action, d.Labels, d.Reason, d.ResultingFact = ActionReject, []FeedbackLabel{LabelRejected}, "wrong", "vf-1"
		}, "only accept/correct produce a fact"},
		{"need more evidence", func(d *ReviewDecision) {
			d.Action, d.Labels, d.Reason = ActionNeedMoreEvidence, []FeedbackLabel{LabelInsufficientEvidence}, "the note does not say which issuers"
		}, ""},
		{"defer with label", func(d *ReviewDecision) { d.Action = ActionDefer }, "no outcome label"},
		{"defer", func(d *ReviewDecision) {
			d.Action, d.Labels, d.Original, d.StartedAt = ActionDefer, nil, nil, time.Time{}
		}, ""},
		{"accept needs startedAt", func(d *ReviewDecision) { d.StartedAt = time.Time{} }, "startedAt is required"},
		{"decided before started", func(d *ReviewDecision) { d.StartedAt = d.DecidedAt.Add(time.Minute) }, "before startedAt"},
		{"human carrying AI provenance", func(d *ReviewDecision) {
			p := aiProvenance("claude-opus-5-5", upEvidence().ID)
			d.ProxyProvenance = &p
		}, "carries no AI provenance"},
		{"proxy without provenance", func(d *ReviewDecision) { d.ReviewerKind = ReviewerProxy }, "requires the proxy model's provenance"},
		{"proxy with provenance", func(d *ReviewDecision) {
			p := aiProvenance("claude-opus-5-5", upEvidence().ID)
			d.ReviewerKind, d.ProxyProvenance, d.Reviewer = ReviewerProxy, &p, "proxy:claude-opus-5-5"
			rehash(d)
		}, ""},
		{"proxy with deterministic provenance", func(d *ReviewDecision) {
			d.ReviewerKind = ReviewerProxy
			d.ProxyProvenance = &Provenance{Method: MethodComputed, Producer: "x@v1", Confidence: ConfidenceHigh}
		}, "must be method ai"},
		{"unknown reviewer kind", func(d *ReviewDecision) { d.ReviewerKind = "intern" }, "unknown reviewerKind"},
		{"id not derived", func(d *ReviewDecision) { d.Reviewer = "someone-else" }, "not derived"},
		{"bulk accept", func(d *ReviewDecision) {
			d.BatchID, d.BatchSize = BatchID([]string{d.ReviewItemID, "ri-other"}, d.Reviewer, d.DecidedAt), 2
		}, ""},
		{"batch of one", func(d *ReviewDecision) { d.BatchID, d.BatchSize = "rb-x", 1 }, "batchSize >= 2"},
		{"batch size without id", func(d *ReviewDecision) { d.BatchSize = 3 }, "batchSize >= 2"},
		{"batch id prefix", func(d *ReviewDecision) { d.BatchID, d.BatchSize = "batch-1", 2 }, "rb- prefix"},
		{"bulk correction", func(d *ReviewDecision) {
			d.Action, d.Corrected, d.Reason = ActionCorrect, corrected(), "x"
			d.Labels = []FeedbackLabel{LabelCorrected, LabelWrongConsequence}
			d.BatchID, d.BatchSize = "rb-x", 2
		}, "individual-only"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := validDecision(item)
			tc.mut(&d)
			expectErr(t, d.Validate(), tc.want)
		})
	}
	if got := validDecision(item).Duration(); got != 95*time.Second {
		t.Errorf("Duration = %v, want 95s", got)
	}
}

func TestVerifiedFactValidate(t *testing.T) {
	cand := validCandidate()
	v := validValidation(cand)
	d := validDecision(validItem(cand))
	cases := []struct {
		name string
		mut  func(*VerifiedFact)
		want string
	}{
		{"valid", func(*VerifiedFact) {}, ""},
		{"incomplete assertion", func(f *VerifiedFact) {
			f.Assertion.Consequence = nil
			f.ID = VerifiedFactID(f.Product, f.Release, f.Assertion)
		}, "consequence is required"},
		{"aspect without verification", func(f *VerifiedFact) { f.Verification = f.Verification[:3] }, "consequence has no verification"},
		{"verification without basis", func(f *VerifiedFact) { f.Verification[3].Basis = nil }, "no justifying"},
		{"deterministic resting on a decision", func(f *VerifiedFact) { f.Verification[0].Basis = []string{d.ID} }, "must rest on validation records"},
		{"human resting on a validation", func(f *VerifiedFact) { f.Verification[3].Basis = []string{v.ID} }, "must rest on decision records"},
		{"environment evidence", func(f *VerifiedFact) {
			f.Evidence = append(f.Evidence, NewEvidence(EvidenceLocalFile, "", "manifests/cert.yaml", "L1", "kind: Certificate", "sha256:m", time.Time{}))
		}, "release-level"},
		{"subject of another product", func(f *VerifiedFact) {
			f.Assertion.Subject.Product = "ingress-nginx"
			f.ID = VerifiedFactID(f.Product, f.Release, f.Assertion)
		}, "differs from fact product"},
		{"supersedes itself", func(f *VerifiedFact) { f.Supersedes = []string{f.ID} }, "not another fact id"},
		{"unknown status", func(f *VerifiedFact) { f.Status = "draft" }, "unknown status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := validFact(cand, v, d)
			tc.mut(&f)
			expectErr(t, f.Validate(), tc.want)
		})
	}
	f := validFact(cand, v, d)
	if f.Level() != VerifiedHuman {
		t.Errorf("Level = %q, want human (the weakest aspect)", f.Level())
	}
	f.Verification[3].Level = VerifiedProxy
	if f.Level() != VerifiedProxy {
		t.Errorf("Level = %q, want proxy", f.Level())
	}
}

func TestValidateFactBasis(t *testing.T) {
	cand := validCandidate()
	item := validItem(cand)
	type fixture struct {
		f     VerifiedFact
		vals  map[string]ValidationResult
		decs  map[string]ReviewDecision
		items map[string]ReviewItem
	}
	build := func() fixture {
		v := validValidation(cand)
		d := validDecision(item)
		return fixture{validFact(cand, v, d), map[string]ValidationResult{v.ID: v}, map[string]ReviewDecision{d.ID: d}, map[string]ReviewItem{item.ID: item}}
	}
	cases := []struct {
		name string
		mut  func(*fixture)
		want string
	}{
		{"valid", func(*fixture) {}, ""},
		{"proxy masquerading as human", func(x *fixture) {
			for id, d := range x.decs {
				p := aiProvenance("claude-opus-5-5", upEvidence().ID)
				d.ReviewerKind, d.ProxyProvenance = ReviewerProxy, &p
				x.decs[id] = d
			}
		}, "claims human verification but decision"},
		{"unresolved basis", func(x *fixture) { x.decs = map[string]ReviewDecision{} }, "does not resolve"},
		{"validation did not confirm", func(x *fixture) {
			for id, v := range x.vals {
				v.Checks = v.Checks[:1] // subject only
				x.vals[id] = v
			}
		}, "does not confirm it"},
		{"validation checked another value", func(x *fixture) {
			for id, v := range x.vals {
				v.Assertion.Change.After = str(`"Sometimes"`)
				x.vals[id] = v
			}
		}, "confirmed a different change"},
		{"decision answered another question", func(x *fixture) {
			it := x.items[item.ID]
			it.QuestionType = QuestionSemanticMapping
			x.items[item.ID] = it
		}, "does not verify consequence"},
		{"decision settled on another consequence", func(x *fixture) {
			for id, d := range x.decs {
				a := rotationAssertion()
				a.Consequence = &Consequence{Kind: ConsequenceNone, ExposedClass: ImpactInformational}
				d.Action, d.Corrected, d.Reason = ActionCorrect, &a, "x"
				d.Labels = []FeedbackLabel{LabelCorrected, LabelWrongConsequence}
				x.decs[id] = d
			}
		}, "settled on a different consequence"},
		{"rejected decision as basis", func(x *fixture) {
			for id, d := range x.decs {
				d.Action = ActionReject
				x.decs[id] = d
			}
		}, "not accept/correct"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x := build()
			tc.mut(&x)
			expectErr(t, ValidateFactBasis(x.f, x.vals, x.decs, x.items), tc.want)
		})
	}
}

func TestKnowledgeRecordRoundTrip(t *testing.T) {
	cand := validCandidate()
	v := validValidation(cand)
	item := validItem(cand)
	d := validDecision(item)
	entities := []any{cand, validProposal(cand), &v, item, d, validFact(cand, v, d)}
	for _, e := range entities {
		r, err := NewRecord(e)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Validate(); err != nil {
			t.Fatalf("%s: %v", r.Kind, err)
		}
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var back KnowledgeRecord
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		if err := back.Validate(); err != nil {
			t.Fatalf("%s after round trip: %v", r.Kind, err)
		}
		if back.ID() != r.ID() {
			t.Fatalf("%s: id changed in round trip", r.Kind)
		}
	}
	if _, err := NewRecord(42); err == nil {
		t.Error("NewRecord accepted a non-entity")
	}
	r, _ := NewRecord(cand)
	r.Fact = &VerifiedFact{}
	expectErr(t, r.Validate(), "exactly its own payload")
	r, _ = NewRecord(cand)
	r.SchemaVersion = "v0"
	expectErr(t, r.Validate(), "schemaVersion")
}

// --- the trust ladder on ImpactFinding (DESIGN.md §4) ---------------------------

func TestImpactFindingKnowledgeRules(t *testing.T) {
	knowledgeFinding := func(r *ImpactReport) *ImpactFinding {
		f := &r.Findings[0]
		f.Rule = "impact:knowledge-exposed"
		f.Provenance.Rule = f.Rule
		f.Knowledge = &KnowledgeRef{Fact: "vf-1", Verification: VerifiedHuman}
		return f
	}
	unknownOf := func(r *ImpactReport, changeID string) ImpactFinding {
		up := r.Evidence[0].ID
		r.Summary.Unknown++
		return ImpactFinding{ID: "imp-u", Classification: ImpactUnknown, Rule: "impact:not-joined", Title: "not evaluated",
			ChangeID: changeID, ChangeTitle: "t", UpstreamEvidence: []EvidenceID{up}, NeededToDetermine: []string{"x"},
			UnknownReason: UnknownReleaseKnowledgeGap,
			Provenance:    Provenance{Method: MethodComputed, Producer: "impact@v1", Confidence: ConfidenceHigh}}
	}
	cases := []struct {
		name string
		mut  func(*ImpactReport)
		want string
	}{
		{"deterministic finding untouched", func(*ImpactReport) {}, ""},
		{"action from a human-verified fact", func(r *ImpactReport) { knowledgeFinding(r) }, ""},
		{"action from a deterministic fact", func(r *ImpactReport) { knowledgeFinding(r).Knowledge.Verification = VerifiedDeterministic }, ""},
		{"action from a proxy fact", func(r *ImpactReport) { knowledgeFinding(r).Knowledge.Verification = VerifiedProxy }, "requires a deterministic- or human-verified fact"},
		{"proxy review at medium", func(r *ImpactReport) {
			f := knowledgeFinding(r)
			f.Knowledge.Verification = VerifiedProxy
			f.Classification, f.Provenance.Confidence = ImpactReviewRequired, ConfidenceMedium
			r.Summary.ActionRequired, r.Summary.ReviewRequired = 0, 1
		}, ""},
		{"proxy at high confidence", func(r *ImpactReport) {
			f := knowledgeFinding(r)
			f.Knowledge.Verification = VerifiedProxy
			f.Classification = ImpactReviewRequired
			r.Summary.ActionRequired, r.Summary.ReviewRequired = 0, 1
		}, "cannot carry high confidence"},
		{"proxy never clears", func(r *ImpactReport) {
			f := knowledgeFinding(r)
			f.Knowledge.Verification = VerifiedProxy
			f.Rule, f.Classification, f.Provenance.Confidence = "impact:knowledge-clear", ImpactNotAffected, ConfidenceMedium
			f.Matches, f.EnvironmentEvidence = nil, nil
			f.Checks = []ImpactCheck{{Dimension: DimensionManifests, Facts: 3, Subjects: []string{"spec.privateKey.rotationPolicy"}}}
			r.Summary.ActionRequired, r.Summary.AffectEnvironment, r.Summary.NotAffected = 0, 0, 1
		}, "consensus and proxy never clear"},
		{"action from a consensus fact without consensusAction", func(r *ImpactReport) {
			k := knowledgeFinding(r).Knowledge
			k.Verification, k.Consensus = VerifiedConsensus, ConsensusSameModel
		}, "or a consensus-action fact"},
		{"ACTION REQUIRED · model consensus (PO-2)", func(r *ImpactReport) {
			k := knowledgeFinding(r).Knowledge
			k.Verification, k.Consensus, k.ConsensusAction = VerifiedConsensus, ConsensusSameModel, true
			if k.ActionLabel() != "model consensus" {
				t.Errorf("label = %q", k.ActionLabel())
			}
		}, ""},
		{"consensus label missing", func(r *ImpactReport) {
			k := knowledgeFinding(r).Knowledge
			k.Verification, k.ConsensusAction = VerifiedConsensus, true
		}, "set exactly for consensus-verified facts"},
		{"consensusAction on a human fact", func(r *ImpactReport) { knowledgeFinding(r).Knowledge.ConsensusAction = true }, "consensus-verified facts only"},
		{"proxy may still not act", func(r *ImpactReport) {
			k := knowledgeFinding(r).Knowledge
			k.Verification = VerifiedProxy
		}, "or a consensus-action fact"},
		{"consensus review at medium", func(r *ImpactReport) {
			f := knowledgeFinding(r)
			f.Knowledge.Verification, f.Knowledge.Consensus = VerifiedConsensus, ConsensusCrossModel
			f.Classification, f.Provenance.Confidence = ImpactReviewRequired, ConfidenceMedium
			r.Summary.ActionRequired, r.Summary.ReviewRequired = 0, 1
		}, ""},
		{"consensus review at high confidence", func(r *ImpactReport) {
			f := knowledgeFinding(r)
			f.Knowledge.Verification, f.Knowledge.Consensus, f.Knowledge.ConsensusAction = VerifiedConsensus, ConsensusCrossModel, true
			f.Classification = ImpactReviewRequired
			r.Summary.ActionRequired, r.Summary.ReviewRequired = 0, 1
		}, "cannot carry high confidence"},
		{"consensus never clears", func(r *ImpactReport) {
			f := knowledgeFinding(r)
			f.Knowledge.Verification, f.Knowledge.Consensus = VerifiedConsensus, ConsensusCrossModel
			f.Rule, f.Classification, f.Provenance.Confidence = "impact:knowledge-clear", ImpactNotAffected, ConfidenceMedium
			f.Matches, f.EnvironmentEvidence = nil, nil
			f.Checks = []ImpactCheck{{Dimension: DimensionManifests, Facts: 3, Subjects: []string{"x"}}}
			r.Summary.ActionRequired, r.Summary.AffectEnvironment, r.Summary.NotAffected = 0, 0, 1
		}, "consensus and proxy never clear"},
		{"knowledge ref on a join rule", func(r *ImpactReport) {
			r.Findings[0].Knowledge = &KnowledgeRef{Fact: "vf-1", Verification: VerifiedHuman}
		}, "carried exactly by impact:knowledge-"},
		{"knowledge rule without ref", func(r *ImpactReport) { r.Findings[0].Rule = "impact:knowledge-exposed" }, "carried exactly by impact:knowledge-"},
		{"bad fact id", func(r *ImpactReport) { knowledgeFinding(r).Knowledge.Fact = "chg-1" }, "vf- prefix"},
		{"unknown reason on an affected finding", func(r *ImpactReport) { r.Findings[0].UnknownReason = UnknownEvidenceGap }, "unknown-only"},
		{"invalid unknown reason", func(r *ImpactReport) {
			u := unknownOf(r, "chg-other")
			u.UnknownReason = "shrug"
			r.Findings = append(r.Findings, u)
		}, "unknown unknownReason"},
		{"unknown with reason", func(r *ImpactReport) { r.Findings = append(r.Findings, unknownOf(r, "chg-other")) }, ""},
		{"knowledge supersedes the unknown record", func(r *ImpactReport) {
			f := knowledgeFinding(r)
			r.Findings = append(r.Findings, unknownOf(r, f.ChangeID))
		}, "supersedes this unknown record"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validReport()
			tc.mut(r)
			expectErr(t, r.Validate(), tc.want)
		})
	}
}

// --- consensus facts, auto-approval, render relations (contract-2) -----------------

// consensusFact: subject/change/applicability confirmed by a render
// validation, consequence agreed by GLM and Claude; no human involved.
func consensusFact() (VerifiedFact, FactRecords) {
	cand := validCandidate()
	v := validValidation(cand)
	glm := validProposal(cand)
	claude := validProposal(cand)
	claude.Provider, claude.Provenance = "anthropic", aiProvenance("claude-sonnet-5-5", upEvidence().ID, crdEvidence().ID)
	claude.ID = ProposalID(claude.CandidateID, claude.Task, claude.Provider, claude.Provenance)
	f := validFact(cand, v, validDecision(validItem(cand)))
	f.Verification[3] = AspectVerification{Aspect: AspectConsequence, Level: VerifiedConsensus, Basis: []string{glm.ID, claude.ID}, Consensus: ConsensusCrossModel}
	f.AutoApproved = true
	return f, FactRecords{
		Validations: map[string]ValidationResult{v.ID: v},
		Proposals:   map[string]SemanticProposal{glm.ID: glm, claude.ID: claude},
	}
}

func TestConsensusFact(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*VerifiedFact, *FactRecords)
		want string
	}{
		{"glm + claude agree, render confirmed the rest", func(*VerifiedFact, *FactRecords) {}, ""},
		{"auto-approved not marked", func(f *VerifiedFact, _ *FactRecords) { f.AutoApproved = false }, "autoApproved must be set"},
		{"single proposal", func(f *VerifiedFact, _ *FactRecords) { f.Verification[3].Basis = f.Verification[3].Basis[:1] }, "at least two agreeing proposals"},
		{"decision as consensus basis", func(f *VerifiedFact, _ *FactRecords) {
			f.Verification[3].Basis = append(f.Verification[3].Basis, "rd-x")
		}, "consensus verification rests on agreeing proposals"},
		{"two Opus calls are consensus (PO-1), labelled same-model", func(f *VerifiedFact, r *FactRecords) {
			for id, p := range r.Proposals {
				p.Provenance.Model = "claude-opus-5-5"
				r.Proposals[id] = p
			}
			f.Verification[3].Consensus = ConsensusSameModel
		}, ""},
		{"wrong scope label", func(f *VerifiedFact, r *FactRecords) {
			for id, p := range r.Proposals {
				p.Provenance.Model = "claude-opus-5-5"
				r.Proposals[id] = p
			}
		}, "labelled \"cross-model\" but the agreeing calls are same-model"},
		{"one call counted twice", func(f *VerifiedFact, r *FactRecords) {
			for id, p := range r.Proposals {
				p.Provenance.CallID = "msg_same"
				r.Proposals[id] = p
			}
		}, "counted twice"},
		{"label on a deterministic aspect", func(f *VerifiedFact, _ *FactRecords) { f.Verification[0].Consensus = ConsensusCrossModel }, "consensus verifications only"},
		{"consensus without label", func(f *VerifiedFact, _ *FactRecords) { f.Verification[3].Consensus = "" }, "labelled cross-model or same-model"},
		{"a proposal disagrees", func(f *VerifiedFact, r *FactRecords) {
			id := f.Verification[3].Basis[1]
			p := r.Proposals[id]
			a := rotationAssertion()
			a.Consequence = &Consequence{Kind: ConsequenceNone, ExposedClass: ImpactInformational}
			p.Assertion = a
			r.Proposals[id] = p
		}, "asserted a different consequence"},
		{"proposal for another candidate", func(f *VerifiedFact, r *FactRecords) {
			id := f.Verification[3].Basis[0]
			p := r.Proposals[id]
			p.CandidateID = "sc-other"
			r.Proposals[id] = p
		}, "not one of the fact's"},
		{"unresolved proposal", func(f *VerifiedFact, r *FactRecords) { r.Proposals = nil }, "does not resolve"},
		{"proposal backing a deterministic aspect", func(f *VerifiedFact, r *FactRecords) {
			f.Verification[0].Basis = append(f.Verification[0].Basis, f.Verification[3].Basis[0])
		}, "must rest on validation records"},
		{"refuting validation in a consensus basis", func(f *VerifiedFact, r *FactRecords) {
			cand := validCandidate()
			v := ValidationResult{CandidateID: cand.ID, Validator: "render.diff@v1", Assertion: rotationAssertion(), CheckedAt: t0,
				Checks: []AspectCheck{{Aspect: AspectConsequence, Outcome: OutcomeRefuted, Rule: "x"}}}
			v.ID = ValidationID(v.CandidateID, v.Validator, v.Assertion)
			r.Validations[v.ID] = v
			f.Verification[3].Basis = append(f.Verification[3].Basis, v.ID)
		}, "refutes it"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, r := consensusFact()
			tc.mut(&f, &r)
			err := f.Validate()
			if err == nil {
				err = ValidateFactRecords(f, r)
			}
			expectErr(t, err, tc.want)
		})
	}
	f, r := consensusFact()
	if f.Level() != VerifiedConsensus {
		t.Errorf("Level = %s, want consensus", f.Level())
	}
	if err := ValidateFactBasis(f, r.Validations, nil, nil); err == nil {
		t.Error("ValidateFactBasis resolves no proposals, so a consensus fact must fail it")
	}
	// a fully deterministic fact is auto-approved too
	cand := validCandidate()
	v := validValidation(cand)
	v.Checks = append(v.Checks, AspectCheck{Aspect: AspectConsequence, Outcome: OutcomeInconclusive, Rule: "x"})
	d := validFact(cand, v, validDecision(validItem(cand)))
	d.Verification[3] = AspectVerification{Aspect: AspectConsequence, Level: VerifiedDeterministic, Basis: []string{v.ID}}
	expectErr(t, d.Validate(), "autoApproved must be set")
	d.AutoApproved = true
	expectErr(t, d.Validate(), "")
	// history: the marker may stay after a human audit upgraded an aspect
	d.Verification[3] = validFact(cand, v, validDecision(validItem(cand))).Verification[3]
	expectErr(t, d.Validate(), "")
}

func TestRenderRelationAndRenderability(t *testing.T) {
	cand := validCandidate()
	rendered := NewEvidence(EvidenceStructured, "chart", "templates/certificate-crd.yaml", "spec.versions[0]", "rotationPolicy", "sha256:r", t0)
	rendered.Render = &RenderProvenance{Scope: RenderRelease, Tool: "helm", ToolVersion: "v3.16.4", ChartDigest: "sha256:chart"}
	inconclusive := func(v *ValidationResult) {
		v.Checks = []AspectCheck{{Aspect: AspectChange, Outcome: OutcomeInconclusive, Rule: "render:not-visible"}}
		v.Evidence = nil
	}
	cases := []struct {
		name string
		mut  func(*ValidationResult)
		want string
	}{
		{"confirmed by render", func(v *ValidationResult) { v.RenderRelation, v.Evidence = RenderConfirmed, []Evidence{rendered} }, ""},
		{"confirmed without rendered evidence", func(v *ValidationResult) { v.RenderRelation = RenderConfirmed }, "must cite rendered evidence"},
		{"contradicted without refutation", func(v *ValidationResult) { v.RenderRelation = RenderContradicted }, "needs a refuted check"},
		{"contradicted", func(v *ValidationResult) {
			v.RenderRelation = RenderContradicted
			v.Checks = []AspectCheck{{Aspect: AspectChange, Outcome: OutcomeRefuted, Rule: "render:default"}}
			v.Evidence = []Evidence{rendered}
		}, ""},
		{"not visible concludes nothing", func(v *ValidationResult) { inconclusive(v); v.RenderRelation = RenderNotVisible }, ""},
		{"not visible but confirming", func(v *ValidationResult) { v.RenderRelation = RenderNotVisible }, "must all be inconclusive"},
		{"render not applicable", func(v *ValidationResult) { inconclusive(v); v.RenderRelation = RenderNotApplicable }, ""},
		{"unknown relation", func(v *ValidationResult) { v.RenderRelation = "maybe" }, "unknown renderRelation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := validValidation(cand)
			tc.mut(&v)
			expectErr(t, v.Validate(), tc.want)
		})
	}
	c := validCandidate()
	c.Renderability = RenderPartiallyVerifiable
	expectErr(t, c.Validate(), "")
	c.Renderability = "sometimes"
	expectErr(t, c.Validate(), "unknown renderability")
}

// --- PO-2: consensus may produce ACTION REQUIRED -----------------------------------

// consensusActionFact: subject/change/applicability deterministic, consequence
// setting-ignored agreed by two separate calls that both requested action.
func consensusActionFact() (VerifiedFact, FactRecords) {
	f, r := consensusFact()
	cons := &Consequence{Kind: ConsequenceSettingIgnored, ExposedClass: ImpactActionRequired, Statement: "the pinned key stops taking effect"}
	f.Assertion.Consequence = cons
	f.ID = VerifiedFactID(f.Product, f.Release, f.Assertion)
	props := map[string]SemanticProposal{}
	var ids []string
	for _, p := range r.Proposals {
		p.Assertion.Consequence = cons
		p.SuggestedClass = ImpactActionRequired
		props[p.ID] = p
		ids = append(ids, p.ID)
	}
	r.Proposals = props
	f.Verification[3].Basis = ids
	f.ConsensusAction = true
	return f, r
}

func TestConsensusActionFact(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*VerifiedFact, *FactRecords)
		want string
	}{
		{"both calls requested action, nothing refuted", func(*VerifiedFact, *FactRecords) {}, ""},
		{"one call only suggested review", func(f *VerifiedFact, r *FactRecords) {
			id := f.Verification[3].Basis[0]
			p := r.Proposals[id]
			p.SuggestedClass = ImpactReviewRequired
			r.Proposals[id] = p
		}, "did not request action-required"},
		{"behavior change cannot act", func(f *VerifiedFact, r *FactRecords) {
			cons := &Consequence{Kind: ConsequenceBehaviorChange, ExposedClass: ImpactReviewRequired}
			f.Assertion.Consequence = cons
			f.ID = VerifiedFactID(f.Product, f.Release, f.Assertion)
		}, "action-eligible consequence"},
		{"a proxy aspect blocks consensus action", func(f *VerifiedFact, _ *FactRecords) {
			f.Verification[0] = AspectVerification{Aspect: AspectSubject, Level: VerifiedProxy, Basis: []string{"rd-x"}}
			f.AutoApproved = false
		}, "no proxy"},
		{"all trusted: not a consensus path", func(f *VerifiedFact, _ *FactRecords) {
			f.Verification[3] = AspectVerification{Aspect: AspectConsequence, Level: VerifiedHuman, Basis: []string{"rd-x"}}
			f.AutoApproved = false
		}, "at least one at consensus"},
		{"a validator refuted an aspect", func(f *VerifiedFact, r *FactRecords) {
			cand := validCandidate()
			v := ValidationResult{CandidateID: cand.ID, Validator: "render.diff@v1", Assertion: f.Assertion, CheckedAt: t0,
				Checks: []AspectCheck{{Aspect: AspectApplicability, Outcome: OutcomeRefuted, Rule: "render:not-set"}}}
			v.ID = ValidationID(v.CandidateID, v.Validator, v.Assertion)
			r.Validations[v.ID] = v
		}, "refutes the applicability aspect"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, r := consensusActionFact()
			tc.mut(&f, &r)
			err := f.Validate()
			if err == nil {
				err = ValidateFactRecords(f, r)
			}
			expectErr(t, err, tc.want)
		})
	}
}

// --- PO-3: unset changed defaults and new keys are decided by rendering ------------

func renderEvidence(scope RenderScope) Evidence {
	e := NewEvidence(EvidenceLocalFile, "render", "render://to/apps/v1/Deployment/kyverno/kyverno-cleanup", "spec.template.spec.containers[0].image",
		"image: bitnami/kubectl:1.30.2", "sha256:render", time.Time{})
	e.Render = &RenderProvenance{Scope: scope, Tool: "helm", ToolVersion: "v3.16.4", ChartDigest: "sha256:chart", ValuesDigest: "sha256:values"}
	return e
}

func TestValuesDefaultRenderVerdicts(t *testing.T) {
	// valuesDefault turns the report's single finding into a PO-3 verdict.
	valuesDefault := func(r *ImpactReport, rule string, class ImpactClass, outcome RenderOutcome, scope RenderScope) *ImpactFinding {
		ev := renderEvidence(scope)
		r.EnvironmentEvidence = append(r.EnvironmentEvidence, ev)
		f := &r.Findings[0]
		f.Rule, f.Provenance.Rule, f.Classification = rule, rule, class
		r.Summary = ImpactSummary{UpstreamChanges: 3}
		switch class {
		case ImpactReviewRequired:
			f.Provenance.Confidence = ConfidenceMedium
			f.Matches = []ImpactMatch{{Kind: MatchRenderedChange, Subject: "Deployment/kyverno/kyverno-cleanup: image changed", Evidence: []EvidenceID{ev.ID}}}
			f.EnvironmentEvidence = []EvidenceID{ev.ID}
			r.Summary.AffectEnvironment, r.Summary.ReviewRequired = 1, 1
		case ImpactActionRequired:
			f.Matches = []ImpactMatch{{Kind: MatchRenderedChange, Subject: "x", Evidence: []EvidenceID{ev.ID}}}
			f.EnvironmentEvidence = []EvidenceID{ev.ID}
			r.Summary.AffectEnvironment, r.Summary.ActionRequired = 1, 1
		case ImpactNotAffected:
			f.Matches, f.EnvironmentEvidence = nil, nil
			chk := ImpactCheck{Dimension: DimensionRender, Facts: 2, Subjects: []string{"policyReportsCleanup.image.tag"},
				Render: &RenderCheck{Outcome: outcome, Key: "policyReportsCleanup.image.tag", Counterfactual: true}}
			if outcome == RenderUnavailable {
				chk.Render.Reason, chk.Render.Counterfactual, chk.Facts = "helm not installed", false, 0
			} else {
				chk.Evidence = []EvidenceID{ev.ID}
			}
			f.Checks = []ImpactCheck{chk}
			r.Summary.NotAffected = 1
		}
		return f
	}
	cases := []struct {
		name string
		mut  func(*ImpactReport)
		want string
	}{
		{"applies: review with a rendered change", func(r *ImpactReport) {
			valuesDefault(r, RuleValuesDefaultApplies, ImpactReviewRequired, "", RenderEnvironment)
		}, ""},
		{"applies cannot be action on its own", func(r *ImpactReport) {
			valuesDefault(r, RuleValuesDefaultApplies, ImpactActionRequired, "", RenderEnvironment)
		}, "is review-required"},
		{"applies backed by a chart-default render", func(r *ImpactReport) {
			valuesDefault(r, RuleValuesDefaultApplies, ImpactReviewRequired, "", RenderRelease)
		}, "backed by environment-render evidence"},
		{"no effect: both renders, nothing attributable", func(r *ImpactReport) {
			valuesDefault(r, RuleValuesDefaultNoEffect, ImpactNotAffected, RenderNoAttributableChange, RenderEnvironment)
		}, ""},
		{"no effect claimed without a render", func(r *ImpactReport) {
			valuesDefault(r, RuleValuesDefaultNoEffect, ImpactNotAffected, RenderUnavailable, RenderEnvironment)
		}, "no-attributable-change render check"},
		{"no effect without render evidence", func(r *ImpactReport) {
			f := valuesDefault(r, RuleValuesDefaultNoEffect, ImpactNotAffected, RenderNoAttributableChange, RenderEnvironment)
			f.Checks[0].Evidence = nil
		}, "must cite environment-render evidence"},
		{"unrendered: not-affected, visibly unavailable (PO choice)", func(r *ImpactReport) {
			valuesDefault(r, RuleValuesDefaultUnrendered, ImpactNotAffected, RenderUnavailable, RenderEnvironment)
		}, ""},
		{"unrendered without a reason", func(r *ImpactReport) {
			f := valuesDefault(r, RuleValuesDefaultUnrendered, ImpactNotAffected, RenderUnavailable, RenderEnvironment)
			f.Checks[0].Render.Reason = ""
		}, "must say why"},
		{"unrendered hiding the missing render", func(r *ImpactReport) {
			f := valuesDefault(r, RuleValuesDefaultUnrendered, ImpactNotAffected, RenderUnavailable, RenderEnvironment)
			f.Checks[0].Dimension, f.Checks[0].Render = DimensionValues, nil
		}, "visible unavailable render check"},
		{"render record on a values check", func(r *ImpactReport) {
			f := valuesDefault(r, RuleValuesDefaultUnrendered, ImpactNotAffected, RenderUnavailable, RenderEnvironment)
			f.Checks[0].Dimension = DimensionValues
		}, "carried exactly by render-dimension checks"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validReport()
			tc.mut(r)
			expectErr(t, r.Validate(), tc.want)
		})
	}
}

// --- PO-4: narrow refinement of deterministic findings by trusted facts -------------

func TestSupersededUpstream(t *testing.T) {
	k := ConsequenceSupersededUpstream
	if k.ActionEligible() || k.ExposedClass() != ImpactReviewRequired {
		t.Fatalf("superseded-upstream: eligible=%v class=%s, want review-required and not action-eligible", k.ActionEligible(), k.ExposedClass())
	}
	expectErr(t, Consequence{Kind: k, ExposedClass: ImpactActionRequired, Statement: "x"}.Validate(), "must be review-required")
}

func TestSubjectCoversMatch(t *testing.T) {
	hv := Subject{Family: SubjectHelmValue, Product: "kyverno", Path: "cleanupJobs"}
	img := Subject{Family: SubjectImage, Product: "kyverno", Name: "bitnami/kubectl"}
	crd := Subject{Family: SubjectCRDField, Product: "p", Group: "g", Kind: "K", Path: "spec.resources"}
	cases := []struct {
		s    Subject
		m    ImpactMatch
		want bool
	}{
		{hv, ImpactMatch{Kind: MatchValuesKey, Subject: "cleanupJobs"}, true},
		{hv, ImpactMatch{Kind: MatchValuesKey, Subject: "cleanupJobs.admissionReports.threshold"}, true},
		{hv, ImpactMatch{Kind: MatchValuesKey, Subject: "cleanupJobsExtra.x"}, false},
		{hv, ImpactMatch{Kind: MatchManifestField, Subject: "cleanupJobs"}, false},
		{img, ImpactMatch{Kind: MatchImage, Subject: "bitnami/kubectl:1.28.5"}, true},
		{img, ImpactMatch{Kind: MatchImage, Subject: "registry.corp.example/bitnami/kubectl:1.28.5"}, false},
		{crd, ImpactMatch{Kind: MatchManifestField, Subject: "spec.resources[]"}, true},
		{Subject{Family: SubjectFeatureGate, Product: "p", Name: "X"}, ImpactMatch{Kind: MatchValuesKey, Subject: "X"}, false},
	}
	for _, c := range cases {
		if got := c.s.CoversMatch(c.m); got != c.want {
			t.Errorf("%s covers %s %q = %v, want %v", c.s.Key(), c.m.Kind, c.m.Subject, got, c.want)
		}
	}
}

func TestRefinementRules(t *testing.T) {
	// refine turns the report's values-removed ACTION (match replicaCount)
	// into a refined review-required finding from a human-verified fact.
	refine := func(r *ImpactReport) *ImpactFinding {
		f := &r.Findings[0]
		f.RefinedFrom = &Refinement{Classification: f.Classification, Rule: f.Rule, Severity: f.Severity}
		f.Rule, f.Provenance.Rule = RuleKnowledgeRefined, RuleKnowledgeRefined
		f.Classification = ImpactReviewRequired
		f.Knowledge = &KnowledgeRef{Fact: "vf-1", Verification: VerifiedHuman,
			Subject: &Subject{Family: SubjectHelmValue, Product: "p", Path: "replicaCount"}}
		r.Summary.ActionRequired, r.Summary.ReviewRequired = 0, 1
		return f
	}
	cases := []struct {
		name string
		mut  func(*ImpactReport)
		want string
	}{
		{"human fact downgrades ACTION to review (kyverno E9 shape)", func(r *ImpactReport) { refine(r) }, ""},
		{"deterministic fact may refine", func(r *ImpactReport) { refine(r).Knowledge.Verification = VerifiedDeterministic }, ""},
		{"to informational", func(r *ImpactReport) {
			refine(r).Classification = ImpactInformational
			r.Summary.ReviewRequired, r.Summary.Informational = 0, 1
		}, ""},
		{"proxy fact tries to refine", func(r *ImpactReport) {
			f := refine(r)
			f.Knowledge.Verification, f.Provenance.Confidence = VerifiedProxy, ConfidenceMedium
		}, "only a trusted"},
		{"consensus-action fact tries to refine", func(r *ImpactReport) {
			f := refine(r)
			f.Knowledge.Verification, f.Knowledge.Consensus, f.Knowledge.ConsensusAction = VerifiedConsensus, ConsensusCrossModel, true
			f.Provenance.Confidence = ConfidenceMedium
		}, "only a trusted"},
		{"refine to not-affected", func(r *ImpactReport) {
			f := refine(r)
			f.Classification, f.Matches, f.EnvironmentEvidence = ImpactNotAffected, nil, nil
			f.Checks = []ImpactCheck{{Dimension: DimensionValues, Facts: 1, Subjects: []string{"replicaCount"}}}
			r.Summary.ReviewRequired, r.Summary.AffectEnvironment, r.Summary.NotAffected = 0, 0, 1
		}, "never yields \"not-affected\""},
		{"refine to unknown", func(r *ImpactReport) {
			f := refine(r)
			f.Classification, f.Matches, f.EnvironmentEvidence = ImpactUnknown, nil, nil
			f.NeededToDetermine = []string{"x"}
			r.Summary.ReviewRequired, r.Summary.AffectEnvironment, r.Summary.Unknown = 0, 0, 1
		}, "never yields \"unknown\""},
		{"subject mismatch (sibling key)", func(r *ImpactReport) {
			refine(r).Knowledge.Subject.Path = "replica"
		}, "lies outside the refining fact's subject"},
		{"subject of another family", func(r *ImpactReport) {
			refine(r).Knowledge.Subject = &Subject{Family: SubjectFeatureGate, Product: "p", Name: "replicaCount"}
		}, "lies outside the refining fact's subject"},
		{"refining fact without subject", func(r *ImpactReport) { refine(r).Knowledge.Subject = nil }, "states its subject"},
		{"original finding kept next to the refinement", func(r *ImpactReport) {
			orig := r.Findings[0]
			refine(r)
			orig.ID = "imp-orig"
			r.Findings = append(r.Findings, orig)
			r.Summary.ActionRequired, r.Summary.AffectEnvironment = 1, 2
		}, "must be replaced, not kept"},
		{"no class change", func(r *ImpactReport) {
			f := refine(r)
			f.Classification = ImpactActionRequired
			r.Summary.ActionRequired, r.Summary.ReviewRequired = 1, 0
		}, "changes the class"},
		{"refining an unknown record", func(r *ImpactReport) { refine(r).RefinedFrom.Classification = ImpactUnknown }, "only an affected deterministic finding"},
		{"refining a knowledge finding", func(r *ImpactReport) { refine(r).RefinedFrom.Rule = "impact:knowledge-exposed" }, "must name the deterministic join rule"},
		{"refinedFrom on another rule", func(r *ImpactReport) {
			f := refine(r)
			f.Rule = "impact:knowledge-exposed"
		}, "carried exactly by rule impact:knowledge-refined"},
		{"refined rule without refinedFrom", func(r *ImpactReport) { refine(r).RefinedFrom = nil }, "carried exactly by rule impact:knowledge-refined"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validReport()
			tc.mut(r)
			expectErr(t, r.Validate(), tc.want)
		})
	}
}

// --- contract-5: prose-only corrections and Provenance.Provider -----------------------

func TestProseOnlyCorrection(t *testing.T) {
	item := validItem(validCandidate())
	better := func() *SemanticAssertion {
		a := rotationAssertion()
		a.Consequence.Statement = "private keys are regenerated on every renewal; consumers pinning the public key break"
		a.Consequence.Remediation = "set rotationPolicy: Never explicitly where keys are pinned"
		return &a
	}
	correct := func(d *ReviewDecision, c *SemanticAssertion, labels ...FeedbackLabel) {
		d.Action, d.Corrected, d.Reason, d.Labels = ActionCorrect, c, "clearer statement", labels
	}
	cases := []struct {
		name string
		mut  func(*ReviewDecision)
		want string
	}{
		{"statement/remediation improved", func(d *ReviewDecision) { correct(d, better(), LabelCorrected, LabelImprovedStatement) }, ""},
		{"prose-only labelled wrong-consequence", func(d *ReviewDecision) { correct(d, better(), LabelCorrected, LabelWrongConsequence) }, "exactly [corrected, improved-statement]"},
		{"prose-only labelled corrected only", func(d *ReviewDecision) { correct(d, better(), LabelCorrected) }, "exactly [corrected, improved-statement]"},
		{"only the assertion's own statement changed", func(d *ReviewDecision) {
			a := rotationAssertion()
			a.Statement = "reworded"
			correct(d, &a, LabelCorrected, LabelImprovedStatement)
		}, "or the consequence statement/remediation"},
		{"typed change plus better prose", func(d *ReviewDecision) {
			c := better()
			c.Consequence.Kind, c.Consequence.ExposedClass = ConsequenceWorkloadFailure, ImpactActionRequired
			correct(d, c, LabelCorrected, LabelWrongConsequence, LabelImprovedStatement)
		}, ""},
		{"improved-statement on a typed change without prose change", func(d *ReviewDecision) {
			c := rotationAssertion()
			c.Consequence = &Consequence{Kind: ConsequenceNone, ExposedClass: ImpactInformational, Statement: c.Consequence.Statement}
			c.Consequence.Statement = rotationAssertion().Consequence.Statement
			correct(d, &c, LabelCorrected, LabelWrongConsequence, LabelImprovedStatement)
		}, "requires the consequence statement/remediation to change"},
		{"improved-statement on accept", func(d *ReviewDecision) { d.Labels = []FeedbackLabel{LabelAccepted, LabelImprovedStatement} }, "exactly [accepted]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := validDecision(item)
			tc.mut(&d)
			expectErr(t, d.Validate(), tc.want)
		})
	}
	// a prose-only correction verifies the consequence aspect of a fact
	// that carries the improved prose (same digests, same fact id)
	cand := validCandidate()
	v := validValidation(cand)
	d := validDecision(item)
	correct(&d, better(), LabelCorrected, LabelImprovedStatement)
	f := validFact(cand, v, d)
	f.Assertion = *better()
	f.ID = VerifiedFactID(f.Product, f.Release, f.Assertion)
	if f.ID != VerifiedFactID(f.Product, f.Release, rotationAssertion()) {
		t.Fatal("prose must not change the fact id")
	}
	expectErr(t, f.Validate(), "")
	expectErr(t, ValidateFactBasis(f, map[string]ValidationResult{v.ID: v}, map[string]ReviewDecision{d.ID: d}, map[string]ReviewItem{item.ID: item}), "")
}

func TestProvenanceProvider(t *testing.T) {
	ai := aiProvenance("claude-opus-5-5", upEvidence().ID)
	if ai.ProviderName() != "" {
		t.Fatalf("no provider stated, got %q", ai.ProviderName())
	}
	legacy := ai
	legacy.Rule = "provider:anthropic"
	if legacy.ProviderName() != "anthropic" {
		t.Fatalf("legacy rule provider = %q", legacy.ProviderName())
	}
	explicit := legacy
	explicit.Provider = "zai"
	if explicit.ProviderName() != "zai" {
		t.Fatal("the explicit field wins over the legacy rule")
	}
	expectErr(t, explicit.Validate(), "")
	det := Provenance{Method: MethodComputed, Producer: "impact@v1", Confidence: ConfidenceHigh, Provider: "anthropic"}
	expectErr(t, det.Validate(), "must not carry model/prompt fields")

	cand := validCandidate()
	rehash := func(p *SemanticProposal) { p.ID = ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance) }
	for _, tc := range []struct {
		name string
		mut  func(*SemanticProposal)
		want string
	}{
		{"provider recorded on provenance", func(p *SemanticProposal) { p.Provenance.Provider = p.Provider; rehash(p) }, ""},
		{"old record, provider in rule", func(p *SemanticProposal) { p.Provenance.Rule = "provider:" + p.Provider; rehash(p) }, ""},
		{"provenance names another provider", func(p *SemanticProposal) { p.Provenance.Provider = "anthropic"; rehash(p) }, "differs from the proposal's provider"},
		{"legacy rule names another provider", func(p *SemanticProposal) { p.Provenance.Rule = "provider:anthropic"; rehash(p) }, "differs from the proposal's provider"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := validProposal(cand)
			tc.mut(&p)
			expectErr(t, p.Validate(), tc.want)
		})
	}
}
