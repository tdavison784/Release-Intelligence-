package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

var exRef = []ConfigReference{{URL: "https://example.io/docs/renew", Quote: "Marks the object for renewal."}}

func validExercise() Exercise {
	return Exercise{ID: "renew", Summary: "renew every Cert", Target: ObjectType{Group: "demo.io", Kind: "Cert"},
		Action: ActionStatusCondition, Condition: &ExerciseCondition{Type: "Issuing", Status: "True", Reason: "Manual"},
		Families: []string{"behavior"}, SettleSeconds: 30, References: exRef}
}

func exSet(xs ...Exercise) *ExerciseSet {
	return &ExerciseSet{APIVersion: APIVersion, Kind: ExercisesKind, Product: "demo", Exercises: xs}
}

func TestExercisesValid(t *testing.T) {
	ann := Exercise{ID: "touch", Summary: "update", Target: ObjectType{Group: "networking.k8s.io", Kind: "Ingress"},
		Action: ActionAnnotate, Annotation: "ri.dev/runtime-exercise", Families: []string{"behavior"}, References: exRef,
		Reaction: &ExerciseReaction{Kind: ReactionEvent, Reason: "Sync"}}
	rr := Exercise{ID: "restart", Summary: "restart", Target: ObjectType{Group: "apps", Kind: "Deployment"},
		Action: ActionRolloutRestart, Families: []string{"procedure"}, References: exRef,
		Reaction: &ExerciseReaction{Kind: ReactionFieldChanged, Path: "status.observedGeneration"}}
	adm := Exercise{ID: "admission", Summary: "dry-run", Target: ObjectType{Group: "networking.k8s.io", Kind: "Ingress"},
		Action: ActionAdmissionDryRun, Families: []string{"behavior"}, References: exRef}
	if rep := ValidateExercises(exSet(validExercise(), ann, rr, adm)); !rep.OK() {
		t.Fatalf("valid exercises rejected: %v", rep.Errors())
	}
}

func TestExercisesRejectBadDeclarations(t *testing.T) {
	mut := func(f func(*Exercise)) Exercise {
		x := validExercise()
		f(&x)
		return x
	}
	cases := map[string]struct {
		set  *ExerciseSet
		want string
	}{
		"bad api version":    {&ExerciseSet{APIVersion: "v1", Kind: ExercisesKind, Product: "demo", Exercises: []Exercise{validExercise()}}, "apiVersion"},
		"bad kind":           {&ExerciseSet{APIVersion: APIVersion, Kind: "Other", Product: "demo", Exercises: []Exercise{validExercise()}}, "kind"},
		"bad product":        {&ExerciseSet{APIVersion: APIVersion, Kind: ExercisesKind, Product: "Demo", Exercises: []Exercise{validExercise()}}, "lowercase DNS label"},
		"no exercises":       {exSet(), "at least one exercise"},
		"bad id":             {exSet(mut(func(x *Exercise) { x.ID = "Bad_ID" })), "lowercase DNS label"},
		"duplicate id":       {exSet(validExercise(), validExercise()), "duplicate exercise id"},
		"no summary":         {exSet(mut(func(x *Exercise) { x.Summary = " " })), "summary"},
		"no target kind":     {exSet(mut(func(x *Exercise) { x.Target.Kind = "" })), "target.kind"},
		"unknown action":     {exSet(mut(func(x *Exercise) { x.Action = "delete" })), "unknown action"},
		"no condition":       {exSet(mut(func(x *Exercise) { x.Condition = nil })), "needs a condition"},
		"no reason":          {exSet(mut(func(x *Exercise) { x.Condition.Reason = "" })), "needs a condition"},
		"bad status":         {exSet(mut(func(x *Exercise) { x.Condition.Status = "yes" })), "True, False or Unknown"},
		"stray annotation":   {exSet(mut(func(x *Exercise) { x.Annotation = "a.io/b" })), "only an annotate exercise"},
		"unqualified annot.": {exSet(mut(func(x *Exercise) { x.Action, x.Condition, x.Annotation = ActionAnnotate, nil, "touch" })), "qualified annotation key"},
		"stray condition": {exSet(mut(func(x *Exercise) {
			x.Action, x.Annotation = ActionAnnotate, "a.io/b"
		})), "only a status-condition exercise"},
		"restart non-workload": {exSet(mut(func(x *Exercise) { x.Action, x.Condition = ActionRolloutRestart, nil })), "rollout-restart targets"},
		"admission with a reaction": {exSet(mut(func(x *Exercise) {
			x.Action, x.Condition, x.Reaction = ActionAdmissionDryRun, nil, &ExerciseReaction{Kind: ReactionEvent, Reason: "Sync"}
		})), "no controller reaction"},
		"unknown family":            {exSet(mut(func(x *Exercise) { x.Families = []string{"behaviour"} })), "unknown change-item family"},
		"no families":               {exSet(mut(func(x *Exercise) { x.Families = nil })), "at least one change-item family"},
		"unknown reaction":          {exSet(mut(func(x *Exercise) { x.Reaction = &ExerciseReaction{Kind: "log"} })), "reaction.kind"},
		"field reaction, no path":   {exSet(mut(func(x *Exercise) { x.Reaction = &ExerciseReaction{Kind: ReactionFieldChanged} })), "reaction.path"},
		"event reaction, no reason": {exSet(mut(func(x *Exercise) { x.Reaction = &ExerciseReaction{Kind: ReactionEvent} })), "reaction.reason"},
		"event reaction with path": {exSet(mut(func(x *Exercise) {
			x.Reaction = &ExerciseReaction{Kind: ReactionEvent, Reason: "Sync", Path: "status.x"}
		})), "not a field of an event reaction"},
		"settle too long": {exSet(mut(func(x *Exercise) { x.SettleSeconds = 601 })), "settleSeconds"},
		"no references":   {exSet(mut(func(x *Exercise) { x.References = nil })), "at least one upstream"},
		"http reference":  {exSet(mut(func(x *Exercise) { x.References = []ConfigReference{{URL: "http://x.io/a", Quote: "q"}} })), "https URL"},
		"eval reference": {exSet(mut(func(x *Exercise) {
			x.References = []ConfigReference{{URL: "https://github.com/o/r/blob/main/eval/investigations/x/NOTES.md", Quote: "q"}}
		})), "evaluation data"},
		"empty quote": {exSet(mut(func(x *Exercise) { x.References = []ConfigReference{{URL: "https://x.io/a", Quote: " "}} })), "verbatim quote"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rep := ValidateExercises(c.set)
			if rep.OK() {
				t.Fatalf("accepted")
			}
			found := false
			for _, i := range rep.Errors() {
				found = found || strings.Contains(i.Path+": "+i.Message, c.want)
			}
			if !found {
				t.Fatalf("errors %v do not mention %q", rep.Errors(), c.want)
			}
		})
	}
}

// TestCheckedInExercises validates every products/exercises file with the Go
// validator and the JSON Schema, and requires its product to exist.
func TestCheckedInExercises(t *testing.T) {
	sets, err := LoadExercises(filepath.Join("..", "..", "products", "exercises"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) == 0 {
		t.Fatal("no exercises found")
	}
	cat, err := LoadDir(filepath.Join("..", "..", "products"))
	if err != nil {
		t.Fatal(err)
	}
	schema := compileSchema(t, filepath.Join("..", "..", "schemas", "product-exercises.schema.json"))
	for _, s := range sets {
		t.Run(s.Product, func(t *testing.T) {
			if _, ok := cat.Get(s.Product); !ok {
				t.Fatalf("exercises for unknown product %q", s.Product)
			}
			if s.Digest() == "" || s.Path() == "" {
				t.Fatal("digest and path are set by the loader")
			}
			raw, err := os.ReadFile(s.Path())
			if err != nil {
				t.Fatal(err)
			}
			if err := schemaCheck(schema, raw); err != nil {
				t.Fatalf("schema validation: %v", err)
			}
		})
	}
}

func schemaCheck(schema *jsonschema.Schema, raw []byte) error {
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return err
	}
	b, _ := json.Marshal(doc)
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	return schema.Validate(inst)
}

func TestExercisesSchemaAndParse(t *testing.T) {
	schema := compileSchema(t, filepath.Join("..", "..", "schemas", "product-exercises.schema.json"))
	doc := `apiVersion: ri.dev/v1alpha1
kind: ProductExercises
product: demo
exercises:
  - id: renew
    summary: renew every Cert
    target: {group: demo.io, kind: Cert}
    action: status-condition
    condition: {type: Issuing, status: "True", reason: Manual}
    families: [behavior]
    references:
      - url: https://example.io/docs/renew
        quote: Marks the object for renewal.
`
	if err := schemaCheck(schema, []byte(doc)); err != nil {
		t.Fatalf("schema rejects a valid exercise: %v", err)
	}
	s, err := ParseExercises([]byte(doc))
	if err != nil || len(s.Exercises) != 1 || s.Exercises[0].Condition.Type != "Issuing" {
		t.Fatalf("parse: %v %+v", err, s)
	}
	if rep := ValidateExercises(s); !rep.OK() {
		t.Fatalf("go validator rejects it: %v", rep.Errors())
	}
	for name, bad := range map[string]string{
		"unknown action":   strings.Replace(doc, "action: status-condition", "action: delete", 1),
		"no condition":     strings.Replace(doc, "    condition: {type: Issuing, status: \"True\", reason: Manual}\n", "", 1),
		"unknown family":   strings.Replace(doc, "[behavior]", "[behaviour]", 1),
		"unknown field":    strings.Replace(doc, "families: [behavior]", "families: [behavior]\n    bogus: 1", 1),
		"http reference":   strings.Replace(doc, "https://example.io", "http://example.io", 1),
		"annotation on it": strings.Replace(doc, "families: [behavior]", "families: [behavior]\n    annotation: a.io/b", 1),
		"no families":      strings.Replace(doc, "    families: [behavior]\n", "", 1),
		"empty families":   strings.Replace(doc, "families: [behavior]", "families: []", 1),
		"reaction no path": strings.Replace(doc, "families: [behavior]", "families: [behavior]\n    reaction: {kind: field-changed}", 1),
	} {
		if schemaCheck(schema, []byte(bad)) == nil {
			t.Errorf("schema accepts an exercise with %s", name)
		}
	}
	if _, err := ParseExercises([]byte(strings.Replace(doc, "families: [behavior]", "families: [behavior]\n    bogus: 1", 1))); err == nil {
		t.Error("parser accepts an unknown field")
	}
}

func TestLoadExercisesRejectsMisnamedAndInvalidFiles(t *testing.T) {
	good := `apiVersion: ri.dev/v1alpha1
kind: ProductExercises
product: demo
exercises:
  - id: renew
    summary: renew
    target: {kind: Cert}
    action: annotate
    annotation: a.io/b
    families: [behavior]
    references: [{url: "https://example.io/a", quote: q}]
`
	write := func(dir, name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	write(dir, "demo.yaml", good)
	write(dir, "README.md", "not yaml")
	if sets, err := LoadExercises(dir); err != nil || len(sets) != 1 {
		t.Fatalf("load: %v %v", sets, err)
	}
	if sets, err := LoadExercises(filepath.Join(dir, "absent")); err != nil || sets != nil {
		t.Fatalf("a missing directory is no exercises: %v %v", sets, err)
	}
	misnamed := t.TempDir()
	write(misnamed, "other.yaml", good)
	if _, err := LoadExercises(misnamed); err == nil || !strings.Contains(err.Error(), "must be named") {
		t.Fatalf("misnamed file accepted: %v", err)
	}
	invalid := t.TempDir()
	write(invalid, "demo.yaml", strings.Replace(good, "annotation: a.io/b", "annotation: b", 1))
	if _, err := LoadExercises(invalid); err == nil || !strings.Contains(err.Error(), "invalid exercises") {
		t.Fatalf("invalid file accepted: %v", err)
	}
}
