package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// renderEval collects the render-scoped R17 measurements of `ri eval -render`
// (RENDER-MISSION R17): each environment case is rendered with its own
// configuration before the join, so verified facts' rendered-change leaves
// are decided against the customer's renders; the eval's comparison against
// the stored (render-free) baseline is the before/after.
type renderEval struct {
	mu    sync.Mutex
	stats map[string]*renderCaseStat
}

// renderCaseStat is one environment case's render measurement.
type renderCaseStat struct {
	Case          string `json:"case"`
	Pairs         int    `json:"pairs"`
	Succeeded     int    `json:"succeeded"`
	Failed        int    `json:"failed"`
	NotApplicable int    `json:"notApplicable"`
	Incomplete    int    `json:"valuesIncomplete"`
	// TargetRejects counts pairs whose target chart refuses a configuration
	// the source accepted (the upgrade would fail at render time).
	TargetRejects int            `json:"targetRejects"`
	FailReasons   map[string]int `json:"failReasons,omitempty"`
	Changes       int            `json:"renderedChanges"`
	Undocumented  int            `json:"undocumented"`
	// Unknown findings, and those whose change a rendered change of a
	// successful customer render restates (Correlate) — the UNKNOWNs a
	// rendered-change fact could decide (potential, not decided).
	Unknown           int `json:"unknown"`
	UnknownRestated   int `json:"unknownRestatedByRender"`
	UnknownRestatedOK int `json:"unknownRestatedInCompleteRender"`
	// Findings decided through a rendered-change leaf (a match of kind
	// rendered-change or a render check): what rendering actually decided.
	DecidedByRender int `json:"decidedByRender"`
	ActionByRender  int `json:"actionWithRenderEvidence"`
	// PO-3 (values defaults / new keys the customer leaves unset):
	// DefaultExposed are impact:values-default-applies findings (an
	// attributable rendered change), DefaultImages those whose attributed
	// changes are all image changes (routine bumps), DefaultCleared
	// impact:values-default-no-effect verdicts, DefaultUnavailable
	// impact:values-default-unrendered verdicts.
	DefaultExposed     int `json:"defaultExposed"`
	DefaultImages      int `json:"defaultExposedImageOnly"`
	DefaultCleared     int `json:"defaultCleared"`
	DefaultUnavailable int `json:"defaultRenderUnavailable"`
	// ActionCorroborated counts ACTION REQUIRED findings (any rule) the
	// customer's render corroborates: the target chart's rejection names the
	// finding's subject, or a complete render restates its change. Evidence
	// strength, not a new class (R9).
	ActionCorroborated int `json:"actionCorroboratedByRender"`
	Action             int `json:"action"`
	// RestatedChanges are the edge change ids a complete customer render
	// restates (for joining with the eval's per-link results).
	RestatedChanges []string `json:"restatedChanges,omitempty"`
}

func (p appPipeline) renderImpact(ctx context.Context, product, from, to string, inputs env.Inputs, facts []domain.VerifiedFact, min domain.VerificationLevel) (*domain.ImpactReport, error) {
	run, err := p.a.ImpactRun(ctx, product, from, to, app.ImpactOptions{Environment: inputs, Facts: facts, MinVerification: min,
		Render: &app.RenderOptions{ValuesFiles: inputs.ValuesFiles, Repo: inputs.Repo, KubeVersion: inputs.KubernetesVersion}})
	if err != nil {
		return nil, err
	}
	p.render.record(fmt.Sprintf("%s %s→%s%s", product, from, to, caseOf(inputs)), run)
	return run.Report, nil
}

func (r *renderEval) record(key string, run *app.ImpactRun) {
	st := &renderCaseStat{Case: key, FailReasons: map[string]int{}}
	restated := map[string]bool{}   // edge change ids restated by a successful pair
	restatedOK := map[string]bool{} // … by a complete pair
	var rejections []string         // target-chart rejection messages
	for _, p := range run.Render.EnvironmentPairs() {
		st.Pairs++
		if !p.Target.ValuesComplete {
			st.Incomplete++
		}
		switch p.Status {
		case render.PairOK:
			st.Succeeded++
			corr := run.Render.Correlations[p.Target.ID]
			st.Changes += len(p.Diff.Changes)
			st.Undocumented += corr.Undocumented
			for _, cc := range corr.Changes {
				for _, l := range cc.Links {
					if l.ChangeID == "" {
						continue
					}
					restated[l.ChangeID] = true
					if p.Complete() {
						restatedOK[l.ChangeID] = true
					}
				}
			}
		case render.PairNotApplicable:
			st.NotApplicable++
		default:
			st.Failed++
		}
		if p.TargetRejects() {
			st.TargetRejects++
			rejections = append(rejections, p.Failure.Detail)
		}
		if p.Failure != nil && p.Status != render.PairOK {
			st.FailReasons[string(p.Failure.Reason)]++
		}
	}
	for id := range restatedOK {
		st.RestatedChanges = append(st.RestatedChanges, id)
	}
	sort.Strings(st.RestatedChanges)
	for _, f := range run.Report.Findings {
		if f.Classification == domain.ImpactUnknown {
			st.Unknown++
			if restated[f.ChangeID] {
				st.UnknownRestated++
			}
			if restatedOK[f.ChangeID] {
				st.UnknownRestatedOK++
			}
		}
		if f.Classification == domain.ImpactActionRequired {
			st.Action++
			corroborated := restatedOK[f.ChangeID]
			for _, m := range f.Matches {
				for _, msg := range rejections {
					corroborated = corroborated || (len(m.Subject) >= 4 && strings.Contains(msg, m.Subject))
				}
			}
			if corroborated {
				st.ActionCorroborated++
			}
		}
		switch f.Rule {
		case impact.RuleValuesDefaultApplies:
			st.DefaultExposed++
			images := len(f.Matches) > 0
			for _, m := range f.Matches {
				images = images && strings.Contains(m.Subject, "→ "+string(render.ImageChanged)+" ")
			}
			if images {
				st.DefaultImages++
			}
			continue
		case impact.RuleValuesDefaultNoEffect:
			st.DefaultCleared++
			continue
		case impact.RuleValuesDefaultUnrendered:
			st.DefaultUnavailable++
			continue
		}
		byRender := false
		for _, m := range f.Matches {
			byRender = byRender || m.Kind == domain.MatchRenderedChange
		}
		for _, c := range f.Checks {
			byRender = byRender || c.Dimension == domain.DimensionRender
		}
		if byRender {
			st.DecidedByRender++
			if f.Classification == domain.ImpactActionRequired {
				st.ActionByRender++
			}
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stats == nil {
		r.stats = map[string]*renderCaseStat{}
	}
	r.stats[key] = st // a knowledge-level re-run of the same case overwrites
}

// writeText prints the render section of `ri eval -render`.
func (r *renderEval) writeText(w io.Writer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.stats))
	for k := range r.stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var tot renderCaseStat
	fmt.Fprintf(w, "\nRendering (R17, --render on every environment case; before = the stored render-free baseline above)\n")
	for _, k := range keys {
		s := r.stats[k]
		var fr []string
		for reason, n := range s.FailReasons {
			fr = append(fr, fmt.Sprintf("%s %d", reason, n))
		}
		sort.Strings(fr)
		fmt.Fprintf(w, "  %-70s renders %d/%d ok (failed %d, n/a %d, values incomplete %d, target rejects values %d%s) · %d rendered changes, %d undocumented · unknown %d, restated by a render %d (complete %d) · rendered-change leaves decided %d (ACTION %d) · ACTION corroborated by render %d/%d · unset defaults: exposed %d (image-only %d), cleared %d, render unavailable %d\n",
			s.Case, s.Succeeded, s.Pairs, s.Failed, s.NotApplicable, s.Incomplete, s.TargetRejects, strings.TrimSuffix(" — "+strings.Join(fr, ", "), " — "),
			s.Changes, s.Undocumented, s.Unknown, s.UnknownRestated, s.UnknownRestatedOK, s.DecidedByRender, s.ActionByRender, s.ActionCorroborated, s.Action, s.DefaultExposed, s.DefaultImages, s.DefaultCleared, s.DefaultUnavailable)
		tot.Pairs += s.Pairs
		tot.Succeeded += s.Succeeded
		tot.Unknown += s.Unknown
		tot.UnknownRestated += s.UnknownRestated
		tot.UnknownRestatedOK += s.UnknownRestatedOK
		tot.DecidedByRender += s.DecidedByRender
		tot.ActionByRender += s.ActionByRender
		tot.ActionCorroborated += s.ActionCorroborated
		tot.Action += s.Action
		tot.TargetRejects += s.TargetRejects
		tot.DefaultExposed += s.DefaultExposed
		tot.DefaultImages += s.DefaultImages
		tot.DefaultCleared += s.DefaultCleared
		tot.DefaultUnavailable += s.DefaultUnavailable
		tot.Changes += s.Changes
		tot.Undocumented += s.Undocumented
	}
	rate := 0.0
	if tot.Pairs > 0 {
		rate = float64(tot.Succeeded) / float64(tot.Pairs)
	}
	fmt.Fprintf(w, "  total: render success %d/%d (%.2f) · %d rendered changes (%d undocumented) · unknown %d, restated by a customer render %d (complete %d) · rendered-change leaves decided %d · ACTION with render evidence %d · ACTION corroborated by render %d/%d · target rejects values %d · unset defaults (PO-3): exposed %d (image-only %d), cleared %d, render unavailable %d\n",
		tot.Succeeded, tot.Pairs, rate, tot.Changes, tot.Undocumented, tot.Unknown, tot.UnknownRestated, tot.UnknownRestatedOK, tot.DecidedByRender, tot.ActionByRender, tot.ActionCorroborated, tot.Action, tot.TargetRejects, tot.DefaultExposed, tot.DefaultImages, tot.DefaultCleared, tot.DefaultUnavailable)
}

// caseOf names the eval case an environment belongs to: the directory above
// "environment" in any input path (" [<case>]"), so two cases of one edge stay
// apart.
func caseOf(in env.Inputs) string {
	paths := append(append(append([]string{}, in.ValuesFiles...), in.Manifests...), in.CRDs...)
	paths = append(paths, in.Inventory, in.Repo)
	for _, p := range paths {
		if i := strings.Index(p, "/environment"); i > 0 {
			return " [" + filepath.Base(p[:i]) + "]"
		}
	}
	return ""
}

// writeJSON writes the per-case render measurements.
func (r *renderEval) writeJSON(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.stats))
	for k := range r.stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*renderCaseStat, 0, len(keys))
	for _, k := range keys {
		out = append(out, r.stats[k])
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
