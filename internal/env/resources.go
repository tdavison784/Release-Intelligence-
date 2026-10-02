// Resource facts: what each manifest resource actually states, field by field,
// as the facts the deterministic applicability engine evaluates verified
// release knowledge against.
//
//   - Field facts keep the scalar VALUE of each leaf (encoded exactly like
//     ValuesKey.Value) and descend into sequences with "[]" markers, the
//     syntax of CRD SchemaPaths ("spec.acme.solvers[].http01.ingress.class").
//     Every fact cites file, line, YAML path (with the element index) and an
//     excerpt.
//   - Text blocks expose multi-line string scalars (and ConfigMap data values)
//     line by line, each line with line-level evidence.
//   - References: `*Ref` objects carrying a name (+kind/group/namespace) are
//     recorded and resolved against the supplied manifests; a reference that
//     does not resolve is explicitly unresolved, never "absent".
//
// These facts are a separate store (Environment.Resources). They do not feed
// ManifestFields, GVKUsage, the report counts or the enrichment prompts, so
// introducing them changes no finding and no prompt digest.
//
// Secrets: values are withheld (Withheld set, Value empty, excerpt without the
// value) for Secret resources, for leaves whose key names a credential, and
// for anything that contains a private key; text lines that assign such a key
// or sit inside a private-key block are withheld the same way. Resource
// values never reach an LLM prompt (internal/impactenrich prints paths only);
// a test pins that.

package env

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// Caps bounding the resource-fact store; hitting one is a warning on the
// manifests dimension, never a silent truncation.
const (
	maxResourceFacts      = 100000 // field facts across all resources
	maxResourceTextLines  = 50000  // text lines across all resources
	maxTextLinesPerBlock  = 5000
	maxFieldValueBytes    = 4096 // longer leaf values are withheld as oversize
	maxRefsPerResource    = 200
	withheldSensitive     = "sensitive"
	withheldOversize      = "oversize"
	redactedExcerptSuffix = "<withheld>"
)

// GVKSelector selects resources by API identity. Group "" selects the core
// group only (apiVersion "v1"); "*" selects any group. Version "" and "*" and
// Kind "*" select any.
type GVKSelector struct {
	Group, Version, Kind string
}

func (s GVKSelector) matches(r *Resource) bool {
	if s.Group != "*" && s.Group != r.Group {
		return false
	}
	if s.Version != "" && s.Version != "*" && s.Version != r.Version {
		return false
	}
	return s.Kind == "*" || s.Kind == r.Kind
}

// FieldFact is one field a resource sets (a leaf, or a mapping/sequence
// container so that "is this subtree set" is answerable).
type FieldFact struct {
	// Path in CRD SchemaPaths syntax: sequences are "[]" ("spec.dnsNames[]").
	Path string
	// Element is the same path with element indexes ("spec.dnsNames[1]").
	Element string
	// Value is the JSON-encoded leaf, as ValuesKey.Value ("\"Always\"", "true",
	// "null"). Empty for containers and when Withheld.
	Value     string
	Container bool   // a mapping or sequence; no Value
	Withheld  string // "" | "sensitive" | "oversize": why Value is empty
	Line      int    // 1-based
	Evidence  []domain.EvidenceID
}

// Ref is a `*Ref` object of a resource ({name, kind?, group?, namespace?}).
type Ref struct {
	Path      string // "spec.issuerRef"; sequence elements use "[]"
	Element   string // with indexes
	Field     string // the key ("issuerRef")
	Name      string
	Kind      string // as stated; "" when not stated
	Group     string // as stated; "" when not stated
	Namespace string // as stated; "" when not stated
	Line      int
	Evidence  []domain.EvidenceID
}

// TextLine is one line of an embedded text block.
type TextLine struct {
	N        int    // 1-based line within the scalar's value
	FileLine int    // 1-based line in the file (see TextBlock.Exact)
	Text     string // "" when Withheld
	Withheld bool
	Evidence domain.EvidenceID
}

// TextBlock is a multi-line string scalar (or a ConfigMap data value) exposed
// line by line.
type TextBlock struct {
	Path  string // values-style path, e.g. `data["policy.csv"]`
	Lines []TextLine
	// Exact reports that every FileLine is the true file line: literal block
	// scalars and single-line scalars. Folded, quoted or plain multi-line
	// scalars cannot be mapped line for line; their lines carry the line the
	// scalar starts on.
	Exact bool
	// Withheld counts lines whose text is withheld; a "no line matches"
	// predicate is not decidable while it is non-zero.
	Withheld int
	// Truncated: the block exceeded the line cap; lines beyond it are unknown.
	Truncated bool
}

// Match returns the lines whose text matches re (withheld lines never match).
func (b TextBlock) Match(re *regexp.Regexp) []TextLine {
	var out []TextLine
	for _, ln := range b.Lines {
		if !ln.Withheld && re.MatchString(ln.Text) {
			out = append(out, ln)
		}
	}
	return out
}

// Resource is one manifest document.
type Resource struct {
	Group, Version, Kind string
	Name, Namespace      string
	Doc                  DocumentRef
	// Evidence cites the document (apiVersion/kind line); the evidence of an
	// UNSET field, since absence has no line of its own.
	Evidence domain.EvidenceID
	Fields   []FieldFact
	Refs     []Ref
	Texts    []TextBlock

	byPath    map[string][]int
	byElement map[string][]int
}

// FieldResult is the answer for one resource: Set is false (and Facts empty)
// when the resource does not state the field.
type FieldResult struct {
	Resource *Resource
	Set      bool
	Facts    []FieldFact
}

// ResourcesOfKind returns the resources of a group/kind (group semantics as
// GVKSelector), in load order.
func (e *Environment) ResourcesOfKind(group, kind string) []*Resource {
	return e.Select(GVKSelector{Group: group, Kind: kind})
}

// Select returns the resources matching sel, in load order.
func (e *Environment) Select(sel GVKSelector) []*Resource {
	if e == nil {
		return nil
	}
	var out []*Resource
	for i := range e.Resources {
		if sel.matches(&e.Resources[i]) {
			out = append(out, &e.Resources[i])
		}
	}
	return out
}

var indexedRe = regexp.MustCompile(`\[\d+\]`)

// FieldValues answers, for every resource matching sel, whether it sets path
// and to what. path uses "[]" markers to cover every sequence element
// ("spec.acme.solvers[].http01.ingress.class") or explicit indexes
// ("spec.dnsNames[0]") for one element. A resource that does not state the
// field is returned with Set=false: "unset" is distinguishable per resource.
func (e *Environment) FieldValues(sel GVKSelector, path string) []FieldResult {
	var out []FieldResult
	for _, r := range e.Select(sel) {
		res := FieldResult{Resource: r}
		idx := r.byPath
		if indexedRe.MatchString(path) {
			idx = r.byElement
		}
		for _, i := range idx[path] {
			res.Facts = append(res.Facts, r.Fields[i])
		}
		res.Set = len(res.Facts) > 0
		out = append(out, res)
	}
	return out
}

// TextBlocks returns, per matching resource, the text blocks whose path equals
// pathPrefix or lies below it (`data` selects every ConfigMap data value;
// "" selects all blocks).
func (e *Environment) TextBlocks(sel GVKSelector, pathPrefix string) []ResourceText {
	var out []ResourceText
	for _, r := range e.Select(sel) {
		rt := ResourceText{Resource: r}
		for _, b := range r.Texts {
			if pathPrefix == "" || b.Path == pathPrefix || strings.HasPrefix(b.Path, pathPrefix+".") || strings.HasPrefix(b.Path, pathPrefix+"[") {
				rt.Blocks = append(rt.Blocks, b)
			}
		}
		out = append(out, rt)
	}
	return out
}

// ResourceText groups the matching text blocks of one resource (possibly none).
type ResourceText struct {
	Resource *Resource
	Blocks   []TextBlock
}

// RefStatus is the outcome of resolving a reference.
type RefStatus string

const (
	// RefResolved: exactly one resource in the supplied manifests matches.
	RefResolved RefStatus = "resolved"
	// RefUnresolved: nothing in the supplied manifests matches. This is NOT
	// "the target does not exist" — see RefResolution.ManifestsComplete.
	RefUnresolved RefStatus = "unresolved"
	// RefAmbiguous: several distinct resources match (name-only reference
	// without a kind, say); Candidates lists them.
	RefAmbiguous RefStatus = "ambiguous"
)

// RefResolution is the result of ResolveRef.
type RefResolution struct {
	Status     RefStatus
	Target     *Resource   // set when Resolved
	Candidates []*Resource // set when Ambiguous
	// ManifestsComplete: the manifests dimension is healthy, so an unresolved
	// reference means the target is not among everything that was supplied;
	// otherwise the target may be in a file that was not parsed.
	ManifestsComplete bool
	Reason            string
}

// ResolveRef resolves ref (taken from `from`) against the supplied manifests.
// Group and Kind constrain the match only when the reference states them; a
// reference without a namespace matches the referring resource's namespace
// (or a resource declaring none).
func (e *Environment) ResolveRef(from *Resource, ref Ref) RefResolution {
	out := RefResolution{Status: RefUnresolved, ManifestsComplete: e != nil && e.Health(DimManifests) == HealthOK}
	if e == nil || ref.Name == "" {
		out.Reason = "reference states no name"
		return out
	}
	var cands []*Resource
	for i := range e.Resources {
		r := &e.Resources[i]
		if r.Name != ref.Name {
			continue
		}
		if ref.Kind != "" && r.Kind != ref.Kind {
			continue
		}
		if ref.Group != "" && r.Group != ref.Group {
			continue
		}
		switch {
		case ref.Namespace != "":
			if r.Namespace != ref.Namespace {
				continue
			}
		case from != nil:
			if r.Namespace != "" && r.Namespace != from.Namespace {
				continue
			}
		}
		cands = append(cands, r)
	}
	switch len(cands) {
	case 0:
		out.Reason = fmt.Sprintf("no manifest named %q%s in the supplied manifests", ref.Name, kindText(ref))
	case 1:
		out.Status, out.Target = RefResolved, cands[0]
	default:
		out.Status, out.Candidates = RefAmbiguous, cands
		out.Reason = fmt.Sprintf("%d manifests match %q%s", len(cands), ref.Name, kindText(ref))
	}
	return out
}

func kindText(ref Ref) string {
	if ref.Kind == "" {
		return ""
	}
	return " of kind " + ref.Kind
}

// ResolvedRef pairs a reference with its resolution.
type ResolvedRef struct {
	From       *Resource
	Ref        Ref
	Resolution RefResolution
}

// References returns every reference at path ("spec.issuerRef") of the
// resources matching sel, each resolved. A resource without the reference
// contributes nothing (use FieldValues to ask whether the field is set).
func (e *Environment) References(sel GVKSelector, path string) []ResolvedRef {
	var out []ResolvedRef
	for _, r := range e.Select(sel) {
		for _, ref := range r.Refs {
			if ref.Path == path {
				out = append(out, ResolvedRef{From: r, Ref: ref, Resolution: e.ResolveRef(r, ref)})
			}
		}
	}
	return out
}

// --- extraction -----------------------------------------------------------------

var (
	sensitiveKeyRe = regexp.MustCompile(`(?i)(password|passwd|passphrase|token|api[-_]?key|secret$|secret[-_]?key|private[-_]?key$|credential|bearer|client[-_]?secret)`)
	sensitiveLine  = regexp.MustCompile(`(?i)(password|passwd|passphrase|secret|token|api[-_]?key|private[-_]?key|client[-_]?secret)\w*\s*[:=]\s*\S`)
	privateKeyMark = regexp.MustCompile(`-----(BEGIN|END) [A-Z ]*PRIVATE KEY-----`)
)

// resourceWalk is the per-document extraction state.
type resourceWalk struct {
	l         *loader
	d         doc
	res       *Resource
	kindIsSec bool
	isCM      bool
}

// collectResource extracts the resource facts of one manifest document.
func (l *loader) collectResource(d doc, group, version, kind string) {
	res := Resource{
		Group: group, Version: version, Kind: kind,
		Name:      scalarOf(d.node, "metadata", "name"),
		Namespace: scalarOf(d.node, "metadata", "namespace"),
		Doc:       DocumentRef{File: d.file, StartLine: d.startLine},
		Evidence:  l.ev(d.file, fmt.Sprintf("L%d", d.startLine), "apiVersion: "+apiVersionOf(group, version)+" / kind: "+kind),
		byPath:    map[string][]int{}, byElement: map[string][]int{},
	}
	w := &resourceWalk{l: l, d: d, res: &res, kindIsSec: group == "" && kind == "Secret", isCM: group == "" && kind == "ConfigMap"}
	for _, kv := range pairsOf(d.node) {
		w.value(kv[1], joinPath("", kv[0].Value), joinPath("", kv[0].Value), kv[0].Value, kv[0].Line, 0)
	}
	for i, f := range res.Fields {
		res.byPath[f.Path] = append(res.byPath[f.Path], i)
		res.byElement[f.Element] = append(res.byElement[f.Element], i)
	}
	l.env.Resources = append(l.env.Resources, res)
}

func apiVersionOf(group, version string) string {
	if group == "" {
		return version
	}
	return group + "/" + version
}

func (w *resourceWalk) full() bool {
	if w.l.resFacts >= maxResourceFacts {
		w.l.warnf(DimManifests, "more than %d resource field facts; the rest were not inventoried", maxResourceFacts)
		return true
	}
	return false
}

// value records the facts of node n at path/element and descends. key is the
// mapping key n sits under ("" under a sequence element); refKey reports that
// the sequence/mapping is a reference collection ("parentRefs").
func (w *resourceWalk) value(n *yaml.Node, path, element, key string, keyLine, depth int) {
	n = resolveAlias(n)
	if n == nil || depth > 64 || w.full() {
		return
	}
	line := n.Line
	if keyLine > 0 && (n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode) {
		line = keyLine
	}
	fact := FieldFact{Path: path, Element: element, Line: line}
	loc := fmt.Sprintf("$.%s (L%d)", element, line)
	switch n.Kind {
	case yaml.MappingNode:
		fact.Container = true
		w.add(fact, loc, path+": {…}")
		if w.res.Kind != "" && strings.HasSuffix(key, "Ref") && len(key) > 3 {
			w.ref(n, path, element, key, line)
		}
		for _, kv := range pairsOf(n) {
			w.value(kv[1], joinPath(path, kv[0].Value), joinPath(element, kv[0].Value), kv[0].Value, kv[0].Line, depth+1)
		}
	case yaml.SequenceNode:
		fact.Container = true
		w.add(fact, loc, path+": […]")
		refs := strings.HasSuffix(key, "Refs") && len(key) > 4
		for i, c := range n.Content {
			c = resolveAlias(c)
			if c == nil {
				continue
			}
			ep := path + "[]"
			ee := element + "[" + strconv.Itoa(i) + "]"
			if refs && c.Kind == yaml.MappingNode {
				w.ref(c, ep, ee, key, c.Line)
			}
			w.value(c, ep, ee, "", 0, depth+1)
		}
	case yaml.ScalarNode:
		w.scalar(n, fact, loc, key)
	}
}

func (w *resourceWalk) add(f FieldFact, locator, excerpt string) {
	if w.full() {
		return
	}
	f.Evidence = []domain.EvidenceID{w.l.ev(w.d.file, locator, excerpt)}
	w.res.Fields = append(w.res.Fields, f)
	w.l.resFacts++
}

func (w *resourceWalk) scalar(n *yaml.Node, fact FieldFact, loc, key string) {
	val := normalize.EncodeLeaf(n)
	switch {
	case w.kindIsSec || (key != "" && sensitiveKeyRe.MatchString(key)) || privateKeyMark.MatchString(n.Value):
		fact.Withheld = withheldSensitive
	case len(val) > maxFieldValueBytes:
		fact.Withheld = withheldOversize
	default:
		fact.Value = val
	}
	excerpt := fact.Path + ": " + redactedExcerptSuffix
	if fact.Withheld == "" {
		excerpt = fact.Path + ": " + val
	}
	w.add(fact, loc, excerpt)
	if w.kindIsSec || n.Tag != "!!str" {
		return
	}
	if strings.Contains(n.Value, "\n") || (w.isCM && strings.HasPrefix(fact.Path, "data")) {
		w.text(n, fact)
	}
}

// text exposes a string scalar line by line.
func (w *resourceWalk) text(n *yaml.Node, fact FieldFact) {
	lines := strings.Split(n.Value, "\n")
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] // the final newline ends the last line
	}
	literal := n.Style&yaml.LiteralStyle != 0
	folded := n.Style&yaml.FoldedStyle != 0
	block := TextBlock{Path: fact.Element, Exact: literal || (len(lines) == 1 && !folded)}
	inKey := false
	for i, text := range lines {
		if i >= maxTextLinesPerBlock {
			block.Truncated = true
			w.l.warnf(DimManifests, "%s: text %s has more than %d lines; the rest were not inventoried", w.d.file, fact.Element, maxTextLinesPerBlock)
			break
		}
		if w.l.resLines >= maxResourceTextLines {
			block.Truncated = true
			w.l.warnf(DimManifests, "more than %d embedded text lines; the rest were not inventoried", maxResourceTextLines)
			break
		}
		w.l.resLines++
		fileLine := n.Line
		switch {
		case literal:
			fileLine = n.Line + 1 + i
		case folded:
			fileLine = n.Line + 1 // folding merges lines: the first content line, not exact
		}
		m := privateKeyMark.FindStringSubmatch(text)
		withheld := inKey || sensitiveLine.MatchString(text) || m != nil
		if m != nil {
			inKey = m[1] == "BEGIN"
		}
		ln := TextLine{N: i + 1, FileLine: fileLine, Withheld: withheld}
		excerpt := fmt.Sprintf("%s line %d: %s", fact.Element, i+1, redactedExcerptSuffix)
		if !withheld {
			ln.Text = text
			excerpt = fmt.Sprintf("%s line %d: %s", fact.Element, i+1, text)
		} else {
			block.Withheld++
		}
		ln.Evidence = w.l.ev(w.d.file, fmt.Sprintf("$.%s line %d (L%d)", fact.Element, i+1, fileLine), excerpt)
		block.Lines = append(block.Lines, ln)
	}
	w.res.Texts = append(w.res.Texts, block)
}

// ref records a reference object when it carries a name.
func (w *resourceWalk) ref(n *yaml.Node, path, element, key string, line int) {
	name := scalarOf(n, "name")
	if name == "" || len(w.res.Refs) >= maxRefsPerResource {
		return
	}
	r := Ref{
		Path: path, Element: element, Field: key, Name: name, Line: line,
		Kind: scalarOf(n, "kind"), Group: scalarOf(n, "group"), Namespace: scalarOf(n, "namespace"),
	}
	if r.Group == "" {
		// some references state the group through an apiVersion
		if av := scalarOf(n, "apiVersion"); av != "" {
			r.Group, _ = splitGroupVersion(av)
		}
	}
	r.Evidence = []domain.EvidenceID{w.l.ev(w.d.file, fmt.Sprintf("$.%s (L%d)", element, line),
		fmt.Sprintf("%s: name=%s%s", key, name, refExcerpt(r)))}
	w.res.Refs = append(w.res.Refs, r)
}

func refExcerpt(r Ref) string {
	var b strings.Builder
	if r.Kind != "" {
		b.WriteString(" kind=" + r.Kind)
	}
	if r.Group != "" {
		b.WriteString(" group=" + r.Group)
	}
	if r.Namespace != "" {
		b.WriteString(" namespace=" + r.Namespace)
	}
	return b.String()
}
