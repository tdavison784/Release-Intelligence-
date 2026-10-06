package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/github"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/linkedev"
)

const evidenceUsage = `Usage:
  ri evidence link [-dir knowledge] [-dry-run] [-edges FILE] [-report FILE] [<product> <from> <to>]

link  fetch the pull request / issue / commit text that an edge's release-note changes reference
      (title, description, changed files; bounded, through the cached GitHub client) and add it,
      as linked-pr / linked-commit evidence, to the stored knowledge candidates that cite those
      changes. Additive and idempotent. Stops, keeping what it has, when GitHub throttles; run
      again later (the fetch cache resumes). -edges FILE lists "product from to" lines.
`

// evidenceCmd implements `ri evidence`.
func (c *cli) evidenceCmd(args []string) error {
	if len(args) == 0 || args[0] != "link" {
		fmt.Fprint(c.err, evidenceUsage)
		return fmt.Errorf("%w: expected `ri evidence link`", app.ErrUsage)
	}
	fs := c.flags("evidence link", "[<product> <from> <to>]")
	dir := fs.String("dir", knowledge.DefaultDir, "knowledge directory")
	dry := fs.Bool("dry-run", false, "fetch and report, but do not write the store")
	edgesFile := fs.String("edges", "", "file with one \"product from to\" per line (instead of positional arguments)")
	reportFile := fs.String("report", "", "write the full JSON report (links, candidates) to this file")
	perChange := fs.Int("max-per-change", linkedev.DefaultMaxPerChange, "referenced items fetched per change")
	maxFetches := fs.Int("max-fetches", linkedev.DefaultMaxFetches, "distinct items fetched per edge")
	pos, err := parse(fs, args[1:])
	if err != nil {
		return err
	}
	var edges [][3]string
	switch {
	case *edgesFile != "" && len(pos) == 0:
		edges, err = readEdges(*edgesFile)
		if err != nil {
			return err
		}
	case len(pos) == 3 && *edgesFile == "":
		edges = [][3]string{{pos[0], pos[1], pos[2]}}
	default:
		fs.Usage()
		return fmt.Errorf("%w: give <product> <from> <to> or -edges FILE", app.ErrUsage)
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	token := app.GitHubTokenFromEnv()
	if token == "" {
		fmt.Fprintln(c.err, "evidence link: no GITHUB_TOKEN; unauthenticated GitHub allows 60 requests per hour")
	}
	gh := github.NewClient(a.Fetch, token)
	store := knowledge.NewFileStore(*dir)

	type edgeReport struct {
		Product, From, To   string
		Changes             int `json:"changes"`
		ChangesWithTargets  int `json:"changesWithTargets"`
		ChangesGained       int `json:"changesGained"`
		ItemsFetched        int `json:"itemsFetched"`
		NotFound            int `json:"notFound"`
		Errors              int `json:"errors"`
		Capped              int `json:"capped"`
		NotFetchedThrottled int `json:"notFetchedThrottled"`
		EvidenceRecords     int `json:"evidenceRecords"`
		CandidatesMatched   int `json:"candidatesMatched"`
		CandidatesGained    int `json:"candidatesGained"`
		CandidatesAdded     int `json:"candidatesAddedThisRun"`
		GainedWithNeedsEvid int `json:"gainedCandidatesWithNeedsEvidenceItems"`
		NeedsEvidenceItems  int `json:"needsEvidenceItemsOnGained"`
		Throttled           bool
		Links               []linkedev.Link          `json:"links,omitempty"`
		CandidateLinks      []linkedev.CandidateLink `json:"candidateLinks,omitempty"`
	}
	var reports []edgeReport
	var throttled error
	for _, e := range edges {
		edge, err := a.Upgrade(c.ctx, e[0], e[1], e[2], app.UpgradeOptions{})
		if err != nil {
			return fmt.Errorf("%s %s→%s: %w", e[0], e[1], e[2], err)
		}
		res := linkedev.Collect(c.ctx, gh, edge.Changes, linkedev.Options{MaxPerChange: *perChange, MaxFetches: *maxFetches, Now: edge.GeneratedAt})
		srep, err := linkedev.Apply(c.ctx, store, domain.ProductID(e[0]), edge.Changes, edge.Evidence, res, *dry)
		if err != nil {
			return err
		}
		r := edgeReport{Product: e[0], From: e[1], To: e[2], Changes: len(edge.Changes), ItemsFetched: res.Fetches, EvidenceRecords: len(res.Evidence),
			Throttled: res.Throttled, CandidatesMatched: srep.CandidatesMatched, Links: res.Links, CandidateLinks: srep.Candidates}
		withTargets, gained := map[string]bool{}, res.ByChange()
		for _, l := range res.Links {
			withTargets[l.ChangeID] = true
			switch l.Status {
			case linkedev.StatusNotFound:
				r.NotFound++
			case linkedev.StatusError:
				r.Errors++
			case linkedev.StatusCapped:
				r.Capped++
			case linkedev.StatusThrottled:
				r.NotFetchedThrottled++
			}
		}
		r.ChangesWithTargets, r.ChangesGained = len(withTargets), len(gained)
		for _, cl := range srep.Gained() {
			r.CandidatesGained++
			r.CandidatesAdded += cl.Added
			if cl.NeedsEvidenceItems > 0 {
				r.GainedWithNeedsEvid++
				r.NeedsEvidenceItems += cl.NeedsEvidenceItems
			}
		}
		reports = append(reports, r)
		fmt.Fprintf(c.out, "%-20s %-9s → %-9s changes %4d  with-refs %3d  gained %3d  items %3d (not-found %d, error %d, capped %d)  candidates matched %3d gained %3d (+%d new)  of which needs-evidence %d\n",
			r.Product, r.From, r.To, r.Changes, r.ChangesWithTargets, r.ChangesGained, r.ItemsFetched, r.NotFound, r.Errors, r.Capped,
			r.CandidatesMatched, r.CandidatesGained, r.CandidatesAdded, r.GainedWithNeedsEvid)
		if res.Throttled {
			ra := ""
			if res.RetryAfter > 0 {
				ra = fmt.Sprintf(" (server asks to retry after %s)", res.RetryAfter.Round(time.Second))
			}
			throttled = fmt.Errorf("GitHub throttled the run at %s %s→%s after %d items%s; %d targets were not fetched. What was gathered is stored; run again later (the cache resumes)",
				e[0], e[1], e[2], res.Fetches, ra, r.NotFetchedThrottled)
			break
		}
	}
	if *reportFile != "" {
		b, err := json.MarshalIndent(reports, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*reportFile, append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	if throttled != nil {
		return throttled
	}
	if *dry {
		fmt.Fprintln(c.err, "evidence link: dry run — the store was not written")
	}
	return nil
}

func readEdges(path string) ([][3]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out [][3]string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		ln := strings.TrimSpace(sc.Text())
		if i := strings.IndexByte(ln, '#'); i >= 0 {
			ln = strings.TrimSpace(ln[:i])
		}
		if ln == "" {
			continue
		}
		p := strings.Fields(ln)
		if len(p) != 3 {
			return nil, fmt.Errorf("%s: want \"product from to\", got %q", path, ln)
		}
		out = append(out, [3]string{p[0], p[1], p[2]})
	}
	return out, sc.Err()
}
