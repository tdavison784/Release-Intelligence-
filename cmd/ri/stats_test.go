package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// statsArgs points ri stats at the fixture definitions, records and reports
// of package stats (no network, no state directory needed).
func statsArgs(t *testing.T, format string, extra ...string) []string {
	t.Helper()
	td := filepath.Join("..", "..", "internal", "stats", "testdata")
	args := []string{"-products", filepath.Join(td, "products"), "stats",
		"-records", filepath.Join(td, "records"), "-checks", filepath.Join(td, "checks")}
	if format != "" {
		args = append(args, "-o", format)
	}
	return append(args, extra...)
}

func TestStatsText(t *testing.T) {
	out, _, err := runCLI(t, statsArgs(t, "")...)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Onboarding stats: 3 definitions, 3 records, 2 check reports", "alpha", "beta", "gamma", "PER-WAVE AGGREGATES"} {
		if !strings.Contains(out, want) {
			t.Errorf("text output lacks %q:\n%s", want, out)
		}
	}
}

func TestStatsJSONAndFilter(t *testing.T) {
	out, _, err := runCLI(t, statsArgs(t, "json", "gamma", "alpha")...)
	if err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Products []struct {
			Product string `json:"product"`
			Order   int    `json:"order"`
			New     int    `json:"new"`
		} `json:"products"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(rep.Products) != 2 || rep.Products[0].Product != "alpha" || rep.Products[1].Product != "gamma" || rep.Products[1].New != 0 {
		t.Errorf("filtered products: %+v", rep.Products)
	}
}

func TestStatsMarkdown(t *testing.T) {
	out, _, err := runCLI(t, statsArgs(t, "markdown")...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "# Onboarding scalability") || !strings.Contains(out, "## Construct-introduction curve") {
		t.Errorf("markdown:\n%s", out[:200])
	}
}

func TestStatsRejectsUnknownFormat(t *testing.T) {
	if _, _, err := runCLI(t, statsArgs(t, "yaml")...); err == nil || !strings.Contains(err.Error(), "unknown output format") {
		t.Errorf("err = %v", err)
	}
}

func TestStatsWithoutInputs(t *testing.T) {
	dir := t.TempDir()
	out, _, err := runCLI(t, "-products", dir, "stats", "-records", filepath.Join(dir, "r"), "-checks", filepath.Join(dir, "c"))
	if err != nil || !strings.Contains(out, "0 definitions, 0 records, 0 check reports") {
		t.Errorf("empty inputs: %v\n%s", err, out)
	}
}
