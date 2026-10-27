package domain

import (
	"errors"
	"fmt"
	"strings"
)

// Phase 6 Milestone 3 (docs/phase6/decision-intel/PLAN-M3.md, mission C1–C5,
// product decisions PD-1 and PD-3): the upgrade-level judgement of one
// upgrade × environment. It combines the run's deterministic records (the
// findings, validations, the edge's security section, pinned settings, the
// deployed-version location) with one model call's committed recommendation.
//
// The deterministic parts (Findings, Target options, Locations, Backout and
// the deterministic residual risks) are computed by RI and copied verbatim;
// the model chooses among them and comments, and a deterministic floor checks
// its answer (internal/synthesize). The judgement is advisory: it is never a
// policy input and never changes findings, the decision, the review or
// knowledge (mission F1; PD-1 guardrail 4). CONTRACT-CHANGE(synthesis):
// additive (Investigation.Judgement).

// UpgradeJudgementSchemaVersion is the serialisation version of a judgement.
const UpgradeJudgementSchemaVersion = "ri.dev/upgrade-judgement/v1alpha1"

// Id prefixes of a judgement and its parts.
const (
	UpgradeJudgementIDPrefix  = "jdg-"
	JudgementFindingIDPrefix  = "jf-"
	ChangeLocationIDPrefix    = "loc-"
	ResidualRiskIDPrefix      = "rr-"
	JudgementTestIDPrefix     = "jt-"
	CrossChangeIDPrefix       = "xc-"
	JudgementProducerPrefix   = "synthesize@"
	judgementStatementMaxSize = 4000
)

// Recommendation is the upgrade-level recommendation (mission C5).
type Recommendation string

const (
	RecommendProceed            Recommendation = "proceed"
	RecommendProceedWithChanges Recommendation = "proceed-with-changes"
	RecommendModifyTarget       Recommendation = "modify-target"
	RecommendValidationRequired Recommendation = "validation-required"
	RecommendDoNotProceed       Recommendation = "do-not-proceed"
)

// Recommendations lists every recommendation.
var Recommendations = []Recommendation{RecommendProceed, RecommendProceedWithChanges, RecommendModifyTarget, RecommendValidationRequired, RecommendDoNotProceed}

// JudgementStatus says whether the model's answer passed the floor.
type JudgementStatus string

const (
	// JudgementComplete: an answer passed the deterministic floor.
	JudgementComplete JudgementStatus = "complete"
	// JudgementFailed: every answer within the retry bound violated the
	// floor. The deterministic parts are kept; there is no recommendation.
	// A failure is recorded, never repaired.
	JudgementFailed JudgementStatus = "failed"
)

// JudgementStatuses lists every status.
var JudgementStatuses = []JudgementStatus{JudgementComplete, JudgementFailed}

// JudgementFindingKind is one kind of disagreement or hard finding.
type JudgementFindingKind string

const (
	// Hard findings (PD-1 guardrail 1): approving against one is an override.
	JFSecurityRegression JudgementFindingKind = "security-regression"      // an advisory affects the target and not the current version
	JFConfirmedBreakage  JudgementFindingKind = "confirmed-breakage"       // a finding classified affected
	JFFailedValidation   JudgementFindingKind = "failed-validation"        // a deterministic or observed validation failed
	JFCandidateAffected  JudgementFindingKind = "candidate-still-affected" // a target option is affected by an assessed advisory, or no release removes the regressions
	// Disagreements and gaps.
	JFAgentDisagreement      JudgementFindingKind = "agent-disagreement"      // the agent's verdict and the classification disagree
	JFUntracedConsumer       JudgementFindingKind = "untraced-consumer"       // a consumer of an exposed object the session never traced
	JFBehaviourUnestablished JudgementFindingKind = "behaviour-unestablished" // upstream behaviour unavailable or not checked
	JFAdvisoryInconsistency  JudgementFindingKind = "advisory-inconsistency"  // an advisory contradicts itself
	JFSecurityIncomplete     JudgementFindingKind = "security-incomplete"     // the advisory picture is incomplete or unavailable
	// JFBehaviourUnsupported: an upstream behaviour the assessment relies on
	// asserts a value its cited passage lacks, or states with the opposite
	// polarity (the report-only semantic check, lane semverify).
	// CONTRACT-CHANGE(semverify): additive.
	JFBehaviourUnsupported JudgementFindingKind = "behaviour-unsupported-by-citation"
)

// JudgementFindingKinds lists every kind.
var JudgementFindingKinds = []JudgementFindingKind{JFSecurityRegression, JFConfirmedBreakage, JFFailedValidation, JFCandidateAffected,
	JFAgentDisagreement, JFUntracedConsumer, JFBehaviourUnestablished, JFAdvisoryInconsistency, JFSecurityIncomplete, JFBehaviourUnsupported}

// Hard reports whether the kind is a hard deterministic finding (PD-1).
func (k JudgementFindingKind) Hard() bool {
	switch k {
	case JFSecurityRegression, JFConfirmedBreakage, JFFailedValidation, JFCandidateAffected:
		return true
	}
	return false
}

// JudgementFinding is one disagreement or hard finding, computed by RI from
// the run's records and copied verbatim into the judgement. The model may
// comment on it (Comment, AI prose); it can never remove one.
type JudgementFinding struct {
	ID         string               `json:"id"` // JudgementFindingID(kind, changeItem, subject)
	Kind       JudgementFindingKind `json:"kind"`
	Hard       bool                 `json:"hard"`
	ChangeItem string               `json:"changeItem,omitempty"`
	// Subject names what the finding is about: an advisory id, a node key, a
	// behaviour, a version, a validation record.
	Subject   string       `json:"subject"`
	Statement string       `json:"statement"`
	Evidence  []EvidenceID `json:"evidence,omitempty"`
	// Refs are the run records the finding is computed from (finding,
	// verdict, assessment, validation, evidence request ids).
	Refs    []string `json:"refs,omitempty"`
	Comment string   `json:"comment,omitempty"`
}

// JudgementFindingID derives a finding id from its content (stable across
// runs over the same records).
func JudgementFindingID(kind JudgementFindingKind, changeItem, subject string) string {
	return JudgementFindingIDPrefix + ShortHash(string(kind), changeItem, subject)
}

// Target option roles.
const (
	TargetProposed    = "proposed"     // the upgrade's own target
	TargetMinimum     = "minimum"      // the lowest safe release (an edge candidate)
	TargetLatestPatch = "latest-patch" // the latest known patch on the chosen line
	TargetClean       = "clean"        // the edge's candidate free of every assessed advisory
)

// TargetOption is one version the judgement may recommend, computed
// deterministically (PD-3). Safe: affected by no assessed advisory the
// current version is free of.
type TargetOption struct {
	Role    string  `json:"role"`
	Version Version `json:"version"`
	Line    string  `json:"line,omitempty"`
	Safe    bool    `json:"safe"`
	// Introduces are the assessed advisories affecting the option that the
	// current version is free of; Leaves every assessed advisory affecting
	// it; Unassessed the advisories it cannot be cleared of.
	Introduces []string         `json:"introduces,omitempty"`
	Leaves     []string         `json:"leaves,omitempty"`
	Unassessed []string         `json:"unassessed,omitempty"`
	Charts     []CandidateChart `json:"charts,omitempty"`
	Statement  string           `json:"statement"`
	Evidence   []EvidenceID     `json:"evidence,omitempty"`
}

// JudgementTarget is the target-version part of the judgement: the computed
// minimum and recommendation (PD-3) and the version the judgement chose.
type JudgementTarget struct {
	Current  Version `json:"current"`
	Proposed Version `json:"proposed"`
	// Minimum is the lowest safe release; Recommended the PD-3 release:
	// the latest patch on the chosen line when it is safe and adds no
	// assessed advisory, otherwise the minimum. Nil when none is safe.
	Minimum     *Version `json:"minimum,omitempty"`
	Recommended *Version `json:"recommended,omitempty"`
	// Rule states how Recommended was chosen, in one sentence.
	Rule    string         `json:"rule"`
	Options []TargetOption `json:"options,omitempty"`
	// Chosen is the version the judgement recommends (one of Options), nil
	// when it names none (validation-required, do-not-proceed).
	Chosen *Version `json:"chosen,omitempty"`
	// Changed: Chosen is not the proposed target. Then the new current →
	// chosen transition has not been investigated by this run
	// (TransitionInvestigated false), and the judgement requires it.
	Changed                bool           `json:"changed,omitempty"`
	TransitionInvestigated bool           `json:"transitionInvestigated,omitempty"`
	SecurityStatus         SecurityStatus `json:"securityStatus,omitempty"`
}

// Change location kinds: each comes from a deterministic fact.
const (
	LocationPinnedSetting   = "pinned-setting"   // ValidationRecord.Pin (override inspection)
	LocationDeployedVersion = "deployed-version" // the GitOps line that states the installed version
	LocationSettingSet      = "setting-set"      // a verified present/value setting claim
	LocationSettingUnset    = "setting-unset"    // a verified absent setting claim
)

// ChangeLocation is where a change is made, established by a deterministic
// fact (never by the model).
type ChangeLocation struct {
	ID         string `json:"id"` // ChangeLocationID(kind, file, key, changeItem)
	Kind       string `json:"kind"`
	File       string `json:"file"`
	Line       int    `json:"line,omitempty"`
	Key        string `json:"key"`
	Value      string `json:"value,omitempty"`
	ChangeItem string `json:"changeItem,omitempty"`
	// Source is the record the location comes from (a validation record,
	// a setting claim, or the impact report's product inventory).
	Source   string       `json:"source"`
	Evidence []EvidenceID `json:"evidence,omitempty"`
}

// ChangeLocationID derives a location id.
func ChangeLocationID(kind, file, key, changeItem string) string {
	return ChangeLocationIDPrefix + ShortHash(kind, file, key, changeItem)
}

// StepKind is the kind of one step of the change sequence (PD-0b).
type StepKind string

const (
	StepPinBehaviour    StepKind = "pin-behaviour"
	StepVersionBump     StepKind = "version-bump"
	StepValidate        StepKind = "validate"
	StepAdoptNewDefault StepKind = "adopt-new-default"
	StepOther           StepKind = "other"
)

// StepKinds lists every kind.
var StepKinds = []StepKind{StepPinBehaviour, StepVersionBump, StepValidate, StepAdoptNewDefault, StepOther}

// JudgementChange is one ordered, isolated step. Location names a
// ChangeLocation; without one the step is Proposed (no file or key is
// established).
type JudgementChange struct {
	Order       int      `json:"order"`
	Kind        StepKind `json:"kind"`
	Summary     string   `json:"summary"`
	ChangeItems []string `json:"changeItems,omitempty"`
	Location    string   `json:"location,omitempty"`
	Proposed    bool     `json:"proposed,omitempty"`
}

// JudgementTest is one test required before proceeding, with its expected
// result. Basis names what it comes from: precomputed test ids (an
// assessment's recommended test, a finding's next step) or judgement
// finding ids.
type JudgementTest struct {
	ID          string   `json:"id"` // JudgementTestID(test, expected)
	Test        string   `json:"test"`
	Expected    string   `json:"expected"`
	Basis       []string `json:"basis"`
	ChangeItems []string `json:"changeItems,omitempty"`
}

// JudgementTestID derives a test id.
func JudgementTestID(test, expected string) string {
	return JudgementTestIDPrefix + ShortHash(test, expected)
}

// Residual risk sources.
const (
	RiskDeterministic = "deterministic" // an item the classification left unresolved
	RiskModel         = "model"         // named by the model (AI prose)
)

// ResidualRisk is one risk an approval accepts (PD-1).
type ResidualRisk struct {
	ID         string   `json:"id"` // ResidualRiskID(source, changeItem, statement)
	Source     string   `json:"source"`
	ChangeItem string   `json:"changeItem,omitempty"`
	Statement  string   `json:"statement"`
	Refs       []string `json:"refs,omitempty"`
}

// ResidualRiskID derives a risk id.
func ResidualRiskID(source, changeItem, statement string) string {
	return ResidualRiskIDPrefix + ShortHash(source, changeItem, statement)
}

// CrossChange flags change items that touch the same rendered object or the
// same setting (mission C4), computed deterministically.
type CrossChange struct {
	ID        string   `json:"id"`   // CrossChangeID(kind, shared)
	Kind      string   `json:"kind"` // "object" | "setting"
	Shared    string   `json:"shared"`
	Items     []string `json:"items"`
	Statement string   `json:"statement"`
	Comment   string   `json:"comment,omitempty"`
}

// CrossChangeID derives an interaction id.
func CrossChangeID(kind, shared string) string { return CrossChangeIDPrefix + ShortHash(kind, shared) }

// JudgementSummary is what is verified (cited), inferred and unknown at the
// upgrade level.
type JudgementSummary struct {
	Verified []AssessmentFinding `json:"verified,omitempty"`
	Inferred []string            `json:"inferred,omitempty"`
	Unknown  []string            `json:"unknown,omitempty"`
}

// Backout is the rollback line: the version pin to restore, where.
type Backout struct {
	Version   Version `json:"version"`
	Location  string  `json:"location,omitempty"`
	Statement string  `json:"statement"`
	Notes     string  `json:"notes,omitempty"`
}

// JudgementFailure records why no answer passed the floor.
type JudgementFailure struct {
	Attempts   int      `json:"attempts"`
	Violations []string `json:"violations"`
	// Answers are the digests of the rejected answers, in order.
	Answers []string `json:"answers"`
}

// UpgradeJudgement is the committed upgrade-level judgement (PD-1).
type UpgradeJudgement struct {
	SchemaVersion string          `json:"schemaVersion"`
	ID            string          `json:"id"` // UpgradeJudgementID(investigation, input, callId)
	Investigation string          `json:"investigation"`
	Status        JudgementStatus `json:"status"`
	Product       ProductRef      `json:"product"`
	From          Version         `json:"from"`
	To            Version         `json:"to"`
	Environment   string          `json:"environment"`

	// Recommendation, Headline, Scope and Preconditions are the model's
	// committed answer (empty when Status is failed).
	Recommendation Recommendation `json:"recommendation,omitempty"`
	Headline       string         `json:"headline,omitempty"`
	Scope          string         `json:"scope,omitempty"`
	Preconditions  []string       `json:"preconditions,omitempty"`
	Rationale      string         `json:"rationale,omitempty"`

	Target        JudgementTarget    `json:"target"`
	Changes       []JudgementChange  `json:"changes,omitempty"`
	Locations     []ChangeLocation   `json:"locations,omitempty"`
	Validation    []JudgementTest    `json:"validation,omitempty"`
	ResidualRisks []ResidualRisk     `json:"residualRisks,omitempty"`
	Findings      []JudgementFinding `json:"findings,omitempty"`
	Interactions  []CrossChange      `json:"interactions,omitempty"`
	Summary       JudgementSummary   `json:"summary"`
	Backout       Backout            `json:"backout"`

	// Evidence are the records the judgement cites that neither the bundle
	// nor the run pool holds (the edge's advisory and chart evidence).
	Evidence []Evidence `json:"evidence,omitempty"`
	// Input is the digest of the pre-computed input the model was given.
	Input   string            `json:"input"`
	Failure *JudgementFailure `json:"failure,omitempty"`

	StatementKind string     `json:"statementKind"` // StatementUnverifiedProse
	Provenance    Provenance `json:"provenance"`    // method ai, producer synthesize@vN
}

// UpgradeJudgementID derives a judgement id.
func UpgradeJudgementID(investigation, input, callID string) string {
	return UpgradeJudgementIDPrefix + ShortHash(investigation, input, callID)
}

// HardFindings returns the hard findings.
func (j *UpgradeJudgement) HardFindings() []JudgementFinding {
	var out []JudgementFinding
	for _, f := range j.Findings {
		if f.Hard {
			out = append(out, f)
		}
	}
	return out
}

// Location returns the location with the id.
func (j *UpgradeJudgement) Location(id string) (ChangeLocation, bool) {
	for _, l := range j.Locations {
		if l.ID == id {
			return l, true
		}
	}
	return ChangeLocation{}, false
}

// citedEvidence lists every evidence id the judgement cites.
func (j *UpgradeJudgement) citedEvidence() []EvidenceID {
	var out []EvidenceID
	for _, f := range j.Findings {
		out = append(out, f.Evidence...)
	}
	for _, o := range j.Target.Options {
		out = append(out, o.Evidence...)
		for _, c := range o.Charts {
			out = append(out, c.Evidence...)
		}
	}
	for _, l := range j.Locations {
		out = append(out, l.Evidence...)
	}
	for _, v := range j.Summary.Verified {
		out = append(out, v.Citations...)
	}
	return append(out, j.Provenance.InputEvidence...)
}

// Validate checks the judgement's shape: identity, the recommendation and
// status, the derived ids, the hard flags, the target choice among the
// options, change locations and the AI provenance. The floor's semantic
// rules (internal/synthesize) are checked when the answer is accepted.
func (j *UpgradeJudgement) Validate() error {
	errs := validateAI("upgrade judgement "+j.ID, j.Provenance)
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("upgrade judgement %s: %s", j.ID, fmt.Sprintf(format, args...)))
	}
	if j.SchemaVersion != UpgradeJudgementSchemaVersion {
		bad("schemaVersion = %q, want %q", j.SchemaVersion, UpgradeJudgementSchemaVersion)
	}
	if j.ID != UpgradeJudgementID(j.Investigation, j.Input, j.Provenance.CallID) {
		bad("id is not derived")
	}
	if !isDigest(j.Input) {
		bad("input must be a digest")
	}
	if !strings.HasPrefix(j.Provenance.Producer, JudgementProducerPrefix) {
		bad("producer %q is not %s<n>", j.Provenance.Producer, JudgementProducerPrefix)
	}
	if j.StatementKind != StatementUnverifiedProse {
		bad("statementKind must be %s", StatementUnverifiedProse)
	}
	switch j.Status {
	case JudgementComplete:
		if !oneOf(j.Recommendation, Recommendations) {
			bad("unknown recommendation %q", j.Recommendation)
		}
		if strings.TrimSpace(j.Headline) == "" {
			bad("a complete judgement has a headline")
		}
		if j.Failure != nil {
			bad("a complete judgement records no failure")
		}
	case JudgementFailed:
		if j.Recommendation != "" || j.Headline != "" || len(j.Changes) > 0 || j.Target.Chosen != nil {
			bad("a failed judgement carries no recommendation, headline, changes or chosen target")
		}
		if j.Failure == nil || j.Failure.Attempts < 1 || len(j.Failure.Violations) == 0 || len(j.Failure.Answers) != j.Failure.Attempts {
			bad("a failed judgement records its attempts, violations and rejected answers")
		}
	default:
		bad("unknown status %q", j.Status)
	}
	for _, f := range j.Findings {
		if !oneOf(f.Kind, JudgementFindingKinds) {
			bad("finding %s: unknown kind %q", f.ID, f.Kind)
		}
		if f.ID != JudgementFindingID(f.Kind, f.ChangeItem, f.Subject) {
			bad("finding %s: id is not derived", f.ID)
		}
		if f.Hard != f.Kind.Hard() {
			bad("finding %s: hard must be %v for kind %s", f.ID, f.Kind.Hard(), f.Kind)
		}
		if strings.TrimSpace(f.Statement) == "" || f.Subject == "" {
			bad("finding %s: subject and statement are required", f.ID)
		}
	}
	locs := map[string]bool{}
	for _, l := range j.Locations {
		if l.ID != ChangeLocationID(l.Kind, l.File, l.Key, l.ChangeItem) {
			bad("location %s: id is not derived", l.ID)
		}
		switch l.Kind {
		case LocationPinnedSetting, LocationDeployedVersion, LocationSettingSet, LocationSettingUnset:
		default:
			bad("location %s: unknown kind %q", l.ID, l.Kind)
		}
		if l.File == "" || l.Key == "" || l.Source == "" {
			bad("location %s: file, key and source are required", l.ID)
		}
		locs[l.ID] = true
	}
	for i, c := range j.Changes {
		if c.Order != i+1 {
			bad("change %d: order %d (must be 1..n)", i+1, c.Order)
		}
		if !oneOf(c.Kind, StepKinds) || strings.TrimSpace(c.Summary) == "" {
			bad("change %d: a known kind and a summary are required", c.Order)
		}
		if c.Proposed != (c.Location == "") {
			bad("change %d: a step is proposed exactly when it names no location", c.Order)
		}
		if c.Location != "" && !locs[c.Location] {
			bad("change %d: location %s is not a judgement location", c.Order, c.Location)
		}
	}
	for _, t := range j.Validation {
		if t.ID != JudgementTestID(t.Test, t.Expected) || strings.TrimSpace(t.Test) == "" || strings.TrimSpace(t.Expected) == "" || len(t.Basis) == 0 {
			bad("validation %s: a derived id, test, expected result and basis are required", t.ID)
		}
	}
	for _, r := range j.ResidualRisks {
		if r.ID != ResidualRiskID(r.Source, r.ChangeItem, r.Statement) || (r.Source != RiskDeterministic && r.Source != RiskModel) || strings.TrimSpace(r.Statement) == "" {
			bad("residual risk %s: a derived id, a known source and a statement are required", r.ID)
		}
	}
	for _, x := range j.Interactions {
		if x.ID != CrossChangeID(x.Kind, x.Shared) || len(x.Items) < 2 {
			bad("interaction %s: a derived id and at least two items are required", x.ID)
		}
	}
	for _, v := range j.Summary.Verified {
		if strings.TrimSpace(v.Statement) == "" || len(v.Citations) == 0 {
			bad("summary: a verified statement cites evidence")
		}
	}
	t := j.Target
	if t.Chosen != nil {
		found, safe := false, false
		for _, o := range t.Options {
			if o.Version.Equal(*t.Chosen) {
				found, safe = true, o.Safe
			}
		}
		switch {
		case !found:
			bad("target: chosen %s is not one of the computed options", t.Chosen)
		case !safe:
			bad("target: chosen %s is not a safe option", t.Chosen)
		}
		if t.Changed != !t.Chosen.Equal(t.Proposed) {
			bad("target: changed must say whether the chosen version is the proposed target")
		}
	} else if t.Changed {
		bad("target: changed without a chosen version")
	}
	if j.Backout.Statement == "" || j.Backout.Location != "" && !locs[j.Backout.Location] {
		bad("backout: a statement is required and its location must be a judgement location")
	}
	for _, s := range []string{j.Headline, j.Rationale, j.Scope} {
		if len(s) > judgementStatementMaxSize {
			bad("a prose field exceeds %d bytes", judgementStatementMaxSize)
		}
	}
	return errors.Join(errs...)
}
