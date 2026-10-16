package renderdiff

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/helm"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/normalize"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// Producer is the producer string of every artifact and evidence record this
// package derives.
const Producer = "renderdiff@v1"

// Renderer implements investigate.EnvironmentRenderer over internal/render.
type Renderer struct {
	st     investigate.ArtifactStore
	engine *render.Engine
	// targets detects the deployments to render (app.RenderTargets).
	targets func(product string, opts app.RenderOptions) ([]render.Target, []string, error)
}

var _ investigate.EnvironmentRenderer = (*Renderer)(nil)

// New returns the environment renderer of an app: its render engine (helm +
// kustomize, content-addressed render cache, chart resolution through the
// product's channels) and its target detection. Artifacts go to st.
func New(a *app.App, st investigate.ArtifactStore) investigate.EnvironmentRenderer {
	return &Renderer{st: st, engine: a.RenderEngine(), targets: a.RenderTargets}
}

// newRenderer builds a renderer from its parts (tests).
func newRenderer(st investigate.ArtifactStore, eng *render.Engine, targets func(string, app.RenderOptions) ([]render.Target, []string, error)) *Renderer {
	return &Renderer{st: st, engine: eng, targets: targets}
}

// inputs are the render inputs a snapshot names.
type inputs struct {
	repo   string
	values []string
	kube   string
}

func inputsOf(s domain.EnvironmentSnapshot, cfg investigate.Config) inputs {
	var in inputs
	if s.Repo != nil {
		in.repo = s.Repo.Path
	}
	for _, x := range s.Inputs {
		if x.Status == domain.CollectionUnavailable {
			continue
		}
		switch x.Kind {
		case domain.InputGitOpsRepo:
			if in.repo == "" {
				in.repo = x.Path
			}
		case domain.InputValues:
			in.values = append(in.values, x.Path)
		case domain.InputClusterVersion:
			if in.kube == "" {
				in.kube = x.Value
			}
		}
	}
	if s.KubernetesVersion != "" {
		in.kube = s.KubernetesVersion
	}
	if cfg.KubeVersion != "" {
		in.kube = cfg.KubeVersion
	}
	return in
}

// RenderEnvironment renders every deployment of the product the snapshot's
// environment declares, at the change's from and to versions, and indexes
// the diffs. A target that cannot be rendered completely is a pair with a
// non-ok status and its reason; only infrastructure failures (the artifact
// store, a missing edge artifact, cancellation) are errors.
func (r *Renderer) RenderEnvironment(ctx context.Context, change *investigate.ChangeResult, snap *investigate.SnapshotResult, cfg investigate.Config) (*investigate.RenderResult, error) {
	if change == nil || snap == nil {
		return nil, investigate.PermanentError(domain.StageAssemble, errors.New("renderdiff: change and snapshot results are required"))
	}
	if r.st == nil {
		return nil, investigate.PermanentError(domain.StageAssemble, errors.New("renderdiff: no artifact store"))
	}
	out := &investigate.RenderResult{}
	if !cfg.Render {
		out.Notes = append(out.Notes, "environment renders disabled (config.render is false): no environment render exists")
		return out, nil
	}
	edge, err := r.loadEdge(ctx, change)
	if err != nil {
		return nil, err
	}
	in := inputsOf(snap.Snapshot, cfg)
	if in.repo == "" && len(in.values) == 0 {
		out.Notes = append(out.Notes, "the snapshot names no repository and no values files: there is no deployment to render")
		return out, nil
	}
	product := string(edge.Product.ID)
	targets, notes, err := r.targets(product, app.RenderOptions{Repo: in.repo, ValuesFiles: in.values, KubeVersion: in.kube})
	rel := relativizer(in.repo)
	for _, n := range notes {
		out.Notes = append(out.Notes, rel(n))
	}
	if err != nil {
		// an unreadable environment is a recorded gap, not an infrastructure failure
		out.Notes = append(out.Notes, "render target detection failed: "+rel(err.Error()))
		return out, nil
	}
	targets, skipped := dropLayeredValuesTargets(targets, len(in.values) > 0)
	for _, n := range skipped {
		out.Notes = append(out.Notes, rel(n))
	}
	ix := newIndexer(change.Record, edge, rel)
	seen := map[string]int{}
	for _, t := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p := r.engine.RenderPair(ctx, render.PairRequest{Product: product, From: edge.From.String(), To: edge.To.String(),
			Target: t, Scope: domain.RenderEnvironment, KubeVersion: in.kube})
		name := rel(t.ID)
		if n := seen[name]; n > 0 {
			name = fmt.Sprintf("%s#%d", name, n+1)
		}
		seen[rel(t.ID)]++
		status, reason := r.assess(ctx, p, edge)
		mp, arts, evs, err := ix.pair(ctx, r.st, p, name, status, rel(reason))
		if err != nil {
			return nil, err
		}
		out.Pairs = append(out.Pairs, mp)
		out.Artifacts = append(out.Artifacts, arts...)
		out.Evidence = append(out.Evidence, evs...)
	}
	sort.Slice(out.Pairs, func(i, j int) bool { return out.Pairs[i].Target < out.Pairs[j].Target })
	sort.Slice(out.Artifacts, func(i, j int) bool { return out.Artifacts[i].Path < out.Artifacts[j].Path })
	out.Evidence = uniqueEvidence(out.Evidence)
	if len(out.Pairs) == 0 {
		out.Notes = append(out.Notes, "no deployment of "+product+" was found in the environment: no environment render exists")
	}
	return out, nil
}

// loadEdge reads the upgrade edge the change record references.
func (r *Renderer) loadEdge(ctx context.Context, change *investigate.ChangeResult) (*domain.UpgradeEdge, error) {
	id := change.Record.Edge
	for _, a := range change.Artifacts {
		if a.ID != id {
			continue
		}
		rc, err := r.st.Open(ctx, a.Digest)
		if err != nil {
			if errors.Is(err, investigate.ErrNotFound) {
				return nil, investigate.PermanentError(domain.StageAssemble, fmt.Errorf("renderdiff: upgrade edge %s: %w", id, err))
			}
			return nil, investigate.TransientError(domain.StageAssemble, fmt.Errorf("renderdiff: upgrade edge %s: %w", id, err))
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			return nil, investigate.TransientError(domain.StageAssemble, fmt.Errorf("renderdiff: upgrade edge %s: %w", id, err))
		}
		var e domain.UpgradeEdge
		if err := json.Unmarshal(b, &e); err != nil {
			return nil, investigate.PermanentError(domain.StageAssemble, fmt.Errorf("renderdiff: upgrade edge %s: %w", id, err))
		}
		return &e, nil
	}
	return nil, investigate.PermanentError(domain.StageAssemble, fmt.Errorf("renderdiff: the change result does not carry its upgrade edge artifact %s", id))
}

// assess decides a pair's collection status (DESIGN §2.4): ok only for a
// complete pair — both renders succeeded, the customer's values are
// complete, and the rendered chart defines every top-level values key the
// customer sets (chart coverage).
func (r *Renderer) assess(ctx context.Context, p *render.Pair, edge *domain.UpgradeEdge) (domain.CollectionStatus, string) {
	switch p.Status {
	case render.PairOK:
		if !p.Target.ValuesComplete {
			return domain.CollectionPartial, "values-incomplete: " + p.Target.IncompleteReason
		}
		unc, undecided := r.coverage(ctx, p)
		if undecided != "" {
			return domain.CollectionPartial, "chart-coverage: undecided: " + undecided
		}
		if len(unc) > 0 {
			return domain.CollectionPartial, fmt.Sprintf("chart-coverage: values keys %s are defined by neither %s chart (%s, %s) nor its dependencies: the render did not consume them",
				strings.Join(unc, ", "), p.Product, p.From, p.To)
		}
		return domain.CollectionOK, ""
	case render.PairNotApplicable:
		detail := ""
		if p.Failure != nil {
			detail = p.Failure.Detail
		}
		if imgs := productImages(p.FromResult, edge); len(imgs) > 0 {
			return domain.CollectionUnavailable, fmt.Sprintf("%s: the overlay deploys %s images (%s) without pinning the version, so its target render cannot be derived: %s",
				render.FailUnsupportedFeature, p.Product, strings.Join(imgs, ", "), detail)
		}
		return domain.CollectionNotApplicable, "version-independent: the overlay neither pins nor deploys " + p.Product +
			" (no image or remote resource carries its version, no object runs its images), so the upgrade does not change what it renders: " + detail
	}
	reason := "render-failed"
	if p.Failure != nil {
		reason = string(p.Failure.Reason) + ": " + p.Failure.Detail
	}
	if p.TargetRejects() {
		reason = string(render.FailInvalidValues) + ": the target chart rejects the values the source chart rendered: " + p.Failure.Detail
	}
	if p.FromResult.OK() || p.ToResult.OK() {
		return domain.CollectionPartial, reason
	}
	return domain.CollectionUnavailable, reason
}

// coverage returns the customer's top-level values keys that neither the
// from nor the to chart defines (nor names as a dependency), or why that
// could not be decided. Kustomize targets carry no values.
func (r *Renderer) coverage(ctx context.Context, p *render.Pair) (uncovered []string, undecided string) {
	t := p.Target
	if t.Kind == render.TargetKustomize {
		return nil, ""
	}
	keys := map[string]bool{}
	for _, l := range t.Values {
		flat, err := normalize.FlattenValues(l.Content)
		if err != nil {
			return nil, fmt.Sprintf("values %s: %v", l.Origin, err)
		}
		for _, f := range flat {
			keys[topSegment(f.Path)] = true
		}
	}
	for _, s := range t.Set {
		keys[topSegment(s.Path)] = true
	}
	delete(keys, "global") // helm's global values are read by every chart
	if len(keys) == 0 {
		return nil, ""
	}
	defined := map[string]bool{}
	for _, v := range []string{p.From, p.To} {
		defaults, why := r.engine.ChartDefaults(ctx, p.Product, v)
		if why != "" {
			return nil, why
		}
		for path := range defaults {
			defined[topSegment(path)] = true
		}
		for _, d := range r.dependencies(ctx, p.Product, v) {
			defined[d] = true
		}
	}
	for k := range keys {
		if !defined[k] {
			uncovered = append(uncovered, k)
		}
	}
	sort.Strings(uncovered)
	return uncovered, ""
}

// dependencies are the subchart names (or aliases) a chart declares: values
// under them reach the subchart, not the chart's own defaults.
func (r *Renderer) dependencies(ctx context.Context, product, version string) []string {
	if r.engine.Charts == nil {
		return nil
	}
	c, err := r.engine.Charts.ResolveChart(ctx, product, version)
	if err != nil {
		return nil
	}
	var raw []byte
	switch {
	case len(c.Archive) > 0:
		arc, err := helm.ReadArchive(bytes.NewReader(c.Archive))
		if err != nil {
			return nil
		}
		raw = arc.ChartYAML
	case c.Path != "":
		raw, _ = os.ReadFile(filepath.Join(c.Path, "Chart.yaml"))
	}
	var meta struct {
		Dependencies []struct {
			Name  string `yaml:"name"`
			Alias string `yaml:"alias"`
		} `yaml:"dependencies"`
	}
	if yaml.Unmarshal(raw, &meta) != nil {
		return nil
	}
	var out []string
	for _, d := range meta.Dependencies {
		if d.Alias != "" {
			out = append(out, d.Alias)
		} else if d.Name != "" {
			out = append(out, d.Name)
		}
	}
	return out
}

// topSegment is the first segment of a flattened values path ("a.b[0]" →
// "a", `["a.b"].c` → "a.b").
func topSegment(p string) string {
	if strings.HasPrefix(p, "[") {
		if q, err := unquotePrefix(p[1:]); err == nil {
			return q
		}
	}
	if i := strings.IndexAny(p, ".["); i >= 0 {
		return p[:i]
	}
	return p
}

// productImages returns the product's container image repositories (from
// the edge's artifacts) that a render runs.
func productImages(res *render.Result, edge *domain.UpgradeEdge) []string {
	if !res.OK() || edge == nil {
		return nil
	}
	repos := map[string]bool{}
	for _, a := range edge.Artifacts {
		if a.Type != domain.ArtifactContainerImage {
			continue
		}
		for _, inst := range []*domain.ArtifactInstance{a.From, a.To} {
			if inst != nil && inst.Coordinate != "" {
				repos[canonicalRepo(inst.Coordinate)] = true
			}
		}
	}
	if len(repos) == 0 {
		return nil
	}
	found := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, x := range t {
				if s, ok := x.(string); ok && k == "image" && repos[canonicalRepo(s)] {
					found[canonicalRepo(s)] = true
				}
				walk(x)
			}
		case []any:
			for _, x := range t {
				walk(x)
			}
		}
	}
	for _, o := range res.Objects {
		walk(o.Body)
	}
	out := make([]string, 0, len(found))
	for k := range found {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// canonicalRepo is an image reference's repository without tag/digest and
// without the implicit Docker Hub prefixes.
func canonicalRepo(ref string) string {
	if i := strings.IndexByte(ref, '@'); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndexByte(ref, ':'); i >= 0 && !strings.Contains(ref[i:], "/") {
		ref = ref[:i]
	}
	for _, p := range []string{"docker.io/", "index.docker.io/"} {
		ref = strings.TrimPrefix(ref, p)
	}
	return strings.TrimPrefix(ref, "library/")
}

// dropLayeredValuesTargets removes repo-discovered values-file targets whose
// file is already a values layer of an Argo CD / Flux / Helmfile target: the
// file alone is not how the environment deploys the product, and rendering
// it alone would index changes of a configuration nobody runs. Explicit
// --values targets are kept.
func dropLayeredValuesTargets(targets []render.Target, explicitValues bool) ([]render.Target, []string) {
	if explicitValues {
		return targets, nil
	}
	layered := map[string]string{}
	for _, t := range targets {
		if t.Kind == render.TargetValuesFiles || t.Kind == render.TargetKustomize {
			continue
		}
		for _, l := range t.Values {
			if _, ok := layered[filepath.Clean(l.Origin)]; !ok {
				layered[filepath.Clean(l.Origin)] = t.ID
			}
		}
	}
	var kept []render.Target
	var notes []string
	for _, t := range targets {
		if t.Kind == render.TargetValuesFiles && len(t.Values) == 1 {
			if by, ok := layered[filepath.Clean(t.Values[0].Origin)]; ok {
				notes = append(notes, fmt.Sprintf("values file %s: rendered as a values layer of %s, not on its own", t.Origin, by))
				continue
			}
		}
		kept = append(kept, t)
	}
	return kept, notes
}

// relativizer rewrites paths under the repository root to repo-relative
// form, so target names, reasons and provenance do not depend on where the
// repository was checked out (reproducible ids).
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

func uniqueEvidence(evs []domain.Evidence) []domain.Evidence {
	sort.Slice(evs, func(i, j int) bool { return evs[i].ID < evs[j].ID })
	out := evs[:0]
	for i, e := range evs {
		if i > 0 && e.ID == evs[i-1].ID {
			continue
		}
		out = append(out, e)
	}
	return out
}
