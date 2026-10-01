package store

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/ingest"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

var at = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func sampleRelease() *domain.Release {
	ev := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example.test/notes.md", "L3-L9", "## v1.18.0 → <b>", "sha256:abc", at)
	pub := at.Add(-24 * time.Hour)
	return &domain.Release{
		Product:     "cert-manager",
		Version:     domain.MustVersion("v1.18.0", "1.18.0"),
		PublishedAt: &pub,
		Artifacts: []domain.ArtifactInstance{{ArtifactID: "controller", Type: domain.ArtifactContainerImage, Name: "controller",
			Coordinate: "quay.io/jetstack/cert-manager-controller:v1.18.0", Version: "v1.18.0", Status: domain.ArtifactReferenced,
			Channel: "reference:manifest", Evidence: []domain.EvidenceID{ev.ID}}},
		Notes: []domain.NoteItem{{ID: "n1", Release: "1.18.0", SourceID: "notes", Role: domain.RoleReleaseNotes, Text: "Added X",
			Category: domain.CategoryFeature, Classification: domain.Provenance{Method: domain.MethodDeclared, Producer: "p", Confidence: domain.ConfidenceHigh},
			Evidence: []domain.EvidenceID{ev.ID}}},
		Snapshots: []domain.Snapshot{{ArtifactID: "chart", Kind: domain.SnapshotHelmValues,
			Values: &domain.ValuesSnapshot{Chart: "cert-manager", Version: "v1.18.0", Entries: map[string]string{"b": "1", "a": `"x"`}}, Evidence: []domain.EvidenceID{ev.ID}}},
		Facts:            []domain.Fact{domain.NewFact(domain.FactDocumentRetrieved, "cert-manager@1.18.0", "1.18.0", "retrieved", "ingest@v1", map[string]string{"z": "1", "a": "2"}, ev.ID)},
		Sources:          []domain.SourceStatus{{SourceID: "notes", Kind: "repo-file", Version: "1.18.0", State: domain.SourceOK}},
		Evidence:         []domain.Evidence{ev},
		IngestedAt:       at,
		DefinitionDigest: "sha256:def1",
	}
}

func TestReleaseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	rel := sampleRelease()
	if err := s.SaveRelease(rel); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "cert-manager", "releases", "1.18.0.json")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
	if !bytes.Contains(first, []byte("\n  \"product\": \"cert-manager\"")) || !bytes.Contains(first, []byte("→ <b>")) || !bytes.HasSuffix(first, []byte("}\n")) {
		t.Fatalf("unexpected encoding:\n%s", first)
	}

	got, err := s.LoadRelease("cert-manager", "1.18.0")
	if err != nil || got == nil {
		t.Fatalf("load: %v %v", got, err)
	}
	want, _ := json.Marshal(rel)
	have, _ := json.Marshal(got)
	if !bytes.Equal(want, have) {
		t.Fatalf("round trip differs:\n%s\n%s", want, have)
	}
	if got.Version.Tag != "v1.18.0" || !got.Version.Less(domain.MustVersion("v1.19.0", "1.19.0")) {
		t.Fatalf("version after load: %+v", got.Version)
	}

	// Saving the loaded release again is byte-identical (stable output).
	if err := s.SaveRelease(got); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if !bytes.Equal(first, second) {
		t.Fatal("re-saving changed the file")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}

	if r, err := s.LoadRelease("cert-manager", "1.19.0"); r != nil || err != nil {
		t.Fatalf("absent release: %v %v", r, err)
	}
	if r, err := s.LoadRelease("nope", "1.0.0"); r != nil || err != nil {
		t.Fatalf("absent product: %v %v", r, err)
	}

	cur, err := s.LoadCurrentRelease("cert-manager", "1.18.0", "sha256:def1")
	if err != nil || cur == nil {
		t.Fatalf("current release: %v %v", cur, err)
	}
	if stale, err := s.LoadCurrentRelease("cert-manager", "1.18.0", "sha256:def2"); stale != nil || err != nil {
		t.Fatalf("stale release must not be reused: %v %v", stale, err)
	}

	if err := s.SaveRelease(&domain.Release{Product: "cert-manager", Version: domain.MustVersion("v1.2.0", "1.2.0")}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListReleases("cert-manager")
	if err != nil || !reflect.DeepEqual(list, []string{"1.2.0", "1.18.0"}) {
		t.Fatalf("list: %v %v", list, err)
	}
}

func TestInvalidPathComponents(t *testing.T) {
	s := New(t.TempDir())
	for _, c := range [][2]string{{"../evil", "1.0.0"}, {"ok", "../../x"}, {"ok", "a/b"}, {"", "1.0.0"}, {"ok", "1..2"}} {
		if _, err := s.LoadRelease(c[0], c[1]); err == nil {
			t.Errorf("LoadRelease(%q, %q) must fail", c[0], c[1])
		}
	}
	if err := s.SaveRelease(&domain.Release{Product: "../evil", Version: domain.MustVersion("v1.0.0", "1.0.0")}); err == nil {
		t.Error("SaveRelease must reject traversal")
	}
}

func TestCorruptFile(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	path, _ := s.ReleasePath("p", "1.0.0")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadRelease("p", "1.0.0"); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("err = %v", err)
	}
}

func TestEdgeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	e := &domain.UpgradeEdge{
		SchemaVersion: domain.UpgradeEdgeSchemaVersion,
		Product:       domain.ProductRef{ID: "istio", Name: "Istio"},
		From:          domain.MustVersion("1.30.0", "1.30.0"),
		To:            domain.MustVersion("1.31.1", "1.31.1"),
		PathPolicy:    "minor-lineage",
		Path:          []domain.PathStep{{Version: domain.MustVersion("1.31.0", "1.31.0"), Reason: "minor-release"}},
		GeneratedAt:   at, DefinitionDigest: "sha256:d",
	}
	if err := s.SaveEdge(e); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "istio", "edges", "1.30.0_1.31.1.json")); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadEdge("istio", "1.30.0", "1.31.1")
	if err != nil || got == nil {
		t.Fatalf("load edge: %v %v", got, err)
	}
	a, _ := json.Marshal(e)
	b, _ := json.Marshal(got)
	if !bytes.Equal(a, b) {
		t.Fatalf("edge round trip:\n%s\n%s", a, b)
	}
	if missing, err := s.LoadEdge("istio", "1.29.0", "1.31.1"); missing != nil || err != nil {
		t.Fatalf("absent edge: %v %v", missing, err)
	}
	if cur, _ := s.LoadCurrentEdge("istio", "1.30.0", "1.31.1", "sha256:d"); cur == nil {
		t.Fatal("current edge not returned")
	}
	if stale, _ := s.LoadCurrentEdge("istio", "1.30.0", "1.31.1", "sha256:other"); stale != nil {
		t.Fatal("stale edge returned")
	}
}

func TestVersionListRoundTrip(t *testing.T) {
	s := New(t.TempDir())
	ev := domain.NewEvidence(domain.EvidenceGitRef, "tags", "https://github.com/x/y", "tag v1.0.0", "v1.0.0", "", at)
	vl := &ingest.VersionList{
		Product:  "x",
		Source:   "tags",
		Versions: []domain.Version{domain.MustVersion("v1.0.0", "1.0.0"), domain.MustVersion("v1.1.0", "1.1.0")},
		Refs:     map[string]sources.ReleaseRef{"1.0.0": {Tag: "v1.0.0", Evidence: ev}, "1.1.0": {Tag: "v1.1.0"}},
		Sources:  []domain.SourceStatus{{SourceID: "tags", Kind: "git-tags", State: domain.SourceOK}},
		Evidence: []domain.Evidence{ev},
	}
	if err := s.SaveVersionList(vl); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadVersionList("x")
	if err != nil || got == nil {
		t.Fatalf("load: %v %v", got, err)
	}
	a, _ := json.Marshal(vl)
	b, _ := json.Marshal(got)
	if !bytes.Equal(a, b) {
		t.Fatalf("version list round trip:\n%s\n%s", a, b)
	}
	if !got.Versions[0].Less(got.Versions[1]) {
		t.Fatal("versions must compare after load")
	}
	if none, err := s.LoadVersionList("y"); none != nil || err != nil {
		t.Fatalf("absent list: %v %v", none, err)
	}
}

func TestDefaultDir(t *testing.T) {
	if New("").Dir() != DefaultDir {
		t.Fatal("default dir")
	}
}

// countingFetcher counts fetches so the test can tell an ingestion from a reuse.
type countingFetcher struct{ docs int }

func (c *countingFetcher) FetchDocument(_ context.Context, loc catalog.Locator) (*sources.Document, error) {
	c.docs++
	return &sources.Document{URI: "https://example.test/notes.md", Content: []byte("# Notes\n")}, nil
}

func TestIngestOrLoad(t *testing.T) {
	def := &catalog.ProductDefinition{
		APIVersion: catalog.APIVersion, Kind: catalog.Kind, ID: "demo", Name: "Demo",
		Versioning: catalog.Versioning{Scheme: domain.SchemeSemver, TagPrefix: "v"},
		Sources: []catalog.Source{
			{ID: "tags", Roles: []domain.SourceRole{domain.RoleVersions}, Locator: catalog.Locator{Kind: catalog.LocatorGitTags, Repository: "github.com/x/demo"}},
			{ID: "compat-page", Roles: []domain.SourceRole{domain.RoleCompatibility}, Locator: catalog.Locator{Kind: catalog.LocatorHTTP, URL: "https://example.test/notes.md"}},
		},
	}
	fetcher := &countingFetcher{}
	reg := sources.NewRegistry()
	reg.RegisterDocumentFetcher(catalog.LocatorHTTP, fetcher)
	ing := ingest.New(reg)
	ing.Clock = func() time.Time { return at }
	s := New(t.TempDir())
	v := domain.MustVersion("v1.0.0", "1.0.0")

	rel, reused, err := s.IngestOrLoad(context.Background(), ing, def, v, nil)
	if err != nil || reused || rel == nil || fetcher.docs != 1 {
		t.Fatalf("first run: rel=%v reused=%v err=%v fetches=%d", rel != nil, reused, err, fetcher.docs)
	}
	if rel.DefinitionDigest != ingest.DefinitionDigest(def) || rel.DefinitionDigest == "" {
		t.Fatalf("definition digest: %q", rel.DefinitionDigest)
	}
	again, reused, err := s.IngestOrLoad(context.Background(), ing, def, v, nil)
	if err != nil || !reused || fetcher.docs != 1 {
		t.Fatalf("second run must reuse: reused=%v err=%v fetches=%d", reused, err, fetcher.docs)
	}
	a, _ := json.Marshal(rel)
	b, _ := json.Marshal(again)
	if !bytes.Equal(a, b) {
		t.Fatal("reused release differs")
	}
	def.Name = "Demo (renamed)" // a new definition revision invalidates the stored release
	if _, reused, err := s.IngestOrLoad(context.Background(), ing, def, v, nil); err != nil || reused || fetcher.docs != 2 {
		t.Fatalf("changed definition must re-ingest: reused=%v err=%v fetches=%d", reused, err, fetcher.docs)
	}
}
