package normalize

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestWeakSectionCategoryRefinedByKeyword(t *testing.T) {
	md := []byte("## v1.18.0\n\n### Other (Cleanup or Flake)\n\n- Remove deprecated feature gate `ValidateCAA`.\n- Use `slices.Contains` to simplify code\n- Bump golang.org/x/net to fix CVE-2025-22872\n")
	items, _, err := ParseNotes(DocInput{SourceID: "notes", Role: domain.RoleReleaseNotes, Release: "1.18.0", URI: "u", Content: md}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]domain.Category{
		"Remove deprecated feature gate `ValidateCAA`.": domain.CategoryDeprecation,
		"Use `slices.Contains` to simplify code":        domain.CategoryOther,
		"Bump golang.org/x/net to fix CVE-2025-22872":   domain.CategorySecurity,
	}
	if len(items) != len(want) {
		t.Fatalf("got %d items", len(items))
	}
	for _, it := range items {
		if c, ok := want[it.Text]; !ok || c != it.Category {
			t.Errorf("%q: got %s (rule %s), want %s", it.Text, it.Category, it.Classification.Rule, c)
		}
		if it.Category != domain.CategoryOther && it.Classification.Method != domain.MethodHeuristic {
			t.Errorf("%q: refined category must be heuristic, got %s", it.Text, it.Classification.Method)
		}
	}
}
