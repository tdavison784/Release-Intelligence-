package upgrade

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run `go test ./internal/upgrade -run %s -update`): %v", t.Name(), err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output differs from %s (run with -update to accept)\n--- got ---\n%s", path, got)
	}
}

func render(t *testing.T, e *domain.UpgradeEdge, opts RenderOptions) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := RenderText(&buf, e, opts); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRenderTextGoldenCertManager(t *testing.T) {
	e := mustBuild(t, cmInput(t))
	golden(t, "cert-manager.golden", render(t, e, RenderOptions{}))
}

// withEnrichment adds an AI enrichment the way the (separate) enrichment step
// would, to exercise its clearly labelled section.
func withEnrichment(t *testing.T, e *domain.UpgradeEdge) *domain.UpgradeEdge {
	t.Helper()
	brk := e.ChangesWhere(func(c domain.Change) bool { return c.Breaking })
	if len(brk) == 0 {
		t.Fatal("fixture has no breaking change")
	}
	gen := fixedNow
	e.Enrichments = append(e.Enrichments, domain.Enrichment{
		ID: "enr-1", Kind: "migration-step", Title: "Grant log access before upgrading",
		Content:   "Add `p, role:dev, logs, get, */*, allow` to argocd-rbac-cm for every role that reads pod logs.\nThen upgrade the control plane.",
		RelatesTo: []string{brk[0].ID},
		Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "llm.enrich@v1", Confidence: domain.ConfidenceMedium,
			Model: "claude-test-model", PromptDigest: "sha256:0123", InputEvidence: brk[0].Evidence, GeneratedAt: &gen},
	})
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestRenderTextGoldenArgoVerbose(t *testing.T) {
	e := withEnrichment(t, mustBuild(t, argoInput(t)))
	golden(t, "argo-cd-verbose.golden", render(t, e, RenderOptions{Verbose: true, MaxPerSection: 4}))
}

func TestRenderTextOptions(t *testing.T) {
	e := mustBuild(t, cmInput(t))

	plain := string(render(t, e, RenderOptions{}))
	if strings.Contains(plain, "\x1b[") {
		t.Error("ANSI codes without Color")
	}
	colored := string(render(t, e, RenderOptions{Color: true}))
	if !strings.Contains(colored, "\x1b[31;1mBreaking changes") {
		t.Error("expected coloured breaking heading")
	}

	small := string(render(t, e, RenderOptions{MaxPerSection: 1}))
	if !strings.Contains(small, "… and ") {
		t.Error("expected truncation marker with MaxPerSection=1")
	}
	unlimited := string(render(t, e, RenderOptions{MaxPerSection: -1, Verbose: true}))
	for _, l := range strings.Split(unlimited, "\n") {
		if strings.HasPrefix(l, "  … and ") {
			t.Errorf("unexpected truncation with MaxPerSection<0: %q", l)
		}
	}

	// features and bug fixes only with Verbose
	if strings.Contains(plain, "Add the `crds.keep` Helm value") {
		t.Error("feature listed without Verbose")
	}
	if !strings.Contains(plain, "1 feature") && !strings.Contains(plain, "2 features") {
		t.Errorf("expected hidden-feature hint:\n%s", plain)
	}
	verbose := string(render(t, e, RenderOptions{Verbose: true}))
	if !strings.Contains(verbose, "Add the `crds.keep` Helm value") {
		t.Error("feature missing with Verbose")
	}
}

func TestRenderNeverPresentsExpectedAsVerified(t *testing.T) {
	e := mustBuild(t, cmInput(t))
	out := string(render(t, e, RenderOptions{Verbose: true}))
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "quay.io/jetstack/charts/cert-manager") && strings.HasPrefix(l, "  chart") {
			if !strings.Contains(l, "[expected]") || strings.Contains(l, "verified") {
				t.Errorf("chart line must show expected status only: %q", l)
			}
			return
		}
	}
	t.Error("chart artifact line not rendered")
}

func TestRenderEveryChangeShowsProvenanceAndEvidence(t *testing.T) {
	e := mustBuild(t, cmInput(t))
	out := string(render(t, e, RenderOptions{Verbose: true, MaxPerSection: -1}))
	for _, c := range e.Changes {
		if isCompatComparison(c) && !c.ActionRequired && !c.Breaking {
			continue // represented by the compatibility summary line
		}
		found := false
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, c.Title) && strings.Contains(l, string(c.Provenance.Method)) && strings.Contains(l, string(c.Evidence[0])) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("change %q not rendered with provenance and evidence", c.Title)
		}
	}
}

func TestRenderHidesDependencyNotesWithoutVerbose(t *testing.T) {
	e := mustBuild(t, argoInput(t))
	const note = "Dex was upgraded to v2.43.0"
	const diff = "Third-party image `ghcr.io/dexidp/dex`: v2.41.1 → v2.43.0"
	plain := string(render(t, e, RenderOptions{MaxPerSection: -1}))
	if strings.Contains(plain, note) || strings.Contains(plain, "Helm was upgraded to 3.18.4") {
		t.Errorf("dependency release notes must be hidden without Verbose:\n%s", plain)
	}
	if !strings.Contains(plain, "2 dependency updates") || !strings.Contains(plain, "not shown; use verbose output") {
		t.Errorf("expected a count hint for the hidden dependency notes:\n%s", plain)
	}
	if !strings.Contains(plain, diff) || !strings.Contains(plain, "Image & dependency changes") {
		t.Errorf("computed image diffs must stay visible:\n%s", plain)
	}
	verbose := string(render(t, e, RenderOptions{Verbose: true, MaxPerSection: -1}))
	if !strings.Contains(verbose, note) || strings.Contains(verbose, "dependency updates") {
		t.Errorf("verbose output lists the dependency notes and has no hint:\n%s", verbose)
	}

	// A dependency note that is breaking or needs action is never hidden.
	from := bare("v1.0.0")
	to := bare("v1.1.0").
		note("notes", domain.RoleReleaseNotes, "https://notes", "Dependencies", "Bump the Go toolchain to 1.23", domain.CategoryDependency).
		note("notes", domain.RoleReleaseNotes, "https://notes", "Dependencies", "Postgres 12 is no longer supported", domain.CategoryDependency, action)
	out := string(render(t, mustBuild(t, simpleInput(from, to)), RenderOptions{}))
	if strings.Contains(out, "Bump the Go toolchain") || !strings.Contains(out, "Postgres 12 is no longer supported") || !strings.Contains(out, "1 dependency update ") {
		t.Errorf("only plain dependency notes are hidden:\n%s", out)
	}
}

func TestRenderTruncatesSourceDetail(t *testing.T) {
	long := "GET https://api.github.com/repos/acme/operator/releases/tags/v1.1.0: HTTP 403 rate limit exceeded\nsecond line of the error body " + strings.Repeat("x", 200)
	from := bare("v1.0.0")
	to := bare("v1.1.0").source("gh", "github-releases", domain.SourceUnavailable, long, domain.RoleChangelog)
	e := mustBuild(t, simpleInput(from, to))
	out := string(render(t, e, RenderOptions{}))
	var line string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "github-releases") {
			line = l
		}
	}
	const prefix = "unavailable: "
	i := strings.Index(line, prefix)
	if line == "" || i < 0 {
		t.Fatalf("source line missing:\n%s", out)
	}
	detail := line[i+len(prefix):]
	if n := len([]rune(detail)); n > 110 || !strings.HasSuffix(detail, "…") || strings.Contains(detail, "\n") {
		t.Errorf("detail must be one line of <= 110 characters ending in an ellipsis (%d): %q", n, detail)
	}
	if !strings.HasPrefix(detail, "GET https://api.github.com/repos/acme/operator/releases/tags/v1.1.0: HTTP 403 rate limit exceeded second") {
		t.Errorf("detail keeps the start of the text: %q", detail)
	}
	found := false
	for _, s := range e.Sources {
		if s.Detail == long {
			found = true
		}
	}
	if !found {
		t.Error("the edge keeps the full detail")
	}
	// short details are untouched
	if got := sourceDetail("quay.io blocked (HTTP 403)"); got != "quay.io blocked (HTTP 403)" {
		t.Errorf("short detail changed: %q", got)
	}
}

func TestRenderNilEdge(t *testing.T) {
	if err := RenderText(&bytes.Buffer{}, nil, RenderOptions{}); err == nil {
		t.Fatal("expected error for nil edge")
	}
}

func TestRenderArtifactStatuses(t *testing.T) {
	from := bare("v1.14.4").
		artifact("ctl", domain.ArtifactContainerImage, "cert-manager-ctl", "quay.io/jetstack/cert-manager-ctl:v1.14.4", domain.ArtifactVerified, "").
		artifact("startupapicheck", domain.ArtifactContainerImage, "cert-manager-startupapicheck", "", domain.ArtifactNotApplicable, "").
		artifact("controller", domain.ArtifactContainerImage, "cert-manager-controller", "quay.io/jetstack/cert-manager-controller:v1.14.4", domain.ArtifactVerified, "").
		artifact("webhook", domain.ArtifactContainerImage, "cert-manager-webhook", "quay.io/jetstack/cert-manager-webhook:v1.14.4", domain.ArtifactVerified, "").
		artifact("crds", domain.ArtifactCRD, "crds", "https://x/crds.yaml", domain.ArtifactVerified, "")
	to := bare("v1.15.0").
		artifact("ctl", domain.ArtifactContainerImage, "cert-manager-ctl", "", domain.ArtifactNotApplicable, "").
		artifact("startupapicheck", domain.ArtifactContainerImage, "cert-manager-startupapicheck", "quay.io/jetstack/cert-manager-startupapicheck:v1.15.0", domain.ArtifactExpected, "").
		artifact("controller", domain.ArtifactContainerImage, "cert-manager-controller", "quay.io/jetstack/cert-manager-controller:v1.15.0", domain.ArtifactExpected, "").
		artifact("webhook", domain.ArtifactContainerImage, "cert-manager-webhook", "quay.io/jetstack/cert-manager-webhook:v1.15.0", domain.ArtifactMissing, "").
		artifact("crds", domain.ArtifactCRD, "crds", "https://x/crds.yaml", domain.ArtifactVerified, "")
	e := mustBuild(t, simpleInput(from, to))
	out := string(render(t, e, RenderOptions{}))
	for _, want := range []string{
		"Artifact changes (2 updated, 1 added, 1 removed, 1 unchanged):",
		"ctl              − quay.io/jetstack/cert-manager-ctl:v1.14.4  (removed)  [verified → not-applicable]",
		"startupapicheck  + quay.io/jetstack/cert-manager-startupapicheck:v1.15.0  (added)  [not-applicable → expected]  (computed · no evidence)",
		"controller       quay.io/jetstack/cert-manager-controller  v1.14.4 → v1.15.0  [verified → expected]",
		"webhook          quay.io/jetstack/cert-manager-webhook  v1.14.4 → v1.15.0  [verified → missing]",
		"(1 unchanged: crds)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	verbose := string(render(t, e, RenderOptions{Verbose: true}))
	if !strings.Contains(verbose, "https://x/crds.yaml  (unchanged)  [verified]") {
		t.Errorf("unchanged artifact must be listed with Verbose:\n%s", verbose)
	}
}
