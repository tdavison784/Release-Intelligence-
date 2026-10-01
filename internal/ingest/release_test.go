package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// ingestFixture lists versions from the default world, applies setup (which
// may break parts of the world after listing) and ingests one release.
func ingestFixture(t *testing.T, semver string, setup func(w *world, def *catalog.ProductDefinition)) (*domain.Release, *world, *fakeParser) {
	t.Helper()
	w, def, p := newWorld(), testDef(), &fakeParser{}
	ing := newTestIngester(w, p)
	vl := mustVersions(t, ing, def)
	if setup != nil {
		setup(w, def)
	}
	rel, err := ing.IngestRelease(context.Background(), def, version(t, vl, semver), vl)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceIntegrity(t, rel)
	return rel, w, p
}

func lineOf(t *testing.T, content, needle string) int {
	t.Helper()
	n, _, ok := findLine([]byte(content), needle)
	if !ok {
		t.Fatalf("%q not in content", needle)
	}
	return n
}

func TestIngestReleaseSources(t *testing.T) {
	rel, w, p := ingestFixture(t, "1.2.0", nil)

	if rel.Version.Tag != "v1.2.0" || rel.Product != "acme" {
		t.Fatalf("release identity: %+v", rel.Version)
	}
	if !rel.IngestedAt.Equal(testNow) || rel.DefinitionDigest == "" {
		t.Fatalf("ingestedAt %v digest %q", rel.IngestedAt, rel.DefinitionDigest)
	}

	// markdown-section: the selected section is parsed with a line offset
	// so that evidence points into the original document.
	site := statusOf(t, rel, "site-notes")
	if site.State != domain.SourceOK || !strings.Contains(site.Detail, `section "v1.2.0"`) {
		t.Fatalf("site-notes: %+v", site)
	}
	calls := p.notesFor("site-notes")
	if len(calls) != 1 {
		t.Fatalf("site-notes parsed %d times", len(calls))
	}
	in := calls[0]
	heading := lineOf(t, notes12, "## `v1.2.0`")
	if in.LineOffset != heading-1 || !strings.HasPrefix(string(in.Content), "## `v1.2.0`") || strings.Contains(string(in.Content), "v1.2.1") {
		t.Fatalf("section input: offset=%d content=%q", in.LineOffset, in.Content)
	}
	if in.Release != "1.2.0" || in.Role != domain.RoleReleaseNotes || in.Repository != website || in.Digest == "" || in.URI == "" {
		t.Fatalf("doc input: %+v", in)
	}
	var widget *domain.NoteItem
	for k := range rel.Notes {
		if rel.Notes[k].Text == "Added widget support" {
			widget = &rel.Notes[k]
		}
	}
	if widget == nil {
		t.Fatalf("note missing: %+v", rel.Notes)
	}
	ev, _ := evidenceByID(rel, widget.Evidence[0])
	if want := lineOf(t, notes12, "- Added widget support"); ev.Locator != "L"+itoa(want) {
		t.Fatalf("note evidence locator %q, want L%d", ev.Locator, want)
	}

	// Fallback group satisfied by the first ok member: the GitHub release
	// body is never fetched.
	gh := statusOf(t, rel, "gh-notes")
	if gh.State != domain.SourceSkipped || !strings.Contains(gh.Detail, "satisfied by site-notes") {
		t.Fatalf("gh-notes: %+v", gh)
	}
	if n := w.callCount("github-releases:acme/operator@v1.2.0"); n != 0 {
		t.Fatalf("fallback fetched %d times", n)
	}

	// Upgrade guide of a minor release uses PrevLine from the known list.
	up := statusOf(t, rel, "upgrade-guide")
	if up.State != domain.SourceOK || !strings.Contains(up.URI, "upgrading-1.1-1.2.md") {
		t.Fatalf("upgrade-guide: %+v", up)
	}
	if len(p.notesFor("upgrade-guide")) != 1 {
		t.Fatal("upgrade guide not parsed into notes")
	}

	// markdown-table → compatibility constraint with the source id filled in.
	var compat *domain.CompatibilityConstraint
	for k := range rel.Compat {
		if rel.Compat[k].SourceID == "support-matrix" {
			compat = &rel.Compat[k]
		}
	}
	if compat == nil || compat.Platform != "kubernetes" || compat.Raw != "1.30 → 1.33" {
		t.Fatalf("compat: %+v", rel.Compat)
	}
	if len(p.notesFor("support-matrix")) != 0 {
		t.Fatal("table extracts must not be parsed as notes")
	}

	// Not applicable / unresolvable / no adapter.
	if s := statusOf(t, rel, "legacy-changelog"); s.State != domain.SourceSkipped || !strings.Contains(s.Detail, `availability "< 1.0.0"`) {
		t.Fatalf("legacy-changelog: %+v", s)
	}
	if s := statusOf(t, rel, "commit-log"); s.State != domain.SourceSkipped || !strings.Contains(s.Detail, "no document adapter") {
		t.Fatalf("commit-log: %+v", s)
	}
	// Security / versions sources are product level, not per release.
	for _, id := range []string{"gh-releases", "git-tags", "advisories", "bulletins"} {
		if len(statusesOf(rel, id)) != 0 {
			t.Errorf("%s must not be consulted per release", id)
		}
	}

	// Facts: release published + one document.retrieved per fetched source.
	pub := factsOf(rel, domain.FactReleasePublished)
	if len(pub) != 1 || pub[0].Release != "1.2.0" || pub[0].Attributes["source"] != "gh-releases" {
		t.Fatalf("release fact: %+v", pub)
	}
	docs := factsOf(rel, domain.FactDocumentRetrieved)
	got := map[string]bool{}
	for _, f := range docs {
		got[f.Attributes["source"]] = true
	}
	for _, want := range []string{"site-notes", "upgrade-guide", "support-matrix"} {
		if !got[want] {
			t.Errorf("no document.retrieved fact for %s: %+v", want, docs)
		}
	}
	if len(factsOf(rel, domain.FactCompatibility)) == 0 {
		t.Error("no compatibility fact")
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestIngestReleaseSectionNotFoundFallsBack(t *testing.T) {
	rel, w, p := ingestFixture(t, "1.1.1", nil)
	site := statusOf(t, rel, "site-notes")
	if site.State != domain.SourceNotFound || !strings.Contains(site.Detail, `no section matching "^v1\\.1\\.1$"`) {
		t.Fatalf("site-notes: %+v", site)
	}
	gh := statusOf(t, rel, "gh-notes")
	if gh.State != domain.SourceOK {
		t.Fatalf("gh-notes: %+v", gh)
	}
	if w.callCount("github-releases:acme/operator@v1.1.1") != 1 {
		t.Fatal("fallback source not fetched")
	}
	if len(p.notesFor("gh-notes")) != 1 || len(rel.Notes) != 1 || rel.Notes[0].SourceID != "gh-notes" {
		t.Fatalf("notes: %+v", rel.Notes)
	}
	if in := p.notesFor("gh-notes")[0]; in.Repository != "acme/operator" || in.LineOffset != 0 {
		t.Fatalf("whole-document input: %+v", in)
	}
	// Patch release: the minor/major-only upgrade guide is not applicable.
	if s := statusOf(t, rel, "upgrade-guide"); s.State != domain.SourceSkipped || !strings.Contains(s.Detail, "patch release") {
		t.Fatalf("upgrade-guide: %+v", s)
	}
}

func TestIngestReleaseFallbackGroupExhausted(t *testing.T) {
	rel, _, _ := ingestFixture(t, "1.1.1", func(w *world, _ *catalog.ProductDefinition) {
		w.down[catalog.LocatorGitHubReleases] = true
	})
	if s := statusOf(t, rel, "site-notes"); s.State != domain.SourceNotFound {
		t.Fatalf("site-notes: %+v", s)
	}
	if s := statusOf(t, rel, "gh-notes"); s.State != domain.SourceUnavailable || !strings.Contains(s.Detail, "blocked by proxy") {
		t.Fatalf("gh-notes: %+v", s)
	}
	if len(rel.Notes) != 0 {
		t.Fatalf("notes: %+v", rel.Notes)
	}
}

// With explicit fallback groups, groups are independent of roles: here an
// upgrade guide stands in for missing release notes.
func TestIngestReleaseFallbackGroupAcrossRoles(t *testing.T) {
	if !hasExplicitFallbackGroups() {
		t.Skip("catalog.Source.FallbackGroup not available in this revision")
	}
	rel, _, _ := ingestFixture(t, "1.2.0", func(_ *world, def *catalog.ProductDefinition) {
		for k := range def.Sources {
			switch def.Sources[k].ID {
			case "site-notes":
				def.Sources[k].Locator.Path = "content/does-not-exist.md"
			case "upgrade-guide":
				def.Sources[k] = withFallbackGroup(def.Sources[k], "release-notes")
				def.Sources[k].Priority = 2
			}
		}
	})
	if s := statusOf(t, rel, "gh-notes"); s.State != domain.SourceOK {
		t.Fatalf("gh-notes: %+v", s)
	}
	if s := statusOf(t, rel, "upgrade-guide"); s.State != domain.SourceSkipped || !strings.Contains(s.Detail, "fallback group release-notes satisfied by gh-notes") {
		t.Fatalf("upgrade-guide: %+v", s)
	}
}

func TestIngestReleaseUnresolvablePreviousRelease(t *testing.T) {
	rel, _, _ := ingestFixture(t, "1.0.0", nil)
	if s := statusOf(t, rel, "upgrade-guide"); s.State != domain.SourceSkipped || !strings.Contains(s.Detail, "previous release line") {
		t.Fatalf("upgrade-guide of the first release: %+v", s)
	}
	if s := statusOf(t, rel, "commit-log"); s.State != domain.SourceSkipped || !strings.Contains(s.Detail, "previous release") {
		t.Fatalf("commit-log of the first release: %+v", s)
	}
}

func TestIngestReleaseFailuresNeverFailTheRelease(t *testing.T) {
	rel, _, _ := ingestFixture(t, "1.2.0", func(w *world, _ *catalog.ProductDefinition) {
		for k := range w.down {
			delete(w.down, k)
		}
		for _, kind := range []string{catalog.LocatorRepoFile, catalog.LocatorHTTP, catalog.LocatorOCI, catalog.LocatorHelmRepo, catalog.LocatorGitHubReleases, catalog.LocatorRepoDir} {
			w.down[kind] = true
		}
	})
	for _, s := range rel.Sources {
		switch s.State {
		case domain.SourceUnavailable, domain.SourceSkipped:
		default:
			t.Errorf("unexpected state for %s (%s): %+v", s.SourceID, s.Kind, s)
		}
	}
	for _, a := range rel.Artifacts {
		if a.Status != domain.ArtifactExpected {
			t.Errorf("%s: %s (%s)", a.ArtifactID, a.Status, a.Detail)
		}
	}
	if len(factsOf(rel, domain.FactReleasePublished)) != 1 {
		t.Fatal("release fact must survive source failures")
	}
}

type failingParser struct{ *fakeParser }

func (failingParser) ParseNotes(normalize.DocInput, []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error) {
	return nil, nil, errors.New("boom")
}

func TestIngestReleaseParseFailureIsPartial(t *testing.T) {
	w, def := newWorld(), testDef()
	ing := newTestIngester(w, failingParser{&fakeParser{}})
	vl := mustVersions(t, ing, def)
	rel, err := ing.IngestRelease(context.Background(), def, version(t, vl, "1.2.0"), vl)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceIntegrity(t, rel)
	s := statusOf(t, rel, "site-notes")
	if s.State != domain.SourcePartial || !strings.Contains(s.Detail, "parsing failed: boom") {
		t.Fatalf("site-notes: %+v", s)
	}
	// partial is not ok: the fallback is consulted.
	if gh := statusOf(t, rel, "gh-notes"); gh.State != domain.SourcePartial {
		t.Fatalf("gh-notes: %+v", gh)
	}
	if len(factsOf(rel, domain.FactDocumentRetrieved)) == 0 {
		t.Fatal("document facts must be kept on parse failure")
	}
}

func TestIngestReleaseYAMLRecords(t *testing.T) {
	rel, _, _ := ingestFixture(t, "1.1.0", func(w *world, def *catalog.ProductDefinition) {
		w.docs[websiteKey("data/supportStatus.yml")] = "- version: \"1.2\"\n  k8sVersions: 1.30, 1.31\n- version: \"1.1\"\n  k8sVersions: 1.29, 1.30\n"
		def.Sources = append(def.Sources, catalog.Source{ID: "support-status", Roles: []domain.SourceRole{domain.RoleCompatibility},
			Locator: catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: website, Ref: "main", Path: "data/supportStatus.yml"},
			Extract: &catalog.Extract{Type: catalog.ExtractYAMLRecords, KeyColumns: []string{"version"}, KeyMatch: `^{{regexQuote .Line}}$`,
				Columns: []catalog.ColumnSpec{{Platform: "kubernetes", Headers: []string{"k8sVersions"}}}}})
	})
	s := statusOf(t, rel, "support-status")
	if s.State != domain.SourceOK || !strings.Contains(s.Detail, "row L3") {
		t.Fatalf("support-status: %+v", s)
	}
	found := false
	for _, c := range rel.Compat {
		if c.SourceID == "support-status" && c.Raw == "1.29, 1.30" {
			found = true
		}
	}
	if !found {
		t.Fatalf("yaml-records constraint missing: %+v", rel.Compat)
	}
}

func TestIngestReleaseReleaseNoteYAMLDirectory(t *testing.T) {
	rel, _, _ := ingestFixture(t, "1.2.0", func(w *world, def *catalog.ProductDefinition) {
		w.dirs["repo-dir:github.com/acme/operator@v1.2.0:releasenotes/notes"] = map[string]string{
			"b.yaml": "kind: feature\nreleaseNotes: B\n", "a.yaml": "kind: bug-fix\nreleaseNotes: A\n",
		}
		def.Sources = append(def.Sources, catalog.Source{ID: "structured-notes", Roles: []domain.SourceRole{domain.RoleReleaseNotes},
			Locator: catalog.Locator{Kind: catalog.LocatorRepoDir, Repository: "github.com/acme/operator", Path: "releasenotes/notes",
				Glob: "*.yaml", BaseRef: "{{.PrevTag}}"},
			Extract: &catalog.Extract{Type: catalog.ExtractReleaseNoteYAML}})
	})
	s := statusOf(t, rel, "structured-notes")
	if s.State != domain.SourceOK || !strings.Contains(s.Detail, "2 files; 2 note items") {
		t.Fatalf("structured-notes: %+v", s)
	}
	var texts []string
	for _, n := range rel.Notes {
		if n.SourceID == "structured-notes" {
			texts = append(texts, n.Text)
		}
	}
	if strings.Join(texts, ",") != "kind: bug-fix,kind: feature" {
		t.Fatalf("files must be parsed in path order: %v", texts)
	}
}

func TestIngestReleaseDeterministic(t *testing.T) {
	encode := func(concurrency int) []byte {
		w, def := newWorld(), testDef()
		w.down[catalog.LocatorOCI] = true // exercise references too
		ing := newTestIngester(w, &fakeParser{})
		ing.Concurrency = concurrency
		vl := mustVersions(t, ing, def)
		rel, err := ing.IngestRelease(context.Background(), def, version(t, vl, "1.2.0"), vl)
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(rel)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	first := encode(1)
	for _, c := range []int{0, 2, 16} {
		for k := 0; k < 5; k++ {
			if got := encode(c); string(got) != string(first) {
				t.Fatalf("concurrency %d run %d differs:\n%s\n---\n%s", c, k, first, got)
			}
		}
	}
}

func TestIngestReleaseWithoutKnownVersions(t *testing.T) {
	w, def := newWorld(), testDef()
	ing := newTestIngester(w, &fakeParser{})
	rel, err := ing.IngestRelease(context.Background(), def, domain.Version{Semver: "1.2.0"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertEvidenceIntegrity(t, rel)
	if rel.Version.Tag != "v1.2.0" {
		t.Fatalf("tag derived from prefix: %+v", rel.Version)
	}
	if len(factsOf(rel, domain.FactReleasePublished)) != 0 {
		t.Fatal("no release fact without a ref")
	}
	// PrevLine falls back to Minor-1.
	if s := statusOf(t, rel, "upgrade-guide"); s.State != domain.SourceOK {
		t.Fatalf("upgrade-guide: %+v", s)
	}
	if _, err := ing.IngestRelease(context.Background(), def, domain.Version{}, nil); err == nil {
		t.Fatal("empty version must fail")
	}
}

// TestDefaultParserSmoke runs the pipeline with package normalize (a stub
// before integration, the real parser after). Whatever normalize returns,
// ingestion must succeed with consistent evidence and a state per status.
func TestDefaultParserSmoke(t *testing.T) {
	w, def := newWorld(), testDef()
	ing := newTestIngester(w, nil) // nil → DefaultParser
	vl := mustVersions(t, ing, def)
	for _, v := range vl.Versions {
		rel, err := ing.IngestRelease(context.Background(), def, v, vl)
		if err != nil {
			t.Fatalf("%s: %v", v, err)
		}
		assertEvidenceIntegrity(t, rel)
		for _, s := range rel.Sources {
			if s.State == "" || s.SourceID == "" {
				t.Errorf("%s: incomplete status %+v", v, s)
			}
		}
	}
	if _, err := ing.CheckRelationships(context.Background(), def, vl.Versions[:2], vl); err != nil {
		t.Fatal(err)
	}
}

func TestIngestReleaseContextCancelled(t *testing.T) {
	w, def := newWorld(), testDef()
	ing := newTestIngester(w, &fakeParser{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ing.IngestRelease(ctx, def, domain.Version{Semver: "1.2.0"}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
