package semantic

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// The prompt teaches exactly the contract: every enum value has a doc entry,
// and every example validates with the domain's own Validate.
func TestVocabularyCoversAndValidatesTheContract(t *testing.T) {
	fams := map[domain.SubjectFamily]bool{}
	for _, d := range familyDocs {
		fams[d.Family] = true
		s := d.Example
		s.Product = "example"
		if err := s.Validate(); err != nil {
			t.Errorf("family %s example invalid: %v", d.Family, err)
		}
	}
	for _, f := range domain.SubjectFamilies {
		if !fams[f] {
			t.Errorf("family %s has no prompt entry", f)
		}
	}
	kinds := map[domain.ChangeKind]bool{}
	for _, d := range changeDocs {
		kinds[d.Kind] = true
	}
	for _, k := range domain.ChangeKinds {
		if !kinds[k] {
			t.Errorf("change kind %s has no prompt entry", k)
		}
	}
	cons := map[domain.ConsequenceKind]bool{}
	for _, d := range consequenceDocs {
		cons[d.Kind] = true
	}
	for _, k := range domain.ConsequenceKinds {
		if !cons[k] {
			t.Errorf("consequence kind %s has no prompt entry", k)
		}
	}
	ops := map[domain.ConditionOp]bool{}
	for _, d := range opDocs {
		ops[d.Op] = true
		c := d.Example
		if c.Op != d.Op {
			t.Errorf("op %s example has op %s", d.Op, c.Op)
		}
		switch c.Op {
		case domain.OpField, domain.OpTextLine, domain.OpRef: // scoped: validate inside a resource
			c = domain.Condition{Op: domain.OpResource, Kind: "Widget", Of: []domain.Condition{c}}
		}
		if err := c.Validate(); err != nil {
			t.Errorf("op %s example invalid: %v", d.Op, err)
		}
	}
	for _, op := range domain.ConditionOps {
		if !ops[op] {
			t.Errorf("op %s has no prompt entry", op)
		}
	}
}

func TestAnswerSchemaShape(t *testing.T) {
	for _, task := range domain.ProposalTasks {
		raw := AnswerSchema(task)
		if raw == nil {
			t.Fatalf("task %s: no schema", task)
		}
		s := string(raw)
		// the structured-output dialect: no length bounds, no recursion
		for _, forbidden := range []string{"maxLength", "minLength", "maxItems", "minItems", "minimum", "maximum", `"exposedClass"`, `"not-affected"`} {
			if strings.Contains(s, forbidden) {
				t.Errorf("task %s schema contains %s", task, forbidden)
			}
		}
		if _, err := compiledSchema(task); err != nil {
			t.Errorf("task %s schema does not compile: %v", task, err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if sc, ok := m["properties"].(map[string]any)["suggestedClass"].(map[string]any); ok {
			ar := contains(toStrings(sc["enum"].([]any)), "action-required")
			if ar != containsAspect(domain.TaskAspects(task), domain.AspectConsequence) {
				t.Errorf("task %s: action-required offered=%v (only with a consequence, PO-2)", task, ar)
			}
		}
		conf := m["properties"].(map[string]any)["confidence"].(map[string]any)["enum"].([]any)
		if len(conf) != 2 || conf[0] != "medium" || conf[1] != "low" {
			t.Errorf("task %s: confidence enum %v (a model reports medium or low)", task, conf)
		}
		if defs, ok := m["$defs"].(map[string]any); ok {
			last := defs["condition4"].(map[string]any)["properties"].(map[string]any)
			if _, hasOf := last["of"]; hasOf {
				t.Errorf("task %s: the last condition level must not recurse", task)
			}
		}
		// every object closes its properties (structured outputs require it)
		walk(m, func(o map[string]any) {
			if o["type"] == "object" && o["additionalProperties"] != false {
				t.Errorf("task %s: open object %v", task, o)
			}
		})
	}
	if AnswerSchema("nonsense") != nil {
		t.Error("an unknown task has no schema")
	}
}

func walk(v any, f func(map[string]any)) {
	switch x := v.(type) {
	case map[string]any:
		f(x)
		for _, y := range x {
			walk(y, f)
		}
	case []any:
		for _, y := range x {
			walk(y, f)
		}
	}
}

// The narrowed request schema enumerates exactly the evidence ids shown.
func TestRequestSchemaNarrowsCitations(t *testing.T) {
	raw := mustJSON(answerSchema(domain.TaskFull, []string{"ev-1", "ev-2"}, nil))
	inst, _ := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	c := jsonschema.NewCompiler()
	if err := c.AddResource("x.json", inst); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("x.json")
	if err != nil {
		t.Fatal(err)
	}
	abstain := `{"statement":"","subject":{"determination":"undetermined","reason":"r"},"change":{"determination":"undetermined","reason":"r"},` +
		`"applicability":{"determination":"undetermined","reason":"r"},"consequence":{"determination":"undetermined","reason":"r"},"confidence":"low","citations":[%s]}`
	for cites, ok := range map[string]bool{`"ev-1"`: true, `"ev-9"`: false} {
		doc, _ := jsonschema.UnmarshalJSON(strings.NewReader(strings.Replace(abstain, "%s", cites, 1)))
		if err := s.Validate(doc); (err == nil) != ok {
			t.Errorf("citations %s: valid=%v, want %v (%v)", cites, err == nil, ok, err)
		}
	}
}

func toStrings(xs []any) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i], _ = x.(string)
	}
	return out
}
