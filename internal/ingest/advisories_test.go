package ingest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// stubAdvisories serves one fixed answer per locator kind.
type stubAdvisories struct {
	advs []domain.Advisory
	evs  []domain.Evidence
	err  error
}

func (s stubAdvisories) ListAdvisories(context.Context, catalog.Locator) ([]domain.Advisory, []domain.Evidence, error) {
	return s.advs, s.evs, s.err
}

func twoSourceDef() *catalog.ProductDefinition {
	sec := []domain.SourceRole{domain.RoleSecurity}
	return &catalog.ProductDefinition{ID: "acme", Sources: []catalog.Source{
		{ID: "repo-advisories", Roles: sec, Locator: catalog.Locator{Kind: catalog.LocatorGitHubAdvisories, Repository: "acme/acme"}},
		{ID: "global-advisories", Roles: sec, Locator: catalog.Locator{Kind: catalog.LocatorGitHubGlobalAdvisories, Ecosystem: "go", Package: "example.com/acme"}},
	}}
}

func advEv(uri, id string) domain.Evidence {
	return domain.NewEvidence(domain.EvidenceAdvisory, "", uri, id, id, "sha256:"+strings.Repeat("0", 64), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}

func ingesterWith(repo, global stubAdvisories) *Ingester {
	reg := sources.NewRegistry()
	reg.RegisterAdvisorySource(catalog.LocatorGitHubAdvisories, repo)
	reg.RegisterAdvisorySource(catalog.LocatorGitHubGlobalAdvisories, global)
	return New(reg)
}

// Copies of one id disagree: the machine-readable copy decides the range,
// whichever source listed it, and both copies stay cited.
func TestAdvisoriesMergePrefersMachineReadable(t *testing.T) {
	t1, t2 := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)
	repoA, repoB, repoC := advEv("https://github.com/acme/acme/security/advisories/GHSA-a", "GHSA-a"),
		advEv("https://github.com/acme/acme/security/advisories/GHSA-b", "GHSA-b"),
		advEv("https://github.com/acme/acme/security/advisories/GHSA-c", "GHSA-c")
	globA, globB, globC := advEv("https://github.com/advisories/GHSA-a", "GHSA-a"),
		advEv("https://github.com/advisories/GHSA-b", "GHSA-b"),
		advEv("https://github.com/advisories/GHSA-c", "GHSA-c")
	repo := stubAdvisories{
		advs: []domain.Advisory{
			// not machine-readable in the repository copy
			{ID: "GHSA-a", Severity: "medium", Aliases: []string{"CVE-1"}, Evidence: []domain.EvidenceID{repoA.ID}, PublishedAt: &t1},
			// both readable, different
			{ID: "GHSA-b", Vulnerable: "= 1.2.0 || = 1.2.1", Patched: []string{"1.2.2"}, Evidence: []domain.EvidenceID{repoB.ID}},
			// only the repository copy is readable
			{ID: "GHSA-c", Vulnerable: "< 2.0.0", Evidence: []domain.EvidenceID{repoC.ID}},
		},
		evs: []domain.Evidence{repoA, repoB, repoC},
	}
	global := stubAdvisories{
		advs: []domain.Advisory{
			{ID: "GHSA-a", Severity: "high", Vulnerable: ">= 1.0.0, < 1.3.0", Patched: []string{"1.3.0"}, Aliases: []string{"CVE-1", "CVE-2"},
				Evidence: []domain.EvidenceID{globA.ID}, UpdatedAt: &t2},
			{ID: "GHSA-b", Vulnerable: ">= 1.2.0, < 1.2.3", Patched: []string{"1.2.3"}, Evidence: []domain.EvidenceID{globB.ID}},
			{ID: "GHSA-c", Evidence: []domain.EvidenceID{globC.ID}},
			{ID: "GHSA-d", Vulnerable: "< 0.5.0", Evidence: []domain.EvidenceID{advEv("https://github.com/advisories/GHSA-d", "GHSA-d").ID}},
		},
		evs: []domain.Evidence{globA, globB, globC, advEv("https://github.com/advisories/GHSA-d", "GHSA-d")},
	}
	advs, evs, sts, err := ingesterWith(repo, global).Advisories(context.Background(), twoSourceDef())
	if err != nil {
		t.Fatal(err)
	}
	if len(advs) != 4 || len(evs) != 7 {
		t.Fatalf("%d advisories, %d evidence", len(advs), len(evs))
	}
	if sts[0].State != domain.SourceOK || sts[1].State != domain.SourceOK || !strings.Contains(sts[1].Detail, "3 already listed") {
		t.Errorf("statuses %+v", sts)
	}
	a, b, c, d := advs[0], advs[1], advs[2], advs[3]
	if a.Vulnerable != ">= 1.0.0, < 1.3.0" || a.SourceID != "global-advisories" || a.Severity != "high" ||
		strings.Join(a.Aliases, ",") != "CVE-1,CVE-2" || strings.Join(a.Patched, ",") != "1.3.0" ||
		a.PublishedAt == nil || a.UpdatedAt == nil || !a.UpdatedAt.Equal(t2) {
		t.Errorf("a = %+v", a)
	}
	if strings.Join(a.Sources, ",") != "repo-advisories,global-advisories" || len(a.Evidence) != 2 || a.Evidence[0] != repoA.ID || a.Evidence[1] != globA.ID {
		t.Errorf("a keeps both copies: sources %v evidence %v", a.Sources, a.Evidence)
	}
	if b.Vulnerable != "= 1.2.0 || = 1.2.1 || >= 1.2.0, < 1.2.3" || b.SourceID != "repo-advisories" || strings.Join(b.Patched, ",") != "1.2.2,1.2.3" {
		t.Errorf("b (union) = %+v", b)
	}
	if c.Vulnerable != "< 2.0.0" || c.SourceID != "repo-advisories" || len(c.Evidence) != 2 {
		t.Errorf("c (readable copy first) = %+v", c)
	}
	if d.SourceID != "global-advisories" || strings.Join(d.Sources, ",") != "global-advisories" {
		t.Errorf("d = %+v", d)
	}
}

// A source that cannot be read is a status of that state, never an empty
// answer; the readable source still contributes.
func TestAdvisoriesUnreadableSource(t *testing.T) {
	ev := advEv("https://github.com/advisories/GHSA-x", "GHSA-x")
	ok := stubAdvisories{advs: []domain.Advisory{{ID: "GHSA-x", Vulnerable: "< 1.0.0", Evidence: []domain.EvidenceID{ev.ID}}}, evs: []domain.Evidence{ev}}
	for name, err := range map[string]error{
		"offline cache miss": &fetch.Error{URL: "https://api.github.com/advisories", Err: fetch.ErrOffline},
		"throttled":          &fetch.Error{URL: "https://api.github.com/advisories", Err: fetch.ErrThrottled},
	} {
		advs, _, sts, e := ingesterWith(stubAdvisories{advs: []domain.Advisory{}}, stubAdvisories{err: err}).Advisories(context.Background(), twoSourceDef())
		if e != nil || len(advs) != 0 || sts[0].State != domain.SourceOK || sts[1].State == domain.SourceOK {
			t.Errorf("%s: %v %+v %+v", name, e, advs, sts)
		}
		advs, _, sts, _ = ingesterWith(stubAdvisories{err: err}, ok).Advisories(context.Background(), twoSourceDef())
		if len(advs) != 1 || sts[0].State == domain.SourceOK || sts[1].State != domain.SourceOK {
			t.Errorf("%s (repository down): %+v %+v", name, advs, sts)
		}
	}
}

// The chart version shipping a candidate release is looked up through the
// definition's own artifact relation (appVersion in the chart index), with
// the index entry as evidence; a release no chart ships is a gap.
func TestChartVersions(t *testing.T) {
	w, def := newWorld(), testDef()
	ing := newTestIngester(w, &fakeParser{})
	ctx := context.Background()
	vl, err := ing.ListVersions(ctx, def)
	if err != nil {
		t.Fatal(err)
	}
	charts, gaps, err := ing.ChartVersions(ctx, def, domain.MustVersion("v1.1.1", "1.1.1"), vl)
	if err != nil || len(gaps) != 0 || len(charts) != 1 {
		t.Fatalf("1.1.1: %+v %v %v", charts, gaps, err)
	}
	if c := charts[0]; c.Artifact != "chart" || c.Name != "acme" || c.Version != "0.4.1" || len(c.Evidence) != 1 || !strings.Contains(c.Evidence[0].Excerpt, "appVersion: v1.1.1") {
		t.Errorf("chart %+v", c)
	}
	// Two charts ship 1.2.0; the definition selects one, as ingestion does.
	charts, _, _ = ing.ChartVersions(ctx, def, domain.MustVersion("v1.2.0", "1.2.0"), vl)
	if len(charts) != 1 || charts[0].Version != "0.5.1" {
		t.Errorf("1.2.0: %+v", charts)
	}
	charts, gaps, err = ing.ChartVersions(ctx, def, domain.MustVersion("v1.2.1", "1.2.1"), vl)
	if err != nil || len(charts) != 0 || len(gaps) != 1 || !strings.Contains(gaps[0], "chart chart for v1.2.1") {
		t.Errorf("1.2.1 (no chart): %+v %v %v", charts, gaps, err)
	}
	// An unreadable index is a gap too.
	w.down[catalog.LocatorHelmRepo] = true
	ing = newTestIngester(w, &fakeParser{})
	charts, gaps, _ = ing.ChartVersions(ctx, def, domain.MustVersion("v1.1.1", "1.1.1"), vl)
	if len(charts) != 0 || len(gaps) != 1 {
		t.Errorf("index down: %+v %v", charts, gaps)
	}
}
