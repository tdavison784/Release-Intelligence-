package reportui

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// NavLink is an extra link in the top bar (e.g. the knowledge review inbox
// when mounted inside `ri review serve`).
type NavLink struct {
	Label, Href string
}

// Options configure the server.
type Options struct {
	// Base is the path prefix the handler is mounted at (default
	// "/investigations"); every route lives below it.
	Base string
	// Reviewer applies human-review decisions (classify.NewReviewApplier()).
	// Without it the pages are read-only.
	Reviewer investigate.ReviewApplier
	// DefaultReviewer pre-fills the reviewer name (it is still required on
	// every decision).
	DefaultReviewer string
	// Nav adds links to the top bar.
	Nav []NavLink
	// Now is the clock (default time.Now).
	Now func() time.Time
	// Logf receives server-side problems (default log.Printf).
	Logf func(format string, args ...any)
}

type server struct {
	src  Source
	opts Options
	tpl  *template.Template
	css  template.CSS

	handler http.Handler

	mu   sync.Mutex
	rows map[string]cachedRow
}

type cachedRow struct {
	version string
	row     Row
}

// NewServer returns the investigation report UI. Routes (below Base):
//
//	GET  {Base}                       investigations list (filters: product, env, decision, run)
//	GET  {Base}/view/{key}            one investigation
//	POST {Base}/review/{key}          human decision: approve | reject | more
//	GET  {Base}/export/{key}?format=  html (self-contained report) | json (run record)
//	GET  {Base}/artifact/{key}?digest= an artifact's bytes (as text), when the store holds them
//	GET  {Base}/static/report.css
func NewServer(src Source, opts Options) http.Handler { return newServer(src, opts).handler }

func newServer(src Source, opts Options) *server {
	if opts.Base == "" {
		opts.Base = "/investigations"
	}
	opts.Base = "/" + strings.Trim(opts.Base, "/")
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logf == nil {
		opts.Logf = log.Printf
	}
	css, err := staticFS.ReadFile("static/report.css")
	if err != nil {
		panic(err)
	}
	s := &server{src: src, opts: opts, css: template.CSS(css), rows: map[string]cachedRow{}}
	s.tpl = template.Must(template.New("").Funcs(funcs()).ParseFS(templateFS, "templates/*.html"))
	b := opts.Base
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET "+b+"/static/", http.StripPrefix(b+"/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET "+b, s.list)
	mux.HandleFunc("GET "+b+"/{$}", s.list)
	mux.HandleFunc("GET "+b+"/view/{key...}", s.view)
	mux.HandleFunc("POST "+b+"/review/{key...}", s.review)
	mux.HandleFunc("GET "+b+"/export/{key...}", s.export)
	mux.HandleFunc("GET "+b+"/artifact/{key...}", s.artifact)
	s.handler = sameOrigin(mux)
	return s
}

// sameOrigin refuses cross-site form posts: there is no authentication (the
// server is a local tool), but a web page must not submit decisions to it.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if o := r.Header.Get("Origin"); o != "" {
				u, err := url.Parse(o)
				if err != nil || u.Host != r.Host {
					http.Error(w, "cross-origin request refused", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) links() Links { return Links{Base: s.opts.Base} }

func (s *server) writable() bool { return s.src.Writable() && s.opts.Reviewer != nil }

// --- list ------------------------------------------------------------------------------------

type listData struct {
	Title   string
	Base    string
	Nav     []NavLink
	Source  string
	Rows    []Row
	All     int
	Filter  Filter
	Options struct {
		Products, Envs []string
		Runs           []Option
	}
	Totals   []ClassCount
	Problems []string
}

// Option is a select option.
type Option struct{ Value, Label string }

// Rows returns every investigation's list row (cached per record version).
func (s *server) Rows(ctx context.Context) ([]Row, []string, error) {
	keys, err := s.src.Keys(ctx)
	if err != nil {
		return nil, nil, err
	}
	var rows []Row
	var problems []string
	for _, k := range keys {
		ver := s.src.Version(k)
		s.mu.Lock()
		c, ok := s.rows[k]
		s.mu.Unlock()
		if ok && c.version == ver {
			rows = append(rows, c.row)
			continue
		}
		rec, err := s.src.Load(ctx, k)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", k, err))
			continue
		}
		row := Summarize(rec)
		s.mu.Lock()
		s.rows[k] = cachedRow{version: ver, row: row}
		s.mu.Unlock()
		rows = append(rows, row)
	}
	SortRows(rows)
	return rows, problems, nil
}

func (s *server) list(w http.ResponseWriter, r *http.Request) {
	all, problems, err := s.Rows(r.Context())
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	q := r.URL.Query()
	f := Filter{Product: q.Get("product"), Env: q.Get("env"), Decision: q.Get("decision"), Run: q.Get("run")}
	d := listData{Title: "Investigations", Base: s.opts.Base, Nav: s.opts.Nav, Source: s.src.Describe(), All: len(all), Filter: f, Problems: problems}
	d.Rows = f.Apply(all)
	prods, envs, runs := map[string]bool{}, map[string]bool{}, map[string]string{}
	for _, r := range all {
		prods[r.Product], envs[r.Env] = true, true
		if r.RunID != "" {
			runs[r.RunID] = r.RunLabel
		}
	}
	d.Options.Products, d.Options.Envs = sortedKeys(prods), sortedKeys(envs)
	for _, id := range sortedKeys(boolKeys(runs)) {
		d.Options.Runs = append(d.Options.Runs, Option{Value: id, Label: runs[id]})
	}
	sort.Slice(d.Options.Runs, func(i, j int) bool { return d.Options.Runs[i].Value > d.Options.Runs[j].Value })
	tot := map[domain.FindingClass]int{}
	for _, r := range d.Rows {
		for _, c := range r.Counts {
			tot[c.Class] += c.N
		}
	}
	for _, c := range classOrder {
		d.Totals = append(d.Totals, ClassCount{Class: c, Label: classLabels[c], Tone: classTones[c], N: tot[c]})
	}
	s.render(w, http.StatusOK, "list", d)
}

func sortedKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		if k != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func boolKeys(m map[string]string) map[string]bool {
	out := map[string]bool{}
	for k := range m {
		out[k] = true
	}
	return out
}

// --- one investigation ---------------------------------------------------------------------

type pageData struct {
	Title     string
	Base      string
	Nav       []NavLink
	V         *View
	L         Links
	Offline   bool
	Writable  bool
	Reviewer  string
	Flash     string
	Error     string
	Form      reviewForm
	CSS       template.CSS
	Generated time.Time
	RecordURL template.URL
	RecordAs  string
}

type reviewForm struct {
	Action, Note, Requests string
}

func (s *server) build(ctx context.Context, key string, links Links) (*View, *Record, error) {
	rec, err := s.src.Load(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	has := func(d string) bool { return s.src.Has(key, d) }
	read := func(ctx context.Context, d string) ([]byte, error) { return s.src.Artifact(ctx, key, d) }
	return Build(ctx, rec, links, has, read), rec, nil
}

func (s *server) view(w http.ResponseWriter, r *http.Request) {
	s.page(w, r, http.StatusOK, "", reviewForm{})
}

func (s *server) page(w http.ResponseWriter, r *http.Request, code int, errText string, form reviewForm) {
	key := r.PathValue("key")
	v, _, err := s.build(r.Context(), key, s.links())
	if err != nil {
		s.notFoundOr(w, err)
		return
	}
	d := pageData{Title: v.Product + " " + v.From + " → " + v.To + " · " + v.Env, Base: s.opts.Base, Nav: s.opts.Nav, V: v, L: s.links(),
		Writable: s.writable(), Reviewer: s.opts.DefaultReviewer, Error: errText, Form: form}
	if !s.writable() && v.Review.CanDecide {
		v.Review.CanDecide = false
		v.Review.Why = "This server is read-only for this store."
	}
	if st := r.URL.Query().Get("reviewed"); st != "" && errText == "" {
		d.Flash = "Recorded: review is now " + st + ". The decision is in the review history and the audit trail below."
	}
	if c := r.URL.Query().Get("reviewer"); c != "" && d.Reviewer == "" {
		d.Reviewer = c
	}
	s.render(w, code, "page", d)
}

// review records a human decision through the ReviewApplier. The reviewer
// kind is always human here: this form is how a person decides. An AI
// reviewer's decision is recorded by tooling that sets ReviewerKind itself,
// and the ReviewApplier never lets it approve.
func (s *server) review(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if err := r.ParseForm(); err != nil {
		s.fail(w, http.StatusBadRequest, err)
		return
	}
	form := reviewForm{Action: r.PostForm.Get("action"), Note: strings.TrimSpace(r.PostForm.Get("note")), Requests: strings.TrimSpace(r.PostForm.Get("requests"))}
	reviewer := strings.TrimSpace(r.PostForm.Get("reviewer"))
	d, err := s.decision(form, reviewer)
	if err == nil && !s.writable() {
		err = ErrReadOnly
	}
	if err == nil {
		err = s.Decide(r.Context(), key, d)
	}
	if err != nil {
		if errors.Is(err, investigate.ErrNotFound) {
			s.notFoundOr(w, err)
			return
		}
		code := http.StatusBadRequest
		if errors.Is(err, ErrReadOnly) {
			code = http.StatusForbidden
		}
		s.page(w, r, code, err.Error(), form)
		return
	}
	u := s.links().View(key) + "?reviewed=" + url.QueryEscape(string(stateOf(d.Action)))
	http.Redirect(w, r, u, http.StatusSeeOther)
}

func stateOf(a investigate.ReviewAction) domain.ReviewState {
	switch a {
	case investigate.ReviewApprove:
		return domain.ReviewApproved
	case investigate.ReviewReject:
		return domain.ReviewRejected
	}
	return domain.ReviewMoreEvidenceWanted
}

func (s *server) decision(f reviewForm, reviewer string) (investigate.ReviewDecision, error) {
	var a investigate.ReviewAction
	switch f.Action {
	case "approve":
		a = investigate.ReviewApprove
	case "reject":
		a = investigate.ReviewReject
	case "more":
		a = investigate.ReviewRequestMoreEvidence
	default:
		return investigate.ReviewDecision{}, errors.New("choose approve, reject or request more evidence")
	}
	if reviewer == "" {
		return investigate.ReviewDecision{}, errors.New("the reviewer name is required")
	}
	if f.Note == "" {
		return investigate.ReviewDecision{}, errors.New("a reason is required: it is recorded with the decision and in the audit trail")
	}
	d := investigate.ReviewDecision{Action: a, Reviewer: reviewer, ReviewerKind: domain.ReviewerHuman, Note: f.Note, At: s.opts.Now().UTC()}
	if a == investigate.ReviewRequestMoreEvidence {
		for _, p := range strings.Split(f.Requests, ",") {
			if p = strings.TrimSpace(p); p != "" {
				d.Requests = append(d.Requests, p)
			}
		}
		if len(d.Requests) == 0 {
			d.Requests = []string{"see note"}
		}
	}
	return d, nil
}

// Decide applies one review decision to the stored run record through the
// ReviewApplier and persists it. As with `ri investigate review` on a stored
// run, a decided review ends the run (a new round of evidence is a new
// `ri investigate`).
func (s *server) Decide(ctx context.Context, key string, d investigate.ReviewDecision) error {
	if s.opts.Reviewer == nil {
		return ErrReadOnly
	}
	return s.src.Update(ctx, key, func(inv *domain.Investigation) error {
		if err := s.opts.Reviewer.ApplyReview(inv, d); err != nil {
			return err
		}
		switch inv.Review.State {
		case domain.ReviewApproved, domain.ReviewRejected, domain.ReviewMoreEvidenceWanted:
			inv.Status = domain.InvestigationCompleted
		}
		return nil
	})
}

// --- export ----------------------------------------------------------------------------------

func (s *server) export(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	format := r.URL.Query().Get("format")
	if format == "json" {
		rec, err := s.src.Load(r.Context(), key)
		if err != nil {
			s.notFoundOr(w, err)
			return
		}
		raw, err := RecordJSON(rec)
		if err != nil {
			s.fail(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", exportName(rec, "json")))
		w.Write(raw)
		return
	}
	if format != "" && format != "html" {
		s.fail(w, http.StatusBadRequest, fmt.Errorf("format must be html or json"))
		return
	}
	rec, err := s.src.Load(r.Context(), key)
	if err != nil {
		s.notFoundOr(w, err)
		return
	}
	out, err := s.Export(r.Context(), rec)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", exportName(rec, "html")))
	w.Write(out)
}

// RecordJSON is the run record as exported (indented JSON of the stored
// Investigation).
func RecordJSON(rec *Record) ([]byte, error) {
	raw, err := json.MarshalIndent(rec.Investigation, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func exportName(rec *Record, ext string) string {
	inv := rec.Investigation
	name := inv.ID
	if inv.Environment != "" {
		name += "-" + inv.Environment
	}
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return '_'
	}, name) + "." + ext
}

// Export renders the self-contained HTML report of one investigation: the
// stylesheet is inlined, nothing links back into the server, and the run
// record JSON travels inside the file as a download link.
func (s *server) Export(ctx context.Context, rec *Record) ([]byte, error) {
	links := Links{Base: s.opts.Base, Offline: true}
	v := Build(ctx, rec, links, nil, func(ctx context.Context, d string) ([]byte, error) { return s.src.Artifact(ctx, rec.Key, d) })
	raw, err := RecordJSON(rec)
	if err != nil {
		return nil, err
	}
	d := pageData{Title: v.Product + " " + v.From + " → " + v.To + " · " + v.Env, V: v, L: links, Offline: true, CSS: s.css,
		Generated: s.opts.Now().UTC(), RecordURL: template.URL("data:application/json;base64," + base64.StdEncoding.EncodeToString(raw)),
		RecordAs: exportName(rec, "json")}
	var buf bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&buf, "export", d); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// --- artifacts -------------------------------------------------------------------------------

// artifact serves an artifact's bytes as plain text (never as HTML: the
// bytes are upstream or environment content).
func (s *server) artifact(w http.ResponseWriter, r *http.Request) {
	key, digest := r.PathValue("key"), r.URL.Query().Get("digest")
	b, err := s.src.Artifact(r.Context(), key, digest)
	if err != nil {
		s.notFoundOr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Write(b)
}

// --- rendering -------------------------------------------------------------------------------

func (s *server) render(w http.ResponseWriter, code int, name string, data any) {
	var buf bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&buf, name, data); err != nil {
		s.opts.Logf("reportui: render %s: %v", name, err)
		http.Error(w, "internal error rendering the page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	w.Write(buf.Bytes())
}

func (s *server) notFoundOr(w http.ResponseWriter, err error) {
	if errors.Is(err, investigate.ErrNotFound) {
		s.fail(w, http.StatusNotFound, err)
		return
	}
	s.fail(w, http.StatusInternalServerError, err)
}

func (s *server) fail(w http.ResponseWriter, code int, err error) {
	if code >= 500 {
		s.opts.Logf("reportui: %v", err)
	}
	s.render(w, code, "error", struct {
		Title, Base, Text string
		Nav               []NavLink
	}{http.StatusText(code), s.opts.Base, err.Error(), s.opts.Nav})
}

func funcs() template.FuncMap {
	return template.FuncMap{
		"date": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return t.UTC().Format("2006-01-02 15:04 MST")
		},
		"dateptr": func(t *time.Time) string {
			if t == nil {
				return "—"
			}
			return t.UTC().Format("2006-01-02 15:04 MST")
		},
		"short": func(s string) string {
			if h, ok := strings.CutPrefix(s, "sha256:"); ok && len(h) > 12 {
				return "sha256:" + h[:12] + "…"
			}
			return s
		},
		"join": strings.Join,
		"sel":  func(a, b string) bool { return a == b },
		"plural": func(n int, one, many string) string {
			if n == 1 {
				return one
			}
			return many
		},
		"nonzero": func(cs []ClassCount) []ClassCount {
			var out []ClassCount
			for _, c := range cs {
				if c.N > 0 {
					out = append(out, c)
				}
			}
			return out
		},
	}
}
