package formalize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Producer is the provenance producer of a formalization and its
// hypotheses; PromptVersion the default prompt.
const (
	Producer      = "formalize@v1"
	PromptVersion = "formalize/v1"
)

// UncertaintyIDPrefix prefixes uncertainty ids.
const UncertaintyIDPrefix = "unc-"

// Kinds of uncertainty an assessment leaves unresolved.
const (
	UncertaintyUnknown      = "unknown"      // an entry of the assessment's unknown list
	UncertaintyBehaviour    = "behaviour"    // a not-checked or unavailable behaviour no unknown entry states
	UncertaintySetting      = "setting"      // an effective-configuration setting in state unknown
	UncertaintyDefault      = "default"      // a setting left to an inherited default (no evidenced default yet)
	UncertaintyPrecondition = "precondition" // a precondition of the assessment's scoped verdict
)

// Input is what one formalization reads: a run record, its bundle, the
// assessment to formalize and a reader of artifact bytes (the spec checks
// look the rule's subject up in the bundle).
type Input struct {
	Investigation *domain.Investigation
	Bundle        *domain.EvidenceBundle
	Assessment    string // aas-…
	Read          func(ctx context.Context, a domain.ArtifactRef) ([]byte, error)
}

// Precomputed is the deterministic input of the call: the model sees its
// JSON form; the floor checks the answer against it.
type Precomputed struct {
	Investigation string          `json:"investigation"`
	Transition    Transition      `json:"transition"`
	Item          Item            `json:"item"`
	Assessment    Assessment      `json:"assessment"`
	Uncertainties []Uncertainty   `json:"uncertainties"`
	Basis         []Basis         `json:"environmentBasis"`
	Evidence      []CitableRecord `json:"evidence"`

	in      Input
	item    domain.ChangeItem
	pool    map[domain.EvidenceID]domain.Evidence
	allowed map[domain.EvidenceID]bool
	arts    map[string]domain.ArtifactRef
}

// Transition is the upgrade the assessment belongs to.
type Transition struct {
	Product domain.ProductID `json:"product"`
	From    string           `json:"from"`
	To      string           `json:"to"`
}

// Item is the change item, with its change text.
type Item struct {
	ID            string                  `json:"id"`
	Title         string                  `json:"title"`
	Family        domain.ChangeItemFamily `json:"family"`
	Manifestation domain.Manifestation    `json:"manifestation"`
	Subjects      []string                `json:"subjects,omitempty"`
	ChangeText    []string                `json:"changeText,omitempty"` // excerpts of the item's evidence
}

// Assessment is the natural assessment as the model reads it (its id,
// verdict link and provenance left out).
type Assessment struct {
	ID                     string                       `json:"id"`
	Headline               string                       `json:"headline,omitempty"`
	Scope                  string                       `json:"scope,omitempty"`
	Preconditions          []string                     `json:"preconditions,omitempty"`
	WhatChanged            string                       `json:"whatChanged"`
	Mechanism              string                       `json:"mechanism"`
	Consumers              []string                     `json:"consumers,omitempty"`
	EffectiveConfiguration []domain.AssessmentSetting   `json:"effectiveConfiguration,omitempty"`
	Verified               []domain.AssessmentFinding   `json:"verified,omitempty"`
	Inferred               []string                     `json:"inferred,omitempty"`
	Unknown                []string                     `json:"unknown,omitempty"`
	Behaviour              []domain.AssessmentBehaviour `json:"behaviour,omitempty"`
	LikelyImpact           string                       `json:"likelyImpact"`
	RecommendedTests       []string                     `json:"recommendedTests,omitempty"`
}

// Uncertainty is one thing the assessment leaves unresolved; the answer
// must keep every one (an undecidable leaf or an unrepresentable record).
type Uncertainty struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Basis is a verified fact of this environment a rule may name as its
// environment basis.
type Basis struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"` // claim | validation
	Summary string `json:"summary"`
}

// CitableRecord is an evidence record the answer may cite.
type CitableRecord struct {
	ID       domain.EvidenceID `json:"id"`
	Kind     string            `json:"kind"`
	URI      string            `json:"uri"`
	Locator  string            `json:"locator,omitempty"`
	Excerpt  string            `json:"excerpt,omitempty"`
	Upstream bool              `json:"upstream"`
}

const maxExcerpt = 600

// Precompute builds the call's input from the run record. It reads no
// artifact and changes nothing.
func Precompute(in Input) (*Precomputed, error) {
	inv, b := in.Investigation, in.Bundle
	if inv == nil || b == nil || inv.Bundle != b.ID {
		return nil, errors.New("formalize: the investigation and its bundle are required")
	}
	var a *domain.AgentAssessment
	for i := range inv.AgentAssessments {
		if inv.AgentAssessments[i].ID == in.Assessment {
			a = &inv.AgentAssessments[i]
		}
	}
	if a == nil {
		return nil, fmt.Errorf("formalize: investigation %s has no assessment %s", inv.ID, in.Assessment)
	}
	it, ok := b.Item(a.ChangeItem)
	if !ok {
		return nil, fmt.Errorf("formalize: assessment %s is about %s, which is not an item of bundle %s", a.ID, a.ChangeItem, b.ID)
	}
	p := &Precomputed{Investigation: inv.ID, in: in, item: it, pool: b.EvidenceByID(), allowed: map[domain.EvidenceID]bool{},
		arts: map[string]domain.ArtifactRef{},
		Transition: Transition{Product: b.Change.Product.ID, From: b.Change.From.Tag, To: b.Change.To.Tag},
		Item:       Item{ID: it.ID, Title: it.Title, Family: it.Family, Manifestation: it.Manifestation, Subjects: it.Subjects},
		Assessment: Assessment{ID: a.ID, Headline: a.Headline, Scope: a.Scope, Preconditions: a.Preconditions, WhatChanged: a.WhatChanged,
			Mechanism: a.Mechanism, Consumers: a.Consumers, EffectiveConfiguration: a.EffectiveConfiguration, Verified: a.Verified,
			Inferred: a.Inferred, Unknown: a.Unknown, Behaviour: a.Behaviour, LikelyImpact: a.LikelyImpact, RecommendedTests: a.RecommendedTests}}
	for _, e := range inv.Evidence {
		if _, ok := p.pool[e.ID]; !ok {
			p.pool[e.ID] = e
		}
	}
	for _, x := range append(append([]domain.ArtifactRef(nil), b.Manifest.Artifacts...), inv.Artifacts...) {
		if _, ok := p.arts[x.ID]; !ok {
			p.arts[x.ID] = x
		}
	}
	for _, id := range it.Evidence {
		if e, ok := p.pool[id]; ok && e.Excerpt != "" {
			p.Item.ChangeText = append(p.Item.ChangeText, clip(e.Excerpt, maxExcerpt))
		}
	}

	// The citable evidence (mission D4): what the assessment's verified and
	// behaviour entries cite, the item's change text when a behaviour quotes
	// it, and the evidence of the run's verified setting claims about the
	// item (its effective configuration).
	for _, f := range a.Verified {
		p.allow(f.Citations...)
	}
	for _, x := range a.Behaviour {
		p.allow(x.Citations...)
		if x.Source == domain.BehaviourChangeText {
			p.allow(it.Evidence...)
		}
	}
	for _, c := range inv.AgentClaims {
		if c.ChangeItem == "" || c.ChangeItem == it.ID {
			p.allow(c.Evidence...)
			p.Basis = append(p.Basis, Basis{ID: c.ID, Kind: "claim", Summary: clip(fmt.Sprintf("%s %s in %s: %s", c.Kind, c.Setting, c.Input, c.Detail), maxExcerpt)})
		}
	}
	for _, v := range inv.Validations {
		if v.ChangeItem == it.ID {
			p.Basis = append(p.Basis, Basis{ID: v.ID, Kind: "validation", Summary: clip(fmt.Sprintf("%s %s: %s", v.Kind, v.Outcome, v.Property), maxExcerpt)})
		}
	}
	sort.Slice(p.Basis, func(i, j int) bool { return p.Basis[i].ID < p.Basis[j].ID })
	ids := make([]string, 0, len(p.allowed))
	for id := range p.allowed {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, id := range ids {
		e := p.pool[domain.EvidenceID(id)]
		p.Evidence = append(p.Evidence, CitableRecord{ID: e.ID, Kind: string(e.Kind), URI: e.URI, Locator: e.Locator,
			Excerpt: clip(e.Excerpt, maxExcerpt), Upstream: p.upstream(e)})
	}
	p.Uncertainties = uncertainties(*a)
	return p, nil
}

// allow adds the ids that resolve in the run's pool to the citable set.
func (p *Precomputed) allow(ids ...domain.EvidenceID) {
	for _, id := range ids {
		if _, ok := p.pool[id]; ok {
			p.allowed[id] = true
		}
	}
}

// upstream reports whether evidence is upstream text (its artifact is of
// upstream origin), as the spec checks read it.
func (p *Precomputed) upstream(e domain.Evidence) bool {
	a, ok := p.arts[e.SourceID]
	return ok && a.Origin == domain.OriginUpstream
}

// uncertainties lists what the assessment leaves unresolved, in its order:
// the unknowns; each not-checked or unavailable behaviour no unknown entry
// already states; each setting whose state is unknown or an inherited
// default; each precondition.
func uncertainties(a domain.AgentAssessment) []Uncertainty {
	var out []Uncertainty
	seen := map[string]bool{}
	add := func(kind, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		u := Uncertainty{ID: UncertaintyIDPrefix + domain.ShortHash(kind, text), Kind: kind, Text: text}
		if !seen[u.ID] {
			seen[u.ID] = true
			out = append(out, u)
		}
	}
	for _, u := range a.Unknown {
		add(UncertaintyUnknown, u)
	}
	for _, b := range a.Behaviour {
		if !b.Gap() {
			continue
		}
		stated := false
		for _, u := range a.Unknown {
			stated = stated || strings.Contains(u, b.Behaviour)
		}
		if !stated {
			text := fmt.Sprintf("behaviour %s: %s", b.Behaviour, b.Source)
			if b.Reason != "" {
				text += " (" + b.Reason + ")"
			}
			add(UncertaintyBehaviour, text)
		}
	}
	for _, s := range a.EffectiveConfiguration {
		switch s.State {
		case domain.AssessmentUnknown:
			add(UncertaintySetting, settingText(s))
		case domain.AssessmentInheritedDefault:
			add(UncertaintyDefault, settingText(s))
		}
	}
	for _, c := range a.Preconditions {
		add(UncertaintyPrecondition, c)
	}
	return out
}

func settingText(s domain.AssessmentSetting) string {
	t := fmt.Sprintf("setting %s: %s", s.Setting, s.State)
	if s.Detail != "" {
		t += " (" + s.Detail + ")"
	}
	return t
}

// Digest identifies the input (the model's view of it).
func (p *Precomputed) Digest() string {
	b, err := json.Marshal(p)
	if err != nil {
		panic(err) // plain data
	}
	return domain.Digest(b)
}

func (p *Precomputed) uncertainty(id string) (Uncertainty, bool) {
	for _, u := range p.Uncertainties {
		if u.ID == id {
			return u, true
		}
	}
	return Uncertainty{}, false
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + "…"
}
