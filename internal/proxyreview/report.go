package proxyreview

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// Report is the proxy run report (brief step 6). Everything is computed from
// the store's proxy decisions; the ledger adds failures, cost and time.
type Report struct {
	Decisions  int                                                   `json:"decisions"`
	ByAction   map[domain.DecisionAction]int                         `json:"byAction"`
	ByLabel    map[domain.FeedbackLabel]int                          `json:"byLabel"`
	ByQuestion map[domain.QuestionType]map[domain.DecisionAction]int `json:"byQuestion"`
	// GateVerdicts counts the evidence-sufficiency verdicts as answered.
	GateVerdicts map[string]int `json:"gateVerdicts,omitempty"`
	// ByProposer splits decisions by who authored the proposed assertion the
	// proxy judged (the proposals asserting its verified aspects).
	ByProposer map[string]map[domain.DecisionAction]int `json:"byProposer"`
	// Models is the per-proposing-model outcome (knowledge.ComputeMetrics over
	// the proxy decisions only), with the proxy's relation to each model.
	Models []ModelRow `json:"models"`
	// Facts minted at proxy level.
	ProxyFacts        int                              `json:"proxyFacts"`
	ProxyFactsByClass map[domain.ImpactClass]int       `json:"proxyFactsByClass"`
	ProxyFactsByFam   map[domain.SubjectFamily]int     `json:"proxyFactsByFamily"`
	FactsByLevel      map[domain.VerificationLevel]int `json:"factsByLevel"`
	// Ledger: attempts, failures, cost and time.
	Attempts     int                   `json:"attempts"`
	Failures     map[string]int        `json:"failures"`
	Skipped      int                   `json:"skipped"`
	FailureList  []LedgerEntry         `json:"failureList,omitempty"`
	CostUSD      float64               `json:"costUSD"`
	MedianCallMs int64                 `json:"medianCallMs"`
	P90CallMs    int64                 `json:"p90CallMs"`
	WallClock    time.Duration         `json:"wallClock"`
	ProxyModels  []string              `json:"proxyModels"`
	Metrics      knowledge.LoopMetrics `json:"metrics"`
}

// ModelRow is one proposing model's outcome under the proxy.
type ModelRow struct {
	Model    string `json:"model"`
	Family   string `json:"family"`
	Relation string `json:"relation"` // self-model | self-family | other-family
	knowledge.ModelMetric
}

// BuildReport computes the report.
func BuildReport(snap *knowledge.Snapshot, ledger []LedgerEntry) *Report {
	r := &Report{ByAction: map[domain.DecisionAction]int{}, ByLabel: map[domain.FeedbackLabel]int{},
		ByQuestion: map[domain.QuestionType]map[domain.DecisionAction]int{}, GateVerdicts: map[string]int{},
		ByProposer: map[string]map[domain.DecisionAction]int{}, ProxyFactsByClass: map[domain.ImpactClass]int{},
		ProxyFactsByFam: map[domain.SubjectFamily]int{}, FactsByLevel: map[domain.VerificationLevel]int{}, Failures: map[string]int{}}
	items := map[string]domain.ReviewItem{}
	for _, it := range snap.ReviewItems {
		items[it.ID] = it
	}
	props := map[string]domain.SemanticProposal{}
	for _, p := range snap.Proposals {
		props[p.ID] = p
	}
	proxyModels := map[string]bool{}
	var proxyDecs []domain.ReviewDecision
	for _, d := range snap.Decisions {
		if d.ReviewerKind != domain.ReviewerProxy {
			continue
		}
		proxyDecs = append(proxyDecs, d)
		if d.ProxyProvenance != nil {
			proxyModels[d.ProxyProvenance.Model] = true
		}
		r.Decisions++
		r.ByAction[d.Action]++
		for _, l := range d.Labels {
			r.ByLabel[l]++
		}
		it := items[d.ReviewItemID]
		if r.ByQuestion[it.QuestionType] == nil {
			r.ByQuestion[it.QuestionType] = map[domain.DecisionAction]int{}
		}
		r.ByQuestion[it.QuestionType][d.Action]++
		if it.QuestionType != domain.QuestionEvidenceSufficiency {
			k := proposerKey(it, props, d.ProxyProvenance)
			if r.ByProposer[k] == nil {
				r.ByProposer[k] = map[domain.DecisionAction]int{}
			}
			r.ByProposer[k][d.Action]++
		}
	}
	for m := range proxyModels {
		r.ProxyModels = append(r.ProxyModels, m)
	}
	sort.Strings(r.ProxyModels)

	// per-model outcomes under the proxy only
	only := *snap
	only.Decisions = proxyDecs
	r.Metrics = knowledge.ComputeMetrics(&only)
	for _, m := range r.Metrics.Models {
		row := ModelRow{Model: m.Model, Family: domain.ModelFamily(m.Model), ModelMetric: m, Relation: "other-family"}
		for pm := range proxyModels {
			switch {
			case pm == m.Model:
				row.Relation = "self-model"
			case domain.ModelFamily(pm) == row.Family && row.Relation != "self-model":
				row.Relation = "self-family"
			}
		}
		r.Models = append(r.Models, row)
	}
	sort.Slice(r.Models, func(i, j int) bool { return r.Models[i].Model < r.Models[j].Model })

	for _, f := range snap.Facts {
		if f.Status != domain.FactActive {
			continue
		}
		r.FactsByLevel[f.Level()]++
		if f.Level() == domain.VerifiedProxy {
			r.ProxyFacts++
			if f.Assertion.Consequence != nil {
				r.ProxyFactsByClass[f.Assertion.Consequence.ExposedClass]++
			}
			if f.Assertion.Subject != nil {
				r.ProxyFactsByFam[f.Assertion.Subject.Family]++
			}
		}
	}

	var durs []int64
	var first, last time.Time
	for _, e := range ledger {
		r.Attempts++
		r.CostUSD += e.CostUSD
		if e.Verdict != "" && e.QuestionType == domain.QuestionEvidenceSufficiency && e.Outcome == OutcomeDecided {
			r.GateVerdicts[e.Verdict]++
		}
		if e.Outcome == OutcomeSkipped {
			r.Skipped++
		}
		if e.Outcome == OutcomeFailed {
			r.Failures[e.Stage]++
			r.FailureList = append(r.FailureList, e)
		}
		if e.DurationMs > 0 {
			durs = append(durs, e.DurationMs)
		}
		if !e.StartedAt.IsZero() && (first.IsZero() || e.StartedAt.Before(first)) {
			first = e.StartedAt
		}
		if e.DecidedAt.After(last) {
			last = e.DecidedAt
		}
	}
	if len(durs) > 0 {
		sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
		r.MedianCallMs = durs[len(durs)/2]
		r.P90CallMs = durs[(len(durs)*9)/10]
	}
	if !first.IsZero() && last.After(first) {
		r.WallClock = last.Sub(first)
	}
	return r
}

// proposerKey names who authored the proposed assertion: the models whose
// proposals assert every verified aspect exactly as proposed, relative to the
// proxy (self-model / self-family / other-family / mixed / none).
func proposerKey(it domain.ReviewItem, props map[string]domain.SemanticProposal, pp *domain.Provenance) string {
	proxy := ""
	if pp != nil {
		proxy = pp.Model
	}
	verify := domain.QuestionAspects(it.QuestionType)
	rel := map[string]bool{}
	for _, id := range it.Proposals {
		p, ok := props[id]
		if !ok {
			continue
		}
		match := len(verify) > 0
		for _, x := range verify {
			match = match && p.Assertion.Has(x) && p.Assertion.AspectDigest(x) == it.Proposed.AspectDigest(x)
		}
		if !match {
			continue
		}
		switch {
		case p.Provenance.Model == proxy:
			rel["self-model"] = true
		case domain.ModelFamily(p.Provenance.Model) == domain.ModelFamily(proxy):
			rel["self-family"] = true
		default:
			rel["other-family"] = true
		}
	}
	switch {
	case len(rel) == 0:
		return "none"
	case rel["other-family"] && (rel["self-model"] || rel["self-family"]):
		return "mixed (self + other family)"
	case rel["other-family"]:
		return "other-family only"
	case rel["self-model"]:
		return "includes self-model"
	}
	return "self-family only (not self-model)"
}

// WriteReport renders the report as text.
func WriteReport(w io.Writer, r *Report) {
	fmt.Fprintf(w, "Proxy review report (proxy models: %s)\n\n", strings.Join(r.ProxyModels, ", "))
	fmt.Fprintf(w, "Decisions recorded: %d\n", r.Decisions)
	writeCounts(w, "By action", r.ByAction)
	writeCounts(w, "By label", r.ByLabel)
	fmt.Fprintln(w, "\nBy question type:")
	for _, q := range sortedKeys(r.ByQuestion) {
		fmt.Fprintf(w, "  %-22s %s\n", q, actionLine(r.ByQuestion[q]))
	}
	if len(r.GateVerdicts) > 0 {
		writeCounts(w, "Evidence-sufficiency verdicts", r.GateVerdicts)
	}
	fmt.Fprintln(w, "\nBy author of the proposed assertion (relative to the proxy):")
	for _, k := range sortedKeys(r.ByProposer) {
		fmt.Fprintf(w, "  %-34s %s\n", k, actionLine(r.ByProposer[k]))
	}
	fmt.Fprintln(w, "\nPer proposing model (knowledge.ComputeMetrics over the proxy decisions only; proposals/abstain count the whole store):")
	fmt.Fprintf(w, "  %-20s %-13s %9s %9s %9s %9s %9s %9s %s\n", "model", "relation", "proposals", "as-is", "corrected", "rejected", "insuff.", "abstain", "accept rate (as-is+corr / judged)")
	for _, m := range r.Models {
		judged := m.AcceptedAsIs + m.AcceptedCorrected + m.Rejected + m.InsufficientEvidence
		rate := 0.0
		if judged > 0 {
			rate = float64(m.AcceptedAsIs+m.AcceptedCorrected) / float64(judged)
		}
		fmt.Fprintf(w, "  %-20s %-13s %9d %9d %9d %9d %9d %9d %.2f (as-is %.2f)\n", m.Model, m.Relation, m.Proposals, m.AcceptedAsIs,
			m.AcceptedCorrected, m.Rejected, m.InsufficientEvidence, m.Abstentions, rate, ratio(m.AcceptedAsIs, judged))
	}
	fmt.Fprintf(w, "\nActive facts by level: %s\n", countLine(r.FactsByLevel))
	fmt.Fprintf(w, "Facts minted at proxy level: %d · by exposed class: %s · by family: %s\n", r.ProxyFacts,
		countLine(r.ProxyFactsByClass), countLine(r.ProxyFactsByFam))
	fmt.Fprintf(w, "\nLedger: %d entries, %d skipped (no longer pending, not called), failures by stage: %s\n", r.Attempts, r.Skipped, countLine(r.Failures))
	fmt.Fprintf(w, "Cost: $%.2f (CLI-reported) · median call %.1fs · p90 %.1fs · wall clock %s\n", r.CostUSD,
		float64(r.MedianCallMs)/1000, float64(r.P90CallMs)/1000, r.WallClock.Round(time.Second))
	for _, f := range r.FailureList {
		fmt.Fprintf(w, "  failed %s [%s] %s: %s\n", f.ItemID, f.QuestionType, f.Stage, shorten(oneLine(f.Error), 200))
	}
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func actionLine(m map[domain.DecisionAction]int) string {
	total := 0
	for _, n := range m {
		total += n
	}
	var parts []string
	for _, a := range []domain.DecisionAction{domain.ActionAccept, domain.ActionCorrect, domain.ActionReject, domain.ActionNeedMoreEvidence, domain.ActionDefer} {
		if n := m[a]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d (%.0f%%)", a, n, 100*float64(n)/float64(total)))
		}
	}
	return fmt.Sprintf("n=%d: %s", total, strings.Join(parts, ", "))
}

func writeCounts[K ~string](w io.Writer, title string, m map[K]int) {
	fmt.Fprintf(w, "%s: %s\n", title, countLine(m))
}

func countLine[K ~string](m map[K]int) string {
	var parts []string
	for _, k := range sortedKeys(m) {
		parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func sortedKeys[K ~string, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
