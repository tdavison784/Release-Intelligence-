package semantic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// RunReport summarises one proposal run: what each model did and how the
// models compare. It is a run summary for the operator, not the loop's
// metrics (the knowledge lane computes those from the store).
type RunReport struct {
	Candidates int                   `json:"candidates"`
	Models     []ModelRun            `json:"models"`
	Agreement  []AspectAgreementStat `json:"agreement"`
}

// ModelRun counts one model's outcomes.
type ModelRun struct {
	Provider, Model string
	Proposals       int            `json:"proposals"`
	Asserted        map[string]int `json:"asserted"`     // per aspect
	Undetermined    map[string]int `json:"undetermined"` // per aspect
	Pending         int            `json:"pending"`
	Failed          map[string]int `json:"failed"` // by kind: rejected, transport, refused, not-asked
	FailureReasons  []string       `json:"failureReasons,omitempty"`
}

// AspectAgreementStat is agreement on one aspect among the answers that
// asserted it for the same candidate and task (equal AspectDigest). Every
// separate call is one answer (PO-1), so two calls of the same model are a
// pair, labelled "model|model".
type AspectAgreementStat struct {
	Aspect domain.Aspect `json:"aspect"`
	// Compared counts candidate×task pairs where ≥2 answers asserted the aspect.
	Compared int `json:"compared"`
	// AllAgree counts those where every asserting answer has the same digest.
	AllAgree int `json:"allAgree"`
	// Pairwise maps "modelA|modelB" to [agree, compared].
	Pairwise map[string][2]int `json:"pairwise"`
	// SameFamily is true when every compared answer is of one model family
	// (domain.ModelFamily); same-model agreement is the case PO-1 made
	// measurable, so it is reported, not hidden.
	SameFamily bool `json:"sameFamily"`
}

// Summarize builds the run report.
func Summarize(ncands int, props []domain.SemanticProposal, fails []knowledge.ProposalFailure) RunReport {
	rep := RunReport{Candidates: ncands}
	runs := map[string]*ModelRun{}
	get := func(provider, model string) *ModelRun {
		k := provider + "|" + model
		if runs[k] == nil {
			runs[k] = &ModelRun{Provider: provider, Model: model, Asserted: map[string]int{}, Undetermined: map[string]int{}, Failed: map[string]int{}}
		}
		return runs[k]
	}
	for _, p := range props {
		r := get(p.Provider, p.Provenance.Model)
		r.Proposals++
		for _, a := range p.Assertion.Stated() {
			r.Asserted[string(a)]++
		}
		for _, a := range p.Undetermined {
			r.Undetermined[string(a)]++
		}
	}
	for _, f := range fails {
		r := get(f.Provider, f.Model)
		if IsPending(f) {
			r.Pending++
			continue
		}
		kind, _, _ := strings.Cut(f.Reason, ":")
		if strings.HasPrefix(f.Reason, "not asked") {
			kind = "not-asked"
		}
		r.Failed[kind]++
		if len(r.FailureReasons) < 20 {
			r.FailureReasons = append(r.FailureReasons, f.CandidateID+" "+string(f.Task)+": "+shorten(f.Reason, 300))
		}
	}
	keys := make([]string, 0, len(runs))
	for k := range runs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rep.Models = append(rep.Models, *runs[k])
	}

	// agreement per aspect over candidate×task groups. Every separate call
	// is its own answer (PO-1: two calls of one model are a comparable pair,
	// labelled "model|model"); provider+model+call id keys the answer, the
	// model labels it. Two answers of one model with the same call id are
	// by definition not separate calls and stay one answer.
	type answer struct{ model, digest string }
	type group map[string]answer
	byKey := map[string]map[domain.Aspect]group{}
	for _, p := range props {
		k := p.CandidateID + "|" + string(p.Task)
		if byKey[k] == nil {
			byKey[k] = map[domain.Aspect]group{}
		}
		for _, a := range p.Assertion.Stated() {
			if byKey[k][a] == nil {
				byKey[k][a] = group{}
			}
			id := p.Provider + "|" + p.Provenance.Model + "|" + p.Provenance.CallID
			byKey[k][a][id] = answer{p.Provenance.Model, p.Assertion.AspectDigest(a)}
		}
	}
	for _, a := range domain.Aspects {
		st := AspectAgreementStat{Aspect: a, Pairwise: map[string][2]int{}, SameFamily: true}
		for _, asp := range byKey {
			g := asp[a]
			if len(g) < 2 {
				continue
			}
			st.Compared++
			keys := make([]string, 0, len(g))
			digests := map[string]bool{}
			for k, an := range g {
				keys = append(keys, k)
				digests[an.digest] = true
			}
			sort.Strings(keys)
			if len(digests) == 1 {
				st.AllAgree++
			}
			for i := range keys {
				if domain.ModelFamily(g[keys[i]].model) != domain.ModelFamily(g[keys[0]].model) {
					st.SameFamily = false
				}
				for j := i + 1; j < len(keys); j++ {
					pk := g[keys[i]].model + "|" + g[keys[j]].model
					v := st.Pairwise[pk]
					v[1]++
					if g[keys[i]].digest == g[keys[j]].digest {
						v[0]++
					}
					st.Pairwise[pk] = v
				}
			}
		}
		rep.Agreement = append(rep.Agreement, st)
	}
	return rep
}

// WriteText renders the report.
func (r RunReport) WriteText(w interface{ Write([]byte) (int, error) }) {
	fmt.Fprintf(w, "candidates: %d\n", r.Candidates)
	for _, m := range r.Models {
		fmt.Fprintf(w, "%s/%s: %d proposals, %d pending, failed %v\n  asserted %v\n  undetermined %v\n",
			m.Provider, m.Model, m.Proposals, m.Pending, m.Failed, m.Asserted, m.Undetermined)
		for _, f := range m.FailureReasons {
			fmt.Fprintf(w, "    ✗ %s\n", f)
		}
	}
	for _, a := range r.Agreement {
		if a.Compared == 0 {
			continue
		}
		fam := ""
		if a.SameFamily {
			fam = " (single model family: not independent)"
		}
		fmt.Fprintf(w, "agreement %-13s all-agree %d/%d%s\n", a.Aspect, a.AllAgree, a.Compared, fam)
		keys := make([]string, 0, len(a.Pairwise))
		for k := range a.Pairwise {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := a.Pairwise[k]
			fmt.Fprintf(w, "  %s: %d/%d\n", k, v[0], v[1])
		}
	}
}

// WriteRecords writes candidates and proposals as KnowledgeRecord files in
// the knowledge/ layout (DESIGN.md §8):
// <dir>/<product>/<release|_endpoint>/{candidates,proposals}/<id>.json.
// Records are validated first; an existing identical file is left alone.
// Failures go to <dir>/<product>/failures/<attempt>.json (not knowledge
// records; removed when the attempt later succeeds).
func WriteRecords(dir string, cands []domain.SemanticCandidate, props []domain.SemanticProposal, fails []knowledge.ProposalFailure) error {
	where := map[string]domain.SemanticCandidate{}
	for _, c := range cands {
		where[c.ID] = c
	}
	put := func(c domain.SemanticCandidate, sub, id string, entity any) error {
		rec, err := domain.NewRecord(entity)
		if err != nil {
			return err
		}
		if err := rec.Validate(); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		rel := c.Release
		if rel == "" {
			rel = "_endpoint"
		}
		path := filepath.Join(dir, string(c.Product), rel, sub, id+".json")
		b, err := json.MarshalIndent(rec, "", "  ")
		if err != nil {
			return err
		}
		b = append(b, '\n')
		if have, err := os.ReadFile(path); err == nil && string(have) == string(b) {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, b, 0o644)
	}
	for _, c := range cands {
		if err := put(c, "candidates", c.ID, c); err != nil {
			return err
		}
	}
	for _, p := range props {
		c, ok := where[p.CandidateID]
		if !ok {
			return fmt.Errorf("proposal %s: candidate %s not written", p.ID, p.CandidateID)
		}
		if err := put(c, "proposals", p.ID, p); err != nil {
			return err
		}
	}
	// one file per attempt (candidate × task × provider × model), so a re-run
	// overwrites its own record and a later success removes it
	attempt := func(c domain.SemanticCandidate, task domain.ProposalTask, provider, model string) string {
		return filepath.Join(dir, string(c.Product), "failures", domain.ShortHash(c.ID, string(task), provider, model)+".json")
	}
	for _, p := range props {
		if c, ok := where[p.CandidateID]; ok {
			_ = os.Remove(attempt(c, p.Task, p.Provider, p.Provenance.Model))
		}
	}
	for _, f := range fails {
		c, ok := where[f.CandidateID]
		if !ok || IsPending(f) {
			continue
		}
		path := attempt(c, f.Task, f.Provider, f.Model)
		b, err := json.MarshalIndent(f, "", "  ")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	return nil
}
