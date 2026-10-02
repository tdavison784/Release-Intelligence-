package eval

// Integrity of the checked-in dataset (../../eval/cases): every entry loads
// and validates, and the dataset keeps the shape the plan promises (>= 6
// products, >= 2 environments). Offline: pure file reading.

import (
	"testing"
)

const datasetRoot = "../../eval"

func TestDatasetLoads(t *testing.T) {
	r := &Runner{CasesDir: datasetRoot}
	cases, err := r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 9 {
		t.Errorf("dataset has %d cases, want >= 9", len(cases))
	}
	products := map[string]bool{}
	envs := 0
	for _, c := range cases {
		if c.ID == "" || c.Product == "" || c.ResearchedAt == "" {
			t.Errorf("case %s: id/product/researchedAt missing", c.ID)
		}
		if len(c.Sources) == 0 {
			t.Errorf("case %s: no sources cited", c.ID)
		}
		for _, e := range c.Expected {
			if len(e.Evidence) == 0 {
				t.Errorf("case %s expected %s: no evidence citation (expectations must be auditable)", c.ID, e.ID)
			}
		}
		products[c.Product] = true
		if c.Environment != nil {
			envs++
			if c.Environment.Description == "" {
				t.Errorf("case %s: environment without description", c.ID)
			}
		}
	}
	if len(products) < 6 {
		t.Errorf("dataset covers %d products, want >= 6", len(products))
	}
	if envs < 2 {
		t.Errorf("dataset has %d environment cases, want >= 2", envs)
	}
}

func TestDatasetEntrySelectionLoads(t *testing.T) {
	r := &Runner{CasesDir: datasetRoot}
	if _, err := r.Load([]string{"cert-manager-1.17-1.18"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Load([]string{"no-such-case"}); err == nil {
		t.Error("unknown entry must fail")
	}
}

// TestDatasetLabelConsistency keeps the G22 labels coherent: in a case with
// an environment, a linked item's classification is the class for that
// environment (the scorer compares the two), an undecided link's item is
// classified unknown, and an affected link's relevance is the class one of the
// item's consequences gives an exposed environment (informational may come
// from an overlap instead).
func TestDatasetLabelConsistency(t *testing.T) {
	r := &Runner{CasesDir: datasetRoot}
	cases, err := r.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	relClass := map[string]string{
		RelevanceActionRequired: ClassActionRequired, RelevanceReview: ClassReviewRequired,
		RelevanceInformational: ClassInformational, RelevanceNotAffected: ClassNotAffected,
	}
	for _, c := range cases {
		if c.Environment == nil {
			continue
		}
		items := map[string]Expected{}
		for _, e := range c.Expected {
			items[e.ID] = e
		}
		for _, l := range c.Environment.ExpectedImpact {
			e, want := items[l.Expected], relClass[l.Relevance]
			if c.TransferOf == "" && e.Classification != "" && e.Classification != want {
				t.Errorf("%s %s: classification %s, but the link says %s", c.ID, e.ID, e.Classification, l.Relevance)
			}
			if len(e.Semantics) == 0 || l.Relevance == RelevanceNotAffected || (l.Relevance == RelevanceInformational && l.Overlap != nil) {
				continue
			}
			ok := false
			for _, s := range e.Semantics {
				ok = ok || string(s.Consequence.ExposedClass) == want
			}
			if !ok {
				t.Errorf("%s %s: relevance %s matches no consequence class of the item's semantics", c.ID, e.ID, l.Relevance)
			}
		}
		for _, u := range c.Environment.UndecidedImpact {
			if e := items[u.Expected]; c.TransferOf == "" && e.Classification != "" && e.Classification != ClassUnknown {
				t.Errorf("%s %s: undecided link, but classification %s (want unknown)", c.ID, e.ID, e.Classification)
			}
		}
	}
}
