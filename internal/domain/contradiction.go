package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Contradicting knowledge (lane backdoor, D-0029). Two active facts of one
// product, release and subject must agree: every route that mints or changes
// a fact names the others (internal/knowledge), the loader warns about a pair
// that does not agree, and the knowledge join never lets either clear a
// change (internal/impact).

// SameFactScope reports whether two facts state one subject of one product
// at one release: the scope in which they must agree.
func SameFactScope(a, b VerifiedFact) bool {
	return a.Product == b.Product && a.Release == b.Release && a.Assertion.Subject != nil && b.Assertion.Subject != nil &&
		a.Assertion.Subject.Key() == b.Assertion.Subject.Key()
}

// FactIncompatibility says why two facts of one scope cannot both hold (""
// when they can). They are compatible when they state
//   - the same change: the same kind, and each stated before, after and
//     replacement equal (a value one fact leaves unstated is no
//     disagreement);
//   - the same exposure condition: who is affected (the overlap only labels
//     unexposed objects as shielded, so it may differ);
//   - the same consequence kind, and so the same class (severity and prose
//     may differ).
//
// Anything else is a contradiction. Conditions are compared as written, so
// an equivalent condition written differently counts as a contradiction
// (fail closed).
func FactIncompatibility(a, b VerifiedFact) string {
	var why []string
	if ca, cb := a.Assertion.Change, b.Assertion.Change; ca != nil && cb != nil {
		switch {
		case ca.Type != cb.Type:
			why = append(why, fmt.Sprintf("change %s vs %s", ca.Type, cb.Type))
		case stated(ca.Before, cb.Before):
			why = append(why, fmt.Sprintf("change before %s vs %s", *ca.Before, *cb.Before))
		case stated(ca.After, cb.After):
			why = append(why, fmt.Sprintf("change after %s vs %s", *ca.After, *cb.After))
		case ca.ReplacedBy != nil && cb.ReplacedBy != nil && ca.ReplacedBy.Key() != cb.ReplacedBy.Key():
			why = append(why, fmt.Sprintf("replaced by %s vs %s", ca.ReplacedBy.Key(), cb.ReplacedBy.Key()))
		}
	}
	if aa, ab := a.Assertion.Applicability, b.Assertion.Applicability; aa != nil && ab != nil && conditionJSON(aa.Exposure) != conditionJSON(ab.Exposure) {
		why = append(why, "exposure condition differs")
	}
	if ka, kb := a.Assertion.Consequence, b.Assertion.Consequence; ka != nil && kb != nil && (ka.Kind != kb.Kind || ka.ExposedClass != kb.ExposedClass) {
		why = append(why, fmt.Sprintf("consequence %s (%s) vs %s (%s)", ka.Kind, ka.ExposedClass, kb.Kind, kb.ExposedClass))
	}
	return strings.Join(why, "; ")
}

// stated reports whether both values are stated and differ.
func stated(a, b *string) bool { return a != nil && b != nil && *a != *b }

func conditionJSON(c Condition) string {
	b, err := json.Marshal(c)
	if err != nil {
		panic(err) // plain data; cannot fail
	}
	return string(b)
}

// FactContradiction is a pair of active facts of one scope that cannot both
// hold. A sorts before B.
type FactContradiction struct {
	A, B    string
	Product ProductID
	Release string
	Subject string
	Why     string
}

// String names both facts, the scope and the disagreement.
func (c FactContradiction) String() string {
	return fmt.Sprintf("active facts %s and %s contradict each other on %s at %s %s (%s)", c.A, c.B, c.Subject, c.Product, c.Release, c.Why)
}

// Contradictions lists every pair of active facts of one scope that cannot
// both hold, sorted.
func Contradictions(facts []VerifiedFact) []FactContradiction {
	groups := map[string][]VerifiedFact{}
	for _, f := range facts {
		if f.Status != FactActive || f.Assertion.Subject == nil {
			continue
		}
		k := string(f.Product) + "\x00" + f.Release + "\x00" + f.Assertion.Subject.Key()
		groups[k] = append(groups[k], f)
	}
	var out []FactContradiction
	for _, fs := range groups {
		sort.Slice(fs, func(i, j int) bool { return fs[i].ID < fs[j].ID })
		for i := range fs {
			for j := i + 1; j < len(fs); j++ {
				if fs[i].ID == fs[j].ID {
					continue
				}
				if why := FactIncompatibility(fs[i], fs[j]); why != "" {
					out = append(out, FactContradiction{A: fs[i].ID, B: fs[j].ID, Product: fs[i].Product, Release: fs[i].Release,
						Subject: fs[i].Assertion.Subject.Key(), Why: why})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].A != out[j].A {
			return out[i].A < out[j].A
		}
		return out[i].B < out[j].B
	})
	return out
}
