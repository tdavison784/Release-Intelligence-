package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Phase 4 contract: environment-aware breaking-change investigation
// (docs/phase4/env-impact/DESIGN.md §2). The types below sit ON TOP of the
// existing pipeline: a ChangeEvidenceRecord references the UpgradeEdge's
// Changes and Evidence by id, an EnvironmentSnapshot pins the inputs env.Load
// parses, and a finding references the deterministic ImpactReport's findings.
//
// The centerpiece evidence (product-owner clarification, MISSION.md) is the
// rendered before/after DIFF of the environment's own configuration
// (internal/render): every rendered change is a citable RenderedChange,
// correlated to the change items it realises. The relationship graph and its
// bounded traversal are lightweight supporting context for consumers the
// render cannot show.
//
// Separation (MISSION P4, AC6): ValidationRecords are produced by
// deterministic validators (or an observed runtime outcome); AgentAssertions,
// Hypotheses and requests carry AI provenance and are never validation
// results. InvestigationFindings are computed by the deterministic policy
// (DESIGN.md §3) and never carry AI provenance.

// Serialisation versions.
const (
	EvidenceBundleSchemaVersion = "ri.dev/evidence-bundle/v1alpha1"
	InvestigationSchemaVersion  = "ri.dev/investigation/v1alpha1"
)

// Id prefixes. Every id is content-derived (ShortHash over the fields that
// define the record's identity), so re-running a stage is idempotent.
const (
	ChangeRecordIDPrefix         = "cer-"
	RenderedChangeIDPrefix       = "rc-"
	ChangeItemIDPrefix           = "ci-"
	SnapshotIDPrefix             = "snap-"
	ArtifactIDPrefix             = "art-"
	BundleIDPrefix               = "bnd-"
	GraphIDPrefix                = "grf-"
	NodeIDPrefix                 = "gn-"
	EdgeIDPrefix                 = "ge-"
	ImpactPathIDPrefix           = "ip-"
	TraversalIDPrefix            = "trv-"
	ValidationRecordIDPrefix     = "vr-"
	HypothesisIDPrefix           = "hyp-"
	AssertionIDPrefix            = "asr-"
	EvidenceRequestIDPrefix      = "erq-"
	ValidationRequestIDPrefix    = "vrq-"
	InvestigationFindingIDPrefix = "inf-"
	InvestigationIDPrefix        = "inv-"
	AgentNoteIDPrefix            = "ntn-" // CONTRACT-CHANGE(agent): investigation-level agent notes
	SettingClaimIDPrefix         = "clm-" // CONTRACT-CHANGE(agent): verified environment claims
)

// --- collection status ------------------------------------------------------------------

// CollectionStatus is the outcome of collecting one evidence source or one
// environment input. Anything but ok/not-applicable is a gap that blocks an
// automatic safe determination for the change items that depend on it.
type CollectionStatus string

const (
	CollectionOK            CollectionStatus = "ok"
	CollectionPartial       CollectionStatus = "partial"
	CollectionUnavailable   CollectionStatus = "unavailable"
	CollectionNotApplicable CollectionStatus = "not-applicable"
)

// CollectionStatuses lists every status.
var CollectionStatuses = []CollectionStatus{CollectionOK, CollectionPartial, CollectionUnavailable, CollectionNotApplicable}

// Valid reports whether s is a known status.
func (s CollectionStatus) Valid() bool { return oneOf(s, CollectionStatuses) }

// Gap reports whether the status leaves something uncollected.
func (s CollectionStatus) Gap() bool { return s == CollectionPartial || s == CollectionUnavailable }

func validateCollection(what string, s CollectionStatus, reason string) []error {
	var errs []error
	if !s.Valid() {
		errs = append(errs, fmt.Errorf("%s: unknown collection status %q", what, s))
	}
	if s != CollectionOK && strings.TrimSpace(reason) == "" {
		errs = append(errs, fmt.Errorf("%s: status %s requires a reason", what, s))
	}
	return errs
}

// --- artifacts, manifest, bundle (P1) ------------------------------------------------------

// ArtifactKind classifies the bytes of an artifact.
type ArtifactKind string

const (
	ArtifactUpgradeEdge       ArtifactKind = "upgrade-edge"       // the domain.UpgradeEdge JSON the change record references
	ArtifactUpstreamDocument  ArtifactKind = "upstream-document"  // release notes, changelog, upgrade guide, linked PR text
	ArtifactSourceDiff        ArtifactKind = "source-diff"        // a commit/compare diff of the upstream source
	ArtifactChartArchive      ArtifactKind = "chart-archive"      // a packaged Helm chart
	ArtifactChartValues       ArtifactKind = "chart-values"       // a chart's default values.yaml
	ArtifactCRDDefinition     ArtifactKind = "crd"                // a CustomResourceDefinition document
	ArtifactRenderedManifests ArtifactKind = "rendered-manifests" // a helm template / kustomize build output stream
	ArtifactRenderDiff        ArtifactKind = "render-diff"        // a semantic diff of two renders (render.DiffResult JSON)
	ArtifactEnvironmentFile   ArtifactKind = "environment-file"   // a file of the environment (values, manifest, Application, HelmRelease, ...)
	ArtifactEnvironmentFacts  ArtifactKind = "environment-facts"  // the parsed env.Environment JSON
	ArtifactInventory         ArtifactKind = "inventory"          // a product inventory
	ArtifactLiveObservation   ArtifactKind = "live-observation"   // read-only objects observed in a live cluster
	ArtifactImpactReport      ArtifactKind = "impact-report"      // the deterministic domain.ImpactReport JSON
	ArtifactKnowledgeFact     ArtifactKind = "knowledge-fact"     // a domain.VerifiedFact JSON
	ArtifactGraph             ArtifactKind = "graph"              // a RelationshipGraph JSON
	ArtifactValidationOutput  ArtifactKind = "validation-output"  // raw output of a validator run
	ArtifactAgentTranscript   ArtifactKind = "agent-transcript"   // the recorded tool calls/results of an agent run (JSONL)
	// ArtifactItemDossier is the change-scoped impact dossier of one item: the
	// agent's initial context, assembled deterministically from the bundle and
	// the run. CONTRACT-CHANGE(verdict): additive.
	ArtifactItemDossier ArtifactKind = "item-dossier"
)

// ArtifactKinds lists every kind.
var ArtifactKinds = []ArtifactKind{
	ArtifactUpgradeEdge, ArtifactUpstreamDocument, ArtifactSourceDiff, ArtifactChartArchive, ArtifactChartValues,
	ArtifactCRDDefinition, ArtifactRenderedManifests, ArtifactRenderDiff, ArtifactEnvironmentFile,
	ArtifactEnvironmentFacts, ArtifactInventory, ArtifactLiveObservation, ArtifactImpactReport,
	ArtifactKnowledgeFact, ArtifactGraph, ArtifactValidationOutput, ArtifactAgentTranscript, ArtifactItemDossier,
}

// ArtifactOrigin says where an artifact's bytes come from. It decides the
// logical path prefix: upstream/, env/ or derived/.
type ArtifactOrigin string

const (
	OriginUpstream    ArtifactOrigin = "upstream"    // fetched from the product's published sources
	OriginEnvironment ArtifactOrigin = "environment" // read from the target environment (repo files, flags, live cluster)
	OriginDerived     ArtifactOrigin = "derived"     // computed by a component from other artifacts (renders, graphs, reports)
)

// ArtifactOrigins lists every origin.
var ArtifactOrigins = []ArtifactOrigin{OriginUpstream, OriginEnvironment, OriginDerived}

// PathPrefix is the logical path prefix of artifacts of this origin.
func (o ArtifactOrigin) PathPrefix() string {
	if o == OriginEnvironment {
		return "env/"
	}
	return string(o) + "/"
}

// ArtifactRef identifies one stored artifact by the digest of its bytes. The
// bytes live in the content-addressed store (DESIGN.md §2.3:
// cas/sha256/<hex>); the ref is what the manifest lists, what the agent
// searches and what findings cite.
type ArtifactRef struct {
	ID        string       `json:"id"` // ArtifactID(kind, path, digest)
	Kind      ArtifactKind `json:"kind"`
	MediaType string       `json:"mediaType"` // e.g. "application/yaml", "text/markdown", "application/json"
	Digest    string       `json:"digest"`    // sha256:<hex> of the stored bytes
	Size      int64        `json:"size"`
	// Path is the logical path inside the bundle: "<origin>/..." — e.g.
	// "upstream/cert-manager/v1.18.0/release-notes.md",
	// "env/apps/cert-manager/values.yaml", "derived/render/env/to.yaml".
	Path   string         `json:"path"`
	Origin ArtifactOrigin `json:"origin"`
	// URI is where the bytes came from: an upstream URL, the environment
	// path as supplied, or "ri:<producer>" for derived artifacts. Evidence
	// that cites the artifact uses it as its URI.
	URI      string `json:"uri"`
	Producer string `json:"producer"` // component@vN that collected or derived it
	// Redacted marks bytes stored in redacted form (Kubernetes Secret data,
	// sensitive values; DESIGN.md §2.3). The digest is of the redacted bytes.
	Redacted bool `json:"redacted,omitempty"`
	// Tags are lowercased search keywords (kind, product, object kinds and
	// names, values keys, ...) for the agent's manifest search.
	Tags []string `json:"tags,omitempty"`
	// ChangeItems are the change items the artifact relates to, when known.
	ChangeItems []string `json:"changeItems,omitempty"`
}

// ArtifactID derives an artifact id.
func ArtifactID(kind ArtifactKind, path, digest string) string {
	return ArtifactIDPrefix + ShortHash(string(kind), path, digest)
}

// Validate checks identity and shape of the ref.
func (a ArtifactRef) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("artifact %s: %s", a.ID, fmt.Sprintf(format, args...)))
	}
	if !oneOf(a.Kind, ArtifactKinds) {
		bad("unknown kind %q", a.Kind)
	}
	if !oneOf(a.Origin, ArtifactOrigins) {
		bad("unknown origin %q", a.Origin)
	} else if !strings.HasPrefix(a.Path, a.Origin.PathPrefix()) {
		bad("path %q must start with %q", a.Path, a.Origin.PathPrefix())
	}
	if strings.Contains(a.Path, "..") || strings.HasPrefix(a.Path, "/") {
		bad("path %q must be relative and must not contain ..", a.Path)
	}
	if !isDigest(a.Digest) {
		bad("digest %q is not sha256:<64 hex>", a.Digest)
	}
	if a.ID != ArtifactID(a.Kind, a.Path, a.Digest) {
		bad("id is not derived from kind+path+digest")
	}
	if a.Size < 0 {
		bad("negative size")
	}
	if a.MediaType == "" || a.URI == "" || a.Producer == "" {
		bad("mediaType, uri and producer are required")
	}
	return errors.Join(errs...)
}

// BundleManifest is the searchable index of a bundle's artifacts (MISSION
// §A4: the agent searches the manifest, it never receives the whole bundle).
// Artifacts are sorted by path; ids and paths are unique.
type BundleManifest struct {
	Artifacts []ArtifactRef `json:"artifacts"`
}

// Digest pins the manifest: sha256 over its canonical JSON.
func (m BundleManifest) Digest() string { return canonicalDigest(m) }

// Lookup returns the artifact with the given id.
func (m BundleManifest) Lookup(id string) (ArtifactRef, bool) {
	for _, a := range m.Artifacts {
		if a.ID == id {
			return a, true
		}
	}
	return ArtifactRef{}, false
}

// Validate checks every ref, uniqueness and ordering.
func (m BundleManifest) Validate() error {
	var errs []error
	ids, paths := map[string]bool{}, map[string]bool{}
	for i, a := range m.Artifacts {
		if err := a.Validate(); err != nil {
			errs = append(errs, err)
		}
		if ids[a.ID] {
			errs = append(errs, fmt.Errorf("manifest: duplicate artifact id %s", a.ID))
		}
		if paths[a.Path] {
			errs = append(errs, fmt.Errorf("manifest: duplicate path %s", a.Path))
		}
		ids[a.ID], paths[a.Path] = true, true
		if i > 0 && m.Artifacts[i-1].Path >= a.Path {
			errs = append(errs, fmt.Errorf("manifest: artifacts must be sorted by path (%s before %s)", m.Artifacts[i-1].Path, a.Path))
		}
	}
	return errors.Join(errs...)
}

// --- change evidence (§A1) ----------------------------------------------------------------

// ChangeSourceKind is one class of change evidence MISSION §A1 lists. A
// ChangeEvidenceRecord states a status for every kind, so an absent source is
// a recorded gap, never a silent omission.
type ChangeSourceKind string

const (
	ChangeSourceReleaseNotes      ChangeSourceKind = "release-notes"      // changelogs and release notes
	ChangeSourceMigrationGuide    ChangeSourceKind = "migration-guide"    // upgrade / migration guides
	ChangeSourceSourceDiff        ChangeSourceKind = "source-diff"        // source commits and diffs
	ChangeSourceAPISchema         ChangeSourceKind = "api-schema"         // CRD / API schema changes
	ChangeSourceChart             ChangeSourceKind = "chart"              // Helm chart values and templates
	ChangeSourceKustomize         ChangeSourceKind = "kustomize"          // upstream Kustomize bases
	ChangeSourceRenderedManifests ChangeSourceKind = "rendered-manifests" // release-scope before/after renders
	ChangeSourceCompatibility     ChangeSourceKind = "compatibility"      // platform / peer compatibility requirements
	ChangeSourceAdvisories        ChangeSourceKind = "advisories"         // security advisories
)

// ChangeSourceKinds lists every kind; a record states each exactly once.
var ChangeSourceKinds = []ChangeSourceKind{
	ChangeSourceReleaseNotes, ChangeSourceMigrationGuide, ChangeSourceSourceDiff, ChangeSourceAPISchema,
	ChangeSourceChart, ChangeSourceKustomize, ChangeSourceRenderedManifests, ChangeSourceCompatibility,
	ChangeSourceAdvisories,
}

// ChangeSourceStatus is the collection status of one change-evidence kind.
type ChangeSourceStatus struct {
	Source    ChangeSourceKind `json:"source"`
	Status    CollectionStatus `json:"status"`
	Reason    string           `json:"reason,omitempty"`    // required unless ok
	Artifacts []string         `json:"artifacts,omitempty"` // artifact ids collected for this source
}

// ChangeItemFamily groups change items by how their exposure can be decided
// (DESIGN.md §3.2): which environment inputs decide them and which
// validators can test them.
type ChangeItemFamily string

const (
	FamilyValues        ChangeItemFamily = "values"        // Helm values keys/defaults
	FamilyAPISchema     ChangeItemFamily = "api-schema"    // CRDs / API versions / schema fields
	FamilyImage         ChangeItemFamily = "image"         // container images
	FamilyCompatibility ChangeItemFamily = "compatibility" // platform / peer version bounds
	FamilyRendered      ChangeItemFamily = "rendered"      // a release-scope rendered change (RBAC, resources, args, ...)
	FamilyBehavior      ChangeItemFamily = "behavior"      // runtime behaviour stated in prose; not render-verifiable
	FamilyProcedure     ChangeItemFamily = "procedure"     // an upgrade procedure / migration step
)

// ChangeItemFamilies lists every family.
var ChangeItemFamilies = []ChangeItemFamily{
	FamilyValues, FamilyAPISchema, FamilyImage, FamilyCompatibility, FamilyRendered, FamilyBehavior, FamilyProcedure,
}

// Manifestation says where a change item takes effect, which decides whether
// a silent render can clear it (DESIGN.md §2.2.1). Derived deterministically
// at collection time from existing evidence by a declared rule
// (ChangeItem.ManifestationRule), never by product-specific code.
type Manifestation string

const (
	// ManifestRenderedConfig: the change takes effect in declared
	// configuration — chart templates/values, CRD schema structure, image
	// references, platform version bounds. Configuration evidence (the
	// render diff, the manifests, the cluster version) is decisive.
	ManifestRenderedConfig Manifestation = "rendered-config"
	// ManifestControllerBehaviour: the change is runtime/default semantics
	// of the shipped software (a default compiled into the controller or
	// webhook binary, server-side defaulting). In a render it appears at most
	// as description text: render silence is NOT decisive.
	ManifestControllerBehaviour Manifestation = "controller-behaviour"
	// ManifestUnknown: no rule decided; treated as controller-behaviour
	// (fail closed).
	ManifestUnknown Manifestation = "unknown"
)

// Manifestations lists every manifestation.
var Manifestations = []Manifestation{ManifestRenderedConfig, ManifestControllerBehaviour, ManifestUnknown}

// RenderDecisive reports whether a complete render showing nothing
// attributable can count as evidence that the item does not apply.
func (m Manifestation) RenderDecisive() bool { return m == ManifestRenderedConfig }

// ChangeItem is one structured breaking-change item. It REFERENCES the
// upgrade edge (ChangeID / Platform) instead of copying it; Title and the
// flags are copies so the record renders standalone.
type ChangeItem struct {
	ID string `json:"id"` // ChangeItemID(changeID, platform)
	// Exactly one anchor is set. ChangeID is the domain.Change id in the
	// referenced UpgradeEdge; Platform names an UpgradeEdge.Compatibility
	// entry (e.g. "kubernetes"); RenderedChange names a release-scope
	// rendered change no Change accounts for (an undocumented chart change,
	// family rendered).
	ChangeID       string           `json:"changeId,omitempty"`
	Platform       string           `json:"platform,omitempty"`
	RenderedChange string           `json:"renderedChange,omitempty"`
	Family         ChangeItemFamily `json:"family"`
	Title          string           `json:"title"`
	Breaking       bool             `json:"breaking,omitempty"`
	ActionRequired bool             `json:"actionRequired,omitempty"`
	Subjects       []string         `json:"subjects,omitempty"` // copied from the Change (keys, CRDs, images, APIs)
	// Evidence is the upstream chain (resolves in the bundle evidence pool).
	Evidence  []EvidenceID `json:"evidence"`
	Artifacts []string     `json:"artifacts,omitempty"`
	// Manifestation says where the item takes effect; ManifestationRule is
	// the declared rule that decided it (DESIGN.md §2.2.1), e.g.
	// "manifest:computed-values", "manifest:declared-behaviour".
	Manifestation     Manifestation `json:"manifestation"`
	ManifestationRule string        `json:"manifestationRule"`
	// InScope says whether the investigation must classify the item; the
	// rule that decided it is ScopeRule (DESIGN.md §1.3), e.g.
	// "scope:breaking", "scope:routine-excluded".
	InScope   bool   `json:"inScope"`
	ScopeRule string `json:"scopeRule"`
}

// ChangeItemID derives a change item id from its anchor.
func ChangeItemID(changeID, platform, renderedChange string) string {
	return ChangeItemIDPrefix + ShortHash(changeID, platform, renderedChange)
}

// Scope rules of items anchored on a rendered change (DESIGN.md §1.3, incl.
// the commander amendment of 2026-10-16).
// CONTRACT-CHANGE(renderdiff): the two amendment rules are new; additive.
const (
	ScopeRenderedStructural = "scope:rendered-structural"
	// ScopeRenderedUnexplained: a rendered change (release or environment
	// scope) nothing upstream explains is its own in-scope item.
	ScopeRenderedUnexplained = "scope:rendered-unexplained"
	// ScopeRoutineSelfArtifact: a rendered change that is only the version
	// bump of an artifact the product definition itself publishes is an
	// out-of-scope item, so the change is correlated.
	ScopeRoutineSelfArtifact = "scope:routine-self-artifact"
	// ScopeDocumentation: a crd-description-changed rendered change (CRD
	// schema documentation text) that no change item correlates is an
	// out-of-scope item, so the change is correlated (never "uncorrelated")
	// and never an in-scope item of its own: documentation configures
	// nothing (DESIGN.md §1.3).
	// CONTRACT-CHANGE(fixes2): additive, mirrors ScopeRoutineSelfArtifact.
	ScopeDocumentation = "scope:documentation"
	// RenderedClassDocumentation is the render class ScopeDocumentation
	// items anchor on (render.CRDDescriptionChanged; domain cannot import
	// render).
	RenderedClassDocumentation = "crd-description-changed"
)

// renderedScopeErrors checks the amendment rules' shape: anchored on a
// rendered change, family rendered, manifestation rendered-config;
// rendered-unexplained in scope, routine-self-artifact and documentation out
// of scope.
func (it ChangeItem) renderedScopeErrors() []string {
	if it.ScopeRule != ScopeRenderedUnexplained && it.ScopeRule != ScopeRoutineSelfArtifact && it.ScopeRule != ScopeDocumentation {
		return nil
	}
	var out []string
	if it.RenderedChange == "" {
		out = append(out, it.ScopeRule+" requires a rendered-change anchor")
	}
	if it.Family != FamilyRendered || it.Manifestation != ManifestRenderedConfig {
		out = append(out, it.ScopeRule+" requires family rendered and manifestation rendered-config")
	}
	if it.InScope != (it.ScopeRule == ScopeRenderedUnexplained) {
		out = append(out, fmt.Sprintf("%s requires inScope=%v", it.ScopeRule, it.ScopeRule == ScopeRenderedUnexplained))
	}
	return out
}

func (it ChangeItem) anchors() int {
	n := 0
	for _, a := range []string{it.ChangeID, it.Platform, it.RenderedChange} {
		if a != "" {
			n++
		}
	}
	return n
}

// ChangeEvidenceRecord is the structured breaking-change evidence of one
// upgrade (MISSION §A1). It is built from an UpgradeEdge (stored in the bundle
// as an upgrade-edge artifact) plus the sources the edge does not carry.
type ChangeEvidenceRecord struct {
	ID      string     `json:"id"`
	Product ProductRef `json:"product"`
	From    Version    `json:"from"`
	To      Version    `json:"to"`
	// Edge is the artifact id of the UpgradeEdge the items reference.
	Edge  string       `json:"edge"`
	Items []ChangeItem `json:"items"`
	// MigrationNotes cite the migration-guide statements that apply to the
	// upgrade (upstream evidence ids).
	MigrationNotes []EvidenceID `json:"migrationNotes,omitempty"`
	// Renders are release-scope (chart-default) before/after renders.
	Renders []ManifestPair       `json:"renders,omitempty"`
	Sources []ChangeSourceStatus `json:"sources"`
	// Producer is the collector, component@vN.
	Producer string `json:"producer"`
}

// DerivedID returns the content-derived id of the record.
func (r ChangeEvidenceRecord) DerivedID() string {
	parts := []string{string(r.Product.ID), r.From.Tag, r.To.Tag, r.Edge}
	for _, it := range r.Items {
		parts = append(parts, it.ID, fmt.Sprint(it.InScope), string(it.Manifestation), it.ManifestationRule)
	}
	for _, s := range r.Sources {
		parts = append(parts, string(s.Source), string(s.Status), strings.Join(s.Artifacts, ","))
	}
	for _, p := range r.Renders {
		parts = append(parts, p.key()...)
	}
	return ChangeRecordIDPrefix + ShortHash(parts...)
}

// Item returns the change item with the given id.
func (r ChangeEvidenceRecord) Item(id string) (ChangeItem, bool) {
	for _, it := range r.Items {
		if it.ID == id {
			return it, true
		}
	}
	return ChangeItem{}, false
}

// InScope returns the ids of the in-scope items, in record order.
func (r ChangeEvidenceRecord) InScope() []string {
	var out []string
	for _, it := range r.Items {
		if it.InScope {
			out = append(out, it.ID)
		}
	}
	return out
}

// SourceStatus returns the status recorded for a source kind.
func (r ChangeEvidenceRecord) SourceStatus(k ChangeSourceKind) (ChangeSourceStatus, bool) {
	for _, s := range r.Sources {
		if s.Source == k {
			return s, true
		}
	}
	return ChangeSourceStatus{}, false
}

// Validate checks identity, items and the per-source collection status.
func (r ChangeEvidenceRecord) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("change record %s: %s", r.ID, fmt.Sprintf(format, args...)))
	}
	if r.ID != r.DerivedID() {
		bad("id is not derived from its content")
	}
	if r.Product.ID == "" || r.From.Tag == "" || r.To.Tag == "" {
		bad("product, from and to are required")
	}
	if !strings.HasPrefix(r.Edge, ArtifactIDPrefix) {
		bad("edge must be an artifact id")
	}
	if r.Producer == "" {
		bad("producer is required")
	}
	releaseChanges := map[string]bool{}
	releaseClass := map[string]string{}
	for _, p := range r.Renders {
		for _, c := range p.Changes {
			releaseChanges[c.ID] = true
			releaseClass[c.ID] = c.Class
		}
	}
	seen := map[string]bool{}
	for _, it := range r.Items {
		if seen[it.ID] {
			bad("duplicate item %s", it.ID)
		}
		seen[it.ID] = true
		if it.anchors() != 1 {
			bad("item %s: exactly one of changeId, platform and renderedChange is required", it.ID)
		}
		if it.ID != ChangeItemID(it.ChangeID, it.Platform, it.RenderedChange) {
			bad("item %s: id is not derived from its anchor", it.ID)
		}
		if it.RenderedChange != "" {
			if !releaseChanges[it.RenderedChange] {
				bad("item %s: rendered change %s is not a release-scope rendered change of this record", it.ID, it.RenderedChange)
			}
			if it.Family != FamilyRendered {
				bad("item %s: a rendered-change item has family rendered", it.ID)
			}
			if it.ScopeRule == ScopeDocumentation && releaseChanges[it.RenderedChange] && releaseClass[it.RenderedChange] != RenderedClassDocumentation {
				bad("item %s: %s is only for a %s rendered change, not %q", it.ID, ScopeDocumentation, RenderedClassDocumentation, releaseClass[it.RenderedChange])
			}
		}
		for _, e := range it.renderedScopeErrors() {
			bad("item %s: %s", it.ID, e)
		}
		if !oneOf(it.Family, ChangeItemFamilies) {
			bad("item %s: unknown family %q", it.ID, it.Family)
		}
		if strings.TrimSpace(it.Title) == "" || it.ScopeRule == "" {
			bad("item %s: title and scopeRule are required", it.ID)
		}
		if !oneOf(it.Manifestation, Manifestations) {
			bad("item %s: unknown manifestation %q", it.ID, it.Manifestation)
		}
		if !strings.HasPrefix(it.ManifestationRule, "manifest:") {
			bad("item %s: manifestationRule must name the declared rule (manifest:...)", it.ID)
		}
		if len(it.Evidence) == 0 {
			bad("item %s: upstream evidence is required", it.ID)
		}
	}
	got := map[ChangeSourceKind]int{}
	for _, s := range r.Sources {
		got[s.Source]++
		if !oneOf(s.Source, ChangeSourceKinds) {
			bad("unknown source kind %q", s.Source)
		}
		for _, err := range validateCollection(fmt.Sprintf("source %s", s.Source), s.Status, s.Reason) {
			bad("%v", err)
		}
		if s.Status == CollectionOK && len(s.Artifacts) == 0 {
			bad("source %s: status ok requires the collected artifacts", s.Source)
		}
	}
	for _, k := range ChangeSourceKinds {
		if got[k] != 1 {
			bad("source %s must be stated exactly once (got %d): an absent source is a recorded gap", k, got[k])
		}
	}
	for _, p := range r.Renders {
		if err := p.validate(RenderRelease); err != nil {
			bad("%v", err)
		}
	}
	return errors.Join(errs...)
}

// ManifestPair is one before/after render (artifact ids) and its diff.
type ManifestPair struct {
	Scope RenderScope `json:"scope"`
	// Target names the render target, e.g. "chart-defaults",
	// "argocd:apps/cert-manager.yaml", "kustomize:overlays/prod".
	Target string           `json:"target"`
	From   string           `json:"from,omitempty"` // rendered-manifests artifact of the from version
	To     string           `json:"to,omitempty"`   // rendered-manifests artifact of the to version
	Diff   string           `json:"diff,omitempty"` // render-diff artifact
	Status CollectionStatus `json:"status"`
	Reason string           `json:"reason,omitempty"` // required unless ok (render.FailureReason + detail)
	// Changes index the diff: one citable record per semantic rendered
	// change, sorted by id. Only a complete (ok) pair has changes; an empty
	// list on an ok pair means "rendered, nothing changed".
	Changes []RenderedChange `json:"changes,omitempty"`
}

func (p ManifestPair) key() []string {
	parts := []string{string(p.Scope), p.Target, p.From, p.To, p.Diff, string(p.Status)}
	for _, c := range p.Changes {
		parts = append(parts, c.ID, strings.Join(c.ChangeItems, ","))
	}
	return parts
}

// RenderedChange is one semantic change between the from and to renders of
// a ManifestPair (a render.Change, summarised for citation; the full record
// is in the pair's render-diff artifact). ChangeItems are the change items
// the render correlation attributes it to (render.Correlate); an
// environment-scope change with none is UNCORRELATED — a change the
// customer's deployment receives that no known change item explains.
type RenderedChange struct {
	ID string `json:"id"` // RenderedChangeID(scope, target, object, path, class)
	// Class is the render change class (internal/render ChangeClass, e.g.
	// "rbac-permission-removed", "image-changed", "field-changed").
	Class  string `json:"class"`
	Object string `json:"object"`         // <group>/<version>/<kind>/<namespace>/<name>
	Path   string `json:"path,omitempty"` // field path inside the object ("" = the whole object)
	// Before/After are JSON-encoded values at Path; never set when Sensitive.
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
	Sensitive bool   `json:"sensitive,omitempty"`
	// ChangeItems the change is attributed to, sorted; Correlation is the
	// rule that attributed it (render ChangeLink rule), set exactly when
	// ChangeItems is non-empty.
	ChangeItems []string     `json:"changeItems,omitempty"`
	Correlation string       `json:"correlation,omitempty"`
	Evidence    []EvidenceID `json:"evidence,omitempty"` // rendered-diff evidence records
}

// RenderedChangeID derives a rendered change id.
func RenderedChangeID(scope RenderScope, target, object, path, class string) string {
	return RenderedChangeIDPrefix + ShortHash(string(scope), target, object, path, class)
}

func (p ManifestPair) validate(scope RenderScope) error {
	var errs []error
	if p.Scope != scope {
		errs = append(errs, fmt.Errorf("render %s: scope %q, want %q", p.Target, p.Scope, scope))
	}
	if p.Target == "" {
		errs = append(errs, errors.New("render: target is required"))
	}
	errs = append(errs, validateCollection("render "+p.Target, p.Status, p.Reason)...)
	if p.Status == CollectionOK && (p.From == "" || p.To == "" || p.Diff == "") {
		errs = append(errs, fmt.Errorf("render %s: status ok requires from, to and diff artifacts", p.Target))
	}
	if p.Status != CollectionOK && len(p.Changes) > 0 {
		errs = append(errs, fmt.Errorf("render %s: only a complete (ok) pair indexes rendered changes; an incomplete render is an evidence gap, never a diff", p.Target))
	}
	for i, c := range p.Changes {
		if c.ID != RenderedChangeID(p.Scope, p.Target, c.Object, c.Path, c.Class) {
			errs = append(errs, fmt.Errorf("render %s: rendered change %s: id is not derived from scope+target+object+path+class", p.Target, c.ID))
		}
		if i > 0 && p.Changes[i-1].ID >= c.ID {
			errs = append(errs, fmt.Errorf("render %s: rendered changes must be sorted by id and unique", p.Target))
		}
		if c.Class == "" || c.Object == "" {
			errs = append(errs, fmt.Errorf("render %s: rendered change %s needs class and object", p.Target, c.ID))
		}
		if c.Sensitive && (c.Before != "" || c.After != "") {
			errs = append(errs, fmt.Errorf("render %s: rendered change %s is sensitive; its values are never recorded", p.Target, c.ID))
		}
		if (len(c.ChangeItems) > 0) != (c.Correlation != "") {
			errs = append(errs, fmt.Errorf("render %s: rendered change %s: correlation is set exactly when change items are", p.Target, c.ID))
		}
		if !sort.StringsAreSorted(c.ChangeItems) {
			errs = append(errs, fmt.Errorf("render %s: rendered change %s: changeItems must be sorted", p.Target, c.ID))
		}
	}
	return errors.Join(errs...)
}

// --- environment snapshot (§A2) -----------------------------------------------------------

// SnapshotInputKind is one class of environment input.
type SnapshotInputKind string

const (
	InputGitOpsRepo     SnapshotInputKind = "gitops-repo"     // a repository walked in repo mode (--repo)
	InputValues         SnapshotInputKind = "values"          // an explicit Helm values file
	InputManifests      SnapshotInputKind = "manifests"       // an explicit manifest file/dir
	InputCRDs           SnapshotInputKind = "crds"            // installed CRDs as supplied
	InputImages         SnapshotInputKind = "images"          // an image list
	InputInventory      SnapshotInputKind = "inventory"       // a product inventory
	InputClusterVersion SnapshotInputKind = "cluster-version" // the --kubernetes flag
	InputLiveCluster    SnapshotInputKind = "live-cluster"    // read-only live observation (off by default)
)

// SnapshotInputKinds lists every kind.
var SnapshotInputKinds = []SnapshotInputKind{
	InputGitOpsRepo, InputValues, InputManifests, InputCRDs, InputImages, InputInventory, InputClusterVersion, InputLiveCluster,
}

// SnapshotInput is one environment input and its collection status.
type SnapshotInput struct {
	Kind SnapshotInputKind `json:"kind"`
	// Path is the input as supplied (file, directory or repo root); Value
	// carries a flag value (cluster-version). One of them is set.
	Path      string           `json:"path,omitempty"`
	Value     string           `json:"value,omitempty"`
	Status    CollectionStatus `json:"status"`
	Reason    string           `json:"reason,omitempty"`    // required unless ok (parse errors, valuesFrom outside the repo, ...)
	Artifacts []string         `json:"artifacts,omitempty"` // environment-file artifacts collected for this input
}

// RepoRef pins a GitOps repository.
type RepoRef struct {
	Path string `json:"path"`          // as supplied
	URL  string `json:"url,omitempty"` // remote URL when known
	// Commit is the HEAD commit when the path is a git work tree ("" otherwise);
	// Dirty reports uncommitted changes. TreeDigest is always set: sha256 over
	// the sorted (path, digest) list of every collected file, so the snapshot
	// is pinned even without git.
	Commit     string `json:"commit,omitempty"`
	Dirty      bool   `json:"dirty,omitempty"`
	TreeDigest string `json:"treeDigest"`
}

// LiveObservation records a read-only live-cluster observation (optional,
// off by default; DESIGN.md §1.2).
type LiveObservation struct {
	ObservedAt time.Time `json:"observedAt"`
	Context    string    `json:"context"`              // kube context name
	Namespaces []string  `json:"namespaces,omitempty"` // scope; empty = cluster-scoped kinds only
	Kinds      []string  `json:"kinds"`                // GVKs listed
	Artifact   string    `json:"artifact"`             // live-observation artifact id
}

// EnvironmentSnapshot is the immutable, digest-identified environment
// evidence of one investigation (MISSION §A2). The parsed facts travel by
// reference (an environment-facts artifact: the env.Environment JSON).
type EnvironmentSnapshot struct {
	ID string `json:"id"`
	// Environment is the caller's name for the environment ("prod-eu").
	Environment       string           `json:"environment"`
	Repo              *RepoRef         `json:"repo,omitempty"`
	KubernetesVersion string           `json:"kubernetesVersion,omitempty"`
	Inputs            []SnapshotInput  `json:"inputs"`
	Facts             string           `json:"facts"` // environment-facts artifact id
	Live              *LiveObservation `json:"live,omitempty"`
	Producer          string           `json:"producer"`
	CollectedAt       time.Time        `json:"collectedAt"` // not part of the id
}

// DerivedID returns the content-derived id of the snapshot.
func (s EnvironmentSnapshot) DerivedID() string {
	parts := []string{s.Environment, s.KubernetesVersion, s.Facts}
	if s.Repo != nil {
		parts = append(parts, s.Repo.Commit, fmt.Sprint(s.Repo.Dirty), s.Repo.TreeDigest)
	}
	for _, in := range s.Inputs {
		parts = append(parts, string(in.Kind), in.Path, in.Value, string(in.Status), strings.Join(in.Artifacts, ","))
	}
	if s.Live != nil {
		parts = append(parts, s.Live.Artifact)
	}
	return SnapshotIDPrefix + ShortHash(parts...)
}

// InputStatus returns the weakest status among the inputs of a kind, and
// whether any input of that kind exists.
func (s EnvironmentSnapshot) InputStatus(k SnapshotInputKind) (CollectionStatus, bool) {
	rank := map[CollectionStatus]int{CollectionOK: 0, CollectionNotApplicable: 0, CollectionPartial: 1, CollectionUnavailable: 2}
	var out CollectionStatus
	found := false
	for _, in := range s.Inputs {
		if in.Kind != k {
			continue
		}
		if !found || rank[in.Status] > rank[out] {
			out = in.Status
		}
		found = true
	}
	return out, found
}

// Validate checks identity and the per-input collection status.
func (s EnvironmentSnapshot) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("snapshot %s: %s", s.ID, fmt.Sprintf(format, args...)))
	}
	if s.ID != s.DerivedID() {
		bad("id is not derived from its content")
	}
	if s.Environment == "" || s.Producer == "" {
		bad("environment and producer are required")
	}
	if !strings.HasPrefix(s.Facts, ArtifactIDPrefix) {
		bad("facts must be an environment-facts artifact id")
	}
	if len(s.Inputs) == 0 {
		bad("at least one input is required")
	}
	if s.Repo != nil && !isDigest(s.Repo.TreeDigest) {
		bad("repo treeDigest is required (sha256:<hex>)")
	}
	for i, in := range s.Inputs {
		if !oneOf(in.Kind, SnapshotInputKinds) {
			bad("input %d: unknown kind %q", i, in.Kind)
		}
		if (in.Path == "") == (in.Value == "") {
			bad("input %d (%s): exactly one of path and value is required", i, in.Kind)
		}
		for _, err := range validateCollection(fmt.Sprintf("input %s %s%s", in.Kind, in.Path, in.Value), in.Status, in.Reason) {
			bad("%v", err)
		}
		if in.Kind == InputLiveCluster && s.Live == nil && in.Status == CollectionOK {
			bad("a live-cluster input with status ok requires the live observation record")
		}
	}
	if s.Live != nil {
		if s.Live.ObservedAt.IsZero() || s.Live.Context == "" || len(s.Live.Kinds) == 0 || !strings.HasPrefix(s.Live.Artifact, ArtifactIDPrefix) {
			bad("live observation requires observedAt, context, kinds and its artifact")
		}
	}
	return errors.Join(errs...)
}

// EvidenceBundle is the P1 deliverable: a reproducible, change- and
// environment-scoped, content-addressed evidence bundle. It is immutable
// once written; an investigation's own derived artifacts live in its
// Investigation record.
type EvidenceBundle struct {
	SchemaVersion string               `json:"schemaVersion"`
	ID            string               `json:"id"` // BundleID(change.id, snapshot.id, manifest digest)
	Change        ChangeEvidenceRecord `json:"change"`
	Snapshot      EnvironmentSnapshot  `json:"snapshot"`
	// Renders are environment-scope before/after renders (the customer's
	// configuration at from and to); they depend on change AND environment.
	Renders []ManifestPair `json:"renders,omitempty"`
	// EnvironmentItems are change items anchored on environment-scope
	// rendered changes nothing else explains (scope:rendered-unexplained,
	// scope:routine-self-artifact; DESIGN.md §1.3 amendment). They depend on
	// the environment, so they live in the bundle, not in the change record.
	// CONTRACT-CHANGE(renderdiff): additive.
	EnvironmentItems []ChangeItem   `json:"environmentItems,omitempty"`
	Manifest         BundleManifest `json:"manifest"`
	// Evidence is the pool every id cited by Change, Snapshot and the
	// manifest resolves in: the edge's upstream records and the
	// environment's local-file/input records.
	Evidence  []Evidence `json:"evidence"`
	Producer  string     `json:"producer"`
	CreatedAt time.Time  `json:"createdAt"` // not part of the id
}

// BundleID derives a bundle id.
func BundleID(changeID, snapshotID, manifestDigest string, renders []ManifestPair) string {
	parts := []string{changeID, snapshotID, manifestDigest}
	for _, p := range renders {
		parts = append(parts, p.key()...)
	}
	return BundleIDPrefix + ShortHash(parts...)
}

// Items returns every change item of the bundle: the change record's, then
// the environment-scope items.
func (b *EvidenceBundle) Items() []ChangeItem {
	out := make([]ChangeItem, 0, len(b.Change.Items)+len(b.EnvironmentItems))
	return append(append(out, b.Change.Items...), b.EnvironmentItems...)
}

// Item returns the change item (record or environment scope) with the id.
func (b *EvidenceBundle) Item(id string) (ChangeItem, bool) {
	for _, it := range b.Items() {
		if it.ID == id {
			return it, true
		}
	}
	return ChangeItem{}, false
}

// InScope returns the ids of the in-scope items (record, then environment).
func (b *EvidenceBundle) InScope() []string {
	var out []string
	for _, it := range b.Items() {
		if it.InScope {
			out = append(out, it.ID)
		}
	}
	return out
}

// RenderedChanges indexes every rendered change of the bundle (release and
// environment scope) by id.
func (b *EvidenceBundle) RenderedChanges() map[string]RenderedChange {
	m := map[string]RenderedChange{}
	for _, ps := range [][]ManifestPair{b.Change.Renders, b.Renders} {
		for _, p := range ps {
			for _, c := range p.Changes {
				m[c.ID] = c
			}
		}
	}
	return m
}

// Uncorrelated returns the environment-scope rendered changes no change item
// accounts for, sorted by id. They block automated approval (DESIGN.md §3.4).
func (b *EvidenceBundle) Uncorrelated() []string {
	var out []string
	for _, p := range b.Renders {
		for _, c := range p.Changes {
			if len(c.ChangeItems) == 0 {
				out = append(out, c.ID)
			}
		}
	}
	sort.Strings(out)
	return out
}

// EvidenceByID returns the bundle's evidence index.
func (b *EvidenceBundle) EvidenceByID() map[EvidenceID]Evidence {
	m := make(map[EvidenceID]Evidence, len(b.Evidence))
	for _, e := range b.Evidence {
		m[e.ID] = e
	}
	return m
}

// Validate checks the bundle and every reference inside it: change items'
// evidence and artifacts, source and input artifacts, render artifacts.
func (b *EvidenceBundle) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("bundle %s: %s", b.ID, fmt.Sprintf(format, args...)))
	}
	if b.SchemaVersion != EvidenceBundleSchemaVersion {
		bad("schemaVersion = %q, want %q", b.SchemaVersion, EvidenceBundleSchemaVersion)
	}
	if b.ID != BundleID(b.Change.ID, b.Snapshot.ID, b.Manifest.Digest(), b.Renders) {
		bad("id is not derived from change+snapshot+manifest+renders")
	}
	if b.Producer == "" {
		bad("producer is required")
	}
	for _, err := range []error{b.Change.Validate(), b.Snapshot.Validate(), b.Manifest.Validate()} {
		if err != nil {
			errs = append(errs, err)
		}
	}
	ev := map[EvidenceID]bool{}
	for _, e := range b.Evidence {
		if e.ID == "" || e.URI == "" {
			bad("evidence needs id and uri")
		}
		if ev[e.ID] {
			bad("duplicate evidence %s", e.ID)
		}
		ev[e.ID] = true
	}
	art := func(what, id string, kinds ...ArtifactKind) {
		a, ok := b.Manifest.Lookup(id)
		if !ok {
			bad("%s: artifact %s is not in the manifest", what, id)
			return
		}
		if len(kinds) > 0 && !oneOf(a.Kind, kinds) {
			bad("%s: artifact %s has kind %s, want one of %v", what, id, a.Kind, kinds)
		}
	}
	art("change.edge", b.Change.Edge, ArtifactUpgradeEdge)
	art("snapshot.facts", b.Snapshot.Facts, ArtifactEnvironmentFacts)
	if b.Snapshot.Live != nil {
		art("snapshot.live", b.Snapshot.Live.Artifact, ArtifactLiveObservation)
	}
	for _, it := range b.Items() {
		for _, id := range it.Evidence {
			if !ev[id] {
				bad("item %s: evidence %s does not resolve", it.ID, id)
			}
		}
		for _, id := range it.Artifacts {
			art("item "+it.ID, id)
		}
	}
	for _, id := range b.Change.MigrationNotes {
		if !ev[id] {
			bad("migration note %s does not resolve", id)
		}
	}
	for _, s := range b.Change.Sources {
		for _, id := range s.Artifacts {
			art("source "+string(s.Source), id)
		}
	}
	for _, in := range b.Snapshot.Inputs {
		for _, id := range in.Artifacts {
			art("input "+string(in.Kind), id)
		}
	}
	renders := func(scope RenderScope, ps []ManifestPair) {
		for _, p := range ps {
			if err := p.validate(scope); err != nil {
				bad("%v", err)
			}
			for _, id := range []string{p.From, p.To} {
				if id != "" {
					art("render "+p.Target, id, ArtifactRenderedManifests)
				}
			}
			if p.Diff != "" {
				art("render "+p.Target, p.Diff, ArtifactRenderDiff)
			}
		}
	}
	renders(RenderRelease, b.Change.Renders)
	renders(RenderEnvironment, b.Renders)
	items := map[string]bool{}
	for _, it := range b.Change.Items {
		items[it.ID] = true
	}
	envChanges := map[string]RenderedChange{}
	for _, p := range b.Renders {
		for _, c := range p.Changes {
			envChanges[c.ID] = c
		}
	}
	for _, it := range b.EnvironmentItems {
		if items[it.ID] {
			bad("environment item %s: duplicate item id", it.ID)
		}
		items[it.ID] = true
		if it.ID != ChangeItemID(it.ChangeID, it.Platform, it.RenderedChange) || it.anchors() != 1 || it.RenderedChange == "" {
			bad("environment item %s: must be anchored on exactly one rendered change, id derived from it", it.ID)
		}
		if it.ScopeRule != ScopeRenderedUnexplained && it.ScopeRule != ScopeRoutineSelfArtifact && it.ScopeRule != ScopeDocumentation {
			bad("environment item %s: scope rule %q; environment items are %s, %s or %s", it.ID, it.ScopeRule, ScopeRenderedUnexplained, ScopeRoutineSelfArtifact, ScopeDocumentation)
		}
		for _, e := range it.renderedScopeErrors() {
			bad("environment item %s: %s", it.ID, e)
		}
		if strings.TrimSpace(it.Title) == "" || !strings.HasPrefix(it.ManifestationRule, "manifest:") || len(it.Evidence) == 0 {
			bad("environment item %s: title, manifestationRule and evidence are required", it.ID)
		}
		if c, ok := envChanges[it.RenderedChange]; !ok {
			bad("environment item %s: rendered change %s is not an environment-scope rendered change of this bundle", it.ID, it.RenderedChange)
		} else if !contains(c.ChangeItems, it.ID) {
			bad("environment item %s: its rendered change %s is not correlated to it", it.ID, it.RenderedChange)
		} else if it.ScopeRule == ScopeDocumentation && c.Class != RenderedClassDocumentation {
			bad("environment item %s: %s is only for a %s rendered change, not %q", it.ID, ScopeDocumentation, RenderedClassDocumentation, c.Class)
		}
	}
	for _, ps := range [][]ManifestPair{b.Change.Renders, b.Renders} {
		for _, p := range ps {
			for _, c := range p.Changes {
				for _, ci := range c.ChangeItems {
					if !items[ci] {
						bad("rendered change %s is correlated to unknown change item %s", c.ID, ci)
					}
				}
				for _, id := range c.Evidence {
					if !ev[id] {
						bad("rendered change %s: evidence %s does not resolve", c.ID, id)
					}
				}
			}
		}
	}
	for _, a := range b.Manifest.Artifacts {
		for _, ci := range a.ChangeItems {
			if !items[ci] {
				bad("artifact %s relates to unknown change item %s", a.ID, ci)
			}
		}
	}
	return errors.Join(errs...)
}

// --- relationship graph and traversal (§A3) ------------------------------------------------

// NodeKind is the generic, declarative vocabulary of graph nodes.
type NodeKind string

const (
	NodeProductRelease    NodeKind = "product-release"    // the dependency being upgraded (product@version)
	NodeHelmRelease       NodeKind = "helm-release"       // a Helm release of a chart in the environment
	NodeGitOpsApplication NodeKind = "gitops-application" // an Argo CD Application / Flux HelmRelease|Kustomization / helmfile release
	NodeKustomization     NodeKind = "kustomization"      // a kustomization (base or overlay)
	NodeValuesSource      NodeKind = "values-source"      // a values file or inline values block
	NodeWorkload          NodeKind = "workload"           // Deployment, StatefulSet, DaemonSet, Job, CronJob, Pod
	NodeService           NodeKind = "service"            // a Service
	NodeIngress           NodeKind = "ingress"            // Ingress / IngressClass / Gateway API routes
	NodeConfigObject      NodeKind = "config-object"      // ConfigMap / Secret
	NodeAPIType           NodeKind = "api-type"           // a served GVK (CRD version or built-in API)
	NodeCustomResource    NodeKind = "custom-resource"    // an object of a CRD-defined kind
	NodeRBAC              NodeKind = "rbac"               // ServiceAccount / Role / Binding
	NodeResource          NodeKind = "resource"           // any other Kubernetes object
	NodeExternal          NodeKind = "external"           // something outside the snapshot (external service, cluster object not in the repo)
)

// NodeKinds lists every kind.
var NodeKinds = []NodeKind{
	NodeProductRelease, NodeHelmRelease, NodeGitOpsApplication, NodeKustomization, NodeValuesSource,
	NodeWorkload, NodeService, NodeIngress, NodeConfigObject, NodeAPIType, NodeCustomResource,
	NodeRBAC, NodeResource, NodeExternal,
}

// GraphNode is one component of the environment (or the upgraded product).
type GraphNode struct {
	ID   string   `json:"id"` // NodeID(kind, key)
	Kind NodeKind `json:"kind"`
	// Key is the canonical identity: "<group>/<version>/<kind>/<namespace>/<name>"
	// for Kubernetes objects (render.ObjectID form), "<product>@<version>" for
	// product releases, the logical artifact path for values sources.
	Key string `json:"key"`
	// Placeholder marks a node that is referenced but absent from the
	// snapshot (e.g. a Secret named by a secretRef that no input defines).
	Placeholder bool         `json:"placeholder,omitempty"`
	Artifacts   []string     `json:"artifacts,omitempty"`
	Evidence    []EvidenceID `json:"evidence,omitempty"`
}

// NodeID derives a node id.
func NodeID(kind NodeKind, key string) string { return NodeIDPrefix + ShortHash(string(kind), key) }

// EdgeKind is the vocabulary of relationships. Every edge points in the
// direction impact PROPAGATES: From is the provider (what changes), To the
// consumer (what may break because of it).
type EdgeKind string

const (
	EdgeDeploys     EdgeKind = "deploys"      // gitops application → helm release / kustomization it deploys
	EdgeRendersTo   EdgeKind = "renders-to"   // helm release / kustomization → rendered object
	EdgeConfigures  EdgeKind = "configures"   // values source → helm release; config object → its reader
	EdgeOverrides   EdgeKind = "overrides"    // overlay patch / values setting → the object or release it overrides
	EdgeReferences  EdgeKind = "references"   // referenced object → referrer (Secret → Deployment mounting it)
	EdgeSelects     EdgeKind = "selects"      // selected object → selector (workload → Service; Service → ServiceMonitor)
	EdgeServesAPI   EdgeKind = "serves-api"   // product release → api type it installs/serves
	EdgeConsumesAPI EdgeKind = "consumes-api" // api type → object of that type
	EdgeProduces    EdgeKind = "produces"     // custom resource → object its controller creates (declared relationship rule)
	EdgeDependsOn   EdgeKind = "depends-on"   // dependency → dependent (generic, last resort)
)

// EdgeKinds lists every kind.
var EdgeKinds = []EdgeKind{
	EdgeDeploys, EdgeRendersTo, EdgeConfigures, EdgeOverrides, EdgeReferences, EdgeSelects,
	EdgeServesAPI, EdgeConsumesAPI, EdgeProduces, EdgeDependsOn,
}

// EdgeStatus is how well a relationship is known.
type EdgeStatus string

const (
	// EdgeEstablished: derived deterministically from evidence in the
	// snapshot (an explicit name reference, a resolved selector, a render).
	EdgeEstablished EdgeStatus = "established"
	// EdgeInferred: plausible but not proven (a naming convention, an
	// agent-proposed relationship). Unresolved for fail-closed purposes.
	EdgeInferred EdgeStatus = "inferred"
	// EdgeMissing: a known-unknown — the relationship must exist but its
	// other end cannot be established from the snapshot. Unresolved.
	EdgeMissing EdgeStatus = "missing"
)

// EdgeStatuses lists every status.
var EdgeStatuses = []EdgeStatus{EdgeEstablished, EdgeInferred, EdgeMissing}

// Resolved reports whether the edge counts as established.
func (s EdgeStatus) Resolved() bool { return s == EdgeEstablished }

// GraphEdge is one relationship.
type GraphEdge struct {
	ID     string     `json:"id"` // EdgeID(kind, from, to, detail)
	Kind   EdgeKind   `json:"kind"`
	From   string     `json:"from"`
	To     string     `json:"to"`
	Status EdgeStatus `json:"status"`
	// Rule is the discovery rule that produced the edge, e.g.
	// "graph.k8s-refs@v1:volumes.secret", "relationship:cert-manager/certificate-secret".
	Rule string `json:"rule"`
	// Detail locates the relationship (field path, selector, patch target).
	Detail string `json:"detail,omitempty"`
	// Reason explains an inferred or missing edge (required then).
	Reason string `json:"reason,omitempty"`
	// Mitigates lists the change items an overrides edge neutralises
	// (overrides edges only; established only through a validation).
	Mitigates []string     `json:"mitigates,omitempty"`
	Evidence  []EvidenceID `json:"evidence,omitempty"`
	Artifacts []string     `json:"artifacts,omitempty"`
	// Assertion is the agent assertion that proposed the edge (inferred only).
	Assertion string `json:"assertion,omitempty"`
}

// EdgeID derives an edge id.
func EdgeID(kind EdgeKind, from, to, detail string) string {
	return EdgeIDPrefix + ShortHash(string(kind), from, to, detail)
}

func (e GraphEdge) validate() []error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("edge %s: %s", e.ID, fmt.Sprintf(format, args...)))
	}
	if e.ID != EdgeID(e.Kind, e.From, e.To, e.Detail) {
		bad("id is not derived from kind+from+to+detail")
	}
	if !oneOf(e.Kind, EdgeKinds) {
		bad("unknown kind %q", e.Kind)
	}
	if !oneOf(e.Status, EdgeStatuses) {
		bad("unknown status %q", e.Status)
	}
	if e.From == e.To {
		bad("self-loop")
	}
	if e.Rule == "" {
		bad("rule is required")
	}
	if e.Status != EdgeEstablished && strings.TrimSpace(e.Reason) == "" {
		bad("%s edge requires a reason", e.Status)
	}
	if e.Status == EdgeEstablished && len(e.Evidence) == 0 && len(e.Artifacts) == 0 {
		bad("established edge requires evidence or artifacts")
	}
	if len(e.Mitigates) > 0 && e.Kind != EdgeOverrides {
		bad("only overrides edges may mitigate change items")
	}
	if e.Assertion != "" && e.Status != EdgeInferred {
		bad("an agent-proposed edge is always inferred")
	}
	return errs
}

// RelationshipGraph is the dependency/configuration/consumption graph of one
// bundle, built deterministically (Builder) from the snapshot and change
// record. Stored as a graph artifact; nodes and edges sorted by id.
type RelationshipGraph struct {
	ID      string      `json:"id"`
	Bundle  string      `json:"bundle"`
	Builder string      `json:"builder"` // component@vN
	Nodes   []GraphNode `json:"nodes"`
	Edges   []GraphEdge `json:"edges"`
}

// DerivedID returns the content-derived id of the graph.
func (g RelationshipGraph) DerivedID() string {
	parts := []string{g.Bundle, g.Builder}
	for _, n := range g.Nodes {
		parts = append(parts, n.ID, fmt.Sprint(n.Placeholder))
	}
	for _, e := range g.Edges {
		parts = append(parts, e.ID, string(e.Status))
	}
	return GraphIDPrefix + ShortHash(parts...)
}

// Validate checks identity, ordering and that every edge connects nodes.
func (g RelationshipGraph) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("graph %s: %s", g.ID, fmt.Sprintf(format, args...)))
	}
	if g.ID != g.DerivedID() {
		bad("id is not derived from its content")
	}
	if !strings.HasPrefix(g.Bundle, BundleIDPrefix) || g.Builder == "" {
		bad("bundle and builder are required")
	}
	nodes := map[string]bool{}
	for i, n := range g.Nodes {
		if n.ID != NodeID(n.Kind, n.Key) {
			bad("node %s: id is not derived from kind+key", n.ID)
		}
		if !oneOf(n.Kind, NodeKinds) {
			bad("node %s: unknown kind %q", n.ID, n.Kind)
		}
		if i > 0 && g.Nodes[i-1].ID >= n.ID {
			bad("nodes must be sorted by id and unique")
		}
		nodes[n.ID] = true
	}
	for i, e := range g.Edges {
		errs = append(errs, e.validate()...)
		if !nodes[e.From] || !nodes[e.To] {
			bad("edge %s connects unknown nodes", e.ID)
		}
		if i > 0 && g.Edges[i-1].ID >= e.ID {
			bad("edges must be sorted by id and unique")
		}
	}
	return errors.Join(errs...)
}

// ExposureRoot is a node directly exposed to a change item: the start of
// every impact path of that item. Rule names the exposure rule.
type ExposureRoot struct {
	ChangeItem string `json:"changeItem"`
	Node       string `json:"node"`
	Rule       string `json:"rule"`
	// RenderedChange is the environment-scope rendered change whose object
	// is the root (the primary exposure source); ImpactFinding the join
	// finding whose match grounded it. At most one is set.
	RenderedChange string       `json:"renderedChange,omitempty"`
	ImpactFinding  string       `json:"impactFinding,omitempty"`
	Evidence       []EvidenceID `json:"evidence,omitempty"`
}

// ImpactPath is one maximal simple path from an exposure root of a change
// item along outgoing edges: Nodes[0] is the root, Edges[i] connects
// Nodes[i] → Nodes[i+1]. Status is the weakest edge status on the path
// (established for a root-only path). Mitigations are overrides edges into
// a node of the path that mitigate the item.
type ImpactPath struct {
	ID          string       `json:"id"` // ImpactPathID(changeItem, nodes, edges)
	ChangeItem  string       `json:"changeItem"`
	Nodes       []string     `json:"nodes"`
	Edges       []string     `json:"edges,omitempty"`
	Status      EdgeStatus   `json:"status"`
	Mitigations []string     `json:"mitigations,omitempty"`
	Evidence    []EvidenceID `json:"evidence,omitempty"`
}

// ImpactPathID derives a path id.
func ImpactPathID(changeItem string, nodes, edges []string) string {
	return ImpactPathIDPrefix + ShortHash(changeItem, strings.Join(nodes, ">"), strings.Join(edges, ">"))
}

// TraversalBound bounds the DFS. Hitting any bound truncates the traversal,
// and a truncated traversal is never complete (DESIGN.md §2.5).
type TraversalBound struct {
	MaxDepth int `json:"maxDepth"` // edges per path
	MaxPaths int `json:"maxPaths"`
	MaxNodes int `json:"maxNodes"` // distinct visited nodes
}

// DefaultTraversalBound is the first-milestone bound. The depth covers real
// GitOps relationship chains with headroom: measured on a cert-manager
// app-of-apps repository with a two-tier internal PKI (the deepest real
// environment in the phase-4 dataset), the longest genuine chain is 9 edges
// (CRD → api-type → bootstrap ClusterIssuer → root-CA Certificate → Secret →
// issuing ClusterIssuer → leaf Certificate → Secret → workload → Service;
// 82 nodes, 97 edges, no cycles or self-references), which the previous
// default of 8 truncated on 22 of 77 paths, one edge short of the workloads
// they reach. Depth stays bounded — it is the guard that keeps worst-case
// simple-path enumeration finite — and hitting it stays honest: the
// truncation records its reason and blocks not-affected.
// CONTRACT-CHANGE(fixes, commander-approved 2026-10-16): MaxDepth 8 -> 12, measured; DESIGN §2.5 updated.
var DefaultTraversalBound = TraversalBound{MaxDepth: 12, MaxPaths: 500, MaxNodes: 5000}

// TraversalResult is the deterministic, bounded DFS from the exposure roots.
// Complete is true exactly when the traversal was not truncated and every
// path is established (no inferred or missing edge reachable from a root).
type TraversalResult struct {
	ID               string         `json:"id"`
	Graph            string         `json:"graph"`     // RelationshipGraph id
	Algorithm        string         `json:"algorithm"` // e.g. "graph.dfs@v1"
	Bound            TraversalBound `json:"bound"`
	Roots            []ExposureRoot `json:"roots"`
	DepthReached     int            `json:"depthReached"`
	Visited          []string       `json:"visited"` // node ids, sorted
	Paths            []ImpactPath   `json:"paths"`
	Unresolved       []string       `json:"unresolved,omitempty"` // non-established edge ids on any path, sorted
	Truncated        bool           `json:"truncated,omitempty"`
	TruncationReason string         `json:"truncationReason,omitempty"`
	Complete         bool           `json:"complete"`
}

// DerivedID returns the content-derived id of the traversal.
func (t TraversalResult) DerivedID() string {
	parts := []string{t.Graph, t.Algorithm, fmt.Sprint(t.Bound.MaxDepth, t.Bound.MaxPaths, t.Bound.MaxNodes)}
	for _, r := range t.Roots {
		parts = append(parts, r.ChangeItem, r.Node)
	}
	for _, p := range t.Paths {
		parts = append(parts, p.ID)
	}
	parts = append(parts, fmt.Sprint(t.Truncated))
	return TraversalIDPrefix + ShortHash(parts...)
}

// CompleteFor reports whether the traversal is complete for one change
// item: not truncated, and every path of the item established.
func (t TraversalResult) CompleteFor(changeItem string) bool {
	if t.Truncated {
		return false
	}
	for _, p := range t.Paths {
		if p.ChangeItem == changeItem && !p.Status.Resolved() {
			return false
		}
	}
	return true
}

// PathsOf returns the paths of one change item.
func (t TraversalResult) PathsOf(changeItem string) []ImpactPath {
	var out []ImpactPath
	for _, p := range t.Paths {
		if p.ChangeItem == changeItem {
			out = append(out, p)
		}
	}
	return out
}

// RootsOf returns the exposure roots of one change item.
func (t TraversalResult) RootsOf(changeItem string) []ExposureRoot {
	var out []ExposureRoot
	for _, r := range t.Roots {
		if r.ChangeItem == changeItem {
			out = append(out, r)
		}
	}
	return out
}

// Validate checks the self-contained invariants (identity, bounds, path
// shape, the completeness rule). ValidateAgainst additionally checks the
// paths against the graph.
func (t TraversalResult) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("traversal %s: %s", t.ID, fmt.Sprintf(format, args...)))
	}
	if t.ID != t.DerivedID() {
		bad("id is not derived from its content")
	}
	if !strings.HasPrefix(t.Graph, GraphIDPrefix) || t.Algorithm == "" {
		bad("graph and algorithm are required")
	}
	if t.Bound.MaxDepth <= 0 || t.Bound.MaxPaths <= 0 || t.Bound.MaxNodes <= 0 {
		bad("every bound must be positive")
	}
	if t.DepthReached > t.Bound.MaxDepth || len(t.Paths) > t.Bound.MaxPaths || len(t.Visited) > t.Bound.MaxNodes {
		bad("result exceeds its bound")
	}
	if t.Truncated != (strings.TrimSpace(t.TruncationReason) != "") {
		bad("truncated and truncationReason go together")
	}
	if !sort.StringsAreSorted(t.Visited) {
		bad("visited must be sorted")
	}
	roots := map[[2]string]bool{}
	for _, r := range t.Roots {
		if r.ChangeItem == "" || r.Node == "" || r.Rule == "" {
			bad("root needs changeItem, node and rule")
		}
		if r.RenderedChange != "" && r.ImpactFinding != "" {
			bad("root %s: at most one of renderedChange and impactFinding grounds it", r.Node)
		}
		roots[[2]string{r.ChangeItem, r.Node}] = true
	}
	unresolved := map[string]bool{}
	seen := map[string]bool{}
	for _, p := range t.Paths {
		if p.ID != ImpactPathID(p.ChangeItem, p.Nodes, p.Edges) {
			bad("path %s: id is not derived from item+nodes+edges", p.ID)
		}
		if seen[p.ID] {
			bad("duplicate path %s", p.ID)
		}
		seen[p.ID] = true
		if len(p.Nodes) == 0 || len(p.Edges) != len(p.Nodes)-1 {
			bad("path %s: needs n nodes and n-1 edges", p.ID)
			continue
		}
		if !roots[[2]string{p.ChangeItem, p.Nodes[0]}] {
			bad("path %s does not start at an exposure root of %s", p.ID, p.ChangeItem)
		}
		if len(p.Edges) > t.Bound.MaxDepth {
			bad("path %s is deeper than the bound", p.ID)
		}
		if !oneOf(p.Status, EdgeStatuses) {
			bad("path %s: unknown status %q", p.ID, p.Status)
		}
	}
	for _, id := range t.Unresolved {
		unresolved[id] = true
	}
	if !sort.StringsAreSorted(t.Unresolved) {
		bad("unresolved must be sorted")
	}
	anyUnresolvedPath := false
	for _, p := range t.Paths {
		if !p.Status.Resolved() {
			anyUnresolvedPath = true
		}
	}
	if anyUnresolvedPath && len(t.Unresolved) == 0 {
		bad("a path is not established but no unresolved edge is listed")
	}
	if want := !t.Truncated && !anyUnresolvedPath && len(t.Unresolved) == 0; t.Complete != want {
		bad("complete = %v, want %v (complete ⇔ not truncated and every path established)", t.Complete, want)
	}
	return errors.Join(errs...)
}

// ValidateAgainst checks the traversal against its graph: roots, visited
// nodes and path edges exist, consecutive nodes are connected by the listed
// edge, the path status is its weakest edge, unresolved lists exactly the
// non-established edges on paths, and mitigations are overrides edges into
// a path node that mitigate the item.
func (t TraversalResult) ValidateAgainst(g RelationshipGraph) error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("traversal %s: %s", t.ID, fmt.Sprintf(format, args...)))
	}
	if t.Graph != g.ID {
		bad("graph %s, want %s", t.Graph, g.ID)
	}
	nodes := map[string]bool{}
	for _, n := range g.Nodes {
		nodes[n.ID] = true
	}
	edges := map[string]GraphEdge{}
	for _, e := range g.Edges {
		edges[e.ID] = e
	}
	for _, r := range t.Roots {
		if !nodes[r.Node] {
			bad("root node %s is not in the graph", r.Node)
		}
	}
	for _, n := range t.Visited {
		if !nodes[n] {
			bad("visited node %s is not in the graph", n)
		}
	}
	want := map[string]bool{}
	for _, p := range t.Paths {
		status := EdgeEstablished
		for i, id := range p.Edges {
			e, ok := edges[id]
			if !ok {
				bad("path %s: edge %s is not in the graph", p.ID, id)
				continue
			}
			if i+1 < len(p.Nodes) && (e.From != p.Nodes[i] || e.To != p.Nodes[i+1]) {
				bad("path %s: edge %s does not connect %s → %s", p.ID, id, p.Nodes[i], p.Nodes[i+1])
			}
			if !e.Status.Resolved() {
				want[id] = true
				if status == EdgeEstablished || e.Status == EdgeMissing {
					status = e.Status
				}
			}
		}
		if p.Status != status {
			bad("path %s: status %s, want %s (weakest edge)", p.ID, p.Status, status)
		}
		onPath := map[string]bool{}
		for _, n := range p.Nodes {
			onPath[n] = true
		}
		for _, id := range p.Mitigations {
			e, ok := edges[id]
			if !ok || e.Kind != EdgeOverrides || !onPath[e.To] || !contains(e.Mitigates, p.ChangeItem) {
				bad("path %s: mitigation %s is not an overrides edge into the path that mitigates %s", p.ID, id, p.ChangeItem)
			}
		}
	}
	got := map[string]bool{}
	for _, id := range t.Unresolved {
		got[id] = true
	}
	for id := range want {
		if !got[id] {
			bad("unresolved edge %s on a path is not listed", id)
		}
	}
	for id := range got {
		if !want[id] {
			bad("listed unresolved edge %s is on no path", id)
		}
	}
	return errors.Join(errs...)
}

// --- validation (§A5, P4) -----------------------------------------------------------------

// ValidationKind is the class of validator.
type ValidationKind string

const (
	ValidationOverrideInspection ValidationKind = "override-inspection" // does an environment override neutralise the change?
	ValidationRenderDiff         ValidationKind = "render-diff"         // helm/kustomize render comparison
	ValidationSchemaCompat       ValidationKind = "schema-compat"       // CRD/API/values schema compatibility
	ValidationStaticReference    ValidationKind = "static-reference"    // static dependency / reference checks
	ValidationRuntime            ValidationKind = "runtime"             // isolated deployment (observed)
)

// ValidationKinds lists every kind.
var ValidationKinds = []ValidationKind{
	ValidationOverrideInspection, ValidationRenderDiff, ValidationSchemaCompat, ValidationStaticReference, ValidationRuntime,
}

// ValidationLevel is how a validation result was established. Only these
// two levels exist for a ValidationRecord: human and proxy decisions are
// reviews (HumanReview), never validations; AI output is an assertion.
type ValidationLevel string

const (
	LevelDeterministic ValidationLevel = "deterministic" // a validator proved it from artifacts
	LevelObserved      ValidationLevel = "observed"      // a runtime outcome in an isolated cluster
)

// ValidationLevels lists every level.
var ValidationLevels = []ValidationLevel{LevelDeterministic, LevelObserved}

// ValidationOutcomeKind is the outcome of one validation run. The checked
// property is always the SAFETY property of (change item, subjects): pass =
// the change demonstrably does not reach / break the subjects; fail = it
// demonstrably does; error = the validator broke; not-run = it could not run
// (missing tool, input or support), with the reason.
type ValidationOutcomeKind string

const (
	ValidationPass   ValidationOutcomeKind = "pass"
	ValidationFail   ValidationOutcomeKind = "fail"
	ValidationError  ValidationOutcomeKind = "error"
	ValidationNotRun ValidationOutcomeKind = "not-run"
)

// ValidationOutcomeKinds lists every outcome.
var ValidationOutcomeKinds = []ValidationOutcomeKind{ValidationPass, ValidationFail, ValidationError, ValidationNotRun}

// Decisive reports whether the outcome decides the property.
func (o ValidationOutcomeKind) Decisive() bool { return o == ValidationPass || o == ValidationFail }

// PinKind is what kind of explicit environment setting pins a behaviour.
type PinKind string

const (
	PinValuesKey  PinKind = "values-key" // a Helm values key the environment sets
	PinField      PinKind = "field"      // a field of a manifest / custom resource
	PinAnnotation PinKind = "annotation" // an annotation on an object
	PinFlag       PinKind = "flag"       // a controller command-line flag (container args)
	PinEnvVar     PinKind = "env-var"    // a controller environment variable
	PinConfigKey  PinKind = "config-key" // a key of a controller ConfigMap / config file
)

// PinKinds lists every kind.
var PinKinds = []PinKind{PinValuesKey, PinField, PinAnnotation, PinFlag, PinEnvVar, PinConfigKey}

// PinnedSetting is an explicit setting in the environment that pins the old
// (safe) behaviour of a change item, established from environment evidence.
// It is what lets a controller-behaviour item be not-affected (DESIGN.md §3.1
// R2.4): the render can show WHERE the setting lands (RenderedChanges /
// RenderedObject), but only the explicit setting proves the behaviour.
type PinnedSetting struct {
	Kind PinKind `json:"kind"`
	// Subject is where the setting is made: an object id
	// (<group>/<version>/<kind>/<namespace>/<name>) or an environment file path.
	Subject string `json:"subject"`
	Path    string `json:"path"`  // key / field path / annotation / flag / variable name
	Value   string `json:"value"` // the pinned value as written (JSON-encoded scalar)
	// RenderedObject is the rendered object the setting lands in, when the
	// render shows it (e.g. the controller Deployment's args, its ConfigMap).
	RenderedObject string `json:"renderedObject,omitempty"`
	// Evidence cites the environment evidence (local-file, input or an
	// environment-scope render) that states the setting.
	Evidence []EvidenceID `json:"evidence"`
}

// ValidationRecord is one deterministic (or observed) validation of a
// change item against environment subjects. It is the ONLY thing that can
// lower a finding toward not-affected or establish affected (DESIGN.md §3).
// Named ValidationRecord because domain.ValidationResult is the learning
// loop's release-level, candidate-scoped validator verdict, which may not
// cite environment evidence.
type ValidationRecord struct {
	ID         string          `json:"id"`        // ValidationRecordID(...)
	Validator  string          `json:"validator"` // component@vN, e.g. "validate.render-diff@v1"
	Kind       ValidationKind  `json:"kind"`
	Level      ValidationLevel `json:"level"`
	ChangeItem string          `json:"changeItem"`
	// Subjects are the graph nodes the property was checked for; Paths the
	// impact paths the result covers.
	Subjects []string `json:"subjects,omitempty"`
	Paths    []string `json:"paths,omitempty"`
	// RenderedChanges are the rendered changes the result covers: for a
	// pass, changes shown harmless for the subjects; for a fail, the changes
	// that demonstrate the breakage.
	RenderedChanges []string `json:"renderedChanges,omitempty"`
	// Property states the checked safety property in one sentence.
	Property string                `json:"property"`
	Inputs   []string              `json:"inputs"`            // artifact ids (by digest) the validator read
	Outputs  []string              `json:"outputs,omitempty"` // artifact ids it produced (renders, diffs, logs)
	Outcome  ValidationOutcomeKind `json:"outcome"`
	Detail   string                `json:"detail,omitempty"` // required unless pass
	Evidence []EvidenceID          `json:"evidence,omitempty"`
	// Answers lists agent assertion / hypothesis ids whose claim this
	// validation tested; a pass that answers a concern lifts its block.
	Answers []string `json:"answers,omitempty"`
	// Request is the validation request that triggered the run, if any.
	Request string `json:"request,omitempty"`
	// Pin is the explicit environment setting an override-inspection pass
	// established (required for that pass to clear a controller-behaviour
	// item; DESIGN.md §3.1 R2.4).
	Pin *PinnedSetting `json:"pin,omitempty"`
	// ImpactFinding is set when the record wraps a verdict of the
	// deterministic ImpactReport (e.g. a not-affected render check).
	ImpactFinding string     `json:"impactFinding,omitempty"`
	Provenance    Provenance `json:"provenance"` // never method ai
	CheckedAt     time.Time  `json:"checkedAt"`  // not part of the id
}

// EchoRequested makes an undecided record (outcome not-run or error) carry
// the coverage it was asked about exactly as requested — the subjects, paths
// and rendered changes of the request — next to whatever coverage the
// validator named itself: the record stays linkable to its request and the
// audit keeps what was asked (an undecided record never counts as coverage
// for clearing, so this can only add gaps, never clear). A decisive record
// is left unchanged: it speaks only for what it decided. Call it before
// deriving the record id.
// CONTRACT-CHANGE(fixes2): additive helper.
func (v *ValidationRecord) EchoRequested(subjects, paths, renderedChanges []string) {
	if v.Outcome != ValidationNotRun && v.Outcome != ValidationError {
		return
	}
	v.Subjects = sortedUnion(v.Subjects, subjects)
	v.Paths = sortedUnion(v.Paths, paths)
	v.RenderedChanges = sortedUnion(v.RenderedChanges, renderedChanges)
}

func sortedUnion(a, b []string) []string {
	m := map[string]bool{}
	for _, xs := range [][]string{a, b} {
		for _, x := range xs {
			if x != "" {
				m[x] = true
			}
		}
	}
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for x := range m {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

// ValidationRecordID derives a validation record id.
func ValidationRecordID(validator, changeItem string, subjects, paths, renderedChanges, inputs []string) string {
	return ValidationRecordIDPrefix + ShortHash(validator, changeItem,
		strings.Join(subjects, ","), strings.Join(paths, ","), strings.Join(renderedChanges, ","), strings.Join(inputs, ","))
}

// Validate checks identity, level/kind consistency and the outcome shape.
func (v ValidationRecord) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("validation %s: %s", v.ID, fmt.Sprintf(format, args...)))
	}
	if v.ID != ValidationRecordID(v.Validator, v.ChangeItem, v.Subjects, v.Paths, v.RenderedChanges, v.Inputs) {
		bad("id is not derived from validator+changeItem+subjects+paths+renderedChanges+inputs")
	}
	if !strings.Contains(v.Validator, "@v") {
		bad("validator %q must be component@vN", v.Validator)
	}
	if !oneOf(v.Kind, ValidationKinds) {
		bad("unknown kind %q", v.Kind)
	}
	if !oneOf(v.Level, ValidationLevels) {
		bad("unknown level %q", v.Level)
	}
	if (v.Kind == ValidationRuntime) != (v.Level == LevelObserved) {
		bad("runtime validations are observed and only they are (kind %s, level %s)", v.Kind, v.Level)
	}
	if !oneOf(v.Outcome, ValidationOutcomeKinds) {
		bad("unknown outcome %q", v.Outcome)
	}
	if !strings.HasPrefix(v.ChangeItem, ChangeItemIDPrefix) || strings.TrimSpace(v.Property) == "" {
		bad("changeItem and property are required")
	}
	if v.Outcome != ValidationPass && strings.TrimSpace(v.Detail) == "" {
		bad("outcome %s requires detail", v.Outcome)
	}
	if v.Outcome.Decisive() && len(v.Inputs) == 0 {
		bad("a decisive outcome requires the input artifacts it was computed from")
	}
	if v.Outcome.Decisive() && len(v.Evidence) == 0 && len(v.Outputs) == 0 {
		bad("a decisive outcome requires evidence or output artifacts")
	}
	if v.Provenance.Method == MethodAI {
		bad("a validation is never AI output (record it as an AgentAssertion)")
	}
	if err := v.Provenance.Validate(); err != nil {
		bad("provenance: %v", err)
	}
	if v.Provenance.Producer != v.Validator {
		bad("provenance producer %q must equal validator %q", v.Provenance.Producer, v.Validator)
	}
	if p := v.Pin; p != nil {
		if v.Kind != ValidationOverrideInspection || v.Outcome != ValidationPass {
			bad("a pinned setting is established only by a passing override-inspection")
		}
		if !oneOf(p.Kind, PinKinds) || p.Subject == "" || p.Path == "" || p.Value == "" {
			bad("pin needs a known kind, subject, path and value")
		}
		if len(p.Evidence) == 0 {
			bad("pin must cite the environment evidence that states the setting")
		}
	}
	return errors.Join(errs...)
}

// ClearsBehaviour reports whether a passing record may clear a change item
// whose manifestation is not rendered-config: an observed outcome, or an
// override inspection that established an explicit pinned setting.
func (v ValidationRecord) ClearsBehaviour() bool {
	return v.Outcome == ValidationPass && (v.Level == LevelObserved || (v.Kind == ValidationOverrideInspection && v.Pin != nil))
}

// --- agent records (§A4, P3) --------------------------------------------------------------

// Hypothesis is a testable statement the agent formulated about a change
// item. Its standing is derived from ValidationRecords that answer it, never
// set by the agent.
type Hypothesis struct {
	ID            string       `json:"id"` // AgentRecordID(HypothesisIDPrefix, ...)
	ChangeItem    string       `json:"changeItem"`
	Statement     string       `json:"statement"`
	Subjects      []string     `json:"subjects,omitempty"` // graph node ids
	Citations     []EvidenceID `json:"citations"`
	StatementKind string       `json:"statementKind,omitempty"` // see AgentAssertion; CONTRACT-CHANGE(agent)
	// Mitigation is the structured mitigation the hypothesis proposes, stored
	// only when well-formed; MitigationRejected says why a spec the agent
	// gave was not (the hypothesis is then free text only).
	// CONTRACT-CHANGE(verdict): additive (PROTOCOL §8).
	Mitigation         *MitigationSpec `json:"mitigation,omitempty"`
	MitigationRejected string          `json:"mitigationRejected,omitempty"`
	Provenance         Provenance      `json:"provenance"` // method ai
}

// AssertionStance is what an agent assertion does to a finding (DESIGN.md
// §3.3). No stance can lower a class; only raises-concern changes one.
type AssertionStance string

const (
	// StanceRaisesConcern: the agent claims a plausible impact. It blocks
	// not-affected until a passing validation answers it, and can move
	// insufficient-evidence to potentially-affected.
	StanceRaisesConcern AssertionStance = "raises-concern"
	// StanceExplains: reasoning attached to a finding; changes nothing.
	StanceExplains AssertionStance = "explains"
	// StanceClaimsMitigation: the agent believes something mitigates the
	// change. Recorded as an unvalidated assumption on the finding; it never
	// lowers a class (a validation must establish the mitigation).
	StanceClaimsMitigation AssertionStance = "claims-mitigation"
)

// AssertionStances lists every stance.
var AssertionStances = []AssertionStance{StanceRaisesConcern, StanceExplains, StanceClaimsMitigation}

// AgentAssertion is a claim the agent made, with its citations. Never a
// validation result (MISSION P4).
type AgentAssertion struct {
	ID         string          `json:"id"`
	ChangeItem string          `json:"changeItem"`
	Stance     AssertionStance `json:"stance"`
	Statement  string          `json:"statement"`
	Subjects   []string        `json:"subjects,omitempty"` // graph node ids
	Paths      []string        `json:"paths,omitempty"`    // impact path ids
	// RenderedChanges are rendered changes the claim is about (cited by id).
	RenderedChanges []string `json:"renderedChanges,omitempty"`
	// Citations are evidence ids the tool layer created after checking the
	// cited quote occurs in the artifact's bytes (DESIGN.md §4.4).
	Citations []EvidenceID `json:"citations"`
	// ProposedEdge is a relationship the agent proposes; always inferred,
	// with Assertion set to this assertion's id.
	ProposedEdge *GraphEdge `json:"proposedEdge,omitempty"`
	Hypothesis   string     `json:"hypothesis,omitempty"`
	// NoValidationReason is the agent's stated reason for not requesting a
	// validation of a raises-concern (prompt depth rules: a concern on a
	// controller-behaviour/unknown item needs a request or a reason).
	// CONTRACT-CHANGE(agent): additive.
	NoValidationReason string `json:"noValidationReason,omitempty"`
	// StatementKind labels the statement: "unverified-ai-prose" (prompt
	// feature claims) for free text no tool checked; facts about the
	// environment are stated as Claims (verified SettingClaim ids).
	// CONTRACT-CHANGE(agent): additive.
	StatementKind string     `json:"statementKind,omitempty"`
	Claims        []string   `json:"claims,omitempty"`
	Provenance    Provenance `json:"provenance"` // method ai
}

// StatementUnverifiedProse labels agent free text no tool verified.
// CONTRACT-CHANGE(agent): additive.
const StatementUnverifiedProse = "unverified-ai-prose"

// ClaimKind is what a setting claim states. CONTRACT-CHANGE(agent): additive.
type ClaimKind string

const (
	ClaimAbsent  ClaimKind = "absent"  // the setting does not occur in the input
	ClaimPresent ClaimKind = "present" // the setting occurs in the input
	ClaimValue   ClaimKind = "value"   // the setting has the value in the input
)

// ClaimKinds lists every kind.
var ClaimKinds = []ClaimKind{ClaimAbsent, ClaimPresent, ClaimValue}

// SettingClaim is a fact about the environment the agent stated in
// structured form ("setting S is absent / present / has value V in input X")
// and the tool layer verified deterministically before recording it. The
// claim is the agent's (AI provenance); the verification is a deterministic
// check over the bundle's artifacts. It supports the agent's records; it is
// never a validation and never a policy input.
// CONTRACT-CHANGE(agent): additive.
type SettingClaim struct {
	ID         string    `json:"id"` // AgentRecordID(SettingClaimIDPrefix, changeItem, kind‖setting‖input‖value, callId)
	ChangeItem string    `json:"changeItem,omitempty"`
	Kind       ClaimKind `json:"kind"`
	// Setting is a dotted key path (controller.metrics.enabled) or a token
	// as written in the input (a flag, annotation key, env var name).
	Setting string `json:"setting"`
	// Input is what was checked: an artifact id, or "environment-files"
	// (every environment-origin text artifact of the bundle).
	Input string `json:"input"`
	Value string `json:"value,omitempty"` // JSON-encoded scalar (kind value)
	// The deterministic check: the verifier, the artifacts read and, for
	// present/value, the evidence of the matching lines.
	Verifier   string       `json:"verifier"`
	Checked    []string     `json:"checked"`
	Evidence   []EvidenceID `json:"evidence,omitempty"`
	Detail     string       `json:"detail"`
	Provenance Provenance   `json:"provenance"` // method ai (the claim is the agent's)
}

// SettingClaimID derives a claim id.
func SettingClaimID(changeItem string, kind ClaimKind, setting, input, value, callID string) string {
	return AgentRecordID(SettingClaimIDPrefix, changeItem, string(kind)+"\x00"+setting+"\x00"+input+"\x00"+value, callID)
}

// Validate checks the claim's shape.
func (c SettingClaim) Validate() error {
	errs := validateAI("setting claim "+c.ID, c.Provenance)
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("setting claim %s: %s", c.ID, fmt.Sprintf(format, args...)))
	}
	if c.ID != SettingClaimID(c.ChangeItem, c.Kind, c.Setting, c.Input, c.Value, c.Provenance.CallID) {
		bad("id is not derived")
	}
	if !oneOf(c.Kind, ClaimKinds) || strings.TrimSpace(c.Setting) == "" || c.Input == "" {
		bad("a known kind, a setting and an input are required")
	}
	if (c.Kind == ClaimValue) != (c.Value != "") {
		bad("a value is set exactly for kind value")
	}
	if !strings.Contains(c.Verifier, "@v") || len(c.Checked) == 0 || strings.TrimSpace(c.Detail) == "" {
		bad("verifier, checked artifacts and detail are required")
	}
	if c.Kind != ClaimAbsent && len(c.Evidence) == 0 {
		bad("a present/value claim cites the matching evidence")
	}
	return errors.Join(errs...)
}

// AgentRecordID derives the id of an agent record (hypothesis, assertion,
// request) from the change item, its content and the model call that
// produced it, so a replayed call yields the same id and two calls never
// collide.
func AgentRecordID(prefix, changeItem, content, callID string) string {
	return prefix + ShortHash(changeItem, content, callID)
}

// EvidenceRequestKind is what the agent asks to be collected.
type EvidenceRequestKind string

const (
	RequestArtifact         EvidenceRequestKind = "artifact"          // an upstream artifact not in the bundle (a linked PR, a file at a tag)
	RequestSource           EvidenceRequestKind = "source"            // re-collect a change source whose status is not ok
	RequestEnvironmentInput EvidenceRequestKind = "environment-input" // an environment file/input not supplied
	RequestLiveObservation  EvidenceRequestKind = "live-observation"  // a read-only live-cluster read (only when enabled)
)

// EvidenceRequestKinds lists every kind.
var EvidenceRequestKinds = []EvidenceRequestKind{RequestArtifact, RequestSource, RequestEnvironmentInput, RequestLiveObservation}

// RequestStatus is the state of an agent request.
type RequestStatus string

const (
	RequestOpen        RequestStatus = "open"
	RequestFulfilled   RequestStatus = "fulfilled"
	RequestUnavailable RequestStatus = "unavailable" // tried; the evidence could not be obtained
	RequestRejected    RequestStatus = "rejected"    // refused by policy (out of scope, live reads disabled, budget)
)

// RequestStatuses lists every status.
var RequestStatuses = []RequestStatus{RequestOpen, RequestFulfilled, RequestUnavailable, RequestRejected}

// EvidenceRequest is the agent asking for more evidence (MISSION §A4).
// Fulfilled artifacts are added to the investigation's derived artifacts.
type EvidenceRequest struct {
	ID         string              `json:"id"`
	ChangeItem string              `json:"changeItem,omitempty"`
	Kind       EvidenceRequestKind `json:"kind"`
	Target     string              `json:"target"` // URL, source kind, path or GVK the request names
	Reason     string              `json:"reason"`
	Status     RequestStatus       `json:"status"`
	Resolution string              `json:"resolution,omitempty"` // required unless open/fulfilled
	Fulfilled  []string            `json:"fulfilled,omitempty"`  // artifact ids
	Provenance Provenance          `json:"provenance"`           // method ai
}

// ValidationRequest is the agent asking a deterministic validator to test a
// hypothesis. The result is a ValidationRecord produced by the validator,
// never by the agent.
type ValidationRequest struct {
	ID         string         `json:"id"`
	ChangeItem string         `json:"changeItem"`
	Kind       ValidationKind `json:"kind"`
	Subjects   []string       `json:"subjects,omitempty"`
	Inputs     []string       `json:"inputs,omitempty"` // artifact ids the agent suggests
	Hypothesis string         `json:"hypothesis,omitempty"`
	// Answers are the agent assertions (raises-concern / claims-mitigation)
	// the request was made to answer; the tool layer links them into the
	// resulting record's Answers only when the record covers what each
	// assertion names. CONTRACT-CHANGE(agent): additive.
	Answers    []string      `json:"answers,omitempty"`
	Status     RequestStatus `json:"status"`
	Resolution string        `json:"resolution,omitempty"`
	Result     string        `json:"result,omitempty"` // validation record id (fulfilled only)
	Provenance Provenance    `json:"provenance"`       // method ai
}

func validateAI(what string, p Provenance) []error {
	var errs []error
	if p.Method != MethodAI {
		errs = append(errs, fmt.Errorf("%s: agent records carry ai provenance (got %q)", what, p.Method))
	}
	if err := p.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("%s: provenance: %w", what, err))
	}
	if p.CallID == "" {
		errs = append(errs, fmt.Errorf("%s: ai provenance requires the callId of the agent run", what))
	}
	return errs
}

// Validate checks the hypothesis.
func (h Hypothesis) Validate() error {
	errs := validateAI("hypothesis "+h.ID, h.Provenance)
	if h.ID != AgentRecordID(HypothesisIDPrefix, h.ChangeItem, h.Statement, h.Provenance.CallID) {
		errs = append(errs, fmt.Errorf("hypothesis %s: id is not derived", h.ID))
	}
	if strings.TrimSpace(h.Statement) == "" || len(h.Citations) == 0 {
		errs = append(errs, fmt.Errorf("hypothesis %s: statement and citations are required", h.ID))
	}
	if m := h.Mitigation; m != nil {
		if err := m.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("hypothesis %s: %w", h.ID, err))
		}
		if m.Item != h.ChangeItem {
			errs = append(errs, fmt.Errorf("hypothesis %s: mitigation item %s is not the hypothesis item", h.ID, m.Item))
		}
		if h.MitigationRejected != "" {
			errs = append(errs, fmt.Errorf("hypothesis %s: a mitigation is either stored or rejected", h.ID))
		}
	}
	return errors.Join(errs...)
}

// Validate checks the assertion.
func (a AgentAssertion) Validate() error {
	errs := validateAI("assertion "+a.ID, a.Provenance)
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("assertion %s: %s", a.ID, fmt.Sprintf(format, args...)))
	}
	if a.ID != AgentRecordID(AssertionIDPrefix, a.ChangeItem, string(a.Stance)+"\x00"+a.Statement, a.Provenance.CallID) {
		bad("id is not derived")
	}
	if !oneOf(a.Stance, AssertionStances) {
		bad("unknown stance %q", a.Stance)
	}
	if !strings.HasPrefix(a.ChangeItem, ChangeItemIDPrefix) || strings.TrimSpace(a.Statement) == "" {
		bad("changeItem and statement are required")
	}
	if len(a.Citations) == 0 {
		bad("an assertion must cite evidence")
	}
	if e := a.ProposedEdge; e != nil {
		errs = append(errs, e.validate()...)
		if e.Status != EdgeInferred || e.Assertion != a.ID {
			bad("a proposed edge is inferred and names this assertion")
		}
	}
	return errors.Join(errs...)
}

// Validate checks the request.
func (r EvidenceRequest) Validate() error {
	errs := validateAI("evidence request "+r.ID, r.Provenance)
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("evidence request %s: %s", r.ID, fmt.Sprintf(format, args...)))
	}
	if r.ID != AgentRecordID(EvidenceRequestIDPrefix, r.ChangeItem, string(r.Kind)+"\x00"+r.Target, r.Provenance.CallID) {
		bad("id is not derived")
	}
	if !oneOf(r.Kind, EvidenceRequestKinds) || !oneOf(r.Status, RequestStatuses) {
		bad("unknown kind %q or status %q", r.Kind, r.Status)
	}
	if r.Target == "" || r.Reason == "" {
		bad("target and reason are required")
	}
	errs = append(errs, validateRequestState("evidence request "+r.ID, r.Status, r.Resolution, len(r.Fulfilled) > 0)...)
	return errors.Join(errs...)
}

// Validate checks the request.
func (r ValidationRequest) Validate() error {
	errs := validateAI("validation request "+r.ID, r.Provenance)
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("validation request %s: %s", r.ID, fmt.Sprintf(format, args...)))
	}
	if r.ID != AgentRecordID(ValidationRequestIDPrefix, r.ChangeItem, string(r.Kind)+"\x00"+strings.Join(r.Subjects, ","), r.Provenance.CallID) {
		bad("id is not derived")
	}
	if !oneOf(r.Kind, ValidationKinds) || !oneOf(r.Status, RequestStatuses) {
		bad("unknown kind %q or status %q", r.Kind, r.Status)
	}
	if !strings.HasPrefix(r.ChangeItem, ChangeItemIDPrefix) {
		bad("changeItem is required")
	}
	errs = append(errs, validateRequestState("validation request "+r.ID, r.Status, r.Resolution, r.Result != "")...)
	return errors.Join(errs...)
}

func validateRequestState(what string, s RequestStatus, resolution string, hasResult bool) []error {
	var errs []error
	if (s == RequestFulfilled) != hasResult {
		errs = append(errs, fmt.Errorf("%s: a result is set exactly when fulfilled", what))
	}
	if (s == RequestUnavailable || s == RequestRejected) && strings.TrimSpace(resolution) == "" {
		errs = append(errs, fmt.Errorf("%s: status %s requires a resolution", what, s))
	}
	return errs
}

// --- findings and policy (§A6) -------------------------------------------------------------

// FindingClass is exactly the four MISSION §A6 classes.
type FindingClass string

const (
	// FindingAffected: evidence demonstrates an incompatible or breaking
	// impact: a failing deterministic/observed validation (an action-required
	// verdict of the deterministic join is wrapped as one).
	FindingAffected FindingClass = "affected"
	// FindingNotAffected: sufficient evidence supports that the change does
	// not affect the environment within the analysis scope (all §3.1 rules).
	FindingNotAffected FindingClass = "not-affected"
	// FindingPotentiallyAffected: a plausible impact exists but cannot yet be
	// confirmed or dismissed.
	FindingPotentiallyAffected FindingClass = "potentially-affected"
	// FindingInsufficientEvidence: missing context or validation prevents a
	// defensible conclusion.
	FindingInsufficientEvidence FindingClass = "insufficient-evidence"
)

// FindingClasses lists every class.
var FindingClasses = []FindingClass{FindingAffected, FindingNotAffected, FindingPotentiallyAffected, FindingInsufficientEvidence}

// GapKind classifies an evidence gap.
type GapKind string

const (
	GapCollection   GapKind = "collection"   // a change source or environment input is partial/unavailable
	GapTraversal    GapKind = "traversal"    // the traversal was truncated
	GapRelationship GapKind = "relationship" // an inferred/missing edge on a path
	GapValidation   GapKind = "validation"   // a path lacks a decisive validation (not-run, error, none available)
	GapAgent        GapKind = "agent"        // an agent concern is unanswered, or the agent stage did not complete
	GapExposure     GapKind = "exposure"     // the deciding environment input for the item was not supplied
)

// GapKinds lists every kind.
var GapKinds = []GapKind{GapCollection, GapTraversal, GapRelationship, GapValidation, GapAgent, GapExposure}

// EvidenceGap is one reason a finding cannot be (more) decisive.
type EvidenceGap struct {
	Kind GapKind `json:"kind"`
	// Subject names what is missing: a source kind, input path, edge, path,
	// validator or assertion id.
	Subject string `json:"subject"`
	Detail  string `json:"detail"`
	// UnknownReason maps the gap to the existing UNKNOWN vocabulary where
	// one fits (routing, metrics).
	UnknownReason UnknownReason `json:"unknownReason,omitempty"`
}

// ImpactFindingRef is a copy of a deterministic ImpactReport finding of the
// same change: its id, class and rule.
type ImpactFindingRef struct {
	ID             string      `json:"id"`
	Classification ImpactClass `json:"classification"`
	Rule           string      `json:"rule"`
}

// ApprovalEligibility says whether a finding may be approved without a
// human (DESIGN.md §3.4). Never computed from LLM confidence or model
// agreement.
type ApprovalEligibility struct {
	Eligible            bool     `json:"eligible"`
	RequiresHumanReview bool     `json:"requiresHumanReview"`
	Blockers            []string `json:"blockers,omitempty"` // why not eligible (required when not eligible)
}

// InvestigationFinding is the per-(change item, environment) conclusion,
// computed by the deterministic policy. It carries every field MISSION §A6
// requires.
type InvestigationFinding struct {
	ID         string       `json:"id"` // InvestigationFindingID(environment, changeItem, class, rule)
	ChangeItem string       `json:"changeItem"`
	Class      FindingClass `json:"class"`
	// Manifestation is copied from the change item (checked against the
	// bundle); it decides which validations can clear the item.
	Manifestation Manifestation `json:"manifestation"`
	Rule          string        `json:"rule"`        // policy rule that decided the class, e.g. "classify:validated-clear"
	Reasoning     string        `json:"reasoning"`   // deterministic explanation of the class
	Environment   string        `json:"environment"` // snapshot environment name
	Components    []string      `json:"components,omitempty"`
	// RenderedChanges are the environment-scope rendered changes correlated
	// to the item: the exposure evidence (empty = the render shows none).
	RenderedChanges []string `json:"renderedChanges,omitempty"`
	Paths           []string `json:"paths,omitempty"`
	Validations     []string `json:"validations,omitempty"`
	// Assertions are the agent assertions related to the item (explanations
	// and concerns); Assumptions the subset that no validation established —
	// shown as unvalidated assumptions (AC6).
	Assertions     []string           `json:"assertions,omitempty"`
	Assumptions    []string           `json:"assumptions,omitempty"`
	ImpactFindings []ImpactFindingRef `json:"impactFindings,omitempty"`
	Evidence       []EvidenceID       `json:"evidence"`
	Artifacts      []string           `json:"artifacts,omitempty"`
	Gaps           []EvidenceGap      `json:"gaps,omitempty"`
	Remediation    string             `json:"remediation,omitempty"`
	NextSteps      []string           `json:"nextSteps,omitempty"`
	// Basis is the strongest validation level the class rests on; empty when
	// it rests on none (potentially-affected / insufficient-evidence).
	Basis      ValidationLevel     `json:"basis,omitempty"`
	Approval   ApprovalEligibility `json:"approval"`
	Provenance Provenance          `json:"provenance"` // deterministic: the policy computed it
}

// InvestigationFindingID derives a finding id.
func InvestigationFindingID(environment, changeItem string, class FindingClass, rule string) string {
	return InvestigationFindingIDPrefix + ShortHash(environment, changeItem, string(class), rule)
}

// Validate checks the class-specific shape of a finding (the self-contained
// half of the §3 policy; Investigation.Validate checks the references).
func (f InvestigationFinding) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("finding %s: %s", f.ID, fmt.Sprintf(format, args...)))
	}
	if f.ID != InvestigationFindingID(f.Environment, f.ChangeItem, f.Class, f.Rule) {
		bad("id is not derived from environment+changeItem+class+rule")
	}
	if !oneOf(f.Class, FindingClasses) {
		bad("unknown class %q", f.Class)
	}
	if !oneOf(f.Manifestation, Manifestations) {
		bad("unknown manifestation %q", f.Manifestation)
	}
	if !strings.HasPrefix(f.ChangeItem, ChangeItemIDPrefix) || f.Rule == "" || f.Environment == "" {
		bad("changeItem, rule and environment are required")
	}
	if strings.TrimSpace(f.Reasoning) == "" {
		bad("reasoning is required")
	}
	if len(f.Evidence) == 0 {
		bad("evidence is required (at least the change item's upstream chain)")
	}
	if f.Provenance.Method == MethodAI {
		bad("findings are computed by the deterministic policy, never by a model")
	}
	if err := f.Provenance.Validate(); err != nil {
		bad("provenance: %v", err)
	}
	if f.Basis != "" && !oneOf(f.Basis, ValidationLevels) {
		bad("unknown basis %q", f.Basis)
	}
	for _, g := range f.Gaps {
		if !oneOf(g.Kind, GapKinds) || g.Subject == "" || g.Detail == "" {
			bad("gap needs a known kind, subject and detail")
		}
		if g.UnknownReason != "" && !g.UnknownReason.Valid() {
			bad("gap: unknown unknownReason %q", g.UnknownReason)
		}
	}
	actionRequired := false
	for _, r := range f.ImpactFindings {
		if r.Classification == ImpactActionRequired {
			actionRequired = true
		}
	}
	// ACTION REQUIRED is never weakened: a change the deterministic join
	// classifies action-required is affected here.
	if actionRequired && f.Class != FindingAffected {
		bad("the deterministic join classifies the change action-required; the finding must be affected")
	}
	switch f.Class {
	case FindingAffected:
		if len(f.Validations) == 0 || f.Basis == "" {
			bad("affected requires a failing deterministic or observed validation (join verdicts are wrapped as validation records)")
		}
	case FindingNotAffected:
		if len(f.Gaps) > 0 {
			bad("not-affected carries no evidence gaps")
		}
		if len(f.Validations) == 0 || f.Basis == "" {
			bad("not-affected requires deterministic or observed validations")
		}
		if len(f.Assumptions) > 0 {
			bad("not-affected rests on no unvalidated assumption")
		}
	case FindingPotentiallyAffected:
		if len(f.NextSteps) == 0 {
			bad("potentially-affected requires the next investigation steps")
		}
	case FindingInsufficientEvidence:
		if len(f.Gaps) == 0 {
			bad("insufficient-evidence names its evidence gaps")
		}
		if len(f.NextSteps) == 0 {
			bad("insufficient-evidence requires the next investigation steps")
		}
	}
	a := f.Approval
	if a.Eligible && f.Class != FindingNotAffected {
		bad("only a not-affected finding can be eligible for automated approval")
	}
	if a.RequiresHumanReview == a.Eligible {
		bad("requiresHumanReview must be the negation of eligible")
	}
	if a.Eligible != (len(a.Blockers) == 0) {
		bad("blockers are listed exactly when not eligible")
	}
	return errors.Join(errs...)
}

// PolicyOutcome is the investigation-level decision.
type PolicyOutcome string

const (
	PolicyAutoApproveEligible PolicyOutcome = "auto-approve-eligible"
	PolicyHumanReview         PolicyOutcome = "human-review-required"
)

// PolicyOutcomes lists every outcome.
var PolicyOutcomes = []PolicyOutcome{PolicyAutoApproveEligible, PolicyHumanReview}

// PolicyDecision is the deterministic approval decision over all findings.
type PolicyDecision struct {
	Policy  string        `json:"policy"` // e.g. "classify.policy@v1"
	Outcome PolicyOutcome `json:"outcome"`
	Reasons []string      `json:"reasons"`
	// AutoApprovalEnabled is false while automated approval runs in shadow
	// mode (first milestone): eligibility is computed and measured, never
	// enacted. It may be true only for an eligible outcome.
	AutoApprovalEnabled bool `json:"autoApprovalEnabled,omitempty"`
}

// ReviewState is the human-review state of an investigation (AC8).
type ReviewState string

const (
	ReviewNotRequired        ReviewState = "not-required"
	ReviewAwaiting           ReviewState = "pending"
	ReviewApproved           ReviewState = "approved"
	ReviewRejected           ReviewState = "rejected"
	ReviewMoreEvidenceWanted ReviewState = "more-evidence-requested"
)

// ReviewStates lists every state.
var ReviewStates = []ReviewState{ReviewNotRequired, ReviewAwaiting, ReviewApproved, ReviewRejected, ReviewMoreEvidenceWanted}

// HumanReview records the review of an investigation. Only a human can
// approve; a proxy (AI reviewer) decision is recorded, labelled, and never
// counts as approval.
type HumanReview struct {
	State        ReviewState  `json:"state"`
	Reviewer     string       `json:"reviewer,omitempty"`
	ReviewerKind ReviewerKind `json:"reviewerKind,omitempty"`
	DecidedAt    *time.Time   `json:"decidedAt,omitempty"`
	Note         string       `json:"note,omitempty"`
	// ProxyProvenance is required for a proxy reviewer.
	ProxyProvenance *Provenance `json:"proxyProvenance,omitempty"`
}

// InvestigationStatus is the lifecycle state of the run.
type InvestigationStatus string

const (
	InvestigationRunning   InvestigationStatus = "running"
	InvestigationAwaiting  InvestigationStatus = "awaiting-review"
	InvestigationCompleted InvestigationStatus = "completed"
	InvestigationFailed    InvestigationStatus = "failed"
	InvestigationCancelled InvestigationStatus = "cancelled"
)

// InvestigationStatuses lists every status.
var InvestigationStatuses = []InvestigationStatus{InvestigationRunning, InvestigationAwaiting, InvestigationCompleted, InvestigationFailed, InvestigationCancelled}

// AgentRunStatus is how the agent stage ended.
type AgentRunStatus string

const (
	AgentNotRun          AgentRunStatus = "not-run"
	AgentSubmitted       AgentRunStatus = "submitted"        // the agent called submit_findings
	AgentBudgetExhausted AgentRunStatus = "budget-exhausted" // a turn/tool/token/wall-clock budget ended it
	AgentFailed          AgentRunStatus = "failed"
)

// AgentRunStatuses lists every status.
var AgentRunStatuses = []AgentRunStatus{AgentNotRun, AgentSubmitted, AgentBudgetExhausted, AgentFailed}

// AgentBudget bounds one agent run (DESIGN.md §4.3).
type AgentBudget struct {
	MaxTurns     int   `json:"maxTurns"`
	MaxToolCalls int   `json:"maxToolCalls"`
	MaxTokens    int   `json:"maxTokens"`    // input+output tokens as reported by the provider
	MaxReadBytes int64 `json:"maxReadBytes"` // total artifact bytes returned to the model
	MaxSeconds   int   `json:"maxSeconds"`   // wall clock
}

// AgentRun summarises the agent stage. The transcript artifact holds every
// tool call and result, so the run can be replayed (DESIGN.md §4.5).
type AgentRun struct {
	Status        AgentRunStatus `json:"status"`
	Reason        string         `json:"reason,omitempty"` // required unless submitted
	Model         string         `json:"model,omitempty"`
	Provider      string         `json:"provider,omitempty"`
	PromptVersion string         `json:"promptVersion,omitempty"`
	PromptDigest  string         `json:"promptDigest,omitempty"`
	CallID        string         `json:"callId,omitempty"`
	Budget        AgentBudget    `json:"budget"`
	Turns         int            `json:"turns"`
	ToolCalls     int            `json:"toolCalls"`
	Tokens        int            `json:"tokens"`
	Transcript    string         `json:"transcript,omitempty"` // agent-transcript artifact id
	// Coverage is the agent's per-item account of its agenda: every in-scope
	// item the deterministic pass did not already decide, in priority order,
	// with the disposition the agent closed it with (or not-examined).
	// BudgetRule names the formula that scaled Budget to the agenda size.
	// CONTRACT-CHANGE(agent): additive (commander decision 2026-10-16:
	// coverage must not depend on the model's diligence).
	Coverage   []AgendaCoverage `json:"coverage,omitempty"`
	BudgetRule string           `json:"budgetRule,omitempty"`
	// ItemCap is the per-investigation cap on per-item sessions (prompt
	// feature perItem); agenda items beyond it are closed not-examined by the
	// runner. CONTRACT-CHANGE(agent): additive.
	ItemCap int `json:"itemCap,omitempty"`
	// Cost of the investigation (lane eval5): per-item sessions run, wall
	// time, and citations / verified facts the tool layer dropped from
	// verdicts. CONTRACT-CHANGE(verdict): additive.
	Sessions             int   `json:"sessions,omitempty"`
	WallMillis           int64 `json:"wallMillis,omitempty"`
	DroppedCitations     int   `json:"droppedCitations,omitempty"`
	DroppedVerifiedFacts int   `json:"droppedVerifiedFacts,omitempty"`
}

// CoverageDisposition is how the agent closed an agenda item.
// CONTRACT-CHANGE(agent): additive.
type CoverageDisposition string

const (
	// CoverRaisesConcern, CoverClaimsMitigation and CoverExplains close the
	// item with an assertion of that stance.
	CoverRaisesConcern    CoverageDisposition = "raises-concern"
	CoverClaimsMitigation CoverageDisposition = "claims-mitigation"
	CoverExplains         CoverageDisposition = "explains"
	// CoverExaminedNoConcern: the agent inspected the item (it cites what it
	// inspected) and has no concern to raise. Recorded as an explains
	// assertion. It is NOT a mitigation and can never lower a class (DESIGN.md
	// §3.3): an absent concern is not evidence of safety.
	CoverExaminedNoConcern CoverageDisposition = "examined-no-concern"
	// CoverNotExamined: the agent did not examine the item (the reason, e.g.
	// the budget, is stated). No assertion.
	CoverNotExamined CoverageDisposition = "not-examined"
)

// CoverageDispositions lists every disposition.
var CoverageDispositions = []CoverageDisposition{CoverRaisesConcern, CoverClaimsMitigation, CoverExplains, CoverExaminedNoConcern, CoverNotExamined}

// AgendaCoverage is one agenda item and how the agent closed it.
// CONTRACT-CHANGE(agent): additive.
type AgendaCoverage struct {
	ChangeItem string `json:"changeItem"`
	// Priority orders the agenda (0 first); PriorityRule is the declared
	// agenda rule that placed the item, e.g. "agenda:behaviour".
	Priority     int                 `json:"priority"`
	PriorityRule string              `json:"priorityRule"`
	Disposition  CoverageDisposition `json:"disposition"`
	// Summary is the agent's closing summary, or the reason for not-examined.
	Summary string `json:"summary"`
	// Assertion is the closing assertion (none for not-examined); Records
	// every hypothesis/assertion the run made about the item; Citations the
	// evidence the closing entry cites.
	Assertion string       `json:"assertion,omitempty"`
	Records   []string     `json:"records,omitempty"`
	Citations []EvidenceID `json:"citations,omitempty"`
	// Depth: Examined = the agent retrieved the item (get_change_item);
	// Inspections = accepted query calls naming the item (get_change_item,
	// query_diff, search_manifest, list_paths); ValidationRequests = recorded
	// requests for the item. CONTRACT-CHANGE(agent): additive.
	Examined           bool `json:"examined,omitempty"`
	Inspections        int  `json:"inspections,omitempty"`
	ValidationRequests int  `json:"validationRequests,omitempty"`
	// Per-item sessions (prompt feature perItem): the item's own session —
	// its call id, status and reason, and what it cost.
	// CONTRACT-CHANGE(agent): additive.
	CallID        string         `json:"callId,omitempty"`
	SessionStatus AgentRunStatus `json:"sessionStatus,omitempty"`
	SessionReason string         `json:"sessionReason,omitempty"`
	Turns         int            `json:"turns,omitempty"`
	ToolCalls     int            `json:"toolCalls,omitempty"`
	Tokens        int            `json:"tokens,omitempty"`
	// SummaryKind labels an agent-written Summary ("unverified-ai-prose");
	// empty for runner-written summaries. CONTRACT-CHANGE(agent): additive.
	SummaryKind string `json:"summaryKind,omitempty"`
	// Dossier is the item-dossier artifact the item's session started from;
	// Verdict the agent's provisional verdict on the item.
	// CONTRACT-CHANGE(verdict): additive.
	Dossier string `json:"dossier,omitempty"`
	Verdict string `json:"verdict,omitempty"`
}

// AgentNote is an investigation-level remark of the agent: a concern that
// could not be attributed to an item's own evidence (it cites no artifact
// retrieved in that item's session, or was raised in another item's
// session). It is AI output, labelled, and never an item concern: it
// cannot block or change any class. CONTRACT-CHANGE(agent): additive.
type AgentNote struct {
	ID string `json:"id"` // AgentRecordID(AgentNoteIDPrefix, about, statement, callId)
	// About is the change item the agent meant, when it named one.
	About         string       `json:"about,omitempty"`
	Statement     string       `json:"statement"`
	Reason        string       `json:"reason"` // why it is a note, not an item concern
	Citations     []EvidenceID `json:"citations,omitempty"`
	StatementKind string       `json:"statementKind,omitempty"` // see AgentAssertion
	Provenance    Provenance   `json:"provenance"`              // method ai
}

// Validate checks the note.
func (n AgentNote) Validate() error {
	errs := validateAI("agent note "+n.ID, n.Provenance)
	if n.ID != AgentRecordID(AgentNoteIDPrefix, n.About, n.Statement, n.Provenance.CallID) {
		errs = append(errs, fmt.Errorf("agent note %s: id is not derived", n.ID))
	}
	if strings.TrimSpace(n.Statement) == "" || strings.TrimSpace(n.Reason) == "" {
		errs = append(errs, fmt.Errorf("agent note %s: statement and reason are required", n.ID))
	}
	return errors.Join(errs...)
}

// closingStance is the assertion stance a disposition is recorded with.
func (d CoverageDisposition) closingStance() (AssertionStance, bool) {
	switch d {
	case CoverRaisesConcern:
		return StanceRaisesConcern, true
	case CoverClaimsMitigation:
		return StanceClaimsMitigation, true
	case CoverExplains, CoverExaminedNoConcern:
		return StanceExplains, true
	}
	return "", false
}

// Stage names a pipeline stage (the orchestration activities, DESIGN.md §5).
type Stage string

const (
	StageCollectChange Stage = "collect-change-evidence"
	StageBuildSnapshot Stage = "build-snapshot"
	StageAssemble      Stage = "assemble-bundle"
	StageBuildGraph    Stage = "build-graph"
	StageTraverse      Stage = "traverse"
	StageInvestigate   Stage = "investigate"
	StageValidate      Stage = "validate"
	StageClassify      Stage = "classify"
	StageHumanReview   Stage = "human-review"
)

// Stages lists every stage in pipeline order.
var Stages = []Stage{StageCollectChange, StageBuildSnapshot, StageAssemble, StageBuildGraph, StageTraverse, StageInvestigate, StageValidate, StageClassify, StageHumanReview}

// AuditEvent is one entry of the append-only audit trail. Events form a hash
// chain: Prev is the Digest of the previous event ("" for the first), so a
// rewritten history fails validation.
type AuditEvent struct {
	Seq    int       `json:"seq"` // 1-based, contiguous
	At     time.Time `json:"at"`
	Stage  Stage     `json:"stage"`
	Event  string    `json:"event"` // started | completed | failed | retried | requested | decided | cancelled | ...
	Actor  string    `json:"actor"` // "system", "agent:<model>", "human:<name>", "proxy:<model>"
	Refs   []string  `json:"refs,omitempty"`
	Detail string    `json:"detail,omitempty"`
	Prev   string    `json:"prev,omitempty"`
}

// Digest pins the event (including Prev, which chains it).
func (e AuditEvent) Digest() string { return canonicalDigest(e) }

// Investigation is the run record: what was investigated, everything the
// run produced, the decision and the review, and the audit trail. Large
// outputs (graph, renders, transcript) are derived artifacts referenced by id.
type Investigation struct {
	SchemaVersion string `json:"schemaVersion"`
	// ID identifies the REQUEST: InvestigationID(bundle, config), so a
	// re-submission of the same bundle and configuration is idempotent.
	ID           string              `json:"id"`
	Bundle       string              `json:"bundle"`
	Snapshot     string              `json:"snapshot"`
	ChangeRecord string              `json:"changeRecord"`
	Environment  string              `json:"environment"`
	Config       string              `json:"config"` // digest of the run configuration (policy, bounds, validators, agent model/prompt)
	Status       InvestigationStatus `json:"status"`
	// ImpactReport is the artifact id of the deterministic ImpactReport the
	// investigation builds on; Graph the RelationshipGraph artifact.
	ImpactReport string `json:"impactReport,omitempty"`
	// ReportStamps are the wall-clock stamps of that report (when the join
	// generated it, when each evidence record was retrieved): the artifact
	// holds the report's identified content without them
	// (ImpactReport.IdentifiedContent), and they are kept here as provenance
	// of the run — excluded from the run record's identity and digests like
	// CreatedAt and audit times (timestamps are never identity, §2.1).
	// CONTRACT-CHANGE(fixes2): additive.
	ReportStamps *ReportStamps    `json:"reportStamps,omitempty"`
	Graph        string           `json:"graph,omitempty"`
	Traversal    *TraversalResult `json:"traversal,omitempty"`
	Agent        *AgentRun        `json:"agent,omitempty"`
	Hypotheses   []Hypothesis     `json:"hypotheses,omitempty"`
	Assertions   []AgentAssertion `json:"assertions,omitempty"`
	// AgentNotes: investigation-level agent remarks (AI), never item concerns.
	// CONTRACT-CHANGE(agent): additive.
	AgentNotes []AgentNote `json:"agentNotes,omitempty"`
	// AgentClaims: the agent's verified environment claims.
	// CONTRACT-CHANGE(agent): additive.
	AgentClaims []SettingClaim `json:"agentClaims,omitempty"`
	// AgentVerdicts: the agent's provisional verdicts, one per investigated
	// item; never read by the classification policy (PROTOCOL §8).
	// CONTRACT-CHANGE(verdict): additive.
	AgentVerdicts      []AgentVerdict         `json:"agentVerdicts,omitempty"`
	EvidenceRequests   []EvidenceRequest      `json:"evidenceRequests,omitempty"`
	ValidationRequests []ValidationRequest    `json:"validationRequests,omitempty"`
	Validations        []ValidationRecord     `json:"validations,omitempty"`
	Findings           []InvestigationFinding `json:"findings,omitempty"`
	Decision           *PolicyDecision        `json:"decision,omitempty"`
	Review             HumanReview            `json:"review"`
	// Artifacts are the run's derived artifacts (graph, renders, validator
	// outputs, transcript, fulfilled evidence requests); Evidence the
	// records the run created (agent citations, validator evidence). Ids
	// cited anywhere resolve here or in the bundle.
	Artifacts []ArtifactRef `json:"artifacts,omitempty"`
	Evidence  []Evidence    `json:"evidence,omitempty"`
	Audit     []AuditEvent  `json:"audit"`
	CreatedAt time.Time     `json:"createdAt"`
}

// InvestigationID derives the investigation (request) id.
func InvestigationID(bundleID, configDigest string) string {
	return InvestigationIDPrefix + ShortHash(bundleID, configDigest)
}

// Validate checks the self-contained invariants of the run record:
// identity, every record, referential integrity among the run's own
// records, one finding per change item, the §3 rules that need the run
// context, the decision and review, and the audit hash chain.
// ValidateAgainst additionally resolves ids in the bundle.
func (inv *Investigation) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("investigation %s: %s", inv.ID, fmt.Sprintf(format, args...)))
	}
	if inv.SchemaVersion != InvestigationSchemaVersion {
		bad("schemaVersion = %q, want %q", inv.SchemaVersion, InvestigationSchemaVersion)
	}
	if inv.ID != InvestigationID(inv.Bundle, inv.Config) {
		bad("id is not derived from bundle+config")
	}
	if !strings.HasPrefix(inv.Bundle, BundleIDPrefix) || !strings.HasPrefix(inv.Snapshot, SnapshotIDPrefix) ||
		!strings.HasPrefix(inv.ChangeRecord, ChangeRecordIDPrefix) || inv.Environment == "" || !isDigest(inv.Config) {
		bad("bundle, snapshot, changeRecord, environment and config (a digest) are required")
	}
	if inv.ReportStamps != nil && inv.ImpactReport == "" {
		bad("reportStamps are the provenance of an impact report artifact, and there is none")
	}
	if !oneOf(inv.Status, InvestigationStatuses) {
		bad("unknown status %q", inv.Status)
	}

	arts := map[string]bool{}
	for _, a := range inv.Artifacts {
		if err := a.Validate(); err != nil {
			errs = append(errs, err)
		}
		arts[a.ID] = true
	}
	ev := map[EvidenceID]bool{}
	for _, e := range inv.Evidence {
		if e.ID == "" || e.URI == "" || ev[e.ID] {
			bad("run evidence needs a unique id and a uri (%s)", e.ID)
		}
		ev[e.ID] = true
	}

	paths := map[string]ImpactPath{}
	if t := inv.Traversal; t != nil {
		if err := t.Validate(); err != nil {
			errs = append(errs, err)
		}
		for _, p := range t.Paths {
			paths[p.ID] = p
		}
	}

	hyps := map[string]bool{}
	for _, h := range inv.Hypotheses {
		if err := h.Validate(); err != nil {
			errs = append(errs, err)
		}
		hyps[h.ID] = true
	}
	asserts := map[string]AgentAssertion{}
	for _, a := range inv.Assertions {
		if err := a.Validate(); err != nil {
			errs = append(errs, err)
		}
		if a.Hypothesis != "" && !hyps[a.Hypothesis] {
			bad("assertion %s names unknown hypothesis %s", a.ID, a.Hypothesis)
		}
		for _, p := range a.Paths {
			if _, ok := paths[p]; !ok {
				bad("assertion %s names unknown path %s", a.ID, p)
			}
		}
		asserts[a.ID] = a
	}
	claims := map[string]bool{}
	for _, c := range inv.AgentClaims {
		if err := c.Validate(); err != nil {
			errs = append(errs, err)
		}
		if claims[c.ID] {
			bad("duplicate setting claim %s", c.ID)
		}
		claims[c.ID] = true
	}
	for _, a := range inv.Assertions {
		for _, c := range a.Claims {
			if !claims[c] {
				bad("assertion %s names unknown setting claim %s", a.ID, c)
			}
		}
	}
	verdictItems := map[string]bool{}
	for _, v := range inv.AgentVerdicts {
		if err := v.Validate(); err != nil {
			errs = append(errs, err)
		}
		if verdictItems[v.ChangeItem] {
			bad("two agent verdicts for change item %s", v.ChangeItem)
		}
		verdictItems[v.ChangeItem] = true
		for _, h := range v.Mitigations {
			if !hyps[h] {
				bad("agent verdict %s names unknown hypothesis %s", v.ID, h)
			}
		}
		for _, f := range v.VerifiedFacts {
			if strings.HasPrefix(f, SettingClaimIDPrefix) && !claims[f] {
				bad("agent verdict %s names unknown setting claim %s", v.ID, f)
			}
		}
	}
	notes := map[string]bool{}
	for _, n := range inv.AgentNotes {
		if err := n.Validate(); err != nil {
			errs = append(errs, err)
		}
		if notes[n.ID] {
			bad("duplicate agent note %s", n.ID)
		}
		notes[n.ID] = true
	}
	for _, r := range inv.EvidenceRequests {
		if err := r.Validate(); err != nil {
			errs = append(errs, err)
		}
		for _, id := range r.Fulfilled {
			if !arts[id] {
				bad("evidence request %s: fulfilled artifact %s is not a run artifact", r.ID, id)
			}
		}
	}
	vals := map[string]ValidationRecord{}
	for _, v := range inv.Validations {
		if err := v.Validate(); err != nil {
			errs = append(errs, err)
		}
		if _, dup := vals[v.ID]; dup {
			bad("duplicate validation %s", v.ID)
		}
		vals[v.ID] = v
		for _, p := range v.Paths {
			if pp, ok := paths[p]; !ok || pp.ChangeItem != v.ChangeItem {
				bad("validation %s covers unknown path %s (or a path of another item)", v.ID, p)
			}
		}
		for _, a := range v.Answers {
			if _, ok := asserts[a]; !ok && !hyps[a] {
				bad("validation %s answers unknown assertion/hypothesis %s", v.ID, a)
			}
		}
	}
	vreqs := map[string]bool{}
	reqByID := map[string]ValidationRequest{}
	for _, r := range inv.ValidationRequests {
		if err := r.Validate(); err != nil {
			errs = append(errs, err)
		}
		vreqs[r.ID] = true
		reqByID[r.ID] = r
		for _, a := range r.Answers {
			if as, ok := asserts[a]; !ok || as.ChangeItem != r.ChangeItem {
				bad("validation request %s answers unknown assertion %s (or one about another item)", r.ID, a)
			}
		}
		if r.Result != "" {
			if v, ok := vals[r.Result]; !ok || v.Request != r.ID {
				bad("validation request %s: result %s is not a validation run for it", r.ID, r.Result)
			}
		}
	}
	for _, v := range inv.Validations {
		if v.Request != "" && !vreqs[v.Request] {
			bad("validation %s names unknown request %s", v.ID, v.Request)
		}
		// an undecided record echoes what it was asked about (EchoRequested)
		if r, ok := reqByID[v.Request]; ok && (v.Outcome == ValidationNotRun || v.Outcome == ValidationError) {
			for _, s := range r.Subjects {
				if !contains(v.Subjects, s) {
					bad("validation %s (%s) does not echo subject %s its request %s asked about", v.ID, v.Outcome, s, r.ID)
				}
			}
		}
	}

	byItem := map[string]bool{}
	allEligible := len(inv.Findings) > 0
	needsReview := false
	for _, f := range inv.Findings {
		if err := f.Validate(); err != nil {
			errs = append(errs, err)
		}
		if byItem[f.ChangeItem] {
			bad("change item %s has more than one finding", f.ChangeItem)
		}
		byItem[f.ChangeItem] = true
		if f.Environment != inv.Environment {
			bad("finding %s is for environment %q, not %q", f.ID, f.Environment, inv.Environment)
		}
		if !f.Approval.Eligible {
			allEligible = false
		}
		if f.Approval.RequiresHumanReview {
			needsReview = true
		}
		errs = append(errs, inv.validateFindingRefs(f, paths, vals, asserts)...)
	}

	if d := inv.Decision; d != nil {
		if d.Policy == "" || !oneOf(d.Outcome, PolicyOutcomes) || len(d.Reasons) == 0 {
			bad("decision needs policy, a known outcome and reasons")
		}
		// Eligible ⇒ every finding eligible. Not the converse: §3.4 adds
		// investigation-level conjuncts (agent completed, complete traversal and
		// renders, no uncorrelated change, tier policy), so all findings may be
		// eligible while the decision is human-review-required.
		// CONTRACT-CHANGE(classify): was "⇔", which left no valid decision when
		// every item is not-affected but e.g. the agent did not complete.
		if d.Outcome == PolicyAutoApproveEligible && !allEligible {
			bad("decision %s contradicts the findings (all eligible = %v)", d.Outcome, allEligible)
		}
		if d.AutoApprovalEnabled && d.Outcome != PolicyAutoApproveEligible {
			bad("automated approval can only be enabled for an eligible outcome")
		}
		if d.Outcome == PolicyAutoApproveEligible && (inv.Agent == nil || inv.Agent.Status != AgentSubmitted) {
			bad("automated approval requires a completed agent stage")
		}
		if d.Outcome == PolicyAutoApproveEligible && (inv.Traversal == nil || !inv.Traversal.Complete) {
			bad("automated approval requires a complete traversal")
		}
	}
	errs = append(errs, inv.validateReview(needsReview)...)
	if a := inv.Agent; a != nil {
		if !oneOf(a.Status, AgentRunStatuses) {
			bad("agent: unknown status %q", a.Status)
		}
		if a.Status != AgentSubmitted && strings.TrimSpace(a.Reason) == "" {
			bad("agent: status %s requires a reason", a.Status)
		}
		if a.Status != AgentNotRun && (a.Model == "" || a.PromptVersion == "" || a.PromptDigest == "" || a.Transcript == "") {
			bad("agent: a run records model, promptVersion, promptDigest and its transcript")
		}
		if a.Transcript != "" && !arts[a.Transcript] {
			bad("agent: transcript %s is not a run artifact", a.Transcript)
		}
		seenCov := map[string]bool{}
		for _, c := range a.Coverage {
			if seenCov[c.ChangeItem] {
				bad("agent: coverage lists %s twice", c.ChangeItem)
			}
			seenCov[c.ChangeItem] = true
			if !strings.HasPrefix(c.ChangeItem, ChangeItemIDPrefix) || !strings.HasPrefix(c.PriorityRule, "agenda:") || strings.TrimSpace(c.Summary) == "" {
				bad("agent: coverage of %s needs a change item, an agenda:* rule and a summary or reason", c.ChangeItem)
			}
			stance, closes := c.Disposition.closingStance()
			switch {
			case !oneOf(c.Disposition, CoverageDispositions):
				bad("agent: coverage of %s: unknown disposition %q", c.ChangeItem, c.Disposition)
			case !closes && c.Assertion != "":
				bad("agent: coverage of %s is not-examined but names assertion %s", c.ChangeItem, c.Assertion)
			case closes:
				as, ok := asserts[c.Assertion]
				if !ok || as.ChangeItem != c.ChangeItem || as.Stance != stance {
					bad("agent: coverage of %s (%s) needs its closing %s assertion about the item (got %q)", c.ChangeItem, c.Disposition, stance, c.Assertion)
				}
			}
		}
	}
	errs = append(errs, validateAudit(inv.Audit)...)
	return errors.Join(errs...)
}

// validateFindingRefs checks one finding against the run's records — the
// §3 rules that need context.
func (inv *Investigation) validateFindingRefs(f InvestigationFinding, paths map[string]ImpactPath, vals map[string]ValidationRecord, asserts map[string]AgentAssertion) []error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("finding %s: %s", f.ID, fmt.Sprintf(format, args...)))
	}
	for _, p := range f.Paths {
		if pp, ok := paths[p]; !ok || pp.ChangeItem != f.ChangeItem {
			bad("path %s is not a path of %s", p, f.ChangeItem)
		}
	}
	strongest := ValidationLevel("")
	failing := false
	for _, id := range f.Validations {
		v, ok := vals[id]
		if !ok || v.ChangeItem != f.ChangeItem {
			bad("validation %s is not a validation of %s", id, f.ChangeItem)
			continue
		}
		if v.Outcome == ValidationFail {
			failing = true
		}
		if v.Outcome.Decisive() && (strongest == "" || v.Level == LevelObserved) {
			strongest = v.Level
		}
	}
	for _, id := range append(append([]string(nil), f.Assertions...), f.Assumptions...) {
		if a, ok := asserts[id]; !ok || a.ChangeItem != f.ChangeItem {
			bad("assertion %s is not an assertion about %s", id, f.ChangeItem)
		}
	}
	for _, id := range f.Assumptions {
		if !contains(f.Assertions, id) {
			bad("assumption %s must also be listed in assertions", id)
		}
	}
	if f.Basis != "" && f.Basis != strongest {
		bad("basis %s, want %q (the strongest decisive validation it cites)", f.Basis, strongest)
	}
	if f.Class == FindingAffected && !failing {
		bad("affected requires a failing validation among those it cites")
	}
	if f.Class != FindingNotAffected {
		return errs
	}
	// §3.1 — the rules that make not-affected defensible.
	if inv.Traversal == nil {
		bad("not-affected requires a traversal")
		return errs
	}
	if !inv.Traversal.CompleteFor(f.ChangeItem) {
		bad("not-affected requires a complete traversal for the item")
	}
	for _, v := range inv.Validations {
		if v.ChangeItem == f.ChangeItem && v.Outcome != ValidationPass && v.Outcome != ValidationNotRun {
			bad("not-affected contradicts validation %s (outcome %s)", v.ID, v.Outcome)
		}
	}
	covered := map[string]bool{}
	answered := map[string]bool{}
	clearing := 0
	for _, id := range f.Validations {
		v := vals[id]
		if v.Outcome != ValidationPass {
			bad("not-affected cites validation %s whose outcome is %s", id, v.Outcome)
		}
		// R2.4: for an item whose effect is not in rendered configuration, a
		// render that shows nothing proves nothing; only an established pin or
		// an observed outcome covers paths, roots, rendered changes or concerns.
		if !f.Manifestation.RenderDecisive() && !v.ClearsBehaviour() {
			continue
		}
		clearing++
		for _, a := range v.Answers {
			answered[a] = true
		}
		for _, p := range v.Paths {
			covered[p] = true
		}
		for _, rc := range v.RenderedChanges {
			covered["rc:"+rc] = true
		}
		for _, r := range v.Subjects {
			covered["root:"+r] = true
		}
	}
	if clearing == 0 {
		if f.Manifestation.RenderDecisive() {
			bad("not-affected requires at least one passing validation")
		} else {
			bad("item manifests as %s: not-affected requires an override inspection that establishes a pinned setting, or an observed validation (render silence is not decisive)", f.Manifestation)
		}
	}
	for _, p := range inv.Traversal.PathsOf(f.ChangeItem) {
		if !covered[p.ID] {
			bad("path %s of the item is covered by no passing validation", p.ID)
		}
	}
	for _, rc := range f.RenderedChanges {
		if !covered["rc:"+rc] {
			bad("rendered change %s of the item is shown harmless by no passing validation", rc)
		}
	}
	if len(inv.Traversal.PathsOf(f.ChangeItem)) == 0 {
		for _, r := range inv.Traversal.RootsOf(f.ChangeItem) {
			if !covered["root:"+r.Node] {
				bad("exposure root %s is covered by no passing validation", r.Node)
			}
		}
	}
	for _, a := range inv.Assertions {
		if a.ChangeItem == f.ChangeItem && a.Stance == StanceRaisesConcern && !answered[a.ID] {
			bad("agent concern %s is not answered by a passing validation", a.ID)
		}
	}
	return errs
}

func (inv *Investigation) validateReview(needsReview bool) []error {
	var errs []error
	r := inv.Review
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("investigation %s: review: %s", inv.ID, fmt.Sprintf(format, args...)))
	}
	if !oneOf(r.State, ReviewStates) {
		bad("unknown state %q", r.State)
	}
	if needsReview && r.State == ReviewNotRequired {
		bad("a finding requires human review (AC8); the review cannot be not-required")
	}
	decided := r.State == ReviewApproved || r.State == ReviewRejected || r.State == ReviewMoreEvidenceWanted
	if decided && (r.Reviewer == "" || r.DecidedAt == nil) {
		bad("a decided review records reviewer and decidedAt")
	}
	if r.ReviewerKind != "" && r.ReviewerKind != ReviewerHuman && r.ReviewerKind != ReviewerProxy {
		bad("unknown reviewerKind %q", r.ReviewerKind)
	}
	if r.ReviewerKind == ReviewerProxy {
		if r.ProxyProvenance == nil || r.ProxyProvenance.Method != MethodAI {
			bad("a proxy reviewer carries ai provenance")
		}
		if r.State == ReviewApproved {
			bad("a proxy (AI) reviewer can never approve")
		}
	}
	if r.State == ReviewApproved && r.ReviewerKind != ReviewerHuman {
		bad("approval requires a human reviewer")
	}
	return errs
}

func validateAudit(events []AuditEvent) []error {
	var errs []error
	if len(events) == 0 {
		return []error{errors.New("audit: the trail must not be empty")}
	}
	prev := ""
	var last time.Time
	for i, e := range events {
		if e.Seq != i+1 {
			errs = append(errs, fmt.Errorf("audit: event %d has seq %d (must be contiguous from 1)", i, e.Seq))
		}
		if e.Prev != prev {
			errs = append(errs, fmt.Errorf("audit: event %d breaks the hash chain", e.Seq))
		}
		if e.At.Before(last) {
			errs = append(errs, fmt.Errorf("audit: event %d goes back in time", e.Seq))
		}
		if !oneOf(e.Stage, Stages) || e.Event == "" || e.Actor == "" {
			errs = append(errs, fmt.Errorf("audit: event %d needs a known stage, event and actor", e.Seq))
		}
		last = e.At
		prev = e.Digest()
	}
	return errs
}

// AppendAudit appends an event to the trail, setting Seq and Prev.
func (inv *Investigation) AppendAudit(e AuditEvent) {
	e.Seq = len(inv.Audit) + 1
	e.Prev = ""
	if n := len(inv.Audit); n > 0 {
		e.Prev = inv.Audit[n-1].Digest()
	}
	inv.Audit = append(inv.Audit, e)
}

// ValidateAgainst validates the run and resolves its ids in the bundle:
// identity links, one finding per in-scope change item (once classified),
// every cited evidence id, every artifact id, every change item.
func (inv *Investigation) ValidateAgainst(b *EvidenceBundle) error {
	errs := []error{inv.Validate()}
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("investigation %s: %s", inv.ID, fmt.Sprintf(format, args...)))
	}
	if inv.Bundle != b.ID || inv.Snapshot != b.Snapshot.ID || inv.ChangeRecord != b.Change.ID || inv.Environment != b.Snapshot.Environment {
		bad("does not belong to bundle %s", b.ID)
	}
	ev := b.EvidenceByID()
	for _, e := range inv.Evidence {
		ev[e.ID] = e
	}
	arts := map[string]bool{}
	for _, a := range b.Manifest.Artifacts {
		arts[a.ID] = true
	}
	for _, a := range inv.Artifacts {
		arts[a.ID] = true
	}
	items := map[string]bool{}
	for _, it := range b.Items() {
		items[it.ID] = true
	}
	evid := func(what string, ids []EvidenceID) {
		for _, id := range ids {
			if _, ok := ev[id]; !ok {
				bad("%s: evidence %s does not resolve", what, id)
			}
		}
	}
	art := func(what string, ids ...string) {
		for _, id := range ids {
			if id != "" && !arts[id] {
				bad("%s: artifact %s does not resolve", what, id)
			}
		}
	}
	item := func(what, id string) {
		if id != "" && !items[id] {
			bad("%s: unknown change item %s", what, id)
		}
	}
	rcs := b.RenderedChanges()
	rendered := func(what string, ids []string) {
		for _, id := range ids {
			if _, ok := rcs[id]; !ok {
				bad("%s: rendered change %s is not in the bundle", what, id)
			}
		}
	}
	envCorrelated := map[string][]string{} // change item -> env-scope rendered changes correlated to it
	for _, p := range b.Renders {
		for _, c := range p.Changes {
			for _, ci := range c.ChangeItems {
				envCorrelated[ci] = append(envCorrelated[ci], c.ID)
			}
		}
	}
	art("impactReport", inv.ImpactReport)
	art("graph", inv.Graph)
	if t := inv.Traversal; t != nil {
		for _, r := range t.Roots {
			item("traversal root", r.ChangeItem)
			if r.RenderedChange != "" {
				rendered("traversal root", []string{r.RenderedChange})
			}
		}
	}
	for _, h := range inv.Hypotheses {
		item("hypothesis "+h.ID, h.ChangeItem)
		evid("hypothesis "+h.ID, h.Citations)
	}
	for _, a := range inv.Assertions {
		item("assertion "+a.ID, a.ChangeItem)
		evid("assertion "+a.ID, a.Citations)
		evid("assertion "+a.ID+" provenance", a.Provenance.InputEvidence)
		rendered("assertion "+a.ID, a.RenderedChanges)
	}
	for _, n := range inv.AgentNotes {
		item("agent note "+n.ID, n.About)
		evid("agent note "+n.ID, n.Citations)
	}
	for _, v := range inv.AgentVerdicts {
		item("agent verdict "+v.ID, v.ChangeItem)
		evid("agent verdict "+v.ID, v.Citations)
		if v.Dossier != "" {
			art("agent verdict "+v.ID, v.Dossier)
		}
	}
	for _, c := range inv.AgentClaims {
		item("setting claim "+c.ID, c.ChangeItem)
		evid("setting claim "+c.ID, c.Evidence)
		art("setting claim "+c.ID, c.Checked...)
	}
	for _, r := range inv.EvidenceRequests {
		item("evidence request "+r.ID, r.ChangeItem)
	}
	for _, r := range inv.ValidationRequests {
		item("validation request "+r.ID, r.ChangeItem)
		art("validation request "+r.ID, r.Inputs...)
	}
	for _, v := range inv.Validations {
		item("validation "+v.ID, v.ChangeItem)
		evid("validation "+v.ID, v.Evidence)
		art("validation "+v.ID, v.Inputs...)
		art("validation "+v.ID, v.Outputs...)
		rendered("validation "+v.ID, v.RenderedChanges)
		if p := v.Pin; p != nil {
			if p.RenderedObject != "" {
				found := false
				for _, rc := range rcs {
					found = found || rc.Object == p.RenderedObject
				}
				for _, pr := range b.Renders {
					if pr.Status == CollectionOK {
						found = true // the object may be in the render without having changed
					}
				}
				if !found {
					bad("validation %s: pin lands in %s but no complete environment render exists", v.ID, p.RenderedObject)
				}
			}
			for _, id := range p.Evidence {
				e, ok := ev[id]
				if !ok {
					bad("validation %s: pin evidence %s does not resolve", v.ID, id)
					continue
				}
				envKind := e.Kind == EvidenceLocalFile || e.Kind == EvidenceInput || (e.Render != nil && e.Render.Scope == RenderEnvironment)
				if !envKind {
					bad("validation %s: pin evidence %s is %s evidence; a pinned setting is established from the environment", v.ID, id, e.Kind)
				}
			}
		}
	}
	if a := inv.Agent; a != nil {
		for _, c := range a.Coverage {
			if it, ok := b.Item(c.ChangeItem); !ok || !it.InScope {
				bad("agent coverage: %s is not an in-scope change item", c.ChangeItem)
			}
			evid("agent coverage "+c.ChangeItem, c.Citations)
		}
	}
	for _, f := range inv.Findings {
		item("finding "+f.ID, f.ChangeItem)
		evid("finding "+f.ID, f.Evidence)
		art("finding "+f.ID, f.Artifacts...)
		if it, ok := b.Item(f.ChangeItem); ok {
			if !it.InScope {
				bad("finding %s classifies out-of-scope item %s", f.ID, f.ChangeItem)
			}
			if f.Manifestation != it.Manifestation {
				bad("finding %s: manifestation %s, but the change item manifests as %s", f.ID, f.Manifestation, it.Manifestation)
			}
		}
		rendered("finding "+f.ID, f.RenderedChanges)
		// The rendered diff is the primary exposure evidence: a finding lists
		// exactly the environment-scope rendered changes correlated to its item.
		want := append([]string(nil), envCorrelated[f.ChangeItem]...)
		got := append([]string(nil), f.RenderedChanges...)
		sort.Strings(want)
		sort.Strings(got)
		if strings.Join(want, ",") != strings.Join(got, ",") {
			bad("finding %s cites rendered changes %v; the item's correlated environment changes are %v", f.ID, got, want)
		}
	}
	if d := inv.Decision; d != nil && d.Outcome == PolicyAutoApproveEligible {
		if un := b.Uncorrelated(); len(un) > 0 {
			bad("automated approval is impossible while rendered changes no change item explains exist: %v", un)
		}
		if len(b.Renders) == 0 {
			bad("automated approval requires a complete environment render: absence of change is only evidence from a render")
		}
		// CONTRACT-CHANGE(renderdiff): a not-applicable pair (a version-
		// independent target, e.g. a consumer overlay that neither pins nor
		// runs the product) is not a gap; at least one pair must be complete.
		complete := false
		for _, p := range b.Renders {
			switch p.Status {
			case CollectionOK:
				complete = true
			case CollectionNotApplicable:
			default:
				bad("automated approval requires every environment render to be complete (%s is %s)", p.Target, p.Status)
			}
		}
		if len(b.Renders) > 0 && !complete {
			bad("automated approval requires a complete environment render: every environment render is not-applicable")
		}
	}
	if len(inv.Findings) > 0 {
		got := map[string]bool{}
		for _, f := range inv.Findings {
			got[f.ChangeItem] = true
		}
		for _, id := range b.InScope() {
			if !got[id] {
				bad("in-scope change item %s has no finding", id)
			}
		}
	}
	return errors.Join(errs...)
}

// --- helpers ------------------------------------------------------------------------------

func oneOf[T comparable](v T, set []T) bool {
	for _, x := range set {
		if v == x {
			return true
		}
	}
	return false
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func isDigest(s string) bool {
	h, ok := strings.CutPrefix(s, "sha256:")
	if !ok || len(h) != 64 {
		return false
	}
	for _, c := range h {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// canonicalDigest is sha256 over the JSON encoding of v (struct field order
// is fixed and map keys are sorted, so the encoding is canonical).
func canonicalDigest(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("canonicalDigest: %v", err))
	}
	return Digest(b)
}
