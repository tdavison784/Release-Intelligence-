package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// renderCmd dispatches `ri render <subcommand>`.
func (c *cli) renderCmd(args []string) error {
	if len(args) == 0 || args[0] != "diff" {
		fmt.Fprintln(c.err, "Usage: ri render diff <product> <from> <to> [flags]")
		return fmt.Errorf("%w: expected `render diff`", app.ErrUsage)
	}
	return c.renderDiff(args[1:])
}

// renderFlags are shared by `ri render diff` and `ri impact --render`.
type renderFlags struct {
	overlays, apiVersions, releaseName, namespace *string
	chartDefaults, showValues                     *bool
}

func addRenderFlags(fs interface {
	String(name, value, usage string) *string
	Bool(name string, value bool, usage string) *bool
}) renderFlags {
	return renderFlags{
		overlays:      fs.String("overlay", "", "comma-separated Kustomize overlay directories to render (default: overlays tied to the environment)"),
		apiVersions:   fs.String("api-versions", "", "comma-separated API versions the cluster serves (helm --api-versions)"),
		releaseName:   fs.String("release-name", "", "Helm release name (default: from the install metadata, else the chart name)"),
		namespace:     fs.String("namespace", "", "Helm release namespace (default: from the install metadata, else the chart name)"),
		chartDefaults: fs.Bool("chart-defaults", false, "also render the chart defaults (release level)"),
		showValues:    fs.Bool("show-values", false, "show before/after values of customer renders (default: paths and summaries only)"),
	}
}

func (c *cli) renderDiff(args []string) error {
	fs := c.flags("render diff", "<product> <from> <to>")
	output := fs.String("o", "text", "output format: text|json")
	values := fs.String("values", "", "comma-separated Helm values files (the customer's configuration)")
	repo := fs.String("repo", "", "customer repository: Argo CD / Flux / Helmfile / values files / Kustomize overlays are detected")
	kube := fs.String("kubernetes", "", "Kubernetes version (helm --kube-version)")
	rf := addRenderFlags(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 3 {
		fs.Usage()
		return fmt.Errorf("%w: expected <product> <from> <to>", app.ErrUsage)
	}
	opts := app.RenderOptions{ValuesFiles: splitList(*values), Repo: *repo, Overlays: splitList(*rf.overlays),
		ReleaseName: *rf.releaseName, Namespace: *rf.namespace, KubeVersion: *kube, APIVersions: splitList(*rf.apiVersions),
		Release: *rf.chartDefaults || (*values == "" && *repo == "" && *rf.overlays == "")}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	res, err := a.RenderDiff(c.ctx, pos[0], pos[1], pos[2], opts)
	if err != nil {
		return err
	}
	if *output == "json" {
		return c.writeJSON(renderJSON(res, *rf.showValues))
	}
	writeRenderText(c.out, res, *rf.showValues)
	return nil
}

// renderReport is the JSON shape of `ri render diff -o json`.
type renderReport struct {
	Product string       `json:"product"`
	From    string       `json:"from"`
	To      string       `json:"to"`
	Release *pairReport  `json:"release,omitempty"`
	Pairs   []pairReport `json:"deployments"`
	Notes   []string     `json:"notes,omitempty"`
}

type pairReport struct {
	*render.Pair
	Changes      []render.Correlated `json:"changes,omitempty"`
	Documented   int                 `json:"documented"`
	Undocumented int                 `json:"undocumented"`
	ValuesHidden bool                `json:"valuesHidden,omitempty"`
}

func renderJSON(res *app.RenderDiffResult, showValues bool) renderReport {
	out := renderReport{Product: string(res.Edge.Product.ID), From: res.Edge.From.String(), To: res.Edge.To.String(), Notes: res.Notes}
	mk := func(p *render.Pair) pairReport {
		pr := pairReport{Pair: p}
		corr := res.Correlations[p.Target.ID]
		pr.Documented, pr.Undocumented = corr.Documented, corr.Undocumented
		hide := p.Scope == domain.RenderEnvironment && !showValues
		for _, cc := range corr.Changes {
			if hide {
				cc.Change.Before, cc.Change.After = nil, nil
				pr.ValuesHidden = true
			}
			pr.Changes = append(pr.Changes, cc)
		}
		// the diff is reported through Changes (correlated, values policy applied)
		cp := *p
		cp.Diff = nil
		if p.Diff != nil && len(p.Diff.Suppressed) > 0 {
			cp.Diff = &render.DiffResult{Suppressed: p.Diff.Suppressed, FromObjects: p.Diff.FromObjects, ToObjects: p.Diff.ToObjects}
		}
		cp.Target.Values = nil // contents are customer data; origins/digests live in the provenance
		pr.Pair = &cp
		return pr
	}
	if res.Release != nil {
		r := mk(res.Release)
		out.Release = &r
	}
	for _, p := range res.Pairs {
		out.Pairs = append(out.Pairs, mk(p))
	}
	return out
}

func writeRenderText(w io.Writer, res *app.RenderDiffResult, showValues bool) {
	fmt.Fprintf(w, "Rendered diff: %s %s → %s\n", res.Edge.Product.ID, res.Edge.From, res.Edge.To)
	if res.Release != nil {
		writePairText(w, res.Release, res.Correlations[res.Release.Target.ID], true)
	}
	for _, p := range res.Pairs {
		writePairText(w, p, res.Correlations[p.Target.ID], showValues)
	}
	for _, n := range res.Notes {
		fmt.Fprintf(w, "\nnote: %s\n", n)
	}
}

func writePairText(w io.Writer, p *render.Pair, corr render.Correlation, showValues bool) {
	title := "Deployment"
	if p.Scope == domain.RenderRelease {
		title = "Release level"
	}
	fmt.Fprintf(w, "\n%s · %s %s\n", title, p.Target.Kind, p.Target.Origin)
	fmt.Fprintf(w, "  why: %s\n", p.Target.Why)
	if p.Target.Kind != render.TargetKustomize {
		assumed := ""
		if p.Target.NamesAssumed {
			assumed = " (assumed: not stated by the configuration)"
		}
		fmt.Fprintf(w, "  release %s, namespace %s%s\n", p.Target.ReleaseName, p.Target.Namespace, assumed)
	}
	for _, r := range []*render.Result{p.FromResult, p.ToResult} {
		if r == nil {
			continue
		}
		pv := r.Provenance
		what := pv.Chart + " " + pv.ChartVersion
		if pv.Tool == render.ToolKustomize {
			what = pv.KustomizeDir
		}
		status := string(r.Status)
		if r.Cached {
			status += ", cached"
		}
		fmt.Fprintf(w, "  render %s: %s %s · artifact %s · values %s · output %s (%s)\n", what, pv.Tool, pv.ToolVersion,
			shortD(pv.ArtifactDigest), shortD(pv.ValuesDigest), shortD(pv.OutputDigest), status)
	}
	if !p.Target.ValuesComplete {
		fmt.Fprintf(w, "  values INCOMPLETE: %s — an absence of change here decides nothing\n", p.Target.IncompleteReason)
	}
	if p.TargetRejects() {
		fmt.Fprintf(w, "  TARGET CHART REJECTS THIS CONFIGURATION (the source rendered it): %s\n", p.Failure.Detail)
		fmt.Fprintf(w, "  → the upgrade with these values fails at render time; review the values before upgrading\n")
		return
	}
	if p.Status != render.PairOK {
		fmt.Fprintf(w, "  UNKNOWN / RENDER %s: %s: %s\n", strings.ToUpper(strings.ReplaceAll(string(p.Status), "-", " ")), p.Failure.Reason, p.Failure.Detail)
		return
	}
	var sup []string
	for k, n := range p.Diff.Suppressed {
		sup = append(sup, fmt.Sprintf("%s %d", k, n))
	}
	sort.Strings(sup)
	fmt.Fprintf(w, "  %d rendered changes (%d objects → %d): %d documented, %d not mentioned by any changelog entry", len(p.Diff.Changes),
		p.Diff.FromObjects, p.Diff.ToObjects, corr.Documented, corr.Undocumented)
	if len(sup) > 0 {
		fmt.Fprintf(w, "; noise suppressed: %s", strings.Join(sup, ", "))
	}
	fmt.Fprintln(w)
	for _, cc := range corr.Changes {
		mark := "?"
		if len(cc.Links) > 0 {
			mark = "✓"
		}
		fmt.Fprintf(w, "    %s %s\n", mark, cc.Change.Summary(showValues))
		for _, l := range cc.Links {
			t := strings.Join(strings.Fields(l.Title), " ")
			if r := []rune(t); len(r) > 90 {
				t = string(r[:90]) + "…"
			}
			fmt.Fprintf(w, "        ↳ %s %s (%s)\n", l.ChangeID, t, l.Rule)
		}
	}
	if corr.Undocumented > 0 {
		fmt.Fprintf(w, "  ? = rendered change no changelog entry mentions (undocumented: review)\n")
	}
}

func shortD(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if d == "" {
		return "–"
	}
	if len(d) > 12 {
		return d[:12]
	}
	return d
}
