package runtime

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
)

// Outcome rules (the provenance rule of each record).
const (
	RuleRunNotRun      = "runtime:run-not-run"         // a precondition of the whole run was missing
	RuleRunError       = "runtime:run-error"           // the run broke
	RuleNoObject       = "runtime:no-object"           // nothing on the item's paths is a cluster object
	RuleDegraded       = "runtime:degraded"            // fail: a degradation only this item reaches
	RuleUnattributed   = "runtime:unattributed"        // error: a degradation on the item's paths other items reach too
	RuleOtherFamilies  = "runtime:other-families"      // not-run: degraded under an exercise declared for other families
	RuleInconclusive   = "runtime:inconclusive-change" // error: data changed under a consumer, no consumer degraded
	RuleReconciledOnly = "runtime:reconciled-only"     // not-run: reached only as a reconciled kind, no item-specific link
	RuleUnreached      = "runtime:unreached"           // error: no object on its paths, but its family's exercise degraded its targets
	RuleSandbox        = "runtime:sandbox"             // error: the sandbox broke an object (image pull)
	RuleUpgradeFailed  = "runtime:upgrade-incomplete"
	RuleNoBaseline     = "runtime:no-baseline" // not-run: an object on the paths was not observed healthy before
	RuleNoExercise     = "runtime:no-exercise" // not-run: quiet window without the item's exercise
	RuleExerciseNoRun  = "runtime:exercise-not-run"
	RuleQuiet          = "runtime:healthy-exercised" // pass
)

// Attribution rules: how the degradations on an item's objects were
// attributed to it (recorded on every record after "+" in the provenance
// rule, e.g. "runtime:degraded+attribution:own-rendered-change"). Precedence
// for a degradation several in-scope items reach: (1) the item whose own
// rendered change is on the degraded object, when the degradation concerns
// that object (rejected, refused exercise, deleted, not ready, not
// reconciled, a failure condition, a warning event); (2) otherwise items
// reaching the object through relationship paths share it; (3) items
// reaching it only as a kind the product reconciles never own it.
const (
	AttrNone            = "attribution:none"                // no degradation on the item's objects
	AttrOwnChange       = "attribution:own-rendered-change" // its own rendered change is on the degraded object
	AttrUniquePath      = "attribution:unique-path"         // the only in-scope item whose specific paths reach the object
	AttrSharedOwnChange = "attribution:shared-own-change"   // several items have their own rendered change on it: no guessing
	AttrSharedPath      = "attribution:shared-path"         // reached through paths by several items, or owned by another item's change
	AttrReconciledOnly  = "attribution:reconciled-only"     // reached only as a reconciled kind
	AttrOtherFamilies   = "attribution:other-families"      // degraded under an exercise declared for other families
	AttrUnreached       = "attribution:unreached"           // no object on its paths; its family's exercise targets degraded
	AttrSandbox         = "attribution:sandbox"             // a sandbox problem (image pull)
)

// concernsObject reports whether a degradation is about the object itself —
// what an own rendered change on that object can explain.
func concernsObject(d Degradation) bool {
	if d.Inconclusive || d.Infrastructure {
		return false
	}
	for _, p := range []string{"rejected", "exercise-rejected:", "deleted", "not-ready", "not-reconciled", "condition:", "warning:"} {
		if d.Signature == p || strings.HasPrefix(d.Signature, p) && strings.HasSuffix(p, ":") {
			return true
		}
	}
	return false
}

// ownChanges returns the rendered changes (environment or release scope)
// correlated to the item whose object is the live object.
func (s *session) ownChanges(item, live string) []string {
	var out []string
	for _, ps := range [][]domain.ManifestPair{s.b.Change.Renders, s.b.Renders} {
		for _, p := range ps {
			for _, c := range p.Changes {
				if contains(c.ChangeItems, item) && s.live(c.Object) == live {
					out = append(out, c.ID)
				}
			}
		}
	}
	return uniqStrings(out)
}

// specificReach is the item's reach through roots that are not the
// product-reconciles exposure (a rendered change, a join match).
func (s *session) specificReach(in investigate.ValidationInput, item domain.ChangeItem) []string {
	if in.Traversal == nil {
		return nil
	}
	specific := map[string]bool{}
	for _, r := range in.Traversal.RootsOf(item.ID) {
		if r.Rule != relations.RuleReconciledKind {
			specific[r.Node] = true
		}
	}
	var nodes []string
	for n := range specific {
		nodes = append(nodes, n)
	}
	for _, p := range in.Traversal.PathsOf(item.ID) {
		if len(p.Nodes) > 0 && specific[p.Nodes[0]] {
			nodes = append(nodes, p.Nodes...)
		}
	}
	return s.reach(uniqStrings(nodes))
}

// attributed is one degradation as attributed to one item.
type attributed struct {
	d    Degradation
	rule string
	rcs  []string // the item's own rendered changes on the object (own-change attribution)
}

// attribute decides how one degradation on the item's objects relates to it.
func (s *session) attribute(in investigate.ValidationInput, item domain.ChangeItem, d Degradation) attributed {
	if d.Infrastructure {
		return attributed{d: d, rule: AttrSandbox}
	}
	cands := s.candidates(in, d)
	if !contains(cands, item.ID) {
		return attributed{d: d, rule: AttrOtherFamilies}
	}
	var own []string
	if concernsObject(d) {
		for _, c := range cands {
			if len(s.ownChanges(c, d.Object)) > 0 {
				own = append(own, c)
			}
		}
	}
	switch {
	case len(own) == 1 && own[0] == item.ID:
		d.Detail += fmt.Sprintf(" [its own rendered change %s is on this object]", strings.Join(s.ownChanges(item.ID, d.Object), ", "))
		return attributed{d: d, rule: AttrOwnChange, rcs: s.ownChanges(item.ID, d.Object)}
	case len(own) == 1:
		d.Detail += fmt.Sprintf(" [attributed to %s, whose own rendered change is on this object]", own[0])
		return attributed{d: d, rule: AttrSharedPath}
	case len(own) > 1:
		d.Detail += fmt.Sprintf(" [%d in-scope items have their own rendered change on this object: %s — not guessed]", len(own), strings.Join(own, ", "))
		return attributed{d: d, rule: AttrSharedOwnChange}
	}
	var specific []string
	for _, c := range cands {
		it, _ := in.Bundle.Item(c)
		if contains(s.specificReach(in, it), d.Object) {
			specific = append(specific, c)
		}
	}
	switch {
	case !contains(specific, item.ID):
		d.Detail += " [reached by this item only as a kind the product's controller reconciles]"
		return attributed{d: d, rule: AttrReconciledOnly}
	case len(specific) == 1:
		return attributed{d: d, rule: AttrUniquePath}
	}
	d.Detail += fmt.Sprintf(" [reached through relationship paths by %d in-scope items: %s]", len(specific), strings.Join(specific, ", "))
	return attributed{d: d, rule: AttrSharedPath}
}

func degradationsOf(as []attributed) []Degradation {
	var out []Degradation
	for _, a := range as {
		out = append(out, a.d)
	}
	return out
}

// strongest returns the first attribution rule of the list present in as.
func strongest(as []attributed, order ...string) string {
	for _, r := range order {
		for _, a := range as {
			if a.rule == r {
				return r
			}
		}
	}
	return AttrNone
}

// coverage is what a record speaks for.
type coverage struct {
	subjects, paths []string
}

// coverageOf resolves the requested subjects/paths, or every root and path
// of the item; unknown ids are rejected.
func coverageOf(in investigate.ValidationInput, item domain.ChangeItem) (coverage, error) {
	if len(in.Subjects) > 0 || len(in.Paths) > 0 {
		if len(in.Paths) > 0 {
			if in.Traversal == nil {
				return coverage{}, fmt.Errorf("runtime: %w: paths requested without a traversal", investigate.ErrRejected)
			}
			mine := map[string]bool{}
			for _, p := range in.Traversal.PathsOf(item.ID) {
				mine[p.ID] = true
			}
			for _, id := range in.Paths {
				if !mine[id] {
					return coverage{}, fmt.Errorf("runtime: %w: %s is not an impact path of %s", investigate.ErrRejected, id, item.ID)
				}
			}
		}
		if len(in.Subjects) > 0 && in.Graph == nil {
			return coverage{}, fmt.Errorf("runtime: %w: subjects requested without a graph to check them against", investigate.ErrRejected)
		}
		if len(in.Subjects) > 0 {
			nodes := map[string]bool{}
			for _, n := range in.Graph.Nodes {
				nodes[n.ID] = true
			}
			for _, id := range in.Subjects {
				if !nodes[id] {
					return coverage{}, fmt.Errorf("runtime: %w: subject %s is not a graph node", investigate.ErrRejected, id)
				}
			}
		}
		return coverage{subjects: uniqStrings(in.Subjects), paths: uniqStrings(in.Paths)}, nil
	}
	var cov coverage
	if t := in.Traversal; t != nil {
		for _, r := range t.RootsOf(item.ID) {
			cov.subjects = append(cov.subjects, r.Node)
		}
		for _, p := range t.PathsOf(item.ID) {
			cov.paths = append(cov.paths, p.ID)
		}
	}
	cov.subjects, cov.paths = uniqStrings(cov.subjects), uniqStrings(cov.paths)
	return cov, nil
}

// nodesOf returns the graph nodes a coverage speaks for: its subjects and the
// nodes of its paths.
func nodesOf(in investigate.ValidationInput, cov coverage) []string {
	nodes := append([]string(nil), cov.subjects...)
	if in.Traversal != nil {
		want := map[string]bool{}
		for _, p := range cov.paths {
			want[p] = true
		}
		for _, p := range in.Traversal.Paths {
			if want[p.ID] {
				nodes = append(nodes, p.Nodes...)
			}
		}
	}
	return uniqStrings(nodes)
}

// reach maps graph nodes to the live objects they name.
func (s *session) reach(nodes []string) []string {
	var out []string
	for _, n := range nodes {
		if l := s.resolveNode(n); l != "" {
			out = append(out, l)
		}
	}
	return uniqStrings(out)
}

// itemReach is the full reach of an item (every root and path).
func (s *session) itemReach(in investigate.ValidationInput, item domain.ChangeItem) []string {
	var nodes []string
	if t := in.Traversal; t != nil {
		for _, r := range t.RootsOf(item.ID) {
			nodes = append(nodes, r.Node)
		}
		for _, p := range t.PathsOf(item.ID) {
			nodes = append(nodes, p.Nodes...)
		}
	}
	return s.reach(uniqStrings(nodes))
}

// applicable returns the selected exercises that exercise an item: its
// family is covered and a target lies within its reach.
func (s *session) applicable(item domain.ChangeItem, reach []string) []DeclaredExercise {
	var out []DeclaredExercise
	for _, d := range s.exercises {
		if !d.covers(item.Family) {
			continue
		}
		for _, id := range reach {
			if r, ok := ParseRef(id); ok && d.targets(r) {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

// candidates are the in-scope items a degradation can be attributed to: the
// items whose impact paths reach the object, and — for a degradation the
// exercises caused — whose exercise ran on their paths.
func (s *session) candidates(in investigate.ValidationInput, d Degradation) []string {
	var out []string
	for _, it := range in.Bundle.Items() {
		if !it.InScope {
			continue
		}
		reach := s.itemReach(in, it)
		if !contains(reach, d.Object) {
			continue
		}
		if d.Window == "exercise" && len(s.applicable(it, reach)) == 0 {
			continue
		}
		out = append(out, it.ID)
	}
	return out
}

// draft is a record under construction.
type draft struct {
	outcome     domain.ValidationOutcomeKind
	rule        string
	attribution string // "" = AttrNone
	detail      string
	rcs         []string
	evidence    []domain.Evidence
}

// record derives the item's validation record from the run.
func (s *session) record(in investigate.ValidationInput, item domain.ChangeItem, cov coverage) (*investigate.ValidationOutput, error) {
	d := s.decide(in, item, cov)
	r := s.run
	rec := domain.ValidationRecord{
		Validator:       Name,
		Kind:            domain.ValidationRuntime,
		Level:           domain.LevelObserved,
		ChangeItem:      item.ID,
		Subjects:        cov.subjects,
		Paths:           cov.paths,
		RenderedChanges: uniqStrings(d.rcs),
		Property: fmt.Sprintf("Upgrading %s %s → %s in an isolated cluster installed from this environment's own renders leaves every object on the item's impact paths healthy, before and after the item's declared exercises (observed).",
			r.Product, r.From, r.To),
		Outcome: d.outcome,
		Detail:  strings.TrimSpace(d.detail + s.footer()),
		Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: Name, Rule: d.rule + "+" + attributionOr(d.attribution),
			Confidence: domain.ConfidenceHigh},
		CheckedAt: s.v.clock.Now().UTC().Truncate(time.Second),
	}
	rec.Inputs = uniqStrings(append(append([]string(nil), s.inputs...), s.exArt.ID))
	rec.Outputs = uniqStrings([]string{s.runArt.ID, s.obsArt.ID})
	evs := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range d.evidence {
		evs[e.ID] = e
		rec.Evidence = append(rec.Evidence, e.ID)
	}
	sort.Slice(rec.Evidence, func(i, j int) bool { return rec.Evidence[i] < rec.Evidence[j] })
	if req := in.Request; req != nil {
		rec.Request = req.ID
		if req.Hypothesis != "" {
			rec.Answers = []string{req.Hypothesis}
		}
	}
	rec.ID = domain.ValidationRecordID(rec.Validator, rec.ChangeItem, rec.Subjects, rec.Paths, rec.RenderedChanges, rec.Inputs)
	if err := rec.Validate(); err != nil {
		return nil, fmt.Errorf("runtime: produced an invalid record: %w", err)
	}
	out := &investigate.ValidationOutput{Records: []domain.ValidationRecord{rec}, Artifacts: append([]domain.ArtifactRef(nil), s.arts...)}
	for _, id := range rec.Evidence {
		out.Evidence = append(out.Evidence, evs[id])
	}
	return out, nil
}

func attributionOr(a string) string {
	if a == "" {
		return AttrNone
	}
	return a
}

// OutcomeRule and AttributionRule split a record's provenance rule.
func OutcomeRule(rule string) string {
	o, _, _ := strings.Cut(rule, "+")
	return o
}

// AttributionRule returns the attribution part of a record's provenance rule.
func AttributionRule(rule string) string {
	_, a, _ := strings.Cut(rule, "+")
	return a
}

// inBundle keeps the rendered change ids the bundle indexes.
func (s *session) inBundle(ids []string) []string {
	all := s.b.RenderedChanges()
	var out []string
	for _, id := range uniqStrings(ids) {
		if _, ok := all[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

// footer summarises the run on every record.
func (s *session) footer() string {
	r := s.run
	if r.Status == RunNotRun {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n\nRun: cluster %s", r.Cluster)
	if r.NodeImage != "" {
		fmt.Fprintf(&b, " (node %s)", r.NodeImage)
	}
	fmt.Fprintf(&b, ", %.0fs of %.0fs budget", r.DurationSeconds, r.BudgetSeconds)
	if c := r.Cleanup; c != nil {
		if c.Deleted && c.Verified {
			b.WriteString(", cluster deleted (verified)")
		} else {
			fmt.Fprintf(&b, ", cluster NOT deleted: %s", c.Error)
		}
	}
	b.WriteString(".")
	return b.String()
}

func (s *session) decide(in investigate.ValidationInput, item domain.ChangeItem, cov coverage) draft {
	r := s.run
	switch r.Status {
	case RunNotRun:
		return draft{outcome: domain.ValidationNotRun, rule: RuleRunNotRun, detail: "The isolated upgrade did not run: " + r.Reason}
	case RunError:
		return draft{outcome: domain.ValidationError, rule: RuleRunError, detail: "The isolated upgrade broke, so nothing about this item was observed: " + r.Reason}
	}
	reach := s.reach(nodesOf(in, cov))
	if len(reach) == 0 {
		if ds := s.unreached(item); len(ds) > 0 {
			return draft{outcome: domain.ValidationError, rule: RuleUnreached, attribution: AttrUnreached, evidence: s.evidenceAll(ds), detail: fmt.Sprintf(
				"Inconclusive: no object on this item's impact paths, but the exercises declared for its family (%s) degraded their targets at the TO version, "+
					"which they did not at the FROM version — a plausible impact the relationship graph does not connect to this item, or to any one item: %s.",
				item.Family, summarize(ds))}
		}
		return draft{outcome: domain.ValidationNotRun, rule: RuleNoObject, detail: fmt.Sprintf(
			"No object on the item's impact paths or exposure roots is a Kubernetes object of this environment (%s, %s): there is nothing to observe for it. "+
				"The run itself %s; its observations and any degradations are in the run artifact.",
			plural(len(cov.subjects), "root", "roots"), plural(len(cov.paths), "path", "paths"), s.runSummary())}
	}
	inReach := map[string]bool{}
	for _, id := range reach {
		inReach[id] = true
	}
	var mine, mineData, shared, foreign, sandbox []attributed
	for _, d := range r.Degradations {
		if !inReach[d.Object] {
			continue
		}
		a := s.attribute(in, item, d)
		switch a.rule {
		case AttrSandbox:
			sandbox = append(sandbox, a)
		case AttrOtherFamilies:
			foreign = append(foreign, a)
		case AttrOwnChange, AttrUniquePath:
			if a.d.Inconclusive {
				mineData = append(mineData, a)
			} else {
				mine = append(mine, a)
			}
		default:
			shared = append(shared, a)
		}
	}
	if len(mine) > 0 {
		var b strings.Builder
		b.WriteString("Observed failure: after upgrading " + r.Product + " " + r.From + " → " + r.To + " in an isolated cluster installed from this environment's own renders, objects on this item's impact paths degraded, and nothing in the baseline explains it:")
		var rcs []string
		var evs []domain.Evidence
		for _, a := range mine {
			fmt.Fprintf(&b, "\n- %s (%s window, %s): %s", a.d.Object, a.d.Window, a.rule, a.d.Detail)
			evs = append(evs, s.evidence(a.d))
			rcs = append(rcs, a.rcs...)
		}
		if len(rcs) == 0 { // unique path: cite the item's rendered changes on the degraded objects, if any
			for _, a := range mine {
				rcs = append(rcs, s.ownChanges(item.ID, a.d.Object)...)
			}
		}
		if len(mineData) > 0 {
			fmt.Fprintf(&b, "\nAlso observed (not failures by themselves): %s.", summarize(degradationsOf(mineData)))
		}
		if len(shared)+len(foreign) > 0 {
			fmt.Fprintf(&b, "\nAlso on its paths, not attributed to this item: %s.", summarize(degradationsOf(append(shared, foreign...))))
		}
		attr := strongest(mine, AttrOwnChange, AttrUniquePath)
		fmt.Fprintf(&b, "\nAttribution: %s.", attr)
		return draft{outcome: domain.ValidationFail, rule: RuleDegraded, attribution: attr, detail: b.String(), rcs: s.inBundle(rcs), evidence: evs}
	}
	if len(sandbox) > 0 {
		return draft{outcome: domain.ValidationError, rule: RuleSandbox, attribution: AttrSandbox, evidence: s.evidenceAll(degradationsOf(sandbox)), detail: fmt.Sprintf(
			"The isolated cluster could not run objects on the item's paths (a sandbox problem, not an observation about the environment): %s.", summarize(degradationsOf(sandbox)))}
	}
	if len(shared) > 0 {
		attr := strongest(shared, AttrSharedOwnChange, AttrSharedPath, AttrReconciledOnly)
		return draft{outcome: domain.ValidationError, rule: RuleUnattributed, attribution: attr, evidence: s.evidenceAll(degradationsOf(shared)), detail: fmt.Sprintf(
			"Inconclusive: objects on this item's impact paths degraded during the isolated upgrade, but the observation is not attributable to this item "+
				"(%s; precedence: an item's own rendered change on the degraded object, then relationship paths, never reconciled kinds alone): %s.",
			attr, summarize(degradationsOf(shared)))}
	}
	if len(mineData) > 0 {
		attr := strongest(mineData, AttrOwnChange, AttrUniquePath)
		mineData := degradationsOf(mineData)
		return draft{outcome: domain.ValidationError, rule: RuleInconclusive, attribution: attr, evidence: s.evidenceAll(mineData), detail: fmt.Sprintf(
			"Inconclusive: on this item's impact paths the TO version behaved differently from the FROM version (data changed under a consumer, "+
				"or the controller did not react to an exercise it reacted to before), and nothing in the baseline explains it, but no degradation was observed — "+
				"a plausible impact, not a demonstrated failure, and never a clearance: %s.", summarize(mineData))}
	}
	if len(foreign) > 0 {
		return draft{outcome: domain.ValidationNotRun, rule: RuleOtherFamilies, attribution: AttrOtherFamilies, detail: fmt.Sprintf(
			"Objects on this item's impact paths degraded only under exercises declared for other change families than %s (the degradation is attributed to the items those exercises test), "+
				"so the run says nothing about this item and its objects were not healthy throughout: %s.", item.Family, summarize(degradationsOf(foreign)))}
	}
	if !r.UpgradeCompleted {
		return draft{outcome: domain.ValidationError, rule: RuleUpgradeFailed, detail: fmt.Sprintf(
			"The upgrade did not complete in the isolated cluster, so a healthy state of this item's objects proves nothing: %s.", strings.Join(r.UpgradeProblems, "; "))}
	}
	var unhealthy, checked []string
	for _, phase := range []string{PhaseFrom, PhaseFromExercised, PhaseTo, PhaseToExercised} {
		if s.obs[phase] != nil {
			checked = append(checked, phase)
		}
	}
	for _, id := range reach {
		for _, phase := range checked {
			st, ok := s.obs[phase].Objects[id]
			if !ok {
				unhealthy = append(unhealthy, fmt.Sprintf("%s was not observed at %s", id, phase))
				break
			}
			if ok, why := st.healthy(); !ok {
				switch {
				case phase == PhaseTo && s.removed(id):
					why = "removed by the upgrade (the new release no longer renders it)"
				case phase == PhaseFrom && !st.Exists && st.ApplyError == "":
					why = "does not exist before the upgrade (the new release adds it)"
				}
				unhealthy = append(unhealthy, fmt.Sprintf("%s at %s: %s", id, phase, why))
				break
			}
		}
	}
	if len(unhealthy) > 0 {
		return draft{outcome: domain.ValidationNotRun, rule: RuleNoBaseline, detail: fmt.Sprintf(
			"Objects on the item's impact paths were not observed healthy in every observation (%s) — unsupported in an isolated cluster, unhealthy already at the FROM version, or degraded by the exercises — so no runtime conclusion is possible: %s.",
			strings.Join(checked, ", "), strings.Join(unhealthy, "; "))}
	}
	exs := s.applicable(item, reach)
	if len(exs) == 0 {
		return draft{outcome: domain.ValidationNotRun, rule: RuleNoExercise, detail: fmt.Sprintf(
			"No exercise is declared for family %s with a target on the item's impact paths (objects: %s; declared exercises with targets in this environment: %s). "+
				"A quiet window without the relevant exercise proves nothing, so it is never a pass. The objects stayed healthy through the upgrade.",
			item.Family, kindsOf(reach), keysOfExercises(s.exercises))}
	}
	var ran, notRan []string
	for _, d := range exs {
		if ok, why := s.ranOn(d, reach); ok {
			ran = append(ran, d.Key())
		} else {
			notRan = append(notRan, d.Key()+": "+why)
		}
	}
	if len(ran) == 0 {
		return draft{outcome: domain.ValidationNotRun, rule: RuleExerciseNoRun, detail: fmt.Sprintf(
			"The exercises declared for this item did not run on its objects at both versions, so the window was quiet and proves nothing: %s.", strings.Join(notRan, "; "))}
	}
	if reconciledOnly(in, item, cov) {
		return draft{outcome: domain.ValidationNotRun, rule: RuleReconciledOnly, detail: fmt.Sprintf(
			"The item reaches its objects only as kinds the product's controller reconciles (exposure %s), with no rendered change or join match of its own: "+
				"the exercises (%s) ran and nothing degraded, but they do not show that this particular change was exercised, so the healthy run is not a pass.",
			relations.RuleReconciledKind, strings.Join(ran, ", "))}
	}
	var rcs []string
	for _, rc := range s.correlated(item.ID) {
		if inReach[s.live(rc.Object)] {
			rcs = append(rcs, rc.ID)
		}
	}
	return draft{outcome: domain.ValidationPass, rule: RuleQuiet, rcs: rcs, detail: fmt.Sprintf(
		"Observed healthy: the upgrade completed; every object on the item's impact paths (%s) was healthy in each observation (%s); "+
			"the exercises declared for it (%s) were applied at both versions, the controller showed the declared reaction to them at both, and nothing degraded.",
		plural(len(reach), "object", "objects"), strings.Join(checked, ", "), strings.Join(ran, ", "))}
}

// unreached returns the exercise-window degradations on targets of the
// selected exercises that cover the item's family. They concern an item whose
// paths reach no object only as an unattributed plausible impact (never a
// fail, never a pass).
func (s *session) unreached(item domain.ChangeItem) []Degradation {
	var out []Degradation
	for _, d := range s.run.Degradations {
		if d.Window != "exercise" || d.Infrastructure {
			continue
		}
		r, ok := ParseRef(d.Object)
		if !ok {
			continue
		}
		for _, x := range s.exercises {
			if x.covers(item.Family) && x.targets(r) {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

// reconciledOnly reports whether every root the coverage speaks for comes
// from the product-reconciles exposure rule (no item-specific exposure).
func reconciledOnly(in investigate.ValidationInput, item domain.ChangeItem, cov coverage) bool {
	if in.Traversal == nil {
		return false
	}
	nodes := map[string]bool{}
	for _, n := range cov.subjects {
		nodes[n] = true
	}
	want := map[string]bool{}
	for _, p := range cov.paths {
		want[p] = true
	}
	for _, p := range in.Traversal.PathsOf(item.ID) {
		if want[p.ID] && len(p.Nodes) > 0 {
			nodes[p.Nodes[0]] = true
		}
	}
	found := false
	for _, r := range in.Traversal.RootsOf(item.ID) {
		if !nodes[r.Node] {
			continue
		}
		if r.Rule != relations.RuleReconciledKind {
			return false
		}
		found = true
	}
	return found
}

// ranOn reports whether an exercise was applied to a target within reach at
// both versions.
func (s *session) ranOn(d DeclaredExercise, reach []string) (bool, string) {
	in := map[string]bool{}
	for _, id := range reach {
		in[id] = true
	}
	if d.Exercise.Reaction == nil {
		return false, "it declares no observable controller reaction, so it can reveal failures but never show that the controller acted"
	}
	applied := func(version string) bool {
		for _, id := range s.exRuns[version][d.Key()].Executed {
			if in[id] {
				return true
			}
		}
		return false
	}
	reacted := func(version string) bool {
		for _, id := range s.exRuns[version][d.Key()].Reacted {
			if in[id] {
				return true
			}
		}
		return false
	}
	switch {
	case !applied("from"):
		return false, "not applied to the item's objects at the FROM version"
	case !applied("to"):
		return false, fmt.Sprintf("not applied to the item's objects at the TO version (failed: %v)", s.exRuns["to"][d.Key()].Failed)
	case !reacted("from"):
		return false, "applied, but the controller showed no reaction (" + reactionText(d.Exercise) + ") on the item's objects at the FROM version"
	case !reacted("to"):
		return false, "applied, but the controller showed no reaction (" + reactionText(d.Exercise) + ") on the item's objects at the TO version"
	}
	return true, ""
}

// correlated returns the environment rendered changes correlated to an item.
func (s *session) correlated(item string) []domain.RenderedChange {
	var out []domain.RenderedChange
	for _, p := range s.b.Renders {
		for _, c := range p.Changes {
			if contains(c.ChangeItems, item) {
				out = append(out, c)
			}
		}
	}
	return out
}

// evidence cites the observation artifact for one degradation.
func (s *session) evidence(d Degradation) domain.Evidence {
	a := s.obsArt
	return domain.NewEvidence(domain.EvidenceStructured, a.ID, a.URI,
		fmt.Sprintf("objects[%s] %s window, %s", d.Object, d.Window, d.Signature), d.Detail, a.Digest, time.Time{})
}

func (s *session) evidenceAll(ds []Degradation) []domain.Evidence {
	var out []domain.Evidence
	for _, d := range ds {
		out = append(out, s.evidence(d))
	}
	return out
}

func (s *session) runSummary() string {
	r := s.run
	if !r.UpgradeCompleted {
		return "did not complete the upgrade (" + strings.Join(r.UpgradeProblems, "; ") + ")"
	}
	return fmt.Sprintf("completed the upgrade with %s observed", plural(len(r.Degradations), "degradation", "degradations"))
}

func summarize(ds []Degradation) string {
	var parts []string
	for i, d := range ds {
		if i == 8 {
			parts = append(parts, fmt.Sprintf("… %d more", len(ds)-8))
			break
		}
		parts = append(parts, fmt.Sprintf("%s (%s): %s", d.Object, d.Window, d.Detail))
	}
	return strings.Join(parts, "; ")
}

func kindsOf(ids []string) string {
	set := map[string]bool{}
	for _, id := range ids {
		if r, ok := ParseRef(id); ok {
			k := r.Kind
			if r.Group != "" {
				k = r.Group + "/" + r.Kind
			}
			set[k] = true
		}
	}
	return strings.Join(sortedKeys(set), ", ")
}

func keysOfExercises(xs []DeclaredExercise) string {
	if len(xs) == 0 {
		return "none"
	}
	var out []string
	for _, x := range xs {
		fam := "every family"
		if len(x.Exercise.Families) > 0 {
			fam = strings.Join(x.Exercise.Families, "/")
		}
		out = append(out, fmt.Sprintf("%s on %s (%s)", x.Key(), x.Exercise.Target.Kind, fam))
	}
	return strings.Join(out, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
