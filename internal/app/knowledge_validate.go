package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// ValidateResolver supplies the artifacts a candidate is validated against:
// the ingested From/To releases and the edge the candidate came from. ok is
// false when the candidate belongs to none of the requested edges (it is then
// reported as skipped, never validated against a guess).
type ValidateResolver func(ctx context.Context, c domain.SemanticCandidate) (from, to *domain.Release, edge *domain.UpgradeEdge, ok bool, err error)

// ValidateOptions configures ValidateKnowledge.
type ValidateOptions struct {
	Validators []knowledge.Validator
	Resolve    ValidateResolver
	Query      knowledge.Query
}

// ValidateCount is the outcome tally of one validator × aspect.
type ValidateCount struct {
	Confirmed, Refuted, Inconclusive int
}

// ValidateReport is what `ri knowledge validate` did.
type ValidateReport struct {
	Candidates int `json:"candidates"`
	Proposals  int `json:"proposals"`
	// SkippedCandidates are candidates no requested edge produces.
	SkippedCandidates int `json:"skippedCandidates"`
	// SkippedProposals are proposals stating neither a subject nor a change
	// to check (an abstention or an applicability-only answer).
	SkippedProposals int `json:"skippedProposals"`
	Written          int `json:"written"`
	Existing         int `json:"existing"`
	// Shared counts results produced for a proposal whose assertion another
	// proposal already had checked (same result id).
	Shared int `json:"shared"`
	// NotApplicable counts validator × proposal pairs for which the validator
	// has no opinion (another family); nothing is stored for them.
	NotApplicable map[string]int `json:"notApplicable"`
	// Unavailable counts validator errors, each recorded as an explicit
	// inconclusive result with an `:unavailable` rule.
	Unavailable map[string]int `json:"unavailable"`
	// Outcomes: validator → aspect → tally, over every stored result.
	Outcomes map[string]map[domain.Aspect]*ValidateCount `json:"outcomes"`
}

func (r *ValidateReport) count(validator string, c domain.AspectCheck) {
	if r.Outcomes[validator] == nil {
		r.Outcomes[validator] = map[domain.Aspect]*ValidateCount{}
	}
	t := r.Outcomes[validator][c.Aspect]
	if t == nil {
		t = &ValidateCount{}
		r.Outcomes[validator][c.Aspect] = t
	}
	switch c.Outcome {
	case domain.OutcomeConfirmed:
		t.Confirmed++
	case domain.OutcomeRefuted:
		t.Refuted++
	default:
		t.Inconclusive++
	}
}

// ValidateKnowledge runs the validators over every proposal of the store and
// writes the ValidationResults back. It is idempotent: result ids are
// content-derived and CheckedAt is the candidate's own creation time, so a
// second run over unchanged artifacts writes nothing new. A validator that
// errors (a render that cannot run, say) is recorded as an explicit
// inconclusive result, never silently dropped.
func ValidateKnowledge(ctx context.Context, store knowledge.Store, opts ValidateOptions) (*ValidateReport, error) {
	q := opts.Query
	q.Kinds = []domain.RecordKind{domain.RecordCandidate, domain.RecordProposal}
	snap, err := store.Load(ctx, q)
	if err != nil {
		return nil, err
	}
	rep := &ValidateReport{NotApplicable: map[string]int{}, Unavailable: map[string]int{}, Outcomes: map[string]map[domain.Aspect]*ValidateCount{}}
	seen := map[string]bool{}
	byCand := map[string][]domain.SemanticProposal{}
	for _, p := range snap.Proposals {
		byCand[p.CandidateID] = append(byCand[p.CandidateID], p)
	}
	cands := snap.Candidates
	sort.Slice(cands, func(i, j int) bool { return cands[i].ID < cands[j].ID })
	for _, c := range cands {
		from, to, edge, ok, err := opts.Resolve(ctx, c)
		if err != nil {
			return rep, fmt.Errorf("candidate %s: %w", c.ID, err)
		}
		if !ok {
			rep.SkippedCandidates++
			continue
		}
		rep.Candidates++
		props := byCand[c.ID]
		sort.Slice(props, func(i, j int) bool { return props[i].ID < props[j].ID })
		for _, p := range props {
			rep.Proposals++
			if p.Assertion.Subject == nil && p.Assertion.Change == nil {
				rep.SkippedProposals++
				continue
			}
			in := knowledge.ValidationInput{Candidate: c, ProposalID: p.ID, Assertion: p.Assertion, From: from, To: to, Edge: edge, Now: c.CreatedAt}
			for _, v := range opts.Validators {
				results, verr := v.Validate(ctx, in)
				if verr != nil {
					rep.Unavailable[v.Name()]++
					results = []domain.ValidationResult{unavailable(v.Name(), in, verr)}
				} else if len(results) == 0 {
					rep.NotApplicable[v.Name()]++
					continue
				}
				for _, r := range results {
					// A result is a function of (candidate, validator,
					// assertion), not of the proposal: proposals that assert
					// the same thing share one result (the first proposal
					// seen names it), so it is stored and tallied once.
					if seen[r.ID] {
						rep.Shared++
						continue
					}
					seen[r.ID] = true
					rec, err := domain.NewRecord(r)
					if err != nil {
						return rep, fmt.Errorf("validation %s: %w", r.ID, err)
					}
					stored, gerr := store.Get(ctx, r.ID)
					switch {
					case gerr == nil && stored.Validation != nil:
						rep.Existing++
					case gerr == nil || !errors.Is(gerr, knowledge.ErrNotFound):
						return rep, fmt.Errorf("validation %s: %v", r.ID, gerr)
					default:
						rep.Written++
						if err := store.Put(ctx, rec); err != nil {
							return rep, fmt.Errorf("validation %s: %w", r.ID, err)
						}
					}
					for _, ck := range r.Checks {
						rep.count(v.Name(), ck)
					}
				}
			}
		}
	}
	return rep, nil
}

// unavailable is the explicit record of a validator that could not run.
func unavailable(name string, in knowledge.ValidationInput, err error) domain.ValidationResult {
	r := domain.ValidationResult{
		CandidateID: in.Candidate.ID, ProposalID: in.ProposalID, Validator: name,
		Assertion: in.Assertion, CheckedAt: in.Now,
	}
	for _, a := range []domain.Aspect{domain.AspectSubject, domain.AspectChange} {
		if in.Assertion.Has(a) {
			r.Checks = append(r.Checks, domain.AspectCheck{Aspect: a, Outcome: domain.OutcomeInconclusive, Rule: name + ":unavailable", Detail: err.Error()})
		}
	}
	r.ID = domain.ValidationID(r.CandidateID, r.Validator, r.Assertion)
	return r
}

// WriteText renders the report: per validator and aspect, then totals.
func (r *ValidateReport) WriteText(w io.Writer) {
	fmt.Fprintf(w, "validated %d candidates (%d skipped: in none of the requested edges), %d proposals (%d without subject/change)\n",
		r.Candidates, r.SkippedCandidates, r.Proposals, r.SkippedProposals)
	fmt.Fprintf(w, "results: %d written, %d already stored, %d shared by proposals with identical assertions\n", r.Written, r.Existing, r.Shared)
	var names []string
	for n := range r.Outcomes {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintf(w, "\n%-34s %-14s %9s %8s %13s\n", "validator", "aspect", "confirmed", "refuted", "inconclusive")
	for _, n := range names {
		for _, a := range domain.Aspects {
			if t := r.Outcomes[n][a]; t != nil {
				fmt.Fprintf(w, "%-34s %-14s %9d %8d %13d\n", n, a, t.Confirmed, t.Refuted, t.Inconclusive)
			}
		}
	}
	if len(r.NotApplicable) > 0 {
		fmt.Fprintf(w, "\nnot applicable (no result stored): %v\n", r.NotApplicable)
	}
	if len(r.Unavailable) > 0 {
		fmt.Fprintf(w, "unavailable (recorded as inconclusive): %v\n", r.Unavailable)
	}
}
