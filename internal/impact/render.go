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

func (r *renderer) header() {
	e := r.r
	name := e.Product.Name
	if name == "" {
		name = string(e.Product.ID)
	}
	r.line("%s", r.paint(ansiBold, fmt.Sprintf("%s %s → %s — impact on this environment", name, e.From, e.To)))
	s := e.Summary
	r.line("%d upstream changes · %d affect this environment · %d action required · %d review · %d informational",
		s.UpstreamChanges, s.AffectEnvironment, s.ActionRequired, s.Review, s.Informational)
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
		domain.ImpactReview:         "Review",
		domain.ImpactInformational:  "Informational",
	}
	colors := map[domain.ImpactClass]string{
		domain.ImpactActionRequired: ansiRed,
		domain.ImpactReview:         ansiYellow,
		domain.ImpactInformational:  ansiGreen,
	}
	for _, class := range domain.AllImpactClasses {
		fs := byClass[class]
		if len(fs) == 0 {
			continue
		}
		r.heading(colors[class], fmt.Sprintf("%s (%d)", titles[class], len(fs)))
		for i, f := range fs {
			r.finding(i+1, f)
		}
	}
}

func (r *renderer) finding(n int, f domain.ImpactFinding) {
	r.line("  %d. %s %s", n, f.Title, r.paint(ansiDim, "["+f.ID+"]"))
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
	// chain 2: what in the environment matched, where
	for _, m := range f.Matches {
		var evs []string
		for _, id := range m.Evidence {
			evs = append(evs, r.citeLoc(id))
		}
		r.line("     environment: %s %s  (%s)", m.Kind, m.Subject, strings.Join(evs, ", "))
	}
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
