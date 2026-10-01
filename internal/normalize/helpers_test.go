package normalize

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func mustRe(s string) *regexp.Regexp { return regexp.MustCompile(s) }

// fixture reads a file below testdata/.
func fixture(t testing.TB, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", rel))
	if err != nil {
		t.Fatalf("read fixture %s: %v", rel, err)
	}
	return b
}
