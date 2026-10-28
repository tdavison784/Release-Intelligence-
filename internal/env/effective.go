package env

// The effective object view (lane join5, D-0028): what the environment
// deploys is the RENDERED form of its entrypoints, not the files they read.
// A kustomize base declares an object; an overlay's patch may set, override
// or remove its fields; only the render shows the object as deployed. An
// applicability evaluation that reads the base file judges an object that
// does not exist in that form — fail-closed where the overlay sets a field
// the base leaves unset, fail-OPEN where the overlay removes or overrides a
// setting the base states.
//
// Effective returns the environment with its Resources replaced by that view
// (the same view the investigation validators use, internal/investigate/
// validate envView):
//
//   - an environment file no render consumed is read as declared;
//   - a file a render consumed is seen only through the render's objects;
//     each rendered field cites the file and line that produced it — the
//     consumed file of the latest kustomize layer that states the field with
//     the rendered value (an overlay patch before its base), or the rendered
//     output itself when no file states it (a transformer or generator);
//   - a file consumed by a render whose objects are unavailable keeps its
//     declaration, marked Unrendered: evaluators never decide on it;
//   - a render that consumed no environment file and whose objects are
//     unavailable (a chart render that failed) is a RenderGap: the objects
//     it would deploy are unknown.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// RenderedObjects is one environment render for Effective.
type RenderedObjects struct {
	// Target names the render ("kustomize:apps/web/overlays/prod", a Helm
	// release target).
	Target string
	// Consumed are the environment files the render read, by path (as Load
	// got them; compared after filepath.Abs), each with its kustomize layer
	// rank (render.KustomizeLayers: a lower rank is applied later and wins).
	Consumed map[string]int
	// Docs is the rendered YAML stream: the target side of a complete pair,
	// or the render of a version-independent overlay. Digest is its digest.
	Docs   []byte
	Digest string
	// Unavailable says why the render's objects are not available ("" when
	// Docs holds them).
	Unavailable string
}

// EffectiveProducer marks evidence records of rendered objects.
const EffectiveProducer = "env.effective@v1"

// Effective returns e with its Resources replaced by the effective object
// view of srcs (see the file comment). Every other part of the environment
// (values, CRDs, images, health, evidence) is e's; the evidence pool gains
// the records of the rendered objects. e is not modified.
func Effective(e *Environment, srcs []RenderedObjects) *Environment {
	if e == nil {
		return nil
	}
	out := *e
	out.Resources = nil
	out.RenderGaps = nil
	out.EffectiveView = true
	out.Evidence = append([]domain.Evidence(nil), e.Evidence...)
	pool := map[domain.EvidenceID]bool{}
	for _, x := range out.Evidence {
		pool[x.ID] = true
	}

	consumers := map[string][]int{} // normalized file → srcs consuming it
	for i, s := range srcs {
		for f := range s.Consumed {
			n := normPath(f)
			consumers[n] = append(consumers[n], i)
		}
	}
	// files read as declared, or kept as unrendered declarations
	for _, r := range e.Resources {
		by := consumers[normPath(r.Doc.File)]
		if len(by) == 0 {
			out.Resources = append(out.Resources, r)
			continue
		}
		var why []string
		for _, i := range by {
			if srcs[i].Unavailable != "" {
				why = append(why, fmt.Sprintf("%s (%s)", srcs[i].Target, srcs[i].Unavailable))
			}
		}
		if len(why) == 0 {
			continue // seen through the render(s) that consumed it
		}
		r.Unrendered = fmt.Sprintf("%s is consumed by render %s, whose rendered objects are not available, so the deployed form of %s is unknown",
			r.Doc.File, strings.Join(why, ", "), resourceID(&r))
		out.Resources = append(out.Resources, r)
	}
	// the rendered objects
	for _, s := range srcs {
		if s.Unavailable != "" {
			if len(s.Consumed) == 0 {
				out.RenderGaps = append(out.RenderGaps, fmt.Sprintf("%s: %s", s.Target, s.Unavailable))
			}
			continue
		}
		for _, r := range renderedResources(s) {
			mapToSources(&r, s, e)
			out.Resources = append(out.Resources, r)
		}
	}
	// the records the rendered objects cite: their own, and the files'
	for _, r := range out.Resources {
		for _, id := range resourceEvidence(&r) {
			if pool[id.ID] {
				continue
			}
			pool[id.ID] = true
			out.Evidence = append(out.Evidence, id)
		}
	}
	for i := range out.Resources {
		out.Resources[i].index()
	}
	return &out
}

// renderedResources parses a render's output into resources (and their
// records, kept on the resources until Effective pools them).
func renderedResources(s RenderedObjects) []Resource {
	uri := "render:" + s.Target
	l := &loader{env: &Environment{}, evidence: map[domain.EvidenceID]bool{}, files: map[string]string{uri: s.Digest},
		warned: map[string]bool{}, dimWarnings: map[string][]string{}, evKind: domain.EvidenceRendered}
	docs, _ := parseDocs(s.Docs, nil) // the renderer parsed this stream already; a bad tail adds nothing
	for _, d := range docs {
		if d.node == nil {
			continue
		}
		apiVersion, kind := scalarOf(d.node, "apiVersion"), scalarOf(d.node, "kind")
		if apiVersion == "" || kind == "" {
			continue
		}
		d.file = uri
		group, version := splitGroupVersion(apiVersion)
		l.collectResource(d, group, version, kind)
	}
	ix := map[domain.EvidenceID]domain.Evidence{}
	for _, x := range l.env.Evidence {
		ix[x.ID] = x
	}
	out := l.env.Resources
	for i := range out {
		out[i].Render = s.Target
		out[i].records = ix
	}
	return out
}

// sourceDoc is a declaration of a rendered object in a consumed file.
type sourceDoc struct {
	r    *Resource
	rank int
}

// mapToSources points every field, text line and reference of the rendered
// resource r at the consumed file that produced it.
func mapToSources(r *Resource, s RenderedObjects, e *Environment) {
	var cands []sourceDoc
	for i := range e.Resources {
		src := &e.Resources[i]
		rank, ok := s.Consumed[src.Doc.File]
		if !ok {
			n := normPath(src.Doc.File)
			for f, rk := range s.Consumed {
				if normPath(f) == n {
					rank, ok = rk, true
					break
				}
			}
		}
		if !ok || src.Group != r.Group || src.Kind != r.Kind || !sameObjectName(r.Name, src.Name) {
			continue
		}
		cands = append(cands, sourceDoc{src, rank})
	}
	// the latest layer first; within a layer, the same namespace first
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].rank != cands[j].rank {
			return cands[i].rank < cands[j].rank
		}
		return cands[i].r.Namespace == r.Namespace && cands[j].r.Namespace != r.Namespace
	})
	var declared []string
	for i := len(cands) - 1; i >= 0; i-- { // the base declaration first in the excerpt
		c := cands[i]
		declared = append(declared, fmt.Sprintf("%s:L%d", c.r.Doc.File, c.r.Doc.StartLine))
	}
	// the object itself: a record of the rendered object (an unset field's
	// evidence), naming the files that declare and patch it
	obj := domain.NewEvidence(domain.EvidenceRendered, EffectiveProducer, "render:"+s.Target,
		fmt.Sprintf("L%d %s", r.Doc.StartLine, resourceID(r)),
		domain.TruncateExcerpt(renderedExcerpt(r, s.Target, declared)), s.Digest, zeroTime)
	r.Evidence = obj.ID
	r.records[obj.ID] = obj

	for i := range r.Fields {
		f := &r.Fields[i]
		if f.Withheld != "" {
			continue // a withheld value cannot be compared: the render is its source
		}
		if sf, ok := sourceField(cands, f); ok {
			f.Evidence, f.Line = sf.Evidence, sf.Line
		}
	}
	for i := range r.Texts {
		b := &r.Texts[i]
		for j := range b.Lines {
			ln := &b.Lines[j]
			if ln.Withheld {
				continue
			}
			if sl, ok := sourceLine(cands, b.Path, ln.Text); ok {
				ln.Evidence, ln.FileLine = sl.Evidence, sl.FileLine
			}
		}
	}
	for i := range r.Refs {
		ref := &r.Refs[i]
		if sr, ok := sourceRef(cands, ref); ok {
			ref.Evidence, ref.Line = sr.Evidence, sr.Line
		}
	}
}

func renderedExcerpt(r *Resource, target string, declared []string) string {
	s := fmt.Sprintf("rendered object %s %s (render %s)", apiVersionOf(r.Group, r.Version), resourceID(r), target)
	if len(declared) > 0 {
		s += "; declared/patched in " + strings.Join(declared, ", ")
	} else {
		s += "; no environment file declares it (generated by the render)"
	}
	return s
}

// sourceField is the fact of the latest layer that states f's path with f's
// value (a container: that states the path), preferring the same element.
func sourceField(cands []sourceDoc, f *FieldFact) (FieldFact, bool) {
	for _, c := range cands {
		var hit *FieldFact
		for i := range c.r.Fields {
			sf := &c.r.Fields[i]
			if sf.Path != f.Path || sf.Container != f.Container || sf.Withheld != "" || (!f.Container && sf.Value != f.Value) {
				continue
			}
			if sf.Element == f.Element {
				return *sf, true
			}
			if hit == nil {
				hit = sf
			}
		}
		if hit != nil {
			return *hit, true
		}
	}
	return FieldFact{}, false
}

func sourceLine(cands []sourceDoc, path, text string) (TextLine, bool) {
	for _, c := range cands {
		for _, b := range c.r.Texts {
			if b.Path != path {
				continue
			}
			for _, ln := range b.Lines {
				if !ln.Withheld && ln.Text == text {
					return ln, true
				}
			}
		}
	}
	return TextLine{}, false
}

func sourceRef(cands []sourceDoc, ref *Ref) (Ref, bool) {
	for _, c := range cands {
		for _, sr := range c.r.Refs {
			if sr.Path == ref.Path && sr.Name == ref.Name && sr.Kind == ref.Kind && sr.Group == ref.Group {
				return sr, true
			}
		}
	}
	return Ref{}, false
}

// sameObjectName: the rendered name is the declared one, possibly with a
// kustomize name prefix/suffix.
func sameObjectName(rendered, declared string) bool {
	return declared != "" && (rendered == declared || strings.Contains(rendered, declared))
}

// resourceEvidence returns the records r cites that its pool knows.
func resourceEvidence(r *Resource) []domain.Evidence {
	if r.records == nil {
		return nil
	}
	var ids []domain.EvidenceID
	ids = append(ids, r.Evidence)
	for _, f := range r.Fields {
		ids = append(ids, f.Evidence...)
	}
	for _, b := range r.Texts {
		for _, ln := range b.Lines {
			ids = append(ids, ln.Evidence)
		}
	}
	for _, ref := range r.Refs {
		ids = append(ids, ref.Evidence...)
	}
	var out []domain.Evidence
	for _, id := range ids {
		if x, ok := r.records[id]; ok {
			out = append(out, x)
		}
	}
	return out
}

func resourceID(r *Resource) string {
	s := r.Kind
	switch {
	case r.Namespace != "":
		s += " " + r.Namespace + "/" + r.Name
	case r.Name != "":
		s += " " + r.Name
	}
	return s
}

func normPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return filepath.Clean(p)
}

// index rebuilds the path indexes FieldValues uses.
func (r *Resource) index() {
	r.byPath, r.byElement = map[string][]int{}, map[string][]int{}
	for i, f := range r.Fields {
		r.byPath[f.Path] = append(r.byPath[f.Path], i)
		r.byElement[f.Element] = append(r.byElement[f.Element], i)
	}
}
