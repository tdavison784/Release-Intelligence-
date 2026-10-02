package domain

import "time"

// ProductID identifies a logical software product (e.g. "cert-manager").
type ProductID string

// ProductRef is a lightweight reference to a product.
type ProductRef struct {
	ID   ProductID `json:"id"`
	Name string    `json:"name"`
}

// ArtifactType enumerates kinds of deliverables a product publishes. A single
// logical release usually maps to many artifacts of different types, and their
// versions do not necessarily equal the release version.
type ArtifactType string

const (
	ArtifactSourceRelease  ArtifactType = "source-release" // a VCS release/tag (GitHub/GitLab release)
	ArtifactHelmChart      ArtifactType = "helm-chart"
	ArtifactContainerImage ArtifactType = "container-image"
	ArtifactOperator       ArtifactType = "operator" // e.g. an OLM bundle
	ArtifactBinary         ArtifactType = "binary"
	ArtifactPackage        ArtifactType = "package"  // OS / language package
	ArtifactManifest       ArtifactType = "manifest" // static install manifests
	ArtifactCRD            ArtifactType = "crd"      // CustomResourceDefinition bundle
	ArtifactDocumentation  ArtifactType = "documentation"
)

// SourceRole names what kind of information a source contributes.
type SourceRole string

const (
	RoleVersions      SourceRole = "versions"      // canonical list of releases/tags
	RoleReleaseNotes  SourceRole = "release-notes" // per-release notes
	RoleChangelog     SourceRole = "changelog"     // cumulative changelog file
	RoleUpgradeGuide  SourceRole = "upgrade-guide" // migration / upgrade documentation
	RoleCompatibility SourceRole = "compatibility" // platform compatibility matrix
	RoleSecurity      SourceRole = "security"      // advisories / bulletins
)

// ArtifactStatus reports how strongly an artifact instance has been confirmed.
type ArtifactStatus string

const (
	// ArtifactVerified: observed directly at its publication channel
	// (registry manifest exists, chart present in index, asset downloadable).
	ArtifactVerified ArtifactStatus = "verified"
	// ArtifactReferenced: not observed directly, but referenced by another
	// verified artifact of the same release (e.g. image tag in install manifest).
	ArtifactReferenced ArtifactStatus = "referenced"
	// ArtifactExpected: derived from the product definition's version
	// relationship but neither observed nor referenced (channel unreachable).
	ArtifactExpected ArtifactStatus = "expected"
	// ArtifactMissing: the channel was reachable and the artifact was absent.
	ArtifactMissing ArtifactStatus = "missing"
	// ArtifactNotApplicable: the artifact does not exist for this release
	// according to the definition's availability constraint.
	ArtifactNotApplicable ArtifactStatus = "not-applicable"
)

// ArtifactInstance is one concrete artifact belonging to a release.
type ArtifactInstance struct {
	ArtifactID string         `json:"artifactId"` // id in the product definition
	Type       ArtifactType   `json:"type"`
	Name       string         `json:"name"`
	Coordinate string         `json:"coordinate"`        // e.g. "quay.io/jetstack/cert-manager-controller:v1.18.0"
	Version    string         `json:"version,omitempty"` // the artifact's own version (a chart version may differ from the app version)
	Digest     string         `json:"digest,omitempty"`
	Channel    string         `json:"channel,omitempty"` // channel that confirmed it
	Status     ArtifactStatus `json:"status"`
	Detail     string         `json:"detail,omitempty"`
	Evidence   []EvidenceID   `json:"evidence,omitempty"`
}

// SourceState is the outcome of consulting a source.
type SourceState string

const (
	SourceOK          SourceState = "ok"
	SourcePartial     SourceState = "partial"
	SourceNotFound    SourceState = "not-found"   // reachable, but nothing for this version
	SourceUnavailable SourceState = "unavailable" // unreachable / blocked / auth required
	SourceThrottled   SourceState = "throttled"   // reachable but rate limited (429); retrying later is expected to work
	SourceSkipped     SourceState = "skipped"     // not applicable or disabled
	SourceError       SourceState = "error"
)

// SourceStatus records what happened when a source was consulted. It is what
// lets the output say "✓ GitHub releases" or "✗ OCI images (unreachable)".
type SourceStatus struct {
	SourceID string       `json:"sourceId"`
	Kind     string       `json:"kind"` // locator kind, e.g. "github-releases", "repo-file", "oci"
	Roles    []SourceRole `json:"roles,omitempty"`
	Version  string       `json:"version,omitempty"` // release the status refers to ("" = product level)
	State    SourceState  `json:"state"`
	Detail   string       `json:"detail,omitempty"`
	URI      string       `json:"uri,omitempty"`
}

// Reference is an external identifier mentioned by a source (CVE, GHSA, PR, issue).
type Reference struct {
	Type string `json:"type"` // "cve", "ghsa", "pull-request", "issue", "url"
	ID   string `json:"id"`
	URL  string `json:"url,omitempty"`
}

// NoteItem is one atomic entry of a release note / changelog / upgrade guide
// as published, together with its deterministic classification.
type NoteItem struct {
	ID             string       `json:"id"`
	Release        string       `json:"release"` // semver of the release that introduced it
	SourceID       string       `json:"sourceId"`
	Role           SourceRole   `json:"role"`
	Section        string       `json:"section,omitempty"` // heading path, e.g. "Breaking Changes" or "Traffic Management"
	Text           string       `json:"text"`
	Category       Category     `json:"category"`
	Breaking       bool         `json:"breaking,omitempty"`
	ActionRequired bool         `json:"actionRequired,omitempty"`
	References     []Reference  `json:"references,omitempty"`
	Classification Provenance   `json:"classification"`
	Evidence       []EvidenceID `json:"evidence"`
}

// CompatibilityConstraint states a platform requirement for a release, e.g.
// Kubernetes "1.29 - 1.33".
type CompatibilityConstraint struct {
	Platform   string       `json:"platform"`           // "kubernetes", "openshift", "helm", ...
	Constraint string       `json:"constraint"`         // normalised semver constraint when derivable, e.g. ">=1.29.0-0 <=1.33.x"
	Versions   []string     `json:"versions,omitempty"` // explicit list when the source enumerates versions
	Raw        string       `json:"raw"`                // verbatim value from the source
	Kind       string       `json:"kind,omitempty"`     // "supported", "tested", "minimum", "chart-kubeVersion"
	SourceID   string       `json:"sourceId"`
	Provenance Provenance   `json:"provenance"`
	Evidence   []EvidenceID `json:"evidence"`
}

// SnapshotKind names a structured snapshot taken for diffing.
type SnapshotKind string

const (
	SnapshotHelmValues SnapshotKind = "helm-values" // flattened default values of a chart
	SnapshotCRDs       SnapshotKind = "crds"        // summary of CRDs shipped by a release
	SnapshotImages     SnapshotKind = "image-refs"  // images referenced by a manifest/chart
)

// Snapshot is a deterministic, structured view of an artifact used to compute
// differences between two releases. Exactly one of the typed payloads is set.
type Snapshot struct {
	ArtifactID string             `json:"artifactId"`
	Kind       SnapshotKind       `json:"kind"`
	Values     *ValuesSnapshot    `json:"values,omitempty"`
	CRDs       *CRDSnapshot       `json:"crds,omitempty"`
	Images     *ImageRefsSnapshot `json:"images,omitempty"`
	Evidence   []EvidenceID       `json:"evidence"`
}

// ValuesSnapshot is a flattened Helm values document: dotted path → JSON
// encoded default value. Lists are treated as leaf values.
type ValuesSnapshot struct {
	Chart   string            `json:"chart"`
	Version string            `json:"version"`
	Entries map[string]string `json:"entries"`
	// Comments holds the doc comment directly above a key when present.
	Comments map[string]string `json:"comments,omitempty"`
}

// CRDSnapshot summarises CustomResourceDefinitions.
type CRDSnapshot struct {
	CRDs []CRDSummary `json:"crds"`
}

// CRDSummary describes one CRD.
type CRDSummary struct {
	Name     string           `json:"name"` // e.g. "certificates.cert-manager.io"
	Group    string           `json:"group"`
	Kind     string           `json:"kind"`
	Scope    string           `json:"scope,omitempty"`
	Versions []CRDVersionInfo `json:"versions"`
}

// CRDVersionInfo describes one API version of a CRD.
type CRDVersionInfo struct {
	Name               string `json:"name"`
	Served             bool   `json:"served"`
	Storage            bool   `json:"storage"`
	Deprecated         bool   `json:"deprecated,omitempty"`
	DeprecationWarning string `json:"deprecationWarning,omitempty"`
	// SchemaPaths lists dotted property paths of the openAPIV3Schema
	// (e.g. "spec.secretTemplate.labels"), used to detect removed/added fields.
	SchemaPaths []string `json:"schemaPaths,omitempty"`
	// Fields carries per-path schema facts (type, default, enum, required),
	// sorted by path, one entry per SchemaPaths element, so validators can
	// prove default/enum/required changes from the published CRD.
	Fields []CRDFieldSchema `json:"fields,omitempty"`
}

// CRDFieldSchema is the machine-comparable schema of one CRD property path.
type CRDFieldSchema struct {
	Path string `json:"path"`
	// Type is the declared OpenAPI type ("string", "object", "array", ...).
	Type string `json:"type,omitempty"`
	// Default is the schema default as canonical JSON ("" = no default).
	Default string `json:"default,omitempty"`
	// Enum lists the allowed values as canonical JSON, in schema order.
	Enum []string `json:"enum,omitempty"`
	// Required reports that the parent schema lists this property in required.
	Required bool `json:"required,omitempty"`
}

// ImageRefsSnapshot lists container images referenced by an artifact.
type ImageRefsSnapshot struct {
	Images []ImageRef `json:"images"`
}

// ImageRef is a parsed image reference.
type ImageRef struct {
	Repository string `json:"repository"` // "quay.io/jetstack/cert-manager-controller"
	Tag        string `json:"tag,omitempty"`
	Digest     string `json:"digest,omitempty"`
}

// Release is everything deterministically known about one product release.
type Release struct {
	Product     ProductID                 `json:"product"`
	Version     Version                   `json:"version"`
	PublishedAt *time.Time                `json:"publishedAt,omitempty"`
	Artifacts   []ArtifactInstance        `json:"artifacts,omitempty"`
	Notes       []NoteItem                `json:"notes,omitempty"`
	Compat      []CompatibilityConstraint `json:"compatibility,omitempty"`
	Snapshots   []Snapshot                `json:"snapshots,omitempty"`
	Facts       []Fact                    `json:"facts,omitempty"`
	Sources     []SourceStatus            `json:"sources,omitempty"`
	Evidence    []Evidence                `json:"evidence,omitempty"`
	IngestedAt  time.Time                 `json:"ingestedAt"`
	// DefinitionDigest identifies the product definition revision used.
	DefinitionDigest string `json:"definitionDigest,omitempty"`
}

// Snapshot returns the first snapshot of the given kind for artifactID
// (artifactID "" matches any).
func (r *Release) Snapshot(kind SnapshotKind, artifactID string) *Snapshot {
	for i := range r.Snapshots {
		s := &r.Snapshots[i]
		if s.Kind == kind && (artifactID == "" || s.ArtifactID == artifactID) {
			return s
		}
	}
	return nil
}

// Advisory is a security advisory affecting a product.
type Advisory struct {
	ID          string     `json:"id"`                // GHSA-xxxx or vendor id (e.g. ISTIO-SECURITY-2025-001)
	Aliases     []string   `json:"aliases,omitempty"` // CVE ids
	Summary     string     `json:"summary"`
	Severity    string     `json:"severity,omitempty"`
	URL         string     `json:"url,omitempty"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	// Vulnerable is a semver constraint of affected versions (">= 1.0.0, < 1.17.2").
	Vulnerable string `json:"vulnerable,omitempty"`
	// Patched lists the first patched version per line.
	Patched  []string     `json:"patched,omitempty"`
	SourceID string       `json:"sourceId"`
	Evidence []EvidenceID `json:"evidence"`
}
