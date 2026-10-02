package semvalidate

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/normalize"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

var now = time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)

// rel builds a Release with evidence for everything added.
type rel struct{ r *domain.Release }

func newRel(tag string) *rel {
	v := domain.MustVersion(tag, strings.TrimPrefix(tag, "v"))
	return &rel{r: &domain.Release{Product: "demo", Version: v, IngestedAt: now}}
}

func (b *rel) evidence(source, uri, excerpt string) domain.EvidenceID {
	e := domain.NewEvidence(domain.EvidenceStructured, source, uri, "", excerpt, domain.Digest([]byte(uri)), now)
	b.r.Evidence = append(b.r.Evidence, e)
	return e.ID
}

func (b *rel) snapshot(s domain.Snapshot) *rel {
	s.Evidence = []domain.EvidenceID{b.evidence(s.ArtifactID, "https://x/"+b.r.Version.Tag+"/"+s.ArtifactID, string(s.Kind))}
	b.r.Snapshots = append(b.r.Snapshots, s)
	return b
}

func (b *rel) values(artifact, chart string, kv ...string) *rel {
	e := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		e[kv[i]] = kv[i+1]
	}
	return b.snapshot(domain.Snapshot{ArtifactID: artifact, Kind: domain.SnapshotHelmValues,
		Values: &domain.ValuesSnapshot{Chart: chart, Version: b.r.Version.Tag, Entries: e}})
}

func (b *rel) crds(artifact string, crds ...domain.CRDSummary) *rel {
	return b.snapshot(domain.Snapshot{ArtifactID: artifact, Kind: domain.SnapshotCRDs, CRDs: &domain.CRDSnapshot{CRDs: crds}})
}

func (b *rel) images(artifact string, refs ...string) *rel {
	var out []domain.ImageRef
	for _, s := range refs {
		repo, tag, _ := strings.Cut(s, ":")
		out = append(out, domain.ImageRef{Repository: repo, Tag: tag})
	}
	return b.snapshot(domain.Snapshot{ArtifactID: artifact, Kind: domain.SnapshotImages, Images: &domain.ImageRefsSnapshot{Images: out}})
}

// compat adds a compatibility row parsed like the ingest does.
func (b *rel) compat(platform, kind, raw string) *rel {
	c, vs, err := normalize.ParseVersionRange(raw)
	if err != nil {
		panic(err)
	}
	id := b.evidence("compat", "https://x/"+b.r.Version.Tag+"/compat", platform+" "+kind+" "+raw)
	b.r.Compat = append(b.r.Compat, domain.CompatibilityConstraint{
		Platform: platform, Kind: kind, Constraint: c, Versions: vs, Raw: raw, SourceID: "compat",
		Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize.table@v1", Confidence: domain.ConfidenceHigh},
		Evidence:   []domain.EvidenceID{id},
	})
	return b
}

func crd(name, group, kind string, versions ...domain.CRDVersionInfo) domain.CRDSummary {
	return domain.CRDSummary{Name: name, Group: group, Kind: kind, Scope: "Namespaced", Versions: versions}
}

// crdVer builds a version whose Fields come from the given schema entries.
func crdVer(name string, served, storage bool, fields ...domain.CRDFieldSchema) domain.CRDVersionInfo {
	v := domain.CRDVersionInfo{Name: name, Served: served, Storage: storage, Fields: fields}
	for _, f := range fields {
		v.SchemaPaths = append(v.SchemaPaths, f.Path)
	}
	return v
}

func fld(path, typ, def string) domain.CRDFieldSchema {
	return domain.CRDFieldSchema{Path: path, Type: typ, Default: def}
}

func str(s string) *string { return &s }

func candidate() domain.SemanticCandidate {
	return domain.SemanticCandidate{ID: domain.CandidateIDPrefix + "test"}
}

func input(from, to *rel, a domain.SemanticAssertion) knowledge.ValidationInput {
	return knowledge.ValidationInput{Candidate: candidate(), ProposalID: domain.ProposalIDPrefix + "p1", Assertion: a, From: from.r, To: to.r, Now: now}
}

// run validates with one validator and checks every result is well-formed.
func run(t *testing.T, v knowledge.Validator, in knowledge.ValidationInput) map[domain.Aspect]domain.ValidationOutcome {
	t.Helper()
	rs, err := v.Validate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	out := map[domain.Aspect]domain.ValidationOutcome{}
	for _, r := range rs {
		if err := r.Validate(); err != nil {
			t.Fatalf("%s produced an invalid result: %v", v.Name(), err)
		}
		if r.Confirms() {
			// a confirmation must cite the artifacts of BOTH releases when it
			// compares two sides
			if len(r.Evidence) == 0 {
				t.Fatalf("confirmation without evidence")
			}
		}
		for _, c := range r.Checks {
			out[c.Aspect] = c.Outcome
		}
	}
	return out
}

func helmSubject(path string) *domain.Subject {
	return &domain.Subject{Family: domain.SubjectHelmValue, Product: "demo", Path: path}
}

func crdSubject(group, kind, path string) *domain.Subject {
	return &domain.Subject{Family: domain.SubjectCRDField, Product: "demo", Group: group, Kind: kind, Path: path}
}

func change(kind domain.ChangeKind, before, after string) *domain.ChangeSpec {
	c := &domain.ChangeSpec{Type: kind}
	if before != "" {
		c.Before = str(before)
	}
	if after != "" {
		c.After = str(after)
	}
	return c
}

func assertion(s *domain.Subject, c *domain.ChangeSpec) domain.SemanticAssertion {
	return domain.SemanticAssertion{Subject: s, Change: c}
}

// build an edge through the real differ, so the restatement parsing is tested
// against the shapes internal/upgrade actually renders.
func edgeOf(t *testing.T, from, to *rel) *domain.UpgradeEdge {
	t.Helper()
	e, err := upgrade.Build(upgrade.Input{
		From: from.r, To: to.r, Path: []*domain.Release{to.r},
		Selection: &upgrade.PathSelection{Policy: upgrade.PolicyAll, Path: []domain.Version{to.r.Version}, Skipped: []domain.Version{}},
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// withRequired marks a field schema required.
func withRequired(f domain.CRDFieldSchema) domain.CRDFieldSchema { f.Required = true; return f }
