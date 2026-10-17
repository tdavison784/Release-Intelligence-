package knowledge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// humanRule is narrowRule (promote_test.go) with any explicit mode as the
// mitigation: a rule no agent item of the fixture proposes.
func humanRule() domain.SemanticAssertion {
	a := narrowRule()
	a.Applicability.Overlap = ptrCond(widgets(retentionField(domain.StateSet)))
	return a
}

// narrowRuleFile is humanRule as a human-authored rule file.
func narrowRuleFile(t *testing.T) []byte { return ruleFileOf(t, humanRule()) }

func ruleFileOf(t *testing.T, a domain.SemanticAssertion) []byte {
	t.Helper()
	doc := map[string]any{
		"_provenance": "synthetic test rule",
		"statement":   a.Statement,
		"assertion": map[string]any{"subject": a.Subject, "change": a.Change, "applicability": a.Applicability, "consequence": a.Consequence,
			"release": map[string]any{"from": "1.9.4", "to": "2.0.0", "bounds": "(from, to]"}},
		"citations":             []string{string(gateEvidence().ID) + " (notes.md L3)"},
		"requestedVerification": "both arms at FROM and TO",
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const author = "commander-drafted (test model), label-aware; adopted by <reviewer> at promotion"

func TestIntakeRoundTrip(t *testing.T) {
	f := newGateFixture(t)
	data := narrowRuleFile(t)
	r, err := ParseRuleFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if r.Assertion.Digest() != humanRule().Digest() || r.Bounds != (domain.ReleaseBounds{From: "1.9.4", To: "2.0.0"}) {
		t.Fatalf("parsed rule differs: %+v", r)
	}
	res, err := Intake(context.Background(), f.s, r, IntakeOptions{Author: author, Now: t0.Add(4 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != IntakeTaken || res.Candidate != f.c.ID || res.FileDigest != domain.Digest(data) || res.RuleDigest != humanRule().Digest() || res.ItemExisted ||
		res.Hypothesis != domain.HumanAuthoredHypothesisID(res.FileDigest) {
		t.Fatalf("result: %+v", res)
	}
	snap, err := f.s.Load(context.Background(), Query{})
	if err != nil {
		t.Fatal(err)
	}
	var p *domain.SemanticProposal
	for i := range snap.Proposals {
		if snap.Proposals[i].ID == res.Proposal {
			p = &snap.Proposals[i]
		}
	}
	if p == nil || p.Provenance.Model != author || p.Provenance.PromptDigest != res.FileDigest || p.Provenance.Rule != "human-authored:"+res.Hypothesis ||
		p.Provider != HumanAuthoredProvider || p.Release == nil || *p.Release != r.Bounds || !HumanAuthored(p.Provenance.Rule) {
		t.Fatalf("proposal: %+v", p)
	}
	var item *domain.ReviewItem
	for i := range snap.ReviewItems {
		if snap.ReviewItems[i].ID == res.ReviewItem {
			item = &snap.ReviewItems[i]
		}
	}
	if item == nil || item.QuestionType != domain.QuestionFactReview || item.Proposed.Digest() != res.RuleDigest || !strings.Contains(item.Question, author) ||
		!strings.Contains(item.Question, res.FileDigest) {
		t.Fatalf("item: %+v", item)
	}
	if hyp, inv, ok := HypothesisSource(snap, *item); !ok || hyp != res.Hypothesis || inv != "" {
		t.Fatalf("hypothesis source: %q %q %v", hyp, inv, ok)
	}
	// an item that already proposes the same rule (here: the agent's) is
	// reused and reported; the gate still finds the new proposal by its rule
	nr, err := ParseRuleFile(ruleFileOf(t, narrowRule()))
	if err != nil {
		t.Fatal(err)
	}
	if x, err := Intake(context.Background(), f.s, nr, IntakeOptions{Author: author}); err != nil || !x.ItemExisted || x.ReviewItem != f.item.ID {
		t.Fatalf("intake of the agent's rule: %+v %v", x, err)
	}
	// idempotent
	again, err := Intake(context.Background(), f.s, r, IntakeOptions{Author: author, Now: t0.Add(time.Hour)})
	if err != nil || again.Status != IntakeExisting || again.ReviewItem != res.ReviewItem {
		t.Fatalf("second intake: %+v %v", again, err)
	}
	// routing never counts it: agreement over the candidate is unchanged
	var others []domain.SemanticProposal
	for _, x := range snap.Proposals {
		if x.ID != p.ID {
			others = append(others, x)
		}
	}
	with, without := RouteWith(nil, f.c, snap.Proposals, snap.Validations), RouteWith(nil, f.c, others, snap.Validations)
	a, _ := json.Marshal(with)
	b, _ := json.Marshal(without)
	if string(a) != string(b) {
		t.Fatalf("a human-authored proposal changed routing:\n%s\n%s", a, b)
	}
}

// The gate applies to a human-authored item exactly as to an agent one.
func TestIntakeItemIsGated(t *testing.T) {
	f := newGateFixture(t)
	r, err := ParseRuleFile(narrowRuleFile(t))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Intake(context.Background(), f.s, r, IntakeOptions{Author: author, Now: t0.Add(4 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	snap, _ := f.s.Load(context.Background(), Query{})
	var it domain.ReviewItem
	for _, x := range snap.ReviewItems {
		if x.ID == res.ReviewItem {
			it = x
		}
	}
	d := humanAccept(it, t0.Add(2*time.Hour))
	d.Supersedes = []string{f.broad.ID}
	_, err = f.decide(t, d)
	refused(t, err, "no proof record")

	proof := proofFor(it, t0.Add(90*time.Minute), domain.MethodPass, domain.MethodPass, gateBounds)
	proof.Hypothesis, proof.Investigation = res.Hypothesis, ""
	proof.ID = domain.ProofRecordID(proof)
	mustPut(t, f.s, proof)
	_, err = f.decide(t, humanAccept(it, t0.Add(2*time.Hour))) // no supersedes/coexists for the broad fact
	refused(t, err, f.broad.ID)
	_, err = f.decide(t, decision(it, "proxy", domain.ReviewerProxy, domain.ActionAccept, t0.Add(2*time.Hour)))
	refused(t, err, "only a human")
	out, err := f.decide(t, d)
	if err != nil || out[0].Fact == nil {
		t.Fatalf("human promotion with a confirmed proof refused: %v", err)
	}
}

func TestParseRuleFileRefusesWhatItCannotExpress(t *testing.T) {
	base := string(narrowRuleFile(t))
	for name, edit := range map[string][2]string{
		"unknown top-level field":   {`"statement":`, `"extra": 1, "statement":`},
		"unknown condition field":   {`"op": "field",`, `"op": "field", "negate": true,`},
		"other bounds shape":        {`"(from, to]"`, `"[from, to)"`},
		"bounds not increasing":     {`"from": "1.9.4"`, `"from": "2.1.0"`},
		"dropped by the grammar":    {`"kind": "Widget",`, `"kind": "Widget", "version": "",`},
		"citation without an id":    {string(gateEvidence().ID) + " (notes.md L3)", "notes.md L3"},
		"no requested verification": {`"requestedVerification": "both arms at FROM and TO"`, `"requestedVerification": " "`},
	} {
		if !strings.Contains(base, edit[0]) {
			t.Fatalf("%s: %q not in the base file", name, edit[0])
		}
		if _, err := ParseRuleFile([]byte(strings.Replace(base, edit[0], edit[1], 1))); err == nil {
			t.Errorf("%s: taken in", name)
		}
	}
	undecidable := humanRule()
	undecidable.Applicability.Overlap = ptrCond(domain.Condition{Op: domain.OpUndecidable, Reason: domain.UnknownEvidenceGap, Needed: "which consumers pin"})
	incomplete := humanRule()
	incomplete.Consequence = nil
	for name, a := range map[string]domain.SemanticAssertion{"undecidable condition": undecidable, "incomplete fact": incomplete} {
		if _, err := ParseRuleFile(ruleFileOf(t, a)); err == nil {
			t.Errorf("%s: taken in", name)
		}
	}
	// no candidate holds the citation
	r, err := ParseRuleFile([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Intake(context.Background(), NewFileStore(t.TempDir()), r, IntakeOptions{Author: author}); err == nil {
		t.Error("taken in without a candidate")
	}
	f := newGateFixture(t)
	if _, err := Intake(context.Background(), f.s, r, IntakeOptions{}); err == nil {
		t.Error("taken in without an author")
	}
}
