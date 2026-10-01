package ingest

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// contentLocator is the content's own locator or the artifact's first
// http / repo-file / repo-dir channel.
func contentLocator(a catalog.Artifact, c catalog.Content) (catalog.Locator, bool) {
	if c.Locator != nil {
		return *c.Locator, true
	}
	for _, ch := range a.Channels {
		if isDocumentKind(ch.Kind) {
			return ch, true
		}
	}
	return catalog.Locator{}, false
}

// captureContents snapshots the artifact's declared contents for artifact
// version av. Every content yields a SourceStatus "<artifact>/<kind>".
func (i *Ingester) captureContents(ctx context.Context, r *run, ar *artifactRun, av string) {
	a := ar.art
	for _, c := range a.Contents {
		id := a.ID + "/" + c.Kind
		st := domain.SourceStatus{SourceID: id, Version: r.v.Semver}
		var evs []domain.Evidence
		noAdapter := false
		finish := func(state domain.SourceState, detail string) {
			st.State, st.Detail = state, detail
			ar.statuses = append(ar.statuses, st)
			ar.checks = append(ar.checks, RelationshipCheck{
				Subject:     id,
				SubjectKind: SubjectContent,
				Channel:     st.Kind,
				Release:     r.v.Semver,
				Outcome:     outcomeForState(state, noAdapter),
				Coordinate:  st.URI,
				Detail:      detail,
				Evidence:    evidenceIDs(evs),
			})
		}
		raw, ok := contentLocator(a, c)
		if !ok {
			finish(domain.SourceError, "no locator: the content declares none and the artifact has no http/repo-file/repo-dir channel")
			continue
		}
		st.Kind = raw.Kind
		applies, err := catalog.AppliesTo(r.v, c.Availability, nil)
		if err != nil {
			finish(domain.SourceError, fmt.Sprintf("availability %q: %v", c.Availability, err))
			continue
		}
		if !applies {
			finish(domain.SourceSkipped, notApplicableDetail(r.v, c.Availability, nil))
			continue
		}
		loc, err := renderLocator(raw, r.rc.WithArtifactVersion(av))
		if err != nil {
			state, detail, _ := stateFor(err)
			finish(state, detail)
			continue
		}
		st.URI = locatorURI(loc)
		f := i.fetch(ctx, r.memo, loc, r.now)
		if f.err != nil {
			state, detail, na := stateFor(f.err)
			noAdapter = na
			finish(state, detail)
			continue
		}
		if len(f.docs) == 0 {
			finish(domain.SourceNotFound, "no files matching "+describeLocator(loc))
			continue
		}
		if len(f.docs) == 1 {
			st.URI = f.docs[0].URI
		}
		ar.addBodies(f.docs, domain.EvidenceDocument)
		for _, d := range f.docs {
			evs = append(evs, domain.NewEvidence(domain.EvidenceStructured, a.ID, d.URI, d.Path, firstLine(d.Content), d.Digest, d.RetrievedAt))
		}
		detail, used, err := i.snapshotContent(r, ar, c.Kind, av, f.docs, evs)
		if err != nil {
			finish(domain.SourceError, fmt.Sprintf("retrieved, but parsing %s failed: %v", c.Kind, err))
			continue
		}
		evs = used
		finish(domain.SourceOK, detail)
	}
}

// snapshotContent turns retrieved content into snapshots / constraints and
// records them (with their evidence and facts) on ar. It returns a summary
// and the document evidence the snapshot rests on.
func (i *Ingester) snapshotContent(r *run, ar *artifactRun, kind, av string, docs []sources.Document, evs []domain.Evidence) (string, []domain.Evidence, error) {
	a := ar.art
	p := i.parser()
	version := av
	if version == "" {
		version = r.v.Tag
	}
	subject := a.ID + "@" + version
	ids := evidenceIDs(evs)
	snapshotFact := func(stmt string, kv ...string) domain.Fact {
		return domain.NewFact(domain.FactSnapshot, subject, r.v.Semver, stmt, Producer,
			attrs(append([]string{"artifact", a.ID, "content", kind, "version", version}, kv...)...), ids...)
	}
	switch kind {
	case catalog.ContentHelmValues:
		vs, err := p.ValuesSnapshot(a.Name, version, docs[0].Content)
		if err != nil {
			return "", nil, err
		}
		if vs == nil {
			return "", nil, errors.New("no values snapshot produced")
		}
		ar.evidence = append(ar.evidence, evs[:1]...)
		ids = ids[:1]
		ar.snapshots = append(ar.snapshots, domain.Snapshot{ArtifactID: a.ID, Kind: domain.SnapshotHelmValues, Values: vs, Evidence: ids})
		ar.facts = append(ar.facts, snapshotFact(fmt.Sprintf("helm values of %s %s: %d keys", a.Name, version, len(vs.Entries))))
		return fmt.Sprintf("%d values keys", len(vs.Entries)), evs[:1], nil

	case catalog.ContentChartMetadata:
		d := docs[0]
		md, err := p.ParseChartMetadata(d.Content)
		if err != nil {
			return "", nil, err
		}
		if md == nil {
			return "", nil, errors.New("no chart metadata produced")
		}
		ar.evidence = append(ar.evidence, evs[:1]...)
		ids = ids[:1]
		ar.facts = append(ar.facts, snapshotFact(
			fmt.Sprintf("chart metadata of %s %s: name=%s version=%s appVersion=%s kubeVersion=%s", a.Name, version, md.Name, md.Version, md.AppVersion, md.KubeVersion),
			"chartName", md.Name, "chartVersion", md.Version, "appVersion", md.AppVersion, "kubeVersion", md.KubeVersion))
		detail := fmt.Sprintf("chart %s version %s appVersion %s", md.Name, md.Version, md.AppVersion)
		if md.KubeVersion == "" {
			return detail + "; no kubeVersion", evs[:1], nil
		}
		locator, excerpt := "", "kubeVersion: "+md.KubeVersion
		if n, line, ok := findLine(d.Content, "kubeVersion:"); ok {
			locator, excerpt = fmt.Sprintf("L%d", n), line
		}
		ev := domain.NewEvidence(domain.EvidenceStructured, a.ID, d.URI, locator, excerpt, d.Digest, d.RetrievedAt)
		cons, versions, err := p.ParseVersionRange(md.KubeVersion)
		if err != nil {
			cons, versions = "", nil
		}
		c := domain.CompatibilityConstraint{
			Platform:   "kubernetes",
			Kind:       "minimum",
			Raw:        md.KubeVersion,
			Constraint: cons,
			Versions:   versions,
			SourceID:   a.ID,
			Provenance: domain.Provenance{
				Method:     domain.MethodDeclared,
				Producer:   Producer,
				Rule:       "chart-metadata:kubeVersion",
				Confidence: domain.ConfidenceHigh,
			},
			Evidence: []domain.EvidenceID{ev.ID},
		}
		ar.evidence = append(ar.evidence, ev)
		ar.compat = append(ar.compat, c)
		ar.facts = append(ar.facts, compatFacts(subject, r.v.Semver, []domain.CompatibilityConstraint{c})...)
		return detail + "; kubeVersion " + md.KubeVersion, append(evs[:1:1], ev), nil

	case catalog.ContentCRDs:
		streams := make([][]byte, len(docs))
		for k, d := range docs {
			streams[k] = d.Content
		}
		snap, err := p.CRDSnapshot(streams...)
		if err != nil {
			return "", nil, err
		}
		if snap == nil {
			return "", nil, errors.New("no CRD snapshot produced")
		}
		ar.evidence = append(ar.evidence, evs...)
		ar.snapshots = append(ar.snapshots, domain.Snapshot{ArtifactID: a.ID, Kind: domain.SnapshotCRDs, CRDs: snap, Evidence: ids})
		ar.facts = append(ar.facts, snapshotFact(fmt.Sprintf("CRDs of %s %s: %d definitions in %d files", a.Name, version, len(snap.CRDs), len(docs))))
		return fmt.Sprintf("%d CRDs from %d files", len(snap.CRDs), len(docs)), evs, nil

	case catalog.ContentImageRefs:
		seen := map[domain.ImageRef]bool{}
		var images []domain.ImageRef
		for _, d := range docs {
			for _, img := range p.ImageRefs(d.Content) {
				if !seen[img] {
					seen[img] = true
					images = append(images, img)
				}
			}
		}
		sort.Slice(images, func(x, y int) bool {
			if images[x].Repository != images[y].Repository {
				return images[x].Repository < images[y].Repository
			}
			if images[x].Tag != images[y].Tag {
				return images[x].Tag < images[y].Tag
			}
			return images[x].Digest < images[y].Digest
		})
		ar.evidence = append(ar.evidence, evs...)
		ar.snapshots = append(ar.snapshots, domain.Snapshot{ArtifactID: a.ID, Kind: domain.SnapshotImages, Images: &domain.ImageRefsSnapshot{Images: images}, Evidence: ids})
		ar.facts = append(ar.facts, snapshotFact(fmt.Sprintf("images referenced by %s %s: %d", a.Name, version, len(images))))
		return fmt.Sprintf("%d image references", len(images)), evs, nil
	}
	return "", nil, fmt.Errorf("unknown content kind %q", kind)
}
