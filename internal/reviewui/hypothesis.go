package reviewui

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// Agent-proposed rules (Phase 5, lane verify): a hypothesis-sourced
// fact-review item shows the proposed rule in plain English, the agent's
// reasoning labelled AI, every proof record (each method with its level,
// outcome, evidence and the rule exercise's two arms), the limitations, and
// the promotion gate. When other active facts state the same subject, the
// reviewer says per fact whether the promoted rule supersedes it or coexists
// with it; the gate refuses a promotion that leaves one unaccounted for.

type hypothesisView struct {
	Hypothesis, Investigation string
	// HumanAuthored: the rule was taken in from a human-authored rule file
	// (`ri knowledge intake-rule`), not proposed by the agent.
	HumanAuthored bool
	Rule          []string // plain-English lines of the proposed rule
	Agent         []agentReasoning
	Proofs        []proofView
	GateOpen      bool
	GateReason    string
	Related       []relatedFact
}

type agentReasoning struct {
	Text, Model, Prompt, Call string
}

type proofView struct {
	ID, Outcome, Level, CheckedAt, Bounds string
	Methods                               []methodView
	Limitations                           []string
	// ObservedDefaults are the defaults the proof observed (PD-5): shown
	// with their limitation before the reviewer decides.
	ObservedDefaults []domain.ObservedDefault
}

type methodView struct {
	Method, Level, LevelLabel, Outcome, Detail string
	Checks                                     []domain.ProofCheck
	Arms                                       []domain.ProofArm
	Evidence                                   []proofLink
}

type proofLink struct{ Text, Href, Digest string }

type relatedFact struct {
	ID, Statement, Level, Choice string
}

// relationField is the form field naming how the decision treats an active
// same-subject fact: "supersedes" or "coexists".
func relationField(factID string) string { return "relation-" + factID }

func newHypothesisView(rc *knowledge.ReviewContext, form url.Values) *hypothesisView {
	g := rc.Promotion
	if g == nil {
		return nil
	}
	a := rc.Item.Proposed
	v := &hypothesisView{Hypothesis: g.Hypothesis, Investigation: g.Investigation, GateOpen: g.Open, GateReason: g.Reason}
	v.Rule = append(v.Rule, "Subject: "+subjectPhrase(a.Subject)+".", changeSentence(a))
	if a.Applicability != nil {
		v.Rule = append(v.Rule, "Exposed: "+exposurePhrase(a.Applicability)+".")
		if a.Applicability.Overlap != nil {
			v.Rule = append(v.Rule, "Preserved (the proposed mitigation): "+overlapPhrase(a.Applicability)+".")
		}
	}
	v.Rule = append(v.Rule, consequenceSentence(a.Consequence))
	for _, p := range rc.Proposals {
		if _, _, ok := knowledge.ParseHypothesisRule(p.Provenance.Rule); !ok {
			continue
		}
		text := strings.TrimSpace(p.Assertion.Statement)
		if text == "" {
			text = "(no statement)"
		}
		v.HumanAuthored = v.HumanAuthored || knowledge.HumanAuthored(p.Provenance.Rule)
		v.Agent = append(v.Agent, agentReasoning{Text: text, Model: p.Provenance.Model, Prompt: p.Provenance.PromptVersion, Call: p.Provenance.CallID})
	}
	for i := len(g.Proofs) - 1; i >= 0; i-- {
		v.Proofs = append(v.Proofs, newProofView(g.Proofs[i]))
	}
	self := domain.VerifiedFactID(rc.Item.Product, rc.Item.Release, a)
	for _, f := range rc.RelatedFacts {
		if f.Status != domain.FactActive || f.ID == self || f.Assertion.Subject == nil || a.Subject == nil || f.Assertion.Subject.Key() != a.Subject.Key() {
			continue
		}
		v.Related = append(v.Related, relatedFact{ID: f.ID, Statement: truncate(f.Assertion.Statement, 220), Level: string(f.Level()),
			Choice: form.Get(relationField(f.ID))})
	}
	sort.Slice(v.Related, func(i, j int) bool { return v.Related[i].ID < v.Related[j].ID })
	return v
}

func newProofView(p domain.ProofRecord) proofView {
	pv := proofView{ID: p.ID, Outcome: string(p.Outcome), Level: string(p.Level), CheckedAt: p.CheckedAt.Format("2006-01-02 15:04 MST"),
		Limitations: p.Limitations, ObservedDefaults: p.ObservedDefaults}
	if b := p.VersionBounds; b != nil {
		pv.Bounds = fmt.Sprintf("(%s, %s]", b.From, b.To)
	}
	for _, m := range p.Methods {
		mv := methodView{Method: string(m.Method), Level: string(m.Level), Outcome: string(m.Outcome), Detail: m.Detail, Checks: m.Checks, Arms: m.Arms}
		switch m.Level {
		case domain.LevelDeterministic:
			mv.LevelLabel = "deterministic — a validator proved it from artifacts"
		case domain.LevelObserved:
			mv.LevelLabel = "observed — a runtime outcome in an isolated cluster"
		}
		for _, e := range m.Evidence {
			l := proofLink{Text: e.URI, Digest: e.ContentDigest}
			if strings.HasPrefix(e.URI, "https://") {
				l.Href = e.URI
			}
			if e.Locator != "" && !strings.HasPrefix(e.URI, "https://") {
				l.Text = e.Locator
			}
			mv.Evidence = append(mv.Evidence, l)
		}
		pv.Methods = append(pv.Methods, mv)
	}
	return pv
}

// relations reads the reviewer's supersedes / coexists choices for the
// same-subject facts the page offered; any other fact id is ignored (the
// gate checks the decision against the store).
func relations(rc *knowledge.ReviewContext, form url.Values) (supersedes, coexists []string, err error) {
	v := newHypothesisView(rc, url.Values{})
	if v == nil {
		return nil, nil, nil
	}
	for _, f := range v.Related {
		switch c := strings.TrimSpace(form.Get(relationField(f.ID))); c {
		case "supersedes":
			supersedes = append(supersedes, f.ID)
		case "coexists":
			coexists = append(coexists, f.ID)
		case "":
		default:
			return nil, nil, fmt.Errorf("unknown relation %q for fact %s (supersedes or coexists)", c, f.ID)
		}
	}
	return supersedes, coexists, nil
}
