package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// AppRenderer re-renders a bundle's environment targets through the same
// path the renderdiff lane used to build them (app.RenderTargets +
// app.RenderEngine().RenderPair, the content-addressed render cache), and
// returns the raw output the normalized artifacts were derived from.
type AppRenderer struct {
	a *app.App
	// Repo overrides where the environment repository is read from. The
	// bundle's snapshot is checkout-independent by design (55ac1cfa) and no
	// longer carries the checkout path, so the wiring that knows where the
	// repo actually is passes it here; without it a re-render resolves the
	// repo to "." and no render target is ever detected again.
	Repo string
	// KubeVersion overrides the render --kube-version (use the investigation
	// config's when it set one; default: the snapshot's cluster version).
	KubeVersion string

	mu    sync.Mutex
	pairs map[string]*render.Pair // bundle id + target name
}

// NewAppRenderer returns a renderer over the app.
func NewAppRenderer(a *app.App) *AppRenderer {
	return &AppRenderer{a: a, pairs: map[string]*render.Pair{}}
}

// RawRender renders (or reuses) the target's pair and returns one side.
func (r *AppRenderer) RawRender(ctx context.Context, b *domain.EvidenceBundle, target, side string) (*RawRender, error) {
	p, err := r.pair(ctx, b, target)
	if err != nil {
		return nil, err
	}
	res := p.FromResult
	if side == "to" {
		res = p.ToResult
	}
	if !res.OK() {
		why := "no result"
		if res != nil && res.Failure != nil {
			why = res.Failure.Error()
		}
		return nil, fmt.Errorf("the %s render of %s failed: %s", side, target, why)
	}
	return &RawRender{Manifests: res.Output, Namespace: p.Target.Namespace, Nondeterministic: res.Nondeterministic}, nil
}

func (r *AppRenderer) pair(ctx context.Context, b *domain.EvidenceBundle, target string) (*render.Pair, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := b.ID + "\x00" + target
	if p, ok := r.pairs[key]; ok {
		return p, nil
	}
	repo, values, kube := snapshotInputs(b.Snapshot)
	if r.Repo != "" {
		repo = r.Repo
	}
	if r.KubeVersion != "" {
		kube = r.KubeVersion
	}
	product := string(b.Change.Product.ID)
	targets, _, err := r.a.RenderTargets(product, app.RenderOptions{Repo: repo, ValuesFiles: values, KubeVersion: kube})
	if err != nil {
		return nil, fmt.Errorf("render targets: %w", err)
	}
	rel := relativizer(repo)
	seen := map[string]int{}
	for _, t := range targets {
		name := rel(t.ID)
		if n := seen[name]; n > 0 {
			name = fmt.Sprintf("%s#%d", name, n+1)
		}
		seen[rel(t.ID)]++
		if name != target {
			continue
		}
		p := r.a.RenderEngine().RenderPair(ctx, render.PairRequest{Product: product, From: b.Change.From.String(), To: b.Change.To.String(),
			Target: t, Scope: domain.RenderEnvironment, KubeVersion: kube})
		r.pairs[key] = p
		return p, nil
	}
	return nil, fmt.Errorf("render target %s is no longer detected in %s", target, repo)
}

// snapshotInputs are the render inputs a snapshot names (as the renderdiff
// lane reads them).
func snapshotInputs(s domain.EnvironmentSnapshot) (repo string, values []string, kube string) {
	if s.Repo != nil {
		repo = s.Repo.Path
	}
	for _, x := range s.Inputs {
		if x.Status == domain.CollectionUnavailable {
			continue
		}
		switch x.Kind {
		case domain.InputGitOpsRepo:
			if repo == "" {
				repo = x.Path
			}
		case domain.InputValues:
			values = append(values, x.Path)
		case domain.InputClusterVersion:
			if kube == "" {
				kube = x.Value
			}
		}
	}
	if s.KubernetesVersion != "" {
		kube = s.KubernetesVersion
	}
	return repo, values, kube
}

// relativizer rewrites paths under the repository root to repo-relative
// form — the target names the bundle records.
func relativizer(repo string) func(string) string {
	if repo == "" {
		return func(s string) string { return s }
	}
	prefixes := []string{filepath.Clean(repo) + string(filepath.Separator)}
	if abs, err := filepath.Abs(repo); err == nil && abs != filepath.Clean(repo) {
		prefixes = append(prefixes, abs+string(filepath.Separator))
	}
	return func(s string) string {
		for _, p := range prefixes {
			if p == "."+string(filepath.Separator) {
				continue
			}
			s = strings.ReplaceAll(s, p, "")
		}
		return s
	}
}
