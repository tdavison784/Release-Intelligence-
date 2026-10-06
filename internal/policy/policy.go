// Package policy implements the org-configurable tiered upgrade policy
// (docs/POLICY.md, product-owner decision PO-7): ordered, declarative rules
// classify every rendered change, non-routine upstream change, finding and
// render state of an upgrade into a tier — auto-pass | review | block — and
// the upgrade-level verdict is the strictest tier present.
//
// A policy only decides among review/informational outcomes. The safety
// invariants (invariants.go) are enforced by the evaluator on top of whatever
// the rules say: a policy can never auto-pass or downgrade an ACTION REQUIRED
// finding, a security advisory affecting the target, a render failure, an
// UNKNOWN on a non-routine change, or anything that is not covered by a
// complete render. Everything is deterministic; no LLM, no product names.
package policy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/render"
	"github.com/tdavison784/release-intelligence/policies"
	"github.com/tdavison784/release-intelligence/schemas"
)

// APIVersion is the policy file's apiVersion.
const APIVersion = "ri.dev/upgrade-policy/v1alpha1"

// Tier is the outcome a rule assigns. Order: AutoPass < Review < Block.
type Tier string

const (
	AutoPass Tier = "auto-pass"
	Review   Tier = "review"
	Block    Tier = "block"
)

func (t Tier) rank() int {
	switch t {
	case AutoPass:
		return 0
	case Review:
		return 1
	case Block:
		return 2
	}
	return -1
}

// Max returns the stricter of two tiers.
func Max(a, b Tier) Tier {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

// Subject is the kind of item a rule matches.
type Subject string

const (
	SubjectRendered    Subject = "rendered"     // one semantic change of a rendered diff
	SubjectUpstream    Subject = "upstream"     // one upstream (release-note / computed) change of the edge
	SubjectFinding     Subject = "finding"      // one impact finding that is not cleared
	SubjectRenderState Subject = "render-state" // a render pair that did not produce a complete diff
)

// Render states a render-state rule can match.
const (
	StateNotRendered      = "not-rendered"      // no render was run at all
	StateFailed           = "failed"            // a render failed (UNKNOWN / render-failed, never "no change")
	StateRejectsConfig    = "rejects-config"    // the target chart rejects the customer's values
	StateNotApplicable    = "not-applicable"    // the target cannot be derived from this configuration
	StateIncompleteValues = "incomplete-values" // the customer values were not complete: absence of change is not evidence
)

// Policy is a parsed policy file.
type Policy struct {
	APIVersion  string  `yaml:"apiVersion" json:"apiVersion"`
	Name        string  `yaml:"name" json:"name"`
	Description string  `yaml:"description,omitempty" json:"description,omitempty"`
	Default     Outcome `yaml:"default" json:"default"`
	Rules       []Rule  `yaml:"rules" json:"rules"`

	// Digest is the sha256 of the policy bytes as loaded (set by Load/Parse):
	// the verdict cites it so a result can be tied to the exact rules.
	Digest string `yaml:"-" json:"-"`
}

// Outcome is a tier with its reason.
type Outcome struct {
	Tier   Tier   `yaml:"tier" json:"tier"`
	Reason string `yaml:"reason" json:"reason"`
}

// Rule assigns a tier to the items its match fits.
type Rule struct {
	ID     string `yaml:"id" json:"id"`
	Tier   Tier   `yaml:"tier" json:"tier"`
	Reason string `yaml:"reason" json:"reason"`
	Match  Match  `yaml:"match" json:"match"`
}

// Match is the conjunction of its set fields; a list field matches when the
// item's value is any element of the list. Fields of another subject than
// Match.Subject are rejected at load time.
type Match struct {
	Subject Subject `yaml:"subject" json:"subject"`

	// rendered
	Classes      []string    `yaml:"classes,omitempty" json:"classes,omitempty"`           // render change classes
	Kinds        []string    `yaml:"kinds,omitempty" json:"kinds,omitempty"`               // object kinds
	Namespaces   []string    `yaml:"namespaces,omitempty" json:"namespaces,omitempty"`     // object namespaces ("" = unset is written as "-")
	PathSuffixes []string    `yaml:"pathSuffixes,omitempty" json:"pathSuffixes,omitempty"` // change path suffixes
	Image        *ImageMatch `yaml:"image,omitempty" json:"image,omitempty"`

	// upstream
	Categories   []string `yaml:"categories,omitempty" json:"categories,omitempty"`
	Routine      *bool    `yaml:"routine,omitempty" json:"routine,omitempty"`
	RoutineKinds []string `yaml:"routineKinds,omitempty" json:"routineKinds,omitempty"`
	Breaking     *bool    `yaml:"breaking,omitempty" json:"breaking,omitempty"`
	ActionReq    *bool    `yaml:"actionRequired,omitempty" json:"actionRequired,omitempty"`
	Security     *bool    `yaml:"security,omitempty" json:"security,omitempty"`

	// finding
	FindingClasses []string `yaml:"findingClasses,omitempty" json:"findingClasses,omitempty"`
	FindingRules   []string `yaml:"findingRules,omitempty" json:"findingRules,omitempty"`
	Verification   []string `yaml:"verification,omitempty" json:"verification,omitempty"` // none|deterministic|human|consensus|proxy

	// render-state
	States []string `yaml:"states,omitempty" json:"states,omitempty"`
}

// ImageMatch constrains an image-changed rendered change.
type ImageMatch struct {
	SameRepository *bool    `yaml:"sameRepository,omitempty" json:"sameRepository,omitempty"`
	Repositories   []string `yaml:"repositories,omitempty" json:"repositories,omitempty"` // exact repositories (registry included)
	Bump           []string `yaml:"bump,omitempty" json:"bump,omitempty"`                 // patch|minor|major|digest|prerelease|downgrade|unknown
}

// Default returns the shipped policy (policies/default.yaml).
func Default() *Policy {
	p, err := Parse(policies.Default)
	if err != nil {
		panic("policy: shipped default policy is invalid: " + err.Error())
	}
	return p
}

// Load reads a policy file. The name "default" selects the shipped policy.
func Load(path string) (*Policy, error) {
	if path == "default" {
		return Default(), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("policy: %w", err)
	}
	p, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("policy %s: %w", path, err)
	}
	return p, nil
}

// Parse validates (JSON schema, then semantic checks) and parses a policy.
func Parse(b []byte) (*Policy, error) {
	var raw any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	js, err := json.Marshal(normalizeYAML(raw))
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	sch, err := compileSchema()
	if err != nil {
		return nil, err
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(js))
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if err := sch.Validate(inst); err != nil {
		return nil, fmt.Errorf("does not match the policy schema: %w", err)
	}
	var p Policy
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	p.Digest = "sha256:" + hex.EncodeToString(sum[:])
	return &p, nil
}

func compileSchema() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemas.Policy))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	const url = "https://ri.dev/schemas/upgrade-policy/v1alpha1.json"
	if err := c.AddResource(url, doc); err != nil {
		return nil, err
	}
	return c.Compile(url)
}

// normalizeYAML converts yaml.v3's map[string]any / []any tree into one
// encoding/json accepts (yaml.v3 already yields string keys for plain maps).
func normalizeYAML(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = normalizeYAML(e)
		}
		return x
	case map[any]any:
		m := map[string]any{}
		for k, e := range x {
			m[fmt.Sprint(k)] = normalizeYAML(e)
		}
		return m
	case []any:
		for i, e := range x {
			x[i] = normalizeYAML(e)
		}
		return x
	}
	return v
}

// Validate checks what the schema cannot: ids are unique, a rule only uses
// fields of its own subject, class names exist, and no auto-pass rule asks for
// something the safety invariants forbid. (The evaluator enforces the
// invariants regardless; rejecting such a rule at load time tells the author.)
func (p *Policy) Validate() error {
	var errs []error
	if p.APIVersion != APIVersion {
		errs = append(errs, fmt.Errorf("apiVersion = %q, want %q", p.APIVersion, APIVersion))
	}
	seen := map[string]bool{}
	for i, r := range p.Rules {
		where := fmt.Sprintf("rules[%d] (%s)", i, r.ID)
		if seen[r.ID] {
			errs = append(errs, fmt.Errorf("%s: duplicate rule id", where))
		}
		seen[r.ID] = true
		errs = append(errs, r.Match.validate(where)...)
		if r.Tier == AutoPass {
			errs = append(errs, r.Match.forbiddenAutoPass(where)...)
		}
	}
	return errors.Join(errs...)
}

func (m Match) validate(where string) []error {
	var errs []error
	bad := func(field string) {
		errs = append(errs, fmt.Errorf("%s: field %q does not apply to subject %q", where, field, m.Subject))
	}
	rendered := len(m.Classes) > 0 || len(m.Kinds) > 0 || len(m.Namespaces) > 0 || len(m.PathSuffixes) > 0 || m.Image != nil
	upstream := len(m.Categories) > 0 || m.Routine != nil || len(m.RoutineKinds) > 0 || m.Breaking != nil || m.ActionReq != nil || m.Security != nil
	finding := len(m.FindingClasses) > 0 || len(m.FindingRules) > 0 || len(m.Verification) > 0
	state := len(m.States) > 0
	switch m.Subject {
	case SubjectRendered:
		if upstream {
			bad("categories/routine/breaking/actionRequired/security")
		}
		if finding {
			bad("findingClasses/findingRules/verification")
		}
		if state {
			bad("states")
		}
		known := map[string]bool{}
		for _, c := range render.ChangeClasses {
			known[string(c)] = true
		}
		for _, c := range m.Classes {
			if !known[c] {
				errs = append(errs, fmt.Errorf("%s: unknown rendered change class %q", where, c))
			}
		}
		if m.Image != nil && len(m.Classes) > 0 {
			for _, c := range m.Classes {
				if c != string(render.ImageChanged) {
					errs = append(errs, fmt.Errorf("%s: an image predicate only fits class image-changed, not %q", where, c))
				}
			}
		}
	case SubjectUpstream:
		if rendered {
			bad("classes/kinds/namespaces/pathSuffixes/image")
		}
		if finding {
			bad("findingClasses/findingRules/verification")
		}
		if state {
			bad("states")
		}
	case SubjectFinding:
		if rendered {
			bad("classes/kinds/namespaces/pathSuffixes/image")
		}
		if upstream {
			bad("categories/routine/breaking/actionRequired/security")
		}
		if state {
			bad("states")
		}
		for _, c := range m.FindingClasses {
			if !knownImpactClass(c) {
				errs = append(errs, fmt.Errorf("%s: unknown finding class %q", where, c))
			}
		}
	case SubjectRenderState:
		if rendered || upstream || finding {
			bad("a render-state rule only takes states")
		}
	}
	return errs
}

// forbiddenAutoPass reports why an auto-pass rule can never be right: it is
// written to cover something the invariants keep out of auto-pass.
func (m Match) forbiddenAutoPass(where string) []error {
	var errs []error
	forbid := func(msg string) { errs = append(errs, fmt.Errorf("%s: auto-pass is not allowed %s (safety invariant)", where, msg)) }
	switch m.Subject {
	case SubjectRenderState:
		forbid("for a render state: a render failure, rejection, missing render or incomplete values is never a pass")
	case SubjectFinding:
		for _, c := range m.FindingClasses {
			if c == string(domain.ImpactActionRequired) || c == string(domain.ImpactUnknown) {
				forbid(fmt.Sprintf("for %s findings", c))
			}
		}
	}
	return errs
}

func knownImpactClass(s string) bool {
	for _, c := range domain.AllImpactClasses {
		if string(c) == s {
			return true
		}
	}
	return false
}
