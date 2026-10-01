package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// TestGoldenEdgesMatchSchema checks that the real upgrade edges recorded as
// golden files conform to the published UpgradeEdge JSON Schema, so the
// schema is a contract the pipeline actually honours.
func TestGoldenEdgesMatchSchema(t *testing.T) {
	schemaPath, err := filepath.Abs(filepath.Join("..", "..", "schemas", "upgrade-edge.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := jsonschema.NewCompiler().Compile(schemaPath)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	files, err := filepath.Glob(filepath.Join("testdata", "e2e", "golden", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden edges found: %v", err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			fh, err := os.Open(f)
			if err != nil {
				t.Fatal(err)
			}
			defer fh.Close()
			inst, err := jsonschema.UnmarshalJSON(fh)
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(inst); err != nil {
				t.Fatalf("%s does not match schemas/upgrade-edge.schema.json: %v", f, err)
			}
		})
	}
}
