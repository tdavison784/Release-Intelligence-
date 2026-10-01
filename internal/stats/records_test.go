package stats

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRecordFull(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "records", "alpha.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	r, warns, err := ParseRecord(data)
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if r.Product != "alpha" || r.Order != 1 || r.Wave == nil || *r.Wave != 0 {
		t.Errorf("identity: %+v", r)
	}
	if r.Onboarding.Minutes == nil || *r.Onboarding.Minutes != 120 {
		t.Errorf("minutes: %v", r.Onboarding.Minutes)
	}
	if len(r.Sources) != 3 || r.Sources[1].Origin != OriginDiscoveredModified || r.Sources[1].Note == "" {
		t.Errorf("sources: %+v", r.Sources)
	}
	if r.Relationships.InitialFailures == nil || *r.Relationships.InitialFailures != 3 || *r.Relationships.FinalFailures != 0 {
		t.Errorf("failures: %+v", r.Relationships)
	}
	if len(r.Relationships.ManualInterventions) != 1 || r.Relationships.ManualInterventions[0].Subject != "chart" {
		t.Errorf("interventions: %+v", r.Relationships.ManualInterventions)
	}
	if len(r.Constructs.New) < 50 || r.Constructs.New[0].Name != "field:sources.fallbackGroup" ||
		r.Constructs.New[0].Justification == "" || len(r.Constructs.New[0].OtherProducts) != 1 {
		t.Errorf("constructs: %d first=%+v", len(r.Constructs.New), r.Constructs.New[0])
	}
	if r.Constructs.New[1].Name == "" || r.Constructs.New[1].Justification != "" {
		t.Errorf("a plain string is a name only: %+v", r.Constructs.New[1])
	}
	if len(r.GoChanges.Generic) != 1 || r.GoChanges.ProductSpecific != 0 || len(r.UnreachableSources) != 1 {
		t.Errorf("go changes / unreachable: %+v %+v", r.GoChanges, r.UnreachableSources)
	}
}

func TestParseRecordTolerance(t *testing.T) {
	// Missing fields, extra fields, wrong types and alternate spellings are
	// all tolerated; what cannot be read is dropped with a warning.
	data := `
product: sloppy
order: "4"                       # quoted number
wave: 1
future_field: {anything: [1, 2]} # unknown top-level field
onboarding:
  minutes: "about 100"           # not a number: dropped, minutes stay unknown
  finishedAt: 2026-10-01T13:40:00Z
sources:
  - {id: a, origin: manual, extra: ignored}
  - not-a-mapping
  - {id: b}                       # origin missing
relationships:
  initialFailures: three          # wrong type
  finalFailures: 0
  unverifiable: just text         # scalar where a list was expected
constructs:
  new:
    - field:one                   # plain string
    - {name: field:two, justification: why, otherProducts: other}   # scalar list
    - {construct: field:three}    # alias for name
    - {}                          # nameless
  considered: none
goChanges:
  productSpecific: 0
  generic: [{file: a.go}]
gaps:
  - plain text
  - {what: mapping, why: because}
representability: Full
`
	r, warns, err := ParseRecord([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if r.Product != "sloppy" || r.Order != 4 || r.Wave == nil || *r.Wave != 1 {
		t.Errorf("identity: %+v", r)
	}
	if r.Onboarding.Minutes != nil {
		t.Errorf("unparsable minutes must stay unknown, got %v", *r.Onboarding.Minutes)
	}
	if r.Onboarding.FinishedAt == "" {
		t.Errorf("a timestamp is read as text: %q", r.Onboarding.FinishedAt)
	}
	if len(r.Sources) != 2 || r.Sources[0].ID != "a" || r.Sources[1].Origin != "" {
		t.Errorf("sources: %+v", r.Sources)
	}
	if r.Relationships.InitialFailures != nil || r.Relationships.FinalFailures == nil {
		t.Errorf("failures: %+v", r.Relationships)
	}
	if len(r.Relationships.Unverifiable) != 0 {
		t.Errorf("a scalar cannot be an unverifiable entry: %+v", r.Relationships.Unverifiable)
	}
	var names []string
	for _, c := range r.Constructs.New {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "field:one,field:two,field:three," {
		t.Errorf("construct names: %q", names)
	}
	if got := r.Constructs.New[1].OtherProducts; len(got) != 1 || got[0] != "other" {
		t.Errorf("a single value is a one-element list: %v", got)
	}
	if len(r.GoChanges.Generic) != 1 || len(r.Gaps) != 2 || r.Gaps[1] != "what: mapping; why: because" {
		t.Errorf("go changes / gaps: %+v %+v", r.GoChanges, r.Gaps)
	}
	if r.Representability != "Full" {
		t.Errorf("representability: %q", r.Representability)
	}
	joined := strings.Join(warns, "\n")
	for _, want := range []string{"onboarding.minutes", "sources[1]", "relationships.initialFailures"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no warning mentioning %s in:\n%s", want, joined)
		}
	}
}

func TestParseRecordEdgeCases(t *testing.T) {
	// an empty document and a comment-only document are empty records
	for _, in := range []string{"", "# nothing here\n"} {
		r, warns, err := ParseRecord([]byte(in))
		if err != nil || len(warns) != 0 || r.Product != "" {
			t.Errorf("%q: %+v %v %v", in, r, warns, err)
		}
	}
	// not a mapping: a warning, no failure
	r, warns, err := ParseRecord([]byte("- a\n- b\n"))
	if err != nil || r == nil || len(warns) == 0 {
		t.Errorf("list document: %+v %v %v", r, warns, err)
	}
	// not YAML at all
	if _, _, err := ParseRecord([]byte("a: [unterminated\n")); err == nil {
		t.Error("invalid YAML must be an error")
	}
	// minutes as a float and an explicit null
	r, _, _ = ParseRecord([]byte("onboarding: {minutes: 12.5}\n"))
	if r.Onboarding.Minutes == nil || *r.Onboarding.Minutes != 12.5 {
		t.Errorf("float minutes: %v", r.Onboarding.Minutes)
	}
	r, _, _ = ParseRecord([]byte("onboarding: {minutes: null}\nwave: ~\n"))
	if r.Onboarding.Minutes != nil || r.Wave != nil {
		t.Errorf("null stays unknown: %v %v", r.Onboarding.Minutes, r.Wave)
	}
}

func TestLoadRecords(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("b.yaml", "product: b\norder: 2\n")
	write("a.yml", "product: a\norder: 1\nconstructs: {new: [x:y]}\n")
	write("nameless.yaml", "order: 9\n")
	write("broken.yaml", "a: [unterminated\n")
	write("notes.txt", "ignored")
	if err := os.Mkdir(filepath.Join(dir, "sub.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	recs, warns, err := LoadRecords(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range recs {
		got = append(got, r.Product)
	}
	if strings.Join(got, ",") != "a,b,nameless" {
		t.Errorf("records: %v", got)
	}
	if recs[0].File != filepath.Join(dir, "a.yml") || len(recs[0].Constructs.New) != 1 {
		t.Errorf("record a: %+v", recs[0])
	}
	joined := strings.Join(warns, "\n")
	if !strings.Contains(joined, "broken.yaml: not YAML") || !strings.Contains(joined, "nameless.yaml: no product field") {
		t.Errorf("warnings:\n%s", joined)
	}

	// a missing directory simply has no records
	recs, warns, err = LoadRecords(filepath.Join(dir, "missing"))
	if err != nil || recs != nil || warns != nil {
		t.Errorf("missing dir: %v %v %v", recs, warns, err)
	}
}

func TestLoadFixtureRecords(t *testing.T) {
	recs, warns, err := LoadRecords(filepath.Join("testdata", "records"))
	if err != nil || len(warns) != 0 {
		t.Fatalf("%v %v", err, warns)
	}
	if len(recs) != 3 {
		t.Fatalf("records: %d", len(recs))
	}
}
