package enrich

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// PromptVersion names the prompt templates below; it is recorded in every
// enrichment's provenance. Change it whenever the system prompt, the user
// prompt layout or the answer schema changes.
const PromptVersion = "enrich/v1"

const systemPrompt = `You consolidate the deterministic findings of a software upgrade report (prompt ` + PromptVersion + `).
You are given one group of CHANGES that a deterministic pre-processor found possibly related, and the EVIDENCE excerpts they were derived from.
Rules:
- Use only what the changes and evidence excerpts state. Never add versions, defaults, flags, steps or consequences they do not state.
- You may group, summarise, connect and explain changes. You never correct, replace or restate a change as something it does not say.
- Every enrichment lists the change ids it is about ("changes") and the evidence ids it relies on ("citations"), copied exactly from the input.
- kind "cluster": two or more changes that state the same change in different sources (release notes, upgrade guide, a computed diff). Content: the one consolidated conclusion. Cite evidence of every member.
- kind "migration-summary": what an operator must do for one migration requirement, summarised from one or more changes. Only steps the evidence states.
- kind "diff-explanation": why a computed diff (method computed) matters. Relate it to the release-note or upgrade-guide change that explains it and cite that statement.
- kind "related": changes that may be related although the evidence does not establish it. It is shown as unverified.
- A change belongs to at most one cluster. If nothing useful can be said, answer {"enrichments": []}.
- title: at most 12 words. content: at most 3 plain sentences. confidence: how well the cited evidence supports the content.`

// answerSchema is the structured output every answer must conform to.
const answerSchema = `{"type":"object","additionalProperties":false,"required":["enrichments"],"properties":{"enrichments":{"type":"array","maxItems":16,"items":{"type":"object","additionalProperties":false,"required":["kind","title","content","changes","citations","confidence"],"properties":{"kind":{"type":"string","enum":["cluster","migration-summary","diff-explanation","related"]},"title":{"type":"string"},"content":{"type":"string"},"changes":{"type":"array","items":{"type":"string"}},"citations":{"type":"array","items":{"type":"string"}},"confidence":{"type":"string","enum":["high","medium","low"]}}}}}}`

// Prompt bounds.
const (
	maxDetail        = 500
	maxExcerpt       = 500
	maxEvidencePer   = 3
	maxPromptLength  = 14000
	minExcerptLength = 120
)

// prompt is a rendered request for one candidate group.
type prompt struct {
	req   llm.Request
	input []domain.EvidenceID // evidence shown to the model, in order
}

// buildPrompt renders the bounded prompt of one group. The evidence ids
// shown become the enrichment's input evidence; citations outside them are
// rejected by the validator.
func buildPrompt(e *domain.UpgradeEdge, cand Candidate, model string, changes map[string]domain.Change, ev map[domain.EvidenceID]domain.Evidence) prompt {
	excerpt := maxExcerpt
	for {
		user, input := renderUser(e, cand, changes, ev, excerpt)
		if len(user) <= maxPromptLength || excerpt <= minExcerptLength {
			return prompt{
				req: llm.Request{Model: model, System: systemPrompt, Messages: []llm.Message{{Role: "user", Content: user}},
					JSONSchema: json.RawMessage(answerSchema)},
				input: input,
			}
		}
		excerpt /= 2
	}
}

func renderUser(e *domain.UpgradeEdge, cand Candidate, changes map[string]domain.Change, ev map[domain.EvidenceID]domain.Evidence, excerpt int) (string, []domain.EvidenceID) {
	var b strings.Builder
	name := e.Product.Name
	if name == "" {
		name = string(e.Product.ID)
	}
	fmt.Fprintf(&b, "Upgrade: %s %s → %s\n", name, e.From, e.To)
	fmt.Fprintf(&b, "Group %s. Why the pre-processor grouped these changes:\n", cand.ID)
	for _, s := range cand.Signals {
		fmt.Fprintf(&b, "- %s\n", s)
	}
	b.WriteString("\nCHANGES\n")
	var input []domain.EvidenceID
	seen := map[domain.EvidenceID]bool{}
	for _, id := range cand.Changes {
		c := changes[id]
		flags := ""
		if c.Breaking {
			flags += " breaking"
		}
		if c.ActionRequired {
			flags += " action-required"
		}
		rel := c.Release
		if rel == "" {
			rel = "diff " + e.From.Semver + "→" + e.To.Semver
		}
		var srcs []string
		var ids []string
		for i, x := range c.Evidence {
			if i == maxEvidencePer {
				break
			}
			ids = append(ids, string(x))
			if r, ok := ev[x]; ok && r.SourceID != "" && !contains(srcs, r.SourceID) {
				srcs = append(srcs, r.SourceID)
			}
			if !seen[x] {
				seen[x] = true
				input = append(input, x)
			}
		}
		fmt.Fprintf(&b, "[%s] category=%s method=%s%s release=%s sources=%s\n", c.ID, c.Category, c.Provenance.Method, flags, rel, strings.Join(srcs, ","))
		fmt.Fprintf(&b, "  title: %s\n", oneLine(c.Title))
		if d := oneLine(c.Detail); d != "" && d != oneLine(c.Title) {
			fmt.Fprintf(&b, "  detail: %s\n", shorten(d, maxDetail))
		}
		if len(c.Subjects) > 0 {
			subs := c.Subjects
			if len(subs) > 8 {
				subs = append(append([]string{}, subs[:8]...), fmt.Sprintf("(+%d more)", len(c.Subjects)-8))
			}
			fmt.Fprintf(&b, "  subjects: %s\n", strings.Join(subs, ", "))
		}
		fmt.Fprintf(&b, "  evidence: %s\n", strings.Join(ids, ", "))
	}
	b.WriteString("\nEVIDENCE\n")
	for _, id := range input {
		x := ev[id]
		fmt.Fprintf(&b, "[%s] source=%s uri=%s", id, x.SourceID, x.URI)
		if x.Locator != "" {
			fmt.Fprintf(&b, " locator=%s", x.Locator)
		}
		b.WriteString("\n")
		if ex := oneLine(x.Excerpt); ex != "" {
			fmt.Fprintf(&b, "  excerpt: %s\n", shorten(ex, excerpt))
		}
	}
	b.WriteString("\nAnswer with JSON matching the schema.\n")
	return b.String(), input
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
