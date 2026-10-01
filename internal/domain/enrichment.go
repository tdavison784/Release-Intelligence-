package domain

import (
	"errors"
	"fmt"
	"strings"
)

// EnrichmentKind classifies what an Enrichment adds on top of the
// deterministic Changes. Every kind groups, summarises, connects or explains
// existing Changes; none of them states a new fact.
type EnrichmentKind string

const (
	// EnrichmentCluster: semantically equivalent Changes from different
	// sources (release notes, upgrade guide, a computed diff) consolidated
	// into one conclusion. Relates to at least two Changes.
	EnrichmentCluster EnrichmentKind = "cluster"
	// EnrichmentMigrationSummary: one migration requirement summarised from
	// one or more Changes.
	EnrichmentMigrationSummary EnrichmentKind = "migration-summary"
	// EnrichmentDiffExplanation: why a computed diff matters, citing the
	// release-note statement that explains it. Relates to at least one
	// computed Change.
	EnrichmentDiffExplanation EnrichmentKind = "diff-explanation"
	// EnrichmentRelated: Changes that are potentially related. The relation
	// is a hypothesis that requires verification; the enrichment is always
	// marked Unverified.
	EnrichmentRelated EnrichmentKind = "related"
)

// EnrichmentKinds lists every kind in display order.
var EnrichmentKinds = []EnrichmentKind{EnrichmentCluster, EnrichmentMigrationSummary, EnrichmentDiffExplanation, EnrichmentRelated}

// Valid reports whether k is a known kind.
func (k EnrichmentKind) Valid() bool {
	for _, x := range EnrichmentKinds {
		if k == x {
			return true
		}
	}
	return false
}

// MinChanges is the number of Changes an enrichment of this kind must relate to.
func (k EnrichmentKind) MinChanges() int {
	switch k {
	case EnrichmentCluster, EnrichmentRelated:
		return 2
	}
	return 1
}

// EnrichmentIDPrefix starts every Enrichment id; Change ids never use it, so
// an enrichment can never be mistaken for (or smuggled in as) a Change.
const EnrichmentIDPrefix = "enr-"

// Enrichment is AI-derived information. It is kept apart from Changes: it
// never replaces or modifies a Change or a source fact, it only refers to
// Changes (RelatesTo) and cites the evidence it relies on (Citations). Its
// provenance records the model, the model version, the prompt version and
// digest, every evidence id the model was given, and when it was generated.
type Enrichment struct {
	ID      string         `json:"id"`
	Kind    EnrichmentKind `json:"kind"`
	Title   string         `json:"title,omitempty"`
	Content string         `json:"content"`
	// RelatesTo are the ids of the Changes the enrichment is about (≥1; ≥2
	// for clusters and related changes). Each resolves within the edge.
	RelatesTo []string `json:"relatesTo"`
	// Citations are the evidence ids the enrichment relies on (≥1). They are
	// a subset of Provenance.InputEvidence, which is a subset of the edge's
	// evidence.
	Citations []EvidenceID `json:"citations"`
	// Unverified marks a hypothesis (kind "related"): the relation between
	// the Changes was suggested by the model, not established by the
	// citations, and must be verified by a human.
	Unverified bool       `json:"unverified,omitempty"`
	Provenance Provenance `json:"provenance"`
}

// EnrichmentRun records how the enrichments of an edge were produced. It is
// metadata about AI output (for evaluation and auditing), never a conclusion.
type EnrichmentRun struct {
	Producer      string `json:"producer"`      // e.g. "enrich@v1"
	PromptVersion string `json:"promptVersion"` // version of the prompt templates
	// CandidateGroups is the number of deterministic candidate groups (sets of
	// changes that might belong together) found before any model was asked.
	CandidateGroups int `json:"candidateGroups"`
	// Requests is the number of prompts built (one per candidate group asked).
	Requests int `json:"requests"`
	// Pending counts requests still waiting for an answer (file exchange).
	Pending int `json:"pending,omitempty"`
	// Failed counts requests that produced no usable answer (transport errors,
	// no model configured, undecodable or schema-invalid answers).
	Failed int `json:"failed,omitempty"`
	// Accepted is the number of enrichments that passed validation (== len(enrichments)).
	Accepted int `json:"accepted"`
	// Rejected lists proposed enrichments (or whole answers) the validator refused.
	Rejected []EnrichmentRejection `json:"rejected,omitempty"`
	// Clusters is the number of accepted cluster enrichments, ClusteredChanges
	// the number of distinct Changes they consolidate, and
	// DuplicatesConsolidated = ClusteredChanges - Clusters: how many Changes
	// were duplicates of another Change of the same cluster.
	Clusters               int `json:"clusters"`
	ClusteredChanges       int `json:"clusteredChanges"`
	DuplicatesConsolidated int `json:"duplicatesConsolidated"`
}

// EnrichmentRejection is one model proposal that did not become an Enrichment.
type EnrichmentRejection struct {
	Group        string         `json:"group"` // candidate group id
	PromptDigest string         `json:"promptDigest,omitempty"`
	Kind         EnrichmentKind `json:"kind,omitempty"`
	RelatesTo    []string       `json:"relatesTo,omitempty"`
	Reason       string         `json:"reason"`
}

// ClusterMetrics counts cluster enrichments and the distinct changes they
// consolidate.
func ClusterMetrics(ens []Enrichment) (clusters, clusteredChanges int) {
	seen := map[string]bool{}
	for _, en := range ens {
		if en.Kind != EnrichmentCluster {
			continue
		}
		clusters++
		for _, id := range en.RelatesTo {
			if !seen[id] {
				seen[id] = true
				clusteredChanges++
			}
		}
	}
	return clusters, clusteredChanges
}

// validateEnrichments checks the enrichments of an edge against its changes
// and evidence:
//   - ids carry EnrichmentIDPrefix, are unique and never equal a Change id;
//   - AI provenance is complete (see Provenance.Validate);
//   - every related Change id exists (≥1, ≥2 for clusters and related);
//   - citations ⊆ provenance.inputEvidence ⊆ edge evidence;
//   - "related" enrichments, and only they, are marked unverified;
//   - a diff explanation relates to at least one computed Change;
//   - a Change belongs to at most one cluster;
//   - the run metadata, when present, matches the enrichments.
func (e *UpgradeEdge) validateEnrichments(ev map[EvidenceID]bool) []error {
	var errs []error
	changes := map[string]Change{}
	for _, c := range e.Changes {
		changes[c.ID] = c
	}
	for _, c := range e.Changes {
		if strings.HasPrefix(c.ID, EnrichmentIDPrefix) {
			errs = append(errs, fmt.Errorf("change %s uses the enrichment id prefix %q: enrichments never appear as Changes", c.ID, EnrichmentIDPrefix))
		}
	}
	ids := map[string]bool{}
	inCluster := map[string]string{}
	for _, en := range e.Enrichments {
		bad := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("enrichment %s: %s", en.ID, fmt.Sprintf(format, args...)))
		}
		switch {
		case !strings.HasPrefix(en.ID, EnrichmentIDPrefix):
			bad("id must start with %q", EnrichmentIDPrefix)
		case ids[en.ID]:
			bad("duplicate id")
		}
		ids[en.ID] = true
		if _, clash := changes[en.ID]; clash {
			bad("id collides with a Change id")
		}
		if en.Provenance.Method != MethodAI {
			bad("must have ai provenance")
		}
		if err := en.Provenance.Validate(); err != nil {
			bad("%v", err)
		}
		if !en.Kind.Valid() {
			bad("unknown kind %q", en.Kind)
		}
		if strings.TrimSpace(en.Content) == "" {
			bad("empty content")
		}
		if n := en.Kind.MinChanges(); len(en.RelatesTo) < n {
			bad("kind %s must relate to at least %d change(s), has %d", en.Kind, n, len(en.RelatesTo))
		}
		seen := map[string]bool{}
		computed := false
		for _, id := range en.RelatesTo {
			if seen[id] {
				bad("relates to change %s twice", id)
			}
			seen[id] = true
			c, ok := changes[id]
			if !ok {
				bad("references unknown change %s", id)
				continue
			}
			computed = computed || c.Provenance.Method == MethodComputed
			if en.Kind == EnrichmentCluster {
				if other, dup := inCluster[id]; dup && other != en.ID {
					bad("change %s is already consolidated by cluster %s", id, other)
				}
				inCluster[id] = en.ID
			}
		}
		if en.Kind == EnrichmentDiffExplanation && !computed {
			bad("a diff explanation must relate to at least one computed change")
		}
		if (en.Kind == EnrichmentRelated) != en.Unverified {
			bad("unverified must be set exactly for kind %q", EnrichmentRelated)
		}
		input := map[EvidenceID]bool{}
		for _, id := range en.Provenance.InputEvidence {
			input[id] = true
			if !ev[id] {
				bad("input evidence %s is not in the edge", id)
			}
		}
		if len(en.Citations) == 0 {
			bad("cites no evidence")
		}
		cited := map[EvidenceID]bool{}
		for _, id := range en.Citations {
			if cited[id] {
				bad("cites %s twice", id)
			}
			cited[id] = true
			if !input[id] {
				bad("cites %s, which was not part of its input evidence", id)
			}
		}
	}
	if r := e.EnrichmentRun; r != nil {
		clusters, clustered := ClusterMetrics(e.Enrichments)
		if r.Accepted != len(e.Enrichments) || r.Clusters != clusters || r.ClusteredChanges != clustered ||
			r.DuplicatesConsolidated != clustered-clusters {
			errs = append(errs, fmt.Errorf("enrichment run metadata does not match the enrichments (accepted %d/%d, clusters %d/%d, clustered changes %d/%d, duplicates %d/%d)",
				r.Accepted, len(e.Enrichments), r.Clusters, clusters, r.ClusteredChanges, clustered, r.DuplicatesConsolidated, clustered-clusters))
		}
		if r.Producer == "" || r.PromptVersion == "" {
			errs = append(errs, errors.New("enrichment run requires producer and promptVersion"))
		}
	}
	return errs
}
