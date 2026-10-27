package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Natural assessments and the completion check (prompt feature assessment;
// docs/phase6/decision-intel/PLAN-M1.md changes 6 and 7). Every closing entry
// except not-examined carries an assessment beside its verdict. The tool
// layer checks the shape and the verified entries (each needs citations that
// resolve: model memory is never evidence); everything else is unverified AI
// prose. It also adds what the session did not do: the consumers of the
// item's exposed objects no tool of the session returned become investigation
// gaps in Unknown. A not-affected verdict is rejected once while such
// consumers exist, so the model can trace them. Like the verdict, the
// assessment is never a policy input (verdict_policy_test.go).

const maxAssessmentList = 30

// assessmentView is what the model receives for a recorded assessment.
type assessmentView struct {
	ID                string   `json:"id"`
	ChangeItem        string   `json:"changeItem"`
	Verified          int      `json:"verified"`
	UntracedConsumers []string `json:"untracedConsumers,omitempty"`
}

// assessmentOf builds the assessment of a closing entry (caller holds s.mu,
// inside a tool implementation: nothing is committed here). verdict is the
// entry's verdict id, untraced the consumer keys the session never retrieved.
func (s *Session) assessmentOf(ctx context.Context, it domain.ChangeItem, in investigate.AssessmentInput, verdict string, untraced []string, at string) (domain.AgentAssessment, []domain.Evidence, error) {
	var a domain.AgentAssessment
	what, err := s.statement(at+".whatChanged", in.WhatChanged)
	if err != nil {
		return a, nil, err
	}
	mech, err := s.statement(at+".mechanism", in.Mechanism)
	if err != nil {
		return a, nil, err
	}
	impact, err := s.statement(at+".likelyImpact", in.LikelyImpact)
	if err != nil {
		return a, nil, err
	}
	a = domain.AgentAssessment{ChangeItem: it.ID, Verdict: verdict, WhatChanged: what, Mechanism: mech, LikelyImpact: impact,
		StatementKind: s.proseKind(), UntracedConsumers: untraced, Headline: trimmed(in.Headline), Scope: trimmed(in.Scope)}
	for _, l := range []struct {
		name string
		in   []string
		out  *[]string
	}{{"consumers", in.Consumers, &a.Consumers}, {"inferred", in.Inferred, &a.Inferred}, {"unknown", in.Unknown, &a.Unknown}, {"recommendedTests", in.RecommendedTests, &a.RecommendedTests},
		{"preconditions", in.Preconditions, &a.Preconditions}} {
		if len(l.in) > maxAssessmentList {
			return a, nil, rejectf("%s.%s: at most %d entries", at, l.name, maxAssessmentList)
		}
		if *l.out, err = textList(at+"."+l.name, l.in); err != nil {
			return a, nil, err
		}
	}
	if len(in.EffectiveConfiguration) > maxAssessmentList || len(in.Verified) > maxAssessmentList {
		return a, nil, rejectf("%s: at most %d effectiveConfiguration and verified entries", at, maxAssessmentList)
	}
	for i, x := range in.EffectiveConfiguration {
		setting := trimmed(x.Setting)
		switch {
		case setting == "":
			return a, nil, rejectf("%s.effectiveConfiguration[%d]: setting is required", at, i)
		case x.State != domain.AssessmentExplicit && x.State != domain.AssessmentInheritedDefault && x.State != domain.AssessmentUnknown:
			return a, nil, rejectf("%s.effectiveConfiguration[%d].state %q: want explicit, inherited-default or unknown", at, i, x.State)
		case len(setting)+len(x.Detail) > MaxStatement:
			return a, nil, rejectf("%s.effectiveConfiguration[%d] is too long (max %d bytes)", at, i, MaxStatement)
		}
		a.EffectiveConfiguration = append(a.EffectiveConfiguration, domain.AssessmentSetting{Setting: setting, State: x.State, Detail: trimmed(x.Detail)})
	}
	var evs, cited []domain.Evidence
	for i, v := range in.Verified {
		stmt, err := s.statement(fmt.Sprintf("%s.verified[%d].statement", at, i), v.Statement)
		if err != nil {
			return a, nil, err
		}
		if len(v.Citations) == 0 {
			return a, nil, rejectf("%s.verified[%d]: a verified statement needs a citation that resolves (move it to inferred if nothing in the evidence states it)", at, i)
		}
		cs, err := s.citations(ctx, v.Citations)
		if err != nil {
			return a, nil, fmt.Errorf("%s.verified[%d] (a verified statement needs citations that resolve; otherwise state it as inferred): %w", at, i, err)
		}
		a.Verified = append(a.Verified, domain.AssessmentFinding{Statement: stmt, Citations: evidenceIDs(cs)})
		evs = append(evs, cs...)
		cited = append(cited, cs...)
	}
	for _, k := range untraced {
		a.Unknown = append(a.Unknown, "investigation gap (recorded by the tool layer): consumer "+k+" of the item's exposed objects was not traced in this session")
	}
	a.Provenance = s.aiProvenance(append(evidenceIDs(cited), it.Evidence...))
	a.ID = domain.AgentAssessmentID(it.ID, a.WhatChanged, a.Mechanism, s.call.ID)
	if err := invalid(func() error { c := a; c.Provenance = placeholder(c.Provenance); return c.Validate() }()); err != nil {
		return domain.AgentAssessment{}, nil, err
	}
	return a, evs, nil
}

// scopedChecks checks an entry's scoped verdict fields before anything is
// recorded (prompt feature scopedVerdict; docs/phase6/decision-intel/briefs/
// scoped.md). Without the feature the fields are refused. With it, the
// headline is required, and a committed class (affected, not-affected) needs
// the scope it was reached in.
func (s *Session) scopedChecks(at string, in investigate.AssessmentInput, verdict *domain.AgentVerdict) error {
	if !s.cfg.Prompt.Features.ScopedVerdict {
		if trimmed(in.Headline) != "" || trimmed(in.Scope) != "" || len(in.Preconditions) > 0 {
			return rejectf("%s: this prompt version (%s) takes no headline, scope or preconditions", at, s.cfg.Prompt.Version)
		}
		return nil
	}
	if _, err := s.statement(at+".headline (one sentence: the class, the scope and the preconditions)", in.Headline); err != nil {
		return err
	}
	if len(in.Scope) > MaxStatement {
		return rejectf("%s.scope is %d bytes (max %d)", at, len(in.Scope), MaxStatement)
	}
	if len(in.Preconditions) > maxAssessmentList {
		return rejectf("%s.preconditions: at most %d entries", at, maxAssessmentList)
	}
	if verdict != nil && committed(verdict.Class) && trimmed(in.Scope) == "" {
		return rejectf("%s: the class %s commits within a declared scope; scope is required: what this session examined (e.g. the consumers the environment declares, the rendered objects, the cited upstream evidence)", at, verdict.Class)
	}
	return nil
}

// committed reports whether a class commits to an outcome.
func committed(c domain.FindingClass) bool {
	return c == domain.FindingAffected || c == domain.FindingNotAffected
}

// scopedCompletion is the completion check of a scoped verdict: a
// not-affected verdict is rejected once while in-scope unknowns remain, the
// model's own unknown entries and the consumers no tool of the session
// returned, and the rejection lists them. It shares the once-flag of the
// consumer completion check, so an item is rejected at most once.
func (s *Session) scopedCompletion(at, item string, in investigate.AssessmentInput, verdict *domain.AgentVerdict, untraced []string) error {
	if verdict == nil || verdict.Class != domain.FindingNotAffected || s.rejectedNA[item] {
		return nil
	}
	var unknown []string
	for _, u := range in.Unknown {
		if u = trimmed(u); u != "" {
			unknown = append(unknown, u)
		}
	}
	if len(unknown) == 0 && len(untraced) == 0 {
		return nil
	}
	s.rejectedNA[item] = true // recorded although the call is rejected: the check rejects once
	var open []string
	if len(unknown) > 0 {
		quoted := make([]string, len(unknown))
		for i, u := range unknown {
			quoted[i] = fmt.Sprintf("%q", u)
		}
		open = append(open, fmt.Sprintf("in-scope unknowns stated in unknown: %s", listKeys(quoted, 20)))
	}
	if len(untraced) > 0 {
		open = append(open, fmt.Sprintf("consumers of the item's exposed objects never retrieved in this session: %s (trace_consumers changeItem=%s, get_rendered_object, read_artifact)", listKeys(untraced, 20), item))
	}
	return rejectf("%s: a not-affected verdict needs every in-scope unknown resolved; %d remain: %s. Resolve them, or submit the class they leave (potentially-affected, insufficient-evidence). A runtime or operational fact the repository cannot state belongs in preconditions; an in-scope fact never does. A second submission is accepted, with each untraced consumer recorded as an investigation gap.",
		at, len(unknown)+len(untraced), strings.Join(open, "; "))
}
