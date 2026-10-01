package gitsrc

import (
	"strings"
	"testing"
)

func TestIsImmutableRef(t *testing.T) {
	sha1 := strings.Repeat("a1", 20)
	sha256 := strings.Repeat("0f", 32)
	tests := []struct {
		ref  string
		want bool
	}{
		// version tags
		{"v1.18.0", true},
		{"1.26.0", true},
		{"1.31.1", true},
		{"v3.6.0-rc1", true},
		{"v1.18.0-alpha.0", true},
		{"v1.2.3+build.5", true},
		{"argo-cd-8.0.0", true},
		{"argo-cd-10.9.5", true},
		{"cmd/ctl/v1.12.0", true},
		{"V2.0.0", true},
		{"1.2.3.4", true},
		// commit ids
		{sha1, true},
		{strings.ToUpper(sha1), true},
		{sha256, true},
		// branches and moving tags
		{"master", false},
		{"main", false},
		{"HEAD", false},
		{"stable", false},
		{"latest", false},
		{"release-1.31", false},
		{"release-1.30.4", false},
		{"release-1.27.4-patch", false},
		{"releases/v1.2.3", false},
		{"hotfix-1.2.3", false},
		{"feature/foo-1.2.3", false},
		// not clearly versioned
		{"v1.18", false},
		{"1.31.x", false},
		{"v1", false},
		{"foo1.2.3", false},
		{strings.Repeat("a1", 19) + "a", false}, // 39 chars
		{strings.Repeat("a1", 20) + "a", false}, // 41 chars
		{strings.Repeat("g1", 20), false},       // not hex
		{"", false},
		{"   ", false},
	}
	for _, tc := range tests {
		if got := IsImmutableRef(tc.ref); got != tc.want {
			t.Errorf("IsImmutableRef(%q) = %v, want %v", tc.ref, got, tc.want)
		}
	}
}

func TestFormatFromPath(t *testing.T) {
	tests := map[string]string{
		"README.md":                      "markdown",
		"docs/release-notes-1.18.MD":     "markdown",
		"docs/x.markdown":                "markdown",
		"Chart.yaml":                     "yaml",
		"supportStatus.yml":              "yaml",
		"content/docs/variables.json":    "json",
		"index.html":                     "html",
		"LICENSE":                        "text",
		"Makefile":                       "text",
		"script.sh":                      "text",
		"archive.tar.gz":                 "text",
		"dir.yaml/file":                  "text",
		"releasenotes/notes/foo.yaml":    "yaml",
		"releasenotes/notes/.hidden.yml": "yaml",
	}
	for p, want := range tests {
		if got := FormatFromPath(p); got != want {
			t.Errorf("FormatFromPath(%q) = %q, want %q", p, got, want)
		}
	}
}

func TestRefCandidates(t *testing.T) {
	tests := []struct {
		ref  string
		want []string
	}{
		{"v1.18.0", []string{"refs/tags/v1.18.0", "refs/heads/v1.18.0"}},
		{"master", []string{"refs/heads/master", "refs/tags/master"}},
		{"release-1.31", []string{"refs/heads/release-1.31", "refs/tags/release-1.31"}},
		{"HEAD", []string{"HEAD"}},
		{"refs/heads/x", []string{"refs/heads/x"}},
		{strings.Repeat("ab", 20), []string{strings.Repeat("ab", 20)}},
	}
	for _, tc := range tests {
		got := refCandidates(tc.ref)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("refCandidates(%q) = %v, want %v", tc.ref, got, tc.want)
		}
	}
}
