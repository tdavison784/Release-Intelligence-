package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	rtval "github.com/tdavison784/release-intelligence/internal/investigate/runtime"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/investigate/verify"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/render"
)

const verifyUsage = `ri verify hypotheses -knowledge DIR [-runtime] [-store DIR] [-again] [-json]

  Independently verify every open agent-proposed rule (hypothesis-sourced
  fact-review item, exported by ` + "`ri investigate export-hypotheses`" + `, or a
  human-authored rule taken in by ` + "`ri knowledge intake-rule`" + `) in a
  knowledge directory and store a proof record beside it (PROTOCOL §7–§8):

    citation check   (deterministic) each cited upstream text resolves in the
                     evidence cache (the investigation store, the fetch cache),
                     contains the cited passage and names only versions inside
                     the rule's release bounds
    rule exercise    (observed, -runtime) a synthetic fixture built from the rule
                     (products/exercises/fixtures/<product>.yaml), the condition
                     present and absent, upgraded FROM → TO in a throwaway kind
                     cluster ri-verify-*, the declared exercise run on both arms;
                     for a rule naming a fixture's consumer role field, the
                     consumer observation instead (x509 verification of a
                     certificate issued after re-issuance against the trust
                     captured before it)

  confirmed only when both pass; any failure refutes; otherwise inconclusive.
  A human accept/correct of the item then needs a confirmed proof of exactly
  that rule (` + "`ri review serve`" + ` shows the proof and the gate). Never runs
  against an evaluation environment.
`

func (c *cli) verifyCmd(args []string) error {
	if len(args) == 0 || args[0] != "hypotheses" {
		fmt.Fprint(c.err, verifyUsage)
		if len(args) > 0 && (args[0] == "help" || args[0] == "-h") {
			return nil
		}
		return fmt.Errorf("%w: ri verify hypotheses", app.ErrUsage)
	}
	fs := flag.NewFlagSet("verify hypotheses", flag.ContinueOnError)
	fs.SetOutput(c.err)
	kdir := fs.String("knowledge", "", "knowledge directory holding the exported hypothesis items (required; never the live tree in an experiment)")
	runtimeOn := fs.Bool("runtime", false, "run the two-arm rule exercise in a throwaway kind cluster (opt-in)")
	storeDir := fs.String("store", "", "state directory of the investigation store with the hypotheses' runs (default: -state)")
	again := fs.Bool("again", false, "re-verify items that already have a confirmed or refuted proof")
	kube := fs.String("kubernetes", "", "Kubernetes version of the fixture cluster (pinned kind node image; default: kind's)")
	timeout := fs.Duration("timeout", 15*time.Minute, "wall-clock budget of one fixture run")
	asJSON := fs.Bool("json", false, "print the results as JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *kdir == "" {
		return fmt.Errorf("%w: -knowledge is required", app.ErrUsage)
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	root := *storeDir
	if root == "" {
		root = c.g.state
	}
	st, err := store.New(root)
	if err != nil {
		return err
	}
	logf := func(string, ...any) {}
	if c.g.verbose {
		logf = func(f string, a ...any) { fmt.Fprintf(c.err, "  · "+f+"\n", a...) }
	}
	o := verify.Options{
		Knowledge: knowledge.NewFileStore(*kdir),
		Resolver: verify.Resolvers{verify.StoreResolver{Store: st},
			&verify.HTTPCacheResolver{Dir: filepath.Join(c.g.state, "cache")}},
		Runs:              verify.StateRuns{Dir: filepath.Join(st.Dir(), "runs")},
		Runtime:           *runtimeOn,
		KubernetesVersion: *kube,
		Again:             *again,
		Logf:              logf,
		Relationships: func(product string) []catalog.Relationship {
			if def, ok := a.Catalog.Get(product); ok {
				return def.Relationships
			}
			return nil
		},
	}
	if *runtimeOn {
		if o.Fixtures, err = verify.LoadFixtures(filepath.Join(c.g.products, "exercises", "fixtures")); err != nil {
			return fmt.Errorf("-runtime: %w", err)
		}
		sets, err := catalog.LoadExercises(filepath.Join(c.g.products, "exercises"))
		if err != nil {
			return fmt.Errorf("-runtime: %w", err)
		}
		o.Exercises = rtval.FromSets(sets...)
		o.Renderer = controllerRenderer{e: a.RenderEngine()}
		o.Runner = rtval.New(rtval.WithStore(st), rtval.WithExercises(o.Exercises...), rtval.WithTimeout(*timeout), rtval.WithLogf(logf))
	}
	rs, err := verify.Hypotheses(c.ctx, o)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(c.out)
		enc.SetIndent("", "  ")
		return enc.Encode(rs)
	}
	if len(rs) == 0 {
		fmt.Fprintln(c.out, "no open hypothesis-sourced review items")
		return nil
	}
	for _, r := range rs {
		if r.Proof == nil {
			fmt.Fprintf(c.out, "%s  %s  skipped: %s\n", r.Item, r.Hypothesis, r.Skipped)
			continue
		}
		p := r.Proof
		fmt.Fprintf(c.out, "%s  %s  %s (%s)  proof %s\n", r.Item, r.Hypothesis, p.Outcome, p.Level, p.ID)
		for _, m := range p.Methods {
			fmt.Fprintf(c.out, "    %-15s %-12s %s\n", m.Method, m.Outcome, m.Detail)
		}
	}
	return nil
}

// controllerRenderer renders a product's chart at one version with the
// fixture's values through the render engine (chart resolution and cache as
// every other render).
type controllerRenderer struct{ e *render.Engine }

func (r controllerRenderer) RenderController(ctx context.Context, product, version string, cf verify.ControllerFixture) (rtval.RawRender, error) {
	ns := cf.Namespace
	if ns == "" {
		ns = product
	}
	t := render.Target{ID: "verify-fixture", Kind: render.TargetDefaults, Origin: "products/exercises/fixtures/" + product + ".yaml",
		Why: "controller of a synthetic verification fixture", ReleaseName: product, Namespace: ns, ValuesComplete: true}
	if len(cf.Values) > 0 {
		b, err := yaml.Marshal(cf.Values)
		if err != nil {
			return rtval.RawRender{}, err
		}
		t.Values = []render.ValuesLayer{{Origin: t.Origin + "#controller.values", Content: b}}
	}
	p := r.e.RenderPair(ctx, render.PairRequest{Product: product, From: version, To: version, Target: t, Scope: domain.RenderEnvironment})
	if !p.FromResult.OK() {
		why := "no result"
		if p.FromResult != nil && p.FromResult.Failure != nil {
			why = p.FromResult.Failure.Error()
		}
		return rtval.RawRender{}, fmt.Errorf("render %s %s: %s", product, version, why)
	}
	return rtval.RawRender{Manifests: p.FromResult.Output, Namespace: ns, Nondeterministic: p.FromResult.Nondeterministic}, nil
}
