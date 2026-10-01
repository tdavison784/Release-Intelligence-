package stats

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func render(t *testing.T, rep *Report, format string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Render(&buf, rep, format); err != nil {
		t.Fatalf("render %s: %v", format, err)
	}
	return buf.String()
}

func TestRenderText(t *testing.T) {
	out := render(t, Build(fixtureInputs(t)), FormatText)
	for _, want := range []string{
		"Onboarding stats: 3 definitions, 3 records, 2 check reports",
		"SIZE AND COMPLEXITY", "CONSTRUCTS AND EFFORT", "DISCOVERY AND RELATIONSHIPS",
		"CONSTRUCT-INTRODUCTION CURVE", "PER-WAVE AGGREGATES",
		"alpha", "beta", "gamma", "██", "40 (n=2)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "DATA QUALITY") {
		t.Errorf("a clean fixture has no data-quality section:\n%s", out)
	}
	if render(t, Build(fixtureInputs(t)), "") != out {
		t.Error("the default format is text")
	}
}

func TestRenderJSON(t *testing.T) {
	rep := Build(fixtureInputs(t))
	out := render(t, rep, FormatJSON)
	var back Report
	if err := json.Unmarshal([]byte(out), &back); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(back.Products) != 3 || back.Summary.Records != 3 || len(back.Waves) != 2 || len(back.Curve) != 3 {
		t.Errorf("round trip: %+v", back.Summary)
	}
	if back.Products[1].Product != "beta" || back.Products[1].New != 20 || len(back.Products[1].UsedList) == 0 {
		t.Errorf("beta: %+v", back.Products[1])
	}
	if back.Products[0].NewDetails["field:sources.fallbackGroup"].Justification == "" {
		t.Error("justifications are part of the JSON")
	}
	if back.Products[2].Check != nil {
		t.Error("gamma has no check report")
	}
}

func TestRenderMarkdown(t *testing.T) {
	rep := Build(fixtureInputs(t))
	out := render(t, rep, FormatMarkdown)
	for _, want := range []string{
		"# Onboarding scalability",
		"## The question", "configuration and data, not software development",
		"## Metrics", "**Constructs**", "**Wave aggregates**", "**Limits.**", "unaccountedConstructs", "*Reused*", "*Zero-new product*", "*Minutes*",
		"## Summary", "1 of 3 recorded products introduced no new construct",
		"## Per-product table", "### Size and complexity", "### Constructs and effort", "### Discovery and relationship validation",
		"| Order | Wave | Product |", "| --- |",
		"## Construct-introduction curve", "```text", "██",
		"## Constructs introduced per product",
		"### gamma (order 3, wave 1): 0 new", "No new constructs: the definition is configuration only.",
		"`field:sources.fallbackGroup`: Alternative release-note channels. (also needed by beta)",
		"## Per-wave aggregates",
		"## Notes from the records", "**alpha**: Fixture record.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown lacks %q", want)
		}
	}
	if render(t, rep, FormatMarkdown) != out || render(t, rep, "md") != out {
		t.Error("markdown output must be deterministic")
	}
	// every table row has as many cells as its header
	var cols int
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "|") {
			cols = 0
			continue
		}
		n := strings.Count(strings.ReplaceAll(line, `\|`, ""), "|")
		if cols == 0 {
			cols = n
		} else if n != cols {
			t.Errorf("ragged table row (%d cells, header %d): %s", n, cols, line)
		}
	}
}

func TestRenderEmptyAndErrors(t *testing.T) {
	empty := Build(&Inputs{})
	if out := render(t, empty, FormatMarkdown); !strings.Contains(out, "No products yet.") {
		t.Errorf("empty markdown:\n%s", out)
	}
	if out := render(t, empty, FormatText); !strings.Contains(out, "no products") {
		t.Errorf("empty text:\n%s", out)
	}
	var buf bytes.Buffer
	if err := Render(&buf, empty, "yaml"); err == nil {
		t.Error("unknown format must be an error")
	}
}

func TestRenderWarningsAndPipes(t *testing.T) {
	in := &Inputs{
		Products: []ProductStats{ps("a", "x:1")},
		Records:  []*Record{rec("a", 1, nil, "x:1")},
		Warnings: []string{"a bad file"},
	}
	in.Records[0].Notes = "a note"
	in.Records[0].Sources = []OriginEntry{{ID: "s", Origin: "manual"}}
	rep := Build(in)
	if out := render(t, rep, FormatMarkdown); !strings.Contains(out, "## Data quality") || !strings.Contains(out, "- a bad file") {
		t.Errorf("markdown warnings:\n%s", out)
	}
	if out := render(t, rep, FormatText); !strings.Contains(out, "DATA QUALITY") {
		t.Errorf("text warnings:\n%s", out)
	}
}

func TestBars(t *testing.T) {
	rep := &Report{Curve: []CurvePoint{
		{Order: 1, Product: "big", HasRecord: true, New: 80, Cumulative: 80},
		{Order: 2, Product: "small", HasRecord: true, New: 1, Cumulative: 81},
		{Order: 3, Product: "none", HasRecord: true, New: 0, Cumulative: 81},
		{Order: 0, Product: "unrecorded", New: 0, Cumulative: 81},
	}}
	out := newBars(rep)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("lines: %q", out)
	}
	count := func(l string) int { return strings.Count(l, "█") }
	if count(lines[0]) != 40 || count(lines[1]) != 1 || count(lines[2]) != 0 || count(lines[3]) != 0 {
		t.Errorf("bar lengths: %d %d %d %d\n%s", count(lines[0]), count(lines[1]), count(lines[2]), count(lines[3]), out)
	}
	for _, l := range strings.Split(strings.TrimRight(cumulativeBars(rep), "\n"), "\n") {
		if strings.Count(l, "▒") != 40 {
			t.Errorf("cumulative bar: %q", l)
		}
	}
}

func TestGroupConstructs(t *testing.T) {
	kinds, by := groupConstructs([]string{"field:a", "locator:x", "field:b", "weird"})
	if strings.Join(kinds, ",") != "field,locator,other" || strings.Join(by["field"], ",") != "a,b" || by["other"][0] != "weird" {
		t.Errorf("%v %v", kinds, by)
	}
}
