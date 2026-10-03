package proxyreview

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

func s(v string) *string { return &v }

var t0 = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

func proposal(c domain.SemanticCandidate, model, call string, a domain.SemanticAssertion) domain.SemanticProposal {
	at := t0
	p := domain.SemanticProposal{CandidateID: c.ID, Task: domain.TaskFull, Provider: "anthropic", Assertion: a, Citations: []domain.EvidenceID{c.Evidence[0].ID},
		Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "semantic.proposer@v1", Confidence: domain.ConfidenceMedium, Model: model,
			ModelVersion: model, PromptVersion: "semantic-full/v1", PromptDigest: "sha256:x", InputEvidence: []domain.EvidenceID{c.Evidence[0].ID},
			GeneratedAt: &at, CallID: call}}
	p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	return p
}

// fixture: a consequence question; P(opus) says behavior-change, P(glm) says setting-ignored.
func fixture(q domain.QuestionType) *knowledge.ReviewContext {
	ev := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example.test/notes.md", "L1",
		"The default of rotationPolicy is now Always.", "sha256:d", t0)
	members := []domain.CandidateMember{{ChangeID: "chg-1"}}
	c := domain.SemanticCandidate{ID: domain.CandidateID("cert-manager", "v1.18.0", members), Product: "cert-manager", Release: "v1.18.0",
		Members: members, Grouping: "single", Category: domain.CategoryConfiguration, Title: "rotationPolicy default is now Always",
		Evidence: []domain.Evidence{ev}, Producer: "semantic.candidates@v1", CreatedAt: t0}
	base := domain.SemanticAssertion{
		Subject:       &domain.Subject{Family: domain.SubjectHelmValue, Product: "cert-manager", Path: "rotation.policy"},
		Change:        &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: s(`"Never"`), After: s(`"Always"`)},
		Applicability: &domain.Applicability{Exposure: domain.Condition{Op: domain.OpValuesKey, Path: "rotation.policy", State: domain.StateUnset}},
		Consequence:   &domain.Consequence{Kind: domain.ConsequenceSettingIgnored, ExposedClass: domain.ImpactActionRequired, Statement: "x"},
	}
	other := base
	other.Consequence = &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired, Statement: "y"}
	p1 := proposal(c, "glm-5.3", "call-1", base)
	p2 := proposal(c, "claude-opus-5-5", "call-2", other)
	it := domain.ReviewItem{ID: "ri-000000000001", CandidateID: c.ID, Product: c.Product, Release: c.Release, QuestionType: q,
		Question: "What happens?", Proposed: base, Proposals: []string{p1.ID, p2.ID}, Status: domain.ReviewPending,
		Routing: domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityNormal},
		Context: &domain.EnvironmentContext{Label: "some-env-label"}, CreatedAt: t0}
	if q == domain.QuestionEvidenceSufficiency {
		it.Proposed = domain.SemanticAssertion{}
	}
	return &knowledge.ReviewContext{Item: it, Candidate: c, Proposals: []domain.SemanticProposal{p1, p2},
		Agreement: knowledge.Agreements([]domain.SemanticProposal{p1, p2})}
}

func respond(t *testing.T, req *Request, v any) *Response {
	t.Helper()
	b, _ := json.Marshal(v)
	return &Response{Format: ResponseFormat, ItemID: req.ItemID, PromptDigest: req.PromptDigest, Model: "claude-opus-5-5",
		ModelVersion: "claude-opus-5-5", CallID: "sess-1", StartedAt: t0.Add(time.Hour), DecidedAt: t0.Add(time.Hour + 5*time.Second), Output: b}
}

func TestBuildIsBlindAndSelfContained(t *testing.T) {
	rc := fixture(domain.QuestionConsequence)
	// related decisions: a human one on another item (shown), a proxy one (never shown),
	// a human one on this very item (never shown)
	rc.RelatedDecisions = []domain.ReviewDecision{
		{ReviewItemID: "ri-other", ReviewerKind: domain.ReviewerHuman, Action: domain.ActionReject, Reason: "HUMAN-OTHER", DecidedAt: t0},
		{ReviewItemID: "ri-other2", ReviewerKind: domain.ReviewerProxy, Action: domain.ActionAccept, Reason: "PROXY-OTHER", DecidedAt: t0},
		{ReviewItemID: rc.Item.ID, ReviewerKind: domain.ReviewerHuman, Action: domain.ActionAccept, Reason: "HUMAN-SAME", DecidedAt: t0},
	}
	req, err := Build(rc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	user := req.Request.Messages[0].Content
	for _, want := range []string{"What happens?", string(rc.Candidate.Evidence[0].ID), "[VERIFY] consequence", "P1", "P2", "HUMAN-OTHER", "AGREEMENT"} {
		if !strings.Contains(user, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	for _, banned := range []string{"glm", "opus", "some-env-label", "PROXY-OTHER", "HUMAN-SAME", "call-1"} {
		if strings.Contains(strings.ToLower(user), strings.ToLower(banned)) {
			t.Errorf("prompt leaks %q", banned)
		}
	}
	if req.PromptDigest != llm.PromptDigest(req.Request) || req.Request.Model != DefaultModel || req.HumanDecisionsShown != 1 {
		t.Errorf("request: %+v", req)
	}
	if len(req.Proposals) != 2 {
		t.Fatalf("authors: %+v", req.Proposals)
	}
	if !strings.Contains(req.Request.System, "CONSEQUENCE KINDS") {
		t.Error("system prompt lacks the vocabulary")
	}
	// the shadow option hides human decisions
	req2, _ := Build(rc, Options{NoHumanContext: true})
	if strings.Contains(req2.Request.Messages[0].Content, "HUMAN-OTHER") || req2.HumanDecisionsShown != 0 {
		t.Error("NoHumanContext still shows human decisions")
	}
	// eval data in evidence is refused
	rc.Candidate.Evidence[0].URI = "file:///repo/eval/cases/x/case.yaml"
	if _, err := Build(rc, Options{}); err == nil {
		t.Error("candidate citing eval data was prompted")
	}
}

// v2: the decision prompt states the correction's hard length limits (a v1
// correction was refused for a >400-character statement the model could not
// know was too long); v3 adds the citation rule (a v2 correction citing only
// validator evidence recorded nothing: the recorder keeps only upstream ids).
// The gate prompt has no correction and neither rule.
func TestPromptStatesCorrectionLimits(t *testing.T) {
	req, err := Build(fixture(domain.QuestionConsequence), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if req.PromptVersion != "proxy-review/v3" {
		t.Errorf("prompt version %s", req.PromptVersion)
	}
	for _, want := range []string{"at most 400 characters", "at most 600 characters", "at most 12 citations",
		"at least one upstream evidence id", "never invented"} {
		if !strings.Contains(req.Request.System, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	gate, err := Build(fixture(domain.QuestionEvidenceSufficiency), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gate.Request.System, "at most 400 characters") || strings.Contains(gate.Request.System, "at least one upstream evidence id") {
		t.Error("gate prompt states correction limits it cannot use")
	}
}

func TestDecisionAcceptRejectCorrect(t *testing.T) {
	rc := fixture(domain.QuestionConsequence)
	req, err := Build(rc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ev := string(req.InputEvidence[0])

	d, _, err := Decision(rc, req, respond(t, req, map[string]any{"action": "accept", "reason": "[" + ev + "] says so", "citations": []string{ev}, "confidence": "medium"}))
	if err != nil {
		t.Fatal(err)
	}
	if d.Action != domain.ActionAccept || d.Labels[0] != domain.LabelAccepted || d.Original == nil || d.ReviewerKind != domain.ReviewerProxy ||
		!d.StartedAt.Equal(t0.Add(time.Hour)) || d.ProxyProvenance.CallID != "sess-1" || len(d.ProxyProvenance.InputEvidence) == 0 {
		t.Errorf("accept: %+v", d)
	}

	d, _, err = Decision(rc, req, respond(t, req, map[string]any{"action": "reject", "reason": "misreads", "citations": []string{ev},
		"confidence": "low", "labels": []string{"wrong-consequence", "wrong-subject"}}))
	if err != nil {
		t.Fatal(err)
	}
	// wrong-subject names an aspect this question does not verify: dropped
	if d.Action != domain.ActionReject || len(d.Labels) != 2 || d.Labels[0] != domain.LabelRejected || d.Labels[1] != domain.LabelWrongConsequence {
		t.Errorf("reject labels: %v", d.Labels)
	}

	corr := map[string]any{"statement": "A changed default; it keeps working.",
		"consequence": map[string]any{"determination": "asserted", "kind": "behavior-change", "statement": "New certificates rotate keys."}}
	d, _, err = Decision(rc, req, respond(t, req, map[string]any{"action": "correct", "reason": "[" + ev + "] only changes a default",
		"citations": []string{ev}, "confidence": "medium", "correction": corr}))
	if err != nil {
		t.Fatal(err)
	}
	c := d.Corrected
	if d.Action != domain.ActionCorrect || c == nil || c.Consequence.Kind != domain.ConsequenceBehaviorChange || c.Consequence.ExposedClass != domain.ImpactReviewRequired {
		t.Fatalf("correct: %+v", d)
	}
	if c.Subject.Key() != rc.Item.Proposed.Subject.Key() || c.Applicability == nil {
		t.Error("the correction dropped or changed aspects the question does not verify")
	}
	has := map[domain.FeedbackLabel]bool{}
	for _, l := range d.Labels {
		has[l] = true
	}
	if !has[domain.LabelCorrected] || !has[domain.LabelWrongConsequence] || !has[domain.LabelWrongClassification] {
		t.Errorf("derived labels: %v", d.Labels)
	}

	// a correction that changes nothing is refused
	same := map[string]any{"statement": "same", "consequence": map[string]any{"determination": "asserted", "kind": "setting-ignored", "statement": "x"}}
	if _, _, err := Decision(rc, req, respond(t, req, map[string]any{"action": "correct", "reason": "r", "citations": []string{ev}, "confidence": "medium", "correction": same})); err == nil {
		t.Error("a no-op correction was accepted")
	}
	// an enum-invalid correction is refused by the schema
	badKind := map[string]any{"statement": "s", "consequence": map[string]any{"determination": "asserted", "kind": "catastrophe", "statement": "x"}}
	if _, _, err := Decision(rc, req, respond(t, req, map[string]any{"action": "correct", "reason": "r", "citations": []string{ev}, "confidence": "medium", "correction": badKind})); err == nil {
		t.Error("an enum-invalid correction was accepted")
	}
	// high confidence is not offered
	if _, _, err := Decision(rc, req, respond(t, req, map[string]any{"action": "accept", "reason": "r", "citations": []string{ev}, "confidence": "high"})); err == nil {
		t.Error("a high-confidence proxy verdict was accepted")
	}
}

func TestDecisionRefusesStaleOrForeignAnswers(t *testing.T) {
	rc := fixture(domain.QuestionConsequence)
	req, _ := Build(rc, Options{})
	ev := string(req.InputEvidence[0])
	ok := map[string]any{"action": "accept", "reason": "r", "citations": []string{ev}, "confidence": "medium"}

	r := respond(t, req, ok)
	r.PromptDigest = "sha256:other"
	if _, _, err := Decision(rc, req, r); err == nil {
		t.Error("a response to another prompt was accepted")
	}
	r = respond(t, req, ok)
	r.CallID = ""
	if _, _, err := Decision(rc, req, r); err == nil {
		t.Error("a response without a call id was accepted")
	}
	edited := *req
	edited.Request.System += " (edited)"
	if _, _, err := Decision(rc, &edited, respond(t, &edited, ok)); err == nil {
		t.Error("an edited request was accepted")
	}
	decided := *rc
	decided.Item.Status = domain.ReviewDecided
	if _, _, err := Decision(&decided, req, respond(t, req, ok)); err == nil {
		t.Error("a decided item was decided again")
	}
	changed := *rc
	changed.Item.Proposed.Consequence = &domain.Consequence{Kind: domain.ConsequenceNone, ExposedClass: domain.ImpactInformational, Statement: "z"}
	if _, _, err := Decision(&changed, req, respond(t, req, ok)); err == nil {
		t.Error("a verdict on a changed item was accepted")
	}
}

func TestEvidenceSufficiencyGate(t *testing.T) {
	rc := fixture(domain.QuestionEvidenceSufficiency)
	req, err := Build(rc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(req.Request.System, "CONSEQUENCE KINDS") || !strings.Contains(req.Request.System, "EVIDENCE-SUFFICIENCY") {
		t.Error("gate prompt is not the gate prompt")
	}
	ev := string(req.InputEvidence[0])
	d, v, err := Decision(rc, req, respond(t, req, map[string]any{"action": ActionEvidenceSufficient, "reason": "[" + ev + "] names the key", "citations": []string{ev}, "confidence": "medium"}))
	if err != nil {
		t.Fatal(err)
	}
	if d.Action != domain.ActionDefer || !strings.HasPrefix(d.Reason, "evidence sufficient") || v.Action != ActionEvidenceSufficient || len(d.Labels) != 0 {
		t.Errorf("sufficient: %+v", d)
	}
	d, _, err = Decision(rc, req, respond(t, req, map[string]any{"action": "need-more-evidence", "reason": "vague", "citations": []string{}, "confidence": "low"}))
	if err != nil || d.Action != domain.ActionNeedMoreEvidence || d.Labels[0] != domain.LabelInsufficientEvidence {
		t.Errorf("insufficient: %+v %v", d, err)
	}
	// a gate verifies nothing: accept is not on offer
	if _, _, err := Decision(rc, req, respond(t, req, map[string]any{"action": "accept", "reason": "r", "citations": []string{}, "confidence": "low"})); err == nil {
		t.Error("accept on an evidence-sufficiency gate was accepted")
	}
}

func TestReportSplitsByProposer(t *testing.T) {
	rc := fixture(domain.QuestionConsequence)
	req, _ := Build(rc, Options{})
	ev := string(req.InputEvidence[0])
	d, _, err := Decision(rc, req, respond(t, req, map[string]any{"action": "accept", "reason": "r", "citations": []string{ev}, "confidence": "medium"}))
	if err != nil {
		t.Fatal(err)
	}
	snap := &knowledge.Snapshot{Candidates: []domain.SemanticCandidate{rc.Candidate}, Proposals: rc.Proposals,
		ReviewItems: []domain.ReviewItem{rc.Item}, Decisions: []domain.ReviewDecision{d}}
	var e LedgerEntry
	e.Authors(req, "claude-opus-5-5")
	if !e.SelfModel || !e.SelfFamily {
		t.Errorf("self flags: %+v", e)
	}
	e.Outcome, e.CostUSD, e.DurationMs, e.StartedAt, e.DecidedAt = OutcomeDecided, 0.1, 5000, d.StartedAt, d.DecidedAt
	r := BuildReport(snap, []LedgerEntry{e, {ItemID: "ri-x", Outcome: OutcomeFailed, Stage: StageCall, Error: "boom"}})
	// the proposed assertion is glm's (setting-ignored), not the proxy's family
	if r.ByProposer["other-family only"][domain.ActionAccept] != 1 || r.Decisions != 1 || r.Failures[StageCall] != 1 {
		t.Errorf("report: %+v", r)
	}
	rel := map[string]string{}
	for _, m := range r.Models {
		rel[m.Model] = m.Relation
	}
	if rel["claude-opus-5-5"] != "self-model" || rel["glm-5.3"] != "other-family" {
		t.Errorf("relations: %v", rel)
	}
}

func TestCompareShadow(t *testing.T) {
	rc := fixture(domain.QuestionConsequence)
	req, _ := Build(rc, Options{})
	ev := string(req.InputEvidence[0])
	pd, _, err := Decision(rc, req, respond(t, req, map[string]any{"action": "accept", "reason": "r", "citations": []string{ev}, "confidence": "medium"}))
	if err != nil {
		t.Fatal(err)
	}
	orig := rc.Item.Proposed
	corr := orig
	corr.Consequence = &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired, Statement: "y"}
	hd := domain.ReviewDecision{ReviewItemID: rc.Item.ID, Action: domain.ActionCorrect, Original: &orig, Corrected: &corr,
		Reviewer: "po", ReviewerKind: domain.ReviewerHuman, DecidedAt: t0.Add(2 * time.Hour)}
	copied := domain.ReviewDecision{ReviewItemID: "ri-copied", Action: domain.ActionAccept, Reviewer: "proxy:x", ReviewerKind: domain.ReviewerProxy, DecidedAt: t0}
	real := &knowledge.Snapshot{ReviewItems: []domain.ReviewItem{rc.Item}, Decisions: []domain.ReviewDecision{hd, copied}}
	shadow := &knowledge.Snapshot{ReviewItems: []domain.ReviewItem{rc.Item}, Decisions: []domain.ReviewDecision{pd, copied}}
	a := CompareShadow(real, shadow)
	if a.Pairs != 1 || a.SameAction != 0 || a.SameOutcome != 1 || a.ShadowOnly != 0 {
		t.Fatalf("agreement: %+v", a)
	}
	if c := a.ByAspect[domain.AspectConsequence]; c == nil || c.N != 1 || c.Agree != 0 {
		t.Errorf("aspect agreement: %+v", a.ByAspect)
	}
	if a.Confusion["correct"]["accept"] != 1 {
		t.Errorf("confusion: %v", a.Confusion)
	}
}

// contract-5: a correction of the consequence prose only is recorded as
// [corrected, improved-statement] — run-1/run-2 refused 37 of them because the
// proxy's own pre-check demanded a digest change.
func TestProseOnlyCorrectionIsRecorded(t *testing.T) {
	rc := fixture(domain.QuestionConsequence)
	req, _ := Build(rc, Options{})
	ev := string(req.InputEvidence[0])
	corr := map[string]any{"statement": "Same kind, better wording.",
		"consequence": map[string]any{"determination": "asserted", "kind": "setting-ignored", "statement": "The rotationPolicy value is no longer honoured.", "remediation": "Set it explicitly."}}
	d, _, err := Decision(rc, req, respond(t, req, map[string]any{"action": "correct", "reason": "[" + ev + "] says it differently",
		"citations": []string{ev}, "confidence": "medium", "labels": []string{"wrong-consequence"}, "correction": corr}))
	if err != nil {
		t.Fatal(err)
	}
	if d.Corrected.Digest() != rc.Item.Proposed.Digest() || len(d.Labels) != 2 || d.Labels[0] != domain.LabelCorrected || d.Labels[1] != domain.LabelImprovedStatement {
		t.Errorf("prose-only correction: labels %v", d.Labels)
	}
	if d.ProxyProvenance.Provider != "anthropic" {
		t.Errorf("provider: %+v", d.ProxyProvenance)
	}
}

func TestSectionContext(t *testing.T) {
	rc := fixture(domain.QuestionConsequence)
	cited := rc.Candidate.Evidence[0]
	other := domain.NewEvidence(domain.EvidenceDocument, "guide", "https://example.test/upgrading.md", "L40", "rotation.policy now defaults to Always; set Never to keep the old behaviour.", "sha256:g", t0)
	n := func(id, section string, role domain.SourceRole, text string, ev domain.EvidenceID) domain.NoteItem {
		return domain.NoteItem{ID: id, Release: "v1.18.0", SourceID: string(role), Role: role, Section: section, Text: text, Evidence: []domain.EvidenceID{ev}}
	}
	sib := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example.test/notes.md", "L2", "Unrelated sibling item.", "sha256:d", t0)
	rel := &domain.Release{Product: "cert-manager", Evidence: []domain.Evidence{cited, other, sib},
		Notes: []domain.NoteItem{
			n("note-1", "Breaking", domain.RoleReleaseNotes, "The default of rotationPolicy is now Always.", cited.ID),
			n("note-2", "Breaking › Details", domain.RoleReleaseNotes, "SIBLING-IN-SECTION", sib.ID),
			n("note-3", "Other", domain.RoleReleaseNotes, "OTHER-SECTION", sib.ID),
			n("note-4", "Upgrading to 1.18", domain.RoleUpgradeGuide, "The policy key: rotation.policy now defaults to Always.", other.ID),
		}}
	src := func(p domain.ProductID, v string) (*domain.Release, error) { return rel, nil }
	req, err := Build(rc, Options{Releases: src})
	if err != nil {
		t.Fatal(err)
	}
	user := req.Request.Messages[0].Content
	for _, want := range []string{"UPSTREAM SECTION CONTEXT", "SIBLING-IN-SECTION", "► [" + string(cited.ID) + "]", "Upgrading to 1.18", string(other.ID)} {
		if !strings.Contains(user, want) {
			t.Errorf("section context lacks %q", want)
		}
	}
	if strings.Contains(user, "OTHER-SECTION") {
		t.Error("an unrelated section was shown")
	}
	if len(req.Sections) != 2 || req.Sections[0].Why != "cited" || !strings.HasPrefix(req.Sections[1].Why, "upgrade-guide mentions") {
		t.Errorf("section refs: %+v", req.Sections)
	}
	shown := map[domain.EvidenceID]bool{}
	for _, id := range req.InputEvidence {
		shown[id] = true
	}
	if !shown[other.ID] {
		t.Error("section evidence is not in inputEvidence (not citable)")
	}
	// a correction citing only section-context evidence is refused (it must cite candidate evidence)
	corr := map[string]any{"statement": "s", "consequence": map[string]any{"determination": "asserted", "kind": "behavior-change", "statement": "keeps working"}}
	if _, _, err := Decision(rc, req, respond(t, req, map[string]any{"action": "correct", "reason": "r", "citations": []string{string(other.ID)}, "confidence": "medium", "correction": corr})); err == nil {
		t.Error("a correction without candidate evidence was recorded")
	}
	// without a release source the prompt is the plain one
	plain, _ := Build(rc, Options{})
	if strings.Contains(plain.Request.Messages[0].Content, "UPSTREAM SECTION CONTEXT") || len(plain.Sections) != 0 {
		t.Error("section context without a release source")
	}
}

// v3: literals are shown decoded — the stored "\"Never\"" is the string Never,
// and printing it raw made the proxy "correct" quotes that are not there.
func TestPromptShowsLiteralsDecoded(t *testing.T) {
	rc := fixture(domain.QuestionApplicability)
	rc.Item.Proposed.Applicability = &domain.Applicability{Exposure: domain.Condition{Op: domain.OpValuesKey, Path: "rotation.policy", State: domain.StateEquals, Values: []string{`"Never"`, `30`}}}
	req, err := Build(rc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	user := req.Request.Messages[0].Content
	for _, want := range []string{`"before":"Never"`, `"after":"Always"`, `"values":["Never",30]`} {
		if !strings.Contains(user, want) {
			t.Errorf("prompt lacks decoded %s", want)
		}
	}
	if strings.Contains(user, `\"Never\"`) {
		t.Error("prompt still shows an encoded literal")
	}
}

// proxy-4: a re-review may decide an item closed as need-more-evidence or
// defer, but only while it still has the status the prompt was built for.
func TestReReviewOfClosedItems(t *testing.T) {
	rc := fixture(domain.QuestionConsequence)
	rc.Item.Status = domain.ReviewNeedsEvidence
	req, err := Build(rc, Options{})
	if err != nil || req.ItemStatus != domain.ReviewNeedsEvidence {
		t.Fatalf("request: %v %+v", err, req)
	}
	ev := string(req.InputEvidence[0])
	ok := map[string]any{"action": "accept", "reason": "r", "citations": []string{ev}, "confidence": "medium"}
	if _, _, err := Decision(rc, req, respond(t, req, ok)); err != nil {
		t.Errorf("re-review of a needs-evidence item: %v", err)
	}
	moved := *rc
	moved.Item.Status = domain.ReviewPending
	if _, _, err := Decision(&moved, req, respond(t, req, ok)); err == nil {
		t.Error("a verdict was recorded on an item whose status changed since the prompt")
	}
	decided := *rc
	decided.Item.Status = domain.ReviewDecided
	dreq := *req
	dreq.ItemStatus = domain.ReviewDecided
	if _, _, err := Decision(&decided, &dreq, respond(t, &dreq, ok)); err == nil {
		t.Error("a decided item was reviewed")
	}
}
