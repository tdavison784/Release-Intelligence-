package catalog

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Exercises are declarative, upstream-documented triggers that make a
// product's controller act on the objects it manages — a certificate
// renewal, an object update the admission webhook re-validates, a rollout
// restart. The investigation's runtime validator
// (internal/investigate/runtime) runs them in a throwaway cluster before and
// after an upgrade: many breaking changes only manifest when something
// happens, and a quiet window without the relevant exercise proves nothing.
//
// They live beside the product definitions, one file per product
// (products/exercises/<product>.yaml, kind ProductExercises), not inside
// them: a product definition's digest pins its ingested releases and upgrade
// edges, and a validation-time trigger must not invalidate release evidence.
// Each entry cites upstream documentation, never an evaluation case.

// ExercisesKind is the document kind of an exercises file.
const ExercisesKind = "ProductExercises"

// ExerciseAction is what an exercise does to each target object.
type ExerciseAction string

const (
	// ActionStatusCondition sets a status condition through the status
	// subresource (cert-manager's documented manual renewal sets Issuing=True).
	ActionStatusCondition ExerciseAction = "status-condition"
	// ActionAnnotate sets an annotation (an update the controller and its
	// admission webhook process like any other change).
	ActionAnnotate ExerciseAction = "annotate"
	// ActionRolloutRestart restarts a workload's pods (kubectl rollout restart).
	ActionRolloutRestart ExerciseAction = "rollout-restart"
)

// ExerciseActions lists every action.
var ExerciseActions = []ExerciseAction{ActionStatusCondition, ActionAnnotate, ActionRolloutRestart}

// ExerciseCondition is the condition a status-condition exercise sets.
type ExerciseCondition struct {
	Type    string `yaml:"type" json:"type"`
	Status  string `yaml:"status" json:"status"` // "True" | "False" | "Unknown"
	Reason  string `yaml:"reason" json:"reason"`
	Message string `yaml:"message,omitempty" json:"message,omitempty"`
}

// Exercise is one declared trigger.
type Exercise struct {
	ID      string `yaml:"id" json:"id"`
	Summary string `yaml:"summary" json:"summary"`
	// Target is the object kind the action is applied to; every object of
	// that kind in the environment is a target.
	Target ObjectType     `yaml:"target" json:"target"`
	Action ExerciseAction `yaml:"action" json:"action"`
	// Condition (status-condition) and Annotation (annotate: the key; the
	// value is the run's exercise token) parameterise the action.
	Condition  *ExerciseCondition `yaml:"condition,omitempty" json:"condition,omitempty"`
	Annotation string             `yaml:"annotation,omitempty" json:"annotation,omitempty"`
	// Families are the change-item families the exercise exercises
	// (domain.ChangeItemFamily). Empty means every family. An item counts as
	// exercised only when its family is listed and a target lies on its
	// impact paths.
	Families []string `yaml:"families,omitempty" json:"families,omitempty"`
	// SettleSeconds is how long the controller gets to act before the
	// observation (0 = the validator's default).
	SettleSeconds int `yaml:"settleSeconds,omitempty" json:"settleSeconds,omitempty"`
	// References are the upstream documentation the trigger rests on.
	References []ConfigReference `yaml:"references" json:"references"`
}

// ExerciseSet is one exercises file.
type ExerciseSet struct {
	APIVersion string     `yaml:"apiVersion" json:"apiVersion"`
	Kind       string     `yaml:"kind" json:"kind"`
	Product    string     `yaml:"product" json:"product"`
	Exercises  []Exercise `yaml:"exercises" json:"exercises"`

	digest string
	path   string
}

// Digest returns the sha256 of the raw document.
func (s *ExerciseSet) Digest() string { return s.digest }

// Path returns the file the set was loaded from ("" if not from disk).
func (s *ExerciseSet) Path() string { return s.path }

// maxSettleSeconds bounds an exercise's settle window.
const maxSettleSeconds = 600

// ParseExercises decodes an exercises file. Unknown fields are rejected.
func ParseExercises(data []byte) (*ExerciseSet, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var s ExerciseSet
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("decode product exercises: %w", err)
	}
	s.digest = domain.Digest(data)
	return &s, nil
}

// LoadExercises loads every *.yaml / *.yml file of dir (products/exercises)
// and validates it. A missing dir is no exercises; an unreadable, invalid or
// duplicate file is an error (exercises decide whether a runtime validation
// may pass, so a broken declaration must not be skipped silently).
func LoadExercises(dir string) ([]*ExerciseSet, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*ExerciseSet
	seen := map[string]string{}
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if e.IsDir() || (ext != ".yaml" && ext != ".yml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		s, err := ParseExercises(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		s.path = path
		if rep := ValidateExercises(s); !rep.OK() {
			return nil, fmt.Errorf("%s: invalid exercises: %v", path, rep.Errors())
		}
		if base := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())); base != s.Product {
			return nil, fmt.Errorf("%s: declares product %q; the file must be named %s%s", path, s.Product, s.Product, ext)
		}
		if prev, dup := seen[s.Product]; dup {
			return nil, fmt.Errorf("%s: product %q already declared by %s", path, s.Product, prev)
		}
		seen[s.Product] = path
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Product < out[j].Product })
	return out, nil
}

// ValidateExercises checks an exercises file.
func ValidateExercises(s *ExerciseSet) ValidationReport {
	v := &validator{r: ValidationReport{Product: s.Product, File: s.path}}
	if s.APIVersion != APIVersion {
		v.errf("apiVersion", "must be %q, got %q", APIVersion, s.APIVersion)
	}
	if s.Kind != ExercisesKind {
		v.errf("kind", "must be %q, got %q", ExercisesKind, s.Kind)
	}
	if !idRe.MatchString(s.Product) {
		v.errf("product", "must be a lowercase DNS label, got %q", s.Product)
	}
	if len(s.Exercises) == 0 {
		v.errf("exercises", "at least one exercise is required")
	}
	families := map[string]bool{}
	for _, f := range domain.ChangeItemFamilies {
		families[string(f)] = true
	}
	seen := map[string]bool{}
	for i, x := range s.Exercises {
		p := fmt.Sprintf("exercises[%d]", i)
		switch {
		case !configSourceIDRe.MatchString(x.ID):
			v.errf(p+".id", "must be a lowercase DNS label, got %q", x.ID)
		case seen[x.ID]:
			v.errf(p+".id", "duplicate exercise id %q", x.ID)
		}
		seen[x.ID] = true
		if strings.TrimSpace(x.Summary) == "" {
			v.errf(p+".summary", "required")
		}
		if strings.TrimSpace(x.Target.Kind) == "" {
			v.errf(p+".target.kind", "required")
		}
		known := false
		for _, a := range ExerciseActions {
			known = known || a == x.Action
		}
		if !known {
			v.errf(p+".action", "unknown action %q (one of %v)", x.Action, ExerciseActions)
		}
		switch x.Action {
		case ActionStatusCondition:
			c := x.Condition
			if c == nil || strings.TrimSpace(c.Type) == "" || strings.TrimSpace(c.Reason) == "" {
				v.errf(p+".condition", "status-condition needs a condition with type and reason")
			} else if c.Status != "True" && c.Status != "False" && c.Status != "Unknown" {
				v.errf(p+".condition.status", "must be True, False or Unknown, got %q", c.Status)
			}
		case ActionAnnotate:
			if !annotationKeyOK(x.Annotation) {
				v.errf(p+".annotation", "annotate needs a qualified annotation key (prefix/name), got %q", x.Annotation)
			}
		case ActionRolloutRestart:
			if x.Target.Group != "apps" || (x.Target.Kind != "Deployment" && x.Target.Kind != "StatefulSet" && x.Target.Kind != "DaemonSet") {
				v.errf(p+".target", "rollout-restart targets apps Deployment, StatefulSet or DaemonSet, got %q", x.Target.Group+"/"+x.Target.Kind)
			}
		}
		if x.Action != ActionStatusCondition && x.Condition != nil {
			v.errf(p+".condition", "only a status-condition exercise has a condition")
		}
		if x.Action != ActionAnnotate && x.Annotation != "" {
			v.errf(p+".annotation", "only an annotate exercise has an annotation")
		}
		for j, f := range x.Families {
			if !families[f] {
				v.errf(fmt.Sprintf("%s.families[%d]", p, j), "unknown change-item family %q", f)
			}
		}
		if x.SettleSeconds < 0 || x.SettleSeconds > maxSettleSeconds {
			v.errf(p+".settleSeconds", "must be 0-%d, got %d", maxSettleSeconds, x.SettleSeconds)
		}
		if len(x.References) == 0 {
			v.errf(p+".references", "at least one upstream documentation reference is required")
		}
		for j, ref := range x.References {
			rp := fmt.Sprintf("%s.references[%d]", p, j)
			u, err := url.Parse(ref.URL)
			if err != nil || u.Scheme != "https" || u.Host == "" {
				v.errf(rp+".url", "must be an https URL of upstream documentation, got %q", ref.URL)
			}
			if strings.Contains(ref.URL, "eval/cases") || strings.Contains(ref.URL, "eval/results") || strings.Contains(ref.URL, "eval/investigations") {
				v.errf(rp+".url", "must not cite evaluation data")
			}
			if q := strings.TrimSpace(ref.Quote); q == "" || len(q) > maxConfigQuote {
				v.errf(rp+".quote", "a short verbatim quote (1-%d characters) is required", maxConfigQuote)
			}
		}
	}
	return v.r
}

// annotationKeyOK reports whether k is a prefixed annotation key
// (<dns subdomain>/<name>): exercises never write unprefixed keys.
func annotationKeyOK(k string) bool {
	prefix, name, ok := strings.Cut(k, "/")
	if !ok || prefix == "" || name == "" || len(name) > 63 || strings.Contains(name, "/") {
		return false
	}
	for _, part := range strings.Split(prefix, ".") {
		if !idRe.MatchString(part) {
			return false
		}
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
