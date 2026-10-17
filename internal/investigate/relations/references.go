package relations

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// Side is one end of the upgrade in an environment render pair.
type Side string

const (
	SideFrom Side = "from"
	SideTo   Side = "to"
)

// Resolution says whether a reference finds its target at one side.
type Resolution string

const (
	// RefResolved: the target is an object of the side (rendered or an
	// environment file), an object a declared produces relationship says a
	// controller creates, or one Kubernetes provides in every namespace.
	RefResolved Resolution = "resolved"
	// RefDangling: nothing at the side provides the target.
	RefDangling Resolution = "dangling"
	// RefUnknown: whether the target exists cannot be decided — matched by
	// name only, its namespace is not known, a controller is only presumed
	// (by convention) to create it, or the reference is optional.
	RefUnknown Resolution = "unknown"
)

// Reference is one reference an object of the environment makes to another,
// resolved at one side of the upgrade with the namespace semantics of the
// relationship graph: a name-only reference resolves in the referrer's
// (effective) namespace, a cluster-scoped kind in no namespace, an explicit
// namespace (namespace/name formats, a *Ref's namespace) where it says.
type Reference struct {
	// Referrer is the key (render object id form) of the referring object.
	Referrer string `json:"referrer"`
	// Rendered: the referrer comes from a render (else an environment file).
	Rendered bool   `json:"rendered"`
	Artifact string `json:"artifact,omitempty"`
	// Field is the referrer's field holding the name; Rule the rule that
	// reads it (graph.k8s-refs@v1:…, relationship:<product>/<id>).
	Field string `json:"field"`
	Rule  string `json:"rule"`
	// The target: group ("" core; GroupStated false when the reference does
	// not say), kind, namespace ("" for a cluster-scoped kind or when it is
	// not known), name.
	Group       string `json:"group,omitempty"`
	GroupStated bool   `json:"groupStated"`
	Kind        string `json:"kind"`
	Namespace   string `json:"namespace,omitempty"`
	Name        string `json:"name"`
	Cluster     bool   `json:"cluster,omitempty"`
	// Optional: the referrer tolerates the target's absence.
	Optional   bool       `json:"optional,omitempty"`
	Resolution Resolution `json:"resolution"`
	// Target is the key of the object it resolves to (RefResolved).
	Target string `json:"target,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// TargetString names the target: "Secret ns/name", "ClusterIssuer name".
func (r Reference) TargetString() string {
	if r.Namespace == "" {
		return r.Kind + " " + r.Name
	}
	return fmt.Sprintf("%s %s/%s", r.Kind, r.Namespace, r.Name)
}

// Identity is the same for one reference at both sides: the referrer, its
// field and the target it names.
func (r Reference) Identity() string {
	return r.Referrer + "|" + r.Field + "|" + r.Kind + "|" + r.Namespace + "|" + r.Name
}

// TargetIdentity names the target alone (kind, namespace, name): the same
// at both sides whatever refers to it, from which field or list position.
func (r Reference) TargetIdentity() string {
	return r.Kind + "|" + r.Namespace + "|" + r.Name
}

// TargetMatches reports whether the reference names the object with key
// (render object id form), ignoring the API version (a reference never
// states it).
func (r Reference) TargetMatches(key string) bool {
	id, ok := parseObjectKey(key)
	if !ok {
		return false
	}
	return id.Kind == r.Kind && id.Name == r.Name && (!r.GroupStated || id.Group == r.Group) &&
		(r.Cluster || id.Namespace == r.Namespace)
}

// Object is one object of the environment at a side.
type Object struct {
	Key string          `json:"key"`
	ID  render.ObjectID `json:"id"`
	// Namespace is the effective namespace (the declared one, else the one
	// kustomize or the deploying Application gives it).
	Namespace string         `json:"namespace,omitempty"`
	Body      map[string]any `json:"-"`
	Rendered  bool           `json:"rendered"`
	Artifact  string         `json:"artifact,omitempty"`
}

// View is the environment at one side of the upgrade: its objects (the
// side's render of every complete environment pair plus the environment
// files) and every reference among them, resolved.
type View struct {
	Side       Side        `json:"side"`
	Objects    []Object    `json:"objects"`
	References []Reference `json:"references"`
}

// Viewer builds Views with the relationship rules of the graph builder.
type Viewer struct{ bd *builder }

// NewViewer returns a Viewer; st serves the bundle's artifacts, opts are the
// graph builder's (WithCatalog, WithRelationships, …).
func NewViewer(st investigate.ArtifactStore, opts ...Option) *Viewer {
	bd := &builder{st: st}
	for _, o := range opts {
		o(bd)
	}
	return &Viewer{bd: bd}
}

// View reads the environment at one side and resolves its references.
func (v *Viewer) View(ctx context.Context, b *domain.EvidenceBundle, side Side) (*View, error) {
	if b == nil {
		return nil, fmt.Errorf("relations: nil bundle: %w", investigate.ErrRejected)
	}
	if side != SideFrom && side != SideTo {
		return nil, fmt.Errorf("relations: side %q: %w", side, investigate.ErrRejected)
	}
	rv := &refView{}
	g, err := v.bd.build(ctx, b, side, rv)
	if err != nil {
		return nil, err
	}
	out := &View{Side: side, Objects: []Object{}, References: rv.refs}
	for _, n := range g.sortedNodes() {
		if o := n.obj; o != nil && g.objNode[o] == n {
			out.Objects = append(out.Objects, Object{Key: o.key(), ID: o.id, Namespace: o.effNS, Body: o.body, Rendered: o.rendered, Artifact: o.artifact})
		}
	}
	sort.Slice(out.References, func(i, j int) bool {
		a, b := out.References[i], out.References[j]
		if a.Referrer != b.Referrer {
			return a.Referrer < b.Referrer
		}
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		return a.Identity() < b.Identity()
	})
	return out, nil
}

// refView collects references while the graph is built.
type refView struct{ refs []Reference }

// sensitiveMark starts a value internal/render replaced by its digest.
const sensitiveMark = "<sensitive "

// providedEverywhere are objects Kubernetes creates in every namespace.
var providedEverywhere = map[string]bool{"ServiceAccount|default": true, "ConfigMap|kube-root-ca.crt": true}

func (v *refView) record(g *gb, referrer *gnode, group string, groupStated bool, kind, name, ns, rule, field string, optional bool,
	target *gnode, status domain.EdgeStatus, reason string) {
	cluster := g.env.cluster[kind]
	if cluster {
		ns = ""
	}
	if !groupStated {
		group = ""
	}
	r := Reference{Referrer: referrer.n.Key, Field: field, Rule: rule, Group: group, GroupStated: groupStated,
		Kind: kind, Namespace: ns, Name: name, Cluster: cluster, Optional: optional}
	if o := referrer.obj; o != nil {
		r.Rendered, r.Artifact = o.rendered, o.artifact
	}
	switch status {
	case domain.EdgeEstablished:
		r.Resolution, r.Target = RefResolved, target.n.Key
		if target.n.Placeholder {
			r.Reason = "not an object of the snapshot; a declared produces relationship says a controller creates it"
		}
	case domain.EdgeInferred:
		r.Resolution, r.Reason = RefUnknown, reason
	default:
		r.Resolution, r.Reason = RefDangling, reason
	}
	if r.Resolution == RefDangling {
		switch {
		case strings.HasPrefix(name, sensitiveMark):
			// the render replaced the value of a credential-named key (an
			// annotation ending in "secret") by its digest: the name is withheld
			r.Resolution, r.Reason = RefUnknown, "the render withholds the name (its key looks like a credential)"
		case (!groupStated || group == "") && providedEverywhere[kind+"|"+name]:
			r.Resolution, r.Reason = RefResolved, "Kubernetes provides it in every namespace"
		case !cluster && ns == "":
			r.Resolution, r.Reason = RefUnknown, fmt.Sprintf("%s %q: the namespace it is looked up in is not known", kind, name)
		case optional:
			r.Resolution, r.Reason = RefUnknown, "optional reference: the referrer tolerates its absence — "+reason
		}
	}
	v.refs = append(v.refs, r)
}
