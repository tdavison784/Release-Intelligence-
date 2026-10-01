package ingest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// isDocumentKind reports whether a locator kind is read as documents.
func isDocumentKind(kind string) bool {
	return kind == catalog.LocatorHTTP || kind == catalog.LocatorRepoFile || kind == catalog.LocatorRepoDir
}

// isPackageKind reports whether a locator kind is read as a packaged chart
// (helm-repo index entry, OCI chart artifact, direct .tgz URL).
func isPackageKind(kind string) bool {
	return kind == catalog.LocatorHelmRepo || kind == catalog.LocatorOCI || kind == catalog.LocatorChartTGZ
}

// contentServable reports whether a channel can serve a content: as
// documents or as a packaged chart.
func contentServableChannel(chs []catalog.Locator) (catalog.Locator, bool) {
	for _, ch := range chs {
		if isDocumentKind(ch.Kind) || isPackageKind(ch.Kind) {
			return ch, true
		}
	}
	return catalog.Locator{}, false
}

// contentLocator is the content's own locator or the artifact's first channel
// a content can be read from (a document channel or a packaged-chart
// channel).
func contentLocator(a catalog.Artifact, c catalog.Content) (catalog.Locator, bool) {
	if c.Locator != nil {
		return *c.Locator, true
	}
	return contentServableChannel(a.Channels)
}

// representationOf derives the representation a rendered content locator
// reads (see docs/ARTIFACTS.md): repo-file / repo-dir read the source tree,
// chart packages are the published artifact, plain http documents are
// release assets when their URL says so.
func representationOf(loc catalog.Locator) domain.Representation {
	switch loc.Kind {
	case catalog.LocatorRepoFile, catalog.LocatorRepoDir:
		return domain.RepresentationSourceTree
	case catalog.LocatorHelmRepo, catalog.LocatorChartTGZ:
		// the package adapter refines release-asset URLs itself; a
		// rendered URL that is a release download is a release asset
		if strings.Contains(loc.URL, "/releases/download/") {
			return domain.RepresentationReleaseAsset
		}
		return domain.RepresentationChartTGZ
	case catalog.LocatorOCI:
		return domain.RepresentationOCIChart
	case catalog.LocatorHTTP:
		if strings.Contains(loc.URL, "/releases/download/") {
			return domain.RepresentationReleaseAsset
		}
		return domain.RepresentationHTTPDocument
	}
	return ""
}

// captureContents snapshots the artifact's declared contents for artifact
// version av. Every content yields a SourceStatus "<artifact>/<kind>".
// Contents whose locator (or artifact channel) is a packaged-chart kind are
// read from the published package; every fact and evidence record then
// carries the representation that produced it.
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
			finish(domain.SourceError, "no locator: the content declares none and the artifact has no channel a content can be read from")
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

		// image-refs of a container image read from its registry manifest
		// (the representation registry-manifest).
		if i.isImageManifestContent(a, c, loc) {
			state, detail, imgEvs := i.captureImageManifest(ctx, r, ar, c, loc, av)
			evs = imgEvs
			finish(state, detail)
			continue
		}

		docs, pkg, extraEvs, rep, err := i.readRepresentation(ctx, r, loc, av)
		if err != nil {
			state, detail, na := stateFor(err)
			noAdapter = na
			finish(state, detail)
			continue
		}
		if pkg != nil {
			// only the members this content kind reads
			docs = contentMembers(c.Kind, docs)
		}
		if len(docs) == 0 {
			finish(domain.SourceNotFound, "no "+c.Kind+" member in "+describeLocator(loc))
			continue
		}
		if len(docs) == 1 {
			st.URI = docs[0].URI
		}
		ar.addBodies(docs, domain.EvidenceDocument)
		evs = append(evidenceForDocs(a.ID, docs, rep), extraEvs...)
		detail, used, err := i.snapshotContent(r, ar, c, av, docs, evs, rep)
		if err != nil {
			finish(domain.SourceError, fmt.Sprintf("retrieved, but parsing %s failed: %v", c.Kind, err))
			continue
		}
		evs = used
		if pkg != nil {
			ar.evidence = append(ar.evidence, pkg.Evidence...)
		}
		// Material-difference check: compare with the alternate
		// representation, when one is declared, and record the divergence.
		if c.CompareWith != nil {
			detail += i.compareRepresentations(ctx, r, ar, c, av, docs, rep, nil)
		}
		finish(domain.SourceOK, detail)
	}
}

// isImageManifestContent reports whether an image-refs content of a
// container-image artifact is read from the image's registry manifest.
func (i *Ingester) isImageManifestContent(a catalog.Artifact, c catalog.Content, loc catalog.Locator) bool {
	if c.Kind != catalog.ContentImageRefs || a.Type != domain.ArtifactContainerImage || loc.Kind != catalog.LocatorOCI {
		return false
	}
	_, err := i.Registry.ImageManifestReader(loc.Kind)
	return err == nil
}

// captureImageManifest snapshots an image-refs content from the registry
// manifest and config of the image itself: the reference (repository, tag,
// manifest digest) with the config labels, evidence being the manifest and
// config blob (representation registry-manifest). Returns the source state
// (SourceOK on success), the status detail and the evidence.
func (i *Ingester) captureImageManifest(ctx context.Context, r *run, ar *artifactRun, c catalog.Content, loc catalog.Locator, av string) (domain.SourceState, string, []domain.Evidence) {
	a := ar.art
	reader, err := i.Registry.ImageManifestReader(loc.Kind)
	if err != nil { // guarded by isImageManifestContent
		return domain.SourceError, err.Error(), nil
	}
	img, err := reader.ReadImage(ctx, loc, av)
	if err != nil {
		state, detail, _ := stateFor(err)
		return state, detail, nil
	}
	version := av
	if version == "" {
		version = r.v.Tag
	}
	subject := a.ID + "@" + version
	ids := evidenceIDs(img.Evidence)
	ref := domain.ImageRef{Repository: strings.TrimPrefix(img.Repository, "oci://"), Digest: img.Digest}
	if !strings.HasPrefix(img.Reference, "sha256:") {
		ref.Tag = img.Reference
	}
	images := &domain.ImageRefsSnapshot{Images: []domain.ImageRef{ref}}
	ar.evidence = append(ar.evidence, img.Evidence...)
	ar.snapshots = append(ar.snapshots, domain.Snapshot{ArtifactID: a.ID, Kind: domain.SnapshotImages, Images: images, Evidence: ids})
	kv := []string{"artifact", a.ID, "content", c.Kind, "version", version,
		"representation", string(domain.RepresentationRegistryManifest),
		"repository", ref.Repository, "tag", ref.Tag, "digest", img.Digest}
	for _, k := range sortedKeys(img.Labels) {
		kv = append(kv, "label."+k, img.Labels[k])
	}
	ar.facts = append(ar.facts, domain.NewFact(domain.FactSnapshot, subject, r.v.Semver,
		fmt.Sprintf("image %s exists at %s (registry manifest, %d config labels)", ref.Repository+"@"+img.Digest, img.URI, len(img.Labels)),
		Producer, attrs(kv...), ids...))
	detail := fmt.Sprintf("1 image reference from the registry manifest (%s, digest %s)", ref.Repository, shortDigest(img.Digest))
	if n := len(img.Labels); n > 0 {
		detail += fmt.Sprintf(", %d config labels", n)
	}
	if c.CompareWith != nil {
		detail += i.compareRepresentations(ctx, r, ar, c, av, nil, domain.RepresentationRegistryManifest, images)
	}
	return domain.SourceOK, detail, img.Evidence
}

// readRepresentation reads one representation of a content: packaged chart
// members for package locator kinds, documents otherwise. The returned
// representation annotates evidence and facts; pkg is set for the package
// path (its evidence records the retrieval: index entry, manifest, layer).
func (i *Ingester) readRepresentation(ctx context.Context, r *run, loc catalog.Locator, av string) ([]sources.Document, *sources.ChartPackage, []domain.Evidence, domain.Representation, error) {
	if isPackageKind(loc.Kind) {
		reader, err := i.Registry.ChartPackageReader(loc.Kind)
		if err != nil {
			return nil, nil, nil, "", err
		}
		if strings.TrimSpace(av) == "" {
			return nil, nil, nil, "", fmt.Errorf("content: reading a packaged chart needs the artifact version, which did not resolve")
		}
		pkg, err := reader.ReadChartPackage(ctx, loc, av)
		if err != nil {
			return nil, nil, nil, "", err
		}
		return packageDocuments(pkg, loc), pkg, nil, pkg.Representation, nil
	}
	f := i.fetch(ctx, r.memo, loc, r.now)
	if f.err != nil {
		return nil, nil, nil, "", f.err
	}
	rep := representationOf(loc)
	return f.docs, nil, nil, rep, nil
}

// packageDocuments turns the interesting members of a chart package into
// documents (one per member, sorted by path; URI and digest pin the package,
// the path names the member).
func packageDocuments(pkg *sources.ChartPackage, loc catalog.Locator) []sources.Document {
	type member struct {
		path    string
		content []byte
	}
	var ms []member
	if pkg.ChartYAML != nil {
		ms = append(ms, member{pkg.ChartDir + "/Chart.yaml", pkg.ChartYAML})
	}
	if pkg.Values != nil {
		ms = append(ms, member{pkg.ChartDir + "/values.yaml", pkg.Values})
	}
	for _, f := range pkg.CRDs {
		ms = append(ms, member{pkg.ChartDir + "/" + f.Path, f.Content})
	}
	for _, f := range pkg.Templates {
		ms = append(ms, member{pkg.ChartDir + "/" + f.Path, f.Content})
	}
	sort.SliceStable(ms, func(x, y int) bool { return ms[x].path < ms[y].path })
	out := make([]sources.Document, 0, len(ms))
	for _, m := range ms {
		out = append(out, sources.Document{
			Locator:     loc,
			URI:         pkg.URI + "#" + m.path,
			FetchURL:    pkg.URI,
			Path:        m.path,
			Content:     m.content,
			Format:      "yaml",
			Digest:      pkg.Digest,
			RetrievedAt: pkg.RetrievedAt,
		})
	}
	return out
}

// contentMembers selects the package members a content kind reads. Document
// (non-package) fetches are already addressed per content by their locator.
func contentMembers(kind string, docs []sources.Document) []sources.Document {
	is := func(d sources.Document, suffix string) bool {
		return strings.HasSuffix(d.Path, suffix)
	}
	switch kind {
	case catalog.ContentHelmValues:
		for _, d := range docs {
			if is(d, "/values.yaml") || is(d, "/values.yml") {
				return []sources.Document{d}
			}
		}
		return nil
	case catalog.ContentChartMetadata:
		for _, d := range docs {
			if is(d, "/Chart.yaml") || is(d, "/Chart.yml") {
				return []sources.Document{d}
			}
		}
		return nil
	case catalog.ContentCRDs:
		var out []sources.Document
		for _, d := range docs {
			if strings.Contains("/"+d.Path+"/", "/crds/") {
				out = append(out, d)
			}
		}
		return out
	default: // image-refs: values, templates and CRDs carry the references
		var out []sources.Document
		for _, d := range docs {
			if is(d, "/Chart.yaml") || is(d, "/Chart.yml") {
				continue
			}
			out = append(out, d)
		}
		return out
	}
}

// evidenceForDocs builds one structured evidence record per content document,
// annotated with the representation it was read from.
func evidenceForDocs(sourceID string, docs []sources.Document, rep domain.Representation) []domain.Evidence {
	out := make([]domain.Evidence, 0, len(docs))
	for _, d := range docs {
		out = append(out, domain.NewEvidence(domain.EvidenceStructured, sourceID, d.URI, d.Path,
			firstLine(d.Content), d.Digest, d.RetrievedAt).WithRepresentation(rep))
	}
	return out
}

// snapshotContent turns retrieved content into snapshots / constraints and
// records them (with their evidence and facts) on ar. It returns a summary
// and the document evidence the snapshot rests on. rep is recorded as a fact
// attribute ("representation") so every fact names the form of the artifact
// it came from.
func (i *Ingester) snapshotContent(r *run, ar *artifactRun, c catalog.Content, av string, docs []sources.Document, evs []domain.Evidence, rep domain.Representation) (string, []domain.Evidence, error) {
	a := ar.art
	kind := c.Kind
	p := i.parser()
	version := av
	if version == "" {
		version = r.v.Tag
	}
	subject := a.ID + "@" + version
	ids := evidenceIDs(evs)
	snapshotFact := func(stmt string, kv ...string) domain.Fact {
		return domain.NewFact(domain.FactSnapshot, subject, r.v.Semver, stmt, Producer,
			attrs(append([]string{"artifact", a.ID, "content", kind, "version", version, "representation", string(rep)}, kv...)...), ids...)
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
		if c.StripPrefix != "" {
			vs = stripValuesPrefix(vs, c.StripPrefix)
		}
		ignored := 0
		if len(c.IgnoreKeys) > 0 {
			vs, ignored = ignoreValuesKeys(vs, c.IgnoreKeys)
		}
		ar.evidence = append(ar.evidence, evs[:1]...)
		ids = ids[:1]
		ar.snapshots = append(ar.snapshots, domain.Snapshot{ArtifactID: a.ID, Kind: domain.SnapshotHelmValues, Values: vs, Evidence: ids})
		ar.facts = append(ar.facts, snapshotFact(fmt.Sprintf("helm values of %s %s: %d keys (%s)", a.Name, version, len(vs.Entries), rep)))
		summary := fmt.Sprintf("%d values keys from %s", len(vs.Entries), rep)
		if ignored > 0 {
			summary += fmt.Sprintf(" (%d ignored)", ignored)
		}
		return summary, evs[:1], nil

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
			fmt.Sprintf("chart metadata of %s %s (%s): name=%s version=%s appVersion=%s kubeVersion=%s", a.Name, version, rep, md.Name, md.Version, md.AppVersion, md.KubeVersion),
			"chartName", md.Name, "chartVersion", md.Version, "appVersion", md.AppVersion, "kubeVersion", md.KubeVersion))
		detail := fmt.Sprintf("chart %s version %s appVersion %s (%s)", md.Name, md.Version, md.AppVersion, rep)
		if md.KubeVersion == "" {
			return detail + "; no kubeVersion", evs[:1], nil
		}
		locator, excerpt := "", "kubeVersion: "+md.KubeVersion
		if n, line, ok := findLine(d.Content, "kubeVersion:"); ok {
			locator, excerpt = fmt.Sprintf("L%d", n), line
		}
		ev := domain.NewEvidence(domain.EvidenceStructured, a.ID, d.URI, locator, excerpt, d.Digest, d.RetrievedAt).WithRepresentation(rep)
		cons, versions, err := p.ParseVersionRange(md.KubeVersion)
		if err != nil {
			cons, versions = "", nil
		}
		c := domain.CompatibilityConstraint{
			Platform:   "kubernetes",
			Kind:       "chart-kubeVersion", // an install guard of the chart, not the product support matrix
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
		ar.facts = append(ar.facts, snapshotFact(fmt.Sprintf("CRDs of %s %s: %d definitions in %d files (%s)", a.Name, version, len(snap.CRDs), len(docs), rep)))
		return fmt.Sprintf("%d CRDs from %d files (%s)", len(snap.CRDs), len(docs), rep), evs, nil

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
		ar.facts = append(ar.facts, snapshotFact(fmt.Sprintf("images referenced by %s %s: %d (%s)", a.Name, version, len(images), rep)))
		return fmt.Sprintf("%d image references (%s)", len(images), rep), evs, nil
	}
	return "", nil, fmt.Errorf("unknown content kind %q", kind)
}

// compareRepresentations reads the alternate representation declared by
// content.compareWith, diffs it against the primary one and records a
// representation.divergence fact when they differ. It returns a note for the
// content status detail ("; representations agree" / "; representations
// differ (…)" / "; comparison representation unavailable (…)"). The primary
// representation is never replaced: the definition chose it explicitly, and
// the divergence stays visible as a fact.
func (i *Ingester) compareRepresentations(ctx context.Context, r *run, ar *artifactRun, c catalog.Content, av string, primary []sources.Document, primaryRep domain.Representation, primaryImages *domain.ImageRefsSnapshot) string {
	a := ar.art
	loc, err := renderLocator(*c.CompareWith, r.rc.WithArtifactVersion(av))
	if err != nil {
		return "; comparison representation not read: " + err.Error()
	}
	if i.isImageManifestContent(a, c, loc) {
		// compare the registry image set with the primary document set
		reader, rerr := i.Registry.ImageManifestReader(loc.Kind)
		if rerr != nil {
			return "; comparison representation not read: " + rerr.Error()
		}
		img, ierr := reader.ReadImage(ctx, loc, av)
		if ierr != nil {
			state, detail, _ := stateFor(ierr)
			return fmt.Sprintf("; comparison representation %s: %s", state, detail)
		}
		alt := domain.ImageRefsSnapshot{Images: []domain.ImageRef{{
			Repository: strings.TrimPrefix(img.Repository, "oci://"), Tag: img.Reference, Digest: img.Digest,
		}}}
		prim := primaryImages
		if prim == nil {
			prim = &domain.ImageRefsSnapshot{Images: i.parser().ImageRefs(concatContent(primary))}
		}
		if diff := diffImageRefs(prim, &alt); !diff.empty() {
			i.recordDivergence(r, ar, c, av, primaryRep, domain.RepresentationRegistryManifest, diff, append([]domain.Evidence{}, img.Evidence...))
			return fmt.Sprintf("; representations differ (%s)", diff.summary())
		}
		return "; representations agree"
	}
	altDocs, pkgAlt, extra, altRep, err := i.readRepresentation(ctx, r, loc, av)
	if err != nil {
		state, detail, _ := stateFor(err)
		return fmt.Sprintf("; comparison representation %s: %s", state, detail)
	}
	if pkgAlt != nil {
		altDocs = contentMembers(c.Kind, altDocs)
	}
	if len(altDocs) == 0 {
		return "; comparison representation: no " + c.Kind + " member in " + describeLocator(loc)
	}
	altEvs := append(evidenceForDocs(a.ID, altDocs, altRep), extra...)
	diff, err := i.diffContent(c, primary, altDocs)
	if err != nil {
		return "; comparison representation not parsed: " + err.Error()
	}
	if diff.empty() {
		return "; representations agree"
	}
	i.recordDivergence(r, ar, c, av, primaryRep, altRep, diff, altEvs)
	return fmt.Sprintf("; representations differ (%s)", diff.summary())
}

// diffContent computes the material difference between two representations
// of the same content, per content kind. Content transforms (stripPrefix,
// ignoreKeys) are applied to both sides, so only real divergence shows.
func (i *Ingester) diffContent(c catalog.Content, primary, alternate []sources.Document) (*representationDiff, error) {
	p := i.parser()
	switch c.Kind {
	case catalog.ContentHelmValues:
		flatten := func(docs []sources.Document) (map[string]string, error) {
			if len(docs) == 0 {
				return nil, errors.New("no values document")
			}
			vs, err := p.ValuesSnapshot("chart", "", docs[0].Content)
			if err != nil {
				return nil, err
			}
			if c.StripPrefix != "" {
				vs = stripValuesPrefix(vs, c.StripPrefix)
			}
			if len(c.IgnoreKeys) > 0 {
				vs, _ = ignoreValuesKeys(vs, c.IgnoreKeys)
			}
			return vs.Entries, nil
		}
		a, err := flatten(primary)
		if err != nil {
			return nil, err
		}
		b, err := flatten(alternate)
		if err != nil {
			return nil, err
		}
		return diffKeyValues(a, b), nil
	case catalog.ContentChartMetadata:
		meta := func(docs []sources.Document) (string, error) {
			if len(docs) == 0 {
				return "", errors.New("no Chart.yaml document")
			}
			md, err := p.ParseChartMetadata(docs[0].Content)
			if err != nil {
				return "", err
			}
			if md == nil {
				return "", errors.New("no chart metadata")
			}
			return fmt.Sprintf("name=%s version=%s appVersion=%s kubeVersion=%s", md.Name, md.Version, md.AppVersion, md.KubeVersion), nil
		}
		a, err := meta(primary)
		if err != nil {
			return nil, err
		}
		b, err := meta(alternate)
		if err != nil {
			return nil, err
		}
		d := &representationDiff{}
		if a != b {
			d.changed = []string{a + " vs " + b}
		}
		return d, nil
	case catalog.ContentCRDs:
		set := func(docs []sources.Document) (map[string]bool, error) {
			streams := make([][]byte, len(docs))
			for k, d := range docs {
				streams[k] = d.Content
			}
			snap, err := p.CRDSnapshot(streams...)
			if err != nil {
				return nil, err
			}
			out := map[string]bool{}
			for _, crd := range snap.CRDs {
				vs := make([]string, len(crd.Versions))
				for k, v := range crd.Versions {
					vs[k] = v.Name
				}
				sort.Strings(vs)
				out[crd.Name+" ("+strings.Join(vs, ",")+")"] = true
			}
			return out, nil
		}
		a, err := set(primary)
		if err != nil {
			return nil, err
		}
		b, err := set(alternate)
		if err != nil {
			return nil, err
		}
		return diffSets(a, b), nil
	case catalog.ContentImageRefs:
		refs := func(docs []sources.Document) map[string]bool {
			out := map[string]bool{}
			for _, img := range p.ImageRefs(concatContent(docs)) {
				out[imageRefKey(img)] = true
			}
			return out
		}
		return diffSets(refs(primary), refs(alternate)), nil
	}
	return nil, fmt.Errorf("comparison is not supported for content kind %q", c.Kind)
}

// recordDivergence records the representation.divergence fact (and the
// alternate side's evidence, so the fact cites both representations).
func (i *Ingester) recordDivergence(r *run, ar *artifactRun, c catalog.Content, av string, repA, repB domain.Representation, diff *representationDiff, altEvs []domain.Evidence) {
	a := ar.art
	version := av
	if version == "" {
		version = r.v.Tag
	}
	subject := a.ID + "@" + version
	ar.evidence = append(ar.evidence, altEvs...)
	ids := evidenceIDs(altEvs)
	kv := []string{
		"artifact", a.ID, "content", c.Kind, "version", version,
		"representationA", string(repA), "representationB", string(repB),
		"added", fmt.Sprint(len(diff.added)), "removed", fmt.Sprint(len(diff.removed)), "changed", fmt.Sprint(len(diff.changed)),
		"keys", boundJoin(diff.all(), 400),
	}
	stmt := fmt.Sprintf("%s content of %s %s differs between %s and %s: %d added, %d removed, %d changed",
		c.Kind, a.Name, version, repA, repB, len(diff.added), len(diff.removed), len(diff.changed))
	if keys := boundJoin(diff.all(), 5); keys != "" {
		stmt += " (" + keys + ")"
	}
	ar.facts = append(ar.facts, domain.NewFact(domain.FactRepresentationDivergence, subject, r.v.Semver, stmt, Producer, attrs(kv...), ids...))
}

// ---------------------------------------------------------------------------
// representation diff

// representationDiff is the difference between two representations of one
// content: added / removed / changed keys (values), fields (metadata), names
// (CRDs) or references (images).
type representationDiff struct {
	added, removed, changed []string
}

func (d *representationDiff) empty() bool {
	return d == nil || (len(d.added) == 0 && len(d.removed) == 0 && len(d.changed) == 0)
}

func (d *representationDiff) all() []string {
	if d == nil {
		return nil
	}
	out := append([]string{}, d.added...)
	out = append(out, d.removed...)
	out = append(out, d.changed...)
	sort.Strings(out)
	return out
}

// summary is a compact, deterministic description of the divergence.
func (d *representationDiff) summary() string {
	if d.empty() {
		return ""
	}
	var parts []string
	if len(d.added) > 0 {
		parts = append(parts, fmt.Sprintf("+%d", len(d.added)))
	}
	if len(d.removed) > 0 {
		parts = append(parts, fmt.Sprintf("-%d", len(d.removed)))
	}
	if len(d.changed) > 0 {
		parts = append(parts, fmt.Sprintf("~%d", len(d.changed)))
	}
	if keys := boundJoin(d.all(), 3); keys != "" {
		parts = append(parts, keys)
	}
	return strings.Join(parts, " ")
}

// diffKeyValues diffs two flattened values maps.
func diffKeyValues(a, b map[string]string) *representationDiff {
	d := &representationDiff{}
	for k, va := range a {
		vb, ok := b[k]
		switch {
		case !ok:
			d.removed = append(d.removed, k)
		case va != vb:
			d.changed = append(d.changed, k+": "+va+" -> "+vb)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			d.added = append(d.added, k)
		}
	}
	sort.Strings(d.added)
	sort.Strings(d.removed)
	sort.Strings(d.changed)
	return d
}

// diffSets diffs two sets of content items.
func diffSets(a, b map[string]bool) *representationDiff {
	d := &representationDiff{}
	for k := range a {
		if !b[k] {
			d.removed = append(d.removed, k)
		}
	}
	for k := range b {
		if !a[k] {
			d.added = append(d.added, k)
		}
	}
	sort.Strings(d.added)
	sort.Strings(d.removed)
	return d
}

// diffImageRefs diffs two image reference sets.
func diffImageRefs(a, b *domain.ImageRefsSnapshot) *representationDiff {
	set := func(s *domain.ImageRefsSnapshot) map[string]bool {
		out := map[string]bool{}
		if s == nil {
			return out
		}
		for _, img := range s.Images {
			out[imageRefKey(img)] = true
		}
		return out
	}
	return diffSets(set(a), set(b))
}

func imageRefKey(img domain.ImageRef) string {
	s := img.Repository
	if img.Tag != "" {
		s += ":" + img.Tag
	}
	if img.Digest != "" {
		s += "@" + img.Digest
	}
	return s
}

func concatContent(docs []sources.Document) []byte {
	var b []byte
	for _, d := range docs {
		b = append(b, d.Content...)
		b = append(b, '\n')
	}
	return b
}

func boundJoin(parts []string, max int) string {
	if len(parts) == 0 {
		return ""
	}
	n := len(parts)
	if n > max {
		n = max
	}
	s := strings.Join(parts[:n], ", ")
	if len(parts) > max {
		s += fmt.Sprintf(" (+%d more)", len(parts)-max)
	}
	if len(s) > 600 {
		s = s[:597] + "..."
	}
	return s
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func shortDigest(d string) string {
	if len(d) > 19 {
		return d[:19]
	}
	return d
}

// stripValuesPrefix removes a wrapper key path ("a.b") from every values key
// that starts with it; other keys are kept as they are.
func stripValuesPrefix(vs *domain.ValuesSnapshot, prefix string) *domain.ValuesSnapshot {
	prefix = strings.TrimSuffix(prefix, ".") + "."
	out := &domain.ValuesSnapshot{Chart: vs.Chart, Version: vs.Version, Entries: map[string]string{}}
	if vs.Comments != nil {
		out.Comments = map[string]string{}
	}
	for k, v := range vs.Entries {
		out.Entries[strings.TrimPrefix(k, prefix)] = v
	}
	for k, v := range vs.Comments {
		out.Comments[strings.TrimPrefix(k, prefix)] = v
	}
	return out
}

// ignoreValuesKeys removes the keys named by ignore from a values snapshot
// (both entries and comments) and returns the number of removed entries. An
// entry matches one key exactly; an entry ending in ".*" matches every key
// below it ("a.b.*" matches "a.b.c", "a.b.c.d" and `a.b["x.y"]`; list "a.b"
// as well to drop a key that is itself a leaf).
func ignoreValuesKeys(vs *domain.ValuesSnapshot, ignore []string) (*domain.ValuesSnapshot, int) {
	exact := map[string]bool{}
	var below []string // prefixes without the trailing ".*"
	for _, k := range ignore {
		if base, ok := strings.CutSuffix(k, ".*"); ok {
			below = append(below, base)
			continue
		}
		exact[k] = true
	}
	// the section a ".*" entry covers has no entries of its own, but its doc
	// comment is dropped with it
	matches := func(key string, withSection bool) bool {
		if exact[key] {
			return true
		}
		for _, base := range below {
			if strings.HasPrefix(key, base+".") || strings.HasPrefix(key, base+"[") || (withSection && key == base) {
				return true
			}
		}
		return false
	}
	out := &domain.ValuesSnapshot{Chart: vs.Chart, Version: vs.Version, Entries: map[string]string{}}
	if vs.Comments != nil {
		out.Comments = map[string]string{}
	}
	removed := 0
	for k, v := range vs.Entries {
		if matches(k, false) {
			removed++
			continue
		}
		out.Entries[k] = v
	}
	for k, v := range vs.Comments {
		if !matches(k, true) {
			out.Comments[k] = v
		}
	}
	return out, removed
}
