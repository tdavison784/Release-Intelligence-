package reportui

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/classify"
	"github.com/tdavison784/release-intelligence/internal/investigate/run"
	"github.com/tdavison784/release-intelligence/internal/investigate/run/runtest"
)

// --- fixtures -------------------------------------------------------------------------------

// stored runs a real investigation (real bundle, relations and store
// packages; the run package's fake validator/classifier) to the point where
// it awaits review, and returns the state directory and its id.
func stored(t *testing.T) (string, string) {
	t.Helper()
	f := runtest.New(t)
	r := &run.Runner{P: run.New(f.Deps), Sleep: func(time.Duration) {}}
	res, err := r.Run(context.Background(), f.Request())
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != run.OutcomeAwaitingReview {
		t.Fatalf("fixture outcome %s", res.Outcome)
	}
	return filepath.Dir(f.St.Dir()), res.Investigation
}

func liveServer(t *testing.T) (*server, *StoreSource, string) {
	t.Helper()
	root, id := stored(t)
	src, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ss, ok := src.(*StoreSource)
	if !ok {
		t.Fatalf("Open(state dir) = %T, want the live store", src)
	}
	s := newServer(ss, Options{Reviewer: classify.NewReviewApplier(), Now: func() time.Time { return runtest.FixedNow.Add(time.Hour) }, Logf: t.Logf})
	return s, ss, id
}

// memSource is an in-memory Source for view tests.
type memSource struct {
	recs map[string]*Record
	arts map[string][]byte
}

func (m *memSource) Keys(context.Context) ([]string, error) {
	var ks []string
	for k := range m.recs {
		ks = append(ks, k)
	}
	return ks, nil
}
func (m *memSource) Load(_ context.Context, k string) (*Record, error) {
	r, ok := m.recs[k]
	if !ok {
		return nil, fmt.Errorf("%s: %w", k, investigate.ErrNotFound)
	}
	return r, nil
}
func (m *memSource) Artifact(_ context.Context, _, d string) ([]byte, error) {
	if b, ok := m.arts[d]; ok {
		return b, nil
	}
	return nil, investigate.ErrNotFound
}
func (m *memSource) Has(_, d string) bool  { _, ok := m.arts[d]; return ok }
func (m *memSource) Version(string) string { return "" }
func (m *memSource) Writable() bool        { return false }
func (m *memSource) Describe() string      { return "memory" }
func (m *memSource) Update(context.Context, string, func(*domain.Investigation) error) error {
	return ErrReadOnly
}

var aiProv = domain.Provenance{Method: domain.MethodAI, Producer: "investigate.agent@v1", Confidence: domain.ConfidenceLow,
	Model: "model-x", Provider: "anthropic", PromptVersion: "investigate-agent/v5", CallID: "call-1"}

// synthetic is a record with one finding per class (titles out of order)
// and agent output about the potentially-affected item.
func synthetic() *Record {
	det := domain.Provenance{Method: domain.MethodComputed, Producer: "classify.policy@v1", Confidence: domain.ConfidenceHigh}
	finding := func(item string, c domain.FindingClass, rule string, vals ...string) domain.InvestigationFinding {
		return domain.InvestigationFinding{ID: "inf-" + item, ChangeItem: item, Class: c, Rule: rule, Reasoning: "reasoning of " + item,
			Environment: "shop", Validations: vals, Provenance: det}
	}
	inv := &domain.Investigation{ID: "inv-synthetic", Environment: "shop", Status: domain.InvestigationAwaiting, CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Findings: []domain.InvestigationFinding{
			finding("ci-na", domain.FindingNotAffected, "classify:validated-clear", "vr-pass"),
			finding("ci-ie", domain.FindingInsufficientEvidence, "classify:insufficient-evidence"),
			finding("ci-pa2", domain.FindingPotentiallyAffected, "classify:plausible-impact"),
			finding("ci-af", domain.FindingAffected, "classify:validated-exposure", "vr-fail"),
			finding("ci-pa1", domain.FindingPotentiallyAffected, "classify:plausible-impact"),
		},
		Validations: []domain.ValidationRecord{
			{ID: "vr-fail", Validator: "validate.static-reference@v1", Kind: domain.ValidationStaticReference, Level: domain.LevelDeterministic,
				ChangeItem: "ci-af", Outcome: domain.ValidationFail, Property: "references resolve", Detail: "Secret x dangles at TO\nmore", Provenance: det},
			{ID: "vr-pass", Validator: "validate.render-diff@v1", Kind: domain.ValidationRenderDiff, Level: domain.LevelDeterministic,
				ChangeItem: "ci-na", Outcome: domain.ValidationPass, Property: "VALIDATED-PROPERTY-TEXT", Provenance: det},
		},
		Assertions: []domain.AgentAssertion{{ID: "asr-1", ChangeItem: "ci-pa1", Stance: domain.StanceRaisesConcern,
			Statement: "AGENT-CONCERN-TEXT the scrape may fail", StatementKind: "unverified-ai-prose", Provenance: aiProv}},
		AgentClaims: []domain.SettingClaim{{ID: "clm-1", ChangeItem: "ci-pa1", Kind: "absent", Setting: "createTokenSecret", Input: "environment-files",
			Verifier: "investigate.claim-check@v1", Detail: "absent everywhere", Provenance: aiProv}},
		Decision: &domain.PolicyDecision{Policy: "classify.policy@v1", Outcome: domain.PolicyHumanReview, Reasons: []string{"4 of 5 not eligible"}},
		Review:   domain.HumanReview{State: domain.ReviewAwaiting},
	}
	inv.AppendAudit(domain.AuditEvent{At: inv.CreatedAt, Stage: domain.StageClassify, Event: "completed", Actor: "system"})
	b := &domain.EvidenceBundle{ID: "bnd-x", Change: domain.ChangeEvidenceRecord{Product: domain.ProductRef{ID: "acme"},
		From: domain.Version{Tag: "v1"}, To: domain.Version{Tag: "v2"}, Items: []domain.ChangeItem{
			{ID: "ci-pa1", Title: "b second potentially", InScope: true}, {ID: "ci-pa2", Title: "a first potentially", InScope: true},
			{ID: "ci-af", Title: "affected item", InScope: true}, {ID: "ci-na", Title: "cleared item", InScope: true}, {ID: "ci-ie", Title: "unknown item", InScope: true},
		}}, Snapshot: domain.EnvironmentSnapshot{Environment: "shop"}}
	return &Record{Key: "k1", Investigation: inv, Bundle: b, Origin: "memory"}
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func post(t *testing.T, h http.Handler, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// --- list and filters -----------------------------------------------------------------------

func TestFilterAndNewestFirst(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC) }
	rows := []Row{
		{Key: "a", Product: "acme", Env: "eu", Date: d(1), Outcome: domain.PolicyHumanReview, RunID: "r1"},
		{Key: "b", Product: "acme", Env: "us", Date: d(3), Outcome: domain.PolicyAutoApproveEligible, RunID: "r2"},
		{Key: "c", Product: "web", Env: "eu", Date: d(2), Outcome: domain.PolicyHumanReview, RunID: "r1"},
		{Key: "d", Product: "web", Env: "us", Date: d(4)},
	}
	keys := func(rs []Row) string {
		var ks []string
		for _, r := range rs {
			ks = append(ks, r.Key)
		}
		return strings.Join(ks, ",")
	}
	for _, tc := range []struct {
		f    Filter
		want string
	}{
		{Filter{}, "d,b,c,a"},
		{Filter{Product: "acme"}, "b,a"},
		{Filter{Env: "eu"}, "c,a"},
		{Filter{Decision: DecisionReview}, "c,a"},
		{Filter{Decision: DecisionEligible}, "b"},
		{Filter{Decision: DecisionNone}, "d"},
		{Filter{Run: "r1", Product: "web"}, "c"},
	} {
		if got := keys(tc.f.Apply(rows)); got != tc.want {
			t.Errorf("%+v: got %s, want %s", tc.f, got, tc.want)
		}
	}
}

func TestListPageFromTheStore(t *testing.T) {
	s, _, id := liveServer(t)
	w := get(t, s.handler, "/investigations")
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	body := w.Body.String()
	for _, want := range []string{id, "acme", "v1.0.0", "v2.0.0", "shop-prod", "needs human review", "awaiting review", "Insufficient evidence"} {
		if !strings.Contains(body, want) {
			t.Errorf("list lacks %q", want)
		}
	}
	if w := get(t, s.handler, "/investigations?product=nope"); !strings.Contains(w.Body.String(), "No investigation matches") {
		t.Error("a filter matching nothing must say so")
	}
}

// --- the investigation page -----------------------------------------------------------------

func TestItemsAttentionFirst(t *testing.T) {
	v := Build(context.Background(), synthetic(), Links{Base: "/investigations"}, nil, nil)
	var got []string
	for _, g := range v.Groups {
		for _, it := range g.Items {
			got = append(got, it.Item)
		}
		if g.Open == (g.Class == domain.FindingNotAffected) {
			t.Errorf("group %s open=%v: only not-affected is collapsed", g.Class, g.Open)
		}
	}
	if want := "ci-af,ci-pa2,ci-pa1,ci-ie,ci-na"; strings.Join(got, ",") != want {
		t.Fatalf("order %v, want %s (affected → potentially → insufficient → not-affected; by title within a class)", got, want)
	}
	if r := v.Groups[0].Items[0].Reason; !strings.Contains(r, "validate.static-reference@v1 failed") || strings.Contains(r, "more") {
		t.Errorf("affected reason %q: must name the failing validation, first line only", r)
	}
	if v.Headline != "Needs human review" || !strings.Contains(strings.Join(v.Decision, " "), "4 of 5 in-scope change item(s) are not cleared") {
		t.Errorf("decision %q %v", v.Headline, v.Decision)
	}
}

func TestAIOutputLabelledAndSeparate(t *testing.T) {
	src := &memSource{recs: map[string]*Record{"k1": synthetic()}}
	s := newServer(src, Options{Logf: t.Logf})
	w := get(t, s.handler, "/investigations/view/k1")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	body := w.Body.String()
	// Every AI statement sits inside an AI box, labelled with its model and call.
	boxes := regexp.MustCompile(`(?s)<div class="ai-box">.*?</div>\s*</div>\s*</details>`).FindAllString(body, -1)
	joined := strings.Join(boxes, "\n")
	for _, ai := range []string{"AGENT-CONCERN-TEXT", "createTokenSecret"} {
		if strings.Count(body, ai) != strings.Count(joined, ai) || !strings.Contains(joined, ai) {
			t.Errorf("%q appears outside the labelled AI box", ai)
		}
	}
	for _, lab := range []string{"AI OUTPUT — advisory, not a validation", "AI · concern", "unverified AI prose", "AI · model-x (anthropic) · prompt investigate-agent/v5 · call call-1", "AI · verified claim"} {
		if !strings.Contains(joined, lab) {
			t.Errorf("AI box lacks label %q", lab)
		}
	}
	// Validated results never appear inside an AI box.
	if strings.Contains(joined, "VALIDATED-PROPERTY-TEXT") || !strings.Contains(body, "VALIDATED-PROPERTY-TEXT") {
		t.Error("validation records must be shown, and outside the AI box")
	}
	if !strings.Contains(body, "not answered by any passing validation") {
		t.Error("an unanswered concern must say it blocks clearing")
	}
}

// --- human review ---------------------------------------------------------------------------

func TestReviewRecordedAsHumanWithReason(t *testing.T) {
	s, ss, id := liveServer(t)
	view := "/investigations/view/" + id
	if w := get(t, s.handler, view); !strings.Contains(w.Body.String(), `id="review-form"`) {
		t.Fatal("a writable store awaiting review must offer the review form")
	}
	// Missing reason, missing reviewer: refused, nothing recorded.
	for _, f := range []url.Values{
		{"action": {"approve"}, "reviewer": {"alice"}},
		{"action": {"approve"}, "note": {"looks fine"}},
		{"action": {"bogus"}, "reviewer": {"alice"}, "note": {"x"}},
	} {
		if w := post(t, s.handler, "/investigations/review/"+id, f); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Not recorded") {
			t.Fatalf("%v: %d", f, w.Code)
		}
	}
	rec, _ := ss.Load(context.Background(), id)
	if rec.Investigation.Review.State != domain.ReviewAwaiting {
		t.Fatalf("a refused decision changed the review: %+v", rec.Investigation.Review)
	}
	n := len(rec.Investigation.Audit)

	// A posted reviewerKind is ignored: the form records a human.
	w := post(t, s.handler, "/investigations/review/"+id, url.Values{"action": {"approve"}, "reviewer": {"alice"},
		"note": {"render diff reviewed; safe for shop-prod"}, "reviewerKind": {"proxy"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("approve: %d %s", w.Code, w.Body)
	}
	rec, _ = ss.Load(context.Background(), id)
	inv := rec.Investigation
	r := inv.Review
	if r.State != domain.ReviewApproved || r.ReviewerKind != domain.ReviewerHuman || r.Reviewer != "alice" || r.Note != "render diff reviewed; safe for shop-prod" {
		t.Fatalf("review %+v", r)
	}
	if inv.Status != domain.InvestigationCompleted {
		t.Errorf("status %s, want completed", inv.Status)
	}
	last := inv.Audit[len(inv.Audit)-1]
	if len(inv.Audit) != n+1 || last.Actor != "human:alice" || last.Stage != domain.StageHumanReview || last.Event != "decided" || !strings.Contains(last.Detail, "safe for shop-prod") {
		t.Fatalf("audit %+v", last)
	}
	if err := inv.Validate(); err != nil {
		t.Fatalf("stored record invalid: %v", err)
	}
	page := get(t, s.handler, w.Header().Get("Location")).Body.String()
	for _, want := range []string{"Recorded: review is now approved", "human:alice", "hash chain intact", "Approved by alice (human)"} {
		if !strings.Contains(page, want) {
			t.Errorf("page after review lacks %q", want)
		}
	}
	if strings.Contains(page, `id="review-form"`) {
		t.Error("a decided review is final: no form")
	}
	// Approving again is refused by the ReviewApplier.
	if w := post(t, s.handler, "/investigations/review/"+id, url.Values{"action": {"reject"}, "reviewer": {"bob"}, "note": {"x"}}); w.Code != http.StatusBadRequest {
		t.Errorf("second decision: %d", w.Code)
	}
}

func TestMoreEvidenceRequest(t *testing.T) {
	s, ss, id := liveServer(t)
	w := post(t, s.handler, "/investigations/review/"+id, url.Values{"action": {"more"}, "reviewer": {"alice"},
		"note": {"prod-pl values missing"}, "requests": {"values:shop/prod-pl.yaml, values:shop/common.yaml"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	rec, _ := ss.Load(context.Background(), id)
	last := rec.Investigation.Audit[len(rec.Investigation.Audit)-1]
	if rec.Investigation.Review.State != domain.ReviewMoreEvidenceWanted || !strings.Contains(last.Detail, "values:shop/common.yaml") {
		t.Fatalf("review %+v audit %+v", rec.Investigation.Review, last)
	}
}

func TestProxyCannotApprove(t *testing.T) {
	s, ss, id := liveServer(t)
	p := aiProv
	at := runtest.FixedNow.Add(time.Hour)
	p.GeneratedAt = &at
	p.InputEvidence = []domain.EvidenceID{"ev-x"}
	p.ModelVersion, p.PromptDigest = "model-x-1", "sha256:"+strings.Repeat("a", 64)
	err := s.Decide(context.Background(), id, investigate.ReviewDecision{Action: investigate.ReviewApprove, Reviewer: "claude-proxy",
		ReviewerKind: domain.ReviewerProxy, At: at, Note: "looks safe", ProxyProvenance: &p})
	if err != nil {
		t.Fatalf("a proxy decision is recorded (not refused): %v", err)
	}
	rec, _ := ss.Load(context.Background(), id)
	if st := rec.Investigation.Review.State; st == domain.ReviewApproved {
		t.Fatal("a proxy approved the investigation")
	}
	page := get(t, s.handler, "/investigations/view/"+id).Body.String()
	if !strings.Contains(page, "AI reviewer — recorded, never approves") || !strings.Contains(page, `id="review-form"`) {
		t.Error("the proxy decision must be shown as AI, and the human form must stay open")
	}
}

func TestReadOnlyRunsAndCrossOrigin(t *testing.T) {
	root, id := stored(t)
	runDir := packRun(t, root, id, true)
	src, err := Open(filepath.Dir(runDir))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := src.(*RunsSource); !ok {
		t.Fatalf("Open(results dir) = %T", src)
	}
	s := newServer(src, Options{Reviewer: classify.NewReviewApplier(), Logf: t.Logf})
	key := "R-1/case-a"
	page := get(t, s.handler, "/investigations/view/"+key)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "stored evaluation run (read-only)") || strings.Contains(page.Body.String(), `id="review-form"`) {
		t.Fatalf("read-only page: %d", page.Code)
	}
	if !strings.Contains(get(t, s.handler, "/investigations").Body.String(), "R-1 · arm A") {
		t.Error("list must name the evaluation run")
	}
	w := post(t, s.handler, "/investigations/review/"+key, url.Values{"action": {"approve"}, "reviewer": {"alice"}, "note": {"x"}})
	if w.Code != http.StatusForbidden {
		t.Errorf("review on a stored run: %d", w.Code)
	}
	r := httptest.NewRequest(http.MethodPost, "/investigations/review/"+key, strings.NewReader("action=approve"))
	r.Header.Set("Origin", "https://evil.example")
	rw := httptest.NewRecorder()
	s.handler.ServeHTTP(rw, r)
	if rw.Code != http.StatusForbidden || !strings.Contains(rw.Body.String(), "cross-origin") {
		t.Errorf("cross-origin post: %d", rw.Code)
	}
	// The unpacked layout reads the same.
	unpacked := packRun(t, root, id, false)
	u, err := Open(unpacked)
	if err != nil {
		t.Fatal(err)
	}
	if ks, _ := u.Keys(context.Background()); len(ks) != 1 || ks[0] != "R-1/case-a" {
		t.Errorf("unpacked keys %v", ks)
	}
}

// packRun writes a stored evaluation run (run.json + cases, packed or not)
// holding the fixture's investigation and bundle; it returns the run dir.
func packRun(t *testing.T, root, id string, packed bool) string {
	t.Helper()
	ss, err := NewStoreSource(root)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := ss.Load(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	inv, _ := json.Marshal(rec.Investigation)
	bnd, _ := json.Marshal(rec.Bundle)
	dir := filepath.Join(t.TempDir(), "results", "R-1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "run.json"), []byte(`{"runId":"R-1","arm":"A","startedAt":"2026-10-16T12:00:00Z"}`), 0o644)
	files := map[string][]byte{"cases/case-a/investigation.json": inv, "cases/case-a/bundle.json": bnd}
	if !packed {
		for n, b := range files {
			p := filepath.Join(dir, filepath.FromSlash(n))
			os.MkdirAll(filepath.Dir(p), 0o755)
			if err := os.WriteFile(p, b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for n, b := range files {
		tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: int64(len(b)), Typeflag: tar.TypeReg})
		tw.Write(b)
	}
	tw.Close()
	zw.Close()
	if err := os.WriteFile(filepath.Join(dir, "cases.tar.gz"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// --- export ---------------------------------------------------------------------------------

func TestExportSelfContained(t *testing.T) {
	s, ss, id := liveServer(t)
	w := get(t, s.handler, "/investigations/export/"+id+"?format=html")
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("export: %d %v", w.Code, w.Header())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<style>") || !strings.Contains(body, "--surface") {
		t.Error("the stylesheet must be inlined")
	}
	// Nothing may depend on the server: no stylesheet/script links, no
	// links into the app, no form.
	for _, re := range []string{`<link\b`, `<script\b`, `\bsrc=`, `href="/`, `<form\b`, `href="#review"`} {
		if regexp.MustCompile(re).MatchString(body) {
			t.Errorf("export contains %s", re)
		}
	}
	m := regexp.MustCompile(`href="data:application/json;base64,([A-Za-z0-9+/=]+)" download="([^"]+)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("the run record must travel inside the export")
	}
	raw, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := ss.Load(context.Background(), id)
	want, _ := RecordJSON(rec)
	if !bytes.Equal(raw, want) || !strings.HasPrefix(m[2], id) {
		t.Error("embedded run record differs from the stored one")
	}
	for _, want := range []string{id, "What was checked", "Audit trail", "Needs human review"} {
		if !strings.Contains(body, want) {
			t.Errorf("export lacks %q", want)
		}
	}
	j := get(t, s.handler, "/investigations/export/"+id+"?format=json")
	var inv domain.Investigation
	if err := json.Unmarshal(j.Body.Bytes(), &inv); err != nil || inv.ID != id || inv.Validate() != nil {
		t.Fatalf("json export: %v", err)
	}
}

func TestArtifactServedAsText(t *testing.T) {
	s, ss, id := liveServer(t)
	rec, _ := ss.Load(context.Background(), id)
	a := rec.Bundle.Manifest.Artifacts[0]
	w := get(t, s.handler, "/investigations/artifact/"+id+"?digest="+url.QueryEscape(a.Digest))
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("artifact: %d %v", w.Code, w.Header())
	}
	if w := get(t, s.handler, "/investigations/artifact/"+id+"?digest=sha256:"+strings.Repeat("0", 64)); w.Code != http.StatusNotFound {
		t.Errorf("missing artifact: %d", w.Code)
	}
	if w := get(t, s.handler, "/investigations/view/inv-nope"); w.Code != http.StatusNotFound {
		t.Errorf("missing investigation: %d", w.Code)
	}
}
