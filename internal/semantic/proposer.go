package semantic

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// ProposerOptions configure an LLMProposer.
type ProposerOptions struct {
	// MaxTokens bounds the answer (0 = the client's default).
	MaxTokens int
	// Clock stamps answers whose client reported no generation time
	// (default time.Now).
	Clock func() time.Time
}

// LLMProposer is one model behind one provider, answering through an
// llm.Client: the Anthropic Messages API, an Anthropic-compatible gateway
// (Z.AI for GLM), the file exchange, or the cache around any of them. It is
// one implementation of knowledge.Proposer; a typed model is another (it
// returns an Answer and calls ProposalFromAnswer).
type LLMProposer struct {
	client   llm.Client
	provider string
	model    string
	opts     ProposerOptions
}

var _ knowledge.Proposer = (*LLMProposer)(nil)

// NewLLMProposer returns a proposer for model served by provider.
func NewLLMProposer(client llm.Client, provider, model string, opts ProposerOptions) *LLMProposer {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	return &LLMProposer{client: client, provider: provider, model: model, opts: opts}
}

// Provider implements knowledge.Proposer.
func (p *LLMProposer) Provider() string { return p.provider }

// Model implements knowledge.Proposer.
func (p *LLMProposer) Model() string { return p.model }

// ProposalError is a failed proposal attempt with the prompt it was for.
type ProposalError struct {
	PromptDigest string
	// Kind: "pending" (waiting for an exchange/cached answer), "transport"
	// (the call failed), "refused" (the model declined), "rejected" (the
	// answer violated the schema or the contract).
	Kind string
	Err  error
}

func (e *ProposalError) Error() string { return e.Kind + ": " + e.Err.Error() }
func (e *ProposalError) Unwrap() error { return e.Err }

// Propose implements knowledge.Proposer.
func (p *LLMProposer) Propose(ctx context.Context, req knowledge.ProposalRequest) (*domain.SemanticProposal, error) {
	pr, err := BuildPrompt(req, p.model)
	if err != nil {
		return nil, &ProposalError{Kind: "rejected", Err: err}
	}
	if p.opts.MaxTokens > 0 {
		pr.Request.MaxTokens = p.opts.MaxTokens
	}
	digest := llm.PromptDigest(pr.Request)
	resp, err := p.client.Complete(ctx, pr.Request)
	switch {
	case err != nil && ctx.Err() != nil:
		return nil, ctx.Err()
	case errors.Is(err, llm.ErrPending), errors.Is(err, llm.ErrNotCached):
		return nil, &ProposalError{PromptDigest: digest, Kind: "pending", Err: err}
	case errors.Is(err, llm.ErrRefusal):
		return nil, &ProposalError{PromptDigest: digest, Kind: "refused", Err: err}
	case errors.Is(err, llm.ErrExchangeMismatch):
		return nil, &ProposalError{PromptDigest: digest, Kind: "rejected", Err: err}
	case err != nil:
		return nil, &ProposalError{PromptDigest: digest, Kind: "transport", Err: err}
	}
	ans, err := DecodeAnswer(req.Task, resp.Text)
	if err != nil {
		return nil, &ProposalError{PromptDigest: digest, Kind: "rejected", Err: err}
	}
	gen := resp.GeneratedAt
	if gen.IsZero() {
		gen = p.opts.Clock()
	}
	prop, err := ProposalFromAnswer(req.Candidate, req.Task, ans, AnswerMeta{
		Provider: p.provider, Model: resp.Model, ModelVersion: resp.ModelVersion,
		PromptVersion: pr.PromptVersion, PromptDigest: digest, Input: pr.Input, KnownFacts: pr.KnownFacts,
		GeneratedAt: gen,
	})
	if err != nil {
		return nil, &ProposalError{PromptDigest: digest, Kind: "rejected", Err: err}
	}
	return prop, nil
}

// ProposeOptions configure ProposeAllWith.
type ProposeOptions struct {
	// Context returns the release-level artifact context of a candidate
	// (e.g. ArtifactContext over the target release); nil = none.
	Context func(domain.SemanticCandidate) knowledge.ProposalContext
	// KnownFacts returns the facts the duplicate task compares against; a
	// candidate without known facts gets no duplicate prompt.
	KnownFacts func(domain.SemanticCandidate) []knowledge.FactSummary
	// Parallel bounds concurrent calls per proposer (default 1).
	Parallel int
	// MaxConsecutiveFailures stops asking a proposer after this many
	// transport failures in a row (default 3), e.g. a bad API key.
	MaxConsecutiveFailures int
	// Clock stamps failures (default time.Now).
	Clock func() time.Time
	// Progress, when set, is called after every attempt.
	Progress func(done, total int)
}

// ProposeAll asks every proposer every task for every candidate. Proposals
// are never merged (G3): each model's answer is its own record. A failure is
// recorded, never dropped and never turned into an empty proposal. Output
// order is deterministic: candidate, task, proposer.
func ProposeAll(ctx context.Context, cands []domain.SemanticCandidate, ps []knowledge.Proposer,
	tasks []domain.ProposalTask) ([]domain.SemanticProposal, []knowledge.ProposalFailure) {
	return ProposeAllWith(ctx, cands, ps, tasks, ProposeOptions{})
}

// FailurePending prefixes the Reason of a ProposalFailure whose answer is
// still pending (file exchange / offline cache): asked, not failed.
const FailurePending = "pending: "

// ProposeAllWith is ProposeAll with context, known facts and concurrency.
func ProposeAllWith(ctx context.Context, cands []domain.SemanticCandidate, ps []knowledge.Proposer,
	tasks []domain.ProposalTask, opts ProposeOptions) ([]domain.SemanticProposal, []knowledge.ProposalFailure) {
	if opts.Parallel <= 0 {
		opts.Parallel = 1
	}
	if opts.MaxConsecutiveFailures <= 0 {
		opts.MaxConsecutiveFailures = 3
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	type job struct {
		slot int
		req  knowledge.ProposalRequest
	}
	type result struct {
		prop *domain.SemanticProposal
		fail *knowledge.ProposalFailure
	}
	var reqs []knowledge.ProposalRequest
	for _, c := range cands {
		var pctx knowledge.ProposalContext
		if opts.Context != nil {
			pctx = opts.Context(c)
		}
		for _, t := range tasks {
			r := knowledge.ProposalRequest{Candidate: c, Task: t, Context: pctx}
			if t == domain.TaskDuplicate {
				if opts.KnownFacts != nil {
					r.KnownFacts = opts.KnownFacts(c)
				}
				if len(r.KnownFacts) == 0 {
					continue
				}
			}
			reqs = append(reqs, r)
		}
	}
	results := make([][]result, len(ps))
	total := len(reqs) * len(ps)
	var doneMu sync.Mutex
	done := 0
	var wg sync.WaitGroup
	for pi, p := range ps {
		results[pi] = make([]result, len(reqs))
		wg.Add(1)
		go func(pi int, p knowledge.Proposer) {
			defer wg.Done()
			jobs := make(chan job)
			var mu sync.Mutex
			consecutive := 0
			var inner sync.WaitGroup
			for w := 0; w < opts.Parallel; w++ {
				inner.Add(1)
				go func() {
					defer inner.Done()
					for j := range jobs {
						fail := func(digest, reason string) *knowledge.ProposalFailure {
							return &knowledge.ProposalFailure{CandidateID: j.req.Candidate.ID, Task: j.req.Task,
								Provider: p.Provider(), Model: p.Model(), PromptDigest: digest, Reason: reason, At: opts.Clock().UTC()}
						}
						mu.Lock()
						stop := consecutive >= opts.MaxConsecutiveFailures
						mu.Unlock()
						var res result
						if stop {
							res.fail = fail("", fmt.Sprintf("not asked: %d transport failures in a row", opts.MaxConsecutiveFailures))
						} else {
							prop, err := p.Propose(ctx, j.req)
							var pe *ProposalError
							switch {
							case err == nil && prop == nil:
								res.fail = fail("", "proposer returned neither a proposal nor an error")
							case err == nil:
								if verr := prop.ValidateAgainst(j.req.Candidate); verr != nil {
									res.fail = fail(prop.Provenance.PromptDigest, "rejected: "+oneLine(verr.Error()))
								} else {
									res.prop = prop
								}
								mu.Lock()
								consecutive = 0
								mu.Unlock()
							case errors.As(err, &pe):
								reason := pe.Kind + ": " + oneLine(pe.Err.Error())
								if pe.Kind == "pending" {
									reason = FailurePending + oneLine(pe.Err.Error())
								}
								res.fail = fail(pe.PromptDigest, reason)
								mu.Lock()
								if pe.Kind == "transport" {
									consecutive++
								} else {
									consecutive = 0
								}
								mu.Unlock()
							default:
								res.fail = fail("", oneLine(err.Error()))
								mu.Lock()
								consecutive++
								mu.Unlock()
							}
						}
						results[pi][j.slot] = res
						if opts.Progress != nil {
							doneMu.Lock()
							done++
							opts.Progress(done, total)
							doneMu.Unlock()
						}
					}
				}()
			}
			for i, r := range reqs {
				if ctx.Err() != nil {
					break
				}
				jobs <- job{slot: i, req: r}
			}
			close(jobs)
			inner.Wait()
		}(pi, p)
	}
	wg.Wait()
	var props []domain.SemanticProposal
	var fails []knowledge.ProposalFailure
	for i := range reqs {
		for pi := range ps {
			r := results[pi][i]
			switch {
			case r.prop != nil:
				props = append(props, *r.prop)
			case r.fail != nil:
				fails = append(fails, *r.fail)
			case ctx.Err() != nil:
				fails = append(fails, knowledge.ProposalFailure{CandidateID: reqs[i].Candidate.ID, Task: reqs[i].Task,
					Provider: ps[pi].Provider(), Model: ps[pi].Model(), Reason: "not asked: " + ctx.Err().Error(), At: opts.Clock().UTC()})
			}
		}
	}
	return props, fails
}

// IsPending reports whether a failure is a pending answer rather than a
// failed one.
func IsPending(f knowledge.ProposalFailure) bool { return strings.HasPrefix(f.Reason, FailurePending) }
