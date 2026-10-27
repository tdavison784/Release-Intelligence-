package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	rtval "github.com/tdavison784/release-intelligence/internal/investigate/runtime"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/investigate/verify"
	"github.com/tdavison784/release-intelligence/internal/investigate/verify/semantic"
)

func init() {
	investigateSubcommands["semcheck"] = (*cli).investigateSemcheck
	investigateSubcommands["admission"] = (*cli).investigateAdmission
}

// semcheckResult is the report-only semantic check of one stored run.
type semcheckResult struct {
	Key           string                    `json:"key"`
	Investigation string                    `json:"investigation"`
	Checks        []semantic.BehaviourCheck `json:"checks"`
}

// investigateSemcheck is `ri investigate semcheck <key>...`: the report-only
// semantic check (Milestone 4, lane semverify) of every agent behaviour entry
// sourced upstream in stored runs: supported, unsupported or undetermined
// against the passages its citations quote. It reads only; it never changes
// a classification or a record.
func (c *cli) investigateSemcheck(args []string) error {
	fs := c.flags("investigate semcheck", "<key>...")
	dir := fs.String("store", "", "where the runs are: a state directory (default: the global -state), or stored evaluation runs (eval/investigations/results, or one run directory; keys <run-id>/<case>)")
	asJSON := fs.Bool("json", false, "print the checks as JSON")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		fs.Usage()
		return fmt.Errorf("%w: name at least one investigation key", app.ErrUsage)
	}
	src, err := c.reportSource(*dir)
	if err != nil {
		return err
	}
	var out []semcheckResult
	for _, key := range pos {
		rec, err := src.Load(c.ctx, key)
		if err != nil {
			return err
		}
		pool := map[domain.EvidenceID]domain.Evidence{}
		if rec.Bundle != nil {
			for _, e := range rec.Bundle.Evidence {
				pool[e.ID] = e
			}
		}
		for _, e := range rec.Investigation.Evidence {
			pool[e.ID] = e
		}
		out = append(out, semcheckResult{Key: key, Investigation: rec.Investigation.ID,
			Checks: semantic.CheckBehaviours(rec.Investigation.AgentAssessments, pool)})
	}
	if *asJSON {
		enc := json.NewEncoder(c.out)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	for _, r := range out {
		fmt.Fprintf(c.out, "%s (%s): %d upstream behaviour entries (report-only; never a classification input)\n", r.Key, r.Investigation, len(r.Checks))
		for _, bc := range r.Checks {
			fmt.Fprintf(c.out, "  %-12s %s  [%s]\n", bc.Verdict, bc.Behaviour, bc.ChangeItem)
			fmt.Fprintf(c.out, "               %s\n", bc.Detail)
			ids := make([]string, len(bc.Citations))
			for i, id := range bc.Citations {
				ids[i] = string(id)
			}
			fmt.Fprintf(c.out, "               cites %s", strings.Join(ids, ", "))
			if len(bc.Unresolved) > 0 {
				fmt.Fprintf(c.out, " (%d not in the run's evidence)", len(bc.Unresolved))
			}
			fmt.Fprintln(c.out)
		}
	}
	return nil
}

// investigateAdmission is `ri investigate admission`: the admission dry run
// (runtime.admission-dry-run@v1, Milestone 4 E5). The product's controller is
// rendered at the target version (chart defaults, the product's verification
// fixture's controller values, or -values), installed in a throwaway kind
// cluster ri-admission-*, and one environment object is submitted as a
// server-side dry run against its validating webhook. Opt-in by being a
// separate command; the cluster is deleted afterwards.
func (c *cli) investigateAdmission(args []string) error {
	fs := c.flags("investigate admission", "")
	product := fs.String("product", "", "product id (required)")
	version := fs.String("version", "", "target version whose webhook decides (required)")
	objectFile := fs.String("object", "", "YAML file with exactly one environment object of the exercise's target kind (required)")
	setupFile := fs.String("setup", "", "YAML file of objects applied for real first (what the object refers to); optional")
	valuesFile := fs.String("values", "", "Helm values for the controller (default: the product's verification fixture controller values, else chart defaults)")
	namespace := fs.String("namespace", "", "namespace of a namespaced object that names none (default: default)")
	exercise := fs.String("exercise", "", "admission-dry-run exercise id (default: the product's one targeting the object's kind)")
	kube := fs.String("kubernetes", "", "Kubernetes version of the cluster (pinned kind node image; default: kind's)")
	timeout := fs.Duration("timeout", 15*time.Minute, "wall-clock budget of the run")
	storeDir := fs.String("store", "", "state directory storing the admission record (default: -state)")
	asJSON := fs.Bool("json", false, "print the record as JSON")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 || *product == "" || *version == "" || *objectFile == "" {
		fs.Usage()
		return fmt.Errorf("%w: -product, -version and -object are required", app.ErrUsage)
	}
	object, err := os.ReadFile(*objectFile)
	if err != nil {
		return err
	}
	var setup []byte
	if *setupFile != "" {
		if setup, err = os.ReadFile(*setupFile); err != nil {
			return err
		}
	}
	sets, err := catalog.LoadExercises(filepath.Join(c.g.products, "exercises"))
	if err != nil {
		return err
	}
	exercises := rtval.FromSets(sets...)
	key, err := admissionExercise(exercises, *product, *exercise, object)
	if err != nil {
		return err
	}
	cf := verify.ControllerFixture{}
	if *valuesFile != "" {
		raw, err := os.ReadFile(*valuesFile)
		if err != nil {
			return err
		}
		if err := yaml.Unmarshal(raw, &cf.Values); err != nil {
			return fmt.Errorf("-values: %w", err)
		}
	} else if fx, err := verify.LoadFixtures(filepath.Join(c.g.products, "exercises", "fixtures")); err == nil && fx[*product] != nil {
		cf = fx[*product].Controller
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	ctrl, err := controllerRenderer{e: a.RenderEngine()}.RenderController(c.ctx, *product, *version, cf)
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
	v := rtval.New(rtval.WithStore(st), rtval.WithExercises(exercises...), rtval.WithTimeout(*timeout), rtval.WithLogf(logf))
	ar, err := v.DryRunAdmission(c.ctx, rtval.AdmissionSpec{
		ID:      "adm-" + domain.ShortHash(*product, *version, string(ctrl.Manifests), string(setup), string(object), *namespace, *kube),
		Product: *product, Version: *version, KubernetesVersion: *kube, Controller: ctrl, Setup: setup, Object: object,
		Namespace: *namespace, Exercise: key})
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(c.out)
		enc.SetIndent("", "  ")
		return enc.Encode(ar)
	}
	fmt.Fprintf(c.out, "%s against %s %s: %s (observed)\n  %s\n", ar.Object, ar.Product, ar.Version, strings.ToUpper(string(ar.Outcome)), ar.Detail)
	if ar.Message != "" {
		fmt.Fprintf(c.out, "  message (%s): %s\n", orDash(ar.RejectedBy), ar.Message)
	}
	for _, h := range ar.Webhooks {
		fmt.Fprintf(c.out, "  webhook %s/%s covers=%v ready=%v failurePolicy=%s sideEffects=%s %s\n", h.Configuration, h.Name, h.Covers, h.Ready, h.FailurePolicy, h.SideEffects, h.Detail)
	}
	r := ar.Run
	fmt.Fprintf(c.out, "  run %s (%s) cluster %s, %d attempt(s), %.0fs", r.Status, r.Reason, r.Cluster, ar.Attempts, r.DurationSeconds)
	if r.Cleanup != nil {
		fmt.Fprintf(c.out, ", deleted=%v", r.Cleanup.Deleted && r.Cleanup.Verified)
	}
	if ar.Artifact != nil {
		fmt.Fprintf(c.out, "\n  record %s (%s)", ar.Artifact.Path, ar.Artifact.Digest)
	}
	fmt.Fprintln(c.out)
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// admissionExercise picks the product's admission-dry-run exercise: the one
// named, else the only one targeting the object's kind.
func admissionExercise(xs []rtval.DeclaredExercise, product, id string, object []byte) (string, error) {
	var doc struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(object, &doc); err != nil {
		return "", fmt.Errorf("-object: %w", err)
	}
	group := ""
	if g, _, ok := strings.Cut(doc.APIVersion, "/"); ok {
		group = g
	}
	var keys []string
	for _, x := range xs {
		e := x.Exercise
		if x.Product != product || e.Action != catalog.ActionAdmissionDryRun {
			continue
		}
		if id != "" {
			if e.ID == id {
				return x.Key(), nil
			}
			continue
		}
		if e.Target.Group == group && e.Target.Kind == doc.Kind {
			keys = append(keys, x.Key())
		}
	}
	switch {
	case id != "":
		return "", fmt.Errorf("%s declares no admission-dry-run exercise %q (products/exercises/%s.yaml)", product, id, product)
	case len(keys) != 1:
		return "", fmt.Errorf("%s declares %d admission-dry-run exercises for %s/%s; name one with -exercise", product, len(keys), group, doc.Kind)
	}
	return keys[0], nil
}
