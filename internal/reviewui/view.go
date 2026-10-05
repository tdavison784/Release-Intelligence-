package reviewui

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// --- enums for dropdowns --------------------------------------------------------------

type kindOption struct{ Kind, Class string }

type enumView struct {
	SubjectFamilies  []domain.SubjectFamily
	ChangeKinds      []domain.ChangeKind
	ConsequenceKinds []kindOption
	Severities       []domain.ImpactSeverity
	Confidences      []domain.Confidence
	QuestionTypes    []domain.QuestionType
	WrongLabels      []domain.FeedbackLabel
	Statuses         []domain.ReviewStatus
	Priorities       []domain.ReviewPriority
}

func enums() enumView {
	e := enumView{
		SubjectFamilies: domain.SubjectFamilies, ChangeKinds: domain.ChangeKinds,
		Severities:    domain.AllImpactSeverities,
		Confidences:   []domain.Confidence{domain.ConfidenceHigh, domain.ConfidenceMedium, domain.ConfidenceLow},
		QuestionTypes: domain.QuestionTypes,
		Priorities:    []domain.ReviewPriority{domain.PriorityHigh, domain.PriorityNormal, domain.PriorityLow},
		Statuses: []domain.ReviewStatus{domain.ReviewPending, domain.ReviewNeedsEvidence, domain.ReviewDeferred,
			domain.ReviewDecided, domain.ReviewSuperseded},
	}
	for _, k := range domain.ConsequenceKinds {
		e.ConsequenceKinds = append(e.ConsequenceKinds, kindOption{string(k), string(k.ExposedClass())})
	}
	for _, l := range domain.FeedbackLabels {
		if strings.HasPrefix(string(l), "wrong-") {
			e.WrongLabels = append(e.WrongLabels, l)
		}
	}
	return e
}

// --- inbox --------------------------------------------------------------------------------

type filterForm struct {
	Product, Release, Subject, Question, Severity, Confidence string
	Model, Disagreement, Source, Reviewer, Priority           string
	Status                                                    []string // selected; "all" = every status
}

func (f filterForm) HasStatus(s string) bool { return slices.Contains(f.Status, s) }

// Active reports whether anything but the default status is filtered.
func (f filterForm) Active() bool {
	return f.Product+f.Release+f.Subject+f.Question+f.Severity+f.Confidence+f.Model+f.Disagreement+f.Source+f.Reviewer+f.Priority != "" ||
		!(len(f.Status) == 1 && f.Status[0] == string(domain.ReviewPending))
}

func parseFilter(q url.Values, limit int) (knowledge.InboxFilter, filterForm) {
	g := func(k string) string { return strings.TrimSpace(q.Get(k)) }
	form := filterForm{
		Product: g("product"), Release: g("release"), Subject: g("subject"), Question: g("question"),
		Severity: g("severity"), Confidence: g("confidence"), Model: g("model"), Disagreement: g("disagreement"),
		Source: g("source"), Reviewer: g("reviewer"), Priority: g("priority"),
	}
	f := knowledge.InboxFilter{
		Product: domain.ProductID(form.Product), Release: form.Release, SubjectType: domain.SubjectFamily(form.Subject),
		QuestionType: domain.QuestionType(form.Question), Severity: domain.ImpactSeverity(form.Severity),
		Confidence: domain.Confidence(form.Confidence), Model: form.Model, Source: form.Source, Reviewer: form.Reviewer, Limit: limit,
	}
	switch domain.ReviewPriority(form.Priority) {
	case domain.PriorityHigh, domain.PriorityNormal, domain.PriorityLow:
		f.Priority = domain.ReviewPriority(form.Priority)
	default:
		form.Priority = ""
	}
	switch form.Disagreement {
	case "yes":
		t := true
		f.Disagreement = &t
	case "no":
		t := false
		f.Disagreement = &t
	default:
		form.Disagreement = ""
	}
	var statuses []domain.ReviewStatus
	all := false
	for _, s := range q["status"] {
		switch domain.ReviewStatus(s) {
		case domain.ReviewPending, domain.ReviewNeedsEvidence, domain.ReviewDeferred, domain.ReviewDecided, domain.ReviewSuperseded:
			statuses = append(statuses, domain.ReviewStatus(s))
			form.Status = append(form.Status, s)
		default:
			all = all || s == "all"
		}
	}
	switch {
	case all:
		statuses, form.Status = nil, []string{"all"}
	case len(statuses) == 0:
		statuses, form.Status = []domain.ReviewStatus{domain.ReviewPending}, []string{string(domain.ReviewPending)}
	}
	f.Status = statuses
	return f, form
}

type tile struct {
	Label, Href, Hint string
	Count             int
	Tone              string
}

func tiles(c knowledge.InboxCounts) []tile {
	href := func(kv ...string) string {
		v := url.Values{}
		for i := 0; i < len(kv); i += 2 {
			v.Add(kv[i], kv[i+1])
		}
		return "/?" + v.Encode()
	}
	return []tile{
		{"Pending", href("status", "pending"), "open questions", c.Pending, "accent"},
		{"High priority", href("status", "pending", "priority", "high"), "route priority high, pending", c.HighPriority, "danger"},
		{"Model disagreement", href("status", "pending", "disagreement", "yes"), "proposals disagree on an aspect", c.ModelDisagreement, "warn"},
		{"Needs semantic mapping", href("status", "pending", "question", "semantic-mapping"), "what changed, exactly?", c.NeedsSemanticMapping, ""},
		{"Applicability", href("status", "pending", "question", "applicability"), "who is exposed?", c.ApplicabilityQuestions, ""},
		{"Needs more evidence", href("status", "needs-evidence"), "waiting for evidence", c.NeedsMoreEvidence, ""},
		{"Deferred", href("status", "deferred"), "parked", c.Deferred, ""},
	}
}

type rowView struct {
	Row       knowledge.InboxRow
	Href      string
	Severity  string
	Agreement string // disagree | agree | single
	AgreeText string // "models disagree" | "consensus · cross-model · 2 calls" | "single call"
}

func newRowView(row knowledge.InboxRow, back string) rowView {
	v := rowView{Row: row, Href: "/items/" + row.Item.ID + "?return=" + url.QueryEscape(back)}
	if c := row.Item.Proposed.Consequence; c != nil && c.Severity != "" {
		v.Severity = string(c.Severity)
	}
	// PO-1: consensus is two SEPARATE calls agreeing, from any models; the
	// scope (cross-model | same-model) only labels it.
	switch {
	case row.Disagreement:
		v.Agreement, v.AgreeText = "disagree", "models disagree"
	case row.Calls >= 2:
		scope := "same-model"
		if len(row.Models) >= 2 {
			scope = "cross-model"
		}
		v.Agreement, v.AgreeText = "agree", fmt.Sprintf("consensus · %s · %d calls", scope, row.Calls)
	default:
		v.Agreement, v.AgreeText = "single", "single call"
	}
	return v
}

type inboxView struct {
	Filter   filterForm
	Counts   knowledge.InboxCounts
	Tiles    []tile
	Rows     []rowView
	Flashes  []flash
	Reviewer string
	Started  string
	Return   string
	Total    int
	// Matches is the filter's full match count; Total < Matches means the
	// list was truncated by Options.InboxLimit and the page must say so.
	Matches int
	Enums   enumView
}

// --- item ----------------------------------------------------------------------------------

type evidenceView struct {
	E       domain.Evidence
	Link    string
	CitedBy []string
}

type validationView struct {
	V        domain.ValidationResult
	Proposal string // model of the proposal checked, if known
}

type matrixCell struct {
	Text         string
	Group        string // A, B, … (same letter = same aspect digest)
	Undetermined bool
	Reason       string
	Empty        bool
	AsProposed   bool
}

type matrixCol struct {
	Model, Provider, Confidence, ID, CallID string
	// Requested is the class the model suggested or REQUESTED (PO-2).
	Requested string
}

type matrixRow struct {
	Aspect     domain.Aspect
	Status     string // agree | disagree | single | none
	StatusText string
	Proposed   string
	Cells      []matrixCell
	Asked      bool // the question verifies this aspect
	// Legend labels each answer group: "A cross-model consensus · 2 calls · B single call".
	Legend []groupLegend
}

type groupLegend struct{ Letter, Text string }

// correctionView is the pre-filled edit form (per aspect).
type correctionView struct {
	Statement                                                                                    string
	HasSubject, HasChange, HasApplicability, HasConsequence                                      bool
	SubjFamily, SubjProduct, SubjGroup, SubjVersion, SubjKind, SubjPath, SubjName, SubjComponent string
	ChangeType, ChangeBefore, ChangeAfter, ChangeReplacedBy                                      string
	Exposure, Overlap                                                                            string
	ConsKind, ConsStatement, ConsRemediation, ConsSeverity, ConsClass                            string
}

type itemView struct {
	Ctx         *knowledge.ReviewContext
	Item        domain.ReviewItem
	Candidate   domain.SemanticCandidate
	Evidence    []evidenceView
	Cols        []matrixCol
	Matrix      []matrixRow
	Proposed    []aspectText
	Validations []validationView
	Related     []domain.ReviewDecision
	Facts       []domain.VerifiedFact
	Rendered    *RenderedDelta
	Correction  *correctionView // nil when the question states nothing to correct
	Editable    bool            // the item can still be decided
	Disagrees   bool
	// ConsensusAction: separate calls agree on an action-eligible consequence
	// and every agreeing proposal REQUESTED action-required (PO-2 (b)). The UI
	// only reports the request; the fact's other conditions are the pipeline's.
	ConsensusAction bool
	// ConsensusDissent lists the calls on the consequence that requested a
	// lower class: they block a consensus ACTION (PO-5).
	ConsensusDissent []string
	ConsensusScope   string

	Reviewer string
	Started  string
	Return   string
	Errors   []string
	Flashes  []flash
	Enums    enumView
	Inline   bool // the fragment inside an inbox card
	Form     url.Values
}

type aspectText struct {
	Aspect domain.Aspect
	Text   string
	Asked  bool
}

func newItemView(rc *knowledge.ReviewContext, rd *RenderedDelta, form url.Values) *itemView {
	v := &itemView{Ctx: rc, Item: rc.Item, Candidate: rc.Candidate, Related: rc.RelatedDecisions, Facts: rc.RelatedFacts,
		Rendered: rd, Editable: eligible(rc.Item.Status), Disagrees: disagrees(rc), Form: form}
	// evidence, with who cited what
	cited := map[domain.EvidenceID][]string{}
	modelOf := map[string]string{}
	for _, p := range rc.Proposals {
		modelOf[p.ID] = p.Provenance.Model
		for _, id := range p.Citations {
			cited[id] = append(cited[id], p.Provenance.Model)
		}
	}
	for _, e := range rc.Candidate.Evidence {
		v.Evidence = append(v.Evidence, evidenceView{E: e, Link: evidenceLink(e), CitedBy: cited[e.ID]})
	}
	asked := rc.Item.Aspects()
	for _, a := range domain.Aspects {
		if rc.Item.Proposed.Has(a) {
			v.Proposed = append(v.Proposed, aspectText{a, describeAspect(rc.Item.Proposed, a), slices.Contains(asked, a)})
		}
	}
	v.buildMatrix(rc, asked)
	for _, val := range rc.Validations {
		v.Validations = append(v.Validations, validationView{V: val, Proposal: modelOf[val.ProposalID]})
	}
	if !rc.Item.Proposed.Empty() {
		v.Correction = newCorrectionView(rc.Item.Proposed, form)
	}
	return v
}

func (v *itemView) buildMatrix(rc *knowledge.ReviewContext, asked []domain.Aspect) {
	props := slices.Clone(rc.Proposals)
	sort.SliceStable(props, func(i, j int) bool { return props[i].Provenance.Model < props[j].Provenance.Model })
	for _, p := range props {
		v.Cols = append(v.Cols, matrixCol{p.Provenance.Model, p.Provider, string(p.Provenance.Confidence), p.ID, p.Provenance.CallID, string(p.SuggestedClass)})
	}
	agree := map[domain.Aspect]knowledge.AspectAgreement{}
	for _, a := range rc.Agreement {
		agree[a.Aspect] = a
	}
	for _, a := range domain.Aspects {
		row := matrixRow{Aspect: a, Asked: slices.Contains(asked, a), Proposed: describeAspect(rc.Item.Proposed, a)}
		// group letters by sorted digest (deterministic)
		digestOf := map[string]string{}
		var digests []string
		for _, p := range props {
			d := p.Assertion.AspectDigest(a)
			if ag, ok := agree[a]; ok {
				for dg, ids := range ag.Groups {
					if slices.Contains(ids, p.ID) {
						d = dg
					}
				}
			}
			if d != "" {
				digestOf[p.ID] = d
				if !slices.Contains(digests, d) {
					digests = append(digests, d)
				}
			}
		}
		sort.Strings(digests)
		models := map[string]bool{}
		any := false
		for _, p := range props {
			cell := matrixCell{}
			switch {
			case digestOf[p.ID] != "":
				cell.Text = describeAspect(p.Assertion, a)
				cell.Group = string(rune('A' + slices.Index(digests, digestOf[p.ID])))
				cell.AsProposed = rc.Item.Proposed.Has(a) && rc.Item.Proposed.AspectDigest(a) == p.Assertion.AspectDigest(a)
				models[p.Provenance.Model] = true
				any = true
			case slices.Contains(p.Undetermined, a):
				cell.Undetermined, cell.Reason = true, p.UndeterminedReason
				any = true
			default:
				cell.Empty = true
			}
			row.Cells = append(row.Cells, cell)
		}
		if !any && !row.Asked && row.Proposed == "" {
			continue
		}
		// groups: the proposals behind each answer, to label consensus (PO-1)
		groups := map[string][]domain.SemanticProposal{}
		for _, p := range props {
			if d := digestOf[p.ID]; d != "" {
				groups[d] = append(groups[d], p)
			}
		}
		consensus := 0
		for i, d := range digests {
			g := groups[d]
			letter := string(rune('A' + i))
			if n := distinctCalls(g); n >= 2 {
				consensus++
				row.Legend = append(row.Legend, groupLegend{letter, fmt.Sprintf("%s consensus · %d calls", domain.ConsensusScopeOf(g), n)})
			} else {
				row.Legend = append(row.Legend, groupLegend{letter, "single call"})
			}
		}
		switch {
		case len(digests) >= 2:
			row.Status, row.StatusText = "disagree", fmt.Sprintf("disagree · %d variants", len(digests))
		case consensus == 1:
			g := groups[digests[0]]
			row.Status, row.StatusText = "agree", fmt.Sprintf("consensus · %s · %d calls", domain.ConsensusScopeOf(g), distinctCalls(g))
			if a == domain.AspectConsequence {
				v.noteConsequenceConsensus(g)
			}
		case len(models) >= 1:
			row.Status, row.StatusText = "single", "single call"
		default:
			row.Status, row.StatusText = "none", "no model committed"
		}
		v.Matrix = append(v.Matrix, row)
	}
}

// distinctCalls counts the separate calls behind a group of proposals.
func distinctCalls(ps []domain.SemanticProposal) int {
	seen := map[string]bool{}
	for _, p := range ps {
		seen[p.Provenance.CallID] = true
	}
	return len(seen)
}

// noteConsequenceConsensus records PO-2 condition (b) for the agreeing group:
// an action-eligible consequence that every agreeing proposal requested as
// action-required, with no validation refuting anything (d).
func (v *itemView) noteConsequenceConsensus(g []domain.SemanticProposal) {
	for _, p := range g {
		if p.SuggestedClass != domain.ImpactActionRequired || p.Assertion.Consequence == nil || !p.Assertion.Consequence.Kind.ActionEligible() {
			return
		}
	}
	for _, val := range v.Ctx.Validations {
		for _, c := range val.Checks {
			if c.Outcome == domain.OutcomeRefuted {
				return
			}
		}
	}
	v.ConsensusScope = string(domain.ConsensusScopeOf(g))
	// CONTRACT-CHANGE(contract-6): PO-5 — a call on the consequence that
	// requested a lower class blocks the consensus ACTION; show who dissents
	if d := domain.ConsequenceDissent([]string{v.Ctx.Item.CandidateID}, v.Ctx.Proposals); len(d) > 0 {
		v.ConsensusDissent = d
		return
	}
	v.ConsensusAction = true
}

func newCorrectionView(a domain.SemanticAssertion, form url.Values) *correctionView {
	c := &correctionView{Statement: a.Statement, HasSubject: a.Subject != nil, HasChange: a.Change != nil,
		HasApplicability: a.Applicability != nil, HasConsequence: a.Consequence != nil}
	if s := a.Subject; s != nil {
		c.SubjFamily, c.SubjProduct, c.SubjGroup, c.SubjVersion = string(s.Family), string(s.Product), s.Group, s.Version
		c.SubjKind, c.SubjPath, c.SubjName, c.SubjComponent = s.Kind, s.Path, s.Name, s.Component
	}
	if ch := a.Change; ch != nil {
		c.ChangeType = string(ch.Type)
		if ch.Before != nil {
			c.ChangeBefore = *ch.Before
		}
		if ch.After != nil {
			c.ChangeAfter = *ch.After
		}
		if ch.ReplacedBy != nil {
			c.ChangeReplacedBy = prettyJSON(ch.ReplacedBy)
		}
	}
	if ap := a.Applicability; ap != nil {
		c.Exposure = prettyJSON(ap.Exposure)
		if ap.Overlap != nil {
			c.Overlap = prettyJSON(ap.Overlap)
		}
	}
	if cs := a.Consequence; cs != nil {
		c.ConsKind, c.ConsStatement, c.ConsRemediation, c.ConsSeverity = string(cs.Kind), cs.Statement, cs.Remediation, string(cs.Severity)
		c.ConsClass = string(cs.Kind.ExposedClass())
	}
	// a re-rendered form after a refusal keeps what the reviewer typed
	over := func(dst *string, key string) {
		if form != nil {
			if vals, ok := form[key]; ok && len(vals) > 0 {
				*dst = vals[0]
			}
		}
	}
	for dst, key := range map[*string]string{
		&c.Statement: "c_statement", &c.SubjFamily: "c_subject_family", &c.SubjProduct: "c_subject_product", &c.SubjGroup: "c_subject_group",
		&c.SubjVersion: "c_subject_version", &c.SubjKind: "c_subject_kind", &c.SubjPath: "c_subject_path", &c.SubjName: "c_subject_name",
		&c.SubjComponent: "c_subject_component", &c.ChangeType: "c_change_type", &c.ChangeBefore: "c_change_before", &c.ChangeAfter: "c_change_after",
		&c.ChangeReplacedBy: "c_change_replacedby", &c.Exposure: "c_app_exposure", &c.Overlap: "c_app_overlap", &c.ConsKind: "c_cons_kind",
		&c.ConsStatement: "c_cons_statement", &c.ConsRemediation: "c_cons_remediation", &c.ConsSeverity: "c_cons_severity",
	} {
		over(dst, key)
	}
	if c.HasConsequence {
		c.ConsClass = string(domain.ConsequenceKind(c.ConsKind).ExposedClass())
	}
	return c
}

func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

// --- evidence links -------------------------------------------------------------------------

var lineLocator = regexp.MustCompile(`^L\d+(-L\d+)?$`)

// evidenceLink returns an openable URL for the evidence, or "" when its URI is
// not a web address (artifact URIs are shown as text). Only http(s).
func evidenceLink(e domain.Evidence) string {
	u, err := url.Parse(e.URI)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	if u.Fragment == "" && u.Host == "github.com" && strings.Contains(u.Path, "/blob/") && lineLocator.MatchString(e.Locator) {
		u.Fragment = e.Locator
	}
	return u.String()
}

// --- describing assertions in words ---------------------------------------------------------

func describeAspect(a domain.SemanticAssertion, x domain.Aspect) string {
	switch x {
	case domain.AspectSubject:
		if a.Subject == nil {
			return ""
		}
		s := a.Subject
		parts := []string{string(s.Family)}
		var id []string
		for _, p := range []string{s.Group, s.Version, s.Kind, s.Name, s.Path, s.Component} {
			if p != "" {
				id = append(id, p)
			}
		}
		return strings.Join(parts, " ") + " · " + string(s.Product) + " · " + strings.Join(id, " / ")
	case domain.AspectChange:
		if a.Change == nil {
			return ""
		}
		c := a.Change
		out := string(c.Type)
		switch {
		case c.Before != nil && c.After != nil:
			out += ": " + *c.Before + " → " + *c.After
		case c.After != nil:
			out += ": → " + *c.After
		case c.Before != nil:
			out += ": " + *c.Before + " →"
		}
		if c.ReplacedBy != nil {
			out += " (replaced by " + c.ReplacedBy.Key() + ")"
		}
		return out
	case domain.AspectApplicability:
		if a.Applicability == nil {
			return ""
		}
		out := "exposed when " + describeCondition(a.Applicability.Exposure, 0)
		if a.Applicability.Overlap != nil {
			out += "\nshielded (overlap) when " + describeCondition(*a.Applicability.Overlap, 0)
		}
		return out
	case domain.AspectConsequence:
		if a.Consequence == nil {
			return ""
		}
		c := a.Consequence
		out := fmt.Sprintf("%s → %s", c.Kind, c.ExposedClass)
		if c.Severity != "" {
			out += " · " + string(c.Severity)
		}
		if c.Statement != "" {
			out += "\n" + c.Statement
		}
		if c.Remediation != "" {
			out += "\nremediation: " + c.Remediation
		}
		return out
	}
	return ""
}

// describeCondition renders the condition tree generically (op, target, state,
// operands); no op is special-cased beyond its structure.
func describeCondition(c domain.Condition, depth int) string {
	ind := strings.Repeat("  ", depth+1)
	switch c.Op {
	case domain.OpAll, domain.OpAny:
		var parts []string
		for _, o := range c.Of {
			parts = append(parts, ind+describeCondition(o, depth+1))
		}
		return string(c.Op) + " of:\n" + strings.Join(parts, "\n")
	case domain.OpNot:
		if len(c.Of) == 1 {
			return "not " + describeCondition(c.Of[0], depth)
		}
	case domain.OpResource, domain.OpRef:
		var sel []string
		for _, p := range []string{c.Group, c.Version, c.Kind, c.Name, c.Path} {
			if p != "" {
				sel = append(sel, p)
			}
		}
		head := string(c.Op) + " " + strings.Join(sel, "/")
		var parts []string
		for _, o := range c.Of {
			parts = append(parts, ind+describeCondition(o, depth+1))
		}
		return head + " where:\n" + strings.Join(parts, "\n")
	case domain.OpUndecidable:
		return fmt.Sprintf("undecidable (%s): %s", c.Reason, c.Needed)
	}
	var sel []string
	for _, p := range []string{c.Group, c.Version, c.Kind, c.Name, c.Path, c.Component} {
		if p != "" {
			sel = append(sel, p)
		}
	}
	out := string(c.Op) + " `" + strings.Join(sel, "/") + "`"
	if c.State != "" {
		out += " " + string(c.State)
	}
	if len(c.Values) > 0 {
		out += " " + strings.Join(c.Values, " | ")
	}
	if c.Pattern != "" {
		out += " /" + c.Pattern + "/"
	}
	if c.Range != "" {
		out += " " + c.Range
	}
	return out
}

// --- template functions --------------------------------------------------------------------------

func funcs() template.FuncMap {
	return template.FuncMap{
		"short": func(id string) string {
			if len(id) > 12 {
				return id[:12] + "…"
			}
			return id
		},
		"ts": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return t.UTC().Format("2006-01-02 15:04 UTC")
		},
		"dur": func(d time.Duration) string { return d.Round(time.Second).String() },
		"join": func(sep string, xs any) string {
			switch v := xs.(type) {
			case []string:
				return strings.Join(v, sep)
			case []domain.FeedbackLabel:
				var s []string
				for _, x := range v {
					s = append(s, string(x))
				}
				return strings.Join(s, sep)
			}
			return ""
		},
		"label":    func(s any) string { return strings.ReplaceAll(fmt.Sprint(s), "-", " ") },
		"selected": func(a, b any) bool { return fmt.Sprint(a) == fmt.Sprint(b) },
		"checked": func(xs []domain.FeedbackLabel, l domain.FeedbackLabel) bool {
			return slices.Contains(xs, l)
		},
		"formVal": func(f url.Values, k string) string {
			if f == nil {
				return ""
			}
			return f.Get(k)
		},
		"formHasKey": func(f url.Values, k string) bool { _, ok := f[k]; return ok },
		"formHas":    func(f url.Values, k, v string) bool { return slices.Contains(f[k], v) },
		"pct":        func(n, d int) string { return fmt.Sprintf("%d/%d", n, d) },
		"relation": func(r RenderRelation) string {
			return strings.ReplaceAll(string(r), "-", " ")
		},
		"aspects": func() []domain.Aspect { return domain.Aspects },
		"statusTone": func(s domain.ReviewStatus) string {
			switch s {
			case domain.ReviewPending:
				return "accent"
			case domain.ReviewNeedsEvidence:
				return "warn"
			case domain.ReviewDecided:
				return "ok"
			}
			return "muted"
		},
		"demo":      func() bool { return false }, // replaced per server (Options.Demo)
		"hasPrefix": strings.HasPrefix,
	}
}
