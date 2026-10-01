package app

// End-to-end reproducibility tests.
//
// They run the real pipeline (App.Upgrade: catalog -> version listing ->
// ingestion of every release on the path -> upgrade.Build) for three real
// products completely OFFLINE, against a checked-in recording of the upstream
// data (testdata/e2e/state/cache), with a fixed clock, and compare the result
// with golden files (testdata/e2e/golden). See testdata/e2e/README.md for how
// to refresh the recording and the goldens.
//
//	go test ./internal/app                               replay + compare
//	go test ./internal/app -run TestE2E -update          rewrite the goldens
//	go test ./internal/app -record -update -count=1      re-record online, then rewrite the goldens

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

var (
	updateGolden = flag.Bool("update", false, "rewrite the golden files in testdata/e2e/golden")
	recordLive   = flag.Bool("record", false, "re-record testdata/e2e/state/cache from the live upstreams (needs network); run together with -update to refresh the goldens")
	recordDeny   = flag.String("record-deny", "api.github.com,quay.io,charts.jetstack.io,argoproj.github.io",
		"comma-separated hosts treated as unreachable while recording, so that the fixture matches the checked-in one "+
			"(these hosts were blocked in the environment of the original recording); pass -record-deny= to record everything")
)

const (
	e2eDir        = "testdata/e2e"
	productsDir   = "../../products"
	fixtureBudget = 8 << 20 // the recording must stay below 8 MiB
)

// fixedNow is the injected clock of every e2e run. Only UpgradeEdge.GeneratedAt
// (and ingestion timestamps that never reach the edge) depend on it.
var fixedNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// e2eCase is one recorded upgrade.
type e2eCase struct {
	product, from, to string
	check             func(t *testing.T, e *domain.UpgradeEdge)
}

func (c e2eCase) name() string { return c.product + "_" + c.from + "_" + c.to }

var e2eCases = []e2eCase{
	{"cert-manager", "v1.17.0", "v1.18.0", checkCertManager},
	{"istio", "1.29.2", "1.30.1", checkIstio},
	{"argo-cd", "v2.14.11", "v3.0.6", checkArgoCD},
}

// TestRecordFixtures re-records the fixture from the live upstreams. It is
// skipped unless -record is given and must stay the first test of this file:
// go runs tests in source order, so "-record -update" records first and then
// rewrites the goldens from the new recording.
func TestRecordFixtures(t *testing.T) {
	if !*recordLive {
		t.Skip("fixtures are re-recorded only with -record (needs network access)")
	}
	// Record into a scratch directory and replace the fixture only on
	// success, so a failed recording never destroys a good one.
	scratch := t.TempDir()
	a, err := New(Config{ProductsDir: productsDir, StateDir: scratch, GitHubToken: GitHubTokenFromEnv(), Logf: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	if hosts := splitList(*recordDeny); len(hosts) > 0 {
		hc, ok := a.Fetch.(*fetch.HTTPClient)
		if !ok {
			t.Fatalf("fetch client is %T, cannot install the -record-deny filter", a.Fetch)
		}
		hc.HTTP = &http.Client{Timeout: hc.HTTP.Timeout, Transport: denyTransport{next: http.DefaultTransport, hosts: hosts}}
		t.Logf("treating as unreachable: %s", strings.Join(hosts, ", "))
	}
	for _, c := range e2eCases {
		edge, err := a.Upgrade(context.Background(), c.product, c.from, c.to, UpgradeOptions{})
		if err != nil {
			t.Fatalf("record %s: %v", c.name(), err)
		}
		if err := edge.Validate(); err != nil {
			t.Fatalf("record %s: edge does not validate: %v", c.name(), err)
		}
	}
	// Keep only what offline replay needs: the HTTP cache and the git result
	// files (tags, log, dir). The store is output, the clones are accelerators.
	dst := filepath.Join(e2eDir, "state")
	if err := os.RemoveAll(dst); err != nil {
		t.Fatal(err)
	}
	err = copyTree(scratch, dst, func(rel string, d fs.DirEntry) bool {
		switch rel {
		case "store", "cache/git/mirrors", "cache/git/work":
			return true
		}
		return strings.HasPrefix(d.Name(), ".tmp-")
	})
	if err != nil {
		t.Fatal(err)
	}
	size, biggest := treeSize(t, dst)
	t.Logf("recorded %s: %.2f MiB; largest entries:", dst, float64(size)/(1<<20))
	for _, f := range biggest {
		t.Logf("  %8d  %s", f.size, f.path)
	}
	if size > fixtureBudget {
		t.Errorf("recording is %d bytes, over the %d byte budget", size, fixtureBudget)
	}
	t.Logf("done; run with -update (or `go test ./internal/app -update`) to refresh the goldens")
}

// TestE2EUpgrade replays each recorded upgrade offline, checks the key
// semantic facts and compares text and JSON with the golden files.
func TestE2EUpgrade(t *testing.T) {
	for _, c := range e2eCases {
		t.Run(c.name(), func(t *testing.T) {
			a := newReplayApp(t, fixedNow)
			e := mustUpgrade(t, a, c)
			if err := e.Validate(); err != nil {
				t.Fatalf("edge does not validate: %v", err)
			}
			checkEdgeInvariants(t, e)
			c.check(t, e)

			var text bytes.Buffer
			if err := upgrade.RenderText(&text, e, upgrade.RenderOptions{MaxPerSection: 25, Color: false}); err != nil {
				t.Fatalf("render text: %v", err)
			}
			checkGolden(t, c.name()+".txt", text.Bytes())
			checkGolden(t, c.name()+".json", edgeJSON(t, e))
		})
	}
}

// TestE2EDeterminism proves that the output is a pure function of the
// recording, the product definitions and the injected clock.
func TestE2EDeterminism(t *testing.T) {
	for _, c := range e2eCases {
		t.Run(c.name(), func(t *testing.T) {
			if testing.Short() && c.product == "argo-cd" {
				t.Skip("slow (about 30 s under -race); TestE2EUpgrade still covers it")
			}
			a := newReplayApp(t, fixedNow)
			first := edgeJSON(t, mustUpgrade(t, a, c))

			// A second run on the same App: the store now holds the first
			// run's output and must not influence the result.
			if got := edgeJSON(t, mustUpgrade(t, a, c)); !bytes.Equal(got, first) {
				t.Fatalf("second run on the same app differs: %s", firstDiff(got, first))
			}

			// An independent App over a fresh copy of the state, two days
			// later: the injected clock must be the only source of time, so
			// generatedAt changes and nothing else does. Real time is made
			// to cross a second boundary first, so that any timestamp that
			// still came from time.Now (evidence times are truncated to
			// seconds) would show up as a difference.
			waitForNextSecond()
			later := fixedNow.Add(48 * time.Hour)
			e := mustUpgrade(t, newReplayApp(t, later), c)
			if !e.GeneratedAt.Equal(later) {
				t.Fatalf("GeneratedAt = %v, want the injected clock %v", e.GeneratedAt, later)
			}
			e.GeneratedAt = fixedNow
			if got := edgeJSON(t, e); !bytes.Equal(got, first) {
				t.Fatalf("output of a fresh app depends on the clock beyond generatedAt: %s", firstDiff(got, first))
			}
		})
	}
}

// TestE2EFixtureHygiene keeps the recording small and free of clones and
// store output, which belong neither in the fixture nor in git.
func TestE2EFixtureHygiene(t *testing.T) {
	state := filepath.Join(e2eDir, "state")
	for _, rel := range []string{"store", "cache/git/mirrors", "cache/git/work"} {
		if _, err := os.Stat(filepath.Join(state, rel)); err == nil {
			t.Errorf("%s/%s must not be part of the fixture (re-record with -record or delete it)", state, rel)
		}
	}
	size, biggest := treeSize(t, state)
	if size == 0 {
		t.Fatalf("%s is empty; record the fixture with -record", state)
	}
	if size > fixtureBudget {
		t.Errorf("fixture is %d bytes, over the %d byte budget; largest entries: %v", size, fixtureBudget, biggest[:min(3, len(biggest))])
	}
}

// --- semantic checks --------------------------------------------------------

func checkCertManager(t *testing.T, e *domain.UpgradeEdge) {
	expectPath(t, e, "minor-lineage", []string{"v1.18.0"}, 4)

	br := breaking(e)
	if len(br) < 3 {
		t.Errorf("want >= 3 breaking changes, got %d", len(br))
	}
	rot := requireChange(t, br, "breaking change about RotationPolicy", titleContains("RotationPolicy"))
	if rot.Provenance.Method != domain.MethodDeclared {
		t.Errorf("RotationPolicy change: method = %s, want declared", rot.Provenance.Method)
	}

	crds := e.ChangesWhere(func(c domain.Change) bool { return c.Category == domain.CategoryCRDSchema })
	if len(crds) < 4 {
		t.Errorf("want >= 4 CRD schema changes, got %d", len(crds))
	}
	requireChange(t, crds, "Certificate gains spec.signatureAlgorithm", func(c domain.Change) bool {
		return slices.Contains(c.Subjects, "spec.signatureAlgorithm") && strings.HasPrefix(c.Title, "Certificate v1")
	})

	k8s := requireCompat(t, e, "kubernetes", "supported")
	wantVersions := []string{"1.29", "1.30", "1.31", "1.32", "1.33"}
	if !slices.Equal(k8s.To.Versions, wantVersions) {
		t.Errorf("target Kubernetes support = %v, want %v", k8s.To.Versions, wantVersions)
	}
	if k8s.Narrowed || !strings.Contains(k8s.Summary, "1.29–1.33") {
		t.Errorf("Kubernetes summary = %q (narrowed=%v), want an unchanged 1.29–1.33", k8s.Summary, k8s.Narrowed)
	}

	// quay.io was unreachable when recording: images are only referenced by
	// the verified install manifest, never claimed as verified.
	for _, id := range []string{"controller-image", "webhook-image", "cainjector-image", "acmesolver-image"} {
		requireStatus(t, e, id, domain.ArtifactReferenced)
	}
	requireStatus(t, e, "startupapicheck-image", domain.ArtifactExpected)
	requireStatus(t, e, "helm-chart", domain.ArtifactExpected)
	requireStatus(t, e, "install-manifest", domain.ArtifactVerified)
	requireStatus(t, e, "crds", domain.ArtifactVerified)

	requireSource(t, e, "upgrade-guide", "1.18.0", domain.SourceOK)
	requireSource(t, e, "supported-releases", "1.18.0", domain.SourceOK)
	requireSource(t, e, "github-advisories", "", domain.SourceUnavailable)
	requireSource(t, e, "controller-image", "1.18.0", domain.SourceUnavailable)
}

func checkIstio(t *testing.T, e *domain.UpgradeEdge) {
	expectPath(t, e, "minor-lineage", []string{"1.30.0", "1.30.1"}, 6)

	ext := requireChange(t, e.Changes, "new TrafficExtension CRD", func(c domain.Change) bool {
		return c.Category == domain.CategoryCRDSchema && c.Provenance.Rule == "crd:added" &&
			slices.Contains(c.Subjects, "trafficextensions.extensions.istio.io")
	})
	if !strings.Contains(ext.Title, "TrafficExtension") || ext.Provenance.Method != domain.MethodComputed {
		t.Errorf("TrafficExtension change = %q (%s), want a computed change naming TrafficExtension", ext.Title, ext.Provenance.Method)
	}

	sup := requireCompat(t, e, "kubernetes", "supported")
	if !sup.Narrowed {
		t.Errorf("Kubernetes support must be narrowed: %s", sup.Summary)
	}
	if !slices.Contains(sup.From.Versions, "1.31") || slices.Contains(sup.To.Versions, "1.31") || !slices.Contains(sup.To.Versions, "1.36") {
		t.Errorf("Kubernetes support %v -> %v: want 1.31 dropped and 1.36 added", sup.From.Versions, sup.To.Versions)
	}
	if tested := requireCompat(t, e, "kubernetes", "tested"); !tested.Narrowed || slices.Contains(tested.To.Versions, "1.26") {
		t.Errorf("Kubernetes tested versions %v -> %v: want a narrowed range without 1.26", tested.From.Versions, tested.To.Versions)
	}
	narrowed := e.ChangesWhere(func(c domain.Change) bool {
		return c.Category == domain.CategoryCompatibility && strings.Contains(c.Title, "Kubernetes support narrowed")
	})
	if len(narrowed) != 1 || !narrowed[0].ActionRequired {
		t.Errorf("want one action-required 'Kubernetes support narrowed' change, got %d", len(narrowed))
	}

	requireChange(t, breaking(e), "breaking change about Gateway API CRDs", titleContains("Gateway API"))

	// Everything istio publishes was reachable when recording.
	for _, ac := range e.Artifacts {
		if ac.To == nil || ac.To.Status != domain.ArtifactVerified {
			t.Errorf("artifact %s: to = %+v, want verified", ac.ArtifactID, ac.To)
		}
	}
	if len(e.Artifacts) != 14 {
		t.Errorf("want 14 artifacts, got %d", len(e.Artifacts))
	}
	requireSource(t, e, "pilot-image", "1.30.1", domain.SourceOK)
	requireSource(t, e, "github-advisories", "", domain.SourceUnavailable)
}

func checkArgoCD(t *testing.T, e *domain.UpgradeEdge) {
	expectPath(t, e, "minor-lineage", []string{"v3.0.0", "v3.0.1", "v3.0.2", "v3.0.3", "v3.0.4", "v3.0.5", "v3.0.6"}, 10)

	// The chart version differs from the app version: it is found by
	// appVersion in the chart repository index.
	chart := requireArtifact(t, e, "helm-chart")
	if chart.From == nil || chart.To == nil {
		t.Fatalf("helm-chart endpoints: %+v", chart)
	}
	if !strings.HasPrefix(chart.From.Version, "7.") || !strings.HasPrefix(chart.To.Version, "8.") {
		t.Errorf("helm-chart %s -> %s, want chart 7.x -> 8.x", chart.From.Version, chart.To.Version)
	}
	if chart.To.Version == e.To.Semver || chart.To.Status != domain.ArtifactVerified {
		t.Errorf("helm-chart target = %+v, want a verified version different from the app version %s", chart.To, e.To.Semver)
	}
	var lookup *domain.Evidence
	for i := range e.Evidence {
		ev := &e.Evidence[i]
		if slices.Contains(chart.To.Evidence, ev.ID) && ev.Kind == domain.EvidenceRegistry {
			lookup = ev
		}
	}
	if lookup == nil || !strings.Contains(lookup.Excerpt, "version: "+chart.To.Version) ||
		!strings.Contains(lookup.Excerpt, "appVersion: "+e.To.Tag) {
		t.Errorf("chart evidence = %+v, want an index entry mapping chart %s to appVersion %s", lookup, chart.To.Version, e.To.Tag)
	}
	requireStatus(t, e, "image", domain.ArtifactReferenced) // quay.io unreachable when recording
	requireStatus(t, e, "install-manifest", domain.ArtifactVerified)

	// Breaking changes come from the 2.14 -> 3.0 upgrade guide.
	evs := evidenceIndex(e)
	var fromGuide []domain.Change
	for _, c := range breaking(e) {
		for _, id := range c.Evidence {
			if strings.HasSuffix(evs[id].URI, "/docs/operator-manual/upgrading/2.14-3.0.md") {
				fromGuide = append(fromGuide, c)
				break
			}
		}
	}
	if len(fromGuide) < 6 {
		t.Errorf("want >= 6 breaking changes from the 2.14->3.0 upgrade guide, got %d", len(fromGuide))
	}
	requireChange(t, fromGuide, "Dex RBAC subject change", titleContains("Changes to RBAC with Dex SSO Authentication"))
	requireChange(t, fromGuide, "annotation tracking default", titleContains("Use Annotation-Based Tracking by Default"))
	if n := len(breaking(e)); n < 10 {
		t.Errorf("want >= 10 breaking changes in total (guide + conventional commits), got %d", n)
	}

	crds := e.ChangesWhere(func(c domain.Change) bool { return c.Category == domain.CategoryCRDSchema })
	requireChange(t, crds, "ApplicationSet schema gains fields", titleContains("ApplicationSet v1alpha1 schema"))

	tested := requireCompat(t, e, "kubernetes", "tested")
	if !tested.Narrowed || !slices.Contains(tested.From.Versions, "1.28") || slices.Contains(tested.To.Versions, "1.28") {
		t.Errorf("Kubernetes tested versions %v -> %v: want 1.28 dropped", tested.From.Versions, tested.To.Versions)
	}

	requireSource(t, e, "upgrade-guide", "3.0.0", domain.SourceOK)
	requireSource(t, e, "commit-log", "3.0.0", domain.SourceOK)
	requireSource(t, e, "github-releases", "", domain.SourceUnavailable)
}

// checkEdgeInvariants are the properties every edge must have, whatever the product.
func checkEdgeInvariants(t *testing.T, e *domain.UpgradeEdge) {
	t.Helper()
	if e.SchemaVersion != domain.UpgradeEdgeSchemaVersion {
		t.Errorf("schemaVersion = %q", e.SchemaVersion)
	}
	if !e.GeneratedAt.Equal(fixedNow) {
		t.Errorf("generatedAt = %v, want the injected clock %v", e.GeneratedAt, fixedNow)
	}
	if len(e.Enrichments) != 0 {
		t.Errorf("the deterministic pipeline produced %d enrichments", len(e.Enrichments))
	}
	if e.DefinitionDigest == "" {
		t.Error("definitionDigest is empty")
	}
	if len(e.Changes) == 0 {
		t.Error("no changes")
	}
	for _, ev := range e.Evidence {
		if ev.URI == "" || ev.RetrievedAt.IsZero() {
			t.Errorf("evidence %s lacks a URI or a retrieval time: %+v", ev.ID, ev)
		}
	}
	for _, s := range e.Sources {
		switch s.State {
		case domain.SourceOK, domain.SourceSkipped, domain.SourcePartial:
		case domain.SourceUnavailable:
			// Offline replay can only report what was missing at recording
			// time as "not in cache"; it must never look like a bug.
			if !strings.Contains(s.Detail, "not in cache (offline mode)") {
				t.Errorf("source %s (%s) unavailable for an unexpected reason: %s", s.SourceID, s.Version, s.Detail)
			}
		default:
			t.Errorf("source %s (%s) is in state %s: %s", s.SourceID, s.Version, s.State, s.Detail)
		}
	}
}

// --- helpers ------------------------------------------------------------------

// newReplayApp builds an App that replays the recording: offline, with the
// definitions of the repository's products/ directory and an injected clock.
// The state is a private copy of the fixture, so tests never modify testdata.
// It also makes accidental network or git use fail loudly.
func newReplayApp(t *testing.T, now time.Time) *App {
	t.Helper()
	state := t.TempDir()
	if err := copyTree(filepath.Join(e2eDir, "state"), state, nil); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	// Offline replay must not need git: with an empty PATH any attempt to
	// start it would fail and show up as an unavailable source.
	t.Setenv("PATH", t.TempDir())
	a, err := New(Config{
		ProductsDir: productsDir,
		StateDir:    state,
		Offline:     true,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if hc, ok := a.Fetch.(*fetch.HTTPClient); ok {
		hc.HTTP = &http.Client{Transport: tripwire{t}}
	} else {
		t.Fatalf("fetch client is %T, cannot install the network tripwire", a.Fetch)
	}
	return a
}

// tripwire fails the test when anything tries to use the network.
type tripwire struct{ t *testing.T }

func (w tripwire) RoundTrip(r *http.Request) (*http.Response, error) {
	w.t.Errorf("offline replay attempted network access: %s %s", r.Method, r.URL)
	return nil, errors.New("network access in offline replay")
}

// denyTransport makes the listed hosts unreachable while recording.
type denyTransport struct {
	next  http.RoundTripper
	hosts []string
}

func (d denyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if slices.Contains(d.hosts, r.URL.Hostname()) {
		return nil, fmt.Errorf("host %s is blocked by -record-deny", r.URL.Hostname())
	}
	return d.next.RoundTrip(r)
}

// waitForNextSecond returns once the wall clock is in a later second than it
// was when the call started.
func waitForNextSecond() {
	now := time.Now()
	time.Sleep(time.Until(now.Truncate(time.Second).Add(time.Second)) + 10*time.Millisecond)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mustUpgrade(t *testing.T, a *App, c e2eCase) *domain.UpgradeEdge {
	t.Helper()
	e, err := a.Upgrade(context.Background(), c.product, c.from, c.to, UpgradeOptions{})
	if err != nil {
		t.Fatalf("upgrade %s %s -> %s: %v", c.product, c.from, c.to, err)
	}
	return e
}

// edgeJSON renders the edge exactly as `ri upgrade -o json` does.
func edgeJSON(t *testing.T, e *domain.UpgradeEdge) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(e); err != nil {
		t.Fatalf("encode edge: %v", err)
	}
	return buf.Bytes()
}

// checkGolden compares got with testdata/e2e/golden/<name> (or rewrites it with -update).
func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join(e2eDir, "golden", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s (%d bytes)", path, len(got))
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (create it with -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the recording-derived output: %s\n(if the change is intended, run: go test ./internal/app -run TestE2E -update)", path, firstDiff(got, want))
	}
}

// firstDiff describes where two outputs first differ.
func firstDiff(got, want []byte) string {
	g, w := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
	clip := func(s string) string {
		if len(s) > 220 {
			return s[:220] + "…"
		}
		return s
	}
	for i := 0; i < min(len(g), len(w)); i++ {
		if g[i] != w[i] {
			return fmt.Sprintf("first difference at line %d of %d (got) / %d (want)\n  got:  %s\n  want: %s", i+1, len(g), len(w), clip(g[i]), clip(w[i]))
		}
	}
	return fmt.Sprintf("identical for the first %d lines, but %d (got) vs %d (want) lines in total", min(len(g), len(w)), len(g), len(w))
}

type fileSize struct {
	path string
	size int64
}

// treeSize returns the total size of the regular files below dir and the
// largest ones, biggest first.
func treeSize(t *testing.T, dir string) (total int64, biggest []fileSize) {
	t.Helper()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		biggest = append(biggest, fileSize{p, info.Size()})
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	sort.Slice(biggest, func(i, j int) bool { return biggest[i].size > biggest[j].size })
	if len(biggest) > 5 {
		biggest = biggest[:5]
	}
	return total, biggest
}

// copyTree copies the directory src to dst; skip (optional) is consulted with
// the slash-separated path relative to src and may exclude files or whole
// directories.
func copyTree(src, dst string, skip func(rel string, d fs.DirEntry) bool) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if skip != nil && skip(filepath.ToSlash(rel), d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}

func expectPath(t *testing.T, e *domain.UpgradeEdge, policy string, tags []string, skipped int) {
	t.Helper()
	var got []string
	for _, s := range e.Path {
		got = append(got, s.Version.Tag)
	}
	if e.PathPolicy != policy || !slices.Equal(got, tags) || len(e.SkippedReleases) != skipped {
		t.Errorf("path = %s %v (%d skipped), want %s %v (%d skipped)", e.PathPolicy, got, len(e.SkippedReleases), policy, tags, skipped)
	}
}

func breaking(e *domain.UpgradeEdge) []domain.Change {
	return e.ChangesWhere(func(c domain.Change) bool { return c.Breaking })
}

func titleContains(s string) func(domain.Change) bool {
	return func(c domain.Change) bool { return strings.Contains(c.Title, s) }
}

func requireChange(t *testing.T, cs []domain.Change, what string, pred func(domain.Change) bool) domain.Change {
	t.Helper()
	for _, c := range cs {
		if pred(c) {
			return c
		}
	}
	t.Errorf("no change found: %s", what)
	return domain.Change{}
}

func requireArtifact(t *testing.T, e *domain.UpgradeEdge, id string) domain.ArtifactChange {
	t.Helper()
	for _, a := range e.Artifacts {
		if a.ArtifactID == id {
			return a
		}
	}
	t.Fatalf("artifact %s missing from the edge", id)
	return domain.ArtifactChange{}
}

// requireStatus checks the status of the artifact at both endpoints.
func requireStatus(t *testing.T, e *domain.UpgradeEdge, id string, want domain.ArtifactStatus) {
	t.Helper()
	a := requireArtifact(t, e, id)
	if a.From == nil || a.To == nil || a.From.Status != want || a.To.Status != want {
		t.Errorf("artifact %s: from=%v to=%v, want %s at both endpoints", id, statusOf(a.From), statusOf(a.To), want)
	}
}

func statusOf(i *domain.ArtifactInstance) any {
	if i == nil {
		return nil
	}
	return i.Status
}

func requireCompat(t *testing.T, e *domain.UpgradeEdge, platform, kind string) domain.CompatibilityChange {
	t.Helper()
	for _, c := range e.Compatibility {
		if c.Platform == platform && c.To != nil && c.To.Kind == kind && c.From != nil {
			return c
		}
	}
	t.Fatalf("no %s %s compatibility change with both endpoints", platform, kind)
	return domain.CompatibilityChange{}
}

// requireSource checks that some status of the source (at the given release
// semver, "" for product-level sources) has the wanted state.
func requireSource(t *testing.T, e *domain.UpgradeEdge, id, version string, want domain.SourceState) {
	t.Helper()
	var seen []domain.SourceState
	for _, s := range e.Sources {
		if s.SourceID == id && s.Version == version {
			if s.State == want {
				return
			}
			seen = append(seen, s.State)
		}
	}
	t.Errorf("source %s (%q): want state %s, got %v", id, version, want, seen)
}

func evidenceIndex(e *domain.UpgradeEdge) map[domain.EvidenceID]domain.Evidence {
	m := make(map[domain.EvidenceID]domain.Evidence, len(e.Evidence))
	for _, ev := range e.Evidence {
		m[ev.ID] = ev
	}
	return m
}
