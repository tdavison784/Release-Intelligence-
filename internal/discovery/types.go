package discovery

import (
	"sort"
	"strconv"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// CandidateKind classifies a discovery candidate.
type CandidateKind string

// Candidate kinds. Values are generic: product-specific knowledge lives only
// in candidate values and attributes, never in detector code.
const (
	KindTagScheme        CandidateKind = "tag-scheme"          // canonical tag naming (value: tag regex)
	KindReleaseTrigger   CandidateKind = "release-trigger"     // CI pipeline triggered by tag pushes (value: tag glob)
	KindReleasePublisher CandidateKind = "release-publisher"   // tooling that creates hosted releases (value: tool)
	KindReleaseNotes     CandidateKind = "release-notes"       // per-release notes (value: path template or "github-release-body")
	KindNotesDir         CandidateKind = "release-notes-dir"   // structured per-change note files (value: directory)
	KindChangelog        CandidateKind = "changelog"           // cumulative changelog file (value: path)
	KindUpgradeGuide     CandidateKind = "upgrade-guide"       // upgrade / migration docs (value: path template)
	KindCompatibility    CandidateKind = "compatibility"       // support matrix (value: path)
	KindSecurityPolicy   CandidateKind = "security-policy"     // SECURITY.md and friends (value: path)
	KindAdvisories       CandidateKind = "security-advisories" // advisory feed (value: owner/name or URL)
	KindSecurityDocs     CandidateKind = "security-bulletins"  // bulletin pages (value: directory)
	KindDocsRepo         CandidateKind = "docs-repo"           // external documentation repository (value: host/owner/name)
	KindChartRepo        CandidateKind = "chart-repo"          // external repository holding the chart (value: host/owner/name/path)
	KindHelmChart        CandidateKind = "helm-chart"          // chart in the scanned tree (value: chart name)
	KindHelmRepo         CandidateKind = "helm-repo"           // HTTP chart repository (value: URL)
	KindHelmOCI          CandidateKind = "helm-oci"            // OCI chart location (value: registry/path)
	KindRegistry         CandidateKind = "registry"            // registry host or host/namespace
	KindImage            CandidateKind = "image"               // fully qualified image repository
	KindImageName        CandidateKind = "image-name"          // image built by the repo, registry not stated (value: name)
	KindReleaseAsset     CandidateKind = "release-asset"       // file attached to hosted releases (value: templated URL)
	KindManifest         CandidateKind = "manifest"            // static install manifest (value: path)
	KindCRD              CandidateKind = "crd"                 // CustomResourceDefinition file or directory (value: path)
	KindVersionRelation  CandidateKind = "version-relation"    // how an artifact version relates to the tag (value: statement)
	KindBuildTool        CandidateKind = "build-tool"          // goreleaser, ko, chart-releaser, cosign … (value: tool)
)

// Candidate is one deterministic finding of the scanner.
type Candidate struct {
	ID         string            `json:"id"`
	Kind       CandidateKind     `json:"kind"`
	Value      string            `json:"value"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Confidence domain.Confidence `json:"confidence"`
	// Rules lists the detector rule ids that produced the candidate.
	Rules    []string          `json:"rules"`
	Evidence []domain.Evidence `json:"evidence"`
}

// Attr returns an attribute ("" when unset).
func (c Candidate) Attr(k string) string { return c.Attributes[k] }

// EvidenceIDs returns the ids of the candidate's evidence.
func (c Candidate) EvidenceIDs() []domain.EvidenceID {
	out := make([]domain.EvidenceID, 0, len(c.Evidence))
	for _, e := range c.Evidence {
		out = append(out, e.ID)
	}
	return out
}

// candidateID derives a stable id from kind and value.
func candidateID(kind CandidateKind, value string) string {
	return "cand-" + domain.ShortHash(string(kind), value)
}

// maxEvidencePerCandidate bounds the evidence kept per candidate; the total
// number of observations is kept in the "occurrences" attribute.
const maxEvidencePerCandidate = 6

// listAttrs are merged as sorted, comma-separated sets.
var listAttrs = map[string]bool{"variable": true, "classes": true, "paths": true, "contexts": true, "files": true, "sources": true, "charts": true, "tags": true, "names": true, "images": true, "triggers": true}

func confRank(c domain.Confidence) int {
	switch c {
	case domain.ConfidenceHigh:
		return 3
	case domain.ConfidenceMedium:
		return 2
	case domain.ConfidenceLow:
		return 1
	}
	return 0
}

func maxConf(a, b domain.Confidence) domain.Confidence {
	if confRank(b) > confRank(a) {
		return b
	}
	return a
}

// merge folds o into c (same kind and value).
func (c *Candidate) merge(o Candidate) {
	c.Confidence = maxConf(c.Confidence, o.Confidence)
	for _, r := range o.Rules {
		if !containsStr(c.Rules, r) {
			c.Rules = append(c.Rules, r)
		}
	}
	sort.Strings(c.Rules)
	if c.Attributes == nil {
		c.Attributes = map[string]string{}
	}
	for k, v := range o.Attributes {
		if v == "" {
			continue
		}
		cur := c.Attributes[k]
		switch {
		case cur == "":
			c.Attributes[k] = v
		case listAttrs[k]:
			c.Attributes[k] = joinSet(cur, v)
		}
	}
	occ := atoiDefault(c.Attributes["occurrences"], 1) + atoiDefault(o.Attributes["occurrences"], 1)
	c.Attributes["occurrences"] = itoa(occ)
	for _, e := range o.Evidence {
		dup := false
		for _, x := range c.Evidence {
			if x.ID == e.ID {
				dup = true
				break
			}
		}
		if !dup && len(c.Evidence) < maxEvidencePerCandidate {
			c.Evidence = append(c.Evidence, e)
		}
	}
}

func joinSet(a, b string) string {
	set := map[string]bool{}
	for _, s := range strings.Split(a+","+b, ",") {
		if s = strings.TrimSpace(s); s != "" {
			set[s] = true
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	if len(out) > 25 {
		out = append(out[:25], "…")
	}
	return strings.Join(out, ",")
}

// Decision records one resolver choice.
type Decision struct {
	// Element is "source:<id>", "artifact:<id>", "versioning" or "candidate:<id>".
	Element string `json:"element"`
	// Action: "include", "exclude", "annotate", "drop", "replace".
	Action     string            `json:"action"`
	Rule       string            `json:"rule"`
	Rationale  string            `json:"rationale"`
	Candidates []string          `json:"candidates,omitempty"`
	Provenance domain.Provenance `json:"provenance"`
}

// Proposal is an LLM-proposed definition element. It never enters the final
// definition unless Status is ProposalValidated.
type Proposal struct {
	ID       string `json:"id"`
	Question string `json:"question"` // question kind, e.g. "registry-roles"
	// Element is the draft element the proposal adds or replaces
	// ("source:<id>" or "artifact:<id>"); empty for informational answers.
	Element  string `json:"element,omitempty"`
	Replaces string `json:"replaces,omitempty"`
	// Summary describes the proposal for humans; Answer is the raw JSON.
	Summary    string            `json:"summary"`
	Rationale  string            `json:"rationale,omitempty"`
	Answer     string            `json:"answer,omitempty"`
	Provenance domain.Provenance `json:"provenance"`
	Status     string            `json:"status"`
	Detail     string            `json:"detail,omitempty"`
}

// Proposal statuses.
const (
	ProposalPending       = "pending"
	ProposalValidated     = "validated"
	ProposalFailed        = "failed"
	ProposalUnverified    = "unverified"    // validation impossible or insufficient
	ProposalInformational = "informational" // advice only, never changes the definition
	ProposalRejected      = "rejected"      // malformed or not applicable
)

// Discovery targets used for the coverage section of the report.
const (
	TargetReleaseSource    = "release-source"
	TargetReleaseNotes     = "release-notes"
	TargetChangelog        = "changelog"
	TargetHelmCharts       = "helm-charts"
	TargetRegistries       = "registries"
	TargetImages           = "images"
	TargetUpgradeDocs      = "upgrade-docs"
	TargetCompatibility    = "compatibility"
	TargetSecurity         = "security"
	TargetVersionRelations = "version-relations"
)

// AllTargets lists the ten discovery targets in report order.
var AllTargets = []string{TargetReleaseSource, TargetReleaseNotes, TargetChangelog, TargetHelmCharts, TargetRegistries,
	TargetImages, TargetUpgradeDocs, TargetCompatibility, TargetSecurity, TargetVersionRelations}

// Coverage summarises what was found for one discovery target.
type Coverage struct {
	Target     string   `json:"target"`
	Candidates int      `json:"candidates"`
	InDraft    []string `json:"inDefinition,omitempty"` // definition elements serving the target
	Findings   []string `json:"findings,omitempty"`     // short human summary
	Status     string   `json:"status"`                 // "found", "candidates-only", "not-found"
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func itoa(n int) string { return strconv.Itoa(n) }
