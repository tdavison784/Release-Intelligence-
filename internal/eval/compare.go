package eval

// Scoring: compare a real UpgradeEdge (and, when the case has an
// environment, a real ImpactReport) against the case's ground truth.
//
// The metrics are the regression signal of the dataset:
//
//   - recall     – of the hand-written must-find items, how many did the
//                  pipeline actually surface (matched by a change)?
//   - false positives – changes matching a notExpected entry: output that is
//                  actively wrong for an operator.
//   - duplicates – several changes restating one ground-truth fact (the
//                  pipeline aggregates many sources; a human must still read
//                  every copy).
//   - unsupported – conclusions whose evidence does not resolve inside the
//                  document that carries them.
//   - environment – for cases with a fixture: which expected items actually
//                  reach THIS environment as findings, and whether the join
//                  invents findings it should not.

import (
	"fmt"
	"sort"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Metrics is the per-entry regression snapshot (all fields deterministic).
type Metrics struct {
	// Expected items (ground truth) and how many were found.
	Expected int `json:"expected"`
	Found    int `json:"found"`
	// Expected by importance (the recall denominators per importance).
	ExpectedCritical  int `json:"expectedCritical"`
	ExpectedImportant int `json:"expectedImportant"`
	// Missed items by importance (Expected - Found distributed).
	MissedCritical  int `json:"missedCritical"`
	MissedImportant int `json:"missedImportant"`
	MissedMinor     int `json:"missedMinor"`
	// Pipeline output size and quality.
	Changes        int `json:"changes"`
	MatchedChanges int `json:"matchedChanges"` // changes that cover >= 1 expected item
	// FalsePositives are changes matching a notExpected entry.
	FalsePositives int `json:"falsePositives"`
	// DuplicateGroups restates the same fact via several changes.
	DuplicateGroups int `json:"duplicateGroups"`
	// Unsupported conclusions: evidence does not resolve.
	Unsupported int `json:"unsupported"`

	// --- classification scoring (G9) --------------------------------------

	// ClassificationScored counts expected items with an expected class whose
	// actual class was observable (a joined finding class, or a change-level
	// action flag); ClassificationMatched counts those where the classes
	// agree. Items without an expected class, or whose actual class carries
	// no signal, score nothing (and are visible in the audits).
	ClassificationScored  int `json:"classificationScored"`
	ClassificationMatched int `json:"classificationMatched"`
	// Finding-level class accounting across the entry's env reports (zero
	// without an environment).
	ActionFindings            int `json:"actionFindings"`            // findings classified action-required
	FalseActionFindings       int `json:"falseActionFindings"`       // ACTION findings judged wrong (unsupported, dataset FP, env FP, or adjudicated FP)
	ActionFindingsUnsupported int `json:"actionFindingsUnsupported"` // ACTION findings failing the provenance audit
	UnknownFindings           int `json:"unknownFindings"`           // UNKNOWN (insufficient evidence) findings
	// EvidenceCovered counts matched changes whose first hit cites a
	// resolving evidence URI (evidenceCoverage = covered / matched).
	EvidenceCovered int `json:"evidenceCovered"`
}

// Recall is found / expected (0 when nothing expected, which cannot happen
// after validation).
func (m Metrics) Recall() float64 {
	if m.Expected == 0 {
		return 0
	}
	return float64(m.Found) / float64(m.Expected)
}

// Precision is matched / (matched + false positives): of the output that is
// explainable against this dataset, how much is right.
func (m Metrics) Precision() float64 {
	den := m.MatchedChanges + m.FalsePositives
	if den == 0 {
		return 0
	}
	return float64(m.MatchedChanges) / float64(den)
}

// Coverage is matched / changes: how much of the pipeline's output this case
// accounts for (informational; a low value is not a failure — the pipeline
// legitimately reports more than any must-find list).
func (m Metrics) Coverage() float64 {
	if m.Changes == 0 {
		return 0
	}
	return float64(m.MatchedChanges) / float64(m.Changes)
}

// Missed is the total missed count.
func (m Metrics) Missed() int { return m.Expected - m.Found }

// EnvMetrics scores the environment join (zero value when the case has no
// environment).
type EnvMetrics struct {
	// Impact links: expected items this environment should surface.
	ImpactLinks      int `json:"impactLinks"`      // relevance != not-affected
	ImpactLinksHit   int `json:"impactLinksHit"`   // some finding joins a matching change
	NotAffectedLinks int `json:"notAffectedLinks"` // relevance == not-affected
	// A not-affected link that still produced a finding is an over-report.
	NotAffectedViolations int `json:"notAffectedViolations"`
	// Findings vocabulary.
	Findings         int `json:"findings"`
	FindingsExpected int `json:"findingsExpected"`
	FindingsFound    int `json:"findingsFound"`
	FindingsFP       int `json:"findingsFalsePositives"` // findings matching notExpectedFindings
	// Unsupported findings (chains do not resolve / change does not exist).
	Unsupported int `json:"unsupported"`
	// Undecided links (environment.undecidedImpact): links whose honest
	// answer is UNKNOWN. UndecidedHonest counts those the engine answered
	// honestly — no AFFECTED and no NOT-AFFECTED finding on any change the
	// item's matchers select (docs/phase3/learning-loop/UNDECIDED-SCORING.md).
	// Reported, never gated.
	UndecidedLinks  int `json:"undecidedLinks,omitempty"`
	UndecidedHonest int `json:"undecidedHonest,omitempty"`
}

// UnknownHonesty is undecided links answered honestly / undecided links (0
// when there are none). Vacuous on its own: an engine that decides nothing
// scores 1.0, so it is always read next to applicability accuracy and the
// affected-link hit rate.
func (m EnvMetrics) UnknownHonesty() float64 {
	return ratio(m.UndecidedLinks, m.UndecidedHonest)
}

// UndecidedAudit is the scoring of one undecided link.
type UndecidedAudit struct {
	ExpectedID string               `json:"expectedId"`
	Reason     domain.UnknownReason `json:"reason"`
	// Honest: no AFFECTED and no NOT-AFFECTED finding joins the item's changes.
	Honest bool `json:"honest"`
	// Overclaims are the findings that broke honesty (affected or not-affected).
	Overclaims []string `json:"overclaims,omitempty"`
	// Facts are the verified facts behind those findings (transfer subset).
	Facts []string `json:"facts,omitempty"`
}

// ImpactAccuracy is links hit / links (0 when no links).
func (m EnvMetrics) ImpactAccuracy() float64 {
	if m.ImpactLinks == 0 {
		return 0
	}
	return float64(m.ImpactLinksHit) / float64(m.ImpactLinks)
}

// FindingRecall is findings found / expected.
func (m EnvMetrics) FindingRecall() float64 {
	if m.FindingsExpected == 0 {
		return 0
	}
	return float64(m.FindingsFound) / float64(m.FindingsExpected)
}

// MatchAudit records how one expectation fared, so every hit and miss can be
// audited without re-running the pipeline.
type MatchAudit struct {
	ExpectedID string `json:"expectedId"`
	Title      string `json:"title"`
	Kind       string `json:"kind,omitempty"`
	Importance string `json:"importance,omitempty"`
	Found      bool   `json:"found"`
	// ExpectedClass (G9) is the class the case says a correct system should
	// output for this item ("" when the case predates the field).
	ExpectedClass string `json:"expectedClass,omitempty"`
	// ActualClass is the strongest class the pipeline output for this item:
	// the strongest joined finding class (environment entries), or the
	// change-level action flag ("action-required") when only the edge is
	// available. Empty = no class signal was observable.
	ActualClass string   `json:"actualClass,omitempty"`
	FindingIDs  []string `json:"findingIds,omitempty"`
	// Hits is one entry per matching change: which change, matched by which
	// matcher, backed by which evidence URI.
	Hits []Hit `json:"hits,omitempty"`
}

// Hit is one change that covered an expectation.
type Hit struct {
	ChangeID    string `json:"changeId"`
	ChangeTitle string `json:"changeTitle"`
	// MatchedBy is the matcher (all fields that had to match).
	MatchedBy string `json:"matchedBy"`
	// Evidence is a human-openable URI backing the matched change.
	Evidence string `json:"evidence,omitempty"`
}

// FPAudit records a false positive: the notExpected entry and the offending
// changes. Classification, when the entry declares one, is the class such
// output should NOT have carried — those cells feed the confusion matrix as
// expected=that-class, actual=<class of the offending findings>.
type FPAudit struct {
	Title          string   `json:"title"`
	Classification string   `json:"classification,omitempty"`
	ChangeIDs      []string `json:"changeIds"`
}

// DupAudit records a group of changes that restate one fact.
type DupAudit struct {
	Key       string   `json:"key"` // "title:…" | "subjects:…" | "near:title…"
	ChangeIDs []string `json:"changeIds"`
	Titles    []string `json:"titles"`
}

// UnsupportedAudit records an unsupported conclusion.
type UnsupportedAudit struct {
	ID     string `json:"id"`   // change or finding id
	Kind   string `json:"kind"` // "change" | "finding"
	Reason string `json:"reason"`
}

// FindingAudit records how one expectedFinding fared.
type FindingAudit struct {
	ID         string   `json:"id,omitempty"`
	Matcher    string   `json:"matcher"`
	Found      bool     `json:"found"`
	FindingIDs []string `json:"findingIds,omitempty"`
	// ExpectedClass is the matcher's classification clause, when it had one
	// ("" = the expectation does not pin a class); ActualClass is the class
	// of the first matching finding (the confusion matrix uses labelled
	// expectedFindings).
	ExpectedClass string `json:"expectedClass,omitempty"`
	ActualClass   string `json:"actualClass,omitempty"`
}

// EnvImpactAudit records how one expectedImpact link fared.
type EnvImpactAudit struct {
	ExpectedID string `json:"expectedId"`
	Relevance  string `json:"relevance"`
	// Hit: a finding exists whose change matches the expected item (only
	// meaningful for relevance != not-affected; for not-affected links Hit
	// true means over-report).
	Hit        bool     `json:"hit"`
	FindingIDs []string `json:"findingIds,omitempty"`
	Why        string   `json:"why,omitempty"`
	// Facts are the verified facts (vf-…) behind the knowledge findings that
	// hit the link (transfer reporting, DESIGN.md §7); empty without -knowledge.
	Facts []string `json:"facts,omitempty"`
}

// EntryResult is the scored outcome of one dataset entry.
type EntryResult struct {
	CaseID  string `json:"caseId"`
	Product string `json:"product"`
	From    string `json:"from"`
	To      string `json:"to"`
	// Error is set when the pipeline could not run at all (the entry then
	// scores zero found / all missed). A pipeline failure is an execution
	// error, never a reasoning miss; aggregate metrics count it as
	// pipelineFailures.
	Error string `json:"error,omitempty"`

	Metrics                Metrics            `json:"metrics"`
	Env                    *EnvMetrics        `json:"env,omitempty"`
	Matches                []MatchAudit       `json:"matches"`
	FalsePos               []FPAudit          `json:"falsePositives,omitempty"`
	Duplicates             []DupAudit         `json:"duplicates,omitempty"`
	UnsupportedConclusions []UnsupportedAudit `json:"unsupportedConclusions,omitempty"`
	EnvImpact              []EnvImpactAudit   `json:"envImpact,omitempty"`
	EnvUndecided           []UndecidedAudit   `json:"envUndecided,omitempty"`
	EnvFindings            []FindingAudit     `json:"envFindings,omitempty"`
	EnvFalsePos            []FPAudit          `json:"envFalsePositives,omitempty"`
	// Suggestions is the enriched-run scoring (opt-in `-enriched`; nil for
	// deterministic runs).
	Suggestions *SuggestionMetrics `json:"suggestions,omitempty"`
	// Confusion is the entry's labelled expected×actual cell counts (present
	// when at least one finding carried a label).
	Confusion *ConfusionMatrix `json:"confusion,omitempty"`
	// Adjudicated is the sample-based human adjudication of this entry's
	// changes (empty without an eval/adjudications/<case>.yaml file).
	Adjudicated AdjudicationStats `json:"adjudicated,omitempty"`
	// FPChangeIDs lists the change ids of the entry's dataset false positives
	// (the adjudication pass and the confusion-matrix exclusion set).
	FPChangeIDs []string `json:"fpChangeIds,omitempty"`
	// AllChangeIDs lists every change id of the edge (adjudication coverage
	// audit).
	AllChangeIDs []string `json:"allChangeIds,omitempty"`
	// Knowledge counts the knowledge findings of the entry (nil without any;
	// `ri eval -knowledge`).
	Knowledge *KnowledgeCounts `json:"knowledge,omitempty"`
	// actionFindings is the per-finding wrongness audit used by the
	// adjudication pass (not serialised; the counts are).
	actionFindings []ActionFindingRecord
}

// ScoreEntry scores one case against its edge (and report, when present).
// Either may be nil (Error must explain why): a pipeline that cannot run
// finds nothing, which is the honest result.
func ScoreEntry(c *Case, edge *domain.UpgradeEdge, report *domain.ImpactReport, runErr error) EntryResult {
	res := EntryResult{
		CaseID: c.ID, Product: c.Product, From: c.From, To: c.To,
	}
	if runErr != nil {
		res.Error = runErr.Error()
	}
	res.Knowledge = knowledgeCounts(report)
	res.Metrics.Expected = len(c.Expected)
	for _, e := range c.Expected {
		switch e.Importance {
		case ImportanceCritical:
			res.Metrics.ExpectedCritical++
		case ImportanceImportant:
			res.Metrics.ExpectedImportant++
		}
		res.Matches = append(res.Matches, MatchAudit{
			ExpectedID: e.ID, Title: e.Title, Kind: e.Kind, Importance: e.Importance,
			ExpectedClass: e.Classification,
		})
	}
	if edge != nil {
		scoreEdge(&res, c, edge)
	}
	if report != nil {
		if res.Env == nil {
			res.Env = &EnvMetrics{}
		}
		scoreReport(&res, c, edge, report)
	} else if c.Environment != nil {
		// the case declares an environment but no report was produced
		// (the join failed, or the pipeline never got that far): every
		// environment expectation misses, so the links stay in the
		// denominators instead of silently leaving them.
		res.Env = &EnvMetrics{}
		scoreMissingReport(&res, c)
	}
	finalizeClassification(&res, c)
	// distribute misses by importance; found is what the match audits say
	found := 0
	for i := range res.Matches {
		if res.Matches[i].Found {
			found++
			continue
		}
		switch res.Matches[i].Importance {
		case ImportanceCritical:
			res.Metrics.MissedCritical++
		case ImportanceImportant:
			res.Metrics.MissedImportant++
		default:
			res.Metrics.MissedMinor++
		}
	}
	res.Metrics.Found = found
	return res
}

// scoreMissingReport scores the environment of a case whose impact report
// does not exist. A missing report decides nothing: affected links count as
// unhit, not-affected links as unearned (a report that does not exist clears
// nothing), undecided links as not honestly answered (no credit either way),
// and expected findings as not found. Without this the case's links vanished from
// every applicability count while the run looked healthy.
func scoreMissingReport(res *EntryResult, c *Case) {
	em := res.Env
	for _, l := range c.Environment.ExpectedImpact {
		res.EnvImpact = append(res.EnvImpact, EnvImpactAudit{ExpectedID: l.Expected, Relevance: l.Relevance, Why: l.Why})
		if l.Relevance == RelevanceNotAffected {
			em.NotAffectedLinks++
			em.NotAffectedViolations++
			continue
		}
		em.ImpactLinks++
	}
	for _, l := range c.Environment.UndecidedImpact {
		res.EnvUndecided = append(res.EnvUndecided, UndecidedAudit{ExpectedID: l.Expected, Reason: l.Reason})
		em.UndecidedLinks++
	}
	em.FindingsExpected = len(c.Environment.ExpectedFindings)
}

// finalizeClassification scores the expected classes against the observed
// ones: an item scores when the case names a class AND the run showed a class
// signal (a joined finding, or a change-level action flag). Everything else
// stays unlabelled in the audits — silently counting unobservable classes as
// misses would manufacture failures the fixture cannot support.
func finalizeClassification(res *EntryResult, c *Case) {
	for i := range res.Matches {
		a := &res.Matches[i]
		if a.ExpectedClass == "" || a.ActualClass == "" {
			continue
		}
		res.Metrics.ClassificationScored++
		if a.ExpectedClass == a.ActualClass {
			res.Metrics.ClassificationMatched++
		}
	}
}

func scoreEdge(res *EntryResult, c *Case, edge *domain.UpgradeEdge) {
	ev := NewEvidenceIndex(edge)
	res.Metrics.Changes = len(edge.Changes)
	for _, ch := range edge.Changes {
		res.AllChangeIDs = append(res.AllChangeIDs, ch.ID)
	}

	// expected items
	for i := range res.Matches {
		audit := &res.Matches[i]
		exp := expectedByID(c, audit.ExpectedID)
		if exp == nil {
			continue
		}
		for _, ch := range edge.Changes {
			for mi := range exp.Match {
				if !exp.Match[mi].Matches(ch, ev) {
					continue
				}
				audit.Found = true
				audit.Hits = append(audit.Hits, Hit{
					ChangeID:    ch.ID,
					ChangeTitle: ch.Title,
					MatchedBy:   exp.Match[mi].String(),
					Evidence:    firstEvidenceURI(ch.Evidence, ev),
				})
				// change-level class signal: the edge vocabulary knows only
				// the action axis (ActionRequired); a finding-level class
				// (env entries) strengthens this in scoreReport.
				if ch.ActionRequired {
					audit.ActualClass = StrongestClass(audit.ActualClass, ClassActionRequired)
				}
			}
		}
	}

	// false positives: notExpected entries matched by real changes
	for _, ne := range c.NotExpected {
		var ids []string
		for _, ch := range edge.Changes {
			for _, m := range ne.Match {
				if m.Matches(ch, ev) {
					ids = append(ids, ch.ID)
					break
				}
			}
		}
		if len(ids) > 0 {
			res.Metrics.FalsePositives += len(ids)
			res.FalsePos = append(res.FalsePos, FPAudit{Title: ne.Title, ChangeIDs: ids, Classification: ne.Classification})
			res.FPChangeIDs = append(res.FPChangeIDs, ids...)
		}
	}

	// matched changes (for precision/coverage)
	matched := map[string]bool{}
	withEvidence := map[string]bool{}
	for i := range res.Matches {
		for _, h := range res.Matches[i].Hits {
			matched[h.ChangeID] = true
			if h.Evidence != "" {
				withEvidence[h.ChangeID] = true
			}
		}
	}
	res.Metrics.MatchedChanges = len(matched)
	for id := range matched {
		if withEvidence[id] {
			res.Metrics.EvidenceCovered++
		}
	}

	// duplicates
	res.Duplicates, res.Metrics.DuplicateGroups = findDuplicates(edge.Changes)

	// unsupported conclusions (independent of edge.Validate; we want the
	// count, not a pass/fail)
	res.UnsupportedConclusions = findUnsupportedChanges(edge)
	res.Metrics.Unsupported = len(res.UnsupportedConclusions)
}

func expectedByID(c *Case, id string) *Expected {
	for i := range c.Expected {
		if c.Expected[i].ID == id {
			return &c.Expected[i]
		}
	}
	return nil
}

// findDuplicates groups changes that restate the same fact: identical
// normalized title, identical category+subject set, or >= 0.75 title-token
// Jaccard overlap with identical non-empty subject sets.
func findDuplicates(changes []domain.Change) ([]DupAudit, int) {
	byTitle := map[string][]int{}
	bySubject := map[string][]int{}
	for i, c := range changes {
		byTitle[normalizeTitle(c.Title)] = append(byTitle[normalizeTitle(c.Title)], i)
		if k := subjectKey(c); k != "" {
			bySubject[k] = append(bySubject[k], i)
		}
	}
	groups := map[string][]int{}
	for k, idx := range byTitle {
		if len(idx) > 1 {
			groups["title:"+k] = idx
		}
	}
	for k, idx := range bySubject {
		if len(idx) > 1 {
			groups["subjects:"+k] = dedupIdx(append(groups["subjects:"+k], idx...))
		}
	}
	// near duplicates: same subject set, very similar titles
	for k, idx := range bySubject {
		if len(idx) < 2 {
			continue
		}
		for a := 0; a < len(idx); a++ {
			for b := a + 1; b < len(idx); b++ {
				if jaccard(tokenSet(changes[idx[a]].Title), tokenSet(changes[idx[b]].Title)) >= nearDuplicateThreshold {
					key := "near:" + k
					groups[key] = dedupIdx(append(groups[key], idx[a], idx[b]))
				}
			}
		}
	}
	var out []DupAudit
	for key, idx := range groups {
		if len(idx) < 2 {
			continue
		}
		sort.Ints(idx)
		var ids, titles []string
		for _, i := range idx {
			ids = append(ids, changes[i].ID)
			titles = append(titles, changes[i].Title)
		}
		out = append(out, DupAudit{Key: key, ChangeIDs: ids, Titles: titles})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, len(out)
}

func dedupIdx(in []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, i := range in {
		if !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	return out
}

// findUnsupportedChanges returns changes whose evidence does not resolve
// within the edge (or resolves to a record without a URI).
func findUnsupportedChanges(edge *domain.UpgradeEdge) []UnsupportedAudit {
	var out []UnsupportedAudit
	pool := map[domain.EvidenceID]domain.Evidence{}
	for _, ev := range edge.Evidence {
		pool[ev.ID] = ev
	}
	for _, ch := range edge.Changes {
		if len(ch.Evidence) == 0 {
			out = append(out, UnsupportedAudit{ID: ch.ID, Kind: "change", Reason: "cites no evidence"})
			continue
		}
		for _, id := range ch.Evidence {
			e, ok := pool[id]
			switch {
			case !ok:
				out = append(out, UnsupportedAudit{ID: ch.ID, Kind: "change", Reason: fmt.Sprintf("evidence %s does not resolve in the edge", id)})
			case e.URI == "":
				out = append(out, UnsupportedAudit{ID: ch.ID, Kind: "change", Reason: fmt.Sprintf("evidence %s has no URI", id)})
			default:
				continue
			}
			break
		}
	}
	return out
}

// findUnsupportedFindings returns findings whose provenance does not resolve
// within the report, or that join a change absent from the edge the report
// was built from (edge may be nil only when there is no edge at all;
// findings of a real report always come from one). The per-class provenance
// rules of docs/ACTION_CLASSIFICATION.md apply: affected findings need both
// chains (except the impact:security-fix rule, whose universal applicability
// carries the upstream chain only), unknown findings need upstream evidence
// plus neededToDetermine, not-affected findings need upstream evidence plus
// their evaluation record.
func findUnsupportedFindings(report *domain.ImpactReport, edge *domain.UpgradeEdge) []UnsupportedAudit {
	var out []UnsupportedAudit
	up := map[domain.EvidenceID]bool{}
	for _, ev := range report.Evidence {
		up[ev.ID] = true
	}
	local := map[domain.EvidenceID]bool{}
	for _, ev := range report.EnvironmentEvidence {
		local[ev.ID] = true
	}
	changes := map[string]bool{}
	for _, ch := range edge.Changes {
		changes[ch.ID] = true
	}
	for _, f := range report.Findings {
		var reason string
		securityFix := f.Rule == "impact:security-fix"
		switch {
		case len(f.UpstreamEvidence) == 0:
			reason = "cites an empty provenance chain"
		case f.Classification.Affected() && !securityFix && len(f.EnvironmentEvidence) == 0:
			reason = "affected finding cites no environment evidence"
		case securityFix && (f.Classification != domain.ImpactInformational || len(f.EnvironmentEvidence) != 0 || len(f.Matches) != 0):
			reason = "security-fix finding must be informational with the upstream chain only"
		case f.Classification == domain.ImpactUnknown && len(f.NeededToDetermine) == 0:
			reason = "unknown finding does not say what evidence was missing"
		case f.Classification == domain.ImpactNotAffected && len(f.Checks) == 0:
			reason = "not-affected finding has no evaluation record"
		default:
			for _, id := range f.UpstreamEvidence {
				if !up[id] {
					reason = fmt.Sprintf("upstream evidence %s does not resolve in the report", id)
					break
				}
			}
			for _, id := range f.EnvironmentEvidence {
				if reason != "" {
					break
				}
				if !local[id] {
					reason = fmt.Sprintf("environment evidence %s does not resolve in the report", id)
					break
				}
			}
			for _, c := range f.Checks {
				if reason != "" {
					break
				}
				for _, id := range c.Evidence {
					if !local[id] {
						reason = fmt.Sprintf("evaluation-record evidence %s does not resolve in the report", id)
						break
					}
				}
			}
			if reason == "" && f.ChangeID != "" && !changes[f.ChangeID] {
				reason = fmt.Sprintf("joins change %s which is not in the edge", f.ChangeID)
			}
		}
		if reason != "" {
			out = append(out, UnsupportedAudit{ID: f.ID, Kind: "finding", Reason: reason})
		}
	}
	return out
}

func scoreReport(res *EntryResult, c *Case, edge *domain.UpgradeEdge, report *domain.ImpactReport) {
	em := res.Env
	em.Findings = len(report.Findings)

	// map findings to the expected items their joined change covers
	expIDForChange := map[string]string{}
	if edge != nil {
		ev := NewEvidenceIndex(edge)
		for _, e := range c.Expected {
			for _, ch := range edge.Changes {
				for _, m := range e.Match {
					if m.Matches(ch, ev) {
						expIDForChange[ch.ID] = e.ID
						break
					}
				}
			}
		}
	}
	findingsFor := func(expID string) []string {
		var ids []string
		for _, f := range report.Findings {
			// only AFFECTED findings establish that an expected item reaches
			// this environment; an unknown ("cannot tell") or not-affected
			// ("checked, clear") record about the same change is not a hit.
			if f.Classification.Affected() && f.ChangeID != "" && expIDForChange[f.ChangeID] == expID {
				ids = append(ids, f.ID)
			}
		}
		return ids
	}

	// expected class per expected item from the environment links
	// (relevance maps 1:1 onto the class vocabulary).
	relevanceFor := map[string]string{}
	for _, l := range c.Environment.ExpectedImpact {
		relevanceFor[l.Expected] = l.Relevance
	}

	// the confusion matrix: labelled findings only. A finding is labelled by
	// (a) an expectedFinding matcher pinning a classification, or (b) the
	// expectedImpact relevance of the expected item its change covers. FP
	// findings (notExpectedFindings) are labelled by the notExpectedFindings
	// classification when present.
	matrix := NewConfusionMatrix()
	labelled := func(expected, actual string) {
		if expected == "" || actual == "" {
			return
		}
		matrix.Add(expected, actual)
	}

	for _, l := range c.Environment.ExpectedImpact {
		audit := EnvImpactAudit{ExpectedID: l.Expected, Relevance: l.Relevance, Why: l.Why}
		ids := findingsFor(l.Expected)
		audit.Hit = len(ids) > 0
		audit.FindingIDs = ids
		for _, f := range report.Findings {
			if f.Knowledge != nil && f.Classification.Affected() && f.ChangeID != "" && expIDForChange[f.ChangeID] == l.Expected {
				audit.Facts = appendUniqueString(audit.Facts, f.Knowledge.Fact)
			}
		}
		res.EnvImpact = append(res.EnvImpact, audit)
		if l.Relevance == RelevanceNotAffected {
			em.NotAffectedLinks++
			if audit.Hit {
				em.NotAffectedViolations++
			}
			continue
		}
		em.ImpactLinks++
		if audit.Hit {
			em.ImpactLinksHit++
		}
	}
	// undecided links: honest iff the engine claims neither AFFECTED nor
	// NOT-AFFECTED on any change the item's matchers select (UNKNOWN or no
	// finding at all is honest). Same attribution as the links above.
	for _, l := range c.Environment.UndecidedImpact {
		audit := UndecidedAudit{ExpectedID: l.Expected, Reason: l.Reason}
		for _, f := range report.Findings {
			if f.ChangeID == "" || expIDForChange[f.ChangeID] != l.Expected {
				continue
			}
			if f.Classification.Affected() || f.Classification == domain.ImpactNotAffected {
				audit.Overclaims = append(audit.Overclaims, f.ID)
				if f.Knowledge != nil {
					audit.Facts = appendUniqueString(audit.Facts, f.Knowledge.Fact)
				}
			}
		}
		audit.Honest = len(audit.Overclaims) == 0
		res.EnvUndecided = append(res.EnvUndecided, audit)
		em.UndecidedLinks++
		if audit.Honest {
			em.UndecidedHonest++
		}
	}
	// strengthen the per-item actual class from the joined findings (the
	// strongest class wins: a miss that hides required action is the worst
	// cell, so it must dominate the audit)
	findingByID := map[string]domain.ImpactFinding{}
	for _, f := range report.Findings {
		findingByID[f.ID] = f
	}
	for i := range res.Matches {
		audit := &res.Matches[i]
		for _, f := range report.Findings {
			if f.ChangeID == "" || expIDForChange[f.ChangeID] != audit.ExpectedID {
				continue
			}
			audit.FindingIDs = append(audit.FindingIDs, f.ID)
			audit.ActualClass = StrongestClass(audit.ActualClass, string(f.Classification))
		}
	}

	// expected findings
	for _, ef := range c.Environment.ExpectedFindings {
		audit := FindingAudit{ID: ef.ID, Matcher: ef.Match.String(), ExpectedClass: ef.Match.Classification}
		for _, f := range report.Findings {
			if ef.Match.Matches(f) {
				audit.Found = true
				audit.ActualClass = string(f.Classification)
				audit.FindingIDs = append(audit.FindingIDs, f.ID)
			}
		}
		res.EnvFindings = append(res.EnvFindings, audit)
	}
	em.FindingsExpected = len(c.Environment.ExpectedFindings)
	for _, a := range res.EnvFindings {
		if a.Found {
			em.FindingsFound++
		}
	}

	// finding false positives
	envFPChange := map[string]bool{} // findings' change ids matching a notExpected entry
	for _, nef := range c.Environment.NotExpectedFindings {
		var ids []string
		for _, f := range report.Findings {
			if nef.Match.Matches(f) {
				ids = append(ids, f.ID)
				envFPChange[f.ChangeID] = true
				labelled(nef.Classification, string(f.Classification))
			}
		}
		if len(ids) > 0 {
			em.FindingsFP += len(ids)
			res.EnvFalsePos = append(res.EnvFalsePos, FPAudit{Title: nef.Title, Classification: nef.Classification, ChangeIDs: ids})
		}
	}

	// per-finding class accounting + false-action detection + matrix cells
	// over labelled findings.
	unsupported := findUnsupportedFindings(report, edge)
	unsupportedIDs := map[string]bool{}
	for _, u := range unsupported {
		unsupportedIDs[u.ID] = true
	}
	fpChange := map[string]bool{}
	for _, id := range res.FPChangeIDs {
		fpChange[id] = true
	}
	// expectedFinding labels: a matcher pinning a classification is the
	// dataset's per-finding label ("this subject's correct class IS x"); it
	// wins over the coarser expectedImpact relevance.
	efLabel := func(f domain.ImpactFinding) string {
		for _, ef := range c.Environment.ExpectedFindings {
			if ef.Match.Classification == "" {
				continue
			}
			if findingMatchesShape(ef.Match, f) {
				return ef.Match.Classification
			}
		}
		return ""
	}
	for _, f := range report.Findings {
		switch string(f.Classification) {
		case ClassActionRequired:
			res.Metrics.ActionFindings++
			unsupportedFinding := unsupportedIDs[f.ID]
			if unsupportedFinding {
				res.Metrics.ActionFindingsUnsupported++
			}
			wrong := unsupportedFinding || envFPChange[f.ChangeID] || fpChange[f.ChangeID]
			if !wrong {
				if expID, ok := expIDForChange[f.ChangeID]; ok {
					// over-classification: ground truth names a softer class
					// for this item and the item is labelled at all
					if cls := classForRelevance(relevanceFor[expID]); cls != "" && cls != ClassActionRequired {
						wrong = true
					}
				}
			}
			if wrong {
				res.Metrics.FalseActionFindings++
			}
			rec := ActionFindingRecord{FindingID: f.ID, ChangeID: f.ChangeID, Wrong: wrong}
			if k := f.Knowledge; k != nil && k.Verification == domain.VerifiedConsensus {
				rec.Consensus = consensusAudit(*k)
			}
			res.actionFindings = append(res.actionFindings, rec)
		case ClassUnknown:
			res.Metrics.UnknownFindings++
		}
		// matrix: one cell per (expected item × change) pair — the strongest
		// class among the findings joining that change — plus per-finding
		// cells for expectedFinding-labelled subjects. Counting every
		// finding would let one fragmented change (a 25-key values-section
		// removal) manufacture dozens of cells out of one underlying
		// judgement.
		if expected := efLabel(f); expected != "" {
			labelled(expected, string(f.Classification))
		}
	}
	strongestByChange := map[string]string{}
	for _, f := range report.Findings {
		strongestByChange[f.ChangeID] = StrongestClass(strongestByChange[f.ChangeID], string(f.Classification))
	}
	for changeID, expID := range expIDForChange {
		expected := classForRelevance(relevanceFor[expID])
		if expected == "" {
			continue
		}
		if actual := strongestByChange[changeID]; actual != "" {
			labelled(expected, actual)
		}
	}
	if matrix.Labelled > 0 {
		res.Confusion = matrix
	}
	scoreSuggestions(res, c, report, expIDForChange, relevanceFor, efLabel)

	// unsupported findings (both chains must resolve and join a real change)
	if len(unsupported) > 0 {
		res.UnsupportedConclusions = append(res.UnsupportedConclusions, unsupported...)
		res.Metrics.Unsupported += len(unsupported)
		em.Unsupported = len(unsupported)
	}
}

// classForRelevance maps the dataset relevance vocabulary onto the class
// vocabulary (1:1 except the name of review).
func classForRelevance(rel string) string {
	switch rel {
	case RelevanceActionRequired:
		return ClassActionRequired
	case RelevanceReview:
		return ClassReviewRequired
	case RelevanceInformational:
		return ClassInformational
	case RelevanceNotAffected:
		return ClassNotAffected
	}
	return ""
}

func appendUniqueString(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
}
