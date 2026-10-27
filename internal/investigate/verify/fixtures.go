package verify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Fixtures are declarative, per product (products/exercises/fixtures/<product>.yaml,
// kind ProductFixtures): how to install the product's controller for a
// synthetic test (chart values, e.g. "install the CRDs") and, per subject
// kind, a minimal valid object of that kind (plus any prerequisite objects it
// needs, e.g. an issuer) and the declared exercise that makes the controller
// act on it. They are release-level and cite upstream documentation, never an
// evaluation case. They live in their own directory so the exercise files'
// digests (recorded by every runtime run) are untouched.

// FixturesKind is the document kind of a fixtures file.
const FixturesKind = "ProductFixtures"

// NamePlaceholder in a subject template is replaced by the arm object's name.
const NamePlaceholder = "{{name}}"

// FixtureSet is one fixtures file.
type FixtureSet struct {
	APIVersion string            `yaml:"apiVersion" json:"apiVersion"`
	Kind       string            `yaml:"kind" json:"kind"`
	Product    string            `yaml:"product" json:"product"`
	Controller ControllerFixture `yaml:"controller" json:"controller"`
	Subjects   []SubjectFixture  `yaml:"subjects" json:"subjects"`

	digest, path string
}

// Digest is the sha256 of the raw file.
func (s *FixtureSet) Digest() string { return s.digest }

// ControllerFixture says how the controller is installed: the product's
// chart at each version with these values (nil = chart defaults).
type ControllerFixture struct {
	Values map[string]any `yaml:"values,omitempty" json:"values,omitempty"`
	// Namespace is the release namespace (default: the product id).
	Namespace  string                    `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	References []catalog.ConfigReference `yaml:"references,omitempty" json:"references,omitempty"`
}

// FixtureType names a Kubernetes type (version required: the fixture applies
// objects of it).
type FixtureType struct {
	Group   string `yaml:"group,omitempty" json:"group,omitempty"`
	Version string `yaml:"version" json:"version"`
	Kind    string `yaml:"kind" json:"kind"`
}

// SubjectFixture is a minimal object of one subject kind.
type SubjectFixture struct {
	ID     string      `yaml:"id" json:"id"`
	Target FixtureType `yaml:"target" json:"target"`
	// Exercise is the id of the product's declared exercise for the target
	// (products/exercises/<product>.yaml); it must declare a reaction.
	Exercise  string `yaml:"exercise" json:"exercise"`
	Namespace string `yaml:"namespace" json:"namespace"`
	// Prerequisites are applied with the arms (e.g. a self-signed issuer).
	Prerequisites []map[string]any `yaml:"prerequisites,omitempty" json:"prerequisites,omitempty"`
	// Template is the subject object; metadata.name and namespace are set
	// per arm, and NamePlaceholder in any string is replaced by the name.
	Template   map[string]any            `yaml:"template" json:"template"`
	References []catalog.ConfigReference `yaml:"references" json:"references"`
	// Consumers are consumer observations of the subject kind (consumer.go).
	Consumers []ConsumerFixture `yaml:"consumers,omitempty" json:"consumers,omitempty"`
}

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ParseFixtures decodes and validates a fixtures file (unknown fields are rejected).
func ParseFixtures(data []byte) (*FixtureSet, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var s FixtureSet
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("decode product fixtures: %w", err)
	}
	s.digest = domain.Digest(data)
	if err := s.validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *FixtureSet) validate() error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	if s.APIVersion != catalog.APIVersion || s.Kind != FixturesKind {
		bad("apiVersion/kind must be %s/%s", catalog.APIVersion, FixturesKind)
	}
	if !dnsLabel.MatchString(s.Product) {
		bad("product must be a lowercase DNS label, got %q", s.Product)
	}
	if len(s.Subjects) == 0 {
		bad("at least one subject fixture is required")
	}
	refs := func(where string, rs []catalog.ConfigReference, required bool) {
		if required && len(rs) == 0 {
			bad("%s: at least one upstream reference is required", where)
		}
		for _, r := range rs {
			u, err := url.Parse(r.URL)
			if err != nil || u.Scheme != "https" || u.Host == "" || strings.Contains(r.URL, "eval/") {
				bad("%s: reference %q must be an https URL of upstream documentation", where, r.URL)
			}
			if q := strings.TrimSpace(r.Quote); q == "" || len(q) > 400 {
				bad("%s: reference %q needs a short verbatim quote", where, r.URL)
			}
		}
	}
	refs("controller", s.Controller.References, len(s.Controller.Values) > 0)
	seen := map[string]bool{}
	for i, x := range s.Subjects {
		w := fmt.Sprintf("subjects[%d] (%s)", i, x.ID)
		if !dnsLabel.MatchString(x.ID) || seen[x.ID] {
			bad("%s: id must be a unique lowercase DNS label", w)
		}
		seen[x.ID] = true
		if x.Target.Kind == "" || x.Target.Version == "" {
			bad("%s: target kind and version are required", w)
		}
		if x.Exercise == "" {
			bad("%s: exercise is required (a quiet window proves nothing)", w)
		}
		if !dnsLabel.MatchString(x.Namespace) {
			bad("%s: namespace must be a DNS label", w)
		}
		apiVersion := x.Target.Version
		if x.Target.Group != "" {
			apiVersion = x.Target.Group + "/" + x.Target.Version
		}
		if x.Template["apiVersion"] != apiVersion || x.Template["kind"] != x.Target.Kind {
			bad("%s: template must be a %s %s", w, apiVersion, x.Target.Kind)
		}
		for j, p := range x.Prerequisites {
			if p["apiVersion"] == nil || p["kind"] == nil {
				bad("%s: prerequisites[%d] needs apiVersion and kind", w, j)
			}
			if meta, _ := p["metadata"].(map[string]any); meta == nil || meta["name"] == nil {
				bad("%s: prerequisites[%d] needs metadata.name", w, j)
			}
		}
		refs(w, x.References, true)
		cseen := map[string]bool{}
		for j, cf := range x.Consumers {
			cw := fmt.Sprintf("%s consumers[%d] (%s)", w, j, cf.ID)
			if !dnsLabel.MatchString(cf.ID) || cseen[cf.ID] {
				bad("%s: id must be a unique lowercase DNS label", cw)
			}
			cseen[cf.ID] = true
			for _, e := range cf.validate(x) {
				bad("%s: %v", cw, e)
			}
			refs(cw, cf.References, true)
			if cf.Outside != nil {
				refs(cw+" outside", cf.Outside.References, true)
			}
		}
	}
	return errors.Join(errs...)
}

// LoadFixtures loads every <product>.yaml of dir. A missing dir is no
// fixtures; an invalid file is an error (never skipped silently).
func LoadFixtures(dir string) (map[string]*FixtureSet, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return map[string]*FixtureSet{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]*FixtureSet{}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if e.IsDir() || (ext != ".yaml" && ext != ".yml") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		s, err := ParseFixtures(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if strings.TrimSuffix(e.Name(), ext) != s.Product {
			return nil, fmt.Errorf("%s: declares product %q; the file must be named after it", p, s.Product)
		}
		s.path = p
		out[s.Product] = s
	}
	return out, nil
}

// Subject returns the fixture for a group/kind.
func (s *FixtureSet) Subject(group, kind string) (*SubjectFixture, bool) {
	for i := range s.Subjects {
		if s.Subjects[i].Target.Group == group && s.Subjects[i].Target.Kind == kind {
			return &s.Subjects[i], true
		}
	}
	return nil, false
}

// --- arms ------------------------------------------------------------------------------------

// Arm is one synthetic subject object built from the rule.
type Arm struct {
	Name      string // domain.ArmConditionPresent / ArmConditionAbsent
	Object    map[string]any
	Condition string // how the object was built (plain text)
	Predicted string // domain.BehaviourPrevious / BehaviourNew
	// NonDiscriminating says why the arm cannot contrast FROM with TO
	// (discriminate.go); "" when it can.
	NonDiscriminating string
}

// Arm object names (DNS labels, fixed so fixture ids are reproducible).
const (
	armPresentName = "ri-arm-present"
	armAbsentName  = "ri-arm-absent"
)

// BuildArms synthesises the two arms of a rule's exercise from the subject
// fixture: condition-present satisfies the rule's mitigating (overlap)
// condition, condition-absent its exposure condition. Only conditions on the
// subject object itself are synthesised (field unset / set / equals /
// not-equals, no list paths); anything else is reported as why (the exercise
// cannot run) — never approximated.
func BuildArms(rule domain.SemanticAssertion, fx *SubjectFixture) ([]Arm, string) {
	if rule.Subject == nil || rule.Applicability == nil || rule.Change == nil {
		return nil, "the rule has no subject, change or applicability to build a fixture from"
	}
	exposure, why := subjectLeaves(rule.Applicability.Exposure, fx.Target)
	if why != "" {
		return nil, "exposure condition: " + why
	}
	var mitigation []domain.Condition
	if rule.Applicability.Overlap != nil {
		if mitigation, why = subjectLeaves(*rule.Applicability.Overlap, fx.Target); why != "" {
			return nil, "mitigating (overlap) condition: " + why
		}
	} else if len(exposure) == 1 && exposure[0].State == domain.StateUnset {
		// "exposed while unset": the mitigation is the explicit previous value
		mitigation = []domain.Condition{{Op: domain.OpField, Path: exposure[0].Path, State: domain.StateSet}}
	} else {
		return nil, "the rule states no mitigating (overlap) condition and its exposure is not a single unset field"
	}
	present, err := buildObject(fx, armPresentName, mitigation, rule.Change.Before)
	if err != nil {
		return nil, "condition-present arm: " + err.Error()
	}
	absent, err := buildObject(fx, armAbsentName, exposure, rule.Change.Before)
	if err != nil {
		return nil, "condition-absent arm: " + err.Error()
	}
	// the arms must be distinguishable by the rule itself
	if ok, known := holds(exposure, present); !known || ok {
		return nil, "the condition-present object would still satisfy the exposure condition: the rule's conditions do not separate the arms"
	}
	if ok, known := holds(exposure, absent); !known || !ok {
		return nil, "the condition-absent object does not satisfy the exposure condition"
	}
	return []Arm{
		{Name: domain.ArmConditionPresent, Object: present, Condition: describe(mitigation, rule.Change.Before), Predicted: domain.BehaviourPrevious},
		{Name: domain.ArmConditionAbsent, Object: absent, Condition: describe(exposure, rule.Change.Before), Predicted: domain.BehaviourNew},
	}, ""
}

// subjectLeaves flattens a condition on the subject kind into field leaves:
// resource(group, kind) over field leaves, possibly within all(...).
func subjectLeaves(c domain.Condition, t FixtureType) ([]domain.Condition, string) {
	switch c.Op {
	case domain.OpAll:
		var out []domain.Condition
		for _, x := range c.Of {
			ls, why := subjectLeaves(x, t)
			if why != "" {
				return nil, why
			}
			out = append(out, ls...)
		}
		return out, ""
	case domain.OpResource:
		if c.Group != t.Group || c.Kind != t.Kind {
			return nil, fmt.Sprintf("it scopes %s/%s, not the subject kind %s/%s", c.Group, c.Kind, t.Group, t.Kind)
		}
		if c.Name != "" {
			return nil, "it names one resource; a synthetic fixture cannot reproduce a name-specific condition"
		}
		var out []domain.Condition
		for _, x := range c.Of {
			switch {
			case x.Op == domain.OpAll:
				ls, why := subjectLeaves(domain.Condition{Op: domain.OpResource, Group: c.Group, Kind: c.Kind, Of: x.Of}, t)
				if why != "" {
					return nil, why
				}
				out = append(out, ls...)
			case x.Op != domain.OpField:
				return nil, fmt.Sprintf("a %s predicate cannot be synthesised on a fixture object", x.Op)
			case strings.Contains(x.Path, "[]"):
				return nil, fmt.Sprintf("the list path %s cannot be synthesised", x.Path)
			default:
				switch x.State {
				case domain.StateUnset, domain.StateSet, domain.StateEquals, domain.StateNotEquals:
					out = append(out, x)
				default:
					return nil, fmt.Sprintf("field state %s cannot be synthesised", x.State)
				}
			}
		}
		if len(out) == 0 {
			return nil, "it states no field of the subject"
		}
		return out, ""
	}
	return nil, fmt.Sprintf("a %s condition is not a condition on the subject object", c.Op)
}

func decodeJSON(s string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("value %s is not JSON: %w", s, err)
	}
	return v, nil
}

// buildObject copies the template, names it, and makes every leaf hold.
func buildObject(fx *SubjectFixture, name string, leaves []domain.Condition, before *string) (map[string]any, error) {
	obj := substitute(deepCopy(fx.Template), name).(map[string]any)
	meta, _ := obj["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		obj["metadata"] = meta
	}
	meta["name"], meta["namespace"] = name, fx.Namespace
	for _, l := range leaves {
		keys, err := catalog.SplitFieldPath(l.Path)
		if err != nil {
			return nil, err
		}
		switch l.State {
		case domain.StateUnset:
			unsetPath(obj, keys)
		case domain.StateEquals:
			if len(l.Values) == 0 {
				return nil, fmt.Errorf("equals %s names no value", l.Path)
			}
			v, err := decodeJSON(l.Values[0])
			if err != nil {
				return nil, err
			}
			setPath(obj, keys, v)
		case domain.StateSet, domain.StateNotEquals:
			if before == nil {
				return nil, fmt.Errorf("%s %s needs a concrete value and the rule states no previous value", l.State, l.Path)
			}
			v, err := decodeJSON(*before)
			if err != nil {
				return nil, err
			}
			setPath(obj, keys, v)
		}
	}
	if ok, known := holds(leaves, obj); !known || !ok {
		return nil, fmt.Errorf("the built object does not satisfy %s", describe(leaves, before))
	}
	return obj, nil
}

// holds evaluates field leaves on an object (known=false when undecidable).
func holds(leaves []domain.Condition, obj map[string]any) (ok, known bool) {
	for _, l := range leaves {
		keys, err := catalog.SplitFieldPath(l.Path)
		if err != nil {
			return false, false
		}
		v, present := getPath(obj, keys)
		var r bool
		switch l.State {
		case domain.StateUnset:
			r = !present
		case domain.StateSet:
			r = present
		case domain.StateEquals, domain.StateNotEquals:
			eq := false
			for _, raw := range l.Values {
				w, err := decodeJSON(raw)
				if err != nil {
					return false, false
				}
				a, _ := json.Marshal(v)
				b, _ := json.Marshal(w)
				eq = eq || (present && bytes.Equal(a, b))
			}
			r = present && (eq == (l.State == domain.StateEquals))
		default:
			return false, false
		}
		if !r {
			return false, true
		}
	}
	return true, true
}

func describe(leaves []domain.Condition, before *string) string {
	var parts []string
	for _, l := range leaves {
		switch l.State {
		case domain.StateUnset:
			parts = append(parts, l.Path+" unset")
		case domain.StateEquals:
			parts = append(parts, l.Path+" = "+strings.Join(l.Values, " | "))
		default:
			b := "?"
			if before != nil {
				b = *before
			}
			parts = append(parts, fmt.Sprintf("%s %s (set to the previous value %s)", l.Path, l.State, b))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, " and ")
}

func deepCopy(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

func substitute(v any, name string) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			t[k] = substitute(x, name)
		}
		return t
	case []any:
		for i, x := range t {
			t[i] = substitute(x, name)
		}
		return t
	case string:
		return strings.ReplaceAll(t, NamePlaceholder, name)
	}
	return v
}

func getPath(obj map[string]any, keys []string) (any, bool) {
	var cur any = obj
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[k]; !ok {
			return nil, false
		}
	}
	return cur, cur != nil
}

func setPath(obj map[string]any, keys []string, v any) {
	cur := obj
	for _, k := range keys[:len(keys)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[k] = next
		}
		cur = next
	}
	cur[keys[len(keys)-1]] = v
}

func unsetPath(obj map[string]any, keys []string) {
	cur := obj
	for _, k := range keys[:len(keys)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return
		}
		cur = next
	}
	delete(cur, keys[len(keys)-1])
}

// Produced returns the object ids a controller produces from obj, from the
// product's declared `produces` relationships (core-group targets only: the
// relationship does not state a version for other groups).
func Produced(rels []catalog.Relationship, t FixtureType, obj map[string]any) (ids []string, notes []string) {
	meta, _ := obj["metadata"].(map[string]any)
	ns, _ := meta["namespace"].(string)
	for _, r := range rels {
		if r.Kind != catalog.RelProduces || r.From.Group != t.Group || r.From.Kind != t.Kind {
			continue
		}
		keys, err := catalog.SplitFieldPath(r.Path)
		if err != nil {
			notes = append(notes, fmt.Sprintf("relationship %s: %v", r.ID, err))
			continue
		}
		v, ok := getPath(obj, keys)
		name, _ := v.(string)
		if !ok || name == "" {
			notes = append(notes, fmt.Sprintf("relationship %s: the fixture object sets no %s", r.ID, r.Path))
			continue
		}
		if r.To.Group != "" {
			notes = append(notes, fmt.Sprintf("relationship %s produces a %s/%s whose version is not declared: not observed", r.ID, r.To.Group, r.To.Kind))
			continue
		}
		ids = append(ids, "core/v1/"+r.To.Kind+"/"+ns+"/"+name)
	}
	sort.Strings(ids)
	return ids, notes
}

// objectID is the runtime object id of a fixture object.
func objectID(obj map[string]any) string {
	apiVersion, _ := obj["apiVersion"].(string)
	kind, _ := obj["kind"].(string)
	meta, _ := obj["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	ns, _ := meta["namespace"].(string)
	g, v := "core", apiVersion
	if i := strings.LastIndexByte(apiVersion, '/'); i >= 0 {
		g, v = apiVersion[:i], apiVersion[i+1:]
	}
	return g + "/" + v + "/" + kind + "/" + ns + "/" + name
}
