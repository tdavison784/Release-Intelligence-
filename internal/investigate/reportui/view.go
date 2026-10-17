package reportui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	runpkg "github.com/tdavison784/release-intelligence/internal/investigate/run"
)

// classOrder is the attention order of the item list (DESIGN §3.1 "ordering
// of concern"): affected first, not-affected last (and collapsed).
var classOrder = []domain.FindingClass{
	domain.FindingAffected, domain.FindingPotentiallyAffected, domain.FindingInsufficientEvidence, domain.FindingNotAffected,
}

var classLabels = map[domain.FindingClass]string{
	domain.FindingAffected:             "Affected",
	domain.FindingPotentiallyAffected:  "Potentially affected",
	domain.FindingInsufficientEvidence: "Insufficient evidence",
	domain.FindingNotAffected:          "Not affected",
}

var classTones = map[domain.FindingClass]string{
	domain.FindingAffected:             "danger",
	domain.FindingPotentiallyAffected:  "warn",
	domain.FindingInsufficientEvidence: "info",
	domain.FindingNotAffected:          "ok",
}

// ruleLabels explain the §3.1 rule ids in plain words.
var ruleLabels = map[string]string{
	"classify:validated-exposure":    "R1 validated exposure — a deterministic or observed validation demonstrates the change reaches the environment and breaks it",
	"classify:validated-clear":       "R2 validated clear — every clearing condition held, backed by passing validations",
	"classify:plausible-impact":      "R3 plausible impact — the change plausibly reaches the environment and nothing has decided it",
	"classify:insufficient-evidence": "R4 insufficient evidence — missing context or validation prevents a defensible conclusion",
}

// ClassCount is one class with its number of findings.
type ClassCount struct {
	Class domain.FindingClass
	Label string
	Tone  string
	N     int
}

// Row is one investigation in the list.
type Row struct {
	Key, ID                   string
	Product, From, To, Env    string
	Date                      time.Time
	Status                    domain.InvestigationStatus
	Outcome                   domain.PolicyOutcome
	OutcomeLabel, OutcomeTone string
	Review                    domain.ReviewState
	ReviewLabel, ReviewTone   string
	Reviewer                  string
	Counts                    []ClassCount
	Items                     int
	RunID, RunLabel           string
	Agent                     string // agent stage status ("" = not run)
}

// Summarize builds the list row of a record.
func Summarize(rec *Record) Row {
	inv := rec.Investigation
	r := Row{Key: rec.Key, ID: inv.ID, Env: inv.Environment, Date: inv.CreatedAt, Status: inv.Status, Review: inv.Review.State,
		Reviewer: inv.Review.Reviewer, Product: "?", From: "?", To: "?"}
	if b := rec.Bundle; b != nil {
		r.Product = string(b.Change.Product.ID)
		r.From, r.To = versionText(b.Change.From), versionText(b.Change.To)
		if r.Env == "" {
			r.Env = b.Snapshot.Environment
		}
	}
	if rec.Run != nil {
		r.RunID, r.RunLabel = rec.Run.ID, rec.Run.Label()
		if r.Date.IsZero() {
			r.Date = rec.Run.StartedAt
		}
	}
	r.Outcome, r.OutcomeLabel, r.OutcomeTone = outcomeOf(inv)
	r.ReviewLabel, r.ReviewTone = reviewLabel(inv.Review)
	n := map[domain.FindingClass]int{}
	for _, f := range inv.Findings {
		n[f.Class]++
	}
	for _, c := range classOrder {
		r.Counts = append(r.Counts, ClassCount{Class: c, Label: classLabels[c], Tone: classTones[c], N: n[c]})
	}
	r.Items = len(inv.Findings)
	if inv.Agent != nil && inv.Agent.Status != domain.AgentNotRun {
		r.Agent = string(inv.Agent.Status)
	}
	return r
}

func versionText(v domain.Version) string {
	if v.Tag != "" {
		return v.Tag
	}
	if v.Semver != "" {
		return v.Semver
	}
	return "?"
}

func outcomeOf(inv *domain.Investigation) (domain.PolicyOutcome, string, string) {
	if inv.Decision == nil {
		switch inv.Status {
		case domain.InvestigationFailed:
			return "", "failed before a decision", "danger"
		case domain.InvestigationCancelled:
			return "", "cancelled before a decision", "muted"
		}
		return "", "not classified yet", "muted"
	}
	if inv.Decision.Outcome == domain.PolicyAutoApproveEligible {
		return inv.Decision.Outcome, "eligible (not enacted)", "ok"
	}
	return inv.Decision.Outcome, "needs human review", "warn"
}

func reviewLabel(r domain.HumanReview) (string, string) {
	switch r.State {
	case domain.ReviewApproved:
		return "approved", "ok"
	case domain.ReviewRejected:
		return "rejected", "danger"
	case domain.ReviewMoreEvidenceWanted:
		return "more evidence requested", "info"
	case domain.ReviewAwaiting:
		return "awaiting review", "warn"
	case domain.ReviewNotRequired:
		return "not required", "muted"
	}
	return string(r.State), "muted"
}

// Filter selects list rows; empty fields match everything.
type Filter struct {
	Product, Env, Decision, Run string
}

// Decision filter values.
const (
	DecisionReview   = "review"   // human-review-required
	DecisionEligible = "eligible" // auto-approve-eligible
	DecisionNone     = "none"     // not classified
)

// Apply returns the matching rows, newest first.
func (f Filter) Apply(rows []Row) []Row {
	var out []Row
	for _, r := range rows {
		if f.Product != "" && r.Product != f.Product || f.Env != "" && r.Env != f.Env || f.Run != "" && r.RunID != f.Run {
			continue
		}
		switch f.Decision {
		case DecisionReview:
			if r.Outcome != domain.PolicyHumanReview {
				continue
			}
		case DecisionEligible:
			if r.Outcome != domain.PolicyAutoApproveEligible {
				continue
			}
		case DecisionNone:
			if r.Outcome != "" {
				continue
			}
		}
		out = append(out, r)
	}
	SortRows(out)
	return out
}

// SortRows orders rows newest first (then by key, for a stable page).
func SortRows(rows []Row) {
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].Date.Equal(rows[j].Date) {
			return rows[i].Date.After(rows[j].Date)
		}
		return rows[i].Key < rows[j].Key
	})
}

// --- the investigation page ----------------------------------------------------------------

// Links builds the URLs a page uses. In an export every link into the server
// is dropped (the file must stand alone).
type Links struct {
	Base    string // e.g. "/investigations"
	Offline bool   // an exported file: no links into the server
}

func (l Links) List() string { return l.Base }
func (l Links) View(key string) string {
	return l.Base + "/view/" + escapeKey(key)
}
func (l Links) Review(key string) string { return l.Base + "/review/" + escapeKey(key) }
func (l Links) Export(key, format string) string {
	return l.Base + "/export/" + escapeKey(key) + "?format=" + format
}
func (l Links) Artifact(key, digest string) string {
	if l.Offline {
		return ""
	}
	return l.Base + "/artifact/" + escapeKey(key) + "?digest=" + url.QueryEscape(digest)
}

func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// View is everything the investigation page shows.
type View struct {
	Row
	Key, Origin  string
	ReadOnly     bool
	Bundle       string
	Snapshot     string
	ChangeRecord string
	Config       string
	RecordDigest string
	Policy       string
	AutoApproval bool

	Headline     string
	HeadlineTone string
	Decision     []string // plain-English sentences
	Reasons      []string // the policy's own reasons, verbatim
	ReviewLine   string

	BundleMissing bool
	Release       ReleaseView
	Environment   EnvView
	Checked       CheckedView
	Groups        []Group
	AINotes       []AIRecord

	Review   ReviewView
	Audit    []AuditRow
	ChainOK  bool
	ChainErr []string
}

// ReleaseView is the upgrade under investigation.
type ReleaseView struct {
	Product, Name, From, To, Edge string
	Items, InScope                int
	Sources                       []StatusRow
}

// EnvView is the environment the upgrade was investigated against.
type EnvView struct {
	Name, RepoPath, RepoURL, Commit, TreeDigest, KubeVersion string
	Dirty                                                    bool
	Inputs                                                   []StatusRow
	Facts                                                    *ArtifactView
}

// StatusRow is one collected source or environment input.
type StatusRow struct {
	Name, Where string
	Status      domain.CollectionStatus
	Tone        string
	Reason      string
	Artifacts   int
}

// CheckedView is the "what was checked" checklist.
type CheckedView struct {
	Renders    []RenderRow
	Validators []ValidatorRow
	Runtime    Check
	Agent      AgentCheck
	Facts      []FactRow
	Pins       []PinView
	Traversal  Check
	Validated  int
}

// Check is one yes/no line of the checklist.
type Check struct {
	Done   bool
	Tone   string
	Text   string
	Detail []string
}

// RenderRow is one render target.
type RenderRow struct {
	Scope, Target            string
	Status                   domain.CollectionStatus
	Tone, Reason             string
	Changes, Correlated, Unc int
	From, To, Diff           *ArtifactView
}

// ValidatorRow counts the records of one validator by outcome.
type ValidatorRow struct {
	Name, Kind, Level            string
	Pass, Fail, Error, NotRun, N int
}

// AgentCheck says whether the AI investigation ran and how.
type AgentCheck struct {
	Check
	Model, Provider, PromptVersion, PromptDigest, CallID string
	Turns, ToolCalls, Tokens, Covered, Examined          int
	Transcript                                           *ArtifactView
	BudgetRule                                           string
}

// FactRow is one knowledge fact the bundle carried for the validators.
type FactRow struct {
	Artifact  ArtifactView
	ID        string
	Level     string
	Trusted   bool
	Status    string
	Statement string
	Items     []string
	Known     bool // the fact's bytes were readable here
}

// PinView is a pinned setting an override inspection established.
type PinView struct {
	Item, Validation                     string
	Kind, Subject, Path, Value, Rendered string
}

// Group is the findings of one class.
type Group struct {
	Class domain.FindingClass
	Label string
	Tone  string
	Open  bool
	Items []ItemView
}

// ItemView is one finding with everything behind it.
type ItemView struct {
	Finding, Item         string
	Title                 string
	Family, Manifestation string
	ScopeRule             string
	Breaking, ActionReq   bool
	Class                 domain.FindingClass
	ClassLabel, Tone      string
	Rule, RuleLabel       string
	Reason                string
	Reasoning             string
	Basis                 string
	Eligible              bool
	Blockers              []string
	Upstream              []EvidenceView
	Rendered              []RenderedView
	Validations           []ValidationView
	Runtime               []ValidationView
	Gaps                  []domain.EvidenceGap
	NextSteps             []string
	Remediation           string
	Assumptions           []string
	Join                  []domain.ImpactFindingRef
	AI                    []AIRecord
}

// EvidenceView is one evidence record with a link when it is a URL.
type EvidenceView struct {
	ID, Kind, URI, Href, Locator, Excerpt string
}

// ArtifactView is one artifact; Href is empty when this store does not hold
// its bytes (or in an export).
type ArtifactView struct {
	ID, Kind, Path, Digest, URI, Href string
	Redacted                          bool
}

// RenderedView is one rendered change of the environment render.
type RenderedView struct {
	ID, Class, Object, Path, Before, After, Correlation, Target string
	Sensitive                                                   bool
}

// ValidationView is one validation record.
type ValidationView struct {
	ID, Validator, Kind, Level, Outcome, Tone, Property, Detail, Rule, Request string
	Evidence                                                                   []EvidenceView
	Inputs, Outputs                                                            []ArtifactView
	Pin                                                                        *PinView
	Answers                                                                    []string
}

// AIRecord is one piece of AI output, always shown labelled.
type AIRecord struct {
	Kind       string // "concern", "explanation", "mitigation claim", "hypothesis", "verified claim", …
	Tone       string
	ID         string
	Statement  string
	Detail     string
	Unverified bool
	Label      string // "AI · model (provider) · prompt … · call …"
	Citations  []EvidenceView
}

// ReviewView is the human review state and its history.
type ReviewView struct {
	State, Label, Tone, Reviewer, Kind, Note string
	DecidedAt                                *time.Time
	Proxy                                    bool
	History                                  []ReviewEvent
	CanDecide                                bool
	Why                                      string // why no decision can be recorded here
}

// ReviewEvent is one recorded review decision (from the audit trail).
type ReviewEvent struct {
	Seq           int
	At            time.Time
	Actor, Action string
	Detail        string
	Proxy         bool
}

// AuditRow is one hash-chained audit event.
type AuditRow struct {
	domain.AuditEvent
	Digest   string
	OK       bool
	Shown    []string // the first refs (the full list is in the run record)
	MoreRefs int
}

// ArtifactReader reads artifact bytes (nil: none available).
type ArtifactReader func(ctx context.Context, digest string) ([]byte, error)

// Build assembles the page of one record. has reports whether the source
// holds an artifact's bytes; read returns them (both may be nil).
func Build(ctx context.Context, rec *Record, links Links, has func(digest string) bool, read ArtifactReader) *View {
	inv, b := rec.Investigation, rec.Bundle
	v := &View{Row: Summarize(rec), Key: rec.Key, Origin: rec.Origin, Bundle: inv.Bundle, Snapshot: inv.Snapshot,
		ChangeRecord: inv.ChangeRecord, Config: inv.Config, RecordDigest: runpkg.Digest(inv), BundleMissing: b == nil}
	x := newIndex(rec, links, has)

	// Decision in plain English.
	notCleared := 0
	n := map[domain.FindingClass]int{}
	for _, f := range inv.Findings {
		n[f.Class]++
		if f.Class != domain.FindingNotAffected {
			notCleared++
		}
	}
	switch {
	case inv.Decision == nil:
		v.Headline, v.HeadlineTone = "Not decided", "muted"
		v.Decision = append(v.Decision, fmt.Sprintf("The run is %s and has no policy decision yet: nothing about this upgrade is concluded.", inv.Status))
	case inv.Decision.Outcome == domain.PolicyAutoApproveEligible:
		v.Policy, v.AutoApproval = inv.Decision.Policy, inv.Decision.AutoApprovalEnabled
		v.Headline, v.HeadlineTone = "Eligible for approval — a human still decides", "ok"
		v.Decision = append(v.Decision,
			fmt.Sprintf("All %d in-scope change item(s) are cleared by validated evidence, and the investigation met every other eligibility condition of %s.", len(inv.Findings), inv.Decision.Policy))
		if !inv.Decision.AutoApprovalEnabled {
			v.Decision = append(v.Decision, "Automated approval is off (shadow mode): eligibility is computed and measured, never enacted. Nothing was approved automatically.")
		}
	default:
		v.Policy, v.AutoApproval = inv.Decision.Policy, inv.Decision.AutoApprovalEnabled
		v.Headline, v.HeadlineTone = "Needs human review", "warn"
		if notCleared > 0 {
			var parts []string
			for _, c := range classOrder[:3] {
				if n[c] > 0 {
					parts = append(parts, fmt.Sprintf("%d %s", n[c], strings.ToLower(classLabels[c])))
				}
			}
			v.Decision = append(v.Decision, fmt.Sprintf("%d of %d in-scope change item(s) are not cleared: %s.", notCleared, len(inv.Findings), strings.Join(parts, ", ")))
		} else {
			v.Decision = append(v.Decision, fmt.Sprintf("All %d in-scope change item(s) are cleared, but the investigation as a whole is not eligible (see the policy's reasons).", len(inv.Findings)))
		}
		if n[domain.FindingAffected] > 0 {
			v.Decision = append(v.Decision, "Affected means a deterministic or observed validation demonstrated the break; resolve those items before upgrading.")
		}
		v.Decision = append(v.Decision, "Only a human can approve; an AI reviewer's recommendation is recorded but never approves.")
	}
	if inv.Decision != nil {
		v.Reasons = inv.Decision.Reasons
	}
	v.Review = reviewView(inv, rec, inv.Decision != nil)
	v.ReviewLine = reviewLine(inv.Review)

	// Release and environment.
	if b != nil {
		c := b.Change
		v.Release = ReleaseView{Product: string(c.Product.ID), Name: c.Product.Name, From: versionText(c.From), To: versionText(c.To), Edge: c.Edge}
		for _, it := range x.items {
			v.Release.Items++
			if it.InScope {
				v.Release.InScope++
			}
		}
		for _, s := range c.Sources {
			v.Release.Sources = append(v.Release.Sources, StatusRow{Name: string(s.Source), Status: s.Status, Tone: statusTone(s.Status), Reason: s.Reason, Artifacts: len(s.Artifacts)})
		}
		sn := b.Snapshot
		v.Environment = EnvView{Name: sn.Environment, KubeVersion: sn.KubernetesVersion}
		if sn.Repo != nil {
			v.Environment.RepoPath, v.Environment.RepoURL, v.Environment.Commit, v.Environment.TreeDigest, v.Environment.Dirty = sn.Repo.Path, sn.Repo.URL, sn.Repo.Commit, sn.Repo.TreeDigest, sn.Repo.Dirty
		}
		for _, in := range sn.Inputs {
			where := in.Path
			if where == "" {
				where = in.Value
			}
			v.Environment.Inputs = append(v.Environment.Inputs, StatusRow{Name: string(in.Kind), Where: where, Status: in.Status, Tone: statusTone(in.Status), Reason: in.Reason, Artifacts: len(in.Artifacts)})
		}
		if sn.Facts != "" {
			v.Environment.Facts = x.artifact(sn.Facts)
		}
	} else {
		v.Environment.Name = inv.Environment
	}

	v.Checked = checked(ctx, inv, b, x, read)

	// Items, attention first.
	byClass := map[domain.FindingClass][]ItemView{}
	for _, f := range inv.Findings {
		byClass[f.Class] = append(byClass[f.Class], itemView(inv, f, x))
	}
	for _, c := range classOrder {
		items := byClass[c]
		if len(items) == 0 {
			continue
		}
		sort.SliceStable(items, func(i, j int) bool {
			if a, b := strings.ToLower(items[i].Title), strings.ToLower(items[j].Title); a != b {
				return a < b
			}
			return items[i].Item < items[j].Item
		})
		v.Groups = append(v.Groups, Group{Class: c, Label: classLabels[c], Tone: classTones[c], Open: c != domain.FindingNotAffected, Items: items})
	}
	for _, nt := range inv.AgentNotes {
		v.AINotes = append(v.AINotes, AIRecord{Kind: "note", Tone: "muted", ID: nt.ID, Statement: nt.Statement, Detail: nt.Reason,
			Unverified: isUnverified(nt.StatementKind), Label: aiLabel(nt.Provenance), Citations: x.evidence(nt.Citations)})
	}

	// Audit trail with the hash chain checked event by event.
	prev := ""
	var last time.Time
	v.ChainOK = len(inv.Audit) > 0
	for i, e := range inv.Audit {
		row := AuditRow{AuditEvent: e, Digest: e.Digest(), OK: true, Shown: e.Refs}
		if len(e.Refs) > 4 {
			row.Shown, row.MoreRefs = e.Refs[:4], len(e.Refs)-4
		}
		if e.Seq != i+1 {
			row.OK = false
			v.ChainErr = append(v.ChainErr, fmt.Sprintf("event %d has seq %d (must be contiguous from 1)", i+1, e.Seq))
		}
		if e.Prev != prev {
			row.OK = false
			v.ChainErr = append(v.ChainErr, fmt.Sprintf("event %d does not chain to the previous event's digest", e.Seq))
		}
		if e.At.Before(last) {
			row.OK = false
			v.ChainErr = append(v.ChainErr, fmt.Sprintf("event %d goes back in time", e.Seq))
		}
		v.ChainOK = v.ChainOK && row.OK
		prev, last = row.Digest, e.At
		v.Audit = append(v.Audit, row)
	}
	return v
}

func reviewLine(r domain.HumanReview) string {
	label, _ := reviewLabel(r)
	switch r.State {
	case domain.ReviewAwaiting:
		if r.ReviewerKind == domain.ReviewerProxy {
			return "Awaiting a human decision (an AI reviewer's recommendation is recorded below; it does not count)."
		}
		return "Awaiting a human decision."
	case domain.ReviewNotRequired:
		return "No review required by the policy."
	}
	s := label
	if r.Reviewer != "" {
		s += " by " + r.Reviewer
		if r.ReviewerKind != "" {
			s += " (" + string(r.ReviewerKind) + ")"
		}
	}
	if r.DecidedAt != nil {
		s += " on " + r.DecidedAt.UTC().Format("2006-01-02 15:04 MST")
	}
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

func reviewView(inv *domain.Investigation, rec *Record, classified bool) ReviewView {
	r := inv.Review
	rv := ReviewView{State: string(r.State), Reviewer: r.Reviewer, Kind: string(r.ReviewerKind), Note: r.Note, DecidedAt: r.DecidedAt,
		Proxy: r.ReviewerKind == domain.ReviewerProxy}
	rv.Label, rv.Tone = reviewLabel(r)
	for _, e := range inv.Audit {
		if e.Stage != domain.StageHumanReview || e.Event != "decided" {
			continue
		}
		ev := ReviewEvent{Seq: e.Seq, At: e.At, Actor: e.Actor, Detail: e.Detail, Proxy: strings.HasPrefix(e.Actor, "proxy:")}
		if len(e.Refs) > 0 {
			ev.Action = e.Refs[0]
		}
		rv.History = append(rv.History, ev)
	}
	switch {
	case rec.Run != nil:
		rv.Why = "This is a stored evaluation run (read-only). Review decisions are recorded on investigations in an investigation store: run `ri investigate` for this upgrade and environment, then review it here."
	case !classified:
		rv.Why = "The investigation has no policy decision yet; there is nothing to review."
	case r.State == domain.ReviewApproved || r.State == domain.ReviewRejected:
		rv.Why = fmt.Sprintf("Already %s by %s: a decided review is final. Start a new investigation to review again.", r.State, r.Reviewer)
	default:
		rv.CanDecide = true
	}
	return rv
}

func statusTone(s domain.CollectionStatus) string {
	switch s {
	case domain.CollectionOK:
		return "ok"
	case domain.CollectionNotApplicable:
		return "muted"
	case domain.CollectionPartial:
		return "warn"
	}
	return "danger"
}

func outcomeTone(o domain.ValidationOutcomeKind) string {
	switch o {
	case domain.ValidationPass:
		return "ok"
	case domain.ValidationFail:
		return "danger"
	case domain.ValidationError:
		return "warn"
	}
	return "muted"
}

// --- indexes over the record ----------------------------------------------------------------

type index struct {
	key      string
	links    Links
	has      func(string) bool
	items    map[string]domain.ChangeItem
	pool     map[domain.EvidenceID]domain.Evidence
	arts     map[string]domain.ArtifactRef
	rendered map[string]RenderedView
	vals     map[string]domain.ValidationRecord
}

func newIndex(rec *Record, links Links, has func(string) bool) *index {
	inv, b := rec.Investigation, rec.Bundle
	x := &index{key: rec.Key, links: links, has: has, items: map[string]domain.ChangeItem{}, pool: map[domain.EvidenceID]domain.Evidence{},
		arts: map[string]domain.ArtifactRef{}, rendered: map[string]RenderedView{}, vals: map[string]domain.ValidationRecord{}}
	if b != nil {
		for _, it := range b.Change.Items {
			x.items[it.ID] = it
		}
		for _, it := range b.EnvironmentItems {
			x.items[it.ID] = it
		}
		for _, e := range b.Evidence {
			x.pool[e.ID] = e
		}
		for _, a := range b.Manifest.Artifacts {
			x.arts[a.ID] = a
		}
		for _, r := range b.Renders {
			for _, c := range r.Changes {
				x.rendered[c.ID] = RenderedView{ID: c.ID, Class: c.Class, Object: c.Object, Path: c.Path, Before: c.Before, After: c.After,
					Sensitive: c.Sensitive, Correlation: c.Correlation, Target: r.Target}
			}
		}
	}
	for _, e := range inv.Evidence {
		x.pool[e.ID] = e
	}
	for _, a := range inv.Artifacts {
		x.arts[a.ID] = a
	}
	for _, vr := range inv.Validations {
		x.vals[vr.ID] = vr
	}
	return x
}

func (x *index) evidence(ids []domain.EvidenceID) []EvidenceView {
	var out []EvidenceView
	for _, id := range ids {
		e, ok := x.pool[id]
		if !ok {
			out = append(out, EvidenceView{ID: string(id), Kind: "unresolved"})
			continue
		}
		ev := EvidenceView{ID: string(id), Kind: string(e.Kind), URI: e.URI, Locator: e.Locator, Excerpt: e.Excerpt}
		if u, err := url.Parse(e.URI); err == nil && (u.Scheme == "https" || u.Scheme == "http") {
			ev.Href = e.URI
		}
		out = append(out, ev)
	}
	return out
}

func (x *index) artifact(id string) *ArtifactView {
	a, ok := x.arts[id]
	if !ok {
		return &ArtifactView{ID: id}
	}
	av := &ArtifactView{ID: a.ID, Kind: string(a.Kind), Path: a.Path, Digest: a.Digest, URI: a.URI, Redacted: a.Redacted}
	if x.has != nil && x.has(a.Digest) {
		av.Href = x.links.Artifact(x.key, a.Digest)
	}
	return av
}

func (x *index) artifacts(ids []string) []ArtifactView {
	var out []ArtifactView
	for _, id := range ids {
		out = append(out, *x.artifact(id))
	}
	return out
}

// --- the checklist ---------------------------------------------------------------------------

func checked(ctx context.Context, inv *domain.Investigation, b *domain.EvidenceBundle, x *index, read ArtifactReader) CheckedView {
	var c CheckedView
	if b != nil {
		add := func(scope string, pairs []domain.ManifestPair) {
			for _, p := range pairs {
				r := RenderRow{Scope: scope, Target: p.Target, Status: p.Status, Tone: statusTone(p.Status), Reason: p.Reason, Changes: len(p.Changes)}
				for _, ch := range p.Changes {
					if len(ch.ChangeItems) > 0 {
						r.Correlated++
					} else {
						r.Unc++
					}
				}
				if p.From != "" {
					r.From = x.artifact(p.From)
				}
				if p.To != "" {
					r.To = x.artifact(p.To)
				}
				if p.Diff != "" {
					r.Diff = x.artifact(p.Diff)
				}
				c.Renders = append(c.Renders, r)
			}
		}
		add("environment", b.Renders)
		add("release (chart defaults)", b.Change.Renders)
	}

	vals := map[string]*ValidatorRow{}
	var runtime []domain.ValidationRecord
	for _, vr := range inv.Validations {
		row := vals[vr.Validator]
		if row == nil {
			row = &ValidatorRow{Name: vr.Validator, Kind: string(vr.Kind), Level: string(vr.Level)}
			vals[vr.Validator] = row
		}
		row.N++
		switch vr.Outcome {
		case domain.ValidationPass:
			row.Pass++
		case domain.ValidationFail:
			row.Fail++
		case domain.ValidationError:
			row.Error++
		default:
			row.NotRun++
		}
		if vr.Kind == domain.ValidationRuntime {
			runtime = append(runtime, vr)
		}
		if vr.Pin != nil {
			c.Pins = append(c.Pins, pinView(vr))
		}
	}
	for _, r := range vals {
		c.Validators = append(c.Validators, *r)
	}
	sort.Slice(c.Validators, func(i, j int) bool { return c.Validators[i].Name < c.Validators[j].Name })
	c.Validated = len(inv.Validations)

	if len(runtime) == 0 {
		c.Runtime = Check{Text: "No — the runtime validator (an isolated upgrade in a throwaway cluster) was not run for this investigation (opt-in: --runtime).", Tone: "muted"}
	} else {
		n := map[domain.ValidationOutcomeKind]int{}
		for _, r := range runtime {
			n[r.Outcome]++
		}
		c.Runtime = Check{Done: true, Tone: "ok", Text: fmt.Sprintf("Yes — %d observed-level record(s): %d pass, %d fail, %d inconclusive, %d not run.",
			len(runtime), n[domain.ValidationPass], n[domain.ValidationFail], n[domain.ValidationError], n[domain.ValidationNotRun])}
	}

	a := inv.Agent
	switch {
	case a == nil || a.Status == domain.AgentNotRun:
		c.Agent.Check = Check{Text: "No — the investigation agent (AI) did not run; every finding rests on deterministic evidence only.", Tone: "muted"}
		if a != nil && a.Reason != "" {
			c.Agent.Detail = []string{a.Reason}
		}
	default:
		c.Agent = AgentCheck{Model: a.Model, Provider: a.Provider, PromptVersion: a.PromptVersion, PromptDigest: a.PromptDigest, CallID: a.CallID,
			Turns: a.Turns, ToolCalls: a.ToolCalls, Tokens: a.Tokens, BudgetRule: a.BudgetRule}
		for _, cv := range a.Coverage {
			c.Agent.Covered++
			if cv.Examined {
				c.Agent.Examined++
			}
		}
		c.Agent.Check = Check{Done: true, Tone: "ai", Text: fmt.Sprintf("Yes — %s (%s), ended %s. AI output is advisory: it can raise concern and request validation, never decide or lower a class.",
			orDash(a.Model), orDash(a.Provider), a.Status)}
		if a.Reason != "" {
			c.Agent.Detail = []string{a.Reason}
		}
		if a.Transcript != "" {
			c.Agent.Transcript = x.artifact(a.Transcript)
		}
	}

	t := inv.Traversal
	switch {
	case t == nil:
		c.Traversal = Check{Tone: "muted", Text: "No relationship traversal recorded."}
	case t.Complete:
		c.Traversal = Check{Done: true, Tone: "ok", Text: fmt.Sprintf("Complete — %d exposure root(s), %d path(s), %d node(s) visited (%s, depth ≤ %d).",
			len(t.Roots), len(t.Paths), len(t.Visited), t.Algorithm, t.Bound.MaxDepth)}
	default:
		c.Traversal = Check{Done: true, Tone: "warn", Text: fmt.Sprintf("Incomplete — %d path(s), %d unresolved edge(s).", len(t.Paths), len(t.Unresolved))}
		if t.Truncated {
			c.Traversal.Detail = append(c.Traversal.Detail, "truncated: "+t.TruncationReason)
		}
	}

	if b != nil {
		for _, ar := range b.Manifest.Artifacts {
			if ar.Kind != domain.ArtifactKnowledgeFact {
				continue
			}
			fr := FactRow{Artifact: *x.artifact(ar.ID), Items: ar.ChangeItems}
			if read != nil {
				if raw, err := read(ctx, ar.Digest); err == nil {
					if f, ok := parseFact(raw); ok {
						fr.Known, fr.ID, fr.Level, fr.Status = true, f.ID, string(f.Level()), string(f.Status)
						fr.Trusted = f.Level().Trusted()
						fr.Statement = f.Assertion.Statement
					}
				}
			}
			c.Facts = append(c.Facts, fr)
		}
	}
	return c
}

// parseFact reads a verified fact in its knowledge/ record form
// ({"kind":"fact","fact":{…}}) or bare.
func parseFact(raw []byte) (domain.VerifiedFact, bool) {
	var rec struct {
		Fact *domain.VerifiedFact `json:"fact"`
	}
	if err := json.Unmarshal(raw, &rec); err == nil && rec.Fact != nil && rec.Fact.ID != "" {
		return *rec.Fact, true
	}
	var f domain.VerifiedFact
	if err := json.Unmarshal(raw, &f); err != nil || f.ID == "" {
		return f, false
	}
	return f, true
}

func pinView(vr domain.ValidationRecord) PinView {
	p := vr.Pin
	return PinView{Item: vr.ChangeItem, Validation: vr.ID, Kind: string(p.Kind), Subject: p.Subject, Path: p.Path, Value: p.Value, Rendered: p.RenderedObject}
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// --- one item ------------------------------------------------------------------------------

func itemView(inv *domain.Investigation, f domain.InvestigationFinding, x *index) ItemView {
	iv := ItemView{Finding: f.ID, Item: f.ChangeItem, Title: f.ChangeItem, Manifestation: string(f.Manifestation), Class: f.Class,
		ClassLabel: classLabels[f.Class], Tone: classTones[f.Class], Rule: f.Rule, RuleLabel: ruleLabels[f.Rule], Reasoning: f.Reasoning,
		Basis: string(f.Basis), Eligible: f.Approval.Eligible, Blockers: f.Approval.Blockers, Gaps: f.Gaps, NextSteps: f.NextSteps,
		Remediation: f.Remediation, Assumptions: f.Assumptions, Join: f.ImpactFindings}
	if iv.RuleLabel == "" {
		iv.RuleLabel = f.Rule
	}
	ev := f.Evidence
	if it, ok := x.items[f.ChangeItem]; ok {
		iv.Title, iv.Family, iv.ScopeRule, iv.Breaking, iv.ActionReq = it.Title, string(it.Family), it.ScopeRule, it.Breaking, it.ActionRequired
		ev = it.Evidence
	}
	iv.Upstream = x.evidence(ev)
	for _, id := range f.RenderedChanges {
		if rc, ok := x.rendered[id]; ok {
			iv.Rendered = append(iv.Rendered, rc)
		} else {
			iv.Rendered = append(iv.Rendered, RenderedView{ID: id, Object: "(not in this bundle)"})
		}
	}
	seen := map[string]bool{}
	addVal := func(vr domain.ValidationRecord) {
		if seen[vr.ID] {
			return
		}
		seen[vr.ID] = true
		vv := ValidationView{ID: vr.ID, Validator: vr.Validator, Kind: string(vr.Kind), Level: string(vr.Level), Outcome: string(vr.Outcome),
			Tone: outcomeTone(vr.Outcome), Property: vr.Property, Detail: vr.Detail, Rule: vr.Provenance.Rule, Request: vr.Request,
			Evidence: x.evidence(vr.Evidence), Inputs: x.artifacts(vr.Inputs), Outputs: x.artifacts(vr.Outputs), Answers: vr.Answers}
		if vr.Pin != nil {
			p := pinView(vr)
			vv.Pin = &p
		}
		if vr.Kind == domain.ValidationRuntime {
			iv.Runtime = append(iv.Runtime, vv)
		} else {
			iv.Validations = append(iv.Validations, vv)
		}
	}
	// The finding's cited validations first, then every other record of the
	// item (not-run and error records explain the gaps).
	for _, id := range f.Validations {
		if vr, ok := x.vals[id]; ok {
			addVal(vr)
		}
	}
	for _, vr := range inv.Validations {
		if vr.ChangeItem == f.ChangeItem {
			addVal(vr)
		}
	}
	iv.Reason = oneReason(f, x)
	iv.AI = aiRecords(inv, f.ChangeItem, x)
	return iv
}

// oneReason is the one-sentence reason of a finding, from its structured
// fields (never from AI output).
func oneReason(f domain.InvestigationFinding, x *index) string {
	switch f.Class {
	case domain.FindingAffected:
		for _, id := range f.Validations {
			if vr, ok := x.vals[id]; ok && vr.Outcome == domain.ValidationFail {
				return fmt.Sprintf("%s failed at the %s level: %s", vr.Validator, vr.Level, firstLine(vr.Detail))
			}
		}
		return "A deterministic or observed validation demonstrated the break."
	case domain.FindingNotAffected:
		var names []string
		seen := map[string]bool{}
		for _, id := range f.Validations {
			if vr, ok := x.vals[id]; ok && vr.Outcome == domain.ValidationPass && !seen[vr.Validator] {
				seen[vr.Validator] = true
				names = append(names, vr.Validator)
			}
		}
		return fmt.Sprintf("Cleared at the %s level by %d passing validation(s) (%s); no gap, open concern or unvalidated assumption remains.",
			orDash(string(f.Basis)), len(f.Validations), strings.Join(names, ", "))
	case domain.FindingPotentiallyAffected:
		var sig []string
		if n := len(f.RenderedChanges); n > 0 {
			sig = append(sig, fmt.Sprintf("%d rendered change(s) of the environment correlate to it", n))
		}
		if n := len(f.Paths); n > 0 {
			sig = append(sig, fmt.Sprintf("%d impact path(s) reach it", n))
		}
		errs := 0
		for _, id := range f.Validations {
			if vr, ok := x.vals[id]; ok && vr.Outcome == domain.ValidationError {
				errs++
			}
		}
		if errs > 0 {
			sig = append(sig, fmt.Sprintf("%d validation(s) were inconclusive", errs))
		}
		if len(sig) == 0 {
			sig = append(sig, "a plausible-impact signal exists")
		}
		return "Plausible impact, not decided: " + strings.Join(sig, "; ") + " — and no validation has shown it harmless or broken."
	}
	if len(f.Gaps) > 0 {
		return "Not enough evidence to decide: " + firstLine(f.Gaps[0].Detail)
	}
	return "Not enough evidence to decide."
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimRight(strings.TrimSpace(s[:i]), ":")
	}
	if r := []rune(s); len(r) > 280 {
		s = string(r[:279]) + "…"
	}
	return s
}

// --- AI output ------------------------------------------------------------------------------

func aiLabel(p domain.Provenance) string {
	s := "AI · " + orDash(p.Model)
	if prov := p.Provider; prov != "" {
		s += " (" + prov + ")"
	}
	if p.PromptVersion != "" {
		s += " · prompt " + p.PromptVersion
	}
	if p.CallID != "" {
		s += " · call " + p.CallID
	}
	return s
}

func isUnverified(kind string) bool { return strings.Contains(kind, "unverified") }

var stanceLabels = map[domain.AssertionStance]string{
	domain.StanceRaisesConcern:    "concern",
	domain.StanceExplains:         "explanation",
	domain.StanceClaimsMitigation: "mitigation claim",
}

func aiRecords(inv *domain.Investigation, item string, x *index) []AIRecord {
	var out []AIRecord
	if inv.Agent != nil {
		for _, cv := range inv.Agent.Coverage {
			if cv.ChangeItem == item && cv.Summary != "" {
				p := domain.Provenance{Model: inv.Agent.Model, Provider: inv.Agent.Provider, PromptVersion: inv.Agent.PromptVersion, CallID: cv.CallID}
				out = append(out, AIRecord{Kind: "summary", Tone: "ai", Statement: cv.Summary,
					Detail:     fmt.Sprintf("disposition %s · %s", cv.Disposition, cv.PriorityRule),
					Unverified: isUnverified(cv.SummaryKind), Label: aiLabel(p), Citations: x.evidence(cv.Citations)})
			}
		}
	}
	for _, a := range inv.Assertions {
		if a.ChangeItem != item {
			continue
		}
		r := AIRecord{Kind: stanceLabels[a.Stance], Tone: "ai", ID: a.ID, Statement: a.Statement, Unverified: isUnverified(a.StatementKind),
			Label: aiLabel(a.Provenance), Citations: x.evidence(a.Citations)}
		if r.Kind == "" {
			r.Kind = string(a.Stance)
		}
		if a.Stance == domain.StanceRaisesConcern {
			r.Tone = "warn"
			if answered(inv, a.ID) {
				r.Detail = "answered by a passing validation"
			} else {
				r.Detail = "not answered by any passing validation — it blocks clearing this item"
			}
		}
		if a.NoValidationReason != "" {
			r.Detail = strings.TrimPrefix(r.Detail+" · ", " · ") + "no validation requested: " + a.NoValidationReason
		}
		out = append(out, r)
	}
	for _, h := range inv.Hypotheses {
		if h.ChangeItem == item {
			out = append(out, AIRecord{Kind: "hypothesis", Tone: "ai", ID: h.ID, Statement: h.Statement, Unverified: isUnverified(h.StatementKind),
				Label: aiLabel(h.Provenance), Citations: x.evidence(h.Citations)})
		}
	}
	for _, c := range inv.AgentClaims {
		if c.ChangeItem != item {
			continue
		}
		st := fmt.Sprintf("%s: %s", c.Kind, c.Setting)
		if c.Value != "" {
			st += " = " + c.Value
		}
		st += " (in " + c.Input + ")"
		out = append(out, AIRecord{Kind: "verified claim", Tone: "ai", ID: c.ID, Statement: st,
			Detail: "checked by " + c.Verifier + ": " + c.Detail, Label: aiLabel(c.Provenance), Citations: x.evidence(c.Evidence)})
	}
	for _, q := range inv.ValidationRequests {
		if q.ChangeItem != item {
			continue
		}
		d := string(q.Status)
		if q.Result != "" {
			d += " → " + q.Result
		}
		if q.Resolution != "" {
			d += ": " + q.Resolution
		}
		out = append(out, AIRecord{Kind: "validation request", Tone: "ai", ID: q.ID, Statement: fmt.Sprintf("asked for a %s validation", q.Kind),
			Detail: d, Label: aiLabel(q.Provenance)})
	}
	for _, q := range inv.EvidenceRequests {
		if q.ChangeItem != item {
			continue
		}
		d := string(q.Status)
		if q.Resolution != "" {
			d += ": " + q.Resolution
		}
		out = append(out, AIRecord{Kind: "evidence request", Tone: "ai", ID: q.ID, Statement: fmt.Sprintf("asked for %s %s — %s", q.Kind, q.Target, q.Reason),
			Detail: d, Label: aiLabel(q.Provenance)})
	}
	return out
}

func answered(inv *domain.Investigation, assertion string) bool {
	for _, v := range inv.Validations {
		if v.Outcome != domain.ValidationPass {
			continue
		}
		for _, a := range v.Answers {
			if a == assertion {
				return true
			}
		}
	}
	return false
}
