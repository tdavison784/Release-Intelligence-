package reviewui

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

func bulkForm(action string, ids []string, extra ...string) url.Values {
	f := form("action", action, "reviewer", "dana", "started", started, "return", "/?status=pending")
	for _, id := range ids {
		f.Add("item", id)
	}
	for i := 0; i < len(extra); i += 2 {
		f.Set(extra[i], extra[i+1])
	}
	return f
}

func TestBulkAcceptWritesOneDecisionPerItemWithABatchID(t *testing.T) {
	r := newRig(t)
	ids := r.routine()[:4]
	// step 1: the confirmation lists the items and the counts, records nothing
	st, body, _ := r.post("/bulk", bulkForm("accept", ids))
	if st != 200 {
		t.Fatalf("%d %s", st, body)
	}
	contains(t, body, "Confirm bulk accept", "4 will be recorded", "Record 4 decisions", "dana")
	for _, id := range ids {
		contains(t, body, id[:12])
	}
	if n := len(r.q.Decisions()); n != 1 {
		t.Fatalf("the confirmation step recorded decisions (%d)", n)
	}
	// step 2: confirm
	st, _, h := r.post("/bulk", bulkForm("accept", ids, "confirm", "1"))
	if st != 303 || h.Get("Location") != "/?status=pending" {
		t.Fatalf("%d %q", st, h.Get("Location"))
	}
	ds := r.q.Decisions()[1:]
	if len(ds) != 4 {
		t.Fatalf("%d decisions, want one per item", len(ds))
	}
	batch := ds[0].BatchID
	if !strings.HasPrefix(batch, domain.BatchIDPrefix) || batch != domain.BatchID(ids, "dana", now) {
		t.Errorf("batch id %q", batch)
	}
	seen := map[string]bool{}
	for _, d := range ds {
		if d.BatchID != batch || d.BatchSize != 4 || d.Action != domain.ActionAccept || d.ReviewerKind != domain.ReviewerHuman || d.Reviewer != "dana" {
			t.Errorf("%+v", d)
		}
		if d.StartedAt.Format(time.RFC3339Nano) != started || !d.DecidedAt.Equal(now) { // started = batch view opened, decided = submit
			t.Errorf("timing %v %v", d.StartedAt, d.DecidedAt)
		}
		if err := d.Validate(); err != nil {
			t.Error(err)
		}
		seen[d.ReviewItemID] = true
	}
	for _, id := range ids {
		if !seen[id] {
			t.Errorf("no decision for %s", id)
		}
		it, _ := r.q.ItemRecord(id)
		if it.Status != domain.ReviewDecided {
			t.Errorf("%s is %s", id, it.Status)
		}
	}
	_, inbox := r.get("/?status=pending")
	contains(t, inbox, "Recorded 4 × accept as batch "+batch)
}

func TestBulkRejectNeedEvidenceDeferShareReasonAndLabels(t *testing.T) {
	r := newRig(t)
	ids := r.routine()
	// reject needs the shared reason
	_, body, _ := r.post("/bulk", bulkForm("reject", ids[:2]))
	contains(t, body, "reject requires a reason", "skip")
	if st, _, _ := r.post("/bulk", bulkForm("reject", ids[:2], "confirm", "1")); st != 422 {
		t.Errorf("reject without reason confirmed: %d", st)
	}
	st, _, _ := r.post("/bulk", bulkForm("reject", ids[:2], "confirm", "1", "reason", "restated nonsense"))
	if st != 303 {
		t.Fatal(st)
	}
	ds := r.q.Decisions()[1:]
	if len(ds) != 2 || ds[0].Reason != "restated nonsense" || ds[0].Labels[0] != domain.LabelRejected || ds[0].BatchID == "" || ds[0].BatchID != ds[1].BatchID {
		t.Errorf("%+v", ds)
	}
	f := bulkForm("need-more-evidence", ids[2:4], "confirm", "1", "reason", "excerpts truncated")
	if st, _, _ := r.post("/bulk", f); st != 303 {
		t.Fatal(st)
	}
	f = bulkForm("defer", ids[4:6], "confirm", "1")
	if st, _, _ := r.post("/bulk", f); st != 303 {
		t.Fatal(st)
	}
	all := r.q.Decisions()
	if n := len(all); n != 7 {
		t.Fatalf("%d", n)
	}
	for _, d := range all[3:5] {
		if d.Action != domain.ActionNeedMoreEvidence || d.Labels[0] != domain.LabelInsufficientEvidence {
			t.Errorf("%+v", d)
		}
	}
	for _, d := range all[5:] {
		if d.Action != domain.ActionDefer || len(d.Labels) != 0 {
			t.Errorf("%+v", d)
		}
	}
	// two different batches have different ids
	if all[1].BatchID == all[3].BatchID {
		t.Error("batches share an id")
	}
}

func TestBulkSharedLabels(t *testing.T) {
	r := newRig(t)
	ids := r.routine()[:2]
	f := bulkForm("reject", ids, "confirm", "1", "reason", "wrong subject")
	f.Add("label", "wrong-subject")
	if st, _, _ := r.post("/bulk", f); st != 303 {
		t.Fatal(st)
	}
	for _, d := range r.q.Decisions()[1:] {
		if len(d.Labels) != 2 || d.Labels[1] != domain.LabelWrongSubject {
			t.Errorf("%v", d.Labels)
		}
	}
}

func TestBulkRefusals(t *testing.T) {
	r := newRig(t)
	ids := r.routine()[:3]
	for name, tc := range map[string]struct {
		f      url.Values
		status int
		msg    string
	}{
		"correct is individual-only": {bulkForm("correct", ids, "confirm", "1"), 400, "individual-only"},
		"proxy kind":                 {bulkForm("accept", ids, "confirm", "1", "reviewer_kind", "proxy"), 400, "human decisions only"},
		"unknown action":             {bulkForm("approve-all", ids, "confirm", "1"), 400, "unknown action"},
		"duplicate outcome":          {bulkForm("reject", ids, "confirm", "1", "reason", "x", "outcome", "duplicate"), 400, "individual-only"},
		"outcome label by hand":      {func() url.Values { f := bulkForm("accept", ids, "confirm", "1"); f.Add("label", "accepted"); return f }(), 400, "cannot be set by hand"},
		"started missing":            {form("action", "accept", "reviewer", "dana", "confirm", "1", "item", ids[0], "item", ids[1]), 400, "start time"},
	} {
		t.Run(name, func(t *testing.T) {
			st, body, _ := r.post("/bulk", tc.f)
			if st != tc.status {
				t.Fatalf("%d\n%s", st, body)
			}
			contains(t, body, tc.msg)
		})
	}
	// a single item is not a batch
	st, body, _ := r.post("/bulk", bulkForm("accept", ids[:1], "confirm", "1"))
	if st != 422 {
		t.Fatalf("%d", st)
	}
	contains(t, body, "at least two")
	// no reviewer
	f := bulkForm("accept", ids, "confirm", "1")
	f.Del("reviewer")
	if st, body, _ := r.post("/bulk", f); st != 422 {
		t.Fatalf("%d", st)
	} else {
		contains(t, body, "Reviewer name is required")
	}
	// nothing selected: back to the inbox with a notice
	st, _, h := r.post("/bulk", bulkForm("accept", nil, "confirm", "1"))
	if st != 303 {
		t.Fatalf("%d", st)
	}
	_ = h
	if n := len(r.q.Decisions()); n != 1 {
		t.Fatalf("refused bulk actions recorded %d decisions", n-1)
	}
}

func TestBulkMixedValidAndInvalidItemsAreReportedPerItem(t *testing.T) {
	r := newRig(t)
	ids := r.routine()[:3]
	decided := r.id("Owner-reference flag")
	sel := append(append([]string{}, ids...), decided, "ri-doesnotexist")
	_, body, _ := r.post("/bulk", bulkForm("accept", sel))
	contains(t, body, "3 will be recorded", "2 will be skipped", "already decided", "not found", "skip")
	st, _, _ := r.post("/bulk", bulkForm("accept", sel, "confirm", "1"))
	if st != 303 {
		t.Fatal(st)
	}
	ds := r.q.Decisions()[1:]
	if len(ds) != 3 || ds[0].BatchSize != 3 { // the batch is the decidable items only
		t.Fatalf("%d decisions, size %d", len(ds), ds[0].BatchSize)
	}
	_, inbox := r.get("/?status=pending")
	contains(t, inbox, "Skipped "+decided+": already decided", "Skipped ri-doesnotexist")
}

func TestBulkAcceptGuardHighPriorityAndDisagreement(t *testing.T) {
	r := newRig(t)
	routine := r.routine()[:2]
	high := r.id("rotationPolicy")   // high priority + models disagree
	disagree := r.id("RBAC default") // normal priority, models disagree
	sel := append(append([]string{}, routine...), high, disagree)

	// the preview names the blockers and why; submit is disabled
	st, body, _ := r.post("/bulk", bulkForm("accept", sel))
	if st != 200 {
		t.Fatal(st)
	}
	contains(t, body, "2 blocked", "high priority", "the models&#39; proposals disagree", "Exclude 2 blocked", "disabled")
	// a confirm is refused outright, nothing recorded
	st, body, _ = r.post("/bulk", bulkForm("accept", sel, "confirm", "1"))
	if st != 409 {
		t.Fatalf("%d", st)
	}
	contains(t, body, "blocked")
	if len(r.q.Decisions()) != 1 {
		t.Fatal("blocked batch recorded decisions")
	}
	// the guard applies to accept only: defer / need-more-evidence of the same items are allowed
	if st, _, _ := r.post("/bulk", bulkForm("defer", []string{high, disagree}, "confirm", "1")); st != 303 {
		t.Fatalf("defer of guarded items: %d", st)
	}
	// reset: use fresh items
	r = newRig(t)
	routine = r.routine()[:2]
	high, disagree = r.id("rotationPolicy"), r.id("RBAC default")
	sel = append(append([]string{}, routine...), high, disagree)

	// opening the item (page or inline detail) lifts its block, only its own
	r.get("/items/" + high)
	r.get("/items/" + disagree + "/detail")
	if st, _, _ := r.post("/bulk", bulkForm("accept", sel, "confirm", "1")); st != 303 {
		t.Fatalf("after opening: %d", st)
	}
	if n := len(r.q.Decisions()); n != 5 {
		t.Fatalf("%d decisions", n)
	}

	// only one of the two was opened: the other still blocks; excluding continues without it
	r = newRig(t)
	routine = r.routine()[:2]
	high, disagree = r.id("rotationPolicy"), r.id("RBAC default")
	sel = append(append([]string{}, routine...), high, disagree)
	r.get("/items/" + high)
	_, body, _ = r.post("/bulk", bulkForm("accept", sel))
	contains(t, body, "1 blocked", "the models&#39; proposals disagree")
	lacks(t, body, "high priority and")
	_, body, _ = r.post("/bulk", bulkForm("accept", sel, "exclude_blocked", "1"))
	contains(t, body, "3 will be recorded")
	lacks(t, body, "blocked</span>")
	if st, _, _ := r.post("/bulk", bulkForm("accept", sel, "exclude_blocked", "1", "confirm", "1")); st != 200 {
		t.Fatalf("exclude must re-show the confirmation, not record (%d)", st)
	}
	if len(r.q.Decisions()) != 1 {
		t.Fatal("recorded on exclude")
	}
	// the session is per browser: a fresh client has opened nothing
	other := newRig(t)
	other.q = r.q
	r.get("/items/" + disagree)
	other2 := &rig{t: t, q: r.q, srv: r.srv, c: &http.Client{CheckRedirect: r.c.CheckRedirect}}
	_, body, _ = other2.post("/bulk", bulkForm("accept", sel))
	contains(t, body, "blocked")
}

func TestBulkIsAtomicWhenTheQueueRefuses(t *testing.T) {
	r := newRigWith(t, func(q *DemoQueue) knowledge.Queue { return failingQueue{q} })
	ids := r.routine()[:3]
	st, body, _ := r.post("/bulk", bulkForm("accept", ids, "confirm", "1"))
	if st != 422 {
		t.Fatalf("%d", st)
	}
	contains(t, body, "nothing was recorded", "store unavailable")
	if len(r.q.Decisions()) != 1 {
		t.Error("decisions recorded despite the refusal")
	}
	// the individual path reports the refusal too
	st, body, _ = r.post("/items/"+ids[0]+"/decision", decisionForm("accept"))
	if st != 422 {
		t.Fatalf("%d", st)
	}
	contains(t, body, "store unavailable")
}

func TestInboxOffersBulkControlsButNotBulkCorrect(t *testing.T) {
	r := newRig(t)
	_, body := r.get("/")
	contains(t, body, `id="bulkform"`, `name="action" value="accept"`, `name="action" value="reject"`, `name="action" value="defer"`,
		`name="action" value="need-more-evidence"`, `name="started"`, `form="bulkform"`, "Correct is per item only")
	lacks(t, body, `name="action" value="correct"`)
}
