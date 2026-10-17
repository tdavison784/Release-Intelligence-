package classify

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Apply records a classification on the run record: findings, decision,
// review pending (milestone 1 never enacts automated approval, so every
// classified investigation waits for a human — AC8 for the non-eligible ones,
// measurement for the eligible ones), status awaiting-review and a classify
// audit event at `at`. The record is validated; on error it is unchanged.
func Apply(inv *domain.Investigation, c *investigate.Classification, at time.Time) error {
	if inv == nil || c == nil {
		return errors.New("classify: apply: nil investigation or classification")
	}
	next := clone(inv)
	next.Findings = append([]domain.InvestigationFinding(nil), c.Findings...)
	d := c.Decision
	next.Decision = &d
	next.Review = domain.HumanReview{State: domain.ReviewAwaiting}
	next.Status = domain.InvestigationAwaiting
	refs := []string{string(d.Outcome)}
	for _, f := range c.Findings {
		refs = append(refs, f.ID)
	}
	if err := appendEvent(next, domain.AuditEvent{At: at, Stage: domain.StageClassify, Event: "completed", Actor: "system", Refs: refs,
		Detail: fmt.Sprintf("%s: %d finding(s); decision %s (shadow: automated approval not enacted)", d.Policy, len(c.Findings), d.Outcome)}); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("classify: apply: %w", err)
	}
	*inv = *next
	return nil
}

// NewReviewApplier returns the human-review applier (DESIGN.md §2.9, §5.3).
func NewReviewApplier() investigate.ReviewApplier { return reviewApplier{} }

type reviewApplier struct{}

// ApplyReview applies one review decision:
//
//   - a HUMAN approve / reject / request-more-evidence sets the review state
//     (approved and rejected are final; more evidence starts a new round);
//   - a PROXY (AI reviewer) decision of any action is recorded — reviewer,
//     AI provenance, note, audit event — but never changes the state: only a
//     human decides, and a proxy can never approve.
//
// The decision is validated, the audit event appended (hash chain), and the
// whole record validated; on any error the record is unchanged.
func (reviewApplier) ApplyReview(inv *domain.Investigation, d investigate.ReviewDecision) error {
	if inv == nil {
		return errors.New("review: nil investigation")
	}
	if err := checkDecision(d); err != nil {
		return fmt.Errorf("review: %w: %w", investigate.ErrRejected, err)
	}
	if inv.Decision == nil {
		return fmt.Errorf("review: %w: investigation %s is not classified yet (no policy decision to review)", investigate.ErrRejected, inv.ID)
	}
	switch inv.Review.State {
	case domain.ReviewApproved, domain.ReviewRejected:
		return fmt.Errorf("review: %w: investigation %s was already %s by %s", investigate.ErrRejected, inv.ID, inv.Review.State, inv.Review.Reviewer)
	}
	next := clone(inv)
	at := d.At.UTC()
	actor := "human:" + d.Reviewer
	detail := string(d.Action)
	if d.ReviewerKind == domain.ReviewerProxy {
		actor = "proxy:" + d.Reviewer
		detail = fmt.Sprintf("proxy (AI) recommendation %q recorded, not enacted: only a human decides, and a proxy never approves", d.Action)
		next.Review = domain.HumanReview{State: stateOr(inv.Review.State), Reviewer: d.Reviewer, ReviewerKind: domain.ReviewerProxy,
			DecidedAt: &at, Note: d.Note, ProxyProvenance: d.ProxyProvenance}
	} else {
		r := domain.HumanReview{Reviewer: d.Reviewer, ReviewerKind: domain.ReviewerHuman, DecidedAt: &at, Note: d.Note}
		switch d.Action {
		case investigate.ReviewApprove:
			r.State, next.Status = domain.ReviewApproved, domain.InvestigationCompleted
			if inv.Decision.Outcome != domain.PolicyAutoApproveEligible {
				detail += fmt.Sprintf(" (human approval of a %s investigation; the policy did not find it eligible)", inv.Decision.Outcome)
			}
		case investigate.ReviewReject:
			r.State, next.Status = domain.ReviewRejected, domain.InvestigationCompleted
		case investigate.ReviewRequestMoreEvidence:
			r.State, next.Status = domain.ReviewMoreEvidenceWanted, domain.InvestigationRunning
			detail += ": " + strings.Join(d.Requests, "; ")
		}
		next.Review = r
	}
	if d.Note != "" {
		detail += " — " + d.Note
	}
	if err := appendEvent(next, domain.AuditEvent{At: at, Stage: domain.StageHumanReview, Event: "decided", Actor: actor,
		Refs: []string{string(d.Action)}, Detail: detail}); err != nil {
		return fmt.Errorf("review: %w: %w", investigate.ErrRejected, err)
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("review: %w", err)
	}
	*inv = *next
	return nil
}

func checkDecision(d investigate.ReviewDecision) error {
	var errs []error
	switch d.Action {
	case investigate.ReviewApprove, investigate.ReviewReject, investigate.ReviewRequestMoreEvidence:
	default:
		errs = append(errs, fmt.Errorf("unknown action %q", d.Action))
	}
	if strings.TrimSpace(d.Reviewer) == "" {
		errs = append(errs, errors.New("reviewer is required"))
	}
	if d.At.IsZero() {
		errs = append(errs, errors.New("the decision time is required"))
	}
	switch d.ReviewerKind {
	case domain.ReviewerHuman:
		if d.ProxyProvenance != nil {
			errs = append(errs, errors.New("a human decision carries no proxy provenance"))
		}
	case domain.ReviewerProxy:
		if p := d.ProxyProvenance; p == nil || p.Method != domain.MethodAI {
			errs = append(errs, errors.New("a proxy (AI) decision requires its AI provenance"))
		} else if err := p.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("proxy provenance: %w", err))
		}
	default:
		errs = append(errs, fmt.Errorf("reviewerKind must be %s or %s (got %q)", domain.ReviewerHuman, domain.ReviewerProxy, d.ReviewerKind))
	}
	if d.Action == investigate.ReviewRequestMoreEvidence && len(d.Requests) == 0 {
		errs = append(errs, errors.New("request-more-evidence names what evidence is wanted"))
	}
	return errors.Join(errs...)
}

func stateOr(s domain.ReviewState) domain.ReviewState {
	if s == "" || s == domain.ReviewNotRequired {
		return domain.ReviewAwaiting
	}
	return s
}

func appendEvent(inv *domain.Investigation, e domain.AuditEvent) error {
	if e.At.IsZero() {
		return errors.New("audit event time is required")
	}
	if n := len(inv.Audit); n > 0 && e.At.Before(inv.Audit[n-1].At) {
		return fmt.Errorf("audit event at %s precedes the last event (%s)", e.At.Format(time.RFC3339), inv.Audit[n-1].At.Format(time.RFC3339))
	}
	inv.AppendAudit(e)
	return nil
}

// clone copies the record deeply enough that mutating the copy's review,
// status, findings, decision and audit never aliases the original.
func clone(inv *domain.Investigation) *domain.Investigation {
	c := *inv
	c.Audit = append([]domain.AuditEvent(nil), inv.Audit...)
	c.Findings = append([]domain.InvestigationFinding(nil), inv.Findings...)
	if inv.Decision != nil {
		d := *inv.Decision
		c.Decision = &d
	}
	return &c
}
