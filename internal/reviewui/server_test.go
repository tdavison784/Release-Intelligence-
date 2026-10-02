package reviewui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

var (
	started = DemoEpoch.Add(time.Hour).Format(time.RFC3339Nano)
	now     = DemoEpoch.Add(time.Hour + 5*time.Minute)
)

type rig struct {
	t   *testing.T
	q   *DemoQueue
	srv *httptest.Server
	c   *http.Client
}

func newRig(t *testing.T) *rig { return newRigWith(t, nil) }

// newRigWith wraps the demo queue (e.g. to inject failures).
func newRigWith(t *testing.T, wrap func(*DemoQueue) knowledge.Queue) *rig {
	t.Helper()
	q := NewDemoQueue()
	var kq knowledge.Queue = q
	if wrap != nil {
		kq = wrap(q)
	}
	srv := httptest.NewServer(NewServer(kq, Options{Now: func() time.Time { return now }, Logf: t.Logf}))
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &rig{t: t, q: q, srv: srv, c: c}
}

func (r *rig) do(req *http.Request) (int, string, http.Header) {
	r.t.Helper()
	resp, err := r.c.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header
}

func (r *rig) get(path string) (int, string) {
	r.t.Helper()
	req, _ := http.NewRequest("GET", r.srv.URL+path, nil)
	s, b, _ := r.do(req)
	return s, b
}

func (r *rig) post(path string, f url.Values) (int, string, http.Header) {
	r.t.Helper()
	req, _ := http.NewRequest("POST", r.srv.URL+path, strings.NewReader(f.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r.do(req)
}

func (r *rig) id(title string) string {
	r.t.Helper()
	id := r.q.ItemIDByTitle(title)
	if id == "" {
		r.t.Fatalf("no fixture item %q", title)
	}
	return id
}

func (r *rig) routine() []string {
	var ids []string
	for _, id := range r.q.ItemIDs() {
		it, _ := r.q.ItemRecord(id)
		if strings.Contains(r.q.candidates[it.CandidateID].Title, "Default of --") {
			ids = append(ids, id)
		}
	}
	return ids
}

func form(kv ...string) url.Values {
	v := url.Values{}
	for i := 0; i < len(kv); i += 2 {
		v.Add(kv[i], kv[i+1])
	}
	return v
}

func contains(t *testing.T, body string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("body lacks %q", w)
		}
	}
}

func lacks(t *testing.T, body string, bad ...string) {
	t.Helper()
	for _, w := range bad {
		if strings.Contains(body, w) {
			t.Errorf("body unexpectedly contains %q", w)
		}
	}
}

func TestFixturesAreValidDomainRecords(t *testing.T) {
	q := NewDemoQueue()
	for _, c := range q.candidates {
		if err := c.Validate(); err != nil {
			t.Errorf("candidate: %v", err)
		}
	}
	for _, p := range q.proposals {
		if err := p.ValidateAgainst(q.candidates[p.CandidateID]); err != nil {
			t.Errorf("proposal: %v", err)
		}
	}
	for _, v := range q.validations {
		if err := v.Validate(); err != nil {
			t.Errorf("validation: %v", err)
		}
	}
	for _, it := range q.items {
		if err := it.Validate(); err != nil {
			t.Errorf("item: %v", err)
		}
	}
	for _, d := range q.decisions {
		if err := d.Validate(); err != nil {
			t.Errorf("decision: %v", err)
		}
	}
}

// --- inbox (G7) ---------------------------------------------------------------------

func TestInboxCountsAndDefaultFilter(t *testing.T) {
	r := newRig(t)
	st, body := r.get("/")
	if st != 200 {
		t.Fatal(st)
	}
	contains(t, body, "Pending", "Model disagreement", "Needs semantic mapping", "Applicability", "Needs more evidence", "Deferred",
		"The default privateKey.rotationPolicy is now Always", `class="card"`, `aria-expanded="false"`, "Expand all", "Collapse all", "Select all")
	// the default view is pending only: decided/deferred/needs-evidence items are not listed
	lacks(t, body, "Owner-reference flag default flipped", "Default retention changed", "Ambient data plane")
	// nothing is truncated: no "first N of M" notice, and select-all counts the filter
	lacks(t, body, `class="truncated"`, "Select all 10 shown")
	contains(t, body, "in this filter")
}

func TestInboxSaysWhenTheListIsTruncated(t *testing.T) {
	q := NewDemoQueue()
	srv := httptest.NewServer(NewServer(q, Options{Now: func() time.Time { return now }, InboxLimit: 2, Logf: t.Logf}))
	t.Cleanup(srv.Close)
	full, err := q.Inbox(context.Background(), knowledge.InboxFilter{Status: []domain.ReviewStatus{domain.ReviewPending}})
	if err != nil || full.Matches < 3 {
		t.Fatalf("fixtures: %d pending matches (%v); want ≥3 so the limit actually truncates", full.Matches, err)
	}
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	body := string(b)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	// the page must not pretend to be the whole filter: it says how much was
	// cut, and select-all covers only the two rows shown
	contains(t, body, fmt.Sprintf("Showing the first 2 of %d matching items", full.Matches), "Select all 2 shown", `class="truncated"`)
	lacks(t, body, "in this filter")
	if got := strings.Count(body, `class="card"`); got != 2 {
		t.Fatalf("cards = %d, want 2", got)
	}
}

// bigQueue serves a synthetic inbox of n pending items (the DemoQueue stands in
// for Item/Decide); it exists to render the inbox at scale.
type bigQueue struct {
	*DemoQueue
	n int
}

func (b *bigQueue) Inbox(_ context.Context, f knowledge.InboxFilter) (*knowledge.Inbox, error) {
	in := &knowledge.Inbox{}
	pending := len(f.Status) == 0 // the UI always sends statuses; default like parseFilter
	for _, st := range f.Status {
		pending = pending || st == domain.ReviewPending
	}
	if !pending {
		return in, nil
	}
	for i := 0; i < b.n; i++ {
		it := domain.ReviewItem{ID: fmt.Sprintf("item-scale-%04d", i), Question: "Did the default change?", Product: "cert-manager",
			Release: "v1.18.0", QuestionType: domain.QuestionSemanticMapping, Status: domain.ReviewPending,
			Routing:  domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityNormal},
			Proposed: domain.SemanticAssertion{Statement: "scale"}}
		in.Items = append(in.Items, knowledge.InboxRow{Item: it, Title: fmt.Sprintf("Scale item %d", i),
			Models: []string{"claude-opus-5-5"}, Calls: 1})
		in.Counts.Pending++
	}
	in.Matches = len(in.Items)
	if f.Limit > 0 && len(in.Items) > f.Limit {
		in.Items = in.Items[:f.Limit]
	}
	return in, nil
}

func TestInboxHandlesHundredsOfItems(t *testing.T) {
	srv := httptest.NewServer(NewServer(&bigQueue{DemoQueue: NewDemoQueue(), n: 400}, Options{Now: func() time.Time { return now }, Logf: t.Logf}))
	t.Cleanup(srv.Close)
	started := time.Now()
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	body := string(b)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	// the default page is the first 200 of 400 (priority order): every card
	// renders, the page says what was cut, and the rest stay reachable
	if got := strings.Count(body, `class="card"`); got != 200 {
		t.Fatalf("cards = %d, want 200", got)
	}
	contains(t, body, "Showing the first 200 of 400 matching items", "Select all 200 shown", `data-id="item-scale-0199"`)
	lacks(t, body, `data-id="item-scale-0200"`)
	t.Logf("inbox of 400 rendered in %s", time.Since(started))
}

func TestInboxFilters(t *testing.T) {
	r := newRig(t)
	for _, tc := range []struct {
		name, query string
		want, not   []string
	}{
		{"product", "product=argo-cd", []string{"RBAC default policy setting moved"}, []string{"rotationPolicy", "Default of --"}},
		{"release", "release=v1.13.0", []string{"enable-ssl-passthrough"}, []string{"max-concurrent-challenges"}},
		{"subject type", "subject=product-relationship", []string{"HTTP01 solver needs"}, []string{"rotationPolicy"}},
		{"question type", "question=semantic-mapping", []string{"RBAC default policy"}, []string{"rotationPolicy"}},
		{"severity", "severity=high", []string{"rotationPolicy"}, []string{"HTTP01 solver"}},
		{"confidence", "confidence=low", []string{"rotationPolicy", "RBAC default policy"}, []string{"HTTP01 solver"}},
		{"model", "model=glm", []string{"rotationPolicy"}, []string{"HTTP01 solver"}},
		{"disagreement yes", "disagreement=yes", []string{"rotationPolicy", "RBAC default policy"}, []string{"HTTP01 solver", "Default of --"}},
		{"disagreement no", "disagreement=no", []string{"HTTP01 solver", "Default of --"}, []string{"rotationPolicy"}},
		{"source", "source=istio", nil, []string{"rotationPolicy"}},
		{"status needs evidence", "status=needs-evidence", []string{"Ambient data plane"}, []string{"rotationPolicy"}},
		{"status deferred", "status=deferred", []string{"Default retention changed"}, []string{"rotationPolicy"}},
		{"status all", "status=all", []string{"rotationPolicy", "Owner-reference flag", "Ambient data plane", "Default retention"}, nil},
		{"reviewer", "status=all&reviewer=morgan", []string{"Owner-reference flag"}, []string{"rotationPolicy"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, body := r.get("/?" + tc.query)
			if st != 200 {
				t.Fatal(st)
			}
			contains(t, body, tc.want...)
			lacks(t, body, tc.not...)
		})
	}
	// counts do not depend on the filter
	_, a := r.get("/")
	_, b := r.get("/?product=argo-cd")
	for _, body := range []string{a, b} {
		contains(t, body, "<b>10</b><span>Pending</span>", "<b>1</b><span>Needs more evidence</span>", "<b>1</b><span>Deferred</span>", "<b>2</b><span>Model disagreement</span>")
	}
}

// --- item page (G8) ------------------------------------------------------------------

func TestItemPageShowsEveryG8Element(t *testing.T) {
	r := newRig(t)
	id := r.id("rotationPolicy")
	st, body := r.get("/items/" + id)
	if st != 200 {
		t.Fatal(st)
	}
	contains(t, body,
		"If a Certificate with rotationPolicy unset does nothing", // the concrete question
		"Upstream statement", "Upgrading 1.17 → 1.18",
		`href="https://github.com/cert-manager/website/blob/master/content/docs/releases/upgrading/upgrading-1.17-1.18.md#L21-L30"`, // openable evidence
		`rel="noopener noreferrer"`, "The default value of Certificate.spec.privateKey.rotationPolicy is now Always",
		"Proposed assertion", "behavior-change → review-required",
		"claude-opus-5-5", "claude-sonnet-5-5", "glm-5.3-flash", // per-model proposals side by side
		"agree-disagree", "disagree · 3 variants", "consensus · cross-model · 3 calls", "call-claude-opus-5-5-", "single call",
		"Validation results", "semvalidate.crd@v1", "out-confirmed", "out-inconclusive",
		"Environment context", "illustration only",
		"Previous related decisions",
		`name="action" value="accept"`, `value="reject"`, `value="need-more-evidence"`, `value="defer"`, `data-correct="1"`,
		`name="started"`, `name="reviewer"`)
	lacks(t, body, "Rendered delta") // this fixture has no render evidence: the panel is hidden
	// the same subject has related decisions listed once decided: see TestDecideAccept
	if st, _ := r.get("/items/ri-nope"); st != 404 {
		t.Errorf("unknown item: %d", st)
	}
}

func TestRenderedDeltaPanelOnlyWhenPresent(t *testing.T) {
	r := newRig(t)
	_, body := r.get("/items/" + r.id("HTTP01 solver"))
	contains(t, body, "Rendered delta", "not visible in render", "ClusterRole/cert-manager-controller-challenges", "rbac-permission-added",
		"rules[3].resources", "source", "target", "[ingresses, httproutes]", "helm v3.17.2", "sha256:9a1c", "Absence from a render is not refutation")
	_, other := r.get("/items/" + r.id("Default of --max-concurrent-challenges"))
	lacks(t, other, "Rendered delta")
}

func TestPerAspectAgreementIsHighlighted(t *testing.T) {
	r := newRig(t)
	_, body := r.get("/items/" + r.id("RBAC default policy"))
	contains(t, body, "m-disagree", "disagree · 2 variants", "g-A", "g-B", "as-proposed")
	_, body = r.get("/items/" + r.id("Default of --max-concurrent-challenges"))
	contains(t, body, "m-agree", "consensus · cross-model · 2 calls")
	lacks(t, body, "m-disagree")
}

func TestDetailFragmentMarksItemOpened(t *testing.T) {
	r := newRig(t)
	id := r.id("Default of --max-concurrent-challenges")
	st, frag := r.get("/items/" + id + "/detail")
	if st != 200 {
		t.Fatal(st)
	}
	lacks(t, frag, "<html", "<body")
	contains(t, frag, "Upstream statement", "Evidence", `action="/items/`+id+`/decision"`)
}

// --- individual decisions ---------------------------------------------------------------------

func decisionForm(action string, extra ...string) url.Values {
	f := form("action", action, "reviewer", "dana", "started", started, "return", "/?status=pending")
	for i := 0; i < len(extra); i += 2 {
		if k := extra[i]; k == "label" {
			f.Add(k, extra[i+1])
		} else {
			f.Set(k, extra[i+1])
		}
	}
	return f
}

func TestDecideAcceptRecordsAHumanDecision(t *testing.T) {
	r := newRig(t)
	id := r.id("Default of --max-concurrent-challenges")
	st, _, h := r.post("/items/"+id+"/decision", decisionForm("accept"))
	if st != http.StatusSeeOther || h.Get("Location") != "/?status=pending" {
		t.Fatalf("status %d location %q", st, h.Get("Location"))
	}
	ds := r.q.Decisions()
	d := ds[len(ds)-1]
	it, _ := r.q.ItemRecord(id)
	if d.ReviewItemID != id || d.Action != domain.ActionAccept || d.ReviewerKind != domain.ReviewerHuman || d.ProxyProvenance != nil ||
		d.Reviewer != "dana" || it.Status != domain.ReviewDecided || d.BatchID != "" || d.BatchSize != 0 {
		t.Errorf("decision %+v item %s", d, it.Status)
	}
	if d.StartedAt.Format(time.RFC3339Nano) != started || !d.DecidedAt.Equal(now) || d.Duration() != 5*time.Minute {
		t.Errorf("timing: %v → %v", d.StartedAt, d.DecidedAt)
	}
	if d.Original == nil || d.Original.Digest() != it.Proposed.Digest() {
		t.Error("the shown assertion is not kept as the original")
	}
	_, inbox := r.get("/?status=pending")
	contains(t, inbox, "Recorded accept on "+id)
	_, again := r.get("/")
	lacks(t, again, "Recorded accept") // shown once
	// the decision shows up as a previous decision on the item page
	_, page := r.get("/items/" + id)
	contains(t, page, "dana", "no further decision can be added")
}

func TestDecideRejectNeedEvidenceDefer(t *testing.T) {
	r := newRig(t)
	ids := r.routine()
	st, _, _ := r.post("/items/"+ids[0]+"/decision", decisionForm("reject", "reason", "the note says the opposite", "label", "wrong-change-type"))
	if st != 303 {
		t.Fatal(st)
	}
	d := r.q.Decisions()[len(r.q.Decisions())-1]
	if d.Action != domain.ActionReject || d.Reason != "the note says the opposite" || len(d.Labels) != 2 || d.Labels[0] != domain.LabelRejected || d.Labels[1] != domain.LabelWrongChangeType {
		t.Errorf("%+v", d)
	}
	if st, _, _ := r.post("/items/"+ids[1]+"/decision", decisionForm("need-more-evidence", "reason", "excerpt cut off")); st != 303 {
		t.Fatal(st)
	}
	it, _ := r.q.ItemRecord(ids[1])
	if it.Status != domain.ReviewNeedsEvidence {
		t.Error(it.Status)
	}
	if st, _, _ := r.post("/items/"+ids[2]+"/decision", decisionForm("defer")); st != 303 {
		t.Fatal(st)
	}
	it, _ = r.q.ItemRecord(ids[2])
	d = r.q.Decisions()[len(r.q.Decisions())-1]
	if it.Status != domain.ReviewDeferred || len(d.Labels) != 0 {
		t.Errorf("%s %+v", it.Status, d)
	}
	// a deferred item can still be decided later
	if st, _, _ := r.post("/items/"+ids[2]+"/decision", decisionForm("accept")); st != 303 {
		t.Fatal(st)
	}
}

func TestDecideRefusals(t *testing.T) {
	r := newRig(t)
	id := r.id("Default of --max-concurrent-challenges")
	cases := []struct {
		name   string
		f      url.Values
		status int
		msg    string
	}{
		{"reject without reason", decisionForm("reject"), 422, "reject requires a reason"},
		{"need evidence without reason", decisionForm("need-more-evidence"), 422, "requires a reason"},
		{"no reviewer", func() url.Values { f := decisionForm("accept"); f.Del("reviewer"); return f }(), 422, "Reviewer name is required"},
		{"blank reviewer", func() url.Values { f := decisionForm("accept"); f.Set("reviewer", "  "); return f }(), 422, "Reviewer name is required"},
		{"proxy kind", decisionForm("accept", "reviewer_kind", "proxy"), 400, "human decisions only"},
		{"proxy provenance", decisionForm("accept", "proxyProvenance", "{}"), 400, "human decisions only"},
		{"unknown action", decisionForm("approve"), 400, "unknown action"},
		{"missing action", form("reviewer", "dana", "started", started), 400, "unknown action"},
		{"correct without change", decisionForm("correct", "reason", "x", "c_statement", "same"), 422, "must change at least one aspect"},
		{"outcome label by hand", decisionForm("accept", "label", "accepted"), 400, "cannot be set by hand"},
		{"duplicate without fact", decisionForm("reject", "reason", "dup", "outcome", "duplicate"), 422, "names the fact"},
		{"started missing", form("action", "accept", "reviewer", "dana"), 400, "start time"},
		{"started in the future", decisionForm("accept", "started", now.Add(time.Hour).Format(time.RFC3339Nano)), 400, "future"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.f
			if tc.name == "started missing" {
				f = form("action", "accept", "reviewer", "dana")
			}
			st, body, _ := r.post("/items/"+id+"/decision", f)
			if st != tc.status {
				t.Fatalf("status %d, want %d\n%s", st, tc.status, body)
			}
			contains(t, body, tc.msg)
			if n := len(r.q.Decisions()); n != 1 { // only the decided fixture
				t.Errorf("a refused decision was recorded (%d)", n)
			}
		})
	}
	// an already decided item
	decided := r.id("Owner-reference flag")
	if st, _, _ := r.post("/items/"+decided+"/decision", decisionForm("accept")); st != 409 {
		t.Errorf("decided item: %d", st)
	}
	if st, _, _ := r.post("/items/ri-missing/decision", decisionForm("accept")); st != 404 {
		t.Errorf("unknown item: %d", st)
	}
}

func TestCorrectKeepsTheOriginalAndTheClassFollowsTheKind(t *testing.T) {
	r := newRig(t)
	id := r.id("rotationPolicy")
	it, _ := r.q.ItemRecord(id)
	f := decisionForm("correct", "reason", "keys rotate → pods fail on pinned keys",
		"c_statement", it.Proposed.Statement,
		"c_cons_kind", "workload-failure", "c_cons_severity", "critical", "c_cons_statement", "Pinned clients refuse the new key.",
		"c_cons_remediation", "Set rotationPolicy: Never explicitly.")
	st, body, _ := r.post("/items/"+id+"/decision", f)
	if st != 303 {
		t.Fatalf("%d %s", st, body)
	}
	d := r.q.Decisions()[len(r.q.Decisions())-1]
	if d.Action != domain.ActionCorrect || d.Corrected == nil || d.Original == nil {
		t.Fatalf("%+v", d)
	}
	if d.Original.Consequence.Kind != domain.ConsequenceBehaviorChange {
		t.Error("the original must be kept untouched")
	}
	c := d.Corrected.Consequence
	if c.Kind != domain.ConsequenceWorkloadFailure || c.ExposedClass != domain.ImpactActionRequired || c.Severity != domain.SeverityCritical {
		t.Errorf("%+v", c)
	}
	// aspects the form did not carry are kept, never dropped
	if d.Corrected.Subject == nil || d.Corrected.Change == nil || d.Corrected.Applicability == nil {
		t.Error("a correction dropped an aspect")
	}
	want := []domain.FeedbackLabel{domain.LabelCorrected, domain.LabelWrongConsequence}
	if len(d.Labels) != 2 || d.Labels[0] != want[0] || d.Labels[1] != want[1] {
		t.Errorf("labels %v", d.Labels)
	}
	if err := d.Validate(); err != nil {
		t.Error(err)
	}
}

func TestCorrectSubjectChangeAndApplicability(t *testing.T) {
	r := newRig(t)
	id := r.id("RBAC default policy")
	f := decisionForm("correct", "reason", "it is a ConfigMap key",
		"c_subject_family", "config-key", "c_subject_product", "argo-cd", "c_subject_path", "server.rbac.policy.default",
		"c_change_type", "renamed", "c_change_replacedby", `{"family":"config-key","product":"argo-cd","path":"configs.rbac.policy.default"}`)
	st, body, _ := r.post("/items/"+id+"/decision", f)
	if st != 303 {
		t.Fatalf("%d %s", st, body)
	}
	d := r.q.Decisions()[len(r.q.Decisions())-1]
	if d.Corrected.Subject.Family != domain.SubjectConfigKey || d.Labels[1] != domain.LabelWrongSubject {
		t.Errorf("%+v %v", d.Corrected.Subject, d.Labels)
	}
	// a bad condition JSON is refused and keeps the typed values in the form
	id2 := r.id("rotationPolicy")
	st, body, _ = r.post("/items/"+id2+"/decision", decisionForm("correct", "reason", "x", "c_app_exposure", `{"op":"all","bogus":1}`))
	if st != 422 {
		t.Fatalf("%d", st)
	}
	contains(t, body, "exposure condition", "bogus")
	// an invalid consequence kind is refused by the domain validation
	st, body, _ = r.post("/items/"+id2+"/decision", decisionForm("correct", "reason", "x", "c_cons_kind", "explodes"))
	if st != 422 {
		t.Fatalf("%d", st)
	}
}

func TestCorrectOnAGateQuestionIsRefused(t *testing.T) {
	r := newRig(t)
	id := r.id("Ambient data plane")
	st, body, _ := r.post("/items/"+id+"/decision", decisionForm("correct", "reason", "x"))
	if st != 422 {
		t.Fatalf("%d %s", st, body)
	}
	// and the page does not offer a correction form for it
	_, page := r.get("/items/" + id)
	lacks(t, page, `data-correct="1"`)
}

func TestCrossOriginPostIsRefused(t *testing.T) {
	r := newRig(t)
	id := r.id("Default of --max-concurrent-challenges")
	req, _ := http.NewRequest("POST", r.srv.URL+"/items/"+id+"/decision", strings.NewReader(decisionForm("accept").Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://evil.example")
	if st, _, _ := r.do(req); st != 403 {
		t.Errorf("%d", st)
	}
	if len(r.q.Decisions()) != 1 {
		t.Error("decision recorded")
	}
}

func TestReturnRedirectStaysLocal(t *testing.T) {
	r := newRig(t)
	id := r.id("Default of --max-concurrent-challenges")
	_, _, h := r.post("/items/"+id+"/decision", decisionForm("accept", "return", "https://evil.example/"))
	if h.Get("Location") != "/" {
		t.Errorf("redirect to %q", h.Get("Location"))
	}
}

func TestStaticAssetsAndThemeHooks(t *testing.T) {
	r := newRig(t)
	st, css := r.get("/static/app.css")
	if st != 200 || !strings.Contains(css, "prefers-color-scheme: dark") || !strings.Contains(css, "--accent") {
		t.Errorf("css %d", st)
	}
	st, js := r.get("/static/app.js")
	if st != 200 || !strings.Contains(js, "ri-theme") {
		t.Errorf("js %d", st)
	}
	_, body := r.get("/")
	contains(t, body, `id="theme"`, "ri-theme", `href="/static/app.css"`)
}

// failingQueue refuses every Decide (to test that a refused batch writes nothing).
type failingQueue struct{ *DemoQueue }

func (failingQueue) Decide(context.Context, []domain.ReviewDecision) ([]knowledge.DecisionOutcome, error) {
	return nil, errors.New("store unavailable")
}

// --- PO-1 / PO-2: consensus is separate calls; models may request action-required ---------------

func TestFixtureCallsAreSeparate(t *testing.T) {
	q := NewDemoQueue()
	calls := map[string]bool{}
	for _, p := range q.proposals {
		if p.Provenance.CallID == "" || calls[p.Provenance.CallID] {
			t.Errorf("proposal %s: call id %q missing or reused", p.ID, p.Provenance.CallID)
		}
		calls[p.Provenance.CallID] = true
	}
	// the same-model example: two separate calls of one model
	id := q.ItemIDByTitle("Default of --txt-prefix")
	it, _ := q.ItemRecord(id)
	a, b := q.proposals[it.Proposals[0]], q.proposals[it.Proposals[1]]
	if a.Provenance.Model != b.Provenance.Model || !domain.SeparateCalls(a, b) {
		t.Errorf("want two separate calls of one model, got %s/%s", a.Provenance.Model, b.Provenance.Model)
	}
}

func TestConsensusScopeIsLabelledPerAspectAndInTheInbox(t *testing.T) {
	r := newRig(t)
	_, same := r.get("/items/" + r.id("Default of --txt-prefix"))
	contains(t, same, "consensus · same-model · 2 calls", "call-claude-opus-5-5-")
	lacks(t, same, "cross-model")
	_, cross := r.get("/items/" + r.id("Default of --max-concurrent-challenges"))
	contains(t, cross, "consensus · cross-model · 2 calls")
	// disagreeing aspects: each answer group is labelled
	_, dis := r.get("/items/" + r.id("rotationPolicy"))
	contains(t, dis, "same-model consensus · 2 calls", "single call") // opus + sonnet share a family; glm is alone
	// inbox badges
	_, inbox := r.get("/")
	contains(t, inbox, "consensus · same-model · 2 calls", "consensus · cross-model · 2 calls", "models disagree")
	_, all := r.get("/?status=all")
	contains(t, all, "single call") // an item with one call
}

func TestRequestedClassAndConsensusActionAreShown(t *testing.T) {
	r := newRig(t)
	_, body := r.get("/items/" + r.id("RSA keys below 2048"))
	contains(t, body, "Consensus requests ACTION REQUIRED", "ACTION REQUIRED · model consensus", "requests action-required",
		"cross-model", "Consensus never produces NOT AFFECTED", "sampled into human review")
	// a review-required suggestion is shown, but is no ACTION banner
	_, rot := r.get("/items/" + r.id("rotationPolicy"))
	contains(t, rot, "review-required")
	lacks(t, rot, "Consensus requests ACTION REQUIRED", "requests action-required")
	_, other := r.get("/items/" + r.id("Default of --max-concurrent-challenges"))
	lacks(t, other, "Consensus requests ACTION REQUIRED")
}

func TestConsensusActionItemNeedsAnOpenBeforeBulkAccept(t *testing.T) {
	r := newRig(t)
	act := r.id("RSA keys below 2048")
	ids := append(r.routine()[:2], act)
	_, body, _ := r.post("/bulk", bulkForm("accept", ids))
	contains(t, body, "1 blocked", "high priority")
	r.get("/items/" + act)
	st, _, _ := r.post("/bulk", bulkForm("accept", ids, "confirm", "1"))
	if st != 303 {
		t.Fatal(st)
	}
}

// --- demo mode: fixtures are never mistaken for upstream evidence --------------------------------

func TestDemoModeBannerAndFixtureTags(t *testing.T) {
	for _, demo := range []bool{true, false} {
		q := NewDemoQueue()
		srv := httptest.NewServer(NewServer(q, Options{Now: func() time.Time { return now }, Logf: t.Logf, Demo: demo}))
		r := &rig{t: t, q: q, srv: srv, c: newRig(t).c}
		id := r.id("rotationPolicy")
		_, bulk, _ := r.post("/bulk", bulkForm("defer", r.routine()[:2]))
		pages := map[string]string{}
		pages["inbox"] = func() string { _, b := r.get("/"); return b }()
		pages["item"] = func() string { _, b := r.get("/items/" + id); return b }()
		pages["detail"] = func() string { _, b := r.get("/items/" + id + "/detail"); return b }()
		pages["bulk confirm"] = bulk
		pages["error"] = func() string { _, b := r.get("/items/ri-nope"); return b }()
		for name, body := range pages {
			if demo {
				if name != "detail" {
					contains(t, body, "DEMO DATA — fixtures, not real upstream evidence", `class="demo-banner"`, `<body class="demo">`)
				}
				if name == "item" || name == "detail" {
					contains(t, body, "fixture excerpt", `class="badge fixture"`)
				}
			} else {
				lacks(t, body, "DEMO DATA", "demo-banner", "fixture excerpt", "badge fixture")
			}
		}
		srv.Close()
	}
	// both themes ship the banner style
	r := newRig(t)
	_, css := r.get("/static/app.css")
	contains(t, css, ".demo-banner", "body.demo .sticky", ".badge.fixture")
}

func TestInboxLimitShowsTheTruncationNotice(t *testing.T) {
	q := NewDemoQueue()
	srv := httptest.NewServer(NewServer(q, Options{Now: func() time.Time { return now }, Logf: t.Logf, Demo: true, InboxLimit: 3}))
	defer srv.Close()
	r := &rig{t: t, q: q, srv: srv, c: newRig(t).c}
	_, body := r.get("/")
	contains(t, body, "Showing the first 3 of 10 matching items", "Select all 3 shown")
}
