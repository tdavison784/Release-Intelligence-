package agent

import (
	"context"
	"fmt"

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
		StatementKind: s.proseKind(), UntracedConsumers: untraced}
	for _, l := range []struct {
		name string
		in   []string
		out  *[]string
	}{{"consumers", in.Consumers, &a.Consumers}, {"inferred", in.Inferred, &a.Inferred}, {"unknown", in.Unknown, &a.Unknown}, {"recommendedTests", in.RecommendedTests, &a.RecommendedTests}} {
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
