package discovery

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestFindVersionTokensBoundaries(t *testing.T) {
	cases := map[string]int{
		"release-notes-1.21.md":       1,
		"announcing-1.30.1/index.md":  1,
		"10.0.0.1":                    0,
		"python3.11":                  0,
		"upgrading-1.20-1.21.md":      2,
		"quay.io/x/y:v1.21.2":         1,
		"1.31.x/announcing-1.31.1/":   2,
		"sha256:abc1.2def":            0,
		"go1.24.7":                    0,
		"docs/upgrading/2.14-3.0.md":  2,
		"chart-10.9.5.tgz":            1,
		"content/en/news/releases/x/": 0,
	}
	for in, want := range cases {
		if got := len(findVersionTokens(in)); got != want {
			t.Errorf("%q: got %d tokens, want %d", in, got, want)
		}
	}
}

func TestTemplatizeRelease(t *testing.T) {
	v := domain.MustVersion("v1.21.2", "1.21.2")
	cases := []struct {
		in, prefix, prev, want string
		preferTag              bool
	}{
		{"quay.io/jetstack/cert-manager-controller:v1.21.2", "v", "1.20", "quay.io/jetstack/cert-manager-controller:{{.Tag}}", false},
		{"cert-manager-1.21.2.tgz", "v", "", "cert-manager-{{.Version}}.tgz", false},
		{"release-notes-1.21.md", "v", "", "release-notes-{{.Major}}.{{.Minor}}.md", false},
		{"upgrading-1.20-1.21.md", "v", "1.20", "upgrading-{{.PrevMajor}}.{{.PrevMinor}}-{{.Major}}.{{.Minor}}.md", false},
		{"upgrading-1.19-1.20.md", "v", "1.20", "upgrading-1.19-1.20.md", false},
		{"istio-1.21.2-linux-amd64.tar.gz", "", "", "istio-{{.Tag}}-linux-amd64.tar.gz", true},
		{"istio-1.21.2-linux-amd64.tar.gz", "", "", "istio-{{.Version}}-linux-amd64.tar.gz", false},
	}
	for _, c := range cases {
		got, _ := templatizeRelease(c.in, v, c.prefix, c.prev, c.preferTag)
		if got != c.want {
			t.Errorf("templatizeRelease(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInferPathTemplate(t *testing.T) {
	cases := []struct {
		in, prefix, want, anchor string
		pair                     bool
	}{
		{"docs/operator-manual/upgrading/2.14-3.0.md", "v", "docs/operator-manual/upgrading/{{.PrevMajor}}.{{.PrevMinor}}-{{.Major}}.{{.Minor}}.md", "3.0", true},
		{"content/docs/releases/upgrading/upgrading-1.20-1.21.md", "v", "content/docs/releases/upgrading/upgrading-{{.PrevMajor}}.{{.PrevMinor}}-{{.Major}}.{{.Minor}}.md", "1.21", true},
		{"content/en/news/releases/1.30.x/announcing-1.30.1/index.md", "", "content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Version}}/index.md", "1.30.1", false},
		{"content/en/news/releases/1.30.x/announcing-1.30/upgrade-notes/index.md", "", "content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Major}}.{{.Minor}}/upgrade-notes/index.md", "1.30", false},
		{"content/docs/releases/release-notes/release-notes-1.21.md", "v", "content/docs/releases/release-notes/release-notes-{{.Major}}.{{.Minor}}.md", "1.21", false},
		{"docs/releases/v1.2.3.md", "v", "docs/releases/{{.Tag}}.md", "1.2.3", false},
	}
	for _, c := range cases {
		pt, ok := inferPathTemplate(c.in, c.prefix)
		if !ok || pt.Template != c.want || pt.Anchor() != c.anchor || pt.Pair != c.pair {
			t.Errorf("inferPathTemplate(%q) = %+v %v, want %q anchor %s pair %v", c.in, pt, ok, c.want, c.anchor, c.pair)
		}
	}
	if _, ok := inferPathTemplate("docs/compare-1.2-and-1.4.md", "v"); ok {
		t.Error("inconsistent tokens must not produce a template")
	}
}

// Rendering a template inferred from a path must give the path back.
func TestInferredTemplatesRenderBack(t *testing.T) {
	known := []domain.Version{domain.MustVersion("2.14.11", "2.14.11"), domain.MustVersion("3.0.0", "3.0.0")}
	pt, _ := inferPathTemplate("upgrading/2.14-3.0.md", "")
	rc := catalog.NewRenderContext("x", known[1], known)
	got, err := catalog.Render(pt.Template, rc)
	if err != nil || got != "upgrading/2.14-3.0.md" {
		t.Fatalf("render back = %q, %v", got, err)
	}
}

func TestTemplatizeAssetURL(t *testing.T) {
	cases := []struct {
		in, prefix, want string
		ok               bool
	}{
		{"https://github.com/o/r/releases/download/${INITIAL_RELEASE}/o.yaml", "v", "https://github.com/o/r/releases/download/{{.Tag}}/o.yaml", true},
		{"https://github.com/o/r/releases/download/v1.14.1/o.crds.yaml", "v", "https://github.com/o/r/releases/download/{{.Tag}}/o.crds.yaml", true},
		{"https://github.com/o/r/releases/download/${V}/istio-${V}-linux.tar.gz", "", "https://github.com/o/r/releases/download/{{.Tag}}/istio-{{.Version}}-linux.tar.gz", true},
		{"https://github.com/o/r/releases/download/[[VAR::latest]]/m.yaml", "v", "https://github.com/o/r/releases/download/{{.Tag}}/m.yaml", true},
		{"https://github.com/o/r/releases/download/${V}/x-${OS}.tar.gz", "v", "https://github.com/o/r/releases/download/{{.Tag}}/x-${OS}.tar.gz", false},
		{"https://github.com/o/r/releases/download/1.2.3/istio-1.2.3-osx.tar.gz", "", "https://github.com/o/r/releases/download/{{.Tag}}/istio-{{.Version}}-osx.tar.gz", true},
	}
	for _, c := range cases {
		got, ok := templatizeAssetURL(c.in, c.prefix)
		if got != c.want || ok != c.ok {
			t.Errorf("templatizeAssetURL(%q) = %q %v, want %q %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
