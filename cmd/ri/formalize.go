package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/formalize"
	"github.com/tdavison784/release-intelligence/internal/investigate/agent"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/investigate/verify"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/llm"
	"github.com/tdavison784/release-intelligence/internal/synthesize"
)

func init() {
	investigateSubcommands["formalize"] = (*cli).investigateFormalize
}

// formalizeFlags are the formalization flags (Milestone 4). Like the
// synthesis, they are not part of the request configuration: formalization
// never changes the run record.
type formalizeFlags struct {
	on       *bool
	model    *string
	exchange *string
}

func addFormalizeFlags(fs interface {
	String(name, value, usage string) *string
	Bool(name string, value bool, usage string) *bool
}, withSwitch bool) formalizeFlags {
	f := formalizeFlags{
		model:    fs.String("formalize-model", formalize.DefaultModel, "model of the formalization stage"),
		exchange: fs.String("formalize-exchange", "", "answer the formalization through request/response files in `DIR` (scripts/semantic-exchange.sh answers them) instead of `claude -p`"),
	}
	if withSwitch {
		f.on = fs.Bool("formalize", false, "after each investigation, formalize its agent assessments into cited hypotheses (one cached model call each; stored beside the run, never a policy input)")
	}
	return f
}

func (f formalizeFlags) enabled() bool { return f.on != nil && *f.on }

// formalizer is the formalization port: llm.Cache over the file exchange or
// a stateless `claude -p` call (nothing but the cache with -offline).
func (c *cli) formalizer(a *app.App, f formalizeFlags, claudeBin string) *formalize.Formalizer {
	var inner llm.Client
	switch {
	case f.exchange != nil && *f.exchange != "":
		inner = &llm.Exchange{Dir: *f.exchange}
	case c.g.offline:
	default:
		inner = &synthesize.ClaudePrint{Binary: claudeBin}
	}
	model := formalize.DefaultModel
	if f.model != nil && *f.model != "" {
		model = *f.model
	}
	return &formalize.Formalizer{Client: &llm.Cache{Dir: a.LLMCacheDir(), Inner: inner}, Model: model}
}

// formalizeOutcome says what became of one assessment.
type formalizeOutcome struct {
	Assessment string                    `json:"assessment"`
	ChangeItem string                    `json:"changeItem"`
	Record     string                    `json:"record,omitempty"`
	Status     string                    `json:"status"` // complete | failed | pending | error
	Hypotheses []formalizedHypothesisRow `json:"hypotheses,omitempty"`
	Detail     string                    `json:"detail,omitempty"`
}

type formalizedHypothesisRow struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Statement  string `json:"statement"`
	Limitation string `json:"limitation,omitempty"`
}

// formalizeRun formalizes the run's assessments (all, or the one named)
// and stores the records. A pending exchange or an unreachable model is
// reported per assessment; the others go on.
func formalizeRun(ctx context.Context, fz *formalize.Formalizer, fst formalize.Store, st *store.Store, inv *domain.Investigation, b *domain.EvidenceBundle, only string) ([]formalizeOutcome, []formalize.Record, error) {
	var out []formalizeOutcome
	var recs []formalize.Record
	found := false
	for _, a := range inv.AgentAssessments {
		if only != "" && a.ID != only {
			continue
		}
		found = true
		o := formalizeOutcome{Assessment: a.ID, ChangeItem: a.ChangeItem}
		r, err := fz.Formalize(ctx, formalize.Input{Investigation: inv, Bundle: b, Assessment: a.ID, Read: agent.StoreReader(st)})
		switch {
		case errors.Is(err, llm.ErrPending):
			o.Status, o.Detail = "pending", err.Error()
		case err != nil:
			o.Status, o.Detail = "error", err.Error()
		default:
			if err := fst.Put(r); err != nil {
				return nil, nil, err
			}
			recs = append(recs, *r)
			o.Record, o.Status, o.Detail = r.ID, r.Status, r.NoHypothesisReason
			if r.Failure != nil {
				o.Detail = strings.Join(r.Failure.Violations, "; ")
			}
			for _, h := range r.Hypotheses {
				row := formalizedHypothesisRow{ID: h.ID, Kind: formalize.KindRule, Statement: h.Statement}
				if h.Mitigation == nil {
					row.Kind, row.Limitation = formalize.KindUnrepresentable, h.MitigationRejected
				}
				o.Hypotheses = append(o.Hypotheses, row)
			}
		}
		out = append(out, o)
	}
	if only != "" && !found {
		return nil, nil, fmt.Errorf("investigation %s has no assessment %s", inv.ID, only)
	}
	return out, recs, nil
}

// investigateFormalize is `ri investigate formalize <inv-id>`: each agent
// assessment of the run becomes cited hypotheses (a mitigation rule, or an
// unrepresentable record), stored under
// <state>/investigations/formalizations/<inv-id>/. With -knowledge, the
// run's formalized hypotheses are exported through ExportHypotheses
// (proposals and review items, never facts), so `ri verify hypotheses`
// can verify them. The run record is never changed.
func (c *cli) investigateFormalize(args []string) error {
	fs := c.flags("investigate formalize", "<inv-id>")
	only := fs.String("assessment", "", "formalize only this assessment (aas-…; default: every assessment of the run)")
	dir := fs.String("knowledge", "", "also export the run's formalized hypotheses as proposals and review items into this knowledge directory (use a copy, not the live knowledge/ tree, for experiments)")
	claudeBin := fs.String("claude-bin", "", "claude CLI binary (default: claude on PATH)")
	ff := addFormalizeFlags(fs, false)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		fs.Usage()
		return fmt.Errorf("%w: give the investigation id", app.ErrUsage)
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	r, err := loadStoredRun(c.ctx, c.g.state, pos[0])
	if err != nil {
		return err
	}
	fst := formalize.Store{Dir: formalize.StateDir(c.g.state)}
	outcomes, _, err := formalizeRun(c.ctx, c.formalizer(a, ff, *claudeBin), fst, r.st, r.inv, r.bundle, *only)
	if err != nil {
		return err
	}
	if outcomes == nil {
		outcomes = []formalizeOutcome{}
	}
	res := map[string]any{"investigation": r.inv.ID, "store": fst.Dir, "assessments": outcomes}
	if *dir != "" {
		recs, err := fst.List(c.ctx, r.inv.ID)
		if err != nil {
			return err
		}
		exp, err := exportFormalized(c.ctx, r.inv, r.bundle, r.st, knowledge.NewFileStore(*dir), recs)
		if err != nil {
			return err
		}
		res["knowledge"], res["exported"] = *dir, exp
	}
	return c.writeJSON(res)
}

// exportFormalized feeds the formalized hypotheses to ExportHypotheses
// unchanged: a view of the run whose hypotheses are the formalized ones
// (the stored run record is not changed).
func exportFormalized(ctx context.Context, inv *domain.Investigation, b *domain.EvidenceBundle, st *store.Store, ks knowledge.Store, recs []formalize.Record) ([]agent.ExportResult, error) {
	view := *inv
	view.Hypotheses = formalize.Hypotheses(recs)
	res, err := agent.ExportHypotheses(ctx, &view, b, st, ks, time.Now())
	if res == nil {
		res = []agent.ExportResult{}
	}
	return res, err
}

// formalizedRuns finds a hypothesis in the run's formalization records,
// and otherwise in the run record (verify.StateRuns).
type formalizedRuns struct {
	store formalize.Store
	next  verify.RunSource
}

func (r formalizedRuns) Mitigation(ctx context.Context, inv, hyp string) (*verify.Mitigation, error) {
	recs, err := r.store.List(ctx, inv)
	if err != nil {
		return nil, err
	}
	for _, h := range formalize.Hypotheses(recs) {
		if h.ID != hyp {
			continue
		}
		if m := h.Mitigation; m != nil {
			rule := m.ProposedRule.Assertion
			return &verify.Mitigation{Rule: &rule, Bounds: &domain.ProofBounds{From: m.ProposedRule.Release.From, To: m.ProposedRule.Release.To}}, nil
		}
		return &verify.Mitigation{Rejected: h.MitigationRejected}, nil
	}
	return r.next.Mitigation(ctx, inv, hyp)
}
