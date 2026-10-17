package eval

// Scoring of investigation runs against expected.yaml (DESIGN.md §6.1–§6.2):
// the matcher from `match` to the system's change items, the item decisions,
// component reach (exposureRecall), required evidence (evidenceCoverage) and
// citation resolution.

import (
	"regexp"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// InvRun is one case's run as the scorer sees it: the stored records only
// (never the pipeline), so a stored run re-scores offline.
type InvRun struct {
	Case          string
	Investigation *domain.Investigation
	Bundle        *domain.EvidenceBundle
	// Graph is the run's graph artifact (paths' node keys); nil = paths
	// cannot be resolved to objects (reach then counts rendered changes only).
	Graph *domain.RelationshipGraph
	// ChangeText maps an UpgradeEdge change id to its detail text (the
	// `match` text alternatives run against title + detail).
	ChangeText map[string]string
	// Reproduced: re-classifying the stored records reproduced the findings
	// byte for byte; nil = not checked.
	Reproduced      *bool
	ReproduceDetail string
	// Verdicts are the agent's provisional verdicts (arm B); scored in a
	// separate, ungated table and never read by the policy metrics.
	Verdicts []VerdictInput
	// Error is set when the pipeline failed instead of classifying.
	Error string
}

// Basis buckets (finding Basis; "none" when it rests on no validation).
const (
	BasisDeterministic = "deterministic"
	BasisObserved      = "observed"
	BasisNone          = "none"
)

// MatchedFinding is one system finding selected by an item's `match`.
type MatchedFinding struct {
	ChangeItem    string              `json:"changeItem"`
	Title         string              `json:"title"`
	ScopeRule     string              `json:"scopeRule,omitempty"`
	Family        string              `json:"family,omitempty"`
	Finding       string              `json:"finding"`
	Class         domain.FindingClass `json:"class"`
	Rule          string              `json:"rule"`
	Basis         string              `json:"basis"`
	Manifestation string              `json:"manifestation"`
	Eligible      bool                `json:"eligible"`
	MatchedBy     string              `json:"matchedBy"` // the alternative that selected it
}

// ComponentResult is the reach of one expected component.
type ComponentResult struct {
	Ref            string              `json:"ref"`
	Classification domain.FindingClass `json:"classification"`
	Reached        bool                `json:"reached"`
	// Via says how: "rendered-change <id>" or "path-end <id>"; OnPath is set
	// when the object lies on an impact path without ending it (reported,
	// not counted).
	Via    string `json:"via,omitempty"`
	OnPath bool   `json:"onPath,omitempty"`
}

// EvidenceResult is one requiredEvidence entry.
type EvidenceResult struct {
	Component string `json:"component"`
	File      string `json:"file"`
	Object    string `json:"object,omitempty"`
	Field     string `json:"field,omitempty"`
	// FileCited: an artifact with the path is cited; ObjectNamed/FieldNamed:
	// the object/field is named by cited evidence, rendered changes, nodes or
	// a pin. Covered = all that the entry asks for.
	FileCited   bool `json:"fileCited"`
	ObjectNamed bool `json:"objectNamed"`
	FieldNamed  bool `json:"fieldNamed"`
	Covered     bool `json:"covered"`
}

// ItemDecision is one scored (case, expected item) unit.
type ItemDecision struct {
	Case         string                `json:"case"`
	Item         string                `json:"item"`
	Expected     domain.FindingClass   `json:"expected"`
	Acceptable   []domain.FindingClass `json:"acceptable,omitempty"`
	Verification string                `json:"verification"`
	// System is the strongest class among the matched findings, or
	// insufficient-evidence when nothing matched (MatchMiss) or the run failed.
	System        domain.FindingClass `json:"system"`
	MatchMiss     bool                `json:"matchMiss,omitempty"`
	RunFailed     bool                `json:"runFailed,omitempty"`
	Decider       *MatchedFinding     `json:"decider,omitempty"`
	Matched       []MatchedFinding    `json:"matched,omitempty"`
	Basis         string              `json:"basis"`
	Manifestation string              `json:"manifestation"`
	// Eligible: a matched finding is approval-eligible (counted
	// conservatively: any). ReviewRequired: a matched finding requires review.
	Eligible       bool              `json:"eligible"`
	ReviewRequired bool              `json:"reviewRequired"`
	Correct        bool              `json:"correct"`
	AcceptableAlt  bool              `json:"acceptableAlternative,omitempty"`
	Components     []ComponentResult `json:"components,omitempty"`
	Evidence       []EvidenceResult  `json:"evidence,omitempty"`
	Gaps           []string          `json:"gaps,omitempty"`
	NextSteps      []string          `json:"nextSteps,omitempty"`
	Reasoning      string            `json:"reasoning,omitempty"`
}

// RunScore is the per-run part of the score.
type RunScore struct {
	Case              string                      `json:"case"`
	Investigation     string                      `json:"investigation,omitempty"`
	Bundle            string                      `json:"bundle,omitempty"`
	Error             string                      `json:"error,omitempty"`
	Decision          string                      `json:"decision,omitempty"`
	AgentStatus       string                      `json:"agentStatus,omitempty"`
	Findings          int                         `json:"findings"`
	ClassCounts       map[domain.FindingClass]int `json:"classCounts,omitempty"`
	Citations         int                         `json:"citations"`
	CitationsResolved int                         `json:"citationsResolved"`
	Unresolved        []string                    `json:"unresolvedCitations,omitempty"`
	Reproduced        *bool                       `json:"reproduced,omitempty"`
	ReproduceDetail   string                      `json:"reproduceDetail,omitempty"`
	EnvRenderComplete bool                        `json:"envRenderComplete"`
	Uncorrelated      int                         `json:"uncorrelated"`
	TraversalComplete bool                        `json:"traversalComplete"`
	// UnmatchedFindings counts findings on in-scope items no expected item
	// selects (not scored); UnmatchedAffected lists the affected ones —
	// reviewer noise or exposure the dataset does not label.
	UnmatchedFindings int      `json:"unmatchedFindings"`
	UnmatchedAffected []string `json:"unmatchedAffected,omitempty"`
}

var classRank = map[domain.FindingClass]int{
	domain.FindingNotAffected: 0, domain.FindingInsufficientEvidence: 1,
	domain.FindingPotentiallyAffected: 2, domain.FindingAffected: 3,
}

// ScoreInvCase scores one case's run against its expectations.
func ScoreInvCase(c *InvCase, r *InvRun) ([]ItemDecision, RunScore) {
	rs := RunScore{Case: c.ID}
	if r == nil || r.Error != "" || r.Investigation == nil || r.Bundle == nil {
		if r != nil {
			rs.Error = r.Error
		}
		if rs.Error == "" {
			rs.Error = "no run record"
		}
		var out []ItemDecision
		for _, it := range c.Expected.Items {
			d := newDecision(c, it)
			d.RunFailed, d.System, d.Basis, d.Manifestation = true, domain.FindingInsufficientEvidence, BasisNone, "unmatched"
			d.ReviewRequired = true
			d.Correct = d.System == d.Expected
			out = append(out, d)
		}
		return out, rs
	}
	inv, b := r.Investigation, r.Bundle
	rs.Investigation, rs.Bundle, rs.Findings = inv.ID, b.ID, len(inv.Findings)
	rs.Reproduced, rs.ReproduceDetail = r.Reproduced, r.ReproduceDetail
	if inv.Decision != nil {
		rs.Decision = string(inv.Decision.Outcome)
	}
	if inv.Agent != nil {
		rs.AgentStatus = string(inv.Agent.Status)
	} else {
		rs.AgentStatus = "none"
	}
	rs.ClassCounts = map[domain.FindingClass]int{}
	for _, f := range inv.Findings {
		rs.ClassCounts[f.Class]++
	}
	for _, p := range b.Renders {
		if p.Status == domain.CollectionOK {
			rs.EnvRenderComplete = true
		}
	}
	rs.Uncorrelated = len(b.Uncorrelated())
	rs.TraversalComplete = inv.Traversal != nil && inv.Traversal.Complete
	rs.Citations, rs.CitationsResolved, rs.Unresolved = citationResolution(inv, b)

	ix := newRunIndex(r)
	var out []ItemDecision
	seen := map[string]bool{}
	for _, it := range c.Expected.Items {
		d := ix.decide(c, it)
		for _, m := range d.Matched {
			seen[m.ChangeItem] = true
		}
		out = append(out, d)
	}
	for _, f := range inv.Findings {
		if seen[f.ChangeItem] {
			continue
		}
		rs.UnmatchedFindings++
		if f.Class == domain.FindingAffected {
			it, _ := b.Item(f.ChangeItem)
			rs.UnmatchedAffected = append(rs.UnmatchedAffected, f.ChangeItem+" "+clipText(it.Title, 90))
		}
	}
	return out, rs
}

func newDecision(c *InvCase, it InvExpectedItem) ItemDecision {
	return ItemDecision{Case: c.ID, Item: it.Item, Expected: it.Classification, Acceptable: it.Acceptable, Verification: it.Verification}
}

// runIndex resolves ids of one run.
type runIndex struct {
	r        *InvRun
	findings map[string]domain.InvestigationFinding // by change item
	rendered map[string]domain.RenderedChange
	nodes    map[string]domain.GraphNode
	paths    map[string]domain.ImpactPath
	vals     map[string]domain.ValidationRecord
	evidence map[domain.EvidenceID]domain.Evidence
	arts     map[string]domain.ArtifactRef // by id
	artsByD  map[string]domain.ArtifactRef // by digest
}

func newRunIndex(r *InvRun) *runIndex {
	ix := &runIndex{r: r, findings: map[string]domain.InvestigationFinding{}, rendered: r.Bundle.RenderedChanges(),
		nodes: map[string]domain.GraphNode{}, paths: map[string]domain.ImpactPath{}, vals: map[string]domain.ValidationRecord{},
		evidence: r.Bundle.EvidenceByID(), arts: map[string]domain.ArtifactRef{}, artsByD: map[string]domain.ArtifactRef{}}
	for _, f := range r.Investigation.Findings {
		ix.findings[f.ChangeItem] = f
	}
	if r.Graph != nil {
		for _, n := range r.Graph.Nodes {
			ix.nodes[n.ID] = n
		}
	}
	if t := r.Investigation.Traversal; t != nil {
		for _, p := range t.Paths {
			ix.paths[p.ID] = p
		}
	}
	for _, v := range r.Investigation.Validations {
		ix.vals[v.ID] = v
	}
	for _, e := range r.Investigation.Evidence {
		ix.evidence[e.ID] = e
	}
	for _, a := range append(append([]domain.ArtifactRef{}, r.Bundle.Manifest.Artifacts...), r.Investigation.Artifacts...) {
		ix.arts[a.ID] = a
		ix.artsByD[a.Digest] = a
	}
	return ix
}

// matchItem returns the alternative of m that selects the change item, or "".
func (ix *runIndex) matchItem(m []InvMatch, it domain.ChangeItem) string {
	text := it.Title
	if it.ChangeID != "" {
		if d := ix.r.ChangeText[it.ChangeID]; d != "" {
			text += "\n" + d
		}
	}
	for _, alt := range m {
		if alt.Subject != "" {
			for _, s := range it.Subjects {
				if strings.Contains(s, alt.Subject) {
					return "subject:" + alt.Subject
				}
			}
			continue
		}
		if re, err := regexp.Compile(alt.Text); err == nil && re.MatchString(text) {
			return "text:" + alt.Text
		}
	}
	return ""
}

func basisOf(f domain.InvestigationFinding) string {
	switch f.Basis {
	case domain.ValidationLevel(BasisDeterministic):
		return BasisDeterministic
	case domain.ValidationLevel(BasisObserved):
		return BasisObserved
	}
	return BasisNone
}

func (ix *runIndex) decide(c *InvCase, exp InvExpectedItem) ItemDecision {
	d := newDecision(c, exp)
	var matched []domain.InvestigationFinding
	for _, id := range ix.r.Bundle.InScope() {
		it, ok := ix.r.Bundle.Item(id)
		if !ok {
			continue
		}
		by := ix.matchItem(exp.Match, it)
		if by == "" {
			continue
		}
		f, ok := ix.findings[id]
		if !ok {
			continue // Validate guarantees one finding per in-scope item
		}
		matched = append(matched, f)
		d.Matched = append(d.Matched, MatchedFinding{ChangeItem: id, Title: it.Title, ScopeRule: it.ScopeRule, Family: string(it.Family),
			Finding: f.ID, Class: f.Class, Rule: f.Rule, Basis: basisOf(f), Manifestation: string(f.Manifestation),
			Eligible: f.Approval.Eligible, MatchedBy: by})
	}
	sort.SliceStable(d.Matched, func(i, j int) bool {
		a, b := d.Matched[i], d.Matched[j]
		if classRank[a.Class] != classRank[b.Class] {
			return classRank[a.Class] > classRank[b.Class]
		}
		return a.ChangeItem < b.ChangeItem
	})
	if len(d.Matched) == 0 {
		d.MatchMiss, d.System, d.Basis, d.Manifestation, d.ReviewRequired = true, domain.FindingInsufficientEvidence, BasisNone, "unmatched", true
	} else {
		dec := d.Matched[0]
		d.Decider = &dec
		d.System, d.Basis, d.Manifestation = dec.Class, dec.Basis, dec.Manifestation
		for _, f := range matched {
			if f.ChangeItem == dec.ChangeItem {
				d.Reasoning, d.NextSteps = f.Reasoning, f.NextSteps
				for _, g := range f.Gaps {
					d.Gaps = append(d.Gaps, string(g.Kind)+": "+g.Subject+" — "+g.Detail)
				}
			}
			if f.Approval.Eligible {
				d.Eligible = true
			}
			if f.Approval.RequiresHumanReview {
				d.ReviewRequired = true
			}
		}
	}
	d.Correct = d.System == d.Expected
	for _, a := range d.Acceptable {
		if d.System == a {
			d.AcceptableAlt = true
		}
	}
	cites := ix.citesOf(matched)
	for _, comp := range exp.Components {
		d.Components = append(d.Components, ix.reach(comp, cites))
		for _, re := range comp.RequiredEvidence {
			d.Evidence = append(d.Evidence, ix.covered(comp, re, cites))
		}
	}
	return d
}

// cited is everything the matched findings cite, directly or through the
// validations they cite.
type cited struct {
	rendered []domain.RenderedChange
	pathEnds map[string]string // node id → path id
	onPath   map[string]bool
	nodes    []string // node ids (components, validation subjects, path nodes)
	paths    []string // artifact paths
	texts    []string // evidence URI/locator/excerpt, rendered change paths, pin paths/subjects
}

func (ix *runIndex) citesOf(fs []domain.InvestigationFinding) cited {
	c := cited{pathEnds: map[string]string{}, onPath: map[string]bool{}}
	addEv := func(ids []domain.EvidenceID) {
		for _, id := range ids {
			e, ok := ix.evidence[id]
			if !ok {
				continue
			}
			c.texts = append(c.texts, e.URI+"\n"+e.Locator+"\n"+e.Excerpt)
			if a, ok := ix.artsByD[e.ContentDigest]; ok && e.ContentDigest != "" {
				c.paths = append(c.paths, a.Path)
			}
			if p := envPathOf(e); p != "" {
				c.paths = append(c.paths, p)
			}
		}
	}
	addArts := func(ids []string) {
		for _, id := range ids {
			if a, ok := ix.arts[id]; ok {
				c.paths = append(c.paths, a.Path)
			}
		}
	}
	addRC := func(ids []string) {
		for _, id := range ids {
			if rc, ok := ix.rendered[id]; ok {
				c.rendered = append(c.rendered, rc)
				c.texts = append(c.texts, rc.Object+"\n"+rc.Path)
				addEv(rc.Evidence)
			}
		}
	}
	for _, f := range fs {
		addEv(f.Evidence)
		addArts(f.Artifacts)
		addRC(f.RenderedChanges)
		c.nodes = append(c.nodes, f.Components...)
		for _, pid := range f.Paths {
			p, ok := ix.paths[pid]
			if !ok || len(p.Nodes) == 0 {
				continue
			}
			c.pathEnds[p.Nodes[len(p.Nodes)-1]] = pid
			for _, n := range p.Nodes {
				c.onPath[n] = true
			}
			c.nodes = append(c.nodes, p.Nodes...)
		}
		for _, vid := range f.Validations {
			v, ok := ix.vals[vid]
			if !ok {
				continue
			}
			addEv(v.Evidence)
			addArts(v.Inputs)
			addArts(v.Outputs)
			addRC(v.RenderedChanges)
			c.nodes = append(c.nodes, v.Subjects...)
			if v.Pin != nil {
				addEv(v.Pin.Evidence)
				c.texts = append(c.texts, v.Pin.Subject+"\n"+v.Pin.Path+"\n"+v.Pin.RenderedObject)
				if strings.HasPrefix(v.Pin.Subject, "env/") {
					c.paths = append(c.paths, v.Pin.Subject)
				}
			}
		}
	}
	for _, n := range c.nodes {
		if node, ok := ix.nodes[n]; ok {
			c.texts = append(c.texts, node.Key)
			addArts(node.Artifacts)
			addEv(node.Evidence)
		}
	}
	return c
}

// envPathOf maps environment evidence to its logical artifact path
// ("env/<path in the repo>", DESIGN.md §2.3): local-file and input records
// carry the repo-relative path as their URI.
func envPathOf(e domain.Evidence) string {
	uri := e.URI
	if i := strings.IndexAny(uri, "#?"); i > 0 {
		uri = uri[:i]
	}
	switch {
	case strings.HasPrefix(uri, "env/"):
		return uri
	case e.Kind == domain.EvidenceLocalFile && !strings.Contains(uri, "://") && !strings.HasPrefix(uri, "ri:") && uri != "":
		return "env/" + strings.TrimPrefix(uri, "./")
	}
	return ""
}

// objectKey is a parsed "<group>/<version>/<kind>/<namespace>/<name>".
type objectKey struct{ group, kind, namespace, name string }

func parseObjectKey(s string) (objectKey, bool) {
	p := strings.Split(s, "/")
	if len(p) != 5 || p[2] == "" || p[4] == "" {
		return objectKey{}, false
	}
	return objectKey{group: p[0], kind: p[2], namespace: p[3], name: p[4]}, true
}

// refMatches: kind + name equal, namespace equal (an object rendered without
// a namespace — it relies on the release namespace — matches any), group
// equal when the ref's apiVersion names one.
func refMatches(ref InvRef, k objectKey) bool {
	if ref.Kind == "" || !strings.EqualFold(ref.Kind, k.kind) || ref.Name != k.name {
		return false
	}
	if ref.Namespace != "" && k.namespace != "" && ref.Namespace != k.namespace {
		return false
	}
	if i := strings.LastIndex(ref.APIVersion, "/"); i > 0 {
		g := ref.APIVersion[:i]
		if k.group != "" && k.group != "core" && g != k.group {
			return false
		}
	}
	return true
}

func (ix *runIndex) reach(comp InvComponent, c cited) ComponentResult {
	res := ComponentResult{Ref: comp.Ref.String(), Classification: comp.Classification}
	if comp.Ref.Kind == "" {
		return res // a whole values file: not an object a rendered change or path can reach
	}
	for _, rc := range c.rendered {
		if k, ok := parseObjectKey(rc.Object); ok && refMatches(comp.Ref, k) {
			res.Reached, res.Via = true, "rendered-change "+rc.ID
			return res
		}
	}
	ends := make([]string, 0, len(c.pathEnds))
	for n := range c.pathEnds {
		ends = append(ends, n)
	}
	sort.Strings(ends)
	for _, n := range ends {
		if node, ok := ix.nodes[n]; ok {
			if k, ok := parseObjectKey(node.Key); ok && refMatches(comp.Ref, k) {
				res.Reached, res.Via = true, "path-end "+c.pathEnds[n]
				return res
			}
		}
	}
	for n := range c.onPath {
		if node, ok := ix.nodes[n]; ok {
			if k, ok := parseObjectKey(node.Key); ok && refMatches(comp.Ref, k) {
				res.OnPath = true
			}
		}
	}
	return res
}

// parseObjectRef parses a requiredEvidence object "Kind/ns/name" or
// "Kind/name" into a ref.
func parseObjectRef(s string) InvRef {
	p := strings.Split(s, "/")
	switch len(p) {
	case 2:
		return InvRef{Kind: p[0], Name: p[1]}
	case 3:
		return InvRef{Kind: p[0], Namespace: p[1], Name: p[2]}
	}
	return InvRef{}
}

// fieldTokens are the names a field is recognised by: its leading path (up
// to a space or bracket) and that path's last segment.
func fieldTokens(field string) []string {
	f := field
	if i := strings.IndexAny(f, " ([{"); i > 0 {
		f = f[:i]
	}
	f = strings.TrimSpace(f)
	if f == "" {
		return nil
	}
	toks := []string{f}
	if i := strings.LastIndex(f, "."); i >= 0 && i < len(f)-1 {
		toks = append(toks, f[i+1:])
	}
	return toks
}

func (ix *runIndex) covered(comp InvComponent, re InvRequiredEvidence, c cited) EvidenceResult {
	res := EvidenceResult{Component: comp.Ref.String(), File: re.File, Object: re.Object, Field: re.Field}
	for _, p := range c.paths {
		if p == re.File {
			res.FileCited = true
			break
		}
	}
	if re.Object == "" {
		res.ObjectNamed = true
	} else {
		ref := parseObjectRef(re.Object)
		for _, t := range c.texts {
			for _, line := range strings.Split(t, "\n") {
				if k, ok := parseObjectKey(line); ok && refMatches(ref, k) {
					res.ObjectNamed = true
				}
			}
			if !res.ObjectNamed && ref.Kind != "" && strings.Contains(t, ref.Kind) && strings.Contains(t, ref.Name) {
				res.ObjectNamed = true
			}
		}
	}
	if re.Field == "" {
		res.FieldNamed = true
	} else {
		for _, tok := range fieldTokens(re.Field) {
			for _, t := range c.texts {
				if strings.Contains(t, tok) {
					res.FieldNamed = true
				}
			}
		}
	}
	res.Covered = res.FileCited && res.ObjectNamed && res.FieldNamed
	return res
}

// citationResolution counts every id a finding, assertion, hypothesis or
// validation cites and whether it resolves in the bundle or the run.
func citationResolution(inv *domain.Investigation, b *domain.EvidenceBundle) (total, resolved int, unresolved []string) {
	ev := b.EvidenceByID()
	for _, e := range inv.Evidence {
		ev[e.ID] = e
	}
	arts := map[string]bool{}
	for _, a := range b.Manifest.Artifacts {
		arts[a.ID] = true
	}
	for _, a := range inv.Artifacts {
		arts[a.ID] = true
	}
	rcs := b.RenderedChanges()
	vals, asr, paths := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, v := range inv.Validations {
		vals[v.ID] = true
	}
	for _, a := range inv.Assertions {
		asr[a.ID] = true
	}
	if inv.Traversal != nil {
		for _, p := range inv.Traversal.Paths {
			paths[p.ID] = true
		}
	}
	check := func(owner, kind, id string, ok bool) {
		total++
		if ok {
			resolved++
		} else {
			unresolved = append(unresolved, owner+" → "+kind+" "+id)
		}
	}
	evs := func(owner string, ids []domain.EvidenceID) {
		for _, id := range ids {
			_, ok := ev[id]
			check(owner, "evidence", string(id), ok)
		}
	}
	as := func(owner string, ids []string) {
		for _, id := range ids {
			check(owner, "artifact", id, arts[id])
		}
	}
	for _, f := range inv.Findings {
		evs(f.ID, f.Evidence)
		as(f.ID, f.Artifacts)
		for _, id := range f.RenderedChanges {
			_, ok := rcs[id]
			check(f.ID, "rendered-change", id, ok)
		}
		for _, id := range f.Validations {
			check(f.ID, "validation", id, vals[id])
		}
		for _, id := range f.Paths {
			check(f.ID, "path", id, paths[id])
		}
		for _, id := range f.Assertions {
			check(f.ID, "assertion", id, asr[id])
		}
	}
	for _, a := range inv.Assertions {
		evs(a.ID, a.Citations)
	}
	for _, h := range inv.Hypotheses {
		evs(h.ID, h.Citations)
	}
	for _, v := range inv.Validations {
		evs(v.ID, v.Evidence)
		as(v.ID, v.Inputs)
		as(v.ID, v.Outputs)
		if v.Pin != nil {
			evs(v.ID+" pin", v.Pin.Evidence)
		}
	}
	return total, resolved, unresolved
}
