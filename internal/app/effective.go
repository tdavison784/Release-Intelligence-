package app

import (
	"fmt"
	"path/filepath"

	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// effectiveObjects is the effective object view the join's object-level
// evaluations read (lane join5, D-0028; env.Effective): the rendered objects
// of the environment's render targets — the target side of a complete pair,
// the render of a version-independent overlay — in place of the files each
// render consumed. A kustomize render whose objects are not available leaves
// the files it consumed unrendered (their objects are unknown, never their
// declaration); a chart render that failed is a render gap. Without a render
// (rr nil) the kustomize targets are detected, not rendered, so the files
// they consume are unrendered too; chart releases are then outside the view,
// as before. This is the view the investigation validators use (validate
// envView: renders over files, unavailable renders undecidable).
func (a *App) effectiveObjects(e *env.Environment, productID string, rr *RenderDiffResult) *env.Environment {
	var srcs []env.RenderedObjects
	if rr != nil {
		for _, p := range rr.Pairs {
			srcs = append(srcs, renderedObjectsOf(p))
		}
		return env.Effective(e, srcs)
	}
	targets, _, err := a.RenderTargets(productID, RenderOptions{Env: e, Repo: e.RepoRoot})
	if err != nil {
		return env.Effective(e, nil)
	}
	for _, t := range targets {
		if t.Kind != render.TargetKustomize || t.Kustomize == nil {
			continue
		}
		src := env.RenderedObjects{Target: t.ID, Unavailable: "not rendered: the join reads the rendered objects of the environment's entrypoints (run with -render)"}
		src.Consumed, err = consumedFiles(t.Kustomize)
		if err != nil {
			src.Unavailable = fmt.Sprintf("its inputs could not be resolved: %v", err)
		}
		srcs = append(srcs, src)
	}
	return env.Effective(e, srcs)
}

// renderedObjectsOf is one environment render pair's contribution to the view.
func renderedObjectsOf(p *render.Pair) env.RenderedObjects {
	src := env.RenderedObjects{Target: p.Target.ID}
	if k := p.Target.Kustomize; p.Target.Kind == render.TargetKustomize && k != nil {
		var err error
		if src.Consumed, err = consumedFiles(k); err != nil {
			src.Unavailable = fmt.Sprintf("its inputs could not be resolved: %v", err)
			return src
		}
	}
	var res *render.Result
	switch {
	case p.Status == render.PairOK:
		res = p.ToResult
	case p.Status == render.PairNotApplicable && p.FromResult.OK():
		// a version-independent overlay: the same objects before and after
		res = p.FromResult
	}
	switch {
	case !res.OK():
		why := string(p.Status)
		if p.Failure != nil {
			why += ": " + string(p.Failure.Reason) + " " + p.Failure.Detail
		}
		src.Unavailable = "render " + why
	case !p.Target.ValuesComplete:
		// objects rendered from part of the configuration are not the
		// deployed ones
		src.Unavailable = "rendered with incomplete values: " + p.Target.IncompleteReason
	default:
		src.Docs, src.Digest = res.Output, res.Provenance.OutputDigest
	}
	return src
}

// consumedFiles are the environment files a kustomize target reads, by path
// as the environment loader names them, with their layer rank.
func consumedFiles(k *render.KustomizeInput) (map[string]int, error) {
	root := k.Root
	if root == "" {
		root = k.Dir
	}
	layers, err := render.KustomizeLayers(root, k.Dir)
	if err != nil {
		return nil, err
	}
	abs, _ := filepath.Abs(root)
	out := make(map[string]int, len(layers))
	for rel, rank := range layers {
		out[filepath.Join(abs, filepath.FromSlash(rel))] = rank
	}
	return out, nil
}
