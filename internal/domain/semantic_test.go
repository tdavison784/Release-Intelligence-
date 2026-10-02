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

func rotationSubject() *Subject {
	return &Subject{Family: SubjectCRDField, Product: "cert-manager", Group: "cert-manager.io", Kind: "Certificate", Path: "spec.privateKey.rotationPolicy"}
}

func rotationAssertion() SemanticAssertion {
	return SemanticAssertion{
		Subject: rotationSubject(),
		Change:  &ChangeSpec{Type: ChangeKindDefaultChanged, Before: str(`"Never"`), After: str(`"Always"`)},
		Applicability: &Applicability{
			Exposure: Condition{Op: OpResourceField, Group: "cert-manager.io", Kind: "Certificate", Path: "spec.privateKey.rotationPolicy", State: StateUnset},
			Overlap:  &Condition{Op: OpResourceField, Group: "cert-manager.io", Kind: "Certificate", Path: "spec.privateKey.rotationPolicy", State: StateSet},
		},
		Consequence: &Consequence{Kind: ConsequenceBehaviorChange, Statement: "private keys are regenerated on every renewal", Severity: SeverityHigh},
		Statement:   "Certificate.spec.privateKey.rotationPolicy default Never → Always",
	}
}

func anchor() ChangeAnchor {
	return ChangeAnchor{ChangeID: "chg-rot", Release: "v1.18.0", EvidenceKeys: []string{EvidenceKey(upEvidence())}}
}

func validCandidate() SemanticCandidate {
	a := anchor()
	return SemanticCandidate{
		ID: CandidateID("cert-manager", a), Product: "cert-manager", Release: a.Release, Anchor: a,
		Category: CategoryConfiguration, Title: "The default rotationPolicy is now Always",
		Evidence: []Evidence{upEvidence(), crdEvidence()}, Producer: "semantic.candidates@v1", CreatedAt: t0,
	}
}

func aiProvenance(model string, input ...EvidenceID) Provenance {
	at := t0
	return Provenance{Method: MethodAI, Producer: "semantic.propose@v1", Confidence: ConfidenceMedium,
		Model: model, ModelVersion: model, PromptVersion: "semantic/v1", PromptDigest: "sha256:" + model,
		InputEvidence: input, GeneratedAt: &at}
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
		Product: c.Product, Release: c.Release, Anchor: c.Anchor, CandidateID: c.ID, Assertion: rotationAssertion(),
		Verification: []AspectVerification{
			{Aspect: AspectSubject, Level: VerifiedDeterministic, Basis: []string{v.ID}},
			{Aspect: AspectChange, Level: VerifiedDeterministic, Basis: []string{v.ID}},
			{Aspect: AspectApplicability, Level: VerifiedDeterministic, Basis: []string{v.ID}},
			{Aspect: AspectConsequence, Level: VerifiedHuman, Basis: []string{d.ID}},
		},
		Evidence: []Evidence{upEvidence(), crdEvidence()}, Status: FactActive, CreatedAt: t0,
	}
	f.ID = VerifiedFactID(f.Product, f.Anchor, f.Assertion)
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
		{"renamed without target", ChangeSpec{Type: ChangeKindRenamed}, SubjectHelmValue, "renamedTo is required"},
		{"renamed across families", ChangeSpec{Type: ChangeKindRenamed, RenamedTo: &Subject{Family: SubjectEnvVar, Product: "p", Name: "X"}}, SubjectHelmValue, "differs from subject family"},
		{"renamedTo on removal", ChangeSpec{Type: ChangeKindRemoved, RenamedTo: &Subject{Family: SubjectHelmValue, Product: "p", Path: "a"}}, SubjectHelmValue, "renamed only"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { expectErr(t, c.c.Validate(c.family), c.want) })
	}
}

func TestConditionValidate(t *testing.T) {
	leaf := Condition{Op: OpValuesKey, Path: "webhook.timeoutSeconds", State: StateUnset}
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
		{"values key equals", Condition{Op: OpValuesKey, Path: "a", State: StateEquals, Values: []string{`"x"`, `2`}}, ""},
		{"equals without values", Condition{Op: OpValuesKey, Path: "a", State: StateEquals}, "needs values"},
		{"values with unset", Condition{Op: OpValuesKey, Path: "a", State: StateUnset, Values: []string{`1`}}, "only used with equals"},
		{"value not json", Condition{Op: OpValuesKey, Path: "a", State: StateEquals, Values: []string{"Never"}}, "not JSON-encoded"},
		{"missing state", Condition{Op: OpValuesKey, Path: "a"}, "state is required"},
		{"gate state on values", Condition{Op: OpValuesKey, Path: "a", State: StateEnabled}, "not allowed"},
		{"foreign field", Condition{Op: OpValuesKey, Path: "a", State: StateSet, Kind: "Pod"}, "kind is not a field"},
		{"unknown op", Condition{Op: "regex"}, "unknown op"},
		{"empty combinator", Condition{Op: OpAny}, "at least one operand"},
		{"combinator with fields", Condition{Op: OpAll, Of: []Condition{leaf}, Path: "x"}, "not allowed on a combinator"},
		{"leaf with operands", Condition{Op: OpGVKInUse, Kind: "Ingress", Of: []Condition{leaf}}, "no operands"},
		{"nested error located", Condition{Op: OpAll, Of: []Condition{leaf, {Op: OpEnvVar}}}, "all[1]"},
		{"too deep", deep, "deeper than"},
		{"gvk no state", Condition{Op: OpGVKInUse, Group: "networking.k8s.io", Version: "v1beta1", Kind: "Ingress"}, ""},
		{"gvk with state", Condition{Op: OpGVKInUse, Kind: "Ingress", State: StateSet}, "takes no state"},
		{"product version", Condition{Op: OpProductVersion, Name: "ingress-nginx", State: StateOutOfRange, Range: ">=1.12.6"}, ""},
		{"product version bad range", Condition{Op: OpProductVersion, Name: "ingress-nginx", State: StateInRange, Range: "recent"}, "range"},
		{"product version no range", Condition{Op: OpProductVersion, Name: "ingress-nginx", State: StateInRange}, "range is required"},
		{"image any tag", Condition{Op: OpImageInUse, Name: "quay.io/jetstack/cert-manager-controller"}, ""},
		{"image range without state", Condition{Op: OpImageInUse, Name: "quay.io/x", Range: "<1.18"}, "go together"},
		{"feature gate", Condition{Op: OpFeatureGate, Name: "ServerSideApply", State: StateEnabled, Path: "featureGates"}, ""},
		{"undecidable", Condition{Op: OpUndecidable, Reason: UnknownRuntimeBehaviorGap, Needed: "live ACME traffic"}, ""},
		{"undecidable bad reason", Condition{Op: OpUndecidable, Reason: "vibes", Needed: "x"}, "unknown reason"},
		{"undecidable without needed", Condition{Op: OpUndecidable, Reason: UnknownEvidenceGap}, "needed is required"},
		{"cross-product composite", Condition{Op: OpAll, Of: []Condition{
			{Op: OpProductVersion, Name: "ingress-nginx", State: StateOutOfRange, Range: ">=1.12.6"},
			{Op: OpResourceField, Group: "acme.cert-manager.io", Kind: "Issuer", Path: "spec.acme.solvers.http01.ingress", State: StateSet},
		}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { expectErr(t, c.c.Validate(), c.want) })
	}
}

func TestConsequence(t *testing.T) {
	expectErr(t, Consequence{Kind: ConsequenceBehaviorChange}.Validate(), "statement")
	expectErr(t, Consequence{Kind: ConsequenceNone}.Validate(), "")
	expectErr(t, Consequence{Kind: ConsequenceDeprecation, Severity: "apocalyptic"}.Validate(), "unknown severity")
	expectErr(t, Consequence{Kind: "explodes", Statement: "x"}.Validate(), "unknown kind")
	for _, k := range ConsequenceKinds {
		cls := k.ExposedClass()
		if cls == ImpactActionRequired && !k.ActionEligible() {
			t.Errorf("%s: only action-eligible kinds may map to action-required", k)
		}
	}
	if ConsequenceDeprecation.ExposedClass() != ImpactReviewRequired || ConsequenceNone.ExposedClass() != ImpactInformational {
		t.Error("deprecation → review-required, none → informational")
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
	moved := ev
	moved.ContentDigest = "sha256:edited-elsewhere"
	moved.ID = "ev-moved"
	pool := map[EvidenceID]Evidence{ev.ID: ev, moved.ID: moved}
	lookup := func(id EvidenceID) (Evidence, bool) { e, ok := pool[id]; return e, ok }
	c := Change{ID: "chg-rot", Release: "v1.18.0", Evidence: []EvidenceID{ev.ID}}
	a := NewChangeAnchor(c, lookup)
	cases := []struct {
		name string
		c    Change
		want bool
	}{
		{"same change", c, true},
		{"same change, release unstated", Change{ID: "chg-rot", Evidence: []EvidenceID{ev.ID}}, true},
		{"re-ingested: new id, same evidence text, digest changed", Change{ID: "chg-other", Release: "v1.18.0", Evidence: []EvidenceID{moved.ID}}, true},
		{"same text in another release", Change{ID: "chg-rot", Release: "v1.19.0", Evidence: []EvidenceID{ev.ID}}, false},
		{"unrelated change", Change{ID: "chg-x", Release: "v1.18.0"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := a.Matches(tc.c, lookup); got != tc.want {
				t.Fatalf("Matches = %v, want %v", got, tc.want)
			}
		})
	}
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
		{"release mismatch", func(c *SemanticCandidate) { c.Release = "v9" }, "differs from anchor"},
		{"no anchor", func(c *SemanticCandidate) { c.Anchor.ChangeID = ""; c.ID = CandidateID(c.Product, c.Anchor) }, "changeId is required"},
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
		{"suggests action", func(p *SemanticProposal) { p.SuggestedClass = ImpactActionRequired }, "never"[:0] + "not \"action-required\""},
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
		a.Consequence = &Consequence{Kind: ConsequenceNone}
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
			f.ID = VerifiedFactID(f.Product, f.Anchor, f.Assertion)
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
			f.ID = VerifiedFactID(f.Product, f.Anchor, f.Assertion)
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
				a.Consequence = &Consequence{Kind: ConsequenceNone}
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
