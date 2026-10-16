package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// The env loader inventories a kustomization's resources as plain manifests
// and warns that it was not built. That warning is about the loader, not
// about the environment: the renders build the kustomizations. The snapshot
// therefore builds each such kustomization itself, and keeps the repo input
// `partial` only for one that genuinely cannot be built (with the reason).
var notBuiltRE = regexp.MustCompile(`^kustomization (.+) is not built with kustomize:`)

// kustomizeBuilder builds one kustomization directory under root; nil error
// means it built completely.
type kustomizeBuilder func(ctx context.Context, root, dir string) error

// realKustomizeBuilder runs `kustomize build` / `kubectl kustomize` through
// internal/render. A missing binary is a failure: nothing was built.
func realKustomizeBuilder(ctx context.Context, root, dir string) error {
	res := render.NewKustomize(nil).Render(ctx, render.Request{
		Tool: render.ToolKustomize, Scope: domain.RenderEnvironment, ValuesComplete: true,
		Kustomize: &render.KustomizeInput{Dir: dir, Root: root},
	})
	if res.Status == render.StatusSucceeded {
		return nil
	}
	if res.Failure != nil {
		return fmt.Errorf("%s: %s", res.Failure.Reason, res.Failure.Detail)
	}
	return fmt.Errorf("render status %s", res.Status)
}

// resolveBuilt drops the "not built" loader warning of every kustomization
// that builds, and rewrites the warning of one that does not into a
// statement of why. It returns the resulting warnings.
func resolveBuilt(ctx context.Context, repo string, warnings []string, build kustomizeBuilder) []string {
	if repo == "" || build == nil {
		return warnings
	}
	root, _ := filepath.Abs(repo)
	var out []string
	for _, w := range warnings {
		m := notBuiltRE.FindStringSubmatch(w)
		if m == nil {
			out = append(out, w)
			continue
		}
		dir := filepath.Join(root, filepath.FromSlash(m[1]))
		if fi, err := os.Stat(dir); err != nil {
			dir = filepath.Dir(dir)
		} else if !fi.IsDir() {
			dir = filepath.Dir(dir)
		}
		if err := build(ctx, root, dir); err != nil {
			out = append(out, fmt.Sprintf("kustomization %s cannot be built: %s", m[1], strings.TrimSpace(err.Error())))
		}
	}
	return out
}
