package semantic

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// CandidateProducer is the producer string of every candidate.
const CandidateProducer = "semantic.candidates@v1"

// Grouping rules (SemanticCandidate.Grouping; DESIGN.md §2.1). A cluster
// formed by several rules lists them joined with "+", sorted.
const (
	GroupSingle           = "single"
	GroupSameStatement    = "same-statement"     // members quote the same statement (shared StatementKey)
	GroupSameTitle        = "same-title"         // identical normalised title
	GroupTitleJaccard     = "title-jaccard"      // same named subject + ≥ minJaccard title-token overlap
	GroupSubjectNamed     = "subject-named"      // a computed diff whose subjects a prose member names verbatim
	minJaccard            = 0.5                  // with an identical, non-empty subject signature
	maxHints              = 24
	maxSubjectsPerCompute = 1 // distinct subject roots a computed member may span
)

// noJoinRules are the computed diff rules the deterministic join has no join
// rule for (internal/impact: unimplementedDiffRules). Their changes are
// unknown by construction and become candidates of their own.
var noJoinRules = map[string]bool{
	upgrade.RuleCRDAdded: true, upgrade.RuleCRDVersionAdded: true,
	upgrade.RuleCRDStorageChanged: true, upgrade.RuleCRDFieldsAdded: true,
	upgrade.RuleImageAdded:        true,
	upgrade.RuleCRDDefaultChanged: true, upgrade.RuleCRDEnumChanged: true,
	upgrade.RuleCRDFieldRequired: true, upgrade.RuleCRDFieldTypeChange: true,
}

// joinRules are the computed diff rules the join already evaluates
// (internal/impact: joinRules). Their changes never start a candidate, but
// may join a prose cluster as a subject-named member so the resulting fact
// also explains them (DESIGN.md §2.1 rule 3).
var joinRules = map[string]bool{
	upgrade.RuleValuesRemoved: true, upgrade.RuleValuesSectionRemoved: true,
	upgrade.RuleValuesDefaultChanged: true, upgrade.RuleValuesAdded: true,
	upgrade.RuleCRDRemoved: true, upgrade.RuleCRDVersionRemoved: true,
	upgrade.RuleCRDVersionUnserved: true, upgrade.RuleCRDVersionDeprecated: true,
	upgrade.RuleCRDFieldsRemoved: true,
	upgrade.RuleImageRemoved:     true, upgrade.RuleImageMoved: true, upgrade.RuleImageTagsChanged: true,
}

// Skip reasons recorded by CandidateReport.
const (
	SkipRoutine      = "routine"
	SkipSecurityFix  = "security-fix"
	SkipUmbrella     = "umbrella"
	SkipNoEvidence   = "no-evidence"
	SkipMultiSubject = "multi-subject-computed"
	SkipInvalid      = "invalid-candidate"
)

// Skip is one edge change that could have been a member but is not.
type Skip struct {
	ChangeID string `json:"changeId"`
	Reason   string `json:"reason"`
	Detail   string `json:"detail,omitempty"`
}

// CandidateReport is what candidate generation did with an edge.
type CandidateReport struct {
	Candidates []domain.SemanticCandidate `json:"candidates"`
	Skipped    []Skip                     `json:"skipped,omitempty"`
	// Members counts the changes that became members.
	Members int `json:"members"`
}

// Candidates builds the restatement clusters of an edge (DESIGN.md §2.1):
// deterministic, environment-independent, never across distinct subjects,
// never including an umbrella. Members are the changes the deterministic join
// leaves unknown by construction — non-routine note-derived changes that are
// not impact:security-fix, and computed changes of diff rules without a join
// rule — plus computed changes with a join rule that a prose member names
// verbatim. Order: by the first member's position in the edge.
func Candidates(edge *domain.UpgradeEdge, now time.Time) []domain.SemanticCandidate {
	return BuildCandidates(edge, now).Candidates
}

// unit is one prospective member.
type unit struct {
	idx      int // position in edge.Changes
	c        domain.Change
	prose    bool
	sig      map[string]bool
	tokens   map[string]bool
	stmtKeys map[string]bool
	anchor   *domain.ChangeAnchor
	roots    []string // computed: subject roots
	kind     string   // computed crd:*: the kind/label parsed from the title
	chart    string   // computed values:*: the subchart named in the title ("(chart cni)")
}

// BuildCandidates is Candidates with the record of what was skipped and why.
func BuildCandidates(edge *domain.UpgradeEdge, now time.Time) CandidateReport {
	var rep CandidateReport
	if edge == nil {
		return rep
	}
	evByID := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range edge.Evidence {
		evByID[e.ID] = e
	}
	lookup := func(id domain.EvidenceID) (domain.Evidence, bool) { e, ok := evByID[id]; return e, ok }

	var prose, primary, secondary []*unit
	for i, c := range edge.Changes {
		switch c.Provenance.Method {
		case domain.MethodDeclared, domain.MethodHeuristic:
			switch {
			case c.Routine:
				rep.Skipped = append(rep.Skipped, Skip{ChangeID: c.ID, Reason: SkipRoutine, Detail: c.RoutineKind})
				continue
			case securityFix(c):
				rep.Skipped = append(rep.Skipped, Skip{ChangeID: c.ID, Reason: SkipSecurityFix})
				continue
			case len(c.Evidence) == 0:
				rep.Skipped = append(rep.Skipped, Skip{ChangeID: c.ID, Reason: SkipNoEvidence})
				continue
			case domain.IsUmbrella(c, lookup):
				rep.Skipped = append(rep.Skipped, Skip{ChangeID: c.ID, Reason: SkipUmbrella, Detail: "bundles several upstream items; facts never attach to umbrellas"})
				continue
			}
			a := domain.NewChangeAnchor(c, lookup)
			if len(a.StatementKeys) == 0 {
				rep.Skipped = append(rep.Skipped, Skip{ChangeID: c.ID, Reason: SkipNoEvidence, Detail: "no evidence record resolves in the edge"})
				continue
			}
			u := &unit{idx: i, c: c, prose: true, sig: subjectSignature(c.Title), tokens: tokens(c.Title), anchor: &a, stmtKeys: map[string]bool{}}
			for _, k := range a.StatementKeys {
				u.stmtKeys[k] = true
			}
			prose = append(prose, u)
		case domain.MethodComputed:
			rule := c.Provenance.Rule
			if !noJoinRules[rule] && !joinRules[rule] {
				continue // advisories, compatibility, lifecycle, artifacts: evaluated elsewhere
			}
			if len(c.Evidence) == 0 {
				rep.Skipped = append(rep.Skipped, Skip{ChangeID: c.ID, Reason: SkipNoEvidence})
				continue
			}
			u := &unit{idx: i, c: c, roots: subjectRoots(c.Subjects), kind: computedKind(c), chart: chartOf(c)}
			if noJoinRules[rule] {
				if len(u.roots) > maxSubjectsPerCompute {
					rep.Skipped = append(rep.Skipped, Skip{ChangeID: c.ID, Reason: SkipMultiSubject,
						Detail: fmt.Sprintf("%d distinct subjects (%s); one fact has one subject", len(u.roots), strings.Join(capStrings(u.roots, 4), ", "))})
					continue
				}
				primary = append(primary, u)
			} else {
				secondary = append(secondary, u)
			}
		}
	}

	// 1. prose clusters (union-find; a merge never mixes releases or
	// distinct non-empty subject signatures).
	parent := make([]int, len(prose))
	rules := make([]map[string]bool, len(prose))
	sigOf := make([]map[string]bool, len(prose))
	for i := range prose {
		parent[i], rules[i], sigOf[i] = i, map[string]bool{}, prose[i].sig
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i := 0; i < len(prose); i++ {
		for j := i + 1; j < len(prose); j++ {
			a, b := prose[i], prose[j]
			if a.c.Release != b.c.Release {
				continue
			}
			rule := ""
			switch {
			case shareKey(a.stmtKeys, b.stmtKeys):
				rule = GroupSameStatement
			case !sameSet(a.sig, b.sig):
				continue // never across distinct subjects
			case normalizeTitle(a.c.Title) == normalizeTitle(b.c.Title):
				rule = GroupSameTitle
			case len(a.sig) > 0 && jaccard(a.tokens, b.tokens) >= minJaccard:
				rule = GroupTitleJaccard
			default:
				continue
			}
			ri, rj := find(i), find(j)
			if ri == rj {
				rules[ri][rule] = true
				continue
			}
			// the merged cluster keeps one subject signature
			if len(sigOf[ri]) > 0 && len(sigOf[rj]) > 0 && !sameSet(sigOf[ri], sigOf[rj]) {
				continue
			}
			parent[rj] = ri
			if len(sigOf[ri]) == 0 {
				sigOf[ri] = sigOf[rj]
			}
			for r := range rules[rj] {
				rules[ri][r] = true
			}
			rules[ri][rule] = true
		}
	}
	type cluster struct {
		members []*unit
		rules   map[string]bool
		sig     map[string]bool
	}
	byRoot := map[int]*cluster{}
	var clusters []*cluster
	for i, u := range prose {
		r := find(i)
		cl, ok := byRoot[r]
		if !ok {
			cl = &cluster{rules: rules[r], sig: sigOf[r]}
			byRoot[r] = cl
			clusters = append(clusters, cl)
		}
		cl.members = append(cl.members, u)
	}

	// 2. subject-named: a computed change joins the one prose cluster whose
	// subject signature names every subject root it has (and, for CRD
	// changes, the kind). Ambiguity (several clusters, or several distinct
	// computed subjects for one cluster) joins nothing.
	matches := map[*cluster][]*unit{}
	joined := map[*unit]bool{}
	for _, u := range append(append([]*unit{}, primary...), secondary...) {
		var hit []*cluster
		for _, cl := range clusters {
			if namesComputed(cl.sig, cl.members, u) {
				hit = append(hit, cl)
			}
		}
		if len(hit) == 1 {
			matches[hit[0]] = append(matches[hit[0]], u)
		}
	}
	for cl, us := range matches {
		roots := map[string]bool{}
		for _, u := range us {
			roots[u.chart+"|"+strings.Join(u.roots, ",")] = true // a subchart's key is another subject
		}
		if len(roots) != 1 {
			continue
		}
		for _, u := range us {
			cl.members = append(cl.members, u)
			joined[u] = true
		}
		cl.rules[GroupSubjectNamed] = true
	}
	for _, u := range primary {
		if !joined[u] {
			clusters = append(clusters, &cluster{members: []*unit{u}, rules: map[string]bool{}})
		}
	}

	sort.SliceStable(clusters, func(i, j int) bool { return firstIdx(clusters[i].members) < firstIdx(clusters[j].members) })
	for _, cl := range clusters {
		sort.SliceStable(cl.members, func(i, j int) bool { return cl.members[i].idx < cl.members[j].idx })
		cand := buildCandidate(edge, cl.members, cl.rules, evByID, now)
		if err := cand.Validate(); err != nil {
			for _, u := range cl.members {
				rep.Skipped = append(rep.Skipped, Skip{ChangeID: u.c.ID, Reason: SkipInvalid, Detail: oneLine(err.Error())})
			}
			continue
		}
		rep.Candidates = append(rep.Candidates, cand)
		rep.Members += len(cand.Members)
	}
	return rep
}

func firstIdx(us []*unit) int {
	m := us[0].idx
	for _, u := range us {
		if u.idx < m {
			m = u.idx
		}
	}
	return m
}

func buildCandidate(edge *domain.UpgradeEdge, us []*unit, rules map[string]bool, evByID map[domain.EvidenceID]domain.Evidence, now time.Time) domain.SemanticCandidate {
	rep := us[0]
	for _, u := range us {
		if u.prose {
			rep = u
			break
		}
	}
	release := ""
	var members []domain.CandidateMember
	var evidence []domain.Evidence
	seenEv := map[domain.EvidenceID]bool{}
	var texts []string
	for _, u := range us {
		m := domain.CandidateMember{ChangeID: u.c.ID}
		if u.prose {
			m.Anchor = u.anchor
			release = u.c.Release
		} else {
			m.Computed = true
			texts = append(texts, u.c.Subjects...)
		}
		members = append(members, m)
		for _, id := range u.c.Evidence {
			if e, ok := evByID[id]; ok && !seenEv[id] {
				seenEv[id] = true
				evidence = append(evidence, e)
			}
		}
		texts = append(texts, u.c.Title, u.c.Detail)
	}
	grouping := GroupSingle
	if len(us) > 1 {
		var rs []string
		for r := range rules {
			rs = append(rs, r)
		}
		sort.Strings(rs)
		grouping = strings.Join(rs, "+")
	}
	return domain.SemanticCandidate{
		ID:        domain.CandidateID(edge.Product.ID, release, members),
		Product:   edge.Product.ID,
		Release:   release,
		Members:   members,
		Grouping:  grouping,
		Category:  rep.c.Category,
		Title:     oneLine(rep.c.Title),
		Text:      candidateText(rep, us),
		Evidence:  evidence,
		Hints:     Hints(maxHints, texts...),
		Producer:  CandidateProducer,
		CreatedAt: now.UTC().Truncate(time.Second),
	}
}

// candidateText makes the candidate self-contained (a proposer sees only
// the candidate): the representative's detail, then every other member's
// statement, computed members with their rule and subjects.
func candidateText(rep *unit, us []*unit) string {
	var b strings.Builder
	if d := strings.TrimSpace(rep.c.Detail); d != "" {
		b.WriteString(shorten(d, maxTextPerMember))
	}
	for _, u := range us {
		if u == rep {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		if u.prose {
			s := strings.TrimSpace(u.c.Detail)
			if s == "" || len(s) < len(u.c.Title) {
				s = u.c.Title
			}
			fmt.Fprintf(&b, "- restated: %s", shorten(oneLine(s), maxTextPerMember))
		} else {
			fmt.Fprintf(&b, "- computed artifact diff (%s): %s; subjects: %s", u.c.Provenance.Rule, oneLine(u.c.Title),
				strings.Join(capStrings(u.c.Subjects, maxSubjectsShown), ", "))
			if d := strings.TrimSpace(u.c.Detail); d != "" {
				fmt.Fprintf(&b, " — %s", shorten(oneLine(d), 300))
			}
		}
	}
	if rep.c.Provenance.Method == domain.MethodComputed {
		head := fmt.Sprintf("computed artifact diff (%s); subjects: %s", rep.c.Provenance.Rule, strings.Join(capStrings(rep.c.Subjects, maxSubjectsShown), ", "))
		if b.Len() > 0 {
			return head + "\n" + b.String()
		}
		return head
	}
	return b.String()
}

const maxTextPerMember = 1500

// securityFix mirrors the impact:security-fix rule (internal/impact): a
// note-derived security remediation without a stronger signal is
// informational for every environment and never unknown.
func securityFix(c domain.Change) bool {
	return c.Provenance.Producer == normalize.ProducerNotes && upgrade.IsSecurityItem(c) && !c.Breaking && !c.ActionRequired
}

func shareKey(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

// subjectRoots reduces subject paths to the roots of their subtrees:
// "spec.nodeSelector" and "spec.nodeSelector.matchLabels" are one subject.
func subjectRoots(subjects []string) []string {
	ss := append([]string(nil), subjects...)
	sort.Strings(ss)
	var roots []string
	for _, s := range ss {
		under := false
		for _, r := range roots {
			if s == r || strings.HasPrefix(s, r+".") || strings.HasPrefix(s, r+"[]") {
				under = true
				break
			}
		}
		if !under {
			roots = append(roots, s)
		}
	}
	return roots
}

// computedKind is the label of a CRD schema change title
// ("Certificate v1 schema: …" → "Certificate"); "" for other rules.
func computedKind(c domain.Change) string {
	if !strings.HasPrefix(c.Provenance.Rule, "crd:") {
		return ""
	}
	head, _, ok := strings.Cut(c.Title, " schema: ")
	if !ok {
		return ""
	}
	if sp := strings.LastIndexByte(head, ' '); sp > 0 {
		return head[:sp]
	}
	return ""
}

// namesComputed reports whether a prose cluster names the computed change's
// subject verbatim: every subject (or, for a values section, the section
// root) is in the cluster's subject signature, and a CRD change's kind
// appears in a member title.
func namesComputed(sig map[string]bool, members []*unit, u *unit) bool {
	if len(sig) == 0 || len(u.c.Subjects) == 0 {
		return false
	}
	all := true
	for _, s := range u.c.Subjects {
		if !sig[strings.ToLower(s)] {
			all = false
			break
		}
	}
	if !all {
		// a section change ("bgp.*") is named by its section
		sec := strings.ToLower(sectionOf(u.c))
		if sec == "" || !(sig[sec] || sig[sec+".*"]) {
			return false
		}
	}
	if u.kind != "" {
		found := false
		for _, m := range members {
			if containsWord(m.c.Title, u.kind) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

var chartRe = regexp.MustCompile(`\(chart ([^)]+)\)\s*$`)

// chartOf returns the subchart a values change is about ("… (chart cni)").
func chartOf(c domain.Change) string {
	if m := chartRe.FindStringSubmatch(c.Title); m != nil {
		return m[1]
	}
	return ""
}

// sectionOf returns the section of a values:section-removed change
// ("Helm values section `bgp.*` removed" → "bgp").
func sectionOf(c domain.Change) string {
	if c.Provenance.Rule != upgrade.RuleValuesSectionRemoved {
		return ""
	}
	if m := codeSpanRe.FindStringSubmatch(c.Title); m != nil {
		return strings.TrimSuffix(m[1], ".*")
	}
	return ""
}

func containsWord(text, word string) bool {
	lt, lw := strings.ToLower(text), strings.ToLower(word)
	for i := 0; ; {
		j := strings.Index(lt[i:], lw)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(lw)
		before := start == 0 || !isWordByte(lt[start-1])
		after := end == len(lt) || !isWordByte(lt[end])
		if before && after {
			return true
		}
		i = start + 1
	}
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

func capStrings(xs []string, n int) []string {
	if len(xs) > n {
		return append(append([]string{}, xs[:n]...), fmt.Sprintf("+%d more", len(xs)-n))
	}
	return xs
}
