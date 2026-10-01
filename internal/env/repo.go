// Repository mode: given a directory (a customer's GitOps/config repository),
// discover the environment inputs by convention instead of flags. The walk is
// bounded and deterministic; every classification is recorded as an
// evidence-backed RepoDiscovery so a human can audit what was picked up — and
// what was skipped.

package env

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// Bounds of the repo walk; hitting one is a warning, never a silent stop.
const (
	maxRepoDepth = 10   // directory levels below the root
	maxRepoFiles = 5000 // files considered
)

// Discovery kinds (RepoDiscovery.Kind).
const (
	DiscValues        = "values"             // Helm values file (by name or helmfile/helmfile reference)
	DiscManifests     = "manifests"          // k8s-shaped YAML (documents with apiVersion/kind)
	DiscKustomization = "kustomization"      // a kustomization.yaml itself
	DiscKustomizeRef  = "kustomize-resource" // a file a kustomization references
	DiscArgoCD        = "argocd"             // an Argo CD Application CR
	DiscFlux          = "flux"               // a Flux HelmRelease CR
	DiscHelmfile      = "helmfile"           // a helmfile.yaml
	DiscWorkflow      = "workflow"           // a GitHub Actions workflow (image references)
	DiscTerraform     = "terraform"          // a .tf file (image references only)
)

// skipRepoDirs are directory names never descended into (build/VCS noise;
// vendored dependency trees whose YAML would drown the environment).
var skipRepoDirs = map[string]bool{
	"node_modules": true, "vendor": true, ".terraform": true,
	"dist": true, "target": true, "tmp": true,
}

// skipRepoDir reports whether a directory is skipped by name.
func skipRepoDir(name string) bool {
	if skipRepoDirs[name] {
		return true
	}
	// hidden directories, except .github (workflow image references live there)
	return strings.HasPrefix(name, ".") && name != ".github"
}

// RepoDiscovery is one file the repo walk classified and why.
type RepoDiscovery struct {
	Path     string // walked path (root-prefixed, as the evidence URIs)
	Kind     string // one of the Disc* constants
	Detail   string // human-readable grounding ("Argo CD Application at L1", "referenced by ... (L3)")
	Evidence []domain.EvidenceID
}

// discoverRepo walks root and classifies every candidate file. It returns the
// discovered values and manifest files (absolute-as-given paths, ready for the
// loaders); inline values (Argo/Flux) and images (workflows, terraform,
// kustomization) are recorded directly on the loader.
func (l *loader) discoverRepo(root string) (values, manifests []string, err error) {
	fi, err := os.Stat(root)
	if err != nil {
		return nil, nil, fmt.Errorf("env: %w", err)
	}
	if !fi.IsDir() {
		return nil, nil, fmt.Errorf("env: %s is not a directory", root)
	}
	l.env.RepoRoot = root
	var vals, mans []string
	if err := l.walkRepoDir(root, root, 0, &vals, &mans); err != nil {
		return nil, nil, err
	}
	sort.Strings(vals)
	sort.Strings(mans)
	vals, _ = mergeFiles(vals, nil) // a file discovered twice (by name and by reference) is loaded once, at its last position
	mans, _ = mergeFiles(mans, nil)
	sort.SliceStable(l.env.Discovered, func(i, j int) bool {
		a, b := l.env.Discovered[i], l.env.Discovered[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Kind < b.Kind
	})
	// drop records identical in path, kind and detail (a resource referenced
	// twice by one kustomization, say); different kinds for one path stay —
	// both groundings are true
	deduped := l.env.Discovered[:0]
	for i, d := range l.env.Discovered {
		if i > 0 {
			prev := l.env.Discovered[i-1]
			if d.Path == prev.Path && d.Kind == prev.Kind && d.Detail == prev.Detail {
				continue
			}
		}
		deduped = append(deduped, d)
	}
	l.env.Discovered = deduped
	return vals, mans, nil
}

func (l *loader) walkRepoDir(root, dir string, depth int, vals, mans *[]string) error {
	if depth > maxRepoDepth {
		l.warnf("", "repo %s is deeper than %d levels; deeper files were ignored", root, maxRepoDepth)
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("env: read %s: %w", dir, err)
	}
	for _, e := range entries {
		name := e.Name()
		p := filepath.Join(dir, name)
		if e.IsDir() {
			if skipRepoDir(name) {
				continue
			}
			if _, err := os.Stat(filepath.Join(p, "Chart.yaml")); err == nil {
				l.warnf("", "skipped %s: a Helm chart source tree (its values and templates are chart inputs, not customer environment)", p)
				continue
			}
			if err := l.walkRepoDir(root, p, depth+1, vals, mans); err != nil {
				return err
			}
			continue
		}
		l.repoFiles++
		if l.repoFiles > maxRepoFiles {
			l.warnf("", "more than %d files in %s; the rest were ignored", maxRepoFiles, root)
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			rel = p
		}
		l.classifyRepoFile(rel, p, vals, mans)
	}
	return nil
}

func (l *loader) classifyRepoFile(rel, path string, vals, mans *[]string) {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	switch {
	case ext == ".tf":
		l.discTerraform(path, rel)
	case ext != ".yaml" && ext != ".yml":
		return
	case strings.HasSuffix("/"+filepath.ToSlash(filepath.Dir(rel)), "/.github/workflows"):
		l.discWorkflow(path, rel)
	case base == "kustomization.yaml" || base == "kustomization.yml" || base == "Kustomization":
		l.discKustomization(path, rel, mans)
	case base == "helmfile.yaml" || base == "helmfile.yml":
		l.discHelmfile(path, rel, vals)
	case isValuesName(base):
		*vals = append(*vals, path)
		l.env.Discovered = append(l.env.Discovered, RepoDiscovery{
			Path: path, Kind: DiscValues, Detail: "Helm values file (by name)",
			Evidence: []domain.EvidenceID{l.fileEvidence(path)},
		})
	default:
		l.discByContent(path, rel, mans)
	}
}

// fileEvidence is the fallback evidence of a classification that has no
// specific line: the first content line of the file. The file is registered
// as a parsed input at that point, so the evidence carries its digest.
func (l *loader) fileEvidence(path string) domain.EvidenceID {
	b, _ := l.readFile(path)
	line, text := firstContentLine(b)
	return l.ev(path, fmt.Sprintf("L%d", line), text)
}

// firstContentLine returns the 1-based number and text of the first
// non-empty, non-comment line.
func firstContentLine(b []byte) (int, string) {
	for i, ln := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		return i + 1, t
	}
	return 1, ""
}

// isValuesName matches the values-file conventions: values.yaml,
// values-prod.yaml, prod-values.yaml, staging_values.yml.
func isValuesName(base string) bool {
	low := strings.ToLower(base)
	ext := filepath.Ext(low)
	switch ext {
	case ".yaml", ".yml":
	default:
		return false
	}
	stem := strings.TrimSuffix(low, ext)
	return stem == "values" || strings.HasPrefix(stem, "values-") ||
		strings.HasSuffix(stem, "-values") || strings.HasSuffix(stem, "_values")
}

// discByContent peeks at a YAML file and classifies it by its documents:
// an Argo CD Application, a Flux HelmRelease, or plain k8s manifests. Files
// without any apiVersion/kind document (compose files, CI configs, chart
// templates) are silently skipped — a repo tree is full of YAML that is not
// an environment. The peek does not register the file as a parsed input;
// only a classified file becomes part of the environment.
func (l *loader) discByContent(path, rel string, mans *[]string) {
	b, err := os.ReadFile(path)
	if err != nil {
		l.warnf("", "%s could not be read: %v", path, err)
		return
	}
	docs, _ := parseDocs(b, nil)
	for _, d := range docs {
		if d.node == nil {
			continue
		}
		apiVersion := scalarOf(d.node, "apiVersion")
		kind := scalarOf(d.node, "kind")
		if apiVersion == "" || kind == "" {
			continue
		}
		group, _ := splitGroupVersion(apiVersion)
		dk, detail := DiscManifests, fmt.Sprintf("%s at L%d (%s)", kind, d.startLine, apiVersion)
		switch {
		case kind == "Application" && group == "argoproj.io":
			dk, detail = DiscArgoCD, fmt.Sprintf("Argo CD Application at L%d", d.startLine)
		case kind == "HelmRelease" && group == "helm.toolkit.fluxcd.io":
			dk, detail = DiscFlux, fmt.Sprintf("Flux HelmRelease at L%d", d.startLine)
		}
		*mans = append(*mans, path)
		l.readFile(path) // register the digest first, so the evidence carries it
		ev := l.ev(path, fmt.Sprintf("L%d", d.startLine), "apiVersion: "+apiVersion+" / kind: "+kind)
		l.env.Discovered = append(l.env.Discovered, RepoDiscovery{
			Path: path, Kind: dk, Detail: detail,
			Evidence: []domain.EvidenceID{ev},
		})
		return
	}
}

// discWorkflow extracts container image references from a GitHub Actions
// workflow (any "image:" scalar); the workflow itself is not an input.
func (l *loader) discWorkflow(path, rel string) {
	b, err := l.readFile(path)
	if err != nil {
		l.warnf("", "%s could not be read: %v", path, err)
		return
	}
	docs, _ := parseDocs(b, nil)
	n, first := 0, domain.EvidenceID("")
	for _, d := range docs {
		if d.node == nil {
			continue
		}
		walkMappings(d.node, func(key string, val *yaml.Node, _ string) {
			if key != "image" || val.Kind != yaml.ScalarNode {
				return
			}
			ref := val.Value
			if strings.ContainsAny(ref, "{}$") || (!strings.Contains(ref, ":") && !strings.Contains(ref, "/")) {
				return
			}
			if _, err := normalize.ParseImageRef(ref); err != nil {
				return
			}
			ev := l.ev(path, fmt.Sprintf("L%d", val.Line), "image: "+ref)
			l.addImageAt(ref, "workflow", path, val.Line)
			if n == 0 {
				first = ev
			}
			n++
		})
	}
	ev := first
	if ev == "" {
		ev = l.fileEvidence(path)
	}
	l.env.Discovered = append(l.env.Discovered, RepoDiscovery{
		Path: path, Kind: DiscWorkflow,
		Detail: fmt.Sprintf("%d image reference(s)", n), Evidence: []domain.EvidenceID{ev},
	})
}

// tfImageRe matches the trivially deterministic Terraform image fact:
// image = "registry/repo:tag" (variables, templating and HCL structure are
// not evaluated — a documented gap).
var tfImageRe = regexp.MustCompile(`(?m)^[ \t]*image[ \t]*=[ \t]*"([^"]*)"`)

func (l *loader) discTerraform(path, rel string) {
	b, err := l.readFile(path)
	if err != nil {
		l.warnf("", "%s could not be read: %v", path, err)
		return
	}
	s := string(b)
	n, first := 0, domain.EvidenceID("")
	for _, loc := range tfImageRe.FindAllStringSubmatchIndex(s, -1) {
		ref := s[loc[2]:loc[3]]
		if ref == "" || strings.ContainsAny(ref, "{}$") {
			continue
		}
		if _, err := normalize.ParseImageRef(ref); err != nil {
			continue
		}
		line := 1 + strings.Count(s[:loc[0]], "\n")
		ev := l.ev(path, fmt.Sprintf("L%d", line), `image = "`+ref+`"`)
		l.addImageAt(ref, "terraform", path, line)
		if n == 0 {
			first = ev
		}
		n++
	}
	ev := first
	if ev == "" {
		ev = l.fileEvidence(path)
	}
	l.env.Discovered = append(l.env.Discovered, RepoDiscovery{
		Path: path, Kind: DiscTerraform,
		Detail:   fmt.Sprintf("%d image reference(s); HCL structure is not evaluated", n),
		Evidence: []domain.EvidenceID{ev},
	})
}

// discKustomization inventories what a kustomize build would include — the
// cheap way: every referenced resource/patch file is added to the manifest
// inputs (with a warning that patches and transformers are NOT applied), and
// the kustomize `images:` overrides become image facts.
func (l *loader) discKustomization(path, rel string, mans *[]string) {
	b, err := l.readFile(path)
	if err != nil {
		l.warnf("", "%s could not be read: %v", path, err)
		return
	}
	docs, _ := parseDocs(b, nil)
	var node *yaml.Node
	for _, d := range docs {
		if d.node != nil {
			node = d.node
			break
		}
	}
	if node == nil {
		return
	}
	dir := filepath.Dir(path)
	addRef := func(ref string, line int, key string) {
		if ref == "" {
			return
		}
		target := filepath.Join(dir, ref)
		files, err := expand(target)
		if err != nil {
			l.warnf("", "kustomization %s references %s, which is not present", rel, ref)
			return
		}
		for _, f := range files {
			*mans = append(*mans, f)
			l.env.Discovered = append(l.env.Discovered, RepoDiscovery{
				Path: f, Kind: DiscKustomizeRef,
				Detail:   fmt.Sprintf("referenced by %s (L%d)", rel, line),
				Evidence: []domain.EvidenceID{l.ev(path, fmt.Sprintf("L%d", line), key+": "+ref)},
			})
		}
	}
	for _, key := range []string{"resources", "bases"} {
		for _, item := range itemsOf(fieldOf(node, key)) {
			if item.Kind == yaml.ScalarNode && strings.TrimSpace(item.Value) != "" {
				addRef(item.Value, item.Line, key)
			}
		}
	}
	for _, item := range itemsOf(fieldOf(node, "patchesStrategicMerge")) {
		if item.Kind == yaml.ScalarNode {
			addRef(item.Value, item.Line, "patchesStrategicMerge")
		}
	}
	for _, key := range []string{"patches", "patchesJson6902"} {
		for _, item := range itemsOf(fieldOf(node, key)) {
			addRef(scalarOf(item, "path"), item.Line, key)
		}
	}
	// kustomize image overrides pin tags the same way a manifest does
	for _, item := range itemsOf(fieldOf(node, "images")) {
		name := scalarOf(item, "name")
		if name == "" {
			continue
		}
		repo := name
		if nn := scalarOf(item, "newName"); nn != "" {
			repo = nn
		}
		ref := repo
		if d := scalarOf(item, "digest"); d != "" {
			ref = repo + "@" + d
		} else if t := scalarOf(item, "newTag"); t != "" {
			ref = repo + ":" + t
		}
		if _, err := normalize.ParseImageRef(ref); err != nil {
			continue
		}
		l.addImageAt(ref, "kustomization", path, item.Line)
	}
	l.warnf("", "kustomization %s is not built with kustomize: its resources are inventoried as plain manifests, patches and transformers are not applied", rel)
	l.env.Discovered = append(l.env.Discovered, RepoDiscovery{
		Path: path, Kind: DiscKustomization, Detail: "resources inventoried (no kustomize build)",
		Evidence: []domain.EvidenceID{l.fileEvidence(path)},
	})
}

// discHelmfile inventories helmfile releases: chart + version become
// installed-product guesses, referenced values files become values inputs.
// Templating (go templates, environments) is NOT evaluated — documented gap.
func (l *loader) discHelmfile(path, rel string, vals *[]string) {
	b, err := l.readFile(path)
	if err != nil {
		l.warnf("", "%s could not be read: %v", path, err)
		return
	}
	docs, _ := parseDocs(b, nil)
	var node *yaml.Node
	for _, d := range docs {
		if d.node != nil {
			node = d.node
			break
		}
	}
	if node == nil {
		return
	}
	releases := 0
	for _, r := range itemsOf(fieldOf(node, "releases")) {
		chart := scalarOf(r, "chart")
		if chart == "" {
			continue
		}
		version := scalarOf(r, "version")
		product := chart
		if i := strings.LastIndexByte(product, '/'); i >= 0 {
			product = product[i+1:]
		}
		ev := l.ev(path, fmt.Sprintf("$.releases[] (L%d)", r.Line), "chart: "+chart+" version: "+version)
		l.installed = append(l.installed, InstalledProduct{
			Product: product, Version: version, Chart: chart,
			Source: InstalledHelmfile, Evidence: []domain.EvidenceID{ev},
		})
		releases++
		for _, v := range itemsOf(fieldOf(r, "values")) {
			if v.Kind != yaml.ScalarNode {
				continue
			}
			target := filepath.Join(filepath.Dir(path), v.Value)
			if fi, err := os.Stat(target); err != nil || fi.IsDir() {
				continue
			}
			switch strings.ToLower(filepath.Ext(target)) {
			case ".yaml", ".yml":
			default:
				continue
			}
			*vals = append(*vals, target)
			l.env.Discovered = append(l.env.Discovered, RepoDiscovery{
				Path: target, Kind: DiscValues,
				Detail:   fmt.Sprintf("referenced by %s (L%d)", rel, v.Line),
				Evidence: []domain.EvidenceID{l.ev(path, fmt.Sprintf("L%d", v.Line), "values: "+v.Value)},
			})
		}
	}
	l.warnf("", "helmfile %s: templating (go templates, environments) is not evaluated; releases are inventoried from the literal file", rel)
	l.env.Discovered = append(l.env.Discovered, RepoDiscovery{
		Path: path, Kind: DiscHelmfile,
		Detail:   fmt.Sprintf("%d release(s); templating not evaluated", releases),
		Evidence: []domain.EvidenceID{l.fileEvidence(path)},
	})
}
