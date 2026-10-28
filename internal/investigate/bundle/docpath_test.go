package bundle

import (
	"strings"
	"testing"
)

// A URL path with a run of dots (a GitHub compare range) must still give an
// artifact path the domain accepts: relative, without "..".
func TestDocPathDotRuns(t *testing.T) {
	for _, u := range []string{
		"https://github.com/example/project/compare/v0.18.0...v0.19.0",
		"https://example.com/docs/a..b/c",
	} {
		p := docPath("product", u)
		if strings.Contains(p, "..") || strings.HasPrefix(p, "/") || !strings.HasPrefix(p, "upstream/product/") {
			t.Errorf("docPath(%q) = %q", u, p)
		}
	}
	if got, want := docPath("product", "https://example.com/docs/v1.2.3/notes.md"), "upstream/product/example.com/docs/v1.2.3/notes.md"; got != want {
		t.Errorf("single dots must be kept: got %q, want %q", got, want)
	}
}
