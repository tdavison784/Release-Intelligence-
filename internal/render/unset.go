package render

// PO-3 (product-owner decision): render-backed classification of changed
// chart defaults and new keys the customer leaves UNSET. For such a key the
// new default is exactly what reaches the customer, so "your values do not
// set it" cannot mean "not affected". The bounded counterfactual of R8
// decides it: render the target with the customer's configuration once as
// they will get it and once with the key pinned to its previous state (the
// source chart's default, or absent for a key new in the target); what
// differs — and also appears in the customer's actual upgrade delta (source
// vs target with their values) — is attributable to the key.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/helm"
	"github.com/tdavison784/release-intelligence/internal/impact"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// UnsetValues implements impact.UnsetValuesEvaluator over the environment's
// Helm render pairs (`ri impact --render`).
type UnsetValues struct {
	Engine            *Engine
	Product, From, To string
	// Pairs are the environment pairs (customer configuration); Kustomize
	// pairs are ignored (Helm values do not reach them).
	Pairs       []*Pair
	KubeVersion string
	APIVersions []string
	Ctx         context.Context

	once       sync.Once
	defaults   map[string]string // source chart values: flattened path → JSON
	defaultErr string
	mu         sync.Mutex
	memo       map[string]impact.ConditionResult
}

var _ impact.UnsetValuesEvaluator = (*UnsetValues)(nil)

// UnsetAttribution is one key's attribution in one deployment.
type UnsetAttribution struct {
	Pair    *Pair
	Changes []Change // changes of the pair's upgrade delta attributable to the keys
	// Counterfactual is the target render with the keys pinned to their
	// previous state.
	Counterfactual *Result
}

// EvaluateUnsetValues implements impact.UnsetValuesEvaluator.
func (u *UnsetValues) EvaluateUnsetValues(c domain.Change, kind string) impact.ConditionResult {
	key := kind + "|" + strings.Join(c.Subjects, ",")
	u.mu.Lock()
	if r, ok := u.memo[key]; ok {
		u.mu.Unlock()
		return r
	}
	u.mu.Unlock()
	r := u.evaluate(c.Subjects, kind)
	u.mu.Lock()
	if u.memo == nil {
		u.memo = map[string]impact.ConditionResult{}
	}
	u.memo[key] = r
	u.mu.Unlock()
	return r
}

func unavailable(reasons ...string) impact.ConditionResult {
	return impact.ConditionResult{Value: impact.Unknown, Reason: domain.UnknownEnvironmentVisibilityGap, Needed: reasons}
}

func (u *UnsetValues) evaluate(keys []string, kind string) impact.ConditionResult {
	ctx := u.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	var helmPairs []*Pair
	for _, p := range u.Pairs {
		if p.Target.Kind != TargetKustomize {
			helmPairs = append(helmPairs, p)
		}
	}
	if len(helmPairs) == 0 {
		return unavailable("no Helm deployment of " + u.Product + " was rendered with your configuration")
	}
	override, err := u.pinPrevious(ctx, keys, kind)
	if err != "" {
		return unavailable(err)
	}
	var (
		matches  []domainMatch
		records  []domain.Evidence
		needed   []string
		compared int
		cleared  int
	)
	for _, p := range helmPairs {
		a, why := u.Attribute(ctx, p, keys, override)
		switch {
		case why != "":
			needed = append(needed, p.Target.ID+": "+why)
		case len(a.Changes) > 0:
			for _, ch := range a.Changes {
				ev := p.Evidence(ch, false)
				records = append(records, ev)
				matches = append(matches, domainMatch{subject: strings.Join(keys, ", ") + " → " + ch.Summary(false) + " (" + p.Target.ID + ")", ev: ev.ID})
			}
		case !p.Target.ValuesComplete:
			needed = append(needed, p.Target.ID+": values incomplete ("+p.Target.IncompleteReason+"), so nothing attributable decides nothing")
		default:
			cleared++
			compared += len(p.ToResult.Objects)
		}
	}
	if len(matches) > 0 {
		// a rendered change attributable to the key stands even when another
		// deployment could not be rendered
		r := impact.ConditionResult{Value: impact.True, Records: records}
		for _, m := range matches {
			r.Matches = append(r.Matches, domain.ImpactMatch{Kind: domain.MatchRenderedChange, Subject: m.subject, Evidence: []domain.EvidenceID{m.ev}})
		}
		return r
	}
	if len(needed) > 0 || cleared == 0 {
		return unavailable(needed...)
	}
	return impact.ConditionResult{Value: impact.False, Checks: []domain.ImpactCheck{{
		Dimension: domain.DimensionRender, Facts: compared,
		Subjects: []string{fmt.Sprintf("rendered %s with %s pinned to its previous state: nothing attributable in %d deployment(s)", u.To, strings.Join(keys, ", "), cleared)},
	}}}
}

type domainMatch struct {
	subject string
	ev      domain.EvidenceID
}

// Attribute renders the pair's target with the override layer and returns
// the changes of the pair's upgrade delta the counterfactual attributes to
// the keys (why != "" when it cannot decide).
func (u *UnsetValues) Attribute(ctx context.Context, p *Pair, keys []string, override []byte) (a UnsetAttribution, why string) {
	a.Pair = p
	switch {
	case p.Status != PairOK:
		reason := string(p.Status)
		if p.Failure != nil {
			reason = string(p.Failure.Reason) + ": " + p.Failure.Detail
		}
		return a, "render failed (" + reason + ")"
	case u.Engine == nil || u.Engine.Helm == nil || u.Engine.Charts == nil:
		return a, "no renderer configured"
	}
	chart, err := u.Engine.Charts.ResolveChart(ctx, u.Product, u.To)
	if err != nil {
		return a, "target chart: " + err.Error()
	}
	t := p.Target
	values := append(append([]ValuesLayer{}, t.Values...), ValuesLayer{Origin: "counterfactual: " + strings.Join(keys, ", ") + " pinned to the previous state", Content: override})
	if p.Scope == domain.RenderRelease {
		values = values[len(values)-1:]
		t.Set = nil
	}
	cf := u.Engine.Helm.Render(ctx, Request{
		Tool: ToolHelm, Scope: p.Scope, Chart: chart, ReleaseName: t.ReleaseName, Namespace: t.Namespace,
		Values: values, Set: t.Set, KubeVersion: u.KubeVersion, APIVersions: u.APIVersions,
		ValuesComplete: t.ValuesComplete, IncompleteReason: t.IncompleteReason, NoProbe: true,
		Variant: &Variant{Name: strings.Join(keys, ",") + "=previous", Question: "does the target's default of " + strings.Join(keys, ", ") + " change this deployment?"},
	})
	a.Counterfactual = cf
	if !cf.OK() {
		reason := "unknown"
		if cf.Failure != nil {
			reason = string(cf.Failure.Reason) + ": " + cf.Failure.Detail
		}
		return a, "counterfactual render failed (" + reason + ")"
	}
	nondet := append(append([]string{}, p.ToResult.Nondeterministic...), p.FromResult.Nondeterministic...)
	effect := Diff(cf.Objects, p.ToResult.Objects, DiffOptions{Nondeterministic: nondet})
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

// changeKey identifies a change across two diffs that end in the same target
// render: version-independent object, path, class and target value.
func changeKey(c Change) string {
	return c.Object.key() + "|" + c.Path + "|" + string(c.Class) + "|" + deref(c.After)
}

// pinPrevious builds the counterfactual values layer: every key pinned to
// the source chart's default, or null (deleted from the target's defaults)
// when the key is new in the target or absent from the source.
func (u *UnsetValues) pinPrevious(ctx context.Context, keys []string, kind string) ([]byte, string) {
	u.once.Do(func() { u.defaults, u.defaultErr = u.sourceDefaults(ctx) })
	if u.defaultErr != "" && kind != "added" {
		return nil, u.defaultErr
	}
	root := map[string]any{}
	for _, k := range keys {
		segs, err := splitValuesPath(k)
		if err != nil {
			return nil, fmt.Sprintf("key %s: %v", k, err)
		}
		var v any // nil: absent before
		if kind != "added" {
			if raw, ok := u.defaults[k]; ok {
				if err := json.Unmarshal([]byte(raw), &v); err != nil {
					return nil, fmt.Sprintf("previous default of %s: %v", k, err)
				}
			}
		}
		setPath(root, segs, v)
	}
	b, err := yaml.Marshal(root)
	if err != nil {
		return nil, err.Error()
	}
	return b, ""
}

// sourceDefaults reads the source chart's values.yaml, flattened exactly like
// the values diff that produced the change.
func (u *UnsetValues) sourceDefaults(ctx context.Context) (map[string]string, string) {
	if u.Engine == nil || u.Engine.Charts == nil {
		return nil, "no chart resolver configured"
	}
	c, err := u.Engine.Charts.ResolveChart(ctx, u.Product, u.From)
	if err != nil {
		return nil, "source chart: " + err.Error()
	}
	var raw []byte
	switch {
	case len(c.Archive) > 0:
		arc, err := helm.ReadArchive(bytes.NewReader(c.Archive))
		if err != nil {
			return nil, "source chart: " + err.Error()
		}
		raw = arc.Values
	case c.Path != "":
		raw, err = os.ReadFile(filepath.Join(c.Path, "values.yaml"))
		if err != nil && !os.IsNotExist(err) {
			return nil, "source chart values: " + err.Error()
		}
	}
	flat, err := normalize.FlattenValues(raw)
	if err != nil {
		return nil, "source chart values: " + err.Error()
	}
	out := make(map[string]string, len(flat))
	for _, f := range flat {
		out[f.Path] = f.Value
	}
	return out, ""
}

// splitValuesPath splits a flattened values path (dotted keys, ["quoted.key"]
// segments) into its keys.
func splitValuesPath(p string) ([]string, error) {
	var segs []string
	for i := 0; i < len(p); {
		switch {
		case p[i] == '.':
			i++
		case strings.HasPrefix(p[i:], `["`):
			j := strings.Index(p[i:], `"]`)
			if j < 0 {
				return nil, fmt.Errorf("unterminated quoted key")
			}
			k, err := strconv.Unquote(p[i+1 : i+j+1])
			if err != nil {
				return nil, err
			}
			segs = append(segs, k)
			i += j + 2
		default:
			j := i
			for j < len(p) && p[j] != '.' && p[j] != '[' {
				j++
			}
			segs = append(segs, p[i:j])
			i = j
		}
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("empty path")
	}
	return segs, nil
}

func setPath(m map[string]any, segs []string, v any) {
	for _, s := range segs[:len(segs)-1] {
		next, ok := m[s].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[s] = next
		}
		m = next
	}
	m[segs[len(segs)-1]] = v
}

// UnsetKeyEvidence is the release-level attribution of a changed default /
// new key: chart defaults with the key pinned to its previous state vs chart
// defaults. For a customer who leaves the key unset this IS their effective
// change, and it carries no customer values — so these release-scope records
// may be offered to proposal prompts (PO-3 note) and back knowledge.
func UnsetKeyEvidence(ctx context.Context, eng *Engine, rp ReleasePairs, product, from, to string, keys []string, kind string) ([]domain.Evidence, string) {
	p := rp.ReleasePair(ctx, product, from, to)
	if p == nil || p.Scope != domain.RenderRelease {
		return nil, "no release-level render"
	}
	u := &UnsetValues{Engine: eng, Product: product, From: from, To: to, Pairs: []*Pair{p}, Ctx: ctx}
	override, why := u.pinPrevious(ctx, keys, kind)
	if why != "" {
		return nil, why
	}
	a, why := u.Attribute(ctx, p, keys, override)
	if why != "" {
		return nil, why
	}
	out := make([]domain.Evidence, 0, len(a.Changes))
	for _, c := range a.Changes {
		out = append(out, p.Evidence(c, true))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, ""
}
