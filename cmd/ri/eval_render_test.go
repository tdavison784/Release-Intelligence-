package main

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/render"
)

func TestRenderEvalRecord(t *testing.T) {
	if got := caseOf(env.Inputs{ValuesFiles: []string{"eval/cases/kyverno-1.12-1.13/environment/values.yaml"}}); got != " [kyverno-1.12-1.13]" {
		t.Errorf("caseOf = %q", got)
	}
	ok := &render.Pair{Status: render.PairOK, Target: render.Target{ID: "values:x", ValuesComplete: true},
		FromResult: &render.Result{Status: render.StatusSucceeded}, ToResult: &render.Result{Status: render.StatusSucceeded},
		Diff: &render.DiffResult{Changes: []render.Change{{Class: render.RBACPermissionRemoved}}}}
	rejected := &render.Pair{Status: render.PairFailed, Target: render.Target{ID: "values:y", ValuesComplete: true},
		FromResult: &render.Result{Status: render.StatusSucceeded},
		ToResult:   &render.Result{Status: render.StatusFailed, Failure: &render.Failure{Reason: render.FailInvalidValues, Detail: "the chart rejects these values: foo.bar was removed"}},
		Failure:    &render.Failure{Reason: render.FailInvalidValues, Detail: "the chart rejects these values: foo.bar was removed"}}
	run := &app.ImpactRun{
		Render: &app.RenderDiffResult{Pairs: []*render.Pair{ok, rejected}, Correlations: map[string]render.Correlation{
			"values:x": {Changes: []render.Correlated{{Links: []render.ChangeLink{{ChangeID: "chg-1"}}}}, Documented: 1}}},
		Report: &domain.ImpactReport{Findings: []domain.ImpactFinding{
			{Classification: domain.ImpactUnknown, ChangeID: "chg-1"},
			{Classification: domain.ImpactActionRequired, ChangeID: "chg-2", Matches: []domain.ImpactMatch{{Subject: "foo.bar"}}},
		}},
	}
	r := &renderEval{}
	r.record("k", run)
	s := r.stats["k"]
	if s.Pairs != 2 || s.Succeeded != 1 || s.Failed != 1 || s.TargetRejects != 1 || s.Unknown != 1 || s.UnknownRestatedOK != 1 ||
		s.Action != 1 || s.ActionCorroborated != 1 || s.DecidedByRender != 0 || len(s.RestatedChanges) != 1 {
		t.Fatalf("stats: %+v", s)
	}
}
