package impactenrich

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// byType indexes candidates by type.
func byType(cs []Candidate) map[CandidateType][]Candidate {
	m := map[CandidateType][]Candidate{}
	for _, c := range cs {
		m[c.Type] = append(m[c.Type], c)
	}
	return m
}

func TestCandidatesApplicabilitySelection(t *testing.T) {
	f := newFixture(t)
	cs := Candidates(f.report, f.edge, CandidateOptions{})
	m := byType(cs)

	// note-derived, non-routine unknowns are candidates, one each, with the
	// finding id they are about
	if len(m[CandApplicability]) != 2 {
		t.Fatalf("applicability candidates = %d, want 2 (the two note-derived unknowns)", len(m[CandApplicability]))
	}
	for _, c := range m[CandApplicability] {
		if len(c.Findings) != 1 {
			t.Fatalf("applicability candidate %s lists %d findings, want 1", c.ID, len(c.Findings))
		}
		fd := findingByID(t, f, c.Findings[0])
		if fd.Classification != domain.ImpactUnknown {
			t.Errorf("candidate %s is about finding %s classified %s, want unknown", c.ID, fd.ID, fd.Classification)
		}
		if len(c.Signals) == 0 {
			t.Errorf("candidate %s records no signals", c.ID)
		}
	}

	// computed diffs and routine items are never candidates, whatever their
	// verdict class is
	for _, c := range cs {
		for _, id := range c.Findings {
			fd := findingByID(t, f, id)
			ch, ok := f.changes[fd.ChangeID]
			if !ok {
				t.Fatalf("finding %s has no change", id)
			}
			if ch.Routine {
				t.Errorf("routine change %s became a candidate", ch.ID)
			}
			if ch.Provenance.Method == domain.MethodComputed && c.Type == CandApplicability {
				t.Errorf("computed change %s became an applicability candidate", ch.ID)
			}
		}
	}
}

func TestCandidatesOrderStable(t *testing.T) {
	f := newFixture(t)
	a := Candidates(f.report, f.edge, CandidateOptions{})
	b := Candidates(f.report, f.edge, CandidateOptions{})
	if len(a) != len(b) || len(a) == 0 {
		t.Fatalf("candidate counts differ or empty: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Fatalf("candidate order is not deterministic at %d: %s vs %s", i, a[i].ID, b[i].ID)
		}
	}
	// applicability candidates come first
	if a[0].Type != CandApplicability {
		t.Errorf("first candidate is %s, want applicability", a[0].Type)
	}
}

func TestCandidatesDuplicates(t *testing.T) {
	f := newFixture(t)
	cs := Candidates(f.report, f.edge, CandidateOptions{})
	m := byType(cs)
	if len(m[CandCluster]) != 1 {
		t.Fatalf("cluster candidates = %d, want 1 (the two Profiling notes)", len(m[CandCluster]))
	}
	c := m[CandCluster][0]
	if len(c.Findings) < 2 {
		t.Fatalf("cluster candidate lists %d findings, want ≥ 2", len(c.Findings))
	}
	for _, id := range c.Findings {
		fd := findingByID(t, f, id)
		if fd.ChangeID == "" {
			t.Errorf("cluster member %s has no upstream change", id)
		}
	}
}

func TestCandidatesMigration(t *testing.T) {
	f := newFixture(t)
	cs := Candidates(f.report, f.edge, CandidateOptions{})
	m := byType(cs)
	if len(m[CandMigration]) != 1 {
		t.Fatalf("migration candidates = %d, want 1 (the breaking values removal)", len(m[CandMigration]))
	}
	fd := findingByID(t, f, m[CandMigration][0].Findings[0])
	switch fd.Classification {
	case domain.ImpactActionRequired, domain.ImpactReviewRequired:
	default:
		t.Errorf("migration candidate is about a %s finding, want an affected class", fd.Classification)
	}
}
