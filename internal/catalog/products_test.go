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

// TestCheckedInProducts validates every checked-in product definition both
// against the JSON Schema (for external tooling) and the Go validator.
func TestCheckedInProducts(t *testing.T) {
	dir := filepath.Join("..", "..", "products")
	cat, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.List()) == 0 {
		t.Fatal("no product definitions found")
	}
	schema := compileSchema(t, filepath.Join("..", "..", "schemas", "product-definition.schema.json"))
	for _, d := range cat.List() {
		t.Run(d.ID, func(t *testing.T) {
			rep := Validate(d)
			for _, i := range rep.Issues {
				t.Log(i.String())
			}
			if !rep.OK() {
				t.Fatalf("%s: invalid definition", d.Path())
			}
			raw, err := os.ReadFile(d.Path())
			if err != nil {
				t.Fatal(err)
			}
			var doc any
			if err := yaml.Unmarshal(raw, &doc); err != nil {
				t.Fatal(err)
			}
			// round-trip through JSON so the validator sees JSON types
			b, _ := json.Marshal(doc)
			inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(b)))
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(inst); err != nil {
				t.Fatalf("schema validation: %v", err)
			}
		})
	}
}

func compileSchema(t *testing.T, path string) *jsonschema.Schema {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	s, err := c.Compile(abs)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return s
}
