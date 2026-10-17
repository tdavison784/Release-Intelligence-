package reviewui

import (
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// Whole-fact review (fact-review items): the page shows every aspect of the
// fact with its current verification level and asks "Is this whole fact
// correct?". The aspects the page displays are exactly the aspects the
// decision records (ReviewDecision.Aspects), so an accept raises what the
// reviewer saw and nothing else; a validator-confirmed aspect stays
// deterministic, an aspect a human already verified keeps that decision.

// wholeFactAspects are the aspects a fact-review item's page displays (and its
// accept/correct verifies): every aspect the question covers that the
// proposed assertion states. nil for any other question type.
func wholeFactAspects(it domain.ReviewItem) []domain.Aspect {
	if it.QuestionType != domain.QuestionFactReview {
		return nil
	}
	var out []domain.Aspect
	for _, x := range it.Aspects() {
		if it.Proposed.Has(x) {
			out = append(out, x)
		}
	}
	return out
}

type wholeFactRow struct {
	Aspect, Word, Text string
	Level, Effect      string
	Trusted            bool
}

type wholeFactView struct {
	FactID, FactLevel string
	Rows              []wholeFactRow
}

// newWholeFactView builds the whole-fact section of a fact-review item (nil
// for other items). The fact is found among the related facts by identity.
func newWholeFactView(rc *knowledge.ReviewContext) *wholeFactView {
	shown := wholeFactAspects(rc.Item)
	if shown == nil {
		return nil
	}
	id := domain.VerifiedFactID(rc.Item.Product, rc.Item.Release, rc.Item.Proposed)
	v := &wholeFactView{FactID: id}
	var fact *domain.VerifiedFact
	for i := range rc.RelatedFacts {
		if rc.RelatedFacts[i].ID == id {
			fact = &rc.RelatedFacts[i]
		}
	}
	if fact != nil {
		v.FactLevel = string(fact.Level())
	}
	for _, x := range shown {
		r := wholeFactRow{Aspect: string(x), Word: aspectWord(x), Text: describeAspect(rc.Item.Proposed, x), Level: "not recorded"}
		if fact != nil {
			if l := fact.AspectLevel(x); l != "" {
				r.Level = string(l)
			}
			r.Trusted = fact.AspectLevel(x).Trusted()
		}
		if r.Trusted {
			r.Effect = "stays " + r.Level
		} else {
			r.Effect = "becomes human"
		}
		v.Rows = append(v.Rows, r)
	}
	return v
}

// groupHead introduces the rows of one pin-source fact in the default view.
type groupHead struct {
	Key, Product, Release, Statement string
	IsFact                           bool
	Levels, Left                     string
}

func newGroupHead(g PinGroup) *groupHead {
	return &groupHead{Key: g.Key, Product: string(g.Product), Release: g.Release, Statement: g.Statement,
		IsFact: g.Fact != nil, Levels: g.LevelLine(), Left: g.LeftLine()}
}
