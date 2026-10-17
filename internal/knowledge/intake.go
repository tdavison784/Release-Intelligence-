package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Human-authored rule intake (`ri knowledge intake-rule`, lane verify2;
// PROTOCOL amendment 2026-10-17 "fallback"). A rule file written outside the
// agent becomes exactly what `ri investigate export-hypotheses` makes of an
// agent hypothesis: a SemanticProposal plus a whole-fact fact-review item
// proposing exactly its rule — never a fact. Its provenance rule is
// "human-authored:<hyp-id>" (the id derived from the file's sha256), so
// routing ignores it, `ri verify hypotheses` verifies it and the promotion
// gate (promote.go) applies unchanged: human only, whole fact, a confirmed
// proof of exactly this rule, no refutation, bounds, explicit
// supersedes/coexists.
//
// The file is encoded faithfully or refused: unknown fields, a release
// bounds shape other than "(from, to]", or any part of the assertion that
// does not survive a round trip through the fact grammar unchanged is an
// error — never reinterpreted.

// IntakeProducer names the intake (component@vN).
const IntakeProducer = "knowledge.intake@v1"

// HumanAuthoredProvider is the provider and model version of a
// human-authored proposal; HumanAuthoredPrompt its prompt version.
const (
	HumanAuthoredProvider = "human-authored"
	HumanAuthoredPrompt   = "human-authored/v1"
)

// ruleFileBounds is the only release-bounds shape: FROM exclusive, TO
// inclusive (domain.ReleaseBounds).
const ruleFileBounds = "(from, to]"

// RuleFile is a human-authored rule file.
type RuleFile struct {
	Provenance            string            `json:"_provenance"`
	Statement             string            `json:"statement"`
	Assertion             RuleFileAssertion `json:"assertion"`
	Citations             []string          `json:"citations"`
	RequestedVerification string            `json:"requestedVerification"`
}

// RuleFileAssertion is the rule in the fact grammar plus its release bounds.
type RuleFileAssertion struct {
	Subject       *domain.Subject       `json:"subject"`
	Change        *domain.ChangeSpec    `json:"change"`
	Applicability *domain.Applicability `json:"applicability"`
	Consequence   *domain.Consequence   `json:"consequence"`
	Release       RuleFileRelease       `json:"release"`
}

// RuleFileRelease are the rule's release bounds.
type RuleFileRelease struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Bounds string `json:"bounds"`
}

// ParsedRule is a validated rule file.
type ParsedRule struct {
	// Digest is the sha256 of the file's bytes (the rule digest the
	// protocol pre-registers).
	Digest    string
	File      RuleFile
	Assertion domain.SemanticAssertion
	Bounds    domain.ReleaseBounds
	Citations []domain.EvidenceID
}

// Hypothesis is the rule's hypothesis id.
func (r *ParsedRule) Hypothesis() string { return domain.HumanAuthoredHypothesisID(r.Digest) }

var citationID = regexp.MustCompile(`^ev-[0-9a-f]{12}$`)

// ParseRuleFile decodes and validates a rule file.
func ParseRuleFile(data []byte) (*ParsedRule, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f RuleFile
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("intake: rule file: %w", err)
	}
	if dec.More() {
		return nil, errors.New("intake: rule file: trailing data after the rule")
	}
	if err := faithful(data, f); err != nil {
		return nil, err
	}
	a := f.Assertion
	r := &ParsedRule{Digest: domain.Digest(data), File: f, Bounds: domain.ReleaseBounds{From: a.Release.From, To: a.Release.To},
		Assertion: domain.SemanticAssertion{Subject: a.Subject, Change: a.Change, Applicability: a.Applicability, Consequence: a.Consequence,
			Statement: strings.TrimSpace(f.Statement)}}
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("intake: rule file: "+format, args...))
	}
	if err := r.Assertion.Validate(true); err != nil {
		bad("the rule is not a complete, valid fact: %v", err)
	}
	if ap := r.Assertion.Applicability; ap != nil && (undecidable(ap.Exposure) || (ap.Overlap != nil && undecidable(*ap.Overlap))) {
		bad("an applicability condition is undecidable; a rule must be decidable on an environment")
	}
	if a.Release.Bounds != ruleFileBounds {
		bad("release bounds %q: only %q (FROM exclusive, TO inclusive) is expressible", a.Release.Bounds, ruleFileBounds)
	}
	if _, _, err := r.Bounds.Versions(); err != nil {
		bad("%v", err)
	}
	if r.Assertion.Statement == "" {
		bad("statement is required")
	}
	if strings.TrimSpace(f.RequestedVerification) == "" {
		bad("requestedVerification is required")
	}
	seen := map[domain.EvidenceID]bool{}
	for _, c := range f.Citations {
		tok, _, _ := strings.Cut(strings.TrimSpace(c), " ")
		if !citationID.MatchString(tok) {
			bad("citation %q does not start with an evidence id (ev-<12 hex>)", c)
			continue
		}
		if id := domain.EvidenceID(tok); !seen[id] {
			seen[id] = true
			r.Citations = append(r.Citations, id)
		}
	}
	if len(r.Citations) == 0 {
		bad("at least one upstream citation is required")
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return r, nil
}

// faithful refuses a file whose assertion does not survive the fact grammar
// unchanged: the typed decode re-encoded must equal the file's own JSON.
func faithful(data []byte, f RuleFile) error {
	var raw struct {
		Assertion map[string]any `json:"assertion"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("intake: rule file: %w", err)
	}
	typed, err := json.Marshal(f.Assertion)
	if err != nil {
		return err
	}
	var back map[string]any
	if err := json.Unmarshal(typed, &back); err != nil {
		return err
	}
	for _, k := range sortedKeys(raw.Assertion, back) {
		if !reflect.DeepEqual(raw.Assertion[k], back[k]) {
			a, _ := json.Marshal(raw.Assertion[k])
			b, _ := json.Marshal(back[k])
			return fmt.Errorf("intake: rule file: assertion.%s cannot be expressed exactly in the fact grammar (file %s, encoded %s): not taken in", k, a, b)
		}
	}
	return nil
}

func sortedKeys(ms ...map[string]any) []string {
	set := map[string]bool{}
	for _, m := range ms {
		for k := range m {
			set[k] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func undecidable(c domain.Condition) bool {
	if c.Op == domain.OpUndecidable {
		return true
	}
	for _, x := range c.Of {
		if undecidable(x) {
			return true
		}
	}
	return false
}

// IntakeOptions configure an intake.
type IntakeOptions struct {
	// Author is recorded verbatim (required), e.g. "commander-drafted
	// (Claude Opus 5.5), label-aware; adopted by <reviewer> at promotion".
	Author string
	// Candidate names the knowledge candidate to anchor the rule on; ""
	// selects the one candidate of the subject's product, inside the
	// bounds, holding every cited evidence.
	Candidate string
	Now       time.Time
}

// Intake outcomes.
const (
	IntakeTaken    = "taken-in"
	IntakeExisting = "existing"
)

// IntakeResult says what the intake wrote.
type IntakeResult struct {
	File       string `json:"file,omitempty"`
	FileDigest string `json:"fileDigest"`
	Hypothesis string `json:"hypothesis"`
	Candidate  string `json:"candidate"`
	Proposal   string `json:"proposal"`
	ReviewItem string `json:"reviewItem"`
	// RuleDigest is the assertion digest a proof record must name;
	// ProposedRuleDigest also covers the bounds.
	RuleDigest         string               `json:"ruleDigest"`
	ProposedRuleDigest string               `json:"proposedRuleDigest"`
	Bounds             domain.ReleaseBounds `json:"bounds"`
	Status             string               `json:"status"`
	// ItemExisted: a review item proposing exactly this rule was already in
	// the store (e.g. an exported agent hypothesis) and is reused as it is.
	ItemExisted bool `json:"itemExisted,omitempty"`
}

// Intake writes the rule's proposal and whole-fact review item. Idempotent:
// records are content-addressed and existing ones are left as they are.
func Intake(ctx context.Context, s Store, r *ParsedRule, o IntakeOptions) (*IntakeResult, error) {
	if strings.TrimSpace(o.Author) == "" {
		return nil, errors.New("intake: the author string is required (recorded verbatim)")
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	product := r.Assertion.Subject.Product
	snap, err := s.Load(ctx, Query{Product: product, Kinds: []domain.RecordKind{domain.RecordCandidate}})
	if err != nil {
		return nil, err
	}
	var fits []domain.SemanticCandidate
	var named *domain.SemanticCandidate
	for i, c := range snap.Candidates {
		if c.ID == o.Candidate {
			named = &snap.Candidates[i]
		}
		if c.Product != product || !r.Bounds.Contains(c.Release) {
			continue
		}
		ev := map[domain.EvidenceID]bool{}
		for _, e := range c.Evidence {
			ev[e.ID] = true
		}
		all := true
		for _, id := range r.Citations {
			all = all && ev[id]
		}
		if all {
			fits = append(fits, c)
		}
	}
	var cand domain.SemanticCandidate
	switch {
	case o.Candidate != "" && named == nil:
		return nil, fmt.Errorf("intake: candidate %s is not in the store", o.Candidate)
	case o.Candidate != "":
		ok := false
		for _, c := range fits {
			ok = ok || c.ID == o.Candidate
		}
		if !ok {
			return nil, fmt.Errorf("intake: candidate %s (%s %s) does not hold every cited evidence inside (%s, %s]", o.Candidate, named.Product, named.Release, r.Bounds.From, r.Bounds.To)
		}
		cand = *named
	case len(fits) == 0:
		return nil, fmt.Errorf("intake: no %s candidate inside (%s, %s] holds every cited evidence (%v)", product, r.Bounds.From, r.Bounds.To, r.Citations)
	case len(fits) > 1:
		var ids []string
		for _, c := range fits {
			ids = append(ids, c.ID)
		}
		return nil, fmt.Errorf("intake: several candidates hold the cited evidence (%s): name one with -candidate", strings.Join(ids, ", "))
	default:
		cand = fits[0]
	}
	hyp := r.Hypothesis()
	at := o.Now.UTC().Truncate(time.Second)
	bounds := r.Bounds
	// Method stays ai: the proposal contract admits only AI provenance, the
	// least trusted label. Rule, provider and prompt version say
	// human-authored; Model holds the author string verbatim; the prompt
	// digest and call id are the rule file's digest.
	p := domain.SemanticProposal{CandidateID: cand.ID, Task: domain.TaskFull, Provider: HumanAuthoredProvider, Assertion: r.Assertion,
		Citations: r.Citations, Release: &bounds,
		Provenance: domain.Provenance{Method: domain.MethodAI, Producer: IntakeProducer, Rule: domain.HumanAuthoredRuleKind + hyp,
			Confidence: domain.ConfidenceLow, Model: o.Author, ModelVersion: HumanAuthoredProvider, PromptVersion: HumanAuthoredPrompt,
			PromptDigest: r.Digest, InputEvidence: r.Citations, GeneratedAt: &at, CallID: "rule-file:" + r.Digest, Provider: HumanAuthoredProvider}}
	p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	if err := p.ValidateAgainst(cand); err != nil {
		return nil, fmt.Errorf("intake: %w", err)
	}
	pr := domain.ProposedRule{Assertion: r.Assertion, Release: bounds}
	q := fmt.Sprintf("Human-authored rule (not agent-proposed; author: %s; rule file %s, hypothesis %s): %s Release bounds (%s, %s]. "+
		"Citations: %s. Is this whole rule correct — subject, change, applicability and consequence? Requested verification: %s "+
		"Accepting or correcting it needs a confirmed proof record for this exact rule.",
		o.Author, r.Digest, hyp, r.Assertion.Statement, bounds.From, bounds.To, strings.Join(r.File.Citations, "; "), strings.TrimSpace(r.File.RequestedVerification))
	it := domain.ReviewItem{ID: domain.ReviewItemID(cand.ID, domain.QuestionFactReview, r.Assertion), CandidateID: cand.ID, Product: cand.Product,
		Release: cand.Release, QuestionType: domain.QuestionFactReview, Question: q, Proposed: r.Assertion, Proposals: []string{p.ID},
		Routing: domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityNormal, Signals: []domain.RoutingSignal{domain.SignalSingleModel}},
		Status:  domain.ReviewPending, CreatedAt: at}
	res := &IntakeResult{FileDigest: r.Digest, Hypothesis: hyp, Candidate: cand.ID, Proposal: p.ID, ReviewItem: it.ID,
		RuleDigest: r.Assertion.Digest(), ProposedRuleDigest: pr.Digest(), Bounds: bounds, Status: IntakeTaken}
	pExisting, err := putNew(ctx, s, p, p.ID)
	if err != nil {
		return nil, err
	}
	iExisting, err := putNew(ctx, s, it, it.ID)
	if err != nil {
		return nil, err
	}
	res.ItemExisted = iExisting
	if pExisting && iExisting {
		res.Status = IntakeExisting
	}
	return res, nil
}

// putNew writes a record unless one with its id exists (reported).
func putNew(ctx context.Context, s Store, entity any, id string) (bool, error) {
	if _, err := s.Get(ctx, id); err == nil {
		return true, nil
	} else if !errors.Is(err, ErrNotFound) {
		return false, err
	}
	rec, err := domain.NewRecord(entity)
	if err != nil {
		return false, err
	}
	return false, s.Put(ctx, rec)
}
