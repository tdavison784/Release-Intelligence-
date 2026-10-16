package investigate

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestConfigDigestIsIdentity(t *testing.T) {
	a := Config{Policy: "classify.policy@v1", Bound: domain.DefaultTraversalBound, Agent: AgentConfig{Enabled: true, PromptVersion: "investigate-agent/v1", Budget: DefaultAgentBudget}}
	b := a
	if a.Digest() != b.Digest() {
		t.Fatal("equal configs must digest equally")
	}
	b.Agent.Model = "claude-opus-5-5"
	if a.Digest() == b.Digest() {
		t.Fatal("a different model is a different investigation")
	}
	r := Request{Config: a}
	if r.ID("bnd-1") != domain.InvestigationID("bnd-1", a.Digest()) {
		t.Fatal("Request.ID must be the domain investigation id")
	}
}

func TestIdempotencyKeys(t *testing.T) {
	if ChangeKey("cert-manager", "v1.17.0", "v1.18.0", "d") != ChangeKey("cert-manager", "v1.17.0", "v1.18.0", "d") {
		t.Fatal("change key must be deterministic")
	}
	if ChangeKey("cert-manager", "v1.17.0", "v1.18.0", "d1") == ChangeKey("cert-manager", "v1.17.0", "v1.18.0", "d2") {
		t.Fatal("a definition change is a new collection")
	}
	in := EnvironmentInputs{Name: "prod", Repo: "./gitops"}
	if SnapshotKey(in) != SnapshotKey(in) {
		t.Fatal("snapshot key must be deterministic")
	}
}

func TestErrorClasses(t *testing.T) {
	cases := map[string]struct {
		err  error
		want ErrorClass
	}{
		"transient wrapped": {TransientError(domain.StageCollectChange, errors.New("429")), Transient},
		"permanent wrapped": {fmt.Errorf("ctx: %w", PermanentError(domain.StageBuildSnapshot, errors.New("bad repo"))), Permanent},
		"not found":         {fmt.Errorf("x: %w", ErrNotFound), Permanent},
		"rejected":          {ErrRejected, Permanent},
		"cancelled":         {context.Canceled, Permanent},
		"unclassified":      {errors.New("boom"), Transient},
	}
	for name, c := range cases {
		if got := ClassOf(c.err); got != c.want {
			t.Errorf("%s: ClassOf = %s, want %s", name, got, c.want)
		}
	}
}

func TestQuoteOccurs(t *testing.T) {
	text := "spec:\n  privateKey:\n    rotationPolicy:   Never\n"
	if !QuoteOccurs(text, "rotationPolicy: Never") {
		t.Error("whitespace-normalised quote must match")
	}
	if QuoteOccurs(text, "rotationPolicy: Always") || QuoteOccurs(text, "   ") {
		t.Error("a quote that does not occur (or is blank) must not match")
	}
}

func TestToolNamesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, n := range ToolNames {
		if seen[n] {
			t.Fatalf("duplicate tool %s", n)
		}
		seen[n] = true
	}
	if len(ToolNames) != 12 {
		t.Fatalf("tool surface changed: %v", ToolNames)
	}
}
