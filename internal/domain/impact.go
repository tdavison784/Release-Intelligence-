package domain

import (
	"errors"
	"fmt"
	"time"
)

// ImpactReportSchemaVersion is the current serialisation version.
const ImpactReportSchemaVersion = "ri.dev/impact-report/v1alpha1"

// ImpactClass is the action classification of a finding: what the operator
// should do. It is a separate axis from severity (how bad) and confidence
// (how certain); see docs/ACTION_CLASSIFICATION.md — the contract this type
// implements.
type ImpactClass string

const (
	// ImpactActionRequired (ACTION REQUIRED): the environment must change
	// before/during the upgrade to avoid concrete failure, incompatibility,
	// or loss of intended behavior. Requires upstream evidence, environment
	// evidence and a deterministic relationship between them; must answer
	// "what exactly will fail if I do nothing?". Confidence below high is
	// prohibited here (demote to ImpactReviewRequired).
	ImpactActionRequired ImpactClass = "action-required"
	// ImpactReviewRequired (REVIEW REQUIRED): credible evidence the change
	// intersects the environment, but applicability/necessity cannot be
	// deterministically proven; the finding says what overlaps and what to
	// inspect. Receives every finding demoted from action-required.
	ImpactReviewRequired ImpactClass = "review-required"
	// ImpactInformational (INFORMATIONAL): a real, evidenced overlap with no
	// action implied ("applies to you, you appear safe": a changed default
	// the customer explicitly overrides, a cluster inside the supported
	// range).
	ImpactInformational ImpactClass = "informational"
	// ImpactNotAffected (NOT AFFECTED): evaluated against the environment
	// with sufficient evidence that it does not apply; carries the
	// evaluation record (Checks) proving what was compared. Summarised
	// always, rendered only in verbose mode.
	ImpactNotAffected ImpactClass = "not-affected"
	// ImpactUnknown (UNKNOWN / INSUFFICIENT EVIDENCE): applicability cannot
	// safely be determined; NeededToDetermine names the missing evidence.
	// Never silently becomes NOT AFFECTED or ACTION REQUIRED.
	ImpactUnknown ImpactClass = "unknown"
)

// AllImpactClasses lists every classification in display order.
var AllImpactClasses = []ImpactClass{ImpactActionRequired, ImpactReviewRequired, ImpactInformational, ImpactNotAffected, ImpactUnknown}

// Affected reports whether the class asserts a evidenced intersection with
// the environment (applicability AFFECTED): only these classes carry
// matches.
func (c ImpactClass) Affected() bool {
	return c == ImpactActionRequired || c == ImpactReviewRequired || c == ImpactInformational
}

// ImpactSeverity is how bad a finding is if it bites: an axis independent of
// classification and confidence. Optional — set only where the join can
// determine it deterministically.
type ImpactSeverity string

const (
	// SeverityCritical: the upgrade fails outright (Helm refuses the install
	// via kubeVersion; resources of a removed CRD/API stop being served).
	SeverityCritical ImpactSeverity = "critical"
	// SeverityHigh: concrete degradation (a removed values key stops taking
	// effect, a pruned CRD field is dropped/rejected, the cluster leaves the
	// supported range).
	SeverityHigh ImpactSeverity = "high"
	// SeverityMedium: needs a look; the outcome depends on intent (adjacent
	// values sections, newly live keys, deprecations, image changes).
	SeverityMedium ImpactSeverity = "medium"
	// SeverityLow: a confirmed no-action overlap (a pin that keeps winning,
	// an in-range cluster).
	SeverityLow ImpactSeverity = "low"
)

// AllImpactSeverities lists every severity in descending order of badness.
var AllImpactSeverities = []ImpactSeverity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow}

// EnvironmentDimension names the class of environment input a check or a
// visibility requirement talks about. For cluster checks the platform is
// carried in ImpactCheck.Platform.
type EnvironmentDimension string

const (
	// DimensionValues: Helm values files (--values).
	DimensionValues EnvironmentDimension = "values"
	// DimensionManifests: Kubernetes manifests (--manifests).
	DimensionManifests EnvironmentDimension = "manifests"
	// DimensionCRDs: installed CustomResourceDefinitions (--crds).
	DimensionCRDs EnvironmentDimension = "crds"
	// DimensionImages: container image references (--images, or images found
	// in values/manifests).
	DimensionImages EnvironmentDimension = "images"
	// DimensionCluster: the cluster version of a platform (Platform names
	// it, e.g. "kubernetes", "openshift").
	DimensionCluster EnvironmentDimension = "cluster-version"
)

// AllEnvironmentDimensions lists every dimension in display order.
var AllEnvironmentDimensions = []EnvironmentDimension{DimensionValues, DimensionManifests, DimensionCRDs, DimensionImages, DimensionCluster}

// ImpactCheck is one entry of a NOT AFFECTED (or partially-evaluated UNKNOWN)
// evaluation record: which environment dimension was consulted, how many
// facts of that dimension were compared, and which upstream subjects were
// compared against them — enough to answer "why do you think this doesn't
// affect me?" without re-running anything.
type ImpactCheck struct {
	Dimension EnvironmentDimension `json:"dimension"`
	// Platform names the cluster platform for DimensionCluster checks
	// ("kubernetes", "openshift", ...); empty otherwise.
	Platform string   `json:"platform,omitempty"`
	Facts    int      `json:"facts"`    // environment facts compared (0 when the dimension was supplied but yielded none)
	Subjects []string `json:"subjects"` // upstream subjects compared against those facts
	// Evidence cites environment evidence proving the dimension was supplied
	// where a direct input record exists (the --kubernetes / --images flag).
	// File-backed dimensions are audited through the report's
	// environment.files section, which pins every input file by digest.
	Evidence []EvidenceID `json:"evidence,omitempty"`
}

// ImpactMatchKind names what part of the environment matched.
type ImpactMatchKind string

const (
	MatchValuesKey     ImpactMatchKind = "values-key"     // a key the user sets in a values file
	MatchAPIVersion    ImpactMatchKind = "api-version"    // an apiVersion (+kind) used by a manifest
	MatchCRD           ImpactMatchKind = "crd"            // an installed CustomResourceDefinition
	MatchCRDVersion    ImpactMatchKind = "crd-version"    // a version declared by an installed CRD
	MatchManifestField ImpactMatchKind = "manifest-field" // a field path set in a manifest
	MatchImage         ImpactMatchKind = "image"          // a container image reference in use
	MatchKubernetes    ImpactMatchKind = "kubernetes"     // the cluster Kubernetes version
)

// ImpactMatch is one environment fact that made a finding fire: what matched
// (subject) and the environment evidence that proves the environment has it.
type ImpactMatch struct {
	Kind     ImpactMatchKind `json:"kind"`
	Subject  string          `json:"subject"`
	Evidence []EvidenceID    `json:"evidence"`
}

// ImpactFinding is one deterministic conclusion of the join: an upstream
// change (or compatibility constraint / artifact move) met the environment —
// or could not be evaluated. AFFECTED findings (action-required /
// review-required / informational) always cite BOTH chains:
// UpstreamEvidence (ids resolving in the report's `evidence`) and
// EnvironmentEvidence (ids resolving in `environmentEvidence`, via Matches).
// not-affected carries Checks instead of matches; unknown carries
// NeededToDetermine. Detail explains the verdict in prose. See
// docs/ACTION_CLASSIFICATION.md for the contract.
type ImpactFinding struct {
	ID             string         `json:"id"`
	Classification ImpactClass    `json:"classification"`
	Severity       ImpactSeverity `json:"severity,omitempty"` // how bad if it bites; set only where determinable
	Rule           string         `json:"rule"`               // join rule, e.g. "impact:values-removed"
	Title          string         `json:"title"`
	Detail         string         `json:"detail,omitempty"`

	// The upstream change this finding joins (empty when the finding comes
	// from a compatibility constraint or artifact move alone). The id
	// resolves within the UpgradeEdge the report was built from; the title
	// and category are copied so the report renders standalone.
	ChangeID             string   `json:"changeId,omitempty"`
	ChangeTitle          string   `json:"changeTitle,omitempty"`
	ChangeCategory       Category `json:"changeCategory,omitempty"`
	ChangeBreaking       bool     `json:"changeBreaking,omitempty"`
	ChangeActionRequired bool     `json:"changeActionRequired,omitempty"`

	Matches             []ImpactMatch `json:"matches,omitempty"`             // environment facts that matched (affected classes only)
	UpstreamEvidence    []EvidenceID  `json:"upstreamEvidence"`              // chain 1: edge evidence (every class)
	EnvironmentEvidence []EvidenceID  `json:"environmentEvidence,omitempty"` // chain 2: local evidence (affected classes)
	Provenance          Provenance    `json:"provenance"`

	// Checks is the evaluation record of a not-affected verdict (and the
	// partial record of an unknown verdict evaluated with some visibility):
	// what was checked against what.
	Checks []ImpactCheck `json:"checks,omitempty"`
	// NeededToDetermine names the evidence missing for an unknown verdict
	// ("Helm values files (--values) not supplied"). Unknown-only.
	NeededToDetermine []string `json:"neededToDetermine,omitempty"`
}

// ImpactSummary counts the funnel: all upstream changes, how many produced
// affected findings, and the explicit count of every verdict class —
// unknowns included, never folded into "not affected".
type ImpactSummary struct {
	UpstreamChanges   int `json:"upstreamChanges"`
	AffectEnvironment int `json:"affectEnvironment"`
	ActionRequired    int `json:"actionRequired"`
	ReviewRequired    int `json:"reviewRequired"`
	Informational     int `json:"informational"`
	NotAffected       int `json:"notAffected"`
	Unknown           int `json:"unknown"`
}

// ImpactFile is one environment input file with the digest of the bytes that
// were parsed, so a finding's excerpt can be audited against the exact input.
type ImpactFile struct {
	Path   string `json:"path"`   // path exactly as supplied on the command line
	Digest string `json:"digest"` // sha256 of the file bytes
}

// ImpactEnvironment summarises what the join was run against.
type ImpactEnvironment struct {
	// Kubernetes is the cluster version as supplied ("" when not given).
	Kubernetes string `json:"kubernetes,omitempty"`
	// Files are all parsed input files with their digests.
	Files []ImpactFile `json:"files,omitempty"`
	// Counts of extracted environment facts.
	ValuesKeys    int `json:"valuesKeys"`
	ManifestDocs  int `json:"manifestDocs"`
	APIVersions   int `json:"apiVersions"`
	CRDs          int `json:"crds"`
	ManifestPaths int `json:"manifestPaths"`
	Images        int `json:"images"`
	// Warnings report degraded parsing (unparsable documents, caps hit, ...).
	Warnings []string `json:"warnings,omitempty"`
}

// ImpactReport is the result of joining an UpgradeEdge with an environment:
// which of the edge's changes matter to THAT environment, each verdict
// explained by provenance (upstream evidence + environment evidence, or the
// evaluation record, or the missing-evidence list). Built deterministically;
// never contains AI output.
type ImpactReport struct {
	SchemaVersion string            `json:"schemaVersion"`
	Product       ProductRef        `json:"product"`
	From          Version           `json:"from"`
	To            Version           `json:"to"`
	Environment   ImpactEnvironment `json:"environment"`
	Summary       ImpactSummary     `json:"summary"`
	Findings      []ImpactFinding   `json:"findings"`

	// Evidence is the upstream evidence cited by findings (a subset of the
	// edge's evidence pool). EnvironmentEvidence is the local chain: records
	// of kind local-file / input pointing at the user's files or flags.
	Evidence            []Evidence `json:"evidence"`
	EnvironmentEvidence []Evidence `json:"environmentEvidence"`

	Warnings         []string  `json:"warnings,omitempty"`
	GeneratedAt      time.Time `json:"generatedAt"`
	DefinitionDigest string    `json:"definitionDigest,omitempty"`
}

// Validate enforces the invariants of an impact report and the action
// classification contract (docs/ACTION_CLASSIFICATION.md): every finding's
// evidence resolves within the document, findings are deterministic and
// classified, and the class-specific provenance rules hold:
//
//   - affected classes (action-required / review-required / informational):
//     at least one environment match, both chains resolve;
//   - action-required: provenance confidence high (low/medium confidence is
//     demoted to review-required and never validated as ACTION REQUIRED);
//   - not-affected: no matches, at least one check (the evaluation record);
//   - unknown: no matches, at least one neededToDetermine item;
//   - the summary counts match the findings.
func (r *ImpactReport) Validate() error {
	var errs []error
	if r.SchemaVersion != ImpactReportSchemaVersion {
		errs = append(errs, fmt.Errorf("schemaVersion = %q, want %q", r.SchemaVersion, ImpactReportSchemaVersion))
	}
	up := map[EvidenceID]bool{}
	for _, x := range r.Evidence {
		if up[x.ID] {
			errs = append(errs, fmt.Errorf("duplicate evidence id %s", x.ID))
		}
		up[x.ID] = true
	}
	local := map[EvidenceID]bool{}
	for _, x := range r.EnvironmentEvidence {
		if local[x.ID] {
			errs = append(errs, fmt.Errorf("duplicate environment evidence id %s", x.ID))
		}
		local[x.ID] = true
	}
	classes := map[ImpactClass]bool{}
	for _, c := range AllImpactClasses {
		classes[c] = true
	}
	severities := map[ImpactSeverity]bool{}
	for _, s := range AllImpactSeverities {
		severities[s] = true
	}
	dimensions := map[EnvironmentDimension]bool{}
	for _, d := range AllEnvironmentDimensions {
		dimensions[d] = true
	}
	counts := map[ImpactClass]int{}
	for _, f := range r.Findings {
		f := f
		if err := f.Provenance.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("finding %s: %w", f.ID, err))
		}
		if !f.Provenance.Deterministic() {
			errs = append(errs, fmt.Errorf("finding %s: AI-derived content must be an Enrichment, not a finding", f.ID))
		}
		if !classes[f.Classification] {
			errs = append(errs, fmt.Errorf("finding %s: unknown classification %q", f.ID, f.Classification))
			continue
		}
		counts[f.Classification]++
		if f.Title == "" {
			errs = append(errs, fmt.Errorf("finding %s has no title", f.ID))
		}
		if f.Severity != "" && !severities[f.Severity] {
			errs = append(errs, fmt.Errorf("finding %s: unknown severity %q", f.ID, f.Severity))
		}
		if len(f.UpstreamEvidence) == 0 {
			errs = append(errs, fmt.Errorf("finding %s (%q) cites no upstream evidence", f.ID, f.Title))
		}
		for _, id := range f.UpstreamEvidence {
			if !up[id] {
				errs = append(errs, fmt.Errorf("finding %s references unknown upstream evidence %s", f.ID, id))
			}
		}
		if f.Classification.Affected() {
			if len(f.Matches) == 0 {
				errs = append(errs, fmt.Errorf("affected finding %s (%q) has no environment match", f.ID, f.Title))
			}
			if len(f.EnvironmentEvidence) == 0 {
				errs = append(errs, fmt.Errorf("finding %s (%q) cites no environment evidence", f.ID, f.Title))
			}
			if f.Classification == ImpactActionRequired && f.Provenance.Confidence != ConfidenceHigh {
				errs = append(errs, fmt.Errorf("finding %s: ACTION REQUIRED requires high confidence, got %q (demote to review-required)", f.ID, f.Provenance.Confidence))
			}
		} else {
			if len(f.Matches) != 0 {
				errs = append(errs, fmt.Errorf("finding %s: class %q must not carry matches", f.ID, f.Classification))
			}
		}
		switch f.Classification {
		case ImpactNotAffected:
			if len(f.Checks) == 0 {
				errs = append(errs, fmt.Errorf("not-affected finding %s (%q) has no evaluation record", f.ID, f.Title))
			}
			if len(f.NeededToDetermine) != 0 {
				errs = append(errs, fmt.Errorf("finding %s: neededToDetermine is unknown-only", f.ID))
			}
		case ImpactUnknown:
			if len(f.NeededToDetermine) == 0 {
				errs = append(errs, fmt.Errorf("unknown finding %s (%q) does not say what evidence was missing", f.ID, f.Title))
			}
		default:
			if len(f.Checks) != 0 {
				errs = append(errs, fmt.Errorf("finding %s: checks are a not-affected/unknown record, not %s", f.ID, f.Classification))
			}
			if len(f.NeededToDetermine) != 0 {
				errs = append(errs, fmt.Errorf("finding %s: neededToDetermine is unknown-only", f.ID))
			}
		}
		for _, id := range f.EnvironmentEvidence {
			if !local[id] {
				errs = append(errs, fmt.Errorf("finding %s references unknown environment evidence %s", f.ID, id))
			}
		}
		for _, m := range f.Matches {
			if len(m.Evidence) == 0 {
				errs = append(errs, fmt.Errorf("finding %s match %q has no evidence", f.ID, m.Subject))
			}
			for _, id := range m.Evidence {
				if !local[id] {
					errs = append(errs, fmt.Errorf("finding %s match %q references unknown environment evidence %s", f.ID, m.Subject, id))
				}
			}
		}
		for _, c := range f.Checks {
			if !dimensions[c.Dimension] {
				errs = append(errs, fmt.Errorf("finding %s: unknown check dimension %q", f.ID, c.Dimension))
			}
			if c.Dimension == DimensionCluster && c.Platform == "" {
				errs = append(errs, fmt.Errorf("finding %s: cluster-version check without platform", f.ID))
			}
			for _, id := range c.Evidence {
				if !local[id] {
					errs = append(errs, fmt.Errorf("finding %s check %q references unknown environment evidence %s", f.ID, c.Dimension, id))
				}
			}
		}
		if (f.ChangeID == "") != (f.ChangeTitle == "") {
			errs = append(errs, fmt.Errorf("finding %s: changeId and changeTitle must be given together", f.ID))
		}
	}
	if r.Summary.AffectEnvironment != counts[ImpactActionRequired]+counts[ImpactReviewRequired]+counts[ImpactInformational] ||
		r.Summary.ActionRequired != counts[ImpactActionRequired] ||
		r.Summary.ReviewRequired != counts[ImpactReviewRequired] ||
		r.Summary.Informational != counts[ImpactInformational] ||
		r.Summary.NotAffected != counts[ImpactNotAffected] ||
		r.Summary.Unknown != counts[ImpactUnknown] {
		errs = append(errs, fmt.Errorf("summary %+v does not match the %d findings (%+v)", r.Summary, len(r.Findings), counts))
	}
	if r.From.Compare(r.To) >= 0 {
		errs = append(errs, errors.New("report 'from' must be lower than 'to'"))
	}
	return errors.Join(errs...)
}
