package agent

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/renderdiff"
)

// DefaultPromptVersion is the prompt set used when the configuration names
// none. Earlier versions (v1–v6) stay embedded so their transcripts replay.
const DefaultPromptVersion = "investigate-agent/v7"

//go:embed prompts
var promptFS embed.FS

// Prompt is one versioned prompt set: the system prompt, the task template
// and the tool schemas the model is offered. A change to any of them is a
// new version (DESIGN.md §4.4).
type Prompt struct {
	Version string
	System  string
	task    *template.Template
	item    *template.Template // per-item task (feature perItem)
	// ToolsJSON is the tool schema list exactly as embedded; it is part of
	// the prompt digest.
	ToolsJSON []byte
	Tools     []ToolSchema
	// MetaJSON is the optional meta.json (part of the digest when present);
	// Features is what it switches on.
	MetaJSON []byte
	Features Features
}

// Features are behaviours a prompt version opts into (meta.json). A version
// without meta.json (v1) has none, so its sessions behave — and replay —
// exactly as recorded.
type Features struct {
	// Agenda: the task lists the agenda (Agenda), the budget scales with it
	// (AgendaBudget), and submit_findings must close every agenda item with
	// a disposition.
	Agenda bool `json:"agenda"`
	// ResolveSubjects: subjects may be named the way a model names them
	// (values keys, setting names, object references, rendered changes,
	// files) and are resolved deterministically to graph nodes.
	ResolveSubjects bool `json:"resolveSubjects"`
	// AnswerLinks: request_validation takes the assertions it should answer
	// and links them into the record's Answers under the coverage guard
	// (answerGuard), so a concern can be lifted by the validation it asked for.
	AnswerLinks bool `json:"answerLinks"`
	// DepthRules: an explains / examined-no-concern record needs
	// get_change_item on its item and a citation no other item shares; a
	// raises-concern on a controller-behaviour/unknown item needs a
	// validation request for the item or a stated reason (depth.go).
	DepthRules bool `json:"depthRules"`
	// PerItem: one focused session per agenda item (item.md task), each with
	// its own budget and one closing disposition; a raises-concern needs an
	// artifact retrieved in the item's own session, else it is an
	// investigation-level note (session.go, focus mode).
	PerItem bool `json:"perItem"`
	// Claims: record_claim (structured, deterministically verified
	// environment claims, claims.go); agent free text is labelled
	// unverified AI prose.
	Claims bool `json:"claims"`
	// Dossier: each item's session starts from its dossier (dossier.go),
	// inlined into the item task and stored as a run artifact; get_dossier
	// serves any item's.
	Dossier bool `json:"dossier"`
	// Verdict: every closing entry except not-examined carries the agent's
	// provisional verdict (verdict.go); never a policy input.
	Verdict bool `json:"verdict"`
	// VerdictInheritsCitations: a verdict that states no citations takes
	// its closing entry's citations once they are checked (never unchecked
	// ones), marked CitationsFromEntry. Needs Verdict.
	VerdictInheritsCitations bool `json:"verdictInheritsCitations"`
	// Mitigation: record_hypothesis takes a structured mitigation, stored
	// only when well-formed (mitigation.go).
	Mitigation bool `json:"mitigation"`
}

// ToolSchema is one tool as offered over MCP (name, description, JSON
// Schema of the arguments).
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// LoadPrompt loads an embedded prompt version ("investigate-agent/v1").
func LoadPrompt(version string) (*Prompt, error) {
	if version == "" {
		version = DefaultPromptVersion
	}
	if strings.Contains(version, "..") || strings.HasPrefix(version, "/") {
		return nil, fmt.Errorf("agent: invalid prompt version %q", version)
	}
	dir := "prompts/" + version + "/"
	read := func(name string) ([]byte, error) {
		b, err := promptFS.ReadFile(dir + name)
		if err != nil {
			return nil, fmt.Errorf("agent: unknown prompt version %q: %w", version, err)
		}
		return b, nil
	}
	sys, err := read("system.md")
	if err != nil {
		return nil, err
	}
	task, err := read("task.md")
	if err != nil {
		return nil, err
	}
	tools, err := read("tools.json")
	if err != nil {
		return nil, err
	}
	p := &Prompt{Version: version, System: string(sys), ToolsJSON: tools}
	if b, err := promptFS.ReadFile(dir + "item.md"); err == nil {
		if p.item, err = template.New("item").Option("missingkey=error").Parse(string(b)); err != nil {
			return nil, fmt.Errorf("agent: prompt %s: item.md: %w", version, err)
		}
	}
	if meta, err := promptFS.ReadFile(dir + "meta.json"); err == nil {
		p.MetaJSON = meta
		dec := json.NewDecoder(bytes.NewReader(meta))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p.Features); err != nil {
			return nil, fmt.Errorf("agent: prompt %s: meta.json: %w", version, err)
		}
	}
	if err := json.Unmarshal(tools, &p.Tools); err != nil {
		return nil, fmt.Errorf("agent: prompt %s: tools.json: %w", version, err)
	}
	have := map[string]bool{}
	for _, t := range p.Tools {
		have[t.Name] = true
	}
	for _, n := range investigate.ToolNames {
		if !have[n] {
			return nil, fmt.Errorf("agent: prompt %s: tools.json lacks tool %s", version, n)
		}
	}
	want := len(investigate.ToolNames)
	if p.Features.Claims {
		if !have[investigate.ToolRecordClaim] {
			return nil, fmt.Errorf("agent: prompt %s: feature claims needs tool %s", version, investigate.ToolRecordClaim)
		}
		want++
	}
	if p.Features.Dossier {
		if !have[investigate.ToolGetDossier] {
			return nil, fmt.Errorf("agent: prompt %s: feature dossier needs tool %s", version, investigate.ToolGetDossier)
		}
		want++
	}
	if p.Features.VerdictInheritsCitations && !p.Features.Verdict {
		return nil, fmt.Errorf("agent: prompt %s: feature verdictInheritsCitations needs feature verdict", version)
	}
	if (p.Features.Dossier || p.Features.Verdict || p.Features.Mitigation) && !p.Features.PerItem {
		return nil, fmt.Errorf("agent: prompt %s: features dossier, verdict and mitigation need per-item sessions", version)
	}
	if len(p.Tools) != want {
		return nil, fmt.Errorf("agent: prompt %s: tools.json lists %d tools, the surface has %d", version, len(p.Tools), want)
	}
	if p.task, err = template.New("task").Option("missingkey=error").Parse(string(task)); err != nil {
		return nil, fmt.Errorf("agent: prompt %s: task.md: %w", version, err)
	}
	return p, nil
}

// Digest is sha256(system ‖ rendered task ‖ tool schemas JSON [‖ meta.json])
// (DESIGN.md §4.4; meta.json only for versions that have one).
func (p *Prompt) Digest(task string) string {
	var b bytes.Buffer
	b.WriteString(p.System)
	b.WriteString(task)
	b.Write(p.ToolsJSON)
	b.Write(p.MetaJSON)
	return domain.Digest(b.Bytes())
}

// Task limits: the task names what exists and how to find the rest; it
// never inlines the bundle (MISSION §A4).
const (
	taskCorrelatedPerItem = 8
	taskUncorrelated      = 30
	taskUnresolved        = 20
	taskExcerpt           = 240
)

// taskData is the task template's input; every list is pre-formatted and
// deterministically ordered.
type taskData struct {
	Product, From, To, Environment, KubernetesVersion, Bundle, Snapshot string
	Items                                                               []taskItem
	Renders                                                             []string
	Uncorrelated                                                        []string
	UncorrelatedCount, UncorrelatedMore                                 int
	Unresolved                                                          []string
	UnresolvedCount, UnresolvedMore                                     int
	OutOfScope                                                          int
	Budget                                                              string
	// Agenda (prompt feature agenda): the items to close, in priority order.
	Agenda []string
}

type taskItem struct {
	ID, Title, Family, Manifestation, ManifestationRule, ScopeRule, Flags, Subjects string
	Citations                                                                       []string
	Correlated                                                                      []string
	CorrelatedCount, CorrelatedMore                                                 int
	Roots, Paths, UnresolvedPaths                                                   int
	Validations                                                                     []string
	JoinFindings                                                                    string
}

// RenderTask renders the task message for an investigation (DESIGN.md
// §4.6): the in-scope items with their upstream citations, the render
// summary, the correlated and uncorrelated rendered changes, the roots and
// unresolved edges, and the validations already run.
func (p *Prompt) RenderTask(in investigate.InvestigateInput, budget domain.AgentBudget) (string, error) {
	return p.renderTask(in, budget, nil)
}

func (p *Prompt) renderTask(in investigate.InvestigateInput, budget domain.AgentBudget, agenda []domain.AgendaCoverage) (string, error) {
	d, err := p.renderTaskData(in, budget)
	if err != nil {
		return "", err
	}
	b := in.Bundle
	titles := map[string]string{}
	for _, it := range b.Items() {
		titles[it.ID] = it.Title
	}
	for i, e := range agenda {
		d.Agenda = append(d.Agenda, fmt.Sprintf("%d. %s [%s] %s", i+1, e.ChangeItem, e.PriorityRule, clip(titles[e.ChangeItem], 120)))
	}
	var out bytes.Buffer
	if err := p.task.Execute(&out, d); err != nil {
		return "", fmt.Errorf("agent: render task: %w", err)
	}
	return out.String(), nil
}

// renderTaskData computes the task's data (shared by the whole-agenda task
// and the per-item task).
func (p *Prompt) renderTaskData(in investigate.InvestigateInput, budget domain.AgentBudget) (*taskData, error) {
	b := in.Bundle
	if b == nil {
		return nil, fmt.Errorf("agent: no bundle")
	}
	d := taskData{
		Product: string(b.Change.Product.ID), From: b.Change.From.Tag, To: b.Change.To.Tag,
		Environment: b.Snapshot.Environment, KubernetesVersion: b.Snapshot.KubernetesVersion,
		Bundle: b.ID, Snapshot: b.Snapshot.ID,
		Budget: fmt.Sprintf("%d tool calls, %d KiB returned, %d turns, %d s", budget.MaxToolCalls, budget.MaxReadBytes>>10, budget.MaxTurns, budget.MaxSeconds),
	}
	pool := b.EvidenceByID()
	envChanges := map[string][]domain.RenderedChange{}
	var uncorrelated []domain.RenderedChange
	for _, pr := range b.Renders {
		for _, c := range pr.Changes {
			if len(c.ChangeItems) == 0 {
				uncorrelated = append(uncorrelated, c)
			}
			for _, it := range c.ChangeItems {
				envChanges[it] = append(envChanges[it], c)
			}
		}
	}
	findings := map[string][]string{}
	if in.Report != nil {
		for _, f := range in.Report.Findings {
			if f.ChangeID != "" {
				findings[f.ChangeID] = append(findings[f.ChangeID], fmt.Sprintf("%s (%s)", f.Classification, f.Rule))
			}
		}
	}
	vals := map[string][]string{}
	for _, v := range in.Validations {
		line := fmt.Sprintf("%s %s → %s", v.ID, v.Validator, v.Outcome)
		if v.Detail != "" {
			line += ": " + clip(v.Detail, taskExcerpt)
		}
		vals[v.ChangeItem] = append(vals[v.ChangeItem], line)
	}
	for _, it := range b.Items() {
		if !it.InScope {
			d.OutOfScope++
			continue
		}
		ti := taskItem{ID: it.ID, Title: it.Title, Family: string(it.Family), Manifestation: string(it.Manifestation),
			ManifestationRule: it.ManifestationRule, ScopeRule: it.ScopeRule, Subjects: strings.Join(it.Subjects, ", "),
			Validations: vals[it.ID]}
		var flags []string
		if it.Breaking {
			flags = append(flags, "declared breaking")
		}
		if it.ActionRequired {
			flags = append(flags, "action required")
		}
		ti.Flags = strings.Join(flags, ", ")
		for _, id := range it.Evidence {
			e, ok := pool[id]
			if !ok {
				continue
			}
			line := e.URI
			if e.Locator != "" {
				line += " " + e.Locator
			}
			if e.Excerpt != "" {
				line += fmt.Sprintf(": %q", clip(e.Excerpt, taskExcerpt))
			}
			ti.Citations = append(ti.Citations, line)
		}
		cs := envChanges[it.ID]
		sortChanges(cs)
		ti.CorrelatedCount = len(cs)
		for i, c := range cs {
			if i == taskCorrelatedPerItem {
				ti.CorrelatedMore = len(cs) - i
				break
			}
			ti.Correlated = append(ti.Correlated, changeLine(c))
		}
		if in.Traversal != nil {
			ti.Roots = len(in.Traversal.RootsOf(it.ID))
			for _, pth := range in.Traversal.PathsOf(it.ID) {
				ti.Paths++
				if !pth.Status.Resolved() {
					ti.UnresolvedPaths++
				}
			}
		}
		if it.ChangeID != "" {
			ti.JoinFindings = strings.Join(findings[it.ChangeID], "; ")
		}
		d.Items = append(d.Items, ti)
	}
	for _, s := range renderdiff.Summaries(b) {
		line := fmt.Sprintf("%s `%s`: %s, %d changes, %d uncorrelated", s.Scope, s.Target, s.Status, s.Changes, s.Uncorrelated)
		if s.Reason != "" {
			line += " — " + clip(s.Reason, taskExcerpt)
		}
		d.Renders = append(d.Renders, line)
	}
	sortChanges(uncorrelated)
	d.UncorrelatedCount = len(uncorrelated)
	for i, c := range uncorrelated {
		if i == taskUncorrelated {
			d.UncorrelatedMore = len(uncorrelated) - i
			break
		}
		d.Uncorrelated = append(d.Uncorrelated, changeLine(c))
	}
	if in.Traversal != nil && in.Graph != nil {
		edges := map[string]domain.GraphEdge{}
		for _, e := range in.Graph.Edges {
			edges[e.ID] = e
		}
		keys := map[string]string{}
		for _, n := range in.Graph.Nodes {
			keys[n.ID] = n.Key
		}
		un := append([]string(nil), in.Traversal.Unresolved...)
		sort.Strings(un)
		d.UnresolvedCount = len(un)
		for i, id := range un {
			if i == taskUnresolved {
				d.UnresolvedMore = len(un) - i
				break
			}
			e, ok := edges[id]
			if !ok {
				d.Unresolved = append(d.Unresolved, id)
				continue
			}
			d.Unresolved = append(d.Unresolved, fmt.Sprintf("%s %s %s → %s (%s): %s", e.ID, e.Kind, keys[e.From], keys[e.To], e.Status, clip(e.Reason, taskExcerpt)))
		}
	}
	return &d, nil
}

func changeLine(c domain.RenderedChange) string {
	line := fmt.Sprintf("%s %s %s", c.ID, c.Class, c.Object)
	if c.Path != "" {
		line += " " + c.Path
	}
	return line
}

func sortChanges(cs []domain.RenderedChange) {
	sort.Slice(cs, func(i, j int) bool { return cs[i].ID < cs[j].ID })
}

// clip shortens s to n bytes on a rune boundary, collapsing whitespace.
func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	for n > 0 && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n] + "…"
}

// itemTaskData is the per-item task's input: one item and pointers to its
// evidence, diffs and relations (the tools fetch the content).
type itemTaskData struct {
	Product, From, To, Environment, KubernetesVersion, Bundle string
	Position, Total, Cap                                      int
	Rule                                                      string
	Item                                                      taskItem
	Sources                                                   []string
	Release                                                   []string
	RootNodes                                                 []string
	PathLines                                                 []string
	Renders                                                   []string
	Budget                                                    string
	// Dossier is the item's dossier JSON (feature dossier) and its artifact.
	Dossier, DossierArtifact string
}

// renderItemTask renders the task of one per-item session.
func (p *Prompt) renderItemTask(in investigate.InvestigateInput, e domain.AgendaCoverage, position, total, cap int, budget domain.AgentBudget, dossier []byte, dossierArt string) (string, error) {
	if p.item == nil {
		return "", fmt.Errorf("agent: prompt %s has no per-item task", p.Version)
	}
	b := in.Bundle
	full, err := p.renderTaskData(in, budget)
	if err != nil {
		return "", err
	}
	d := itemTaskData{Product: full.Product, From: full.From, To: full.To, Environment: full.Environment, KubernetesVersion: full.KubernetesVersion,
		Bundle: b.ID, Position: position, Total: total, Cap: cap, Rule: e.PriorityRule, Renders: full.Renders,
		Budget:  fmt.Sprintf("%d tool calls, %d KiB returned, %d turns, %d s", budget.MaxToolCalls, budget.MaxReadBytes>>10, budget.MaxTurns, budget.MaxSeconds),
		Dossier: strings.TrimSpace(string(dossier)), DossierArtifact: dossierArt}
	for _, ti := range full.Items {
		if ti.ID == e.ChangeItem {
			d.Item = ti
		}
	}
	it, _ := b.Item(e.ChangeItem)
	if d.Item.ID == "" {
		return "", fmt.Errorf("agent: agenda item %s is not an in-scope item of the bundle", e.ChangeItem)
	}
	uris := map[string]bool{}
	for _, id := range it.Evidence {
		if ev, ok := b.EvidenceByID()[id]; ok {
			uris[ev.URI] = true
		}
	}
	for _, a := range b.Manifest.Artifacts {
		if a.Origin == domain.OriginUpstream && (contains(a.ChangeItems, it.ID) || uris[a.URI] || contains(it.Artifacts, a.ID)) {
			d.Sources = append(d.Sources, fmt.Sprintf("%s %s", a.ID, a.Path))
		}
	}
	for _, pr := range b.Change.Renders {
		for _, c := range pr.Changes {
			if contains(c.ChangeItems, it.ID) || c.ID == it.RenderedChange {
				d.Release = append(d.Release, changeLine(c))
			}
		}
	}
	if t := in.Traversal; t != nil {
		keys := map[string]string{}
		if in.Graph != nil {
			for _, n := range in.Graph.Nodes {
				keys[n.ID] = n.Key
			}
		}
		for _, r := range t.RootsOf(it.ID) {
			d.RootNodes = append(d.RootNodes, fmt.Sprintf("%s %s", r.Node, keys[r.Node]))
		}
		for _, pth := range t.PathsOf(it.ID) {
			d.PathLines = append(d.PathLines, fmt.Sprintf("%s (%s, %d nodes)", pth.ID, pth.Status, len(pth.Nodes)))
		}
	}
	var out bytes.Buffer
	if err := p.item.Execute(&out, d); err != nil {
		return "", fmt.Errorf("agent: render item task: %w", err)
	}
	return out.String(), nil
}
