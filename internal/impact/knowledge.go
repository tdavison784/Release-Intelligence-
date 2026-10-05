package impact

// Verified knowledge in the join (docs/phase3/learning-loop/DESIGN.md §2.6,
// §4). A VerifiedFact is release-level: its applicability condition is
// evaluated against THIS environment (condition.go), the trust ladder turns
// the result into a class (ClassifyKnowledge), and the finding attaches to
// every change of the edge that restates the fact (AttachedChanges).
//
// Composition with the deterministic join:
//   - deterministic findings are kept exactly as they are;
//   - per attached change, a knowledge finding REPLACES the change's unknown
//     records (supersession: nothing is counted twice);
//   - next to a deterministic affected or not-affected finding it is added
//     only when stronger (action > review > informational > not-affected),
//     so a knowledge not-affected never hides a computed action-required;
//   - the fact's upstream evidence is copied into the report's pool.
// Model proposals never reach this file: Build receives facts only.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// KnowledgeProducer is the producer of knowledge findings: the evaluation is
// deterministic; the fact's own verification travels in Finding.Knowledge.
const KnowledgeProducer = "impact.knowledge@v1"

// Rules of findings evaluated from verified knowledge.
const (
	RuleKnowledgeExposed   = domain.KnowledgeRulePrefix + "exposed"   // affected, from the exposure condition
	RuleKnowledgeOverlap   = domain.KnowledgeRulePrefix + "overlap"   // informational, from the overlap condition
	RuleKnowledgeClear     = domain.KnowledgeRulePrefix + "clear"     // not-affected
	RuleKnowledgeUndecided = domain.KnowledgeRulePrefix + "undecided" // unknown
)

// ClassifyKnowledge applies the trust ladder (DESIGN.md §4) to one fact and
// the evaluated exposure/overlap conditions (an overlap with an empty Value
// means the fact states none: false).
//
//	X       O              trusted (deterministic/human)   untrusted (consensus/proxy)
//	true    –              ExposedClass, high              min(ExposedClass, review), medium
//	false   true           informational                   informational, medium
//	false   false/unknown  not-affected, high              unknown · release-knowledge-gap
//	unknown –              unknown · the leaf's reason     unknown · the leaf's reason
//
// PO-2 (DECISIONS.md): a consensus fact marked ConsensusAction (every aspect
// at consensus or better, an action-eligible consequence every agreeing
// proposal requested as action, no validator refuting anything — proven when
// the fact is loaded) keeps its action-required class when the exposure is
// true with environment evidence; the finding is labelled "model consensus"
// through Knowledge.Verification/ConsensusAction. Consensus never clears.
func ClassifyKnowledge(f domain.VerifiedFact, exposure, overlap ConditionResult) (domain.ImpactClass, domain.Confidence, domain.UnknownReason) {
	trusted := f.Level().Trusted()
	consensusAction := f.ConsensusAction && f.Level() == domain.VerifiedConsensus
	conf := domain.ConfidenceHigh
	if !trusted {
		conf = domain.ConfidenceMedium // an untrusted fact never carries high confidence …
	}
	ov := overlap.Value
	if ov == "" {
		ov = False
	}
	switch exposure.Value {
	case True:
		class := domain.ImpactReviewRequired
		if c := f.Assertion.Consequence; c != nil && c.ExposedClass.Affected() {
			class = c.ExposedClass
		}
		if !trusted && class == domain.ImpactActionRequired {
			if consensusAction && len(exposure.Matches) > 0 {
				return class, domain.ConfidenceHigh, "" // … except a consensus-action ACTION REQUIRED (PO-2)
			}
			class = domain.ImpactReviewRequired
		}
		return class, conf, ""
	case False:
		if ov == True {
			return domain.ImpactInformational, conf, ""
		}
		if trusted {
			return domain.ImpactNotAffected, conf, ""
		}
		return domain.ImpactUnknown, conf, domain.UnknownReleaseKnowledgeGap
	}
	reason := exposure.Reason
	if !reason.Valid() {
		reason = domain.UnknownEvidenceGap
	}
	return domain.ImpactUnknown, conf, reason
}

// knowledgeRuleFor names the rule of a classified knowledge finding.
func knowledgeRuleFor(class domain.ImpactClass, exposed bool) string {
	switch {
	case class == domain.ImpactUnknown:
		return RuleKnowledgeUndecided
	case class == domain.ImpactNotAffected:
		return RuleKnowledgeClear
	case exposed:
		return RuleKnowledgeExposed
	}
	return RuleKnowledgeOverlap
}

// --- attachment (DESIGN.md §2.6) --------------------------------------------------------

// edgeLookup resolves edge evidence ids.
func edgeLookup(edge *domain.UpgradeEdge) func(domain.EvidenceID) (domain.Evidence, bool) {
	ix := make(map[domain.EvidenceID]domain.Evidence, len(edge.Evidence))
	for _, e := range edge.Evidence {
		ix[e.ID] = e
	}
	return func(id domain.EvidenceID) (domain.Evidence, bool) {
		e, ok := ix[id]
		return e, ok
	}
}

// factInEdge reports whether the fact belongs to this edge: same product,
// and its introducing release (when stated and parsable) lies in (From, To].
func factInEdge(f domain.VerifiedFact, edge *domain.UpgradeEdge) bool {
	if f.Status != domain.FactActive || f.Product != edge.Product.ID {
		return false
	}
	if f.Release == "" {
		return true
	}
	v, err := semver.NewVersion(strings.TrimSpace(f.Release))
	from, err1 := semver.NewVersion(edge.From.Semver)
	to, err2 := semver.NewVersion(edge.To.Semver)
	if err != nil || err1 != nil || err2 != nil {
		return true // unparsable releases are matched per change (anchor release rule)
	}
	return from.LessThan(v) && !to.LessThan(v)
}

// AttachedChanges returns every change of the edge that restates the fact
// (DESIGN.md §2.6): (1) note-derived, non-umbrella changes matching one of the
// fact's statement anchors; (2) computed changes whose subject IS the fact's
// subject (no subject outside its subtree — such a change is an umbrella for
// the fact); (3) the closure of (1)+(2) under the deterministic restatement
// grouping (identical normalized title; identical category + subject set;
// ≥ 0.75 title-token Jaccard on the same non-empty subjects), again skipping
// umbrellas. Computed changes enter only through (2).
func AttachedChanges(f domain.VerifiedFact, edge *domain.UpgradeEdge) []domain.Change {
	if edge == nil || !factInEdge(f, edge) {
		return nil
	}
	lookup := edgeLookup(edge)
	in := map[int]bool{}
	for i, c := range edge.Changes {
		if f.AttachesByAnchor(c, lookup) || restatesSubject(f, c) {
			in[i] = true
		}
	}
	// (3) closure under restatement grouping
	for changed := true; changed; {
		changed = false
		for i, c := range edge.Changes {
			if in[i] || !noteDerivedChange(c) || domain.IsUmbrella(c, lookup) || !releaseCompatible(f.Release, c.Release) {
				continue
			}
			for j := range in {
				if restates(c, edge.Changes[j]) {
					in[i], changed = true, true
					break
				}
			}
		}
	}
	idx := make([]int, 0, len(in))
	for i := range in {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	out := make([]domain.Change, 0, len(idx))
	for _, i := range idx {
		out = append(out, edge.Changes[i])
	}
	return out
}

func noteDerivedChange(c domain.Change) bool {
	return c.Provenance.Method == domain.MethodDeclared || c.Provenance.Method == domain.MethodHeuristic
}

func releaseCompatible(a, b string) bool { return a == "" || b == "" || a == b }

func normalizedTitle(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

func subjectSetKey(c domain.Change) string {
	if len(c.Subjects) == 0 {
		return ""
	}
	s := append([]string(nil), c.Subjects...)
	sort.Strings(s)
	return strings.Join(s, ",")
}

func titleJaccard(a, b string) float64 {
	x, y := map[string]bool{}, map[string]bool{}
	for _, w := range strings.Fields(normalizedTitle(a)) {
		x[w] = true
	}
	for _, w := range strings.Fields(normalizedTitle(b)) {
		y[w] = true
	}
	if len(x) == 0 || len(y) == 0 {
		return 0
	}
	inter := 0
	for w := range x {
		if y[w] {
			inter++
		}
	}
	return float64(inter) / float64(len(x)+len(y)-inter)
}

// restates is the deterministic duplicate grouping shared with internal/eval
// and internal/impactenrich.
func restates(a, b domain.Change) bool {
	if normalizedTitle(a.Title) != "" && normalizedTitle(a.Title) == normalizedTitle(b.Title) {
		return true
	}
	ka, kb := subjectSetKey(a), subjectSetKey(b)
	if ka == "" || ka != kb {
		return false
	}
	return a.Category == b.Category || titleJaccard(a.Title, b.Title) >= 0.75
}

// restatementRules are the computed diff rules that can restate a (family,
// change kind) — a fact never attaches to a computed change of another kind
// on the same subject (a deprecation is not the later removal).
var restatementRules = map[domain.SubjectFamily]map[domain.ChangeKind][]string{
	domain.SubjectHelmValue: {
		domain.ChangeKindRemoved:        {upgrade.RuleValuesRemoved, upgrade.RuleValuesSectionRemoved},
		domain.ChangeKindRenamed:        {upgrade.RuleValuesRemoved, upgrade.RuleValuesSectionRemoved},
		domain.ChangeKindDefaultChanged: {upgrade.RuleValuesDefaultChanged},
		domain.ChangeKindValueChanged:   {upgrade.RuleValuesDefaultChanged},
		domain.ChangeKindAdded:          {upgrade.RuleValuesAdded},
	},
	domain.SubjectCRDField: {
		domain.ChangeKindRemoved:             {upgrade.RuleCRDFieldsRemoved},
		domain.ChangeKindAdded:               {upgrade.RuleCRDFieldsAdded},
		domain.ChangeKindDefaultChanged:      {upgrade.RuleCRDDefaultChanged},
		domain.ChangeKindValidationTightened: {upgrade.RuleCRDEnumChanged, upgrade.RuleCRDFieldTypeChange},
		domain.ChangeKindNowRequired:         {upgrade.RuleCRDFieldRequired},
	},
	domain.SubjectGVK: {
		domain.ChangeKindRemoved:    {upgrade.RuleCRDVersionRemoved, upgrade.RuleCRDVersionUnserved},
		domain.ChangeKindDeprecated: {upgrade.RuleCRDVersionDeprecated},
		domain.ChangeKindAdded:      {upgrade.RuleCRDVersionAdded},
	},
	domain.SubjectImage: {
		domain.ChangeKindRemoved:      {upgrade.RuleImageRemoved},
		domain.ChangeKindRenamed:      {upgrade.RuleImageMoved},
		domain.ChangeKindValueChanged: {upgrade.RuleImageTagsChanged, upgrade.RuleImageMoved},
		domain.ChangeKindAdded:        {upgrade.RuleImageAdded},
	},
}

// restatesSubject reports whether the computed change c is a restatement of
// the fact by subject: a diff rule that matches the fact's change kind, and
// every subject of c is the fact's subject (or under it).
func restatesSubject(f domain.VerifiedFact, c domain.Change) bool {
	s, ch := f.Assertion.Subject, f.Assertion.Change
	if s == nil || ch == nil || noteDerivedChange(c) || len(c.Subjects) == 0 || !releaseCompatible(f.Release, c.Release) {
		return false
	}
	ok := false
	for _, r := range restatementRules[s.Family][ch.Type] {
		ok = ok || r == c.Provenance.Rule
	}
	if !ok {
		return false
	}
	under := func(subjectPath, p string) bool {
		rel := relate(stripArrayMarkers(subjectPath), stripArrayMarkers(p))
		return rel == relExact || rel == relSubjectAncestor
	}
	switch s.Family {
	case domain.SubjectHelmValue:
		for _, p := range c.Subjects {
			if !under(s.Path, p) {
				return false
			}
		}
		return true
	case domain.SubjectCRDField:
		id, ok := identityFromSchemaChange(c)
		if !ok || id.Group != s.Group || id.Kind == "" || id.Kind != s.Kind || (s.Version != "" && id.Version != s.Version) {
			return false
		}
		for _, p := range c.Subjects {
			if !under(s.Path, p) {
				return false
			}
		}
		return true
	case domain.SubjectGVK:
		kind := kindFromVersionTitle(c.Title)
		for _, sub := range c.Subjects {
			id, ok := parseCRDNameVersion(sub)
			if !ok || id.Group != s.Group || id.Version != s.Version || kind != s.Kind {
				return false
			}
		}
		return true
	case domain.SubjectImage:
		for _, sub := range c.Subjects {
			if sub != s.Name {
				return false
			}
		}
		return true
	}
	return false
}

// kindFromVersionTitle recovers the kind of a crd:version-* change from the
// differ's title ("API version `g/v` of <Kind> removed", "… no longer
// served", "… deprecated", "New API version `g/v` of <Kind>"); "" when the
// label is a CRD name (contains a dot) or the title has another shape.
func kindFromVersionTitle(title string) string {
	i := strings.Index(title, "` of ")
	if i < 0 {
		return ""
	}
	rest := title[i+len("` of "):]
	if j := strings.IndexByte(rest, ' '); j >= 0 {
		rest = rest[:j]
	}
	if rest == "" || strings.Contains(rest, ".") {
		return ""
	}
	return rest
}

// --- the Build pass -----------------------------------------------------------------------

// classStrength orders affected/not-affected classes for "added only if
// stronger"; unknown is never stronger than a decided finding.
var classStrength = map[domain.ImpactClass]int{
	domain.ImpactActionRequired: 4, domain.ImpactReviewRequired: 3, domain.ImpactInformational: 2,
	domain.ImpactNotAffected: 1, domain.ImpactUnknown: 0,
}

// usableFacts filters the input facts to those that may be evaluated: active
// and at or above the minimum verification level.
func usableFacts(facts []domain.VerifiedFact, min domain.VerificationLevel) []domain.VerifiedFact {
	if min == "" {
		min = domain.VerifiedHuman
	}
	var out []domain.VerifiedFact
	for _, f := range facts {
		if f.Status == domain.FactActive && f.Level().AtLeast(min) {
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// knowledge evaluates the usable facts and composes their findings with the
// deterministic ones.
func (b *builder) knowledge(facts []domain.VerifiedFact, min domain.VerificationLevel, render RenderedChangeEvaluator) {
	type evaluated struct {
		f        domain.VerifiedFact
		exposure ConditionResult
		overlap  ConditionResult
	}
	perChange := map[string][]domain.ImpactFinding{}
	var order []string
	changes := map[string]domain.Change{}
	for _, f := range usableFacts(facts, min) {
		attached := AttachedChanges(f, b.edge)
		if len(attached) == 0 || f.Assertion.Applicability == nil {
			continue
		}
		opts := ConditionOptions{Render: render}
		e := evaluated{f: f, exposure: EvaluateConditionWith(f.Assertion.Applicability.Exposure, b.env, b.edge, opts)}
		if ov := f.Assertion.Applicability.Overlap; ov != nil && e.exposure.Value == False {
			e.overlap = EvaluateConditionWith(*ov, b.env, b.edge, opts)
		}
		for _, c := range attached {
			kf, ok := b.knowledgeFinding(e.f, e.exposure, e.overlap, c)
			if !ok {
				continue
			}
			if _, seen := changes[c.ID]; !seen {
				order = append(order, c.ID)
				changes[c.ID] = c
			}
			perChange[c.ID] = append(perChange[c.ID], kf)
		}
	}
	superseded := map[string]bool{}
	for _, id := range order {
		strongest := -1
		for _, f := range b.findings {
			if f.ChangeID == id && f.Classification != domain.ImpactUnknown && classStrength[f.Classification] > strongest {
				strongest = classStrength[f.Classification]
			}
		}
		for _, kf := range perChange[id] {
			if strongest >= 0 && classStrength[kf.Classification] <= strongest {
				continue // never hides or downgrades a deterministic verdict
			}
			b.findings = append(b.findings, kf)
			superseded[id] = true
			b.cite(kf)
		}
	}
	if len(superseded) == 0 {
		return
	}
	kept := b.findings[:0]
	for _, f := range b.findings {
		if f.Knowledge == nil && f.Classification == domain.ImpactUnknown && superseded[f.ChangeID] {
			continue // the knowledge finding replaces this unknown record
		}
		kept = append(kept, f)
	}
	b.findings = kept
}

// cite records the evidence a knowledge finding uses.
func (b *builder) cite(f domain.ImpactFinding) {
	for _, id := range f.UpstreamEvidence {
		b.upCited[id] = true
	}
	for _, id := range f.EnvironmentEvidence {
		b.locCited[id] = true
	}
	for _, c := range f.Checks {
		for _, id := range c.Evidence {
			b.locCited[id] = true
		}
	}
}

// knowledgeFinding builds the finding of one fact for one attached change.
func (b *builder) knowledgeFinding(f domain.VerifiedFact, exposure, overlap ConditionResult, c domain.Change) (domain.ImpactFinding, bool) {
	class, conf, reason := ClassifyKnowledge(f, exposure, overlap)
	exposed := exposure.Value == True
	rule := knowledgeRuleFor(class, exposed)
	// upstream chain: the change's evidence ∪ the fact's evidence
	var up []domain.EvidenceID
	for _, id := range c.Evidence {
		if b.edgeEv[id] {
			up = appendUnique(up, id)
		}
	}
	for _, e := range f.Evidence {
		if !b.edgeEv[e.ID] {
			if _, ok := b.factEv[e.ID]; !ok {
				b.factEv[e.ID] = e
				b.factEvOrder = append(b.factEvOrder, e.ID)
			}
		}
		up = appendUnique(up, e.ID)
	}
	if len(up) == 0 {
		return domain.ImpactFinding{}, false
	}
	level := f.Level()
	kf := domain.ImpactFinding{
		ID:               "imp-" + domain.ShortHash(rule, c.ID, f.ID),
		Classification:   class,
		Rule:             rule,
		UpstreamEvidence: up,
		Provenance:       domain.Provenance{Method: domain.MethodComputed, Producer: KnowledgeProducer, Rule: rule, Confidence: conf},
		Knowledge:        &domain.KnowledgeRef{Fact: f.ID, Verification: level, Statement: factStatement(f)},
	}
	if level == domain.VerifiedConsensus {
		kf.Knowledge.Consensus = consensusScope(f)
		kf.Knowledge.ConsensusAction = f.ConsensusAction
		kf.Knowledge.Audited = f.ConsensusAction && f.AuditedBy != "" // PO-6
	}
	b.attachChange(&kf, c)
	var records []domain.Evidence
	switch class {
	case domain.ImpactActionRequired, domain.ImpactReviewRequired, domain.ImpactInformational:
		src := exposure
		if !exposed {
			src = overlap
		}
		kf.Matches = src.Matches
		for _, m := range src.Matches {
			kf.EnvironmentEvidence = appendUnique(kf.EnvironmentEvidence, m.Evidence...)
		}
		records = src.Records
		if cons := f.Assertion.Consequence; cons != nil && exposed {
			kf.Severity = cons.Severity
		}
		if len(kf.Matches) == 0 {
			return domain.ImpactFinding{}, false // a true without evidence is never stated
		}
	case domain.ImpactNotAffected:
		kf.Checks = append(append([]domain.ImpactCheck{}, exposure.Checks...), overlap.Checks...)
		records = append(append([]domain.Evidence{}, exposure.Records...), overlap.Records...)
		if len(kf.Checks) == 0 {
			return domain.ImpactFinding{}, false
		}
	case domain.ImpactUnknown:
		kf.UnknownReason = reason
		if exposure.Value == False {
			// an untrusted fact may not clear: the partial record stays, the
			// verdict needs a trusted verification
			kf.Checks = exposure.Checks
			records = exposure.Records
			kf.NeededToDetermine = []string{fmt.Sprintf("a deterministic or human verification of fact %s (it is %s-verified; untrusted knowledge never clears a change)", f.ID, level)}
		} else {
			kf.NeededToDetermine = exposure.Needed
		}
		if len(kf.NeededToDetermine) == 0 {
			kf.NeededToDetermine = []string{"the applicability condition could not be evaluated"}
		}
	}
	for _, r := range records {
		if _, ok := b.extraLocal[r.ID]; !ok {
			b.extraLocal[r.ID] = r
			b.extraLocalOrder = append(b.extraLocalOrder, r.ID)
		}
	}
	kf.Title, kf.Detail = knowledgeText(f, class, rule, exposure, overlap)
	return kf, true
}

// consensusScope labels a consensus fact: same-model when any consensus
// aspect rests on calls of one model family (the weaker, correlated-error
// case, PO-1), cross-model otherwise.
func consensusScope(f domain.VerifiedFact) domain.ConsensusScope {
	scope := domain.ConsensusCrossModel
	for _, v := range f.Verification {
		if v.Level == domain.VerifiedConsensus && v.Consensus == domain.ConsensusSameModel {
			scope = domain.ConsensusSameModel
		}
	}
	return scope
}

func factStatement(f domain.VerifiedFact) string {
	if s := strings.TrimSpace(f.Assertion.Statement); s != "" {
		return s
	}
	var parts []string
	if s := f.Assertion.Subject; s != nil {
		parts = append(parts, s.Key())
	}
	if c := f.Assertion.Change; c != nil {
		parts = append(parts, string(c.Type))
	}
	return strings.Join(parts, " ")
}

func knowledgeText(f domain.VerifiedFact, class domain.ImpactClass, rule string, exposure, overlap ConditionResult) (title, detail string) {
	st := factStatement(f)
	cons := f.Assertion.Consequence
	switch rule {
	case RuleKnowledgeExposed:
		title = "Applies to you: " + st
		if class == domain.ImpactActionRequired && f.Level() == domain.VerifiedConsensus {
			// CONTRACT-CHANGE(contract-6): PO-6 label, from the one source of truth
			ref := domain.KnowledgeRef{Verification: domain.VerifiedConsensus, ConsensusAction: f.ConsensusAction, Audited: f.AuditedBy != ""}
			title = "Applies to you (ACTION REQUIRED · " + ref.ActionLabel() + "): " + st
		}
	case RuleKnowledgeOverlap:
		title = "Touches your environment, which appears shielded: " + st
	case RuleKnowledgeClear:
		title = "Does not apply to you: " + st
	default:
		title = "Cannot tell whether this applies to you: " + st
	}
	var d []string
	if cons != nil {
		line := fmt.Sprintf("Consequence for an exposed environment: %s (%s).", cons.Kind, cons.ExposedClass)
		if s := strings.TrimSpace(cons.Statement); s != "" {
			line += " " + s
		}
		d = append(d, line)
		if r := strings.TrimSpace(cons.Remediation); r != "" && class.Affected() {
			d = append(d, "Remediation: "+r)
		}
	}
	if a := f.Assertion.Applicability; a != nil {
		d = append(d, fmt.Sprintf("Exposure condition (%s here): %s", exposure.Value, describe(a.Exposure)))
		if a.Overlap != nil && overlap.Value != "" {
			d = append(d, fmt.Sprintf("Overlap condition (%s here): %s", overlap.Value, describe(*a.Overlap)))
		}
	}
	level := f.Level()
	v := fmt.Sprintf("Verified knowledge %s (%s-verified", f.ID, level)
	switch {
	case level == domain.VerifiedConsensus && f.ConsensusAction && f.AuditedBy == "":
		v += fmt.Sprintf(", %s; ACTION REQUIRED here rests on model consensus (PO-2), not on a human or a validator, and is UNAUDITED: its human audit item is not yet accepted (PO-6); consensus never clears a change", consensusScope(f))
	case level == domain.VerifiedConsensus && f.ConsensusAction:
		v += fmt.Sprintf(", %s; ACTION REQUIRED here rests on model consensus (PO-2), audited and accepted by a human (%s); consensus never clears a change", consensusScope(f), f.AuditedBy)
	case level == domain.VerifiedConsensus:
		v += fmt.Sprintf(", %s; consensus knowledge is capped at review-required and never clears a change", consensusScope(f))
	case !level.Trusted():
		v += "; untrusted knowledge is capped at review-required and never clears a change"
	}
	d = append(d, v+").")
	return title, strings.Join(d, "\n")
}

// --- which facts may be evaluated ---------------------------------------------------------

// VerifyFacts returns the facts of the snapshot whose verification is proven
// by the snapshot's own records: each passes VerifiedFact.Validate and
// domain.ValidateFactRecords (a deterministic aspect rests on a confirming
// validation, a human/proxy aspect on an accept/correct decision by a
// reviewer of exactly that kind, a consensus aspect on agreeing independent
// proposals). A fact file that claims a level its records do not prove — a
// proxy decision dressed up as human, a basis that does not resolve — is
// rejected, never evaluated. rejected explains each refusal.
func VerifyFacts(s *knowledge.Snapshot) (facts []domain.VerifiedFact, rejected []string) {
	if s == nil {
		return nil, nil
	}
	rec := domain.FactRecords{
		Validations: map[string]domain.ValidationResult{}, Decisions: map[string]domain.ReviewDecision{},
		Items: map[string]domain.ReviewItem{}, Proposals: map[string]domain.SemanticProposal{},
	}
	for _, v := range s.Validations {
		rec.Validations[v.ID] = v
	}
	for _, d := range s.Decisions {
		rec.Decisions[d.ID] = d
	}
	for _, it := range s.ReviewItems {
		rec.Items[it.ID] = it
	}
	for _, p := range s.Proposals {
		rec.Proposals[p.ID] = p
	}
	for _, f := range s.Facts {
		if err := f.Validate(); err != nil {
			rejected = append(rejected, fmt.Sprintf("%s: %v", f.ID, err))
			continue
		}
		if err := domain.ValidateFactRecords(f, rec); err != nil {
			rejected = append(rejected, fmt.Sprintf("%s: verification not proven by its records: %v", f.ID, err))
			continue
		}
		facts = append(facts, f)
	}
	sort.SliceStable(facts, func(i, j int) bool { return facts[i].ID < facts[j].ID })
	return facts, rejected
}

// ReviewContexts maps each fact to the environment labels its human/proxy
// decisions were reviewed with (ReviewItem.Context), for the transfer subset
// of the evaluation (DESIGN.md §7): a link decided by a fact that was never
// reviewed with that environment as context measures transfer.
func ReviewContexts(s *knowledge.Snapshot) map[string][]string {
	out := map[string][]string{}
	if s == nil {
		return out
	}
	items := map[string]domain.ReviewItem{}
	for _, it := range s.ReviewItems {
		items[it.ID] = it
	}
	decisions := map[string]domain.ReviewDecision{}
	for _, d := range s.Decisions {
		decisions[d.ID] = d
	}
	for _, f := range s.Facts {
		for _, v := range f.Verification {
			for _, id := range v.Basis {
				d, ok := decisions[id]
				if !ok {
					continue
				}
				if it, ok := items[d.ReviewItemID]; ok && it.Context != nil && it.Context.Label != "" {
					out[f.ID] = appendUnique(out[f.ID], it.Context.Label)
				}
			}
		}
	}
	return out
}
