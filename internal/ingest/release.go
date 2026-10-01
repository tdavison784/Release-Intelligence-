package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// run holds the per-release context shared by all steps of one ingestion.
type run struct {
	def     *catalog.ProductDefinition
	v       domain.Version
	rc      catalog.RenderContext
	known   *VersionList
	subject string // "<product>@<semver>", subject of release-level facts
	now     time.Time
	memo    *memo
	// exhaustive consults every member of a fallback group (relationship
	// checks); only the members up to the first ok contribute content.
	exhaustive bool
	// repo is the product repository (from the versions sources), used to
	// resolve bare "#1234" references when a source has no repository.
	repo string
}

// ingestion is the full result of one release ingestion.
type ingestion struct {
	release *domain.Release
	checks  []RelationshipCheck
	// checkEvidence is evidence referenced only by checks (sources consulted
	// in exhaustive mode after their fallback group was satisfied).
	checkEvidence []domain.Evidence
}

func (i *Ingester) ingestRelease(ctx context.Context, def *catalog.ProductDefinition, v domain.Version, known *VersionList) (*domain.Release, error) {
	res, err := i.ingest(ctx, def, v, known, false)
	if err != nil {
		return nil, err
	}
	return res.release, nil
}

func (i *Ingester) ingest(ctx context.Context, def *catalog.ProductDefinition, v domain.Version, known *VersionList, exhaustive bool) (*ingestion, error) {
	if def == nil {
		return nil, errors.New("ingest: nil product definition")
	}
	if i.Registry == nil {
		return nil, errors.New("ingest: no source registry")
	}
	v, err := canonicalVersion(def, v, known)
	if err != nil {
		return nil, err
	}
	var knownVersions []domain.Version
	if known != nil {
		knownVersions = known.Versions
	}
	r := &run{
		def:        def,
		v:          v,
		rc:         catalog.NewRenderContext(def.ID, v, knownVersions),
		known:      known,
		subject:    fmt.Sprintf("%s@%s", def.ID, v.Semver),
		now:        i.now(),
		memo:       newMemo(),
		exhaustive: exhaustive,
		repo:       productRepository(def),
	}

	groups := groupSources(perReleaseSources(def), fallbackGroup)
	referenced := map[string]bool{}
	for _, a := range def.Artifacts {
		for _, ref := range a.References {
			referenced[ref.Artifact] = true
		}
	}
	srcRes := make([]groupResult, len(groups))
	artRes := make([]*artifactRun, len(def.Artifacts))
	i.parallel(len(groups)+len(def.Artifacts), func(k int) {
		if k < len(groups) {
			srcRes[k] = i.runGroup(ctx, r, groups[k])
			return
		}
		a := def.Artifacts[k-len(groups)]
		artRes[k-len(groups)] = i.runArtifact(ctx, r, a, referenced[a.ID])
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolveReferences(r, artRes)
	return r.assemble(srcRes, artRes, DefinitionDigest(def)), nil
}

// canonicalVersion fills in the published tag (from the known list, else the
// definition's tag prefix) and checks the semantic version.
func canonicalVersion(def *catalog.ProductDefinition, v domain.Version, known *VersionList) (domain.Version, error) {
	if v.Semver == "" && v.Tag != "" {
		p, err := def.VersionParser()
		if err != nil {
			return v, fmt.Errorf("ingest: %s versioning: %w", def.ID, err)
		}
		pv, err := p.Parse(v.Tag)
		if err != nil {
			return v, fmt.Errorf("ingest: %w", err)
		}
		v = pv
	}
	if v.Semver == "" {
		return v, errors.New("ingest: empty release version")
	}
	if _, err := semver.NewVersion(v.Semver); err != nil {
		return v, fmt.Errorf("ingest: release version %q: %w", v.Semver, err)
	}
	if v.Tag == "" && known != nil {
		for _, k := range known.Versions {
			if k.Semver == v.Semver {
				v.Tag = k.Tag
				break
			}
		}
	}
	if v.Tag == "" {
		v.Tag = def.TagFor(v.Semver)
	}
	return v, nil
}

// perReleaseSources are the sources consulted for each release: all but
// those whose roles are only versions and/or security.
func perReleaseSources(def *catalog.ProductDefinition) []catalog.Source {
	var out []catalog.Source
	for _, s := range def.Sources {
		if _, ok := primaryRole(s); ok {
			out = append(out, s)
		}
	}
	return out
}

// productRepository is the repository of the first versions source.
func productRepository(def *catalog.ProductDefinition) string {
	for _, s := range def.SourcesWithRole(domain.RoleVersions) {
		loc, err := renderLocator(s.Locator, catalog.RenderContext{Product: def.ID})
		if err != nil {
			continue
		}
		if repo := repositoryOf(loc); repo != "" {
			return repo
		}
	}
	return ""
}

// assemble builds the Release in a deterministic order: release fact, then
// sources (in group order), then artifacts (in definition order).
func (r *run) assemble(srcRes []groupResult, artRes []*artifactRun, digest string) *ingestion {
	rel := &domain.Release{
		Product:          domain.ProductID(r.def.ID),
		Version:          r.v,
		IngestedAt:       r.now,
		DefinitionDigest: digest,
	}
	out := &ingestion{release: rel}
	var evs domain.EvidenceSet
	add := func(list []domain.Evidence) {
		for _, e := range list {
			if e.ID != "" {
				evs.Add(e)
			}
		}
	}

	if r.known != nil {
		if ref, ok := r.known.Refs[r.v.Semver]; ok {
			rel.PublishedAt = ref.PublishedAt
			if ref.Evidence.ID != "" {
				add([]domain.Evidence{ref.Evidence})
				published := ""
				if ref.PublishedAt != nil {
					published = ref.PublishedAt.UTC().Format(time.RFC3339)
				}
				rel.Facts = append(rel.Facts, domain.NewFact(domain.FactReleasePublished, r.subject, r.v.Semver,
					fmt.Sprintf("%s %s is published as tag %s", r.def.Name, r.v.Semver, ref.Tag), Producer,
					attrs("tag", ref.Tag, "source", r.known.Source, "commit", ref.Commit, "url", ref.URL, "publishedAt", published),
					ref.Evidence.ID))
			}
		}
	}

	for _, g := range srcRes {
		rel.Sources = append(rel.Sources, g.statuses...)
		rel.Notes = append(rel.Notes, g.notes...)
		rel.Compat = append(rel.Compat, g.compat...)
		rel.Facts = append(rel.Facts, g.facts...)
		add(g.evidence)
		out.checks = append(out.checks, g.checks...)
		out.checkEvidence = append(out.checkEvidence, g.checkEvidence...)
	}

	for _, ar := range artRes {
		for _, ir := range ar.instances {
			add(ir.evidence)
			inst := ir.inst
			inst.Evidence = evidenceIDs(ir.evidence)
			rel.Artifacts = append(rel.Artifacts, inst)
			if f, ok := artifactFact(r, inst); ok {
				rel.Facts = append(rel.Facts, f)
			}
			out.checks = append(out.checks, artifactCheck(r, ar.art, inst))
		}
		rel.Sources = append(rel.Sources, ar.statuses...)
		rel.Snapshots = append(rel.Snapshots, ar.snapshots...)
		rel.Compat = append(rel.Compat, ar.compat...)
		rel.Facts = append(rel.Facts, ar.facts...)
		add(ar.evidence)
		out.checks = append(out.checks, ar.checks...)
	}
	rel.Evidence = evs.List()
	return out
}

// attrs builds a fact attribute map from key/value pairs, dropping empty values.
func attrs(kv ...string) map[string]string {
	m := map[string]string{}
	for k := 0; k+1 < len(kv); k += 2 {
		if kv[k+1] != "" {
			m[kv[k]] = kv[k+1]
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// compatFacts records one compatibility fact per constraint.
func compatFacts(subject, release string, cs []domain.CompatibilityConstraint) []domain.Fact {
	var out []domain.Fact
	for _, c := range cs {
		kind := c.Kind
		if kind == "" {
			kind = "supported"
		}
		out = append(out, domain.NewFact(domain.FactCompatibility, subject, release,
			fmt.Sprintf("%s %s: %s", c.Platform, kind, c.Raw), Producer,
			attrs("platform", c.Platform, "kind", kind, "raw", c.Raw, "constraint", c.Constraint, "source", c.SourceID),
			c.Evidence...))
	}
	return out
}
