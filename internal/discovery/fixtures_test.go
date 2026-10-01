package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/ingest"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// fixed clock for deterministic evidence and reports
var testNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func clock() time.Time { return testNow }

// fixture describes a fake upstream: repositories backed by testdata
// directories plus their tag lists.
type fixture struct {
	repo     string
	dirs     map[string]string
	tags     map[string][]string
	branches map[string]string
}

var (
	certFixture = fixture{
		repo: "github.com/example/certmgr",
		dirs: map[string]string{"github.com/example/certmgr": "testdata/certlike", "github.com/example/website": "testdata/certlike-website"},
		tags: map[string][]string{"github.com/example/certmgr": {"v1.0.0", "v1.0.1", "v1.1.0", "v1.1.1", "v1.1.2", "v1.2.0-alpha.0", "v1.2.0-beta.0",
			"v1.2.0", "v1.2.1", "cmd/ctl/v1.0.0", "v0.15-alpha.3"}},
		branches: map[string]string{"github.com/example/website": "master"},
	}
	argoFixture = fixture{
		repo: "github.com/example/deployer",
		dirs: map[string]string{"github.com/example/deployer": "testdata/argolike"},
		tags: map[string][]string{
			"github.com/example/deployer":      {"v2.8.0", "v2.8.4", "v2.9.0", "v2.9.5", "v3.0.0", "v3.0.3", "v3.1.0-rc1", "v3.1.0", "v3.1.2", "v3.2.0-rc1", "stable", "v2.1.2-hf1"},
			"github.com/example/deployer-helm": {"deployer-1.0.0", "deployer-1.1.0", "other-0.1.0"},
		},
	}
	istioFixture = fixture{
		repo: "github.com/meshproj/mesh",
		dirs: map[string]string{"github.com/meshproj/mesh": "testdata/istiolike", "github.com/meshproj/mesh.io": "testdata/istiolike-docs"},
		tags: map[string][]string{"github.com/meshproj/mesh": {"1.29.0", "1.29.1", "1.29.2", "1.30.0", "1.30.1", "1.30.2", "1.30.3", "1.31.0-rc.1",
			"1.31.0", "1.31.1", "1.0.0-snapshot.1", "1.15-beta.0"}},
		branches: map[string]string{"github.com/meshproj/mesh.io": "master"},
	}
)

func (f fixture) checkout() *LocalCheckout {
	return &LocalCheckout{Dirs: f.dirs, Tags: f.tags, Branches: f.branches, Clock: clock}
}

func (f fixture) tagAnalysis(t *testing.T) *TagAnalysis {
	t.Helper()
	repo, _ := ParseRepo(f.repo)
	tags, err := f.checkout().ListTags(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	ta := AnalyzeTags(repo, tags)
	ta.ListedAt = testNow
	return ta
}

// scanFixture scans one fixture repository with the given profile.
func scanFixture(t *testing.T, f fixture, repoName string, profile Profile) *ScanResult {
	t.Helper()
	main, _ := ParseRepo(f.repo)
	repo, _ := ParseRepo(repoName)
	ta := f.tagAnalysis(t)
	ref := ""
	if repo.Same(main) {
		ref = ta.Latest
	}
	tree, err := f.checkout().Checkout(context.Background(), repo, ref, CheckoutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	rv, _ := ta.Find(ref)
	res, err := (&Scanner{}).Scan(context.Background(), tree, ScanInput{Profile: profile, Product: ProductHints{Owner: main.Owner, Name: main.Name, ID: main.Name},
		Main: main, Tags: ta, RefVersion: rv})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func runFixture(t *testing.T, f fixture, d *Discoverer, req Request) *Result {
	t.Helper()
	lc := f.checkout()
	if d == nil {
		d = &Discoverer{}
	}
	d.Checkout, d.Tags, d.Clock = lc, lc, clock
	req.Repository = f.repo
	res, err := d.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if rep := catalog.Validate(res.Definition); !rep.OK() {
		t.Fatalf("definition does not pass catalog.Validate: %v\n%s", rep.Errors(), res.YAML)
	}
	// the YAML must round-trip through the strict loader
	if _, err := catalog.Parse(res.YAML); err != nil {
		t.Fatalf("YAML does not parse: %v\n%s", err, res.YAML)
	}
	return res
}

func findCand(cs []Candidate, kind CandidateKind, value string) (Candidate, bool) {
	for _, c := range cs {
		if c.Kind == kind && c.Value == value {
			return c, true
		}
	}
	return Candidate{}, false
}

func mustCand(t *testing.T, cs []Candidate, kind CandidateKind, value string) Candidate {
	t.Helper()
	c, ok := findCand(cs, kind, value)
	if !ok {
		var have []string
		for _, x := range cs {
			if x.Kind == kind {
				have = append(have, x.Value)
			}
		}
		t.Fatalf("missing candidate %s %q; have %q", kind, value, have)
	}
	return c
}

func source(t *testing.T, def *catalog.ProductDefinition, id string) catalog.Source {
	t.Helper()
	s, ok := def.Source(id)
	if !ok {
		var ids []string
		for _, x := range def.Sources {
			ids = append(ids, x.ID)
		}
		t.Fatalf("missing source %q; have %v", id, ids)
	}
	return s
}

func artifact(t *testing.T, def *catalog.ProductDefinition, id string) catalog.Artifact {
	t.Helper()
	a, ok := def.Artifact(id)
	if !ok {
		var ids []string
		for _, x := range def.Artifacts {
			ids = append(ids, x.ID)
		}
		t.Fatalf("missing artifact %q; have %v", id, ids)
	}
	return a
}

// fakeChecker implements RelationshipChecker with a scripted outcome per
// (subject, release).
type fakeChecker struct {
	outcome func(subjectKind, subject, release string) (string, string)
	calls   int
	defs    []*catalog.ProductDefinition
}

func (f *fakeChecker) CheckRelationships(ctx context.Context, def *catalog.ProductDefinition, releases []domain.Version, known *ingest.VersionList) (*ingest.RelationshipReport, error) {
	f.calls++
	f.defs = append(f.defs, def)
	rep := &ingest.RelationshipReport{Product: domain.ProductID(def.ID)}
	for _, r := range releases {
		rep.Releases = append(rep.Releases, r.Tag)
	}
	subjects := [][2]string{}
	for _, s := range def.Sources {
		subjects = append(subjects, [2]string{"source", s.ID})
	}
	for _, a := range def.Artifacts {
		subjects = append(subjects, [2]string{"artifact", a.ID})
	}
	for _, s := range subjects {
		for _, r := range releases {
			out, detail := ingest.OutcomePass, ""
			if f.outcome != nil {
				out, detail = f.outcome(s[0], s[1], r.Tag)
			}
			rep.Checks = append(rep.Checks, ingest.RelationshipCheck{Subject: s[1], SubjectKind: s[0], Release: r.Tag, Outcome: out, Detail: detail})
		}
	}
	return rep, nil
}

var _ RelationshipChecker = (*ingest.Ingester)(nil)
var _ llm.Client = (*llm.Fake)(nil)
