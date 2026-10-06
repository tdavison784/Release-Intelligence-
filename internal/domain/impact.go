package domain

import (
	"errors"
	"fmt"
	"strings"
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
	// DimensionProducts: the product inventory (--inventory, or detected
	// from the environment), consulted by product-version conditions of
	// verified knowledge.
	DimensionProducts EnvironmentDimension = "products"
	// DimensionFromVersion: the version the environment runs today, as
	// supplied to `ri impact <product> <from> <to>` (edge-from-version
	// conditions of verified knowledge). Always supplied.
	// CONTRACT-CHANGE(applicability): a false edge-from-version leaf needs a
	// check, and no other dimension names the from-version input.
	DimensionFromVersion EnvironmentDimension = "from-version"
	// DimensionRender: the environment's From/To renders with the customer's
	// configuration (--render), consulted by rendered-change conditions.
	// CONTRACT-CHANGE(render).
	DimensionRender EnvironmentDimension = "render"
)

// AllEnvironmentDimensions lists every dimension in display order.
var AllEnvironmentDimensions = []EnvironmentDimension{DimensionValues, DimensionManifests, DimensionCRDs, DimensionImages, DimensionCluster, DimensionProducts, DimensionFromVersion, DimensionRender}

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
	// Render is the render-based evaluation record of a DimensionRender
	// check (PO-3): what the customer render (and its counterfactual
	// variant) established. Set exactly on render-dimension checks.
	Render *RenderCheck `json:"render,omitempty"`
}

// RenderOutcome is what a render-based evaluation established (PO-3).
type RenderOutcome string

const (
	// RenderAttributableChange: the customer From/To renders differ, and the
	// counterfactual variant (target with the key unset vs pinned to the old
	// default) attributes the difference to the key.
	RenderAttributableChange RenderOutcome = "attributable-change"
	// RenderNoAttributableChange: both renders succeeded and nothing
	// attributable to the key changed.
	RenderNoAttributableChange RenderOutcome = "no-attributable-change"
	// RenderUnavailable: no render could be produced (renderer, chart or
	// values unavailable, or a render failed); Reason says which.
	RenderUnavailable RenderOutcome = "unavailable"
)

// RenderCheck is the render evaluation behind a values-default verdict.
type RenderCheck struct {
	Outcome RenderOutcome `json:"outcome"`
	// Key is the values key whose changed or new default was evaluated.
	Key string `json:"key"`
	// Counterfactual reports that the counterfactual variant (target with
	// the key unset vs pinned to the old default) was rendered.
	Counterfactual bool `json:"counterfactual,omitempty"`
	// Reason explains an unavailable render (required then).
	Reason string `json:"reason,omitempty"`
}

// Join rules of the PO-3 values-default verdicts (docs/phase3/learning-loop/
// DECISIONS.md): a changed default (values:default-changed) or a new key
// (values:added) that the customer leaves unset is decided by rendering.
// They are domain constants because Validate enforces their shape.
const (
	// RuleValuesDefaultApplies: the render shows a change attributable to
	// the key → review-required (stronger classes only through knowledge).
	// Carries ≥1 rendered-change match backed by environment-render evidence.
	RuleValuesDefaultApplies = "impact:values-default-applies"
	// RuleValuesDefaultNoEffect: both renders succeeded and nothing
	// attributable to the key changed → not-affected, with a render check
	// (outcome no-attributable-change) citing the render evidence.
	RuleValuesDefaultNoEffect = "impact:values-default-no-effect"
	// RuleValuesDefaultUnrendered: no render was possible → not-affected (the
	// product owner's choice), with a render check (outcome unavailable,
	// reason) so the missing render is visible on the finding.
	RuleValuesDefaultUnrendered = "impact:values-default-unrendered"
)

// CONTRACT-CHANGE(renderfirst): join rules of the PO-7a render-first
// verdicts (docs/RENDER-FIRST.md). The customer's render decides exposure for
// render-expressible changes; these are the two verdicts no earlier rule
// could express.
const (
	// RuleRenderTargetRejects: the target chart refuses the customer's
	// configuration (values.schema.json, a chart-authored fail/required)
	// that the source chart rendered, and the refusal names a values key of
	// the change the customer sets. The upgrade as configured fails: the
	// consequence is established deterministically, so it is
	// action-required. Carries a values-key match (chain 2, the customer's
	// file) and a rendered-change match citing the environment render record
	// of the rejection.
	RuleRenderTargetRejects = "impact:render-target-rejects"
	// RuleValuesSetNoEffect: the customer sets a key the target removes or
	// newly reads, and rendering their configuration with and without it
	// (source and target) attributes nothing of their upgrade to it →
	// not-affected, with a no-attributable-change render check citing the
	// render evidence.
	RuleValuesSetNoEffect = "impact:values-set-no-effect"
)

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
	// CONTRACT-CHANGE(applicability): match kinds of findings evaluated from
	// verified knowledge (internal/impact condition evaluation).
	MatchProduct     ImpactMatchKind = "product"      // a product-inventory entry (product + version)
	MatchTextLine    ImpactMatchKind = "text-line"    // a line of embedded text (ConfigMap data, a multi-line string)
	MatchReference   ImpactMatchKind = "reference"    // a resolved *Ref reference between two resources
	MatchFromVersion ImpactMatchKind = "from-version" // the version the environment runs today (the edge's from)
	// MatchAbsence is an examined environment record that proves something
	// is NOT stated: a resource that leaves a field unset, a values file that
	// does not set a key, a text block none of whose lines match. Only ever
	// produced from a supplied, healthy dimension.
	MatchAbsence ImpactMatchKind = "absence"
	// MatchRenderedChange: a difference between the environment's From and
	// To renders (customer configuration). CONTRACT-CHANGE(render).
	MatchRenderedChange ImpactMatchKind = "rendered-change"
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

	// SuggestedClassification is the AI layer's suggestion for this finding
	// ("review-required"), recorded when an accepted plausibly-applies
	// enrichment relates to it. The deterministic classification is never
	// overwritten: the finding stays exactly as the join produced it, and
	// the suggestion is attached with full AI provenance on the enrichment.
	// Unknown-only, and review-required only (the AI never suggests
	// action-required). Validate enforces the 1:1 with those enrichments.
	SuggestedClassification ImpactClass `json:"suggestedClassification,omitempty"`

	// UnknownReason says why an unknown finding is unknown (MISSION G18;
	// docs/phase3/learning-loop/DESIGN.md §1.5). Unknown-only.
	UnknownReason UnknownReason `json:"unknownReason,omitempty"`
	// Knowledge is set exactly on findings produced by evaluating a
	// VerifiedFact against the environment (rules impact:knowledge-*): which
	// fact, and how trusted it is. The trust ladder is enforced by Validate:
	// not-affected requires a trusted (deterministic or human) fact;
	// action-required a trusted fact or a consensus-action fact (PO-2,
	// labelled "model consensus"); a proxy-verified fact yields neither.
	Knowledge *KnowledgeRef `json:"knowledge,omitempty"`
	// RefinedFrom is set exactly on a finding (rule impact:knowledge-refined)
	// that refines a deterministic join finding of the same change with a
	// TRUSTED fact whose subject covers the finding's subject (PO-4): the
	// original class and rule stay visible. The refined finding keeps the
	// original matches and both evidence chains and replaces the original.
	RefinedFrom *Refinement `json:"refinedFrom,omitempty"`
}

// Refinement records the deterministic finding a knowledge finding refined.
type Refinement struct {
	Classification ImpactClass    `json:"classification"`
	Rule           string         `json:"rule"`
	Severity       ImpactSeverity `json:"severity,omitempty"`
}

// RuleKnowledgeRefined is the rule of a refined finding (PO-4).
const RuleKnowledgeRefined = KnowledgeRulePrefix + "refined"

// KnowledgeRulePrefix starts the rule of every finding produced from
// verified knowledge (impact:knowledge-exposed / -overlap / -clear /
// -undecided).
const KnowledgeRulePrefix = "impact:knowledge-"

// KnowledgeRef links a finding to the verified fact it was evaluated from.
type KnowledgeRef struct {
	Fact string `json:"fact"` // VerifiedFact id (vf-…)
	// Verification is the fact's level (its weakest aspect): deterministic,
	// human, consensus or proxy.
	Verification VerificationLevel `json:"verification"`
	// Consensus labels a consensus-level fact cross-model or same-model.
	Consensus ConsensusScope `json:"consensus,omitempty"`
	// ConsensusAction is copied from the fact: it may yield ACTION REQUIRED
	// through model consensus (PO-2).
	ConsensusAction bool `json:"consensusAction,omitempty"`
	// Audited is copied from the fact (AuditedBy set): a human accepted the
	// fact's audit item (PO-6). Only on consensus-action findings.
	Audited   bool   `json:"audited,omitempty"`
	Statement string `json:"statement,omitempty"`
	// Subject is the fact's subject; required on refined findings, whose
	// matches it must cover (Subject.CoversMatch).
	Subject *Subject `json:"subject,omitempty"`
}

// ActionLabel is how an ACTION REQUIRED finding from this fact is labelled
// for humans: "verified" (deterministic/human) or "model consensus" (PO-2).
//
// PO-6: until a human accepts the fact's audit item the consensus label
// reads "model consensus · unaudited".
func (k KnowledgeRef) ActionLabel() string {
	if k.Verification == VerifiedConsensus {
		if !k.Audited {
			return "model consensus · unaudited"
		}
		return "model consensus"
	}
	return "verified"
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
	// SuggestedReview counts the unknown findings that carry an AI
	// review-required suggestion (suggestedClassification). They still count
	// in Unknown: the deterministic verdict is unchanged. 0 without -enrich.
	SuggestedReview int `json:"suggestedReview,omitempty"`
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
	// Products is the product inventory (which products run at which
	// versions); omitted when the products dimension is absent. Entries of one
	// product that state different versions are all kept, each naming the
	// others in Conflict. Evidence ids resolve in ImpactReport.EnvironmentEvidence.
	Products []ImpactProduct `json:"products,omitempty"`
	// ProductsHealth is the state of the products dimension: "absent" (no
	// inventory supplied or detected — NOT "no other products"), "ok" or
	// "partial". Omitted when absent.
	ProductsHealth string `json:"productsHealth,omitempty"`
	// Warnings report degraded parsing (unparsable documents, caps hit, ...).
	Warnings []string `json:"warnings,omitempty"`
}

// ImpactProduct is one product-inventory entry of the environment.
type ImpactProduct struct {
	Product    string       `json:"product"`
	Catalog    bool         `json:"catalog,omitempty"` // Product is a products/<id>.yaml id
	Version    string       `json:"version,omitempty"` // normalized semver; "" when unparsable or unstated
	RawVersion string       `json:"rawVersion,omitempty"`
	VersionOf  string       `json:"versionOf,omitempty"` // "app" | "chart"
	Source     string       `json:"source"`              // declared | detected-helm | detected-argo | detected-flux | detected-image | ...
	Note       string       `json:"note,omitempty"`
	Conflict   string       `json:"conflict,omitempty"` // other entries of this product that state a different version
	Evidence   []EvidenceID `json:"evidence,omitempty"`
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

	// Enrichments are AI-derived additions produced by the optional impact
	// enrichment step (`ri impact … -enrich`, internal/impactenrich). Like
	// the edge's enrichments they are kept apart from the deterministic
	// findings: they refer to findings (RelatesTo), cite evidence
	// (Citations ⊆ Provenance.InputEvidence) and carry complete AI
	// provenance. They never replace, edit or reclassify a finding — a
	// review suggestion is recorded on the finding as
	// SuggestedClassification, with the finding itself untouched.
	// Absent without -enrich.
	Enrichments []Enrichment `json:"enrichments,omitempty"`
	// EnrichmentRun records how the enrichments were produced (prompts,
	// acceptances, rejections). Present when enrichment was attempted.
	EnrichmentRun *EnrichmentRun `json:"enrichmentRun,omitempty"`
}

// Validate enforces the invariants of an impact report and the action
// classification contract (docs/ACTION_CLASSIFICATION.md): every finding's
// evidence resolves within the document, findings are deterministic and
// classified, and the class-specific provenance rules hold:
//
//   - affected classes (action-required / review-required / informational):
//     at least one environment match, both chains resolve — except the
//     impact:security-fix rule, whose informational findings apply to every
//     environment that upgrades and therefore cite the upstream chain only
//     (no matches, no environment evidence, no checks);
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
	suggested := map[string]bool{} // finding ids carrying an AI review suggestion
	for _, f := range r.Findings {
		f := f
		if err := f.Provenance.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("finding %s: %w", f.ID, err))
		}
		if !f.Provenance.Deterministic() {
			errs = append(errs, fmt.Errorf("finding %s: AI-derived content must be an Enrichment, not a finding", f.ID))
		}
		if f.SuggestedClassification != "" {
			switch {
			case f.Classification != ImpactUnknown:
				errs = append(errs, fmt.Errorf("finding %s: a suggested classification requires class unknown, got %q", f.ID, f.Classification))
			case f.SuggestedClassification != ImpactReviewRequired:
				errs = append(errs, fmt.Errorf("finding %s: the AI may only suggest %q, got %q (action-required is never suggested)", f.ID, ImpactReviewRequired, f.SuggestedClassification))
			default:
				suggested[f.ID] = true
			}
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
			// The impact:security-fix rule (defined in internal/impact; the
			// wire value is pinned by its constant) is the one affected shape
			// that consults no environment dimension: a security remediation
			// that ships with the target applies to every environment that
			// upgrades, so it carries the upstream chain only — and it must
			// carry no matches (claiming a configuration-specific match would
			// contradict its universal applicability).
			securityFix := f.Rule == "impact:security-fix"
			if securityFix && f.Classification != ImpactInformational {
				errs = append(errs, fmt.Errorf("finding %s: rule impact:security-fix is informational-only, got %q", f.ID, f.Classification))
			}
			if !securityFix && len(f.Matches) == 0 {
				errs = append(errs, fmt.Errorf("affected finding %s (%q) has no environment match", f.ID, f.Title))
			}
			if securityFix && len(f.Matches) != 0 {
				errs = append(errs, fmt.Errorf("security-fix finding %s claims environment matches; its applicability is universal and configuration-independent", f.ID))
			}
			if f.Classification == ImpactActionRequired && f.Provenance.Confidence != ConfidenceHigh {
				errs = append(errs, fmt.Errorf("finding %s: ACTION REQUIRED requires high confidence, got %q (demote to review-required)", f.ID, f.Provenance.Confidence))
			}
			if !securityFix && len(f.EnvironmentEvidence) == 0 {
				errs = append(errs, fmt.Errorf("finding %s (%q) cites no environment evidence", f.ID, f.Title))
			}
			if securityFix && len(f.EnvironmentEvidence) != 0 {
				errs = append(errs, fmt.Errorf("security-fix finding %s cites environment evidence; it consults no environment dimension", f.ID))
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
			if (c.Dimension == DimensionRender) != (c.Render != nil) {
				errs = append(errs, fmt.Errorf("finding %s: a render record is carried exactly by render-dimension checks", f.ID))
			}
		}
		errs = append(errs, f.validateKnowledge()...)
		if (f.ChangeID == "") != (f.ChangeTitle == "") {
			errs = append(errs, fmt.Errorf("finding %s: changeId and changeTitle must be given together", f.ID))
		}
	}
	errs = append(errs, r.validateKnowledgeSupersession()...)
	errs = append(errs, r.validateValuesDefaults()...)
	errs = append(errs, r.validateRefinements()...)
	if r.Summary.AffectEnvironment != counts[ImpactActionRequired]+counts[ImpactReviewRequired]+counts[ImpactInformational] ||
		r.Summary.ActionRequired != counts[ImpactActionRequired] ||
		r.Summary.ReviewRequired != counts[ImpactReviewRequired] ||
		r.Summary.Informational != counts[ImpactInformational] ||
		r.Summary.NotAffected != counts[ImpactNotAffected] ||
		r.Summary.Unknown != counts[ImpactUnknown] {
		errs = append(errs, fmt.Errorf("summary %+v does not match the %d findings (%+v)", r.Summary, len(r.Findings), counts))
	}
	if r.Summary.SuggestedReview != len(suggested) {
		errs = append(errs, fmt.Errorf("summary.suggestedReview = %d, want %d (the findings with a review suggestion)", r.Summary.SuggestedReview, len(suggested)))
	}
	errs = append(errs, r.validateEnrichments(up, local, suggested)...)
	if r.From.Compare(r.To) >= 0 {
		errs = append(errs, errors.New("report 'from' must be lower than 'to'"))
	}
	return errors.Join(errs...)
}

// validateKnowledge enforces the unknown-reason and trust-ladder rules of one
// finding (docs/phase3/learning-loop/DESIGN.md §4).
func (f ImpactFinding) validateKnowledge() []error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("finding %s: %s", f.ID, fmt.Sprintf(format, args...)))
	}
	if f.UnknownReason != "" {
		if !f.UnknownReason.Valid() {
			bad("unknown unknownReason %q", f.UnknownReason)
		}
		if f.Classification != ImpactUnknown {
			bad("unknownReason is unknown-only, class is %q", f.Classification)
		}
	}
	// CONTRACT-CHANGE(applicability): mandatory now that the join assigns a
	// reason to every unknown verdict (DESIGN.md §1.5).
	if f.Classification == ImpactUnknown && f.UnknownReason == "" {
		bad("an unknown finding must say why it is unknown (unknownReason)")
	}
	knowledgeRule := strings.HasPrefix(f.Rule, KnowledgeRulePrefix)
	if knowledgeRule != (f.Knowledge != nil) {
		bad("a knowledge reference is carried exactly by %s* rules", KnowledgeRulePrefix)
	}
	k := f.Knowledge
	if k == nil {
		return errs
	}
	// CONTRACT-CHANGE(applicability): each knowledge rule carries exactly its
	// class(es) (DESIGN.md §4): exposed → affected, overlap → informational,
	// clear → not-affected, undecided → unknown.
	ruleClasses := map[string][]ImpactClass{
		KnowledgeRulePrefix + "exposed":   {ImpactActionRequired, ImpactReviewRequired, ImpactInformational},
		KnowledgeRulePrefix + "overlap":   {ImpactInformational},
		KnowledgeRulePrefix + "clear":     {ImpactNotAffected},
		KnowledgeRulePrefix + "undecided": {ImpactUnknown},
		// PO-4: a refinement stays affected (validateRefinements has the rest).
		RuleKnowledgeRefined: {ImpactActionRequired, ImpactReviewRequired, ImpactInformational},
	}
	if want, ok := ruleClasses[f.Rule]; !ok {
		bad("unknown knowledge rule %q", f.Rule)
	} else {
		fits := false
		for _, c := range want {
			fits = fits || c == f.Classification
		}
		if !fits {
			bad("rule %s cannot carry class %q", f.Rule, f.Classification)
		}
	}
	if !strings.HasPrefix(k.Fact, FactIDPrefix) {
		bad("knowledge fact %q lacks the %s prefix", k.Fact, FactIDPrefix)
	}
	if !k.Verification.Valid() {
		bad("unknown knowledge verification %q", k.Verification)
	}
	if f.ChangeID == "" {
		bad("a knowledge finding joins an upstream change (changeId required)")
	}
	if (k.Verification == VerifiedConsensus) != (k.Consensus != "") {
		bad("knowledge.consensus (cross-model | same-model) is set exactly for consensus-verified facts")
	}
	if k.Consensus != "" && k.Consensus != ConsensusCrossModel && k.Consensus != ConsensusSameModel {
		bad("unknown knowledge.consensus %q", k.Consensus)
	}
	if k.ConsensusAction && k.Verification != VerifiedConsensus {
		bad("knowledge.consensusAction is for consensus-verified facts only")
	}
	if k.Audited && !k.ConsensusAction {
		bad("knowledge.audited marks a human-audited consensus-action fact (PO-6)")
	}
	consensusAction := k.Verification == VerifiedConsensus && k.ConsensusAction
	switch f.Classification {
	case ImpactActionRequired:
		if !k.Verification.Trusted() && !consensusAction {
			bad("ACTION REQUIRED from knowledge requires a deterministic- or human-verified fact, or a consensus-action fact (PO-2); got %q (cap at review-required)", k.Verification)
		}
	case ImpactNotAffected:
		if !k.Verification.Trusted() {
			bad("NOT AFFECTED from knowledge requires a deterministic- or human-verified fact, got %q (consensus and proxy never clear)", k.Verification)
		}
	}
	if k.Verification.Valid() && !k.Verification.Trusted() && f.Provenance.Confidence == ConfidenceHigh &&
		!(consensusAction && f.Classification == ImpactActionRequired) {
		bad("a %s-verified fact cannot carry high confidence (only a consensus-action ACTION REQUIRED may, labelled model consensus)", k.Verification)
	}
	return errs
}

// validateValuesDefaults enforces the PO-3 shapes: default-applies is
// review-required with a rendered-change match backed by an environment
// render; default-no-effect is not-affected with a no-attributable-change
// render check citing render evidence; default-unrendered is not-affected
// with an unavailable render check that says why.
func (r *ImpactReport) validateValuesDefaults() []error {
	local := map[EvidenceID]Evidence{}
	for _, e := range r.EnvironmentEvidence {
		local[e.ID] = e
	}
	rendered := func(ids []EvidenceID) bool {
		if len(ids) == 0 {
			return false
		}
		for _, id := range ids {
			e, ok := local[id]
			if !ok || e.Render == nil || e.Render.Scope != RenderEnvironment {
				return false
			}
		}
		return true
	}
	var errs []error
	for _, f := range r.Findings {
		bad := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("finding %s: %s", f.ID, fmt.Sprintf(format, args...)))
		}
		for _, c := range f.Checks {
			if c.Render == nil {
				continue
			}
			switch c.Render.Outcome {
			case RenderAttributableChange, RenderNoAttributableChange:
				if !rendered(c.Evidence) {
					bad("a %s render check must cite environment-render evidence", c.Render.Outcome)
				}
			case RenderUnavailable:
				if strings.TrimSpace(c.Render.Reason) == "" {
					bad("an unavailable render check must say why")
				}
			default:
				bad("unknown render outcome %q", c.Render.Outcome)
			}
			if strings.TrimSpace(c.Render.Key) == "" {
				bad("a render check names the values key it evaluated")
			}
		}
		renderCheck := func(want RenderOutcome) bool {
			for _, c := range f.Checks {
				if c.Render != nil && c.Render.Outcome == want {
					return true
				}
			}
			return false
		}
		switch f.Rule {
		case RuleValuesDefaultApplies:
			if f.Classification != ImpactReviewRequired {
				bad("%s is review-required (stronger classes come only from knowledge), got %q", f.Rule, f.Classification)
			}
			ok := false
			for _, m := range f.Matches {
				ok = ok || (m.Kind == MatchRenderedChange && rendered(m.Evidence))
			}
			if !ok {
				bad("%s needs a rendered-change match backed by environment-render evidence", f.Rule)
			}
		case RuleRenderTargetRejects:
			if f.Classification != ImpactActionRequired {
				bad("%s is action-required, got %q", f.Rule, f.Classification)
			}
			ok, keyed := false, false
			for _, m := range f.Matches {
				ok = ok || (m.Kind == MatchRenderedChange && rendered(m.Evidence))
				keyed = keyed || m.Kind == MatchValuesKey
			}
			if !ok || !keyed {
				bad("%s needs a values-key match and a rendered-change match backed by environment-render evidence", f.Rule)
			}
		case RuleValuesDefaultNoEffect, RuleValuesSetNoEffect:
			if f.Classification != ImpactNotAffected || !renderCheck(RenderNoAttributableChange) {
				bad("%s is not-affected with a no-attributable-change render check", f.Rule)
			}
		case RuleValuesDefaultUnrendered:
			if f.Classification != ImpactNotAffected || !renderCheck(RenderUnavailable) {
				bad("%s is not-affected with a visible unavailable render check", f.Rule)
			}
		}
	}
	return errs
}

// validateRefinements enforces PO-4: a refined finding comes from a TRUSTED
// fact (never consensus or proxy), stays affected (never not-affected or
// unknown), refines an affected deterministic finding of a different class,
// every match lies within the fact's subject, and the original finding is
// gone (the refinement replaces it).
func (r *ImpactReport) validateRefinements() []error {
	var errs []error
	for _, f := range r.Findings {
		bad := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("finding %s: %s", f.ID, fmt.Sprintf(format, args...)))
		}
		if (f.RefinedFrom != nil) != (f.Rule == RuleKnowledgeRefined) {
			bad("refinedFrom is carried exactly by rule %s", RuleKnowledgeRefined)
		}
		rf := f.RefinedFrom
		if rf == nil {
			continue
		}
		k := f.Knowledge
		if k == nil || !k.Verification.Trusted() {
			bad("only a trusted (deterministic or human) fact may refine a deterministic finding")
		}
		if !f.Classification.Affected() {
			bad("a refinement never yields %q (only action-required, review-required or informational)", f.Classification)
		}
		if !rf.Classification.Affected() {
			bad("only an affected deterministic finding can be refined, not %q", rf.Classification)
		}
		if rf.Classification == f.Classification {
			bad("a refinement changes the class")
		}
		if rf.Rule == "" || strings.HasPrefix(rf.Rule, KnowledgeRulePrefix) {
			bad("refinedFrom.rule must name the deterministic join rule, got %q", rf.Rule)
		}
		if k != nil {
			if k.Subject == nil {
				bad("a refining fact states its subject")
			} else {
				for _, m := range f.Matches {
					if !k.Subject.CoversMatch(m) {
						bad("match %s %q lies outside the refining fact's subject %s", m.Kind, m.Subject, k.Subject.Key())
					}
				}
			}
		}
		for _, o := range r.Findings {
			if o.ID != f.ID && o.ChangeID == f.ChangeID && o.Rule == rf.Rule {
				bad("the refined %s finding %s of change %s must be replaced, not kept", rf.Rule, o.ID, f.ChangeID)
			}
		}
	}
	return errs
}

// validateKnowledgeSupersession: a knowledge finding replaces the unknown
// records of its change — a change with a knowledge finding carries no other
// unknown finding (no double counting).
func (r *ImpactReport) validateKnowledgeSupersession() []error {
	known := map[string]bool{}
	for _, f := range r.Findings {
		if f.Knowledge != nil && f.ChangeID != "" {
			known[f.ChangeID] = true
		}
	}
	var errs []error
	for _, f := range r.Findings {
		if known[f.ChangeID] && f.Knowledge == nil && f.Classification == ImpactUnknown {
			errs = append(errs, fmt.Errorf("finding %s: change %s has a knowledge finding, which supersedes this unknown record", f.ID, f.ChangeID))
		}
	}
	return errs
}

// validateReportEnrichmentKinds are the enrichment kinds an ImpactReport may
// carry: the shared group/summarise kinds plus the impact-only applicability
// kinds. The edge-only kinds are rejected.
var validateReportEnrichmentKinds = func() map[EnrichmentKind]bool {
	m := map[EnrichmentKind]bool{EnrichmentCluster: true, EnrichmentMigrationSummary: true}
	for _, k := range ImpactOnlyKinds {
		m[k] = true
	}
	return m
}()

// validateEnrichments checks the report's AI enrichments against its findings
// and evidence pools:
//   - ids carry EnrichmentIDPrefix, are unique and never equal a finding id;
//   - AI provenance is complete (see Provenance.Validate);
//   - the kind is one a report may carry (edge-only kinds are rejected);
//   - every related finding id exists (≥1, ≥2 for clusters; the applicability
//     kinds relate to exactly one unknown finding);
//   - citations ⊆ provenance.inputEvidence ⊆ the report's evidence pools;
//   - nothing is marked unverified (that is the edge-only "related" kind);
//   - every plausibly-applies enrichment backs exactly one suggestion, and
//     every suggestion is backed by one (SuggestedClassification);
//   - a finding belongs to at most one cluster;
//   - the run metadata, when present, matches the enrichments.
func (r *ImpactReport) validateEnrichments(up, local map[EvidenceID]bool, suggested map[string]bool) []error {
	if len(r.Enrichments) == 0 && r.EnrichmentRun == nil {
		return nil
	}
	var errs []error
	ev := map[EvidenceID]bool{}
	for _, e := range r.Evidence {
		ev[e.ID] = true
	}
	for _, e := range r.EnvironmentEvidence {
		ev[e.ID] = true
	}
	findings := map[string]ImpactFinding{}
	for _, f := range r.Findings {
		findings[f.ID] = f
	}
	ids := map[string]bool{}
	inCluster := map[string]string{}
	backed := map[string]string{} // finding id → the plausibly-applies enrichment backing its suggestion
	for _, en := range r.Enrichments {
		en := en
		bad := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("enrichment %s: %s", en.ID, fmt.Sprintf(format, args...)))
		}
		switch {
		case !strings.HasPrefix(en.ID, EnrichmentIDPrefix):
			bad("id must start with %q", EnrichmentIDPrefix)
		case ids[en.ID]:
			bad("duplicate id")
		}
		ids[en.ID] = true
		if _, clash := findings[en.ID]; clash {
			bad("id collides with a finding id")
		}
		if en.Provenance.Method != MethodAI {
			bad("must have ai provenance")
		}
		if err := en.Provenance.Validate(); err != nil {
			bad("%v", err)
		}
		switch {
		case !en.Kind.Valid():
			bad("unknown kind %q", en.Kind)
		case !validateReportEnrichmentKinds[en.Kind]:
			bad("kind %q is an upgrade-edge enrichment kind; an impact-report enrichment never carries it", en.Kind)
		}
		if strings.TrimSpace(en.Content) == "" {
			bad("empty content")
		}
		if en.Unverified {
			bad("unverified is reserved for the edge-only %q kind", EnrichmentRelated)
		}
		if n := en.Kind.MinChanges(); len(en.RelatesTo) < n {
			bad("kind %s must relate to at least %d finding(s), has %d", en.Kind, n, len(en.RelatesTo))
		}
		seen := map[string]bool{}
		for _, id := range en.RelatesTo {
			if seen[id] {
				bad("relates to finding %s twice", id)
				continue
			}
			seen[id] = true
			f, ok := findings[id]
			if !ok {
				bad("references unknown finding %s", id)
				continue
			}
			switch en.Kind {
			case EnrichmentCluster:
				if other, dup := inCluster[id]; dup && other != en.ID {
					bad("finding %s is already consolidated by cluster %s", id, other)
				}
				inCluster[id] = en.ID
			case EnrichmentPlausiblyApplies:
				if len(en.RelatesTo) != 1 {
					bad("a plausibly-applies suggestion is about exactly one finding, lists %d", len(en.RelatesTo))
					continue
				}
				if f.Classification != ImpactUnknown {
					bad("a plausibly-applies suggestion requires an unknown finding, %s is %q", id, f.Classification)
				} else if prev, dup := backed[id]; dup && prev != en.ID {
					errs = append(errs, fmt.Errorf("finding %s carries a suggestion backed by two enrichments (%s, %s)", id, prev, en.ID))
				}
				backed[id] = en.ID
			case EnrichmentNotApplicable, EnrichmentUndetermined:
				if len(en.RelatesTo) != 1 {
					bad("a %s note is about exactly one finding, lists %d", en.Kind, len(en.RelatesTo))
				}
			}
		}
		input := map[EvidenceID]bool{}
		for _, id := range en.Provenance.InputEvidence {
			input[id] = true
			if !up[id] && !local[id] {
				bad("input evidence %s is not in the report", id)
			}
		}
		if len(en.Citations) == 0 {
			bad("cites no evidence")
		}
		cited := map[EvidenceID]bool{}
		for _, id := range en.Citations {
			if cited[id] {
				bad("cites %s twice", id)
			}
			cited[id] = true
			if !input[id] {
				bad("cites %s, which was not part of its input evidence", id)
			}
		}
	}
	// every suggestion must be backed by exactly one plausibly-applies
	// enrichment of the same report
	for id := range suggested {
		if _, ok := backed[id]; !ok {
			errs = append(errs, fmt.Errorf("finding %s carries a suggested classification but no plausibly-applies enrichment backs it", id))
		}
	}
	if run := r.EnrichmentRun; run != nil {
		clusters, clustered := ClusterMetrics(r.Enrichments)
		if run.Accepted != len(r.Enrichments) || run.Clusters != clusters || run.ClusteredChanges != clustered ||
			run.DuplicatesConsolidated != clustered-clusters {
			errs = append(errs, fmt.Errorf("enrichment run metadata does not match the enrichments (accepted %d/%d, clusters %d/%d, clustered findings %d/%d, duplicates %d/%d)",
				run.Accepted, len(r.Enrichments), run.Clusters, clusters, run.ClusteredChanges, clustered, run.DuplicatesConsolidated, clustered-clusters))
		}
		if run.Producer == "" || run.PromptVersion == "" {
			errs = append(errs, errors.New("enrichment run requires producer and promptVersion"))
		}
	}
	return errs
}
