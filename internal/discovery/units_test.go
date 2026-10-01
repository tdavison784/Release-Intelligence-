package discovery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
)

func TestAnalyzeTags(t *testing.T) {
	cases := []struct {
		f                       fixture
		prefix, latest, lineage string
		strict                  bool
		junk                    int
	}{
		{certFixture, "v", "v1.2.1", catalog.LineageMinor, false, 2},
		{argoFixture, "v", "v3.1.2", catalog.LineageMinor, true, 2},
		{istioFixture, "", "1.31.1", catalog.LineageMinor, true, 2},
	}
	for _, c := range cases {
		ta := c.f.tagAnalysis(t)
		if ta.Prefix != c.prefix || ta.Latest != c.latest || ta.Lineage != c.lineage || (ta.TagPattern != "") != c.strict || ta.JunkCount != c.junk {
			t.Errorf("%s: %s pattern=%q junk=%v", c.f.repo, ta.Summary(), ta.TagPattern, ta.Junk)
		}
		if ta.TagPattern != "" {
			p, err := (&catalog.ProductDefinition{Versioning: catalog.Versioning{TagPattern: ta.TagPattern}}).VersionParser()
			if err != nil {
				t.Fatal(err)
			}
			for _, j := range ta.Junk {
				if _, err := p.Parse(j); err == nil {
					t.Errorf("%s: strict pattern accepts junk tag %s", c.f.repo, j)
				}
			}
			if _, err := p.Parse(ta.Latest); err != nil {
				t.Errorf("strict pattern rejects %s", ta.Latest)
			}
		}
	}
	ta := certFixture.tagAnalysis(t)
	if ta.PrereleaseStyles["alpha.N"] != 1 || ta.PrereleaseStyles["beta.N"] != 1 {
		t.Errorf("styles %v", ta.PrereleaseStyles)
	}
	if mj, mn, ok := ta.PrevLine(1, 2); !ok || mj != 1 || mn != 1 {
		t.Errorf("prev line of 1.2: %d.%d %v", mj, mn, ok)
	}
}

func TestParseLsRemote(t *testing.T) {
	out := []byte("aaa\trefs/tags/v1.0.0\nbbb\trefs/tags/v1.0.0^{}\nccc\trefs/tags/v1.1.0\nddd\trefs/heads/main\n")
	tags := ParseLsRemote(out)
	if len(tags) != 2 || tags[0].Name != "v1.0.0" || tags[0].Object != "aaa" || tags[0].Commit != "bbb" || tags[1].Commit != "ccc" {
		t.Fatalf("got %+v", tags)
	}
}

func TestParseRepo(t *testing.T) {
	cases := map[string]string{
		"argoproj/argo-cd":                            "github.com/argoproj/argo-cd",
		"github.com/istio/istio":                      "github.com/istio/istio",
		"https://github.com/cert-manager/website.git": "github.com/cert-manager/website",
		"git@gitlab.com:group/sub/proj.git":           "gitlab.com/group/sub/proj",
	}
	for in, want := range cases {
		r, err := ParseRepo(in)
		if err != nil || r.String() != want {
			t.Errorf("ParseRepo(%q) = %v %v, want %s", in, r, err, want)
		}
	}
	if _, err := ParseRepo("nope"); err == nil {
		t.Error("expected error")
	}
	r, _ := ParseRepo("istio/istio")
	if r.BlobURL("1.31.1", "a/b.md") != "https://github.com/istio/istio/blob/1.31.1/a/b.md" || r.Slug() != "istio/istio" {
		t.Error("URLs")
	}
}

// fakeRunner scripts git for GitCheckout.
type fakeRunner struct {
	calls [][]string
	stdin []string
	root  string // directory standing in for the cloned tree
}

func (f *fakeRunner) Run(ctx context.Context, dir string, stdin []byte, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	f.stdin = append(f.stdin, string(stdin))
	switch args[0] {
	case "clone":
		dst := args[len(args)-1]
		if err := os.MkdirAll(filepath.Join(dst, ".git"), 0o755); err != nil {
			return nil, err
		}
		if !containsStr(args, "--no-checkout") {
			return nil, copyTree(f.root, dst)
		}
		return nil, nil
	case "rev-parse":
		if containsStr(args, "--abbrev-ref") {
			return []byte("main\n"), nil
		}
		return []byte("0123456789abcdef0123456789abcdef01234567\n"), nil
	case "tag":
		return []byte("v1.2.1\n"), nil
	case "ls-tree":
		return []byte("100644 blob 1111111111111111111111111111111111111111\tdocs/a.md\x00100644 blob 2222222222222222222222222222222222222222\tREADME.md\x00120000 blob 3333333333333333333333333333333333333333\tlink\x00"), nil
	case "-c": // bulk prefetch
		return nil, nil
	case "cat-file":
		return []byte("1111111111111111111111111111111111111111 blob 5\nhello\n2222222222222222222222222222222222222222 blob 0\n\n"), nil
	case "ls-remote":
		if containsStr(args, "--symref") {
			return []byte("ref: refs/heads/master\tHEAD\nabc\tHEAD\n"), nil
		}
		return []byte("abc\trefs/tags/v1.0.0\n"), nil
	}
	return nil, fmt.Errorf("unexpected git %v", args)
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
}

func TestGitCheckoutFullAndPartial(t *testing.T) {
	fr := &fakeRunner{root: "testdata/argolike"}
	gc := &GitCheckout{CacheDir: t.TempDir(), Runner: fr, Clock: clock}
	repo, _ := ParseRepo("example/deployer")
	tree, err := gc.Checkout(context.Background(), repo, "v1.2.1", CheckoutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	info := tree.Info()
	if !info.RefIsTag || info.PinRef != "v1.2.1" || info.Commit == "" || len(tree.Files()) == 0 {
		t.Errorf("full checkout: %+v", info)
	}
	clone := fr.calls[0]
	if strings.Join(clone[:5], " ") != "git clone --quiet --depth 1" || !containsStr(clone, "--branch") || !containsStr(clone, "https://github.com/example/deployer") {
		t.Errorf("clone command: %v", clone)
	}
	// a second checkout reuses the cache
	n := len(fr.calls)
	if _, err := gc.Checkout(context.Background(), repo, "v1.2.1", CheckoutOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, c := range fr.calls[n:] {
		if c[1] == "clone" {
			t.Error("cached clone should be reused")
		}
	}

	docs, _ := ParseRepo("example/website")
	pt, err := gc.Checkout(context.Background(), docs, "", CheckoutOptions{Partial: true})
	if err != nil {
		t.Fatal(err)
	}
	if pt.Info().Ref != "main" || pt.Info().PinRef == "main" || len(pt.Files()) != 2 {
		t.Errorf("partial tree: %+v files=%v", pt.Info(), pt.Files())
	}
	got, err := pt.ReadFiles(context.Background(), []string{"docs/a.md", "README.md", "missing"})
	if err != nil || string(got["docs/a.md"]) != "hello" || len(got) != 2 {
		t.Errorf("ReadFiles = %q, %v", got, err)
	}
	last := fr.stdin[len(fr.stdin)-1]
	if !strings.Contains(last, "1111111111111111111111111111111111111111") {
		t.Errorf("cat-file should receive object ids, got %q", last)
	}
	for _, c := range fr.calls {
		if c[1] == "ls-tree" && containsStr(c, "-l") {
			t.Error("ls-tree -l would fetch every blob of a partial clone")
		}
	}
	tags, _ := gc.ListTags(context.Background(), repo)
	branch, _ := gc.DefaultBranch(context.Background(), docs)
	if len(tags) != 1 || branch != "master" {
		t.Errorf("tags %v branch %q", tags, branch)
	}
}

func TestCollectVarsExpand(t *testing.T) {
	mk := collectVars("git_version := $(shell git describe --tags)\nVERSION ?= $(git_version)\nHUB ?=istio # comment\n\t$(eval TAG := x-$(VERSION))\n", "make")
	if got := mk.expand("$(HUB)/pilot:$(VERSION)"); got != "istio/pilot:"+markGitTag {
		t.Errorf("make expand: %q", got)
	}
	if mk["TAG"] == "" {
		t.Error("$(eval X := ...) assignments are collected")
	}
	sh := collectVars("NS=\"${{ vars.NS || 'argoproj' }}\"\nX=\"$(curl -sL https://x | \\\nX=${X##*/}\nTAG=\"$(cat ./VERSION)-${GITHUB_SHA::8}\"\n", "shell")
	if sh["NS"] != "argoproj" {
		t.Errorf("actions default: %q", sh["NS"])
	}
	if _, ok := sh["X"]; ok {
		t.Error("multi-line command substitutions are skipped")
	}
	if _, ok := sh["X"]; ok {
		t.Error("self references are skipped")
	}
	if got := sh.expand("img:$TAG"); got != "img:"+markVersionFile+"-"+markGitCommit {
		t.Errorf("snapshot tag: %q", got)
	}
	if got := sh.expand("${UNSET:-def}/x"); got != "def/x" {
		t.Errorf("default expansion: %q", got)
	}
}

func TestFindImageRefsAndTags(t *testing.T) {
	refs := findImageRefs(`echo "img=quay.io/argoproj/argocd:v3.5.3" && helm push x.tgz oci://ghcr.io/o/charts && see https://github.com/o/r and quay.io/argoproj/$IMAGE_REPOSITORY`)
	var got []string
	for _, r := range refs {
		got = append(got, fmt.Sprintf("%s|%s|oci=%v|partial=%v", r.Repository(), r.Tag, r.OCI, r.Partial))
	}
	want := "quay.io/argoproj/argocd|v3.5.3|oci=false|partial=false ghcr.io/o/charts||oci=true|partial=false"
	if !strings.HasPrefix(strings.Join(got, " "), want) || len(got) != 3 || !refs[2].Partial {
		t.Errorf("got %v", got)
	}
	if r, ok := parseImageValue("redis:7.0.15-alpine"); !ok || r.Repository() != "docker.io/library/redis" || r.Tag != "7.0.15-alpine" {
		t.Errorf("short name: %+v", r)
	}
	if r, ok := parseImageValue("istio/pilot:1.2"); !ok || r.Repository() != "docker.io/istio/pilot" {
		t.Errorf("hub name: %+v", r)
	}
	cases := []struct{ tag, class, tmpl string }{
		{"v1.2.1", tagRelease, tmplTag},
		{"1.2.1", tagRelease, tmplVersion},
		{markGitTag, tagRelease, tmplTag},
		{"${{ github.ref_name }}", tagRelease, tmplTag},
		{"latest", tagFloating, ""},
		{"3.7.0-f955a903", tagSnapshot, ""},
		{markVersionFile + "-" + markGitCommit, tagSnapshot, ""},
		{"7.2.7-alpine", tagPinned, ""},
		{"$(IMAGE_TAG)", tagUnresolved, ""},
	}
	for _, c := range cases {
		cl, tm := classifyTag(c.tag, "v1.2.1", "1.2.1")
		if cl != c.class || tm != c.tmpl {
			t.Errorf("classifyTag(%q) = %s %s, want %s %s", c.tag, cl, tm, c.class, c.tmpl)
		}
	}
	if !hasDevSegment("ghcr.io/istio/testing") || !hasDevSegment("gcr.io/istio-testing/pilot") || !hasDevSegment("docker.io/istionightly/pilot") || hasDevSegment("docker.io/istio/pilot") {
		t.Error("dev registry detection")
	}
	if registryFromValue("istio") != "docker.io/istio" || registryFromValue("quay.io/jetstack") != "quay.io/jetstack" || registryFromValue("$(X)") != "" {
		t.Error("registryFromValue")
	}
}

func TestCommonGlobAndColumns(t *testing.T) {
	cs := []Candidate{{Value: "m/crds/application-crd.yaml"}, {Value: "m/crds/appproject-crd.yaml"}}
	if g := commonGlob(cs); g != "*-crd.yaml" {
		t.Errorf("glob %q", g)
	}
	c := Candidate{Attributes: map[string]string{"supportedHeaders": "A / Kubernetes,Kubernetes", "testedHeaders": "Tested Kubernetes", "separatorParts": "A / Kubernetes=1"}}
	cols := compatColumns(c, false)
	if len(cols) != 3 || cols[0].Separator != "/" || cols[0].Part != 1 || cols[1].Separator != "" || cols[2].Kind != "tested" {
		t.Errorf("columns %+v", cols)
	}
}

func TestChartTagPattern(t *testing.T) {
	p, note := chartTagPattern("argo-cd", []RemoteTag{{Name: "argo-cd-1.0.0"}, {Name: "argo-workflows-1.0.0"}})
	if p != `^argo-cd-(?P<version>\d+\.\d+\.\d+)$` || !strings.Contains(note, "1 of 2") {
		t.Errorf("%s %s", p, note)
	}
}
