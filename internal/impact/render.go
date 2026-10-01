package impact

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

const (
	ansiBold    = "1"
	ansiDim     = "2"
	ansiRed     = "31"
	ansiGreen   = "32"
	ansiYellow  = "33"
	ansiMagenta = "35"
)

type renderer struct {
	r        *domain.ImpactReport
	opts     RenderOptions
	sb       strings.Builder
	upEv     map[domain.EvidenceID]domain.Evidence
	locEv    map[domain.EvidenceID]domain.Evidence
	upCited  []domain.EvidenceID
	locCited []domain.EvidenceID
	cited    map[domain.EvidenceID]bool
}

func renderText(w io.Writer, r *domain.ImpactReport, opts RenderOptions) error {
	if r == nil {
		return errors.New("impact: nil report")
	}
	rr := &renderer{
		r: r, opts: opts, cited: map[domain.EvidenceID]bool{},
		upEv: map[domain.EvidenceID]domain.Evidence{}, locEv: map[domain.EvidenceID]domain.Evidence{},
	}
	for _, e := range r.Evidence {
		rr.upEv[e.ID] = e
	}
	for _, e := range r.EnvironmentEvidence {
		rr.locEv[e.ID] = e
	}
	rr.render()
	_, err := io.WriteString(w, rr.sb.String())
	return err
}

func (r *renderer) paint(code, s string) string {
	if !r.opts.Color || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (r *renderer) line(format string, args ...any) {
	fmt.Fprintf(&r.sb, format, args...)
	r.sb.WriteByte('\n')
}

func (r *renderer) heading(code, title string) {
	r.sb.WriteByte('\n')
	r.line("%s", r.paint(code, title))
}

// cite records an id as cited and returns the bare id.
func (r *renderer) citeUp(id domain.EvidenceID) string {
	if !r.cited[id] {
		r.cited[id] = true
		r.upCited = append(r.upCited, id)
	}
	return string(id)
}

func (r *renderer) citeLoc(id domain.EvidenceID) string {
	if !r.cited[id] {
		r.cited[id] = true
		r.locCited = append(r.locCited, id)
	}
	return string(id)
}

func (r *renderer) render() {
	r.header()
	r.environmentSection()
	r.findingSections()
	r.warnings()
	r.evidenceLegend()
}

// header prints the funnel: every upstream change is analyzed and every
// verdict class is counted explicitly — unknowns are never folded into
// "not affected" (docs/ACTION_CLASSIFICATION.md).
func (r *renderer) header() {
	e := r.r
	name := e.Product.Name
	if name == "" {
		name = string(e.Product.ID)
	}
	r.line("%s", r.paint(ansiBold, fmt.Sprintf("%s %s → %s — impact on this environment", name, e.From, e.To)))
	r.line("%d upstream changes analyzed", e.Summary.UpstreamChanges)
	s := e.Summary
	counts := []struct {
		label string
		n     int
		color string
	}{
		{"ACTION REQUIRED", s.ActionRequired, ansiRed},
		{"REVIEW REQUIRED", s.ReviewRequired, ansiYellow},
		{"INFORMATIONAL", s.Informational, ansiGreen},
		{"NOT AFFECTED", s.NotAffected, ansiDim},
		{"UNKNOWN", s.Unknown, ansiMagenta},
	}
	width := 1
	for _, c := range counts {
		if d := len(fmt.Sprint(c.n)); d > width {
			width = d
		}
	}
	for _, c := range counts {
		r.line("%s %*d", r.paint(c.color, c.label+":"), width+2, c.n)
	}
}

func (r *renderer) environmentSection() {
	env := r.r.Environment
	r.heading(ansiDim, "Environment")
	var parts []string
	if env.Kubernetes != "" {
		parts = append(parts, "kubernetes "+env.Kubernetes)
	}
	if env.ValuesKeys > 0 {
		parts = append(parts, fmt.Sprintf("values: %d keys set", env.ValuesKeys))
	}
	if env.ManifestDocs > 0 {
		parts = append(parts, fmt.Sprintf("manifests: %d docs, %d apiVersions, %d field paths", env.ManifestDocs, env.APIVersions, env.ManifestPaths))
	}
	if env.CRDs > 0 {
		parts = append(parts, fmt.Sprintf("installed CRDs: %d", env.CRDs))
	}
	if env.Images > 0 {
		parts = append(parts, fmt.Sprintf("images: %d", env.Images))
	}
	if len(parts) == 0 {
		r.line("  (no environment facts)")
	}
	for _, p := range parts {
		r.line("  %s", p)
	}
	for _, f := range env.Files {
		r.line("  %s  %s", f.Path, r.paint(ansiDim, f.Digest))
	}
}

func (r *renderer) findingSections() {
	byClass := map[domain.ImpactClass][]domain.ImpactFinding{}
	for _, f := range r.r.Findings {
		byClass[f.Classification] = append(byClass[f.Classification], f)
	}
	titles := map[domain.ImpactClass]string{
		domain.ImpactActionRequired: "Action required",
		domain.ImpactReviewRequired: "Review required",
		domain.ImpactInformational:  "Informational",
		domain.ImpactUnknown:        "Unknown — insufficient evidence",
		domain.ImpactNotAffected:    "Not affected",
	}
	colors := map[domain.ImpactClass]string{
		domain.ImpactActionRequired: ansiRed,
		domain.ImpactReviewRequired: ansiYellow,
		domain.ImpactInformational:  ansiGreen,
		domain.ImpactUnknown:        ansiMagenta,
		domain.ImpactNotAffected:    ansiDim,
	}
	for _, class := range domain.AllImpactClasses {
		fs := byClass[class]
		if len(fs) == 0 {
			continue
		}
		if class == domain.ImpactNotAffected && !r.opts.ShowNotAffected {
			continue // counted in the funnel; rendered only in verbose mode
		}
		r.heading(colors[class], fmt.Sprintf("%s (%d)", titles[class], len(fs)))
		for i, f := range fs {
			if class == domain.ImpactUnknown {
				r.unknownFinding(i+1, f)
			} else {
				r.finding(i+1, f)
			}
		}
	}
}

// unknownFinding renders one unknown verdict. The missing-evidence list is
// the point of the class, so it is always printed; otherwise identical to a
// normal finding.
func (r *renderer) unknownFinding(n int, f domain.ImpactFinding) {
	r.line("  %d. %s %s", n, f.Title, r.paint(ansiDim, "["+f.ID+"]"))
	r.line("     missing: %s", strings.Join(f.NeededToDetermine, "; "))
	r.changeAndDetail(f)
	r.upstreamChain(f)
}

func (r *renderer) finding(n int, f domain.ImpactFinding) {
	r.line("  %d. %s %s", n, f.Title, r.paint(ansiDim, "["+f.ID+"]"))
	r.changeAndDetail(f)
	// chain 2: what in the environment matched, where
	for _, m := range f.Matches {
		var evs []string
		for _, id := range m.Evidence {
			evs = append(evs, r.citeLoc(id))
		}
		r.line("     environment: %s %s  (%s)", m.Kind, m.Subject, strings.Join(evs, ", "))
	}
	// evaluation record of a not-affected verdict: what was checked against what
	for _, c := range f.Checks {
		d := string(c.Dimension)
		if c.Platform != "" {
			d += " (" + c.Platform + ")"
		}
		var evs []string
		for _, id := range c.Evidence {
			evs = append(evs, r.citeLoc(id))
		}
		evPart := ""
		if len(evs) > 0 {
			evPart = "  [" + strings.Join(evs, ", ") + "]"
		}
		r.line("     checked: %s, %d fact(s)  ← %s%s", d, c.Facts, strings.Join(c.Subjects, ", "), evPart)
	}
	if len(f.NeededToDetermine) > 0 {
		r.line("     missing: %s", strings.Join(f.NeededToDetermine, "; "))
	}
	r.upstreamChain(f)
}

func (r *renderer) changeAndDetail(f domain.ImpactFinding) {
	if f.ChangeID != "" {
		marks := ""
		if f.ChangeBreaking {
			marks = "breaking · "
		} else if f.ChangeActionRequired {
			marks = "action required · "
		}
		r.line("     upstream change: %s%s (%s) [%s]", marks, f.ChangeTitle, f.ChangeCategory, f.ChangeID)
	}
	for _, ln := range strings.Split(f.Detail, "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		r.line("     %s", ln)
	}
}

func (r *renderer) upstreamChain(f domain.ImpactFinding) {
	// chain 1: upstream evidence
	var ups []string
	for _, id := range f.UpstreamEvidence {
		ups = append(ups, r.citeUp(id))
	}
	r.line("     upstream evidence: %s", strings.Join(ups, ", "))
}

func (r *renderer) warnings() {
	warns := r.r.Warnings
	if len(warns) == 0 {
		return
	}
	r.heading(ansiYellow, fmt.Sprintf("Warnings (%d)", len(warns)))
	for _, w := range warns {
		r.line("  · %s", w)
	}
}

func (r *renderer) evidenceLegend() {
	if len(r.locCited) == 0 && len(r.upCited) == 0 {
		return
	}
	r.heading(ansiDim, "Evidence")
	for _, id := range r.locCited {
		e := r.locEv[id]
		r.line("  %s  %s  %s  %s", id, e.Kind, e.URI, e.Locator)
	}
	for _, id := range r.upCited {
		e := r.upEv[id]
		r.line("  %s  %s  %s  %s", id, e.Kind, e.URI, e.Locator)
	}
}
