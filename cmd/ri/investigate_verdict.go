package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/agent"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

func init() {
	investigateSubcommands["dossier"] = (*cli).investigateDossier
	investigateSubcommands["export-hypotheses"] = (*cli).investigateExportHypotheses
}

// storedRun is a stored investigation with what its stages read.
type storedRun struct {
	st     *store.Store
	inv    *domain.Investigation
	bundle *domain.EvidenceBundle
	graph  *domain.RelationshipGraph
	report *domain.ImpactReport
	// consumers is the TO-side consumer graph a session with prompt feature
	// traceConsumers stored (nil otherwise).
	consumers *domain.RelationshipGraph
}

func loadStoredRun(ctx context.Context, state, run string) (*storedRun, error) {
	st, err := store.New(state)
	if err != nil {
		return nil, err
	}
	inv, err := st.GetInvestigation(ctx, run)
	if err != nil {
		return nil, err
	}
	b, err := st.GetBundle(ctx, inv.Bundle)
	if err != nil {
		return nil, err
	}
	r := &storedRun{st: st, inv: inv, bundle: b}
	read := func(id string, v any) error {
		for _, a := range inv.Artifacts {
			if a.ID != id {
				continue
			}
			rc, err := st.Open(ctx, a.Digest)
			if err != nil {
				return err
			}
			defer rc.Close()
			raw, err := io.ReadAll(rc)
			if err != nil {
				return err
			}
			return json.Unmarshal(raw, v)
		}
		return fmt.Errorf("investigation %s: artifact %s is not a run artifact", run, id)
	}
	if inv.Graph != "" {
		r.graph = &domain.RelationshipGraph{}
		if err := read(inv.Graph, r.graph); err != nil {
			return nil, err
		}
	}
	if inv.ImpactReport != "" {
		r.report = &domain.ImpactReport{}
		if err := read(inv.ImpactReport, r.report); err != nil {
			return nil, err
		}
	}
	for _, a := range inv.Artifacts {
		if a.Kind == domain.ArtifactGraph && a.Producer == agent.ConsumerGraphProducer && strings.HasPrefix(a.Path, "derived/agent/consumers/to/") {
			r.consumers = &domain.RelationshipGraph{}
			if err := read(a.ID, r.consumers); err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}

// investigateDossier is `ri investigate dossier <inv-id> <item>`: the item
// dossier an agent session starts from, rebuilt read-only from the stored run
// (the deterministic pass's validations, as the agent saw them). When the run
// stored a dossier for the item, the rebuilt one is compared with it.
func (c *cli) investigateDossier(args []string) error {
	fs := c.flags("investigate dossier", "<inv-id> <change-item>")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		fs.Usage()
		return fmt.Errorf("%w: give the investigation id and the change item", app.ErrUsage)
	}
	r, err := loadStoredRun(c.ctx, c.g.state, pos[0])
	if err != nil {
		return err
	}
	var pass []domain.ValidationRecord
	for _, v := range r.inv.Validations {
		if v.Request == "" { // the deterministic pass; agent-requested records carry their request
			pass = append(pass, v)
		}
	}
	d, err := agent.BuildDossier(c.ctx, agent.DossierInput{Bundle: r.bundle, Graph: r.graph, Traversal: r.inv.Traversal, Report: r.report,
		Validations: pass, Read: agent.StoreReader(r.st), Consumers: r.consumers}, pos[1])
	if err != nil {
		return err
	}
	data, err := d.Encode()
	if err != nil {
		return err
	}
	if r.inv.Agent != nil {
		for _, cov := range r.inv.Agent.Coverage {
			if cov.ChangeItem != pos[1] || cov.Dossier == "" {
				continue
			}
			for _, a := range r.inv.Artifacts {
				if a.ID == cov.Dossier {
					if a.Digest == domain.Digest(data) {
						fmt.Fprintf(c.err, "matches the dossier the agent session started from (%s)\n", a.ID)
					} else {
						fmt.Fprintf(c.err, "differs from the dossier the agent session started from (%s): the run's graph or traversal changed after the agent stage\n", a.ID)
					}
				}
			}
		}
	}
	_, err = c.out.Write(data)
	return err
}

// investigateExportHypotheses is `ri investigate export-hypotheses <inv-id>
// -knowledge <dir>`: every well-formed mitigation hypothesis of the run
// becomes a proposal (AI) and a whole-fact review item in the knowledge
// directory — never a fact. Idempotent.
func (c *cli) investigateExportHypotheses(args []string) error {
	fs := c.flags("investigate export-hypotheses", "<inv-id>")
	dir := fs.String("knowledge", "", "knowledge directory to write proposals and review items into (required; use a copy, not the live knowledge/ tree, for experiments)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || *dir == "" {
		fs.Usage()
		return fmt.Errorf("%w: give the investigation id and -knowledge", app.ErrUsage)
	}
	r, err := loadStoredRun(c.ctx, c.g.state, pos[0])
	if err != nil {
		return err
	}
	res, err := agent.ExportHypotheses(c.ctx, r.inv, r.bundle, r.st, knowledge.NewFileStore(*dir), time.Now())
	if err != nil {
		return err
	}
	if res == nil {
		res = []agent.ExportResult{}
	}
	return c.writeJSON(map[string]any{"investigation": r.inv.ID, "knowledge": *dir, "hypotheses": res})
}
