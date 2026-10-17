package run_test

import (
	"context"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/run"
	"github.com/tdavison784/release-intelligence/internal/investigate/run/runtest"
)

// Two runs on the same bundle and store that differ only in their config
// (agent off, then on — and the other order) are two investigations, each
// carried to the end with its own config. Before stage keys covered the refs
// a stage passes on, the second run's build-graph hit the first run's memo
// and every later stage continued the FIRST investigation: the second record
// stayed `running` and the agent setting of the second run was ignored.
func TestSecondRunWithOtherConfigIsItsOwnInvestigation(t *testing.T) {
	for _, order := range [][2]bool{{false, true}, {true, false}} {
		f := runtest.New(t)
		ag := &scriptedAgent{st: f.St}
		f.Deps.Investigator = ag
		approve := runtest.Human(investigate.ReviewApprove)
		var got [2]*run.Result
		for i, agentOn := range order {
			req := f.Request()
			req.Config.Agent.Enabled = agentOn
			res, err := runner(f, once(approve)).Run(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			got[i] = res
			inv, err := f.St.GetInvestigation(context.Background(), res.Investigation)
			if err != nil {
				t.Fatal(err)
			}
			checkOwnRun(t, inv, req, agentOn)
			// The record is the one a fresh store would produce for this request.
			solo := runtest.New(t)
			solo.Repo = f.Repo
			solo.Deps.Investigator = &scriptedAgent{st: solo.St}
			want, err := runner(solo, once(approve)).Run(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if res.Investigation != want.Investigation || res.RecordDigest != want.RecordDigest {
				t.Fatalf("order %v run %d: %s %s, a fresh store gives %s %s", order, i, res.Investigation, res.RecordDigest, want.Investigation, want.RecordDigest)
			}
		}
		if got[0].Investigation == got[1].Investigation || got[0].Bundle != got[1].Bundle {
			t.Fatalf("order %v: investigations %s / %s on bundles %s / %s, want two investigations of one bundle",
				order, got[0].Investigation, got[1].Investigation, got[0].Bundle, got[1].Bundle)
		}
		if ag.calls != 1 {
			t.Fatalf("order %v: agent ran %d times, want once (only the agent run)", order, ag.calls)
		}
	}
}

// checkOwnRun asserts a record was carried to the end under req's config.
func checkOwnRun(t *testing.T, inv *domain.Investigation, req investigate.Request, agentOn bool) {
	t.Helper()
	if inv.Config != req.Config.Digest() {
		t.Fatalf("%s: config %s, want %s", inv.ID, inv.Config, req.Config.Digest())
	}
	if inv.Status != domain.InvestigationCompleted || inv.Review.State != domain.ReviewApproved {
		t.Fatalf("%s: status %s review %s, want completed/approved (a hijacked run stays running)", inv.ID, inv.Status, inv.Review.State)
	}
	if inv.Graph == "" || inv.Traversal == nil || len(inv.Findings) == 0 {
		t.Fatalf("%s: later stages did not run on this record", inv.ID)
	}
	want := domain.AgentNotRun
	if agentOn {
		want = domain.AgentSubmitted
	}
	if inv.Agent == nil || inv.Agent.Status != want {
		t.Fatalf("%s: agent %+v, want %s", inv.ID, inv.Agent, want)
	}
	completed := map[domain.Stage]int{}
	for _, e := range inv.Audit {
		if e.Event == "completed" {
			completed[e.Stage]++
		}
	}
	if completed[domain.StageBuildGraph] != 1 || completed[domain.StageClassify] != 1 {
		t.Fatalf("%s: completed stages %v", inv.ID, completed)
	}
}

// Every stage key covers the refs the stage's output carries forward (a memo
// hit returns them) and the request inputs the stage reads; audit times are
// not identity.
func TestStageKeysCoverPassedRefs(t *testing.T) {
	f := runtest.New(t)
	base := run.StageInput{Request: f.Request(), Definition: "sha256:def", Refs: run.Refs{
		Change: "sha256:c", Snapshot: "sha256:s", Bundle: "bnd-1", Investigation: "inv-1", Previous: "inv-0", Record: "sha256:r",
		Pending: []domain.AuditEvent{{At: runtest.FixedNow, Stage: domain.StageCollectChange, Event: "completed", Actor: "system", Detail: "chg-1"}},
	}}
	vary := map[string]func(*run.StageInput){
		"change":        func(in *run.StageInput) { in.Refs.Change = "sha256:c2" },
		"snapshot":      func(in *run.StageInput) { in.Refs.Snapshot = "sha256:s2" },
		"bundle":        func(in *run.StageInput) { in.Refs.Bundle = "bnd-2" },
		"investigation": func(in *run.StageInput) { in.Refs.Investigation = "inv-2" },
		"previous":      func(in *run.StageInput) { in.Refs.Previous = "inv-9" },
	}
	for _, s := range run.RoundPlan(true) {
		in := base
		in.Stage, in.Phase = s.Stage, s.Phase
		k := run.Key(in)
		for name, f := range vary {
			v := in
			f(&v)
			if run.Key(v) == k {
				t.Errorf("%s: key ignores the passed %s ref", s.Stage, name)
			}
		}
		if s.Stage != domain.StageCollectChange && s.Stage != domain.StageBuildSnapshot && s.Stage != domain.StageAssemble {
			v := in
			v.Refs.Record = "sha256:r2"
			if run.Key(v) == k {
				t.Errorf("%s: key ignores the run record it updates", s.Stage)
			}
		}
		v := in
		v.Refs.Pending = []domain.AuditEvent{{At: runtest.FixedNow.Add(time.Hour), Stage: domain.StageCollectChange, Event: "completed", Actor: "system", Detail: "chg-1"}}
		if run.Key(v) != k {
			t.Errorf("%s: key depends on audit times", s.Stage)
		}
	}
	collect := base
	collect.Stage = domain.StageCollectChange
	render := collect
	render.Request.Config.Render = true
	if run.Key(collect) == run.Key(render) {
		t.Error("collect-change: key ignores Render (the collector renders the release when it is on)")
	}
}
