package upgrade

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func tagsOf(vs []domain.Version) []string {
	out := []string{}
	for _, v := range vs {
		out = append(out, v.String())
	}
	return out
}

func concat(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestSelectPath(t *testing.T) {
	cm := concat(series("v", 1, 16, 5), series("v", 1, 17, 4), series("v", 1, 18, 6))
	argo := concat(series("v", 2, 13, 2), series("v", 2, 14, 3), series("v", 3, 0, 2), []string{"v3.1.0"})
	tests := []struct {
		name     string
		versions []string
		from, to string
		lineage  string
		policy   string
		path     []string
		skipped  []string
	}{
		{
			name: "minor lineage across two lines (spec example)", versions: cm, from: "v1.16.3", to: "v1.18.2", lineage: "minor",
			policy:  PolicyMinorLineage,
			path:    []string{"v1.17.0", "v1.18.0", "v1.18.1", "v1.18.2"},
			skipped: []string{"v1.16.4", "v1.16.5", "v1.17.1", "v1.17.2", "v1.17.3", "v1.17.4"},
		},
		{
			name: "minor lineage to an X.Y.0", versions: cm, from: "v1.17.0", to: "v1.18.0", lineage: "minor",
			policy: PolicyMinorLineage, path: []string{"v1.18.0"}, skipped: []string{"v1.17.1", "v1.17.2", "v1.17.3", "v1.17.4"},
		},
		{
			name: "same line patches", versions: cm, from: "v1.17.1", to: "v1.17.4", lineage: "minor",
			policy: PolicyMinorLineage, path: []string{"v1.17.2", "v1.17.3", "v1.17.4"}, skipped: []string{},
		},
		{
			name: "major boundary 2.14 → 3.0", versions: argo, from: "v2.14.1", to: "v3.0.2", lineage: "minor",
			policy: PolicyMinorLineage, path: []string{"v3.0.0", "v3.0.1", "v3.0.2"}, skipped: []string{"v2.14.2", "v2.14.3"},
		},
		{
			name: "across a major boundary and further lines", versions: argo, from: "v2.13.1", to: "v3.1.0", lineage: "minor",
			policy:  PolicyMinorLineage,
			path:    []string{"v2.14.0", "v3.0.0", "v3.1.0"},
			skipped: []string{"v2.13.2", "v2.14.1", "v2.14.2", "v2.14.3", "v3.0.1", "v3.0.2"},
		},
		{
			name: "intermediate line without X.Y.0 uses its lowest release", versions: []string{"v1.0.0", "v1.1.2", "v1.1.3", "v1.2.0"}, from: "v1.0.0", to: "v1.2.0", lineage: "minor",
			policy: PolicyMinorLineage, path: []string{"v1.1.2", "v1.2.0"}, skipped: []string{"v1.1.3"},
		},
		{
			name: "prereleases are never traversed", versions: []string{"v1.17.0", "v1.18.0-rc.1", "v1.18.0", "v1.19.0-alpha.0", "v1.19.0"}, from: "v1.17.0", to: "v1.19.0", lineage: "minor",
			policy: PolicyMinorLineage, path: []string{"v1.18.0", "v1.19.0"}, skipped: []string{"v1.18.0-rc.1", "v1.19.0-alpha.0"},
		},
		{
			name: "prerelease target", versions: []string{"v1.20.0", "v1.20.1", "v1.21.0-alpha.0", "v1.21.0-beta.0"}, from: "v1.20.0", to: "v1.21.0-beta.0", lineage: "minor",
			policy: PolicyMinorLineage, path: []string{"v1.21.0-beta.0"}, skipped: []string{"v1.20.1", "v1.21.0-alpha.0"},
		},
		{
			name: "unsorted input with duplicates", versions: []string{"v1.18.1", "v1.17.0", "v1.18.0", "v1.17.0", "v1.17.2", "v1.18.1"}, from: "v1.17.0", to: "v1.18.1", lineage: "minor",
			policy: PolicyMinorLineage, path: []string{"v1.18.0", "v1.18.1"}, skipped: []string{"v1.17.2"},
		},
		{
			name: "linear lineage traverses everything", versions: cm, from: "v1.17.3", to: "v1.18.1", lineage: "linear",
			policy: PolicyAll, path: []string{"v1.17.4", "v1.18.0", "v1.18.1"}, skipped: []string{},
		},
		{
			name: "empty lineage means all", versions: []string{"1.29.0", "1.29.1", "1.30.0"}, from: "1.29.0", to: "1.30.0", lineage: "",
			policy: PolicyAll, path: []string{"1.29.1", "1.30.0"}, skipped: []string{},
		},
		{
			name: "all skips prereleases", versions: []string{"v1.0.0", "v1.1.0-rc.1", "v1.1.0"}, from: "v1.0.0", to: "v1.1.0", lineage: "linear",
			policy: PolicyAll, path: []string{"v1.1.0"}, skipped: []string{"v1.1.0-rc.1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel, err := SelectPath(versions(tt.versions...), ver(tt.from), ver(tt.to), tt.lineage)
			if err != nil {
				t.Fatal(err)
			}
			if sel.Policy != tt.policy {
				t.Errorf("policy = %q, want %q", sel.Policy, tt.policy)
			}
			if got := tagsOf(sel.Path); !reflect.DeepEqual(got, tt.path) {
				t.Errorf("path = %v, want %v", got, tt.path)
			}
			if got := tagsOf(sel.Skipped); !reflect.DeepEqual(got, tt.skipped) {
				t.Errorf("skipped = %v, want %v", got, tt.skipped)
			}
			if last := sel.Path[len(sel.Path)-1]; !last.Equal(ver(tt.to)) {
				t.Errorf("path must end at to, got %s", last)
			}
		})
	}
}

func TestSelectPathErrors(t *testing.T) {
	vs := versions("v1.0.0", "v1.1.0", "v1.2.0")
	tests := []struct {
		name, from, to, want string
	}{
		{"unknown from", "v0.9.0", "v1.2.0", "from version"},
		{"unknown to", "v1.0.0", "v1.3.0", "to version"},
		{"equal", "v1.1.0", "v1.1.0", "must be lower"},
		{"downgrade", "v1.2.0", "v1.0.0", "must be lower"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SelectPath(vs, ver(tt.from), ver(tt.to), "minor")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestStepReasons(t *testing.T) {
	path := versions("v1.17.0", "v2.0.0", "v2.1.3", "v2.2.0", "v2.2.1")
	got := stepReasons(PolicyMinorLineage, ver("v1.16.3"), path)
	want := []string{ReasonMinorRelease, ReasonMajorRelease, ReasonLineEntry, ReasonMinorRelease, ReasonTargetLinePatch}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reasons = %v, want %v", got, want)
	}
	if got := stepReasons(PolicyMinorLineage, ver("v1.17.0"), versions("v1.17.1", "v1.17.2")); !reflect.DeepEqual(got, []string{ReasonPatch, ReasonPatch}) {
		t.Errorf("same-line reasons = %v", got)
	}
	if got := stepReasons(PolicyAll, ver("v1.0.0"), versions("v1.0.1", "v1.1.0")); !reflect.DeepEqual(got, []string{ReasonRelease, ReasonRelease}) {
		t.Errorf("all reasons = %v", got)
	}
}
