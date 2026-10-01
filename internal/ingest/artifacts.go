package ingest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// artifactRun accumulates everything learnt about one artifact of a release.
type artifactRun struct {
	art        catalog.Artifact
	applicable bool
	instances  []*instanceRun
	statuses   []domain.SourceStatus // channel probes / index lookups, then contents
	snapshots  []domain.Snapshot
	compat     []domain.CompatibilityConstraint
	facts      []domain.Fact
	evidence   []domain.Evidence
	checks     []RelationshipCheck // content checks
	// bodies are the raw bytes retrieved for this artifact (channels, then
	// contents), searched when another artifact references this one.
	bodies []rawBody
}

type instanceRun struct {
	inst     domain.ArtifactInstance
	av       string
	evidence []domain.Evidence
}

type rawBody struct {
	uri     string
	digest  string
	at      time.Time
	content []byte
	kind    domain.EvidenceKind
}

func (ar *artifactRun) addBodies(docs []sources.Document, kind domain.EvidenceKind) {
	for _, d := range docs {
		dup := false
		for _, b := range ar.bodies {
			if b.uri == d.URI && b.digest == d.Digest {
				dup = true
				break
			}
		}
		if !dup {
			ar.bodies = append(ar.bodies, rawBody{uri: d.URI, digest: d.Digest, at: d.RetrievedAt, content: d.Content, kind: kind})
		}
	}
}

// resolvedVersion is one artifact version derived from the release version.
type resolvedVersion struct {
	version  string
	digest   string
	evidence []domain.Evidence // index entry evidence (lookup)
}

// resolution is the outcome of resolving an artifact's version(s). When
// versions is empty, status/detail describe why.
type resolution struct {
	versions   []resolvedVersion
	status     domain.ArtifactStatus
	detail     string
	coordinate string
}

func contentUsesChannel(a catalog.Artifact) bool {
	for _, c := range a.Contents {
		if c.Locator == nil {
			return true
		}
	}
	return false
}

// runArtifact resolves, probes and snapshots one artifact. References are
// resolved later, once every artifact's bodies are known.
func (i *Ingester) runArtifact(ctx context.Context, r *run, a catalog.Artifact, referenced bool) *artifactRun {
	ar := &artifactRun{art: a}
	base := domain.ArtifactInstance{ArtifactID: a.ID, Type: a.Type, Name: a.Name}
	ok, err := catalog.AppliesTo(r.v, a.Availability, nil)
	if err != nil || !ok {
		inst := base
		inst.Status = domain.ArtifactNotApplicable
		inst.Detail = notApplicableDetail(r.v, a.Availability, nil)
		inst.Coordinate = firstCoordinate(a, r.rc)
		if err != nil {
			inst.Status = domain.ArtifactExpected
			inst.Detail = fmt.Sprintf("cannot evaluate availability %q: %v", a.Availability, err)
		}
		ar.instances = []*instanceRun{{inst: inst}}
		return ar
	}
	if reason, ok := catalog.ExceptionFor(r.v, a.Exceptions); ok {
		inst := base
		inst.Status = domain.ArtifactNotApplicable
		inst.Detail = "known exception: " + reason
		inst.Coordinate = firstCoordinate(a, r.rc)
		ar.instances = []*instanceRun{{inst: inst}}
		return ar
	}
	ar.applicable = true
	res := i.resolveVersions(ctx, r, ar)
	if len(res.versions) == 0 {
		inst := base
		inst.Status, inst.Detail, inst.Coordinate = res.status, res.detail, res.coordinate
		ar.instances = []*instanceRun{{inst: inst}}
	} else {
		needBody := referenced || contentUsesChannel(a)
		for _, rv := range res.versions {
			ar.instances = append(ar.instances, i.probeChannels(ctx, r, ar, base, rv, needBody))
		}
	}
	// Contents are captured for the selected (latest) artifact version.
	i.captureContents(ctx, r, ar, ar.instances[len(ar.instances)-1].av)
	return ar
}

func (i *Ingester) resolveVersions(ctx context.Context, r *run, ar *artifactRun) resolution {
	a := ar.art
	switch a.Version.Strategy {
	case catalog.VersionTemplate:
		av, err := catalog.Render(a.Version.Template, r.rc)
		if err != nil {
			return resolution{status: domain.ArtifactExpected, detail: "version template: " + err.Error(), coordinate: firstCoordinate(a, r.rc)}
		}
		if strings.TrimSpace(av) == "" {
			return resolution{status: domain.ArtifactExpected, detail: fmt.Sprintf("version template %q rendered empty", a.Version.Template), coordinate: firstCoordinate(a, r.rc)}
		}
		return resolution{versions: []resolvedVersion{{version: av}}}
	case catalog.VersionLookup:
		return i.lookupVersions(ctx, r, ar)
	case catalog.VersionField:
		return i.fieldVersion(ctx, r, ar)
	case catalog.VersionIndependent:
		return resolution{
			status:     domain.ArtifactExpected,
			detail:     "artifact version is independent of the release version (strategy independent); not resolvable",
			coordinate: firstCoordinate(a, r.rc),
		}
	default:
		return resolution{status: domain.ArtifactExpected, detail: fmt.Sprintf("unknown version strategy %q", a.Version.Strategy), coordinate: firstCoordinate(a, r.rc)}
	}
}

// firstCoordinate describes the artifact at its first channel without a version.
func firstCoordinate(a catalog.Artifact, rc catalog.RenderContext) string {
	if len(a.Channels) == 0 {
		return ""
	}
	loc, err := renderLocator(a.Channels[0], rc)
	if err != nil {
		return ""
	}
	return coordinateFor(loc, "")
}

// lookupVersions searches the version index of the artifact's channels (in
// order; the first index that answers and exposes the field decides) for
// entries whose Field equals the rendered Match.
func (i *Ingester) lookupVersions(ctx context.Context, r *run, ar *artifactRun) resolution {
	a := ar.art
	field := a.Version.Field
	match, err := catalog.Render(a.Version.Match, r.rc)
	if err != nil {
		return resolution{status: domain.ArtifactExpected, detail: "lookup match: " + err.Error(), coordinate: firstCoordinate(a, r.rc)}
	}
	var notes []string
	anyIndex := false
	for _, ch := range a.Channels {
		idx, err := i.Registry.VersionIndex(ch.Kind)
		if err != nil {
			continue
		}
		anyIndex = true
		st := domain.SourceStatus{SourceID: a.ID, Kind: ch.Kind, Version: r.v.Semver}
		loc, err := renderLocator(ch, r.rc)
		if err != nil {
			st.State, st.Detail, _ = stateFor(err)
			ar.statuses = append(ar.statuses, st)
			notes = append(notes, fmt.Sprintf("%s: %s", ch.Kind, st.Detail))
			continue
		}
		st.URI = locatorURI(loc)
		entries, err := idx.ListArtifactVersions(ctx, loc)
		if err != nil {
			st.State, st.Detail, _ = stateFor(err)
			ar.statuses = append(ar.statuses, st)
			notes = append(notes, fmt.Sprintf("%s index %s: %s", ch.Kind, describeLocator(loc), st.State))
			continue
		}
		var matched []sources.ArtifactVersion
		exposes := false
		for _, e := range entries {
			val, ok := e.Fields[field]
			if !ok {
				continue
			}
			exposes = true
			if val == match {
				matched = append(matched, e)
			}
		}
		if !exposes {
			st.State = domain.SourcePartial
			st.Detail = fmt.Sprintf("version index lists %d versions but none exposes %s", len(entries), field)
			ar.statuses = append(ar.statuses, st)
			notes = append(notes, fmt.Sprintf("%s index %s does not expose %s", ch.Kind, describeLocator(loc), field))
			continue
		}
		if len(matched) == 0 {
			st.State = domain.SourceOK
			st.Detail = fmt.Sprintf("%d versions; none with %s %s", len(entries), field, match)
			ar.statuses = append(ar.statuses, st)
			return resolution{
				status:     domain.ArtifactMissing,
				detail:     fmt.Sprintf("no %s version ships %s %s (%s lists %d versions)", typeNoun(a.Type), field, match, describeLocator(loc), len(entries)),
				coordinate: coordinateFor(loc, ""),
			}
		}
		selected := selectArtifactVersions(matched, a.Version.Select)
		names := make([]string, len(selected))
		for k, e := range selected {
			names[k] = e.Version
		}
		st.State = domain.SourceOK
		st.Detail = fmt.Sprintf("%d versions; %d with %s %s; selected %s", len(entries), len(matched), field, match, strings.Join(names, ", "))
		ar.statuses = append(ar.statuses, st)
		out := make([]resolvedVersion, 0, len(selected))
		for _, e := range selected {
			rv := resolvedVersion{version: e.Version, digest: e.Digest}
			if e.Evidence.ID != "" {
				rv.evidence = []domain.Evidence{e.Evidence}
			}
			out = append(out, rv)
		}
		return resolution{versions: out}
	}
	if !anyIndex {
		return resolution{
			status:     domain.ArtifactExpected,
			detail:     fmt.Sprintf("no version index available for the channels of %s; %s %s cannot be looked up", a.ID, field, match),
			coordinate: firstCoordinate(a, r.rc),
		}
	}
	return resolution{
		status:     domain.ArtifactExpected,
		detail:     "no version index answered: " + strings.Join(notes, "; "),
		coordinate: firstCoordinate(a, r.rc),
	}
}

// fieldVersion resolves an artifact version with strategy "field": the
// version is read out of a YAML document fetched at the release ref (the
// pinned sub-component versions of an aggregating chart — appVersion,
// dependencies[name=x].version, a values.yaml image tag — or any other
// per-release pin inside a file). The evidence is the document itself, with
// the path (and line) that carried the value.
func (i *Ingester) fieldVersion(ctx context.Context, r *run, ar *artifactRun) resolution {
	a := ar.art
	from := a.Version.From
	if from == nil {
		return resolution{status: domain.ArtifactExpected, detail: "strategy field declares no from locator", coordinate: firstCoordinate(a, r.rc)}
	}
	loc, err := renderLocator(*from, r.rc)
	if err != nil {
		return resolution{status: domain.ArtifactExpected, detail: "field from locator: " + err.Error(), coordinate: firstCoordinate(a, r.rc)}
	}
	f := i.fetch(ctx, r.memo, loc, r.now)
	st := domain.SourceStatus{SourceID: a.ID, Kind: loc.Kind, Version: r.v.Semver, URI: locatorURI(loc)}
	if f.err != nil {
		state, detail, _ := stateFor(f.err)
		st.State, st.Detail = state, detail
		ar.statuses = append(ar.statuses, st)
		return resolution{status: domain.ArtifactExpected, detail: fmt.Sprintf("cannot read %s: %s", describeLocator(loc), detail), coordinate: firstCoordinate(a, r.rc)}
	}
	if len(f.docs) == 0 {
		st.State, st.Detail = domain.SourceNotFound, "no document at "+describeLocator(loc)
		ar.statuses = append(ar.statuses, st)
		return resolution{status: domain.ArtifactMissing, detail: "no document at " + describeLocator(loc), coordinate: firstCoordinate(a, r.rc)}
	}
	d := f.docs[0]
	st.URI = d.URI
	value, line, err := i.parser().ReadYAMLPath(d.Content, a.Version.Field)
	if err != nil {
		if errors.Is(err, normalize.ErrNoMatch) {
			st.State, st.Detail = domain.SourceNotFound, err.Error()
			ar.statuses = append(ar.statuses, st)
			return resolution{status: domain.ArtifactMissing, detail: fmt.Sprintf("%s does not carry %s (%s)", describeLocator(loc), a.Version.Field, err), coordinate: firstCoordinate(a, r.rc)}
		}
		st.State, st.Detail = domain.SourceError, err.Error()
		ar.statuses = append(ar.statuses, st)
		return resolution{status: domain.ArtifactExpected, detail: fmt.Sprintf("reading %s from %s: %v", a.Version.Field, describeLocator(loc), err), coordinate: firstCoordinate(a, r.rc)}
	}
	if strings.TrimSpace(value) == "" {
		st.State, st.Detail = domain.SourcePartial, fmt.Sprintf("%s is empty", a.Version.Field)
		ar.statuses = append(ar.statuses, st)
		return resolution{status: domain.ArtifactExpected, detail: fmt.Sprintf("%s of %s is empty", a.Version.Field, describeLocator(loc)), coordinate: firstCoordinate(a, r.rc)}
	}
	st.State = domain.SourceOK
	st.Detail = fmt.Sprintf("%s = %s (%s)", a.Version.Field, value, describeLocator(loc))
	ar.statuses = append(ar.statuses, st)
	elocator := "$." + a.Version.Field
	if line > 0 {
		elocator = fmt.Sprintf("L%d", line)
	}
	ev := domain.NewEvidence(domain.EvidenceStructured, a.ID, d.URI, elocator, fmt.Sprintf("%s: %s", a.Version.Field, value), d.Digest, d.RetrievedAt)
	return resolution{versions: []resolvedVersion{{version: value, evidence: []domain.Evidence{ev}}}}
}

func typeNoun(t domain.ArtifactType) string {
	switch t {
	case domain.ArtifactHelmChart:
		return "chart"
	case domain.ArtifactContainerImage:
		return "image"
	}
	return string(t)
}

// selectArtifactVersions orders matches by artifact version (semver, else
// string order), drops duplicates and applies the select policy.
func selectArtifactVersions(vs []sources.ArtifactVersion, sel string) []sources.ArtifactVersion {
	sorted := append([]sources.ArtifactVersion(nil), vs...)
	sort.SliceStable(sorted, func(a, b int) bool { return compareArtifactVersions(sorted[a].Version, sorted[b].Version) < 0 })
	var uniq []sources.ArtifactVersion
	for _, e := range sorted {
		if len(uniq) > 0 && uniq[len(uniq)-1].Version == e.Version {
			continue
		}
		uniq = append(uniq, e)
	}
	switch sel {
	case "earliest":
		return uniq[:1]
	case "all":
		return uniq
	default:
		return uniq[len(uniq)-1:]
	}
}

// probeChannels checks one artifact version at each channel in order. The
// first channel where it exists makes the instance verified. Otherwise the
// instance is missing when every channel answered "absent", and expected
// when at least one channel could not be checked.
func (i *Ingester) probeChannels(ctx context.Context, r *run, ar *artifactRun, base domain.ArtifactInstance, rv resolvedVersion, needBody bool) *instanceRun {
	a := ar.art
	ir := &instanceRun{inst: base, av: rv.version}
	ir.inst.Version = rv.version
	ir.inst.Digest = rv.digest
	ir.evidence = append(ir.evidence, rv.evidence...)
	rc := r.rc.WithArtifactVersion(rv.version)
	var absent, unreachable []string
	var absentEv []domain.Evidence
	record := func(st domain.SourceStatus, coord string) {
		ar.statuses = append(ar.statuses, st)
		if st.State == domain.SourceNotFound {
			absent = append(absent, coord)
		} else {
			unreachable = append(unreachable, fmt.Sprintf("%s (%s: %s)", coord, st.Kind, st.State))
		}
	}
	for _, ch := range a.Channels {
		st := domain.SourceStatus{SourceID: a.ID, Kind: ch.Kind, Version: r.v.Semver}
		loc, err := renderLocator(ch, rc)
		if err != nil {
			st.State, st.Detail, _ = stateFor(err)
			record(st, ch.Kind+" channel")
			continue
		}
		st.URI = locatorURI(loc)
		coord := coordinateFor(loc, rv.version)
		if ir.inst.Coordinate == "" {
			ir.inst.Coordinate = coord
		}
		if needBody && isDocumentKind(loc.Kind) && i.hasDocumentAdapter(loc.Kind) {
			f := i.fetch(ctx, r.memo, loc, r.now)
			if f.err == nil && len(f.docs) > 0 {
				kind := domain.EvidenceDocument
				if loc.Kind == catalog.LocatorHTTP {
					kind = domain.EvidenceReleaseAsset
				}
				ar.addBodies(f.docs, kind)
				for _, d := range f.docs {
					ir.evidence = append(ir.evidence, domain.NewEvidence(kind, a.ID, d.URI, "", firstLine(d.Content), d.Digest, d.RetrievedAt))
				}
				st.State = domain.SourceOK
				if len(f.docs) == 1 {
					st.URI = f.docs[0].URI
					st.Detail = fmt.Sprintf("retrieved (%d bytes)", len(f.docs[0].Content))
					ir.inst.Digest = f.docs[0].Digest
				} else {
					st.Detail = fmt.Sprintf("retrieved %d files", len(f.docs))
				}
				ar.statuses = append(ar.statuses, st)
				ir.inst.Status = domain.ArtifactVerified
				ir.inst.Channel = loc.Kind
				ir.inst.Coordinate = coord
				ir.inst.Detail = "retrieved from " + st.URI
				return ir
			}
			if f.err == nil {
				st.State, st.Detail = domain.SourceNotFound, "no files matching "+describeLocator(loc)
			} else {
				st.State, st.Detail, _ = stateFor(f.err)
			}
			record(st, coord)
			continue
		}
		probe, err := i.Registry.Probe(loc.Kind)
		if err != nil {
			st.State, st.Detail, _ = stateFor(err)
			record(st, coord)
			continue
		}
		res, err := probe.Probe(ctx, loc, rv.version)
		if err == nil && res == nil {
			err = errors.New("probe returned no result")
		}
		if err != nil {
			st.State, st.Detail, _ = stateFor(err)
			record(st, coord)
			continue
		}
		if res.Coordinate != "" {
			coord = res.Coordinate
		}
		if res.URI != "" {
			st.URI = res.URI
		}
		if res.Exists {
			st.State, st.Detail = domain.SourceOK, "exists"
			if res.Digest != "" {
				st.Detail += " (" + res.Digest + ")"
				ir.inst.Digest = res.Digest
			}
			ar.statuses = append(ar.statuses, st)
			ev := res.Evidence
			if ev.ID == "" {
				uri := res.URI
				if uri == "" {
					uri = locatorURI(loc)
				}
				ev = domain.NewEvidence(domain.EvidenceRegistry, a.ID, uri, coord, coord, res.Digest, r.now)
			}
			ir.evidence = append(ir.evidence, ev)
			ir.inst.Status = domain.ArtifactVerified
			ir.inst.Channel = loc.Kind
			ir.inst.Coordinate = coord
			ir.inst.Detail = "observed at " + loc.Kind + " channel"
			return ir
		}
		st.State, st.Detail = domain.SourceNotFound, "absent"
		if res.Evidence.ID != "" {
			absentEv = append(absentEv, res.Evidence)
		}
		record(st, coord)
	}
	ir.evidence = append(ir.evidence, absentEv...)
	switch {
	case len(absent) > 0 && len(unreachable) == 0:
		ir.inst.Status = domain.ArtifactMissing
		ir.inst.Detail = "absent from " + strings.Join(absent, ", ")
	default:
		ir.inst.Status = domain.ArtifactExpected
		var parts []string
		if len(absent) > 0 {
			parts = append(parts, "absent from "+strings.Join(absent, ", "))
		}
		if len(unreachable) > 0 {
			parts = append(parts, "not verifiable: "+strings.Join(unreachable, ", "))
		}
		if len(parts) == 0 {
			parts = append(parts, "no channels declared")
		}
		ir.inst.Detail = strings.Join(parts, "; ")
	}
	return ir
}

// resolveReferences cross-checks artifacts that could not be verified at a
// channel against the raw content of the artifacts that reference them
// (e.g. the image tag inside the install manifest). An expected artifact
// that is found becomes referenced; a missing one stays missing (a reachable
// channel said it is absent) and the reference is noted in its detail.
func resolveReferences(r *run, arts []*artifactRun) {
	byID := map[string]*artifactRun{}
	for _, ar := range arts {
		byID[ar.art.ID] = ar
	}
	for _, ar := range arts {
		if !ar.applicable || len(ar.art.References) == 0 {
			continue
		}
		for _, ir := range ar.instances {
			status := ir.inst.Status
			if status != domain.ArtifactExpected && status != domain.ArtifactMissing {
				continue
			}
			var notes []string
			for _, ref := range ar.art.References {
				if strings.Contains(ref.Pattern, ".ArtifactVersion") && ir.av == "" {
					notes = append(notes, fmt.Sprintf("reference in %s needs the artifact version", ref.Artifact))
					continue
				}
				pattern, err := catalog.Render(ref.Pattern, r.rc.WithArtifactVersion(ir.av))
				if err != nil {
					notes = append(notes, fmt.Sprintf("reference pattern: %v", err))
					continue
				}
				src := byID[ref.Artifact]
				if src == nil || len(src.bodies) == 0 {
					notes = append(notes, fmt.Sprintf("referencing artifact %s was not retrieved", ref.Artifact))
					continue
				}
				hit := false
				for _, b := range src.bodies {
					n, line, ok := findReference(b.content, pattern)
					if !ok {
						continue
					}
					ir.evidence = append(ir.evidence, domain.NewEvidence(b.kind, ref.Artifact, b.uri, fmt.Sprintf("L%d", n), line, b.digest, b.at))
					where := fmt.Sprintf("%s (%s L%d)", ref.Artifact, b.uri, n)
					if status == domain.ArtifactExpected {
						ir.inst.Status = domain.ArtifactReferenced
						ir.inst.Channel = "reference:" + ref.Artifact
						ir.inst.Detail = joinNonEmpty("; ", "referenced by "+where, ir.inst.Detail)
					} else {
						ir.inst.Detail = joinNonEmpty("; ", ir.inst.Detail, "referenced by "+where+" although absent from every reachable channel")
					}
					hit = true
					break
				}
				if hit {
					notes = nil
					break
				}
				notes = append(notes, fmt.Sprintf("%q not found in %s", pattern, ref.Artifact))
			}
			if len(notes) > 0 {
				ir.inst.Detail = joinNonEmpty("; ", ir.inst.Detail, strings.Join(notes, "; "))
			}
		}
	}
}

// artifactFact records an artifact.published fact for verified and
// referenced instances.
func artifactFact(r *run, inst domain.ArtifactInstance) (domain.Fact, bool) {
	var stmt string
	switch inst.Status {
	case domain.ArtifactVerified:
		stmt = fmt.Sprintf("%s %s is published at %s (%s)", inst.Name, inst.Version, inst.Coordinate, inst.Channel)
	case domain.ArtifactReferenced:
		stmt = fmt.Sprintf("%s %s is referenced as %s by %s", inst.Name, inst.Version, inst.Coordinate, strings.TrimPrefix(inst.Channel, "reference:"))
	default:
		return domain.Fact{}, false
	}
	subject := inst.Coordinate
	if subject == "" {
		subject = inst.ArtifactID
	}
	return domain.NewFact(domain.FactArtifactPublished, subject, r.v.Semver, stmt, Producer,
		attrs("artifact", inst.ArtifactID, "type", string(inst.Type), "status", string(inst.Status),
			"channel", inst.Channel, "version", inst.Version, "digest", inst.Digest, "coordinate", inst.Coordinate),
		inst.Evidence...), true
}

func artifactCheck(r *run, a catalog.Artifact, inst domain.ArtifactInstance) RelationshipCheck {
	channel := inst.Channel
	if channel == "" && len(a.Channels) > 0 {
		channel = a.Channels[0].Kind
	}
	var outcome string
	switch inst.Status {
	case domain.ArtifactVerified, domain.ArtifactReferenced:
		outcome = OutcomePass
	case domain.ArtifactMissing:
		outcome = OutcomeFail
		if a.Optional {
			// absence of an optional artifact is a fact, not a broken relationship
			outcome = OutcomeNotApplicable
		}
	case domain.ArtifactNotApplicable:
		outcome = OutcomeNotApplicable
	default:
		outcome = OutcomeUnverifiable
	}
	return RelationshipCheck{
		Subject:     a.ID,
		SubjectKind: SubjectArtifact,
		Channel:     channel,
		Release:     r.v.Semver,
		Outcome:     outcome,
		Coordinate:  inst.Coordinate,
		Detail:      inst.Detail,
		Evidence:    inst.Evidence,
	}
}
