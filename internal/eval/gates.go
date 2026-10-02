package eval

// Hard gates (G11): `ri eval` reports a gate panel and exits non-zero when a
// gate fails. Thresholds are pre-registered — they were fixed in
// eval/gates.yaml from the phase plan BEFORE the expanded dataset was scored
// (dataset first, gates activate after `-update` stabilises the stored
// results); gate failures on the honest numbers are FINDINGS, not things the
// pipeline gets tuned against in this workstream.

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// GatesFileName is the dataset-root file holding the thresholds.
const GatesFileName = "gates.yaml"

// GateConfig is the pre-registered thresholds (all optional in the file;
// defaults come from DefaultGateConfig).
type GateConfig struct {
	// CriticalRecall minimum (plan: >= 0.95).
	CriticalRecall *float64 `json:"criticalRecall" yaml:"criticalRecall"`
	// ImportantRecall minimum (plan: >= 0.90).
	ImportantRecall *float64 `json:"importantRecall" yaml:"importantRecall"`
	// ApplicabilityAccuracy minimum (plan: >= 0.80).
	ApplicabilityAccuracy *float64 `json:"applicabilityAccuracy" yaml:"applicabilityAccuracy"`
	// FalseActionRate maximum EXCLUSIVE (plan: < 0.05).
	FalseActionRate *float64 `json:"falseActionRate" yaml:"falseActionRate"`
	// ActionFindingEvidence minimum share of ACTION findings whose both
	// provenance chains resolve (plan: 1.0 — 100 %).
	ActionFindingEvidence *float64 `json:"actionFindingEvidence" yaml:"actionFindingEvidence"`
	// UnsupportedMax is the maximum absolute number of unsupported
	// conclusions (plan: ≈ 0).
	UnsupportedMax *int `json:"unsupportedMax" yaml:"unsupportedMax"`
	// PipelineFailuresMax is the maximum absolute number of pipeline
	// execution errors (counted separately from reasoning misses; plan: 0).
	PipelineFailuresMax *int `json:"pipelineFailuresMax" yaml:"pipelineFailuresMax"`
}

// DefaultGateConfig returns the plan's defaults (used for every threshold
// the file omits).
func DefaultGateConfig() GateConfig {
	mm := func(f float64) *float64 { return &f }
	ii := func(i int) *int { return &i }
	return GateConfig{
		CriticalRecall:        mm(0.95),
		ImportantRecall:       mm(0.90),
		ApplicabilityAccuracy: mm(0.80),
		FalseActionRate:       mm(0.05),
		ActionFindingEvidence: mm(1.0),
		UnsupportedMax:        ii(0),
		PipelineFailuresMax:   ii(0),
	}
}

// LoadGates reads <root>/gates.yaml over the defaults. A missing file is the
// defaults; a malformed file is an error.
func LoadGates(root string) (GateConfig, error) {
	cfg := DefaultGateConfig()
	b, err := os.ReadFile(joinRoot(root, GatesFileName))
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	var raw GateConfig
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return cfg, fmt.Errorf("%s: %w", GatesFileName, err)
	}
	if raw.CriticalRecall != nil {
		cfg.CriticalRecall = raw.CriticalRecall
	}
	if raw.ImportantRecall != nil {
		cfg.ImportantRecall = raw.ImportantRecall
	}
	if raw.ApplicabilityAccuracy != nil {
		cfg.ApplicabilityAccuracy = raw.ApplicabilityAccuracy
	}
	if raw.FalseActionRate != nil {
		cfg.FalseActionRate = raw.FalseActionRate
	}
	if raw.ActionFindingEvidence != nil {
		cfg.ActionFindingEvidence = raw.ActionFindingEvidence
	}
	if raw.UnsupportedMax != nil {
		cfg.UnsupportedMax = raw.UnsupportedMax
	}
	if raw.PipelineFailuresMax != nil {
		cfg.PipelineFailuresMax = raw.PipelineFailuresMax
	}
	return cfg, nil
}

// GateResult is one evaluated gate.
type GateResult struct {
	Name      string  `json:"name"`
	Kind      string  `json:"kind"` // "minimum" | "maximum" | "maximum-exclusive" | "maximum-absolute"
	Threshold float64 `json:"threshold"`
	Actual    float64 `json:"actual"`
	Pass      bool    `json:"pass"`
	// Vacuous is true when the dataset observed nothing for this gate (e.g.
	// no action-required findings produced). A vacuous gate passes but says
	// so: it must not read as evidence of quality.
	Vacuous bool   `json:"vacuous,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// EvaluateGates judges the aggregate against the thresholds.
func EvaluateGates(cfg GateConfig, agg Aggregate) []GateResult {
	var out []GateResult
	// add records one gate. A vacuous gate (the dataset observed nothing it
	// could judge) always passes — but says so: it must not read as
	// evidence of quality.
	add := func(name, kind string, threshold, actual float64, pass, vacuous bool, detail string) GateResult {
		return GateResult{Name: name, Kind: kind, Threshold: threshold, Actual: actual,
			Pass: pass || vacuous, Vacuous: vacuous, Detail: detail}
	}
	if cfg.CriticalRecall != nil {
		out = append(out, add("criticalRecall", "minimum", *cfg.CriticalRecall, agg.CriticalRecall,
			agg.CriticalRecall >= *cfg.CriticalRecall, agg.FoundCritical+agg.MissedCritical == 0,
			fmt.Sprintf("%d critical items, %d missed", agg.FoundCritical+agg.MissedCritical, agg.MissedCritical)))
	}
	if cfg.ImportantRecall != nil {
		out = append(out, add("importantRecall", "minimum", *cfg.ImportantRecall, agg.ImportantRecall,
			agg.ImportantRecall >= *cfg.ImportantRecall, agg.FoundImportant+agg.MissedImportant == 0,
			fmt.Sprintf("%d important items, %d missed", agg.FoundImportant+agg.MissedImportant, agg.MissedImportant)))
	}
	if cfg.ApplicabilityAccuracy != nil {
		decisions := agg.ImpactLinks + agg.NotAffectedLinks
		out = append(out, add("applicabilityAccuracy", "minimum", *cfg.ApplicabilityAccuracy, agg.ApplicabilityAccuracy,
			agg.ApplicabilityAccuracy >= *cfg.ApplicabilityAccuracy, decisions == 0,
			fmt.Sprintf("%d applicability decisions (affected %d, not-affected %d)", decisions, agg.ImpactLinks, agg.NotAffectedLinks)))
	}
	if cfg.FalseActionRate != nil {
		out = append(out, add("falseActionRate", "maximum-exclusive", *cfg.FalseActionRate, agg.FalseActionRate,
			agg.FalseActionRate < *cfg.FalseActionRate, agg.ActionFindings == 0,
			fmt.Sprintf("%d action-required findings, %d wrong", agg.ActionFindings, agg.FalseActionFindings)))
	}
	if cfg.ActionFindingEvidence != nil {
		share := 1.0
		vacuous := agg.ActionFindings == 0
		if agg.ActionFindings > 0 {
			share = 1.0 - float64(agg.ActionFindingsUnsupported)/float64(agg.ActionFindings)
		}
		out = append(out, add("actionFindingEvidence", "minimum", *cfg.ActionFindingEvidence, share,
			share >= *cfg.ActionFindingEvidence, vacuous,
			fmt.Sprintf("%d action-required findings, %d with unresolving provenance", agg.ActionFindings, agg.ActionFindingsUnsupported)))
	}
	if cfg.UnsupportedMax != nil {
		out = append(out, add("unsupported", "maximum-absolute", float64(*cfg.UnsupportedMax), float64(agg.Unsupported),
			agg.Unsupported <= *cfg.UnsupportedMax, false,
			fmt.Sprintf("%d unsupported conclusions", agg.Unsupported)))
	}
	if cfg.PipelineFailuresMax != nil {
		out = append(out, add("pipelineFailures", "maximum-absolute", float64(*cfg.PipelineFailuresMax), float64(agg.PipelineFailures),
			agg.PipelineFailures <= *cfg.PipelineFailuresMax, false,
			fmt.Sprintf("%d entries failed to execute (counted separately from reasoning misses)", agg.PipelineFailures)))
	}
	return out
}

// HasGateFailure reports whether any gate failed (vacuous passes are passes).
func HasGateFailure(gates []GateResult) bool {
	for _, g := range gates {
		if !g.Pass {
			return true
		}
	}
	return false
}

// RenderGates prints the gate panel.
func RenderGates(w io.Writer, gates []GateResult) error {
	fmt.Fprintf(w, "── gates (pre-registered thresholds, eval/gates.yaml) ────\n")
	for _, g := range gates {
		mark := "✓"
		if !g.Pass {
			mark = "✗"
		}
		fmt.Fprintf(w, "%s %-24s %-10s actual %.3f %s\n", mark, g.Name, g.Kind, g.Actual, gateThresholdLabel(g))
		if g.Vacuous {
			fmt.Fprintf(w, "    vacuous: %s (nothing observed — passes, proves nothing)\n", g.Detail)
		} else if g.Detail != "" {
			fmt.Fprintf(w, "    %s\n", g.Detail)
		}
	}
	return nil
}

func gateThresholdLabel(g GateResult) string {
	switch g.Kind {
	case "minimum":
		return fmt.Sprintf("≥ %.2f", g.Threshold)
	case "maximum":
		return fmt.Sprintf("≤ %.2f", g.Threshold)
	case "maximum-exclusive":
		return fmt.Sprintf("< %.2f", g.Threshold)
	default:
		return fmt.Sprintf("≤ %.0f", g.Threshold)
	}
}

func joinRoot(root, name string) string { return root + string(os.PathSeparator) + name }
