package reviewui

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// triageQueue serves the demo fixtures with a snapshot the test edits: the
// store records (decisions, facts) that decide what needs review.
type triageQueue struct {
	*DemoQueue
	edit func(*knowledge.Snapshot)
}

func (q *triageQueue) Snapshot(ctx context.Context) (*knowledge.Snapshot, error) {
	s, _ := q.DemoQueue.Snapshot(ctx)
	if q.edit != nil {
		q.edit(s)
	}
	return s, nil
}

func newTriageRig(t *testing.T, scope BlockingScope, edit func(*knowledge.Snapshot, *rig)) *rig {
	t.Helper()
	dq := NewDemoQueue()
	r := &rig{t: t, q: dq}
	tq := &triageQueue{DemoQueue: dq}
	if edit != nil {
		tq.edit = func(s *knowledge.Snapshot) { edit(s, r) }
	}
	r.srv = httptest.NewServer(NewServer(tq, Options{Now: func() time.Time { return now }, Logf: t.Logf, Scope: scope}))
	t.Cleanup(r.srv.Close)
	jar, _ := cookiejar.New(nil)
	r.c = &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return r
}

func (r *rig) itemOf(title string) domain.ReviewItem {
	it, _ := r.q.ItemRecord(r.id(title))
	return it
}

func humanFact(it domain.ReviewItem, level domain.VerificationLevel, st domain.FactStatus) domain.VerifiedFact {
	f := domain.VerifiedFact{ID: "fact-" + it.ID, Product: it.Product, Candidates: []string{it.CandidateID}, Assertion: it.Proposed, Status: st}
	for _, a := range domain.Aspects {
		f.Verification = append(f.Verification, domain.AspectVerification{Aspect: a, Level: level})
	}
	return f
}

var cardID = regexp.MustCompile(`<li class="card [^"]*" data-id="([^"]+)"`)

func cardOrder(body string) []string {
	var ids []string
	for _, m := range cardID.FindAllStringSubmatch(body, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

const (
	tRSA      = "RSA keys below 2048 bits are rejected"               // pending, high
	tHTTP01   = "HTTP01 solver needs ingress-nginx >= 1.12"           // pending
	tRBAC     = "RBAC default policy setting moved"                   // pending
	tRotation = "The default privateKey.rotationPolicy is now Always" // pending, high
	tDecided  = "Owner-reference flag default flipped"                // human decision, decided
	tParked   = "Default retention changed 10d"                       // deferred
	tEvidence = "Ambient data plane behaviour changed"                // needs-evidence
)

// editHidden makes every hidden category occur once in the demo queue.
func editHidden(s *knowledge.Snapshot, r *rig) {
	rsa, http01, rbac := r.itemOf(tRSA), r.itemOf(tHTTP01), r.itemOf(tRBAC)
	// an AI proxy acted on it (status left pending: still hidden, never "human")
	s.Decisions = append(s.Decisions, domain.ReviewDecision{ID: "rd-proxy", ReviewItemID: rsa.ID, Action: domain.ActionAccept,
		Reviewer: "claude-proxy", ReviewerKind: domain.ReviewerProxy, DecidedAt: DemoEpoch})
	// already verified by a human
	s.Facts = append(s.Facts, humanFact(http01, domain.VerifiedHuman, domain.FactActive))
	// consensus-only staging does NOT hide the item
	s.Facts = append(s.Facts, humanFact(rbac, domain.VerifiedConsensus, domain.FactActive))
}

func TestNeedsReviewHidesEveryNonReviewCategory(t *testing.T) {
	r := newTriageRig(t, BlockingScope{}, editHidden)
	st, body := r.get("/")
	if st != 200 {
		t.Fatal(st)
	}
	ids := cardOrder(body)
	for _, c := range []struct {
		title string
		shown bool
	}{
		{tRotation, true}, {tRBAC, true}, // pending; consensus-only fact does not hide
		{tRSA, false},      // proxy-acted
		{tHTTP01, false},   // human-verified fact
		{tDecided, false},  // human decision
		{tParked, false},   // deferred
		{tEvidence, false}, // needs evidence
	} {
		if got := slices.Contains(ids, r.id(c.title)); got != c.shown {
			t.Errorf("%q shown=%v, want %v", c.title, got, c.shown)
		}
	}
	// what is left: the pending fixtures minus the proxy-acted and the human-verified ones
	want := 0
	for _, id := range r.q.ItemIDs() {
		it, _ := r.q.ItemRecord(id)
		if it.Status == domain.ReviewPending && id != r.id(tRSA) && id != r.id(tHTTP01) {
			want++
		}
	}
	if len(ids) != want {
		t.Errorf("cards = %d, want %d", len(ids), want)
	}
	contains(t, body, "Needs review: "+itoa(want), "Hidden from this view: "+itoa(len(r.q.ItemIDs())-want),
		"acted on by an AI proxy", "already verified", "already decided", "parked", `href="/?scope=all"`, "Show all staged data")
	lacks(t, body, "Showing all staged data")
	// the status filter does not exist in this view; counts reflect it
	lacks(t, body, `<option value="decided"`, "Needs more evidence", ">Deferred<")
	contains(t, body, "<b>"+itoa(want)+"</b><span>Needs review</span>")
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestShowAllStagedDataIsExplicitAndLabelsWhatItHides(t *testing.T) {
	r := newTriageRig(t, BlockingScope{}, editHidden)
	_, all := r.get("/?scope=all&status=all")
	if got := len(cardOrder(all)); got != len(r.q.ItemIDs()) {
		t.Errorf("show-all with status all lists %d, want every item %d", got, len(r.q.ItemIDs()))
	}
	contains(t, all, "Showing all staged data", `href="/"`, "Back to Needs review",
		"Not in Needs review: an AI proxy (claude-proxy) acted on it; that is not a human decision",
		"Not in Needs review: already verified at human or deterministic level by fact",
		"Not in Needs review: decided by morgan")
	lacks(t, all, "Hidden from this view")
	// the toggle survives filtering inside the wide view
	_, filtered := r.get("/?scope=all&product=cert-manager")
	contains(t, filtered, `name="scope" value="all"`)
}

func TestProxyDecisionNeverAppearsAsHuman(t *testing.T) {
	r := newTriageRig(t, BlockingScope{}, editHidden)
	_, all := r.get("/?scope=all&status=all")
	i := strings.Index(all, r.id(tRSA))
	if i < 0 || !strings.Contains(all[i:i+3000], "AI proxy") || strings.Contains(all[i:i+3000], "decided by claude-proxy") {
		t.Errorf("the proxy-acted item must be labelled proxy, never decided by a human")
	}
	if hum := triageOne(r.itemOf(tRSA), domain.SemanticCandidate{}, []domain.ReviewDecision{{Reviewer: "x", ReviewerKind: domain.ReviewerProxy}}, nil, BlockingScope{}); hum.Hidden != HiddenProxy {
		t.Errorf("proxy decision hidden as %q", hum.Hidden)
	}
}

func TestBlockingItemsComeFirstThenTheRest(t *testing.T) {
	// the "Default of --…" items are semantic mappings of a settable flag whose default changed:
	// verified, they are pin sources. Scope them in by product.
	probe := newTriageRig(t, BlockingScope{}, nil)
	var pinProduct domain.ProductID
	var pins []string
	for _, id := range probe.routine() {
		it, _ := probe.q.ItemRecord(id)
		pinProduct = it.Product
		pins = append(pins, id)
	}
	if len(pins) == 0 {
		t.Fatal("no routine fixtures")
	}
	r := newTriageRig(t, BlockingScope{Products: []domain.ProductID{pinProduct}}, nil)
	_, body := r.get("/")
	ids := cardOrder(body)
	tr, _ := func() (map[string]Triage, error) {
		snap, _ := r.q.Snapshot(context.Background())
		return triageItems(snap, BlockingScope{Products: []domain.ProductID{pinProduct}}), nil
	}()
	rank := func(id string) int {
		switch x := tr[id]; {
		case x.PinSource:
			return 0
		case x.Blocking:
			return 1
		}
		return 2
	}
	if !slices.IsSortedFunc(ids, func(a, b string) int { return rank(a) - rank(b) }) {
		t.Errorf("not blocking-first: %v", ids)
	}
	nPin := 0
	for _, id := range ids {
		if tr[id].PinSource {
			nPin++
		}
	}
	if nPin == 0 {
		t.Fatalf("no pin-source item among %d cards; fixtures changed?", len(ids))
	}
	for _, id := range ids[:nPin] {
		if !tr[id].PinSource {
			t.Errorf("%s is ranked among the first %d but is not a pin source", id, nPin)
		}
	}
	contains(t, body, "Blocks investigations.", pinSourceWhy, "Blocking investigations")
	// the generic rule: an unscoped product ranks nothing first
	r2 := newTriageRig(t, BlockingScope{}, nil)
	_, b2 := r2.get("/")
	lacks(t, b2, "Blocks investigations.")
	// the Blocking filter narrows to the blockers and keeps the counts
	_, only := r.get("/?blocking=yes")
	if got := len(cardOrder(only)); got == 0 || got > len(ids) {
		t.Errorf("blocking filter lists %d of %d", got, len(ids))
	}
	for _, id := range cardOrder(only) {
		if !tr[id].Blocking {
			t.Errorf("%s listed under the blocking filter but is not blocking", id)
		}
	}
	contains(t, only, "<b>"+itoa(len(ids))+"</b><span>Needs review</span>")
}

func TestPriorityStaysInsideEachRankAndCountsIgnoreIt(t *testing.T) {
	r := newTriageRig(t, BlockingScope{}, nil)
	_, body := r.get("/")
	ids := cardOrder(body)
	hi := map[string]bool{}
	for _, id := range ids {
		it, _ := r.q.ItemRecord(id)
		hi[id] = it.Routing.Priority == domain.PriorityHigh
	}
	if len(ids) < 3 || !hi[ids[0]] {
		t.Errorf("highest priority first expected, got %v", ids)
	}
	_, high := r.get("/?priority=high")
	contains(t, high, "<b>2</b><span>High priority</span>", "<b>"+itoa(len(ids))+"</b><span>Needs review</span>")
	if got := len(cardOrder(high)); got != 2 {
		t.Errorf("priority=high lists %d", got)
	}
}

func TestDecidingLeavesTheDefaultView(t *testing.T) {
	r := newTriageRig(t, BlockingScope{}, nil)
	_, before := r.get("/")
	n := len(cardOrder(before))
	id := r.id(tRBAC)
	r.get("/items/" + id + "/detail")
	st, _, _ := r.post("/items/"+id+"/decision", form("action", "accept", "reviewer", "dana", "started", started, "return", "/"))
	if st != http.StatusSeeOther {
		t.Fatalf("decision status %d", st)
	}
	_, after := r.get("/")
	if got := len(cardOrder(after)); got != n-1 || slices.Contains(cardOrder(after), id) {
		t.Errorf("after deciding: %d cards (was %d), item still listed=%v", got, n, slices.Contains(cardOrder(after), id))
	}
	contains(t, after, "Needs review: "+itoa(n-1))
	_, all := r.get("/?scope=all&status=all")
	contains(t, all, "Not in Needs review: decided by dana")
	// the recorded decision is the Phase 3 contract: human, reviewer named
	last := r.q.Decisions()[len(r.q.Decisions())-1]
	if last.ReviewerKind != domain.ReviewerHuman || last.Reviewer != "dana" {
		t.Errorf("decision recorded as %s/%s", last.Reviewer, last.ReviewerKind)
	}
}

func TestRetractedAuditAndSupersededAreHidden(t *testing.T) {
	it := domain.ReviewItem{ID: "ri-x", CandidateID: "cand-x", QuestionType: domain.QuestionClassification,
		Proposed: domain.SemanticAssertion{Statement: "s"}, Status: domain.ReviewPending}
	fact := func(st domain.FactStatus) domain.VerifiedFact {
		return domain.VerifiedFact{ID: "fact-x", Candidates: []string{"cand-x"}, Assertion: it.Proposed, Status: st}
	}
	for _, st := range []domain.FactStatus{domain.FactRetracted, domain.FactSuperseded} {
		if got := triageOne(it, domain.SemanticCandidate{}, nil, []domain.VerifiedFact{fact(st)}, BlockingScope{}); got.NeedsReview || got.Hidden != HiddenSuperseded {
			t.Errorf("audit of a %s fact: %+v", st, got)
		}
	}
	if got := triageOne(it, domain.SemanticCandidate{}, nil, []domain.VerifiedFact{fact(domain.FactActive)}, BlockingScope{}); !got.NeedsReview {
		t.Errorf("audit of an active, unverified fact must need review: %+v", got)
	}
	it.Status = domain.ReviewSuperseded
	if got := triageOne(it, domain.SemanticCandidate{}, nil, nil, BlockingScope{}); got.Hidden != HiddenSuperseded {
		t.Errorf("superseded item: %+v", got)
	}
	// verified deterministically also hides (aspect-wise)
	it2 := domain.ReviewItem{ID: "ri-y", CandidateID: "cand-y", QuestionType: domain.QuestionApplicability, Status: domain.ReviewPending}
	f := domain.VerifiedFact{Candidates: []string{"cand-y"}, Status: domain.FactActive,
		Verification: []domain.AspectVerification{{Aspect: domain.AspectApplicability, Level: domain.VerifiedDeterministic}}}
	if got := triageOne(it2, domain.SemanticCandidate{}, nil, []domain.VerifiedFact{f}, BlockingScope{}); got.Hidden != HiddenVerified {
		t.Errorf("deterministic fact: %+v", got)
	}
	f.Verification[0].Level = domain.VerifiedProxy
	if got := triageOne(it2, domain.SemanticCandidate{}, nil, []domain.VerifiedFact{f}, BlockingScope{}); !got.NeedsReview {
		t.Errorf("a proxy-level fact must not hide the item: %+v", got)
	}
}

func TestWithoutASnapshotTheInboxListsEverythingAsBefore(t *testing.T) {
	dq := NewDemoQueue()
	// a Queue that is not a SnapshotSource and no Options.Snapshot
	srv := httptest.NewServer(NewServer(struct{ knowledge.Queue }{dq}, Options{Now: func() time.Time { return now }, Logf: t.Logf}))
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	b := new(strings.Builder)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	lacks(t, b.String(), "Show all staged data", "Needs review:")
	contains(t, b.String(), "<span>Pending</span>")
}

func TestScopeFromInvestigationsReadsInputsAndProductSources(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, s string) {
		p := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("inv/case-a/input.yaml", "id: a\nproduct: chart-x\n")
	write("inv/case-b/input.yaml", "id: b\nproduct: chart-x\n")
	// a case's expected.yaml is never read
	write("inv/case-a/expected.yaml", "product: secret-product\n")
	write("prod/chart-x.yaml", "sources:\n  - locator:\n      kind: github-releases\n      repository: acme/operator\n  - locator:\n      repository: 'acme/chart'\n")
	sc := ScopeFromInvestigations(filepath.Join(dir, "inv"), filepath.Join(dir, "prod"))
	if !slices.Equal(sc.Products, []domain.ProductID{"chart-x"}) || !slices.Equal(sc.Sources, []string{"acme/operator", "acme/chart"}) {
		t.Errorf("scope = %+v", sc)
	}
	cand := domain.SemanticCandidate{Evidence: []domain.Evidence{{URI: "https://github.com/acme/operator/blob/v1/CHANGELOG.md"}}}
	if !sc.covers("other-product", cand) || sc.covers("other-product", domain.SemanticCandidate{}) {
		t.Error("an investigated product's bundled upstream repo must be covered, nothing else")
	}
	if !ScopeFromInvestigations(filepath.Join(dir, "nope"), dir).Empty() {
		t.Error("missing directories give an empty scope")
	}
}

func TestCountNeedsReview(t *testing.T) {
	q := NewDemoQueue()
	snap, _ := q.Snapshot(context.Background())
	needs, blocking := CountNeedsReview(snap, BlockingScope{})
	if needs != 10 || blocking != 0 {
		t.Errorf("needs=%d blocking=%d, want 10/0", needs, blocking)
	}
}
