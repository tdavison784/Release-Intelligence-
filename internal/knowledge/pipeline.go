package knowledge

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// RouteSummary counts what RouteStore did.
type RouteSummary struct {
	Candidates   int
	Facts        int // facts minted by auto-verification
	Items        int // review items newly created
	Existing     int // items/facts that were already stored (left untouched)
	Superseded   int // open items made moot by an auto-verified fact
	ByRoute      map[domain.Route]int
	Autoverified []string // fact ids
	Audits       []string // review item ids sampling auto-approved facts into human review
	// Skipped lists records the store refused (invalid), with the reason; the
	// pass continues so one bad candidate cannot block a whole store.
	Skipped []string
}

// RouteOptions configure RouteStore.
type RouteOptions struct {
	// Policy is the auto-approval policy (nil: none), e.g. AutoApproveRenderVerifiable.
	Policy AutoApprovePolicy
	// AuditEvery samples one in N auto-approved facts into human review
	// (a fact-review item), for the auto-approval agreement measurement
	// (RENDER-MISSION R19). The sample is a deterministic function of the fact
	// id. 0 disables sampling; 1 audits every auto-approved fact. Auto-approved
	// facts with an action-eligible consequence are always audited (PO-2: 100%),
	// whatever this is set to.
	AuditEvery int
	// Environment, when set, is recorded as ReviewItem.Context on every review
	// item this call CREATES: the environment illustration the reviewer will be
	// shown. Label is the eval case id (the directory name under eval/cases)
	// and nothing else — the eval's transfer subset compares it for equality.
	// nil (the default) leaves Context absent: items are environment-free.
	Environment *domain.EnvironmentContext
	// Now stamps the audit items (zero: the fact's creation time).
	Now func() time.Time
}

// RouteStore routes every candidate of the store matching q (using the stored
// proposals and validations, and any decisions already made) and writes the
// resulting review items and auto-verified facts. It is idempotent: records
// that exist are never rewritten, so a decided item keeps its status.
func RouteStore(ctx context.Context, s Store, opts RouteOptions, q Query) (*RouteSummary, error) {
	if err := ValidateEnvironment(opts.Environment); err != nil {
		return nil, err
	}
	q.Kinds = nil
	snap, err := s.Load(ctx, q)
	if err != nil {
		return nil, err
	}
	sum := &RouteSummary{ByRoute: map[domain.Route]int{}}
	items := map[string]domain.ReviewItem{}
	for _, it := range snap.ReviewItems {
		items[it.ID] = it
	}
	facts := map[string]domain.VerifiedFact{}
	for _, f := range snap.Facts {
		facts[f.ID] = f
	}
	for _, c := range snap.Candidates {
		sum.Candidates++
		var ps []domain.SemanticProposal
		var vs []domain.ValidationResult
		var ds []domain.ReviewDecision
		candItems := map[string]domain.ReviewItem{}
		for _, p := range snap.Proposals {
			if p.CandidateID == c.ID {
				ps = append(ps, p)
			}
		}
		for _, v := range snap.Validations {
			if v.CandidateID == c.ID {
				vs = append(vs, v)
			}
		}
		for id, it := range items {
			if it.CandidateID == c.ID {
				candItems[id] = it
			}
		}
		for _, d := range snap.Decisions {
			if _, ok := candItems[d.ReviewItemID]; ok {
				ds = append(ds, d)
			}
		}
		var res RouteResult
		if len(ds) == 0 {
			res = RouteWith(opts.Policy, c, ps, vs)
		} else {
			state := candidateState(nil, vs, candItems, ds)
			now := latestTime(c, ps, vs)
			if len(state) == len(domain.Aspects) {
				if f, err := buildFact(c, state, ps, vs, now); err == nil {
					res.Fact = f
				}
			} else {
				res.ReviewItems = buildItems(c, ps, vs, state, now)
			}
		}
		if res.Fact != nil {
			if _, ok := facts[res.Fact.ID]; ok {
				sum.Existing++
			} else {
				rec, err := domain.NewRecord(*res.Fact)
				if err != nil {
					return nil, err
				}
				if err := s.Put(ctx, rec); err != nil {
					sum.Skipped = append(sum.Skipped, c.ID+": fact: "+err.Error())
					continue
				}
				facts[res.Fact.ID] = *res.Fact
				sum.Facts++
				sum.Autoverified = append(sum.Autoverified, res.Fact.ID)
				sum.ByRoute[domain.RouteAutoVerify]++
				if AuditRequired(*res.Fact) || (res.Fact.AutoApproved && SampledForAudit(res.Fact.ID, opts.AuditEvery)) {
					at := res.Fact.CreatedAt
					if opts.Now != nil {
						at = opts.Now()
					}
					it, err := OpenFactReviewWith(ctx, s, res.Fact.ID, at, res.Signals)
					if err != nil {
						return nil, err
					}
					sum.Audits = append(sum.Audits, it.ID)
				}
				// questions routed before the fact existed are moot now
				for id, old := range candItems {
					if old.Status == domain.ReviewPending || old.Status == domain.ReviewNeedsEvidence {
						old.Status = domain.ReviewSuperseded
						rec, err := domain.NewRecord(old)
						if err != nil {
							return nil, err
						}
						if err := s.Put(ctx, rec); err != nil {
							return nil, err
						}
						items[id] = old
						sum.Superseded++
					}
				}
			}
		}
		for _, it := range res.ReviewItems {
			if opts.Environment != nil {
				env := *opts.Environment
				it.Context = &env
			}
			if _, ok := items[it.ID]; ok {
				sum.Existing++
				continue
			}
			rec, err := domain.NewRecord(it)
			if err != nil {
				return nil, err
			}
			if err := s.Put(ctx, rec); err != nil {
				sum.Skipped = append(sum.Skipped, c.ID+": item: "+err.Error())
				continue
			}
			items[it.ID] = it
			sum.Items++
			sum.ByRoute[it.Routing.Route]++
		}
	}
	return sum, nil
}

// SampledForAudit is the deterministic audit sample: one fact id in every
// `every` (0 or negative: never).
func SampledForAudit(factID string, every int) bool {
	if every <= 0 {
		return false
	}
	h := domain.ShortHash("audit", factID)
	n, err := strconv.ParseUint(h[:8], 16, 64)
	return err == nil && n%uint64(every) == 0
}

// AuditRequired reports whether an auto-approved fact must go to human review
// regardless of the sampling rate: its consequence is action-eligible, so
// consensus (not a person) is what would stand behind mandatory work (PO-2).
func AuditRequired(f domain.VerifiedFact) bool {
	return f.ConsensusAction || (f.AutoApproved && f.Assertion.Consequence != nil && f.Assertion.Consequence.Kind.ActionEligible())
}

// ValidateEnvironment checks an environment illustration: nil is fine
// (environment-free); otherwise the label is required and must be a bare eval
// case id (no spaces or path separators), because the eval compares
// ReviewItem.Context.Label with the case id for equality.
func ValidateEnvironment(e *domain.EnvironmentContext) error {
	if e == nil {
		return nil
	}
	if e.Label == "" || e.Label != strings.TrimSpace(e.Label) || strings.ContainsAny(e.Label, " \t\n/\\") {
		return fmt.Errorf("knowledge: environment label %q must be an eval case id (no spaces or path separators)", e.Label)
	}
	return nil
}
