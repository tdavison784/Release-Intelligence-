package knowledge

import (
	"os"
	"path/filepath"
	"testing"
)

// The proxy lane's shadow store and its evaluation view (proxy-incl-shadow)
// are knowledge stores too: the same blind-authoring scan applies.
func TestProxyShadowStoresNeverReferenceEvalCases(t *testing.T) {
	root := filepath.Join("..", "..")
	ids := caseIDs(t, root)
	for _, rel := range []string{"docs/phase3/learning-loop/proxy-shadow/knowledge", "docs/phase3/learning-loop/proxy-shadow/eval-knowledge"} {
		dir := filepath.Join(root, rel)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		bad, err := scanKnowledge(dir, ids)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range bad {
			t.Errorf("%s: %s", rel, b)
		}
	}
}
