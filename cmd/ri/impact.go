package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/impact"
)

// impact runs `ri impact <product> <from> <to>`: the upgrade edge joined with
// the caller's environment. Every environment input is optional, but at least
// one must be given — the whole point of the command is the join.
func (c *cli) impact(args []string) error {
	fs := c.flags("impact", "<product> <from> <to>")
	output := fs.String("o", "text", "output format: text|json")
	kubernetes := fs.String("kubernetes", "", "Kubernetes version of the cluster (e.g. 1.31 or 1.31.5)")
	values := fs.String("values", "", "comma-separated Helm values files of the environment")
	manifests := fs.String("manifests", "", "comma-separated manifest files or directories (multi-document YAML)")
	crds := fs.String("crds", "", "comma-separated CustomResourceDefinition files or directories")
	images := fs.String("images", "", "comma-separated image references, or one file listing them (one per line)")
	policy := fs.String("policy", "", "path policy override: minor-lineage|all")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 3 {
		fs.Usage()
		return fmt.Errorf("%w: expected <product> <from> <to>", app.ErrUsage)
	}
	in := app.ImpactOptions{Policy: *policy}
	in.Environment.KubernetesVersion = *kubernetes
	in.Environment.ValuesFiles = splitList(*values)
	in.Environment.Manifests = splitList(*manifests)
	in.Environment.CRDs = splitList(*crds)
	in.Environment.Images, err = imageList(*images)
	if err != nil {
		return err
	}
	a, err := c.newApp()
	if err != nil {
		return err
	}
	rep, err := a.Impact(c.ctx, pos[0], pos[1], pos[2], in)
	if err != nil {
		return err
	}
	if *output == "json" {
		return c.writeJSON(rep)
	}
	return impact.RenderText(c.out, rep, impact.RenderOptions{Color: isTerminal(c.out)})
}

// imageList reads the --images flag: a comma-separated list, or the name of
// an existing file holding one reference per line (# comments allowed).
func imageList(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	if strings.Contains(s, ",") {
		return splitList(s), nil
	}
	if fi, err := os.Stat(s); err == nil && !fi.IsDir() {
		b, err := os.ReadFile(s)
		if err != nil {
			return nil, fmt.Errorf("read image list: %w", err)
		}
		var out []string
		for _, ln := range strings.Split(string(b), "\n") {
			if i := strings.IndexByte(ln, '#'); i >= 0 {
				ln = ln[:i]
			}
			if ln = strings.TrimSpace(ln); ln != "" {
				out = append(out, ln)
			}
		}
		return out, nil
	}
	return []string{s}, nil
}
