package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// ChangeProducer identifies the change collector in provenance.
const ChangeProducer = "bundle.change@v1"

const (
	maxDocsPerSource = 25      // documents fetched per change source kind
	maxDocBytes      = 8 << 20 // bound of one fetched upstream document
)

// Upstream is what the change collector needs from the application: the
// upgrade edge, the release-scope (chart-default) render of an edge, and
// document retrieval through the fetch cache. *app.App satisfies it through
// NewChangeCollector; tests use fakes.
type Upstream interface {
	Upgrade(ctx context.Context, product, from, to string) (*domain.UpgradeEdge, error)
	// RenderRelease renders the chart defaults of both releases and diffs
	// them. A nil pair means the render could not be attempted.
	RenderRelease(ctx context.Context, edge *domain.UpgradeEdge, kubeVersion string) (*render.Pair, error)
	// Fetch returns the bytes and media type of an upstream document.
	Fetch(ctx context.Context, url string) (data []byte, mediaType string, err error)
}

type appUpstream struct{ a *app.App }

func (u appUpstream) Upgrade(ctx context.Context, product, from, to string) (*domain.UpgradeEdge, error) {
	return u.a.Upgrade(ctx, product, from, to, app.UpgradeOptions{})
}

func (u appUpstream) RenderRelease(ctx context.Context, edge *domain.UpgradeEdge, kubeVersion string) (*render.Pair, error) {
	res, err := u.a.RenderDiffEdge(ctx, edge, app.RenderOptions{Release: true, KubeVersion: kubeVersion})
	if err != nil {
		return nil, err
	}
	return res.Release, nil
}

func (u appUpstream) Fetch(ctx context.Context, rawURL string) ([]byte, string, error) {
	doc, err := fetch.Get(ctx, u.a.Fetch, rawURL, false)
	if err != nil {
		return nil, "", err
	}
	if len(doc.Body) > maxDocBytes {
		return nil, "", fmt.Errorf("%d bytes exceeds the %d byte bound", len(doc.Body), maxDocBytes)
	}
	mt := ""
	if ct := doc.Header.Get("Content-Type"); ct != "" {
		mt, _, _ = mime.ParseMediaType(ct)
	}
	return doc.Body, mt, nil
}

// ChangeCollector collects the change evidence of one upgrade (MISSION §A1).
type ChangeCollector struct {
	up  Upstream
	st  investigate.ArtifactStore
	now func() time.Time
}

var _ investigate.ChangeCollector = (*ChangeCollector)(nil)

// NewChangeCollector returns the collector over the application's Upgrade and
// release-scope RenderDiffEdge, writing artifacts to st.
func NewChangeCollector(a *app.App, st investigate.ArtifactStore) investigate.ChangeCollector {
	return &ChangeCollector{up: appUpstream{a}, st: st, now: time.Now}
}

// NewChangeCollectorOver is NewChangeCollector over any Upstream.
func NewChangeCollectorOver(up Upstream, st investigate.ArtifactStore) investigate.ChangeCollector {
	return &ChangeCollector{up: up, st: st, now: time.Now}
}

// CollectChange builds the ChangeEvidenceRecord. A source it cannot collect is
// recorded with a non-ok status, never an error.
func (c *ChangeCollector) CollectChange(ctx context.Context, req investigate.ChangeRequest) (*investigate.ChangeResult, error) {
	edge, err := c.up.Upgrade(ctx, req.Product, req.From, req.To)
	if err != nil {
		return nil, fmt.Errorf("change: upgrade %s %s → %s: %w", req.Product, req.From, req.To, err)
	}
	res := &investigate.ChangeResult{}
	pool := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range edge.Evidence {
		pool[e.ID] = e
	}
	put := func(spec investigate.ArtifactSpec, data []byte) (domain.ArtifactRef, error) {
		ref, err := c.st.Put(ctx, spec, data)
		if err == nil {
			res.Artifacts = append(res.Artifacts, ref)
		}
		return ref, err
	}

	// 1. The edge, stored once; items reference it by id. The stored copy has
	// its wall-clock fields zeroed so the artifact — and therefore the
	// bundle — is reproducible from the same upstream evidence.
	edgeRef, err := put(investigate.ArtifactSpec{
		Kind: domain.ArtifactUpgradeEdge, Path: "derived/change/upgrade-edge.json", Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:" + ChangeProducer, Producer: ChangeProducer,
		Tags: []string{"upgrade-edge", string(edge.Product.ID), edge.From.Tag, edge.To.Tag},
	}, normalizedEdgeJSON(edge))
	if err != nil {
		return nil, fmt.Errorf("change: %w", err)
	}

	// 2. Items from the edge (phase A); render correlation follows (phase B).
	b := &itemBuilder{edge: edge, pool: pool}
	b.fromEdge()

	var pair *render.Pair
	var renderErr string
	if req.Render {
		p, rerr := c.up.RenderRelease(ctx, edge, "")
		pair = p
		if rerr != nil {
			renderErr = rerr.Error()
		}
	}
	var mp *domain.ManifestPair
	if req.Render {
		rp, err := c.releasePair(ctx, put, pool, b, pair, renderErr)
		if err != nil {
			return nil, err
		}
		mp = rp
	}
	b.finish() // manifestation / scope need the correlated paths

	// 3. Anchored items for unexplained structural release changes (phase C).
	if mp != nil && mp.Status == domain.CollectionOK {
		b.anchorStructural(mp, pair)
	}
	items := b.items()

	// 4. Source collection status and documents.
	sources, err := c.sources(ctx, put, edge, items, mp, renderErr, req.Render)
	if err != nil {
		return nil, err
	}

	rec := domain.ChangeEvidenceRecord{
		Product: edge.Product, From: edge.From, To: edge.To, Edge: edgeRef.ID,
		Items: items, MigrationNotes: migrationNotes(edge), Sources: sources, Producer: ChangeProducer,
	}
	if mp != nil {
		rec.Renders = []domain.ManifestPair{*mp}
	}
	rec.ID = rec.DerivedID()
	if err := rec.Validate(); err != nil {
		return nil, fmt.Errorf("change: record does not validate: %w", err)
	}

	res.Record = rec
	for _, e := range pool {
		res.Evidence = append(res.Evidence, e)
	}
	sort.Slice(res.Evidence, func(i, j int) bool { return res.Evidence[i].ID < res.Evidence[j].ID })
	sort.Slice(res.Artifacts, func(i, j int) bool { return res.Artifacts[i].Path < res.Artifacts[j].Path })
	res.Artifacts = dedupeRefs(res.Artifacts)
	return res, nil
}

// normalizedEdgeJSON serialises the edge without wall-clock fields.
func normalizedEdgeJSON(e *domain.UpgradeEdge) []byte {
	cp := *e
	cp.GeneratedAt = time.Time{}
	cp.Evidence = append([]domain.Evidence(nil), e.Evidence...)
	for i := range cp.Evidence {
		cp.Evidence[i].RetrievedAt = time.Time{}
	}
	b, _ := json.MarshalIndent(&cp, "", "  ")
	return append(b, '\n')
}

func migrationNotes(edge *domain.UpgradeEdge) []domain.EvidenceID {
	roles := sourceRoles(edge)
	var out []domain.EvidenceID
	for _, e := range edge.Evidence {
		if hasRole(roles[e.SourceID], domain.RoleUpgradeGuide) {
			out = append(out, e.ID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sourceRoles(edge *domain.UpgradeEdge) map[string][]domain.SourceRole {
	m := map[string][]domain.SourceRole{}
	for _, s := range edge.Sources {
		m[s.SourceID] = append(m[s.SourceID], s.Roles...)
	}
	return m
}

func hasRole(rs []domain.SourceRole, want ...domain.SourceRole) bool {
	for _, r := range rs {
		for _, w := range want {
			if r == w {
				return true
			}
		}
	}
	return false
}

// --- items ------------------------------------------------------------------------------

type itemBuilder struct {
	edge *domain.UpgradeEdge
	pool map[domain.EvidenceID]domain.Evidence

	byChange map[string]*domain.ChangeItem // change id → item
	facts    map[string]changeFacts        // item id → facts
	list     []*domain.ChangeItem
	// corr: change id → paths of correlated release-scope rendered changes
	// (rule subject, values-default or artifact), for manifest:rendered-correlated.
	corr map[string][]string
}

func (b *itemBuilder) fromEdge() {
	b.byChange = map[string]*domain.ChangeItem{}
	b.facts = map[string]changeFacts{}
	b.corr = map[string][]string{}
	for _, ch := range b.edge.Changes {
		if ch.Routine {
			continue // routine maintenance churn is summarised in the edge, not itemised
		}
		if ch.Provenance.Method == domain.MethodComputed && strings.HasPrefix(ch.Provenance.Rule, "compat:") {
			continue // covered by the platform item of the compatibility change
		}
		it := &domain.ChangeItem{
			ID: domain.ChangeItemID(ch.ID, "", ""), ChangeID: ch.ID, Title: ch.Title,
			Breaking: ch.Breaking, ActionRequired: ch.ActionRequired, Subjects: append([]string(nil), ch.Subjects...),
			Evidence: b.cited(ch.Evidence),
		}
		b.byChange[ch.ID] = it
		b.list = append(b.list, it)
		b.facts[it.ID] = factsOf(ch, nil)
	}
	// Platform items: one per compatibility change whose constraint moved.
	changed := map[string][]domain.CompatibilityChange{}
	for _, cc := range b.edge.Compatibility {
		if !compatMoved(cc) {
			continue
		}
		changed[cc.Platform] = append(changed[cc.Platform], cc)
	}
	var platforms []string
	for p := range changed {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)
	for _, p := range platforms {
		for _, cc := range changed[p] {
			anchor := p
			if len(changed[p]) > 1 {
				anchor = p + "/" + compatKind(cc) // several constraints of one platform (supported, chart-kubeVersion)
			}
			var ev []domain.EvidenceID
			for _, k := range []*domain.CompatibilityConstraint{cc.From, cc.To} {
				if k != nil {
					ev = append(ev, k.Evidence...)
				}
			}
			action := false
			for _, ch := range b.edge.Changes {
				if ch.Category == domain.CategoryCompatibility && ch.ActionRequired && mentionsPlatform(ch, p) {
					action = true
				}
			}
			it := &domain.ChangeItem{
				ID: domain.ChangeItemID("", anchor, ""), Platform: anchor, Title: cc.Summary,
				ActionRequired: action, Subjects: []string{p}, Evidence: b.cited(ev),
			}
			b.list = append(b.list, it)
			b.facts[it.ID] = changeFacts{Category: domain.CategoryCompatibility, ActionRequired: action, Platform: true, Narrowed: cc.Narrowed}
		}
	}
}

func compatMoved(cc domain.CompatibilityChange) bool {
	if cc.From == nil || cc.To == nil {
		return cc.From != nil || cc.To != nil
	}
	return cc.From.Constraint != cc.To.Constraint || cc.From.Raw != cc.To.Raw
}

func compatKind(cc domain.CompatibilityChange) string {
	for _, k := range []*domain.CompatibilityConstraint{cc.To, cc.From} {
		if k != nil && k.Kind != "" {
			return k.Kind
		}
	}
	return "constraint"
}

func mentionsPlatform(ch domain.Change, platform string) bool {
	for _, s := range ch.Subjects {
		if strings.Contains(strings.ToLower(s), strings.ToLower(platform)) {
			return true
		}
	}
	return false
}

// cited keeps the evidence ids that resolve in the pool, sorted.
func (b *itemBuilder) cited(ids []domain.EvidenceID) []domain.EvidenceID {
	var out []domain.EvidenceID
	seen := map[domain.EvidenceID]bool{}
	for _, id := range ids {
		if _, ok := b.pool[id]; ok && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// finish decides family, manifestation and scope of every edge item, now that
// the release correlation is known.
func (b *itemBuilder) finish() {
	for _, it := range b.list {
		f := b.facts[it.ID]
		if it.ChangeID != "" {
			f.CorrelatedPaths = b.corr[it.ChangeID]
		}
		b.facts[it.ID] = f
		it.Family = familyOf(f)
		it.Manifestation, it.ManifestationRule = manifestationOf(f)
		it.InScope, it.ScopeRule = scopeOf(f)
	}
}

// anchorStructural adds an in-scope item for every release-scope rendered
// change no change explains whose class the default tier policy blocks
// (scope:rendered-structural) and points the change at it.
func (b *itemBuilder) anchorStructural(mp *domain.ManifestPair, pair *render.Pair) {
	structural := map[string]bool{}
	if pair != nil && pair.Diff != nil {
		for _, c := range pair.Diff.Changes {
			if isStructuralRender(c) {
				structural[domain.RenderedChangeID(domain.RenderRelease, mp.Target, c.Object.String(), c.Path, string(c.Class))] = true
			}
		}
	}
	evidence := b.diffEvidence(mp)
	for i := range mp.Changes {
		rc := &mp.Changes[i]
		if len(rc.ChangeItems) > 0 || !structural[rc.ID] || len(evidence) == 0 {
			continue
		}
		f := changeFacts{RenderedAnchored: true}
		it := &domain.ChangeItem{
			ID: domain.ChangeItemID("", "", rc.ID), RenderedChange: rc.ID, Family: domain.FamilyRendered,
			Title:    strings.Join(strings.Fields(fmt.Sprintf("%s: %s %s (not mentioned by any change)", rc.Class, rc.Object, rc.Path)), " "),
			Subjects: []string{rc.Object}, Evidence: evidence,
		}
		it.Manifestation, it.ManifestationRule = manifestationOf(f)
		it.InScope, it.ScopeRule = scopeOf(f)
		b.list = append(b.list, it)
		b.facts[it.ID] = f
		rc.ChangeItems, rc.Correlation = []string{it.ID}, "rendered-anchor"
	}
}

func (b *itemBuilder) diffEvidence(mp *domain.ManifestPair) []domain.EvidenceID {
	for _, c := range mp.Changes {
		if len(c.Evidence) > 0 {
			return c.Evidence
		}
	}
	return nil
}

func (b *itemBuilder) items() []domain.ChangeItem {
	out := make([]domain.ChangeItem, 0, len(b.list))
	for _, it := range b.list {
		out = append(out, *it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// --- release-scope render ----------------------------------------------------------------

const releaseTarget = "chart-defaults"

type putFunc func(investigate.ArtifactSpec, []byte) (domain.ArtifactRef, error)

// releasePair converts the chart-default render pair into a ManifestPair:
// renders and diff stored as artifacts, one RenderedChange per semantic change
// with its correlation to the change items. An incomplete render is a pair
// with a non-ok status and the failure reason, never an empty diff.
func (c *ChangeCollector) releasePair(ctx context.Context, put putFunc, pool map[domain.EvidenceID]domain.Evidence, b *itemBuilder, p *render.Pair, renderErr string) (*domain.ManifestPair, error) {
	mp := &domain.ManifestPair{Scope: domain.RenderRelease, Target: releaseTarget}
	switch {
	case p == nil:
		mp.Status, mp.Reason = domain.CollectionUnavailable, "the release-scope render could not be attempted"
		if renderErr != "" {
			mp.Reason += ": " + renderErr
		}
		return mp, nil
	case p.Status != render.PairOK || p.Diff == nil:
		mp.Status = domain.CollectionUnavailable
		if p.Status == render.PairNotApplicable {
			mp.Status = domain.CollectionNotApplicable
		}
		mp.Reason = "release-scope render " + string(p.Status)
		if p.Failure != nil {
			mp.Reason = fmt.Sprintf("%s: %s (%s)", mp.Reason, p.Failure.Reason, p.Failure.Detail)
		}
		return mp, nil
	}
	producer := "render@release"
	store := func(name string, kind domain.ArtifactKind, mt string, data []byte, tags ...string) (domain.ArtifactRef, error) {
		return put(investigate.ArtifactSpec{
			Kind: kind, Path: "derived/render/release/" + releaseTarget + "/" + name, Origin: domain.OriginDerived, MediaType: mt,
			URI: "ri:" + producer, Producer: ChangeProducer, Redacted: kind == domain.ArtifactRenderedManifests,
			Tags: append([]string{"render", "release", releaseTarget}, tags...),
		}, data)
	}
	fromRef, err := store("from.yaml", domain.ArtifactRenderedManifests, "application/yaml", redactRendered(p.FromResult.Output), "from")
	if err != nil {
		return nil, fmt.Errorf("change: %w", err)
	}
	toRef, err := store("to.yaml", domain.ArtifactRenderedManifests, "application/yaml", redactRendered(p.ToResult.Output), "to")
	if err != nil {
		return nil, fmt.Errorf("change: %w", err)
	}
	diffJSON, err := json.MarshalIndent(p.Diff, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("change: %w", err)
	}
	diffRef, err := store("diff.json", domain.ArtifactRenderDiff, "application/json", append(diffJSON, '\n'), "diff")
	if err != nil {
		return nil, fmt.Errorf("change: %w", err)
	}
	mp.From, mp.To, mp.Diff, mp.Status = fromRef.ID, toRef.ID, diffRef.ID, domain.CollectionOK

	ev := domain.NewEvidence(domain.EvidenceRenderedDiff, "", "ri:"+producer+"/"+releaseTarget,
		fmt.Sprintf("%s → %s", p.From, p.To), fmt.Sprintf("%d rendered changes between the chart defaults of %s and %s", len(p.Diff.Changes), p.From, p.To),
		diffRef.Digest, time.Time{})
	pool[ev.ID] = ev

	corr := render.Correlate(p.Diff, b.edge)
	linked := map[string]correlated{}
	for _, cc := range corr.Changes {
		linked[changeKey(cc.Change)] = correlatedOf(cc, b)
	}
	byID := map[string]*domain.RenderedChange{}
	for _, ch := range p.Diff.Changes {
		id := domain.RenderedChangeID(domain.RenderRelease, releaseTarget, ch.Object.String(), ch.Path, string(ch.Class))
		l := linked[changeKey(ch)]
		if existing, dup := byID[id]; dup { // two diff entries with one identity: merge their attributions
			existing.ChangeItems = mergeSorted(existing.ChangeItems, l.items)
			continue
		}
		rc := &domain.RenderedChange{
			ID: id, Class: string(ch.Class), Object: ch.Object.String(), Path: ch.Path, Sensitive: ch.Sensitive,
			ChangeItems: l.items, Correlation: l.rule, Evidence: []domain.EvidenceID{ev.ID},
		}
		if !ch.Sensitive {
			rc.Before, rc.After = deref(ch.Before), deref(ch.After)
		}
		byID[id] = rc
		for _, cid := range l.changeIDs {
			if l.pathRule {
				b.corr[cid] = append(b.corr[cid], ch.Path)
			}
		}
	}
	for _, rc := range byID {
		if len(rc.ChangeItems) == 0 {
			rc.Correlation = ""
		} else if rc.Correlation == "" {
			rc.Correlation = "subject"
		}
		mp.Changes = append(mp.Changes, *rc)
	}
	sort.Slice(mp.Changes, func(i, j int) bool { return mp.Changes[i].ID < mp.Changes[j].ID })
	return mp, nil
}

type correlated struct {
	items     []string
	changeIDs []string
	rule      string
	pathRule  bool // a rule that counts for manifest:rendered-correlated
}

// correlatedOf maps a render correlation to change items. Links to routine
// changes (which have no item) are dropped; an "artifact" link, which names
// no change, attaches to the computed image items of that repository.
func correlatedOf(cc render.Correlated, b *itemBuilder) correlated {
	var out correlated
	for _, l := range cc.Links {
		if l.ChangeID != "" {
			it, ok := b.byChange[l.ChangeID]
			if !ok {
				continue
			}
			out.items = append(out.items, it.ID)
			out.changeIDs = append(out.changeIDs, l.ChangeID)
			if out.rule == "" {
				out.rule = l.Rule
			}
			if l.Rule == "subject" || l.Rule == "values-default" || l.Rule == "artifact" {
				out.pathRule = true
			}
			continue
		}
		if l.Rule != "artifact" {
			continue
		}
		repo := strings.TrimPrefix(l.Title, "artifact version change of ")
		for _, it := range b.list {
			f := b.facts[it.ID]
			if it.ChangeID == "" || !f.Computed || !(strings.HasPrefix(f.Rule, "images:") || strings.HasPrefix(f.Rule, "artifacts:")) {
				continue
			}
			if itemMentions(it, repo) {
				out.items = append(out.items, it.ID)
				out.changeIDs = append(out.changeIDs, it.ChangeID)
				if out.rule == "" {
					out.rule = "artifact"
				}
				out.pathRule = true
			}
		}
	}
	sort.Strings(out.items)
	out.items = uniqStrings(out.items)
	return out
}

func itemMentions(it *domain.ChangeItem, repo string) bool {
	if repo == "" {
		return false
	}
	base := path.Base(repo)
	for _, s := range append([]string{it.Title}, it.Subjects...) {
		if strings.Contains(s, repo) || (len(base) >= 4 && strings.Contains(s, base)) {
			return true
		}
	}
	return false
}

// changeKey identifies a render change within one diff.
func changeKey(c render.Change) string {
	return string(c.Class) + "|" + c.Object.String() + "|" + c.Path + "|" + c.Name
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mergeSorted(a, b []string) []string {
	return uniqStrings(append(append([]string(nil), a...), b...))
}

func uniqStrings(in []string) []string {
	sort.Strings(in)
	out := in[:0:0]
	for i, s := range in {
		if i == 0 || s != in[i-1] {
			out = append(out, s)
		}
	}
	return out
}

// redactRendered withholds Secret values in a rendered stream.
func redactRendered(b []byte) []byte {
	out, _ := redactYAML(b)
	return out
}

// --- sources ----------------------------------------------------------------------------

var unsafePath = regexp.MustCompile(`[^A-Za-z0-9._/-]+`)

// docURL normalises an evidence URI to the URL to fetch: the fragment is
// dropped and a GitHub blob URL becomes its raw-content URL.
func docURL(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", false
	}
	u.Fragment = ""
	if u.Host == "github.com" {
		parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 5)
		if len(parts) == 5 && parts[2] == "blob" {
			return "https://raw.githubusercontent.com/" + parts[0] + "/" + parts[1] + "/" + parts[3] + "/" + parts[4], true
		}
	}
	return u.String(), true
}

func docPath(product, rawURL string) string {
	u, _ := url.Parse(rawURL)
	p := path.Clean("/" + u.Host + u.Path)
	return "upstream/" + product + unsafePath.ReplaceAllString(p, "_")
}

// sourceSpec says which evidence supports one change source kind.
type sourceSpec struct {
	kind     domain.ChangeSourceKind
	artifact domain.ArtifactKind
	label    string
	// roles: evidence of definition sources with one of these roles.
	roles []domain.SourceRole
	// artifactType: evidence of the definition artifact(s) of this type.
	artifactType domain.ArtifactType
	// evidenceKinds restricts the evidence kinds taken.
	evidenceKinds []domain.EvidenceKind
	// always: evidence of these kinds counts whatever its source (advisory
	// evidence carries no definition source id).
	always []domain.EvidenceKind
}

var documentKinds = []domain.EvidenceKind{domain.EvidenceDocument, domain.EvidenceStructured, domain.EvidenceReleaseAsset, domain.EvidenceAdvisory}

var fetchedSources = []sourceSpec{
	{kind: domain.ChangeSourceReleaseNotes, artifact: domain.ArtifactUpstreamDocument, label: "release notes / changelog", roles: []domain.SourceRole{domain.RoleReleaseNotes, domain.RoleChangelog}, evidenceKinds: documentKinds},
	{kind: domain.ChangeSourceMigrationGuide, artifact: domain.ArtifactUpstreamDocument, label: "upgrade guide", roles: []domain.SourceRole{domain.RoleUpgradeGuide}, evidenceKinds: documentKinds},
	{kind: domain.ChangeSourceCompatibility, artifact: domain.ArtifactUpstreamDocument, label: "compatibility matrix", roles: []domain.SourceRole{domain.RoleCompatibility}, evidenceKinds: documentKinds},
	{kind: domain.ChangeSourceAdvisories, artifact: domain.ArtifactUpstreamDocument, label: "security advisories", roles: []domain.SourceRole{domain.RoleSecurity}, evidenceKinds: documentKinds, always: []domain.EvidenceKind{domain.EvidenceAdvisory}},
	{kind: domain.ChangeSourceAPISchema, artifact: domain.ArtifactCRDDefinition, label: "CRD bundle", artifactType: domain.ArtifactCRD, evidenceKinds: documentKinds},
	{kind: domain.ChangeSourceChart, artifact: domain.ArtifactChartValues, label: "Helm chart", artifactType: domain.ArtifactHelmChart, evidenceKinds: documentKinds},
}

func (c *ChangeCollector) sources(ctx context.Context, put putFunc, edge *domain.UpgradeEdge, items []domain.ChangeItem, mp *domain.ManifestPair, renderErr string, renderRequested bool) ([]domain.ChangeSourceStatus, error) {
	var out []domain.ChangeSourceStatus
	roles := sourceRoles(edge)
	itemsByDoc := itemsCitingDocs(edge, items)
	for _, spec := range fetchedSources {
		st, err := c.fetchedSource(ctx, put, edge, spec, roles, itemsByDoc)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	out = append(out,
		domain.ChangeSourceStatus{Source: domain.ChangeSourceSourceDiff, Status: domain.CollectionUnavailable,
			Reason: "no source-diff collector in milestone 1 (DESIGN.md §0 §A1); an agent may request a linked PR as evidence"},
		domain.ChangeSourceStatus{Source: domain.ChangeSourceKustomize, Status: domain.CollectionNotApplicable,
			Reason: "release-side Kustomize bases are out of milestone 1 scope (Helm-delivered add-ons); the environment's overlays are evaluated through the environment renders"},
	)
	rs := domain.ChangeSourceStatus{Source: domain.ChangeSourceRenderedManifests}
	switch {
	case !renderRequested:
		rs.Status, rs.Reason = domain.CollectionUnavailable, "release-scope renders were not requested"
	case mp == nil:
		rs.Status, rs.Reason = domain.CollectionUnavailable, "no release-scope render was produced"
	case mp.Status == domain.CollectionOK:
		rs.Status, rs.Artifacts = domain.CollectionOK, []string{mp.From, mp.To, mp.Diff}
	default:
		rs.Status, rs.Reason = mp.Status, mp.Reason
	}
	out = append(out, rs)
	sort.Slice(out, func(i, j int) bool { return kindRank(out[i].Source) < kindRank(out[j].Source) })
	return out, nil
}

func kindRank(k domain.ChangeSourceKind) int {
	for i, x := range domain.ChangeSourceKinds {
		if x == k {
			return i
		}
	}
	return len(domain.ChangeSourceKinds)
}

// itemsCitingDocs maps a fetched URL to the items whose upstream evidence
// cites it.
func itemsCitingDocs(edge *domain.UpgradeEdge, items []domain.ChangeItem) map[string][]string {
	ev := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range edge.Evidence {
		ev[e.ID] = e
	}
	out := map[string][]string{}
	for _, it := range items {
		for _, id := range it.Evidence {
			if u, ok := docURL(ev[id].URI); ok {
				out[u] = append(out[u], it.ID)
			}
		}
	}
	for u := range out {
		out[u] = uniqStrings(out[u])
	}
	return out
}

func (c *ChangeCollector) fetchedSource(ctx context.Context, put putFunc, edge *domain.UpgradeEdge, spec sourceSpec, roles map[string][]domain.SourceRole, itemsByDoc map[string][]string) (domain.ChangeSourceStatus, error) {
	st := domain.ChangeSourceStatus{Source: spec.kind}

	// Which definition sources / artifacts speak for this kind?
	srcIDs := map[string]bool{}
	var consulted []domain.SourceStatus
	if len(spec.roles) > 0 {
		for _, s := range edge.Sources {
			if hasRole(s.Roles, spec.roles...) {
				srcIDs[s.SourceID] = true
				consulted = append(consulted, s)
			}
		}
	}
	if spec.artifactType != "" {
		declared := false
		for _, a := range edge.Artifacts {
			if a.Type == spec.artifactType {
				declared = true
				srcIDs[a.ArtifactID] = true
			}
		}
		if !declared {
			st.Status, st.Reason = domain.CollectionNotApplicable, "the product publishes no "+spec.label+" artifact"
			return st, nil
		}
	}
	if len(srcIDs) == 0 {
		st.Status, st.Reason = domain.CollectionUnavailable, "the product definition declares no "+spec.label+" source"
		return st, nil
	}

	// Evidence of those sources, by fetchable URL.
	digests := map[string]map[string]bool{}
	for _, e := range edge.Evidence {
		sid := e.SourceID
		if !kindIn(e.Kind, spec.always) && (!(srcIDs[sid] || srcIDs[strings.SplitN(sid, "/", 2)[0]]) || !kindIn(e.Kind, spec.evidenceKinds)) {
			continue
		}
		u, ok := docURL(e.URI)
		if !ok {
			continue
		}
		if digests[u] == nil {
			digests[u] = map[string]bool{}
		}
		if e.ContentDigest != "" {
			digests[u][e.ContentDigest] = true
		}
	}
	var urls []string
	for u := range digests {
		urls = append(urls, u)
	}
	sort.Strings(urls)

	if len(urls) == 0 {
		st.Status = domain.CollectionPartial
		st.Reason = "the " + spec.label + " source was consulted but cites no retrievable document"
		if len(consulted) > 0 && !anyState(consulted, domain.SourceOK, domain.SourcePartial) {
			st.Status, st.Reason = domain.CollectionUnavailable, stateReason(consulted)
		}
		return st, nil
	}

	var problems []string
	if len(urls) > maxDocsPerSource {
		problems = append(problems, fmt.Sprintf("%d documents cited, the first %d collected", len(urls), maxDocsPerSource))
		urls = urls[:maxDocsPerSource]
	}
	for _, u := range urls {
		data, mt, err := c.up.Fetch(ctx, u)
		if err != nil {
			if ctx.Err() != nil {
				return st, ctx.Err()
			}
			problems = append(problems, fmt.Sprintf("%s: %v", u, err))
			continue
		}
		sum := domain.Digest(data)
		if want := digests[u]; len(want) > 0 && !want[sum] {
			problems = append(problems, fmt.Sprintf("%s: the document changed since it was ingested (digest %s)", u, sum))
		}
		if mt == "" {
			mt = mediaTypeOf(u)
		}
		ref, err := put(investigate.ArtifactSpec{
			Kind: spec.artifact, Path: docPath(string(edge.Product.ID), u), Origin: domain.OriginUpstream, MediaType: mt, URI: u,
			Producer: ChangeProducer, Tags: []string{string(spec.kind), string(edge.Product.ID), "upstream"}, ChangeItems: itemsByDoc[u],
		}, data)
		if err != nil {
			return st, fmt.Errorf("change: %w", err)
		}
		st.Artifacts = append(st.Artifacts, ref.ID)
	}
	st.Artifacts = uniqStrings(st.Artifacts)
	switch {
	case len(st.Artifacts) == 0:
		st.Status, st.Reason = domain.CollectionUnavailable, firstN(problems, 3, "no document could be retrieved")
	case len(problems) > 0:
		st.Status, st.Reason = domain.CollectionPartial, firstN(problems, 3, "")
	default:
		st.Status = domain.CollectionOK
	}
	return st, nil
}

func kindIn(k domain.EvidenceKind, ks []domain.EvidenceKind) bool {
	for _, x := range ks {
		if x == k {
			return true
		}
	}
	return false
}

func anyState(ss []domain.SourceStatus, states ...domain.SourceState) bool {
	for _, s := range ss {
		for _, w := range states {
			if s.State == w {
				return true
			}
		}
	}
	return false
}

func stateReason(ss []domain.SourceStatus) string {
	var parts []string
	for _, s := range ss {
		p := fmt.Sprintf("%s: %s", s.SourceID, s.State)
		if s.Detail != "" {
			p += " (" + s.Detail + ")"
		}
		parts = append(parts, p)
	}
	return firstN(uniqStrings(parts), 3, "no source answered")
}
