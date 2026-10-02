package knowledge

import (
	"context"
	"strconv"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// RouteSummary counts what RouteStore did.
type RouteSummary struct {
	Candidates   int
	Facts        int // facts minted by auto-verification
	Items        int // review items newly created
	Existing     int // items/facts that were already stored (left untouched)
	ByRoute      map[domain.Route]int
	Autoverified []string // fact ids
	Audits       []string // review item ids sampling auto-approved facts into human review
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
	// Now stamps the audit items (zero: the fact's creation time).
	Now func() time.Time
}

// RouteStore routes every candidate of the store matching q (using the stored
// proposals and validations, and any decisions already made) and writes the
// resulting review items and auto-verified facts. It is idempotent: records
// that exist are never rewritten, so a decided item keeps its status.
func RouteStore(ctx context.Context, s Store, opts RouteOptions, q Query) (*RouteSummary, error) {
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
					return nil, err
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
					it, err := OpenFactReview(ctx, s, res.Fact.ID, at)
					if err != nil {
						return nil, err
					}
					sum.Audits = append(sum.Audits, it.ID)
				}
			}
		}
		for _, it := range res.ReviewItems {
			if _, ok := items[it.ID]; ok {
				sum.Existing++
				continue
			}
			rec, err := domain.NewRecord(it)
			if err != nil {
				return nil, err
			}
			if err := s.Put(ctx, rec); err != nil {
				return nil, err
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
	return f.AutoApproved && f.Assertion.Consequence != nil && f.Assertion.Consequence.Kind.ActionEligible()
}
