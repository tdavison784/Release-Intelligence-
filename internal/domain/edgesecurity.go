package domain

import (
	"fmt"
	"time"
)

// EdgeSecuritySchemaVersion is the serialisation version of
// UpgradeEdge.Security, versioned apart from the edge so the section can
// evolve without a new edge version.
const EdgeSecuritySchemaVersion = "ri.dev/edge-security/v1alpha1"

// SecurityStatus says how much of the advisory picture an EdgeSecurity
// covers.
type SecurityStatus string

const (
	// SecurityComplete: every declared security source was read.
	SecurityComplete SecurityStatus = "complete"
	// SecurityIncomplete: at least one source was read and at least one
	// could not be; what was read is reported, the rest is a gap.
	SecurityIncomplete SecurityStatus = "incomplete"
	// SecurityUnavailable: no source could be read, or none is declared.
	// Nothing is known; that is never "no advisories".
	SecurityUnavailable SecurityStatus = "unavailable"
)

// CandidateKind names which question a VersionCandidate answers.
type CandidateKind string

const (
	// CandidateTargetLine: the lowest release on the target's line, at or
	// above the target, affected by no advisory the current version is free
	// of (it removes every regression).
	CandidateTargetLine CandidateKind = "target-line"
	// CandidateAnyLine: the same on any line, at or above the target.
	CandidateAnyLine CandidateKind = "any-line"
	// CandidateClean: the lowest release at or above the target affected by
	// no assessed advisory.
	CandidateClean CandidateKind = "clean"
	// CandidateCurrentLine is information, not an upgrade target: the
	// lowest later patch of the current version's line, below the target,
	// that removes every advisory persisting into the target and introduces
	// none. It informs a defer or stay decision.
	CandidateCurrentLine CandidateKind = "current-line"
)

// EdgeSecurity is the deterministic advisory assessment of an upgrade: which
// advisories the target introduces, keeps or removes relative to the current
// version, and which known releases would avoid them. Every advisory entry
// and candidate cites the advisory evidence (source page, digest, retrieval
// time) its range came from.
type EdgeSecurity struct {
	SchemaVersion string         `json:"schemaVersion"`
	Status        SecurityStatus `json:"status"`
	// Sources are the security sources consulted, with their state.
	Sources []SourceStatus `json:"sources"`
	// Gaps lists every reason the assessment is not complete: a source that
	// could not be read, an advisory without a machine-readable range, no
	// known release list. Never empty when Status is not complete.
	Gaps []string `json:"gaps,omitempty"`
	// Regressions affect the target but not the current version.
	Regressions []SecurityAdvisory `json:"regressions,omitempty"`
	// Persisting affect both.
	Persisting []SecurityAdvisory `json:"persisting,omitempty"`
	// Fixed affect the current version but not the target.
	Fixed []SecurityAdvisory `json:"fixed,omitempty"`
	// Unassessed have no machine-readable affected range: whether they
	// apply to either version is unknown (a manual check). They are matched
	// against nothing, so their evidence records are not part of the edge;
	// URL and SourceID locate them.
	Unassessed []SecurityAdvisory `json:"unassessed,omitempty"`
	// Candidates are computed when the target is affected by an assessed
	// advisory and the source status allows it; see CandidateKind.
	Candidates []VersionCandidate `json:"candidates,omitempty"`
	// VersionsConsidered is how many known stable releases at or above the
	// target the upgrade-target candidates were chosen from.
	VersionsConsidered int `json:"versionsConsidered,omitempty"`
}

// SecurityAdvisory is one advisory as the assessment used it.
type SecurityAdvisory struct {
	ID       string   `json:"id"`
	Aliases  []string `json:"aliases,omitempty"`
	Severity string   `json:"severity,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	URL      string   `json:"url,omitempty"`
	// Vulnerable is the affected-version constraint that was evaluated
	// (empty for an unassessed advisory).
	Vulnerable string   `json:"vulnerable,omitempty"`
	Patched    []string `json:"patched,omitempty"`
	// SourceID is the source whose range was used; Sources every source
	// that lists the advisory.
	SourceID    string     `json:"sourceId"`
	Sources     []string   `json:"sources,omitempty"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty"`
	// ChangeID is the edge change that reports the advisory, if any.
	ChangeID string       `json:"changeId,omitempty"`
	Evidence []EvidenceID `json:"evidence"`
}

// AdvisoryRef names an advisory and the evidence of its range.
type AdvisoryRef struct {
	ID       string       `json:"id"`
	Evidence []EvidenceID `json:"evidence"`
}

// VersionCandidate is one candidate target version, or the statement that
// none exists (Version nil).
type VersionCandidate struct {
	Kind    CandidateKind `json:"kind"`
	Version *Version      `json:"version,omitempty"`
	// Line is the release line searched (target-line and current-line).
	Line string `json:"line,omitempty"`
	// Statement says in one sentence what the candidate establishes.
	Statement string `json:"statement"`
	// Removes are the advisories affecting the target that do not affect the
	// candidate; Leaves those that affect the candidate.
	Removes []AdvisoryRef `json:"removes,omitempty"`
	Leaves  []AdvisoryRef `json:"leaves,omitempty"`
	// Unassessed are the advisories the candidate cannot be cleared of
	// (no machine-readable range).
	Unassessed []string `json:"unassessed,omitempty"`
	// Charts are the chart versions that ship the candidate, where the
	// product ships as a chart; ChartGaps say why a lookup gave nothing.
	Charts    []CandidateChart `json:"charts,omitempty"`
	ChartGaps []string         `json:"chartGaps,omitempty"`
	Evidence  []EvidenceID     `json:"evidence"`
}

// Label names the candidate in a report: its kind, its line, and for the
// current-line entry the fact that it is not an upgrade target.
func (c VersionCandidate) Label() string {
	name := string(c.Kind)
	if c.Line != "" {
		name += " " + c.Line
	}
	if c.Kind == CandidateCurrentLine {
		name += " (information for a defer or stay decision, not a target)"
	}
	return name
}

// CandidateChart is a chart version shipping a candidate release.
type CandidateChart struct {
	Artifact string       `json:"artifact"`
	Name     string       `json:"name,omitempty"`
	Version  string       `json:"version"`
	Evidence []EvidenceID `json:"evidence"`
}

// validate checks the section's invariants against the edge's evidence ids.
func (s *EdgeSecurity) validate(ev map[EvidenceID]bool) []error {
	var errs []error
	if s.SchemaVersion != EdgeSecuritySchemaVersion {
		errs = append(errs, fmt.Errorf("security: schemaVersion %q, want %q", s.SchemaVersion, EdgeSecuritySchemaVersion))
	}
	switch s.Status {
	case SecurityComplete:
	case SecurityIncomplete, SecurityUnavailable:
		if len(s.Gaps) == 0 {
			errs = append(errs, fmt.Errorf("security: status %s without a gap", s.Status))
		}
	default:
		errs = append(errs, fmt.Errorf("security: unknown status %q", s.Status))
	}
	cite := func(what string, ids []EvidenceID, required bool) {
		if required && len(ids) == 0 {
			errs = append(errs, fmt.Errorf("security: %s cites no evidence", what))
		}
		for _, id := range ids {
			if !ev[id] {
				errs = append(errs, fmt.Errorf("security: %s references unknown evidence %s", what, id))
			}
		}
	}
	for _, list := range [][]SecurityAdvisory{s.Regressions, s.Persisting, s.Fixed} {
		for _, a := range list {
			cite("advisory "+a.ID, a.Evidence, true)
		}
	}
	for _, a := range s.Unassessed {
		cite("advisory "+a.ID, a.Evidence, false)
	}
	for _, c := range s.Candidates {
		what := "candidate " + string(c.Kind)
		cite(what, c.Evidence, c.Version != nil)
		for _, r := range append(append([]AdvisoryRef(nil), c.Removes...), c.Leaves...) {
			cite(what+" advisory "+r.ID, r.Evidence, true)
		}
		for _, ch := range c.Charts {
			cite(what+" chart "+ch.Version, ch.Evidence, true)
		}
	}
	return errs
}
