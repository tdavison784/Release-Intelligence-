package render

import (
	"bytes"
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
	// layeredBy: repo-local values files an Argo CD / Flux target already
	// layers (valueFiles), so they are not rendered again on their own — that
	// configuration is not deployed anywhere.
	layeredBy := map[string]string{}
	layer := func(ts []Target) {
		for _, t := range ts {
			for _, l := range t.Values {
				if _, ok := layeredBy[filepath.Clean(l.Origin)]; !ok {
					layeredBy[filepath.Clean(l.Origin)] = t.ID
				}
			}
		}
	}
	for _, f := range o.Files.ArgoCD {
		ts, n := argoTargets(o, f)
		targets, notes = append(targets, ts...), append(notes, n...)
		layer(ts)
	}
	for _, f := range o.Files.Flux {
		ts, n := fluxTargets(o, f)
		targets, notes = append(targets, ts...), append(notes, n...)
		layer(ts)
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
			case layeredBy[filepath.Clean(f)] != "":
				notes = append(notes, fmt.Sprintf("values file %s: rendered as a values layer of %s, not on its own", f, layeredBy[filepath.Clean(f)]))
				continue
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
		refs := map[string]bool{}
		for _, s := range srcs {
			if r := str(s, "ref"); r != "" {
				refs[r] = true
			}
		}
		for i, s := range srcs {
			origin := fmt.Sprintf("%s#%d/$.%s", file, di, locs[i])
			chart, path := str(s, "chart"), str(s, "path")
			helm, _ := get(s, "helm").(map[string]any)
			identity := chart
			if chart == "" {
				if path == "" || o.Repo == "" {
					continue // a values-only (ref) source deploys nothing
				}
				dir := filepath.Join(o.Repo, path)
				if KustomizationFile(dir) != "" {
					out = append(out, Target{Kind: TargetKustomize, Kustomize: &KustomizeInput{Dir: dir, Root: o.Repo},
						Why: fmt.Sprintf("Argo CD Application %s deploys this overlay (%s.path: %s)", app, locs[i], path)})
					continue
				}
				// a git Helm chart source: Helm settings, or a Chart.yaml in this
				// repository (env.argoReleases applies the same rule)
				if _, err := os.Stat(filepath.Join(dir, "Chart.yaml")); helm == nil && err != nil {
					continue
				}
				identity = path
			}
			if !o.matchesChart(identity) {
				continue
			}
			t := Target{ID: "argocd:" + origin, Kind: TargetArgoCD, Origin: origin, Namespace: ns, ValuesComplete: true,
				Why: fmt.Sprintf("Argo CD Application %s installs chart %s", app, identity)}
			t.ReleaseName = str(s, "helm", "releaseName")
			if t.ReleaseName == "" {
				t.ReleaseName = app
			}
			incomplete := func(reason string) {
				if t.ValuesComplete {
					t.ValuesComplete, t.IncompleteReason = false, reason
				}
			}
			// Argo CD "Helm Value Precedence": parameters > valuesObject > values
			// > valueFiles; the stack below is env.argoReleases' (one rule set)
			if vf, ok := helm["valueFiles"].([]any); ok {
				ignoreMissing, _ := helm["ignoreMissingValueFiles"].(bool)
				for _, f := range vf {
					fs, _ := f.(string)
					if strings.TrimSpace(fs) == "" {
						continue
					}
					p, why := argoValueFile(o.Repo, fs, path, refs)
					if why == "" {
						b, err := os.ReadFile(p)
						if err == nil {
							t.Values = append(t.Values, ValuesLayer{Origin: p, Content: b})
							continue
						}
						why = fmt.Sprintf("helm.valueFiles %q is not present in the repository", fs)
						if ignoreMissing {
							why += " (ignoreMissingValueFiles: its absence here does not show its absence in the revision Argo CD deploys)"
						}
					}
					incomplete(why)
				}
			}
			_, hasValues := helm["values"]
			if vo, ok := helm["valuesObject"].(map[string]any); ok {
				t.Values = append(t.Values, ValuesLayer{Origin: origin + ".helm.valuesObject", Content: yamlBytes(vo)})
				if hasValues && helm["values"] != nil {
					notes = append(notes, fmt.Sprintf("Argo CD Application %s (%s): helm.values is ignored — valuesObject is set, and Argo CD applies valuesObject only (\"valuesObject trumps values\")", app, origin))
				}
			} else if v, ok := helm["values"].(string); ok && strings.TrimSpace(v) != "" {
				t.Values = append(t.Values, ValuesLayer{Origin: origin + ".helm.values", Content: []byte(v)})
			} else if vm, ok := helm["values"].(map[string]any); ok {
				t.Values = append(t.Values, ValuesLayer{Origin: origin + ".helm.values", Content: yamlBytes(vm)})
			}
			if ps, ok := helm["parameters"].([]any); ok {
				for _, p := range ps {
					pm, _ := p.(map[string]any)
					name, _ := pm["name"].(string)
					if name == "" {
						continue
					}
					val := ""
					if pm["value"] != nil {
						val = fmt.Sprint(pm["value"])
					}
					force, _ := pm["forceString"].(bool)
					if !env.SetSupported(name, val, force) {
						incomplete(fmt.Sprintf("helm.parameters %s uses list syntax (an index or {a,b}) that is not evaluated", name))
						continue
					}
					// Argo CD passes --set (--set-string with forceString): Helm's
					// strvals typing decides the type, as env.SetValueJSON encodes it
					t.Set = append(t.Set, SetValue{Path: name, Value: env.SetValueJSON(val, force), Origin: origin + ".helm.parameters"})
				}
			}
			if fp, ok := helm["fileParameters"].([]any); ok && len(fp) > 0 {
				incomplete("helm.fileParameters (--set-file) are not evaluated")
			}
			out = append(out, t)
		}
	}
	return out, notes
}

// argoValueFile resolves one helm.valueFiles entry as Argo CD does: "$<ref>/path"
// from the repository root (the ref source is taken to be this repository), a
// plain path relative to the chart root — readable only for a git source whose
// path lies in this repository. why says why it is not readable ("" if it is).
func argoValueFile(repo, entry, srcPath string, refs map[string]bool) (path, why string) {
	inRepo := func(parts ...string) (string, bool) {
		if repo == "" {
			return "", false
		}
		p := filepath.Join(append([]string{repo}, parts...)...)
		r, err := filepath.Rel(repo, p)
		if err != nil || r == ".." || strings.HasPrefix(r, "../") {
			return "", false
		}
		return p, true
	}
	switch {
	case strings.Contains(entry, "://"):
		return "", fmt.Sprintf("helm.valueFiles %q is a remote URL, not readable from files", entry)
	case strings.HasPrefix(entry, "$"):
		ref, rest, _ := strings.Cut(entry[1:], "/")
		if !refs[ref] {
			return "", fmt.Sprintf("helm.valueFiles %q names $%s, which no source of the Application declares as ref", entry, ref)
		}
		if p, ok := inRepo(rest); ok {
			return p, ""
		}
	case srcPath == "":
		return "", fmt.Sprintf("helm.valueFiles %q resolves inside the chart package (relative to the chart root), not from this repository", entry)
	default:
		if p, ok := inRepo(srcPath, entry); ok {
			return p, ""
		}
	}
	return "", fmt.Sprintf("helm.valueFiles %q is not readable from this repository", entry)
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
	setTexts := helmfileSetTexts(raw)
	dir := filepath.Dir(file)
	for di, d := range docs {
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
				for si, s := range sets {
					sm, _ := s.(map[string]any)
					name, _ := sm["name"].(string)
					if name == "" {
						continue
					}
					// helmfile's set value is a string field: the scalar's text as
					// written ("1.50", "0123", a quoted "12") reaches helm --set,
					// which types it — env.SetValueJSON, as the values stack does
					text, ok := setTexts[[3]int{di, ri, si}]
					if !ok || !env.SetSupported(name, text, false) {
						if t.ValuesComplete {
							t.ValuesComplete, t.IncompleteReason = false, fmt.Sprintf("set entry %s is not evaluated (list syntax, --set-file or a non-scalar value)", name)
						}
						continue
					}
					t.Set = append(t.Set, SetValue{Path: name, Value: env.SetValueJSON(text, false), Origin: origin + ".set"})
				}
			}
			out = append(out, t)
		}
	}
	return out, referenced, notes
}

// helmfileSetTexts returns the scalar text of every releases[].set[].value
// of a helmfile, keyed by (document, release, set entry) — documents counted
// as yamlDocs counts them (null documents skipped). An entry with no single
// scalar value (values:, file:, a mapping or list) is absent; one without a
// value is "".
func helmfileSetTexts(raw []byte) map[[3]int]string {
	out := map[[3]int]string{}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	for di := 0; ; {
		var root yaml.Node
		if err := dec.Decode(&root); err != nil {
			return out
		}
		if len(root.Content) == 0 || (root.Content[0].Kind == yaml.ScalarNode && root.Content[0].Tag == "!!null") {
			continue
		}
		for ri, r := range nodeSeq(nodeField(root.Content[0], "releases")) {
			for si, s := range nodeSeq(nodeField(r, "set")) {
				v := nodeField(s, "value")
				switch {
				case nodeField(s, "values") != nil || nodeField(s, "file") != nil:
				case v == nil:
					out[[3]int{di, ri, si}] = ""
				case v.Kind == yaml.ScalarNode:
					out[[3]int{di, ri, si}] = v.Value
				}
			}
		}
		di++
	}
}

func nodeField(n *yaml.Node, key string) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			v := n.Content[i+1]
			for v != nil && v.Kind == yaml.AliasNode {
				v = v.Alias
			}
			return v
		}
	}
	return nil
}

func nodeSeq(n *yaml.Node) []*yaml.Node {
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	return n.Content
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
