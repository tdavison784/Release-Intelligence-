package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// LLMResolver asks a language model small, focused questions about the
// ambiguities the deterministic resolver recorded. Every answer becomes a
// Proposal with AI provenance; proposals that would change the definition
// are staged as AI elements (Draft.AISources / Draft.AIArtifacts) and only
// enter the definition after validation.
type LLMResolver struct {
	Client llm.Client
	// Model overrides the client's default model ("" = client default).
	Model string
	// MaxCandidates and MaxEvidence bound the prompt size.
	MaxCandidates int
	MaxEvidence   int
	Clock         func() time.Time
}

// llmPromptVersion names the discovery prompt templates in provenance.
const llmPromptVersion = "discovery/v1"

const llmSystem = `You help map how an open-source project publishes releases. You are given deterministic findings from its repository (file:line excerpts). Answer only with JSON matching the schema. Base every statement on the findings; use "unknown" instead of guessing. Keep rationales to one sentence citing the evidence ids you relied on.`

// Resolve answers the draft's questions and stages proposals.
func (l *LLMResolver) Resolve(ctx context.Context, in ResolveInput, d *Draft) ([]Proposal, []error) {
	var props []Proposal
	var errs []error
	for i, q := range d.Questions {
		p, err := l.answer(ctx, in, d, q, i)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", q.Kind, err))
			continue
		}
		props = append(props, p...)
	}
	return props, errs
}

func (l *LLMResolver) now() time.Time {
	if l.Clock != nil {
		return l.Clock().UTC()
	}
	return time.Now().UTC()
}

// promptCandidates renders candidates compactly and returns the evidence ids
// shown to the model.
func (l *LLMResolver) promptCandidates(cs []Candidate) (string, []domain.EvidenceID) {
	maxC, maxE := l.MaxCandidates, l.MaxEvidence
	if maxC <= 0 {
		maxC = 12
	}
	if maxE <= 0 {
		maxE = 2
	}
	sorted := append([]Candidate(nil), cs...)
	sort.SliceStable(sorted, func(i, j int) bool { return confRank(sorted[i].Confidence) > confRank(sorted[j].Confidence) })
	var b strings.Builder
	var ids []domain.EvidenceID
	for i, c := range sorted {
		if i >= maxC {
			fmt.Fprintf(&b, "(%d more candidates omitted)\n", len(sorted)-maxC)
			break
		}
		fmt.Fprintf(&b, "- [%s] %s %q (confidence %s", c.ID, c.Kind, c.Value, c.Confidence)
		for _, k := range []string{"sources", "triggers", "classes", "contexts", "variable", "charts", "tags", "repo"} {
			if v := c.Attr(k); v != "" {
				if len(v) > 80 {
					v = v[:80] + "…"
				}
				fmt.Fprintf(&b, ", %s=%s", k, v)
			}
		}
		b.WriteString(")\n")
		for j, e := range c.Evidence {
			if j >= maxE {
				break
			}
			ex := e.Excerpt
			if len(ex) > 160 {
				ex = ex[:160] + "…"
			}
			fmt.Fprintf(&b, "    evidence %s: %s %s: %s\n", e.ID, shortURI(e.URI), e.Locator, strings.TrimSpace(ex))
			ids = append(ids, e.ID)
		}
	}
	return b.String(), ids
}

func shortURI(u string) string {
	for _, p := range []string{"https://github.com/", "https://"} {
		u = strings.TrimPrefix(u, p)
	}
	if i := strings.Index(u, "/blob/"); i >= 0 {
		rest := u[i+len("/blob/"):]
		if j := strings.Index(rest, "/"); j >= 0 {
			return u[:i] + ":" + rest[j+1:]
		}
	}
	return u
}

func (l *LLMResolver) ask(ctx context.Context, user string, schema string) (*llm.Response, llm.Request, error) {
	req := llm.Request{Model: l.Model, System: llmSystem, Messages: []llm.Message{{Role: "user", Content: user}}, JSONSchema: json.RawMessage(schema)}
	resp, err := l.Client.Complete(ctx, req)
	return resp, req, err
}

func (l *LLMResolver) provenance(q Question, resp *llm.Response, req llm.Request, ids []domain.EvidenceID) domain.Provenance {
	t := l.now()
	return domain.Provenance{Method: domain.MethodAI, Producer: ProducerLLM, Rule: "question:" + q.Kind, Confidence: domain.ConfidenceMedium,
		Model: resp.Model, ModelVersion: resp.ModelVersion, PromptVersion: llmPromptVersion,
		PromptDigest: llm.PromptDigest(req), InputEvidence: ids, GeneratedAt: &t}
}

func header(in ResolveInput) string {
	latest, prefix := "", ""
	if in.Tags != nil {
		latest, prefix = in.Tags.Latest, in.Tags.Prefix
	}
	return fmt.Sprintf("Product repository: %s. Latest stable tag: %s. Tag prefix: %q.\n", in.Repo.String(), latest, prefix)
}

func (l *LLMResolver) answer(ctx context.Context, in ResolveInput, d *Draft, q Question, n int) ([]Proposal, error) {
	switch q.Kind {
	case QuestionRegistryRoles:
		return l.registryRoles(ctx, in, d, q, n)
	case QuestionDocsSources:
		return l.docsSources(ctx, in, d, q, n)
	case QuestionMainChart:
		return l.mainChart(ctx, in, d, q, n)
	case QuestionChartVersion:
		return l.chartVersion(ctx, in, d, q, n)
	}
	return nil, fmt.Errorf("unknown question kind %q", q.Kind)
}

const registrySchema = `{"type":"object","additionalProperties":false,"required":["registries"],"properties":{"registries":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["registry","role","rationale"],"properties":{"registry":{"type":"string"},"role":{"type":"string","enum":["release","mirror","snapshot","development","third-party","unknown"]},"rationale":{"type":"string"}}}}}}`

func (l *LLMResolver) registryRoles(ctx context.Context, in ResolveInput, d *Draft, q Question, n int) ([]Proposal, error) {
	cands, ids := l.promptCandidates(q.Candidates)
	user := header(in) + "Question: " + q.Summary + "\nFor each registry (host or host/namespace) below, classify its role for the product's container images: release (where release tags are published), mirror (secondary copy of release tags), snapshot (branch/commit builds), development, third-party, or unknown.\nFindings:\n" + cands
	resp, req, err := l.ask(ctx, user, registrySchema)
	if err != nil {
		return nil, err
	}
	var ans struct {
		Registries []struct{ Registry, Role, Rationale string } `json:"registries"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &ans); err != nil {
		return nil, fmt.Errorf("decode answer: %w", err)
	}
	prov := l.provenance(q, resp, req, ids)
	base := fmt.Sprintf("prop-%d-%s", n, q.Kind)
	release := map[string]int{} // registry prefix → rank (release 0, mirror 1)
	var lines []string
	for _, r := range ans.Registries {
		lines = append(lines, r.Registry+": "+r.Role)
		switch r.Role {
		case "release":
			release[strings.TrimSuffix(r.Registry, "/")] = 0
		case "mirror":
			release[strings.TrimSuffix(r.Registry, "/")] = 1
		}
	}
	out := []Proposal{{ID: base, Question: q.Kind, Summary: "Registry roles: " + strings.Join(lines, "; "), Answer: resp.Text, Provenance: prov, Status: ProposalInformational}}
	if len(release) == 0 {
		return out, nil
	}
	for _, a := range d.Definition.Artifacts {
		if a.Type != domain.ArtifactContainerImage {
			continue
		}
		var chans []catalog.Locator
		seen := map[string]bool{}
		type ranked struct {
			l    catalog.Locator
			rank int
		}
		var rs []ranked
		for _, ch := range a.Channels {
			if rk, ok := registryRank(ch.Repository, release); ok {
				rs = append(rs, ranked{ch, rk})
				seen[ch.Repository] = true
			}
		}
		for reg, rk := range release {
			repo := reg + "/" + a.Name
			if _, err := ParseRepo(repo); err == nil && !seen[repo] && strings.Contains(reg, "/") {
				rs = append(rs, ranked{catalog.Locator{Kind: catalog.LocatorOCI, Repository: repo}, rk + 2})
				seen[repo] = true
			}
		}
		sort.SliceStable(rs, func(i, j int) bool {
			if rs[i].rank != rs[j].rank {
				return rs[i].rank < rs[j].rank
			}
			return rs[i].l.Repository < rs[j].l.Repository
		})
		for _, r := range rs {
			chans = append(chans, r.l)
		}
		if len(chans) == 0 || sameChannels(chans, a.Channels) {
			continue
		}
		v := a
		v.ID = a.ID + "--ai"
		v.Channels = chans
		v.References = nil
		pid := fmt.Sprintf("%s-%s", base, a.ID)
		d.AIArtifacts = append(d.AIArtifacts, v)
		d.Elements["artifact:"+v.ID] = &Element{Key: "artifact:" + v.ID, Kind: "artifact", ID: v.ID, Origin: OriginAI, Rule: "question:" + q.Kind,
			Confidence: domain.ConfidenceMedium, ProposalID: pid, Replaces: "artifact:" + a.ID, Targets: []string{TargetImages, TargetRegistries}}
		out = append(out, Proposal{ID: pid, Question: q.Kind, Element: "artifact:" + v.ID, Replaces: "artifact:" + a.ID,
			Summary: fmt.Sprintf("Image %s channels → %s", a.Name, locatorList(chans)), Answer: resp.Text, Provenance: prov, Status: ProposalPending})
	}
	return out, nil
}

func registryRank(repo string, release map[string]int) (int, bool) {
	best, ok := 0, false
	for reg, rk := range release {
		if repo == reg || strings.HasPrefix(repo, reg+"/") {
			if !ok || rk < best {
				best, ok = rk, true
			}
		}
	}
	return best, ok
}

func sameChannels(a, b []catalog.Locator) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func locatorList(ls []catalog.Locator) string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Kind+":"+l.Repository+l.URL)
	}
	return strings.Join(out, ", ")
}

const docsSchema = `{"type":"object","additionalProperties":false,"required":["sources"],"properties":{"sources":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["role","repository","ref","path","releaseKinds","rationale"],"properties":{"role":{"type":"string","enum":["release-notes","upgrade-guide","compatibility"]},"repository":{"type":"string"},"ref":{"type":"string"},"path":{"type":"string"},"releaseKinds":{"type":"array","items":{"type":"string","enum":["major","minor","patch"]}},"rationale":{"type":"string"}}}}}}`

func (l *LLMResolver) docsSources(ctx context.Context, in ResolveInput, d *Draft, q Question, n int) ([]Proposal, error) {
	cands, ids := l.promptCandidates(q.Candidates)
	paths := q.Paths
	if len(paths) > 80 {
		paths = paths[:80]
	}
	user := header(in) + "Question: " + q.Summary + "\nPropose repo-file locators for the missing roles (" + q.Facts["missing"] + "). Paths may use the templates {{.Tag}}, {{.Version}} (e.g. 1.2.3), {{.Major}}, {{.Minor}}, {{.PrevMajor}}, {{.PrevMinor}} (previous release line). repository is host/owner/name; ref is a branch. Propose nothing for a role you cannot support with the listing.\nFindings:\n" + cands +
		"Documentation repository listing (repo:path):\n" + strings.Join(paths, "\n") + "\n"
	resp, req, err := l.ask(ctx, user, docsSchema)
	if err != nil {
		return nil, err
	}
	var ans struct {
		Sources []struct {
			Role, Repository, Ref, Path, Rationale string
			ReleaseKinds                           []string `json:"releaseKinds"`
		} `json:"sources"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &ans); err != nil {
		return nil, fmt.Errorf("decode answer: %w", err)
	}
	prov := l.provenance(q, resp, req, ids)
	allowed := map[string]bool{}
	for _, c := range q.Candidates {
		allowed[c.Value] = true
	}
	var out []Proposal
	for i, s := range ans.Sources {
		pid := fmt.Sprintf("prop-%d-%s-%d", n, q.Kind, i)
		p := Proposal{ID: pid, Question: q.Kind, Summary: fmt.Sprintf("%s from %s@%s:%s", s.Role, s.Repository, s.Ref, s.Path), Rationale: s.Rationale, Answer: resp.Text, Provenance: prov}
		repo, err := ParseRepo(s.Repository)
		switch {
		case err != nil || !allowed[repo.String()]:
			p.Status, p.Detail = ProposalRejected, "repository is not one of the documentation repositories found by the scanner"
		case !strings.Contains(q.Facts["missing"], s.Role):
			p.Status, p.Detail = ProposalRejected, "role already covered deterministically"
		default:
			src := catalog.Source{ID: sanitizeID(s.Role) + "--ai", Roles: []domain.SourceRole{domain.SourceRole(s.Role)},
				Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: repo.String(), Ref: s.Ref, Path: s.Path}, ReleaseKinds: s.ReleaseKinds}
			if issues := staticIssues(src); issues != "" {
				p.Status, p.Detail = ProposalRejected, issues
				break
			}
			src.ID = uniqueAIID(d, src.ID)
			p.Element, p.Status = "source:"+src.ID, ProposalPending
			d.AISources = append(d.AISources, src)
			d.Elements[p.Element] = &Element{Key: p.Element, Kind: "source", ID: src.ID, Origin: OriginAI, Rule: "question:" + q.Kind,
				Confidence: domain.ConfidenceMedium, ProposalID: pid, Targets: roleTargets(s.Role)}
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		out = append(out, Proposal{ID: fmt.Sprintf("prop-%d-%s", n, q.Kind), Question: q.Kind, Summary: "no source proposed", Answer: resp.Text, Provenance: prov, Status: ProposalInformational})
	}
	return out, nil
}

func roleTargets(role string) []string {
	switch role {
	case string(domain.RoleUpgradeGuide):
		return []string{TargetUpgradeDocs}
	case string(domain.RoleCompatibility):
		return []string{TargetCompatibility}
	}
	return []string{TargetReleaseNotes}
}

func uniqueAIID(d *Draft, id string) string {
	used := func(x string) bool {
		_, a := d.Elements["source:"+x]
		_, b := d.Elements["artifact:"+x]
		return a || b
	}
	if !used(id) {
		return id
	}
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s-%d", id, i)
		if !used(c) {
			return c
		}
	}
}

// staticIssues validates one source in a minimal definition.
func staticIssues(s catalog.Source) string {
	def := &catalog.ProductDefinition{APIVersion: catalog.APIVersion, Kind: catalog.Kind, ID: "probe", Name: "probe",
		Versioning: catalog.Versioning{Scheme: domain.SchemeSemver},
		Sources:    []catalog.Source{{ID: "tags", Roles: []domain.SourceRole{domain.RoleVersions}, Locator: catalog.Locator{Kind: catalog.LocatorGitTags, Repository: "github.com/x/y"}}, s}}
	var msgs []string
	for _, i := range catalog.Validate(def).Errors() {
		msgs = append(msgs, i.String())
	}
	return strings.Join(msgs, "; ")
}

const mainChartSchema = `{"type":"object","additionalProperties":false,"required":["main","rationale"],"properties":{"main":{"type":"string"},"rationale":{"type":"string"}}}`

func (l *LLMResolver) mainChart(ctx context.Context, in ResolveInput, d *Draft, q Question, n int) ([]Proposal, error) {
	cands, ids := l.promptCandidates(q.Candidates)
	user := header(in) + "Question: " + q.Summary + " Answer with the chart name (or \"unknown\").\nFindings:\n" + cands
	resp, req, err := l.ask(ctx, user, mainChartSchema)
	if err != nil {
		return nil, err
	}
	var ans struct{ Main, Rationale string }
	if err := json.Unmarshal([]byte(resp.Text), &ans); err != nil {
		return nil, fmt.Errorf("decode answer: %w", err)
	}
	return []Proposal{{ID: fmt.Sprintf("prop-%d-%s", n, q.Kind), Question: q.Kind, Summary: "Main chart: " + ans.Main, Rationale: ans.Rationale,
		Answer: resp.Text, Provenance: l.provenance(q, resp, req, ids), Status: ProposalInformational,
		Detail: "advice only: the definition keeps every published chart"}}, nil
}

const chartVersionSchema = `{"type":"object","additionalProperties":false,"required":["strategy","template","field","match","rationale"],"properties":{"strategy":{"type":"string","enum":["template","lookup","independent","unknown"]},"template":{"type":"string"},"field":{"type":"string"},"match":{"type":"string"},"rationale":{"type":"string"}}}`

func (l *LLMResolver) chartVersion(ctx context.Context, in ResolveInput, d *Draft, q Question, n int) ([]Proposal, error) {
	cands, ids := l.promptCandidates(q.Candidates)
	user := header(in) + "Question: " + q.Summary + "\nCurrent assumption: strategy=" + q.Facts["strategy"] + " " + q.Facts["template"] +
		".\nAnswer with strategy \"template\" (chart version = template such as {{.Tag}} or {{.Version}}), \"lookup\" (find chart versions whose index field, usually appVersion, equals match, e.g. {{.Tag}}), \"independent\" or \"unknown\". Leave unused fields empty.\nFindings:\n" + cands
	resp, req, err := l.ask(ctx, user, chartVersionSchema)
	if err != nil {
		return nil, err
	}
	var ans struct{ Strategy, Template, Field, Match, Rationale string }
	if err := json.Unmarshal([]byte(resp.Text), &ans); err != nil {
		return nil, fmt.Errorf("decode answer: %w", err)
	}
	prov := l.provenance(q, resp, req, ids)
	pid := fmt.Sprintf("prop-%d-%s", n, q.Kind)
	p := Proposal{ID: pid, Question: q.Kind, Rationale: ans.Rationale, Answer: resp.Text, Provenance: prov,
		Summary: fmt.Sprintf("Chart %s: strategy=%s %s%s", q.Facts["chart"], ans.Strategy, ans.Template, strings.TrimSpace(" "+ans.Field+" "+ans.Match))}
	if ans.Strategy == "unknown" {
		p.Status = ProposalInformational
		return []Proposal{p}, nil
	}
	vr := catalog.VersionRelation{Strategy: ans.Strategy, Template: ans.Template, Field: ans.Field, Match: ans.Match}
	if ans.Strategy == catalog.VersionLookup {
		vr.Select = "latest"
	}
	artIDs := splitList(q.Facts["artifacts"])
	if len(artIDs) == 0 {
		artIDs = []string{strings.TrimPrefix(q.Subject, "artifact:")}
	}
	var out []Proposal
	for i, id := range artIDs {
		art, ok := d.Definition.Artifact(id)
		if !ok {
			continue
		}
		pi := p
		pi.ID = fmt.Sprintf("%s-%d", pid, i)
		pi.Summary = fmt.Sprintf("Chart %s: strategy=%s %s%s", art.Name, ans.Strategy, ans.Template, strings.TrimSpace(" "+ans.Field+" "+ans.Match))
		if vr.Strategy == art.Version.Strategy && vr.Template == art.Version.Template && vr.Field == art.Version.Field && vr.Match == art.Version.Match {
			pi.Status, pi.Detail = ProposalInformational, "confirms the deterministic relation"
			out = append(out, pi)
			continue
		}
		v := art
		v.ID = art.ID + "--ai"
		v.Version = vr
		probe := &catalog.ProductDefinition{APIVersion: catalog.APIVersion, Kind: catalog.Kind, ID: "probe", Name: "probe",
			Versioning: catalog.Versioning{Scheme: domain.SchemeSemver},
			Sources:    []catalog.Source{{ID: "tags", Roles: []domain.SourceRole{domain.RoleVersions}, Locator: catalog.Locator{Kind: catalog.LocatorGitTags, Repository: "github.com/x/y"}}},
			Artifacts:  []catalog.Artifact{v}}
		if errs := catalog.Validate(probe).Errors(); len(errs) > 0 {
			pi.Status, pi.Detail = ProposalRejected, errs[0].String()
			out = append(out, pi)
			continue
		}
		key := "artifact:" + art.ID
		d.AIArtifacts = append(d.AIArtifacts, v)
		d.Elements["artifact:"+v.ID] = &Element{Key: "artifact:" + v.ID, Kind: "artifact", ID: v.ID, Origin: OriginAI, Rule: "question:" + q.Kind,
			Confidence: domain.ConfidenceMedium, ProposalID: pi.ID, Replaces: key, Targets: []string{TargetHelmCharts, TargetVersionRelations}}
		pi.Element, pi.Replaces, pi.Status = "artifact:"+v.ID, key, ProposalPending
		out = append(out, pi)
	}
	if len(out) == 0 {
		p.Status = ProposalInformational
		out = append(out, p)
	}
	return out, nil
}
