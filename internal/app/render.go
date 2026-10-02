package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// RenderOptions selects what `ri render diff` / `ri impact --render` render.
type RenderOptions struct {
	// Customer configuration (R7): explicit values files, a repository
	// (Argo CD / Flux / Helmfile / values files / Kustomize overlays), and
	// explicit overlays.
	ValuesFiles []string
	Repo        string
	Overlays    []string
	// Env is an already loaded environment (repo-mode discoveries); when nil
	// and Repo is set, the repository is loaded.
	Env *env.Environment
	// ReleaseName / Namespace override the install metadata.
	ReleaseName, Namespace string
	// Platform inputs: --kube-version and --api-versions.
	KubeVersion string
	APIVersions []string
	// Release also renders the chart defaults (release level).
	Release bool
}

// RenderDiffResult is the rendered view of one upgrade edge.
type RenderDiffResult struct {
	Edge *domain.UpgradeEdge
	// Release is the chart-default (release-level) pair, when requested.
	Release *render.Pair
	// Pairs are the environment pairs, one per detected deployment.
	Pairs        []*render.Pair
	Correlations map[string]render.Correlation // by target id ("chart-defaults" for Release)
	Notes        []string
}

// RenderEngine returns the render engine bound to this app: helm and
// kustomize renderers with a content-addressed cache under the state dir,
// and chart resolution through the product's artifact channels.
func (a *App) RenderEngine() *render.Engine {
	cache := render.NewCache(filepath.Join(a.cfg.StateDir, "render"))
	h := render.NewHelm(cache)
	h.Now = a.now
	k := render.NewKustomize(cache)
	k.Now = a.now
	return &render.Engine{Helm: h, Kustomize: k, Charts: &chartResolver{a: a}}
}

// ChartNames are the chart names a product publishes (helm-chart artifacts).
func (a *App) ChartNames(productID string) []string {
	def, err := a.Product(productID)
	if err != nil {
		return nil
	}
	var out []string
	add := func(s string) {
		for _, x := range out {
			if x == s {
				return
			}
		}
		if s != "" {
			out = append(out, s)
		}
	}
	for _, art := range def.Artifacts {
		if art.Type != domain.ArtifactHelmChart {
			continue
		}
		add(art.Name)
		for _, ch := range art.Channels {
			add(ch.Chart)
		}
	}
	return out
}

// RenderDiff renders the product's source and target releases with the
// customer's configuration (and, with opts.Release, with chart defaults),
// diffs them and correlates the rendered changes with the edge.
func (a *App) RenderDiff(ctx context.Context, productID, from, to string, opts RenderOptions) (*RenderDiffResult, error) {
	edge, err := a.Upgrade(ctx, productID, from, to, UpgradeOptions{})
	if err != nil {
		return nil, err
	}
	return a.RenderDiffEdge(ctx, edge, opts)
}

// RenderDiffEdge is RenderDiff over an already computed edge (the impact
// command has one, and its loaded environment is reused through opts.Env).
func (a *App) RenderDiffEdge(ctx context.Context, edge *domain.UpgradeEdge, opts RenderOptions) (*RenderDiffResult, error) {
	productID := string(edge.Product.ID)
	res := &RenderDiffResult{Edge: edge, Correlations: map[string]render.Correlation{}}
	eng := a.RenderEngine()
	fromV, toV := edge.From.String(), edge.To.String()
	if opts.Release {
		res.Release = eng.RenderPair(ctx, render.PairRequest{Product: productID, From: fromV, To: toV,
			Target: render.DefaultsTarget(productID), Scope: domain.RenderRelease, KubeVersion: opts.KubeVersion, APIVersions: opts.APIVersions})
		if res.Release.Diff != nil {
			res.Correlations[res.Release.Target.ID] = render.Correlate(res.Release.Diff, edge)
		}
	}
	targets, notes, err := a.RenderTargets(productID, opts)
	if err != nil {
		return nil, err
	}
	res.Notes = notes
	for _, t := range targets {
		p := eng.RenderPair(ctx, render.PairRequest{Product: productID, From: fromV, To: toV, Target: t,
			Scope: domain.RenderEnvironment, KubeVersion: opts.KubeVersion, APIVersions: opts.APIVersions})
		res.Pairs = append(res.Pairs, p)
		if p.Diff != nil {
			res.Correlations[t.ID] = render.Correlate(p.Diff, edge)
		}
	}
	return res, nil
}

// RenderTargets detects the deployments to render from the options.
func (a *App) RenderTargets(productID string, opts RenderOptions) ([]render.Target, []string, error) {
	e := opts.Env
	if e == nil && opts.Repo != "" {
		loaded, err := env.Load(env.Inputs{Repo: opts.Repo, ProductHints: env.HintsFromCatalog(a.Catalog)})
		if err != nil {
			return nil, nil, fmt.Errorf("environment: %w", err)
		}
		e = loaded
	}
	repo := opts.Repo
	if repo == "" && e != nil {
		repo = e.RepoRoot
	}
	targets, notes := render.DetectTargets(render.TargetOptions{
		Product: productID, Charts: a.ChartNames(productID), ValuesFiles: opts.ValuesFiles,
		ReleaseName: opts.ReleaseName, Namespace: opts.Namespace, Repo: repo, Files: render.RepoFilesFrom(e), Overlays: opts.Overlays,
	})
	if len(targets) == 0 {
		notes = append(notes, "no deployment of "+productID+" was found in the customer configuration; nothing customer-specific was rendered (use --values, --repo or --overlay)")
	}
	return targets, notes, nil
}

// chartResolver resolves a product's chart package at a product version
// through the ingested release (which records the chart artifact version)
// and the registered chart package readers — the same resolution ingest
// uses; packages are fetched through the fetch cache.
type chartResolver struct{ a *App }

func (r *chartResolver) ResolveChart(ctx context.Context, productID, version string) (*render.Chart, error) {
	fail := func(format string, args ...any) error {
		return &render.Failure{Reason: render.FailChartUnavailable, Detail: fmt.Sprintf(format, args...)}
	}
	def, err := r.a.Product(productID)
	if err != nil {
		return nil, fail("%v", err)
	}
	vl, err := r.a.Versions(ctx, def)
	if err != nil {
		return nil, fail("versions of %s: %v", productID, err)
	}
	v, err := ResolveVersion(vl, version)
	if err != nil {
		return nil, fail("%v", err)
	}
	rel, err := r.a.Release(ctx, def, v, vl)
	if err != nil {
		return nil, fail("%v", err)
	}
	var problems []string
	for _, art := range def.Artifacts {
		if art.Type != domain.ArtifactHelmChart {
			continue
		}
		av := ""
		for _, inst := range rel.Artifacts {
			if inst.ArtifactID == art.ID && inst.Version != "" {
				av = inst.Version
				break
			}
		}
		if av == "" {
			problems = append(problems, fmt.Sprintf("%s: release %s records no version of artifact %s", productID, v, art.ID))
			continue
		}
		rc := catalog.NewRenderContext(productID, v, vl.Versions).WithArtifactVersion(av)
		for _, ch := range art.Channels {
			reader, err := r.a.Registry.ChartPackageReader(ch.Kind)
			if err != nil {
				continue
			}
			loc, err := catalog.RenderLocator(ch, rc)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s channel: %v", ch.Kind, err))
				continue
			}
			pkg, err := reader.ReadChartPackage(ctx, loc, av)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s channel: %v", ch.Kind, err))
				continue
			}
			if len(pkg.Archive) == 0 {
				problems = append(problems, fmt.Sprintf("%s channel: package bytes unavailable", ch.Kind))
				continue
			}
			var meta struct {
				Name       string `yaml:"name"`
				AppVersion string `yaml:"appVersion"`
			}
			_ = yaml.Unmarshal(pkg.ChartYAML, &meta)
			name := meta.Name
			if name == "" {
				name = art.Name
			}
			return &render.Chart{Name: name, Version: av, AppVersion: meta.AppVersion, URI: pkg.URI, Digest: pkg.Digest,
				Representation: pkg.Representation, Archive: pkg.Archive, Evidence: pkg.Evidence}, nil
		}
	}
	if len(problems) == 0 {
		return nil, fail("%s defines no helm-chart artifact with a chart package channel", productID)
	}
	return nil, fail("%s", strings.Join(problems, "; "))
}

// RenderValidator returns the `rendered-diff` knowledge.Validator over this
// app's release-level (chart-default) renders; register it beside the
// validate lane's validators (DESIGN.md §2.3).
func (a *App) RenderValidator(kubeVersion string) knowledge.Validator {
	return render.NewValidator(a.RenderEngine().ReleasePairs(kubeVersion))
}

// EnvironmentPairs returns the environment render pairs of a RenderDiff
// result: what the rendered-change evaluator decides against.
func (r *RenderDiffResult) EnvironmentPairs() []*render.Pair {
	if r == nil {
		return nil
	}
	return r.Pairs
}
