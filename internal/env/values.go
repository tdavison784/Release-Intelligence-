// Per-release Helm values. Values belong to a release — a deployment target:
// an Argo CD Application's Helm source, a Flux HelmRelease, a helmfile
// release, a repo values file no release references, or the explicit
// --values files. Each release gets its own ordered values stack, applied
// exactly as the deploying tool applies it, and its own effective values with
// per-key provenance (the file or block, and line, of the layer that set the
// effective value). Stacks are never merged across releases: two clusters'
// releases of one product are two configurations, and a value that holds for
// one is never assumed for the other.
//
// Helm merge semantics (helm.sh/helm/v3/pkg/cli/values mergeMaps, then the
// chart coalesce): mappings deep-merge; lists and scalars replace the whole
// subtree at their path; an empty mapping {} merges nothing and so deletes
// nothing (Helm keeps the keys); null replaces the subtree and deletes the
// key (it also deletes the chart default at render time — the null leaf is
// kept as the fact "the customer unsets this key").

package env

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// Release kinds (ValuesRelease.Kind), named like the render targets.
const (
	ReleaseArgoCD      = "argocd"
	ReleaseFlux        = "flux"
	ReleaseHelmfile    = "helmfile"
	ReleaseValuesFiles = "values-files" // a repo values file no release references, or the explicit --values files
)

// Layer kinds (ValuesLayer.Kind).
const (
	LayerFile       = "file"        // a values file the release applies (-f)
	LayerInline     = "inline"      // values embedded in the release definition
	LayerSet        = "set"         // --set style entries (Argo parameters, helmfile set, Flux targetPath)
	LayerValuesFrom = "values-from" // a Flux valuesFrom object
	LayerExplicit   = "explicit"    // an explicit --values file given to ri (applied last: it wins per key)
)

// ValuesLayer is one entry of a release's values stack, in application order
// (lowest precedence first).
type ValuesLayer struct {
	Kind   string
	Source string // file path, or file + locator of an inline block / reference
	Line   int    // line of the reference in the release definition (0: none)
	// Applied: the layer was read and merged. Gap: the stack is incomplete
	// because of this layer (not readable, partly parsed, not evaluated).
	// Reason says why (also for a layer the tool itself ignores).
	Applied bool
	Gap     bool
	Reason  string
}

// ValuesRelease is one deployment target's values: the ordered stack and the
// effective result. Health is ok only when the whole stack was read.
type ValuesRelease struct {
	ID     string // "argocd:<file>#<doc>/$.spec.source", "flux:<file>#<doc>", "helmfile:<file>/$.releases[i]", "values:<files>"
	Kind   string
	Name   string // Application / HelmRelease / helmfile release name ("" for a values-files group)
	Chart  string // chart as stated (an Argo git source: its path)
	Origin string // the file declaring the release
	Layers []ValuesLayer
	// Keys are the effective values, flattened like ValuesKeys (sorted by
	// path), each citing the layer that set it.
	Keys     []ValuesKey
	Health   Health
	Reasons  []string
	Evidence []domain.EvidenceID
}

// ValuesScope records where Environment.ValuesKeys come from.
type ValuesScope struct {
	// Product is Inputs.Product ("" = every release of the inputs).
	Product string
	// Releases are the ids of the releases whose effective values are in
	// ValuesKeys (ValuesKey.Release), one per distinct configuration;
	// Equivalent lists further releases of the product whose effective values
	// are identical to one of them.
	Releases   []string
	Equivalent []string
	// Unattributed are values files no release references whose name does
	// not attribute them to exactly one release; their keys are in ValuesKeys
	// per file (ValuesKey.Release ""), and the values dimension is partial.
	Unattributed []string
	// Ambiguous: several releases deploy the product with different
	// effective values (the values dimension is partial).
	Ambiguous bool
}

// --- collection -------------------------------------------------------------------

// parsedValues is one values file, parsed once.
type parsedValues struct {
	entries []ValuesKey
	problem string // non-empty: not (fully) read
}

// parseValuesFile reads and flattens a values file (cached). Images set
// through the usual chart conventions are extracted on first parse.
func (l *loader) parseValuesFile(path string) *parsedValues {
	if p, ok := l.valuesCache[path]; ok {
		return p
	}
	p := &parsedValues{}
	l.valuesCache[path] = p
	b, err := l.readFile(path)
	if err != nil {
		p.problem = fmt.Sprintf("%s could not be read: %v", path, err)
		return p
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return p
	}
	flat, err := normalize.FlattenValues(b)
	if err != nil {
		p.problem = fmt.Sprintf("%s was not parsed as a Helm values mapping: %v", path, err)
		return p
	}
	// yaml.Unmarshal decodes only a stream's FIRST document and silently
	// drops the rest; a Helm values file must be a single document, so make
	// any extra ones an explicit, dimension-degrading problem.
	if n := countDocs(b); n > 1 {
		p.problem = fmt.Sprintf("%s holds %d YAML documents; a Helm values file is one document — the rest was ignored", path, n)
	}
	if len(flat) > maxValuesKeys {
		p.problem = fmt.Sprintf("more than %d values keys; the rest of %s was ignored", maxValuesKeys, path)
		flat = flat[:maxValuesKeys]
	}
	for _, kv := range flat {
		p.entries = append(p.entries, ValuesKey{
			Path: kv.Path, Value: kv.Value, Line: kv.Line,
			Evidence: []domain.EvidenceID{l.ev(path, fmt.Sprintf("$.%s (L%d)", kv.Path, kv.Line), kv.Path+": "+kv.Value)},
		})
	}
	l.valuesImages(path, flat)
	return p
}

// valuesImages extracts the images a values file sets through the usual chart
// conventions (repository[+tag], hub, image).
func (l *loader) valuesImages(f string, flat []normalize.FlattenedValue) {
	byPath := make(map[string]normalize.FlattenedValue, len(flat))
	for _, kv := range flat {
		byPath[kv.Path] = kv
	}
	for _, kv := range flat {
		base := lastSegment(kv.Path)
		if base != "repository" && base != "hub" && base != "image" {
			continue
		}
		ref := strings.Trim(strings.Trim(kv.Value, `"`), `'`)
		if !strings.Contains(ref, "/") {
			continue // a bare name or a non-reference value
		}
		tag := ""
		if base != "image" {
			if sib, ok := byPath[siblingPath(kv.Path, "tag")]; ok {
				tag = strings.Trim(strings.Trim(sib.Value, `"`), `'`)
			}
		}
		full := ref
		if tag != "" && base != "image" {
			full = ref + ":" + tag
		}
		if _, err := normalize.ParseImageRef(full); err != nil {
			continue
		}
		l.addImageAt(full, "values", f, kv.Line)
	}
}

// stackLayer is a layer with its flattened entries, ready to merge.
type stackLayer struct {
	info    ValuesLayer
	entries []ValuesKey
}

// fileLayer is a values-file layer read from disk.
func (l *loader) fileLayer(kind, path string, line int) stackLayer {
	if fi, err := os.Stat(path); err != nil || fi.IsDir() {
		return gapLayer(kind, path, line, fmt.Sprintf("values file %s is not present", path))
	}
	p := l.parseValuesFile(path)
	s := stackLayer{info: ValuesLayer{Kind: kind, Source: path, Line: line, Applied: true}, entries: p.entries}
	if p.problem != "" {
		s.info.Gap, s.info.Reason = true, p.problem
		if len(p.entries) == 0 {
			s.info.Applied = false
		}
	}
	return s
}

func gapLayer(kind, source string, line int, reason string) stackLayer {
	return stackLayer{info: ValuesLayer{Kind: kind, Source: source, Line: line, Gap: true, Reason: reason}}
}

// inlineLayer flattens an inline values mapping (or a string holding YAML)
// with the path syntax and value encoding of a parsed values file. Lines come
// from the original nodes where possible and from the inline node itself
// otherwise.
func (l *loader) inlineLayer(file, locatorPrefix string, node *yaml.Node) stackLayer {
	node = resolveAlias(node)
	s := stackLayer{info: ValuesLayer{Kind: LayerInline, Source: file + "#" + locatorPrefix, Applied: true}}
	if node == nil {
		return s
	}
	s.info.Line = node.Line
	var flat []normalize.FlattenedValue
	var lines map[string]int
	var err error
	switch {
	case node.Kind == yaml.MappingNode:
		var b []byte
		if b, err = yaml.Marshal(node); err == nil {
			flat, err = normalize.FlattenValues(b)
		}
		lines = map[string]int{}
		walkFieldPaths(node, "", func(p string, v *yaml.Node) {
			if v != nil {
				lines[p] = v.Line
			}
		})
	case node.Kind == yaml.ScalarNode && node.Tag != "!!null":
		flat, err = normalize.FlattenValues([]byte(node.Value))
	case node.Kind == yaml.ScalarNode:
		return s // null: nothing set
	default:
		err = fmt.Errorf("a %s is not a values mapping", kindName(node.Kind))
	}
	if err != nil {
		s.info.Applied, s.info.Gap = false, true
		s.info.Reason = fmt.Sprintf("%s: inline values at L%d were not parsed as a values mapping: %v", file, node.Line, err)
		return s
	}
	for _, kv := range flat {
		line := node.Line
		if ln, ok := lines[kv.Path]; ok && ln > 0 {
			line = ln
		}
		s.entries = append(s.entries, ValuesKey{
			Path: kv.Path, Value: kv.Value, Line: line,
			Evidence: []domain.EvidenceID{l.ev(file, fmt.Sprintf("%s.%s (L%d)", locatorPrefix, kv.Path, line), kv.Path+": "+kv.Value)},
		})
	}
	return s
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	}
	return "scalar"
}

// setEntry is one --set style assignment.
type setEntry struct {
	name, value string
	forceString bool
	line        int
	locator     string // evidence locator
}

// setLayer applies --set style assignments with Helm's strvals semantics:
// name is a dotted path (\. escapes a dot), the value is typed like --set
// (true/false/null, integers without a leading zero; everything else a
// string) unless forceString (--set-string). Assignments Helm would read as a
// list ({a,b}) or that address a list element (a[0]) are not evaluated: the
// layer is a gap.
func (l *loader) setLayer(file, source string, line int, sets []setEntry) stackLayer {
	s := stackLayer{info: ValuesLayer{Kind: LayerSet, Source: file + "#" + source, Line: line, Applied: true}}
	var skipped []string
	for _, e := range sets {
		segs, ok := parseSetName(e.name)
		v := strings.TrimSpace(e.value)
		if !ok || (!e.forceString && strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}")) {
			skipped = append(skipped, e.name)
			continue
		}
		path := joinKeySegments(segs)
		enc := SetValueJSON(e.value, e.forceString)
		s.entries = append(s.entries, ValuesKey{
			Path: path, Value: enc, Line: e.line,
			Evidence: []domain.EvidenceID{l.ev(file, fmt.Sprintf("%s (L%d)", e.locator, e.line), path+": "+enc)},
		})
	}
	if len(skipped) > 0 {
		s.info.Gap = true
		s.info.Reason = fmt.Sprintf("%s: --set style entries %s use list syntax (an index or {a,b}) that is not evaluated", file, strings.Join(skipped, ", "))
	}
	return s
}

// parseSetName splits a strvals key ("a.b\.c" → [a, b.c]); false when it
// addresses a list element or is empty.
func parseSetName(name string) ([]string, bool) {
	var segs []string
	var cur strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '\\' && i+1 < len(name):
			i++
			cur.WriteByte(name[i])
		case c == '.':
			segs = append(segs, cur.String())
			cur.Reset()
		case c == '[' || c == ']':
			return nil, false
		default:
			cur.WriteByte(c)
		}
	}
	segs = append(segs, cur.String())
	for _, s := range segs {
		if s == "" {
			return nil, false
		}
	}
	return segs, true
}

// joinKeySegments builds a values path with normalize.FlattenValues' syntax
// (dotted, ["quoted.key"] escaping).
func joinKeySegments(segs []string) string {
	p := ""
	for _, s := range segs {
		p = joinPath(p, s)
	}
	return p
}

// SetValueJSON encodes a --set value the way Helm's strvals types it
// (typedVal): true/false/null case-insensitively, "0", integers that do not
// start with 0; anything else (floats included) is a string.
func SetValueJSON(v string, forceString bool) string {
	if !forceString {
		switch {
		case strings.EqualFold(v, "true"):
			return "true"
		case strings.EqualFold(v, "false"):
			return "false"
		case strings.EqualFold(v, "null"):
			return "null"
		case v == "0":
			return "0"
		case v != "" && v[0] != '0':
			if iv, err := strconv.ParseInt(v, 10, 64); err == nil {
				return strconv.FormatInt(iv, 10)
			}
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimRight(buf.String(), "\n")
}

// --- merge --------------------------------------------------------------------------

// vnode is one node of an effective values tree: a mapping (kids != nil) or
// a leaf. An empty mapping set explicitly ({}) keeps its provenance in leaf.
type vnode struct {
	kids map[string]*vnode
	leaf *ValuesKey
}

// apply merges one flattened entry (a leaf of its layer) the way Helm merges
// a later values source over an earlier one.
func (n *vnode) apply(k ValuesKey) {
	segs := SplitKeyPath(k.Path)
	if len(segs) == 0 {
		return
	}
	cur := n
	for i, s := range segs {
		if i == len(segs)-1 {
			kk := k
			if k.Value == "{}" { // an empty mapping
				if old := cur.kids[s]; old != nil && old.kids != nil {
					return // merging {} into a mapping changes nothing: Helm keeps the keys
				}
				cur.kids[s] = &vnode{kids: map[string]*vnode{}, leaf: &kk}
				return
			}
			cur.kids[s] = &vnode{leaf: &kk} // scalars, lists and null replace the subtree
			return
		}
		next := cur.kids[s]
		if next == nil || next.kids == nil {
			next = &vnode{kids: map[string]*vnode{}} // a mapping replaces a leaf
			cur.kids[s] = next
		}
		next.leaf = nil // a mapping with content is no longer the {} marker
		cur = next
	}
}

func (n *vnode) flatten(out *[]ValuesKey) {
	if n.kids == nil || len(n.kids) == 0 {
		if n.leaf != nil {
			*out = append(*out, *n.leaf)
		}
		return
	}
	for _, c := range n.kids {
		c.flatten(out)
	}
}

// mergeLayers computes the effective values of a stack, each key citing the
// layer that set it, sorted by path.
func mergeLayers(release string, layers []stackLayer) []ValuesKey {
	root := &vnode{kids: map[string]*vnode{}}
	for _, ly := range layers {
		for _, e := range ly.entries {
			root.apply(e)
		}
	}
	var out []ValuesKey
	for _, c := range root.kids {
		c.flatten(&out)
	}
	for i := range out {
		out[i].Release = release
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// --- releases -----------------------------------------------------------------------

// release is a ValuesRelease under construction.
type release struct {
	rel    ValuesRelease
	layers []stackLayer
	ident  []string // names the release is known by (release name, chart base)
}

func (r *release) add(ls ...stackLayer) { r.layers = append(r.layers, ls...) }

// finish merges the stack and states the release's health.
func (r *release) finish() ValuesRelease {
	out := r.rel
	out.Layers = nil
	out.Reasons = nil
	for _, ly := range r.layers {
		out.Layers = append(out.Layers, ly.info)
		if ly.info.Gap {
			out.Reasons = append(out.Reasons, ly.info.Reason)
		}
	}
	out.Keys = mergeLayers(out.ID, r.layers)
	out.Health = HealthOK
	if len(out.Reasons) > 0 {
		out.Health = HealthPartial
	}
	return out
}

// label names a release in warnings.
func (r ValuesRelease) label() string {
	switch r.Kind {
	case ReleaseArgoCD:
		return fmt.Sprintf("Argo CD Application %s (%s)", r.Name, r.Origin)
	case ReleaseFlux:
		return fmt.Sprintf("Flux HelmRelease %s (%s)", r.Name, r.Origin)
	case ReleaseHelmfile:
		return fmt.Sprintf("helmfile release %s (%s)", r.Name, r.Origin)
	}
	return "values files " + r.Origin
}

// relTo is path relative to the repo root (path itself outside repo mode).
func (l *loader) relTo(path string) string {
	if l.env.RepoRoot == "" {
		return path
	}
	if r, err := filepath.Rel(l.env.RepoRoot, path); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return path
}

// inRepo joins a repo-relative path, refusing one that escapes the root.
func (l *loader) inRepo(parts ...string) (string, bool) {
	if l.env.RepoRoot == "" {
		return "", false
	}
	p := filepath.Join(append([]string{l.env.RepoRoot}, parts...)...)
	r, err := filepath.Rel(l.env.RepoRoot, p)
	if err != nil || r == ".." || strings.HasPrefix(r, "../") {
		return "", false
	}
	return p, true
}

// referenceValuesFile records a values file a release definition references
// (the repo walk classifies it as values, citing the reference).
func (l *loader) referenceValuesFile(target, defFile string, line int, ref string) {
	if l.env.RepoRoot == "" {
		return
	}
	l.env.Discovered = append(l.env.Discovered, RepoDiscovery{
		Path: target, Kind: DiscValues,
		Detail:   fmt.Sprintf("referenced by %s (L%d)", l.relTo(defFile), line),
		Evidence: []domain.EvidenceID{l.ev(defFile, fmt.Sprintf("L%d", line), "valueFiles: "+ref)},
	})
}

// argoReleases builds one release per Helm source of an Argo CD Application:
// helm.valueFiles in declared order, then helm.valuesObject — or helm.values
// when no valuesObject is set — then helm.parameters (Argo CD "Helm Value
// Precedence": parameters > valuesObject > values > valueFiles; "valuesObject
// trumps values - therefore values will be ignored").
func (l *loader) argoReleases(d doc) []*release {
	app := scalarOf(d.node, "metadata", "name")
	var srcs []*yaml.Node
	var locs []string
	if s := fieldOf(d.node, "spec", "source"); s != nil && resolveAlias(s).Kind == yaml.MappingNode {
		srcs, locs = append(srcs, s), append(locs, "spec.source")
	}
	for i, s := range itemsOf(fieldOf(d.node, "spec", "sources")) {
		srcs, locs = append(srcs, s), append(locs, fmt.Sprintf("spec.sources[%d]", i))
	}
	refs := map[string]bool{}
	for _, s := range srcs {
		if r := scalarOf(s, "ref"); r != "" {
			refs[r] = true
		}
	}
	var out []*release
	for i, s := range srcs {
		chart, path := scalarOf(s, "chart"), scalarOf(s, "path")
		helm := fieldOf(s, "helm")
		if chart == "" && path == "" {
			continue // a values-only (ref) source deploys nothing; Argo CD renders no chart from it
		}
		identity := chart
		if chart == "" {
			// a git source is a Helm chart when it carries Helm settings or the
			// directory (in this repository) holds a Chart.yaml
			dir, ok := l.inRepo(path)
			isChart := false
			if ok {
				_, err := os.Stat(filepath.Join(dir, "Chart.yaml"))
				isChart = err == nil
			}
			if helm == nil && !isChart {
				continue // a directory / kustomize source: no Helm values
			}
			identity = path
		}
		_, sline := scalarLine(s, "repoURL")
		if sline == 0 {
			sline = resolveAlias(s).Line
		}
		r := &release{rel: ValuesRelease{
			ID: fmt.Sprintf("argocd:%s#%d/$.%s", d.file, d.index, locs[i]), Kind: ReleaseArgoCD,
			Name: app, Chart: identity, Origin: d.file,
			Evidence: []domain.EvidenceID{l.ev(d.file, fmt.Sprintf("$.%s (L%d)", locs[i], sline), fmt.Sprintf("Argo CD Application %s source %s", app, identity))},
		}, ident: []string{app, chartBase(identity)}}
		if rn := scalarOf(helm, "releaseName"); rn != "" {
			r.ident = append(r.ident, rn)
		}
		prefix := "$." + locs[i] + ".helm"
		ignoreMissing := scalarOf(helm, "ignoreMissingValueFiles") == "true"
		for _, vf := range itemsOf(fieldOf(helm, "valueFiles")) {
			if vf.Kind != yaml.ScalarNode || strings.TrimSpace(vf.Value) == "" {
				continue
			}
			r.add(l.argoValueFile(d.file, vf.Value, vf.Line, path, refs, ignoreMissing))
		}
		vo, vs := fieldOf(helm, "valuesObject"), fieldOf(helm, "values")
		switch {
		case vo != nil && !isNull(vo):
			r.add(l.inlineLayer(d.file, prefix+".valuesObject", vo))
			if vs != nil && !isNull(vs) {
				r.add(stackLayer{info: ValuesLayer{Kind: LayerInline, Source: d.file + "#" + prefix + ".values", Line: resolveAlias(vs).Line,
					Reason: "ignored by Argo CD: valuesObject is set (valuesObject trumps values)"}})
			}
		case vs != nil && !isNull(vs):
			r.add(l.inlineLayer(d.file, prefix+".values", vs))
		}
		if ps := itemsOf(fieldOf(helm, "parameters")); len(ps) > 0 {
			var sets []setEntry
			for _, p := range ps {
				name := scalarOf(p, "name")
				if name == "" {
					continue
				}
				sets = append(sets, setEntry{name: name, value: scalarOf(p, "value"), forceString: scalarOf(p, "forceString") == "true",
					line: resolveAlias(p).Line, locator: fmt.Sprintf("%s.parameters[%s]", prefix, name)})
			}
			r.add(l.setLayer(d.file, prefix+".parameters", resolveAlias(fieldOf(helm, "parameters")).Line, sets))
		}
		if fp := fieldOf(helm, "fileParameters"); mappingOrSequence(fp) != nil {
			r.add(gapLayer(LayerSet, d.file+"#"+prefix+".fileParameters", resolveAlias(fp).Line,
				fmt.Sprintf("%s: helm.fileParameters (--set-file) are not evaluated", d.file)))
		}
		out = append(out, r)
	}
	return out
}

// argoValueFile resolves one helm.valueFiles entry: "$<ref>/path" against the
// repository root (the ref source is taken to be this repository, as the
// renderer does), a plain path relative to the chart root — readable only
// for a git source whose path lies in this repository.
func (l *loader) argoValueFile(appFile, entry string, line int, srcPath string, refs map[string]bool, ignoreMissing bool) stackLayer {
	src := fmt.Sprintf("%s (valueFiles L%d: %s)", appFile, line, entry)
	if strings.Contains(entry, "://") {
		return gapLayer(LayerFile, src, line, fmt.Sprintf("%s L%d: valueFiles %q is a remote URL, not readable from files", appFile, line, entry))
	}
	var target string
	var ok bool
	if strings.HasPrefix(entry, "$") {
		ref, rest, _ := strings.Cut(entry[1:], "/")
		if !refs[ref] {
			return gapLayer(LayerFile, src, line, fmt.Sprintf("%s L%d: valueFiles %q names $%s, which no source of the Application declares as ref", appFile, line, entry, ref))
		}
		if target, ok = l.inRepo(rest); !ok {
			return gapLayer(LayerFile, src, line, fmt.Sprintf("%s L%d: valueFiles %q is not readable from this repository", appFile, line, entry))
		}
	} else {
		if srcPath == "" {
			return gapLayer(LayerFile, src, line, fmt.Sprintf("%s L%d: valueFiles %q resolves inside the chart package (relative to the chart root), not from this repository", appFile, line, entry))
		}
		if target, ok = l.inRepo(srcPath, entry); !ok {
			return gapLayer(LayerFile, src, line, fmt.Sprintf("%s L%d: valueFiles %q is not readable from this repository", appFile, line, entry))
		}
	}
	if fi, err := os.Stat(target); err != nil || fi.IsDir() {
		reason := fmt.Sprintf("%s L%d: valueFiles %q (%s) is not present in the repository", appFile, line, entry, target)
		if ignoreMissing {
			reason += " (ignoreMissingValueFiles: Argo CD skips a missing file, but its absence here does not show its absence in the revision Argo CD deploys)"
		}
		return gapLayer(LayerFile, src, line, reason)
	}
	l.referenceValuesFile(target, appFile, line, entry)
	return l.fileLayer(LayerFile, target, line)
}

// fluxReleases builds the release of a Flux HelmRelease: valuesFrom entries
// in order, then spec.values, then valuesFrom entries with a targetPath
// (Flux: "The values are merged in the order given, with the later values
// overwriting earlier"; inline values overwrite them; "When targetPath is
// set, it will overwrite everything before, including inline values").
func (l *loader) fluxReleases(d doc) []*release {
	name := scalarOf(d.node, "metadata", "name")
	ns := scalarOf(d.node, "metadata", "namespace")
	chart := scalarOf(d.node, "spec", "chart", "spec", "chart")
	identity := chart
	if identity == "" {
		identity = name // spec.chartRef: the chart is named by another object
	}
	r := &release{rel: ValuesRelease{
		ID: fmt.Sprintf("flux:%s#%d", d.file, d.index), Kind: ReleaseFlux, Name: name, Chart: chart, Origin: d.file,
		Evidence: []domain.EvidenceID{l.ev(d.file, fmt.Sprintf("L%d", d.startLine), "HelmRelease "+name)},
	}, ident: []string{name, chartBase(identity)}}
	if rn := scalarOf(d.node, "spec", "releaseName"); rn != "" {
		r.ident = append(r.ident, rn)
	}
	for _, key := range []string{"valuesFiles", "valuesFile"} {
		if v := fieldOf(d.node, "spec", "chart", "spec", key); v != nil && !isNull(v) {
			r.add(gapLayer(LayerFile, fmt.Sprintf("%s#$.spec.chart.spec.%s", d.file, key), resolveAlias(v).Line,
				fmt.Sprintf("HelmRelease %s: spec.chart.spec.%s are files inside the chart source, not readable from this repository", name, key)))
		}
	}
	var late []stackLayer
	for i, vf := range itemsOf(fieldOf(d.node, "spec", "valuesFrom")) {
		ly := l.fluxValuesFrom(d, name, ns, i, vf)
		if scalarOf(vf, "targetPath") != "" {
			late = append(late, ly)
			continue
		}
		r.add(ly)
	}
	if v := fieldOf(d.node, "spec", "values"); v != nil && !isNull(v) {
		r.add(l.inlineLayer(d.file, "$.spec.values", v))
	}
	r.add(late...)
	return []*release{r}
}

// fluxValuesFrom reads one valuesFrom reference from a ConfigMap the inputs
// define (same name, the HelmRelease's namespace); a Secret, or an object no
// input defines, lives only in the cluster: a gap.
func (l *loader) fluxValuesFrom(d doc, hr, ns string, i int, vf *yaml.Node) stackLayer {
	kind, name := scalarOf(vf, "kind"), scalarOf(vf, "name")
	key := scalarOf(vf, "valuesKey")
	if key == "" {
		key = "values.yaml"
	}
	line := resolveAlias(vf).Line
	src := fmt.Sprintf("%s#$.spec.valuesFrom[%d] (%s %s)", d.file, i, kind, name)
	gap := func(why string) stackLayer {
		return stackLayer{info: ValuesLayer{Kind: LayerValuesFrom, Source: src, Line: line, Gap: true,
			Reason: fmt.Sprintf("HelmRelease %s: spec.valuesFrom %s %s %s", hr, kind, name, why)}}
	}
	if kind != "ConfigMap" {
		return gap("references a cluster object; its values are not read from files (Secret values are never read)")
	}
	var found []doc
	for _, c := range l.cfgDocs {
		cns := scalarOf(c.node, "metadata", "namespace")
		if scalarOf(c.node, "metadata", "name") == name && (cns == ns || cns == "" || ns == "") {
			found = append(found, c)
		}
	}
	switch {
	case len(found) == 0:
		return gap("references a cluster object no input defines; those values are not readable from files")
	case len(found) > 1:
		return gap(fmt.Sprintf("matches %d ConfigMaps in the inputs; which one the cluster holds is unknown", len(found)))
	}
	c := found[0]
	val := fieldOf(c.node, "data", key)
	if val == nil || resolveAlias(val).Kind != yaml.ScalarNode {
		return gap(fmt.Sprintf("(%s L%d) has no data key %q", c.file, c.startLine, key))
	}
	val = resolveAlias(val)
	locator := fmt.Sprintf("$.data[%s]", key)
	if tp := scalarOf(vf, "targetPath"); tp != "" {
		s := l.setLayer(c.file, locator, val.Line, []setEntry{{name: tp, value: val.Value, forceString: scalarOf(vf, "literal") == "true",
			line: val.Line, locator: fmt.Sprintf("%s → targetPath %s", locator, tp)}})
		s.info.Kind, s.info.Source = LayerValuesFrom, src
		return s
	}
	s := l.inlineLayer(c.file, locator, val)
	s.info.Kind, s.info.Source = LayerValuesFrom, src
	return s
}

// helmfileReleases builds one release per helmfile release: values entries
// in order (files relative to the helmfile, inline mappings), then set. A
// templated helmfile, a template values entry or secrets leave the stack
// incomplete.
func (l *loader) helmfileReleases(h helmfileDoc) []*release {
	var out []*release
	for ri, rn := range itemsOf(fieldOf(h.node, "releases")) {
		chart := scalarOf(rn, "chart")
		if chart == "" {
			continue
		}
		name := scalarOf(rn, "name")
		loc := fmt.Sprintf("$.releases[%d]", ri)
		r := &release{rel: ValuesRelease{
			ID: fmt.Sprintf("helmfile:%s/%s", h.path, loc), Kind: ReleaseHelmfile, Name: name, Chart: chart, Origin: h.path,
			Evidence: []domain.EvidenceID{l.ev(h.path, fmt.Sprintf("%s (L%d)", loc, rn.Line), "chart: "+chart)},
		}, ident: []string{name, chartBase(chart)}}
		if h.templated {
			r.add(gapLayer(LayerFile, h.path, 0, fmt.Sprintf("helmfile %s is templated ({{ … }}); values may depend on the helmfile environment", h.rel)))
		}
		for vi, v := range itemsOf(fieldOf(rn, "values")) {
			v = resolveAlias(v)
			switch v.Kind {
			case yaml.ScalarNode:
				if strings.HasSuffix(v.Value, ".gotmpl") || strings.Contains(v.Value, "{{") {
					r.add(gapLayer(LayerFile, v.Value, v.Line, fmt.Sprintf("helmfile %s L%d: values entry %s is a template, not evaluated", h.rel, v.Line, v.Value)))
					continue
				}
				r.add(l.fileLayer(LayerFile, filepath.Join(filepath.Dir(h.path), v.Value), v.Line))
			case yaml.MappingNode:
				r.add(l.inlineLayer(h.path, fmt.Sprintf("%s.values[%d]", loc, vi), v))
			}
		}
		if s := fieldOf(rn, "secrets"); mappingOrSequence(s) != nil {
			r.add(gapLayer(LayerFile, h.path+"#"+loc+".secrets", resolveAlias(s).Line, fmt.Sprintf("helmfile %s: release %s secrets are encrypted and not read", h.rel, name)))
		}
		if sets := itemsOf(fieldOf(rn, "set")); len(sets) > 0 {
			var es []setEntry
			for _, s := range sets {
				n := scalarOf(s, "name")
				if n == "" {
					continue
				}
				es = append(es, setEntry{name: n, value: scalarOf(s, "value"), line: resolveAlias(s).Line, locator: fmt.Sprintf("%s.set[%s]", loc, n)})
			}
			r.add(l.setLayer(h.path, loc+".set", resolveAlias(fieldOf(rn, "set")).Line, es))
		}
		out = append(out, r)
	}
	return out
}

func isNull(n *yaml.Node) bool {
	n = resolveAlias(n)
	return n == nil || (n.Kind == yaml.ScalarNode && n.Tag == "!!null")
}

// --- scoping ------------------------------------------------------------------------

// productIdentity is the names a product's releases are recognised by.
func productIdentity(product string, hints []ProductHint) []string {
	if product == "" {
		return nil
	}
	ids := []string{product}
	for _, h := range hints {
		if normalizeProductID(h.ID) == normalizeProductID(product) {
			ids = append(ids, h.Charts...)
		}
	}
	return ids
}

// deploys reports whether a release installs one of the product's charts.
func (r *release) deploys(ids []string) bool {
	base := chartBase(r.rel.Chart)
	if r.rel.Chart == "" && r.rel.Kind == ReleaseFlux {
		base = r.rel.Name // chartRef: only the release name can tell
	}
	for _, id := range ids {
		if id != "" && strings.EqualFold(base, id) {
			return true
		}
	}
	return false
}

// attribution of an unreferenced values file by its name.
type attribution int

const (
	attrNone    attribution = iota // names nothing (or several): unattributable
	attrProduct                    // names the product (or one of its releases)
	attrOther                      // names exactly one other release / product
)

// attribute decides by name which release an unreferenced values file
// configures: the file stem and its directory are matched against the names
// of every release and catalog product (a name contained in a longer matched
// name counts once).
func attribute(file string, product []string, releases []*release, hints []ProductHint) attribution {
	stem := strings.ToLower(strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)))
	dir := strings.ToLower(filepath.Base(filepath.Dir(file)))
	type cand struct {
		name string
		own  bool
	}
	var cands []cand
	addC := func(n string, own bool) {
		n = strings.ToLower(strings.TrimSpace(n))
		if len(n) >= 3 {
			cands = append(cands, cand{n, own})
		}
	}
	for _, p := range product {
		addC(p, true)
	}
	for _, r := range releases {
		own := r.deploys(product)
		for _, n := range r.ident {
			addC(n, own)
		}
	}
	for _, h := range hints {
		own := false
		for _, p := range product {
			own = own || normalizeProductID(h.ID) == normalizeProductID(p)
		}
		addC(h.ID, own)
		for _, c := range h.Charts {
			addC(c, own)
		}
	}
	var hit []cand
	for _, c := range cands {
		if strings.Contains(stem, c.name) || strings.Contains(dir, c.name) {
			hit = append(hit, c)
		}
	}
	// drop names contained in a longer hit ("nginx" inside "ingress-nginx")
	targets := map[string]bool{} // "own" or the other name
	for _, c := range hit {
		shadowed := false
		for _, o := range hit {
			if len(o.name) > len(c.name) && strings.Contains(o.name, c.name) {
				shadowed = true
			}
		}
		if shadowed {
			continue
		}
		if c.own {
			targets["\x00own"] = true
		} else {
			targets[c.name] = true
		}
	}
	switch {
	case len(targets) != 1:
		return attrNone
	case targets["\x00own"]:
		return attrProduct
	}
	return attrOther
}

// loadValues builds every release's values stack, then scopes ValuesKeys
// and the values dimension to the product's release(s) (Inputs.Product; every
// release when it is empty). discovered are the repo values files the walk
// found; explicit the expanded --values files, applied last on top of each
// selected stack (an explicit values file wins per key).
func (l *loader) loadValues(product string, hints []ProductHint, discovered, explicit []string) {
	var rels []*release
	for _, h := range l.helmfiles {
		rels = append(rels, l.helmfileReleases(h)...)
	}
	for _, d := range l.argoDocs {
		rels = append(rels, l.argoReleases(d)...)
	}
	for _, d := range l.fluxDocs {
		rels = append(rels, l.fluxReleases(d)...)
	}
	referenced := map[string]bool{}
	for _, r := range rels {
		for _, ly := range r.layers {
			if ly.info.Kind == LayerFile && ly.info.Applied {
				referenced[filepath.Clean(ly.info.Source)] = true
			}
		}
	}
	isExplicit := map[string]bool{}
	for _, f := range explicit {
		isExplicit[filepath.Clean(f)] = true
	}
	ids := productIdentity(product, hints)
	var selected []*release
	var unattributed []string
	for _, r := range rels {
		if product == "" || r.deploys(ids) {
			selected = append(selected, r)
		}
	}
	// repo values files no release references: a values-files group of their
	// own (as the renderer renders them) when the name attributes them to the
	// product, nothing of the product's when it names one other release, and a
	// gap otherwise
	seenFile := map[string]bool{}
	var groups []*release
	for _, f := range discovered {
		c := filepath.Clean(f)
		if referenced[c] || isExplicit[c] || seenFile[c] {
			continue
		}
		seenFile[c] = true
		g := &release{rel: ValuesRelease{ID: "values:" + f, Kind: ReleaseValuesFiles, Origin: f,
			Evidence: []domain.EvidenceID{l.fileEvidence(f)}}}
		g.add(l.fileLayer(LayerFile, f, 0))
		groups = append(groups, g)
		switch {
		case product == "":
			selected = append(selected, g)
		default:
			switch attribute(f, ids, rels, hints) {
			case attrProduct:
				selected = append(selected, g)
			case attrNone:
				unattributed = append(unattributed, f)
			}
		}
	}
	if len(explicit) > 0 {
		var ex []stackLayer
		for _, f := range explicit {
			ex = append(ex, l.fileLayer(LayerExplicit, f, 0))
		}
		if len(selected) == 0 {
			g := &release{rel: ValuesRelease{ID: "values:" + strings.Join(explicit, ","), Kind: ReleaseValuesFiles, Origin: strings.Join(explicit, ",")}}
			g.add(ex...)
			selected = append(selected, g)
			groups = append(groups, g)
		} else {
			for _, r := range selected {
				r.add(ex...)
			}
		}
	}

	// every release is recorded (with its own effective values and health);
	// only the selected ones decide the values dimension
	done := map[*release]ValuesRelease{}
	for _, r := range append(append([]*release{}, rels...), groups...) {
		v := r.finish()
		done[r] = v
		l.env.ValuesReleases = append(l.env.ValuesReleases, v)
	}
	scope := &ValuesScope{Product: product}
	l.env.ValuesScope = scope
	var distinct []ValuesRelease
	for _, r := range selected {
		v := done[r]
		for _, why := range v.Reasons {
			l.warnf(DimValues, "values of %s: %s", v.label(), why)
		}
		dup := false
		for _, o := range distinct {
			if sameValues(o.Keys, v.Keys) {
				dup = true
				break
			}
		}
		if dup {
			scope.Equivalent = append(scope.Equivalent, v.ID)
			continue
		}
		distinct = append(distinct, v)
		scope.Releases = append(scope.Releases, v.ID)
	}
	if len(distinct) > 1 {
		scope.Ambiguous = true
		var names []string
		for _, v := range distinct {
			names = append(names, v.label())
		}
		what := "the product"
		if product != "" {
			what = product
		}
		l.warnf(DimValues, "%d releases deploy %s with different effective values (%s): values are per release and never merged across them — a value that holds for one release is not assumed for another",
			len(distinct), what, strings.Join(names, "; "))
	}
	for _, v := range distinct {
		for _, k := range v.Keys {
			if !l.addValuesKey(k) {
				l.warnf(DimValues, "more than %d values keys; the rest of %s was ignored", maxValuesKeys, v.label())
				break
			}
		}
	}
	for _, f := range unattributed {
		scope.Unattributed = append(scope.Unattributed, f)
		what := "this product"
		if product != "" {
			what = product
		}
		l.warnf(DimValues, "values file %s is not referenced by any release and its name does not attribute it to exactly one release; it may configure %s, so the values are partial",
			f, what)
		for _, k := range l.parseValuesFile(f).entries {
			if !l.addValuesKey(k) {
				break
			}
		}
	}
	l.env.Supplied.Values = len(selected) > 0 || len(unattributed) > 0
}

// sameValues reports whether two effective value sets are identical (path
// and value; provenance aside).
func sameValues(a, b []ValuesKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path != b[i].Path || a[i].Value != b[i].Value {
			return false
		}
	}
	return true
}
