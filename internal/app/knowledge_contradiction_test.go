package app

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Lane backdoor (D-0029): the checked-in store still loads, and every active
// pair of one product, release and subject that contradicts each other is a
// loader warning naming both. The stored pairs are reported, not changed.
func TestCheckedInKnowledgeContradictions(t *testing.T) {
	ks, err := LoadKnowledge(filepath.Join("..", "..", "knowledge"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, w := range ks.Warnings {
		if strings.HasPrefix(w, "knowledge contradiction: ") {
			got = append(got, w)
		}
		if strings.HasPrefix(w, "fact refused: ") {
			t.Errorf("a stored fact is refused: %s", w)
		}
	}
	var pairs []string
	for _, x := range domain.Contradictions(ks.Facts) {
		pairs = append(pairs, x.A+" "+x.B)
	}
	sort.Strings(pairs)
	// both proxy-level (never clear a change); found when this lane landed
	want := []string{"vf-7f12b0dedc81 vf-b446bc665090", "vf-a78f85d3eec5 vf-afa8a757b5f1"}
	if strings.Join(pairs, ",") != strings.Join(want, ",") || len(got) != len(want) {
		t.Fatalf("stored contradictions = %v (warnings %v); want %v", pairs, got, want)
	}
	for i, w := range got {
		a, b, _ := strings.Cut(want[i], " ")
		if !strings.Contains(w, a) || !strings.Contains(w, b) {
			t.Errorf("warning %q does not name both %s and %s", w, a, b)
		}
	}
}
