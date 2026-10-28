package synthesize

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Answer is the model's structured answer (prompts/synthesize/v1/schema.json).
type Answer struct {
	Recommendation      domain.Recommendation `json:"recommendation"`
	Headline            string                `json:"headline"`
	Scope               string                `json:"scope"`
	Preconditions       []string              `json:"preconditions"`
	TargetVersion       string                `json:"targetVersion"`
	Rationale           string                `json:"rationale"`
	Changes             []AnswerChange        `json:"changes"`
	Validation          []AnswerTest          `json:"validation"`
	ResidualRisks       []AnswerRisk          `json:"residualRisks"`
	FindingComments     []AnswerComment       `json:"findingComments"`
	InteractionComments []AnswerComment       `json:"interactionComments"`
	Verified            []AnswerVerified      `json:"verified"`
	Inferred            []string              `json:"inferred"`
	Unknown             []string              `json:"unknown"`
	BackoutNotes        string                `json:"backoutNotes"`
}

// AnswerChange is one step of the answer.
type AnswerChange struct {
	Kind        domain.StepKind `json:"kind"`
	Summary     string          `json:"summary"`
	ChangeItems []string        `json:"changeItems"`
	Location    string          `json:"location"`
}

// AnswerTest is one required test of the answer.
type AnswerTest struct {
	Test        string   `json:"test"`
	Expected    string   `json:"expected"`
	Basis       []string `json:"basis"`
	ChangeItems []string `json:"changeItems"`
}

// AnswerRisk is one residual risk the model names.
type AnswerRisk struct {
	Statement  string   `json:"statement"`
	ChangeItem string   `json:"changeItem"`
	Refs       []string `json:"refs"`
}

// AnswerComment comments on a finding or an interaction by id.
type AnswerComment struct {
	ID      string `json:"id"`
	Comment string `json:"comment"`
}

// AnswerVerified is a verified statement with its citations.
type AnswerVerified struct {
	Statement string   `json:"statement"`
	Citations []string `json:"citations"`
}

// decode reads an answer strictly: unknown fields (a rewritten finding, a
// location written as a path) are a violation, not ignored.
func decode(text string) (*Answer, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(strings.TrimSpace(text))))
	dec.DisallowUnknownFields()
	var a Answer
	if err := dec.Decode(&a); err != nil {
		return nil, fmt.Errorf("the answer is not the schema's JSON document: %v", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("the answer has content after the JSON document")
	}
	return &a, nil
}

// Floor checks an answer against the deterministic rules of PLAN-M3 §2 and
// returns every violation (none: the answer may be accepted):
//
//   - proceed is impossible while a hard finding exists;
//   - the target version is the proposed target or a computed option, and a
//     target with a regression (an unsafe option) is never recommended;
//   - proceed and proceed-with-changes keep the proposed target,
//     modify-target changes it and names it in the headline;
//   - every reference resolves: finding, interaction, test, location, change
//     item and evidence ids;
//   - a change location is a computed location (or none: a proposed step).
func Floor(p *Precomputed, a *Answer) []string {
	var v []string
	bad := func(format string, args ...any) { v = append(v, fmt.Sprintf(format, args...)) }
	known := func(list []domain.Recommendation, r domain.Recommendation) bool {
		for _, x := range list {
			if x == r {
				return true
			}
		}
		return false
	}
	if !known(domain.Recommendations, a.Recommendation) {
		bad("recommendation %q is not one of %v", a.Recommendation, domain.Recommendations)
	}
	if strings.TrimSpace(a.Headline) == "" {
		bad("the headline is empty")
	}
	hard := 0
	for _, f := range p.Findings {
		if f.Hard {
			hard++
		}
	}
	if a.Recommendation == domain.RecommendProceed && hard > 0 {
		bad("proceed is impossible while %d hard finding(s) exist; choose another recommendation", hard)
	}

	t := p.Target
	chosen, err := p.option(a.TargetVersion)
	switch {
	case err != nil:
		bad("%v", err)
	case chosen != nil && !chosen.Safe:
		bad("targetVersion %s is not safe (it introduces %s, or its security status is not established); a target with a regression cannot be recommended", chosen.Version, orNone(strings.Join(chosen.Introduces, ", ")))
	}
	if err == nil {
		isProposed := chosen != nil && chosen.Version.Equal(t.Proposed)
		switch a.Recommendation {
		case domain.RecommendProceed, domain.RecommendProceedWithChanges:
			if !isProposed {
				bad("%s takes the proposed target %s as targetVersion", a.Recommendation, t.Proposed)
			}
		case domain.RecommendModifyTarget:
			if chosen == nil || isProposed {
				bad("modify-target takes a computed option other than the proposed target %s as targetVersion", t.Proposed)
			} else if !mentions(a.Headline, chosen.Version) {
				bad("a modify-target headline names the target version %s", chosen.Version)
			}
		}
	}

	ids := p.ids()
	ref := func(what, id string, kinds ...string) {
		for _, k := range kinds {
			if ids[k][id] {
				return
			}
		}
		bad("%s: %q is not an id of the input (%s)", what, id, strings.Join(kinds, ", "))
	}
	items := func(what string, xs []string) {
		for _, x := range xs {
			ref(what, x, "item")
		}
	}
	for i, c := range a.Changes {
		what := fmt.Sprintf("changes[%d]", i)
		if !knownStep(c.Kind) || strings.TrimSpace(c.Summary) == "" {
			bad("%s: a known kind and a summary are required", what)
		}
		if c.Location != "" {
			ref(what+" location", c.Location, "location")
		}
		items(what+" changeItems", c.ChangeItems)
	}
	for i, x := range a.Validation {
		what := fmt.Sprintf("validation[%d]", i)
		if strings.TrimSpace(x.Test) == "" || strings.TrimSpace(x.Expected) == "" || len(x.Basis) == 0 {
			bad("%s: a test, its expected result and its basis are required", what)
		}
		for _, b := range x.Basis {
			ref(what+" basis", b, "test", "finding")
		}
		items(what+" changeItems", x.ChangeItems)
	}
	if len(a.Validation) == 0 && a.Recommendation != domain.RecommendDoNotProceed {
		bad("validation lists no test required before proceeding")
	}
	for i, r := range a.ResidualRisks {
		what := fmt.Sprintf("residualRisks[%d]", i)
		if strings.TrimSpace(r.Statement) == "" {
			bad("%s: the statement is empty", what)
		}
		if r.ChangeItem != "" {
			ref(what+" changeItem", r.ChangeItem, "item")
		}
		for _, x := range r.Refs {
			ref(what+" refs", x, "finding", "test", "location", "item", "risk", "interaction")
		}
	}
	for i, c := range a.FindingComments {
		ref(fmt.Sprintf("findingComments[%d]", i), c.ID, "finding")
	}
	for i, c := range a.InteractionComments {
		ref(fmt.Sprintf("interactionComments[%d]", i), c.ID, "interaction")
	}
	for i, x := range a.Verified {
		what := fmt.Sprintf("verified[%d]", i)
		if len(x.Citations) == 0 {
			bad("%s: a verified statement cites evidence ids (otherwise it is inferred)", what)
		}
		for _, c := range x.Citations {
			ref(what+" citations", c, "evidence")
		}
	}
	return v
}

func knownStep(k domain.StepKind) bool {
	for _, x := range domain.StepKinds {
		if x == k {
			return true
		}
	}
	return false
}

// option resolves an answer's targetVersion: nil for none, the option whose
// semver or tag it names, or an error for a version that is not an option.
func (p *Precomputed) option(s string) (*domain.TargetOption, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	for i := range p.Target.Options {
		o := &p.Target.Options[i]
		if s == o.Version.Semver || s == o.Version.Tag || strings.TrimPrefix(s, "v") == o.Version.Semver {
			return o, nil
		}
	}
	var names []string
	for _, o := range p.Target.Options {
		names = append(names, o.Version.String())
	}
	return nil, fmt.Errorf("targetVersion %q is not one of the computed options (%s)", s, strings.Join(names, ", "))
}

// mentions reports whether text names the version (its tag or semver).
func mentions(text string, v domain.Version) bool {
	return (v.Semver != "" && strings.Contains(text, v.Semver)) || (v.Tag != "" && strings.Contains(text, v.Tag))
}

// ids are the input's ids by kind.
func (p *Precomputed) ids() map[string]map[string]bool {
	m := map[string]map[string]bool{}
	add := func(kind, id string) {
		if m[kind] == nil {
			m[kind] = map[string]bool{}
		}
		m[kind][id] = true
	}
	for _, f := range p.Findings {
		add("finding", f.ID)
	}
	for _, l := range p.Locations {
		add("location", l.ID)
	}
	for _, t := range p.Tests {
		add("test", t.ID)
	}
	for _, it := range p.Items {
		add("item", it.ChangeItem)
	}
	for _, r := range p.ResidualRisks {
		add("risk", r.ID)
	}
	for _, x := range p.Interactions {
		add("interaction", x.ID)
	}
	for _, e := range p.Evidence {
		add("evidence", string(e.ID))
	}
	return m
}
