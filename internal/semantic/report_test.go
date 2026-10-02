package semantic

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

func TestWriteRecordsAndSummary(t *testing.T) {
	cands := Candidates(rotationEdge(), t0)
	c := candidateFor(t, cands, "chg-guide")
	answer := fullAnswer(c, nil)
	mk := func(model string) knowledge.Proposer {
		f := &llm.Fake{Model: model, Respond: func(req llm.Request) (string, error) {
			if strings.Contains(req.Messages[0].Content, c.ID) {
				return answer, nil
			}
			return "not json", nil
		}}
		return NewLLMProposer(f, "anthropic", model, ProposerOptions{})
	}
	ps := []knowledge.Proposer{mk("claude-sonnet-5-5"), mk("glm-5.3-flash")}
	props, fails := ProposeAllWith(context.Background(), cands, ps, []domain.ProposalTask{domain.TaskFull}, ProposeOptions{Clock: func() time.Time { return t0 }})
	if len(props) != 2 || len(fails) != 2*(len(cands)-1) {
		t.Fatalf("props=%d fails=%d", len(props), len(fails))
	}
	dir := t.TempDir()
	if err := WriteRecords(dir, cands, props, fails); err != nil {
		t.Fatal(err)
	}
	for _, p := range props {
		b, err := os.ReadFile(filepath.Join(dir, "widget-operator", "1.1.0", "proposals", p.ID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var rec domain.KnowledgeRecord
		if err := json.Unmarshal(b, &rec); err != nil || rec.Validate() != nil || rec.Proposal == nil {
			t.Fatalf("record invalid: %v %v", err, rec.Validate())
		}
	}
	fs, _ := filepath.Glob(filepath.Join(dir, "widget-operator", "failures", "*.json"))
	if len(fs) != len(fails) {
		t.Errorf("failure files = %d, want %d", len(fs), len(fails))
	}
	// idempotent
	if err := WriteRecords(dir, cands, props, fails); err != nil {
		t.Fatal(err)
	}
	rep := Summarize(len(cands), props, fails)
	for _, a := range rep.Agreement {
		if a.Compared != 1 || a.AllAgree != 1 || a.SameFamily {
			t.Errorf("aspect %s: %+v (claude vs glm agree, independent families)", a.Aspect, a)
		}
	}
}

// PO-1: two separate calls of the SAME model are a comparable pair; the
// agreement report must count them (labelled model|model), not collapse
// them into one answer.
func TestSummaryCountsSameModelSeparateCalls(t *testing.T) {
	cands := Candidates(rotationEdge(), t0)
	c := candidateFor(t, cands, "chg-guide")
	f := &llm.Fake{Model: "claude-sonnet-5-5", Respond: func(llm.Request) (string, error) {
		return fullAnswer(c, nil), nil // identical answers: agreement must be 1/1
	}}
	ps := []knowledge.Proposer{ // same model asked twice: two separate calls
		NewLLMProposer(f, "anthropic", "claude-sonnet-5-5", ProposerOptions{}),
		NewLLMProposer(f, "anthropic", "claude-sonnet-5-5", ProposerOptions{}),
	}
	props, _ := ProposeAllWith(context.Background(), cands, ps, []domain.ProposalTask{domain.TaskFull}, ProposeOptions{Clock: func() time.Time { return t0 }})
	if len(props) != 2 || !domain.SeparateCalls(props[0], props[1]) {
		t.Fatalf("two separate calls expected: %d proposals", len(props))
	}
	rep := Summarize(len(cands), props, nil)
	for _, a := range rep.Agreement {
		if a.Compared != 1 || a.AllAgree != 1 {
			t.Errorf("aspect %s: %+v (two same-model calls both count)", a.Aspect, a)
		}
		if v := a.Pairwise["claude-sonnet-5-5|claude-sonnet-5-5"]; v != [2]int{1, 1} {
			t.Errorf("aspect %s: same-model pair = %v", a.Aspect, v)
		}
	}
}
