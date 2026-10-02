package impactenrich

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// The forbidden-transition table. This is the ONLY place a model verdict
// becomes a report change, and the mapping is a constant: there is no input
// the model can supply that produces an action-required suggestion, an
// affected classification, a deleted finding or AI provenance on a finding.
var verdictEffects = map[string]struct {
	kind    domain.EnrichmentKind // the enrichment the verdict becomes
	message string                // what the model must put into content
}{
	"plausibly-applies": {domain.EnrichmentPlausiblyApplies,
		"what overlaps, why it may matter and what to inspect (a review SUGGESTION, never an action mandate)"},
	"not-applicable": {domain.EnrichmentNotApplicable,
		"why the change does not reach the environment (a note; a human decides not-affected)"},
	"undetermined": {domain.EnrichmentUndetermined,
		"what the input was missing"},
}

// suggestedClass is the ceiling of an accepted plausibly-applies verdict.
// It is a constant: the AI may suggest review-required and nothing else —
// never action-required, never not-affected, and never a different class on
// the finding itself.
var suggestedClass = domain.ImpactReviewRequired

// applicabilityAnswer is one applicability verdict as the model proposed it.
type applicabilityAnswer struct {
	Verdict    string   `json:"verdict"`
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	Citations  []string `json:"citations"`
	Confidence string   `json:"confidence"`
}

// clusterAnswer is one duplicate adjudication.
type clusterAnswer struct {
	SameChange bool     `json:"sameChange"`
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	Citations  []string `json:"citations"`
	Confidence string   `json:"confidence"`
}

// migrationAnswer is one migration-steps synthesis.
type migrationAnswer struct {
	Title      string   `json:"title"`
	Steps      []string `json:"steps"`
	Citations  []string `json:"citations"`
	Confidence string   `json:"confidence"`
}

var (
	schemas   = map[string]*jsonschema.Schema{}
	schemaErr error
	schemaOne sync.Once
)

func compileSchemas() error {
	schemaOne.Do(func() {
		for name, doc := range map[string]string{
			"applicability": applicabilityAnswerSchema,
			"cluster":       clusterAnswerSchema,
			"migration":     migrationAnswerSchema,
		} {
			inst, err := jsonschema.UnmarshalJSON(strings.NewReader(doc))
			if err != nil {
				schemaErr = err
				return
			}
			c := jsonschema.NewCompiler()
			url := "https://ri.dev/schemas/impact-enrich-answer/" + name + "/v1.json"
			if err := c.AddResource(url, inst); err != nil {
				schemaErr = err
				return
			}
			s, err := c.Compile(url)
			if err != nil {
				schemaErr = err
				return
			}
			schemas[name] = s
		}
	})
	return schemaErr
}

// decode checks the answer text against the schema of the candidate type and
// decodes it. Fenced JSON (```json … ```) is tolerated, like the edge
// enricher.
func decode(typ CandidateType, text string) (any, error) {
	if err := compileSchemas(); err != nil {
		return nil, fmt.Errorf("answer schema: %w", err)
	}
	t := strings.TrimSpace(text)
	if fenced, ok := unwrapFence(t); ok {
		t = fenced
	}
	s := schemas[string(typ)]
	if s == nil {
		return nil, fmt.Errorf("no answer schema for %s", typ)
	}
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(t))
	if err != nil {
		return nil, fmt.Errorf("answer is not JSON: %v", err)
	}
	if err := s.Validate(inst); err != nil {
		return nil, fmt.Errorf("answer does not match the schema: %s", oneLine(err.Error()))
	}
	switch typ {
	case CandApplicability:
		var a applicabilityAnswer
		if err := json.Unmarshal([]byte(t), &a); err != nil {
			return nil, fmt.Errorf("answer: %v", err)
		}
		return a, nil
	case CandCluster:
		var a clusterAnswer
		if err := json.Unmarshal([]byte(t), &a); err != nil {
			return nil, fmt.Errorf("answer: %v", err)
		}
		return a, nil
	default:
		var a migrationAnswer
		if err := json.Unmarshal([]byte(t), &a); err != nil {
			return nil, fmt.Errorf("answer: %v", err)
		}
		return a, nil
	}
}

// unwrapFence strips a leading ```lang and trailing ``` if both are present.
func unwrapFence(s string) (string, bool) {
	const fence = "```"
	if !strings.HasPrefix(s, fence) {
		return "", false
	}
	body := s[len(fence):]
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		body = body[i+1:]
	}
	if j := strings.LastIndex(body, fence); j >= 0 {
		body = body[:j]
	}
	return strings.TrimSpace(body), true
}

// answerContext is what the validator checks an answer against: the
// candidate's findings, their changes and the evidence shown in the prompt.
type answerContext struct {
	cand    Candidate
	members map[string]domain.ImpactFinding
	input   map[domain.EvidenceID]bool
	owner   map[domain.EvidenceID][]string // evidence id → findings whose upstream change cites it
}

func newAnswerContext(cand Candidate, findings []domain.ImpactFinding, in inputs, input []domain.EvidenceID) *answerContext {
	g := &answerContext{cand: cand, members: map[string]domain.ImpactFinding{}, input: map[domain.EvidenceID]bool{},
		owner: map[domain.EvidenceID][]string{}}
	for _, id := range input {
		g.input[id] = true
	}
	for _, f := range findings {
		g.members[f.ID] = f
		if c, ok := in.changes[f.ChangeID]; ok {
			for _, x := range c.Evidence {
				g.owner[x] = append(g.owner[x], f.ID)
			}
		}
	}
	return g
}

// checkApplicability validates one verdict against its candidate. The
// forbidden transitions live here and in verdictEffects:
//   - the verdict names the effect; nothing else in the answer can;
//   - a plausibly-applies / not-applicable verdict must cite evidence of the
//     finding's own upstream change (grounding in the deterministic record);
//   - every citation must have been shown in the prompt.
func (g *answerContext) checkApplicability(a applicabilityAnswer) (domain.EnrichmentKind, error) {
	eff, ok := verdictEffects[a.Verdict]
	if !ok {
		// unreachable through the schema (the enum rejects it); kept as the
		// belt-and-braces guard of the forbidden-transition table
		return "", fmt.Errorf("unknown verdict %q", a.Verdict)
	}
	if strings.TrimSpace(a.Content) == "" {
		return "", fmt.Errorf("empty content")
	}
	if len(g.members) != 1 {
		return "", fmt.Errorf("an applicability answer is about one finding, the candidate lists %d", len(g.members))
	}
	cited, err := g.citations(a.Citations)
	if err != nil {
		return "", err
	}
	if a.Verdict != "undetermined" && !cited {
		return "", fmt.Errorf("a %s verdict must cite evidence of the finding's upstream change", a.Verdict)
	}
	return eff.kind, nil
}

// checkCluster validates one duplicate adjudication.
func (g *answerContext) checkCluster(a clusterAnswer) (bool, error) {
	if strings.TrimSpace(a.Content) == "" {
		return false, fmt.Errorf("empty content")
	}
	if len(g.members) < 2 {
		return false, fmt.Errorf("a cluster candidate lists %d findings, need 2", len(g.members))
	}
	cited := map[string]bool{}
	for _, id := range a.Citations {
		if !g.input[domain.EvidenceID(id)] {
			return false, fmt.Errorf("citation %s was not part of the input evidence", id)
		}
		for _, fid := range g.owner[domain.EvidenceID(id)] {
			cited[fid] = true
		}
	}
	if !a.SameChange {
		return false, nil
	}
	for id := range g.members {
		if !cited[id] {
			return false, fmt.Errorf("cluster member %s has none of its change evidence cited", id)
		}
	}
	return true, nil
}

// checkMigration validates one migration synthesis.
func (g *answerContext) checkMigration(a migrationAnswer) (string, error) {
	if len(g.members) != 1 {
		return "", fmt.Errorf("a migration answer is about one finding, the candidate lists %d", len(g.members))
	}
	if len(a.Steps) == 0 {
		return "", fmt.Errorf("no steps")
	}
	citedChange := false
	for _, id := range a.Citations {
		if !g.input[domain.EvidenceID(id)] {
			return "", fmt.Errorf("citation %s was not part of the input evidence", id)
		}
		for _, fid := range g.owner[domain.EvidenceID(id)] {
			if _, is := g.members[fid]; is {
				citedChange = true
			}
		}
	}
	if !citedChange {
		return "", fmt.Errorf("the steps must cite evidence of the finding's upstream change")
	}
	for i, s := range a.Steps {
		a.Steps[i] = fmt.Sprintf("%d. %s", i+1, strings.TrimSpace(s))
	}
	return strings.Join(a.Steps, "\n"), nil
}

func (g *answerContext) citations(ids []string) (citedChange bool, err error) {
	for _, id := range ids {
		if !g.input[domain.EvidenceID(id)] {
			return false, fmt.Errorf("citation %s was not part of the input evidence", id)
		}
		for _, fid := range g.owner[domain.EvidenceID(id)] {
			if _, is := g.members[fid]; is {
				citedChange = true
			}
		}
	}
	return citedChange, nil
}

// impactConfidence caps AI output: impact enrichments are never presented
// with high confidence, whatever the model said.
func impactConfidence() domain.Confidence {
	return domain.ConfidenceMedium
}
