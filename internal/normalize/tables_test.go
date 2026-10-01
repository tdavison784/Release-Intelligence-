package normalize

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestExtractTableRow(t *testing.T) {
	md := strings.Join([]string{
		"# Title",           // 1
		"",                  // 2
		"| Other | Thing |", // 3
		"|---|---|",         // 4
		"| a | b |",         // 5
		"",                  // 6
		"## Supported",      // 7
		"",                  // 8
		"| [Release][r] | **Kubernetes** | `Notes` |", // 9
		"|:--|:-:|--:|", // 10
		"| [1.21][] | 1.33 → 1.36 | pipe \\| inside |", // 11
		"| [1.12 LTS][] | 1.22 → 1.32 | x |",           // 12
		"| 1.2 | 1.1 → 1.2 | y |",                      // 13
		"",                                             // 14
		"## Old",                                       // 15
		"",                                             // 16
		"Release | Kubernetes",                         // 17
		"--- | ---",                                    // 18
		"1.9 | 1.9 → 1.20",                             // 19
		"",                                             // 20
		"```",                                          // 21
		"| Release | Kubernetes |",                     // 22
		"|---|---|",                                    // 23
		"| 9.9 | fenced |",                             // 24
		"```",                                          // 25
	}, "\n")

	t.Run("key with link, case-insensitive header", func(t *testing.T) {
		row, err := ExtractTableRow([]byte(md), TableSelector{KeyColumns: []string{"release"}, KeyRe: mustRe(`^1\.21$`)})
		if err != nil {
			t.Fatal(err)
		}
		if row.Line != 11 {
			t.Errorf("line = %d", row.Line)
		}
		if want := []string{"Release", "Kubernetes", "Notes"}; !reflect.DeepEqual(row.Headers, want) {
			t.Errorf("headers = %v", row.Headers)
		}
		if row.Cells["Release"] != "[1.21][]" || row.Cells["Kubernetes"] != "1.33 → 1.36" || row.Cells["Notes"] != "pipe | inside" {
			t.Errorf("cells = %v", row.Cells)
		}
		if row.Excerpt != "| [Release][r] | **Kubernetes** | `Notes` |\n| [1.21][] | 1.33 → 1.36 | pipe \\| inside |" {
			t.Errorf("excerpt = %q", row.Excerpt)
		}
	})
	t.Run("key with suffix", func(t *testing.T) {
		row, err := ExtractTableRow([]byte(md), TableSelector{KeyColumns: []string{"Release"}, KeyRe: mustRe(`^1\.12\b`)})
		if err != nil || row.Cells["Kubernetes"] != "1.22 → 1.32" {
			t.Errorf("%+v %v", row, err)
		}
	})
	t.Run("no leading and trailing pipes", func(t *testing.T) {
		row, err := ExtractTableRow([]byte(md), TableSelector{KeyColumns: []string{"Release"}, KeyRe: mustRe(`^1\.9$`)})
		if err != nil || row.Line != 19 || row.Cells["Kubernetes"] != "1.9 → 1.20" {
			t.Errorf("%+v %v", row, err)
		}
	})
	t.Run("alternative key columns, first present wins per table", func(t *testing.T) {
		row, err := ExtractTableRow([]byte(md), TableSelector{KeyColumns: []string{"Version", "Release"}, KeyRe: mustRe(`^1\.2$`)})
		if err != nil || row.Line != 13 {
			t.Errorf("%+v %v", row, err)
		}
	})
	t.Run("table heading restriction", func(t *testing.T) {
		if _, err := ExtractTableRow([]byte(md), TableSelector{KeyColumns: []string{"Release"}, KeyRe: mustRe(`^1\.9$`), TableHeading: mustRe(`^Supported$`)}); !errors.Is(err, ErrNoMatch) {
			t.Errorf("1.9 is under Old: %v", err)
		}
		row, err := ExtractTableRow([]byte(md), TableSelector{KeyColumns: []string{"Release"}, KeyRe: mustRe(`^1\.9$`), TableHeading: mustRe(`(?i)^old$`)})
		if err != nil || row.Line != 19 {
			t.Errorf("%+v %v", row, err)
		}
	})
	t.Run("tables in code fences are ignored", func(t *testing.T) {
		if _, err := ExtractTableRow([]byte(md), TableSelector{KeyColumns: []string{"Release"}, KeyRe: mustRe(`^9\.9$`)}); !errors.Is(err, ErrNoMatch) {
			t.Errorf("fenced table matched: %v", err)
		}
	})
	t.Run("no match", func(t *testing.T) {
		for _, sel := range []TableSelector{
			{KeyColumns: []string{"Release"}, KeyRe: mustRe(`^7\.7$`)},
			{KeyColumns: []string{"Nope"}, KeyRe: mustRe(`.`)},
			{KeyRe: mustRe(`.`)},
		} {
			if _, err := ExtractTableRow([]byte(md), sel); !errors.Is(err, ErrNoMatch) {
				t.Errorf("%+v: %v", sel, err)
			}
		}
	})
}

func TestExtractTableRowRealREADME(t *testing.T) {
	readme := fixture(t, "certmanager/releases-README.md")

	// old releases table
	row, err := ExtractTableRow(readme, TableSelector{KeyColumns: []string{"Release"}, KeyRe: mustRe(`^1\.18\b`)})
	if err != nil {
		t.Fatal(err)
	}
	if row.Cells["Compatible Kubernetes versions"] != "1.29 → 1.33" || row.Cells["Compatible OpenShift versions"] != "4.16 → 4.20" {
		t.Errorf("1.18 row: %v", row.Cells)
	}
	// an LTS key
	row, err = ExtractTableRow(readme, TableSelector{KeyColumns: []string{"Release"}, KeyRe: mustRe(`^1\.12\b`), TableHeading: mustRe(`(?i)old cert-manager releases`)})
	if err != nil || row.Cells["Compatible Kubernetes versions"] != "1.22 → 1.32" {
		t.Errorf("1.12 LTS: %+v %v", row, err)
	}
	// the "currently supported" table has different, link-wrapped headers
	row, err = ExtractTableRow(readme, TableSelector{KeyColumns: []string{"Release"}, KeyRe: mustRe(`^1\.21$`), TableHeading: mustRe(`(?i)currently supported`)})
	if err != nil {
		t.Fatal(err)
	}
	if row.Cells["Supported Kubernetes / OpenShift Versions"] != "1.33 → 1.36 / 4.20 → 4.22" || row.Cells["Tested Kubernetes Versions"] != "1.33 → 1.36" {
		t.Errorf("1.21 row: %v", row.Cells)
	}

	// CompatibilityFromRow over the real row: "K8sMin → K8sMax / OpenShiftMin → OpenShiftMax"
	in := DocInput{SourceID: "compat", URI: "https://example.com/README.md", Digest: "sha256:r", RetrievedAt: testTime, LineOffset: 0}
	cols := []catalog.ColumnSpec{
		{Platform: "kubernetes", Headers: []string{"Supported Kubernetes / OpenShift Versions", "Compatible Kubernetes versions"}, Separator: "/", Part: 0},
		{Platform: "openshift", Headers: []string{"Supported Kubernetes / OpenShift Versions"}, Separator: "/", Part: 1},
		{Platform: "kubernetes", Kind: "tested", Headers: []string{"Tested Kubernetes Versions"}},
		{Platform: "helm", Headers: []string{"Not There"}},
	}
	cc, evs := CompatibilityFromRow(in, row, cols)
	if len(cc) != 3 || len(evs) != 1 {
		t.Fatalf("constraints=%d evidence=%d", len(cc), len(evs))
	}
	k8s := cc[0]
	if k8s.Platform != "kubernetes" || k8s.Constraint != ">=1.33.0-0, <1.37.0-0" || k8s.Raw != "1.33 → 1.36" || k8s.Kind != "supported" ||
		!reflect.DeepEqual(k8s.Versions, []string{"1.33", "1.34", "1.35", "1.36"}) || k8s.SourceID != "compat" {
		t.Errorf("kubernetes: %+v", k8s)
	}
	os := cc[1]
	if os.Platform != "openshift" || os.Constraint != ">=4.20.0-0, <4.23.0-0" || os.Raw != "4.20 → 4.22" {
		t.Errorf("openshift: %+v", os)
	}
	if cc[2].Kind != "tested" {
		t.Errorf("tested: %+v", cc[2])
	}
	for _, c := range cc {
		if c.Provenance.Method != domain.MethodDeclared || c.Provenance.Producer != ProducerTable || c.Provenance.Confidence != domain.ConfidenceHigh {
			t.Errorf("provenance: %+v", c.Provenance)
		}
		if err := c.Provenance.Validate(); err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(c.Evidence, []domain.EvidenceID{evs[0].ID}) {
			t.Errorf("evidence ids: %v", c.Evidence)
		}
	}
	ev := evs[0]
	lines := strings.Split(string(readme), "\n")
	if ev.Locator != "L24" || !strings.Contains(lines[23], "[1.21][]") || ev.Kind != domain.EvidenceDocument || ev.URI != in.URI || ev.ContentDigest != "sha256:r" {
		t.Errorf("evidence: %+v (line 24 = %q)", ev, lines[23])
	}
	if !strings.HasSuffix(ev.Excerpt, "| 1.33 → 1.36                        |") || !strings.HasPrefix(ev.Excerpt, "| Release  |") {
		t.Errorf("excerpt = %q", ev.Excerpt)
	}

	// LineOffset shifts the locator
	in.LineOffset = 1000
	_, evs = CompatibilityFromRow(in, row, cols)
	if evs[0].Locator != "L1024" {
		t.Errorf("locator = %q", evs[0].Locator)
	}

	// upcoming release with TBD: kept raw, empty constraint
	row, err = ExtractTableRow(readme, TableSelector{KeyColumns: []string{"Release"}, KeyRe: mustRe(`^1\.22$`)})
	if err != nil {
		t.Fatal(err)
	}
	cc, _ = CompatibilityFromRow(in, row, cols[:1])
	if len(cc) != 1 || cc[0].Raw != "TBD" || cc[0].Constraint != "" || cc[0].Versions != nil {
		t.Errorf("TBD: %+v", cc)
	}
}

func TestExtractTableRowArgoTestedVersions(t *testing.T) {
	md := fixture(t, "argocd/tested-kubernetes-versions.md")
	row, err := ExtractTableRow(md, TableSelector{KeyColumns: []string{"Argo CD version"}, KeyRe: mustRe(`^3\.5$`)})
	if err != nil {
		t.Fatal(err)
	}
	if row.Cells["Kubernetes versions"] != "v1.36, v1.35, v1.34, v1.33" || row.Line != 3 {
		t.Errorf("row: %+v", row)
	}
	cc, evs := CompatibilityFromRow(DocInput{SourceID: "tested", URI: "u"}, row, []catalog.ColumnSpec{
		{Platform: "kubernetes", Kind: "tested", Headers: []string{"kubernetes versions"}},
	})
	if len(cc) != 1 || cc[0].Constraint != ">=1.33.0-0, <1.37.0-0" || !reflect.DeepEqual(cc[0].Versions, []string{"1.33", "1.34", "1.35", "1.36"}) || len(evs) != 1 {
		t.Errorf("%+v", cc)
	}
	// "3.5" must not match "3.50" or "13.5"
	if _, err := ExtractTableRow(md, TableSelector{KeyColumns: []string{"Argo CD version"}, KeyRe: mustRe(`^3\.50$`)}); !errors.Is(err, ErrNoMatch) {
		t.Errorf("%v", err)
	}
}

func TestCompatibilityFromRowEdgeCases(t *testing.T) {
	in := DocInput{SourceID: "s", URI: "u"}
	row := &TableRow{
		Headers: []string{"Name", "Combined", "Empty"},
		Cells:   map[string]string{"Name": "x", "Combined": "1.29 → 1.31 / 4.14 → 4.16 / odd", "Empty": ""},
		Line:    5,
		Excerpt: "| Name | Combined | Empty |\n| x | ... | |",
	}
	cc, evs := CompatibilityFromRow(in, row, []catalog.ColumnSpec{
		{Platform: "a", Headers: []string{"COMBINED"}, Separator: "/", Part: 0},
		{Platform: "b", Headers: []string{"Combined"}, Separator: "/", Part: 1},
		{Platform: "c", Headers: []string{"Combined"}, Separator: "/", Part: 2},
		{Platform: "d", Headers: []string{"Combined"}, Separator: "/", Part: 3}, // out of range: skipped
		{Platform: "e", Headers: []string{"Empty"}},                             // empty cell: skipped
		{Platform: "f", Headers: []string{"Missing", "Name"}},                   // first present header, unparsable value
	})
	if len(cc) != 4 || len(evs) != 1 {
		t.Fatalf("got %d constraints", len(cc))
	}
	if cc[0].Constraint != ">=1.29.0-0, <1.32.0-0" || cc[1].Constraint != ">=4.14.0-0, <4.17.0-0" {
		t.Errorf("%+v %+v", cc[0], cc[1])
	}
	if cc[2].Raw != "odd" || cc[2].Constraint != "" || cc[3].Raw != "x" || cc[3].Constraint != "" || cc[3].Platform != "f" {
		t.Errorf("unparsable: %+v %+v", cc[2], cc[3])
	}

	// nothing selected: no evidence
	cc, evs = CompatibilityFromRow(in, row, []catalog.ColumnSpec{{Platform: "z", Headers: []string{"Nope"}}})
	if len(cc) != 0 || len(evs) != 0 {
		t.Errorf("%v %v", cc, evs)
	}
	if cc, evs = CompatibilityFromRow(in, nil, nil); cc != nil || evs != nil {
		t.Errorf("nil row: %v %v", cc, evs)
	}
}

func TestExtractRecordIstioSupportStatus(t *testing.T) {
	yml := fixture(t, "istio/supportStatus.yml")
	row, err := ExtractRecord(yml, TableSelector{KeyColumns: []string{"version"}, KeyRe: mustRe(`^1\.31$`)})
	if err != nil {
		t.Fatal(err)
	}
	if row.Cells["version"] != "1.31" || row.Cells["k8sVersions"] != "1.32, 1.33, 1.34, 1.35, 1.36, 1.37" ||
		row.Cells["testedK8sVersions"] != "1.27, 1.28, 1.29, 1.30, 1.31" || row.Cells["supported"] != "Yes" {
		t.Errorf("cells = %v", row.Cells)
	}
	if want := []string{"version", "supported", "releaseDate", "eolDate", "k8sVersions", "testedK8sVersions"}; !reflect.DeepEqual(row.Headers, want) {
		t.Errorf("headers = %v", row.Headers)
	}
	lines := strings.Split(string(yml), "\n")
	if !strings.Contains(lines[row.Line-1], `version: "1.31"`) {
		t.Errorf("line %d = %q", row.Line, lines[row.Line-1])
	}
	if !strings.Contains(row.Excerpt, "k8sVersions:") || !strings.HasPrefix(row.Excerpt, "version:") {
		t.Errorf("excerpt = %q", row.Excerpt)
	}

	in := DocInput{SourceID: "istio-support", URI: "https://example.com/supportStatus.yml", Digest: "sha256:y", RetrievedAt: testTime}
	cc, evs := CompatibilityFromRow(in, row, []catalog.ColumnSpec{
		{Platform: "kubernetes", Headers: []string{"k8sVersions"}},
		{Platform: "kubernetes", Kind: "tested", Headers: []string{"testedK8sVersions"}},
	})
	if len(cc) != 2 || len(evs) != 1 {
		t.Fatalf("%v", cc)
	}
	if cc[0].Constraint != ">=1.32.0-0, <1.38.0-0" || !reflect.DeepEqual(cc[0].Versions, []string{"1.32", "1.33", "1.34", "1.35", "1.36", "1.37"}) {
		t.Errorf("supported: %+v", cc[0])
	}
	if cc[1].Kind != "tested" || cc[1].Constraint != ">=1.27.0-0, <1.32.0-0" {
		t.Errorf("tested: %+v", cc[1])
	}
	if evs[0].Kind != domain.EvidenceStructured || evs[0].Locator != "L"+strconv.Itoa(row.Line) {
		t.Errorf("evidence: %+v", evs[0])
	}

	// master entry with empty (null) fields
	row, err = ExtractRecord(yml, TableSelector{KeyColumns: []string{"version"}, KeyRe: mustRe(`^master$`)})
	if err != nil || row.Cells["releaseDate"] != "" || row.Cells["supported"] != "No, development only" {
		t.Errorf("%+v %v", row, err)
	}
	// unquoted numbers keep their text ("1.30", not 1.3)
	row, err = ExtractRecord([]byte("- version: 1.30\n  k8s: [1.30, 1.31]\n- version: 1.5\n"), TableSelector{KeyColumns: []string{"version"}, KeyRe: mustRe(`^1\.30$`)})
	if err != nil || row.Cells["version"] != "1.30" || row.Cells["k8s"] != "1.30, 1.31" {
		t.Errorf("%+v %v", row, err)
	}
	if _, err := ExtractRecord(yml, TableSelector{KeyColumns: []string{"version"}, KeyRe: mustRe(`^9\.9$`)}); !errors.Is(err, ErrNoMatch) {
		t.Errorf("%v", err)
	}
}

func TestExtractRecordShapes(t *testing.T) {
	sel := TableSelector{KeyColumns: []string{"Version", "name"}, KeyRe: mustRe(`^b$`)}
	// a mapping whose first list-valued field holds the records
	row, err := ExtractRecord([]byte("generated: yesterday\nentries:\n  - name: a\n    x: 1\n  - name: b\n    x: 2\n    nested: {k: v}\n"), sel)
	if err != nil || row.Cells["x"] != "2" || row.Cells["nested"] != "{k: v}" || row.Line != 5 {
		t.Errorf("mapping form: %+v %v", row, err)
	}
	// JSON
	row, err = ExtractRecord([]byte(`[{"name": "a"}, {"name": "b", "vs": ["1.1", "1.2"]}]`), sel)
	if err != nil || row.Cells["vs"] != "1.1, 1.2" {
		t.Errorf("json: %+v %v", row, err)
	}
	for _, bad := range []string{"", "just a string", "a: 1\nb: 2\n", "[1, 2]"} {
		if _, err := ExtractRecord([]byte(bad), sel); !errors.Is(err, ErrNoMatch) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if _, err := ExtractRecord([]byte("a: [unclosed"), sel); err == nil || errors.Is(err, ErrNoMatch) {
		t.Errorf("invalid yaml should be a parse error: %v", err)
	}
}
