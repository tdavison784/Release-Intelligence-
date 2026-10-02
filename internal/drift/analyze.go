package drift

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/ingest"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// ChannelRegistry is the subset of *sources.Registry that drift re-probes
// declared channels with (a lookup artifact missing from its primary index,
// availability windows). Nil disables every probe that needs it.
type ChannelRegistry interface {
	Probe(kind string) (sources.ArtifactProbe, error)
	VersionIndex(kind string) (sources.VersionIndex, error)
}

// Input is everything Analyze needs. Current must be the exhaustive
// relationship report of the checked releases and Releases the ingested
// releases of the same run, in the same order (see ingest.IngestChecked).
// Baseline is the previously-held state: a saved `ri check -o json` report
// (optional; nil falls back to the definition's validatedAgainst lists).
type Input struct {
	Definition *catalog.ProductDefinition
	Versions   *ingest.VersionList
	Current    *ingest.RelationshipReport
	Releases   []*domain.Release
	Baseline   *ingest.RelationshipReport
	// BaselinePath records where Baseline was loaded from (display only).
	BaselinePath string
	Registry     ChannelRegistry
	Now          time.Time
}

// Analyze compares the fresh relationship checks of the newest releases with
// the baseline and returns the drift report. It is deterministic: given the
// same inputs (and the same answers from the registry) it returns the same
// report; Registry probes are only consulted through already-declared
// channels.
func Analyze(ctx context.Context, in Input) (*Report, error) {
	if in.Definition == nil {
		return nil, errors.New("drift: nil product definition")
	}
	if in.Current == nil {
		return nil, errors.New("drift: nil current report")
	}
	if len(in.Releases) != len(in.Current.Releases) {
		return nil, fmt.Errorf("drift: %d ingested releases for %d checked releases: pass the releases of the same run", len(in.Releases), len(in.Current.Releases))
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	a := &analyzer{
		def:       in.Definition,
		vl:        in.Versions,
		reg:       in.Registry,
		now:       now,
		order:     append([]string(nil), in.Current.Releases...),
		relBySem:  map[string]*domain.Release{},
		base:      indexOutcomes(in.Baseline),
		validated: validatedAgainst(in.Definition),
		evs:       &domain.EvidenceSet{},
	}
	for _, e := range in.Current.Evidence {
		a.evs.Add(e)
	}
	for _, r := range in.Releases {
		a.relBySem[r.Version.Semver] = r
	}

	rep := &Report{
		Product:          domain.ProductID(in.Definition.ID),
		DefinitionPath:   in.Definition.Path(),
		DefinitionDigest: ingest.DefinitionDigest(in.Definition),
		GeneratedAt:      now.Truncate(time.Second),
		Checked:          append([]string(nil), in.Current.Releases...),
	}
	rep.Baseline = describeBaseline(in)

	a.compareChecks(ctx, in.Current)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.checkAvailability(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	a.checkImages()

	rep.Events = a.events
	rep.Summary = summarizeEvents(rep.Events, len(rep.Checked))
	rep.Evidence = a.evs.List()
	if in.Versions != nil {
		rep.VersionsSources = in.Versions.Sources
	}
	return rep, nil
}

// VersionListFailed builds a report for the case where no versions source
// answered at all: the canonical channel itself broke. Each versions source
// becomes one event — an unreachable host is unverifiable (not drift), a
// reachable source whose tags no longer match the declared convention is a
// relationship-broken drift event (tag convention change). This is the one
// event type that can carry no evidence: upstream answered with a failure,
// and there is nothing retrieved to cite.
func VersionListFailed(def *catalog.ProductDefinition, vl *ingest.VersionList, now time.Time) *Report {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rep := &Report{
		Product:          domain.ProductID(def.ID),
		DefinitionPath:   def.Path(),
		DefinitionDigest: ingest.DefinitionDigest(def),
		GeneratedAt:      now.Truncate(time.Second),
		Baseline:         Baseline{Source: BaselineDefinition, Releases: provenanceReleases(def), Cutoff: BaselineCutoff(def, nil)},
	}
	if vl != nil {
		rep.VersionsSources = vl.Sources
	}
	for _, src := range def.SourcesWithRole(domain.RoleVersions) {
		for _, st := range rep.VersionsSources {
			if st.SourceID != src.ID {
				continue
			}
			ev := Event{
				Subject:     src.ID,
				SubjectKind: SubjectVersions,
				Channel:     st.Kind,
				Coordinate:  st.URI,
				Detail:      st.Detail,
				Releases:    nil, // product-level: the channel itself is broken
			}
			switch st.State {
			case domain.SourceNotFound:
				// The source answered and listed tags, but none matched the
				// declared tag convention: the convention changed.
				ev.Kind, ev.Status, ev.Severity = KindRelationshipBroken, StatusDrift, SeverityHigh
				ev.Explanation = fmt.Sprintf("versions source %s answered but no tag matches the declared convention: the upstream tag convention changed (or the repository moved away)", src.ID)
				ev.Baseline = "canonical releases were previously listed from " + src.ID
				ev.Proposal = &Proposal{Action: "re-research the tag convention (versioning.tagPattern / tagPrefix) and the canonical repository"}
			case domain.SourceUnavailable, domain.SourceThrottled:
				ev.Kind, ev.Status, ev.Severity = KindUnverifiable, StatusUnverifiable, SeverityLow
				ev.Explanation = fmt.Sprintf("versions source %s is unreachable: the canonical channel cannot be checked, so drift cannot be determined", src.ID)
			case domain.SourceOK, domain.SourcePartial, domain.SourceSkipped:
				continue // another source was consulted; not a failure of its own
			default:
				ev.Kind, ev.Status, ev.Severity = KindRelationshipBroken, StatusDrift, SeverityMedium
				ev.Explanation = fmt.Sprintf("versions source %s failed: %s", src.ID, st.State)
			}
			rep.Events = append(rep.Events, ev)
		}
	}
	rep.Summary = summarizeEvents(rep.Events, 0)
	return rep
}

// analyzer carries the state of one Analyze run.
type analyzer struct {
	def       *catalog.ProductDefinition
	vl        *ingest.VersionList
	reg       ChannelRegistry
	now       time.Time
	order     []string // checked releases, ascending (Current.Releases)
	relBySem  map[string]*domain.Release
	events    []Event
	evs       *domain.EvidenceSet
	base      map[string]map[string]string // subject → release → outcome
	validated map[string][]string          // subject → validatedAgainst releases
}

// outcomeRank orders outcomes within one subject+release: the worst wins.
func outcomeRank(o string) int {
	switch o {
	case ingest.OutcomeFail:
		return 4
	case ingest.OutcomePass:
		return 3
	case ingest.OutcomeUnverifiable:
		return 2
	case ingest.OutcomeCovered:
		return 1
	}
	return 0
}

// indexOutcomes reduces a baseline report to subject → release → worst
// outcome (ignoring not-applicable).
func indexOutcomes(rep *ingest.RelationshipReport) map[string]map[string]string {
	if rep == nil {
		return nil
	}
	out := map[string]map[string]string{}
	for _, c := range rep.Checks {
		if c.Outcome == ingest.OutcomeNotApplicable {
			continue
		}
		m, ok := out[c.Subject]
		if !ok {
			m = map[string]string{}
			out[c.Subject] = m
		}
		if prev, seen := m[c.Release]; !seen || outcomeRank(c.Outcome) > outcomeRank(prev) {
			m[c.Release] = c.Outcome
		}
	}
	return out
}

// validatedAgainst maps every subject of the definition (sources, artifacts
// and their contents) to the releases it was validated against.
func validatedAgainst(def *catalog.ProductDefinition) map[string][]string {
	out := map[string][]string{}
	for _, s := range def.Sources {
		if len(s.ValidatedAgainst) > 0 {
			out[s.ID] = s.ValidatedAgainst
		}
	}
	for _, a := range def.Artifacts {
		if len(a.ValidatedAgainst) == 0 {
			continue
		}
		out[a.ID] = a.ValidatedAgainst
		for _, c := range a.Contents {
			out[a.ID+"/"+c.Kind] = a.ValidatedAgainst
		}
	}
	return out
}

// baselineState classifies how a failing subject related to the baseline.
type baselineState struct {
	text     string // what held before, and where it is recorded
	passes   []string
	failed   bool
	neverHad bool
}

func (a *analyzer) baselineOf(subject string) baselineState {
	if m := a.base[subject]; len(m) > 0 {
		var passes, fails, unvers []string
		for rel, o := range m {
			switch o {
			case ingest.OutcomePass:
				passes = append(passes, rel)
			case ingest.OutcomeFail:
				fails = append(fails, rel)
			case ingest.OutcomeUnverifiable:
				unvers = append(unvers, rel)
			}
		}
		sortStrings(passes, fails, unvers)
		st := baselineState{passes: passes}
		switch {
		case len(passes) > 0:
			st.text = fmt.Sprintf("passed on %s (saved check report)", strings.Join(passes, ", "))
		case len(fails) > 0:
			st.failed = true
			st.text = fmt.Sprintf("already failing on %s (saved check report): not a regression", strings.Join(fails, ", "))
		default:
			st.text = fmt.Sprintf("could not be verified on %s (saved check report): hosts unreachable", strings.Join(unvers, ", "))
		}
		return st
	}
	if vs := a.validated[subject]; len(vs) > 0 {
		return baselineState{passes: vs, text: fmt.Sprintf("validatedAgainst %s (definition)", strings.Join(vs, ", "))}
	}
	return baselineState{neverHad: true, text: "no historical baseline for this subject"}
}

// compareChecks turns the fresh checks into events, subject by subject.
func (a *analyzer) compareChecks(ctx context.Context, rep *ingest.RelationshipReport) {
	// fresh per-subject outcomes, in check order (deterministic: it follows
	// the definition's declaration order).
	fresh := map[string]map[string]string{}
	var order []string
	for _, c := range rep.Checks {
		if c.Outcome == ingest.OutcomeNotApplicable || c.Outcome == ingest.OutcomeCovered {
			continue
		}
		m, ok := fresh[c.Subject]
		if !ok {
			m = map[string]string{}
			fresh[c.Subject] = m
			order = append(order, c.Subject)
		}
		if prev, seen := m[c.Release]; !seen || outcomeRank(c.Outcome) > outcomeRank(prev) {
			m[c.Release] = c.Outcome
		}
	}
	for _, subject := range order {
		m := fresh[subject]
		var fails, unvers []ingest.RelationshipCheck
		for _, c := range rep.Checks {
			if c.Subject != subject {
				continue
			}
			switch m[c.Release] {
			case ingest.OutcomeFail:
				if c.Outcome == ingest.OutcomeFail {
					fails = append(fails, c)
				}
			case ingest.OutcomeUnverifiable:
				if c.Outcome == ingest.OutcomeUnverifiable {
					unvers = append(unvers, c)
				}
			}
		}
		if len(fails) > 0 {
			a.addFailureEvent(ctx, subject, fails)
		}
		if len(unvers) > 0 {
			a.addUnverifiableEvent(subject, unvers)
		}
	}
}

// addFailureEvent reports a subject whose declared relationship failed on
// reachable channels of the checked releases.
func (a *analyzer) addFailureEvent(ctx context.Context, subject string, fails []ingest.RelationshipCheck) {
	head := fails[0]
	releases := releasesOf(fails)
	base := a.baselineOf(subject)
	ev := Event{
		Subject:     subject,
		SubjectKind: head.SubjectKind,
		Releases:    releases,
		Channel:     head.Channel,
		Coordinate:  head.Coordinate,
		Baseline:    base.text,
	}
	// kind by subject: a source that vanished, an artifact that is missing,
	// a content capture that broke — unless a more specific probe finds the
	// artifact on another declared channel (a move).
	defArt := art(a.def, subject)
	switch head.SubjectKind {
	case ingest.SubjectArtifact:
		if defArt == nil || defArt.Optional {
			// optional artifacts never fail (absence is not-applicable);
			// reaching here means an unknown or unexpected subject: say so.
			ev.Kind = KindRelationshipBroken
		} else {
			ev.Kind = KindArtifactMissing
		}
	case ingest.SubjectSource:
		ev.Kind = KindSourceMoved
	default:
		ev.Kind = KindRelationshipBroken
	}
	status, severity := StatusDrift, SeverityHigh
	switch {
	case base.failed:
		status = StatusNote
		severity = SeverityLow
	case base.neverHad:
		severity = SeverityMedium
	case len(a.base[subject]) > 0 && len(base.passes) == 0 && !base.failed:
		// unverifiable at baseline, failing now: reachable for the first
		// time and absent — weaker history than a pass.
		severity = SeverityMedium
	}
	ev.Status, ev.Severity = status, severity

	if ev.Kind == KindArtifactMissing {
		var moved *foundAt
		if defArt != nil && a.reg != nil {
			for _, rel := range releases {
				if r, ok := a.relBySem[rel]; ok {
					moved = a.findArtifactAt(ctx, defArt, r.Version)
				}
			}
		}
		if moved != nil {
			ev.Kind = KindSourceMoved
			ev.Coordinate = moved.coordinate
			ev.Detail = joinDetail(fails) + "; present at " + moved.where
			ev.Explanation = fmt.Sprintf("artifact %s is absent from its leading declared channel for %s but present at %s: the publication moved between channels the definition already declares",
				subject, strings.Join(releases, ", "), moved.where)
			ev.Evidence = append(evidenceOf(fails), moved.evidence.ID)
			ev.Proposal = reorderChannelsProposal(defArt, moved.channelIndex)
			a.events = append(a.events, ev)
			return
		}
		ev.Explanation = fmt.Sprintf("declared artifact %s is absent from every reachable channel for %s (the channel answered: upstream no longer publishes it there)",
			subject, strings.Join(releases, ", "))
		ev.Detail = joinDetail(fails)
		ev.Evidence = evidenceOf(fails)
		ev.Proposal = a.missingArtifactProposal(defArt, releases, base)
		a.events = append(a.events, ev)
		return
	}
	if ev.Kind == KindSourceMoved {
		ev.Explanation = fmt.Sprintf("source %s did not resolve for %s: the location is reachable but returned nothing (the document may have moved; drift never guesses new hosts)",
			subject, strings.Join(releases, ", "))
		ev.Detail = joinDetail(fails)
		ev.Evidence = evidenceOf(fails)
		ev.Proposal = sourceExceptionsProposal(subject, releases)
		a.events = append(a.events, ev)
		return
	}
	ev.Explanation = fmt.Sprintf("declared %s relationship %q fails for %s on reachable channels", head.SubjectKind, subject, strings.Join(releases, ", "))
	ev.Detail = joinDetail(fails)
	ev.Evidence = evidenceOf(fails)
	a.events = append(a.events, ev)
}

// addUnverifiableEvent reports a subject that could not be checked: every
// channel was unreachable. This is explicitly NOT drift.
func (a *analyzer) addUnverifiableEvent(subject string, checks []ingest.RelationshipCheck) {
	head := checks[0]
	releases := releasesOf(checks)
	base := a.baselineOf(subject)
	ev := Event{
		Kind:        KindUnverifiable,
		Status:      StatusUnverifiable,
		Severity:    SeverityLow,
		Subject:     subject,
		SubjectKind: head.SubjectKind,
		Releases:    releases,
		Channel:     head.Channel,
		Coordinate:  head.Coordinate,
		Explanation: fmt.Sprintf("cannot determine whether %s still holds for %s: a declared channel could not be reached, and absence needs every channel to answer", subject, strings.Join(releases, ", ")),
		Detail:      joinDetail(checks),
		Baseline:    base.text,
		Evidence:    evidenceOf(checks),
	}
	a.events = append(a.events, ev)
}

// checkAvailability probes declared artifacts whose availability constraint
// excludes a checked release: if the artifact exists anyway, the window in
// the definition is stale (the retirement never happened, or the artifact
// started earlier than declared). Only positive observations are reported —
// an unreachable channel says nothing. One event per artifact, listing every
// violating release and citing the newest observation.
func (a *analyzer) checkAvailability(ctx context.Context) {
	if a.reg == nil {
		return
	}
	for i := range a.def.Artifacts {
		art := &a.def.Artifacts[i]
		if art.Availability == "" {
			continue
		}
		var found *foundAt
		var newest domain.Version
		var releases []string
		for _, rel := range a.checkedReleases() {
			v := rel.Version
			ok, err := catalog.AppliesTo(v, art.Availability, nil)
			if err != nil || ok {
				continue
			}
			if _, except := catalog.ExceptionFor(v, art.Exceptions); except {
				continue
			}
			f := a.findArtifactAt(ctx, art, v)
			if f == nil {
				continue
			}
			a.evs.Add(f.evidence)
			found, newest = f, v // checkedReleases is ascending: the last wins
			releases = append(releases, v.Semver)
		}
		if found == nil {
			continue
		}
		ev := Event{
			Kind:        KindAvailabilityViolated,
			Status:      StatusDrift,
			Severity:    SeverityLow,
			Subject:     art.ID,
			SubjectKind: SubjectArtifact,
			Releases:    releases,
			Channel:     found.channel,
			Coordinate:  found.coordinate,
			Explanation: fmt.Sprintf("artifact %s is declared with availability %q (not applicable to %s) but exists there: %s",
				art.ID, art.Availability, strings.Join(releases, ", "), found.coordinate),
			Baseline: fmt.Sprintf("availability %q curated in the definition", art.Availability),
			Evidence: []domain.EvidenceID{found.evidence.ID},
			Proposal: availabilityProposal(art, newest),
		}
		a.events = append(a.events, ev)
		if err := ctx.Err(); err != nil {
			return
		}
	}
}

// checkImages looks for image references carrying the release tag inside the
// snapshots of declared artifacts (image-refs content): an image repository
// the definition does not declare at all is a new artifact discoverable
// through channels already declared.
func (a *analyzer) checkImages() {
	type seen struct {
		event  *Event
		tagFor string // "{{.Tag}}" or "{{.Version}}"
	}
	byRepo := map[string]*seen{}
	var order []string
	for _, rel := range a.checkedReleases() {
		rc := a.renderContext(rel.Version)
		declared := a.declaredImageRepos(rc)
		for i := range rel.Snapshots {
			snap := &rel.Snapshots[i]
			if snap.Kind != domain.SnapshotImages || snap.Images == nil {
				continue
			}
			for _, img := range snap.Images.Images {
				if img.Tag == "" || declared[img.Repository] {
					continue
				}
				tmpl := ""
				switch img.Tag {
				case rel.Version.Tag:
					tmpl = "{{.Tag}}"
				case rel.Version.Semver:
					tmpl = "{{.Version}}"
				default:
					continue // not a release-tagged image: a dependency, not a product artifact
				}
				s, ok := byRepo[img.Repository]
				if !ok {
					s = &seen{event: &Event{
						Kind:        KindArtifactAppeared,
						Status:      StatusDrift,
						Severity:    SeverityLow,
						Subject:     img.Repository,
						SubjectKind: SubjectImage,
						Releases:    []string{rel.Version.Semver},
						Explanation: fmt.Sprintf("image %s:%s is referenced by the snapshot of %s but no artifact is declared for that repository",
							img.Repository, img.Tag, snap.ArtifactID),
						Baseline: "not declared in the definition",
					}, tagFor: tmpl}
					byRepo[img.Repository] = s
					order = append(order, img.Repository)
				} else {
					s.event.Releases = appendUnique(s.event.Releases, rel.Version.Semver)
					if s.tagFor != tmpl {
						s.tagFor = "" // inconsistent tagging: no deterministic template
					}
				}
				s.event.Evidence = appendUniqueIDs(s.event.Evidence, snap.Evidence)
				s.event.Detail = fmt.Sprintf("referenced by %s (snapshot evidence)", snap.ArtifactID)
			}
		}
	}
	for _, repo := range order {
		s := byRepo[repo]
		if s.tagFor != "" {
			s.event.Proposal = appearedImageProposal(repo, s.tagFor)
		}
		a.events = append(a.events, *s.event)
	}
}

// declaredImageRepos renders every oci channel of the definition for rc.
func (a *analyzer) declaredImageRepos(rc catalog.RenderContext) map[string]bool {
	out := map[string]bool{}
	for _, art := range a.def.Artifacts {
		for _, ch := range art.Channels {
			if ch.Kind != catalog.LocatorOCI {
				continue
			}
			loc, err := catalog.RenderLocator(ch, rc)
			if err != nil {
				continue
			}
			out[strings.TrimPrefix(loc.Repository, "oci://")] = true
		}
	}
	return out
}

func (a *analyzer) renderContext(v domain.Version) catalog.RenderContext {
	var known []domain.Version
	if a.vl != nil {
		known = a.vl.Versions
	}
	return catalog.NewRenderContext(a.def.ID, v, known)
}

func (a *analyzer) checkedReleases() []*domain.Release {
	var out []*domain.Release
	for _, sem := range a.order {
		if r, ok := a.relBySem[sem]; ok {
			out = append(out, r)
		}
	}
	return out
}

// --- helpers -----------------------------------------------------------------

func art(def *catalog.ProductDefinition, id string) *catalog.Artifact {
	for i := range def.Artifacts {
		if def.Artifacts[i].ID == id {
			return &def.Artifacts[i]
		}
	}
	return nil
}

func releasesOf(checks []ingest.RelationshipCheck) []string {
	var out []string
	for _, c := range checks {
		out = appendUnique(out, c.Release)
	}
	return out
}

func evidenceOf(checks []ingest.RelationshipCheck) []domain.EvidenceID {
	var out []domain.EvidenceID
	for _, c := range checks {
		out = appendUniqueIDs(out, c.Evidence)
	}
	return out
}

func joinDetail(checks []ingest.RelationshipCheck) string {
	var parts []string
	for _, c := range checks {
		if c.Detail == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", c.Release, c.Detail))
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts, "; ")
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

func appendUniqueIDs(list []domain.EvidenceID, ids []domain.EvidenceID) []domain.EvidenceID {
	for _, id := range ids {
		if id == "" {
			continue
		}
		found := false
		for _, x := range list {
			if x == id {
				found = true
				break
			}
		}
		if !found {
			list = append(list, id)
		}
	}
	return list
}

func sortStrings(lists ...[]string) {
	for _, l := range lists {
		for i := 1; i < len(l); i++ {
			for j := i; j > 0 && l[j] < l[j-1]; j-- {
				l[j], l[j-1] = l[j-1], l[j]
			}
		}
	}
}
