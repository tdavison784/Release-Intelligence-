package gitsrc

import (
	"errors"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/fetch"
)

func TestParseRepo(t *testing.T) {
	tests := []struct {
		in       string
		host     string
		path     string
		clone    string
		web      string
		wantErr  string
		wantFlav flavor
	}{
		{in: "github.com/cert-manager/cert-manager", host: "github.com", path: "cert-manager/cert-manager",
			clone: "https://github.com/cert-manager/cert-manager", web: "https://github.com/cert-manager/cert-manager", wantFlav: flavorGitHub},
		{in: "cert-manager/cert-manager", host: "github.com", path: "cert-manager/cert-manager",
			clone: "https://github.com/cert-manager/cert-manager", web: "https://github.com/cert-manager/cert-manager", wantFlav: flavorGitHub},
		{in: "https://github.com/istio/istio", host: "github.com", path: "istio/istio",
			clone: "https://github.com/istio/istio", web: "https://github.com/istio/istio", wantFlav: flavorGitHub},
		{in: "https://github.com/istio/istio.git", host: "github.com", path: "istio/istio",
			clone: "https://github.com/istio/istio.git", web: "https://github.com/istio/istio", wantFlav: flavorGitHub},
		{in: "https://github.com/istio/istio/", host: "github.com", path: "istio/istio",
			clone: "https://github.com/istio/istio", web: "https://github.com/istio/istio", wantFlav: flavorGitHub},
		{in: "github.com/argoproj/argo-helm.git", host: "github.com", path: "argoproj/argo-helm",
			clone: "https://github.com/argoproj/argo-helm", web: "https://github.com/argoproj/argo-helm", wantFlav: flavorGitHub},
		{in: "GitHub.com/o/r", host: "github.com", path: "o/r",
			clone: "https://github.com/o/r", web: "https://github.com/o/r", wantFlav: flavorGitHub},
		{in: "gitlab.com/group/sub/proj", host: "gitlab.com", path: "group/sub/proj",
			clone: "https://gitlab.com/group/sub/proj", web: "https://gitlab.com/group/sub/proj", wantFlav: flavorGitLab},
		{in: "https://gitlab.example.com/g/p.git", host: "gitlab.example.com", path: "g/p",
			clone: "https://gitlab.example.com/g/p.git", web: "https://gitlab.example.com/g/p", wantFlav: flavorGitLab},
		{in: "git.example.org/team/tool", host: "git.example.org", path: "team/tool",
			clone: "https://git.example.org/team/tool", web: "https://git.example.org/team/tool", wantFlav: flavorOther},
		{in: "ssh://git@github.com/o/r.git", host: "github.com", path: "o/r",
			clone: "ssh://git@github.com/o/r.git", web: "https://github.com/o/r", wantFlav: flavorGitHub},
		{in: "https://github.com/.github/.github", host: "github.com", path: ".github/.github",
			clone: "https://github.com/.github/.github", web: "https://github.com/.github/.github", wantFlav: flavorGitHub},
		{in: "file:///tmp/some/repo.git", host: "", path: "tmp/some/repo",
			clone: "file:///tmp/some/repo.git", web: "file:///tmp/some/repo.git", wantFlav: flavorOther},

		{in: "", wantErr: "empty"},
		{in: "cert-manager", wantErr: "want host/owner/name"},
		{in: "a/b/c", wantErr: "want host/owner/name"},
		{in: "go.uber.org/zap", wantErr: "want host/owner/name"},
		{in: "-evil/x", wantErr: "must not start"},
		{in: "--upload-pack=x", wantErr: "must not start"},
		{in: "o/r with space", wantErr: "whitespace"},
		{in: "o/..", wantErr: "bad path segment"},
		{in: "github.com/o/../r", wantErr: "bad path segment"},
		{in: "https://github.com/", wantErr: "want scheme://host/path"},
		{in: "https://user:secret@github.com/o/r", wantErr: "passwords"},
		{in: "ftp://example.com/o/r", wantErr: "unsupported scheme"},
		{in: "https://github.com/o/r/tree/main", wantErr: "owner/name"},
		{in: "github.com/o/r/issues/1", wantErr: "owner/name"},
		{in: "github.com/justowner", wantErr: "want host/owner/name"},
		{in: "github.mycorp.com/team/sub/tool", host: "github.mycorp.com", path: "team/sub/tool",
			clone: "https://github.mycorp.com/team/sub/tool", web: "https://github.mycorp.com/team/sub/tool", wantFlav: flavorGitHub},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			r, err := ParseRepo(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ParseRepo(%q) error = %v, want containing %q", tc.in, err, tc.wantErr)
				}
				if strings.Contains(err.Error(), "secret") {
					t.Fatalf("error leaks the password: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRepo(%q): %v", tc.in, err)
			}
			if r.Host != tc.host || r.Path != tc.path || r.CloneURL != tc.clone || r.WebURL != tc.web || r.flavor != tc.wantFlav {
				t.Errorf("ParseRepo(%q) = %+v, want host=%q path=%q clone=%q web=%q flavor=%v",
					tc.in, r, tc.host, tc.path, tc.clone, tc.web, tc.wantFlav)
			}
		})
	}
}

func TestRepoSlugIsStableAndDistinct(t *testing.T) {
	a, _ := ParseRepo("github.com/o/r")
	b, _ := ParseRepo("https://github.com/o/r")
	c, _ := ParseRepo("https://github.com/o/r.git")
	d, _ := ParseRepo("gitlab.com/o/r")
	if a.slug() != b.slug() {
		t.Errorf("same clone URL must give the same slug: %q vs %q", a.slug(), b.slug())
	}
	if a.slug() == d.slug() || a.slug() == c.slug() && a.CloneURL != c.CloneURL {
		t.Errorf("different repositories must not collide: %q %q %q", a.slug(), c.slug(), d.slug())
	}
	if strings.ContainsAny(a.slug(), "/\\: ") {
		t.Errorf("slug %q is not file-system friendly", a.slug())
	}
}

func TestRepoURLs(t *testing.T) {
	gh, _ := ParseRepo("github.com/cert-manager/cert-manager")
	gl, _ := ParseRepo("gitlab.com/group/sub/proj")
	other, _ := ParseRepo("git.example.org/team/tool")

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"gh tag", gh.TagURL("v1.18.0"), "https://github.com/cert-manager/cert-manager/releases/tag/v1.18.0"},
		{"gh tag with slash", gh.TagURL("cmd/ctl/v1.12.0"), "https://github.com/cert-manager/cert-manager/releases/tag/cmd/ctl/v1.12.0"},
		{"gh blob", gh.BlobURL("v1.18.0", "deploy/charts/values.yaml"), "https://github.com/cert-manager/cert-manager/blob/v1.18.0/deploy/charts/values.yaml"},
		{"gh blob escaped", gh.BlobURL("master", "a b/c#d.md"), "https://github.com/cert-manager/cert-manager/blob/master/a%20b/c%23d.md"},
		{"gh commit", gh.CommitURL("abc123"), "https://github.com/cert-manager/cert-manager/commit/abc123"},
		{"gh compare", gh.CompareURL("v1.17.0", "v1.18.0"), "https://github.com/cert-manager/cert-manager/compare/v1.17.0...v1.18.0"},
		{"gl tag", gl.TagURL("v1.0.0"), "https://gitlab.com/group/sub/proj/-/tags/v1.0.0"},
		{"gl blob", gl.BlobURL("main", "docs/a.md"), "https://gitlab.com/group/sub/proj/-/blob/main/docs/a.md"},
		{"gl commit", gl.CommitURL("abc123"), "https://gitlab.com/group/sub/proj/-/commit/abc123"},
		{"gl compare", gl.CompareURL("v1", "v2"), "https://gitlab.com/group/sub/proj/-/compare/v1...v2"},
		{"other tag falls back to repo page", other.TagURL("v1.0.0"), "https://git.example.org/team/tool"},
		{"other blob falls back to repo page", other.BlobURL("v1.0.0", "a.md"), "https://git.example.org/team/tool"},
		{"other commit has no link", other.CommitURL("abc"), ""},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestRawURL(t *testing.T) {
	gh, _ := ParseRepo("github.com/cert-manager/website")
	gl, _ := ParseRepo("gitlab.com/group/sub/proj")
	other, _ := ParseRepo("git.example.org/team/tool")
	ghe, _ := ParseRepo("github.mycorp.com/team/tool")

	if got, err := gh.RawURL("master", "content/docs/a b.md"); err != nil ||
		got != "https://raw.githubusercontent.com/cert-manager/website/master/content/docs/a%20b.md" {
		t.Errorf("github raw: %q %v", got, err)
	}
	if got, err := gl.RawURL("v1.0.0", "docs/a.md"); err != nil || got != "https://gitlab.com/group/sub/proj/-/raw/v1.0.0/docs/a.md" {
		t.Errorf("gitlab raw: %q %v", got, err)
	}
	for _, r := range []Repo{other, ghe} {
		_, err := r.RawURL("v1", "a.md")
		if err == nil || !errors.Is(err, fetch.ErrUnavailable) || !strings.Contains(err.Error(), "unsupported host") {
			t.Errorf("%s: want unsupported host wrapping ErrUnavailable, got %v", r.Host, err)
		}
	}
}

func TestSplitRange(t *testing.T) {
	tests := []struct {
		in, a, b, wantErr string
	}{
		{in: "v3.0.0..v3.1.0", a: "v3.0.0", b: "v3.1.0"},
		{in: " 1.30.0..1.31.0 ", a: "1.30.0", b: "1.31.0"},
		{in: "argo-cd-8.0.0..argo-cd-8.0.1", a: "argo-cd-8.0.0", b: "argo-cd-8.0.1"},
		{in: "v1...v2", wantErr: "symmetric"},
		{in: "v1", wantErr: "want a range"},
		{in: "..v2", wantErr: "want a range"},
		{in: "v1..", wantErr: "want a range"},
		{in: "a..b..c", wantErr: "want a range"},
		{in: "--output=x..v2", wantErr: "must not start"},
		{in: "v1..v2^", wantErr: "invalid ref"},
		{in: "", wantErr: "want a range"},
	}
	for _, tc := range tests {
		a, b, err := splitRange(tc.in)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("splitRange(%q) error = %v, want %q", tc.in, err, tc.wantErr)
			}
			continue
		}
		if err != nil || a != tc.a || b != tc.b {
			t.Errorf("splitRange(%q) = %q, %q, %v", tc.in, a, b, err)
		}
	}
}

func TestCleanRepoPath(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "releasenotes/notes", want: "releasenotes/notes"},
		{in: "/releasenotes/notes/", want: "releasenotes/notes"},
		{in: "", want: ""},
		{in: ".", want: ""},
		{in: "a/../b", wantErr: true},
		{in: "a//b", wantErr: true},
		{in: "../x", wantErr: true},
	}
	for _, tc := range tests {
		got, err := cleanRepoPath(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("cleanRepoPath(%q) = %q, %v", tc.in, got, err)
		}
	}
}
