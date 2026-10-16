package validate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// objSource is one parsed YAML stream of objects: an environment file, or
// one side of an environment render pair.
type objSource struct {
	artifact domain.ArtifactRef
	side     string // "file" | "from" | "to"
	target   string // render target (render sides)
	objects  []render.Object
}

// renderSides is one complete environment render pair, parsed.
type renderSides struct {
	pair     domain.ManifestPair
	from, to objSource
}

// envView is what the environment states, read from the bundle's artifacts:
// the environment files (as declared; overlays are evaluated through the
// renders, DESIGN.md conflict 6) and every complete environment render.
type envView struct {
	pairs    []domain.ManifestPair
	complete bool // ≥1 environment render pair and every one complete
	files    []objSource
	renders  []renderSides
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
	v := &envView{unparsed: map[string][]byte{}}
	v.pairs, v.complete = envPairs(b)
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
	return v, nil
}

func yamlArtifact(a domain.ArtifactRef) bool {
	p := strings.ToLower(a.Path)
	return strings.Contains(a.MediaType, "yaml") || strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml")
}

// mentionsKind reports whether an unparsed environment file may declare an
// object of the kind (so a universal statement about the kind is unsafe).
func (v *envView) mentionsKind(kind string) []string {
	var out []string
	for path, data := range v.unparsed {
		if bytes.Contains(data, []byte("kind: "+kind)) {
			out = append(out, path)
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
	var walk func(v any, segs []segment)
	walk = func(v any, segs []segment) {
		if len(segs) == 0 {
			if v == nil {
				missing++
				return
			}
			vals = append(vals, encodeJSON(v))
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
