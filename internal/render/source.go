package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/env"
)

// TargetKind names where a render target's configuration came from.
type TargetKind string

const (
	TargetValuesFiles TargetKind = "values-files" // --values, or a repo-local values file
	TargetArgoCD      TargetKind = "argocd"       // an Argo CD Application (helm values/valuesObject/parameters)
	TargetFlux        TargetKind = "flux"         // a Flux HelmRelease (spec.values)
	TargetHelmfile    TargetKind = "helmfile"     // a Helmfile release (values files / inline / set)
	TargetKustomize   TargetKind = "kustomize"    // a Kustomize overlay
	TargetDefaults    TargetKind = "chart-defaults"
)

// Target is one deployment of the product found in the customer's
// configuration: what to render, with which values, under which release
// name and namespace — and why it was chosen.
type Target struct {
	ID          string     `json:"id"`
	Kind        TargetKind `json:"kind"`
	Origin      string     `json:"origin"` // file (and document) that declares it
	Why         string     `json:"why"`
	ReleaseName string     `json:"releaseName,omitempty"`
	Namespace   string     `json:"namespace,omitempty"`
	// NamesAssumed: release name/namespace were not stated and default to
	// the chart name (recorded; resource names may differ from the cluster).
	NamesAssumed     bool            `json:"namesAssumed,omitempty"`
	Values           []ValuesLayer   `json:"values,omitempty"`
	Set              []SetValue      `json:"set,omitempty"`
	ValuesComplete   bool            `json:"valuesComplete"`
	IncompleteReason string          `json:"incompleteReason,omitempty"`
	Kustomize        *KustomizeInput `json:"kustomize,omitempty"`
}

// TargetOptions selects the customer configuration for a product.
type TargetOptions struct {
	// Product is the catalog id; Charts are the chart names the product
	// publishes (a Helm install matches when its chart is one of them).
	Product string
	Charts  []string
	// ValuesFiles are explicit --values files (one target, in order).
	ValuesFiles []string
	// ReleaseName/Namespace override the install metadata.
	ReleaseName, Namespace string
	// Repo is the repository root; Files are the repo files to inspect,
	// by role (from env.Environment.Discovered, or a walk).
	Repo  string
	Files RepoFiles
	// Overlays are explicit kustomize overlay directories.
	Overlays []string
}

// RepoFiles are repository files by discovery role (paths as walked).
type RepoFiles struct {
	ArgoCD, Flux, Helmfile, Values, Kustomizations []string
}

// matchesChart reports whether a chart reference names one of the product's charts.
func (o TargetOptions) matchesChart(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	base := ref
	if i := strings.LastIndexAny(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	for _, c := range append([]string{o.Product}, o.Charts...) {
		if c != "" && (base == c || ref == c) {
			return true
		}
	}
	return false
}

// mentions reports whether text names the product or one of its charts.
func (o TargetOptions) mentions(text string) bool {
	for _, c := range append([]string{o.Product}, o.Charts...) {
		if c != "" && strings.Contains(text, c) {
			return true
		}
	}
	return false
}

// DetectTargets finds the deployments of the product in the customer's
// configuration (RENDER-MISSION R7): explicit values files; Argo CD
// Applications, Flux HelmReleases and Helmfile releases whose chart is the
// product's; repo-local values files no Helmfile release references and
// whose name names the product; and Kustomize overlays tied to the
// environment (Argo CD source paths, explicit overlays, else top-level
// kustomizations that reference the product). Every choice records why;
// notes explain what was skipped.
func DetectTargets(o TargetOptions) (targets []Target, notes []string) {
	chart := o.Product
	if len(o.Charts) > 0 {
		chart = o.Charts[0]
	}
	names := func(t *Target) {
		if o.ReleaseName != "" {
			t.ReleaseName = o.ReleaseName
		}
		if o.Namespace != "" {
			t.Namespace = o.Namespace
		}
		if t.ReleaseName == "" || t.Namespace == "" {
			t.NamesAssumed = true
			if t.ReleaseName == "" {
				t.ReleaseName = chart
			}
			if t.Namespace == "" {
				t.Namespace = chart
			}
		}
	}
	if len(o.ValuesFiles) > 0 {
		t := Target{ID: "values:" + strings.Join(o.ValuesFiles, ","), Kind: TargetValuesFiles, Origin: strings.Join(o.ValuesFiles, ","),
			Why: "values files given with --values", ValuesComplete: true}
		for _, f := range o.ValuesFiles {
			b, err := os.ReadFile(f)
			if err != nil {
				t.ValuesComplete, t.IncompleteReason = false, fmt.Sprintf("values file %s unreadable: %v", f, err)
				continue
			}
			t.Values = append(t.Values, ValuesLayer{Origin: f, Content: b})
		}
		names(&t)
		targets = append(targets, t)
	}
	referenced := map[string]bool{}
	for _, f := range o.Files.ArgoCD {
		ts, n := argoTargets(o, f)
		targets, notes = append(targets, ts...), append(notes, n...)
	}
	for _, f := range o.Files.Flux {
		ts, n := fluxTargets(o, f)
		targets, notes = append(targets, ts...), append(notes, n...)
	}
	for _, f := range o.Files.Helmfile {
		ts, refs, n := helmfileTargets(o, f)
		targets, notes = append(targets, ts...), append(notes, n...)
		for _, r := range refs {
			referenced[filepath.Clean(r)] = true
		}
	}
	if len(o.ValuesFiles) == 0 {
		for _, f := range o.Files.Values {
			switch {
			case referenced[filepath.Clean(f)]:
				continue // rendered as part of the Helmfile release
			case !o.mentions(filepath.Base(f)) && !o.mentions(filepath.Base(filepath.Dir(f))):
				notes = append(notes, fmt.Sprintf("values file %s: skipped (its name does not name %s; pass it with --values to render it)", f, o.Product))
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				notes = append(notes, fmt.Sprintf("values file %s: unreadable: %v", f, err))
				continue
			}
			t := Target{ID: "values:" + f, Kind: TargetValuesFiles, Origin: f, ValuesComplete: true,
				Why:    fmt.Sprintf("repo-local values file naming %s, not referenced by a Helmfile release", o.Product),
				Values: []ValuesLayer{{Origin: f, Content: b}}}
			names(&t)
			targets = append(targets, t)
		}
	}
	for i := range targets {
		if targets[i].Kind != TargetKustomize && targets[i].Kind != TargetValuesFiles {
			names(&targets[i])
		}
	}
	// Kustomize overlays
	overlays := map[string]string{}
	for _, d := range o.Overlays {
		overlays[filepath.Clean(d)] = "overlay given with --overlay"
	}
	for _, t := range targets {
		if t.Kind == TargetKustomize && t.Kustomize != nil {
			overlays[filepath.Clean(t.Kustomize.Dir)] = t.Why
		}
	}
	// drop the placeholder kustomize targets argoTargets produced; rebuilt below
	kept := targets[:0]
	for _, t := range targets {
		if t.Kind != TargetKustomize {
			kept = append(kept, t)
		}
	}
	targets = kept
	if len(overlays) == 0 && len(o.Files.Kustomizations) > 0 {
		for d, why := range topLevelOverlays(o) {
			overlays[d] = why
		}
	}
	dirs := make([]string, 0, len(overlays))
	for d := range overlays {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		root := o.Repo
		if root == "" {
			root = d
		}
		targets = append(targets, Target{ID: "kustomize:" + d, Kind: TargetKustomize, Origin: d, Why: overlays[d], ValuesComplete: true,
			Kustomize: &KustomizeInput{Dir: d, Root: root, Why: overlays[d]}})
	}
	sort.SliceStable(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })
	return targets, notes
}

// topLevelOverlays picks kustomizations that no other kustomization
// references (the overlays, not the bases) and whose inputs name the product.
func topLevelOverlays(o TargetOptions) map[string]string {
	dirs := map[string]bool{}
	for _, f := range o.Files.Kustomizations {
		d, _ := filepath.Abs(filepath.Dir(f))
		dirs[d] = true
	}
	referencedDirs := map[string]bool{}
	for d := range dirs {
		root := o.Repo
		if root == "" {
			root = d
		}
		files, _, _, err := KustomizeInputs(root, d)
		if err != nil {
			continue
		}
		absRoot, _ := filepath.Abs(root)
		for _, f := range files {
			fd := filepath.Clean(filepath.Join(absRoot, filepath.Dir(f)))
			if fd != d {
				referencedDirs[fd] = true
			}
		}
	}
	out := map[string]string{}
	for d := range dirs {
		if referencedDirs[d] {
			continue
		}
		root := o.Repo
		if root == "" {
			root = d
		}
		files, _, _, err := KustomizeInputs(root, d)
		if err != nil {
			out[d] = "top-level kustomization (its inputs could not be resolved: the render will say why)"
			continue
		}
		for _, f := range files {
			absRoot, _ := filepath.Abs(root)
			b, err := os.ReadFile(filepath.Join(absRoot, f))
			if err == nil && o.mentions(string(b)) {
				out[d] = fmt.Sprintf("top-level kustomization (no other kustomization references it) whose inputs reference %s (%s)", o.Product, f)
				break
			}
		}
	}
	return out
}

// yamlDocs decodes every document of a file.
func yamlDocs(path string) ([]map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	var out []map[string]any
	for {
		var m map[string]any
		err := dec.Decode(&m)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return out, err
		}
		if m != nil {
			out = append(out, m)
		}
	}
	return out, nil
}

func get(m map[string]any, path ...string) any {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[p]
	}
	return cur
}

func str(m map[string]any, path ...string) string {
	s, _ := get(m, path...).(string)
	return s
}

func yamlBytes(v any) []byte {
	b, _ := yaml.Marshal(v)
	return b
}

func argoTargets(o TargetOptions, file string) (out []Target, notes []string) {
	docs, err := yamlDocs(file)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: %v", file, err)}
	}
	for di, d := range docs {
		if str(d, "kind") != "Application" {
			continue
		}
		app := str(d, "metadata", "name")
		var srcs []map[string]any
		var locs []string
		if s, ok := get(d, "spec", "source").(map[string]any); ok {
			srcs, locs = append(srcs, s), append(locs, "spec.source")
		}
		if l, ok := get(d, "spec", "sources").([]any); ok {
			for i, s := range l {
				if sm, ok := s.(map[string]any); ok {
					srcs, locs = append(srcs, sm), append(locs, fmt.Sprintf("spec.sources[%d]", i))
				}
			}
		}
		ns := str(d, "spec", "destination", "namespace")
		for i, s := range srcs {
			origin := fmt.Sprintf("%s#%d/$.%s", file, di, locs[i])
			if p := str(s, "path"); p != "" && o.Repo != "" {
				dir := filepath.Join(o.Repo, p)
				if KustomizationFile(dir) != "" {
					out = append(out, Target{Kind: TargetKustomize, Kustomize: &KustomizeInput{Dir: dir, Root: o.Repo},
						Why: fmt.Sprintf("Argo CD Application %s deploys this overlay (%s.path: %s)", app, locs[i], p)})
				}
				continue
			}
			if !o.matchesChart(str(s, "chart")) {
				continue
			}
			t := Target{ID: "argocd:" + origin, Kind: TargetArgoCD, Origin: origin, Namespace: ns, ValuesComplete: true,
				Why: fmt.Sprintf("Argo CD Application %s installs chart %s", app, str(s, "chart"))}
			t.ReleaseName = str(s, "helm", "releaseName")
			if t.ReleaseName == "" {
				t.ReleaseName = app
			}
			helm, _ := get(s, "helm").(map[string]any)
			if vf, ok := helm["valueFiles"].([]any); ok && len(vf) > 0 {
				for _, f := range vf {
					fs, _ := f.(string)
					if strings.HasPrefix(fs, "$") && o.Repo != "" {
						if i := strings.IndexByte(fs, '/'); i >= 0 {
							p := filepath.Join(o.Repo, fs[i+1:])
							if b, err := os.ReadFile(p); err == nil {
								t.Values = append(t.Values, ValuesLayer{Origin: p, Content: b})
								continue
							}
						}
					}
					t.ValuesComplete = false
					t.IncompleteReason = fmt.Sprintf("helm.valueFiles %q is not readable from this repository", fs)
				}
			}
			if v, ok := helm["values"].(string); ok && strings.TrimSpace(v) != "" {
				t.Values = append(t.Values, ValuesLayer{Origin: origin + ".helm.values", Content: []byte(v)})
			} else if vm, ok := helm["values"].(map[string]any); ok {
				t.Values = append(t.Values, ValuesLayer{Origin: origin + ".helm.values", Content: yamlBytes(vm)})
			}
			if vo, ok := helm["valuesObject"].(map[string]any); ok {
				t.Values = append(t.Values, ValuesLayer{Origin: origin + ".helm.valuesObject", Content: yamlBytes(vo)})
			}
			if ps, ok := helm["parameters"].([]any); ok {
				for _, p := range ps {
					pm, _ := p.(map[string]any)
					name, _ := pm["name"].(string)
					if name == "" {
						continue
					}
					val := fmt.Sprint(pm["value"])
					var enc string
					if force, _ := pm["forceString"].(bool); force {
						enc = encode(val)
					} else {
						enc = encodeSetValue(val)
					}
					t.Set = append(t.Set, SetValue{Path: name, Value: enc, Origin: origin + ".helm.parameters"})
				}
			}
			out = append(out, t)
		}
	}
	return out, notes
}

// encodeSetValue encodes a --set style value the way helm types it: bools,
// numbers and null as such, everything else as a string.
func encodeSetValue(v string) string {
	switch v {
	case "true", "false", "null":
		return v
	}
	var n json.Number
	if err := json.Unmarshal([]byte(v), &n); err == nil {
		return v
	}
	return encode(v)
}

func fluxTargets(o TargetOptions, file string) (out []Target, notes []string) {
	docs, err := yamlDocs(file)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: %v", file, err)}
	}
	for di, d := range docs {
		if str(d, "kind") != "HelmRelease" || !o.matchesChart(str(d, "spec", "chart", "spec", "chart")) {
			continue
		}
		name := str(d, "metadata", "name")
		origin := fmt.Sprintf("%s#%d", file, di)
		t := Target{ID: "flux:" + origin, Kind: TargetFlux, Origin: origin, ValuesComplete: true,
			Why: fmt.Sprintf("Flux HelmRelease %s installs chart %s", name, str(d, "spec", "chart", "spec", "chart"))}
		target := str(d, "spec", "targetNamespace")
		t.Namespace = target
		if t.Namespace == "" {
			t.Namespace = str(d, "metadata", "namespace")
		}
		t.ReleaseName = str(d, "spec", "releaseName")
		if t.ReleaseName == "" {
			// Flux's default release name: [<targetNamespace>-]<name>
			t.ReleaseName = name
			if target != "" {
				t.ReleaseName = target + "-" + name
			}
		}
		if vf, ok := get(d, "spec", "valuesFrom").([]any); ok && len(vf) > 0 {
			t.ValuesComplete = false
			t.IncompleteReason = "spec.valuesFrom references cluster objects (ConfigMap/Secret) no file can answer for"
		}
		if v, ok := get(d, "spec", "values").(map[string]any); ok {
			t.Values = append(t.Values, ValuesLayer{Origin: origin + "/$.spec.values", Content: yamlBytes(v)})
		}
		out = append(out, t)
	}
	return out, notes
}

func helmfileTargets(o TargetOptions, file string) (out []Target, referenced []string, notes []string) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, nil, []string{fmt.Sprintf("%s: %v", file, err)}
	}
	templated := bytes.Contains(raw, []byte("{{"))
	docs, err := yamlDocs(file)
	if err != nil {
		return nil, nil, []string{fmt.Sprintf("%s: %v (templated helmfiles are not rendered)", file, err)}
	}
	dir := filepath.Dir(file)
	for _, d := range docs {
		rels, _ := d["releases"].([]any)
		for ri, r := range rels {
			rm, _ := r.(map[string]any)
			if rm == nil || !o.matchesChart(str(rm, "chart")) {
				continue
			}
			origin := fmt.Sprintf("%s/$.releases[%d]", file, ri)
			t := Target{ID: "helmfile:" + origin, Kind: TargetHelmfile, Origin: origin, ValuesComplete: true,
				ReleaseName: str(rm, "name"), Namespace: str(rm, "namespace"),
				Why: fmt.Sprintf("Helmfile release %s installs chart %s", str(rm, "name"), str(rm, "chart"))}
			if templated {
				t.ValuesComplete, t.IncompleteReason = false, "the helmfile is templated ({{ … }}); values may depend on the helmfile environment"
			}
			vals, _ := rm["values"].([]any)
			for vi, v := range vals {
				switch x := v.(type) {
				case string:
					p := filepath.Join(dir, x)
					referenced = append(referenced, p)
					if strings.HasSuffix(x, ".gotmpl") || strings.Contains(x, "{{") {
						t.ValuesComplete, t.IncompleteReason = false, fmt.Sprintf("values entry %s is a template", x)
						continue
					}
					b, err := os.ReadFile(p)
					if err != nil {
						t.ValuesComplete, t.IncompleteReason = false, fmt.Sprintf("values file %s unreadable: %v", x, err)
						continue
					}
					t.Values = append(t.Values, ValuesLayer{Origin: p, Content: b})
				case map[string]any:
					t.Values = append(t.Values, ValuesLayer{Origin: fmt.Sprintf("%s.values[%d]", origin, vi), Content: yamlBytes(x)})
				}
			}
			if s, ok := rm["secrets"].([]any); ok && len(s) > 0 {
				t.ValuesComplete, t.IncompleteReason = false, "secrets entries are encrypted and not read"
			}
			if sets, ok := rm["set"].([]any); ok {
				for _, s := range sets {
					sm, _ := s.(map[string]any)
					name, _ := sm["name"].(string)
					if name == "" {
						continue
					}
					t.Set = append(t.Set, SetValue{Path: name, Value: encode(sm["value"]), Origin: origin + ".set"})
				}
			}
			out = append(out, t)
		}
	}
	return out, referenced, notes
}

// RepoFilesFrom collects the repo-mode discoveries of an environment by role.
func RepoFilesFrom(e *env.Environment) RepoFiles {
	var rf RepoFiles
	if e == nil {
		return rf
	}
	seen := map[string]bool{}
	for _, d := range e.Discovered {
		k := d.Kind + "|" + d.Path
		if seen[k] {
			continue
		}
		seen[k] = true
		switch d.Kind {
		case env.DiscArgoCD:
			rf.ArgoCD = append(rf.ArgoCD, d.Path)
		case env.DiscFlux:
			rf.Flux = append(rf.Flux, d.Path)
		case env.DiscHelmfile:
			rf.Helmfile = append(rf.Helmfile, d.Path)
		case env.DiscValues:
			rf.Values = append(rf.Values, d.Path)
		case env.DiscKustomization:
			rf.Kustomizations = append(rf.Kustomizations, d.Path)
		}
	}
	return rf
}
