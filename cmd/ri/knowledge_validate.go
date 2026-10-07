package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/semantic"
)

// knowledgeValidate implements `ri knowledge validate`: run every
// deterministic validator over the proposals of a knowledge store, against the
// ingested From/To releases of the edges given with -edge, and write the
// ValidationResults back (idempotent).
func (c *cli) knowledgeValidate(args []string) error {
	fs := c.flags("knowledge validate", "")
	dir := fs.String("dir", "knowledge", "knowledge directory")
	edges := fs.String("edge", "", "comma-separated product:from:to edges the stored candidates came from (required), e.g. cert-manager:v1.16.0:v1.17.0")
	kube := fs.String("kubernetes", "", "Kubernetes version for the rendered-diff validator (chart kubeVersion gates)")
	output := fs.String("o", "text", "output format: text|json")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	if *edges == "" {
		return fmt.Errorf("%w: -edge product:from:to[,…] is required (a stored candidate does not record its source release)", app.ErrUsage)
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	byID := map[string]resolved{}
	for _, e := range splitList(*edges) {
		parts := strings.Split(e, ":")
		if len(parts) != 3 {
			return fmt.Errorf("%w: -edge %q: want product:from:to", app.ErrUsage, e)
		}
		edge, err := a.Upgrade(c.ctx, parts[0], parts[1], parts[2], app.UpgradeOptions{})
		if err != nil {
			return fmt.Errorf("edge %s: %w", e, err)
		}
		from, err := c.release(a, parts[0], parts[1])
		if err != nil {
			return err
		}
		to, err := c.release(a, parts[0], parts[2])
		if err != nil {
			return err
		}
		for _, cand := range semantic.BuildCandidates(edge, edge.GeneratedAt).Candidates {
			if _, dup := byID[cand.ID]; !dup { // the first requested edge wins
				byID[cand.ID] = resolved{from, to, edge}
			}
		}
	}
	rep, err := app.ValidateKnowledge(c.ctx, c.knowledgeStore(*dir), app.ValidateOptions{
		Validators: a.Validators(*kube),
		Resolve: func(_ context.Context, cand domain.SemanticCandidate) (*domain.Release, *domain.Release, *domain.UpgradeEdge, bool, error) {
			r, ok := byID[cand.ID]
			return r.from, r.to, r.edge, ok, nil
		},
	})
	if err != nil {
		return err
	}
	if *output == "json" {
		return c.writeJSON(rep)
	}
	rep.WriteText(c.out)
	return nil
}

type resolved struct {
	from, to *domain.Release
	edge     *domain.UpgradeEdge
}
