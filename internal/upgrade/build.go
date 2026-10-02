package upgrade

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// builder accumulates the parts of an edge while Build runs.
type builder struct {
	in   Input
	edge *domain.UpgradeEdge

	from     *domain.Release
	to       *domain.Release
	releases []*domain.Release // path releases, ascending, last == To

	ev        domain.EvidenceSet
	facts     []domain.Fact
	factSeen  map[domain.FactID]bool
	changes   []domain.Change
	changeIDs map[string]int
	warnings  []string
	warnSeen  map[string]bool

	defOrder map[string]int // artifact id → index in the definition
}

func build(in Input) (*domain.UpgradeEdge, error) {
	if in.From == nil || in.To == nil {
		return nil, errors.New("upgrade: Input.From and Input.To are required")
	}
	if in.From.Version.Compare(in.To.Version) >= 0 {
		return nil, fmt.Errorf("upgrade: from version %s must be lower than to version %s", in.From.Version, in.To.Version)
	}
	b := &builder{
		in:        in,
		from:      in.From,
		to:        in.To,
		factSeen:  map[domain.FactID]bool{},
		changeIDs: map[string]int{},
		warnSeen:  map[string]bool{},
		defOrder:  map[string]int{},
	}
	b.edge = &domain.UpgradeEdge{
		SchemaVersion: domain.UpgradeEdgeSchemaVersion,
		Product:       b.productRef(),
		From:          in.From.Version,
		To:            in.To.Version,
		Sources:       []domain.SourceStatus{},
		Changes:       []domain.Change{},
		Evidence:      []domain.Evidence{},
		GeneratedAt:   in.Now.UTC(),
	}
	if in.Definition != nil {
		b.edge.DefinitionDigest = in.Definition.Digest()
		for i, a := range in.Definition.Artifacts {
			b.defOrder[a.ID] = i
		}
	}
	if b.edge.DefinitionDigest == "" {
		b.edge.DefinitionDigest = in.To.DefinitionDigest
	}

	b.assemblePath()
	b.collectEvidenceAndFacts()
	b.collectSources()
	b.gapWarnings()

	b.noteChanges()
	b.valuesChanges()
	b.crdChanges()
	b.artifactChanges()
	b.imageChanges()
	b.compatChanges()
	b.advisoryChanges()
	b.lifecycleChanges()

	b.linkFacts()
	sortChanges(b.changes)
	b.edge.Routine = routineSummary(b.changes)

	b.edge.Changes = b.changes
	b.edge.Facts = b.facts
	b.edge.Evidence = b.ev.List()
	b.edge.Warnings = b.warnings
	if err := b.edge.Validate(); err != nil {
		return nil, fmt.Errorf("upgrade: assembled edge is invalid: %w", err)
	}
	return b.edge, nil
}

func (b *builder) productRef() domain.ProductRef {
	if b.in.Definition != nil {
		return b.in.Definition.Ref()
	}
	id := b.in.To.Product
	if id == "" {
		id = b.in.From.Product
	}
	return domain.ProductRef{ID: id, Name: string(id)}
}

// warnf records a de-duplicated warning.
func (b *builder) warnf(format string, args ...any) {
	w := fmt.Sprintf(format, args...)
	if b.warnSeen[w] {
		return
	}
	b.warnSeen[w] = true
	b.warnings = append(b.warnings, w)
}

// assemblePath orders the ingested path releases and builds PathSteps.
func (b *builder) assemblePath() {
	seen := map[string]bool{}
	var rels []*domain.Release
	for _, r := range b.in.Path {
		if r == nil || seen[r.Version.Semver] || r.Version.Compare(b.from.Version) <= 0 || r.Version.Compare(b.to.Version) > 0 {
			continue
		}
		seen[r.Version.Semver] = true
		rels = append(rels, r)
	}
	if !seen[b.to.Version.Semver] {
		rels = append(rels, b.to)
	}
	sort.SliceStable(rels, func(i, j int) bool { return rels[i].Version.Less(rels[j].Version) })
	b.releases = rels

	byVersion := map[string]*domain.Release{}
	for _, r := range rels {
		byVersion[r.Version.Semver] = r
	}

	var versions []domain.Version
	policy := "unspecified"
	if sel := b.in.Selection; sel != nil {
		policy = sel.Policy
		versions = append(versions, sel.Path...)
		b.edge.SkippedReleases = append([]domain.Version(nil), sel.Skipped...)
		for _, v := range sel.Path {
			if byVersion[v.Semver] == nil {
				b.warnf("Release %s is on the upgrade path but was not ingested; its release notes are not included", v)
			}
		}
		inSel := map[string]bool{}
		for _, v := range sel.Path {
			inSel[v.Semver] = true
		}
		for _, r := range rels {
			if !inSel[r.Version.Semver] {
				versions = append(versions, r.Version)
			}
		}
		domain.SortVersions(versions)
	} else {
		if b.in.Definition != nil {
			policy = PolicyForLineage(b.in.Definition.Versioning.Lineage)
		}
		for _, r := range rels {
			versions = append(versions, r.Version)
		}
	}
	b.edge.PathPolicy = policy
	reasons := stepReasons(policy, b.from.Version, versions)
	b.edge.Path = make([]domain.PathStep, 0, len(versions))
	for i, v := range versions {
		step := domain.PathStep{Version: v, Reason: reasons[i]}
		if r := byVersion[v.Semver]; r != nil {
			step.Version = r.Version
			step.PublishedAt = r.PublishedAt
		}
		b.edge.Path = append(b.edge.Path, step)
	}
}

// collectEvidenceAndFacts pools evidence and facts of From and the path releases.
func (b *builder) collectEvidenceAndFacts() {
	all := append([]*domain.Release{b.from}, b.releases...)
	for _, r := range all {
		b.ev.AddAll(r.Evidence)
	}
	dropped := 0
	for _, r := range all {
		for _, f := range r.Facts {
			if !b.addFact(f) {
				dropped++
			}
		}
	}
	if dropped > 0 {
		b.warnf("%s dropped because their evidence was not part of the ingested releases", plural(dropped, "fact was", "facts were"))
	}
}

// addFact adds a fact whose evidence resolves; it reports false otherwise.
func (b *builder) addFact(f domain.Fact) bool {
	if b.factSeen[f.ID] {
		return true
	}
	for _, id := range f.Evidence {
		if !b.ev.Has(id) {
			return false
		}
	}
	b.factSeen[f.ID] = true
	b.facts = append(b.facts, f)
	return true
}

// resolve keeps the evidence ids present in the pool (de-duplicated, ordered).
func (b *builder) resolve(ids ...domain.EvidenceID) []domain.EvidenceID {
	var out []domain.EvidenceID
	for _, id := range ids {
		if b.ev.Has(id) {
			out = appendUniqueIDs(out, id)
		}
	}
	return out
}

// addChange assigns a deterministic id and appends c. Changes without
// resolvable evidence are not added; a warning is recorded instead.
func (b *builder) addChange(c domain.Change, idParts ...string) bool {
	c.Evidence = b.resolve(c.Evidence...)
	if len(c.Evidence) == 0 {
		b.warnf("Not reported as a change because no evidence is available: %s", c.Title)
		return false
	}
	if c.Category == "" {
		c.Category = domain.CategoryOther
	}
	c.Title = shorten(c.Title, MaxTitle)
	id := changeID(idParts...)
	if n := b.changeIDs[id]; n > 0 {
		b.changeIDs[id] = n + 1
		id = fmt.Sprintf("%s-%d", id, n+1)
	} else {
		b.changeIDs[id] = 1
	}
	c.ID = id
	b.changes = append(b.changes, c)
	return true
}

// collectSources merges product-level and per-release source statuses.
func (b *builder) collectSources() {
	seen := map[string]bool{}
	add := func(ss []domain.SourceStatus) {
		for _, s := range ss {
			roles := make([]string, len(s.Roles))
			for i, r := range s.Roles {
				roles[i] = string(r)
			}
			key := strings.Join([]string{s.SourceID, s.Kind, strings.Join(roles, ","), s.Version, string(s.State), s.Detail, s.URI}, "\x00")
			if seen[key] {
				continue
			}
			seen[key] = true
			b.edge.Sources = append(b.edge.Sources, s)
		}
	}
	add(b.in.ExtraSources)
	add(b.from.Sources)
	for _, r := range b.releases {
		add(r.Sources)
	}
}

func hasRole(s domain.SourceStatus, roles ...domain.SourceRole) bool {
	for _, r := range s.Roles {
		for _, want := range roles {
			if r == want {
				return true
			}
		}
	}
	return false
}

func statusFor(r *domain.Release, s domain.SourceStatus) bool {
	return s.Version == "" || s.Version == r.Version.Semver || s.Version == r.Version.Tag
}

func statusPhrase(s domain.SourceStatus) string {
	p := s.SourceID + ": " + string(s.State)
	if s.Detail != "" {
		p += " — " + s.Detail
	}
	return p
}

// unretrievedSources returns the sources of the given role that failed to
// answer for release r (unavailable, error, not found). It returns nil as
// soon as one source of that role answered: alternatives, for example the
// members of a fallback group such as a docs page on master and on an
// archive tag, cover each other, as they do for release notes.
func unretrievedSources(r *domain.Release, role domain.SourceRole) []domain.SourceStatus {
	var failed []domain.SourceStatus
	for _, s := range r.Sources {
		if !statusFor(r, s) || !hasRole(s, role) {
			continue
		}
		switch s.State {
		case domain.SourceOK, domain.SourcePartial:
			return nil
		case domain.SourceUnavailable, domain.SourceError, domain.SourceNotFound:
			failed = append(failed, s)
		}
	}
	return failed
}

// gapWarnings reports missing release notes, upgrade guides, compatibility
// data and advisories.
func (b *builder) gapWarnings() {
	for _, r := range b.releases {
		var notes []domain.SourceStatus
		ok := false
		for _, s := range r.Sources {
			if !statusFor(r, s) || !hasRole(s, domain.RoleReleaseNotes, domain.RoleChangelog) {
				continue
			}
			notes = append(notes, s)
			if s.State == domain.SourceOK || s.State == domain.SourcePartial {
				ok = true
			}
		}
		switch {
		case ok:
		case len(notes) == 0:
			b.warnf("No release-notes source was consulted for %s; changes introduced by %s may be missing", r.Version, r.Version)
		default:
			var why []string
			for _, s := range notes {
				why = append(why, statusPhrase(s))
			}
			b.warnf("Release notes for %s were not retrieved (%s); changes introduced by %s may be missing", r.Version, strings.Join(why, "; "), r.Version)
		}
		for _, s := range unretrievedSources(r, domain.RoleUpgradeGuide) {
			b.warnf("Upgrade guide for %s was not retrieved (%s)", r.Version, statusPhrase(s))
		}
	}
	for _, r := range []*domain.Release{b.from, b.to} {
		for _, s := range unretrievedSources(r, domain.RoleCompatibility) {
			b.warnf("Compatibility data for %s was not retrieved (%s)", r.Version, statusPhrase(s))
		}
	}
	for _, s := range b.in.ExtraSources {
		if !hasRole(s, domain.RoleSecurity) {
			continue
		}
		switch s.State {
		case domain.SourceOK, domain.SourceSkipped:
		default:
			b.warnf("Security advisories were not fully retrieved (%s); advisory matching may be incomplete", statusPhrase(s))
		}
	}
}

// linkFacts attaches facts that share evidence with a change.
func (b *builder) linkFacts() {
	byEv := map[domain.EvidenceID][]domain.FactID{}
	for _, f := range b.facts {
		for _, id := range f.Evidence {
			byEv[id] = append(byEv[id], f.ID)
		}
	}
	order := map[domain.FactID]int{}
	for i, f := range b.facts {
		order[f.ID] = i
	}
	for i := range b.changes {
		c := &b.changes[i]
		set := map[domain.FactID]bool{}
		for _, id := range c.Facts {
			if _, ok := order[id]; ok {
				set[id] = true
			}
		}
		for _, id := range c.Evidence {
			for _, f := range byEv[id] {
				set[f] = true
			}
		}
		if len(set) == 0 {
			c.Facts = nil
			continue
		}
		ids := make([]domain.FactID, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(x, y int) bool { return order[ids[x]] < order[ids[y]] })
		c.Facts = ids
	}
}

var categoryIndex = func() map[domain.Category]int {
	m := map[domain.Category]int{}
	for i, c := range domain.AllCategories {
		m[c] = i
	}
	return m
}()

func catIdx(c domain.Category) int {
	if i, ok := categoryIndex[c]; ok {
		return i
	}
	return len(domain.AllCategories)
}

// sortChanges orders changes: breaking first, then action-required, then by
// category display order, release, title and id.
func sortChanges(cs []domain.Change) {
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.Breaking != b.Breaking {
			return a.Breaking
		}
		if a.ActionRequired != b.ActionRequired {
			return a.ActionRequired
		}
		if ci, cj := catIdx(a.Category), catIdx(b.Category); ci != cj {
			return ci < cj
		}
		if c := compareReleaseStrings(a.Release, b.Release); c != 0 {
			return c < 0
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.ID < b.ID
	})
}

// snapshotsByArtifact returns the first snapshot of kind per artifact id.
func snapshotsByArtifact(r *domain.Release, kind domain.SnapshotKind) map[string]*domain.Snapshot {
	out := map[string]*domain.Snapshot{}
	for i := range r.Snapshots {
		s := &r.Snapshots[i]
		if s.Kind != kind {
			continue
		}
		if _, ok := out[s.ArtifactID]; !ok {
			out[s.ArtifactID] = s
		}
	}
	return out
}

// artifactOrder sorts artifact ids by definition order, then lexically.
func (b *builder) artifactOrder(ids []string) {
	sort.SliceStable(ids, func(i, j int) bool {
		oi, iok := b.defOrder[ids[i]]
		oj, jok := b.defOrder[ids[j]]
		switch {
		case iok && jok && oi != oj:
			return oi < oj
		case iok != jok:
			return iok
		}
		return ids[i] < ids[j]
	})
}

// snapshotPairs returns artifact ids having a snapshot of kind on both sides
// (ordered) and warns about one-sided snapshots.
func (b *builder) snapshotPairs(kind domain.SnapshotKind, label string, ok func(*domain.Snapshot) bool) (ids []string, from, to map[string]*domain.Snapshot) {
	from = snapshotsByArtifact(b.from, kind)
	to = snapshotsByArtifact(b.to, kind)
	union := map[string]bool{}
	for id := range from {
		union[id] = true
	}
	for id := range to {
		union[id] = true
	}
	all := sortedKeys(union)
	b.artifactOrder(all)
	for _, id := range all {
		f, t := from[id], to[id]
		fo, to2 := f != nil && ok(f), t != nil && ok(t)
		switch {
		case fo && to2:
			ids = append(ids, id)
		case fo:
			b.warnf("%s of %s were not captured for %s; %s diff skipped", label, id, b.to.Version, strings.ToLower(label))
		case to2:
			b.warnf("%s of %s were not captured for %s; %s diff skipped", label, id, b.from.Version, strings.ToLower(label))
		}
	}
	return ids, from, to
}
