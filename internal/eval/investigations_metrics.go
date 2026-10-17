package eval

// DESIGN.md §6.2 metrics over item decisions and runs, the splits (per basis,
// manifestation, expected verification) and the evaluation of the frozen
// gates (§6.3): a gate whose denominator is below minSample is
// INSUFFICIENT-DATA — printed as such and never a pass.

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Metric names (gates.yaml keys and the reported, ungated ones).
const (
	MFalseNegativeRate       = "falseNegativeRate"
	MUnsafeEligibilityRate   = "unsafeEligibilityRate"
	MFalsePositiveRate       = "falsePositiveRate"
	MAffectedDetection       = "affectedDetection"
	MAffectedConfirmation    = "affectedConfirmation"
	MNotAffectedConfirmation = "notAffectedConfirmation"
	MExposureRecall          = "exposureRecall"
	MEvidenceCoverage        = "evidenceCoverage"
	MCitationResolution      = "citationResolution"
	MUnresolvedRate          = "unresolvedRate"
	MObservedAgreement       = "observedAgreement"
	MReproducibility         = "reproducibility"
	MPipelineFailures        = "pipelineFailures"
	// Reported, not gated.
	MHumanReviewRate             = "humanReviewRate"
	MAutoApprovalEligibilityRate = "autoApprovalEligibilityRate"
	MRunHumanReviewRate          = "runHumanReviewRate"
	MRunAutoApproveEligibleRate  = "runAutoApproveEligibleRate"
	MItemMatchMisses             = "itemMatchMisses"
	MEvidenceFileCoverage        = "evidenceFileCoverage"
	MAccuracy                    = "accuracy"
)

// decisionMetrics are computed per item-decision subset (the splits).
var decisionMetrics = []string{
	MFalseNegativeRate, MUnsafeEligibilityRate, MFalsePositiveRate, MAffectedDetection, MAffectedConfirmation,
	MNotAffectedConfirmation, MExposureRecall, MEvidenceCoverage, MUnresolvedRate, MObservedAgreement,
	MAccuracy, MHumanReviewRate, MAutoApprovalEligibilityRate, MItemMatchMisses, MEvidenceFileCoverage,
}

// runMetrics are per run (no split).
var runMetrics = []string{MCitationResolution, MReproducibility, MPipelineFailures, MRunHumanReviewRate, MRunAutoApproveEligibleRate}

func knownInvMetric(name string) bool {
	for _, m := range append(append([]string{}, decisionMetrics...), runMetrics...) {
		if m == name {
			return true
		}
	}
	return false
}

// MetricValue is one metric: Num/Den, or a Count (pipelineFailures,
// itemMatchMisses) over Den units.
type MetricValue struct {
	Name  string  `json:"name"`
	Num   int     `json:"num"`
	Den   int     `json:"den"`
	Value float64 `json:"value"`
	Count bool    `json:"count,omitempty"` // Value is Num (an absolute count), Den is the units it was counted over
}

func (m MetricValue) String() string {
	if m.Count {
		return fmt.Sprintf("%d (of %d)", m.Num, m.Den)
	}
	if m.Den == 0 {
		return "n/a (0/0)"
	}
	return fmt.Sprintf("%.3f (%d/%d)", m.Value, m.Num, m.Den)
}

func invRatio(name string, num, den int) MetricValue {
	m := MetricValue{Name: name, Num: num, Den: den}
	if den > 0 {
		m.Value = float64(num) / float64(den)
	}
	return m
}

func isAffectedish(c domain.FindingClass) bool {
	return c == domain.FindingAffected || c == domain.FindingPotentiallyAffected
}

// DecisionMetrics computes the item-decision metrics over ds.
func DecisionMetrics(ds []ItemDecision) map[string]MetricValue {
	var fnN, fnD, ueN, ueD, fpN, fpD, adN, acN, aD, naN, naD, exN, exD, evN, evFN, evD, urN, urD, obN, obD, accN, hrN, elN, miss int
	for _, d := range ds {
		if isAffectedish(d.Expected) {
			fnD++
			if d.System == domain.FindingNotAffected {
				fnN++
			}
		}
		if d.Eligible {
			ueD++
			if d.Expected != domain.FindingNotAffected {
				ueN++
			}
			elN++
		}
		if d.Expected == domain.FindingNotAffected {
			fpD++
			naD++
			if d.System == domain.FindingAffected {
				fpN++
			}
			if d.System == domain.FindingNotAffected {
				naN++
			}
		}
		if d.Expected == domain.FindingAffected {
			aD++
			if isAffectedish(d.System) {
				adN++
			}
			if d.System == domain.FindingAffected {
				acN++
			}
		}
		for _, c := range d.Components {
			if c.Classification == domain.FindingAffected {
				exD++
				if c.Reached {
					exN++
				}
			}
		}
		for _, e := range d.Evidence {
			evD++
			if e.Covered {
				evN++
			}
			if e.FileCited {
				evFN++
			}
		}
		if d.Expected == domain.FindingAffected || d.Expected == domain.FindingNotAffected {
			urD++
			if d.System == domain.FindingInsufficientEvidence {
				urN++
			}
		}
		if d.Verification == VerifyObserved {
			obD++
			if d.System == d.Expected {
				obN++
			}
		}
		if d.Correct {
			accN++
		}
		if d.ReviewRequired {
			hrN++
		}
		if d.MatchMiss {
			miss++
		}
	}
	n := len(ds)
	return map[string]MetricValue{
		MFalseNegativeRate:           invRatio(MFalseNegativeRate, fnN, fnD),
		MUnsafeEligibilityRate:       invRatio(MUnsafeEligibilityRate, ueN, ueD),
		MFalsePositiveRate:           invRatio(MFalsePositiveRate, fpN, fpD),
		MAffectedDetection:           invRatio(MAffectedDetection, adN, aD),
		MAffectedConfirmation:        invRatio(MAffectedConfirmation, acN, aD),
		MNotAffectedConfirmation:     invRatio(MNotAffectedConfirmation, naN, naD),
		MExposureRecall:              invRatio(MExposureRecall, exN, exD),
		MEvidenceCoverage:            invRatio(MEvidenceCoverage, evN, evD),
		MEvidenceFileCoverage:        invRatio(MEvidenceFileCoverage, evFN, evD),
		MUnresolvedRate:              invRatio(MUnresolvedRate, urN, urD),
		MObservedAgreement:           invRatio(MObservedAgreement, obN, obD),
		MAccuracy:                    invRatio(MAccuracy, accN, n),
		MHumanReviewRate:             invRatio(MHumanReviewRate, hrN, n),
		MAutoApprovalEligibilityRate: invRatio(MAutoApprovalEligibilityRate, elN, n),
		MItemMatchMisses:             {Name: MItemMatchMisses, Num: miss, Den: n, Value: float64(miss), Count: true},
	}
}

// RunMetrics computes the per-run metrics.
func RunMetrics(rs []RunScore) map[string]MetricValue {
	var cN, cD, rN, rD, fail, hr, el, ok int
	for _, r := range rs {
		if r.Error != "" {
			fail++
			continue
		}
		ok++
		cN += r.CitationsResolved
		cD += r.Citations
		if r.Reproduced != nil {
			rD++
			if *r.Reproduced {
				rN++
			}
		}
		switch domain.PolicyOutcome(r.Decision) {
		case domain.PolicyHumanReview:
			hr++
		case domain.PolicyAutoApproveEligible:
			el++
		}
	}
	return map[string]MetricValue{
		MCitationResolution:         invRatio(MCitationResolution, cN, cD),
		MReproducibility:            invRatio(MReproducibility, rN, rD),
		MPipelineFailures:           {Name: MPipelineFailures, Num: fail, Den: len(rs), Value: float64(fail), Count: true},
		MRunHumanReviewRate:         invRatio(MRunHumanReviewRate, hr, ok),
		MRunAutoApproveEligibleRate: invRatio(MRunAutoApproveEligibleRate, el, ok),
	}
}

// Gate states.
const (
	GatePass             = "PASS"
	GateFail             = "FAIL"
	GateInsufficientData = "INSUFFICIENT-DATA"
)

// InvGateResult is one evaluated gate.
type InvGateResult struct {
	Name      string      `json:"name"`
	Op        string      `json:"op"` // "min" | "max"
	Threshold float64     `json:"threshold"`
	MinSample int         `json:"minSample"`
	Metric    MetricValue `json:"metric"`
	State     string      `json:"state"`
}

// EvaluateInvGates judges the overall metrics against the frozen gates, in
// gates-file name order.
func EvaluateInvGates(g *InvGates, m map[string]MetricValue) []InvGateResult {
	names := make([]string, 0, len(g.Gates))
	for n := range g.Gates {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []InvGateResult
	for _, n := range names {
		gate := g.Gates[n]
		mv := m[n]
		r := InvGateResult{Name: n, MinSample: gate.MinSample, Metric: mv}
		v := mv.Value
		switch {
		case gate.Min != nil:
			r.Op, r.Threshold = "min", *gate.Min
		default:
			r.Op, r.Threshold = "max", *gate.Max
		}
		switch {
		case mv.Den < gate.MinSample || (mv.Den == 0 && !mv.Count):
			r.State = GateInsufficientData
		case r.Op == "min" && v >= r.Threshold, r.Op == "max" && v <= r.Threshold:
			r.State = GatePass
		default:
			r.State = GateFail
		}
		out = append(out, r)
	}
	return out
}

// AutonomyResult is the §6.3 autonomy check (never automatic: it only says
// whether enabling automated approval may even be considered).
type AutonomyResult struct {
	Eligible            bool     `json:"eligible"`
	ObservedNotAffected int      `json:"observedNotAffected"`
	ObservedAffected    int      `json:"observedAffected"`
	Reasons             []string `json:"reasons"`
}

// EvaluateAutonomy applies gates.yaml `autonomy`: every required gate PASS
// (INSUFFICIENT-DATA is a FAIL here) and enough observed decisions.
func EvaluateAutonomy(g *InvGates, gates []InvGateResult, ds []ItemDecision) AutonomyResult {
	var a AutonomyResult
	for _, d := range ds {
		if d.Verification != VerifyObserved {
			continue
		}
		if d.Expected == domain.FindingNotAffected {
			a.ObservedNotAffected++
		}
		if isAffectedish(d.Expected) {
			a.ObservedAffected++
		}
	}
	state := map[string]string{}
	for _, r := range gates {
		state[r.Name] = r.State
	}
	for _, req := range g.Autonomy.Requires {
		if state[req] != GatePass {
			a.Reasons = append(a.Reasons, fmt.Sprintf("%s is %s", req, state[req]))
		}
	}
	if a.ObservedNotAffected < g.Autonomy.MinObservedNotAffected {
		a.Reasons = append(a.Reasons, fmt.Sprintf("%d observed not-affected decisions < %d", a.ObservedNotAffected, g.Autonomy.MinObservedNotAffected))
	}
	if a.ObservedAffected < g.Autonomy.MinObservedAffected {
		a.Reasons = append(a.Reasons, fmt.Sprintf("%d observed affected/potentially-affected decisions < %d", a.ObservedAffected, g.Autonomy.MinObservedAffected))
	}
	a.Eligible = len(a.Reasons) == 0
	return a
}

// Split is the decision metrics of one subset.
type Split struct {
	By      string                 `json:"by"`    // basis | manifestation | verification
	Value   string                 `json:"value"` // e.g. deterministic
	N       int                    `json:"n"`
	Metrics map[string]MetricValue `json:"metrics"`
}

// Splits computes the decision metrics per finding basis, manifestation and
// expected verification (DESIGN.md §6.2).
func Splits(ds []ItemDecision) []Split {
	keys := []struct {
		by string
		f  func(ItemDecision) string
	}{
		{"basis", func(d ItemDecision) string { return d.Basis }},
		{"manifestation", func(d ItemDecision) string { return d.Manifestation }},
		{"verification", func(d ItemDecision) string { return d.Verification }},
	}
	var out []Split
	for _, k := range keys {
		groups := map[string][]ItemDecision{}
		for _, d := range ds {
			groups[k.f(d)] = append(groups[k.f(d)], d)
		}
		vals := make([]string, 0, len(groups))
		for v := range groups {
			vals = append(vals, v)
		}
		sort.Strings(vals)
		for _, v := range vals {
			out = append(out, Split{By: k.by, Value: v, N: len(groups[v]), Metrics: DecisionMetrics(groups[v])})
		}
	}
	return out
}

// InvScore is a whole scored run.
type InvScore struct {
	Decisions []ItemDecision         `json:"decisions"`
	Runs      []RunScore             `json:"runs"`
	Metrics   map[string]MetricValue `json:"metrics"`
	Splits    []Split                `json:"splits"`
	Gates     []InvGateResult        `json:"gates"`
	Autonomy  AutonomyResult         `json:"autonomy"`
	// Verdicts is the agent-verdict table (ungated; absent when the arm
	// stored no verdicts).
	Verdicts []VerdictDecision `json:"agentVerdicts,omitempty"`
}

// ScoreInvestigations scores every case (runs keyed by case id; a missing
// run is a pipeline failure) and evaluates the gates.
func ScoreInvestigations(cases []*InvCase, runs map[string]*InvRun, g *InvGates) *InvScore {
	s := &InvScore{}
	for _, c := range cases {
		ds, rs := ScoreInvCase(c, runs[c.ID])
		s.Decisions = append(s.Decisions, ds...)
		s.Runs = append(s.Runs, rs)
	}
	s.Metrics = DecisionMetrics(s.Decisions)
	for k, v := range RunMetrics(s.Runs) {
		s.Metrics[k] = v
	}
	s.Splits = Splits(s.Decisions)
	s.Verdicts = ScoreVerdicts(cases, runs)
	if g != nil {
		s.Gates = EvaluateInvGates(g, s.Metrics)
		s.Autonomy = EvaluateAutonomy(g, s.Gates, s.Decisions)
	}
	return s
}

// GateFailures counts gates that are not PASS (FAIL and INSUFFICIENT-DATA).
func (s *InvScore) GateFailures() (fail, insufficient int) {
	for _, g := range s.Gates {
		switch g.State {
		case GateFail:
			fail++
		case GateInsufficientData:
			insufficient++
		}
	}
	return fail, insufficient
}

// WriteInvText prints the score: per-case table, metrics, splits, gates.
func WriteInvText(w io.Writer, title string, s *InvScore) {
	fmt.Fprintf(w, "══ %s ══\n", title)
	fmt.Fprintf(w, "n = %d item decisions over %d cases (small dataset: every rate below has a wide uncertainty)\n\n", len(s.Decisions), len(s.Runs))
	fmt.Fprintf(w, "── runs ──\n")
	for _, r := range s.Runs {
		if r.Error != "" {
			fmt.Fprintf(w, "✗ %-44s PIPELINE FAILURE: %s\n", r.Case, r.Error)
			continue
		}
		repro := "not checked"
		if r.Reproduced != nil {
			repro = map[bool]string{true: "reproduced", false: "NOT reproduced: " + r.ReproduceDetail}[*r.Reproduced]
		}
		var cc []string
		for _, k := range domain.FindingClasses {
			cc = append(cc, fmt.Sprintf("%s=%d", k, r.ClassCounts[k]))
		}
		fmt.Fprintf(w, "  %-44s %s  findings %d (%s)  agent %s  citations %d/%d  %s\n", r.Case, r.Decision, r.Findings,
			strings.Join(cc, " "), r.AgentStatus, r.CitationsResolved, r.Citations, repro)
		if r.UnmatchedFindings > 0 {
			fmt.Fprintf(w, "      %d finding(s) on items no expected item selects (unscored), %d affected", r.UnmatchedFindings, len(r.UnmatchedAffected))
			if len(r.UnmatchedAffected) > 0 {
				fmt.Fprintf(w, ": %s", strings.Join(r.UnmatchedAffected, "; "))
			}
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintf(w, "\n── item decisions (expected vs system) ──\n")
	for _, d := range s.Decisions {
		mark := "✓"
		switch {
		case d.Correct:
		case d.AcceptableAlt:
			mark = "≈"
		case isAffectedish(d.Expected) && d.System == domain.FindingNotAffected:
			mark = "✗✗" // the unsafe error
		default:
			mark = "✗"
		}
		dec := "no matching item"
		if d.RunFailed {
			dec = "run failed"
		} else if d.Decider != nil {
			dec = fmt.Sprintf("%s [%s, basis %s, %s] %q", d.Decider.Rule, d.Manifestation, d.Basis, d.Decider.ChangeItem, clipText(d.Decider.Title, 60))
		}
		fmt.Fprintf(w, "%-2s %-44s %-3s expected %-21s system %-21s (%s, %d matched) %s\n", mark, d.Case, d.Item, d.Expected, d.System,
			d.Verification, len(d.Matched), dec)
	}
	fmt.Fprintf(w, "\n── metrics (overall) ──\n")
	for _, n := range append(append([]string{}, decisionMetrics...), runMetrics...) {
		fmt.Fprintf(w, "  %-30s %s\n", n, s.Metrics[n])
	}
	fmt.Fprintf(w, "\n── splits (decision metrics per subset) ──\n")
	show := []string{MAccuracy, MFalseNegativeRate, MAffectedDetection, MAffectedConfirmation, MNotAffectedConfirmation, MUnresolvedRate, MExposureRecall, MEvidenceCoverage}
	for _, sp := range s.Splits {
		var parts []string
		for _, n := range show {
			if m := sp.Metrics[n]; m.Den > 0 {
				parts = append(parts, fmt.Sprintf("%s %s", n, m))
			}
		}
		fmt.Fprintf(w, "  %-13s %-22s n=%-3d %s\n", sp.By, sp.Value, sp.N, strings.Join(parts, "; "))
	}
	WriteVerdictText(w, s.Verdicts)
	if len(s.Gates) > 0 {
		fmt.Fprintf(w, "\n── gates (pre-registered, eval/investigations/gates.yaml; INSUFFICIENT-DATA is never a pass) ──\n")
		for _, g := range s.Gates {
			mark := map[string]string{GatePass: "✓", GateFail: "✗", GateInsufficientData: "?"}[g.State]
			op := "≥"
			if g.Op == "max" {
				op = "≤"
			}
			fmt.Fprintf(w, "%s %-26s %-17s actual %-18s %s %.2f  (minSample %d)\n", mark, g.Name, g.State, g.Metric, op, g.Threshold, g.MinSample)
		}
		fmt.Fprintf(w, "\nautonomy (§6.3): eligible=%v — observed not-affected %d, observed affected %d\n", s.Autonomy.Eligible,
			s.Autonomy.ObservedNotAffected, s.Autonomy.ObservedAffected)
		for _, r := range s.Autonomy.Reasons {
			fmt.Fprintf(w, "    %s\n", r)
		}
	}
}

func clipText(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
