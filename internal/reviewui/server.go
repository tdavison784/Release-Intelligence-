// Package reviewui is the engineering review UI (`ri review serve`, MISSION
// G7–G10): an inbox with counts and filters, collapsible review items with the
// full G8 detail, and accept / reject / correct / need-more-evidence / defer
// decisions, individually or in a group. It is stdlib only (net/http,
// html/template, embed) and talks to the knowledge store through
// knowledge.Queue.
package reviewui

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Options configure the server.
type Options struct {
	// Now is the clock (default time.Now); tests inject a fixed one.
	Now func() time.Time
	// DefaultReviewer pre-fills the reviewer name (never assumed: the reviewer
	// must still be present on every decision).
	DefaultReviewer string
	// InboxLimit caps the inbox rows (default 200).
	InboxLimit int
	// Logf receives server-side problems (default log.Printf).
	Logf func(format string, args ...any)
}

type server struct {
	q    knowledge.Queue
	opts Options
	tpl  *template.Template
	mux  *http.ServeMux

	mu       sync.Mutex
	sessions map[string]*session
}

// session is the server-side memory of one browser: which items were opened
// (the bulk-accept guard) and the messages to show on the next page.
type session struct {
	expanded map[string]bool
	flashes  []flash
}

type flash struct {
	Level string // ok | error | warn
	Text  string
}

// NewServer returns the review UI. Routes:
//
//	GET  /                          inbox (G7)
//	GET  /items/{id}                item page (G8; deep link)
//	GET  /items/{id}/detail         the item's detail fragment (inline expansion)
//	POST /items/{id}/decision       accept|reject|correct|need-more-evidence|defer
//	POST /bulk                      bulk accept|reject|defer|need-more-evidence
func NewServer(q knowledge.Queue, opts Options) http.Handler {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.InboxLimit <= 0 {
		opts.InboxLimit = 200
	}
	if opts.Logf == nil {
		opts.Logf = log.Printf
	}
	s := &server{q: q, opts: opts, sessions: map[string]*session{}, mux: http.NewServeMux()}
	s.tpl = template.Must(template.New("").Funcs(funcs()).ParseFS(templateFS, "templates/*.html"))
	static, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	s.mux.HandleFunc("GET /{$}", s.inbox)
	s.mux.HandleFunc("GET /items/{id}", s.itemPage)
	s.mux.HandleFunc("GET /items/{id}/detail", s.itemDetail)
	s.mux.HandleFunc("POST /items/{id}/decision", s.decide)
	s.mux.HandleFunc("POST /bulk", s.bulk)
	return s.sameOrigin(s.mux)
}

// sameOrigin refuses cross-site form posts (there is no auth: the server is a
// local tool, but a web page must not be able to submit decisions to it).
func (s *server) sameOrigin(next http.Handler) http.Handler {
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

// --- sessions ----------------------------------------------------------------------

const sessionCookie = "ri_session"

func (s *server) session(w http.ResponseWriter, r *http.Request) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, err := r.Cookie(sessionCookie); err == nil {
		if ss, ok := s.sessions[c.Value]; ok {
			return ss
		}
	}
	if len(s.sessions) > 1000 {
		s.sessions = map[string]*session{}
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	ss := &session{expanded: map[string]bool{}}
	s.sessions[id] = ss
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return ss
}

func (s *server) markExpanded(ss *session, id string) {
	s.mu.Lock()
	ss.expanded[id] = true
	s.mu.Unlock()
}

func (s *server) wasExpanded(ss *session, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ss.expanded[id]
}

func (s *server) addFlash(ss *session, level, text string) {
	s.mu.Lock()
	ss.flashes = append(ss.flashes, flash{level, text})
	s.mu.Unlock()
}

func (s *server) takeFlashes(ss *session) []flash {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := ss.flashes
	ss.flashes = nil
	return f
}

// --- helpers ------------------------------------------------------------------------

const reviewerCookie = "ri_reviewer"

func (s *server) reviewerFrom(r *http.Request) string {
	if v := strings.TrimSpace(r.FormValue("reviewer")); v != "" {
		return v
	}
	if c, err := r.Cookie(reviewerCookie); err == nil {
		if v, err := url.QueryUnescape(c.Value); err == nil && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return s.opts.DefaultReviewer
}

func setReviewerCookie(w http.ResponseWriter, name string) {
	if name != "" {
		http.SetCookie(w, &http.Cookie{Name: reviewerCookie, Value: url.QueryEscape(name), Path: "/", SameSite: http.SameSiteStrictMode, MaxAge: 365 * 24 * 3600})
	}
}

// localPath keeps redirects on this server: only absolute paths.
func localPath(p, def string) string {
	if strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") && !strings.Contains(p, "\\") {
		return p
	}
	return def
}

func (s *server) render(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&buf, name, data); err != nil {
		s.opts.Logf("reviewui: render %s: %v", name, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, knowledge.ErrNotFound) {
		s.render(w, http.StatusNotFound, "error.html", errorView{Title: "Not found", Message: err.Error()})
		return
	}
	s.opts.Logf("reviewui: %s %s: %v", r.Method, r.URL.Path, err)
	s.render(w, http.StatusInternalServerError, "error.html", errorView{Title: "Something went wrong", Message: err.Error()})
}

type errorView struct{ Title, Message string }

// renderedDelta returns the optional render evidence of an item.
func (s *server) renderedDelta(ctx context.Context, id string) *RenderedDelta {
	src, ok := s.q.(RenderedDeltaSource)
	if !ok {
		return nil
	}
	d, err := src.RenderedDelta(ctx, id)
	if err != nil {
		s.opts.Logf("reviewui: rendered delta for %s: %v", id, err)
		return nil
	}
	return d
}

// eligible reports whether an item can still be decided.
func eligible(st domain.ReviewStatus) bool {
	return st == domain.ReviewPending || st == domain.ReviewNeedsEvidence || st == domain.ReviewDeferred
}

// --- inbox -----------------------------------------------------------------------------

func (s *server) inbox(w http.ResponseWriter, r *http.Request) {
	ss := s.session(w, r)
	f, form := parseFilter(r.URL.Query(), s.opts.InboxLimit)
	in, err := s.q.Inbox(r.Context(), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reviewer := s.reviewerFrom(r)
	v := inboxView{
		Filter: form, Counts: in.Counts, Tiles: tiles(in.Counts), Flashes: s.takeFlashes(ss),
		Reviewer: reviewer, Started: s.opts.Now().UTC().Format(time.RFC3339Nano),
		Return: r.URL.RequestURI(), Total: len(in.Items),
		Enums: enums(),
	}
	for _, row := range in.Items {
		v.Rows = append(v.Rows, newRowView(row, r.URL.RequestURI()))
	}
	s.render(w, http.StatusOK, "inbox.html", v)
}

// --- item page and fragment ------------------------------------------------------

func (s *server) itemView(r *http.Request, id string, form map[string][]string, errs []string) (*itemView, error) {
	rc, err := s.q.Item(r.Context(), id)
	if err != nil {
		return nil, err
	}
	v := newItemView(rc, s.renderedDelta(r.Context(), id), form)
	v.Reviewer = s.reviewerFrom(r)
	v.Started = s.opts.Now().UTC().Format(time.RFC3339Nano)
	v.Return = localPath(r.FormValue("return"), "/")
	v.Errors = errs
	v.Enums = enums()
	return v, nil
}

func (s *server) itemPage(w http.ResponseWriter, r *http.Request) {
	ss := s.session(w, r)
	id := r.PathValue("id")
	v, err := s.itemView(r, id, nil, nil)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.markExpanded(ss, id) // the page shows everything the guard asks to have seen
	v.Flashes = s.takeFlashes(ss)
	s.render(w, http.StatusOK, "item.html", v)
}

func (s *server) itemDetail(w http.ResponseWriter, r *http.Request) {
	ss := s.session(w, r)
	id := r.PathValue("id")
	v, err := s.itemView(r, id, nil, nil)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.markExpanded(ss, id)
	v.Inline = true
	s.render(w, http.StatusOK, "detail", v)
}

// --- helpers for forms --------------------------------------------------------------

func parseStarted(raw string, now time.Time) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, fmt.Errorf("the page's start time is missing or malformed; reload the page")
	}
	if t.After(now) {
		return time.Time{}, fmt.Errorf("the page's start time is in the future; reload the page")
	}
	return t.UTC(), nil
}
