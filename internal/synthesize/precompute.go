// Package synthesize is the upgrade-level synthesis of Milestone 3
// (docs/phase6/decision-intel/PLAN-M3.md, mission C1–C5, PD-1, PD-3): a
// deterministic pre-computation over one classified investigation, one
// structured model call that commits a recommendation, and a deterministic
// floor that checks the answer. The result is a domain.UpgradeJudgement.
//
// Plain Go: no Temporal, no policy. The judgement is advisory; nothing here
// reads or writes findings, the decision, the review or knowledge.
package synthesize

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/verify/semantic"
)

// Producer is the provenance producer of a judgement; PromptVersion the
// default prompt.
const (
	Producer      = "synthesize@v1"
	PromptVersion = "synthesize/v1"
)

// Releases are the facts the latest-patch question (PD-3) needs beyond the
// edge: the product's known releases and every advisory as the edge read
// them, with the advisories' evidence records, and the chart versions that
// ship a release (looked up for the latest patch only). Nil: the latest
// patch is not computed and the minimum is recommended.
type Releases struct {
	Versions   []domain.Version
	Advisories []domain.Advisory
	Evidence   []domain.Evidence
	// Charts maps a release's semver to the chart versions shipping it.
	Charts map[string][]domain.CandidateChart
	// ChartEvidence holds the chart records Charts cites.
	ChartEvidence []domain.Evidence
}

// Input is everything the pre-computation reads. Investigation and Bundle
// are required; Report, Edge and Releases are optional (each missing one
// leaves a stated gap, never a silent "nothing").
type Input struct {
	Investigation *domain.Investigation
	Bundle        *domain.EvidenceBundle
	Report        *domain.ImpactReport
	Edge          *domain.UpgradeEdge
	Releases      *Releases
}

// TestCandidate is one test the judgement may require, taken from an
// assessment's recommended tests, a finding's next steps, or the
// transition a changed target needs.
type TestCandidate struct {
	ID         string `json:"id"`
	ChangeItem string `json:"changeItem,omitempty"`
	Test       string `json:"test"`
	Source     string `json:"source"` // assessment, finding or target option the test comes from
}

// ItemDigest is the per-item part of the model's input: the classification,
// the agent's verdict and the assessment's headline, effective
// configuration, preconditions and tests.
type ItemDigest struct {
	ChangeItem     string                     `json:"changeItem"`
	Title          string                     `json:"title"`
	Manifestation  string                     `json:"manifestation"`
	Classification domain.FindingClass        `json:"classification"`
	Rule           string                     `json:"rule"`
	Finding        string                     `json:"finding"`
	AgentVerdict   domain.FindingClass        `json:"agentVerdict,omitempty"`
	Headline       string                     `json:"headline,omitempty"`
	WhatChanged    string                     `json:"whatChanged,omitempty"`
	Mechanism      string                     `json:"mechanism,omitempty"`
	LikelyImpact   string                     `json:"likelyImpact,omitempty"`
	Effective      []domain.AssessmentSetting `json:"effectiveConfiguration,omitempty"`
	Preconditions  []string                   `json:"preconditions,omitempty"`
	Unknown        []string                   `json:"unknown,omitempty"`
	Tests          []string                   `json:"tests,omitempty"` // TestCandidate ids
	Assessment     string                     `json:"assessment,omitempty"`
	// Evidence are the finding's and the assessment's verified citations
	// (bounded), citable by the answer.
	Evidence []domain.EvidenceID `json:"evidence,omitempty"`
}

// EvidenceRef is a citable evidence record as the model sees it.
type EvidenceRef struct {
	ID      domain.EvidenceID   `json:"id"`
	Kind    domain.EvidenceKind `json:"kind"`
	URI     string              `json:"uri"`
	Locator string              `json:"locator,omitempty"`
	Excerpt string              `json:"excerpt,omitempty"`
}

// Upgrade names the upgrade and environment.
type Upgrade struct {
	Investigation string            `json:"investigation"`
	Product       domain.ProductRef `json:"product"`
	From          domain.Version    `json:"from"`
	To            domain.Version    `json:"to"`
	Environment   string            `json:"environment"`
	Decision      string            `json:"decision,omitempty"`
}

// Precomputed is the deterministic input of the model call. Everything in it
// is computed by RI; the judgement copies Findings, Target, Locations,
// ResidualRisks, Interactions and Backout verbatim.
type Precomputed struct {
	Upgrade       Upgrade                   `json:"upgrade"`
	Findings      []domain.JudgementFinding `json:"findings"`
	Target        domain.JudgementTarget    `json:"target"`
	Locations     []domain.ChangeLocation   `json:"locations"`
	Items         []ItemDigest              `json:"items"`
	Tests         []TestCandidate           `json:"tests"`
	ResidualRisks []domain.ResidualRisk     `json:"residualRisks"`
	Interactions  []domain.CrossChange      `json:"interactions"`
	Backout       domain.Backout            `json:"backout"`
	Evidence      []EvidenceRef             `json:"evidence"`

	// external are the cited records the bundle and the run pool lack (they
	// become UpgradeJudgement.Evidence); not part of the model's input.
	external []domain.Evidence
}

// Digest is the content digest of the pre-computed input.
func (p *Precomputed) Digest() string {
	b, _ := json.Marshal(p)
	return domain.Digest(b)
}

// Bounds of the model input (text is clipped, lists are capped; the
// judgement's own records are never clipped).
const (
	maxText     = 400
	maxList     = 6
	maxEvidence = 4
)

type pre struct {
	in     Input
	inv    *domain.Investigation
	b      *domain.EvidenceBundle
	sec    *domain.EdgeSecurity
	pool   map[domain.EvidenceID]domain.Evidence // every record known to the run, with source
	local  map[domain.EvidenceID]bool            // bundle + run pool (resolve in ValidateAgainst)
	cited  map[domain.EvidenceID]bool
	out    *Precomputed
	titles map[string]string
	items  map[string]domain.ChangeItem
}

// Precompute computes the deterministic input of the synthesis. Pure.
func Precompute(in Input) (*Precomputed, error) {
	if in.Investigation == nil || in.Bundle == nil {
		return nil, fmt.Errorf("synthesize: an investigation and its bundle are required")
	}
	if len(in.Investigation.Findings) == 0 || in.Investigation.Decision == nil {
		return nil, fmt.Errorf("synthesize: investigation %s is not classified", in.Investigation.ID)
	}
	p := &pre{in: in, inv: in.Investigation, b: in.Bundle, pool: map[domain.EvidenceID]domain.Evidence{},
		local: map[domain.EvidenceID]bool{}, cited: map[domain.EvidenceID]bool{}, titles: map[string]string{}, items: map[string]domain.ChangeItem{}}
	for _, e := range in.Bundle.Evidence {
		p.pool[e.ID], p.local[e.ID] = e, true
	}
	for _, e := range in.Investigation.Evidence {
		p.pool[e.ID], p.local[e.ID] = e, true
	}
	var extra [][]domain.Evidence
	if in.Report != nil {
		extra = append(extra, in.Report.Evidence, in.Report.EnvironmentEvidence)
		p.sec = in.Report.Security
	}
	if in.Edge != nil {
		extra = append(extra, in.Edge.Evidence)
		if p.sec == nil {
			p.sec = in.Edge.Security
		}
	}
	if r := in.Releases; r != nil {
		extra = append(extra, r.Evidence, r.ChartEvidence)
	}
	for _, es := range extra {
		for _, e := range es {
			if _, ok := p.pool[e.ID]; !ok {
				p.pool[e.ID] = e
			}
		}
	}
	for _, it := range in.Bundle.Items() {
		p.items[it.ID], p.titles[it.ID] = it, it.Title
	}
	rec := in.Bundle.Change
	p.out = &Precomputed{Upgrade: Upgrade{Investigation: p.inv.ID, Product: rec.Product, From: rec.From, To: rec.To,
		Environment: p.inv.Environment, Decision: string(p.inv.Decision.Outcome)}}
	p.target()
	p.locations()
	p.itemFindings()
	p.securityFindings()
	p.digests()
	p.risks()
	p.interactions()
	p.backout()
	p.evidence()
	return p.out, nil
}

// cite records evidence ids as cited and returns the ones that resolve.
func (p *pre) cite(ids []domain.EvidenceID, max int) []domain.EvidenceID {
	var out []domain.EvidenceID
	for _, id := range ids {
		if _, ok := p.pool[id]; !ok || containsID(out, id) {
			continue
		}
		if max > 0 && len(out) == max {
			break
		}
		out = append(out, id)
		p.cited[id] = true
	}
	return out
}

func (p *pre) finding(f domain.JudgementFinding) {
	f.Hard = f.Kind.Hard()
	f.ID = domain.JudgementFindingID(f.Kind, f.ChangeItem, f.Subject)
	for _, have := range p.out.Findings {
		if have.ID == f.ID {
			return
		}
	}
	p.out.Findings = append(p.out.Findings, f)
}

func (p *pre) itemLabel(id string) string {
	if t := p.titles[id]; t != "" {
		return fmt.Sprintf("%q (%s)", clip(t, 120), id)
	}
	return id
}

// itemFindings: confirmed breakage, failed validations, agent
// disagreements, untraced consumers, unestablished behaviour and behaviour
// its citations do not support.
func (p *pre) itemFindings() {
	verdicts := p.verdicts()
	for _, f := range p.inv.Findings {
		if f.Class == domain.FindingAffected {
			p.finding(domain.JudgementFinding{Kind: domain.JFConfirmedBreakage, ChangeItem: f.ChangeItem, Subject: f.ID,
				Statement: fmt.Sprintf("Item %s is classified affected (%s): %s", p.itemLabel(f.ChangeItem), f.Rule, clip(f.Reasoning, maxText)),
				Evidence:  p.cite(f.Evidence, maxEvidence), Refs: append([]string{f.ID}, f.Validations...)})
		}
		if v, ok := verdicts[f.ChangeItem]; ok && disagree(v.Class, f.Class) {
			p.finding(domain.JudgementFinding{Kind: domain.JFAgentDisagreement, ChangeItem: f.ChangeItem, Subject: string(v.Class) + "/" + string(f.Class),
				Statement: fmt.Sprintf("For item %s the agent's verdict is %s and the deterministic classification is %s (%s).", p.itemLabel(f.ChangeItem), v.Class, f.Class, f.Rule),
				Evidence:  p.cite(f.Evidence, 2), Refs: []string{v.ID, f.ID}})
		}
	}
	for _, v := range p.inv.Validations {
		if v.Outcome != domain.ValidationFail {
			continue
		}
		p.finding(domain.JudgementFinding{Kind: domain.JFFailedValidation, ChangeItem: v.ChangeItem, Subject: v.ID,
			Statement: fmt.Sprintf("Validation %s (%s, %s) of item %s failed: %s. %s", v.ID, v.Kind, v.Level, p.itemLabel(v.ChangeItem), clip(v.Property, 200), clip(v.Detail, 200)),
			Evidence:  p.cite(v.Evidence, maxEvidence), Refs: []string{v.ID}})
	}
	for _, a := range p.inv.AgentAssessments {
		for _, n := range a.UntracedConsumers {
			p.finding(domain.JudgementFinding{Kind: domain.JFUntracedConsumer, ChangeItem: a.ChangeItem, Subject: n,
				Statement: fmt.Sprintf("Consumer %s of an object item %s exposes was never traced by the investigation.", n, p.itemLabel(a.ChangeItem)),
				Refs:      []string{a.ID}})
		}
		named := map[string]bool{}
		for _, b := range a.Behaviour {
			if b.Request != "" {
				named[b.Request] = true
			}
			if !b.Gap() {
				continue
			}
			how := "not checked"
			switch {
			case b.Source == domain.BehaviourUnavailable:
				how = "unavailable (request " + b.Request + ")"
			case b.Reason != "":
				how = "not checked (reason: " + clip(b.Reason, 200) + ")"
			}
			p.finding(domain.JudgementFinding{Kind: domain.JFBehaviourUnestablished, ChangeItem: a.ChangeItem, Subject: b.Behaviour,
				Statement: fmt.Sprintf("Upstream behaviour %q, which the assessment of item %s relies on, is %s.", clip(b.Behaviour, 300), p.itemLabel(a.ChangeItem), how),
				Refs:      nonEmpty(a.ID, b.Request)})
		}
		// the report-only semantic check (lane semverify): an upstream
		// behaviour whose cited passages lack a value it asserts, or state
		// it reversed. Recorded as a disagreement, never a classification input.
		for _, bc := range semantic.CheckBehaviours([]domain.AgentAssessment{a}, p.pool) {
			if bc.Verdict != semantic.Unsupported {
				continue
			}
			p.finding(domain.JudgementFinding{Kind: domain.JFBehaviourUnsupported, ChangeItem: a.ChangeItem, Subject: bc.Behaviour,
				Statement: fmt.Sprintf("Upstream behaviour %q, which the assessment of item %s relies on, is not supported by its cited passages: %s", clip(bc.Behaviour, 300), p.itemLabel(a.ChangeItem), clip(bc.Detail, 400)),
				Evidence:  p.cite(bc.Citations, maxEvidence), Refs: []string{a.ID}})
		}
		for _, r := range a.UnavailableEvidence {
			if named[r] {
				continue
			}
			p.finding(domain.JudgementFinding{Kind: domain.JFBehaviourUnestablished, ChangeItem: a.ChangeItem, Subject: r,
				Statement: fmt.Sprintf("Upstream evidence request %s of item %s could not be fulfilled%s.", r, p.itemLabel(a.ChangeItem), p.requestReason(r)),
				Refs:      []string{a.ID, r}})
		}
	}
}

func (p *pre) requestReason(id string) string {
	for _, r := range p.inv.EvidenceRequests {
		if r.ID == id && r.Resolution != "" {
			return ": " + clip(r.Resolution, 200)
		}
	}
	return ""
}

// verdicts returns each item's agent verdict: the one its assessment links,
// else the item's last.
func (p *pre) verdicts() map[string]domain.AgentVerdict {
	byID := map[string]domain.AgentVerdict{}
	out := map[string]domain.AgentVerdict{}
	for _, v := range p.inv.AgentVerdicts {
		byID[v.ID] = v
		out[v.ChangeItem] = v
	}
	for _, a := range p.inv.AgentAssessments {
		if v, ok := byID[a.Verdict]; ok {
			out[a.ChangeItem] = v
		}
	}
	return out
}

// disagree: the agent and the classification differ and either says
// affected or not-affected (a difference between the two undecided classes
// is not a disagreement about the outcome).
func disagree(agent, cls domain.FindingClass) bool {
	if agent == cls || agent == "" {
		return false
	}
	decided := func(c domain.FindingClass) bool { return c == domain.FindingAffected || c == domain.FindingNotAffected }
	return decided(agent) || decided(cls)
}

// securityFindings: regressions, inconsistencies, an incomplete picture and
// target options still affected.
func (p *pre) securityFindings() {
	s := p.sec
	if s == nil {
		p.finding(domain.JudgementFinding{Kind: domain.JFSecurityIncomplete, Subject: "security",
			Statement: "The upgrade has no advisory assessment: whether advisories affect the current version, the target or another release is unknown, not absent."})
		return
	}
	for _, a := range s.Regressions {
		p.finding(domain.JudgementFinding{Kind: domain.JFSecurityRegression, ChangeItem: p.itemOfChange(a.ChangeID), Subject: a.ID,
			Statement: fmt.Sprintf("%s affects the target %s and not the current %s (affected range %s; patched %s).", advisoryLabel(a.ID, a.Aliases), p.out.Upgrade.To, p.out.Upgrade.From, orNone(a.Vulnerable), orNone(strings.Join(a.Patched, ", "))),
			Evidence:  p.cite(a.Evidence, maxEvidence)})
	}
	for _, a := range s.Inconsistencies {
		p.finding(domain.JudgementFinding{Kind: domain.JFAdvisoryInconsistency, ChangeItem: p.itemOfChange(a.ChangeID), Subject: a.ID,
			Statement: a.Statement, Evidence: p.cite(a.Evidence, maxEvidence)})
	}
	if s.Status != domain.SecurityComplete {
		p.finding(domain.JudgementFinding{Kind: domain.JFSecurityIncomplete, Subject: "security",
			Statement: fmt.Sprintf("The advisory assessment is %s: %s", s.Status, clip(strings.Join(s.Gaps, "; "), 600))})
	}
	t := p.out.Target
	if len(s.Regressions) > 0 && t.Minimum == nil {
		p.finding(domain.JudgementFinding{Kind: domain.JFCandidateAffected, Subject: "no-safe-release",
			Statement: fmt.Sprintf("No known release at or above %s removes every regression of the target.", p.out.Upgrade.To)})
	}
	for _, o := range t.Options {
		if !o.Safe || len(o.Leaves) == 0 {
			continue
		}
		p.finding(domain.JudgementFinding{Kind: domain.JFCandidateAffected, Subject: o.Version.Semver,
			Statement: fmt.Sprintf("%s (%s option) is still affected by %s.", o.Version, o.Role, strings.Join(o.Leaves, ", ")),
			Evidence:  o.Evidence})
	}
}

func (p *pre) itemOfChange(changeID string) string {
	if changeID == "" {
		return ""
	}
	for _, it := range p.b.Items() {
		if it.ChangeID == changeID {
			return it.ID
		}
	}
	return ""
}

// digests: the per-item input and the test candidates.
func (p *pre) digests() {
	assessments := map[string]domain.AgentAssessment{}
	for _, a := range p.inv.AgentAssessments {
		assessments[a.ChangeItem] = a
	}
	verdicts := p.verdicts()
	for _, f := range p.inv.Findings {
		it := p.items[f.ChangeItem]
		d := ItemDigest{ChangeItem: f.ChangeItem, Title: clip(it.Title, 200), Manifestation: string(f.Manifestation),
			Classification: f.Class, Rule: f.Rule, Finding: f.ID, Evidence: p.cite(f.Evidence, maxEvidence)}
		if v, ok := verdicts[f.ChangeItem]; ok {
			d.AgentVerdict = v.Class
		}
		if a, ok := assessments[f.ChangeItem]; ok {
			d.Assessment, d.Headline, d.WhatChanged = a.ID, clip(a.Headline, maxText), clip(a.WhatChanged, maxText)
			d.Mechanism, d.LikelyImpact = clip(a.Mechanism, maxText), clip(a.LikelyImpact, maxText)
			for i, s := range a.EffectiveConfiguration {
				if i == maxList {
					break
				}
				d.Effective = append(d.Effective, domain.AssessmentSetting{Setting: clip(s.Setting, 200), State: s.State, Detail: clip(s.Detail, 200)})
			}
			for _, v := range a.Verified {
				d.Evidence = append(d.Evidence, p.cite(v.Citations, 2)...)
			}
			d.Preconditions = clipList(a.Preconditions, maxList, 300)
			d.Unknown = clipList(a.Unknown, maxList, 300)
			for i, t := range a.RecommendedTests {
				if i == maxList {
					break
				}
				d.Tests = append(d.Tests, p.test(f.ChangeItem, t, a.ID))
			}
		}
		for i, s := range f.NextSteps {
			if i == maxList {
				break
			}
			d.Tests = append(d.Tests, p.test(f.ChangeItem, s, f.ID))
		}
		p.out.Items = append(p.out.Items, d)
	}
	for _, o := range p.out.Target.Options {
		if o.Role != domain.TargetProposed && o.Safe {
			p.test("", transitionTest(p.out.Upgrade.From, o.Version), "target:"+o.Version.Semver)
		}
	}
}

// transitionTest is the investigation a changed target requires (mission C2).
func transitionTest(from, to domain.Version) string {
	return fmt.Sprintf("Investigate the %s → %s upgrade against this environment (this investigation covered only the proposed target)", from, to)
}

func (p *pre) test(item, text, source string) string {
	text = clip(text, maxText)
	id := "tc-" + domain.ShortHash(item, text)
	for _, t := range p.out.Tests {
		if t.ID == id {
			return id
		}
	}
	p.out.Tests = append(p.out.Tests, TestCandidate{ID: id, ChangeItem: item, Test: text, Source: source})
	return id
}

// risks: every item the classification left unresolved is a residual risk
// an approval accepts.
func (p *pre) risks() {
	for _, f := range p.inv.Findings {
		if f.Class != domain.FindingPotentiallyAffected && f.Class != domain.FindingInsufficientEvidence {
			continue
		}
		s := fmt.Sprintf("Item %s is classified %s (%s): not established either way by validation.", p.itemLabel(f.ChangeItem), f.Class, f.Rule)
		p.out.ResidualRisks = append(p.out.ResidualRisks, domain.ResidualRisk{ID: domain.ResidualRiskID(domain.RiskDeterministic, f.ChangeItem, s),
			Source: domain.RiskDeterministic, ChangeItem: f.ChangeItem, Statement: s, Refs: []string{f.ID}})
	}
}

// interactions: items that touch the same rendered object or the same
// setting.
func (p *pre) interactions() {
	rcs := p.b.RenderedChanges()
	touch := map[[2]string]map[string]bool{}
	add := func(kind, key, item string) {
		if key == "" || item == "" {
			return
		}
		k := [2]string{kind, key}
		if touch[k] == nil {
			touch[k] = map[string]bool{}
		}
		touch[k][item] = true
	}
	for _, f := range p.inv.Findings {
		for _, id := range f.RenderedChanges {
			if rc, ok := rcs[id]; ok {
				add("object", rc.Object, f.ChangeItem)
			}
		}
	}
	for _, l := range p.out.Locations {
		if l.Kind != domain.LocationDeployedVersion {
			add("setting", l.Key, l.ChangeItem)
		}
	}
	var keys [][2]string
	for k, items := range touch {
		if len(items) > 1 {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i][0]+keys[i][1] < keys[j][0]+keys[j][1] })
	for _, k := range keys {
		items := sortedKeys(touch[k])
		var labels []string
		for _, it := range items {
			labels = append(labels, p.itemLabel(it))
		}
		p.out.Interactions = append(p.out.Interactions, domain.CrossChange{ID: domain.CrossChangeID(k[0], k[1]), Kind: k[0], Shared: k[1], Items: items,
			Statement: fmt.Sprintf("%d items touch the same %s %s: %s.", len(items), k[0], k[1], strings.Join(labels, ", "))})
	}
}

// backout: the version pin to restore, where.
func (p *pre) backout() {
	from := p.out.Upgrade.From
	b := domain.Backout{Version: from}
	for _, l := range p.out.Locations {
		if l.Kind == domain.LocationDeployedVersion {
			b.Location = l.ID
			b.Statement = fmt.Sprintf("Restore %s in %s%s to %s (the value before the upgrade), then revert any configuration step of this change in reverse order.", l.Key, l.File, lineSuffix(l.Line), l.Value)
			break
		}
	}
	if b.Location == "" {
		b.Statement = fmt.Sprintf("Restore the deployed version to %s, then revert any configuration step of this change in reverse order (no deterministic location of the version setting was found).", from)
	}
	p.out.Backout = b
}

// evidence lists every cited record for the model, and keeps the ones the
// run pool and bundle lack for the judgement.
func (p *pre) evidence() {
	var ids []string
	for id := range p.cited {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, s := range ids {
		e := p.pool[domain.EvidenceID(s)]
		p.out.Evidence = append(p.out.Evidence, EvidenceRef{ID: e.ID, Kind: e.Kind, URI: e.URI, Locator: e.Locator, Excerpt: clip(e.Excerpt, 300)})
		if !p.local[e.ID] {
			p.out.external = append(p.out.external, e)
		}
	}
}

// --- locations ---------------------------------------------------------------------------

var lineRE = regexp.MustCompile(`\(L(\d+)\)\s*$`)

// fileLine splits an evidence record into the file and line it states: the
// URI without a #L fragment, and the line from an "(L<n>)" or "L<n>"
// locator. The key part is the locator before "(L<n>)".
func fileLine(e domain.Evidence) (file string, line int, key string) {
	file = e.URI
	if i := strings.Index(file, "#L"); i >= 0 {
		file = file[:i]
	}
	key = strings.TrimSpace(e.Locator)
	if m := lineRE.FindStringSubmatchIndex(key); m != nil {
		line, _ = strconv.Atoi(key[m[2]:m[3]])
		key = strings.TrimSpace(key[:m[0]])
	} else if n, err := strconv.Atoi(strings.TrimPrefix(key, "L")); err == nil && strings.HasPrefix(key, "L") {
		line, key = n, ""
	}
	return file, line, key
}

func (p *pre) location(l domain.ChangeLocation) {
	l.ID = domain.ChangeLocationID(l.Kind, l.File, l.Key, l.ChangeItem)
	for _, have := range p.out.Locations {
		if have.ID == l.ID {
			return
		}
	}
	p.out.Locations = append(p.out.Locations, l)
}

// locations: pinned settings, verified setting claims and the line that
// states the deployed version. Nothing else is a location.
func (p *pre) locations() {
	for _, v := range p.inv.Validations {
		pin := v.Pin
		if pin == nil || v.Outcome != domain.ValidationPass {
			continue
		}
		file, line := pin.Subject, 0
		for _, id := range pin.Evidence {
			if e, ok := p.pool[id]; ok {
				file, line, _ = fileLine(e)
				break
			}
		}
		p.location(domain.ChangeLocation{Kind: domain.LocationPinnedSetting, File: file, Line: line, Key: pin.Path, Value: pin.Value,
			ChangeItem: v.ChangeItem, Source: v.ID, Evidence: p.cite(pin.Evidence, maxEvidence)})
	}
	arts := map[string]string{}
	for _, a := range p.b.Manifest.Artifacts {
		arts[a.ID] = a.Path
	}
	for _, c := range p.inv.AgentClaims {
		l := domain.ChangeLocation{Key: c.Setting, Value: c.Value, ChangeItem: c.ChangeItem, Source: c.ID}
		switch c.Kind {
		case domain.ClaimAbsent:
			l.Kind, l.File = domain.LocationSettingUnset, c.Input
			if path, ok := arts[c.Input]; ok {
				l.File = path
			}
		default:
			l.Kind = domain.LocationSettingSet
			ev := p.cite(c.Evidence, 1)
			if len(ev) == 0 {
				continue
			}
			l.File, l.Line, _ = fileLine(p.pool[ev[0]])
			l.Evidence = ev
		}
		p.location(l)
	}
	if r := p.in.Report; r != nil {
		product := string(p.out.Upgrade.Product.ID)
		for _, ip := range r.Environment.Products {
			if !ip.Catalog || ip.Product != product || (ip.Source != "detected-argo" && ip.Source != "detected-flux") {
				continue
			}
			raw := ip.RawVersion
			if raw == "" {
				raw = ip.Version
			}
			if raw == "" {
				continue
			}
			for _, id := range ip.Evidence {
				e, ok := p.pool[id]
				if !ok || !strings.HasSuffix(strings.TrimSpace(e.Excerpt), ": "+raw) {
					continue
				}
				file, line, key := fileLine(e)
				if key == "" {
					key = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(e.Excerpt), ": "+raw))
				}
				p.location(domain.ChangeLocation{Kind: domain.LocationDeployedVersion, File: file, Line: line, Key: key, Value: raw,
					Source: "impact-report:environment.products", Evidence: p.cite([]domain.EvidenceID{id}, 1)})
			}
		}
	}
}

// --- helpers ------------------------------------------------------------------------------

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func clipList(xs []string, max, n int) []string {
	var out []string
	for i, x := range xs {
		if i == max {
			out = append(out, fmt.Sprintf("(%d more)", len(xs)-max))
			break
		}
		out = append(out, clip(x, n))
	}
	return out
}

func containsID(xs []domain.EvidenceID, id domain.EvidenceID) bool {
	for _, x := range xs {
		if x == id {
			return true
		}
	}
	return false
}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "none stated"
	}
	return s
}

func lineSuffix(n int) string {
	if n > 0 {
		return fmt.Sprintf(" line %d", n)
	}
	return ""
}

func advisoryLabel(id string, aliases []string) string {
	var cves []string
	for _, a := range aliases {
		if strings.HasPrefix(strings.ToUpper(a), "CVE-") {
			cves = append(cves, a)
		}
	}
	if len(cves) > 0 {
		return id + " (" + strings.Join(cves, ", ") + ")"
	}
	return "Advisory " + id
}
