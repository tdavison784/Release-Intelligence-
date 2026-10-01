package domain

import (
	"errors"
	"fmt"
	"time"
)

// ImpactReportSchemaVersion is the current serialisation version.
const ImpactReportSchemaVersion = "ri.dev/impact-report/v1alpha1"

// ImpactClass is the action classification of a finding.
type ImpactClass string

const (
	// ImpactActionRequired: the environment must change (values, manifests,
	// cluster version, mirrors) or the upgrade fails / silently misbehaves.
	ImpactActionRequired ImpactClass = "action-required"
	// ImpactReview: the change plausibly affects the environment but whether
	// action is needed depends on intent the files cannot show.
	ImpactReview ImpactClass = "review"
	// ImpactInformational: confirmed overlap with no action implied (a pinned
	// value that keeps winning, a cluster inside the supported range, ...).
	ImpactInformational ImpactClass = "informational"
)

// AllImpactClasses lists every classification in display order.
var AllImpactClasses = []ImpactClass{ImpactActionRequired, ImpactReview, ImpactInformational}

// ImpactMatchKind names what part of the environment matched.
type ImpactMatchKind string

const (
	MatchValuesKey     ImpactMatchKind = "values-key"     // a key the user sets in a values file
	MatchAPIVersion    ImpactMatchKind = "api-version"    // an apiVersion (+kind) used by a manifest
	MatchCRD           ImpactMatchKind = "crd"            // an installed CustomResourceDefinition
	MatchCRDVersion    ImpactMatchKind = "crd-version"    // a version declared by an installed CRD
	MatchManifestField ImpactMatchKind = "manifest-field" // a field path set in a manifest
	MatchImage         ImpactMatchKind = "image"          // a container image reference in use
	MatchKubernetes    ImpactMatchKind = "kubernetes"     // the cluster Kubernetes version
)

// ImpactMatch is one environment fact that made a finding fire: what matched
// (subject) and the environment evidence that proves the environment has it.
type ImpactMatch struct {
	Kind     ImpactMatchKind `json:"kind"`
	Subject  string          `json:"subject"`
	Evidence []EvidenceID    `json:"evidence"`
}

// ImpactFinding is one deterministic conclusion of the join: an upstream
// change (or compatibility constraint) met an environment fact. It always
// cites BOTH chains: UpstreamEvidence (ids resolving in the report's
// `evidence`, copied from the UpgradeEdge) and EnvironmentEvidence (ids
// resolving in `environmentEvidence`, pointing at local files or direct
// inputs). Detail explains in prose how the two chains meet.
type ImpactFinding struct {
	ID             string      `json:"id"`
	Classification ImpactClass `json:"classification"`
	Rule           string      `json:"rule"` // join rule, e.g. "impact:values-removed"
	Title          string      `json:"title"`
	Detail         string      `json:"detail,omitempty"`

	// The upstream change this finding joins (empty when the finding comes
	// from a compatibility constraint alone, e.g. the cluster-version check).
	// The id resolves within the UpgradeEdge the report was built from; the
	// title and category are copied so the report renders standalone.
	ChangeID             string   `json:"changeId,omitempty"`
	ChangeTitle          string   `json:"changeTitle,omitempty"`
	ChangeCategory       Category `json:"changeCategory,omitempty"`
	ChangeBreaking       bool     `json:"changeBreaking,omitempty"`
	ChangeActionRequired bool     `json:"changeActionRequired,omitempty"`

	Matches             []ImpactMatch `json:"matches"`             // environment facts that matched
	UpstreamEvidence    []EvidenceID  `json:"upstreamEvidence"`    // chain 1: edge evidence
	EnvironmentEvidence []EvidenceID  `json:"environmentEvidence"` // chain 2: local evidence
	Provenance          Provenance    `json:"provenance"`
}

// ImpactSummary counts the funnel: all upstream changes → those that affect
// this environment → those needing action.
type ImpactSummary struct {
	UpstreamChanges   int `json:"upstreamChanges"`
	AffectEnvironment int `json:"affectEnvironment"`
	ActionRequired    int `json:"actionRequired"`
	Review            int `json:"review"`
	Informational     int `json:"informational"`
}

// ImpactFile is one environment input file with the digest of the bytes that
// were parsed, so a finding's excerpt can be audited against the exact input.
type ImpactFile struct {
	Path   string `json:"path"`   // path exactly as supplied on the command line
	Digest string `json:"digest"` // sha256 of the file bytes
}

// ImpactEnvironment summarises what the join was run against.
type ImpactEnvironment struct {
	// Kubernetes is the cluster version as supplied ("" when not given).
	Kubernetes string `json:"kubernetes,omitempty"`
	// Files are all parsed input files with their digests.
	Files []ImpactFile `json:"files,omitempty"`
	// Counts of extracted environment facts.
	ValuesKeys    int `json:"valuesKeys"`
	ManifestDocs  int `json:"manifestDocs"`
	APIVersions   int `json:"apiVersions"`
	CRDs          int `json:"crds"`
	ManifestPaths int `json:"manifestPaths"`
	Images        int `json:"images"`
	// Warnings report degraded parsing (unparsable documents, caps hit, ...).
	Warnings []string `json:"warnings,omitempty"`
}

// ImpactReport is the result of joining an UpgradeEdge with an environment:
// which of the edge's changes matter to THAT environment, each explained by
// two provenance chains (upstream evidence + environment evidence). Built
// deterministically; never contains AI output.
type ImpactReport struct {
	SchemaVersion string            `json:"schemaVersion"`
	Product       ProductRef        `json:"product"`
	From          Version           `json:"from"`
	To            Version           `json:"to"`
	Environment   ImpactEnvironment `json:"environment"`
	Summary       ImpactSummary     `json:"summary"`
	Findings      []ImpactFinding   `json:"findings"`

	// Evidence is the upstream evidence cited by findings (a subset of the
	// edge's evidence pool). EnvironmentEvidence is the local chain: records
	// of kind local-file / input pointing at the user's files or flags.
	Evidence            []Evidence `json:"evidence"`
	EnvironmentEvidence []Evidence `json:"environmentEvidence"`

	Warnings         []string  `json:"warnings,omitempty"`
	GeneratedAt      time.Time `json:"generatedAt"`
	DefinitionDigest string    `json:"definitionDigest,omitempty"`
}

// Validate enforces the invariants of an impact report: the two evidence
// chains of every finding resolve within the document, findings are
// deterministic and classified, and the summary counts match the findings.
func (r *ImpactReport) Validate() error {
	var errs []error
	if r.SchemaVersion != ImpactReportSchemaVersion {
		errs = append(errs, fmt.Errorf("schemaVersion = %q, want %q", r.SchemaVersion, ImpactReportSchemaVersion))
	}
	up := map[EvidenceID]bool{}
	for _, x := range r.Evidence {
		if up[x.ID] {
			errs = append(errs, fmt.Errorf("duplicate evidence id %s", x.ID))
		}
		up[x.ID] = true
	}
	local := map[EvidenceID]bool{}
	for _, x := range r.EnvironmentEvidence {
		if local[x.ID] {
			errs = append(errs, fmt.Errorf("duplicate environment evidence id %s", x.ID))
		}
		local[x.ID] = true
	}
	classes := map[ImpactClass]bool{}
	for _, c := range AllImpactClasses {
		classes[c] = true
	}
	counts := map[ImpactClass]int{}
	for _, f := range r.Findings {
		f := f
		if err := f.Provenance.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("finding %s: %w", f.ID, err))
		}
		if !f.Provenance.Deterministic() {
			errs = append(errs, fmt.Errorf("finding %s: AI-derived content must be an Enrichment, not a finding", f.ID))
		}
		if !classes[f.Classification] {
			errs = append(errs, fmt.Errorf("finding %s: unknown classification %q", f.ID, f.Classification))
		}
		counts[f.Classification]++
		if f.Title == "" {
			errs = append(errs, fmt.Errorf("finding %s has no title", f.ID))
		}
		if len(f.UpstreamEvidence) == 0 {
			errs = append(errs, fmt.Errorf("finding %s (%q) cites no upstream evidence", f.ID, f.Title))
		}
		for _, id := range f.UpstreamEvidence {
			if !up[id] {
				errs = append(errs, fmt.Errorf("finding %s references unknown upstream evidence %s", f.ID, id))
			}
		}
		if len(f.EnvironmentEvidence) == 0 {
			errs = append(errs, fmt.Errorf("finding %s (%q) cites no environment evidence", f.ID, f.Title))
		}
		for _, id := range f.EnvironmentEvidence {
			if !local[id] {
				errs = append(errs, fmt.Errorf("finding %s references unknown environment evidence %s", f.ID, id))
			}
		}
		for _, m := range f.Matches {
			if len(m.Evidence) == 0 {
				errs = append(errs, fmt.Errorf("finding %s match %q has no evidence", f.ID, m.Subject))
			}
			for _, id := range m.Evidence {
				if !local[id] {
					errs = append(errs, fmt.Errorf("finding %s match %q references unknown environment evidence %s", f.ID, m.Subject, id))
				}
			}
		}
		if (f.ChangeID == "") != (f.ChangeTitle == "") {
			errs = append(errs, fmt.Errorf("finding %s: changeId and changeTitle must be given together", f.ID))
		}
	}
	if r.Summary.AffectEnvironment != len(r.Findings) ||
		r.Summary.ActionRequired != counts[ImpactActionRequired] ||
		r.Summary.Review != counts[ImpactReview] ||
		r.Summary.Informational != counts[ImpactInformational] {
		errs = append(errs, fmt.Errorf("summary %+v does not match the %d findings (%+v)", r.Summary, len(r.Findings), counts))
	}
	if r.From.Compare(r.To) >= 0 {
		errs = append(errs, errors.New("report 'from' must be lower than 'to'"))
	}
	return errors.Join(errs...)
}
