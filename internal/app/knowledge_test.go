package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Hand-built records in the contract's own shapes (DESIGN.md §2): a fact on
// cert-manager's rotationPolicy whose subject/change/applicability rest on a
// validation and whose consequence rests on a review decision.

var kt0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func kStr(s string) *string { return &s }

func kEvidence() domain.Evidence {
	return domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example/upgrading-1.17-1.18.md", "L10-L12",
		"The default rotationPolicy is now Always", "sha256:doc", kt0)
}

func kAssertion() domain.SemanticAssertion {
	cert := func(st domain.FieldState) domain.Condition {
		return domain.Condition{Op: domain.OpResource, Group: "cert-manager.io", Kind: "Certificate",
			Of: []domain.Condition{{Op: domain.OpField, Path: "spec.privateKey.rotationPolicy", State: st}}}
	}
	overlap := cert(domain.StateSet)
	return domain.SemanticAssertion{
		Subject:       &domain.Subject{Family: domain.SubjectCRDField, Product: "cert-manager", Group: "cert-manager.io", Kind: "Certificate", Path: "spec.privateKey.rotationPolicy"},
		Change:        &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: kStr(`"Never"`), After: kStr(`"Always"`)},
		Applicability: &domain.Applicability{Exposure: cert(domain.StateUnset), Overlap: &overlap},
		Consequence: &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired,
			Statement: "private keys are regenerated on every renewal"},
		Statement: "Certificate.spec.privateKey.rotationPolicy default Never → Always",
	}
}

// kRecords returns candidate, validation, item, decision (by the given
// reviewer kind) and a fact claiming the consequence at claimed.
func kRecords(t *testing.T, reviewer domain.ReviewerKind, claimed domain.VerificationLevel) []any {
	t.Helper()
	ev := kEvidence()
	a := domain.ChangeAnchor{Release: "v1.18.0", EvidenceKeys: []string{domain.EvidenceKey(ev)}, StatementKeys: []string{domain.StatementKey(ev)}, ChangeIDs: []string{"chg-rot"}}
	members := []domain.CandidateMember{{ChangeID: "chg-rot", Anchor: &a}}
	c := domain.SemanticCandidate{
		ID: domain.CandidateID("cert-manager", a.Release, members), Product: "cert-manager", Release: a.Release,
		Members: members, Grouping: "single", Category: domain.CategoryConfiguration, Title: "The default rotationPolicy is now Always",
		Evidence: []domain.Evidence{ev}, Producer: "semantic.candidates@v1", CreatedAt: kt0,
	}
	v := domain.ValidationResult{CandidateID: c.ID, Validator: "semvalidate.crd@v1", Assertion: kAssertion(),
		Checks: []domain.AspectCheck{
			{Aspect: domain.AspectSubject, Outcome: domain.OutcomeConfirmed, Rule: "crd-schema:field"},
			{Aspect: domain.AspectChange, Outcome: domain.OutcomeConfirmed, Rule: "crd-schema:default"},
			{Aspect: domain.AspectApplicability, Outcome: domain.OutcomeConfirmed, Rule: "canonical:crd-field/default-changed"},
		},
		Evidence: []domain.Evidence{ev}, CheckedAt: kt0}
	v.ID = domain.ValidationID(v.CandidateID, v.Validator, v.Assertion)
	item := domain.ReviewItem{CandidateID: c.ID, Product: c.Product, Release: c.Release, QuestionType: domain.QuestionConsequence,
		Question: "If a Certificate with rotationPolicy unset does nothing, what happens?", Proposed: kAssertion(),
		Routing: domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityNormal, Signals: []domain.RoutingSignal{domain.SignalSingleModel}},
		Status:  domain.ReviewDecided, CreatedAt: kt0, Context: &domain.EnvironmentContext{Label: "cert-manager-1.17-1.18"}}
	item.ID = domain.ReviewItemID(item.CandidateID, item.QuestionType, item.Proposed)
	orig := item.Proposed
	d := domain.ReviewDecision{ReviewItemID: item.ID, Action: domain.ActionAccept, Labels: []domain.FeedbackLabel{domain.LabelAccepted},
		Original: &orig, Reviewer: "engineer-1", ReviewerKind: reviewer, StartedAt: kt0, DecidedAt: kt0.Add(90 * time.Second)}
	if reviewer == domain.ReviewerProxy {
		at := kt0
		d.Reviewer = "claude-opus-5-5"
		d.ProxyProvenance = &domain.Provenance{Method: domain.MethodAI, Producer: "knowledge.proxy@v1", Confidence: domain.ConfidenceMedium,
			Model: "claude-opus-5-5", ModelVersion: "claude-opus-5-5", PromptVersion: "proxy/v1", PromptDigest: "sha256:p",
			InputEvidence: []domain.EvidenceID{ev.ID}, GeneratedAt: &at, CallID: "call-proxy"}
	}
	d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	f := domain.VerifiedFact{Product: "cert-manager", Release: "v1.18.0", Anchors: []domain.ChangeAnchor{a}, Candidates: []string{c.ID},
		Assertion: kAssertion(), Evidence: []domain.Evidence{ev}, Status: domain.FactActive, CreatedAt: kt0,
		Verification: []domain.AspectVerification{
			{Aspect: domain.AspectSubject, Level: domain.VerifiedDeterministic, Basis: []string{v.ID}},
			{Aspect: domain.AspectChange, Level: domain.VerifiedDeterministic, Basis: []string{v.ID}},
			{Aspect: domain.AspectApplicability, Level: domain.VerifiedDeterministic, Basis: []string{v.ID}},
			{Aspect: domain.AspectConsequence, Level: claimed, Basis: []string{d.ID}},
		}}
	f.ID = domain.VerifiedFactID(f.Product, f.Release, f.Assertion)
	return []any{c, v, item, d, f}
}

func writeKnowledge(t *testing.T, records ...any) string {
	t.Helper()
	dir := t.TempDir()
	for _, x := range records {
		r, err := domain.NewRecord(x)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.MarshalIndent(r, "", "  ")
		p := filepath.Join(dir, "cert-manager", "v1.18.0", string(r.Kind), r.ID()+".json")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadKnowledgeProvesEveryFact(t *testing.T) {
	recs := kRecords(t, domain.ReviewerHuman, domain.VerifiedHuman)
	for _, r := range recs {
		x, _ := domain.NewRecord(r)
		if err := x.Validate(); err != nil {
			t.Fatalf("fixture record %s invalid: %v", x.Kind, err)
		}
	}
	ks, err := LoadKnowledge(writeKnowledge(t, recs...))
	if err != nil {
		t.Fatal(err)
	}
	if len(ks.Facts) != 1 || len(ks.Warnings) != 0 {
		t.Fatalf("a proven human fact: facts=%d warnings=%v", len(ks.Facts), ks.Warnings)
	}
	if got := ks.Contexts[ks.Facts[0].ID]; len(got) != 1 || got[0] != "cert-manager-1.17-1.18" {
		t.Errorf("review contexts = %v", got)
	}

	// trap: a proxy decision dressed up as human verification
	ks, err = LoadKnowledge(writeKnowledge(t, kRecords(t, domain.ReviewerProxy, domain.VerifiedHuman)...))
	if err != nil {
		t.Fatal(err)
	}
	if len(ks.Facts) != 0 || !strings.Contains(strings.Join(ks.Warnings, "\n"), "fact refused") {
		t.Errorf("a proxy decision claimed as human must be refused: facts=%d warnings=%v", len(ks.Facts), ks.Warnings)
	}

	// trap: a fact whose decision record is missing (the basis does not resolve)
	recs = kRecords(t, domain.ReviewerHuman, domain.VerifiedHuman)
	ks, err = LoadKnowledge(writeKnowledge(t, recs[0], recs[1], recs[2], recs[4]))
	if err != nil {
		t.Fatal(err)
	}
	if len(ks.Facts) != 0 {
		t.Errorf("a fact without its decision record must be refused: %d facts", len(ks.Facts))
	}

	// an honestly proxy-labelled fact is loaded (the ladder caps it later)
	ks, err = LoadKnowledge(writeKnowledge(t, kRecords(t, domain.ReviewerProxy, domain.VerifiedProxy)...))
	if err != nil {
		t.Fatal(err)
	}
	if len(ks.Facts) != 1 || ks.Facts[0].Level() != domain.VerifiedProxy {
		t.Errorf("an honest proxy fact: %d facts", len(ks.Facts))
	}

	// a non-JSON file is a warning, never a fact
	dir := writeKnowledge(t)
	if err := os.WriteFile(filepath.Join(dir, "junk.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ks, err := LoadKnowledge(dir); err != nil || len(ks.Warnings) != 1 {
		t.Errorf("junk record: err=%v warnings=%v", err, ks.Warnings)
	}
	if _, err := LoadKnowledge(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing knowledge directory must fail")
	}
}

func TestParseVerificationLevel(t *testing.T) {
	if l, err := ParseVerificationLevel(""); err != nil || l != domain.VerifiedHuman {
		t.Errorf("default = %q %v, want human", l, err)
	}
	if _, err := ParseVerificationLevel("trusted"); err == nil {
		t.Error("an unknown level must fail")
	}
}
