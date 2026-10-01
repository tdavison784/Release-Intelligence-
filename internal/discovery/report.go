package discovery

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/ingest"
)

// Report is the discovery report: what was scanned, every candidate with
// evidence, every decision with its rule or AI provenance, the validation
// matrix and open questions for a human reviewer.
type Report struct {
	Product     string    `json:"product"`
	Name        string    `json:"name"`
	Repository  string    `json:"repository"`
	Ref         string    `json:"ref"`
	Commit      string    `json:"commit,omitempty"`
	GeneratedAt time.Time `json:"generatedAt"`
	// LLM is the model used for ambiguity resolution ("" when disabled).
	LLM          string            `json:"llm,omitempty"`
	Tags         *TagAnalysis      `json:"tags,omitempty"`
	Scans        []ScanResult      `json:"scans"`
	Candidates   []Candidate       `json:"candidates"`
	Elements     []*Element        `json:"elements"`
	Decisions    []Decision        `json:"decisions"`
	Questions    []Question        `json:"questions,omitempty"`
	Proposals    []Proposal        `json:"proposals,omitempty"`
	Validation   *ValidationResult `json:"validation,omitempty"`
	Dropped      []DroppedElement  `json:"dropped,omitempty"`
	Coverage     []Coverage        `json:"coverage"`
	Open         []string          `json:"openQuestions,omitempty"`
	StaticIssues []catalog.Issue   `json:"staticIssues,omitempty"`
	Errors       []string          `json:"errors,omitempty"`
	Definition   string            `json:"definition"` // YAML
}

// UnverifiedProposals returns AI proposals that did not enter the definition.
func (r *Report) UnverifiedProposals() []Proposal {
	var out []Proposal
	for _, p := range r.Proposals {
		if p.Status == ProposalUnverified || p.Status == ProposalFailed || p.Status == ProposalRejected {
			out = append(out, p)
		}
	}
	return out
}

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Markdown renders the full report for humans.
func (r *Report) Markdown() string {
	return r.render(true)
}

// Summary renders a compact report: coverage, open questions, validation,
// AI proposals and decisions, without the embedded definition, the
// excluded-candidate table and the candidate listing.
func (r *Report) Summary() string {
	return r.render(false)
}

func (r *Report) render(full bool) string {
	var b strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	p("# Discovery report: %s\n\n", r.Name)
	p("- Repository: `%s` at `%s`", r.Repository, r.Ref)
	if r.Commit != "" {
		p(" (commit `%s`)", trunc(r.Commit, 12))
	}
	p("\n- Generated: %s\n", r.GeneratedAt.Format(time.RFC3339))
	if r.LLM != "" {
		p("- LLM: `%s` (ambiguity resolution only; AI proposals enter the definition only when validated)\n", r.LLM)
	} else {
		p("- LLM: not used (deterministic resolver only)\n")
	}
	if r.Tags != nil {
		p("- Tags: %s\n", r.Tags.Summary())
		if r.Tags.TagPattern != "" {
			p("- Strict tag pattern: `%s` (junk sample: %s)\n", r.Tags.TagPattern, strings.Join(firstN(r.Tags.Junk, 6), ", "))
		}
	}
	for _, s := range r.Scans {
		p("- Scanned `%s@%s` (%s profile): %d files listed, %d read\n", s.Tree.Repo.String(), s.Tree.Ref, s.Profile, s.Listed, s.Read)
	}
	if v := r.Validation; v != nil {
		p("- Validation: %s", v.Status)
		if v.Detail != "" {
			p(" (%s)", v.Detail)
		}
		if len(v.Releases) > 0 {
			p(" against %s", strings.Join(v.Releases, ", "))
		}
		p("\n")
	}
	for _, e := range r.Errors {
		p("- Error: %s\n", e)
	}

	p("\n## Coverage of the discovery targets\n\n| Target | Status | Candidates | In definition |\n|---|---|---|---|\n")
	for _, c := range r.Coverage {
		p("| %s | %s | %d | %s |\n", c.Target, c.Status, c.Candidates, mdEscape(trunc(strings.Join(c.Findings, "; "), 220)))
	}

	if full {
		p("\n## Proposed definition\n\n```yaml\n%s```\n", r.Definition)
	}

	if len(r.Open) > 0 {
		p("\n## Open questions for the reviewer\n\n")
		for _, o := range r.Open {
			p("- %s\n", o)
		}
	}

	if v := r.Validation; v != nil && v.Status == ValidationDone {
		p("\n## Validation matrix\n\n")
		releases := v.Releases
		p("| Element | Verdict |")
		for _, rel := range releases {
			p(" %s |", rel)
		}
		p("\n|---|---|")
		for range releases {
			p("---|")
		}
		p("\n")
		cells := map[string]map[string]string{}
		var keys []string
		for _, c := range v.Checks {
			k := c.SubjectKind + ":" + c.Subject
			if cells[k] == nil {
				cells[k] = map[string]string{}
				keys = append(keys, k)
			}
			cell := c.Outcome
			if c.Outcome != ingest.OutcomePass && c.Detail != "" {
				cell += ": " + trunc(c.Detail, 60)
			}
			cells[k][c.Release] = cell
		}
		sort.Strings(keys)
		for _, k := range keys {
			p("| %s | %s |", k, v.Verdicts[k])
			for _, rel := range releases {
				p(" %s |", mdEscape(cells[k][rel]))
			}
			p("\n")
		}
	}

	if len(r.Proposals) > 0 {
		p("\n## AI proposals\n\n| Id | Question | Status | Summary | Model | Prompt digest | Input evidence |\n|---|---|---|---|---|---|---|\n")
		for _, pr := range r.Proposals {
			p("| %s | %s | %s | %s | %s | %s | %d |\n", pr.ID, pr.Question, pr.Status, mdEscape(trunc(pr.Summary+" "+pr.Detail, 160)),
				pr.Provenance.Model, shortDigest(pr.Provenance.PromptDigest), len(pr.Provenance.InputEvidence))
		}
		if un := r.UnverifiedProposals(); len(un) > 0 {
			p("\nUnverified proposals (NOT in the definition):\n\n")
			for _, pr := range un {
				p("- %s: %s — %s\n", pr.ID, pr.Summary, pr.Detail)
			}
		}
	}

	if len(r.Dropped) > 0 {
		p("\n## Dropped elements\n\n")
		for _, d := range r.Dropped {
			p("- `%s` (%s): %s\n", d.Key, d.Origin, d.Reason)
		}
	}

	p("\n## Decisions\n\n| Element | Action | Rule | Method | Rationale |\n|---|---|---|---|---|\n")
	for _, d := range r.Decisions {
		if d.Action == "exclude" {
			continue
		}
		p("| %s | %s | %s | %s | %s |\n", d.Element, d.Action, d.Rule, d.Provenance.Method, mdEscape(trunc(d.Rationale, 220)))
	}
	excluded := 0
	for _, d := range r.Decisions {
		if d.Action == "exclude" {
			excluded++
		}
	}
	if !full {
		p("\n%d candidates were found; %d were excluded by resolver rules (see the full report).\n", len(r.Candidates), excluded)
		return b.String()
	}
	if excluded > 0 {
		p("\n<details><summary>%d excluded candidates</summary>\n\n| Candidate | Rule | Rationale |\n|---|---|---|\n", excluded)
		cands := map[string]Candidate{}
		for _, c := range r.Candidates {
			cands[c.ID] = c
		}
		for _, d := range r.Decisions {
			if d.Action != "exclude" {
				continue
			}
			c := cands[strings.TrimPrefix(d.Element, "candidate:")]
			p("| %s `%s` | %s | %s |\n", c.Kind, mdEscape(trunc(c.Value, 90)), d.Rule, mdEscape(trunc(d.Rationale, 140)))
		}
		p("\n</details>\n")
	}

	p("\n## Candidates\n\n")
	byKind := map[CandidateKind][]Candidate{}
	var kinds []string
	for _, c := range r.Candidates {
		if _, ok := byKind[c.Kind]; !ok {
			kinds = append(kinds, string(c.Kind))
		}
		byKind[c.Kind] = append(byKind[c.Kind], c)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		cs := byKind[CandidateKind(k)]
		sort.Slice(cs, func(i, j int) bool {
			if confRank(cs[i].Confidence) != confRank(cs[j].Confidence) {
				return confRank(cs[i].Confidence) > confRank(cs[j].Confidence)
			}
			return cs[i].Value < cs[j].Value
		})
		p("\n### %s (%d)\n\n", k, len(cs))
		for i, c := range cs {
			if i >= 25 {
				p("- … %d more\n", len(cs)-25)
				break
			}
			p("- `%s` (%s; %s)", trunc(c.Value, 120), c.Confidence, strings.Join(c.Rules, ", "))
			if len(c.Evidence) > 0 {
				e := c.Evidence[0]
				loc := e.URI
				if e.Locator != "" {
					loc += "#" + e.Locator
				}
				p(" — [%s](%s)", shortURI(e.URI)+" "+e.Locator, loc)
				if e.Excerpt != "" {
					p(": `%s`", strings.ReplaceAll(trunc(strings.TrimSpace(e.Excerpt), 110), "`", "'"))
				}
			}
			p("\n")
		}
	}
	return b.String()
}
