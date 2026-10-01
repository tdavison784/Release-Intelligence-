package helm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

const (
	argoRepo    = "github.com/argoproj/argo-helm"
	argoPattern = `^argo-cd-(?P<version>.+)$`
)

func gitLoc() catalog.Locator {
	return catalog.Locator{
		Kind: catalog.LocatorHelmGit, Repository: argoRepo, Path: "charts/argo-cd", TagPattern: argoPattern,
	}
}

// fakeTags is a git-tags VersionLister.
type fakeTags struct {
	refs  []sources.ReleaseRef
	err   error
	calls atomic.Int32
	last  catalog.Locator
	mu    sync.Mutex
}

func (f *fakeTags) ListReleases(_ context.Context, loc catalog.Locator) ([]sources.ReleaseRef, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.last = loc
	f.mu.Unlock()
	return f.refs, f.err
}

// fakeFiles is a repo-file DocumentFetcher keyed by "<ref>:<path>".
type fakeFiles struct {
	files map[string]string
	// errFor returns an error for a given ref (checked first).
	errFor func(ref string) error
	delay  time.Duration

	mu          sync.Mutex
	locs        []catalog.Locator
	inFlight    int
	maxInFlight int
}

func (f *fakeFiles) FetchDocument(ctx context.Context, loc catalog.Locator) (*sources.Document, error) {
	f.mu.Lock()
	f.locs = append(f.locs, loc)
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
	}()
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.errFor != nil {
		if err := f.errFor(loc.Ref); err != nil {
			return nil, err
		}
	}
	content, ok := f.files[loc.Ref+":"+loc.Path]
	if !ok {
		return nil, &fetch.Error{URL: "fake://" + loc.Ref + "/" + loc.Path, Status: 404, Err: fetch.ErrNotFound}
	}
	return &sources.Document{
		Locator:     loc,
		URI:         "https://github.com/argoproj/argo-helm/blob/" + loc.Ref + "/" + loc.Path,
		FetchURL:    "https://raw.githubusercontent.com/argoproj/argo-helm/" + loc.Ref + "/" + loc.Path,
		Path:        loc.Path,
		Content:     []byte(content),
		Format:      "yaml",
		Digest:      domain.Digest([]byte(content)),
		RetrievedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}, nil
}

func (f *fakeFiles) reads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.locs)
}

func chartYAML(version, appVersion, kubeVersion string) string {
	s := "apiVersion: v2\nname: argo-cd\nversion: " + version + "\n"
	if appVersion != "" {
		s += "appVersion: " + appVersion + "\n"
	}
	if kubeVersion != "" {
		s += "kubeVersion: '" + kubeVersion + "'\n"
	}
	return s
}

func ref(tag string) sources.ReleaseRef {
	return sources.ReleaseRef{
		Tag:    tag,
		Commit: "c0mm1t-" + tag,
		URL:    "https://github.com/argoproj/argo-helm/releases/tag/" + tag,
		Evidence: domain.NewEvidence(domain.EvidenceGitRef, "", "https://github.com/argoproj/argo-helm/tree/"+tag,
			"refs/tags/"+tag, "tag "+tag, "", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)),
	}
}

// argoFixture mimics argo-helm: out-of-order tags, tags of other charts,
// a non-semver tag and a draft.
func argoFixture() (*fakeTags, *fakeFiles) {
	created := time.Date(2025, 5, 12, 18, 30, 21, 0, time.UTC)
	r801 := ref("argo-cd-8.0.1")
	r801.PublishedAt = &created
	draft := ref("argo-cd-99.0.0")
	draft.Draft = true
	tags := &fakeTags{refs: []sources.ReleaseRef{
		ref("argo-cd-8.0.0"),
		r801,
		ref("argo-cd-10.9.5"),
		ref("argo-workflows-1.0.0"), // another chart in the mono-repo
		ref("argo-cd-latest"),       // not semver
		draft,
		ref("argo-cd-7.9.1"),
		ref("argo-cd-8.0.1-rc.1"),
	}}
	files := &fakeFiles{files: map[string]string{
		"argo-cd-10.9.5:charts/argo-cd/Chart.yaml":     chartYAML("10.9.5", "v3.5.3", ">=1.25.0-0"),
		"argo-cd-8.0.1:charts/argo-cd/Chart.yaml":      chartYAML("8.0.1", "v3.0.0", ">=1.25.0-0"),
		"argo-cd-8.0.1-rc.1:charts/argo-cd/Chart.yaml": chartYAML("8.0.1-rc.1", "v3.0.0-rc2", ""),
		"argo-cd-8.0.0:charts/argo-cd/Chart.yaml":      chartYAML("8.0.0", "v3.0.0", ">=1.25.0-0"),
		"argo-cd-7.9.1:charts/argo-cd/Chart.yaml":      chartYAML("7.9.1", "v2.14.11", ""),
	}}
	return tags, files
}

func TestGitListArtifactVersions(t *testing.T) {
	tags, files := argoFixture()
	g := NewGitAdapter(tags, files)
	vs, err := g.ListArtifactVersions(context.Background(), gitLoc())
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, v := range vs {
		got = append(got, v.Version+"="+v.Fields["appVersion"])
	}
	want := []string{"10.9.5=v3.5.3", "8.0.1=v3.0.0", "8.0.1-rc.1=v3.0.0-rc2", "8.0.0=v3.0.0", "7.9.1=v2.14.11"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("versions = %v\nwant       %v", got, want)
	}

	// Ports are called with the locators they understand.
	if tags.last.Kind != catalog.LocatorGitTags || tags.last.Repository != argoRepo || tags.last.TagPattern != argoPattern {
		t.Errorf("git-tags locator = %+v", tags.last)
	}
	for _, l := range files.locs {
		if l.Kind != catalog.LocatorRepoFile || l.Repository != argoRepo || l.Path != "charts/argo-cd/Chart.yaml" ||
			!strings.HasPrefix(l.Ref, "argo-cd-") {
			t.Errorf("repo-file locator = %+v", l)
		}
	}
	if files.reads() != 5 {
		t.Errorf("Chart.yaml reads = %d, want 5 (non-matching, non-semver and draft tags are not read)", files.reads())
	}

	v := vs[1] // 8.0.1
	if v.Fields["kubeVersion"] != ">=1.25.0-0" || v.Fields["tag"] != "argo-cd-8.0.1" ||
		v.Fields["commit"] != "c0mm1t-argo-cd-8.0.1" || v.Fields["created"] != "2025-05-12T18:30:21Z" {
		t.Errorf("fields = %v", v.Fields)
	}
	if _, ok := vs[4].Fields["kubeVersion"]; ok {
		t.Errorf("kubeVersion must be absent for 7.9.1: %v", vs[4].Fields)
	}
	if v.URI != "https://github.com/argoproj/argo-helm/blob/argo-cd-8.0.1/charts/argo-cd/Chart.yaml" {
		t.Errorf("URI = %q", v.URI)
	}
	ev := v.Evidence
	if ev.Kind != domain.EvidenceStructured || ev.URI != v.URI || ev.Excerpt != "version: 8.0.1, appVersion: v3.0.0" ||
		!strings.HasPrefix(ev.ContentDigest, "sha256:") || ev.ID == "" {
		t.Errorf("evidence = %+v", ev)
	}

	// Lookup: chart versions whose appVersion is v3.0.0.
	var match []string
	for _, v := range vs {
		if v.Fields["appVersion"] == "v3.0.0" {
			match = append(match, v.Version)
		}
	}
	if !reflect.DeepEqual(match, []string{"8.0.1", "8.0.0"}) {
		t.Errorf("appVersion v3.0.0 -> %v", match)
	}
}

func TestGitMaxTags(t *testing.T) {
	tags, files := argoFixture()
	g := NewGitAdapter(tags, files, WithMaxTags(2))
	vs, err := g.ListArtifactVersions(context.Background(), gitLoc())
	if err != nil {
		t.Fatal(err)
	}
	if files.reads() != 2 {
		t.Errorf("Chart.yaml reads = %d, want 2", files.reads())
	}
	if len(vs) != 5 {
		t.Fatalf("all tags stay listed, got %d", len(vs))
	}
	for i, v := range vs {
		_, hasApp := v.Fields["appVersion"]
		if i < 2 && !hasApp {
			t.Errorf("newest tag %s must be inspected", v.Version)
		}
		if i >= 2 {
			if hasApp {
				t.Errorf("tag %s is beyond the cap but carries appVersion", v.Version)
			}
			if v.Fields["tag"] == "" || v.Evidence.Kind != domain.EvidenceGitRef || v.Evidence.ID == "" {
				t.Errorf("uninspected entry = %+v", v)
			}
		}
	}
	if vs[0].Version != "10.9.5" || vs[1].Version != "8.0.1" {
		t.Errorf("the cap must keep the newest tags by semver, got %s, %s", vs[0].Version, vs[1].Version)
	}
}

func TestGitConcurrencyIsBounded(t *testing.T) {
	var refs []sources.ReleaseRef
	fs := map[string]string{}
	for i := 0; i < 40; i++ {
		tag := fmt.Sprintf("argo-cd-1.%d.0", i)
		refs = append(refs, ref(tag))
		fs[tag+":charts/argo-cd/Chart.yaml"] = chartYAML(fmt.Sprintf("1.%d.0", i), "v1.0.0", "")
	}
	files := &fakeFiles{files: fs, delay: 2 * time.Millisecond}
	g := NewGitAdapter(&fakeTags{refs: refs}, files, WithConcurrency(3))
	vs, err := g.ListArtifactVersions(context.Background(), gitLoc())
	if err != nil || len(vs) != 40 {
		t.Fatalf("%d versions, %v", len(vs), err)
	}
	if files.maxInFlight > 3 || files.maxInFlight < 2 {
		t.Errorf("max concurrent reads = %d, want 2..3", files.maxInFlight)
	}
	// Order is newest first regardless of read completion order.
	if vs[0].Version != "1.39.0" || vs[39].Version != "1.0.0" {
		t.Errorf("order: %s ... %s", vs[0].Version, vs[39].Version)
	}
}

func TestGitChartYAMLEdgeCases(t *testing.T) {
	tags := &fakeTags{refs: []sources.ReleaseRef{ref("argo-cd-3.0.0"), ref("argo-cd-2.0.0"), ref("argo-cd-1.0.0")}}
	files := &fakeFiles{files: map[string]string{
		"argo-cd-3.0.0:charts/argo-cd/Chart.yaml": chartYAML("3.0.0", "v3", ""),
		"argo-cd-2.0.0:charts/argo-cd/Chart.yaml": "name: [unterminated\n",
		// argo-cd-1.0.0 has no Chart.yaml at that path
	}}
	vs, err := NewGitAdapter(tags, files).ListArtifactVersions(context.Background(), gitLoc())
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 2 {
		t.Fatalf("got %d entries, want 2 (a tag without Chart.yaml is not a release of this chart): %+v", len(vs), vs)
	}
	if vs[0].Version != "3.0.0" || vs[0].Fields["appVersion"] != "v3" {
		t.Errorf("entry 0 = %+v", vs[0])
	}
	if vs[1].Version != "2.0.0" || vs[1].Fields["appVersion"] != "" || vs[1].Fields["tag"] != "argo-cd-2.0.0" {
		t.Errorf("an undecodable Chart.yaml keeps the entry without chart fields: %+v", vs[1])
	}
}

func TestGitListErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("tags unavailable", func(t *testing.T) {
		tags := &fakeTags{err: &fetch.Error{URL: "git://x", Err: fetch.ErrUnavailable}}
		_, err := NewGitAdapter(tags, &fakeFiles{}).ListArtifactVersions(ctx, gitLoc())
		if !errors.Is(err, fetch.ErrUnavailable) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a Chart.yaml read fails hard", func(t *testing.T) {
		tags, files := argoFixture()
		files.errFor = func(ref string) error {
			if ref == "argo-cd-8.0.0" {
				return &fetch.Error{URL: "raw://x", Err: fetch.ErrUnavailable, Status: 403}
			}
			return nil
		}
		_, err := NewGitAdapter(tags, files, WithConcurrency(1)).ListArtifactVersions(ctx, gitLoc())
		if !errors.Is(err, fetch.ErrUnavailable) || !strings.Contains(err.Error(), "argo-cd-8.0.0") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("context cancelled", func(t *testing.T) {
		tags, files := argoFixture()
		files.delay = time.Second
		cctx, cancel := context.WithCancel(ctx)
		time.AfterFunc(10*time.Millisecond, cancel)
		_, err := NewGitAdapter(tags, files).ListArtifactVersions(cctx, gitLoc())
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("bad locators", func(t *testing.T) {
		g := NewGitAdapter(&fakeTags{}, &fakeFiles{})
		for name, loc := range map[string]catalog.Locator{
			"no repository": {Path: "c", TagPattern: argoPattern},
			"no tagPattern": {Repository: argoRepo, Path: "c"},
			"bad regex":     {Repository: argoRepo, Path: "c", TagPattern: "("},
		} {
			if _, err := g.ListArtifactVersions(ctx, loc); err == nil {
				t.Errorf("%s: expected an error", name)
			}
		}
	})
	t.Run("empty repository", func(t *testing.T) {
		vs, err := NewGitAdapter(&fakeTags{}, &fakeFiles{}).ListArtifactVersions(ctx, gitLoc())
		if err != nil || len(vs) != 0 {
			t.Fatalf("got %v, %v", vs, err)
		}
	})
}

func TestGitTagPatternGroups(t *testing.T) {
	// Pattern without a named group: the first capture group is the version;
	// without any group the whole tag is.
	tags := &fakeTags{refs: []sources.ReleaseRef{ref("v1.18.0"), ref("v1.17.2"), ref("junk")}}
	files := &fakeFiles{files: map[string]string{}}
	for _, tag := range []string{"v1.18.0", "v1.17.2"} {
		files.files[tag+":Chart.yaml"] = chartYAML(strings.TrimPrefix(tag, "v"), "v1", "")
	}
	g := NewGitAdapter(tags, files)
	vs, err := g.ListArtifactVersions(context.Background(), catalog.Locator{Repository: argoRepo, TagPattern: `^v(\d+\.\d+\.\d+)$`})
	if err != nil || len(vs) != 2 || vs[0].Version != "1.18.0" {
		t.Fatalf("group 1: %+v, %v", vs, err)
	}
	// Chart at the repository root: the path is just Chart.yaml.
	if files.locs[0].Path != "Chart.yaml" {
		t.Errorf("path = %q", files.locs[0].Path)
	}
	vs, err = g.ListArtifactVersions(context.Background(), catalog.Locator{Repository: argoRepo, TagPattern: `^v\d+\.\d+\.\d+$`})
	if err != nil || len(vs) != 2 || vs[0].Version != "v1.18.0" {
		t.Fatalf("whole match: %+v, %v", vs, err)
	}
}

func TestGitProbe(t *testing.T) {
	ctx := context.Background()
	tags, files := argoFixture()
	files.files["argo-cd-7.9.1:charts/argo-cd/Chart.yaml"] = "::: not yaml ["
	// tag exists, Chart.yaml absent:
	tags.refs = append(tags.refs, ref("argo-cd-5.0.0"))
	g := NewGitAdapter(tags, files)

	tests := []struct {
		name       string
		version    string
		wantExists bool
		wantCoord  string
		wantExcerp string
		wantKind   domain.EvidenceKind
	}{
		{"exists", "8.0.1", true, argoRepo + " charts/argo-cd@argo-cd-8.0.1", "version: 8.0.1, appVersion: v3.0.0", domain.EvidenceStructured},
		{"prerelease exists", "8.0.1-rc.1", true, argoRepo + " charts/argo-cd@argo-cd-8.0.1-rc.1", "", domain.EvidenceStructured},
		{"semantic equality", "v10.9.5", true, argoRepo + " charts/argo-cd@argo-cd-10.9.5", "", domain.EvidenceStructured},
		{"no such tag", "9.9.9", false, argoRepo + " charts/argo-cd@9.9.9", "no tag matching", domain.EvidenceGitRef},
		{"tag without Chart.yaml", "5.0.0", false, argoRepo + " charts/argo-cd@argo-cd-5.0.0", "tag argo-cd-5.0.0 exists but charts/argo-cd/Chart.yaml is missing", domain.EvidenceGitRef},
		{"undecodable Chart.yaml", "7.9.1", false, argoRepo + " charts/argo-cd@argo-cd-7.9.1", "unusable", domain.EvidenceStructured},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := g.Probe(ctx, gitLoc(), tt.version)
			if err != nil {
				t.Fatal(err)
			}
			if res.Exists != tt.wantExists || res.Coordinate != tt.wantCoord {
				t.Fatalf("exists=%v coordinate=%q; want %v %q", res.Exists, res.Coordinate, tt.wantExists, tt.wantCoord)
			}
			if tt.wantExcerp != "" && !strings.Contains(res.Evidence.Excerpt, tt.wantExcerp) {
				t.Errorf("excerpt = %q, want it to contain %q", res.Evidence.Excerpt, tt.wantExcerp)
			}
			if res.Evidence.Kind != tt.wantKind || res.Evidence.ID == "" || res.URI == "" {
				t.Errorf("evidence = %+v, uri = %q", res.Evidence, res.URI)
			}
		})
	}

	// Probe reads exactly one Chart.yaml per existing tag and lists tags once.
	if got := tags.calls.Load(); got != 1 {
		t.Errorf("git-tags lister called %d times, want 1 (memoized)", got)
	}
	g2 := NewGitAdapter(tags, files, WithMaxTags(1))
	before := files.reads()
	if res, err := g2.Probe(ctx, gitLoc(), "8.0.0"); err != nil || !res.Exists {
		t.Fatalf("probe beyond the cap: %+v, %v", res, err)
	}
	if got := files.reads() - before; got != 1 {
		t.Errorf("Probe read %d files, want 1", got)
	}

	if _, err := g.Probe(ctx, gitLoc(), " "); err == nil {
		t.Error("expected an error for an empty version")
	}
}

func TestGitProbeUnavailable(t *testing.T) {
	ctx := context.Background()
	t.Run("tags", func(t *testing.T) {
		g := NewGitAdapter(&fakeTags{err: &fetch.Error{Err: fetch.ErrUnavailable}}, &fakeFiles{})
		if _, err := g.Probe(ctx, gitLoc(), "1.0.0"); !errors.Is(err, fetch.ErrUnavailable) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("file", func(t *testing.T) {
		tags, files := argoFixture()
		files.errFor = func(string) error { return &fetch.Error{Err: fetch.ErrUnavailable, Status: 403} }
		g := NewGitAdapter(tags, files)
		if _, err := g.Probe(ctx, gitLoc(), "8.0.1"); !errors.Is(err, fetch.ErrUnavailable) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestRegister(t *testing.T) {
	f := fetch.NewHTTPClient(nil, fetch.ModeOnline)

	reg := sources.NewRegistry()
	Register(reg, f, nil, nil)
	if _, err := reg.VersionLister(catalog.LocatorHelmRepo); err != nil {
		t.Errorf("helm-repo version lister: %v", err)
	}
	if _, err := reg.VersionIndex(catalog.LocatorHelmRepo); err != nil {
		t.Errorf("helm-repo version index: %v", err)
	}
	if _, err := reg.Probe(catalog.LocatorHelmRepo); err != nil {
		t.Errorf("helm-repo probe: %v", err)
	}
	if _, err := reg.VersionIndex(catalog.LocatorHelmGit); err == nil {
		t.Error("helm-git must not be registered without its ports")
	}
	if _, err := reg.Probe(catalog.LocatorHelmGit); err == nil {
		t.Error("helm-git probe must not be registered without its ports")
	}

	tags, files := argoFixture()
	reg = sources.NewRegistry()
	Register(reg, f, tags, files, WithMaxTags(10))
	for name, get := range map[string]func() error{
		"helm-git index": func() error { _, err := reg.VersionIndex(catalog.LocatorHelmGit); return err },
		"helm-git probe": func() error { _, err := reg.Probe(catalog.LocatorHelmGit); return err },
	} {
		if err := get(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Only one of the ports: still not registered.
	reg = sources.NewRegistry()
	Register(reg, f, tags, nil)
	if _, err := reg.Probe(catalog.LocatorHelmGit); err == nil {
		t.Error("helm-git must need both ports")
	}
}

func TestParseChartYAML(t *testing.T) {
	m, err := ParseChartYAML([]byte("apiVersion: v2\nname: x\nversion: 1.10\nappVersion: 1.0\nkubeVersion: \">=1.25.0-0\"\ntype: application\ndependencies:\n- name: y\n  version: 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "1.10" || m.AppVersion != "1.0" || m.KubeVersion != ">=1.25.0-0" || m.Name != "x" || m.Type != "application" {
		t.Errorf("meta = %+v", m)
	}
	if _, err := ParseChartYAML([]byte("foo: bar\n")); err == nil {
		t.Error("a document without name and version is not a Chart.yaml")
	}
	if _, err := ParseChartYAML([]byte("name: [")); err == nil {
		t.Error("expected a decode error")
	}
}
