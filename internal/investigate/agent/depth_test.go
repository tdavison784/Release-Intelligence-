package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

func rejected(t *testing.T, s *Session, tool string, args any, want string) {
	t.Helper()
	if _, err := call(t, s, tool, args); !errors.Is(err, investigate.ErrRejected) || !strings.Contains(err.Error(), want) {
		t.Fatalf("%s: got %v, want a rejection mentioning %q", tool, err, want)
	}
}

func TestDepthExaminedAndUnsharedCitation(t *testing.T) {
	f := newFixture(t)
	s := f.v3session(t, f.input(investigate.AgentConfig{}))
	values := []map[string]any{{"artifact": f.values.ID, "quote": "rotationPolicy: Never"}}
	notes := []map[string]any{{"artifact": f.notes.ID, "quote": "The values key `legacy.enabled` was removed."}}
	explains := func(item string, cites []map[string]any) map[string]any {
		return map[string]any{"changeItem": item, "stance": "explains", "statement": "reasoning about " + item, "citations": cites}
	}
	// D1a: no get_change_item yet.
	rejected(t, s, investigate.ToolRecordAssertion, explains(f.behaviour, values), DepthRuleExamined)
	ok(t, s, investigate.ToolGetChangeItem, map[string]any{"id": f.behaviour})
	ok(t, s, investigate.ToolRecordAssertion, explains(f.behaviour, values))
	// D1b: values2 retrieved, but its only citation is the behaviour item's —
	// even with a different locator.
	ok(t, s, investigate.ToolGetChangeItem, map[string]any{"id": f.values2})
	relocated := []map[string]any{{"artifact": f.values.ID, "locator": "L2", "quote": "rotationPolicy:   Never"}}
	rejected(t, s, investigate.ToolRecordAssertion, explains(f.values2, relocated), DepthRuleUnique)
	// One specific citation is enough, next to a shared one.
	ok(t, s, investigate.ToolRecordAssertion, explains(f.values2, append(relocated, notes...)))
	// Concerns and mitigation claims are not subject to D1.
	ok(t, s, investigate.ToolRecordAssertion, map[string]any{"changeItem": f.values2, "stance": "claims-mitigation", "statement": "unset", "citations": values})
}

func TestDepthAtSubmission(t *testing.T) {
	f := newFixture(t)
	s := f.v3session(t, f.input(investigate.AgentConfig{}), &fakeOverride{f: f})
	values := []map[string]any{{"artifact": f.values.ID, "quote": "rotationPolicy: Never"}}
	notes := []map[string]any{{"artifact": f.notes.ID, "quote": "The values key `legacy.enabled` was removed."}}
	entry := func(item, disp string, cites []map[string]any, extra ...string) map[string]any {
		e := map[string]any{"changeItem": item, "disposition": disp, "summary": disp + " " + item, "citations": cites}
		if len(extra) > 0 {
			e["noValidationReason"] = extra[0]
		}
		return e
	}
	// examined-no-concern before get_change_item.
	rejected(t, s, investigate.ToolSubmitFindings, submitArgs(entry(f.behaviour, "not-examined", nil), entry(f.values2, "examined-no-concern", notes)), DepthRuleExamined)
	ok(t, s, investigate.ToolGetChangeItem, map[string]any{"id": f.values2})
	ok(t, s, investigate.ToolGetChangeItem, map[string]any{"id": f.behaviour})
	// Two entries of one submission sharing their only citation.
	rejected(t, s, investigate.ToolSubmitFindings, submitArgs(entry(f.behaviour, "explains", values), entry(f.values2, "examined-no-concern", values)), DepthRuleUnique)
	// D2: a concern on the controller-behaviour item, no request, no reason.
	rejected(t, s, investigate.ToolSubmitFindings, submitArgs(entry(f.behaviour, "raises-concern", values), entry(f.values2, "examined-no-concern", notes)), DepthRuleConcern)
	// noValidationReason only on a concern; and it satisfies D2.
	rejected(t, s, investigate.ToolSubmitFindings, submitArgs(entry(f.behaviour, "explains", values, "x"), entry(f.values2, "examined-no-concern", notes)), "raises-concern only")
	// A concern recorded during the session counts too.
	concern(t, s, f, "unpinned default", nil)
	rejected(t, s, investigate.ToolSubmitFindings, submitArgs(entry(f.behaviour, "raises-concern", values, "no validator inspects controller defaults"),
		entry(f.values2, "examined-no-concern", notes)), DepthRuleConcern)
	// A request_validation for the item satisfies D2 for both concerns.
	ok(t, s, investigate.ToolRequestValidation, map[string]any{"changeItem": f.behaviour, "kind": "override-inspection"})
	ok(t, s, investigate.ToolSubmitFindings, submitArgs(entry(f.behaviour, "raises-concern", values), entry(f.values2, "examined-no-concern", notes)))
	out, err := s.Outcome(context.Background(), f.st, DriverScripted, investigate.DriverResult{Model: "test-model", ModelVersion: "v", Stop: "submitted"})
	must(t, err)
	c := out.Run.Coverage
	if len(c) != 2 || !c[0].Examined || c[0].Inspections != 1 || c[0].ValidationRequests != 1 || !c[1].Examined || c[1].ValidationRequests != 0 {
		t.Fatalf("depth in coverage: %+v", c)
	}
	if err := runRecord(t, f, out).ValidateAgainst(f.b); err != nil {
		t.Fatal(err)
	}
}

func TestDepthReasonSatisfiesConcernRule(t *testing.T) {
	f := newFixture(t)
	s := f.v3session(t, f.input(investigate.AgentConfig{}))
	values := []map[string]any{{"artifact": f.values.ID, "quote": "rotationPolicy: Never"}}
	ok(t, s, investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "raises-concern", "statement": "default flips",
		"citations": values, "noValidationReason": "no validator is configured for this investigation"})
	ok(t, s, investigate.ToolSubmitFindings, submitArgs(
		map[string]any{"changeItem": f.behaviour, "disposition": "not-examined", "summary": "concern recorded"},
		map[string]any{"changeItem": f.values2, "disposition": "not-examined", "summary": "budget"}))
	out, err := s.Outcome(context.Background(), f.st, DriverScripted, investigate.DriverResult{Model: "m", ModelVersion: "v", Stop: "submitted"})
	must(t, err)
	if out.Assertions[0].NoValidationReason == "" || out.Run.Status != domain.AgentSubmitted {
		t.Fatalf("reason: %+v", out.Assertions)
	}
	// v2 has no depth rules and takes no reason.
	s2 := f.v2session(t, f.input(investigate.AgentConfig{}))
	rejected(t, s2, investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "raises-concern", "statement": "x",
		"citations": values, "noValidationReason": "y"}, "takes no noValidationReason")
	ok(t, s2, investigate.ToolRecordAssertion, map[string]any{"changeItem": f.behaviour, "stance": "explains", "statement": "no get_change_item needed in v2", "citations": values})
}
