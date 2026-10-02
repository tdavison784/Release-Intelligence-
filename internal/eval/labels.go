package eval

// Semantic ground-truth labels (MISSION G22, DESIGN.md §9 `groundtruth`).
//
// An expected item may carry `semantics:` (the release-level subject, change
// and consequence an expert reads out of the upstream statement), and every
// environment link may carry `exposure:` / `overlap:` (the applicability
// condition, in the domain condition language, verbatim) plus
// `environmentEvidence:` (the fixture files/lines that decide it). Links the
// fixture genuinely cannot decide are recorded under
// `environment.undecidedImpact` with the UNKNOWN reason a correct engine
// would give.
//
// These labels score the semantic stage (per-model subject / change /
// applicability accuracy) and the transfer of verified facts across
// environments. They are authored blind from upstream sources and are never
// read by knowledge authoring. The field names mirror the domain JSON names,
// so the blocks decode directly into domain.Subject / ChangeSpec /
// Consequence / Condition — strictly: an unknown key is an error, so a typo
// can never silently drop a label.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ItemSemantics is the expected semantic reading of an expected item: what
// changed (subject), how (change), and what happens to an exposed
// environment (consequence, whose exposedClass follows its kind). An item
// that bundles several subjects (one upstream bullet renaming three env
// vars) carries one entry per subject.
type ItemSemantics struct {
	Subject     *domain.Subject     `json:"subject"`
	Change      *domain.ChangeSpec  `json:"change"`
	Consequence *domain.Consequence `json:"consequence"`
	// Note records the labelling judgement when the reading is not literal
	// (optional; the full rationale belongs in NOTES.md).
	Note string `json:"note,omitempty"`
}

// SemanticsLabels is the `semantics:` block of an expected item. In YAML it
// is either one mapping (the common case) or a list of mappings.
type SemanticsLabels []ItemSemantics

// UnmarshalYAML accepts a mapping or a sequence and decodes strictly through
// the domain JSON names.
func (s *SemanticsLabels) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.MappingNode:
		var one ItemSemantics
		if err := decodeViaJSON(n, &one); err != nil {
			return fmt.Errorf("semantics (line %d): %w", n.Line, err)
		}
		*s = SemanticsLabels{one}
	case yaml.SequenceNode:
		var many []ItemSemantics
		if err := decodeViaJSON(n, &many); err != nil {
			return fmt.Errorf("semantics (line %d): %w", n.Line, err)
		}
		*s = many
	default:
		return fmt.Errorf("semantics (line %d): want a mapping or a list of mappings", n.Line)
	}
	return nil
}

// UndecidedLink is an environment link whose correct answer is UNKNOWN: the
// fixture lacks (or withholds) the input that decides it. A correct engine
// emits neither an affected nor a not-affected finding for the item, and
// names Reason. Undecided links are recorded, never counted in
// applicabilityAccuracy (see eval/FORMAT.md: scoring them is a scoring
// change to be pre-registered).
type UndecidedLink struct {
	Expected string               `json:"expected"`
	Reason   domain.UnknownReason `json:"reason"`
	// Needed is the input that would decide the link ("argocd-cm ConfigMap",
	// "product inventory with the istio-csr version").
	Needed string `json:"needed"`
	Why    string `json:"why,omitempty"`
	// Exposure is the condition that cannot be decided here (optional).
	Exposure *domain.Condition `json:"exposure,omitempty"`
	// EnvironmentEvidence names the fixture parts that were examined.
	EnvironmentEvidence []string `json:"environmentEvidence,omitempty"`
}

// linkKeys / undecidedKeys are the accepted YAML keys (anything else is a typo).
var (
	linkKeys      = []string{"expected", "relevance", "why", "exposure", "overlap", "environmentEvidence"}
	undecidedKeys = []string{"expected", "reason", "needed", "why", "exposure", "environmentEvidence"}
)

// UnmarshalYAML decodes an expectedImpact link: the original scalar fields,
// plus the optional semantic labels decoded strictly into domain conditions.
func (l *ImpactLink) UnmarshalYAML(n *yaml.Node) error {
	if err := checkKeys(n, "expectedImpact link", linkKeys); err != nil {
		return err
	}
	var raw struct {
		Expected            string    `yaml:"expected"`
		Relevance           string    `yaml:"relevance"`
		Why                 string    `yaml:"why"`
		Exposure            yaml.Node `yaml:"exposure"`
		Overlap             yaml.Node `yaml:"overlap"`
		EnvironmentEvidence yaml.Node `yaml:"environmentEvidence"`
	}
	if err := n.Decode(&raw); err != nil {
		return err
	}
	*l = ImpactLink{Expected: raw.Expected, Relevance: raw.Relevance, Why: raw.Why}
	var err error
	if l.Exposure, err = optionalCondition(&raw.Exposure, "exposure"); err != nil {
		return fmt.Errorf("expectedImpact %s: %w", raw.Expected, err)
	}
	if l.Overlap, err = optionalCondition(&raw.Overlap, "overlap"); err != nil {
		return fmt.Errorf("expectedImpact %s: %w", raw.Expected, err)
	}
	if l.EnvironmentEvidence, err = stringOrList(&raw.EnvironmentEvidence); err != nil {
		return fmt.Errorf("expectedImpact %s environmentEvidence: %w", raw.Expected, err)
	}
	return nil
}

// UnmarshalYAML decodes an undecidedImpact link (same strictness).
func (u *UndecidedLink) UnmarshalYAML(n *yaml.Node) error {
	if err := checkKeys(n, "undecidedImpact link", undecidedKeys); err != nil {
		return err
	}
	var raw struct {
		Expected            string    `yaml:"expected"`
		Reason              string    `yaml:"reason"`
		Needed              string    `yaml:"needed"`
		Why                 string    `yaml:"why"`
		Exposure            yaml.Node `yaml:"exposure"`
		EnvironmentEvidence yaml.Node `yaml:"environmentEvidence"`
	}
	if err := n.Decode(&raw); err != nil {
		return err
	}
	*u = UndecidedLink{Expected: raw.Expected, Reason: domain.UnknownReason(raw.Reason), Needed: raw.Needed, Why: raw.Why}
	var err error
	if u.Exposure, err = optionalCondition(&raw.Exposure, "exposure"); err != nil {
		return fmt.Errorf("undecidedImpact %s: %w", raw.Expected, err)
	}
	if u.EnvironmentEvidence, err = stringOrList(&raw.EnvironmentEvidence); err != nil {
		return fmt.Errorf("undecidedImpact %s environmentEvidence: %w", raw.Expected, err)
	}
	return nil
}

func checkKeys(n *yaml.Node, what string, allowed []string) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("%s (line %d): want a mapping", what, n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i].Value
		ok := false
		for _, a := range allowed {
			ok = ok || a == k
		}
		if !ok {
			return fmt.Errorf("%s (line %d): unknown key %q (allowed: %s)", what, n.Content[i].Line, k, strings.Join(allowed, ", "))
		}
	}
	return nil
}

func optionalCondition(n *yaml.Node, what string) (*domain.Condition, error) {
	if n.Kind == 0 {
		return nil, nil
	}
	var c domain.Condition
	if err := decodeViaJSON(n, &c); err != nil {
		return nil, fmt.Errorf("%s (line %d): %w", what, n.Line, err)
	}
	return &c, nil
}

func stringOrList(n *yaml.Node) ([]string, error) {
	switch n.Kind {
	case 0:
		return nil, nil
	case yaml.ScalarNode:
		return []string{n.Value}, nil
	case yaml.SequenceNode:
		var out []string
		if err := n.Decode(&out); err != nil {
			return nil, err
		}
		return out, nil
	}
	return nil, fmt.Errorf("line %d: want a string or a list of strings", n.Line)
}

// decodeViaJSON decodes a YAML node into out through JSON, so the domain JSON
// field names (replacedBy, exposedClass, …) apply and unknown keys fail.
// Scalars keep their YAML types: a number where the domain wants a string
// (before/after/values are JSON-encoded strings) is an error, not a silent
// conversion.
func decodeViaJSON(n *yaml.Node, out any) error {
	var v any
	if err := n.Decode(&v); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

// Special environmentEvidence locators that point at case.yaml fields rather
// than fixture files.
const (
	EvidenceClusterVersion = "environment.kubernetes" // the declared cluster version
	EvidenceEdgeFrom       = "from"                   // the edge's from-version (what runs today)
)

var evidenceLocator = regexp.MustCompile(`^([^#]+?)(?:#L(\d+)(?:-L?(\d+))?)?$`)

// validateLabels checks the semantic labels of a case (called from Validate):
// every semantics block is a complete, valid assertion about the case's own
// product; every condition validates; every environmentEvidence locator
// resolves to an existing fixture file (and line range).
func (c *Case) validateLabels() []error {
	var errs []error
	labelled := map[string]bool{}
	for _, e := range c.Expected {
		for i, s := range e.Semantics {
			if err := s.validate(c.Product); err != nil {
				errs = append(errs, fmt.Errorf("expected %s semantics[%d]: %w", e.ID, i, err))
			}
		}
		labelled[e.ID] = len(e.Semantics) > 0
	}
	if c.Environment == nil {
		return errs
	}
	env := c.Environment
	seen := map[string]string{}
	for i, l := range env.ExpectedImpact {
		where := fmt.Sprintf("environment.expectedImpact[%d] (%s)", i, l.Expected)
		if prev, dup := seen[l.Expected]; dup {
			errs = append(errs, fmt.Errorf("%s: %s is already linked in %s", where, l.Expected, prev))
		}
		seen[l.Expected] = "expectedImpact"
		errs = append(errs, c.validateLinkLabels(where, labelled[l.Expected], l.Exposure, l.Overlap, l.EnvironmentEvidence)...)
	}
	for i, u := range env.UndecidedImpact {
		where := fmt.Sprintf("environment.undecidedImpact[%d] (%s)", i, u.Expected)
		if _, ok := labelled[u.Expected]; !ok {
			errs = append(errs, fmt.Errorf("%s: unknown expected id %q", where, u.Expected))
		}
		if prev, dup := seen[u.Expected]; dup {
			errs = append(errs, fmt.Errorf("%s: %s is already linked in %s", where, u.Expected, prev))
		}
		seen[u.Expected] = "undecidedImpact"
		if !u.Reason.Valid() {
			errs = append(errs, fmt.Errorf("%s: reason %q is not an UNKNOWN reason (%v)", where, u.Reason, domain.UnknownReasons))
		}
		if strings.TrimSpace(u.Needed) == "" {
			errs = append(errs, fmt.Errorf("%s: needed (the input that would decide the link) is required", where))
		}
		if u.Exposure != nil || len(u.EnvironmentEvidence) > 0 {
			errs = append(errs, c.validateLinkLabels(where, labelled[u.Expected], u.Exposure, nil, u.EnvironmentEvidence)...)
		}
	}
	return errs
}

func (c *Case) validateLinkLabels(where string, itemLabelled bool, exposure, overlap *domain.Condition, evidence []string) []error {
	var errs []error
	if exposure == nil && overlap != nil {
		errs = append(errs, fmt.Errorf("%s: overlap without exposure", where))
	}
	if exposure != nil {
		if !itemLabelled {
			errs = append(errs, fmt.Errorf("%s: exposure labelled but the expected item has no semantics block", where))
		}
		if err := exposure.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("%s exposure: %w", where, err))
		}
		if len(evidence) == 0 {
			errs = append(errs, fmt.Errorf("%s: environmentEvidence is required with exposure (every decision cites the fixture)", where))
		}
	}
	if overlap != nil {
		if err := overlap.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("%s overlap: %w", where, err))
		}
	}
	for _, ev := range evidence {
		if err := c.checkEvidenceLocator(ev); err != nil {
			errs = append(errs, fmt.Errorf("%s environmentEvidence %q: %w", where, ev, err))
		}
	}
	return errs
}

// checkEvidenceLocator resolves "<path under environment/>[#L<n>[-L<m>]]",
// or one of the case-field locators. Without a case directory (in-code test
// cases) only the syntax is checked.
func (c *Case) checkEvidenceLocator(loc string) error {
	switch loc {
	case EvidenceClusterVersion:
		if c.Environment == nil || c.Environment.Kubernetes == "" {
			return errors.New("environment.kubernetes is not declared")
		}
		return nil
	case EvidenceEdgeFrom:
		return nil
	}
	m := evidenceLocator.FindStringSubmatch(loc)
	if m == nil {
		return errors.New("want <path>[#L<n>[-L<m>]]")
	}
	rel := m[1]
	if filepath.IsAbs(rel) || strings.HasPrefix(filepath.Clean(rel), "..") {
		return errors.New("path must be relative to environment/")
	}
	from, to := 0, 0
	if m[2] != "" {
		from, _ = strconv.Atoi(m[2])
		to = from
		if m[3] != "" {
			to, _ = strconv.Atoi(m[3])
		}
		if from < 1 || to < from {
			return errors.New("bad line range")
		}
	}
	if c.Dir == "" {
		return nil
	}
	p := filepath.Join(c.Dir, EnvironmentDir, rel)
	st, err := os.Stat(p)
	if err != nil {
		return fmt.Errorf("no such fixture file: %w", err)
	}
	if st.IsDir() {
		if from != 0 {
			return errors.New("line range on a directory")
		}
		return nil
	}
	if to > 0 {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		lines := bytes.Count(b, []byte("\n"))
		if len(b) > 0 && b[len(b)-1] != '\n' {
			lines++
		}
		if to > lines {
			return fmt.Errorf("line %d is past the end of the file (%d lines)", to, lines)
		}
	}
	return nil
}

func (s ItemSemantics) validate(product string) error {
	var errs []error
	if s.Subject == nil || s.Change == nil || s.Consequence == nil {
		return errors.New("subject, change and consequence are all required (a ground-truth label is complete)")
	}
	if err := s.Subject.Validate(); err != nil {
		errs = append(errs, err)
	} else if string(s.Subject.Product) != product {
		errs = append(errs, fmt.Errorf("subject product %q is not the case product %q (release knowledge belongs to the upgraded product; name other products via product-relationship)", s.Subject.Product, product))
	}
	if err := s.Change.Validate(s.Subject.Family); err != nil {
		errs = append(errs, err)
	}
	if s.Change.ReplacedBy != nil && string(s.Change.ReplacedBy.Product) != product {
		errs = append(errs, fmt.Errorf("replacedBy product %q is not the case product %q", s.Change.ReplacedBy.Product, product))
	}
	if err := s.Consequence.Validate(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Assertion renders the label as a domain assertion (applicability is
// per-environment and lives on the links).
func (s ItemSemantics) Assertion() domain.SemanticAssertion {
	return domain.SemanticAssertion{Subject: s.Subject, Change: s.Change, Consequence: s.Consequence}
}
