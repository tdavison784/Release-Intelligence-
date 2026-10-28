package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Behaviour dependencies (prompt feature behaviour; docs/phase6/
// decision-intel/briefs/upstream2.md). The assessment lists the upstream
// behaviours its verdict relies on, each with its source: upstream (cited
// upstream text beyond the item's change text: a fetched run artifact or an
// upstream source of the bundle), change-text (a verbatim quote of the
// item's own change text), unavailable (an upstream request of the item that
// was recorded unavailable) or not-checked. The tool layer checks only these
// structured fields, the change classification, the request records and the
// citations, never the wording: a not-affected verdict is rejected while any
// behaviour is unavailable or not-checked, and once when a behavioural item
// declares none. Each unestablished behaviour becomes a gap in Unknown.
// Feature fetchFirst (briefs/upstream3.md) adds the fetch-first check.

// behaviourGap prefixes the tool layer's unknown entry for an unestablished
// behaviour.
const behaviourGap = "investigation gap (recorded by the tool layer): behaviour "

// behaviouralChange reports whether the item's change is behavioural, by the
// bundle's declared classification (bundle/scope.go): its manifestation is
// controller-behaviour (runtime or default semantics of the shipped
// software) or unknown (treated as controller-behaviour, fail closed), or its
// family is behavior (runtime behaviour stated in prose, even when a rendered
// change is correlated to it). The others (values, schema, images, platform
// bounds, chart-rendered changes, procedures) take effect in declared
// configuration the render decides; documentation-only changes are out of
// scope and get no session.
func behaviouralChange(it domain.ChangeItem) bool {
	return !it.Manifestation.RenderDecisive() || it.Family == domain.FamilyBehavior
}

// changeText is the item's own change text: its title, the upstream change's
// title and detail, and the excerpts of the item's upstream evidence.
func (s *Session) changeText(ctx context.Context, it domain.ChangeItem) ([]string, error) {
	out := []string{it.Title}
	if it.ChangeID != "" {
		edge, err := s.upgradeEdge(ctx)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err == nil { // an unreadable edge leaves less text to quote (fail closed)
			for _, c := range edge.Changes {
				if c.ID == it.ChangeID {
					out = append(out, c.Title, c.Detail)
				}
			}
		}
	}
	for _, id := range it.Evidence {
		if e, ok := s.pool[id]; ok {
			out = append(out, e.Excerpt)
		}
	}
	return out, nil
}

func quotedIn(texts []string, quote string) bool {
	for _, t := range texts {
		if investigate.QuoteOccurs(t, quote) {
			return true
		}
	}
	return false
}

// behaviourDeps are an entry's checked behaviour dependencies and the
// evidence their upstream citations became.
type behaviourDeps struct {
	list     []domain.AssessmentBehaviour
	evidence []domain.Evidence
}

// behaviourOf checks an entry's behaviour list (caller holds s.mu; nothing
// is committed here). Any malformed entry rejects the call, every time.
func (s *Session) behaviourOf(ctx context.Context, at string, it domain.ChangeItem, in []investigate.BehaviourInput) (behaviourDeps, error) {
	if !s.cfg.Prompt.Features.Behaviour {
		if in != nil {
			return behaviourDeps{}, rejectf("%s: this prompt version (%s) takes no behaviour", at, s.cfg.Prompt.Version)
		}
		return behaviourDeps{}, nil
	}
	list, evs, err := s.behaviourList(ctx, at, it, in)
	return behaviourDeps{list: list, evidence: evs}, err
}

func (s *Session) behaviourList(ctx context.Context, at string, it domain.ChangeItem, in []investigate.BehaviourInput) ([]domain.AssessmentBehaviour, []domain.Evidence, error) {
	if len(in) > maxAssessmentList {
		return nil, nil, rejectf("%s.behaviour: at most %d entries", at, maxAssessmentList)
	}
	var text []string
	var out []domain.AssessmentBehaviour
	var evs []domain.Evidence
	for i, b := range in {
		where := fmt.Sprintf("%s.behaviour[%d]", at, i)
		stmt, err := s.statement(where+".behaviour (the upstream behaviour the verdict relies on)", b.Behaviour)
		if err != nil {
			return nil, nil, err
		}
		if text == nil {
			if text, err = s.changeText(ctx, it); err != nil {
				return nil, nil, err
			}
		}
		e := domain.AssessmentBehaviour{Behaviour: stmt, Source: b.Source}
		quote, request := trimmed(b.Quote), trimmed(b.Request)
		if trimmed(b.Reason) != "" && b.Source != domain.BehaviourNotChecked {
			return nil, nil, rejectf("%s: only source not-checked takes reason", where)
		}
		switch b.Source {
		case domain.BehaviourUpstream:
			if quote != "" || request != "" {
				return nil, nil, rejectf("%s: source upstream takes citations, not quote or request", where)
			}
			if len(b.Citations) == 0 {
				return nil, nil, rejectf("%s: source upstream needs citations of the upstream text that states the behaviour (a fetched upstream-document, or an upstream source of the bundle)", where)
			}
			beyond := false
			for k, c := range b.Citations {
				if a, ok := s.artifact(c.Artifact); ok && a.Origin != domain.OriginUpstream {
					return nil, nil, rejectf("%s.citations[%d]: artifact %s (%s) is not upstream evidence; cite the upstream implementation, documentation or tests (request_evidence upstream-file / upstream-search), or an upstream source of the bundle", where, k, a.ID, a.Path)
				}
				beyond = beyond || !quotedIn(text, c.Quote)
			}
			cs, err := s.citations(ctx, b.Citations)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", where, err)
			}
			if !beyond {
				return nil, nil, rejectf("%s: every citation quotes the item's own change text; use source change-text when that text states the behaviour itself, or cite upstream text beyond it", where)
			}
			e.Citations = evidenceIDs(cs)
			evs = append(evs, cs...)
		case domain.BehaviourChangeText:
			if len(b.Citations) > 0 || request != "" {
				return nil, nil, rejectf("%s: source change-text takes quote, not citations or request", where)
			}
			if quote == "" {
				return nil, nil, rejectf("%s: source change-text needs quote: the words of the item's change text that state the behaviour itself", where)
			}
			if len(quote) > MaxQuote {
				return nil, nil, rejectf("%s.quote is %d bytes (max %d)", where, len(quote), MaxQuote)
			}
			if !quotedIn(text, quote) {
				return nil, nil, rejectf("%s: the quote does not occur in the item's change text (its title, the change's detail, the excerpts of its upstream evidence); copy it verbatim, or cite other upstream text with source upstream", where)
			}
			e.Quote = quote
		case domain.BehaviourUnavailable:
			if len(b.Citations) > 0 || quote != "" {
				return nil, nil, rejectf("%s: source unavailable takes request, not citations or quote", where)
			}
			found := false
			for _, r := range s.evReqs {
				found = found || (r.ID == request && r.ChangeItem == it.ID && r.Upstream != nil && r.Status == domain.RequestUnavailable)
			}
			if !found {
				return nil, nil, rejectf("%s: request %q is not an upstream request_evidence of this item recorded unavailable; name the id of such a request, or use source not-checked", where, request)
			}
			e.Request = request
		case domain.BehaviourNotChecked:
			if len(b.Citations) > 0 || quote != "" || request != "" {
				return nil, nil, rejectf("%s: source not-checked takes no citations, quote or request", where)
			}
			if trimmed(b.Reason) != "" {
				if !s.cfg.Prompt.Features.FetchFirst {
					return nil, nil, rejectf("%s: this prompt version (%s) takes no reason", where, s.cfg.Prompt.Version)
				}
				if e.Reason, err = s.statement(where+".reason (why the upstream source cannot answer the behaviour)", b.Reason); err != nil {
					return nil, nil, err
				}
			}
		default:
			return nil, nil, rejectf("%s.source %q: want %s", where, b.Source, strings.Join(domain.BehaviourSources, ", "))
		}
		out = append(out, e)
	}
	return out, evs, nil
}

// fetchFirst (prompt feature fetchFirst) asks for the fetch before the
// conclusion. While the session can fetch upstream behaviour for the item (a
// fetch port, the product's repositories, budget left) and the item is
// behavioural, a not-checked behaviour is rejected, whatever the verdict:
// once while the item made no upstream request (resubmitted, it is accepted,
// with the reason the upstream source cannot answer it when the entry gives
// one), and every time after a request, when each behaviour is cited
// (upstream) or names a request recorded unavailable. Only the structured
// fields and the request records are read, never the wording.
func (s *Session) fetchFirst(at string, it domain.ChangeItem, bs []domain.AssessmentBehaviour) error {
	if !s.cfg.Prompt.Features.FetchFirst || !behaviouralChange(it) || s.cfg.Upstream == nil || s.exhausted != "" {
		return nil
	}
	sc := s.upstreamScope()
	if len(sc.repos) == 0 {
		return nil
	}
	var open []string
	for i, b := range bs {
		if b.Source == domain.BehaviourNotChecked {
			open = append(open, fmt.Sprintf("behaviour[%d] %q", i, b.Behaviour))
		}
	}
	if len(open) == 0 {
		return nil
	}
	var reqs []string
	for _, r := range s.evReqs {
		if r.ChangeItem == it.ID && r.Upstream != nil {
			reqs = append(reqs, fmt.Sprintf("%s (%s)", r.ID, r.Status))
		}
	}
	if len(reqs) > 0 {
		return rejectf("%s: this session requested upstream evidence for item %s (%s), so each behaviour is cited or reported unavailable; %d are not-checked: %s. Cite the fetched text with source upstream, name a request recorded unavailable with source unavailable, or request the behaviour first (request_evidence kind upstream-search or upstream-file).",
			at, it.ID, strings.Join(reqs, ", "), len(open), strings.Join(open, "; "))
	}
	if s.rejectedNoFetch[it.ID] {
		return nil
	}
	s.rejectedNoFetch[it.ID] = true // recorded although the call is rejected: the check rejects once
	return rejectf("%s: %d behaviours are not-checked and this session made no upstream request for item %s: %s. Upstream fetching is available for %s at %s (target) and %s (current): request the behaviour, or state in the entry why upstream source cannot answer it. Request it with request_evidence (kind upstream-search or upstream-file) and cite it with source upstream, or name the request with source unavailable if it is recorded unavailable; or keep source not-checked and give reason. A resubmission is accepted, with each not-checked behaviour recorded as a gap.",
		at, len(open), it.ID, strings.Join(open, "; "), strings.Join(sc.repos, ", "), sc.target, sc.current)
}

// behaviourVerdictCheck rejects a not-affected verdict, every time, while a
// behaviour it relies on is unavailable or not-checked.
func behaviourVerdictCheck(at string, verdict *domain.AgentVerdict, bs []domain.AssessmentBehaviour) error {
	if verdict == nil || verdict.Class != domain.FindingNotAffected {
		return nil
	}
	var open []string
	for i, b := range bs {
		if b.Gap() {
			open = append(open, fmt.Sprintf("behaviour[%d] %q (%s)", i, b.Behaviour, b.Source))
		}
	}
	if len(open) == 0 {
		return nil
	}
	return rejectf("%s: a not-affected verdict cannot rest on upstream behaviour that is not established; %d behaviours are unavailable or not-checked: %s. Fetch the behaviour at the target's tag (request_evidence kind upstream-search or upstream-file) and cite it with source upstream, or submit the class the gap leaves (potentially-affected, insufficient-evidence); the gap is recorded in unknown.",
		at, len(open), strings.Join(open, "; "))
}

// behaviourCompletion rejects once a not-affected verdict on a behavioural
// item that declares no behaviour.
func (s *Session) behaviourCompletion(at string, it domain.ChangeItem, verdict *domain.AgentVerdict, bs []domain.AssessmentBehaviour) error {
	if !s.cfg.Prompt.Features.Behaviour || verdict == nil || verdict.Class != domain.FindingNotAffected || len(bs) > 0 ||
		!behaviouralChange(it) || s.rejectedNoBehaviour[it.ID] {
		return nil
	}
	s.rejectedNoBehaviour[it.ID] = true // recorded although the call is rejected: the check rejects once
	return rejectf("%s: item %s is a behavioural change (manifestation %s, family %s) and the not-affected verdict declares no behaviour. List in behaviour each upstream behaviour the verdict relies on (validation, defaulting, parsing, feature gates, API semantics) with its source: upstream (cite the fetched or bundled upstream text), change-text (quote the item's change text where it states the behaviour itself), unavailable (the id of the request recorded unavailable) or not-checked. A second submission is accepted.",
		at, it.ID, it.Manifestation, it.Family)
}

// behaviourGaps are the unknown entries of the unestablished behaviours.
func (s *Session) behaviourGaps(bs []domain.AssessmentBehaviour) []string {
	var out []string
	target := ""
	for _, b := range bs {
		switch b.Source {
		case domain.BehaviourNotChecked:
			if target == "" {
				target = s.upstreamScope().target
			}
			gap := fmt.Sprintf("%s%q at %s: %s", behaviourGap, b.Behaviour, target, b.Source)
			if b.Reason != "" {
				gap += " (reason: " + b.Reason + ")"
			}
			out = append(out, gap)
		case domain.BehaviourUnavailable:
			for _, r := range s.evReqs {
				if r.ID == b.Request {
					out = append(out, fmt.Sprintf("%s%q at %s: %s (request %s: %s)", behaviourGap, b.Behaviour, r.Upstream.Ref, b.Source, r.ID, r.Resolution))
				}
			}
		}
	}
	return out
}
