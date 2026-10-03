package proxyreview

import (
	"fmt"
	"io"
	"sort"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// Agreement compares the human decisions of the real store with the proxy's
// shadow decisions on the same items (brief step 5): which items both decided,
// whether they took the same action, and, where both verified the item
// (accept/correct), whether they verified the same value per aspect.
type Agreement struct {
	Pairs       int                                      `json:"pairs"`
	HumanOnly   int                                      `json:"humanOnly"`  // human decided, shadow did not
	ShadowOnly  int                                      `json:"shadowOnly"` // shadow decided, human not (yet)
	SameAction  int                                      `json:"sameAction"`
	SameOutcome int                                      `json:"sameOutcome"` // coarse: verify | reject | open
	Confusion   map[string]map[string]int                `json:"confusion"`   // human action -> proxy action
	ByQuestion  map[domain.QuestionType]*AgreementCounts `json:"byQuestion"`
	ByAspect    map[domain.Aspect]*AgreementCounts       `json:"byAspect"`
	Disagreeing []AgreementPair                          `json:"disagreeing,omitempty"`
}

// AgreementCounts is n and agreements in one slice.
type AgreementCounts struct {
	N, Agree int
}

// AgreementPair is one item both decided, differently.
type AgreementPair struct {
	ItemID       string              `json:"itemId"`
	QuestionType domain.QuestionType `json:"questionType"`
	Human, Proxy domain.DecisionAction
}

// coarse groups actions: verify (accept/correct), reject, open (need-more-evidence/defer).
func coarse(a domain.DecisionAction) string {
	switch a {
	case domain.ActionAccept, domain.ActionCorrect:
		return "verify"
	case domain.ActionReject:
		return "reject"
	}
	return "open"
}

func latest(snap *knowledge.Snapshot, kind domain.ReviewerKind) map[string]domain.ReviewDecision {
	out := map[string]domain.ReviewDecision{}
	for _, d := range snap.Decisions {
		if d.ReviewerKind != kind {
			continue
		}
		if prev, ok := out[d.ReviewItemID]; !ok || d.DecidedAt.After(prev.DecidedAt) {
			out[d.ReviewItemID] = d
		}
	}
	return out
}

// CompareShadow computes the agreement of human decisions in real with proxy
// decisions in shadow. Only shadow-pass decisions count on the proxy side: a
// proxy decision that the real store also holds was copied into the shadow
// (run-1/run-2), so it is ignored.
func CompareShadow(real, shadow *knowledge.Snapshot) *Agreement {
	a := &Agreement{Confusion: map[string]map[string]int{}, ByQuestion: map[domain.QuestionType]*AgreementCounts{},
		ByAspect: map[domain.Aspect]*AgreementCounts{}}
	human := latest(real, domain.ReviewerHuman)
	realProxy := latest(real, domain.ReviewerProxy)
	shadowProxy := latest(shadow, domain.ReviewerProxy)
	items := map[string]domain.ReviewItem{}
	for _, it := range shadow.ReviewItems {
		items[it.ID] = it
	}
	for id := range shadowProxy {
		if _, inReal := realProxy[id]; inReal {
			delete(shadowProxy, id) // a run-1/run-2 proxy decision copied into the shadow, not a shadow decision
		}
	}
	ids := map[string]bool{}
	for id := range human {
		ids[id] = true
	}
	for id := range shadowProxy {
		ids[id] = true
	}
	sorted := make([]string, 0, len(ids))
	for id := range ids {
		sorted = append(sorted, id)
	}
	sort.Strings(sorted)
	for _, id := range sorted {
		h, hok := human[id]
		p, pok := shadowProxy[id]
		switch {
		case hok && !pok:
			a.HumanOnly++
			continue
		case pok && !hok:
			a.ShadowOnly++
			continue
		}
		a.Pairs++
		q := items[id].QuestionType
		if a.ByQuestion[q] == nil {
			a.ByQuestion[q] = &AgreementCounts{}
		}
		a.ByQuestion[q].N++
		if a.Confusion[string(h.Action)] == nil {
			a.Confusion[string(h.Action)] = map[string]int{}
		}
		a.Confusion[string(h.Action)][string(p.Action)]++
		if h.Action == p.Action {
			a.SameAction++
			a.ByQuestion[q].Agree++
		} else {
			a.Disagreeing = append(a.Disagreeing, AgreementPair{ItemID: id, QuestionType: q, Human: h.Action, Proxy: p.Action})
		}
		if coarse(h.Action) == coarse(p.Action) {
			a.SameOutcome++
		}
		hf, pf := h.Final(), p.Final()
		if coarse(h.Action) == "verify" && coarse(p.Action) == "verify" && hf != nil && pf != nil {
			for _, x := range domain.QuestionAspects(q) {
				if a.ByAspect[x] == nil {
					a.ByAspect[x] = &AgreementCounts{}
				}
				a.ByAspect[x].N++
				if hf.AspectDigest(x) == pf.AspectDigest(x) {
					a.ByAspect[x].Agree++
				}
			}
		}
	}
	return a
}

// WriteAgreement renders the agreement as text.
func WriteAgreement(w io.Writer, a *Agreement) {
	fmt.Fprintf(w, "Proxy (shadow) vs human: %d items decided by both; %d human-only; %d shadow-only (human not yet)\n",
		a.Pairs, a.HumanOnly, a.ShadowOnly)
	if a.Pairs == 0 {
		return
	}
	fmt.Fprintf(w, "same action %d/%d (%.2f) · same outcome (verify|reject|open) %d/%d (%.2f)\n",
		a.SameAction, a.Pairs, ratio(a.SameAction, a.Pairs), a.SameOutcome, a.Pairs, ratio(a.SameOutcome, a.Pairs))
	fmt.Fprintln(w, "\nBy question type (same action):")
	for _, q := range sortedKeys(a.ByQuestion) {
		c := a.ByQuestion[q]
		fmt.Fprintf(w, "  %-22s %d/%d (%.2f)\n", q, c.Agree, c.N, ratio(c.Agree, c.N))
	}
	fmt.Fprintln(w, "\nBy aspect, where both verified (same final value):")
	for _, x := range sortedKeys(a.ByAspect) {
		c := a.ByAspect[x]
		fmt.Fprintf(w, "  %-14s %d/%d (%.2f)\n", x, c.Agree, c.N, ratio(c.Agree, c.N))
	}
	fmt.Fprintln(w, "\nConfusion (human → proxy):")
	for _, h := range sortedKeys(a.Confusion) {
		fmt.Fprintf(w, "  %-20s %s\n", h, countLine(a.Confusion[h]))
	}
}
