package render

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ChartResolver resolves a product's published chart package at a product
// version through the existing artifact resolution (catalog + ingested
// release + chart package readers); internal/app implements it.
type ChartResolver interface {
	ResolveChart(ctx context.Context, product, version string) (*Chart, error)
}

// Engine renders source/target pairs.
type Engine struct {
	Helm      Renderer
	Kustomize Renderer
	Charts    ChartResolver
}

// PairStatus is the outcome of a source/target render pair.
type PairStatus string

const (
	PairOK PairStatus = "ok"
	// PairFailed: a render failed — UNKNOWN / render-failed, never "no change".
	PairFailed PairStatus = "failed"
	// PairNotApplicable: the target cannot be derived from this configuration
	// (a Kustomize overlay that does not pin the product version).
	PairNotApplicable PairStatus = "not-applicable"
)

// Pair is the source and target render of one configuration.
type Pair struct {
	Product string             `json:"product"`
	From    string             `json:"from"`
	To      string             `json:"to"`
	Scope   domain.RenderScope `json:"scope"`
	Target  Target             `json:"target"`
	Status  PairStatus         `json:"status"`
	// Failure is the first failure (failed / not-applicable).
	Failure    *Failure    `json:"failure,omitempty"`
	FromResult *Result     `json:"fromRender,omitempty"`
	ToResult   *Result     `json:"toRender,omitempty"`
	Diff       *DiffResult `json:"diff,omitempty"`
}

// Complete reports whether an absence of change in this pair is evidence:
// both renders succeeded with complete customer values.
func (p *Pair) Complete() bool {
	return p != nil && p.Status == PairOK && p.Target.ValuesComplete
}

// PairRequest asks for one pair.
type PairRequest struct {
	Product, From, To string
	Target            Target
	Scope             domain.RenderScope
	KubeVersion       string
	APIVersions       []string
	Variant           *Variant
}

// DefaultsTarget is the release-level target: chart defaults only, a fixed
// release name and namespace (the product id), no customer values.
func DefaultsTarget(product string) Target {
	return Target{ID: "chart-defaults", Kind: TargetDefaults, Origin: "chart defaults", Why: "release-level render: chart defaults only",
		ReleaseName: product, Namespace: product, ValuesComplete: true}
}

// RenderPair renders the target's configuration against the From and To
// artifacts and diffs them.
func (e *Engine) RenderPair(ctx context.Context, req PairRequest) *Pair {
	p := &Pair{Product: req.Product, From: req.From, To: req.To, Scope: req.Scope, Target: req.Target}
	if p.Scope == "" {
		p.Scope = domain.RenderEnvironment
	}
	switch req.Target.Kind {
	case TargetKustomize:
		e.kustomizePair(ctx, p, req)
	default:
		e.helmPair(ctx, p, req)
	}
	if p.FromResult.OK() && p.ToResult.OK() {
		nondet := append(append([]string{}, p.FromResult.Nondeterministic...), p.ToResult.Nondeterministic...)
		d := Diff(p.FromResult.Objects, p.ToResult.Objects, DiffOptions{Nondeterministic: nondet})
		p.Diff = &d
		p.Status = PairOK
	} else if p.Status == "" {
		p.Status = PairFailed
		for _, r := range []*Result{p.FromResult, p.ToResult} {
			if r != nil && r.Failure != nil {
				p.Failure = r.Failure
				break
			}
		}
	}
	return p
}

func (e *Engine) helmPair(ctx context.Context, p *Pair, req PairRequest) {
	if e.Helm == nil {
		p.Status, p.Failure = PairFailed, &Failure{Reason: FailRendererUnavailable, Detail: "no Helm renderer configured"}
		return
	}
	render := func(version string) *Result {
		var chart *Chart
		if e.Charts == nil {
			return &Result{Status: StatusFailed, Failure: &Failure{Reason: FailChartUnavailable, Detail: "no chart resolver configured"}}
		}
		c, err := e.Charts.ResolveChart(ctx, req.Product, version)
		if err != nil {
			var f *Failure
			if errors.As(err, &f) {
				return &Result{Status: StatusFailed, Failure: f}
			}
			return &Result{Status: StatusFailed, Failure: &Failure{Reason: FailChartUnavailable, Detail: err.Error()}}
		}
		chart = c
		t := req.Target
		values := t.Values
		if p.Scope == domain.RenderRelease {
			values, t.Set = nil, nil
		}
		return e.Helm.Render(ctx, Request{
			Tool: ToolHelm, Scope: p.Scope, Chart: chart, ReleaseName: t.ReleaseName, Namespace: t.Namespace,
			Values: values, Set: t.Set, KubeVersion: req.KubeVersion, APIVersions: req.APIVersions,
			ValuesComplete: t.ValuesComplete, IncompleteReason: t.IncompleteReason, Variant: req.Variant,
		})
	}
	p.FromResult = render(req.From)
	p.ToResult = render(req.To)
}

func (e *Engine) kustomizePair(ctx context.Context, p *Pair, req PairRequest) {
	if e.Kustomize == nil {
		p.Status, p.Failure = PairFailed, &Failure{Reason: FailRendererUnavailable, Detail: "no Kustomize renderer configured"}
		return
	}
	in := *req.Target.Kustomize
	base := Request{Tool: ToolKustomize, Scope: p.Scope, ValuesComplete: true}
	from := base
	fromIn := in
	fromIn.Substitutions = nil
	from.Kustomize = &fromIn
	p.FromResult = e.Kustomize.Render(ctx, from)
	files, _, _, err := KustomizeInputs(in.Root, in.Dir)
	if err != nil {
		p.ToResult = &Result{Status: StatusFailed, Failure: &Failure{Reason: FailKustomizeDependency, Detail: err.Error()}}
		return
	}
	subs, err := PlanVersionSubstitution(in.Root, files, req.From, req.To)
	if err != nil {
		p.ToResult = &Result{Status: StatusFailed, Failure: &Failure{Reason: FailKustomizeDependency, Detail: err.Error()}}
		return
	}
	if len(subs) == 0 {
		p.Status = PairNotApplicable
		p.Failure = &Failure{Reason: FailUnsupportedFeature, Detail: fmt.Sprintf(
			"overlay %s does not pin %s %s (no images newTag or remote resource carries the version), so the target render cannot be derived from it",
			in.Dir, req.Product, req.From)}
		return
	}
	to := base
	toIn := in
	toIn.Substitutions = subs
	to.Kustomize = &toIn
	p.ToResult = e.Kustomize.Render(ctx, to)
}

// Describe summarises a pair on one line.
func (p *Pair) Describe() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s → %s [%s] %s", p.Product, p.From, p.To, p.Target.Kind, p.Target.Origin)
	switch p.Status {
	case PairOK:
		fmt.Fprintf(&b, ": %d rendered changes", len(p.Diff.Changes))
	default:
		if p.Failure != nil {
			fmt.Fprintf(&b, ": %s (%s)", strings.ToUpper(string(p.Status)), p.Failure.Reason)
		}
	}
	return b.String()
}
