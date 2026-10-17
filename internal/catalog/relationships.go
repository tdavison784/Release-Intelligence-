package catalog

import (
	"fmt"
	"net/url"
	"strings"
)

// RelationshipKind says how one object relates to another.
type RelationshipKind string

const (
	// RelProduces: an object of From names, at Path, an object of To that its
	// controller creates (a cert-manager Certificate's spec.secretName).
	RelProduces RelationshipKind = "produces"
	// RelReferences: an object of From names, at Path, an existing object of
	// To that it reads (an Issuer's spec.ca.secretName).
	RelReferences RelationshipKind = "references"
	// RelSelects: an object of From selects objects of To by label
	// (a ServiceMonitor's spec.selector).
	RelSelects RelationshipKind = "selects"
)

// RelationshipKinds lists every kind.
var RelationshipKinds = []RelationshipKind{RelProduces, RelReferences, RelSelects}

// Relationship formats of a name found at Path.
const (
	RelFormatName          = "name"
	RelFormatNamespaceName = "namespace/name"
	// RelFormatOptionalNamespace: "name" (in the namespace of From) or
	// "namespace/name" (ingress-nginx's auth-secret).
	RelFormatOptionalNamespace = "[namespace/]name"
)

// ObjectType identifies a Kubernetes kind (Group "" is the core group).
type ObjectType struct {
	Group string `yaml:"group,omitempty" json:"group,omitempty"`
	Kind  string `yaml:"kind" json:"kind"`
}

// Relationship is a declarative, upstream-documented relationship between
// Kubernetes object kinds that the investigation's relationship graph cannot
// know from field semantics alone (internal/investigate/relations). It is a
// release-level fact: References are required and must be upstream
// documentation, never an evaluation case.
type Relationship struct {
	ID   string           `yaml:"id" json:"id"`
	Kind RelationshipKind `yaml:"kind" json:"kind"`
	// From is the object kind that carries the field(s); To the kind it
	// produces, references or selects.
	From ObjectType `yaml:"from" json:"from"`
	To   ObjectType `yaml:"to" json:"to"`
	// Summary says in one sentence what the relationship is.
	Summary string `yaml:"summary" json:"summary"`
	// Path is the field of From holding the name (produces, references):
	// dotted keys, a key containing dots or slashes in brackets, e.g.
	// spec.secretName or metadata.annotations[example.io/secret]; `[]` after a
	// key stands for every element of that list (spec.endpoints[].tlsConfig.ca.secret.name).
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
	// Format of the name at Path: "name" (default; the namespace is the
	// namespace of From), "namespace/name", or "[namespace/]name" (either).
	Format string `yaml:"format,omitempty" json:"format,omitempty"`
	// Scope of To: "namespaced" (default) or "cluster".
	Scope string `yaml:"scope,omitempty" json:"scope,omitempty"`
	// SelectorPath is the field of From holding a Kubernetes LabelSelector
	// (selects). NamespaceSelectorPath, optionally, holds a prometheus-operator
	// style {any, matchNames} namespace selector; without it To is looked
	// for in the namespace of From. To is Service (metadata.labels) or Pod
	// (the pod template labels of workloads, or a Pod's labels).
	SelectorPath          string `yaml:"selectorPath,omitempty" json:"selectorPath,omitempty"`
	NamespaceSelectorPath string `yaml:"namespaceSelectorPath,omitempty" json:"namespaceSelectorPath,omitempty"`
	// WhenPath/WhenEquals restrict a produces/references relationship to
	// objects whose field at WhenPath equals WhenEquals (cert-manager's
	// spec.issuerRef.kind == ClusterIssuer).
	WhenPath   string `yaml:"whenPath,omitempty" json:"whenPath,omitempty"`
	WhenEquals string `yaml:"whenEquals,omitempty" json:"whenEquals,omitempty"`
	// References are the upstream documentation the entry rests on.
	References []ConfigReference `yaml:"references" json:"references"`
}

// SplitFieldPath parses a relationship field path into keys: dot-separated
// keys, or a key in brackets (which may contain dots and slashes).
func SplitFieldPath(p string) ([]string, error) { return splitFieldPath(p, false) }

// ListElements is the key SplitListFieldPath returns for `[]`.
const ListElements = "[]"

// SplitListFieldPath is SplitFieldPath that also accepts `[]` after a key:
// every element of that list (returned as the key ListElements). Only a
// relationship's Path may iterate lists.
func SplitListFieldPath(p string) ([]string, error) { return splitFieldPath(p, true) }

func splitFieldPath(p string, lists bool) ([]string, error) {
	bad := func(why string) ([]string, error) { return nil, fmt.Errorf("field path %q: %s", p, why) }
	if p == "" || strings.TrimSpace(p) != p {
		return bad("empty or surrounded by whitespace")
	}
	var out []string
	i := 0
	for i < len(p) {
		switch {
		case lists && strings.HasPrefix(p[i:], "[]") && len(out) > 0 && out[len(out)-1] != ListElements && p[i-1] != '.':
			out = append(out, ListElements)
			i += 2
		case p[i] == '[':
			j := strings.IndexByte(p[i:], ']')
			if j < 2 {
				return bad("unterminated or empty [key]")
			}
			out = append(out, p[i+1:i+j])
			i += j + 1
		default:
			j := i
			for j < len(p) && p[j] != '.' && p[j] != '[' && p[j] != ']' {
				j++
			}
			if j == i {
				return bad("empty key")
			}
			out = append(out, p[i:j])
			i = j
		}
		if i < len(p) {
			switch p[i] {
			case '.':
				i++
				if i == len(p) {
					return bad("trailing dot")
				}
			case '[':
			default:
				return bad("expected . or [ after a key")
			}
		}
	}
	return out, nil
}

func (v *validator) relationships(d *ProductDefinition) {
	seen := map[string]bool{}
	for i, r := range d.Relationships {
		p := fmt.Sprintf("relationships[%d]", i)
		switch {
		case !configSourceIDRe.MatchString(r.ID):
			v.errf(p+".id", "must be a lowercase DNS label, got %q", r.ID)
		case seen[r.ID]:
			v.errf(p+".id", "duplicate relationship id %q", r.ID)
		}
		seen[r.ID] = true
		known := false
		for _, k := range RelationshipKinds {
			known = known || k == r.Kind
		}
		if !known {
			v.errf(p+".kind", "unknown kind %q (one of %v)", r.Kind, RelationshipKinds)
		}
		if strings.TrimSpace(r.From.Kind) == "" {
			v.errf(p+".from.kind", "required")
		}
		if strings.TrimSpace(r.To.Kind) == "" {
			v.errf(p+".to.kind", "required")
		}
		if strings.TrimSpace(r.Summary) == "" {
			v.errf(p+".summary", "required")
		}
		for _, f := range []struct{ name, val string }{{"path", r.Path}, {"selectorPath", r.SelectorPath}, {"namespaceSelectorPath", r.NamespaceSelectorPath}, {"whenPath", r.WhenPath}} {
			if f.val != "" {
				split := SplitFieldPath
				if f.name == "path" {
					split = SplitListFieldPath
				}
				if _, err := split(f.val); err != nil {
					v.errf(p+"."+f.name, "%v", err)
				}
			}
		}
		if (r.WhenPath == "") != (r.WhenEquals == "") {
			v.errf(p+".whenPath", "whenPath and whenEquals go together")
		}
		if r.Kind == RelSelects && r.WhenPath != "" {
			v.errf(p+".whenPath", "not a field of kind selects")
		}
		switch r.Kind {
		case RelProduces, RelReferences:
			if r.Path == "" {
				v.errf(p+".path", "required for kind %s", r.Kind)
			}
			if r.SelectorPath != "" || r.NamespaceSelectorPath != "" {
				v.errf(p+".selectorPath", "not a field of kind %s", r.Kind)
			}
		case RelSelects:
			if r.SelectorPath == "" {
				v.errf(p+".selectorPath", "required for kind selects")
			}
			if r.Path != "" || r.Format != "" || r.Scope != "" {
				v.errf(p+".path", "path, format and scope are not fields of kind selects")
			}
			if r.To.Group != "" || (r.To.Kind != "Service" && r.To.Kind != "Pod") {
				v.errf(p+".to", "a selects relationship targets the core kinds Service or Pod, got %q", r.To.Group+"/"+r.To.Kind)
			}
		}
		if r.Format != "" && r.Format != RelFormatName && r.Format != RelFormatNamespaceName && r.Format != RelFormatOptionalNamespace {
			v.errf(p+".format", "must be %q, %q or %q, got %q", RelFormatName, RelFormatNamespaceName, RelFormatOptionalNamespace, r.Format)
		}
		if r.Scope != "" && r.Scope != "namespaced" && r.Scope != "cluster" {
			v.errf(p+".scope", "must be namespaced or cluster, got %q", r.Scope)
		}
		if len(r.References) == 0 {
			v.errf(p+".references", "at least one upstream documentation reference is required")
		}
		for j, ref := range r.References {
			rp := fmt.Sprintf("%s.references[%d]", p, j)
			u, err := url.Parse(ref.URL)
			if err != nil || u.Scheme != "https" || u.Host == "" {
				v.errf(rp+".url", "must be an https URL of upstream documentation, got %q", ref.URL)
			}
			if strings.Contains(ref.URL, "eval/cases") || strings.Contains(ref.URL, "eval/results") || strings.Contains(ref.URL, "eval/investigations") {
				v.errf(rp+".url", "must not cite evaluation data")
			}
			if q := strings.TrimSpace(ref.Quote); q == "" || len(q) > maxConfigQuote {
				v.errf(rp+".quote", "a short verbatim quote (1-%d characters) is required", maxConfigQuote)
			}
		}
	}
}
