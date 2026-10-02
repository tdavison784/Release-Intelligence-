package render

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// kustomizationNames are the file names kustomize recognises, in its order.
var kustomizationNames = []string{"kustomization.yaml", "kustomization.yml", "Kustomization"}

// KustomizationFile returns the kustomization file of dir, or "".
func KustomizationFile(dir string) string {
	for _, n := range kustomizationNames {
		p := filepath.Join(dir, n)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

var kustomizeLimitations = []string{
	"no cluster access; remote bases/resources (URLs) are fetched by kustomize at render time and are not digested",
}

// Kustomize renders overlays with `kustomize build`, or `kubectl kustomize`
// when the standalone binary is absent.
type Kustomize struct {
	Runner Runner
	Cache  *Cache
	Now    func() time.Time

	once    sync.Once
	argv    []string // binary + subcommand
	version string
	verErr  error
}

// NewKustomize returns a Kustomize renderer with the default runner.
func NewKustomize(cache *Cache) *Kustomize {
	return &Kustomize{Runner: ExecRunner{}, Cache: cache, Now: time.Now}
}

// Tool implements Renderer.
func (k *Kustomize) Tool() Tool { return ToolKustomize }

// Version implements Renderer.
func (k *Kustomize) Version(ctx context.Context) (string, error) {
	k.once.Do(func() {
		if _, err := k.Runner.LookPath("kustomize"); err == nil {
			out, _, err := k.Runner.Run(ctx, "", "kustomize", "version")
			if err == nil {
				k.argv = []string{"kustomize", "build"}
				k.version = strings.TrimSpace(string(out))
				return
			}
		}
		if _, err := k.Runner.LookPath("kubectl"); err == nil {
			out, _, err := k.Runner.Run(ctx, "", "kubectl", "version", "--client", "-o", "json")
			if err == nil {
				var v struct {
					Client struct {
						GitVersion string `json:"gitVersion"`
					} `json:"clientVersion"`
					Kustomize string `json:"kustomizeVersion"`
				}
				if json.Unmarshal(out, &v) == nil && v.Kustomize != "" {
					k.argv = []string{"kubectl", "kustomize"}
					k.version = v.Kustomize + " (kubectl " + v.Client.GitVersion + ")"
					return
				}
			}
		}
		k.verErr = &Failure{Reason: FailRendererUnavailable, Detail: "neither kustomize nor kubectl (with built-in kustomize) is installed"}
	})
	return k.version, k.verErr
}

// Render implements Renderer.
func (k *Kustomize) Render(ctx context.Context, req Request) *Result {
	now := time.Now
	if k.Now != nil {
		now = k.Now
	}
	prov := Provenance{Scope: req.Scope, Tool: ToolKustomize, RenderedAt: now().UTC().Truncate(time.Second),
		Limitations: kustomizeLimitations, ValuesComplete: req.ValuesComplete, IncompleteReason: req.IncompleteReason}
	fail := func(reason FailureReason, format string, args ...any) *Result {
		return &Result{Status: StatusFailed, Failure: &Failure{Reason: reason, Detail: fmt.Sprintf(format, args...)}, Provenance: prov}
	}
	in := req.Kustomize
	if in == nil || in.Dir == "" {
		return fail(FailUnsupportedFeature, "no overlay directory")
	}
	root := in.Root
	if root == "" {
		root = in.Dir
	}
	root, _ = filepath.Abs(root)
	dir, _ := filepath.Abs(in.Dir)
	rel, err := filepath.Rel(root, dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fail(FailKustomizeDependency, "overlay %s is outside the root %s", in.Dir, root)
	}
	prov.KustomizeRoot, prov.KustomizeDir, prov.SourceRevision = root, filepath.ToSlash(rel), in.SourceRevision
	prov.Substitutions = in.Substitutions
	ver, err := k.Version(ctx)
	if err != nil {
		if f, ok := err.(*Failure); ok {
			return &Result{Status: StatusFailed, Failure: f, Provenance: prov}
		}
		return fail(FailRendererUnavailable, "%v", err)
	}
	prov.ToolVersion = ver
	inputs, remote, unsupported, err := KustomizeInputs(root, dir)
	if err != nil {
		return fail(FailKustomizeDependency, "%v", err)
	}
	if unsupported != "" {
		return fail(FailUnsupportedFeature, "%s", unsupported)
	}
	for _, r := range remote {
		prov.Limitations = append(prov.Limitations, "remote resource: "+r)
	}
	for _, p := range inputs {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			return fail(FailKustomizeDependency, "read %s: %v", p, err)
		}
		prov.Inputs = append(prov.Inputs, InputDigest{Origin: p, Digest: domain.Digest(b)})
	}
	prov.ArtifactDigest = treeDigest(prov.Inputs, in.Substitutions)
	prov.ValuesDigest = prov.ArtifactDigest // the overlay IS the customer configuration
	prov.Command = append(append([]string{}, k.argv...), filepath.ToSlash(rel))
	prov.CacheKey = cacheKey(prov)
	if res, ok := k.Cache.Get(prov.CacheKey); ok {
		res.Provenance.RenderedAt = prov.RenderedAt
		res.Provenance.KustomizeRoot = prov.KustomizeRoot
		res.Cached = true
		return res
	}
	buildRoot := root
	if len(in.Substitutions) > 0 {
		tmp, err := os.MkdirTemp("", "ri-kustomize-")
		if err != nil {
			return fail(FailUnsupportedFeature, "temp dir: %v", err)
		}
		defer os.RemoveAll(tmp)
		if err := copyInputs(root, tmp, inputs); err != nil {
			return fail(FailKustomizeDependency, "%v", err)
		}
		if err := applySubstitutions(tmp, in.Substitutions); err != nil {
			return fail(FailUnsupportedFeature, "%v", err)
		}
		buildRoot = tmp
	}
	args := append(append([]string{}, k.argv[1:]...), filepath.Join(buildRoot, rel))
	out, errb, err := k.Runner.Run(ctx, buildRoot, k.argv[0], args...)
	if err != nil {
		return fail(classifyKustomize(string(errb)), "%s", boundDetail(strings.ReplaceAll(string(errb), buildRoot, "<root>")))
	}
	res, f := finish(prov, out)
	if f != nil {
		return &Result{Status: StatusFailed, Failure: f, Provenance: prov}
	}
	k.Cache.Put(res)
	return res
}

func classifyKustomize(stderr string) FailureReason {
	switch {
	case strings.Contains(stderr, "helmCharts") || strings.Contains(stderr, "--enable-helm"):
		return FailUnsupportedFeature
	case strings.Contains(stderr, "invalid Kustomization") || strings.Contains(stderr, "unknown field"):
		return FailUnsupportedFeature
	}
	return FailKustomizeDependency
}

func treeDigest(inputs []InputDigest, subs []Substitution) string {
	h := sha256.New()
	for _, in := range inputs {
		h.Write([]byte(in.Origin + "\x00" + in.Digest + "\x00"))
	}
	for _, s := range subs {
		h.Write([]byte("sub\x00" + s.File + "\x00" + s.Field + "\x00" + s.From + "\x00" + s.To + "\x00"))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// isRemote reports whether a kustomize resource entry is remote.
func isRemote(s string) bool {
	return strings.Contains(s, "://") || strings.Contains(s, "?ref=") || strings.HasPrefix(s, "github.com/") || strings.HasPrefix(s, "git@")
}

// kustomization is the subset of fields that reference files.
type kustomization struct {
	Resources             []string `yaml:"resources"`
	Bases                 []string `yaml:"bases"`
	Components            []string `yaml:"components"`
	CRDs                  []string `yaml:"crds"`
	PatchesStrategicMerge []string `yaml:"patchesStrategicMerge"`
	Patches               []struct {
		Path string `yaml:"path"`
	} `yaml:"patches"`
	PatchesJSON6902 []struct {
		Path string `yaml:"path"`
	} `yaml:"patchesJson6902"`
	ConfigMapGenerator []generator `yaml:"configMapGenerator"`
	SecretGenerator    []generator `yaml:"secretGenerator"`
	Replacements       []struct {
		Path string `yaml:"path"`
	} `yaml:"replacements"`
	Transformers []string  `yaml:"transformers"`
	Generators   []string  `yaml:"generators"`
	HelmCharts   []any     `yaml:"helmCharts"`
	Images       []kzImage `yaml:"images"`
}

type generator struct {
	Files []string `yaml:"files"`
	Envs  []string `yaml:"envs"`
	Env   string   `yaml:"env"`
}

type kzImage struct {
	Name    string `yaml:"name"`
	NewName string `yaml:"newName"`
	NewTag  string `yaml:"newTag"`
	Digest  string `yaml:"digest"`
}

// KustomizeInputs returns the local input files (relative to root, sorted)
// an overlay transitively reads, the remote references it makes, and a
// non-empty reason when it uses a feature this path does not support.
func KustomizeInputs(root, dir string) (files, remote []string, unsupported string, err error) {
	root, _ = filepath.Abs(root)
	dir, _ = filepath.Abs(dir)
	seen := map[string]bool{}
	visited := map[string]bool{}
	var walk func(d string, depth int) error
	add := func(abs string) error {
		rel, err := filepath.Rel(root, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("kustomize input %s is outside the root %s", abs, root)
		}
		rel = filepath.ToSlash(rel)
		if !seen[rel] {
			seen[rel] = true
			files = append(files, rel)
		}
		return nil
	}
	walk = func(d string, depth int) error {
		if depth > 20 {
			return fmt.Errorf("kustomization nesting deeper than 20 at %s", d)
		}
		if visited[d] {
			return nil
		}
		visited[d] = true
		kf := KustomizationFile(d)
		if kf == "" {
			return fmt.Errorf("%s has no kustomization file", d)
		}
		if err := add(kf); err != nil {
			return err
		}
		b, err := os.ReadFile(kf)
		if err != nil {
			return err
		}
		var k kustomization
		if err := yaml.Unmarshal(b, &k); err != nil {
			return fmt.Errorf("%s: %v", kf, err)
		}
		if len(k.HelmCharts) > 0 && unsupported == "" {
			unsupported = fmt.Sprintf("%s uses helmCharts (requires --enable-helm and chart downloads); render the chart through the Helm path instead", kf)
		}
		var refs []string
		refs = append(refs, k.Resources...)
		refs = append(refs, k.Bases...)
		refs = append(refs, k.Components...)
		refs = append(refs, k.CRDs...)
		refs = append(refs, k.PatchesStrategicMerge...)
		refs = append(refs, k.Transformers...)
		refs = append(refs, k.Generators...)
		for _, p := range k.Patches {
			refs = append(refs, p.Path)
		}
		for _, p := range k.PatchesJSON6902 {
			refs = append(refs, p.Path)
		}
		for _, p := range k.Replacements {
			refs = append(refs, p.Path)
		}
		for _, g := range append(k.ConfigMapGenerator, k.SecretGenerator...) {
			for _, f := range g.Files {
				if i := strings.IndexByte(f, '='); i >= 0 {
					f = f[i+1:]
				}
				refs = append(refs, f)
			}
			refs = append(refs, g.Envs...)
			refs = append(refs, g.Env)
		}
		for _, r := range refs {
			r = strings.TrimSpace(r)
			if r == "" || strings.Contains(r, "\n") { // inline patches
				continue
			}
			if isRemote(r) {
				remote = append(remote, r)
				continue
			}
			p := filepath.Join(d, r)
			fi, err := os.Stat(p)
			if err != nil {
				return fmt.Errorf("%s references %s, which does not exist", kf, r)
			}
			if fi.IsDir() {
				if err := walk(p, depth+1); err != nil {
					return err
				}
				continue
			}
			if err := add(p); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(dir, 0); err != nil {
		return nil, nil, "", err
	}
	sort.Strings(files)
	sort.Strings(remote)
	return files, remote, unsupported, nil
}

func copyInputs(root, dst string, files []string) error {
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			return err
		}
		p := filepath.Join(dst, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, b, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// PlanVersionSubstitution plans the target render of an overlay that pins
// the product's source version: every kustomization file among the inputs
// whose images[].newTag or remote resource reference carries the from
// version gets it replaced by the to version. Nothing else is touched. An
// empty plan means the overlay does not pin the product version, so a target
// render is not derivable from it (the caller reports render-not-applicable,
// never "no change").
func PlanVersionSubstitution(root string, files []string, from, to string) ([]Substitution, error) {
	var out []Substitution
	for _, f := range files {
		base := filepath.Base(f)
		isK := false
		for _, n := range kustomizationNames {
			isK = isK || base == n
		}
		if !isK {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			return nil, err
		}
		var k kustomization
		if err := yaml.Unmarshal(b, &k); err != nil {
			return nil, fmt.Errorf("%s: %v", f, err)
		}
		for _, img := range k.Images {
			if nt, ok := replaceVersion(img.NewTag, from, to); ok && img.NewTag != "" {
				out = append(out, Substitution{File: f, Field: "images[" + img.Name + "].newTag", From: img.NewTag, To: nt})
			}
		}
		for field, list := range map[string][]string{"resources": k.Resources, "bases": k.Bases, "components": k.Components} {
			for _, r := range list {
				if !isRemote(r) {
					continue
				}
				if nr, ok := replaceVersion(r, from, to); ok {
					out = append(out, Substitution{File: f, Field: field + "[" + r + "]", From: r, To: nr})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Field < out[j].Field
	})
	return out, nil
}

// replaceVersion replaces a whole-token occurrence of the from version
// ("v1.17.0" or "1.17.0") in s by the to version, keeping the v prefix style.
func replaceVersion(s, from, to string) (string, bool) {
	from, to = strings.TrimPrefix(from, "v"), strings.TrimPrefix(to, "v")
	if from == "" || to == "" || from == to {
		return s, false
	}
	re := regexp.MustCompile(`(^|[^0-9A-Za-z.])(v?)` + regexp.QuoteMeta(from) + `($|[^0-9A-Za-z.-])`)
	if !re.MatchString(s) {
		return s, false
	}
	return re.ReplaceAllString(s, "${1}${2}"+to+"${3}"), true
}

// applySubstitutions rewrites the copied kustomization files.
func applySubstitutions(root string, subs []Substitution) error {
	byFile := map[string][]Substitution{}
	for _, s := range subs {
		byFile[s.File] = append(byFile[s.File], s)
	}
	for f, ss := range byFile {
		p := filepath.Join(root, f)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("%s: %v", f, err)
		}
		applied := 0
		for _, s := range ss {
			applied += substituteNode(&doc, s)
		}
		if applied < len(ss) {
			return fmt.Errorf("%s: %d of %d substitutions did not apply", f, len(ss)-applied, len(ss))
		}
		out, err := yaml.Marshal(&doc)
		if err != nil {
			return err
		}
		if err := os.WriteFile(p, out, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func substituteNode(doc *yaml.Node, s Substitution) int {
	root := doc
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	field := s.Field[:strings.IndexByte(s.Field, '[')]
	sel := s.Field[len(field)+1 : strings.LastIndexByte(s.Field, ']')]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != field {
			continue
		}
		list := root.Content[i+1]
		for _, item := range list.Content {
			switch {
			case field == "images" && item.Kind == yaml.MappingNode:
				name := ""
				for j := 0; j+1 < len(item.Content); j += 2 {
					if item.Content[j].Value == "name" {
						name = item.Content[j+1].Value
					}
				}
				if name != sel {
					continue
				}
				for j := 0; j+1 < len(item.Content); j += 2 {
					if item.Content[j].Value == "newTag" && item.Content[j+1].Value == s.From {
						item.Content[j+1].Value = s.To
						return 1
					}
				}
			case item.Kind == yaml.ScalarNode && item.Value == s.From:
				item.Value = s.To
				return 1
			}
		}
	}
	return 0
}
