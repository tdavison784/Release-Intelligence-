package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
)

// This file is the semantic-knowledge contract of the learning loop
// (docs/phase3/learning-loop/DESIGN.md): the typed vocabulary that turns an
// upstream change into machine-comparable knowledge, and the entities that
// carry it from model proposal to verified, release-level fact.
//
//	SemanticCandidate → SemanticProposal (one per model × task, AI)
//	                  → ValidationResult (deterministic validators)
//	                  → ReviewItem → ReviewDecision (human | proxy)
//	                  → VerifiedFact (per-aspect verification level)
//
// A model output never becomes a classification: impact.Build consumes
// VerifiedFacts only, and ImpactReport.Validate enforces the trust ladder on
// the findings they produce (see KnowledgeRef).

// KnowledgeSchemaVersion is the serialisation version of KnowledgeRecord files.
const KnowledgeSchemaVersion = "ri.dev/knowledge/v1alpha1"

// ID prefixes of the knowledge entities. Every id is content-derived
// (ShortHash) so writes are idempotent and re-runs stable.
const (
	CandidateIDPrefix  = "sc-"
	ProposalIDPrefix   = "sp-"
	ValidationIDPrefix = "val-"
	ReviewItemIDPrefix = "ri-"
	DecisionIDPrefix   = "rd-"
	FactIDPrefix       = "vf-"
)

// --- subject ------------------------------------------------------------------

// SubjectFamily is the generic kind of thing a change is about (MISSION G1).
type SubjectFamily string

const (
	SubjectCRDField              SubjectFamily = "crd-field"
	SubjectHelmValue             SubjectFamily = "helm-value"
	SubjectGVK                   SubjectFamily = "gvk"
	SubjectFeatureGate           SubjectFamily = "feature-gate"
	SubjectImage                 SubjectFamily = "image"
	SubjectConfigKey             SubjectFamily = "config-key"
	SubjectRBACPermission        SubjectFamily = "rbac-permission"
	SubjectCLIFlag               SubjectFamily = "cli-flag"
	SubjectEnvVar                SubjectFamily = "env-var"
	SubjectCompatibilityBoundary SubjectFamily = "compatibility-boundary"
	SubjectAPIEndpoint           SubjectFamily = "api-endpoint"
	SubjectProtocolBehavior      SubjectFamily = "protocol-behavior"
	SubjectTerraformAttribute    SubjectFamily = "terraform-attribute"
	SubjectProductRelationship   SubjectFamily = "product-relationship"
	SubjectMigration             SubjectFamily = "migration"
)

// SubjectFamilies lists every family.
var SubjectFamilies = []SubjectFamily{
	SubjectCRDField, SubjectHelmValue, SubjectGVK, SubjectFeatureGate, SubjectImage, SubjectConfigKey,
	SubjectRBACPermission, SubjectCLIFlag, SubjectEnvVar, SubjectCompatibilityBoundary, SubjectAPIEndpoint,
	SubjectProtocolBehavior, SubjectTerraformAttribute, SubjectProductRelationship, SubjectMigration,
}

// subjectFields declares, per family, which identity fields are required and
// which are allowed besides them. Every other identity field must be empty, so
// one subject has exactly one spelling (agreement is measured on Key()).
var subjectFields = map[SubjectFamily]struct{ required, optional []string }{
	SubjectCRDField:              {[]string{"group", "kind", "path"}, []string{"version"}},
	SubjectHelmValue:             {[]string{"path"}, []string{"name"}},
	SubjectGVK:                   {[]string{"version", "kind"}, []string{"group"}},
	SubjectFeatureGate:           {[]string{"name"}, []string{"component"}},
	SubjectImage:                 {[]string{"name"}, nil},
	SubjectConfigKey:             {[]string{"path"}, []string{"component"}},
	SubjectRBACPermission:        {[]string{"name"}, []string{"group", "component"}},
	SubjectCLIFlag:               {[]string{"name"}, []string{"component"}},
	SubjectEnvVar:                {[]string{"name"}, []string{"component"}},
	SubjectCompatibilityBoundary: {[]string{"name"}, nil},
	SubjectAPIEndpoint:           {[]string{"name"}, []string{"component"}},
	SubjectProtocolBehavior:      {[]string{"name"}, []string{"component"}},
	SubjectTerraformAttribute:    {[]string{"kind", "path"}, nil},
	SubjectProductRelationship:   {[]string{"name"}, nil},
	SubjectMigration:             {[]string{"name"}, []string{"component"}},
}

// Subject identifies what changed, generically. Which fields identify a
// subject depends on its Family (see DESIGN.md §1.1).
type Subject struct {
	Family SubjectFamily `json:"family"`
	// Product is the catalog id owning the subject (for product-relationship
	// the fact's own product; Name names the related product).
	Product   ProductID `json:"product"`
	Group     string    `json:"group,omitempty"`
	Version   string    `json:"version,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	Path      string    `json:"path,omitempty"`
	Name      string    `json:"name,omitempty"`
	Component string    `json:"component,omitempty"`
}

func (s Subject) field(name string) string {
	switch name {
	case "group":
		return s.Group
	case "version":
		return s.Version
	case "kind":
		return s.Kind
	case "path":
		return s.Path
	case "name":
		return s.Name
	case "component":
		return s.Component
	}
	panic("domain: unknown subject field " + name)
}

var subjectFieldNames = []string{"group", "version", "kind", "path", "name", "component"}

// Key is the canonical, comparable identity of the subject, e.g.
// "crd-field:cert-manager|group=cert-manager.io|kind=Certificate|path=spec.privateKey.rotationPolicy".
func (s Subject) Key() string {
	var b strings.Builder
	b.WriteString(string(s.Family))
	b.WriteString(":")
	b.WriteString(string(s.Product))
	for _, n := range subjectFieldNames {
		if v := s.field(n); v != "" {
			b.WriteString("|" + n + "=" + v)
		}
	}
	return b.String()
}

// Validate checks the family and that exactly the family's identity fields are set.
func (s Subject) Validate() error {
	var errs []error
	spec, ok := subjectFields[s.Family]
	if !ok {
		return fmt.Errorf("subject: unknown family %q", s.Family)
	}
	if s.Product == "" {
		errs = append(errs, errors.New("subject: product is required"))
	}
	allowed := map[string]bool{}
	for _, n := range spec.required {
		allowed[n] = true
		if strings.TrimSpace(s.field(n)) == "" {
			errs = append(errs, fmt.Errorf("subject %s: %s is required", s.Family, n))
		}
	}
	for _, n := range spec.optional {
		allowed[n] = true
	}
	for _, n := range subjectFieldNames {
		if !allowed[n] && s.field(n) != "" {
			errs = append(errs, fmt.Errorf("subject %s: %s is not part of this family's identity", s.Family, n))
		}
	}
	return errors.Join(errs...)
}

// --- change -------------------------------------------------------------------

// ChangeKind is how a subject changed. (ChangeType is the artifact-delta
// vocabulary of UpgradeEdge and is unrelated.)
type ChangeKind string

const (
	ChangeKindAdded               ChangeKind = "added"
	ChangeKindRemoved             ChangeKind = "removed"
	ChangeKindRenamed             ChangeKind = "renamed"
	ChangeKindDefaultChanged      ChangeKind = "default-changed"
	ChangeKindValueChanged        ChangeKind = "value-changed"
	ChangeKindBehaviorChanged     ChangeKind = "behavior-changed"
	ChangeKindDeprecated          ChangeKind = "deprecated"
	ChangeKindNowRequired         ChangeKind = "now-required"
	ChangeKindValidationTightened ChangeKind = "validation-tightened"
	ChangeKindRequirementChanged  ChangeKind = "requirement-changed"
	ChangeKindMigrationRequired   ChangeKind = "migration-required"
)

// ChangeKinds lists every change kind.
var ChangeKinds = []ChangeKind{
	ChangeKindAdded, ChangeKindRemoved, ChangeKindRenamed, ChangeKindDefaultChanged, ChangeKindValueChanged,
	ChangeKindBehaviorChanged, ChangeKindDeprecated, ChangeKindNowRequired, ChangeKindValidationTightened,
	ChangeKindRequirementChanged, ChangeKindMigrationRequired,
}

// ChangeSpec is the typed change: kind plus before/after state. Before and
// After are JSON-encoded scalars for values ("\"Never\"", "30") and semver
// constraints for requirement changes (">=1.29"); nil means "not stated".
// ReplacedBy names the subject that takes over (required for renamed,
// optional for deprecated/removed): with it, "old set ∧ new unset" is
// expressible (UNKNOWN-ANALYSIS.md §3.5-5).
type ChangeSpec struct {
	Type       ChangeKind `json:"type"`
	Before     *string    `json:"before,omitempty"`
	After      *string    `json:"after,omitempty"`
	ReplacedBy *Subject   `json:"replacedBy,omitempty"`
}

// Validate checks the kind-specific shape. family is the subject's family
// ("" when the assertion carries no subject).
func (c ChangeSpec) Validate(family SubjectFamily) error {
	var errs []error
	known := false
	for _, k := range ChangeKinds {
		known = known || c.Type == k
	}
	if !known {
		return fmt.Errorf("change: unknown type %q", c.Type)
	}
	both := c.Type == ChangeKindDefaultChanged || c.Type == ChangeKindValueChanged
	if both && (c.Before == nil || c.After == nil) {
		errs = append(errs, fmt.Errorf("change %s: before and after are required", c.Type))
	}
	if both && c.Before != nil && c.After != nil && *c.Before == *c.After {
		errs = append(errs, fmt.Errorf("change %s: before equals after", c.Type))
	}
	if c.Type == ChangeKindRequirementChanged {
		if c.After == nil {
			errs = append(errs, errors.New("change requirement-changed: after (the new requirement range) is required"))
		} else if _, err := semver.NewConstraint(*c.After); err != nil {
			errs = append(errs, fmt.Errorf("change requirement-changed: after %q is not a version constraint: %v", *c.After, err))
		}
		if family != "" && family != SubjectCompatibilityBoundary && family != SubjectProductRelationship {
			errs = append(errs, fmt.Errorf("change requirement-changed applies to compatibility-boundary/product-relationship subjects, not %s", family))
		}
	}
	if family != "" && (c.Type == ChangeKindMigrationRequired) != (family == SubjectMigration) {
		errs = append(errs, fmt.Errorf("change migration-required and subject family migration go together (got %s on %s)", c.Type, family))
	}
	switch c.Type {
	case ChangeKindRenamed, ChangeKindDeprecated, ChangeKindRemoved:
		if c.Type == ChangeKindRenamed && c.ReplacedBy == nil {
			errs = append(errs, errors.New("change renamed: replacedBy is required"))
		}
		if c.ReplacedBy != nil {
			if err := c.ReplacedBy.Validate(); err != nil {
				errs = append(errs, fmt.Errorf("change %s: replacedBy: %w", c.Type, err))
			} else if family != "" && c.ReplacedBy.Family != family {
				errs = append(errs, fmt.Errorf("change %s: replacedBy family %s differs from subject family %s", c.Type, c.ReplacedBy.Family, family))
			}
		}
	default:
		if c.ReplacedBy != nil {
			errs = append(errs, fmt.Errorf("change %s: replacedBy is for renamed/deprecated/removed only", c.Type))
		}
	}
	return errors.Join(errs...)
}

// --- applicability: the condition language -------------------------------------

// ConditionOp is a combinator or a leaf predicate of the applicability
// condition language (DESIGN.md §1.3). Evaluation is three-valued
// (true / false / unknown, Kleene logic).
//
// Top-level leaves read environment-wide facts. A `resource` node scopes its
// operands to ONE resource of a kind ("some Certificate such that …"); the
// scoped leaves `field`, `text-line` and `ref` exist only inside such a scope
// (or inside a `ref`, which re-scopes to the referenced resource).
type ConditionOp string

const (
	// combinators
	OpAll ConditionOp = "all" // conjunction
	OpAny ConditionOp = "any" // disjunction
	// OpNot negates exactly one operand. not(unknown) = unknown; a `true`
	// produced by negating `false` carries the examined-but-unmatched
	// environment evidence and is `unknown` when nothing was examined.
	OpNot ConditionOp = "not"
	// OpResource: some resource of Group/Kind (optionally Version, Name)
	// satisfies all operands, evaluated against that one resource.
	OpResource ConditionOp = "resource"

	// scoped leaves (inside resource / ref)
	OpField    ConditionOp = "field"     // a field of the scoped resource (Path with [] list markers)
	OpTextLine ConditionOp = "text-line" // a line of embedded text at Path (e.g. ConfigMap data) matches Pattern
	OpRef      ConditionOp = "ref"       // the reference at Path resolves to a resource (Kind/Group) satisfying the operands

	// environment-wide leaves
	OpValuesKey      ConditionOp = "values-key"
	OpGVKInUse       ConditionOp = "gvk-in-use"
	OpImageInUse     ConditionOp = "image-in-use"
	OpCLIFlag        ConditionOp = "cli-flag"
	OpEnvVar         ConditionOp = "env-var"
	OpFeatureGate    ConditionOp = "feature-gate"
	OpProductVersion ConditionOp = "product-version"
	OpClusterVersion ConditionOp = "cluster-version"
	// OpUpgradeFrom compares the edge's from-version (the version the
	// environment runs today, an input of `ri impact`) with Range.
	OpUpgradeFrom ConditionOp = "upgrade-from"
	// OpUndecidable is an explicit "not statically decidable": always
	// unknown with Reason and Needed.
	OpUndecidable ConditionOp = "undecidable"
)

// ConditionOps lists every op.
var ConditionOps = []ConditionOp{
	OpAll, OpAny, OpNot, OpResource, OpField, OpTextLine, OpRef, OpValuesKey, OpGVKInUse, OpImageInUse,
	OpCLIFlag, OpEnvVar, OpFeatureGate, OpProductVersion, OpClusterVersion, OpUpgradeFrom, OpUndecidable,
}

// FieldState is the state a leaf predicate tests.
type FieldState string

const (
	StateUnset       FieldState = "unset"
	StateSet         FieldState = "set"
	StateEquals      FieldState = "equals"        // set to any of Values (equals / in)
	StateNotEquals   FieldState = "not-equals"    // set, to none of Values
	StateMatches     FieldState = "matches"       // set, value (or a line, for text-line) matches Pattern (RE2)
	StateHasToken    FieldState = "has-token"     // a Separator-delimited list contains any of Values exactly ("ValidateCAA=true")
	StateHasTokenKey FieldState = "has-token-key" // … contains a k=v token whose key is any of Values ("ValidateCAA")
	StateEnabled     FieldState = "enabled"       // feature-gate
	StateDisabled    FieldState = "disabled"      // feature-gate
	StateInRange     FieldState = "in-range"      // present, version inside Range
	StateOutOfRange  FieldState = "out-of-range"  // present, version outside Range
)

// FieldStates lists every state.
var FieldStates = []FieldState{
	StateUnset, StateSet, StateEquals, StateNotEquals, StateMatches, StateHasToken, StateHasTokenKey,
	StateEnabled, StateDisabled, StateInRange, StateOutOfRange,
}

var (
	valueStates   = []FieldState{StateUnset, StateSet, StateEquals, StateNotEquals, StateMatches, StateHasToken, StateHasTokenKey}
	gateStates    = []FieldState{StateEnabled, StateDisabled, StateUnset}
	versionStates = []FieldState{StateInRange, StateOutOfRange}
	lineStates    = []FieldState{StateMatches}
)

// conditionSpec declares, per op, the required and optional fields and the
// allowed states (nil: State must be empty), whether operands are taken, and
// where the op may appear.
type conditionSpec struct {
	required, optional []string
	states             []FieldState
	stateOptional      bool
	operands           int  // 0 none, 1 exactly one, -1 one or more
	scoped             bool // only inside resource/ref
	environmentWide    bool // never inside resource/ref
}

var conditionSpecs = map[ConditionOp]conditionSpec{
	OpAll:            {operands: -1},
	OpAny:            {operands: -1},
	OpNot:            {operands: 1},
	OpResource:       {required: []string{"kind"}, optional: []string{"group", "version", "name"}, operands: -1, environmentWide: true},
	OpField:          {required: []string{"path"}, optional: []string{"pattern", "separator"}, states: valueStates, scoped: true},
	OpTextLine:       {required: []string{"path", "pattern"}, states: lineStates, scoped: true},
	OpRef:            {required: []string{"path", "kind"}, optional: []string{"group"}, operands: -1, scoped: true},
	OpValuesKey:      {required: []string{"path"}, optional: []string{"pattern", "separator"}, states: valueStates, environmentWide: true},
	OpGVKInUse:       {required: []string{"kind"}, optional: []string{"group", "version"}, environmentWide: true},
	OpImageInUse:     {required: []string{"name"}, optional: []string{"range"}, states: versionStates, stateOptional: true, environmentWide: true},
	OpCLIFlag:        {required: []string{"name"}, optional: []string{"component", "pattern", "separator"}, states: valueStates, environmentWide: true},
	OpEnvVar:         {required: []string{"name"}, optional: []string{"component", "pattern", "separator"}, states: valueStates, environmentWide: true},
	OpFeatureGate:    {required: []string{"name"}, optional: []string{"path", "component"}, states: gateStates, environmentWide: true},
	OpProductVersion: {required: []string{"name", "range"}, states: versionStates, environmentWide: true},
	OpClusterVersion: {required: []string{"name", "range"}, states: versionStates, environmentWide: true},
	OpUpgradeFrom:    {required: []string{"range"}, states: versionStates, environmentWide: true},
	OpUndecidable:    {required: []string{"reason", "needed"}},
}

// MaxConditionDepth bounds condition trees.
const MaxConditionDepth = 8

// Condition is one node of an applicability condition: a combinator, a
// resource scope, or a leaf predicate whose fields depend on Op.
type Condition struct {
	Op        ConditionOp   `json:"op"`
	Of        []Condition   `json:"of,omitempty"`
	Group     string        `json:"group,omitempty"`
	Version   string        `json:"version,omitempty"`
	Kind      string        `json:"kind,omitempty"`
	Name      string        `json:"name,omitempty"` // resource name, flag/env/gate name, image repository, product id, platform
	Path      string        `json:"path,omitempty"` // field path in values/schema syntax, [] marks list elements
	Component string        `json:"component,omitempty"`
	State     FieldState    `json:"state,omitempty"`
	Values    []string      `json:"values,omitempty"`    // JSON-encoded for equals/not-equals; plain tokens for has-token(-key)
	Pattern   string        `json:"pattern,omitempty"`   // RE2; matches / text-line only
	Separator string        `json:"separator,omitempty"` // has-token(-key); default ","
	Range     string        `json:"range,omitempty"`     // semver constraint
	Reason    UnknownReason `json:"reason,omitempty"`    // undecidable only
	Needed    string        `json:"needed,omitempty"`    // undecidable only
}

func (c Condition) field(name string) string {
	switch name {
	case "group":
		return c.Group
	case "version":
		return c.Version
	case "kind":
		return c.Kind
	case "name":
		return c.Name
	case "path":
		return c.Path
	case "component":
		return c.Component
	case "pattern":
		return c.Pattern
	case "separator":
		return c.Separator
	case "range":
		return c.Range
	case "reason":
		return string(c.Reason)
	case "needed":
		return c.Needed
	}
	panic("domain: unknown condition field " + name)
}

var conditionFieldNames = []string{"group", "version", "kind", "name", "path", "component", "pattern", "separator", "range", "reason", "needed"}

// Validate checks the tree: known ops, exactly the op's fields, allowed
// states, state-specific values/pattern/separator, scoping, parsable ranges and
// patterns, bounded depth.
func (c Condition) Validate() error { return c.validate(1, false) }

func (c Condition) validate(depth int, inScope bool) error {
	if depth > MaxConditionDepth {
		return fmt.Errorf("condition: deeper than %d", MaxConditionDepth)
	}
	spec, ok := conditionSpecs[c.Op]
	if !ok {
		return fmt.Errorf("condition: unknown op %q", c.Op)
	}
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("condition %s: %s", c.Op, fmt.Sprintf(format, args...)))
	}
	if spec.scoped && !inScope {
		bad("only valid inside a resource or ref scope")
	}
	if spec.environmentWide && inScope {
		bad("an environment-wide predicate cannot appear inside a resource or ref scope")
	}
	switch {
	case spec.operands == 0 && len(c.Of) != 0:
		bad("takes no operands")
	case spec.operands == 1 && len(c.Of) != 1:
		bad("takes exactly one operand, has %d", len(c.Of))
	case spec.operands == -1 && len(c.Of) == 0:
		bad("needs at least one operand")
	}
	allowed := map[string]bool{}
	for _, n := range spec.required {
		allowed[n] = true
		if strings.TrimSpace(c.field(n)) == "" {
			bad("%s is required", n)
		}
	}
	for _, n := range spec.optional {
		allowed[n] = true
	}
	for _, n := range conditionFieldNames {
		if !allowed[n] && c.field(n) != "" {
			bad("%s is not a field of this op", n)
		}
	}
	switch {
	case spec.states == nil && c.State != "":
		bad("takes no state")
	case spec.states != nil && c.State == "" && !spec.stateOptional:
		bad("state is required (one of %v)", spec.states)
	case c.State != "" && !containsState(spec.states, c.State):
		bad("state %q not allowed (one of %v)", c.State, spec.states)
	}
	switch c.State {
	case StateEquals, StateNotEquals:
		if len(c.Values) == 0 {
			bad("state %s needs values", c.State)
		}
		for _, v := range c.Values {
			if !json.Valid([]byte(v)) {
				bad("value %q is not JSON-encoded", v)
			}
		}
	case StateHasToken, StateHasTokenKey:
		if len(c.Values) == 0 {
			bad("state %s needs values (the tokens)", c.State)
		}
	default:
		if len(c.Values) != 0 {
			bad("values are only used with equals/not-equals/has-token/has-token-key")
		}
	}
	if (c.State == StateMatches) != (c.Pattern != "") {
		bad("pattern is used exactly with state matches")
	}
	if c.Pattern != "" {
		if _, err := regexp.Compile(c.Pattern); err != nil {
			bad("pattern %q: %v", c.Pattern, err)
		}
	}
	if c.Separator != "" && c.State != StateHasToken && c.State != StateHasTokenKey {
		bad("separator is only used with has-token/has-token-key")
	}
	if c.Op == OpImageInUse && (c.State == "") != (c.Range == "") {
		bad("state and range go together")
	}
	if c.Range != "" {
		if _, err := semver.NewConstraint(c.Range); err != nil {
			bad("range %q: %v", c.Range, err)
		}
	}
	if c.Op == OpUndecidable && c.Reason != "" && !c.Reason.Valid() {
		bad("unknown reason %q", c.Reason)
	}
	childScope := inScope || c.Op == OpResource || c.Op == OpRef
	for i, sub := range c.Of {
		if err := sub.validate(depth+1, childScope); err != nil {
			errs = append(errs, fmt.Errorf("%s[%d]: %w", c.Op, i, err))
		}
	}
	return errors.Join(errs...)
}

func containsState(xs []FieldState, s FieldState) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Applicability is which environments are exposed: Exposure is the state
// under which the consequence happens; Overlap (optional) is the state in
// which the environment touches the subject but is shielded (→ informational).
type Applicability struct {
	Exposure Condition  `json:"exposure"`
	Overlap  *Condition `json:"overlap,omitempty"`
}

// Validate checks both conditions.
func (a Applicability) Validate() error {
	var errs []error
	if err := a.Exposure.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("exposure: %w", err))
	}
	if a.Overlap != nil {
		if err := a.Overlap.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("overlap: %w", err))
		}
	}
	return errors.Join(errs...)
}

// --- consequence ----------------------------------------------------------------

// ConsequenceKind is what happens to an exposed environment that does nothing.
type ConsequenceKind string

const (
	ConsequenceUpgradeBlocked    ConsequenceKind = "upgrade-blocked"
	ConsequenceResourceRejected  ConsequenceKind = "resource-rejected"
	ConsequenceSettingIgnored    ConsequenceKind = "setting-ignored"
	ConsequenceBehaviorChange    ConsequenceKind = "behavior-change"
	ConsequencePermissionChange  ConsequenceKind = "permission-change"
	ConsequenceMigrationRequired ConsequenceKind = "migration-required"
	ConsequenceDeprecation       ConsequenceKind = "deprecation"
	ConsequenceNone              ConsequenceKind = "none"
)

// ConsequenceKinds lists every kind.
var ConsequenceKinds = []ConsequenceKind{
	ConsequenceUpgradeBlocked, ConsequenceResourceRejected, ConsequenceSettingIgnored, ConsequenceBehaviorChange,
	ConsequencePermissionChange, ConsequenceMigrationRequired, ConsequenceDeprecation, ConsequenceNone,
}

// Valid reports whether k is a known kind.
func (k ConsequenceKind) Valid() bool {
	for _, x := range ConsequenceKinds {
		if k == x {
			return true
		}
	}
	return false
}

// ActionEligible reports whether the kind describes concrete failure or loss
// of intended behaviour — the only kinds whose when-exposed class may be
// action-required.
func (k ConsequenceKind) ActionEligible() bool {
	switch k {
	case ConsequenceUpgradeBlocked, ConsequenceResourceRejected, ConsequenceSettingIgnored,
		ConsequenceBehaviorChange, ConsequencePermissionChange, ConsequenceMigrationRequired:
		return true
	}
	return false
}

// DefaultExposedClass is the SUGGESTED when-exposed class for a change kind
// and consequence kind (UNKNOWN-ANALYSIS.md §3.5-3). It only pre-fills the
// review form: the class a fact carries is the one its consequence aspect was
// verified with (Consequence.ExposedClass).
func DefaultExposedClass(change ChangeKind, k ConsequenceKind) ImpactClass {
	switch {
	case k == ConsequenceNone:
		return ImpactInformational
	case k == ConsequenceDeprecation, change == ChangeKindDeprecated, change == ChangeKindDefaultChanged, change == ChangeKindBehaviorChanged:
		return ImpactReviewRequired
	case k.ActionEligible():
		return ImpactActionRequired
	}
	return ImpactReviewRequired
}

// Consequence is the typed consequence of a change for an exposed environment,
// including the class an exposed environment receives. ExposedClass is part
// of the consequence aspect: it is verified with it (by a reviewer), and the
// trust ladder still caps it (DESIGN.md §4).
type Consequence struct {
	Kind ConsequenceKind `json:"kind"`
	// ExposedClass is the class for an exposed environment: action-required,
	// review-required or informational.
	ExposedClass ImpactClass `json:"exposedClass"`
	// Statement answers "what exactly will fail if I do nothing?"; required
	// whenever ExposedClass is action-required.
	Statement   string         `json:"statement,omitempty"`
	Remediation string         `json:"remediation,omitempty"`
	Severity    ImpactSeverity `json:"severity,omitempty"`
}

// Validate checks the kind, the class mapping, the statement rule and the severity.
func (c Consequence) Validate() error {
	var errs []error
	if !c.Kind.Valid() {
		errs = append(errs, fmt.Errorf("consequence: unknown kind %q", c.Kind))
	}
	if !c.ExposedClass.Affected() {
		errs = append(errs, fmt.Errorf("consequence: exposedClass must be action-required, review-required or informational, got %q", c.ExposedClass))
	}
	if c.ExposedClass == ImpactActionRequired {
		if !c.Kind.ActionEligible() {
			errs = append(errs, fmt.Errorf("consequence %s: not action-eligible, so exposedClass cannot be action-required", c.Kind))
		}
		if strings.TrimSpace(c.Statement) == "" {
			errs = append(errs, errors.New("consequence: an action-required class needs the statement of what fails if nothing is done"))
		}
	}
	if c.Kind == ConsequenceNone && c.ExposedClass != ImpactInformational {
		errs = append(errs, errors.New("consequence none: exposedClass must be informational"))
	}
	if c.Severity != "" {
		ok := false
		for _, s := range AllImpactSeverities {
			ok = ok || s == c.Severity
		}
		if !ok {
			errs = append(errs, fmt.Errorf("consequence: unknown severity %q", c.Severity))
		}
	}
	return errors.Join(errs...)
}

// --- assertion and aspects -------------------------------------------------------

// Aspect is one verifiable part of a semantic assertion.
type Aspect string

const (
	AspectSubject       Aspect = "subject"
	AspectChange        Aspect = "change"
	AspectApplicability Aspect = "applicability"
	AspectConsequence   Aspect = "consequence"
)

// Aspects lists every aspect in canonical order.
var Aspects = []Aspect{AspectSubject, AspectChange, AspectApplicability, AspectConsequence}

// Valid reports whether a is a known aspect.
func (a Aspect) Valid() bool {
	for _, x := range Aspects {
		if a == x {
			return true
		}
	}
	return false
}

// SemanticAssertion is the machine-comparable meaning of an upstream change.
// Proposals may carry a subset of the aspects; a VerifiedFact carries all four.
// Statement is a human rendering and is never parsed or compared.
type SemanticAssertion struct {
	Subject       *Subject       `json:"subject,omitempty"`
	Change        *ChangeSpec    `json:"change,omitempty"`
	Applicability *Applicability `json:"applicability,omitempty"`
	Consequence   *Consequence   `json:"consequence,omitempty"`
	Statement     string         `json:"statement,omitempty"`
}

// Has reports whether the assertion states the aspect.
func (a SemanticAssertion) Has(x Aspect) bool {
	switch x {
	case AspectSubject:
		return a.Subject != nil
	case AspectChange:
		return a.Change != nil
	case AspectApplicability:
		return a.Applicability != nil
	case AspectConsequence:
		return a.Consequence != nil
	}
	return false
}

// Stated lists the aspects the assertion states, in canonical order.
func (a SemanticAssertion) Stated() []Aspect {
	var out []Aspect
	for _, x := range Aspects {
		if a.Has(x) {
			out = append(out, x)
		}
	}
	return out
}

// Empty reports whether no aspect is stated.
func (a SemanticAssertion) Empty() bool { return len(a.Stated()) == 0 }

// Validate checks every stated aspect; complete additionally requires all four.
func (a SemanticAssertion) Validate(complete bool) error {
	var errs []error
	var family SubjectFamily
	if a.Subject != nil {
		family = a.Subject.Family
		if err := a.Subject.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	if a.Change != nil {
		if err := a.Change.Validate(family); err != nil {
			errs = append(errs, err)
		}
	}
	if a.Applicability != nil {
		if err := a.Applicability.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("applicability: %w", err))
		}
	}
	if a.Consequence != nil {
		if err := a.Consequence.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	if complete {
		for _, x := range Aspects {
			if !a.Has(x) {
				errs = append(errs, fmt.Errorf("assertion: %s is required", x))
			}
		}
	}
	return errors.Join(errs...)
}

// AspectDigest is a content digest of one aspect ("" when not stated): two
// assertions agree on an aspect iff their digests are equal. The subject
// digest is over Subject.Key(); the others over canonical JSON. Prose
// (Statement, Consequence.Statement/Remediation) is excluded so wording never
// counts as disagreement.
func (a SemanticAssertion) AspectDigest(x Aspect) string {
	if !a.Has(x) {
		return ""
	}
	var part any
	switch x {
	case AspectSubject:
		return ShortHash(string(x), a.Subject.Key())
	case AspectChange:
		part = a.Change
	case AspectApplicability:
		part = a.Applicability
	case AspectConsequence:
		part = struct {
			Kind         ConsequenceKind
			ExposedClass ImpactClass
			Severity     ImpactSeverity
		}{a.Consequence.Kind, a.Consequence.ExposedClass, a.Consequence.Severity}
	}
	b, err := json.Marshal(part)
	if err != nil {
		panic(err) // plain data; cannot fail
	}
	return ShortHash(string(x), string(b))
}

// Digest is the digest over all aspect digests.
func (a SemanticAssertion) Digest() string {
	parts := make([]string, 0, len(Aspects))
	for _, x := range Aspects {
		parts = append(parts, a.AspectDigest(x))
	}
	return ShortHash(parts...)
}

// --- UNKNOWN reasons and verification levels ---------------------------------------

// UnknownReason says why an UNKNOWN finding is unknown (MISSION G18); it
// drives routing (DESIGN.md §1.5).
type UnknownReason string

const (
	UnknownReleaseKnowledgeGap      UnknownReason = "release-knowledge-gap"
	UnknownEnvironmentVisibilityGap UnknownReason = "environment-visibility-gap"
	UnknownCrossProductContextGap   UnknownReason = "cross-product-context-gap"
	UnknownRuntimeBehaviorGap       UnknownReason = "runtime-behavior-gap"
	UnknownEvidenceGap              UnknownReason = "evidence-gap"
	UnknownSemanticAmbiguity        UnknownReason = "semantic-ambiguity"
)

// UnknownReasons lists every reason.
var UnknownReasons = []UnknownReason{
	UnknownReleaseKnowledgeGap, UnknownEnvironmentVisibilityGap, UnknownCrossProductContextGap,
	UnknownRuntimeBehaviorGap, UnknownEvidenceGap, UnknownSemanticAmbiguity,
}

// Valid reports whether r is a known reason.
func (r UnknownReason) Valid() bool {
	for _, x := range UnknownReasons {
		if r == x {
			return true
		}
	}
	return false
}

// VerificationLevel is how an aspect (or a whole fact) was verified.
type VerificationLevel string

const (
	// VerifiedDeterministic: a validator proved it from ingested artifacts.
	VerifiedDeterministic VerificationLevel = "deterministic"
	// VerifiedHuman: a named human reviewer decided it.
	VerifiedHuman VerificationLevel = "human"
	// VerifiedProxy: an AI acting as reviewer decided it — always labelled,
	// never trusted for ACTION REQUIRED or NOT AFFECTED.
	VerifiedProxy VerificationLevel = "proxy"
)

// VerificationLevels lists every level, most trusted first.
var VerificationLevels = []VerificationLevel{VerifiedDeterministic, VerifiedHuman, VerifiedProxy}

// Valid reports whether l is a known level.
func (l VerificationLevel) Valid() bool {
	return l == VerifiedDeterministic || l == VerifiedHuman || l == VerifiedProxy
}

// Trusted reports whether the level may back ACTION REQUIRED / NOT AFFECTED.
func (l VerificationLevel) Trusted() bool { return l == VerifiedDeterministic || l == VerifiedHuman }

// AtLeast reports whether l satisfies the minimum level min. deterministic and
// human are equally trusted; proxy satisfies only a proxy minimum.
func (l VerificationLevel) AtLeast(min VerificationLevel) bool {
	switch min {
	case VerifiedProxy:
		return l.Valid()
	case VerifiedHuman:
		return l.Trusted()
	case VerifiedDeterministic:
		return l == VerifiedDeterministic
	}
	return false
}

// weaker returns the less trusted of two levels.
func weaker(a, b VerificationLevel) VerificationLevel {
	rank := map[VerificationLevel]int{VerifiedDeterministic: 0, VerifiedHuman: 1, VerifiedProxy: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// ReviewerKind distinguishes human reviewers from AI proxies.
type ReviewerKind string

const (
	ReviewerHuman ReviewerKind = "human"
	ReviewerProxy ReviewerKind = "proxy"
)

// --- change anchor -------------------------------------------------------------------

// EvidenceKey is a digest-free identity of an evidence record (kind, URI,
// locator, excerpt): stable when the upstream document changes elsewhere.
func EvidenceKey(e Evidence) string {
	return "ek-" + ShortHash(string(e.Kind), e.URI, e.Locator, e.Excerpt)
}

// StatementKey is a locator-free identity of the statement an evidence
// record quotes (kind, URI, excerpt reduced to lowercase letters and digits):
// stable when lines shift above it or markup changes.
func StatementKey(e Evidence) string {
	return "sk-" + ShortHash(string(e.Kind), e.URI, alnum(e.Excerpt))
}

func alnum(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// noteDerived reports whether a change comes from upstream prose (declared /
// heuristic provenance) rather than a computed diff. Statement anchors are
// meaningful only for prose: computed diffs share structured evidence (one
// values.yaml record backs every values change), so they attach to facts by
// subject restatement instead (DESIGN.md §2.1).
func noteDerived(c Change) bool {
	return c.Provenance.Method == MethodDeclared || c.Provenance.Method == MethodHeuristic
}

// ChangeAnchor is the statement-level identity of one upstream change that
// states a fact: its introducing release and the keys of the evidence that
// quotes it. A fact holds one anchor per restatement it knows of; it never
// keys on a chg- id (those are informational only — they shift with note
// text and differ between edges).
type ChangeAnchor struct {
	Release       string   `json:"release,omitempty"` // introducing release as the change states it
	EvidenceKeys  []string `json:"evidenceKeys,omitempty"`
	StatementKeys []string `json:"statementKeys"`
	// ChangeIDs are the chg- ids the anchored change had when it was seen;
	// informational, never matched on.
	ChangeIDs []string `json:"changeIds,omitempty"`
}

// NewChangeAnchor anchors c; lookup resolves c's evidence ids.
func NewChangeAnchor(c Change, lookup func(EvidenceID) (Evidence, bool)) ChangeAnchor {
	a := ChangeAnchor{Release: c.Release, ChangeIDs: []string{c.ID}}
	for _, id := range c.Evidence {
		if e, ok := lookup(id); ok {
			a.EvidenceKeys = appendUniqueString(a.EvidenceKeys, EvidenceKey(e))
			a.StatementKeys = appendUniqueString(a.StatementKeys, StatementKey(e))
		}
	}
	sort.Strings(a.EvidenceKeys)
	sort.Strings(a.StatementKeys)
	return a
}

// Matches reports whether the note-derived change c states the anchored
// statement: the releases are compatible (equal, or either unstated) and one
// of c's evidence records has an anchored evidence or statement key. Computed
// changes never match an anchor.
func (a ChangeAnchor) Matches(c Change, lookup func(EvidenceID) (Evidence, bool)) bool {
	if !noteDerived(c) {
		return false
	}
	if a.Release != "" && c.Release != "" && a.Release != c.Release {
		return false
	}
	keys := map[string]bool{}
	for _, k := range a.EvidenceKeys {
		keys[k] = true
	}
	for _, k := range a.StatementKeys {
		keys[k] = true
	}
	for _, id := range c.Evidence {
		if e, ok := lookup(id); ok && (keys[EvidenceKey(e)] || keys[StatementKey(e)]) {
			return true
		}
	}
	return false
}

// Validate checks the anchor's shape.
func (a ChangeAnchor) Validate() error {
	var errs []error
	if len(a.StatementKeys) == 0 {
		errs = append(errs, errors.New("anchor: at least one statement key is required"))
	}
	for _, k := range a.StatementKeys {
		if !strings.HasPrefix(k, "sk-") {
			errs = append(errs, fmt.Errorf("anchor: statement key %q lacks the sk- prefix", k))
		}
	}
	for _, k := range a.EvidenceKeys {
		if !strings.HasPrefix(k, "ek-") {
			errs = append(errs, fmt.Errorf("anchor: evidence key %q lacks the ek- prefix", k))
		}
	}
	return errors.Join(errs...)
}

// IsUmbrella reports whether a note-derived change bundles several distinct
// upstream items (a heading such as "Feature Flag Promotions / Deprecations"
// with a list under it, or items merged from one document): its evidence
// quotes two or more different statements of the same document, or its
// detail lists two or more items. Facts never attach to umbrellas — a fact
// about one item must not claim the others (UNKNOWN-ANALYSIS.md D13).
func IsUmbrella(c Change, lookup func(EvidenceID) (Evidence, bool)) bool {
	if !noteDerived(c) {
		return false
	}
	perURI := map[string]map[string]bool{}
	for _, id := range c.Evidence {
		e, ok := lookup(id)
		if !ok {
			continue
		}
		if perURI[e.URI] == nil {
			perURI[e.URI] = map[string]bool{}
		}
		perURI[e.URI][alnum(e.Excerpt)] = true
		if len(perURI[e.URI]) >= 2 {
			return true
		}
	}
	items := 0
	for _, line := range strings.Split(c.Detail, "\n") {
		if listItem.MatchString(line) {
			items++
		}
	}
	return items >= 2
}

var listItem = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+\S`)

func appendUniqueString(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
}

// validateUpstreamEvidence checks a self-contained evidence snapshot: unique
// ids, and only upstream kinds — knowledge is release-level and never cites the
// user's environment.
func validateUpstreamEvidence(what string, evs []Evidence) []error {
	var errs []error
	seen := map[EvidenceID]bool{}
	for _, e := range evs {
		if e.ID == "" || e.URI == "" {
			errs = append(errs, fmt.Errorf("%s: evidence needs id and uri", what))
		}
		if seen[e.ID] {
			errs = append(errs, fmt.Errorf("%s: duplicate evidence %s", what, e.ID))
		}
		seen[e.ID] = true
		if e.Kind == EvidenceLocalFile || e.Kind == EvidenceInput {
			errs = append(errs, fmt.Errorf("%s: evidence %s is environment evidence (%s); knowledge is release-level", what, e.ID, e.Kind))
		}
	}
	return errs
}

func evidenceIDs(evs []Evidence) map[EvidenceID]bool {
	m := map[EvidenceID]bool{}
	for _, e := range evs {
		m[e.ID] = true
	}
	return m
}

// --- SemanticCandidate ---------------------------------------------------------------

// SemanticCandidate is one upstream change the join cannot evaluate,
// environment-independent, with a self-contained snapshot of its evidence
// (the change's evidence plus any artifact-snapshot evidence the generator
// attaches as context; proposals may cite only these).
type SemanticCandidate struct {
	ID       string    `json:"id"`
	Product  ProductID `json:"product"`
	Release  string    `json:"release,omitempty"`
	ChangeID string    `json:"changeId"` // the edge change it was generated from (informational identity)
	// Anchor is set for note-derived changes; computed changes have none
	// (they attach by subject restatement).
	Anchor    *ChangeAnchor `json:"anchor,omitempty"`
	Category  Category      `json:"category"`
	Title     string        `json:"title"`
	Text      string        `json:"text,omitempty"`
	Evidence  []Evidence    `json:"evidence"`
	Hints     []string      `json:"hints,omitempty"` // deterministic pre-extractions (tokens), never conclusions
	Producer  string        `json:"producer"`
	CreatedAt time.Time     `json:"createdAt"`
}

// CandidateID derives a candidate id: the anchor's statement keys for prose,
// the change id for computed diffs (whose ids are rule + subjects).
func CandidateID(product ProductID, release, changeID string, a *ChangeAnchor) string {
	if a != nil {
		return CandidateIDPrefix + ShortHash(append([]string{string(product), release}, a.StatementKeys...)...)
	}
	return CandidateIDPrefix + ShortHash(string(product), release, changeID)
}

// Validate checks identity, anchor and evidence.
func (c SemanticCandidate) Validate() error {
	var errs []error
	want := CandidateID(c.Product, c.Release, c.ChangeID, c.Anchor)
	if c.ID != want {
		errs = append(errs, fmt.Errorf("candidate %s: id is not derived from product+release+anchor (want %s)", c.ID, want))
	}
	if c.Product == "" || c.ChangeID == "" || strings.TrimSpace(c.Title) == "" || c.Producer == "" || c.CreatedAt.IsZero() {
		errs = append(errs, fmt.Errorf("candidate %s: product, changeId, title, producer and createdAt are required", c.ID))
	}
	if c.Anchor != nil {
		if c.Release != c.Anchor.Release {
			errs = append(errs, fmt.Errorf("candidate %s: release %q differs from anchor release %q", c.ID, c.Release, c.Anchor.Release))
		}
		if err := c.Anchor.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("candidate %s: %w", c.ID, err))
		}
	}
	if len(c.Evidence) == 0 {
		errs = append(errs, fmt.Errorf("candidate %s: no evidence", c.ID))
	}
	errs = append(errs, validateUpstreamEvidence("candidate "+c.ID, c.Evidence)...)
	return errors.Join(errs...)
}

// --- SemanticProposal ------------------------------------------------------------------

// ProposalTask is what a model was asked.
type ProposalTask string

const (
	TaskSemanticMapping ProposalTask = "semantic-mapping" // subject + change
	TaskApplicability   ProposalTask = "applicability"
	TaskConsequence     ProposalTask = "consequence"
	TaskRelationship    ProposalTask = "relationship" // cross-product: subject + change + applicability
	TaskDuplicate       ProposalTask = "duplicate"    // is this an already-known fact?
	TaskFull            ProposalTask = "full"         // all four aspects
)

// ProposalTasks lists every task.
var ProposalTasks = []ProposalTask{TaskSemanticMapping, TaskApplicability, TaskConsequence, TaskRelationship, TaskDuplicate, TaskFull}

// TaskAspects returns the aspects a task's answer must address (asserted or
// undetermined); nil for an unknown task.
func TaskAspects(t ProposalTask) []Aspect {
	switch t {
	case TaskSemanticMapping:
		return []Aspect{AspectSubject, AspectChange}
	case TaskApplicability:
		return []Aspect{AspectApplicability}
	case TaskConsequence:
		return []Aspect{AspectConsequence}
	case TaskRelationship:
		return []Aspect{AspectSubject, AspectChange, AspectApplicability}
	case TaskFull:
		return Aspects
	case TaskDuplicate:
		return []Aspect{}
	}
	return nil
}

func containsAspect(xs []Aspect, a Aspect) bool {
	for _, x := range xs {
		if x == a {
			return true
		}
	}
	return false
}

// SemanticProposal is ONE model's answer to ONE task for ONE candidate. It is
// never merged with other proposals (MISSION G3): disagreement is data.
type SemanticProposal struct {
	ID          string       `json:"id"`
	CandidateID string       `json:"candidateId"`
	Task        ProposalTask `json:"task"`
	// Provider is who served the model ("anthropic", "zai", "typesafe", …).
	Provider  string            `json:"provider"`
	Assertion SemanticAssertion `json:"assertion"`
	// Undetermined lists the task aspects the model would not commit to.
	Undetermined       []Aspect `json:"undetermined,omitempty"`
	UndeterminedReason string   `json:"undeterminedReason,omitempty"`
	// DuplicateOf names a known fact this candidate restates (task duplicate).
	DuplicateOf string `json:"duplicateOf,omitempty"`
	// SuggestedClass is the model's class suggestion: review-required,
	// informational or unknown — never action-required or not-affected.
	SuggestedClass ImpactClass  `json:"suggestedClass,omitempty"`
	Citations      []EvidenceID `json:"citations,omitempty"`
	Provenance     Provenance   `json:"provenance"`
}

// ProposalID derives a proposal id.
func ProposalID(candidateID string, task ProposalTask, provider string, p Provenance) string {
	return ProposalIDPrefix + ShortHash(candidateID, string(task), provider, p.Model, p.ModelVersion, p.PromptDigest)
}

// Validate checks AI provenance, task scope, abstention and the class cap.
func (p SemanticProposal) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("proposal %s: %s", p.ID, fmt.Sprintf(format, args...)))
	}
	if p.ID != ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance) {
		bad("id is not derived from candidate+task+provider+model+prompt")
	}
	if !strings.HasPrefix(p.CandidateID, CandidateIDPrefix) {
		bad("candidateId %q lacks the %s prefix", p.CandidateID, CandidateIDPrefix)
	}
	if strings.TrimSpace(p.Provider) == "" {
		bad("provider is required")
	}
	if p.Provenance.Method != MethodAI {
		bad("must have ai provenance")
	}
	if err := p.Provenance.Validate(); err != nil {
		bad("%v", err)
	}
	if p.Provenance.Confidence == ConfidenceHigh {
		bad("model confidence is capped at medium")
	}
	scope := TaskAspects(p.Task)
	if scope == nil {
		bad("unknown task %q", p.Task)
	}
	if err := p.Assertion.Validate(false); err != nil {
		bad("%v", err)
	}
	for _, x := range p.Assertion.Stated() {
		if scope != nil && !containsAspect(scope, x) {
			bad("asserts %s, outside task %s", x, p.Task)
		}
	}
	seen := map[Aspect]bool{}
	for _, x := range p.Undetermined {
		switch {
		case !x.Valid():
			bad("unknown undetermined aspect %q", x)
		case seen[x]:
			bad("undetermined %s twice", x)
		case p.Assertion.Has(x):
			bad("%s is both asserted and undetermined", x)
		case scope != nil && !containsAspect(scope, x):
			bad("undetermined %s is outside task %s", x, p.Task)
		}
		seen[x] = true
	}
	for _, x := range scope {
		if !p.Assertion.Has(x) && !seen[x] {
			bad("task %s: %s is neither asserted nor undetermined", p.Task, x)
		}
	}
	if (len(p.Undetermined) > 0) != (strings.TrimSpace(p.UndeterminedReason) != "") {
		bad("undeterminedReason is required exactly when aspects are undetermined")
	}
	if p.DuplicateOf != "" && (p.Task != TaskDuplicate || !strings.HasPrefix(p.DuplicateOf, FactIDPrefix)) {
		bad("duplicateOf must be a %s… fact id on a duplicate task", FactIDPrefix)
	}
	switch p.SuggestedClass {
	case "", ImpactReviewRequired, ImpactInformational, ImpactUnknown:
	default:
		bad("a model may suggest review-required, informational or unknown, not %q", p.SuggestedClass)
	}
	if (!p.Assertion.Empty() || p.DuplicateOf != "") && len(p.Citations) == 0 {
		bad("an answer that asserts something must cite evidence")
	}
	input := map[EvidenceID]bool{}
	for _, id := range p.Provenance.InputEvidence {
		input[id] = true
	}
	cited := map[EvidenceID]bool{}
	for _, id := range p.Citations {
		if cited[id] {
			bad("cites %s twice", id)
		}
		cited[id] = true
		if !input[id] {
			bad("cites %s, which was not part of its input evidence", id)
		}
	}
	return errors.Join(errs...)
}

// ValidateAgainst additionally checks the proposal against its candidate:
// the ids match and every citation is candidate evidence.
func (p SemanticProposal) ValidateAgainst(c SemanticCandidate) error {
	errs := []error{p.Validate()}
	if p.CandidateID != c.ID {
		errs = append(errs, fmt.Errorf("proposal %s: belongs to %s, not %s", p.ID, p.CandidateID, c.ID))
	}
	ev := evidenceIDs(c.Evidence)
	for _, id := range p.Citations {
		if !ev[id] {
			errs = append(errs, fmt.Errorf("proposal %s: citation %s is not evidence of candidate %s", p.ID, id, c.ID))
		}
	}
	return errors.Join(errs...)
}

// --- ValidationResult --------------------------------------------------------------------

// ValidationOutcome is what a deterministic validator concluded for an aspect.
type ValidationOutcome string

const (
	OutcomeConfirmed    ValidationOutcome = "confirmed"
	OutcomeRefuted      ValidationOutcome = "refuted"
	OutcomeInconclusive ValidationOutcome = "inconclusive"
)

// AspectCheck is one validator conclusion about one aspect.
type AspectCheck struct {
	Aspect  Aspect            `json:"aspect"`
	Outcome ValidationOutcome `json:"outcome"`
	Rule    string            `json:"rule"` // e.g. "crd-schema:default", "canonical:crd-field/default-changed"
	Detail  string            `json:"detail,omitempty"`
}

// ValidationResult is a deterministic validator's verdict on an assertion,
// proven from ingested artifacts (MISSION G5).
type ValidationResult struct {
	ID          string `json:"id"`
	CandidateID string `json:"candidateId"`
	ProposalID  string `json:"proposalId,omitempty"` // the proposal whose assertion was checked, if any
	// Validator is the producer, component@vN (e.g. "semvalidate.crd@v1").
	Validator string            `json:"validator"`
	Assertion SemanticAssertion `json:"assertion"`
	Checks    []AspectCheck     `json:"checks"`
	// Evidence are the artifact records the checks rest on (upstream kinds only).
	Evidence  []Evidence `json:"evidence,omitempty"`
	CheckedAt time.Time  `json:"checkedAt"`
}

// ValidationID derives a validation id.
func ValidationID(candidateID, validator string, a SemanticAssertion) string {
	return ValidationIDPrefix + ShortHash(candidateID, validator, a.Digest())
}

// Validate checks identity, checks and evidence.
func (v ValidationResult) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("validation %s: %s", v.ID, fmt.Sprintf(format, args...)))
	}
	if v.ID != ValidationID(v.CandidateID, v.Validator, v.Assertion) {
		bad("id is not derived from candidate+validator+assertion")
	}
	if !strings.HasPrefix(v.CandidateID, CandidateIDPrefix) {
		bad("candidateId lacks the %s prefix", CandidateIDPrefix)
	}
	if v.ProposalID != "" && !strings.HasPrefix(v.ProposalID, ProposalIDPrefix) {
		bad("proposalId lacks the %s prefix", ProposalIDPrefix)
	}
	if !strings.Contains(v.Validator, "@") {
		bad("validator must be a producer string component@vN, got %q", v.Validator)
	}
	if v.CheckedAt.IsZero() {
		bad("checkedAt is required")
	}
	if err := v.Assertion.Validate(false); err != nil {
		bad("%v", err)
	}
	if len(v.Checks) == 0 {
		bad("no checks")
	}
	seen := map[Aspect]bool{}
	for _, c := range v.Checks {
		switch {
		case !c.Aspect.Valid():
			bad("unknown aspect %q", c.Aspect)
		case seen[c.Aspect]:
			bad("aspect %s checked twice", c.Aspect)
		case !v.Assertion.Has(c.Aspect):
			bad("checks %s, which the assertion does not state", c.Aspect)
		}
		seen[c.Aspect] = true
		switch c.Outcome {
		case OutcomeConfirmed, OutcomeRefuted, OutcomeInconclusive:
		default:
			bad("unknown outcome %q", c.Outcome)
		}
		if c.Rule == "" {
			bad("check of %s has no rule", c.Aspect)
		}
		if c.Outcome == OutcomeConfirmed && c.Aspect == AspectConsequence &&
			(v.Assertion.Consequence == nil || v.Assertion.Consequence.Kind != ConsequenceNone) {
			bad("a consequence is a judgement; validators confirm only consequence kind none")
		}
	}
	if v.Confirms() && len(v.Evidence) == 0 {
		bad("a confirmation must cite the artifact evidence it rests on")
	}
	errs = append(errs, validateUpstreamEvidence("validation "+v.ID, v.Evidence)...)
	return errors.Join(errs...)
}

// Confirms reports whether the result confirms any aspect; with an argument,
// whether it confirms that aspect.
func (v ValidationResult) Confirms(aspects ...Aspect) bool {
	for _, c := range v.Checks {
		if c.Outcome != OutcomeConfirmed {
			continue
		}
		if len(aspects) == 0 || containsAspect(aspects, c.Aspect) {
			return true
		}
	}
	return false
}

// --- ReviewItem ------------------------------------------------------------------------------

// QuestionType is the kind of review question (MISSION G10).
type QuestionType string

const (
	QuestionSemanticMapping     QuestionType = "semantic-mapping"
	QuestionApplicability       QuestionType = "applicability"
	QuestionConsequence         QuestionType = "consequence"
	QuestionClassification      QuestionType = "classification"
	QuestionRelationship        QuestionType = "relationship"
	QuestionDuplicate           QuestionType = "duplicate"
	QuestionEvidenceSufficiency QuestionType = "evidence-sufficiency"
)

// QuestionTypes lists every question type.
var QuestionTypes = []QuestionType{
	QuestionSemanticMapping, QuestionApplicability, QuestionConsequence, QuestionClassification,
	QuestionRelationship, QuestionDuplicate, QuestionEvidenceSufficiency,
}

// QuestionAspects returns the aspects an accept/correct decision on a
// question of this type verifies (empty for duplicate/evidence-sufficiency;
// nil for an unknown type).
func QuestionAspects(q QuestionType) []Aspect {
	switch q {
	case QuestionSemanticMapping:
		return []Aspect{AspectSubject, AspectChange}
	case QuestionApplicability:
		return []Aspect{AspectApplicability}
	case QuestionConsequence, QuestionClassification:
		return []Aspect{AspectConsequence}
	case QuestionRelationship:
		return []Aspect{AspectSubject, AspectChange, AspectApplicability}
	case QuestionDuplicate, QuestionEvidenceSufficiency:
		return []Aspect{}
	}
	return nil
}

// ReviewStatus is a review item's state.
type ReviewStatus string

const (
	ReviewPending       ReviewStatus = "pending"
	ReviewNeedsEvidence ReviewStatus = "needs-evidence"
	ReviewDeferred      ReviewStatus = "deferred"
	ReviewDecided       ReviewStatus = "decided"
	ReviewSuperseded    ReviewStatus = "superseded"
)

// Route is where routing sent a candidate's open aspects (MISSION G16).
type Route string

const (
	RouteAutoVerify      Route = "auto-verify"
	RouteReview          Route = "review"
	RouteMissingEvidence Route = "missing-evidence"
)

// ReviewPriority orders the review queue.
type ReviewPriority string

const (
	PriorityHigh   ReviewPriority = "high"
	PriorityNormal ReviewPriority = "normal"
	PriorityLow    ReviewPriority = "low"
)

// RoutingSignal is one observation routing based its decision on.
type RoutingSignal string

const (
	SignalModelsAgree         RoutingSignal = "models-agree"
	SignalModelsDisagree      RoutingSignal = "models-disagree"
	SignalSingleModel         RoutingSignal = "single-model"
	SignalValidationConfirmed RoutingSignal = "validation-confirmed"
	SignalValidationRefuted   RoutingSignal = "validation-refuted"
	SignalAllUndetermined     RoutingSignal = "all-undetermined"
	SignalHighImpact          RoutingSignal = "high-impact"
)

// RoutingSignals lists every signal.
var RoutingSignals = []RoutingSignal{
	SignalModelsAgree, SignalModelsDisagree, SignalSingleModel, SignalValidationConfirmed,
	SignalValidationRefuted, SignalAllUndetermined, SignalHighImpact,
}

// Routing records why an item is in the queue and how urgent it is; recorded
// so outcomes can test the routing hypotheses later.
type Routing struct {
	Route    Route           `json:"route"`
	Priority ReviewPriority  `json:"priority"`
	Signals  []RoutingSignal `json:"signals,omitempty"`
}

// EnvironmentContext is an optional environment illustration shown to the
// reviewer. Review items are environment-free by default; when one is shown
// it is recorded so the eval can measure transfer (DESIGN.md §7).
type EnvironmentContext struct {
	Label  string `json:"label"`
	Digest string `json:"digest,omitempty"`
}

// ReviewItem is one durable question for an engineer (MISSION G6).
type ReviewItem struct {
	ID           string       `json:"id"`
	CandidateID  string       `json:"candidateId"`
	Product      ProductID    `json:"product"`
	Release      string       `json:"release,omitempty"`
	QuestionType QuestionType `json:"questionType"`
	Question     string       `json:"question"`
	// Proposed is the assertion the reviewer accepts or corrects.
	Proposed    SemanticAssertion   `json:"proposed"`
	Proposals   []string            `json:"proposals,omitempty"`
	Validations []string            `json:"validations,omitempty"`
	Routing     Routing             `json:"routing"`
	Context     *EnvironmentContext `json:"context,omitempty"`
	Status      ReviewStatus        `json:"status"`
	CreatedAt   time.Time           `json:"createdAt"`
}

// ReviewItemID derives a review item id.
func ReviewItemID(candidateID string, q QuestionType, proposed SemanticAssertion) string {
	return ReviewItemIDPrefix + ShortHash(candidateID, string(q), proposed.Digest())
}

// Aspects returns the aspects an accept/correct on this item verifies.
func (r ReviewItem) Aspects() []Aspect { return QuestionAspects(r.QuestionType) }

// Validate checks identity, the question, references, routing and status.
func (r ReviewItem) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("review item %s: %s", r.ID, fmt.Sprintf(format, args...)))
	}
	if r.ID != ReviewItemID(r.CandidateID, r.QuestionType, r.Proposed) {
		bad("id is not derived from candidate+question type+proposed assertion")
	}
	if !strings.HasPrefix(r.CandidateID, CandidateIDPrefix) || r.Product == "" {
		bad("candidateId and product are required")
	}
	asp := QuestionAspects(r.QuestionType)
	if asp == nil {
		bad("unknown question type %q", r.QuestionType)
	}
	if strings.TrimSpace(r.Question) == "" {
		bad("question is required")
	}
	if err := r.Proposed.Validate(false); err != nil {
		bad("%v", err)
	}
	for _, x := range asp {
		if !r.Proposed.Has(x) {
			bad("a %s question must propose the %s aspect", r.QuestionType, x)
		}
	}
	for _, id := range r.Proposals {
		if !strings.HasPrefix(id, ProposalIDPrefix) {
			bad("proposal ref %q lacks the %s prefix", id, ProposalIDPrefix)
		}
	}
	for _, id := range r.Validations {
		if !strings.HasPrefix(id, ValidationIDPrefix) {
			bad("validation ref %q lacks the %s prefix", id, ValidationIDPrefix)
		}
	}
	switch r.Routing.Route {
	case RouteReview, RouteMissingEvidence:
	case RouteAutoVerify:
		bad("auto-verified candidates produce no review item")
	default:
		bad("unknown route %q", r.Routing.Route)
	}
	switch r.Routing.Priority {
	case PriorityHigh, PriorityNormal, PriorityLow:
	default:
		bad("unknown priority %q", r.Routing.Priority)
	}
	sig := map[RoutingSignal]bool{}
	for _, s := range r.Routing.Signals {
		known := false
		for _, x := range RoutingSignals {
			known = known || s == x
		}
		if !known || sig[s] {
			bad("unknown or repeated signal %q", s)
		}
		sig[s] = true
	}
	if sig[SignalModelsAgree] && sig[SignalModelsDisagree] {
		bad("models cannot both agree and disagree")
	}
	if r.Context != nil && strings.TrimSpace(r.Context.Label) == "" {
		bad("context needs a label")
	}
	switch r.Status {
	case ReviewPending, ReviewNeedsEvidence, ReviewDeferred, ReviewDecided, ReviewSuperseded:
	default:
		bad("unknown status %q", r.Status)
	}
	if r.CreatedAt.IsZero() {
		bad("createdAt is required")
	}
	return errors.Join(errs...)
}

// --- ReviewDecision ----------------------------------------------------------------------------

// DecisionAction is what the reviewer did (MISSION G8).
type DecisionAction string

const (
	ActionAccept           DecisionAction = "accept"
	ActionReject           DecisionAction = "reject"
	ActionCorrect          DecisionAction = "correct"
	ActionNeedMoreEvidence DecisionAction = "need-more-evidence"
	ActionDefer            DecisionAction = "defer"
)

// FeedbackLabel labels a decision as a training/evaluation example (MISSION G13).
type FeedbackLabel string

const (
	LabelAccepted             FeedbackLabel = "accepted"
	LabelRejected             FeedbackLabel = "rejected"
	LabelCorrected            FeedbackLabel = "corrected"
	LabelInsufficientEvidence FeedbackLabel = "insufficient-evidence"
	LabelWrongSubject         FeedbackLabel = "wrong-subject"
	LabelWrongChangeType      FeedbackLabel = "wrong-change-type"
	LabelWrongApplicability   FeedbackLabel = "wrong-applicability"
	LabelWrongConsequence     FeedbackLabel = "wrong-consequence"
	LabelWrongClassification  FeedbackLabel = "wrong-classification"
	LabelDuplicate            FeedbackLabel = "duplicate"
)

// FeedbackLabels lists every label.
var FeedbackLabels = []FeedbackLabel{
	LabelAccepted, LabelRejected, LabelCorrected, LabelInsufficientEvidence, LabelWrongSubject,
	LabelWrongChangeType, LabelWrongApplicability, LabelWrongConsequence, LabelWrongClassification, LabelDuplicate,
}

func (l FeedbackLabel) wrong() bool { return strings.HasPrefix(string(l), "wrong-") }

// ReviewDecision is one reviewer's decision on one review item. Decisions
// are append-only; a rejected decision is a retained negative example.
type ReviewDecision struct {
	ID           string          `json:"id"`
	ReviewItemID string          `json:"reviewItemId"`
	Action       DecisionAction  `json:"action"`
	Labels       []FeedbackLabel `json:"labels,omitempty"`
	// Original is the assertion the reviewer was shown; Corrected the
	// reviewer's replacement (correct only). Both are kept (MISSION G9).
	Original  *SemanticAssertion `json:"original,omitempty"`
	Corrected *SemanticAssertion `json:"corrected,omitempty"`
	Reviewer  string             `json:"reviewer"`
	// ReviewerKind proxy requires ProxyProvenance (complete AI provenance);
	// human forbids it. A proxy can never be recorded as human.
	ReviewerKind    ReviewerKind `json:"reviewerKind"`
	ProxyProvenance *Provenance  `json:"proxyProvenance,omitempty"`
	Reason          string       `json:"reason,omitempty"`
	StartedAt       time.Time    `json:"startedAt,omitzero"`
	DecidedAt       time.Time    `json:"decidedAt"`
	ResultingFact   string       `json:"resultingFact,omitempty"`
	DuplicateOf     string       `json:"duplicateOf,omitempty"`
}

// DecisionID derives a decision id.
func DecisionID(itemID, reviewer string, decidedAt time.Time) string {
	return DecisionIDPrefix + ShortHash(itemID, reviewer, decidedAt.UTC().Format(time.RFC3339Nano))
}

// Final is the assertion the decision settles on (Corrected, else Original).
func (d ReviewDecision) Final() *SemanticAssertion {
	if d.Corrected != nil {
		return d.Corrected
	}
	return d.Original
}

// Duration is the time to decision (0 when StartedAt is unknown).
func (d ReviewDecision) Duration() time.Duration {
	if d.StartedAt.IsZero() {
		return 0
	}
	return d.DecidedAt.Sub(d.StartedAt)
}

// Validate checks the action/label/value consistency and the reviewer rules.
func (d ReviewDecision) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("decision %s: %s", d.ID, fmt.Sprintf(format, args...)))
	}
	if d.ID != DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt) {
		bad("id is not derived from item+reviewer+decidedAt")
	}
	if !strings.HasPrefix(d.ReviewItemID, ReviewItemIDPrefix) {
		bad("reviewItemId lacks the %s prefix", ReviewItemIDPrefix)
	}
	if strings.TrimSpace(d.Reviewer) == "" {
		bad("reviewer is required")
	}
	switch d.ReviewerKind {
	case ReviewerHuman:
		if d.ProxyProvenance != nil {
			bad("a human decision carries no AI provenance (a proxy must be recorded as proxy)")
		}
	case ReviewerProxy:
		switch {
		case d.ProxyProvenance == nil:
			bad("a proxy decision requires the proxy model's provenance")
		case d.ProxyProvenance.Method != MethodAI:
			bad("proxy provenance must be method ai")
		default:
			if err := d.ProxyProvenance.Validate(); err != nil {
				bad("proxy provenance: %v", err)
			}
		}
	default:
		bad("unknown reviewerKind %q", d.ReviewerKind)
	}
	if d.DecidedAt.IsZero() {
		bad("decidedAt is required")
	}
	decisive := d.Action == ActionAccept || d.Action == ActionCorrect || d.Action == ActionReject
	if decisive && d.StartedAt.IsZero() {
		bad("startedAt is required for %s (time to decision is measured)", d.Action)
	}
	if !d.StartedAt.IsZero() && d.DecidedAt.Before(d.StartedAt) {
		bad("decidedAt is before startedAt")
	}
	if decisive && d.Original == nil {
		bad("%s requires the original (shown) assertion", d.Action)
	}
	if d.Original != nil {
		if err := d.Original.Validate(false); err != nil {
			bad("original: %v", err)
		}
	}
	if (d.Action == ActionCorrect) != (d.Corrected != nil) {
		bad("corrected is set exactly for action correct")
	}
	if d.Corrected != nil {
		if err := d.Corrected.Validate(false); err != nil {
			bad("corrected: %v", err)
		}
		if d.Original != nil {
			if d.Corrected.Digest() == d.Original.Digest() {
				bad("a correction must change at least one aspect")
			}
			for _, x := range d.Original.Stated() {
				if !d.Corrected.Has(x) {
					bad("the correction drops the %s aspect", x)
				}
			}
		}
	}
	labels := map[FeedbackLabel]bool{}
	wrong := false
	for _, l := range d.Labels {
		known := false
		for _, x := range FeedbackLabels {
			known = known || l == x
		}
		if !known || labels[l] {
			bad("unknown or repeated label %q", l)
		}
		labels[l] = true
		wrong = wrong || l.wrong()
	}
	only := func(allowed ...FeedbackLabel) {
		ok := map[FeedbackLabel]bool{}
		for _, a := range allowed {
			ok[a] = true
		}
		for l := range labels {
			if !ok[l] && !l.wrong() {
				bad("label %s does not fit action %s", l, d.Action)
			}
		}
	}
	switch d.Action {
	case ActionAccept:
		if !labels[LabelAccepted] || wrong || len(labels) != 1 {
			bad("accept is labelled exactly [accepted]")
		}
	case ActionCorrect:
		if !labels[LabelCorrected] || !wrong {
			bad("correct is labelled corrected plus at least one wrong-* label")
		}
		only(LabelCorrected)
	case ActionReject:
		if !labels[LabelRejected] && !labels[LabelDuplicate] {
			bad("reject is labelled rejected or duplicate")
		}
		only(LabelRejected, LabelDuplicate)
	case ActionNeedMoreEvidence:
		if !labels[LabelInsufficientEvidence] || len(labels) != 1 {
			bad("need-more-evidence is labelled exactly [insufficient-evidence]")
		}
	case ActionDefer:
		if len(labels) != 0 {
			bad("defer carries no outcome label")
		}
	default:
		bad("unknown action %q", d.Action)
	}
	if (d.Action == ActionReject || d.Action == ActionCorrect || d.Action == ActionNeedMoreEvidence) && strings.TrimSpace(d.Reason) == "" {
		bad("%s requires a reason", d.Action)
	}
	if d.ResultingFact != "" {
		if d.Action != ActionAccept && d.Action != ActionCorrect {
			bad("only accept/correct produce a fact")
		}
		if !strings.HasPrefix(d.ResultingFact, FactIDPrefix) {
			bad("resultingFact lacks the %s prefix", FactIDPrefix)
		}
	}
	if (d.DuplicateOf != "") != labels[LabelDuplicate] {
		bad("duplicateOf is set exactly with the duplicate label")
	}
	if d.DuplicateOf != "" && !strings.HasPrefix(d.DuplicateOf, FactIDPrefix) {
		bad("duplicateOf lacks the %s prefix", FactIDPrefix)
	}
	return errors.Join(errs...)
}

// --- VerifiedFact ------------------------------------------------------------------------------

// FactStatus is a fact's lifecycle state; only active facts are used.
type FactStatus string

const (
	FactActive     FactStatus = "active"
	FactRetracted  FactStatus = "retracted"
	FactSuperseded FactStatus = "superseded"
)

// AspectVerification records how one aspect of a fact was verified and the
// validation (val-…) or decision (rd-…) records that justify it.
type AspectVerification struct {
	Aspect Aspect            `json:"aspect"`
	Level  VerificationLevel `json:"level"`
	Basis  []string          `json:"basis"`
}

// VerifiedFact is reusable, release-level semantic knowledge (MISSION G11,
// G12): the same fact is evaluated against every environment, never re-reviewed
// per environment. Its identity is product + introducing release + assertion
// (which includes the subject); it attaches to EVERY change of an edge that
// restates it — through any of its statement Anchors (prose), or by subject
// restatement (computed diffs) — and never to an umbrella change
// (DESIGN.md §2.6). Anchors grow as restatements are found (a duplicate
// decision adds the duplicate candidate's anchor); status and anchors are the
// only mutable fields.
type VerifiedFact struct {
	ID      string    `json:"id"`
	Product ProductID `json:"product"`
	// Release is the release that introduced the change ("" only for facts
	// about endpoint diffs that state no release).
	Release      string               `json:"release,omitempty"`
	Anchors      []ChangeAnchor       `json:"anchors,omitempty"`
	Candidates   []string             `json:"candidates"`
	Assertion    SemanticAssertion    `json:"assertion"`
	Verification []AspectVerification `json:"verification"`
	Evidence     []Evidence           `json:"evidence"`
	Status       FactStatus           `json:"status"`
	Supersedes   []string             `json:"supersedes,omitempty"`
	CreatedAt    time.Time            `json:"createdAt"`
}

// VerifiedFactID derives a verified-fact id from product, introducing
// release and assertion — never from a change id.
func VerifiedFactID(product ProductID, release string, assertion SemanticAssertion) string {
	return FactIDPrefix + ShortHash(string(product), release, assertion.Digest())
}

// AttachesByAnchor reports whether the fact attaches to change c through a
// statement anchor: c restates one of the fact's anchored statements in the
// fact's release, and c is not an umbrella. Subject restatement by computed
// diffs and propagation through the deterministic duplicate grouping are the
// applicability lane's (DESIGN.md §2.6); they apply the same umbrella and
// release rules.
func (f VerifiedFact) AttachesByAnchor(c Change, lookup func(EvidenceID) (Evidence, bool)) bool {
	if f.Status != FactActive || IsUmbrella(c, lookup) {
		return false
	}
	if f.Release != "" && c.Release != "" && f.Release != c.Release {
		return false
	}
	for _, a := range f.Anchors {
		if a.Matches(c, lookup) {
			return true
		}
	}
	return false
}

// AspectLevel returns the verification level of one aspect ("" if absent).
func (f VerifiedFact) AspectLevel(x Aspect) VerificationLevel {
	for _, v := range f.Verification {
		if v.Aspect == x {
			return v.Level
		}
	}
	return ""
}

// Level is the weakest aspect level: the fact is as trusted as its least
// verified aspect.
func (f VerifiedFact) Level() VerificationLevel {
	l := VerifiedDeterministic
	for _, x := range Aspects {
		al := f.AspectLevel(x)
		if !al.Valid() {
			return ""
		}
		l = weaker(l, al)
	}
	return l
}

// Validate checks completeness, per-aspect verification and release-level evidence.
func (f VerifiedFact) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("fact %s: %s", f.ID, fmt.Sprintf(format, args...)))
	}
	if f.ID != VerifiedFactID(f.Product, f.Release, f.Assertion) {
		bad("id is not derived from product+release+assertion")
	}
	if f.Product == "" || f.CreatedAt.IsZero() {
		bad("product and createdAt are required")
	}
	if len(f.Candidates) == 0 {
		bad("names no candidate it was derived from")
	}
	for _, id := range f.Candidates {
		if !strings.HasPrefix(id, CandidateIDPrefix) {
			bad("candidate ref %q lacks the %s prefix", id, CandidateIDPrefix)
		}
	}
	for i, a := range f.Anchors {
		if a.Release != f.Release {
			bad("anchor %d: release %q differs from fact release %q", i, a.Release, f.Release)
		}
		if err := a.Validate(); err != nil {
			bad("anchor %d: %v", i, err)
		}
	}
	if err := f.Assertion.Validate(true); err != nil {
		bad("%v", err)
	}
	if f.Assertion.Subject != nil && f.Assertion.Subject.Product != f.Product {
		bad("subject product %q differs from fact product %q", f.Assertion.Subject.Product, f.Product)
	}
	seen := map[Aspect]bool{}
	for _, v := range f.Verification {
		if !v.Aspect.Valid() || seen[v.Aspect] {
			bad("unknown or repeated verification aspect %q", v.Aspect)
			continue
		}
		seen[v.Aspect] = true
		if !v.Level.Valid() {
			bad("%s: unknown level %q", v.Aspect, v.Level)
		}
		if len(v.Basis) == 0 {
			bad("%s: no justifying validation or decision record", v.Aspect)
		}
		for _, id := range v.Basis {
			switch {
			case v.Level == VerifiedDeterministic && !strings.HasPrefix(id, ValidationIDPrefix):
				bad("%s: deterministic verification must rest on validation records (%s…), got %q", v.Aspect, ValidationIDPrefix, id)
			case (v.Level == VerifiedHuman || v.Level == VerifiedProxy) && !strings.HasPrefix(id, DecisionIDPrefix):
				bad("%s: %s verification must rest on decision records (%s…), got %q", v.Aspect, v.Level, DecisionIDPrefix, id)
			}
		}
	}
	for _, x := range Aspects {
		if !seen[x] {
			bad("%s has no verification", x)
		}
	}
	if len(f.Evidence) == 0 {
		bad("no upstream evidence")
	}
	errs = append(errs, validateUpstreamEvidence("fact "+f.ID, f.Evidence)...)
	switch f.Status {
	case FactActive, FactRetracted, FactSuperseded:
	default:
		bad("unknown status %q", f.Status)
	}
	for _, id := range f.Supersedes {
		if !strings.HasPrefix(id, FactIDPrefix) || id == f.ID {
			bad("supersedes %q is not another fact id", id)
		}
	}
	return errors.Join(errs...)
}

// ValidateFactBasis proves a fact's per-aspect verification from the records
// it cites: every basis id resolves; a validation basis confirms that aspect
// on an assertion with the same aspect digest; a decision basis is an
// accept/correct whose reviewerKind EQUALS the claimed level (a proxy cannot
// masquerade as human), whose item verifies that aspect, and whose final
// assertion has the same aspect digest.
func ValidateFactBasis(f VerifiedFact, validations map[string]ValidationResult, decisions map[string]ReviewDecision, items map[string]ReviewItem) error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("fact %s: %s", f.ID, fmt.Sprintf(format, args...)))
	}
	for _, v := range f.Verification {
		want := f.Assertion.AspectDigest(v.Aspect)
		for _, id := range v.Basis {
			switch {
			case strings.HasPrefix(id, ValidationIDPrefix):
				val, ok := validations[id]
				switch {
				case !ok:
					bad("%s: basis %s does not resolve", v.Aspect, id)
				case v.Level != VerifiedDeterministic:
					bad("%s: validation %s can only back deterministic verification", v.Aspect, id)
				case !val.Confirms(v.Aspect):
					bad("%s: validation %s does not confirm it", v.Aspect, id)
				case val.Assertion.AspectDigest(v.Aspect) != want:
					bad("%s: validation %s confirmed a different %s", v.Aspect, id, v.Aspect)
				}
			case strings.HasPrefix(id, DecisionIDPrefix):
				d, ok := decisions[id]
				if !ok {
					bad("%s: basis %s does not resolve", v.Aspect, id)
					continue
				}
				if d.Action != ActionAccept && d.Action != ActionCorrect {
					bad("%s: decision %s is %s, not accept/correct", v.Aspect, id, d.Action)
				}
				if string(d.ReviewerKind) != string(v.Level) {
					bad("%s: claims %s verification but decision %s was made by a %s reviewer", v.Aspect, v.Level, id, d.ReviewerKind)
				}
				item, ok := items[d.ReviewItemID]
				if !ok {
					bad("%s: decision %s's review item %s does not resolve", v.Aspect, id, d.ReviewItemID)
				} else if !containsAspect(item.Aspects(), v.Aspect) {
					bad("%s: decision %s answered a %s question, which does not verify %s", v.Aspect, id, item.QuestionType, v.Aspect)
				}
				if fin := d.Final(); fin == nil || fin.AspectDigest(v.Aspect) != want {
					bad("%s: decision %s settled on a different %s", v.Aspect, id, v.Aspect)
				}
			default:
				bad("%s: basis %q is neither a validation nor a decision", v.Aspect, id)
			}
		}
	}
	return errors.Join(errs...)
}

// --- feedback dataset and the record envelope ----------------------------------------------------

// FeedbackExample is one labelled example of the human-feedback dataset
// (MISSION G13): everything the reviewer saw and decided, exported as JSONL.
type FeedbackExample struct {
	Decision    ReviewDecision     `json:"decision"`
	Item        ReviewItem         `json:"item"`
	Candidate   SemanticCandidate  `json:"candidate"`
	Proposals   []SemanticProposal `json:"proposals,omitempty"`
	Validations []ValidationResult `json:"validations,omitempty"`
	Fact        *VerifiedFact      `json:"fact,omitempty"`
}

// RecordKind names the entity a KnowledgeRecord carries.
type RecordKind string

const (
	RecordCandidate  RecordKind = "candidate"
	RecordProposal   RecordKind = "proposal"
	RecordValidation RecordKind = "validation"
	RecordReviewItem RecordKind = "review-item"
	RecordDecision   RecordKind = "decision"
	RecordFact       RecordKind = "fact"
)

// KnowledgeRecord is the self-describing envelope of one file under
// knowledge/ (schemas/knowledge-record.schema.json): exactly the payload
// named by Kind is set.
type KnowledgeRecord struct {
	SchemaVersion string             `json:"schemaVersion"`
	Kind          RecordKind         `json:"kind"`
	Candidate     *SemanticCandidate `json:"candidate,omitempty"`
	Proposal      *SemanticProposal  `json:"proposal,omitempty"`
	Validation    *ValidationResult  `json:"validation,omitempty"`
	ReviewItem    *ReviewItem        `json:"reviewItem,omitempty"`
	Decision      *ReviewDecision    `json:"decision,omitempty"`
	Fact          *VerifiedFact      `json:"fact,omitempty"`
}

// NewRecord wraps an entity (one of the six knowledge types, value or pointer).
func NewRecord(entity any) (KnowledgeRecord, error) {
	r := KnowledgeRecord{SchemaVersion: KnowledgeSchemaVersion}
	switch e := entity.(type) {
	case SemanticCandidate:
		r.Kind, r.Candidate = RecordCandidate, &e
	case *SemanticCandidate:
		r.Kind, r.Candidate = RecordCandidate, e
	case SemanticProposal:
		r.Kind, r.Proposal = RecordProposal, &e
	case *SemanticProposal:
		r.Kind, r.Proposal = RecordProposal, e
	case ValidationResult:
		r.Kind, r.Validation = RecordValidation, &e
	case *ValidationResult:
		r.Kind, r.Validation = RecordValidation, e
	case ReviewItem:
		r.Kind, r.ReviewItem = RecordReviewItem, &e
	case *ReviewItem:
		r.Kind, r.ReviewItem = RecordReviewItem, e
	case ReviewDecision:
		r.Kind, r.Decision = RecordDecision, &e
	case *ReviewDecision:
		r.Kind, r.Decision = RecordDecision, e
	case VerifiedFact:
		r.Kind, r.Fact = RecordFact, &e
	case *VerifiedFact:
		r.Kind, r.Fact = RecordFact, e
	default:
		return r, fmt.Errorf("knowledge record: unsupported entity %T", entity)
	}
	return r, nil
}

// ID returns the payload's id ("" when the payload is missing).
func (r KnowledgeRecord) ID() string {
	switch {
	case r.Kind == RecordCandidate && r.Candidate != nil:
		return r.Candidate.ID
	case r.Kind == RecordProposal && r.Proposal != nil:
		return r.Proposal.ID
	case r.Kind == RecordValidation && r.Validation != nil:
		return r.Validation.ID
	case r.Kind == RecordReviewItem && r.ReviewItem != nil:
		return r.ReviewItem.ID
	case r.Kind == RecordDecision && r.Decision != nil:
		return r.Decision.ID
	case r.Kind == RecordFact && r.Fact != nil:
		return r.Fact.ID
	}
	return ""
}

// Validate checks the envelope and validates the payload.
func (r KnowledgeRecord) Validate() error {
	if r.SchemaVersion != KnowledgeSchemaVersion {
		return fmt.Errorf("knowledge record: schemaVersion = %q, want %q", r.SchemaVersion, KnowledgeSchemaVersion)
	}
	set := 0
	for _, p := range []bool{r.Candidate != nil, r.Proposal != nil, r.Validation != nil, r.ReviewItem != nil, r.Decision != nil, r.Fact != nil} {
		if p {
			set++
		}
	}
	if set != 1 || r.ID() == "" {
		return fmt.Errorf("knowledge record: kind %q must carry exactly its own payload", r.Kind)
	}
	switch r.Kind {
	case RecordCandidate:
		return r.Candidate.Validate()
	case RecordProposal:
		return r.Proposal.Validate()
	case RecordValidation:
		return r.Validation.Validate()
	case RecordReviewItem:
		return r.ReviewItem.Validate()
	case RecordDecision:
		return r.Decision.Validate()
	case RecordFact:
		return r.Fact.Validate()
	}
	return fmt.Errorf("knowledge record: unknown kind %q", r.Kind)
}
