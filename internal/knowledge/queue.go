package knowledge

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// reviewQueue is the default Queue over a Store.
type reviewQueue struct {
	s   Store
	now func() time.Time
}

// NewQueue returns the review queue over s. now stamps items the queue
// creates itself (fact re-reviews); decisions carry their own times.
func NewQueue(s Store, now func() time.Time) Queue {
	if now == nil {
		now = time.Now
	}
	return &reviewQueue{s: s, now: now}
}

var _ Queue = (*reviewQueue)(nil)

var priorityRank = map[domain.ReviewPriority]int{domain.PriorityHigh: 0, domain.PriorityNormal: 1, domain.PriorityLow: 2}

func hasSignal(it domain.ReviewItem, s domain.RoutingSignal) bool {
	for _, x := range it.Routing.Signals {
		if x == s {
			return true
		}
	}
	return false
}

// Inbox lists the items matching f with the G7 counters. The counters cover
// the items matching every filter except Status, so they stay meaningful when
// the list is narrowed to one status.
func (q *reviewQueue) Inbox(ctx context.Context, f InboxFilter) (*Inbox, error) {
	snap, err := q.s.Load(ctx, Query{Product: f.Product})
	if err != nil {
		return nil, err
	}
	cands := map[string]domain.SemanticCandidate{}
	for _, c := range snap.Candidates {
		cands[c.ID] = c
	}
	props := map[string]domain.SemanticProposal{}
	for _, p := range snap.Proposals {
		props[p.ID] = p
	}
	decs := map[string][]domain.ReviewDecision{}
	for _, d := range snap.Decisions {
		decs[d.ReviewItemID] = append(decs[d.ReviewItemID], d)
	}
	inbox := &Inbox{}
	var rows []InboxRow
	for _, it := range snap.ReviewItems {
		c := cands[it.CandidateID]
		if f.Release != "" && it.Release != f.Release {
			continue
		}
		if f.QuestionType != "" && it.QuestionType != f.QuestionType {
			continue
		}
		if f.SubjectType != "" && (it.Proposed.Subject == nil || it.Proposed.Subject.Family != f.SubjectType) {
			continue
		}
		if f.Severity != "" && (it.Proposed.Consequence == nil || it.Proposed.Consequence.Severity != f.Severity) {
			continue
		}
		var models, calls []string
		confOK := f.Confidence == ""
		modelOK := f.Model == ""
		for _, id := range it.Proposals {
			p, ok := props[id]
			if !ok {
				continue
			}
			models = appendUnique(models, p.Provenance.Model)
			calls = appendUnique(calls, p.Provenance.CallID)
			confOK = confOK || p.Provenance.Confidence == f.Confidence
			modelOK = modelOK || p.Provenance.Model == f.Model
		}
		if !confOK || !modelOK {
			continue
		}
		disagree := hasSignal(it, domain.SignalModelsDisagree)
		if f.Disagreement != nil && *f.Disagreement != disagree {
			continue
		}
		if f.Source != "" {
			hit := false
			for _, e := range c.Evidence {
				hit = hit || strings.Contains(e.URI, f.Source)
			}
			if !hit {
				continue
			}
		}
		if f.Reviewer != "" {
			hit := false
			for _, d := range decs[it.ID] {
				hit = hit || d.Reviewer == f.Reviewer
			}
			if !hit {
				continue
			}
		}
		// counters (status-independent)
		switch it.Status {
		case domain.ReviewPending:
			inbox.Counts.Pending++
			if disagree {
				inbox.Counts.ModelDisagreement++
			}
			switch it.QuestionType {
			case domain.QuestionSemanticMapping, domain.QuestionRelationship:
				inbox.Counts.NeedsSemanticMapping++
			case domain.QuestionApplicability:
				inbox.Counts.ApplicabilityQuestions++
			}
		case domain.ReviewNeedsEvidence:
			inbox.Counts.NeedsMoreEvidence++
		case domain.ReviewDeferred:
			inbox.Counts.Deferred++
		}
		if len(f.Status) > 0 {
			ok := false
			for _, st := range f.Status {
				ok = ok || st == it.Status
			}
			if !ok {
				continue
			}
		}
		sort.Strings(models)
		rows = append(rows, InboxRow{Item: it, Title: c.Title, Disagreement: disagree, Models: models, Calls: len(calls)})
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i].Item, rows[j].Item
		if ra, rb := priorityRank[a.Routing.Priority], priorityRank[b.Routing.Priority]; ra != rb {
			return ra < rb
		}
		return lessTimeID(a.CreatedAt.UnixNano(), a.ID, b.CreatedAt.UnixNano(), b.ID)
	})
	inbox.Matches = len(rows)
	if f.Limit > 0 && len(rows) > f.Limit {
		rows = rows[:f.Limit]
	}
	inbox.Items = rows
	return inbox, nil
}

// Item assembles everything the review page shows (G8), including earlier
// decisions and facts about the same subject key or the same candidate members.
func (q *reviewQueue) Item(ctx context.Context, id string) (*ReviewContext, error) {
	snap, err := q.s.Load(ctx, Query{})
	if err != nil {
		return nil, err
	}
	return AssembleContext(snap, id)
}

// AssembleContext builds the ReviewContext of one item from a snapshot.
func AssembleContext(snap *Snapshot, id string) (*ReviewContext, error) {
	var item *domain.ReviewItem
	for i := range snap.ReviewItems {
		if snap.ReviewItems[i].ID == id {
			item = &snap.ReviewItems[i]
		}
	}
	if item == nil {
		return nil, fmt.Errorf("%w: review item %s", ErrNotFound, id)
	}
	rc := &ReviewContext{Item: *item, Environment: item.Context}
	var cand *domain.SemanticCandidate
	for i := range snap.Candidates {
		if snap.Candidates[i].ID == item.CandidateID {
			cand = &snap.Candidates[i]
		}
	}
	if cand == nil {
		return nil, fmt.Errorf("%w: candidate %s of item %s", ErrOrphan, item.CandidateID, id)
	}
	rc.Candidate = *cand
	wantP := map[string]bool{}
	for _, p := range item.Proposals {
		wantP[p] = true
	}
	wantV := map[string]bool{}
	for _, v := range item.Validations {
		wantV[v] = true
	}
	for _, p := range snap.Proposals {
		if wantP[p.ID] {
			rc.Proposals = append(rc.Proposals, p)
		}
	}
	for _, v := range snap.Validations {
		if wantV[v.ID] {
			rc.Validations = append(rc.Validations, v)
		}
	}
	rc.Agreement = Agreements(rc.Proposals)

	// related: same subject key, or a candidate sharing a member
	subjects := map[string]bool{}
	addSubject := func(a domain.SemanticAssertion) {
		if a.Subject != nil {
			subjects[a.Subject.Key()] = true
		}
	}
	addSubject(item.Proposed)
	for _, p := range rc.Proposals {
		addSubject(p.Assertion)
	}
	members := map[string]bool{}
	for _, m := range cand.Members {
		members["c:"+m.ChangeID] = true
		if m.Anchor != nil {
			for _, k := range m.Anchor.StatementKeys {
				members["s:"+k] = true
			}
		}
	}
	relatedCand := map[string]bool{cand.ID: true}
	for _, c := range snap.Candidates {
		for _, m := range c.Members {
			hit := members["c:"+m.ChangeID]
			if m.Anchor != nil {
				for _, k := range m.Anchor.StatementKeys {
					hit = hit || members["s:"+k]
				}
			}
			if hit {
				relatedCand[c.ID] = true
			}
		}
	}
	relatedItem := map[string]bool{}
	for _, it := range snap.ReviewItems {
		if relatedCand[it.CandidateID] || (it.Proposed.Subject != nil && subjects[it.Proposed.Subject.Key()]) {
			relatedItem[it.ID] = true
		}
	}
	for _, d := range snap.Decisions {
		if relatedItem[d.ReviewItemID] {
			rc.RelatedDecisions = append(rc.RelatedDecisions, d)
		}
	}
	for _, f := range snap.Facts {
		hit := f.Assertion.Subject != nil && subjects[f.Assertion.Subject.Key()]
		for _, c := range f.Candidates {
			hit = hit || relatedCand[c]
		}
		if hit {
			rc.RelatedFacts = append(rc.RelatedFacts, f)
		}
	}
	switch {
	case item.Proposed.Consequence != nil:
		rc.SuggestedExposedClass = item.Proposed.Consequence.Kind.ExposedClass()
	default:
		for _, p := range rc.Proposals {
			if p.Assertion.Consequence != nil {
				rc.SuggestedExposedClass = p.Assertion.Consequence.Kind.ExposedClass()
				break
			}
		}
	}
	return rc, nil
}

// Decide records decisions (one per item; several for one bulk action),
// updates item status and mints or updates facts. Every decision of the call
// is validated, and its consequences computed against an in-memory view,
// before anything is written.
func (q *reviewQueue) Decide(ctx context.Context, ds []domain.ReviewDecision) ([]DecisionOutcome, error) {
	snap, err := q.s.Load(ctx, Query{})
	if err != nil {
		return nil, err
	}
	if err := validateDecisions(snap, ds); err != nil {
		return nil, err
	}
	known := map[string]domain.ReviewDecision{}
	for _, d := range snap.Decisions {
		known[d.ID] = d
	}
	var (
		outcomes []DecisionOutcome
		decWrite []domain.ReviewDecision
		newItems []domain.ReviewItem
		facts    []domain.VerifiedFact
		touched  []domain.VerifiedFact
		status   = map[string]domain.ReviewStatus{}
	)
	// work on a copy so each decision sees the earlier ones of the call
	work := *snap
	work.ReviewItems = append([]domain.ReviewItem(nil), snap.ReviewItems...)
	work.Decisions = append([]domain.ReviewDecision(nil), snap.Decisions...)
	work.Facts = append([]domain.VerifiedFact(nil), snap.Facts...)
	for _, d := range ds {
		if prev, ok := known[d.ID]; ok { // idempotent retry
			d = prev
		}
		m, err := FactFromDecision(&work, d)
		if err != nil {
			return nil, err
		}
		if m.Fact != nil && (d.Action == domain.ActionAccept || d.Action == domain.ActionCorrect) && d.ResultingFact == "" && known[d.ID].ID == "" {
			d.ResultingFact = m.Fact.ID
		}
		out := DecisionOutcome{Decision: d, Fact: m.Fact, OpenAspects: m.Open, FollowUps: m.FollowUps}
		if _, ok := known[d.ID]; !ok {
			decWrite = append(decWrite, d)
			work.Decisions = append(work.Decisions, d)
		}
		for i := range work.ReviewItems {
			if work.ReviewItems[i].ID == d.ReviewItemID {
				work.ReviewItems[i].Status = statusFor(d.Action)
				status[d.ReviewItemID] = work.ReviewItems[i].Status
			}
		}
		for _, it := range m.FollowUps {
			newItems = append(newItems, it)
			work.ReviewItems = append(work.ReviewItems, it)
		}
		for _, t := range m.Touched {
			touched = append(touched, t)
			work.Facts = replaceFact(work.Facts, t)
		}
		if m.Fact != nil {
			facts = append(facts, *m.Fact)
			work.Facts = replaceFact(work.Facts, *m.Fact)
		}
		outcomes = append(outcomes, out)
	}
	// dry-run the fact basis against the in-memory view, so a bad fact fails
	// the whole call before any write
	vals := map[string]domain.ValidationResult{}
	for _, v := range work.Validations {
		vals[v.ID] = v
	}
	decs := map[string]domain.ReviewDecision{}
	for _, d := range work.Decisions {
		decs[d.ID] = d
	}
	its := map[string]domain.ReviewItem{}
	for _, it := range work.ReviewItems {
		its[it.ID] = it
	}
	pmap := map[string]domain.SemanticProposal{}
	for _, p := range work.Proposals {
		pmap[p.ID] = p
	}
	for _, f := range append(append([]domain.VerifiedFact(nil), facts...), touched...) {
		if err := f.Validate(); err != nil {
			return nil, err
		}
		if err := domain.ValidateFactRecords(f, domain.FactRecords{Validations: vals, Decisions: decs, Items: its, Proposals: pmap}); err != nil {
			return nil, err
		}
	}
	for _, d := range decWrite {
		if err := q.put(ctx, d); err != nil {
			return nil, err
		}
	}
	for _, it := range newItems {
		if err := q.put(ctx, it); err != nil {
			return nil, err
		}
	}
	for _, f := range facts {
		if err := q.put(ctx, f); err != nil {
			return nil, err
		}
	}
	for _, f := range touched {
		if err := q.put(ctx, f); err != nil {
			return nil, err
		}
	}
	for _, it := range work.ReviewItems {
		if st, ok := status[it.ID]; ok {
			it.Status = st
			if err := q.put(ctx, it); err != nil {
				return nil, err
			}
		}
	}
	return outcomes, nil
}

func (q *reviewQueue) put(ctx context.Context, entity any) error {
	rec, err := domain.NewRecord(entity)
	if err != nil {
		return err
	}
	return q.s.Put(ctx, rec)
}

func statusFor(a domain.DecisionAction) domain.ReviewStatus {
	switch a {
	case domain.ActionNeedMoreEvidence:
		return domain.ReviewNeedsEvidence
	case domain.ActionDefer:
		return domain.ReviewDeferred
	}
	return domain.ReviewDecided
}

func replaceFact(fs []domain.VerifiedFact, f domain.VerifiedFact) []domain.VerifiedFact {
	for i := range fs {
		if fs[i].ID == f.ID {
			fs[i] = f
			return fs
		}
	}
	return append(fs, f)
}

// OpenFactReview creates (idempotently) a review item that re-asks whether an
// existing fact is still right: a reject retracts it, a correction mints a
// superseding fact (FactFromDecision). The item carries the full assertion.
func OpenFactReview(ctx context.Context, s Store, factID string, now time.Time) (*domain.ReviewItem, error) {
	r, err := s.Get(ctx, factID)
	if err != nil {
		return nil, err
	}
	if r.Fact == nil {
		return nil, fmt.Errorf("knowledge: %s is not a fact", factID)
	}
	f := r.Fact
	if f.Status != domain.FactActive {
		return nil, fmt.Errorf("knowledge: fact %s is %s", f.ID, f.Status)
	}
	cid := f.Candidates[0]
	it := domain.ReviewItem{
		ID:           domain.ReviewItemID(cid, domain.QuestionClassification, f.Assertion),
		CandidateID:  cid,
		Product:      f.Product,
		Release:      f.Release,
		QuestionType: domain.QuestionClassification,
		Question:     fmt.Sprintf("Is fact %s (%s) still correct? Reject to retract it; correct to supersede it.", f.ID, f.Assertion.Statement),
		Proposed:     f.Assertion,
		Routing:      domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityNormal},
		Status:       domain.ReviewPending,
		CreatedAt:    now,
	}
	rec, err := domain.NewRecord(it)
	if err != nil {
		return nil, err
	}
	if err := s.Put(ctx, rec); err != nil {
		return nil, err
	}
	return &it, nil
}
