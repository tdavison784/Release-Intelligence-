package render

// PO-7a (product-owner decision, briefs/renderfirst.md): render-backed
// classification of values keys the customer SETS whose key the target release
// removes — the counterpart of the PO-3 counterfactual (unset.go). The
// customer's own render answers what the deterministic join could only
// speculate about:
//
//   - the target chart refuses the customer's values at render time
//     (values.schema.json or a chart-authored fail/required) and the refusal
//     names the removed key or an ancestor of it → rejected: the upgrade as
//     configured fails before rendering anything (decisive on its own);
//   - the counterfactual — the SOURCE chart rendered with the customer's
//     values and the key unset (an explicit null layer deletes it, as in Helm
//     values coalescing) — attributes changes of the customer's upgrade delta
//     (From→To with their values) to the key → attributable;
//   - every complete render attributes nothing → no-effect (the key never
//     reached the customer's rendered deployment).
//
// Anything else (no Helm deployment, failed renders, incomplete values, a
// refusal naming no key of the change, keys set via --set parameters the
// counterfactual cannot unset) is undecided, and the join keeps today's
// verdict.

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/impact"
)

// SetValues implements impact.SetValuesEvaluator over the environment's Helm
// render pairs (`ri impact --render`).
type SetValues struct {
	Engine            *Engine
	Product, From, To string
	// Pairs are the environment pairs (customer configuration); Kustomize and
	// release-scope pairs are ignored (customer values reach neither).
	Pairs       []*Pair
	KubeVersion string
	APIVersions []string
	Ctx         context.Context

	mu   sync.Mutex
	memo map[string]impact.SetValuesResult

	once        sync.Once
	defaults    map[string]string // source chart values: flattened path → JSON
	defaultsErr string
}

var _ impact.SetValuesEvaluator = (*SetValues)(nil)

// SetAttribution is one key's attribution in one deployment.
type SetAttribution struct {
	Pair    *Pair
	Changes []Change // changes of the pair's upgrade delta attributable to the keys
	// Counterfactual is the source render with the keys unset.
	Counterfactual *Result
}

// EvaluateSetValues implements impact.SetValuesEvaluator.
func (s *SetValues) EvaluateSetValues(c domain.Change) impact.SetValuesResult {
	key := strings.Join(c.Subjects, ",")
	s.mu.Lock()
	if r, ok := s.memo[key]; ok {
		s.mu.Unlock()
		return r
	}
	s.mu.Unlock()
	r := s.evaluate(c.Subjects)
	s.mu.Lock()
	if s.memo == nil {
		s.memo = map[string]impact.SetValuesResult{}
	}
	s.memo[key] = r
	s.mu.Unlock()
	return r
}

func undecided(needed ...string) impact.SetValuesResult {
	return impact.SetValuesResult{Outcome: impact.SetValuesUndecided, Needed: needed}
}

// fromDefaults lazily flattens the source chart's values — the chart the
// environment pairs render — for the no-effect coverage guard.
func (s *SetValues) fromDefaults(ctx context.Context) (map[string]string, string) {
	s.once.Do(func() { s.defaults, s.defaultsErr = chartDefaults(ctx, s.Engine, s.Product, s.From) })
	return s.defaults, s.defaultsErr
}

func (s *SetValues) evaluate(keys []string) impact.SetValuesResult {
	ctx := s.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	var helmPairs []*Pair
	for _, p := range s.Pairs {
		if p.Target.Kind != TargetKustomize && p.Scope != domain.RenderRelease {
			helmPairs = append(helmPairs, p)
		}
	}
	if len(helmPairs) == 0 {
		return undecided("no Helm deployment of " + s.Product + " was rendered with your configuration")
	}
	// A refusal that names a key of the change is decisive on its own, even
	// when other deployments could not be rendered.
	for _, p := range helmPairs {
		if p.TargetRejects() && refusalNames(refusalText(p), keys) {
			ev := p.rejectionEvidence()
			return impact.SetValuesResult{
				Outcome: impact.SetValuesRejected, Refusal: refusalText(p), Records: []domain.Evidence{ev},
				Rejection: &domain.ImpactMatch{Kind: domain.MatchRenderedChange,
					Subject:  s.To + " refuses the customer's values at render time: " + refusalText(p),
					Evidence: []domain.EvidenceID{ev.ID}},
			}
		}
	}
	override, why := nullLayer(keys)
	if why != "" {
		return undecided(why)
	}
	if setPathTouches(helmPairs, keys) {
		return undecided("the key is set through --set parameters (Argo/Helmfile-style overrides), which a values layer cannot unset")
	}
	var (
		matches  []domain.ImpactMatch
		records  []domain.Evidence
		needed   []string
		cleared  int
		compared int
		// clearRecords: environment-render state records of every cleared pair
		clearRecords []domain.Evidence
	)
	for _, p := range helmPairs {
		a, why := s.attribute(ctx, p, keys, override)
		switch {
		case why != "":
			needed = append(needed, p.Target.ID+": "+why)
		case len(a.Changes) > 0:
			for _, ch := range a.Changes {
				ev := p.Evidence(ch, false)
				records = append(records, ev)
				matches = append(matches, domain.ImpactMatch{Kind: domain.MatchRenderedChange,
					Subject: strings.Join(keys, ", ") + " → " + ch.Summary(false) + " (" + p.Target.ID + ")", Evidence: []domain.EvidenceID{ev.ID}})
			}
		case !p.Target.ValuesComplete:
			needed = append(needed, p.Target.ID+": values incomplete ("+p.Target.IncompleteReason+"), so nothing attributable decides nothing")
		default:
			cleared++
			compared += len(p.FromResult.Objects)
			subject := "with " + strings.Join(keys, ", ") + " as the customer sets it"
			clearRecords = append(clearRecords,
				p.stateEvidence(p.FromResult, p.From, subject, objOcc(p.FromResult.Objects)),
				p.stateEvidence(a.Counterfactual, p.From+" (counterfactual: "+strings.Join(keys, ", ")+" unset)", subject, objOcc(a.Counterfactual.Objects)))
		}
	}
	if len(matches) > 0 {
		// an attributable change stands even when another deployment could
		// not be rendered
		return impact.SetValuesResult{Outcome: impact.SetValuesAttributable, Matches: matches, Records: records}
	}
	// chart-coverage guard: a key the rendered chart does not define never
	// reached any rendered deployment, so "nothing attributable" would be
	// vacuous — the pair that consumes it (istio's cni chart under the
	// istiod render) was never rendered. Undecided, never no-effect.
	if defaults, derr := s.fromDefaults(ctx); derr != "" {
		needed = append(needed, "the chart's values could not be read: "+derr)
	} else if unc := uncoveredKeys(defaults, keys); len(unc) > 0 {
		needed = append(needed, "the rendered chart defines none of "+strings.Join(unc, ", ")+
			" (another chart of "+s.Product+" owns them; no deployment consuming them was rendered)")
	}
	if len(needed) > 0 || cleared == 0 {
		return undecided(needed...)
	}
	ids := make([]domain.EvidenceID, 0, len(clearRecords))
	for _, e := range clearRecords {
		ids = append(ids, e.ID)
	}
	return impact.SetValuesResult{Outcome: impact.SetValuesNoEffect, Records: clearRecords, Checks: []domain.ImpactCheck{{
		Dimension: domain.DimensionRender, Facts: compared, Evidence: ids,
		Subjects: []string{fmt.Sprintf("rendered %s with %s unset: nothing attributable in %d deployment(s)", s.From, strings.Join(keys, ", "), cleared)},
		Render:   &domain.RenderCheck{Outcome: domain.RenderNoAttributableChange, Key: strings.Join(keys, ", "), Counterfactual: true},
	}}}
}

// attribute renders the pair's source chart with the override layer and
// returns the changes of the pair's upgrade delta the counterfactual
// attributes to the keys (why != "" when it cannot decide).
func (s *SetValues) attribute(ctx context.Context, p *Pair, keys []string, override []byte) (a SetAttribution, why string) {
	a.Pair = p
	switch {
	case p.Status != PairOK:
		reason := string(p.Status)
		if p.Failure != nil {
			reason = string(p.Failure.Reason) + ": " + p.Failure.Detail
		}
		return a, "render failed (" + reason + ")"
	case p.Diff == nil:
		return a, "no upgrade delta"
	case s.Engine == nil || s.Engine.Helm == nil || s.Engine.Charts == nil:
		return a, "no renderer configured"
	}
	chart, err := s.Engine.Charts.ResolveChart(ctx, s.Product, s.From)
	if err != nil {
		return a, "source chart: " + err.Error()
	}
	t := p.Target
	values := append(append([]ValuesLayer{}, t.Values...), ValuesLayer{Origin: "counterfactual: " + strings.Join(keys, ", ") + " unset", Content: override})
	cf := s.Engine.Helm.Render(ctx, Request{
		Tool: ToolHelm, Scope: p.Scope, Chart: chart, ReleaseName: t.ReleaseName, Namespace: t.Namespace,
		Values: values, Set: t.Set, KubeVersion: s.KubeVersion, APIVersions: s.APIVersions,
		ValuesComplete: t.ValuesComplete, IncompleteReason: t.IncompleteReason, NoProbe: true,
		Variant: &Variant{Name: strings.Join(keys, ",") + "=unset", Question: "does the customer's setting of " + strings.Join(keys, ", ") + " change this deployment?"},
	})
	a.Counterfactual = cf
	if !cf.OK() {
		reason := "unknown"
		if cf.Failure != nil {
			reason = string(cf.Failure.Reason) + ": " + cf.Failure.Detail
		}
		return a, "counterfactual render failed (" + reason + ")"
	}
	nondet := append(append([]string{}, p.FromResult.Nondeterministic...), cf.Nondeterministic...)
	// the counterfactual's effect in the pair-delta orientation: with the key
	// (FromResult) → without it (cf), so a removal here intersects the pair's
	// From→To removal (unset.go mirrors this: old state → new state)
	effect := Diff(p.FromResult.Objects, cf.Objects, DiffOptions{Nondeterministic: nondet})
	inEffect := map[string]bool{}
	for _, c := range effect.Changes {
		inEffect[changeKey(c)] = true
	}
	for _, c := range p.Diff.Changes {
		if inEffect[changeKey(c)] {
			a.Changes = append(a.Changes, c)
		}
	}
	return a, ""
}

// nullLayer builds the counterfactual values layer: every key explicitly null,
// which deletes it from the merged values (Helm values coalescing).
func nullLayer(keys []string) ([]byte, string) {
	root := map[string]any{}
	for _, k := range keys {
		segs, err := splitValuesPath(k)
		if err != nil {
			return nil, fmt.Sprintf("key %s: %v", k, err)
		}
		setPath(root, segs, nil)
	}
	b, err := yaml.Marshal(root)
	if err != nil {
		return nil, err.Error()
	}
	return b, ""
}

// setPathTouches reports whether any pair passes a --set override at or under
// one of the keys (or the key at or under it): a later values layer cannot
// delete an override, so the counterfactual could not unset the key.
func setPathTouches(pairs []*Pair, keys []string) bool {
	for _, p := range pairs {
		for _, s := range p.Target.Set {
			for _, k := range keys {
				if pathOverlaps(s.Path, k) {
					return true
				}
			}
		}
	}
	return false
}

// pathOverlaps reports whether two values paths name the same key or one lies
// under the other, in dotted syntax ('[' after the shared prefix is a list
// index and counts as a boundary too).
func pathOverlaps(a, b string) bool {
	shorter, longer := a, b
	if len(a) > len(b) {
		shorter, longer = b, a
	}
	if !strings.HasPrefix(longer, shorter) {
		return false
	}
	rest := longer[len(shorter):]
	return rest == "" || rest[0] == '.' || rest[0] == '['
}

// refusalText is the bounded refusal message of a target render that refused
// the customer's values.
func refusalText(p *Pair) string {
	if p != nil && p.ToResult != nil && p.ToResult.Failure != nil {
		return boundDetail(p.ToResult.Failure.Detail)
	}
	return "the chart rejected these values"
}

// rejectionEvidence is the environment-render record of a target render that
// refused the customer's values: the target's render provenance with the
// refusal as the excerpt (scope environment, R5 shape).
func (p *Pair) rejectionEvidence() domain.Evidence {
	from, to := p.FromResult.Provenance, p.ToResult.Provenance
	uri := to.ChartURI
	if uri == "" {
		uri = "render:" + p.Product + "@" + p.To
	}
	refusal := refusalText(p)
	ev := domain.NewEvidence(domain.EvidenceRenderedDiff, Producer, uri, "values refused",
		fmt.Sprintf("environment render of %s %s with the customer's values: refused — %s", p.Product, p.To, refusal),
		to.OutputDigest, to.RenderedAt)
	rp := to.Domain()
	rp.FromArtifact = &domain.RenderArtifact{Name: from.Chart, Version: from.ChartVersion, Digest: from.ArtifactDigest, OutputDigest: from.OutputDigest}
	rp.ToArtifact = &domain.RenderArtifact{Name: to.Chart, Version: to.ChartVersion, Digest: to.ArtifactDigest, OutputDigest: to.OutputDigest}
	rp.Change = "render-rejected"
	ev.Render = &rp
	return ev
}

// refusalNames reports whether the renderer's refusal text names one of the
// change's values keys or an ancestor of one. Helm's schema failures name
// JSON-schema paths ("- (root): Additional property legacy is not allowed"
// names the top-level property when the chart removed a whole section);
// chart-authored fail messages usually name the full key. A refusal naming an
// unrelated key decides nothing about this change.
func refusalNames(detail string, subjects []string) bool {
	for _, k := range subjects {
		segs, err := splitValuesPath(k)
		if err != nil {
			continue
		}
		for n := len(segs); n > 0; n-- {
			if containsWord(detail, strings.Join(segs[:n], ".")) {
				return true
			}
		}
	}
	return false
}

// containsWord reports whether s contains pat as a whole word: the byte on
// each side is absent or not a word byte ('.' counts as a boundary, so the
// pattern "legacy" matches inside "legacy.feature").
func containsWord(s, pat string) bool {
	if pat == "" {
		return false
	}
	for i := 0; i+len(pat) <= len(s); i++ {
		if s[i:i+len(pat)] != pat {
			continue
		}
		before := i == 0 || !isWordByte(s[i-1])
		after := i+len(pat) == len(s) || !isWordByte(s[i+len(pat)])
		if before && after {
			return true
		}
	}
	return false
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
