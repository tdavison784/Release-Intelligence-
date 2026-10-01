package stats

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadFixtureChecks(t *testing.T) {
	checks, warns, err := LoadChecks(filepath.Join("testdata", "checks"))
	if err != nil || len(warns) != 0 {
		t.Fatalf("%v %v", err, warns)
	}
	if len(checks) != 2 {
		t.Fatalf("reports: %d", len(checks))
	}
	a := checks["alpha"]
	if a == nil {
		t.Fatal("no report for alpha")
	}
	if a.Subjects != 5 || a.Validated != 1 || a.Failing != 1 || a.Insufficient != 3 {
		t.Errorf("verdicts: %+v", a)
	}
	// image is unverifiable for 2 releases and never verified; chart/helm-values
	// has one unverifiable release next to two passes
	if a.Unverifiable != 2 || a.OnlyUnverifiable != 1 {
		t.Errorf("unverifiable: %d / only %d", a.Unverifiable, a.OnlyUnverifiable)
	}
	if !reflect.DeepEqual(a.FailingSubjects, []string{"chart"}) ||
		!reflect.DeepEqual(a.UnverifiableSubjects, []string{"chart/helm-values", "image"}) {
		t.Errorf("subjects: failing %v unverifiable %v", a.FailingSubjects, a.UnverifiableSubjects)
	}
	wantOutcomes := map[string]int{"pass": 5, "covered": 1, "unverifiable": 2, "not-applicable": 1, "fail": 1}
	if !reflect.DeepEqual(a.Outcomes, wantOutcomes) {
		t.Errorf("outcomes: %v", a.Outcomes)
	}
	if len(a.Releases) != 3 || a.File == "" {
		t.Errorf("releases/file: %+v", a)
	}
}

func TestLoadChecksSkipsJunk(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	good := `{"product":"p","releases":["1.0.0"],"checks":[{"subject":"s","subjectKind":"source","release":"1.0.0","outcome":"pass"}],"summary":[{"subject":"s","subjectKind":"source","passed":1,"verdict":"insufficient"}]}`
	write("p.json", good)
	write("q.json", strings.Replace(good, `"product":"p"`, `"product":"other"`, 1)) // name mismatch: warned, still used
	write("dup.json", good)                                                         // second report for p
	write("noproduct.json", strings.Replace(good, `"product":"p",`, "", 1))         // product taken from the file name
	write("garbage.json", "not json")
	write("empty.json", `{"product":"e"}`)
	write("readme.md", "ignored")
	checks, warns, err := LoadChecks(dir)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for id := range checks {
		ids = append(ids, id)
	}
	if len(checks) != 3 || checks["p"] == nil || checks["other"] == nil || checks["noproduct"] == nil {
		t.Errorf("reports: %v", ids)
	}
	joined := strings.Join(warns, "\n")
	for _, want := range []string{"garbage.json: not a relationship report", "empty.json: report has no summary", "q.json: report is for product \"other\"", "second report for \"p\""} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing warning %q in:\n%s", want, joined)
		}
	}

	checks, warns, err = LoadChecks(filepath.Join(dir, "missing"))
	if err != nil || checks != nil || warns != nil {
		t.Errorf("missing dir: %v %v %v", checks, warns, err)
	}
}
