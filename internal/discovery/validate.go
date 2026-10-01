package discovery

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/ingest"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// RelationshipChecker verifies a definition's sources and artifacts against
// releases. (*ingest.Ingester) satisfies it; tests use fakes.
type RelationshipChecker interface {
	CheckRelationships(ctx context.Context, def *catalog.ProductDefinition, releases []domain.Version, known *ingest.VersionList) (*ingest.RelationshipReport, error)
}

// Validation statuses.
const (
	ValidationDone    = "done"
	ValidationSkipped = "skipped"
	ValidationError   = "error"
)

// Element verdicts.
const (
	VerdictValidated    = "validated"
	VerdictFailing      = "failing"
	VerdictInsufficient = "insufficient"
	VerdictUnverifiable = "unverifiable"
	VerdictNotChecked   = "not-checked"
)

// ValidationResult is the historical validation of a draft.
type ValidationResult struct {
	Status   string                       `json:"status"`
	Detail   string                       `json:"detail,omitempty"`
	Releases []string                     `json:"releases"`
	Checks   []ingest.RelationshipCheck   `json:"checks,omitempty"`
	Summary  []ingest.RelationshipSummary `json:"summary,omitempty"`
	// Verdicts maps element keys ("source:<id>", "artifact:<id>") to verdicts.
	Verdicts map[string]string `json:"verdicts,omitempty"`
	// Passed lists, per element key, the releases for which it held.
	Passed   map[string][]string `json:"passed,omitempty"`
	Evidence []domain.Evidence   `json:"evidence,omitempty"`
}

// Verdict returns the verdict for an element key.
func (v *ValidationResult) Verdict(key string) string {
	if v == nil || v.Status != ValidationDone {
		return VerdictNotChecked
	}
	if s, ok := v.Verdicts[key]; ok {
		return s
	}
	return VerdictNotChecked
}

// Validator runs relationship checks.
type Validator struct {
	Checker RelationshipChecker
	// Releases is how many recent stable releases to check (default
	// ingest.MinValidations+1, never fewer than ingest.MinValidations).
	Releases int
}

// KnownVersions builds the version list handed to the checker.
func KnownVersions(product string, ta *TagAnalysis) *ingest.VersionList {
	vl := &ingest.VersionList{Product: domain.ProductID(product), Refs: map[string]sources.ReleaseRef{}}
	for _, v := range ta.Stable() {
		vl.Versions = append(vl.Versions, v)
		rt := ta.byTag[v.Tag]
		vl.Refs[v.Semver] = sources.ReleaseRef{Tag: v.Tag, Commit: rt.Commit, URL: ta.repo.TagURL(v.Tag), Evidence: ta.Evidence(v.Tag, ta.ListedAt)}
	}
	return vl
}

// Validate checks the draft definition together with every pending AI
// element. It never fails: problems are reported in the result.
func (v *Validator) Validate(ctx context.Context, d *Draft, ta *TagAnalysis) *ValidationResult {
	res := &ValidationResult{Verdicts: map[string]string{}, Passed: map[string][]string{}}
	if v == nil || v.Checker == nil {
		res.Status, res.Detail = ValidationSkipped, "no relationship checker configured"
		return res
	}
	n := v.Releases
	if n < ingest.MinValidations {
		n = ingest.MinValidations + 1
	}
	releases := ta.SelectValidationReleases(n)
	for _, r := range releases {
		res.Releases = append(res.Releases, r.Tag)
	}
	if len(releases) < ingest.MinValidations {
		res.Status = ValidationSkipped
		res.Detail = fmt.Sprintf("only %d stable releases available, %d required", len(releases), ingest.MinValidations)
		return res
	}
	combined := *d.Definition
	combined.Sources = append(append([]catalog.Source{}, d.Definition.Sources...), d.AISources...)
	combined.Artifacts = append(append([]catalog.Artifact{}, d.Definition.Artifacts...), d.AIArtifacts...)
	rep, err := v.Checker.CheckRelationships(ctx, &combined, releases, KnownVersions(d.Definition.ID, ta))
	if err != nil {
		res.Status, res.Detail = ValidationError, err.Error()
		return res
	}
	res.Status = ValidationDone
	res.Checks, res.Summary, res.Evidence = rep.Checks, rep.Summary, rep.Evidence
	type agg struct{ pass, fail, unver, na int }
	counts := map[string]*agg{}
	for _, c := range rep.Checks {
		key := c.SubjectKind + ":" + c.Subject
		a := counts[key]
		if a == nil {
			a = &agg{}
			counts[key] = a
		}
		switch c.Outcome {
		case ingest.OutcomePass:
			a.pass++
			if !containsStr(res.Passed[key], c.Release) {
				res.Passed[key] = append(res.Passed[key], c.Release)
			}
		case ingest.OutcomeFail:
			a.fail++
		case ingest.OutcomeUnverifiable:
			a.unver++
		default:
			a.na++
		}
	}
	summaries := map[string]string{}
	for _, s := range rep.Summary {
		summaries[s.SubjectKind+":"+s.Subject] = s.Verdict
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	for k := range summaries {
		if _, ok := counts[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		a := counts[k]
		if a == nil {
			a = &agg{}
		}
		verdict := summaries[k]
		switch {
		case a.fail > 0:
			verdict = VerdictFailing
		case a.pass >= ingest.MinValidations:
			verdict = VerdictValidated
		case a.pass == 0 && a.unver > 0:
			verdict = VerdictUnverifiable
		case verdict == "" || verdict == VerdictValidated:
			verdict = VerdictInsufficient
		}
		res.Verdicts[k] = verdict
	}
	return res
}

// detail summarises the non-passing checks of an element.
func (v *ValidationResult) detail(key string) string {
	if v == nil {
		return ""
	}
	var parts []string
	for _, c := range v.Checks {
		if c.SubjectKind+":"+c.Subject != key || c.Outcome == ingest.OutcomePass {
			continue
		}
		s := c.Release + " " + c.Outcome
		if c.Detail != "" {
			s += " (" + c.Detail + ")"
		}
		parts = append(parts, s)
	}
	if len(parts) > 4 {
		parts = append(parts[:4], "…")
	}
	return strings.Join(parts, "; ")
}

// counts returns the number of passing and failing checks of an element.
func (v *ValidationResult) counts(key string) (pass, fail int) {
	if v == nil {
		return 0, 0
	}
	for _, c := range v.Checks {
		if c.SubjectKind+":"+c.Subject != key {
			continue
		}
		switch c.Outcome {
		case ingest.OutcomePass:
			pass++
		case ingest.OutcomeFail:
			fail++
		}
	}
	return pass, fail
}

// failuresBeforePasses reports whether every failing release is older than
// every passing one, returning the oldest passing release.
func (v *ValidationResult) failuresBeforePasses(key string, ta *TagAnalysis) (domain.Version, bool) {
	var fails, passes []domain.Version
	for _, c := range v.Checks {
		if c.SubjectKind+":"+c.Subject != key {
			continue
		}
		ver, ok := ta.Find(c.Release)
		if !ok {
			continue
		}
		switch c.Outcome {
		case ingest.OutcomePass:
			passes = append(passes, ver)
		case ingest.OutcomeFail:
			fails = append(fails, ver)
		}
	}
	if len(fails) == 0 || len(passes) == 0 {
		return domain.Version{}, false
	}
	domain.SortVersions(passes)
	for _, f := range fails {
		if !f.Less(passes[0]) {
			return domain.Version{}, false
		}
	}
	return passes[0], true
}
