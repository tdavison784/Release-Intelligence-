package normalize

import (
	"reflect"
	"testing"

	"github.com/Masterminds/semver/v3"
)

func TestParseVersionRange(t *testing.T) {
	tests := []struct {
		in       string
		wantC    string
		wantV    []string
		wantFail bool
	}{
		{in: "1.29 → 1.33", wantC: ">=1.29.0-0, <1.34.0-0", wantV: []string{"1.29", "1.30", "1.31", "1.32", "1.33"}},
		{in: "1.29 -> 1.33", wantC: ">=1.29.0-0, <1.34.0-0", wantV: []string{"1.29", "1.30", "1.31", "1.32", "1.33"}},
		{in: "1.22 - 1.27", wantC: ">=1.22.0-0, <1.28.0-0", wantV: []string{"1.22", "1.23", "1.24", "1.25", "1.26", "1.27"}},
		{in: "1.31-1.33", wantC: ">=1.31.0-0, <1.34.0-0", wantV: []string{"1.31", "1.32", "1.33"}},
		{in: "4.14 to 4.18", wantC: ">=4.14.0-0, <4.19.0-0", wantV: []string{"4.14", "4.15", "4.16", "4.17", "4.18"}},
		{in: "v1.36, v1.35, v1.34, v1.33", wantC: ">=1.33.0-0, <1.37.0-0", wantV: []string{"1.33", "1.34", "1.35", "1.36"}},
		{in: "1.29, 1.30", wantC: ">=1.29.0-0, <1.31.0-0", wantV: []string{"1.29", "1.30"}},
		{in: "1.29, 1.31", wantC: ">=1.29.0-0, <1.30.0-0 || >=1.31.0-0, <1.32.0-0", wantV: []string{"1.29", "1.31"}},
		{in: "1.25+", wantC: ">=1.25.0-0"},
		{in: "1.25 and above", wantC: ">=1.25.0-0"},
		{in: ">= 1.22.0-0", wantC: ">= 1.22.0-0"},
		{in: "≥ 1.25", wantC: ">= 1.25"},
		{in: ">=1.22, <1.30", wantC: ">=1.22, <1.30"},
		{in: "1.33", wantC: ">=1.33.0-0, <1.34.0-0", wantV: []string{"1.33"}},
		{in: "v1.33", wantC: ">=1.33.0-0, <1.34.0-0", wantV: []string{"1.33"}},
		{in: "1.29.x", wantC: ">=1.29.0-0, <1.30.0-0", wantV: []string{"1.29"}},
		{in: "1.29.3", wantC: "1.29.3", wantV: []string{"1.29.3"}},
		{in: "1.22.0-0", wantC: "1.22.0-0", wantV: []string{"1.22.0-0"}},
		{in: "1.29.0 - 1.29.5", wantC: ">=1.29.0, <=1.29.5"},
		{in: "4", wantC: ">=4.0.0-0, <5.0.0-0", wantV: []string{"4"}},
		{in: "3 → 5", wantC: ">=3.0.0-0, <6.0.0-0", wantV: []string{"3", "4", "5"}},
		{in: "3.09 → 4.7", wantC: ">=3.9.0-0, <4.8.0-0"}, // across majors: no enumeration
		{in: " `1.29` → `1.33` ", wantC: ">=1.29.0-0, <1.34.0-0", wantV: []string{"1.29", "1.30", "1.31", "1.32", "1.33"}},
		{in: "1.29 – 1.31", wantC: ">=1.29.0-0, <1.32.0-0", wantV: []string{"1.29", "1.30", "1.31"}},
		{in: "1.31, 1.29, 1.30", wantC: ">=1.29.0-0, <1.32.0-0", wantV: []string{"1.29", "1.30", "1.31"}},
		{in: "1.29 or 1.30", wantC: ">=1.29.0-0, <1.31.0-0", wantV: []string{"1.29", "1.30"}},
		{in: "1.29.*", wantC: "1.29.*"},
		{in: "^1.2", wantC: "^1.2"},

		{in: "", wantFail: true},
		{in: "   ", wantFail: true},
		{in: "TBD", wantFail: true},
		{in: "N/A", wantFail: true},
		{in: "1.12 LTS", wantFail: true},
		{in: "1.33 → 1.29", wantFail: true},
		{in: "→", wantFail: true},
		{in: ">= nonsense", wantFail: true},
		{in: "1.33 → 1.36 / 4.20 → 4.22", wantFail: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			c, v, err := ParseVersionRange(tt.in)
			if tt.wantFail {
				if err == nil {
					t.Fatalf("expected an error, got constraint %q versions %v", c, v)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c != tt.wantC {
				t.Errorf("constraint = %q, want %q", c, tt.wantC)
			}
			if !reflect.DeepEqual(v, tt.wantV) {
				t.Errorf("versions = %v, want %v", v, tt.wantV)
			}
			if _, err := semver.NewConstraint(c); err != nil {
				t.Errorf("constraint %q does not parse: %v", c, err)
			}
		})
	}
}

func TestParseVersionRangeSemantics(t *testing.T) {
	check := func(raw string, in, out []string) {
		t.Helper()
		c, _, err := ParseVersionRange(raw)
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		cons, err := semver.NewConstraint(c)
		if err != nil {
			t.Fatalf("%q => %q: %v", raw, c, err)
		}
		for _, v := range in {
			if !cons.Check(semver.MustParse(v)) {
				t.Errorf("%q (%s) should contain %s", raw, c, v)
			}
		}
		for _, v := range out {
			if cons.Check(semver.MustParse(v)) {
				t.Errorf("%q (%s) should not contain %s", raw, c, v)
			}
		}
	}
	// minor granularity includes every patch of the upper line and the
	// prereleases of the lower line, but nothing of the next line
	check("1.29 → 1.33", []string{"1.29.0", "1.29.0-rc.1", "1.31.7", "1.33.0", "1.33.12"}, []string{"1.28.9", "1.34.0", "1.34.0-rc.1"})
	check("1.25+", []string{"1.25.0", "1.99.1", "2.0.0"}, []string{"1.24.9"})
	check("1.29, 1.31", []string{"1.29.2", "1.31.0"}, []string{"1.30.1", "1.32.0"})
	check("1.29.0 - 1.29.5", []string{"1.29.0", "1.29.5"}, []string{"1.29.6", "1.28.9"})
	check("4", []string{"4.0.0", "4.9.9"}, []string{"3.9.9", "5.0.0"})
}
