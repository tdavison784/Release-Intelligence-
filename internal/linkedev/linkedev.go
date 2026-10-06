// Package linkedev enriches release-note changes with the context behind the
// pull requests, issues and commits they reference.
//
// A one-line changelog entry or PR title rarely lets a reviewer decide which
// setting changed, to what, and who breaks; the answer usually sits in the
// linked PR's description or changed-file list. This package fetches that
// text through the cached GitHub client (bounded in size and count, resumable
// from the fetch cache, stopping when GitHub throttles) and records it as
// domain.Evidence of kind linked-pr / linked-commit. The evidence is context,
// never a conclusion; it is attached to the knowledge candidates whose
// members cite the change (store.go), so prompts and reviewers can show it.
//
// It is generic: the references come from the changes' own Reference lists
// (extracted deterministically from the note text); nothing is
// product-specific.
package linkedev

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/github"
)

// Producer names this enricher in evidence.
const Producer = "linkedev@v1"

// Defaults bounding one run.
const (
	DefaultMaxPerChange = 4
	DefaultMaxFetches   = 2000
	// descriptionChunk is the size of one description evidence record (the
	// store bounds excerpts at domain.MaxExcerpt).
	descriptionChunk = 520
	maxDescChunks    = 2
)

// Fetcher reads one linked item; *github.Client implements it.
type Fetcher interface {
	LinkedItem(ctx context.Context, owner, repo, kind string, number int, sha string) (*github.LinkedItem, error)
}

// Target is one referenced item on a github-hosted repository.
type Target struct {
	Kind        string // github.LinkedPullRequest | LinkedIssue | LinkedCommit | "github-ref" (PR or issue; GitHub decides)
	Owner, Repo string
	Number      int
	SHA         string
	URL         string
}

// Key identifies the item independent of how it was spelled.
func (t Target) Key() string {
	if t.Kind == github.LinkedCommit {
		return strings.ToLower(t.Owner+"/"+t.Repo) + "@" + strings.ToLower(t.SHA)
	}
	return strings.ToLower(t.Owner+"/"+t.Repo) + "#" + strconv.Itoa(t.Number)
}

var (
	itemIDRe   = regexp.MustCompile(`^([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)#(\d+)$`)
	commitURLR = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)/commit/([0-9a-fA-F]{40})`)
)

// kindRank orders targets within a change: the PR carries the description and
// files, a bare #N likely too, an issue states the problem, a commit the diff.
var kindRank = map[string]int{github.LinkedPullRequest: 0, "github-ref": 1, github.LinkedIssue: 2, github.LinkedCommit: 3}

// TargetsOf extracts the github-hosted pull requests, issues and commits a
// change references, de-duplicated and in a deterministic order (kind rank,
// then first appearance). Advisory ids, CVEs and non-github URLs are ignored.
func TargetsOf(refs []domain.Reference) []Target {
	var out []Target
	seen := map[string]bool{}
	for _, r := range refs {
		var t Target
		switch r.Type {
		case "pull-request", "issue", "github-ref":
			m := itemIDRe.FindStringSubmatch(r.ID)
			if m == nil {
				continue
			}
			n, err := strconv.Atoi(m[3])
			if err != nil || n <= 0 {
				continue
			}
			t = Target{Kind: r.Type, Owner: m[1], Repo: m[2], Number: n, URL: r.URL}
		case "commit":
			m := commitURLR.FindStringSubmatch(r.URL)
			if m == nil {
				continue // an abbreviated id without a github URL cannot be resolved
			}
			t = Target{Kind: github.LinkedCommit, Owner: m[1], Repo: m[2], SHA: strings.ToLower(m[3]), URL: r.URL}
		default:
			continue
		}
		if seen[t.Key()] {
			continue
		}
		seen[t.Key()] = true
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return kindRank[out[i].Kind] < kindRank[out[j].Kind] })
	return out
}

// Link statuses.
const (
	StatusFetched   = "fetched"
	StatusNotFound  = "not-found"
	StatusError     = "error"
	StatusThrottled = "throttled" // GitHub rate limited the run; not fetched, retry later
	StatusCapped    = "capped"    // beyond the per-change or per-run bound
)

// Link records what happened to one referenced item of one change.
type Link struct {
	ChangeID string
	Target   Target
	Status   string
	Detail   string
	// Evidence lists the ids of the records built from the item (Status fetched).
	Evidence []domain.EvidenceID
}

// Options bound one run.
type Options struct {
	MaxPerChange int       // referenced items fetched per change (default 4)
	MaxFetches   int       // distinct items fetched per run (default 2000)
	Now          time.Time // retrievedAt of the evidence (fetch time when zero)
}

// Result is the outcome of Collect.
type Result struct {
	Links []Link
	// Evidence is every record built, in first-built order, de-duplicated by id.
	Evidence []domain.Evidence
	// Fetches counts distinct items requested from the API (cache hits
	// included: the fetch layer decides whether the network is touched).
	Fetches int
	// Throttled reports that GitHub rate limited the run; the remaining
	// targets are Links with StatusThrottled. RetryAfter is the server's
	// request when known.
	Throttled  bool
	RetryAfter time.Duration
}

// ByChange maps a change id to the evidence ids its links produced.
func (r *Result) ByChange() map[string][]domain.EvidenceID {
	m := map[string][]domain.EvidenceID{}
	for _, l := range r.Links {
		if l.Status != StatusFetched {
			continue
		}
		for _, id := range l.Evidence {
			dup := false
			for _, x := range m[l.ChangeID] {
				dup = dup || x == id
			}
			if !dup {
				m[l.ChangeID] = append(m[l.ChangeID], id)
			}
		}
	}
	return m
}

// Collect fetches the items referenced by changes and builds their evidence.
// Changes are visited in the given order; an item shared by several changes is
// fetched once. A throttled response stops the run (nothing is retried here:
// the fetch layer already honoured Retry-After); a missing or failing item is
// recorded and skipped.
func Collect(ctx context.Context, f Fetcher, changes []domain.Change, opts Options) *Result {
	if opts.MaxPerChange <= 0 {
		opts.MaxPerChange = DefaultMaxPerChange
	}
	if opts.MaxFetches <= 0 {
		opts.MaxFetches = DefaultMaxFetches
	}
	res := &Result{}
	type outcome struct {
		status, detail string
		ids            []domain.EvidenceID
	}
	done := map[string]*outcome{}
	haveEv := map[domain.EvidenceID]bool{}
	for _, c := range changes {
		targets := TargetsOf(c.References)
		for i, t := range targets {
			l := Link{ChangeID: c.ID, Target: t}
			switch o := done[t.Key()]; {
			case i >= opts.MaxPerChange:
				l.Status, l.Detail = StatusCapped, fmt.Sprintf("more than %d referenced items on one change", opts.MaxPerChange)
			case o != nil:
				l.Status, l.Detail, l.Evidence = o.status, o.detail, o.ids
			case res.Throttled:
				l.Status, l.Detail = StatusThrottled, "not fetched: an earlier request was rate limited"
			case res.Fetches >= opts.MaxFetches:
				l.Status, l.Detail = StatusCapped, fmt.Sprintf("more than %d items in one run", opts.MaxFetches)
			default:
				res.Fetches++
				item, err := f.LinkedItem(ctx, t.Owner, t.Repo, t.Kind, t.Number, t.SHA)
				switch {
				case errors.Is(err, fetch.ErrThrottled):
					res.Throttled = true
					var fe *fetch.Error
					if errors.As(err, &fe) {
						res.RetryAfter = fe.RetryAfter
					}
					l.Status, l.Detail = StatusThrottled, err.Error()
				case errors.Is(err, fetch.ErrNotFound):
					l.Status, l.Detail = StatusNotFound, "the item does not exist or is not visible to this token"
					done[t.Key()] = &outcome{status: l.Status, detail: l.Detail}
				case err != nil:
					l.Status, l.Detail = StatusError, err.Error()
					done[t.Key()] = &outcome{status: l.Status, detail: l.Detail}
				default:
					at := opts.Now
					if at.IsZero() {
						at = time.Now()
					}
					evs := EvidenceFor(item, at)
					for _, e := range evs {
						l.Evidence = append(l.Evidence, e.ID)
						if !haveEv[e.ID] {
							haveEv[e.ID] = true
							res.Evidence = append(res.Evidence, e)
						}
					}
					l.Status = StatusFetched
					done[t.Key()] = &outcome{status: l.Status, ids: l.Evidence}
				}
			}
			res.Links = append(res.Links, l)
		}
	}
	return res
}

var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)

// cleanBody drops HTML comments (PR templates are full of them) and collapses
// runs of blank lines and spaces; it selects nothing else.
func cleanBody(s string) string {
	s = htmlComment.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	var out []string
	blank := false
	for _, ln := range lines {
		ln = strings.Join(strings.Fields(ln), " ")
		if ln == "" {
			if blank {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// EvidenceFor builds the evidence records of one fetched item: the title with
// the start of the description, up to maxDescChunks-1 continuation chunks of
// the description, and the changed-file list. Each stays within
// domain.MaxExcerpt, and ids are content-derived (item digest included), so
// re-running on unchanged content reproduces the same ids.
func EvidenceFor(it *github.LinkedItem, at time.Time) []domain.Evidence {
	kind := domain.EvidenceLinkedPR
	label := "PR"
	switch it.Kind {
	case github.LinkedCommit:
		kind, label = domain.EvidenceLinkedCommit, "commit"
	case github.LinkedIssue:
		label = "issue"
	}
	mk := func(locator, excerpt string) domain.Evidence {
		return domain.NewEvidence(kind, Producer, it.URL, locator, excerpt, it.Digest, at)
	}
	head := fmt.Sprintf("%s: %s", label, it.Title)
	if it.State != "" || len(it.Labels) > 0 {
		var meta []string
		if it.State != "" {
			meta = append(meta, it.State)
		}
		if len(it.Labels) > 0 {
			meta = append(meta, "labels "+strings.Join(it.Labels, ", "))
		}
		head += " [" + strings.Join(meta, "; ") + "]"
	}
	body := cleanBody(it.Body)
	var out []domain.Evidence
	chunks := chunk(body, descriptionChunk, maxDescChunks)
	first := head
	if len(chunks) > 0 {
		first += "\n" + chunks[0]
	}
	out = append(out, mk("title and description", first))
	for i := 1; i < len(chunks); i++ {
		out = append(out, mk(fmt.Sprintf("description (part %d)", i+1), chunks[i]))
	}
	if len(it.Files) > 0 {
		out = append(out, mk("changed files", filesExcerpt(it)))
	}
	return out
}

// chunk splits s into at most n pieces of about size bytes at whitespace.
func chunk(s string, size, n int) []string {
	var out []string
	for s != "" && len(out) < n {
		if len(s) <= size {
			out = append(out, s)
			break
		}
		cut := size
		for cut > 0 && s[cut] != ' ' && s[cut] != '\n' {
			cut--
		}
		if cut < size/2 { // no whitespace nearby: cut hard, on a rune boundary
			cut = size
			for cut > 0 && (s[cut]&0xC0) == 0x80 {
				cut--
			}
		}
		piece := strings.TrimSpace(s[:cut])
		if len(out) == n-1 {
			piece += " …" // the description continues beyond what is kept
		}
		out = append(out, piece)
		s = strings.TrimSpace(s[cut:])
	}
	return out
}

func filesExcerpt(it *github.LinkedItem) string {
	total := it.FilesKnown
	if total == 0 {
		total = len(it.Files)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d changed file(s): ", total)
	for i, f := range it.Files {
		piece := f.Name
		if f.Status == "removed" || f.Status == "added" {
			piece += " (" + f.Status + ")"
		}
		if b.Len()+len(piece)+8 > domain.MaxExcerpt-20 {
			fmt.Fprintf(&b, "… +%d more", len(it.Files)-i)
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(piece)
	}
	return b.String()
}
