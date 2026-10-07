package reviewui

// english_test.go pins every plain-English renderer (ux-v2): each condition
// op and field state, each question type, each routing signal, and the
// differ/agreement builders. The renderers are pure functions of the review
// data — the tests construct minimal domain records and assert the wording.

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

func sigItem(prio domain.ReviewPriority, sigs ...domain.RoutingSignal) domain.ReviewItem {
	return domain.ReviewItem{ID: "ri-t", Product: "cert-manager", Release: "v1.18.0",
		QuestionType: domain.QuestionConsequence, Question: "recorded question",
		Status:  domain.ReviewPending,
		Routing: domain.Routing{Route: domain.RouteReview, Priority: prio, Signals: sigs}}
}

func ctxOf(it domain.ReviewItem, props ...domain.SemanticProposal) *knowledge.ReviewContext {
	return &knowledge.ReviewContext{Item: it, Proposals: props}
}

func prop(id, model, call string, a domain.SemanticAssertion) domain.SemanticProposal {
	return domain.SemanticProposal{ID: id, CandidateID: "c1", Assertion: a, Provider: "test",
		Provenance: domain.Provenance{Model: model, CallID: call, Confidence: domain.ConfidenceHigh}}
}

func fullAssertion() domain.SemanticAssertion {
	return domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectCRDField, Kind: "Certificate", Path: "spec.privateKey.rotationPolicy"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str("Never"), After: str("Always")},
		Applicability: &domain.Applicability{Exposure: domain.Condition{Op: domain.OpResource, Kind: "Certificate",
			Of: []domain.Condition{{Op: domain.OpField, Path: "spec.privateKey.rotationPolicy", State: domain.StateUnset}}}},
		Consequence: &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired,
			Statement: "Certificates left unset now rotate keys on every renewal.", Remediation: "Set rotationPolicy: Never explicitly.",
			Severity: domain.SeverityMedium},
		Statement: "rotationPolicy default flipped",
	}
}

// --- condition trees -----------------------------------------------------------------

func TestCondEnglishEveryOp(t *testing.T) {
	cases := []struct {
		name string
		c    domain.Condition
		want string
	}{
		{"all of two", domain.Condition{Op: domain.OpAll, Of: []domain.Condition{
			{Op: domain.OpEnvVar, Name: "HTTP_PROXY", State: domain.StateSet},
			{Op: domain.OpCLIFlag, Name: "dns01-recursive-nameservers", State: domain.StateSet}}},
			"the environment variable `HTTP_PROXY` is set and the flag `--dns01-recursive-nameservers` is set"},
		{"any of two", domain.Condition{Op: domain.OpAny, Of: []domain.Condition{
			{Op: domain.OpEnvVar, Name: "A", State: domain.StateUnset},
			{Op: domain.OpEnvVar, Name: "B", State: domain.StateUnset}}},
			"the environment variable `A` is left unset or the environment variable `B` is left unset"},
		{"all of three needs the list form", domain.Condition{Op: domain.OpAll, Of: []domain.Condition{
			{Op: domain.OpEnvVar, Name: "A", State: domain.StateSet}, {Op: domain.OpEnvVar, Name: "B", State: domain.StateSet},
			{Op: domain.OpEnvVar, Name: "C", State: domain.StateSet}}},
			"all of: the environment variable `A` is set; the environment variable `B` is set; the environment variable `C` is set"},
		{"any of three needs the list form", domain.Condition{Op: domain.OpAny, Of: []domain.Condition{
			{Op: domain.OpEnvVar, Name: "A", State: domain.StateSet}, {Op: domain.OpEnvVar, Name: "B", State: domain.StateSet},
			{Op: domain.OpEnvVar, Name: "C", State: domain.StateSet}}},
			"any of: the environment variable `A` is set; the environment variable `B` is set; the environment variable `C` is set"},
		{"nested non-root gets parentheses", domain.Condition{Op: domain.OpAll, Of: []domain.Condition{
			{Op: domain.OpAny, Of: []domain.Condition{{Op: domain.OpEnvVar, Name: "A", State: domain.StateSet}, {Op: domain.OpEnvVar, Name: "B", State: domain.StateSet}}},
			{Op: domain.OpEnvVar, Name: "C", State: domain.StateSet}}},
			"(the environment variable `A` is set or the environment variable `B` is set) and the environment variable `C` is set"},
		{"not", domain.Condition{Op: domain.OpNot, Of: []domain.Condition{{Op: domain.OpField, Path: "spec.x", State: domain.StateSet}}},
			"not (`spec.x` is set (any value))"},
		{"resource with children", domain.Condition{Op: domain.OpResource, Kind: "Certificate", Of: []domain.Condition{
			{Op: domain.OpField, Path: "spec.privateKey.rotationPolicy", State: domain.StateUnset}}},
			"Certificates where `spec.privateKey.rotationPolicy` is left unset"},
		{"resource without children", domain.Condition{Op: domain.OpResource, Kind: "Deployment"},
			"any Deployments"},
		{"ref", domain.Condition{Op: domain.OpRef, Path: "spec.template.spec.containers[].image", Of: []domain.Condition{
			{Op: domain.OpImageInUse, Name: "quay.io/jetstack/cert-manager-controller", State: domain.StateSet}}},
			"the resource behind `spec.template.spec.containers[].image` where the image `quay.io/jetstack/cert-manager-controller` is set"},
		{"undecidable with needed", domain.Condition{Op: domain.OpUndecidable, Needed: "the live defaulting webhook config"},
			"cannot be decided from the environment (needs the live defaulting webhook config)"},
		{"undecidable without needed", domain.Condition{Op: domain.OpUndecidable},
			"cannot be decided from the environment (needs more environment input)"},
		{"values-key", domain.Condition{Op: domain.OpValuesKey, Path: "crds.enabled", State: domain.StateEquals, Values: []string{`"false"`}},
			"the Helm values set `crds.enabled` is false"},
		{"gvk-in-use", domain.Condition{Op: domain.OpGVKInUse, Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", State: domain.StateSet},
			"the resource kind gateway.networking.k8s.io HTTPRoute in use is set"},
		{"generic op fallback", domain.Condition{Op: "weird-op", Path: "spec.x", Name: "thing"},
			"weird-op `spec.x` thing"},
		{"generic op with values", domain.Condition{Op: "weird-op", Path: "spec.x", Values: []string{`"a"`, `"b"`}},
			"weird-op `spec.x` is a or b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := condEnglish(tc.c, true); got != tc.want {
				t.Fatalf("\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestFieldClauseEveryState(t *testing.T) {
	cases := []struct {
		name  string
		state domain.FieldState
		extra domain.Condition
		want  string
	}{
		{"unset", domain.StateUnset, domain.Condition{}, "`spec.replicas` is left unset"},
		{"set", domain.StateSet, domain.Condition{}, "`spec.replicas` is set (any value)"},
		{"equals", domain.StateEquals, domain.Condition{Values: []string{`"RollingUpdate"`}}, "`strategy.type` is RollingUpdate"},
		{"equals many", domain.StateEquals, domain.Condition{Values: []string{`"a"`, `"b"`}}, "`x` is a or b"},
		{"not-equals", domain.StateNotEquals, domain.Condition{Values: []string{`"Cluster"`}}, "`issuer.kind` is not Cluster"},
		{"matches", domain.StateMatches, domain.Condition{Pattern: `.*\.svc\.cluster\.local`}, "`server` matches /.*\\.svc\\.cluster\\.local/"},
		{"has-token", domain.StateHasToken, domain.Condition{Values: []string{"ValidateCAA"}}, "`config` contains the entry ValidateCAA"},
		{"has-token-key", domain.StateHasTokenKey, domain.Condition{Values: []string{"caaid"}}, "`config` contains an entry keyed caaid"},
		{"in-range falls to leaf wording", domain.StateInRange, domain.Condition{Range: "1.16.x"}, "`x` is within 1.16.x"},
		{"out-of-range", domain.StateOutOfRange, domain.Condition{Range: "1.16.x"}, "`x` is outside 1.16.x"},
		{"exists", domain.StateExists, domain.Condition{Pattern: `^- name:`}, "`x` exists"},
		{"none", domain.StateNone, domain.Condition{Pattern: `^- name:`}, "`x` is absent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := domain.Condition{Op: domain.OpField, Path: "x"}
			if tc.name == "unset" || tc.name == "set" {
				c.Path = "spec.replicas"
			}
			if strings.HasPrefix(tc.want, "`strategy.") {
				c.Path = "strategy.type"
			} else if strings.HasPrefix(tc.want, "`issuer.") {
				c.Path = "issuer.kind"
			} else if strings.HasPrefix(tc.want, "`server`") {
				c.Path = "server"
			} else if strings.HasPrefix(tc.want, "`config`") {
				c.Path = "config"
			}
			c.State = tc.state
			c.Values = tc.extra.Values
			c.Pattern = tc.extra.Pattern
			c.Range = tc.extra.Range
			if got := condEnglish(c, true); got != tc.want {
				t.Fatalf("\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestVersionClauseEveryRangeShape(t *testing.T) {
	cases := []struct {
		r    string
		want string
	}{
		{">=1.16.0", "cert-manager is older than 1.16.0"},
		{">1.16.0", "cert-manager is 1.16.0 or older"},
		{"<=1.18.0", "cert-manager is newer than 1.18.0"},
		{"<1.18.0", "cert-manager is 1.18.0 or newer"},
		{"~1.17", "cert-manager is outside ~1.17"},
		{"", "cert-manager is outside "},
	}
	for _, tc := range cases {
		got := condEnglish(domain.Condition{Op: domain.OpProductVersion, Name: "cert-manager", Range: tc.r, State: domain.StateOutOfRange}, true)
		if got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.r, got, tc.want)
		}
	}
}

// --- the card's question, answer and reasons --------------------------------------------

func TestPlainQuestionEveryType(t *testing.T) {
	it := sigItem(domain.PriorityNormal)
	it.Proposed = fullAssertion()
	cases := []struct {
		q    domain.QuestionType
		want string
	}{
		{domain.QuestionConsequence,
			"Does cert-manager v1.18.0 change what happens to Certificates where `spec.privateKey.rotationPolicy` is left unset — and does that require action?"},
		{domain.QuestionEvidenceSufficiency,
			"Is the cert-manager v1.18.0 release note specific enough to record what actually changed?"},
		{domain.QuestionSemanticMapping,
			"What exactly is the Certificate field `spec.privateKey.rotationPolicy` in cert-manager v1.18.0 — and how did it change?"},
		{domain.QuestionApplicability,
			"Who exactly is exposed to the cert-manager v1.18.0 change to the Certificate field `spec.privateKey.rotationPolicy`?"},
		{domain.QuestionClassification,
			"Is the cert-manager v1.18.0 change to the Certificate field `spec.privateKey.rotationPolicy` something to act on, or just to know?"},
		{domain.QuestionRelationship,
			"Does cert-manager v1.18.0 change a product relationship — and which environments are exposed?"},
		{domain.QuestionDuplicate, "recorded question"},
		{"future-type", "recorded question"},
	}
	for _, tc := range cases {
		t.Run(string(tc.q), func(t *testing.T) {
			it.QuestionType = tc.q
			if got := plainQuestion(it); got != tc.want {
				t.Fatalf("\n got %q\nwant %q", got, tc.want)
			}
		})
	}
	// a relationship question with a named counterpart names it
	rel := it
	rel.QuestionType = domain.QuestionRelationship
	rel.Proposed.Subject = &domain.Subject{Family: domain.SubjectProductRelationship, Product: "cert-manager", Name: "gateway-api"}
	if got, want := plainQuestion(rel),
		"Does cert-manager v1.18.0 change how it works together with gateway-api — and which environments are exposed?"; got != want {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
	// a consequence question without applicability falls back to the subject
	bare := it
	bare.QuestionType = domain.QuestionConsequence
	bare.Proposed = domain.SemanticAssertion{Subject: fullAssertion().Subject, Consequence: fullAssertion().Consequence}
	if got := plainQuestion(bare); !strings.Contains(got, "environments using the Certificate field `spec.privateKey.rotationPolicy`") {
		t.Fatalf("subject fallback missing: %q", got)
	}
}

func TestSuggestedAnswer(t *testing.T) {
	it := sigItem(domain.PriorityNormal)
	it.Proposed = fullAssertion()
	got := suggestedAnswer(it, "")
	want := "The default of the Certificate field `spec.privateKey.rotationPolicy` changed from `Never` to `Always`. " +
		"Certificates left unset now rotate keys on every renewal. Fix: Set rotationPolicy: Never explicitly. → REVIEW REQUIRED · medium"
	if got != want {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
	// a gate item: always "No", with the models' reason or a generic one
	gate := sigItem(domain.PriorityNormal)
	gate.QuestionType = domain.QuestionEvidenceSufficiency
	if got, want := suggestedAnswer(gate, "the note never names the field"), "No — not specific enough to record: the note never names the field."; got != want {
		t.Fatalf("gate with reason: %q", got)
	}
	if got, want := suggestedAnswer(gate, ""), "No — not specific enough to record: the note is too vague."; got != want {
		t.Fatalf("gate without reason: %q", got)
	}
	// statement-only assertions still produce an answer
	thin := sigItem(domain.PriorityNormal)
	thin.Proposed = domain.SemanticAssertion{Statement: "Something changed."}
	if got := suggestedAnswer(thin, ""); !strings.Contains(got, "Something changed.") {
		t.Fatalf("statement fallback: %q", got)
	}
}

func TestGateReasonPicksTheFirstUndeterminedReason(t *testing.T) {
	rc := ctxOf(sigItem(domain.PriorityNormal),
		prop("p1", "claude-opus-5-5", "c1", domain.SemanticAssertion{}),
		prop("p2", "glm-5.3-flash", "c2", domain.SemanticAssertion{}))
	rc.Proposals[0].UndeterminedReason = "note is prose only"
	rc.Proposals[1].UndeterminedReason = "later reason"
	if got := gateReason(rc); got != "note is prose only" {
		t.Fatalf("got %q", got)
	}
	if got := gateReason(ctxOf(sigItem(domain.PriorityNormal))); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

// --- why-this-needs-you: every routing signal ------------------------------------------------

func disagreeCtx() *knowledge.ReviewContext {
	// three separate calls: two opus calls agree on the consequence, glm differs
	a := fullAssertion()
	other := a
	other.Consequence = &domain.Consequence{Kind: domain.ConsequenceSettingIgnored, ExposedClass: domain.ImpactInformational,
		Statement: "The default is ignored for existing resources."}
	p1 := prop("p1", "claude-opus-5-5", "c1", a)
	p2 := prop("p2", "claude-opus-5-5", "c2", a)
	p3 := prop("p3", "glm-5.3-flash", "c3", other)
	rc := ctxOf(sigItem(domain.PriorityNormal, domain.SignalModelsDisagree), p1, p2, p3)
	consGroups := map[string][]string{
		a.AspectDigest(domain.AspectConsequence):     {"p1", "p2"},
		other.AspectDigest(domain.AspectConsequence): {"p3"},
	}
	rc.Agreement = []knowledge.AspectAgreement{
		{Aspect: domain.AspectSubject, Groups: map[string][]string{a.AspectDigest(domain.AspectSubject): {"p1", "p2", "p3"}}},
		{Aspect: domain.AspectConsequence, Groups: consGroups},
	}
	rc.Item.Proposed = a
	return rc
}

func TestWhyYouEverySignal(t *testing.T) {
	base := sigItem(domain.PriorityNormal)
	normal := ctxOf(base,
		prop("p1", "claude-opus-5-5", "c1", fullAssertion()),
		prop("p2", "glm-5.3-flash", "c2", fullAssertion()))
	cases := []struct {
		name string
		rc   *knowledge.ReviewContext
		act  bool
		want string
	}{
		{"consensus action signal", ctxOf(sigItem(domain.PriorityNormal, domain.SignalConsensusAction),
			prop("p1", "claude-opus-5-5", "c1", fullAssertion()),
			prop("p2", "glm-5.3-flash", "c2", fullAssertion())), false,
			"High impact: separate model calls (Opus 5.5, GLM 5.3 Flash) agree this is ACTION REQUIRED — policy sends every such consensus to a human audit. This is that audit."},
		{"consensus action computed", normal, true,
			"High impact: separate model calls (Opus 5.5, GLM 5.3 Flash) agree this is ACTION REQUIRED — policy sends every such consensus to a human audit. This is that audit."},
		{"models disagree", disagreeCtx(), false,
			"The models disagree on what happens — up to 2 different answers from 3 calls. A human settles it."},
		{"all undetermined", func() *knowledge.ReviewContext {
			rc := ctxOf(sigItem(domain.PriorityNormal, domain.SignalAllUndetermined),
				prop("p1", "claude-opus-5-5", "c1", domain.SemanticAssertion{}))
			rc.Proposals[0].UndeterminedReason = "the note is prose only"
			return rc
		}(), false, "No model could tell what changed: the note is prose only."},
		{"all undetermined generic", ctxOf(sigItem(domain.PriorityNormal, domain.SignalAllUndetermined),
			prop("p1", "claude-opus-5-5", "c1", domain.SemanticAssertion{})), false,
			"No model could tell what changed: no model could pin down what changed."},
		{"validation refuted", ctxOf(sigItem(domain.PriorityNormal, domain.SignalValidationRefuted),
			prop("p1", "claude-opus-5-5", "c1", fullAssertion())), false,
			"A deterministic check contradicted part of a proposal — the recorded answer may be wrong."},
		{"single model", ctxOf(sigItem(domain.PriorityNormal, domain.SignalSingleModel),
			prop("p1", "claude-opus-5-5", "c1", fullAssertion())), false,
			"Only one model call has answered this — no second opinion yet."},
		{"high priority", ctxOf(sigItem(domain.PriorityHigh),
			prop("p1", "claude-opus-5-5", "c1", fullAssertion()),
			prop("p2", "glm-5.3-flash", "c2", fullAssertion())), false,
			"Routed high priority: models see real impact for exposed environments."},
		{"routine", normal, false, "Models agree; this is a routine confirmation."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := whyYou(tc.rc, tc.act); got != tc.want {
				t.Fatalf("\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestWhyYouBriefAndTone(t *testing.T) {
	cases := []struct {
		name  string
		it    domain.ReviewItem
		calls int
		want  string
	}{
		{"consensus action", sigItem(domain.PriorityNormal, domain.SignalConsensusAction), 2,
			"High impact: separate model calls agree this is ACTION REQUIRED — this is the human audit policy requires."},
		{"disagree", sigItem(domain.PriorityNormal, domain.SignalModelsDisagree), 3,
			"The models disagree — open the item to see the answers side by side."},
		{"undetermined", sigItem(domain.PriorityNormal, domain.SignalAllUndetermined), 2,
			"No model could tell what changed."},
		{"refuted", sigItem(domain.PriorityNormal, domain.SignalValidationRefuted), 2,
			"A deterministic check contradicted part of a proposal."},
		{"single-model signal", sigItem(domain.PriorityNormal, domain.SignalSingleModel), 1,
			"Only one model call has answered this — no second opinion yet."},
		{"one call, no signal", sigItem(domain.PriorityNormal), 1,
			"Only one model call has answered this — no second opinion yet."},
		{"high priority", sigItem(domain.PriorityHigh), 2, "High priority: models see real impact for exposed environments."},
		{"routine", sigItem(domain.PriorityNormal), 2, "Models agree; this is a routine confirmation."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := whyYouBrief(tc.it, tc.calls); got != tc.want {
				t.Fatalf("\n got %q\nwant %q", got, tc.want)
			}
		})
	}
	// the tone mirrors the strongest signal: danger only for consensus ACTION
	if got := whyTone(sigItem(domain.PriorityNormal), true); got != "danger" {
		t.Errorf("consensus action tone %q", got)
	}
	for _, s := range []domain.RoutingSignal{domain.SignalModelsDisagree, domain.SignalAllUndetermined, domain.SignalValidationRefuted} {
		if got := whyTone(sigItem(domain.PriorityNormal, s), false); got != "warn" {
			t.Errorf("signal %v tone %q", s, got)
		}
	}
	if got := whyTone(sigItem(domain.PriorityNormal), false); got != "" {
		t.Errorf("routine tone %q", got)
	}
}

// --- §5: differ rows and agreement chips ---------------------------------------------------

func TestBuildDifferGroupsCellsAndChips(t *testing.T) {
	rc := disagreeCtx()
	rc.Item.Routing.Signals = nil
	rows, agrees, und := buildDiffer(rc)
	if und != "" {
		t.Fatalf("unexpected undetermined %q", und)
	}
	if len(rows) != 1 || rows[0].Label != "what happens" {
		t.Fatalf("rows %+v", rows)
	}
	cells := rows[0].Cells
	if len(cells) != 2 {
		t.Fatalf("cells %+v", cells)
	}
	// biggest group first: the two opus calls, labelled same-model consensus
	first := cells[0]
	if first.Who != "Opus 5.5 + Opus 5.5" || first.Consensus != "same-model consensus · 2 calls" {
		t.Fatalf("first cell %+v", first)
	}
	if !first.AsProposed {
		t.Error("the group matching the suggested assertion must be marked")
	}
	if len(first.Avatars) != 2 || first.Avatars[0].Initial != "O" {
		t.Fatalf("avatars %+v", first.Avatars)
	}
	second := cells[1]
	if second.Who != "GLM 5.3 Flash" || second.Consensus != "single call" {
		t.Fatalf("second cell %+v", second)
	}
	if second.AsProposed {
		t.Error("a differing answer is never the suggested one")
	}
	// the agreeing subject collapses to a cross-model chip (3 separate calls)
	if len(agrees) != 1 || agrees[0].Label != "what the thing is" {
		t.Fatalf("agrees %+v", agrees)
	}
	if agrees[0].Text != "consensus · cross-model · 3 calls" || agrees[0].Tone != "ok" {
		t.Fatalf("chip %+v", agrees[0])
	}
}

func TestBuildDifferSingleCallChipAndUndetermined(t *testing.T) {
	// two proposals, but only one commits to the subject: a muted chip, and
	// the undetermined aspect surfaces its reason
	a := fullAssertion()
	p1 := prop("p1", "claude-opus-5-5", "c1", a)
	p2 := prop("p2", "claude-opus-5-5", "c2", domain.SemanticAssertion{Consequence: a.Consequence})
	p2.Undetermined = []domain.Aspect{domain.AspectSubject, domain.AspectChange}
	p2.UndeterminedReason = "the note never names the resource"
	it := sigItem(domain.PriorityNormal)
	it.Proposed = a
	rc := ctxOf(it, p1, p2)
	rc.Agreement = []knowledge.AspectAgreement{
		{Aspect: domain.AspectSubject, Groups: map[string][]string{a.AspectDigest(domain.AspectSubject): {"p1"}}, Undetermined: []string{"p2"}},
		{Aspect: domain.AspectChange, Undetermined: []string{"p2"}},
		{Aspect: domain.AspectConsequence, Groups: map[string][]string{a.AspectDigest(domain.AspectConsequence): {"p1", "p2"}}},
	}
	rows, agrees, und := buildDiffer(rc)
	if len(rows) != 0 {
		t.Fatalf("rows %+v", rows)
	}
	found := false
	for _, ch := range agrees {
		if ch.Label == "what the thing is" {
			found = true
			if ch.Text != "single call" || ch.Tone != "muted" {
				t.Fatalf("chip %+v", ch)
			}
		}
		if ch.Label == "what happens" && ch.Text != "consensus · same-model · 2 calls" {
			t.Fatalf("chip %+v", ch)
		}
	}
	if !found {
		t.Fatalf("no subject chip in %+v", agrees)
	}
	if und != "the note never names the resource" {
		t.Fatalf("undetermined %q", und)
	}
}

func TestBuildDifferGroupOrderIsStable(t *testing.T) {
	// two equally-sized groups: alphabetical model name decides
	a := fullAssertion()
	b := a
	b.Consequence = &domain.Consequence{Kind: domain.ConsequenceDeprecation, ExposedClass: domain.ImpactInformational}
	p1 := prop("p1", "glm-5.3-flash", "c1", a)
	p2 := prop("p2", "claude-opus-5-5", "c2", b)
	it := sigItem(domain.PriorityNormal)
	it.Proposed = a
	rc := ctxOf(it, p1, p2)
	rc.Agreement = []knowledge.AspectAgreement{{Aspect: domain.AspectConsequence, Groups: map[string][]string{
		a.AspectDigest(domain.AspectConsequence): {"p1"},
		b.AspectDigest(domain.AspectConsequence): {"p2"},
	}}}
	rows, _, _ := buildDiffer(rc)
	if len(rows) != 1 || len(rows[0].Cells) != 2 {
		t.Fatalf("rows %+v", rows)
	}
	if rows[0].Cells[0].Who != "Opus 5.5" {
		t.Fatalf("alphabetical order broken: %+v", rows[0].Cells)
	}
	if rows[0].Cells[0].Class != "g0" || rows[0].Cells[1].Class != "g1" {
		t.Fatalf("group colors %+v", rows[0].Cells)
	}
}

// --- why-to-trust bullets -------------------------------------------------------------------

func TestWhyConfidentEveryTone(t *testing.T) {
	rc := disagreeCtx()
	rc.Item.Routing.Signals = nil
	rc.Validations = []domain.ValidationResult{{
		ID: "v1", Validator: "semvalidate.crd@v1", Checks: []domain.AspectCheck{
			{Aspect: domain.AspectChange, Outcome: domain.OutcomeConfirmed, Rule: "crd-schema:default", Detail: "the CRD default is Always from 1.18"},
			{Aspect: domain.AspectApplicability, Outcome: domain.OutcomeInconclusive, Rule: "canonical:env", Detail: "depends on the cluster"},
		}},
	}
	rc.Candidate.Evidence = []domain.Evidence{
		{ID: "e1", Kind: "release-note", URI: "https://example/1", Excerpt: "The default value of Certificate.spec.privateKey.rotationPolicy is now Always, affecting every renewal."},
		{ID: "e2", Kind: "upgrade-guide", URI: "https://example/2", Excerpt: "Set rotationPolicy explicitly when you pin keys."},
	}
	lines := whyConfident(rc, false)
	var have map[string]string
	have = map[string]string{}
	for _, l := range lines {
		have[l.Text] = l.Tone
	}
	if have["The 3 calls give 2 different answers on what happens — see below."] != "doubt" {
		t.Fatalf("doubt line missing: %+v", lines)
	}
	if have["All 3 calls give the same answer on what the thing is."] != "" {
		t.Fatalf("support line missing: %+v", lines)
	}
	if have["Checked: the CRD default is Always from 1.18 (crd-schema:default)."] != "" {
		t.Fatalf("checked line missing: %+v", lines)
	}
	if have["Not checkable: depends on the cluster."] != "doubt" {
		t.Fatalf("inconclusive line missing: %+v", lines)
	}
	if have["2 independent sources back the statement."] != "" {
		t.Fatalf("breadth line missing: %+v", lines)
	}
	// refuted + thin evidence + no validators
	thin := ctxOf(sigItem(domain.PriorityNormal), prop("p1", "claude-opus-5-5", "c1", fullAssertion()))
	thin.Validations = []domain.ValidationResult{{ID: "v2", Validator: "semvalidate@v1", Checks: []domain.AspectCheck{
		{Aspect: domain.AspectChange, Outcome: domain.OutcomeRefuted, Rule: "canonical:default", Detail: "the CRD still says Never"}}}}
	thin.Candidate.Evidence = []domain.Evidence{{ID: "e1", Kind: "release-note", URI: "https://example/1", Excerpt: "short line"}}
	lines = whyConfident(thin, false)
	have = map[string]string{}
	for _, l := range lines {
		have[l.Text] = l.Tone
	}
	if have["Contradicted: the CRD still says Never (canonical:default)."] != "bad" {
		t.Fatalf("bad line missing: %+v", lines)
	}
	if have["No deterministic validator has checked this item."] != "" {
		t.Error("the no-validator line must not appear when a validator ran")
	}
	if have["Evidence is thin: one source, a single line."] != "doubt" {
		t.Fatalf("thin line missing: %+v", lines)
	}
	// consensus action adds the separate-calls line
	ca := whyConfident(ctxOf(sigItem(domain.PriorityNormal),
		prop("p1", "claude-opus-5-5", "c1", fullAssertion()),
		prop("p2", "glm-5.3-flash", "c2", fullAssertion())), true)
	seen := false
	for _, l := range ca {
		if l.Text == "The agreeing calls are separate and from different models." && l.Tone == "" {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("consensus line missing: %+v", ca)
	}
}

// --- model names, avatars, sentences ---------------------------------------------------------

func TestModelShortAndAvatar(t *testing.T) {
	cases := map[string]string{
		"claude-opus-5-5":           "Opus 5.5",
		"claude-sonnet-5-5":         "Sonnet 5.5",
		"claude-haiku-4-5-20251001": "Haiku 4.5 20251001",
		"glm-5.3-flash":             "GLM 5.3 Flash",
		"glm-5.3":                   "GLM 5.3",
		"codex-mini-latest":         "Codex Mini Latest",
		"mystery-7b":                "Mystery 7b",
		"":                          "",
	}
	for raw, want := range cases {
		if got := modelShort(raw); got != want {
			t.Errorf("modelShort(%q) = %q, want %q", raw, got, want)
		}
	}
	av := avatarOf("claude-opus-5-5")
	if av.Name != "Opus 5.5" || av.Initial != "O" {
		t.Fatalf("avatar %+v", av)
	}
	if !strings.HasPrefix(av.Class, "av") || len(av.Class) != 3 {
		t.Fatalf("avatar class %q", av.Class)
	}
	if got := avatarOf(""); got.Initial != "·" {
		t.Fatalf("empty model initial %q", got.Initial)
	}
}

func TestSentences(t *testing.T) {
	// subject phrases across families
	subs := []struct {
		s    *domain.Subject
		want string
	}{
		{nil, "this"},
		{&domain.Subject{Family: domain.SubjectCRDField, Kind: "Certificate", Path: "spec.x"}, "the Certificate field `spec.x`"},
		{&domain.Subject{Family: domain.SubjectCRDField, Path: "spec.x"}, "the resource field `spec.x`"},
		{&domain.Subject{Family: domain.SubjectCLIFlag, Component: "controller", Name: "foo"}, "the controller flag `--foo`"},
		{&domain.Subject{Family: domain.SubjectCLIFlag, Name: "foo"}, "the flag `--foo`"},
		{&domain.Subject{Family: domain.SubjectHelmValue, Path: "crds.enabled"}, "the Helm value `crds.enabled`"},
		{&domain.Subject{Family: domain.SubjectConfigKey, Path: "server.rbac.policy.default"}, "the config key `server.rbac.policy.default`"},
		{&domain.Subject{Family: domain.SubjectProductRelationship, Product: "cert-manager", Name: "gateway-api"},
			"how cert-manager works together with gateway-api"},
		{&domain.Subject{Family: "future-family", Kind: "Widget"}, "future family `Widget`"},
		{&domain.Subject{Family: "future-family"}, "future-family"},
	}
	for _, tc := range subs {
		if got := subjectPhrase(tc.s); got != tc.want {
			t.Errorf("subjectPhrase(%+v) = %q, want %q", tc.s, got, tc.want)
		}
	}
	// change sentences
	ca := fullAssertion()
	if got := changeSentence(ca); got != "the default of the Certificate field `spec.privateKey.rotationPolicy` changed from `Never` to `Always`" {
		t.Fatalf("default-changed: %q", got)
	}
	added := domain.SemanticAssertion{Subject: ca.Subject, Change: &domain.ChangeSpec{Type: domain.ChangeKindAdded, After: str(`"true"`)}}
	if got := changeSentence(added); got != "the Certificate field `spec.privateKey.rotationPolicy` was added to `true`" {
		t.Fatalf("added: %q", got)
	}
	renamed := domain.SemanticAssertion{Subject: ca.Subject, Change: &domain.ChangeSpec{Type: domain.ChangeKindRenamed,
		ReplacedBy: &domain.Subject{Family: domain.SubjectConfigKey, Path: "configs.rbac.policy.default"}}}
	if got := changeSentence(renamed); got != "the Certificate field `spec.privateKey.rotationPolicy` was renamed (its replacement is the config key `configs.rbac.policy.default`)" {
		t.Fatalf("renamed: %q", got)
	}
	tightened := domain.SemanticAssertion{Subject: ca.Subject, Change: &domain.ChangeSpec{Type: domain.ChangeKindValidationTightened}}
	if got := changeSentence(tightened); got != "the Certificate field `spec.privateKey.rotationPolicy` has stricter validation now" {
		t.Fatalf("tightened: %q", got)
	}
	unknown := domain.SemanticAssertion{Subject: ca.Subject, Change: &domain.ChangeSpec{Type: "mystery-kind", Before: str("a")}}
	if got := changeSentence(unknown); got != "the Certificate field `spec.privateKey.rotationPolicy`: mystery kind (was `a`)" {
		t.Fatalf("unknown kind: %q", got)
	}
	if got := changeSentence(domain.SemanticAssertion{Subject: ca.Subject}); got != "" {
		t.Fatalf("no change: %q", got)
	}
	// consequence + aspect sentences
	if got := consequenceSentence(ca.Consequence); got != "its behaviour changes — Certificates left unset now rotate keys on every renewal (severity: medium)" {
		t.Fatalf("consequence: %q", got)
	}
	if got := consequenceSentence(&domain.Consequence{Kind: domain.ConsequenceNone}); got != "nothing needs to be done" {
		t.Fatalf("bare consequence: %q", got)
	}
	if got := consequenceSentence(nil); got != "" {
		t.Fatalf("nil consequence: %q", got)
	}
	if got := aspectSentence(ca, domain.AspectConsequence); got != "Its behaviour changes — Certificates left unset now rotate keys on every renewal (severity: medium)" {
		t.Fatalf("consequence aspect: %q", got)
	}
	if got := aspectSentence(ca, domain.AspectApplicability); got != "Exposed: Certificates where `spec.privateKey.rotationPolicy` is left unset" {
		t.Fatalf("applicability aspect: %q", got)
	}
	// shielded when the overlap repeats the resource noun
	shielded := fullAssertion()
	shielded.Applicability.Overlap = &domain.Condition{Op: domain.OpResource, Kind: "Certificate",
		Of: []domain.Condition{{Op: domain.OpField, Path: "spec.privateKey.rotationPolicy", State: domain.StateEquals, Values: []string{`"Never"`}}}}
	if got := aspectSentence(shielded, domain.AspectApplicability); got != "Exposed: Certificates where `spec.privateKey.rotationPolicy` is left unset. Shielded when `spec.privateKey.rotationPolicy` is Never" {
		t.Fatalf("shielded: %q", got)
	}
	bare := domain.SemanticAssertion{}
	if got := aspectSentence(bare, domain.AspectChange); got != "no change stated" {
		t.Fatalf("no change aspect: %q", got)
	}
	if got := aspectSentence(bare, domain.AspectConsequence); got != "no consequence stated" {
		t.Fatalf("no consequence aspect: %q", got)
	}
	if got := aspectSentence(bare, domain.AspectSubject); got != "this" {
		t.Fatalf("nil subject aspect: %q", got)
	}
}

func TestTruncate(t *testing.T) {
	for _, tc := range []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly-10", 10, "exactly-10"},
		{"much too long", 8, "much to…"},
	} {
		if got := truncate(tc.in, tc.n); got != tc.want {
			t.Errorf("truncate(%q,%d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}
