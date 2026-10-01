package stats

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeSize(t *testing.T) {
	d := loadFixture(t, "alpha")
	ps := Analyze(d, nil) // reads the file the definition came from
	want := Size{
		YAMLLines: 56, YAMLCodeLines: 54,
		Sources: 3, Artifacts: 2, Channels: 2, Contents: 1, ClassifyRules: 2,
		TemplateExprs: 5, Exceptions: 1, OptionalArtifacts: 1, FallbackGroups: 1,
		AvailabilityConstraints: 3, // notes source, chart artifact, chart helm-values content
	}
	if ps.Size != want {
		t.Fatalf("size = %+v\nwant   %+v", ps.Size, want)
	}
	if ps.Product != "alpha" || ps.Name != "Alpha" {
		t.Errorf("identity: %+v", ps)
	}
	if got := len(ps.SourceIDs) + len(ps.ArtifactIDs); got != 5 {
		t.Errorf("element ids = %d", got)
	}
}

func TestAnalyzeWithoutFile(t *testing.T) {
	d := loadFixture(t, "gamma")
	raw, err := os.ReadFile(filepath.Join("testdata", "products", "gamma.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if ps := Analyze(d, raw); ps.YAMLLines != 25 || ps.YAMLCodeLines != 25 {
		t.Errorf("lines from raw: %d/%d", ps.YAMLLines, ps.YAMLCodeLines)
	}
	// a definition that did not come from disk has no line counts
	d2 := *d
	if ps := Analyze(&d2, []byte{}); ps.YAMLLines != 0 {
		t.Errorf("empty raw: %d", ps.YAMLLines)
	}
}

func TestCountLines(t *testing.T) {
	for _, tc := range []struct {
		in          string
		lines, code int
	}{
		{"", 0, 0},
		{"a: 1\n", 1, 1},
		{"a: 1", 1, 1},
		{"# c\n\n  # indented comment\na: 1\n  b: 2\n", 5, 2},
		{"\n\n", 2, 0},
	} {
		if l, c := countLines([]byte(tc.in)); l != tc.lines || c != tc.code {
			t.Errorf("countLines(%q) = %d,%d want %d,%d", tc.in, l, c, tc.lines, tc.code)
		}
	}
}
