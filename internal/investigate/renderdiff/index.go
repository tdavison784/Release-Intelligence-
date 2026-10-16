package renderdiff

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// DiffSchemaVersion is the version of the render-diff artifact document.
const DiffSchemaVersion = "ri.dev/render-diff/v1alpha1"

// RuleReleaseAnchor is the correlation rule of a rendered change attributed
// to the change item anchored on the matching release-scope rendered change
// (the same chart change, seen with the environment's configuration).
const RuleReleaseAnchor = "release-anchor"

// DiffDocument is the render-diff artifact: the full semantic diff of one
// pair with its provenance and, per change, the id, correlation links and
// change items. Rendered changes cite their entry by "changes[id=<rc>]".
type DiffDocument struct {
	SchemaVersion string                  `json:"schemaVersion"`
	Product       string                  `json:"product"`
	From          string                  `json:"from"`
	To            string                  `json:"to"`
	Scope         domain.RenderScope      `json:"scope"`
	Target        string                  `json:"target"`
	TargetKind    render.TargetKind       `json:"targetKind"`
	Why           string                  `json:"why,omitempty"`
	ReleaseName   string                  `json:"releaseName,omitempty"`
	Namespace     string                  `json:"namespace,omitempty"`
	NamesAssumed  bool                    `json:"namesAssumed,omitempty"`
	Status        domain.CollectionStatus `json:"status"`
	Reason        string                  `json:"reason,omitempty"`
	// Indexed says whether the changes are indexed as RenderedChanges: only
	// a complete (ok) pair's are. The changes of an incomplete pair are
	// recorded for inspection and decide nothing.
	Indexed     bool               `json:"indexed"`
	FromRender  *render.Provenance `json:"fromRender,omitempty"`
	ToRender    *render.Provenance `json:"toRender,omitempty"`
	FromObjects int                `json:"fromObjects"`
	ToObjects   int                `json:"toObjects"`
	Suppressed  map[string]int     `json:"suppressed,omitempty"`
	Changes     []DiffEntry        `json:"changes"`
}

// DiffEntry is one change of a DiffDocument.
type DiffEntry struct {
	ID string `json:"id"`
	// Path is the RenderedChange path: the render path, plus a member
	// selector for set-valued paths (see changePath).
	Path string `json:"path,omitempty"`
	render.Change
	Links       []render.ChangeLink `json:"links,omitempty"`
	ChangeItems []string            `json:"changeItems,omitempty"`
	Correlation string              `json:"correlation,omitempty"`
}

// MarshalJSON keeps the entry's Path (the RenderedChange path) over the
// embedded render path, which is kept as renderPath.
func (e DiffEntry) MarshalJSON() ([]byte, error) {
	type change render.Change
	return json.Marshal(struct {
		ID         string `json:"id"`
		Path       string `json:"path,omitempty"`
		RenderPath string `json:"renderPath,omitempty"`
		change
		Links       []render.ChangeLink `json:"links,omitempty"`
		ChangeItems []string            `json:"changeItems,omitempty"`
		Correlation string              `json:"correlation,omitempty"`
	}{e.ID, e.Path, e.Change.Path, change(e.Change), e.Links, e.ChangeItems, e.Correlation})
}

// indexer builds the pairs of one RenderEnvironment call.
type indexer struct {
	record  domain.ChangeEvidenceRecord
	edge    *domain.UpgradeEdge
	rel     func(string) string
	items   map[string]bool
	anchors map[string][]string // anchor key → items anchored on that release-scope rendered change
	images  []domain.ChangeItem // computed image/artifact items (targets of "artifact" links)
}

// ManifestComputedImage is the manifestation rule of computed image and
// artifact items (DESIGN §2.2.1; lane bundle's collector assigns it).
const ManifestComputedImage = "manifest:computed-image"

func newIndexer(rec domain.ChangeEvidenceRecord, edge *domain.UpgradeEdge, rel func(string) string) *indexer {
	ix := &indexer{record: rec, edge: edge, rel: rel, items: map[string]bool{}, anchors: map[string][]string{}}
	byAnchor := map[string][]string{}
	for _, it := range rec.Items {
		ix.items[it.ID] = true
		if it.ChangeID != "" && it.ManifestationRule == ManifestComputedImage {
			ix.images = append(ix.images, it)
		}
		if it.RenderedChange != "" {
			byAnchor[it.RenderedChange] = append(byAnchor[it.RenderedChange], it.ID)
		}
	}
	release := string(edge.Product.ID)
	for _, p := range rec.Renders {
		for _, c := range p.Changes {
			for _, it := range byAnchor[c.ID] {
				for _, k := range anchorKeys(c.Class, c.Object, c.Path, release) {
					ix.anchors[k] = appendUnique(ix.anchors[k], it)
				}
			}
		}
	}
	return ix
}

// anchorKeys identify a rendered change across render targets: class, API
// group, kind, and the object name and path with the release name replaced
// (a release-scope render uses the product id as release name; an
// environment one the customer's). The namespace and API version are left
// out. The second key drops a trailing member selector, so a release-scope
// change recorded without one still matches.
func anchorKeys(class, object, path, release string) []string {
	g, _, kind, _, name := splitObject(object)
	norm := func(s string) string {
		if release == "" {
			return s
		}
		return strings.ReplaceAll(s, release, "{release}")
	}
	k := []string{strings.Join([]string{class, g, kind, norm(name), norm(path)}, "|")}
	if base := memberSelector.ReplaceAllString(path, ""); base != path {
		k = append(k, strings.Join([]string{class, g, kind, norm(name), norm(base)}, "|"))
	}
	return k
}

var memberSelector = regexp.MustCompile(`\[=[^\]]*\]$`)

// pair stores one render pair's artifacts and indexes its changes.
func (ix *indexer) pair(ctx context.Context, st investigate.ArtifactStore, p *render.Pair, name string, status domain.CollectionStatus, reason string) (domain.ManifestPair, []domain.ArtifactRef, []domain.Evidence, error) {
	mp := domain.ManifestPair{Scope: domain.RenderEnvironment, Target: name, Status: status, Reason: reason}
	dir := "derived/render/env/" + slug(name)
	product := string(ix.edge.Product.ID)
	var arts []domain.ArtifactRef
	put := func(spec investigate.ArtifactSpec, b []byte) (domain.ArtifactRef, error) {
		spec.Origin, spec.Producer = domain.OriginDerived, Producer
		spec.URI = "ri:" + Producer + "/" + spec.Path
		ref, err := st.Put(ctx, spec, b)
		if err != nil {
			return ref, investigate.TransientError(domain.StageAssemble, fmt.Errorf("renderdiff: store %s: %w", spec.Path, err))
		}
		arts = append(arts, ref)
		return ref, nil
	}
	for _, side := range []struct {
		name string
		res  *render.Result
		dst  *string
	}{{"from", p.FromResult, &mp.From}, {"to", p.ToResult, &mp.To}} {
		if !side.res.OK() {
			continue
		}
		b, redacted := EncodeObjects(side.res.Objects)
		ref, err := put(investigate.ArtifactSpec{Kind: domain.ArtifactRenderedManifests, Path: dir + "/" + side.name + ".yaml",
			MediaType: "application/yaml", Redacted: redacted, Tags: manifestTags(product, p, side.name, side.res.Objects)}, b)
		if err != nil {
			return mp, nil, nil, err
		}
		*side.dst = ref.ID
	}
	if p.Diff == nil {
		return mp, arts, nil, nil
	}
	indexed := status == domain.CollectionOK
	entries := ix.entries(p, name)
	doc := DiffDocument{SchemaVersion: DiffSchemaVersion, Product: product, From: p.From, To: p.To, Scope: domain.RenderEnvironment,
		Target: name, TargetKind: p.Target.Kind, Why: ix.rel(p.Target.Why), ReleaseName: p.Target.ReleaseName, Namespace: p.Target.Namespace,
		NamesAssumed: p.Target.NamesAssumed, Status: status, Reason: reason, Indexed: indexed,
		FromRender: ix.provenance(p.FromResult), ToRender: ix.provenance(p.ToResult),
		FromObjects: p.Diff.FromObjects, ToObjects: p.Diff.ToObjects, Suppressed: p.Diff.Suppressed, Changes: entries}
	if doc.Changes == nil {
		doc.Changes = []DiffEntry{}
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return mp, nil, nil, investigate.PermanentError(domain.StageAssemble, fmt.Errorf("renderdiff: encode diff: %w", err))
	}
	var items []string
	sensitive := false
	for _, e := range entries {
		sensitive = sensitive || e.Sensitive
		if indexed {
			for _, it := range e.ChangeItems {
				items = appendUnique(items, it)
			}
		}
	}
	sort.Strings(items)
	diffRef, err := put(investigate.ArtifactSpec{Kind: domain.ArtifactRenderDiff, Path: dir + "/diff.json", MediaType: "application/json",
		Redacted: sensitive, Tags: diffTags(product, p, entries), ChangeItems: items}, b)
	if err != nil {
		return mp, nil, nil, err
	}
	mp.Diff = diffRef.ID
	if !indexed {
		return mp, arts, nil, nil
	}
	var evs []domain.Evidence
	for _, e := range entries {
		rc := domain.RenderedChange{ID: e.ID, Class: string(e.Class), Object: e.Object.String(), Path: e.Path, Sensitive: e.Sensitive,
			ChangeItems: e.ChangeItems, Correlation: e.Correlation}
		if !e.Sensitive {
			rc.Before, rc.After = deref(e.Before), deref(e.After)
		}
		ev := ix.evidence(p, e, diffRef)
		rc.Evidence = []domain.EvidenceID{ev.ID}
		evs = append(evs, ev)
		mp.Changes = append(mp.Changes, rc)
	}
	return mp, arts, evs, nil
}

// entries derives the indexed form of every change of the pair's diff,
// sorted by id.
func (ix *indexer) entries(p *render.Pair, target string) []DiffEntry {
	corr := render.Correlate(p.Diff, ix.edge)
	release := p.Target.ReleaseName
	seen := map[string]int{}
	out := make([]DiffEntry, 0, len(p.Diff.Changes))
	for i, c := range p.Diff.Changes {
		e := DiffEntry{Change: c, Path: changePath(c)}
		id := domain.RenderedChangeID(domain.RenderEnvironment, target, c.Object.String(), e.Path, string(c.Class))
		if n := seen[id]; n > 0 { // still not unique: disambiguate in diff order
			e.Path = fmt.Sprintf("%s[#%d]", e.Path, n+1)
		}
		seen[id]++
		e.ID = domain.RenderedChangeID(domain.RenderEnvironment, target, c.Object.String(), e.Path, string(c.Class))
		if i < len(corr.Changes) {
			e.Links = corr.Changes[i].Links
		}
		var rules []string
		for _, l := range e.Links {
			if l.ChangeID == "" {
				// an artifact-delta link names no change: it attaches to the
				// computed image items of that repository (the release-scope
				// collector's rule), else to nothing
				if l.Rule != "artifact" {
					continue
				}
				for _, it := range ix.images {
					if itemMentions(it, strings.TrimPrefix(l.Title, "artifact version change of ")) {
						e.ChangeItems = appendUnique(e.ChangeItems, it.ID)
						rules = appendUnique(rules, l.Rule)
					}
				}
				continue
			}
			if it := domain.ChangeItemID(l.ChangeID, "", ""); ix.items[it] {
				e.ChangeItems = appendUnique(e.ChangeItems, it)
				rules = appendUnique(rules, l.Rule)
			}
		}
		for _, k := range anchorKeys(string(c.Class), c.Object.String(), e.Path, release) {
			if its := ix.anchors[k]; len(its) > 0 {
				for _, it := range its {
					e.ChangeItems = appendUnique(e.ChangeItems, it)
				}
				rules = appendUnique(rules, RuleReleaseAnchor)
				break
			}
		}
		sort.Strings(e.ChangeItems)
		sort.Strings(rules)
		e.Correlation = strings.Join(rules, ",")
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// itemMentions reports whether an item's title or subjects name an image
// repository (or its last path segment, when that has 4+ characters).
func itemMentions(it domain.ChangeItem, repo string) bool {
	if repo == "" {
		return false
	}
	base := path.Base(repo)
	for _, s := range append([]string{it.Title}, it.Subjects...) {
		if strings.Contains(s, repo) || len(base) >= 4 && strings.Contains(s, base) {
			return true
		}
	}
	return false
}

// changePath is the RenderedChange path of a render change. Container args
// and RBAC rules are sets addressed by one path, so their changes get a
// member selector — "...args[=--flag]", "rules[=apps/deployments:get]" —
// which keeps (scope, target, object, path, class) unique per change.
func changePath(c render.Change) string {
	switch c.Class {
	case render.ContainerArgAdded, render.ContainerArgRemoved, render.ContainerArgChanged:
		if c.Name != "" {
			return c.Path + "[=" + c.Name + "]"
		}
	case render.RBACPermissionAdded, render.RBACPermissionRemoved:
		if c.Permission != nil {
			return c.Path + "[=" + c.Permission.String() + "]"
		}
		if c.Name != "" {
			return c.Path + "[=" + c.Name + "]"
		}
	}
	return c.Path
}

// evidence is the rendered-diff evidence record of one indexed change: the
// render's own record (template, endpoints, digests; values hidden), cited
// against the diff artifact's bytes.
func (ix *indexer) evidence(p *render.Pair, e DiffEntry, diff domain.ArtifactRef) domain.Evidence {
	ev := p.Evidence(e.Change, false)
	ev.SourceID = diff.ID
	ev.URI = diff.URI
	ev.Locator = "changes[id=" + e.ID + "]"
	ev.ContentDigest = diff.Digest
	ev.RetrievedAt = time.Time{}
	if ev.Render != nil {
		ev.Render.Path = e.Path
	}
	ev.ID = domain.EvidenceID("ev-" + domain.ShortHash(string(ev.Kind), ev.URI, ev.Locator, ev.Excerpt, ev.ContentDigest))
	return ev
}

// provenance is a render's provenance without the volatile fields (render
// time, cache hit) and with repo-relative paths, so the diff artifact's
// digest is reproducible.
func (ix *indexer) provenance(r *render.Result) *render.Provenance {
	if r == nil {
		return nil
	}
	p := r.Provenance
	p.RenderedAt = time.Time{}
	p.ChartURI = ix.rel(p.ChartURI)
	p.KustomizeRoot = ix.rel(p.KustomizeRoot)
	if p.KustomizeRoot == "" && r.Provenance.KustomizeRoot != "" {
		p.KustomizeRoot = "."
	}
	p.KustomizeDir = ix.rel(p.KustomizeDir)
	p.IncompleteReason = ix.rel(p.IncompleteReason)
	p.Values = relDigests(p.Values, ix.rel)
	p.Inputs = relDigests(p.Inputs, ix.rel)
	if len(p.Set) > 0 {
		set := append([]render.SetValue(nil), p.Set...)
		for i := range set {
			set[i].Origin = ix.rel(set[i].Origin)
		}
		p.Set = set
	}
	if len(p.Substitutions) > 0 {
		subs := append([]render.Substitution(nil), p.Substitutions...)
		for i := range subs {
			subs[i].File = ix.rel(subs[i].File)
		}
		p.Substitutions = subs
	}
	cmd := append([]string(nil), p.Command...)
	for i := range cmd {
		cmd[i] = ix.rel(cmd[i])
	}
	p.Command = cmd
	return &p
}

func relDigests(in []render.InputDigest, rel func(string) string) []render.InputDigest {
	if len(in) == 0 {
		return in
	}
	out := append([]render.InputDigest(nil), in...)
	for i := range out {
		out[i].Origin = rel(out[i].Origin)
	}
	return out
}

func manifestTags(product string, p *render.Pair, side string, objs []render.Object) []string {
	tags := []string{"rendered-manifests", "environment", product, string(p.Target.Kind), side}
	const maxNames = 300
	names := 0
	for _, o := range objs {
		tags = append(tags, o.ID.Kind)
		if names < maxNames && o.ID.Name != "" {
			tags = append(tags, o.ID.Name)
			names++
		}
	}
	return normTags(tags)
}

func diffTags(product string, p *render.Pair, entries []DiffEntry) []string {
	tags := []string{"render-diff", "environment", product, string(p.Target.Kind)}
	for _, e := range entries {
		tags = append(tags, string(e.Class), e.Object.Kind)
	}
	return normTags(tags)
}

func normTags(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

var unsafePath = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// slug is a path-safe, unique directory name for a target.
func slug(target string) string {
	s := strings.Trim(unsafePath.ReplaceAllString(target, "-"), "-")
	if len(s) > 80 {
		s = strings.TrimRight(s[:80], "-")
	}
	return s + "-" + domain.ShortHash(target)[:8]
}

// splitObject splits "<group>/<version>/<kind>/<namespace>/<name>".
func splitObject(id string) (group, version, kind, namespace, name string) {
	parts := strings.SplitN(id, "/", 5)
	for len(parts) < 5 {
		parts = append(parts, "")
	}
	return parts[0], parts[1], parts[2], parts[3], parts[4]
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func appendUnique(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
}

func unquotePrefix(s string) (string, error) {
	q, err := strconv.QuotedPrefix(s)
	if err != nil {
		return "", err
	}
	return strconv.Unquote(q)
}
