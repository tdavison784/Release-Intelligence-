package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// EvidenceID is a stable, content-derived identifier for an Evidence record.
type EvidenceID string

// EvidenceKind classifies what an Evidence record points at.
type EvidenceKind string

const (
	EvidenceDocument     EvidenceKind = "document"      // a text document or a section of one
	EvidenceGitRef       EvidenceKind = "git-ref"       // a tag/commit in a repository
	EvidenceRegistry     EvidenceKind = "registry"      // a registry/index entry (OCI manifest, helm index entry)
	EvidenceReleaseAsset EvidenceKind = "release-asset" // a downloadable asset attached to a release
	EvidenceStructured   EvidenceKind = "structured"    // a structured file (YAML/JSON) such as values.yaml or a CRD
	EvidenceAdvisory     EvidenceKind = "advisory"      // a security advisory record
	EvidenceRepoFile     EvidenceKind = "repo-file"     // a file inside a source repository (discovery)
	EvidenceLocalFile    EvidenceKind = "local-file"    // a file of the user's environment (values, manifests)
	EvidenceInput        EvidenceKind = "input"         // a value supplied directly (a CLI flag)
	// EvidenceRenderedDiff: one semantic difference between two renders
	// (Evidence.Render says how, at which scope). CONTRACT-CHANGE(render):
	// RENDER-MISSION R5 names this evidence kind.
	EvidenceRenderedDiff EvidenceKind = "rendered-diff"
)

// Evidence is a verifiable pointer to source material that supports a fact or
// conclusion. URI must be something a human (or a later run) can open to check
// the claim; Locator narrows it down within the document.
type Evidence struct {
	ID       EvidenceID   `json:"id"`
	Kind     EvidenceKind `json:"kind"`
	SourceID string       `json:"sourceId,omitempty"` // id of the product-definition source or artifact that produced it
	URI      string       `json:"uri"`
	Locator  string       `json:"locator,omitempty"` // e.g. "L120-L131", "## Breaking Changes", "$.spec.versions[1]"
	Excerpt  string       `json:"excerpt,omitempty"` // short verbatim excerpt (truncated)
	// ContentDigest is the sha256 of the complete retrieved document, so the
	// exact bytes the conclusion was drawn from can be identified later.
	ContentDigest string `json:"contentDigest,omitempty"`
	// Representation records which published form of the artifact these bytes
	// came from (source-tree, published-chart-tgz, ...); see Representation.
	// Empty on evidence whose origin the pipeline does not classify.
	Representation Representation `json:"representation,omitempty"`
	// Render is set on evidence that cites a field of a rendered manifest
	// (the template file is the URI/locator): how the render was produced.
	// Environment-scoped renders never enter knowledge/ (semantic.go).
	Render      *RenderProvenance `json:"render,omitempty"`
	RetrievedAt time.Time         `json:"retrievedAt,omitzero"`
}

// MaxExcerpt bounds stored excerpts.
const MaxExcerpt = 600

// NewEvidence constructs an Evidence record with a deterministic ID derived
// from kind, URI, locator and excerpt (and digest when present).
func NewEvidence(kind EvidenceKind, sourceID, uri, locator, excerpt, contentDigest string, retrievedAt time.Time) Evidence {
	excerpt = TruncateExcerpt(excerpt)
	e := Evidence{
		Kind:          kind,
		SourceID:      sourceID,
		URI:           uri,
		Locator:       locator,
		Excerpt:       excerpt,
		ContentDigest: contentDigest,
		RetrievedAt:   retrievedAt.UTC().Truncate(time.Second),
	}
	e.ID = EvidenceID("ev-" + ShortHash(string(kind), uri, locator, excerpt, contentDigest))
	return e
}

// WithRepresentation returns a copy of e annotated with the representation
// that produced it. The id is recomputed over the representation as well, so
// records that differ only in representation stay distinct; evidence without
// a representation keeps the id NewEvidence assigned it.
func (e Evidence) WithRepresentation(rep Representation) Evidence {
	if rep == "" || e.Representation == rep {
		return e
	}
	e.Representation = rep
	e.ID = EvidenceID("ev-" + ShortHash(string(e.Kind), e.URI, e.Locator, e.Excerpt, e.ContentDigest, string(rep)))
	return e
}

// TruncateExcerpt trims whitespace and bounds the length of an excerpt.
func TruncateExcerpt(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= MaxExcerpt {
		return s
	}
	cut := MaxExcerpt
	// avoid cutting a UTF-8 sequence in half
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + "…"
}

// ShortHash returns the first 12 hex chars of sha256 over the joined parts.
func ShortHash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// Digest returns the full hex sha256 of b prefixed with "sha256:".
func Digest(b []byte) string {
	s := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(s[:])
}

// EvidenceSet is an ordered, de-duplicated collection of Evidence.
type EvidenceSet struct {
	order []EvidenceID
	byID  map[EvidenceID]Evidence
}

// Add inserts evidence (idempotent) and returns its ID.
func (s *EvidenceSet) Add(e Evidence) EvidenceID {
	if s.byID == nil {
		s.byID = map[EvidenceID]Evidence{}
	}
	if _, ok := s.byID[e.ID]; !ok {
		s.order = append(s.order, e.ID)
		s.byID[e.ID] = e
	}
	return e.ID
}

// AddAll inserts many records.
func (s *EvidenceSet) AddAll(es []Evidence) {
	for _, e := range es {
		s.Add(e)
	}
}

// Get returns the evidence for id.
func (s *EvidenceSet) Get(id EvidenceID) (Evidence, bool) {
	e, ok := s.byID[id]
	return e, ok
}

// Has reports whether id is present.
func (s *EvidenceSet) Has(id EvidenceID) bool {
	_, ok := s.byID[id]
	return ok
}

// List returns records in insertion order.
func (s *EvidenceSet) List() []Evidence {
	out := make([]Evidence, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.byID[id])
	}
	return out
}

// Len returns the number of records.
func (s *EvidenceSet) Len() int { return len(s.order) }
