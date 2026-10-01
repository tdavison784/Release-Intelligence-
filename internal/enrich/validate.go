package enrich

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// proposal is one enrichment as the model proposed it.
type proposal struct {
	Kind       string   `json:"kind"`
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	Changes    []string `json:"changes"`
	Citations  []string `json:"citations"`
	Confidence string   `json:"confidence"`
}

type answer struct {
	Enrichments []proposal `json:"enrichments"`
}

var (
	schemaOnce sync.Once
	schema     *jsonschema.Schema
	schemaErr  error
)

func answerValidator() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		doc, err := jsonschema.UnmarshalJSON(strings.NewReader(answerSchema))
		if err != nil {
			schemaErr = err
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("https://ri.dev/schemas/enrich-answer/v1.json", doc); err != nil {
			schemaErr = err
			return
		}
		schema, schemaErr = c.Compile("https://ri.dev/schemas/enrich-answer/v1.json")
	})
	return schema, schemaErr
}

// decodeAnswer checks the answer text against the answer schema and decodes it.
func decodeAnswer(text string) (*answer, error) {
	sch, err := answerValidator()
	if err != nil {
		return nil, fmt.Errorf("answer schema: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(strings.TrimSpace(text)))
	if err != nil {
		return nil, fmt.Errorf("answer is not JSON: %v", err)
	}
	if err := sch.Validate(inst); err != nil {
		return nil, fmt.Errorf("answer does not match the schema: %s", oneLine(err.Error()))
	}
	var a answer
	if err := json.Unmarshal([]byte(text), &a); err != nil {
		return nil, fmt.Errorf("answer: %v", err)
	}
	return &a, nil
}

// groupContext is what the validator checks a proposal against.
type groupContext struct {
	cand    Candidate
	members map[string]bool                // change ids of the group
	changes map[string]domain.Change       // every change of the edge
	input   map[domain.EvidenceID]bool     // evidence shown in the prompt
	owner   map[domain.EvidenceID][]string // evidence id → group changes citing it
}

func newGroupContext(cand Candidate, changes map[string]domain.Change, input []domain.EvidenceID) *groupContext {
	g := &groupContext{cand: cand, members: map[string]bool{}, changes: changes, input: map[domain.EvidenceID]bool{}, owner: map[domain.EvidenceID][]string{}}
	for _, id := range input {
		g.input[id] = true
	}
	for _, id := range cand.Changes {
		g.members[id] = true
		for _, x := range changes[id].Evidence {
			g.owner[x] = append(g.owner[x], id)
		}
	}
	return g
}

// check validates one proposal. clustered maps change ids already
// consolidated by an accepted cluster (of any group) to that cluster.
func (g *groupContext) check(p proposal, clustered map[string]string) error {
	kind := domain.EnrichmentKind(p.Kind)
	if !kind.Valid() {
		return fmt.Errorf("unknown kind %q", p.Kind)
	}
	if strings.TrimSpace(p.Content) == "" {
		return fmt.Errorf("empty content")
	}
	if len(p.Changes) == 0 {
		return fmt.Errorf("relates to no change")
	}
	seen := map[string]bool{}
	computed := false
	for _, id := range p.Changes {
		switch {
		case seen[id]:
			return fmt.Errorf("lists change %s twice", id)
		case g.changes[id].ID == "":
			return fmt.Errorf("unknown change id %s (not in the edge)", id)
		case !g.members[id]:
			return fmt.Errorf("change %s was not part of this group's input", id)
		}
		seen[id] = true
		computed = computed || g.changes[id].Provenance.Method == domain.MethodComputed
	}
	if n := kind.MinChanges(); len(p.Changes) < n {
		return fmt.Errorf("kind %s needs at least %d changes, got %d", kind, n, len(p.Changes))
	}
	if len(p.Citations) == 0 {
		return fmt.Errorf("cites no evidence")
	}
	cited := map[string]bool{} // change ids with a cited evidence record
	citedStatement := false    // a citation from a non-computed change
	seenCite := map[string]bool{}
	for _, id := range p.Citations {
		if seenCite[id] {
			return fmt.Errorf("cites %s twice", id)
		}
		seenCite[id] = true
		if !g.input[domain.EvidenceID(id)] {
			return fmt.Errorf("citation %s was not part of the input evidence", id)
		}
		for _, cid := range g.owner[domain.EvidenceID(id)] {
			cited[cid] = true
			if g.changes[cid].Provenance.Method != domain.MethodComputed {
				citedStatement = true
			}
		}
	}
	switch kind {
	case domain.EnrichmentCluster:
		for _, id := range p.Changes {
			if !cited[id] {
				return fmt.Errorf("cluster member %s has none of its evidence cited", id)
			}
			if other, ok := clustered[id]; ok {
				return fmt.Errorf("change %s is already consolidated by cluster %s", id, other)
			}
		}
	case domain.EnrichmentDiffExplanation:
		if !computed {
			return fmt.Errorf("a diff explanation must relate to a computed change")
		}
		if !citedStatement {
			return fmt.Errorf("a diff explanation must cite the release-note or upgrade-guide statement that explains the diff")
		}
	}
	return nil
}

// canonical returns the proposal's change ids and citations in a stable order.
func canonical(xs []string) []string {
	out := append([]string(nil), xs...)
	sort.Strings(out)
	return out
}
