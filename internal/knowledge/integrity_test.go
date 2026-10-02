package knowledge

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Blind authoring (DESIGN.md §7, FLEET.md): knowledge is release-level and
// authored without the eval expectations. No file under knowledge/ may refer to
// eval cases, their expectation blocks, or a case id. The test reads only the
// NAMES of the case directories, never their contents.

var forbiddenPatterns = []*regexp.Regexp{
	regexp.MustCompile(`eval/cases`),
	regexp.MustCompile(`case\.yaml`),
	regexp.MustCompile(`expectedImpact|expectedFindings|environmentEvidence`),
	regexp.MustCompile(`(?i)expected[ _-]?item`),
}

func caseIDs(t *testing.T, root string) []string {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(root, "eval", "cases"))
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range ents {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids
}

// scanKnowledge returns one message per violation under dir.
func scanKnowledge(dir string, caseIDs []string) ([]string, error) {
	var bad []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		s := stripEnvironmentContext(b)
		for _, re := range forbiddenPatterns {
			if re.MatchString(s) {
				bad = append(bad, p+": matches "+re.String())
			}
		}
		for _, id := range caseIDs {
			if strings.Contains(s, id) {
				bad = append(bad, p+": names eval case "+id)
			}
		}
		return nil
	})
	return bad, err
}

// stripEnvironmentContext drops reviewItem.context from a record before it is
// scanned. That field is the one place a case id is allowed: it names the
// environment shown to the reviewer (the transfer measurement needs the id),
// and says nothing about what the case expects.
func stripEnvironmentContext(b []byte) string {
	var rec map[string]any
	if json.Unmarshal(b, &rec) != nil {
		return string(b)
	}
	if it, ok := rec["reviewItem"].(map[string]any); ok {
		delete(it, "context")
		if out, err := json.Marshal(rec); err == nil {
			return string(out)
		}
	}
	return string(b)
}

func TestKnowledgeDirectoryNeverReferencesEvalCases(t *testing.T) {
	root := filepath.Join("..", "..")
	ids := caseIDs(t, root)
	dir := filepath.Join(root, DefaultDir)
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no committed knowledge/ directory yet")
	}
	bad, err := scanKnowledge(dir, ids)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		t.Error(b)
	}
}

func TestIntegrityScanDetectsViolations(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ok.json", `{"statement":"rotationPolicy default Never to Always"}`)
	if bad, _ := scanKnowledge(dir, []string{"cert-manager-1.17-1.18"}); len(bad) != 0 {
		t.Fatalf("clean tree flagged: %v", bad)
	}
	write("a.json", `{"note":"see eval/cases/x"}`)
	write("b.json", `{"note":"expectedImpact link"}`)
	write("c.json", `{"note":"as in cert-manager-1.17-1.18"}`)
	write("d.json", `{"note":"expected item E1"}`)
	bad, _ := scanKnowledge(dir, []string{"cert-manager-1.17-1.18"})
	if len(bad) != 4 {
		t.Fatalf("violations = %v, want 4", bad)
	}
}

// Everything the store writes in tests is itself clean.
func TestFixturesPassTheIntegrityScan(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(dir)
	c := fixtureCandidate()
	mustPut(t, s, c)
	mustPut(t, s, proposal(c, "zai", "glm", rotationAssertion("behavior-change")))
	bad, err := scanKnowledge(dir, caseIDs(t, filepath.Join("..", "..")))
	if err != nil || len(bad) != 0 {
		t.Fatalf("%v %v", bad, err)
	}
}

func TestItemContextMayNameACaseButNothingElseMay(t *testing.T) {
	dir := t.TempDir()
	rec := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rec("ok.json", `{"kind":"review-item","reviewItem":{"question":"q","context":{"label":"cert-manager-1.17-1.18"}}}`)
	if bad, _ := scanKnowledge(dir, []string{"cert-manager-1.17-1.18"}); len(bad) != 0 {
		t.Fatalf("context label flagged: %v", bad)
	}
	rec("bad.json", `{"kind":"review-item","reviewItem":{"question":"as in cert-manager-1.17-1.18","context":{"label":"x"}}}`)
	if bad, _ := scanKnowledge(dir, []string{"cert-manager-1.17-1.18"}); len(bad) != 1 {
		t.Fatalf("case id outside context not flagged: %v", bad)
	}
}
