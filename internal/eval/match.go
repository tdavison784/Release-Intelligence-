package eval

// The matching engine: recognising that a generated Change (or ImpactFinding)
// covers a ground-truth item. Every match records WHICH matcher hit and the
// evidence that backs the matched change, so a human can audit every hit and
// miss without re-running anything.

import (
	"regexp"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// EvidenceIndex resolves the evidence ids cited by a change.
type EvidenceIndex map[domain.EvidenceID]domain.Evidence

// NewEvidenceIndex indexes an edge's evidence pool.
func NewEvidenceIndex(e *domain.UpgradeEdge) EvidenceIndex {
	m := make(EvidenceIndex, len(e.Evidence))
	for _, ev := range e.Evidence {
		m[ev.ID] = ev
	}
	return m
}

// Matches reports whether the change satisfies the matcher (all set fields).
// ev indexes the edge's evidence pool; it may be nil when the matcher has no
// evidence clause.
func (m *Matcher) Matches(c domain.Change, ev EvidenceIndex) bool {
	m.prepare() // cases built in code (tests) may skip load-time compilation
	if m.textRe != nil && !m.textRe.MatchString(c.Title+"\n"+c.Detail) {
		return false
	}
	if m.Subject != "" && !containsSubject(c.Subjects, m.Subject) {
		return false
	}
	if m.Category != "" && string(c.Category) != m.Category {
		return false
	}
	if m.releaseRe != nil && !m.releaseRe.MatchString(c.Release) {
		return false
	}
	if m.evRe != nil && !matchesEvidenceURI(c.Evidence, ev, m.evRe) {
		return false
	}
	if m.Breaking != nil && c.Breaking != *m.Breaking {
		return false
	}
	if m.ActionRequired != nil && c.ActionRequired != *m.ActionRequired {
		return false
	}
	return true
}

func containsSubject(subjects []string, want string) bool {
	for _, s := range subjects {
		if s == want {
			return true
		}
	}
	return false
}

func matchesEvidenceURI(ids []domain.EvidenceID, ev EvidenceIndex, re *regexp.Regexp) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		e, ok := ev[id]
		if !ok {
			continue
		}
		if re.MatchString(e.URI) {
			return true
		}
	}
	return false
}

// firstEvidenceURI returns a human-openable pointer for the change's first
// resolving evidence record ("" when none resolves).
func firstEvidenceURI(ids []domain.EvidenceID, ev EvidenceIndex) string {
	for _, id := range ids {
		if e, ok := ev[id]; ok {
			return e.URI
		}
	}
	return ""
}

// Matches reports whether the finding satisfies all set fields of the
// finding matcher.
func (f FindingMatcher) Matches(finding domain.ImpactFinding) bool {
	if f.Subject != "" {
		found := false
		for _, m := range finding.Matches {
			if m.Subject == f.Subject {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if f.Rule != "" && finding.Rule != f.Rule {
		return false
	}
	if f.Classification != "" && string(finding.Classification) != f.Classification {
		return false
	}
	return true
}

// String renders the finding matcher for audit output.
func (f FindingMatcher) String() string {
	var parts []string
	if f.Subject != "" {
		parts = append(parts, "subject:"+f.Subject)
	}
	if f.Rule != "" {
		parts = append(parts, "rule:"+f.Rule)
	}
	if f.Classification != "" {
		parts = append(parts, "classification:"+f.Classification)
	}
	return strings.Join(parts, " & ")
}

// normalizeTitle is the key for exact-duplicate detection: lowercase,
// whitespace collapsed.
func normalizeTitle(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// subjectKey is the key for subject-duplicate detection: the sorted subject
// set of a change ("" when it has none). Two changes with the same category
// AND the same non-empty subject set say the same thing about the same thing.
func subjectKey(c domain.Change) string {
	if len(c.Subjects) == 0 {
		return ""
	}
	s := append([]string(nil), c.Subjects...)
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	key := string(c.Category) + "|" + strings.Join(s, ",")
	if id := crdIdentity(c); id != "" {
		key = "crd:" + id + "|" + key
	}
	return key
}

// crdIdentity is the API identity (group/kind) a CRD diff's subjects belong
// to, "" for other changes. Schema-path diffs (crd:fields-removed,
// crd:default-changed, …) carry bare paths as subjects, so the same path on
// two CRDs ("status.conditions[]" added to several CRDs) is not the same
// subject. The identity is read from the differ's deterministic wording
// (internal/upgrade/crds.go): the title "<Kind> <version> schema: …" and
// the detail "… the <group>/<version> schema of <crd name> …"; the CRD name
// alone is used when the title labels the CRD by name.
func crdIdentity(c domain.Change) string {
	if !strings.HasPrefix(c.Provenance.Rule, "crd:") {
		return ""
	}
	name := ""
	if i := strings.Index(c.Detail, " schema of "); i >= 0 {
		tail := c.Detail[i+len(" schema of "):]
		if j := strings.IndexAny(tail, " ;:(\n\t"); j >= 0 {
			tail = tail[:j]
		}
		name = strings.TrimRight(tail, ".")
	}
	kind := ""
	if i := strings.Index(c.Title, " schema: "); i > 0 {
		head := c.Title[:i]
		if sp := strings.LastIndexByte(head, ' '); sp > 0 && !strings.Contains(head[:sp], ".") {
			kind = head[:sp]
		} else if sp > 0 && name == "" {
			name = head[:sp]
		}
	}
	group := ""
	if i := strings.IndexByte(name, '.'); i > 0 {
		group = name[i+1:]
	}
	switch {
	case group != "" && kind != "":
		return group + "/" + kind
	case name != "":
		return name
	}
	return kind
}

// tokenSet is the bag of words for near-duplicate detection.
func tokenSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(normalizeTitle(s)) {
		out[w] = true
	}
	return out
}

// jaccard is the overlap coefficient helper for near-duplicate detection.
func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for w := range a {
		if b[w] {
			inter++
		}
	}
	return float64(inter) / float64(len(a)+len(b)-inter)
}

// nearDuplicateThreshold: titles sharing this much vocabulary are near
// duplicates (cert-manager's upgrade guide and release notes phrase the same
// fact differently: "We have changed the default value of X from Never to
// Always" vs "The default value of X is now Always").
const nearDuplicateThreshold = 0.75
