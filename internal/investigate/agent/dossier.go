package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/validate"
	"github.com/tdavison784/release-intelligence/internal/redaction"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// The item dossier (MISSION §1, prompt investigate-agent/v6): the focused,
// change-scoped context an item's session starts from, assembled
// deterministically from the bundle and the run — no new store, no model.
// It states what is known (the change, the settings that decide it and their
// effective state on each exposed object, correlated and unchanged rendered
// configuration, attached knowledge facts, validations already run) and,
// explicitly, what is not (Gaps). Everything else stays behind the tools.
// Same inputs ⇒ byte-identical dossier (no timestamps, sorted lists).

const (
	DossierFormat   = "ri.dev/item-dossier/v1"
	DossierProducer = "investigate.dossier@v3" // v3: the pin validator's effective object view; JSON field paths
	// MaxDossierBytes bounds the dossier's JSON: it is inlined into the task.
	MaxDossierBytes = 48 << 10
)

// Effective setting states.
const (
	SettingExplicit         = "explicit"          // set in the environment (value and where)
	SettingInheritedDefault = "inherited-default" // left unset in a complete view: the shipped default applies
	SettingUnknown          = "unknown"           // cannot be decided from the evidence (reason)
	// Summary states of a setting across objects.
	SettingMixed     = "mixed"      // objects differ (explicit on some, default on others); see exposed
	SettingNoObjects = "no-objects" // a complete view holds no object of the setting's kind
)

// Gap kinds.
const (
	GapMissing       = "missing"
	GapPartial       = "partial"
	GapContradictory = "contradictory"
)

const (
	dossierUpstream      = 8
	dossierExcerpt       = 400
	dossierDetail        = 1500
	dossierRendered      = 25
	dossierObjects       = 40
	dossierUnchanged     = 25
	dossierFacts         = 10
	dossierValidations   = 15
	dossierValue         = 160
	dossierUnresolved    = 10
	dossierSettings      = 12
	dossierGaps          = 30
	dossierSourcesListed = 12
)

// Dossier is one item's dossier.
type Dossier struct {
	Format      string `json:"format"`
	Producer    string `json:"producer"`
	Bundle      string `json:"bundle"`
	Snapshot    string `json:"snapshot"`
	Item        string `json:"item"`
	Product     string `json:"product"`
	From        string `json:"from"`
	To          string `json:"to"`
	Environment string `json:"environment"`

	Change          DossierChange           `json:"change"`
	Settings        []DossierSetting        `json:"settings"`
	Exposed         []DossierObject         `json:"exposed"`
	Roots           []DossierRoot           `json:"roots,omitempty"`
	RenderedChanges []DossierRenderedChange `json:"renderedChanges"`
	Unchanged       []DossierUnchanged      `json:"unchanged"`
	Knowledge       []DossierFact           `json:"knowledge"`
	Validations     []DossierValidation     `json:"validations"`
	Relations       DossierRelations        `json:"relations"`
	Gaps            []DossierGap            `json:"gaps"`
	Truncated       []string                `json:"truncated,omitempty"`
}

// DossierChange is the change item and what upstream says about it.
type DossierChange struct {
	Title             string                      `json:"title"`
	Family            string                      `json:"family"`
	Manifestation     string                      `json:"manifestation"`
	ManifestationRule string                      `json:"manifestationRule"`
	Scope             string                      `json:"scope"`
	Breaking          bool                        `json:"breaking,omitempty"`
	ActionRequired    bool                        `json:"actionRequired,omitempty"`
	Category          string                      `json:"category,omitempty"`
	Release           string                      `json:"release,omitempty"`
	Detail            string                      `json:"detail,omitempty"`
	Subjects          []string                    `json:"subjects,omitempty"`
	Links             []string                    `json:"links,omitempty"`
	Upstream          []DossierEvidence           `json:"upstream"`
	Sources           []DossierArtifact           `json:"sources,omitempty"`
	MigrationNotes    int                         `json:"migrationNotes,omitempty"`
	JoinFindings      []DossierJoinFinding        `json:"joinFindings,omitempty"`
	Compatibility     *domain.CompatibilityChange `json:"compatibility,omitempty"`
}

// DossierEvidence is one upstream evidence record (verbatim excerpt).
type DossierEvidence struct {
	ID       domain.EvidenceID `json:"id"`
	Artifact string            `json:"artifact,omitempty"`
	URI      string            `json:"uri"`
	Locator  string            `json:"locator,omitempty"`
	Excerpt  string            `json:"excerpt,omitempty"`
}

// DossierArtifact names an artifact to read with read_artifact.
type DossierArtifact struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	URI  string `json:"uri,omitempty"`
}

// DossierJoinFinding is one deterministic join finding of the change.
type DossierJoinFinding struct {
	ID             string `json:"id"`
	Classification string `json:"classification"`
	Rule           string `json:"rule"`
	Title          string `json:"title,omitempty"`
}

// DossierSetting is a setting that decides the item, how it was derived, and
// its state across the environment.
type DossierSetting struct {
	Setting string `json:"setting"`
	Kind    string `json:"kind"` // object-field | values-key | key-path | token
	From    string `json:"from"` // where the dossier learned the setting
	// State summarises the per-object / per-input states: explicit,
	// inherited-default, unknown, or mixed (some objects one way, some another).
	State  string   `json:"state"`
	Detail string   `json:"detail"`
	Where  []string `json:"where,omitempty"` // artifact (path) L<line>: text, for explicit values found by key
}

// DossierObject is an object exposed to a setting and the setting's
// effective state on it.
type DossierObject struct {
	Object   string `json:"object"`
	Setting  string `json:"setting"`
	State    string `json:"state"`
	Value    string `json:"value,omitempty"`
	View     string `json:"view"` // "render <target> (complete)" | "file <path>"
	Artifact string `json:"artifact"`
	Root     bool   `json:"root,omitempty"` // an exposure root of the item
	Reason   string `json:"reason,omitempty"`
}

// DossierRoot is an exposure root of the item.
type DossierRoot struct {
	Node string `json:"node"`
	Key  string `json:"key"`
	Rule string `json:"rule"`
}

// DossierRenderedChange is a rendered change correlated to the item.
type DossierRenderedChange struct {
	ID          string `json:"id"`
	Scope       string `json:"scope"`
	Target      string `json:"target"`
	Class       string `json:"class"`
	Object      string `json:"object"`
	Path        string `json:"path,omitempty"`
	Before      string `json:"before,omitempty"`
	After       string `json:"after,omitempty"`
	Correlation string `json:"correlation,omitempty"`
}

// DossierUnchanged is relevant configuration that is identical before and
// after: an explicit setting the upgrade does not touch.
type DossierUnchanged struct {
	Object  string `json:"object"`
	Setting string `json:"setting"`
	Value   string `json:"value"`
	Target  string `json:"target"`
}

// DossierFact is a knowledge fact attached to the item.
type DossierFact struct {
	Fact        string `json:"fact"`
	Artifact    string `json:"artifact"`
	Status      string `json:"status"`
	Level       string `json:"level"`
	Release     string `json:"release,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Change      string `json:"change,omitempty"`
	Consequence string `json:"consequence,omitempty"`
	Exposure    string `json:"exposure,omitempty"`
	Statement   string `json:"statement,omitempty"`
}

// DossierValidation is a validation already run for the item.
type DossierValidation struct {
	ID              string `json:"id"`
	Validator       string `json:"validator"`
	Kind            string `json:"kind"`
	Level           string `json:"level"`
	Outcome         string `json:"outcome"`
	ClearsBehaviour bool   `json:"clearsBehaviour,omitempty"`
	Pin             string `json:"pin,omitempty"`
	Detail          string `json:"detail,omitempty"`
}

// DossierRelations summarises the item's relations.
type DossierRelations struct {
	Roots      int      `json:"roots"`
	Paths      int      `json:"paths"`
	Unresolved int      `json:"unresolvedPaths"`
	Truncated  bool     `json:"truncated,omitempty"`
	Edges      []string `json:"unresolvedEdges,omitempty"`
}

// DossierGap is evidence that is missing, partial or contradictory.
type DossierGap struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Detail  string `json:"detail"`
}

// DossierInput is what a dossier is built from.
type DossierInput struct {
	Bundle      *domain.EvidenceBundle
	Graph       *domain.RelationshipGraph
	Traversal   *domain.TraversalResult
	Report      *domain.ImpactReport
	Validations []domain.ValidationRecord
	// Read returns an artifact's bytes (digest-verified by the store).
	Read func(ctx context.Context, a domain.ArtifactRef) ([]byte, error)
}

// StoreReader reads artifacts from an artifact store.
func StoreReader(st investigate.ArtifactStore) func(context.Context, domain.ArtifactRef) ([]byte, error) {
	return func(ctx context.Context, a domain.ArtifactRef) ([]byte, error) {
		rc, err := st.Open(ctx, a.Digest)
		if err != nil {
			return nil, fmt.Errorf("artifact %s (%s): %w", a.ID, a.Path, err)
		}
		defer rc.Close()
		var b bytes.Buffer
		if _, err := b.ReadFrom(rc); err != nil {
			return nil, fmt.Errorf("artifact %s (%s): %w", a.ID, a.Path, err)
		}
		return b.Bytes(), nil
	}
}

// BuildDossier assembles the dossier of one item.
func BuildDossier(ctx context.Context, in DossierInput, itemID string) (*Dossier, error) {
	b := in.Bundle
	if b == nil || in.Read == nil {
		return nil, fmt.Errorf("agent: dossier needs a bundle and an artifact reader")
	}
	it, ok := b.Item(itemID)
	if !ok {
		return nil, fmt.Errorf("%w: change item %s is not in bundle %s", investigate.ErrNotFound, itemID, b.ID)
	}
	db := &dossierBuilder{in: in, b: b, it: it, arts: map[string]domain.ArtifactRef{}}
	for _, a := range b.Manifest.Artifacts {
		db.arts[a.ID] = a
	}
	d := &Dossier{Format: DossierFormat, Producer: DossierProducer, Bundle: b.ID, Snapshot: b.Snapshot.ID, Item: it.ID,
		Product: string(b.Change.Product.ID), From: b.Change.From.Tag, To: b.Change.To.Tag, Environment: b.Snapshot.Environment,
		Settings: []DossierSetting{}, Exposed: []DossierObject{}, RenderedChanges: []DossierRenderedChange{}, Unchanged: []DossierUnchanged{},
		Knowledge: []DossierFact{}, Validations: []DossierValidation{}, Gaps: []DossierGap{}}
	db.d = d
	if err := db.change(ctx); err != nil {
		return nil, err
	}
	db.rendered()
	if err := db.knowledge(ctx); err != nil {
		return nil, err
	}
	if err := db.view(ctx); err != nil {
		return nil, err
	}
	db.settings(ctx)
	db.validations()
	db.relations()
	db.gaps()
	db.bound()
	return d, nil
}

// Encode is the dossier's canonical JSON (the stored artifact's bytes).
func (d *Dossier) Encode() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	if err := enc.Encode(d); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Artifacts are the artifact ids the dossier quotes or points to: the
// session that starts from it has retrieved them.
func (d *Dossier) Artifacts() []string {
	var out []string
	for _, e := range d.Change.Upstream {
		out = append(out, e.Artifact)
	}
	for _, a := range d.Change.Sources {
		out = append(out, a.ID)
	}
	for _, o := range d.Exposed {
		out = append(out, o.Artifact)
	}
	for _, f := range d.Knowledge {
		out = append(out, f.Artifact)
	}
	var keep []string
	for _, a := range uniqSorted(out) {
		if a != "" {
			keep = append(keep, a)
		}
	}
	return keep
}

// StoreDossier stores the dossier as a content-addressed run artifact.
func StoreDossier(ctx context.Context, st investigate.ArtifactStore, d *Dossier) (domain.ArtifactRef, error) {
	data, err := d.Encode()
	if err != nil {
		return domain.ArtifactRef{}, err
	}
	return st.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactItemDossier,
		Path:   "derived/dossier/" + d.Item + "/" + domain.ShortHash(string(data)) + ".json",
		Origin: domain.OriginDerived, MediaType: "application/json", URI: "ri:" + DossierProducer, Producer: DossierProducer,
		Tags: []string{"item-dossier", d.Item}, ChangeItems: []string{d.Item}}, data)
}

type dossierBuilder struct {
	in   DossierInput
	b    *domain.EvidenceBundle
	it   domain.ChangeItem
	d    *Dossier
	arts map[string]domain.ArtifactRef
	edge *domain.UpgradeEdge

	specs []settingSpec
	facts []domain.VerifiedFact
	objs  []viewObject
	// complete: at least one environment render pair is complete.
	complete bool
	envText  []textArt       // environment files (text)
	schemas  []render.Object // CRDs of the effective view (objectSpec)
	toText   []textArt       // complete environment renders, to side
}

type textArt struct {
	art  domain.ArtifactRef
	text string
}

func (db *dossierBuilder) read(ctx context.Context, id string) ([]byte, domain.ArtifactRef, error) {
	a, ok := db.arts[id]
	if !ok {
		return nil, a, fmt.Errorf("%w: artifact %s", investigate.ErrNotFound, id)
	}
	data, err := db.in.Read(ctx, a)
	return data, a, err
}

func (db *dossierBuilder) gap(kind, subject, detail string) {
	db.d.Gaps = append(db.d.Gaps, DossierGap{Kind: kind, Subject: subject, Detail: clip(detail, dossierExcerpt)})
}

// --- the change -------------------------------------------------------------------------

func (db *dossierBuilder) change(ctx context.Context) error {
	it, b, d := db.it, db.b, db.d
	d.Change = DossierChange{Title: it.Title, Family: string(it.Family), Manifestation: string(it.Manifestation), ManifestationRule: it.ManifestationRule,
		Scope: it.ScopeRule, Breaking: it.Breaking, ActionRequired: it.ActionRequired, Subjects: it.Subjects, Upstream: []DossierEvidence{}}
	if data, _, err := db.read(ctx, b.Change.Edge); err == nil {
		var e domain.UpgradeEdge
		if err := json.Unmarshal(data, &e); err != nil {
			db.gap(GapMissing, "upgrade edge", "the upgrade-edge artifact does not parse: "+err.Error())
		} else {
			db.edge = &e
		}
	} else if ctx.Err() != nil {
		return ctx.Err()
	} else {
		db.gap(GapMissing, "upgrade edge", "the upgrade-edge artifact is unreadable: "+err.Error())
	}
	if e := db.edge; e != nil {
		for _, c := range e.Changes {
			if it.ChangeID == "" || c.ID != it.ChangeID {
				continue
			}
			d.Change.Category, d.Change.Release, d.Change.Detail = string(c.Category), c.Release, clip(c.Detail, dossierDetail)
			for _, r := range c.References {
				d.Change.Links = append(d.Change.Links, r.URL)
			}
		}
		for i := range e.Compatibility {
			if it.Platform != "" && e.Compatibility[i].Platform == it.Platform {
				c := e.Compatibility[i]
				d.Change.Compatibility = &c
			}
		}
	}
	pool := b.EvidenceByID()
	uris := map[string]bool{}
	for i, id := range it.Evidence {
		ev, ok := pool[id]
		if !ok {
			continue
		}
		uris[ev.URI] = true
		if i == dossierUpstream {
			d.Truncated = append(d.Truncated, fmt.Sprintf("upstream evidence: %d more (get_change_item)", len(it.Evidence)-i))
			break
		}
		d.Change.Upstream = append(d.Change.Upstream, DossierEvidence{ID: ev.ID, Artifact: db.sourceArtifact(ev), URI: ev.URI, Locator: ev.Locator, Excerpt: clip(ev.Excerpt, dossierExcerpt)})
	}
	for _, a := range b.Manifest.Artifacts {
		if a.Origin == domain.OriginUpstream && (contains(a.ChangeItems, it.ID) || uris[a.URI] || contains(it.Artifacts, a.ID)) {
			if len(d.Change.Sources) == dossierSourcesListed {
				d.Truncated = append(d.Truncated, "upstream sources: more (get_change_item)")
				break
			}
			d.Change.Sources = append(d.Change.Sources, DossierArtifact{ID: a.ID, Path: a.Path, URI: a.URI})
		}
	}
	d.Change.MigrationNotes = len(b.Change.MigrationNotes)
	if r := db.in.Report; r != nil && it.ChangeID != "" {
		for _, f := range r.Findings {
			if f.ChangeID == it.ChangeID {
				d.Change.JoinFindings = append(d.Change.JoinFindings, DossierJoinFinding{ID: f.ID, Classification: string(f.Classification), Rule: f.Rule, Title: clip(f.Title, 160)})
			}
		}
	}
	if len(d.Change.Upstream) == 0 {
		db.gap(GapMissing, "upstream evidence", "the item cites no upstream evidence the bundle resolves")
	}
	return nil
}

// sourceArtifact finds the artifact an evidence record was cut from.
func (db *dossierBuilder) sourceArtifact(ev domain.Evidence) string {
	if _, ok := db.arts[ev.SourceID]; ok {
		return ev.SourceID
	}
	for _, a := range db.b.Manifest.Artifacts {
		if a.URI == ev.URI && (ev.ContentDigest == "" || ev.ContentDigest == a.Digest) {
			return a.ID
		}
	}
	return ""
}

// --- rendered changes --------------------------------------------------------------------

func (db *dossierBuilder) rendered() {
	var out []DossierRenderedChange
	for _, ps := range [][]domain.ManifestPair{db.b.Renders, db.b.Change.Renders} {
		for _, p := range ps {
			for _, c := range p.Changes {
				if !contains(c.ChangeItems, db.it.ID) && c.ID != db.it.RenderedChange {
					continue
				}
				rc := DossierRenderedChange{ID: c.ID, Scope: string(p.Scope), Target: p.Target, Class: c.Class, Object: c.Object, Path: c.Path,
					Correlation: c.Correlation}
				if !c.Sensitive {
					rc.Before, rc.After = clip(c.Before, dossierValue), clip(c.After, dossierValue)
				}
				out = append(out, rc)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > dossierRendered {
		db.d.Truncated = append(db.d.Truncated, fmt.Sprintf("rendered changes: %d more (query_diff changeItem=%s)", len(out)-dossierRendered, db.it.ID))
		out = out[:dossierRendered]
	}
	db.d.RenderedChanges = append(db.d.RenderedChanges, out...)
}

// --- knowledge -------------------------------------------------------------------------

func (db *dossierBuilder) knowledge(ctx context.Context) error {
	var arts []domain.ArtifactRef
	for _, a := range db.b.Manifest.Artifacts {
		if a.Kind == domain.ArtifactKnowledgeFact && (contains(a.ChangeItems, db.it.ID) || contains(db.it.Artifacts, a.ID)) {
			arts = append(arts, a)
		}
	}
	sort.Slice(arts, func(i, j int) bool { return arts[i].ID < arts[j].ID })
	bySubject := map[string][]domain.VerifiedFact{}
	for i, a := range arts {
		data, err := db.in.Read(ctx, a)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			db.gap(GapMissing, "knowledge fact "+a.Path, "unreadable: "+err.Error())
			continue
		}
		var f domain.VerifiedFact
		if err := json.Unmarshal(data, &f); err != nil {
			db.gap(GapMissing, "knowledge fact "+a.Path, "does not parse: "+err.Error())
			continue
		}
		db.facts = append(db.facts, f)
		if i < dossierFacts {
			db.d.Knowledge = append(db.d.Knowledge, factView(f, a.ID))
		} else if i == dossierFacts {
			db.d.Truncated = append(db.d.Truncated, fmt.Sprintf("knowledge facts: %d more (search_manifest kinds=[knowledge-fact])", len(arts)-i))
		}
		if f.Status == domain.FactActive && f.Assertion.Subject != nil {
			k := f.Assertion.Subject.Key()
			bySubject[k] = append(bySubject[k], f)
		}
	}
	keys := make([]string, 0, len(bySubject))
	for k := range bySubject {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fs := bySubject[k]
		for i := 1; i < len(fs); i++ {
			a, c := fs[0].Assertion, fs[i].Assertion
			if a.AspectDigest(domain.AspectChange) != c.AspectDigest(domain.AspectChange) || a.AspectDigest(domain.AspectConsequence) != c.AspectDigest(domain.AspectConsequence) {
				db.gap(GapContradictory, "knowledge "+k, fmt.Sprintf("active facts %s and %s state a different change or consequence for the same subject", fs[0].ID, fs[i].ID))
			}
		}
	}
	return nil
}

func factView(f domain.VerifiedFact, artifact string) DossierFact {
	v := DossierFact{Fact: f.ID, Artifact: artifact, Status: string(f.Status), Level: string(f.Level()), Release: f.Release,
		Statement: clip(f.Assertion.Statement, dossierExcerpt)}
	a := f.Assertion
	if a.Subject != nil {
		v.Subject = a.Subject.Key()
	}
	if c := a.Change; c != nil {
		v.Change = string(c.Type)
		if c.Before != nil || c.After != nil {
			v.Change += fmt.Sprintf(" %s → %s", deref(c.Before), deref(c.After))
		}
	}
	if c := a.Consequence; c != nil {
		v.Consequence = string(c.Kind)
	}
	if ap := a.Applicability; ap != nil {
		if raw, err := json.Marshal(ap.Exposure); err == nil {
			v.Exposure = clip(string(raw), dossierExcerpt)
		}
	}
	return v
}

func deref(s *string) string {
	if s == nil {
		return "?"
	}
	return *s
}

// --- the environment's object view ---------------------------------------------------------

// viewObject is one environment object: from a complete render pair (the
// effective object, definitive about absence) or from an environment file as
// declared (an overlay or patch elsewhere may change it).
type viewObject struct {
	id       render.ObjectID
	body     map[string]any
	from     map[string]any // the same object in the from render (complete pairs only)
	target   string
	artifact string
	// complete: seen through a render (the effective object); an unset
	// field there means the shipped default applies.
	complete bool
	label    string // the view label, when not a product render's or a file's
	// unrendered: why its effective state is unknown (it decides nothing).
	unrendered string
	// readBack: withheld enum values read back in body.
	readBack []redaction.Recovered
}

func (o viewObject) view() string {
	if o.label != "" {
		return o.label
	}
	if o.complete {
		return "render " + o.target + " (complete)"
	}
	return "file " + o.target
}

// view reads the environment the dossier states settings from: the texts of
// the environment files and complete renders (values keys, tokens), and the
// objects of the effective object view the pin validator decides on
// (validate.EffectiveView: complete product renders, version-independent
// overlay renders — a kustomize base merged with its patches — and the files
// no render consumed), withheld enum values read back.
func (db *dossierBuilder) view(ctx context.Context) error {
	pairs := append([]domain.ManifestPair(nil), db.b.Renders...)
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Target < pairs[j].Target })
	for _, p := range pairs {
		if p.Status != domain.CollectionOK {
			continue
		}
		db.complete = true
		toB, toA, err := db.read(ctx, p.To)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			db.gap(GapMissing, "render "+p.Target, "the to render is unreadable: "+err.Error())
			continue
		}
		db.toText = append(db.toText, textArt{art: toA, text: string(toB)})
		if _, err := render.ParseObjects(toB); err != nil {
			db.gap(GapPartial, "render "+p.Target, "the to render does not parse: "+err.Error())
		}
	}
	var envArts []domain.ArtifactRef
	for _, a := range db.b.Manifest.Artifacts {
		if a.Origin == domain.OriginEnvironment {
			envArts = append(envArts, a)
		}
	}
	sort.Slice(envArts, func(i, j int) bool { return envArts[i].Path < envArts[j].Path })
	for _, a := range envArts {
		data, err := db.in.Read(ctx, a)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			db.gap(GapMissing, "environment file "+a.Path, "unreadable: "+err.Error())
			continue
		}
		if !isBinary(data) {
			db.envText = append(db.envText, textArt{art: a, text: string(data)})
		}
	}
	ev, err := validate.EffectiveView(ctx, readStore{db.in.Read}, db.b)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		db.gap(GapMissing, "effective object view", err.Error())
		return nil
	}
	for _, g := range ev.Gaps {
		db.gap(GapPartial, "effective object view", g)
	}
	db.schemas = ev.Schemas
	// one entry per object: a render's (product, then version-independent
	// overlay) before a file's — a render shows the effective object
	rank := map[string]int{"to": 0, "overlay": 1, "file": 2}
	eff := append([]validate.EffectiveObject(nil), ev.Objects...)
	sort.SliceStable(eff, func(i, j int) bool {
		if a, b := eff[i].Object.ID.String(), eff[j].Object.ID.String(); a != b {
			return a < b
		}
		if rank[eff[i].Side] != rank[eff[j].Side] {
			return rank[eff[i].Side] < rank[eff[j].Side]
		}
		return eff[i].Target+eff[i].Artifact.Path < eff[j].Target+eff[j].Artifact.Path
	})
	seen := map[string]bool{}
	for _, e := range eff {
		k := e.Object.ID.String()
		if seen[k] {
			continue
		}
		seen[k] = true
		o := viewObject{id: e.Object.ID, body: e.Object.Body, target: e.Target, artifact: e.Artifact.ID,
			complete: e.Rendered, unrendered: e.Unrendered, readBack: e.ReadBack}
		switch e.Side {
		case "file":
			o.target = strings.TrimPrefix(e.Artifact.Path, "env/")
		case "overlay":
			o.label = "render " + e.Target + " (version-independent overlay)"
		}
		if e.From != nil && !e.SameSides {
			o.from = e.From.Body
		}
		db.objs = append(db.objs, o)
	}
	return nil
}

// readStore serves the dossier's reader as a read-only artifact store (the
// effective view reads by digest).
type readStore struct {
	read func(context.Context, domain.ArtifactRef) ([]byte, error)
}

func (r readStore) Put(context.Context, investigate.ArtifactSpec, []byte) (domain.ArtifactRef, error) {
	return domain.ArtifactRef{}, errors.New("dossier: read-only store")
}

func (r readStore) Open(ctx context.Context, digest string) (io.ReadCloser, error) {
	b, err := r.read(ctx, domain.ArtifactRef{Digest: digest})
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// --- deciding settings -------------------------------------------------------------------

// settingSpec is one setting that decides the item.
type settingSpec struct {
	kind         string // object-field | values-key | token
	group, objKd string
	path         string
	token        string
	from         string
	// unverified: path was converted from Go field names (upstream text)
	// and no CRD of the bundle confirms it, so an unset field proves nothing.
	unverified bool
}

func (s settingSpec) key() string {
	switch s.kind {
	case "object-field":
		g := s.group
		if g == "" {
			g = "*"
		}
		return g + "/" + s.objKd + " " + s.path
	case "values-key":
		return "values " + s.path
	case "key-path":
		return "key " + s.path
	}
	return "token " + s.token
}

var upperRe = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)

// objectSpec is the object-field setting a "Kind.field.path" subject names,
// its path as the object's JSON/YAML field path: upstream text often names
// Go fields (Certificate.Spec.PrivateKey.RotationPolicy). A Go-cased path is
// resolved against the CRD schemas of the view (case-insensitively, by
// property); without one confirming it, lowerCamel case is assumed and the
// setting is marked unverified.
func (db *dossierBuilder) objectSpec(segs []string, from string) settingSpec {
	s := settingSpec{kind: "object-field", objKd: segs[0], path: strings.Join(segs[1:], "."), from: from}
	goCased := false
	for _, seg := range segs[1:] {
		if seg != "" && unicode.IsUpper(rune(seg[0])) {
			goCased = true
		}
	}
	if !goCased {
		return s
	}
	resolved := map[string]bool{}
	for _, crd := range db.schemas {
		spec, _ := crd.Body["spec"].(map[string]any)
		names, _ := spec["names"].(map[string]any)
		if k, _ := names["kind"].(string); k != segs[0] {
			continue
		}
		versions, _ := spec["versions"].([]any)
		for _, v := range versions {
			vm, _ := v.(map[string]any)
			sch, _ := vm["schema"].(map[string]any)
			root, _ := sch["openAPIV3Schema"].(map[string]any)
			if p, ok := schemaPath(root, segs[1:]); ok {
				resolved[p] = true
			}
		}
	}
	if len(resolved) == 1 {
		for p := range resolved {
			s.path = p
		}
		return s
	}
	out := make([]string, 0, len(segs)-1)
	for _, seg := range segs[1:] {
		out = append(out, lowerCamel(seg))
	}
	s.path, s.unverified = strings.Join(out, "."), true
	return s
}

// schemaPath resolves Go-cased segments to the schema's property names
// (case-insensitively, through array items).
func schemaPath(node map[string]any, segs []string) (string, bool) {
	var out []string
	for _, seg := range segs {
		for node != nil && node["type"] == "array" {
			node, _ = node["items"].(map[string]any)
		}
		props, _ := node["properties"].(map[string]any)
		var hit string
		for k := range props {
			if strings.EqualFold(k, seg) {
				if hit != "" {
					return "", false // ambiguous
				}
				hit = k
			}
		}
		if hit == "" {
			return "", false
		}
		out = append(out, hit)
		node, _ = props[hit].(map[string]any)
	}
	return strings.Join(out, "."), true
}

// lowerCamel is the conventional JSON name of a Go field (RotationPolicy →
// rotationPolicy, TLSConfig → tlsConfig).
func lowerCamel(s string) string {
	r := []rune(s)
	n := 0
	for n < len(r) && unicode.IsUpper(r[n]) {
		n++
	}
	switch {
	case n == 0:
		return s
	case n == len(r):
		return strings.ToLower(s)
	case n > 1:
		n-- // the last capital starts the next word
	}
	return strings.ToLower(string(r[:n])) + string(r[n:])
}

// deriveSettings lists the settings the evidence names for the item, in a
// fixed order of trust: attached knowledge facts (subject and the settable
// leaves of their exposure condition), the join's matches, the item's subjects.
func (db *dossierBuilder) deriveSettings() {
	seen := map[string]bool{}
	add := func(s settingSpec) {
		for _, x := range db.specs { // a group-less subject names the same field as a fact's
			if s.kind == "object-field" && x.kind == s.kind && x.objKd == s.objKd && x.path == s.path && (s.group == "" || x.group == "" || s.group == x.group) {
				return
			}
			if s.kind == "key-path" && x.kind == "values-key" && x.path == s.path {
				return // the same key, already looked up as a values key
			}
		}
		if k := s.key(); !seen[k] {
			seen[k] = true
			db.specs = append(db.specs, s)
		}
	}
	for _, f := range db.facts {
		if f.Status != domain.FactActive {
			continue
		}
		from := "knowledge fact " + f.ID
		if sub := f.Assertion.Subject; sub != nil {
			switch sub.Family {
			case domain.SubjectCRDField:
				add(settingSpec{kind: "object-field", group: sub.Group, objKd: sub.Kind, path: sub.Path, from: from + " (subject)"})
			case domain.SubjectHelmValue:
				add(settingSpec{kind: "values-key", path: sub.Path, from: from + " (subject)"})
			case domain.SubjectCLIFlag, domain.SubjectEnvVar, domain.SubjectFeatureGate:
				add(settingSpec{kind: "token", token: sub.Name, from: from + " (subject)"})
			case domain.SubjectConfigKey:
				add(settingSpec{kind: "token", token: sub.Path, from: from + " (subject)"})
			}
		}
		if ap := f.Assertion.Applicability; ap != nil {
			walkLeaves(ap.Exposure, "", "", func(group, kind string, c domain.Condition) {
				switch {
				case c.Op == domain.OpField && kind != "":
					add(settingSpec{kind: "object-field", group: group, objKd: kind, path: c.Path, from: from + " (exposure)"})
				case c.Op == domain.OpValuesKey:
					add(settingSpec{kind: "values-key", path: c.Path, from: from + " (exposure)"})
				case c.Op == domain.OpCLIFlag, c.Op == domain.OpEnvVar:
					add(settingSpec{kind: "token", token: c.Name, from: from + " (exposure)"})
				}
			})
		}
	}
	if r := db.in.Report; r != nil && db.it.ChangeID != "" {
		for _, f := range r.Findings {
			if f.ChangeID != db.it.ChangeID {
				continue
			}
			for _, m := range f.Matches {
				if m.Kind == domain.MatchValuesKey {
					add(settingSpec{kind: "values-key", path: m.Subject, from: "join finding " + f.ID})
				}
			}
		}
	}
	for _, sub := range db.it.Subjects {
		sub = strings.TrimSpace(strings.TrimPrefix(sub, "values:"))
		segs := strings.Split(sub, ".")
		switch {
		case strings.HasPrefix(sub, "-"):
			add(settingSpec{kind: "token", token: sub, from: "item subject"})
		case valuesKeyRe.MatchString(sub) && upperRe.MatchString(segs[0]):
			add(db.objectSpec(segs, "item subject"))
		case valuesKeyRe.MatchString(sub):
			add(settingSpec{kind: "values-key", path: sub, from: "item subject"})
		case identRe.MatchString(sub):
			add(settingSpec{kind: "token", token: sub, from: "item subject"})
		}
	}
	for _, span := range codeSpans(db.upstreamText()) {
		key := span
		if i := strings.IndexAny(key, ":= "); i > 0 {
			key = key[:i] // "a.b: value", "--flag=value"
		}
		segs := strings.Split(key, ".")
		switch {
		case valuesKeyRe.MatchString(key) && upperRe.MatchString(segs[0]):
			add(db.objectSpec(segs, "upstream text"))
		case valuesKeyRe.MatchString(key):
			add(settingSpec{kind: "key-path", path: key, from: "upstream text"})
		case identRe.MatchString(key) && strings.ContainsAny(key, "-_"):
			add(settingSpec{kind: "token", token: key, from: "upstream text"})
		}
	}
	if len(db.specs) > dossierSettings {
		db.d.Truncated = append(db.d.Truncated, fmt.Sprintf("settings: %d more not evaluated", len(db.specs)-dossierSettings))
		db.specs = db.specs[:dossierSettings]
	}
}

// upstreamText is the change's own upstream wording: the title, the detail
// and the item's evidence excerpts.
func (db *dossierBuilder) upstreamText() string {
	parts := []string{db.it.Title, db.d.Change.Detail}
	for _, e := range db.d.Change.Upstream {
		parts = append(parts, e.Excerpt)
	}
	return strings.Join(parts, "\n")
}

var codeSpanRe = regexp.MustCompile("`([^`\n]{2,120})`")

// codeSpans are the code spans of a text, in order of first occurrence: the
// way release notes name flags, keys and fields.
func codeSpans(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range codeSpanRe.FindAllStringSubmatch(text, -1) {
		if x := strings.TrimSpace(m[1]); !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// walkLeaves visits every leaf with its enclosing resource scope.
func walkLeaves(c domain.Condition, group, kind string, f func(group, kind string, c domain.Condition)) {
	if c.Op == domain.OpResource {
		group, kind = c.Group, c.Kind
	}
	if len(c.Of) == 0 {
		f(group, kind, c)
		return
	}
	if c.Op == domain.OpRef { // a ref re-scopes to another resource the dossier does not resolve
		return
	}
	for _, x := range c.Of {
		walkLeaves(x, group, kind, f)
	}
}

func (db *dossierBuilder) settings(ctx context.Context) {
	db.deriveSettings()
	if len(db.specs) == 0 {
		db.gap(GapMissing, "deciding setting", "neither an attached knowledge fact, the join nor the item's subjects name a setting the dossier can check; read the upstream text (get_change_item, read_artifact) for the setting that decides the item")
		return
	}
	roots := map[string]bool{}
	if t := db.in.Traversal; t != nil && db.in.Graph != nil {
		keys := map[string]string{}
		for _, n := range db.in.Graph.Nodes {
			keys[n.ID] = n.Key
		}
		for _, r := range t.RootsOf(db.it.ID) {
			roots[keys[r.Node]] = true
		}
	}
	for _, s := range db.specs {
		var ds DossierSetting
		switch s.kind {
		case "object-field":
			ds = db.objectSetting(s, roots)
		case "values-key":
			ds = db.keySetting(s, db.envText, "a complete render consumed the values, so the chart default applies")
		case "key-path":
			ds = db.keySetting(s, append(append([]textArt(nil), db.envText...), db.toText...), "it occurs at no key path of the environment files or the complete renders: the shipped default applies")
		default:
			ds = db.tokenSetting(s)
		}
		if s.kind != "object-field" && ds.State == SettingUnknown {
			db.gap(GapMissing, "setting "+ds.Setting, ds.Detail)
		}
		db.d.Settings = append(db.d.Settings, ds)
	}
}

func (db *dossierBuilder) objectSetting(s settingSpec, roots map[string]bool) DossierSetting {
	ds := DossierSetting{Setting: s.key(), Kind: s.kind, From: s.from}
	segs := splitFieldPath(s.path)
	counts := map[string]int{}
	n := 0
	for _, o := range db.objs {
		if o.id.Kind != s.objKd || (s.group != "" && o.id.Group != s.group) {
			continue
		}
		n++
		obj := DossierObject{Object: o.id.String(), Setting: s.key(), View: o.view(), Artifact: o.artifact, Root: roots[o.id.String()]}
		vals := fieldValues(o.body, segs)
		switch {
		case o.unrendered != "":
			obj.State, obj.Reason = SettingUnknown, o.unrendered
		case len(vals) > 0:
			obj.State, obj.Value = SettingExplicit, clip(strings.Join(vals, ", "), dossierValue)
			for _, r := range o.readBack {
				if r.Path == s.path {
					obj.Reason = readBackNote(r)
				}
			}
			if o.from != nil {
				if before := fieldValues(o.from, segs); strings.Join(before, ", ") == strings.Join(vals, ", ") {
					db.unchanged(DossierUnchanged{Object: o.id.String(), Setting: s.key(), Value: obj.Value, Target: o.target})
				}
			}
		case o.complete && s.unverified:
			obj.State, obj.Reason = SettingUnknown, "unset at "+s.path+", a path converted from the upstream text's Go field names that no CRD in the bundle confirms"
		case o.complete:
			obj.State, obj.Reason = SettingInheritedDefault, "unset in the effective (rendered) object: the shipped default applies"
		default:
			obj.State, obj.Reason = SettingUnknown, "unset in the file as declared; no complete render shows the effective object (an overlay or patch may set it)"
		}
		counts[obj.State]++
		if n <= dossierObjects {
			db.d.Exposed = append(db.d.Exposed, obj)
		}
	}
	if n > dossierObjects {
		db.d.Truncated = append(db.d.Truncated, fmt.Sprintf("objects for %s: %d more", s.key(), n-dossierObjects))
	}
	switch {
	case n == 0 && db.complete:
		ds.State, ds.Detail = SettingNoObjects, fmt.Sprintf("no %s object in the environment's complete renders or files", s.objKd)
	case n == 0:
		ds.State, ds.Detail = SettingUnknown, fmt.Sprintf("no %s object found and no complete environment render: objects may exist that the evidence does not show", s.objKd)
	default:
		ds.State = summaryState(counts)
		ds.Detail = fmt.Sprintf("%d %s object(s): %d explicit, %d inherited-default, %d unknown", n, s.objKd, counts[SettingExplicit], counts[SettingInheritedDefault], counts[SettingUnknown])
	}
	if ds.State == SettingUnknown || counts[SettingUnknown] > 0 {
		db.gap(GapMissing, "setting "+ds.Setting, ds.Detail)
	}
	return ds
}

func summaryState(counts map[string]int) string {
	var states []string
	for _, s := range []string{SettingExplicit, SettingInheritedDefault, SettingUnknown} {
		if counts[s] > 0 {
			states = append(states, s)
		}
	}
	if len(states) == 1 {
		return states[0]
	}
	return SettingMixed
}

func (db *dossierBuilder) unchanged(u DossierUnchanged) {
	if len(db.d.Unchanged) == dossierUnchanged {
		db.d.Truncated = append(db.d.Truncated, "unchanged settings: more")
	}
	if len(db.d.Unchanged) >= dossierUnchanged {
		return
	}
	db.d.Unchanged = append(db.d.Unchanged, u)
}

// keySetting looks the key path up in the inputs (environment files, and
// for a key path from the upstream text the complete renders too): at the
// document root, or under a parent (a HelmRelease spec.values, a nested
// values block, an object's spec). Absent everywhere with a complete render
// ⇒ the default; the leaf occurring elsewhere ⇒ unknown.
func (db *dossierBuilder) keySetting(s settingSpec, inputs []textArt, inherited string) DossierSetting {
	ds := DossierSetting{Setting: s.key(), Kind: s.kind, From: s.from}
	path := strings.Split(s.path, ".")
	leaf := path[len(path)-1]
	values := map[string]bool{}
	var loose []string
	for _, ta := range inputs {
		dec := yaml.NewDecoder(strings.NewReader(ta.text))
		lines := strings.Split(ta.text, "\n")
		for {
			var doc yaml.Node
			if err := dec.Decode(&doc); err != nil {
				break
			}
			root := &doc
			if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
				root = root.Content[0]
			}
			suffixMatches(root, path, func(k, v *yaml.Node) {
				val := canonical(decodeNode(v))
				values[val] = true
				ds.Where = append(ds.Where, fmt.Sprintf("%s (%s) L%d: %s", ta.art.ID, ta.art.Path, k.Line, clip(strings.TrimSpace(lines[k.Line-1]), dossierValue)))
			})
		}
		if tokenLine(ta.text, leaf) > 0 {
			loose = append(loose, fmt.Sprintf("%s L%d", ta.art.Path, tokenLine(ta.text, leaf)))
		}
	}
	sort.Strings(ds.Where)
	switch {
	case len(ds.Where) > 0:
		ds.State = SettingExplicit
		vs := make([]string, 0, len(values))
		for v := range values {
			vs = append(vs, v)
		}
		sort.Strings(vs)
		ds.Detail = fmt.Sprintf("set explicitly to %s at %d place(s)", strings.Join(vs, ", "), len(ds.Where))
		if s.kind == "key-path" {
			ds.Detail += "; a key path names no object kind, so objects that leave it unset are not listed here"
		}
		if len(vs) > 1 {
			db.gap(GapContradictory, "setting "+s.key(), "set to different values in several environment files ("+strings.Join(vs, ", ")+"); which is effective depends on the order the files are applied")
		}
	case len(loose) > 0:
		ds.State, ds.Detail = SettingUnknown, fmt.Sprintf("the key %s occurs (%s) but not at the key path %s: the dossier cannot tell whether it sets this setting", leaf, strings.Join(loose, ", "), s.path)
	case db.complete:
		ds.State, ds.Detail = SettingInheritedDefault, "not set in any environment file; "+inherited
	default:
		ds.State, ds.Detail = SettingUnknown, "not set in any environment file the bundle holds, and no complete render shows the deployed configuration"
	}
	return ds
}

// suffixMatches calls f for every key whose path from the root ends with path.
func suffixMatches(n *yaml.Node, path []string, f func(k, v *yaml.Node)) {
	if n == nil {
		return
	}
	if n.Kind == yaml.MappingNode {
		if k, v := yamlAt(n, path); k != nil {
			f(k, v)
		}
		for i := 1; i < len(n.Content); i += 2 {
			suffixMatches(n.Content[i], path, f)
		}
		return
	}
	for _, c := range n.Content {
		suffixMatches(c, path, f)
	}
}

// tokenSetting searches complete renders, then environment files, for the
// token as a whole word.
func (db *dossierBuilder) tokenSetting(s settingSpec) DossierSetting {
	ds := DossierSetting{Setting: s.key(), Kind: s.kind, From: s.from}
	find := func(ts []textArt) {
		for _, ta := range ts {
			if l := tokenLine(ta.text, s.token); l > 0 {
				ds.Where = append(ds.Where, fmt.Sprintf("%s (%s) L%d: %s", ta.art.ID, ta.art.Path, l, clip(strings.TrimSpace(strings.Split(ta.text, "\n")[l-1]), dossierValue)))
			}
		}
	}
	find(db.toText)
	if len(ds.Where) == 0 {
		find(db.envText)
	}
	switch {
	case len(ds.Where) > 0:
		ds.State, ds.Detail = SettingExplicit, "the token occurs in the environment (see where); check the value it is given"
	case db.complete:
		ds.State, ds.Detail = SettingInheritedDefault, "the token occurs in no complete environment render nor environment file: the shipped default applies"
	default:
		ds.State, ds.Detail = SettingUnknown, "the token occurs in no environment file, and no complete render shows the deployed configuration"
	}
	return ds
}

func tokenLine(text, token string) int {
	if token == "" {
		return 0
	}
	re := regexp.MustCompile(`(^|[^A-Za-z0-9_.-])` + regexp.QuoteMeta(token) + `($|[^A-Za-z0-9_-])`)
	for i, l := range strings.Split(text, "\n") {
		if re.MatchString(l) {
			return i + 1
		}
	}
	return 0
}

// splitFieldPath splits "spec.a[].b" into segments; "[]" marks list elements.
func splitFieldPath(p string) []string {
	var out []string
	for _, seg := range strings.Split(p, ".") {
		if name, ok := strings.CutSuffix(seg, "[]"); ok {
			out = append(out, name, "[]")
			continue
		}
		out = append(out, seg)
	}
	return out
}

// fieldValues returns the canonical values at the path (every list element
// for "[]").
func fieldValues(v any, segs []string) []string {
	if len(segs) == 0 {
		if v == nil {
			return nil
		}
		return []string{canonical(v)}
	}
	switch t := v.(type) {
	case map[string]any:
		if segs[0] == "[]" {
			return nil
		}
		x, ok := t[segs[0]]
		if !ok {
			return nil
		}
		return fieldValues(x, segs[1:])
	case []any:
		if segs[0] != "[]" {
			return nil
		}
		var out []string
		for _, e := range t {
			out = append(out, fieldValues(e, segs[1:])...)
		}
		return out
	}
	return nil
}

// --- validations, relations, gaps -------------------------------------------------------

func (db *dossierBuilder) validations() {
	var pass, fail bool
	for _, v := range db.in.Validations {
		if v.ChangeItem != db.it.ID {
			continue
		}
		pass = pass || v.Outcome == domain.ValidationPass
		fail = fail || v.Outcome == domain.ValidationFail
		if len(db.d.Validations) == dossierValidations {
			db.d.Truncated = append(db.d.Truncated, "validations: more")
		}
		if len(db.d.Validations) >= dossierValidations {
			continue
		}
		dv := DossierValidation{ID: v.ID, Validator: v.Validator, Kind: string(v.Kind), Level: string(v.Level), Outcome: string(v.Outcome),
			ClearsBehaviour: v.ClearsBehaviour(), Detail: clip(v.Detail, dossierExcerpt)}
		if p := v.Pin; p != nil {
			dv.Pin = clip(fmt.Sprintf("%s %s %s = %s", p.Kind, p.Subject, p.Path, p.Value), dossierValue)
		}
		db.d.Validations = append(db.d.Validations, dv)
		if v.Outcome == domain.ValidationError || v.Outcome == domain.ValidationNotRun {
			db.gap(GapPartial, "validation "+v.ID, fmt.Sprintf("%s did not decide (%s): %s", v.Validator, v.Outcome, v.Detail))
		}
	}
	if pass && fail {
		db.gap(GapContradictory, "validations", "validations of the item both pass and fail")
	}
}

func (db *dossierBuilder) relations() {
	t := db.in.Traversal
	if t == nil {
		db.gap(GapMissing, "relations", "no traversal: consumers of the changed objects are not known")
		return
	}
	keys := map[string]string{}
	edges := map[string]domain.GraphEdge{}
	if g := db.in.Graph; g != nil {
		for _, n := range g.Nodes {
			keys[n.ID] = n.Key
		}
		for _, e := range g.Edges {
			edges[e.ID] = e
		}
	}
	r := &db.d.Relations
	for _, x := range t.RootsOf(db.it.ID) {
		r.Roots++
		db.d.Roots = append(db.d.Roots, DossierRoot{Node: x.Node, Key: keys[x.Node], Rule: x.Rule})
	}
	sort.Slice(db.d.Roots, func(i, j int) bool { return db.d.Roots[i].Node < db.d.Roots[j].Node })
	if len(db.d.Roots) > dossierObjects {
		db.d.Truncated = append(db.d.Truncated, fmt.Sprintf("roots: %d more (list_paths)", len(db.d.Roots)-dossierObjects))
		db.d.Roots = db.d.Roots[:dossierObjects]
	}
	var un []string
	for _, p := range t.PathsOf(db.it.ID) {
		r.Paths++
		if !p.Status.Resolved() {
			r.Unresolved++
			for _, e := range p.Edges {
				if x, ok := edges[e]; ok && x.Status != domain.EdgeEstablished {
					un = append(un, fmt.Sprintf("%s %s %s → %s (%s): %s", x.ID, x.Kind, keys[x.From], keys[x.To], x.Status, clip(x.Reason, 160)))
				}
			}
		}
	}
	un = uniqSorted(un)
	if len(un) > dossierUnresolved {
		db.d.Truncated = append(db.d.Truncated, fmt.Sprintf("unresolved edges: %d more (list_paths unresolved=true)", len(un)-dossierUnresolved))
		un = un[:dossierUnresolved]
	}
	r.Edges = un
	r.Truncated = t.Truncated
	if t.Truncated {
		db.gap(GapPartial, "relations", "the traversal hit its bound: "+t.TruncationReason)
	}
	if r.Unresolved > 0 {
		db.gap(GapPartial, "relations", fmt.Sprintf("%d of %d impact paths have an inferred or missing edge", r.Unresolved, r.Paths))
	}
}

func (db *dossierBuilder) gaps() {
	if len(db.b.Renders) == 0 {
		db.gap(GapMissing, "environment render", "no environment render: the deployed configuration before and after is not known")
	}
	uncorrelated := 0
	for _, p := range db.b.Renders {
		if p.Status != domain.CollectionOK && p.Status != domain.CollectionNotApplicable {
			kind := GapPartial
			if p.Status == domain.CollectionUnavailable {
				kind = GapMissing
			}
			db.gap(kind, "render "+p.Target, fmt.Sprintf("%s: %s", p.Status, p.Reason))
		}
		for _, c := range p.Changes {
			if len(c.ChangeItems) == 0 {
				uncorrelated++
			}
		}
	}
	if uncorrelated > 0 {
		db.gap(GapPartial, "rendered changes", fmt.Sprintf("%d environment rendered change(s) no change item explains", uncorrelated))
	}
	for _, s := range db.b.Change.Sources {
		switch s.Status {
		case domain.CollectionPartial:
			db.gap(GapPartial, "source "+string(s.Source), s.Reason)
		case domain.CollectionUnavailable:
			db.gap(GapMissing, "source "+string(s.Source), s.Reason)
		}
	}
	sort.SliceStable(db.d.Gaps, func(i, j int) bool {
		if db.d.Gaps[i].Kind != db.d.Gaps[j].Kind {
			return gapRank(db.d.Gaps[i].Kind) < gapRank(db.d.Gaps[j].Kind)
		}
		return db.d.Gaps[i].Subject < db.d.Gaps[j].Subject
	})
	if len(db.d.Gaps) > dossierGaps {
		db.d.Truncated = append(db.d.Truncated, fmt.Sprintf("gaps: %d more", len(db.d.Gaps)-dossierGaps))
		db.d.Gaps = db.d.Gaps[:dossierGaps]
	}
}

func gapRank(k string) int {
	switch k {
	case GapContradictory:
		return 0
	case GapMissing:
		return 1
	}
	return 2
}

// bound trims the largest lists until the encoded dossier fits.
func (db *dossierBuilder) bound() {
	d := db.d
	for i := 0; i < 32; i++ {
		data, err := d.Encode()
		if err != nil || len(data) <= MaxDossierBytes {
			return
		}
		lists := []struct {
			name string
			n    int
			cut  func(int)
		}{
			{"exposed objects", len(d.Exposed), func(k int) { d.Exposed = d.Exposed[:k] }},
			{"rendered changes", len(d.RenderedChanges), func(k int) { d.RenderedChanges = d.RenderedChanges[:k] }},
			{"unchanged settings", len(d.Unchanged), func(k int) { d.Unchanged = d.Unchanged[:k] }},
			{"roots", len(d.Roots), func(k int) { d.Roots = d.Roots[:k] }},
			{"knowledge facts", len(d.Knowledge), func(k int) { d.Knowledge = d.Knowledge[:k] }},
			{"upstream evidence", len(d.Change.Upstream), func(k int) { d.Change.Upstream = d.Change.Upstream[:k] }},
		}
		best := 0
		for j := range lists {
			if lists[j].n > lists[best].n {
				best = j
			}
		}
		l := lists[best]
		if l.n <= 1 {
			d.Change.Detail = clip(d.Change.Detail, len(d.Change.Detail)/2)
			d.Truncated = append(d.Truncated, "change detail clipped to fit")
			continue
		}
		l.cut(l.n / 2)
		d.Truncated = append(d.Truncated, fmt.Sprintf("%s: cut to %d to fit %d bytes", l.name, l.n/2, MaxDossierBytes))
	}
}
