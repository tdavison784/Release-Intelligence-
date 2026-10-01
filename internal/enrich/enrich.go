// Package enrich adds provenance-preserving semantic enrichment to an
// UpgradeEdge. AI may group, summarise, connect and explain the edge's
// deterministic Changes; it never replaces or modifies a source fact or a
// Change. The pipeline is:
//
//  1. Candidates: deterministic candidate groups (shared subjects/keys,
//     shared CVE/GHSA/PR references, normalised token similarity across
//     different sources, a computed diff whose key a note mentions,
//     upgrade-guide ↔ release-note pairs). Candidates are not conclusions.
//  2. One bounded prompt per candidate group, asking for structured JSON
//     (validated against a JSON Schema).
//  3. A validator that rejects hallucinated change ids, citations outside the
//     evidence shown in the prompt, empty content and kind-specific
//     violations, recording every rejection.
//  4. domain.Enrichments with AI provenance: model and model version as
//     reported with the answer, prompt version and digest, input evidence,
//     generation time.
//
// Without a model client, Run produces candidates only and no enrichment.
package enrich

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// Producer is recorded in the provenance of every enrichment.
const Producer = "enrich@v1"

// Options configure Run.
type Options struct {
	// Client answers the prompts (typically an llm.Cache around the API or an
	// llm.Exchange). nil: candidates only, no enrichment.
	Client llm.Client
	// Model is the requested model ("" = the client's default). It is part
	// of the prompt digest; the recorded model is the one that answered.
	Model string
	// MaxGroups bounds the number of prompts per edge (default 40).
	MaxGroups  int
	Candidates CandidateOptions
	// MaxConsecutiveFailures stops asking after this many failed requests in
	// a row, e.g. a bad API key (default 3).
	MaxConsecutiveFailures int
	// Clock stamps answers whose client did not report a generation time.
	Clock func() time.Time
}

// Request statuses.
const (
	StatusAnswered = "answered"
	StatusPending  = "pending"
	StatusFailed   = "failed"
	StatusRejected = "rejected" // the whole answer was refused (not JSON, wrong schema)
	StatusSkipped  = "skipped"  // not asked: over MaxGroups or after repeated failures
)

// RequestRecord reports what happened to the prompt of one candidate group.
type RequestRecord struct {
	Group        string `json:"group"`
	PromptDigest string `json:"promptDigest,omitempty"`
	Status       string `json:"status"`
	Origin       string `json:"origin,omitempty"` // api, cache, exchange, fake
	Detail       string `json:"detail,omitempty"`
}

// Result is the outcome of Run. It does not modify the edge; Apply does.
type Result struct {
	Candidates  []Candidate          `json:"candidates"`
	Requests    []RequestRecord      `json:"requests"`
	Enrichments []domain.Enrichment  `json:"enrichments"`
	Run         domain.EnrichmentRun `json:"run"`
}

// Run computes candidate groups for e and, when a client is configured, asks
// it about each group and validates the answers.
func Run(ctx context.Context, e *domain.UpgradeEdge, opts Options) (*Result, error) {
	if e == nil {
		return nil, errors.New("enrich: nil edge")
	}
	if opts.MaxGroups <= 0 {
		opts.MaxGroups = 40
	}
	if opts.MaxConsecutiveFailures <= 0 {
		opts.MaxConsecutiveFailures = 3
	}
	now := opts.Clock
	if now == nil {
		now = time.Now
	}
	res := &Result{Candidates: Candidates(e, opts.Candidates), Requests: []RequestRecord{}, Enrichments: []domain.Enrichment{}}
	res.Run = domain.EnrichmentRun{Producer: Producer, PromptVersion: PromptVersion, CandidateGroups: len(res.Candidates)}
	if opts.Client == nil {
		return res, nil
	}

	changes := map[string]domain.Change{}
	for _, c := range e.Changes {
		changes[c.ID] = c
	}
	ev := map[domain.EvidenceID]domain.Evidence{}
	for _, x := range e.Evidence {
		ev[x.ID] = x
	}
	clustered := map[string]string{}
	usedIDs := map[string]bool{}
	failures := 0
	for i, cand := range res.Candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if i >= opts.MaxGroups || failures >= opts.MaxConsecutiveFailures {
			detail := fmt.Sprintf("over the limit of %d prompts per edge", opts.MaxGroups)
			if failures >= opts.MaxConsecutiveFailures {
				detail = fmt.Sprintf("not asked after %d failed requests in a row", failures)
			}
			res.Requests = append(res.Requests, RequestRecord{Group: cand.ID, Status: StatusSkipped, Detail: detail})
			continue
		}
		p := buildPrompt(e, cand, opts.Model, changes, ev)
		digest := llm.PromptDigest(p.req)
		res.Run.Requests++
		rec := RequestRecord{Group: cand.ID, PromptDigest: digest}
		resp, err := opts.Client.Complete(ctx, p.req)
		switch {
		case err != nil && ctx.Err() != nil:
			return nil, ctx.Err()
		case errors.Is(err, llm.ErrPending), errors.Is(err, llm.ErrNotCached):
			rec.Status, rec.Detail = StatusPending, err.Error()
			res.Run.Pending++
			res.Requests = append(res.Requests, rec)
			continue
		case err != nil:
			rec.Status, rec.Detail = StatusFailed, err.Error()
			res.Run.Failed++
			failures++
			res.Requests = append(res.Requests, rec)
			continue
		}
		failures = 0
		rec.Origin = resp.Origin
		if resp.Model == "" || resp.ModelVersion == "" {
			rec.Status, rec.Detail = StatusRejected, "the answer does not report the model and model version that produced it"
			res.reject(cand.ID, digest, "", nil, rec.Detail)
			res.Requests = append(res.Requests, rec)
			continue
		}
		ans, err := decodeAnswer(resp.Text)
		if err != nil {
			rec.Status, rec.Detail = StatusRejected, err.Error()
			res.reject(cand.ID, digest, "", nil, rec.Detail)
			res.Requests = append(res.Requests, rec)
			continue
		}
		rec.Status = StatusAnswered
		res.Requests = append(res.Requests, rec)

		gen := resp.GeneratedAt
		if gen.IsZero() {
			gen = now()
		}
		gen = gen.UTC()
		g := newGroupContext(cand, changes, p.input)
		accepted := map[string]bool{}
		for _, pr := range ans.Enrichments {
			if err := g.check(pr, clustered); err != nil {
				res.reject(cand.ID, digest, domain.EnrichmentKind(pr.Kind), pr.Changes, err.Error())
				continue
			}
			key := pr.Kind + "\x00" + strings.Join(canonical(pr.Changes), "\x00")
			if accepted[key] {
				res.reject(cand.ID, digest, domain.EnrichmentKind(pr.Kind), pr.Changes, "duplicate of an accepted enrichment of the same kind about the same changes")
				continue
			}
			accepted[key] = true
			en := toEnrichment(pr, cand, digest, p.input, resp, gen)
			for usedIDs[en.ID] {
				en.ID = "enr-" + domain.ShortHash(en.ID)
			}
			usedIDs[en.ID] = true
			if en.Kind == domain.EnrichmentCluster {
				for _, id := range en.RelatesTo {
					clustered[id] = en.ID
				}
			}
			res.Enrichments = append(res.Enrichments, en)
		}
	}
	res.Run.Accepted = len(res.Enrichments)
	res.Run.Clusters, res.Run.ClusteredChanges = domain.ClusterMetrics(res.Enrichments)
	res.Run.DuplicatesConsolidated = res.Run.ClusteredChanges - res.Run.Clusters
	return res, nil
}

func (r *Result) reject(group, digest string, kind domain.EnrichmentKind, changes []string, reason string) {
	r.Run.Rejected = append(r.Run.Rejected, domain.EnrichmentRejection{Group: group, PromptDigest: digest, Kind: kind,
		RelatesTo: append([]string(nil), changes...), Reason: reason})
}

// confidence caps AI output: it is never presented with high confidence, and a
// relation the evidence does not establish is low by definition.
func confidence(kind domain.EnrichmentKind, said string) domain.Confidence {
	if kind == domain.EnrichmentRelated || said == string(domain.ConfidenceLow) {
		return domain.ConfidenceLow
	}
	return domain.ConfidenceMedium
}

// toEnrichment builds the Enrichment of a validated proposal. Change ids are
// listed in edge order and citations in prompt order; the text is the
// model's, trimmed.
func toEnrichment(p proposal, cand Candidate, digest string, input []domain.EvidenceID, resp *llm.Response, gen time.Time) domain.Enrichment {
	kind := domain.EnrichmentKind(p.Kind)
	want := toSet(p.Changes...)
	var relates []string
	for _, id := range cand.Changes {
		if want[id] {
			relates = append(relates, id)
		}
	}
	cited := toSet(p.Citations...)
	var citations []domain.EvidenceID
	for _, id := range input {
		if cited[string(id)] {
			citations = append(citations, id)
		}
	}
	idParts := append([]string{digest, p.Kind}, relates...)
	return domain.Enrichment{
		ID:         domain.EnrichmentIDPrefix + domain.ShortHash(idParts...),
		Kind:       kind,
		Title:      oneLine(p.Title),
		Content:    strings.TrimSpace(p.Content),
		RelatesTo:  relates,
		Citations:  citations,
		Unverified: kind == domain.EnrichmentRelated,
		Provenance: domain.Provenance{
			Method:        domain.MethodAI,
			Producer:      Producer,
			Rule:          "group:" + cand.ID,
			Confidence:    confidence(kind, p.Confidence),
			Model:         resp.Model,
			ModelVersion:  resp.ModelVersion,
			PromptVersion: PromptVersion,
			PromptDigest:  digest,
			InputEvidence: append([]domain.EvidenceID(nil), input...),
			GeneratedAt:   &gen,
		},
	}
}

// Apply attaches the result to the edge: Enrichments and EnrichmentRun. The
// edge's changes, facts and evidence are not touched. The edge is validated
// afterwards; on failure it is left unchanged.
func Apply(e *domain.UpgradeEdge, r *Result) error {
	prevE, prevR := e.Enrichments, e.EnrichmentRun
	e.Enrichments = append([]domain.Enrichment(nil), r.Enrichments...)
	run := r.Run
	e.EnrichmentRun = &run
	if err := e.Validate(); err != nil {
		e.Enrichments, e.EnrichmentRun = prevE, prevR
		return fmt.Errorf("enrich: the enriched edge is invalid: %w", err)
	}
	return nil
}
