package validate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/investigate/redaction"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// objSource is one parsed YAML stream of objects: an environment file, one
// side of an environment render pair, or the render of a version-independent
// overlay.
type objSource struct {
	artifact domain.ArtifactRef
	side     string // "file" | "from" | "to" | "overlay"
	target   string // render target (render sides, overlays)
	objects  []render.Object
	// readBack: object id → the withheld enum values read back in it
	// (envView.recoverWithheld).
	readBack map[string][]redaction.Recovered
}

// label names the view an object of the source came from (validation
// details record it for every object they decide on).
func (s objSource) label() string {
	switch s.side {
	case "schema":
		return "CRD artifact " + s.artifact.Path
	case "file":
		return "environment file " + s.artifact.Path + ", no render consumed it"
	case "overlay":
		return "environment render " + s.target + ", version-independent overlay"
	}
	return "environment render " + s.target + " (" + s.side + ")"
}

// renderSides is one complete environment render pair, parsed.
type renderSides struct {
	pair     domain.ManifestPair
	from, to objSource
}

// envView is what the environment states, read from the bundle's artifacts:
// the environment files, every complete environment render, and the renders
// of version-independent overlays (not-applicable pairs: a kustomize overlay
// that does not deploy the product renders the same objects at both
// versions).
//
// The effective object view: an environment file a render consumed (a
// kustomize base, a patch, the kustomization itself) is seen through that
// render's objects — the base merged with its patches — never on its own. A
// file no render consumed is used as declared (DESIGN.md conflict 6). A file
// consumed by a render whose objects are unavailable decides nothing.
type envView struct {
	pairs    []domain.ManifestPair
	complete bool // ≥1 environment render pair and every one complete
	files    []objSource
	renders  []renderSides
	overlays []objSource
	// schemas are the CustomResourceDefinitions the bundle holds (upstream
	// CRD artifacts, environment files and renders): the only source of a
	// schema default the view may use.
	schemas []effObj
	// consumers: environment file path → the renders in the view that
	// consumed it; unrendered: … → the renders that consumed it but whose
	// objects are not available (with why).
	consumers  map[string][]string
	unrendered map[string][]string
	// unparsed are environment YAML files the object parser rejected, with
	// their raw bytes (a pin over a kind they mention cannot be universal).
	unparsed map[string][]byte
	// gaps names environment inputs that were not collected completely.
	gaps  []string
	notes []string
}

// loadView reads the environment files and renders of a bundle. Unreadable
// artifacts are recorded as notes (and gaps), never silently dropped.
func loadView(ctx context.Context, c config, b *domain.EvidenceBundle) (*envView, error) {
	v := &envView{unparsed: map[string][]byte{}, consumers: map[string][]string{}, unrendered: map[string][]string{}}
	v.pairs, v.complete = envPairs(b)
	raw := map[string][]byte{} // every readable environment YAML file, by bundle path
	for _, in := range b.Snapshot.Inputs {
		switch in.Kind {
		case domain.InputGitOpsRepo, domain.InputManifests, domain.InputValues:
			if in.Status.Gap() {
				v.gaps = append(v.gaps, fmt.Sprintf("input %s %s is %s (%s)", in.Kind, in.Path, in.Status, in.Reason))
			}
		}
	}
	for _, a := range b.Manifest.Artifacts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if a.Kind != domain.ArtifactEnvironmentFile || !yamlArtifact(a) {
			continue
		}
		data, _, err := c.read(ctx, b, a.ID)
		if err != nil {
			v.gaps = append(v.gaps, fmt.Sprintf("environment file %s unreadable: %v", a.Path, err))
			continue
		}
		raw[a.Path] = data
		objs, err := render.ParseObjects(data)
		if err != nil {
			v.unparsed[a.Path] = data
			v.notes = append(v.notes, fmt.Sprintf("environment file %s is not a plain YAML object stream (%v)", a.Path, err))
			continue
		}
		if len(objs) > 0 {
			v.files = append(v.files, objSource{artifact: a, side: "file", objects: objs})
		}
	}
	for _, p := range v.pairs {
		if p.Status != domain.CollectionOK {
			continue
		}
		rs := renderSides{pair: p}
		ok := true
		for _, side := range []struct {
			id  string
			dst *objSource
			tag string
		}{{p.From, &rs.from, "from"}, {p.To, &rs.to, "to"}} {
			data, ref, err := c.read(ctx, b, side.id)
			if err != nil {
				v.gaps = append(v.gaps, fmt.Sprintf("render %s (%s) unreadable: %v", p.Target, side.tag, err))
				ok = false
				break
			}
			objs, err := render.ParseObjects(data)
			if err != nil {
				v.gaps = append(v.gaps, fmt.Sprintf("render %s (%s) unparsable: %v", p.Target, side.tag, err))
				ok = false
				break
			}
			*side.dst = objSource{artifact: ref, side: side.tag, target: p.Target, objects: objs}
		}
		if ok {
			v.renders = append(v.renders, rs)
		} else {
			v.complete = false
		}
	}
	rendered := map[string]bool{}
	for _, r := range v.renders {
		rendered[r.pair.Target] = true
	}
	// version-independent overlays: their (from) render is the effective
	// state of the objects they declare, before and after the upgrade
	for _, p := range b.Renders {
		if p.Status != domain.CollectionNotApplicable || p.From == "" {
			continue
		}
		data, ref, err := c.read(ctx, b, p.From)
		if err != nil {
			v.notes = append(v.notes, fmt.Sprintf("render %s (version-independent overlay) unreadable: %v", p.Target, err))
			continue
		}
		objs, err := render.ParseObjects(data)
		if err != nil {
			v.notes = append(v.notes, fmt.Sprintf("render %s (version-independent overlay) unparsable: %v", p.Target, err))
			continue
		}
		v.overlays = append(v.overlays, objSource{artifact: ref, side: "overlay", target: p.Target, objects: objs})
		rendered[p.Target] = true
	}
	// CustomResourceDefinitions: upstream CRD artifacts, plus those the
	// environment files and renders declare
	for _, a := range b.Manifest.Artifacts {
		if a.Kind != domain.ArtifactCRDDefinition || !yamlArtifact(a) {
			continue
		}
		data, ref, err := c.read(ctx, b, a.ID)
		if err != nil {
			v.notes = append(v.notes, fmt.Sprintf("CRD artifact %s unreadable: %v", a.Path, err))
			continue
		}
		objs, err := render.ParseObjects(data)
		if err != nil {
			v.notes = append(v.notes, fmt.Sprintf("CRD artifact %s unparsable: %v", a.Path, err))
			continue
		}
		src := objSource{artifact: ref, side: "schema", objects: objs}
		for _, o := range objs {
			if isCRD(o) {
				v.schemas = append(v.schemas, effObj{src: src, obj: o})
			}
		}
	}
	// which environment files each kustomize render consumed
	for _, p := range b.Renders {
		if p.Scope != domain.RenderEnvironment {
			continue
		}
		dir, ok := kustomizeDir(p.Target)
		if !ok {
			continue
		}
		files, missing := kustomizeConsumes(raw, dir)
		for _, m := range missing {
			v.notes = append(v.notes, fmt.Sprintf("render %s: %s", p.Target, m))
		}
		for _, f := range files {
			if rendered[p.Target] {
				v.consumers[f] = append(v.consumers[f], p.Target)
			} else {
				v.unrendered[f] = append(v.unrendered[f], fmt.Sprintf("%s (%s)", p.Target, p.Status))
			}
		}
	}
	for _, e := range v.objects() {
		if isCRD(e.obj) && e.unrendered == "" {
			v.schemas = append(v.schemas, e)
		}
	}
	v.recoverWithheld()
	return v, nil
}

// recoverWithheld reads back, in every object of the view, the values the
// collector withheld below a credential-named mapping that are one of the
// string enum values the bundle's CRDs declare for that exact field
// (redaction.Schemas.Recover): only a schema constant is ever shown;
// anything else stays withheld.
func (v *envView) recoverWithheld() {
	ix := redaction.NewSchemas()
	for _, e := range v.schemas {
		ix.AddCRD(e.obj.Body)
	}
	if ix.Len() == 0 {
		return
	}
	each := func(src *objSource) {
		for _, o := range src.objects {
			r := ix.Recover(o.ID.Group, o.ID.Version, o.ID.Kind, o.Body)
			if len(r) == 0 {
				continue
			}
			if src.readBack == nil {
				src.readBack = map[string][]redaction.Recovered{}
			}
			src.readBack[o.ID.String()] = append(src.readBack[o.ID.String()], r...)
		}
	}
	for i := range v.files {
		each(&v.files[i])
	}
	for i := range v.renders {
		each(&v.renders[i].from)
		each(&v.renders[i].to)
	}
	for i := range v.overlays {
		each(&v.overlays[i])
	}
}

// readBackNote says that the value at path of o was read back from its
// redaction digest ("" when it was not).
func (s objSource) readBackNote(o render.ObjectID, path string) string {
	for _, r := range s.readBack[o.String()] {
		if r.Path == path {
			return fmt.Sprintf(" (stored redacted as %s: the digest of %q, an enum value its CRD declares)", r.Marker, r.Value)
		}
	}
	return ""
}

// redactedValue reports whether a value — decoded, or JSON-encoded as
// lookup returns it — is a redaction marker (the bundle's or the
// renderer's): what it stands for is not known.
func redactedValue(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	var dec string
	if strings.HasPrefix(s, `"`) && json.Unmarshal([]byte(s), &dec) == nil {
		s = dec
	}
	return redaction.IsMarker(s) || strings.HasPrefix(s, "<sensitive sha256:")
}

func isCRD(o render.Object) bool {
	return o.ID.Kind == "CustomResourceDefinition" && o.ID.Group == "apiextensions.k8s.io"
}

// effObj is one object of the effective view.
type effObj struct {
	src objSource
	obj render.Object
	// from is the same object before the upgrade, for objects of a render of
	// the product (nil with fromKnown false: no counterpart). Files and
	// version-independent overlays state the same object on both sides.
	from      *render.Object
	sameSides bool
	rendered  bool
	// unrendered: why the object's effective state is unknown (its file was
	// consumed by a render whose objects are not available).
	unrendered string
}

// objects is the effective object view: the to side of every complete
// render of the product, every version-independent overlay render, and the
// environment files no render in the view consumed.
func (v *envView) objects() []effObj {
	var out []effObj
	for _, f := range v.files {
		path := f.artifact.Path
		why := ""
		if u := v.unrendered[path]; len(u) > 0 {
			why = "consumed by " + strings.Join(u, ", ") + ", whose rendered objects are not available, so its effective state is unknown"
		} else if len(v.consumers[path]) > 0 {
			continue // seen through the render(s) that consumed it
		}
		for _, o := range f.objects {
			out = append(out, effObj{src: f, obj: o, sameSides: true, unrendered: why})
		}
	}
	for _, r := range v.renders {
		for _, o := range r.to.objects {
			e := effObj{src: r.to, obj: o, rendered: true}
			if fo, ok := r.fromObject(o.ID); ok {
				e.from = &fo
			}
			out = append(out, e)
		}
	}
	for _, s := range v.overlays {
		for _, o := range s.objects {
			out = append(out, effObj{src: s, obj: o, sameSides: true, rendered: true})
		}
	}
	return out
}

// replaced lists the environment files the view sees through renders, with
// the renders (for a record's detail).
func (v *envView) replaced() []string {
	var out []string
	for _, f := range v.files {
		if c := v.consumers[f.artifact.Path]; len(c) > 0 && len(v.unrendered[f.artifact.Path]) == 0 {
			out = append(out, fmt.Sprintf("%s → %s", f.artifact.Path, strings.Join(uniq(c), ", ")))
		}
	}
	sort.Strings(out)
	return out
}

// kustomizeDir is the bundle directory of a kustomize render target
// ("kustomize:overlays/prod" → "env/overlays/prod").
func kustomizeDir(target string) (string, bool) {
	rest, ok := strings.CutPrefix(target, "kustomize:")
	if !ok || rest == "" || path.IsAbs(rest) {
		return "", false
	}
	if i := strings.LastIndexByte(rest, '#'); i >= 0 {
		rest = rest[:i] // a repeated target name ("…#2")
	}
	return path.Join("env", path.Clean(rest)), true
}

var kustomizationNames = []string{"kustomization.yaml", "kustomization.yml", "Kustomization"}

// kustomizeConsumes walks the kustomization in dir over the bundle's
// environment files (by bundle path) and returns every file it consumes:
// kustomization files, resources, components, bases, CRDs and patch files
// (what render.KustomizeInputs reads from a checkout). References to files
// the bundle does not hold are reported, never guessed; remote references
// are not files of the environment.
func kustomizeConsumes(files map[string][]byte, dir string) (consumed, missing []string) {
	seen := map[string]bool{}
	visited := map[string]bool{}
	kfile := func(d string) string {
		for _, n := range kustomizationNames {
			if _, ok := files[path.Join(d, n)]; ok {
				return path.Join(d, n)
			}
		}
		return ""
	}
	var walk func(d string, depth int)
	walk = func(d string, depth int) {
		if depth > 20 || visited[d] {
			return
		}
		visited[d] = true
		kf := kfile(d)
		if kf == "" {
			missing = append(missing, "the bundle holds no kustomization file in "+d)
			return
		}
		seen[kf] = true
		var k map[string]any
		if err := yaml.Unmarshal(files[kf], &k); err != nil {
			missing = append(missing, fmt.Sprintf("%s: %v", kf, err))
			return
		}
		var refs []string
		for _, key := range []string{"resources", "bases", "components", "crds", "patchesStrategicMerge", "patches", "patchesJson6902"} {
			l, _ := k[key].([]any)
			for _, x := range l {
				switch t := x.(type) {
				case string:
					refs = append(refs, t)
				case map[string]any:
					if s, ok := t["path"].(string); ok {
						refs = append(refs, s)
					}
				}
			}
		}
		for _, r := range refs {
			r = strings.TrimSpace(r)
			if r == "" || strings.Contains(r, "\n") || remoteRef(r) {
				continue
			}
			p := path.Clean(path.Join(d, r))
			if kfile(p) != "" {
				walk(p, depth+1)
				continue
			}
			if _, ok := files[p]; ok {
				seen[p] = true
				continue
			}
			missing = append(missing, fmt.Sprintf("%s references %s, which the bundle does not hold as an environment YAML file", kf, r))
		}
	}
	walk(dir, 0)
	for f := range seen {
		consumed = append(consumed, f)
	}
	sort.Strings(consumed)
	return consumed, missing
}

func remoteRef(r string) bool {
	return strings.Contains(r, "://") || strings.HasPrefix(r, "github.com/") || strings.HasPrefix(r, "git@") || strings.Contains(r, "?ref=")
}

func yamlArtifact(a domain.ArtifactRef) bool {
	p := strings.ToLower(a.Path)
	return strings.Contains(a.MediaType, "yaml") || strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml")
}

// mentionsKind reports whether an unparsed environment file may declare an
// object of the kind (so a universal statement about the kind is unsafe).
func (v *envView) mentionsKind(kind string) []string {
	var out []string
	for p, data := range v.unparsed {
		if len(v.consumers[p]) > 0 && len(v.unrendered[p]) == 0 {
			continue // its effective content is in the render that consumed it
		}
		if bytes.Contains(data, []byte("kind: "+kind)) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// inputs lists the artifact ids the view read (for a record's Inputs).
func (v *envView) inputs(files bool) []string {
	var ids []string
	if files {
		for _, f := range v.files {
			ids = append(ids, f.artifact.ID)
		}
		for _, o := range v.overlays {
			ids = append(ids, o.artifact.ID)
		}
	}
	for _, r := range v.renders {
		ids = append(ids, r.from.artifact.ID, r.to.artifact.ID)
	}
	return ids
}

// fromObject finds the from-side counterpart of a to-side object (same
// group, kind, namespace and name; the version may differ).
func (r renderSides) fromObject(id render.ObjectID) (render.Object, bool) {
	for _, o := range r.from.objects {
		if o.ID.Group == id.Group && o.ID.Kind == id.Kind && o.ID.Namespace == id.Namespace && o.ID.Name == id.Name {
			return o, true
		}
	}
	return render.Object{}, false
}

// --- field lookup -------------------------------------------------------------

type segment struct {
	name string
	list bool // "name[]": every element of the list
}

// segments splits a values/schema-style path ("spec.a[].b",
// `data["x.y"]`, "metadata.annotations.example.com/key").
func segments(path string) []segment {
	for _, prefix := range []string{"metadata.annotations.", "metadata.labels."} {
		if rest, ok := strings.CutPrefix(path, prefix); ok && !strings.HasPrefix(rest, "[") {
			parts := strings.Split(strings.TrimSuffix(prefix, "."), ".")
			return []segment{{name: parts[0]}, {name: parts[1]}, {name: strings.Trim(rest, `"`)}}
		}
	}
	var out []segment
	for _, s := range env.SplitKeyPath(path) {
		name, list := strings.CutSuffix(s, "[]")
		out = append(out, segment{name: name, list: list})
	}
	return out
}

// lookup returns the JSON-encoded values at path in an object body and how
// many instances along the path do not state it (a list element without the
// field, an absent parent). An instance that does not state the field is
// never a pin.
func lookup(body map[string]any, path string) (vals []string, missing int) {
	raw, missing := lookupAny(body, path)
	for _, v := range raw {
		vals = append(vals, encodeJSON(v))
	}
	return vals, missing
}

// lookupAny is lookup with the decoded values.
func lookupAny(body map[string]any, path string) (vals []any, missing int) {
	var walk func(v any, segs []segment)
	walk = func(v any, segs []segment) {
		if len(segs) == 0 {
			if v == nil {
				missing++
				return
			}
			vals = append(vals, v)
			return
		}
		m, ok := v.(map[string]any)
		if !ok {
			missing++
			return
		}
		x, present := m[segs[0].name]
		if !present || x == nil {
			missing++
			return
		}
		if !segs[0].list {
			walk(x, segs[1:])
			return
		}
		l, ok := x.([]any)
		if !ok || len(l) == 0 {
			missing++
			return
		}
		for _, el := range l {
			walk(el, segs[1:])
		}
	}
	walk(body, segments(path))
	return vals, missing
}

func encodeJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprintf("%q", fmt.Sprint(v))
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// encodeValue makes a raw setting value (a flag's "=value" part) a
// JSON-encoded scalar; a value that already is one is kept.
func encodeValue(raw string) string {
	var x any
	if json.Unmarshal([]byte(raw), &x) == nil {
		switch x.(type) {
		case string, bool, float64, nil:
			return raw
		}
	}
	return encodeJSON(raw)
}

// plainValue is the comparable text of a JSON-encoded (or raw) value.
func plainValue(v string) string {
	var x any
	if json.Unmarshal([]byte(v), &x) == nil {
		switch t := x.(type) {
		case string:
			return t
		case nil:
			return "null"
		default:
			return encodeJSON(t)
		}
	}
	return v
}

// sameValue compares two values JSON-aware ("\"Never\"" equals Never).
func sameValue(a, b string) bool { return plainValue(a) == plainValue(b) }
