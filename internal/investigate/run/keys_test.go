package run

import (
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// The run record digests ignore every wall-clock stamp the record keeps as
// provenance — CreatedAt, audit times, the impact report's stamps and the
// validators' CheckedAt — and nothing else: two identical runs get the same
// digests, a changed outcome does not.
func TestRecordDigestsIgnoreOnlyStamps(t *testing.T) {
	rec := func(at time.Time, outcome domain.ValidationOutcomeKind) *domain.Investigation {
		inv := &domain.Investigation{ID: "inv-1", ImpactReport: "art-1", CreatedAt: at,
			ReportStamps: &domain.ReportStamps{GeneratedAt: at, RetrievedAt: map[domain.EvidenceID]time.Time{"ev-1": at}},
			Validations:  []domain.ValidationRecord{{ID: "vr-1", Outcome: outcome, CheckedAt: at}}}
		inv.AppendAudit(domain.AuditEvent{At: at, Stage: domain.StageValidate, Event: "completed", Actor: "system"})
		return inv
	}
	a := rec(time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC), domain.ValidationPass)
	b := rec(time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC), domain.ValidationPass)
	if Digest(a) != Digest(b) || ContentDigest(a) != ContentDigest(b) {
		t.Fatal("records that differ only in their stamps have different digests")
	}
	if a.Validations[0].CheckedAt.Equal(b.Validations[0].CheckedAt) || a.ReportStamps == nil {
		t.Fatal("the digests must not mutate the record: stamps are kept")
	}
	if c := rec(a.CreatedAt, domain.ValidationFail); Digest(c) == Digest(a) || ContentDigest(c) == ContentDigest(a) {
		t.Fatal("a different validation outcome must change the digests")
	}
}
