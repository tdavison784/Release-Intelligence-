package domain

// FactID identifies a Fact.
type FactID string

// FactKind classifies deterministic facts.
type FactKind string

const (
	FactReleasePublished  FactKind = "release.published"  // the release exists in the canonical channel
	FactArtifactPublished FactKind = "artifact.published" // an artifact instance was observed or referenced
	FactDocumentRetrieved FactKind = "document.retrieved" // a release-notes / upgrade / compat document was retrieved
	FactCompatibility     FactKind = "compatibility"      // a platform constraint was stated
	FactSnapshot          FactKind = "snapshot"           // a structured snapshot was taken
	FactAdvisory          FactKind = "advisory"           // a security advisory applies
	FactRelationship      FactKind = "relationship"       // a validated release↔artifact relationship
)

// Fact is a deterministic statement extracted from sources, with evidence.
// Facts never contain interpretation; interpretation lives in Changes.
type Fact struct {
	ID         FactID            `json:"id"`
	Kind       FactKind          `json:"kind"`
	Subject    string            `json:"subject"`           // e.g. "cert-manager@1.18.0" or an artifact coordinate
	Release    string            `json:"release,omitempty"` // semver the fact is about
	Statement  string            `json:"statement"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Extractor  string            `json:"extractor"` // component@version that extracted it
	Evidence   []EvidenceID      `json:"evidence"`
}

// NewFact builds a fact with a deterministic ID.
func NewFact(kind FactKind, subject, release, statement, extractor string, attrs map[string]string, evidence ...EvidenceID) Fact {
	ids := make([]string, 0, len(evidence)+4)
	ids = append(ids, string(kind), subject, release, statement)
	for _, e := range evidence {
		ids = append(ids, string(e))
	}
	return Fact{
		ID:         FactID("fact-" + ShortHash(ids...)),
		Kind:       kind,
		Subject:    subject,
		Release:    release,
		Statement:  statement,
		Attributes: attrs,
		Extractor:  extractor,
		Evidence:   evidence,
	}
}
