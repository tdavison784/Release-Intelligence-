package knowledge

import (
	"sort"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ComputeMetrics computes every metric of DESIGN.md §7's table from a
// snapshot. It is deterministic: GeneratedAt is the latest timestamp in the
// snapshot, not the wall clock.
func ComputeMetrics(s *Snapshot) LoopMetrics {
	m := LoopMetrics{GeneratedAt: latestSnapshotTime(s)}
	m.Agreement = agreementMetrics(s)
	m.Models = modelMetrics(s)
	m.Review = reviewMetrics(s)
	m.Facts = factMetrics(s)
	return m
}

func latestSnapshotTime(s *Snapshot) time.Time {
	var t time.Time
	bump := func(x time.Time) {
		if x.After(t) {
			t = x
		}
	}
	for _, c := range s.Candidates {
		bump(c.CreatedAt)
	}
	for _, p := range s.Proposals {
		if p.Provenance.GeneratedAt != nil {
			bump(*p.Provenance.GeneratedAt)
		}
	}
	for _, v := range s.Validations {
		bump(v.CheckedAt)
	}
	for _, i := range s.ReviewItems {
		bump(i.CreatedAt)
	}
	for _, d := range s.Decisions {
		bump(d.DecidedAt)
	}
	for _, f := range s.Facts {
		bump(f.CreatedAt)
	}
	return t
}

// --- agreement ----------------------------------------------------------------

func agreementMetrics(s *Snapshot) []AgreementMetric {
	type key struct {
		aspect domain.Aspect
		task   domain.ProposalTask
	}
	// per (aspect, task, candidate): model → digests asserted
	byKey := map[key]map[string]map[string]map[string]bool{}
	for _, p := range s.Proposals {
		for _, x := range domain.Aspects {
			if !p.Assertion.Has(x) {
				continue
			}
			k := key{x, p.Task}
			if byKey[k] == nil {
				byKey[k] = map[string]map[string]map[string]bool{}
			}
			if byKey[k][p.CandidateID] == nil {
				byKey[k][p.CandidateID] = map[string]map[string]bool{}
			}
			mm := byKey[k][p.CandidateID]
			if mm[model(p)] == nil {
				mm[model(p)] = map[string]bool{}
			}
			mm[model(p)][p.Assertion.AspectDigest(x)] = true
		}
	}
	var out []AgreementMetric
	for k, cands := range byKey {
		am := AgreementMetric{Aspect: k.aspect, Task: k.task, Pairwise: map[string]float64{}}
		agreeing := 0
		pairTotal := map[string]int{}
		pairAgree := map[string]int{}
		for _, models := range cands {
			if len(models) < 2 {
				continue
			}
			am.Candidates++
			all := map[string]bool{}
			for _, ds := range models {
				for d := range ds {
					all[d] = true
				}
			}
			if len(all) == 1 {
				agreeing++
			}
			names := make([]string, 0, len(models))
			for n := range models {
				names = append(names, n)
			}
			sort.Strings(names)
			for i := range names {
				for j := i + 1; j < len(names); j++ {
					pk := names[i] + "|" + names[j]
					pairTotal[pk]++
					a, b := models[names[i]], models[names[j]]
					if len(a) == 1 && len(b) == 1 && sameKeys(a, b) {
						pairAgree[pk]++
					}
				}
			}
		}
		if am.Candidates == 0 {
			continue
		}
		am.Agreement = float64(agreeing) / float64(am.Candidates)
		for pk, n := range pairTotal {
			am.Pairwise[pk] = float64(pairAgree[pk]) / float64(n)
		}
		out = append(out, am)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Aspect != out[j].Aspect {
			return aspectOrder(out[i].Aspect) < aspectOrder(out[j].Aspect)
		}
		return out[i].Task < out[j].Task
	})
	return out
}

func aspectOrder(a domain.Aspect) int {
	for i, x := range domain.Aspects {
		if x == a {
			return i
		}
	}
	return len(domain.Aspects)
}

func sameKeys(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// --- per model ------------------------------------------------------------------

func modelMetrics(s *Snapshot) []ModelMetric {
	type mk struct{ provider, model string }
	ms := map[mk]*ModelMetric{}
	get := func(p domain.SemanticProposal) *ModelMetric {
		k := mk{p.Provider, model(p)}
		if ms[k] == nil {
			ms[k] = &ModelMetric{Provider: p.Provider, Model: model(p),
				FalsePositive: map[domain.Aspect]int{}, FalseNegative: map[domain.Aspect]int{},
				ByTask: map[domain.ProposalTask]ModelTaskMetric{}}
		}
		return ms[k]
	}
	bump := func(p domain.SemanticProposal, f func(*ModelMetric), g func(*ModelTaskMetric)) {
		mm := get(p)
		f(mm)
		t := mm.ByTask[p.Task]
		g(&t)
		mm.ByTask[p.Task] = t
	}
	props := map[string]domain.SemanticProposal{}
	for _, p := range s.Proposals {
		props[p.ID] = p
		bump(p, func(m *ModelMetric) {
			m.Proposals++
			if len(p.Undetermined) > 0 {
				m.Abstentions++
			}
		}, func(t *ModelTaskMetric) {
			t.Proposals++
			if len(p.Undetermined) > 0 {
				t.Abstentions++
			}
		})
	}
	items := map[string]domain.ReviewItem{}
	for _, it := range s.ReviewItems {
		items[it.ID] = it
	}
	// decisions ↔ proposals
	for _, d := range s.Decisions {
		it, ok := items[d.ReviewItemID]
		if !ok || d.Original == nil {
			continue
		}
		fin := d.Final()
		aspects := it.Aspects()
		for _, id := range it.Proposals {
			p, ok := props[id]
			if !ok {
				continue
			}
			asserted := 0
			matchOrig, matchFin := true, true
			for _, x := range aspects {
				if !p.Assertion.Has(x) {
					continue
				}
				asserted++
				dg := p.Assertion.AspectDigest(x)
				matchOrig = matchOrig && d.Original.AspectDigest(x) == dg
				matchFin = matchFin && fin != nil && fin.AspectDigest(x) == dg
			}
			if asserted == 0 {
				continue
			}
			var inc func(*ModelMetric)
			var incT func(*ModelTaskMetric)
			switch d.Action {
			case domain.ActionAccept:
				if matchFin {
					inc, incT = func(m *ModelMetric) { m.AcceptedAsIs++ }, func(t *ModelTaskMetric) { t.AcceptedAsIs++ }
				} else {
					inc, incT = func(m *ModelMetric) { m.Rejected++ }, func(t *ModelTaskMetric) { t.Rejected++ }
				}
			case domain.ActionCorrect:
				switch {
				case matchFin:
					inc, incT = func(m *ModelMetric) { m.AcceptedAsIs++ }, func(t *ModelTaskMetric) { t.AcceptedAsIs++ }
				case matchOrig:
					inc, incT = func(m *ModelMetric) { m.AcceptedCorrected++ }, func(t *ModelTaskMetric) { t.AcceptedCorrected++ }
				default:
					inc, incT = func(m *ModelMetric) { m.Rejected++ }, func(t *ModelTaskMetric) { t.Rejected++ }
				}
			case domain.ActionReject:
				if matchOrig {
					inc, incT = func(m *ModelMetric) { m.Rejected++ }, func(t *ModelTaskMetric) { t.Rejected++ }
				}
			case domain.ActionNeedMoreEvidence:
				if matchOrig {
					inc, incT = func(m *ModelMetric) { m.InsufficientEvidence++ }, func(t *ModelTaskMetric) { t.InsufficientEvidence++ }
				}
			}
			if inc != nil {
				bump(p, inc, incT)
			}
		}
	}
	// FP / FN and grounding vs the active facts of each candidate
	factsBy := map[string][]domain.VerifiedFact{}
	for _, f := range s.Facts {
		if f.Status != domain.FactActive {
			continue
		}
		for _, c := range f.Candidates {
			factsBy[c] = append(factsBy[c], f)
		}
	}
	cited := map[mk][2]int{} // grounded, total
	for _, p := range s.Proposals {
		fs := factsBy[p.CandidateID]
		if len(fs) == 0 {
			continue
		}
		mm := get(p)
		for _, x := range domain.TaskAspects(p.Task) {
			matched := false
			for _, f := range fs {
				matched = matched || (p.Assertion.Has(x) && p.Assertion.AspectDigest(x) == f.Assertion.AspectDigest(x))
			}
			switch {
			case p.Assertion.Has(x) && !matched:
				mm.FalsePositive[x]++
			case !p.Assertion.Has(x):
				mm.FalseNegative[x]++
			}
		}
		ev := map[domain.EvidenceID]bool{}
		for _, f := range fs {
			for _, e := range f.Evidence {
				ev[e.ID] = true
			}
		}
		k := mk{p.Provider, model(p)}
		c := cited[k]
		for _, id := range p.Citations {
			c[1]++
			if ev[id] {
				c[0]++
			}
		}
		cited[k] = c
	}
	var out []ModelMetric
	for k, mm := range ms {
		if c := cited[k]; c[1] > 0 {
			mm.GroundedCitations = float64(c[0]) / float64(c[1])
		}
		out = append(out, *mm)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Model < out[j].Model
	})
	return out
}

// --- review cost ------------------------------------------------------------------

func reviewMetrics(s *Snapshot) ReviewMetrics {
	r := ReviewMetrics{
		Items:            len(s.ReviewItems),
		ItemsPerRelease:  map[string]int{},
		ItemsPerProduct:  map[string]int{},
		ItemsPerQuestion: map[domain.QuestionType]int{},
	}
	for _, it := range s.ReviewItems {
		r.ItemsPerRelease[string(it.Product)+"@"+it.Release]++
		r.ItemsPerProduct[string(it.Product)]++
		r.ItemsPerQuestion[it.QuestionType]++
	}
	var indiv, batch []domain.ReviewDecision
	for _, d := range s.Decisions {
		if d.BatchID != "" {
			batch = append(batch, d)
		} else {
			indiv = append(indiv, d)
		}
	}
	r.Individual = decisionStats(indiv)
	r.Batch = decisionStats(batch)
	// auto-validation: facts whose every aspect is deterministic
	facts := s.Facts
	if len(facts) > 0 {
		auto := 0
		for _, f := range facts {
			if f.Level() == domain.VerifiedDeterministic {
				auto++
			}
		}
		r.AutoValidationRate = float64(auto) / float64(len(facts))
	}
	// repeat pattern: decisions that produced a fact whose pattern an earlier fact already had
	type pat struct {
		fam   domain.SubjectFamily
		ch    domain.ChangeKind
		kind  domain.ConsequenceKind
		class domain.ImpactClass
	}
	patOf := func(f domain.VerifiedFact) pat {
		return pat{f.Assertion.Subject.Family, f.Assertion.Change.Type, f.Assertion.Consequence.Kind, f.Assertion.Consequence.ExposedClass}
	}
	byID := map[string]domain.VerifiedFact{}
	for _, f := range facts {
		byID[f.ID] = f
	}
	total, repeat := 0, 0
	for _, d := range s.Decisions {
		if d.Action != domain.ActionAccept && d.Action != domain.ActionCorrect {
			continue
		}
		f := factOf(d, byID)
		if f == nil || f.Assertion.Subject == nil || f.Assertion.Change == nil || f.Assertion.Consequence == nil {
			continue
		}
		total++
		for _, g := range facts {
			if g.ID != f.ID && g.CreatedAt.Before(f.CreatedAt) && patOf(g) == patOf(*f) {
				repeat++
				break
			}
		}
	}
	if total > 0 {
		r.RepeatPatternRate = float64(repeat) / float64(total)
	}
	return r
}

func decisionStats(ds []domain.ReviewDecision) DecisionStats {
	st := DecisionStats{Decisions: len(ds)}
	batches := map[string]bool{}
	var durs []time.Duration
	for _, d := range ds {
		switch d.Action {
		case domain.ActionAccept:
			st.Accept++
		case domain.ActionCorrect:
			st.Correct++
		case domain.ActionReject:
			st.Reject++
		case domain.ActionNeedMoreEvidence:
			st.NeedMoreEvidence++
		case domain.ActionDefer:
			st.Defer++
		}
		if d.BatchID != "" {
			batches[d.BatchID] = true
		}
		if dur := d.Duration(); dur > 0 {
			durs = append(durs, dur)
		}
	}
	st.Batches = len(batches)
	st.MedianDuration = percentile(durs, 50)
	st.P90Duration = percentile(durs, 90)
	return st
}

// percentile is the nearest-rank percentile (0 for no samples).
func percentile(ds []time.Duration, p int) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), ds...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	rank := (p*len(sorted) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

// --- facts --------------------------------------------------------------------------

func factMetrics(s *Snapshot) FactMetrics {
	fm := FactMetrics{ByLevel: map[domain.VerificationLevel]int{}, ByFamily: map[domain.SubjectFamily]int{}}
	anchors := 0
	for _, f := range s.Facts {
		switch f.Status {
		case domain.FactActive:
			fm.Active++
			fm.ByLevel[f.Level()]++
			if f.Assertion.Subject != nil {
				fm.ByFamily[f.Assertion.Subject.Family]++
			}
			anchors += len(f.Anchors)
		case domain.FactRetracted:
			fm.Retracted++
		}
	}
	if fm.Active > 0 {
		fm.AnchorsPerFact = float64(anchors) / float64(fm.Active)
	}
	auditAutoApproved(s, &fm)
	return fm
}

// auditAutoApproved measures auto-approval against human review (R19): the
// auto-approved facts, how many a reviewer was sampled onto (a fact-review
// item with a human decision), and how often the verdict agreed (accept
// agrees; reject or correct disagrees). Per subject family as well.
func auditAutoApproved(s *Snapshot, fm *FactMetrics) {
	items := map[string]domain.ReviewItem{}
	for _, it := range s.ReviewItems {
		items[it.ID] = it
	}
	verdict := map[string]domain.DecisionAction{} // fact id → latest human verdict
	when := map[string]int64{}
	for _, d := range s.Decisions {
		it, ok := items[d.ReviewItemID]
		if !ok || d.ReviewerKind != domain.ReviewerHuman || it.Proposed.Validate(true) != nil {
			continue
		}
		if d.Action != domain.ActionAccept && d.Action != domain.ActionCorrect && d.Action != domain.ActionReject {
			continue
		}
		id := domain.VerifiedFactID(it.Product, it.Release, it.Proposed)
		if t := d.DecidedAt.UnixNano(); t >= when[id] {
			verdict[id], when[id] = d.Action, t
		}
	}
	famTotal, famAgree := map[domain.SubjectFamily]int{}, map[domain.SubjectFamily]int{}
	scopeTotal, scopeAgree := map[domain.ConsensusScope]int{}, map[domain.ConsensusScope]int{}
	agree, caAgree := 0, 0
	for _, f := range s.Facts {
		if f.ConsensusAction {
			fm.ConsensusAction++
		}
		if !f.AutoApproved {
			continue
		}
		fm.AutoApproved++
		v, ok := verdict[f.ID]
		if !ok {
			continue
		}
		if f.ConsensusAction {
			fm.ConsensusActionAudited++
			if v == domain.ActionAccept {
				caAgree++
			}
		}
		scopes := map[domain.ConsensusScope]bool{}
		for _, av := range f.Verification {
			if av.Level == domain.VerifiedConsensus && av.Consensus != "" {
				scopes[av.Consensus] = true
			}
		}
		for sc := range scopes {
			scopeTotal[sc]++
			if v == domain.ActionAccept {
				scopeAgree[sc]++
			}
		}
		fm.AutoApprovedAudited++
		fam := domain.SubjectFamily("")
		if f.Assertion.Subject != nil {
			fam = f.Assertion.Subject.Family
		}
		famTotal[fam]++
		if v == domain.ActionAccept {
			agree++
			famAgree[fam]++
		}
	}
	if fm.ConsensusActionAudited > 0 {
		fm.ConsensusActionAgreement = float64(caAgree) / float64(fm.ConsensusActionAudited)
	}
	if len(scopeTotal) > 0 {
		fm.ConsensusAgreementByScope = map[domain.ConsensusScope]float64{}
		for sc, n := range scopeTotal {
			fm.ConsensusAgreementByScope[sc] = float64(scopeAgree[sc]) / float64(n)
		}
	}
	if fm.AutoApprovedAudited > 0 {
		fm.AutoApprovalAgreement = float64(agree) / float64(fm.AutoApprovedAudited)
		fm.AutoApprovalAgreementBy = map[domain.SubjectFamily]float64{}
		for fam, n := range famTotal {
			fm.AutoApprovalAgreementBy[fam] = float64(famAgree[fam]) / float64(n)
		}
	}
}
