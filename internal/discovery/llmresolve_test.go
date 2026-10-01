package discovery

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/ingest"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

const docsAnswer = `{"sources":[
 {"role":"release-notes","repository":"github.com/example/website","ref":"master","path":"content/docs/releases/release-notes/release-notes-{{.Major}}.{{.Minor}}.md","releaseKinds":[],"rationale":"per-line notes files"},
 {"role":"upgrade-guide","repository":"github.com/example/website","ref":"master","path":"content/docs/upgrade-{{.Version}}.md","releaseKinds":["minor","major"],"rationale":"guess"},
 {"role":"compatibility","repository":"github.com/evil/elsewhere","ref":"main","path":"compat.md","releaseKinds":[],"rationale":"hallucinated"}]}`

// fakeLLM answers by question schema.
func fakeLLM(answers map[string]string) *llm.Fake {
	return &llm.Fake{Model: "fake-model-1", Respond: func(req llm.Request) (string, error) {
		s := string(req.JSONSchema)
		switch {
		case strings.Contains(s, `"releaseKinds"`):
			return answers["docs"], nil
		case strings.Contains(s, `"registries"`):
			return answers["registry"], nil
		case strings.Contains(s, `"main"`):
			return answers["main"], nil
		case strings.Contains(s, `"strategy"`):
			return answers["chart"], nil
		}
		return "{}", nil
	}}
}

func evidenceIndex(r *Report) map[domain.EvidenceID]bool {
	m := map[domain.EvidenceID]bool{}
	for _, c := range r.Candidates {
		for _, e := range c.Evidence {
			m[e.ID] = true
		}
	}
	return m
}

func TestLLMProposalValidatedEntersDefinition(t *testing.T) {
	fake := fakeLLM(map[string]string{"docs": docsAnswer})
	checker := &fakeChecker{outcome: func(kind, subject, release string) (string, string) {
		if strings.HasPrefix(subject, "upgrade-guide") {
			return ingest.OutcomeFail, "404 Not Found"
		}
		return ingest.OutcomePass, ""
	}}
	// NoFollow: the docs repository is referenced but not scanned, so the
	// deterministic resolver cannot place release notes / upgrade docs.
	res := runFixture(t, certFixture, &Discoverer{LLM: fake, Checker: checker}, Request{NoFollow: true})
	def, rep := res.Definition, res.Report

	reqs := fake.Requests()
	if len(reqs) == 0 {
		t.Fatal("LLM was not asked")
	}
	var docsReq llm.Request
	for _, r := range reqs {
		if strings.Contains(string(r.JSONSchema), "releaseKinds") {
			docsReq = r
		}
		if len(r.Messages) != 1 || len(r.Messages[0].Content) > 8000 || len(r.JSONSchema) == 0 {
			t.Errorf("prompts must be small and request structured output: %d bytes", len(r.Messages[0].Content))
		}
	}
	if !strings.Contains(docsReq.Messages[0].Content, "github.com/example/website") {
		t.Error("the prompt should show the docs-repository candidate")
	}

	notes := source(t, def, "release-notes")
	if notes.Locator.Repository != "github.com/example/website" || len(notes.ValidatedAgainst) != 4 || !strings.Contains(notes.Notes, "AI-proposed") || !strings.Contains(notes.Notes, "fake-model-1") {
		t.Errorf("validated AI source should enter with provenance notes: %+v", notes)
	}
	for _, s := range def.Sources {
		if strings.Contains(s.ID, "--ai") || s.Locator.Path == "content/docs/upgrade-{{.Version}}.md" || strings.Contains(s.Locator.Repository, "evil") {
			t.Errorf("unvalidated or rejected AI element emitted: %+v", s)
		}
	}
	if def.Provenance.Method != "discovery+llm" {
		t.Errorf("method: %s", def.Provenance.Method)
	}

	statuses := map[string]string{}
	evIdx := evidenceIndex(rep)
	for _, p := range rep.Proposals {
		statuses[p.Summary] = p.Status
		if err := p.Provenance.Validate(); err != nil {
			t.Errorf("proposal %s provenance invalid: %v", p.ID, err)
		}
		if p.Provenance.Method != domain.MethodAI || p.Provenance.Model != "fake-model-1" || p.Provenance.GeneratedAt == nil {
			t.Errorf("proposal %s provenance: %+v", p.ID, p.Provenance)
		}
		for _, id := range p.Provenance.InputEvidence {
			if !evIdx[id] {
				t.Errorf("proposal %s cites evidence %s that is not in the report", p.ID, id)
			}
		}
		if p.Question == QuestionDocsSources && p.Provenance.PromptDigest != llm.PromptDigest(docsReq) {
			t.Errorf("prompt digest must be llm.PromptDigest(request)")
		}
	}
	want := map[string]string{
		"release-notes from github.com/example/website@master:content/docs/releases/release-notes/release-notes-{{.Major}}.{{.Minor}}.md": ProposalValidated,
		"upgrade-guide from github.com/example/website@master:content/docs/upgrade-{{.Version}}.md":                                       ProposalFailed,
		"compatibility from github.com/evil/elsewhere@main:compat.md":                                                                     ProposalRejected,
	}
	for s, st := range want {
		if statuses[s] != st {
			t.Errorf("proposal %q status %q, want %q (all: %v)", s, statuses[s], st, statuses)
		}
	}
	un := rep.UnverifiedProposals()
	if len(un) != 2 {
		t.Errorf("failed + rejected proposals are listed as unverified: %+v", un)
	}
	md := rep.Markdown()
	if !strings.Contains(md, "## AI proposals") || !strings.Contains(md, "Unverified proposals (NOT in the definition)") {
		t.Error("markdown should list AI proposals and unverified ones")
	}
	dropped := false
	for _, d := range rep.Dropped {
		if d.Origin == OriginAI && strings.Contains(d.Reason, "404") {
			dropped = true
		}
	}
	if !dropped {
		t.Errorf("failed AI element should appear among dropped elements: %+v", rep.Dropped)
	}
}

// Without validation an AI proposal must never be emitted.
func TestUnvalidatedAIProposalIsNeverEmitted(t *testing.T) {
	fake := fakeLLM(map[string]string{"docs": docsAnswer})
	res := runFixture(t, certFixture, &Discoverer{LLM: fake}, Request{NoFollow: true})
	for _, s := range res.Definition.Sources {
		if s.HasRole(domain.RoleUpgradeGuide) || s.Locator.Repository == "github.com/example/website" {
			t.Errorf("AI-proposed source emitted without validation: %+v", s)
		}
	}
	if res.Definition.Provenance.Method != "discovery" {
		t.Errorf("method: %s", res.Definition.Provenance.Method)
	}
	n := 0
	for _, p := range res.Report.Proposals {
		if p.Status == ProposalUnverified {
			n++
			if !strings.Contains(p.Detail, "validation did not run") {
				t.Errorf("detail: %q", p.Detail)
			}
		}
	}
	if n != 2 {
		t.Errorf("two proposals should be unverified: %+v", res.Report.Proposals)
	}
	found := false
	for _, o := range res.Report.Open {
		if strings.Contains(o, "Unverified AI proposal") {
			found = true
		}
	}
	if !found {
		t.Error("unverified proposals belong in the open questions")
	}
}

// An AI variant of an artifact replaces the deterministic one only when it
// validates and the deterministic one does not.
func TestLLMChartVersionVariant(t *testing.T) {
	fake := fakeLLM(map[string]string{
		"chart":    `{"strategy":"lookup","template":"","field":"appVersion","match":"{{.Version}}","rationale":"appVersion has no v prefix"}`,
		"registry": `{"registries":[{"registry":"quay.io/example","role":"release","rationale":"tag-triggered workflow"}]}`,
	})
	checker := &fakeChecker{outcome: func(kind, subject, release string) (string, string) {
		if subject == "deployer-chart" {
			return ingest.OutcomeFail, "no chart with appVersion " + release
		}
		return ingest.OutcomePass, ""
	}}
	res := runFixture(t, argoFixture, &Discoverer{LLM: fake, Checker: checker}, Request{})
	chart := artifact(t, res.Definition, "deployer-chart")
	if chart.Version.Match != "{{.Version}}" || !strings.Contains(chart.Notes, "AI-proposed") {
		t.Errorf("validated AI variant should replace the failing relation: %+v", chart)
	}
	if !hasDecision(res.Report, "validate.ai-variant-validated") {
		t.Error("replacement decision missing")
	}
	if res.Definition.Provenance.Method != "discovery+llm" {
		t.Errorf("method %s", res.Definition.Provenance.Method)
	}
}

func TestLLMMainChartIsAdviceOnly(t *testing.T) {
	fake := fakeLLM(map[string]string{"main": `{"main":"meshd","rationale":"control plane"}`, "registry": `{"registries":[]}`, "chart": `{"strategy":"unknown","template":"","field":"","match":"","rationale":""}`})
	res := runFixture(t, istioFixture, &Discoverer{LLM: fake, Checker: &fakeChecker{}}, Request{})
	found := false
	for _, p := range res.Report.Proposals {
		if p.Question == QuestionMainChart {
			found = true
			if p.Status != ProposalInformational || p.Element != "" {
				t.Errorf("main-chart answers never change the definition: %+v", p)
			}
		}
	}
	if !found {
		t.Fatal("two published charts should raise the main-chart question")
	}
	artifact(t, res.Definition, "base-chart")
	artifact(t, res.Definition, "meshd-chart")
	n := 0
	for _, r := range fake.Requests() {
		if strings.Contains(string(r.JSONSchema), `"strategy"`) {
			n++
			if !strings.Contains(r.Messages[0].Content, "base, meshd") {
				t.Errorf("charts sharing a relation are asked about together: %s", r.Messages[0].Content[:200])
			}
		}
	}
	if n != 1 {
		t.Errorf("one chart-version question per distinct relation, got %d", n)
	}
}

func TestLLMErrorsAreReported(t *testing.T) {
	fake := &llm.Fake{Responses: nil}
	res := runFixture(t, certFixture, &Discoverer{LLM: fake}, Request{NoFollow: true})
	if len(res.Report.Errors) == 0 || !strings.Contains(strings.Join(res.Report.Errors, " "), "llm:") {
		t.Errorf("LLM failures are non-fatal and reported: %v", res.Report.Errors)
	}
}
