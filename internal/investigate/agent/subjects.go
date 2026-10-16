package agent

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Subject resolution (prompt feature resolveSubjects). A model names the
// thing it wants checked the way the evidence names it — a values key, a
// controller flag, an annotation, an object, a file, a rendered change — and
// a validator needs graph nodes. Each subject is resolved by the first rule
// that matches, deterministically; anything else is rejected with the list
// of accepted forms.
const (
	SubjectNodeID          = "subject:node-id"          // gn-…
	SubjectNodeKey         = "subject:node-key"         // <group>/<version>/<kind>/<namespace>/<name>
	SubjectRenderedChange  = "subject:rendered-change"  // rc-… → the node of its object
	SubjectArtifact        = "subject:artifact"         // art-… or an environment file path → nodes carrying that file
	SubjectObjectRef       = "subject:object-ref"       // Kind/name or Kind/namespace/name
	SubjectValuesKey       = "subject:values-key"       // [values:]a.b.c → the values files that set it, else the product's values files
	SubjectRenderedSetting = "subject:rendered-setting" // a flag, annotation, env var or config key in a complete environment render → those objects
)

// maxSubjectMatches bounds how many nodes one subject may resolve to.
const maxSubjectMatches = 20

// SubjectResolution says how one named subject was resolved.
type SubjectResolution struct {
	Given string   `json:"given"`
	Rule  string   `json:"rule"`
	Nodes []string `json:"nodes"`
	Note  string   `json:"note,omitempty"`
}

const subjectForms = "accepted subject forms: a node id (gn-…); a node key <group>/<version>/<kind>/<namespace>/<name> (core for the core group); " +
	"a rendered change id (rc-…); an environment file (art-… or its path, e.g. env/apps/x/values.yaml); an object reference Kind/name or Kind/namespace/name; " +
	"a Helm values key (controller.replicaCount, optionally values:controller.replicaCount); a setting as it appears in a complete environment render " +
	"(a container flag such as --enable-x, an annotation key, an environment variable or config key)"

var valuesKeyRe = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)+$`)

// resolveSubjects resolves every subject; the node ids are sorted and unique.
func (s *Session) resolveSubjects(ctx context.Context, what string, refs []string) ([]string, []SubjectResolution, error) {
	if !s.cfg.Prompt.Features.ResolveSubjects {
		ids, err := s.nodeIDs(what, refs)
		return ids, nil, err
	}
	if len(refs) > 0 && s.cfg.Graph == nil {
		return nil, nil, rejectf("%s: this investigation has no relationship graph", what)
	}
	var all []string
	var res []SubjectResolution
	for _, ref := range refs {
		r, err := s.resolveSubject(ctx, strings.TrimSpace(ref))
		if err != nil {
			return nil, nil, err
		}
		if len(r.Nodes) == 0 {
			return nil, nil, rejectf("%s %q: %s; %s", what, ref, r.Note, subjectForms)
		}
		if len(r.Nodes) > maxSubjectMatches {
			return nil, nil, rejectf("%s %q matches %d nodes by %s (max %d): name it more precisely; %s", what, ref, len(r.Nodes), r.Rule, maxSubjectMatches, subjectForms)
		}
		res = append(res, r)
		all = append(all, r.Nodes...)
	}
	return uniqSorted(all), res, nil
}

func (s *Session) resolveSubject(ctx context.Context, ref string) (SubjectResolution, error) {
	r := SubjectResolution{Given: ref}
	if ref == "" {
		r.Note = "empty subject"
		return r, nil
	}
	if _, ok := s.nodes[ref]; ok {
		r.Rule, r.Nodes = SubjectNodeID, []string{ref}
		return r, nil
	}
	if id, ok := s.nodeKey[ref]; ok {
		r.Rule, r.Nodes = SubjectNodeKey, []string{id}
		return r, nil
	}
	if rc, ok := s.rcs[ref]; ok {
		r.Rule = SubjectRenderedChange
		if id, ok := s.nodeKey[rc.Object]; ok {
			r.Nodes = []string{id}
		} else {
			r.Note = fmt.Sprintf("rendered change %s is on %s, which has no graph node", ref, rc.Object)
		}
		return r, nil
	}
	if nodes := s.nodesOfFile(ref); len(nodes) > 0 {
		r.Rule, r.Nodes = SubjectArtifact, nodes
		return r, nil
	}
	if nodes := s.nodesOfObjectRef(ref); len(nodes) > 0 {
		r.Rule, r.Nodes = SubjectObjectRef, nodes
		return r, nil
	}
	if key, isValues := strings.CutPrefix(ref, "values:"); isValues || valuesKeyRe.MatchString(ref) {
		nodes, note, err := s.nodesOfValuesKey(ctx, key)
		if err != nil {
			return r, err
		}
		if len(nodes) > 0 || isValues {
			r.Rule, r.Nodes, r.Note = SubjectValuesKey, nodes, note
			return r, nil
		}
	}
	nodes, err := s.nodesOfRenderedSetting(ctx, ref)
	if err != nil {
		return r, err
	}
	if len(nodes) > 0 {
		r.Rule, r.Nodes = SubjectRenderedSetting, nodes
		r.Note = "objects of a complete environment render whose to-side text contains it"
		return r, nil
	}
	r.Note = "no graph node, file, object, values key or rendered setting matches it"
	return r, nil
}

// nodesOfFile: nodes that carry an environment file artifact named by id,
// logical path or repo path.
func (s *Session) nodesOfFile(ref string) []string {
	var art string
	for _, a := range s.cfg.Bundle.Manifest.Artifacts {
		if a.Origin == domain.OriginEnvironment && (a.ID == ref || a.Path == ref || a.URI == ref || a.Path == "env/"+strings.TrimPrefix(ref, "./")) {
			art = a.ID
			break
		}
	}
	if art == "" {
		return nil
	}
	var out []string
	for id, n := range s.nodes {
		if contains(n.Artifacts, art) || n.Key == "env/"+strings.TrimPrefix(ref, "env/") {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// nodesOfObjectRef: Kind/name or Kind/namespace/name (kind case-insensitive).
func (s *Session) nodesOfObjectRef(ref string) []string {
	parts := strings.Split(ref, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return nil
	}
	kind, ns, name := parts[0], "", parts[len(parts)-1]
	if len(parts) == 3 {
		ns = parts[1]
	}
	var out []string
	for id, n := range s.nodes {
		k := strings.Split(n.Key, "/")
		if len(k) != 5 || !strings.EqualFold(k[2], kind) || k[4] != name || (ns != "" && k[3] != ns) {
			continue
		}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// nodesOfValuesKey: the values-source nodes whose file sets the key; when
// none does, the values-source nodes of the product's environment render
// targets (where the key would be set) — with a note saying it is unset.
func (s *Session) nodesOfValuesKey(ctx context.Context, key string) ([]string, string, error) {
	var setIn, product []string
	targets := s.renderTargetFiles()
	for id, n := range s.nodes {
		if n.Kind != domain.NodeValuesSource || n.Placeholder {
			continue
		}
		for _, aid := range n.Artifacts {
			a, ok := s.artifact(aid)
			if !ok || a.Origin != domain.OriginEnvironment {
				continue
			}
			b, err := s.bytesOf(ctx, a)
			if err != nil {
				return nil, "", err
			}
			if yamlSets(b, key) {
				setIn = append(setIn, id)
			}
		}
		if s.configuresTarget(id, targets) {
			product = append(product, id)
		}
	}
	sort.Strings(setIn)
	sort.Strings(product)
	if len(setIn) > 0 {
		return uniqSorted(setIn), "set in the environment values files named by these nodes", nil
	}
	if len(product) > 0 {
		return uniqSorted(product), "not set by any environment values file; these are the values files of the product's render targets, where it would be set", nil
	}
	return nil, fmt.Sprintf("values key %s is not set by any environment values file and no values file of the product's render targets is in the graph", key), nil
}

// renderTargetFiles: the environment files the environment-scope render
// targets are declared in ("argocd:<file>#…", "flux:<file>", …).
func (s *Session) renderTargetFiles() map[string]bool {
	out := map[string]bool{}
	for _, p := range s.cfg.Bundle.Renders {
		_, rest, ok := strings.Cut(p.Target, ":")
		if !ok {
			continue
		}
		file, _, _ := strings.Cut(rest, "#")
		out["env/"+file] = true
	}
	return out
}

// configuresTarget: the node configures (directly) a node declared in one
// of the target files.
func (s *Session) configuresTarget(node string, files map[string]bool) bool {
	for _, e := range s.edges {
		if e.From != node || e.Kind != domain.EdgeConfigures {
			continue
		}
		for _, aid := range s.nodes[e.To].Artifacts {
			if a, ok := s.artifact(aid); ok && files[a.Path] {
				return true
			}
		}
	}
	return false
}

// yamlSets reports whether any YAML document of b sets the dotted key path.
func yamlSets(b []byte, key string) bool {
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			return false
		}
		var cur any = doc
		found := true
		for _, seg := range strings.Split(key, ".") {
			m, ok := cur.(map[string]any)
			if !ok {
				found = false
				break
			}
			if cur, ok = m[seg]; !ok {
				found = false
				break
			}
		}
		if found {
			return true
		}
	}
}

// nodesOfRenderedSetting: objects of complete environment renders (to side)
// whose stored text contains the setting as a whole token.
func (s *Session) nodesOfRenderedSetting(ctx context.Context, setting string) ([]string, error) {
	if len(setting) < 3 || strings.ContainsAny(setting, " \t\n") {
		return nil, nil
	}
	tok := regexp.MustCompile(`(^|[^A-Za-z0-9_.-])` + regexp.QuoteMeta(setting) + `($|[^A-Za-z0-9_-])`)
	var out []string
	for _, p := range s.cfg.Bundle.Renders {
		if p.Status != domain.CollectionOK || p.To == "" {
			continue
		}
		a, ok := s.artifact(p.To)
		if !ok {
			continue
		}
		b, err := s.bytesOf(ctx, a)
		if err != nil {
			return nil, err
		}
		for _, doc := range strings.Split(string(b), "\n---\n") {
			head, body, ok := strings.Cut(strings.TrimPrefix(doc, "---\n"), "\n")
			obj, isObj := strings.CutPrefix(head, "# Object: ")
			if !ok || !isObj || !tok.MatchString(body) {
				continue
			}
			if id, ok := s.nodeKey[obj]; ok {
				out = append(out, id)
			}
		}
	}
	return uniqSorted(out), nil
}
