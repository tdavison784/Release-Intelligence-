package runtime

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ObjectRef identifies a Kubernetes object. Its String form is the render
// object id and graph node key, "<group>/<version>/<kind>/<namespace>/<name>"
// with "core" for the core group and "" for no namespace.
type ObjectRef struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

func (r ObjectRef) String() string {
	g := r.Group
	if g == "" {
		g = "core"
	}
	return g + "/" + r.Version + "/" + r.Kind + "/" + r.Namespace + "/" + r.Name
}

// APIVersion renders group/version as apiVersion.
func (r ObjectRef) APIVersion() string {
	if r.Group == "" {
		return r.Version
	}
	return r.Group + "/" + r.Version
}

// GVK is the type part of the ref.
func (r ObjectRef) GVK() GVK { return GVK{Group: r.Group, Version: r.Version, Kind: r.Kind} }

// GVK names an object type.
type GVK struct{ Group, Version, Kind string }

func (g GVK) String() string {
	return ObjectRef{Group: g.Group, Version: g.Version, Kind: g.Kind}.String()
}

// typeKey is the version-independent type identity (an object keeps its
// identity across an API version migration).
func (r ObjectRef) typeKey() string { return r.Group + "|" + r.Kind }

// ParseRef parses an object id / graph node key; ok is false for keys that
// are not Kubernetes objects (api types, values sources, releases).
func ParseRef(key string) (ObjectRef, bool) {
	parts := strings.Split(key, "/")
	if len(parts) != 5 || parts[1] == "" || parts[2] == "" || parts[4] == "" {
		return ObjectRef{}, false
	}
	g := parts[0]
	if g == "core" {
		g = ""
	}
	return ObjectRef{Group: g, Version: parts[1], Kind: parts[2], Namespace: parts[3], Name: parts[4]}, true
}

// refOf reads the identity of a raw object.
func refOf(obj map[string]any) (ObjectRef, bool) {
	apiVersion, _ := obj["apiVersion"].(string)
	kind, _ := obj["kind"].(string)
	meta, _ := obj["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	if apiVersion == "" || kind == "" || name == "" {
		return ObjectRef{}, false
	}
	ns, _ := meta["namespace"].(string)
	g, v := "", apiVersion
	if i := strings.LastIndexByte(apiVersion, '/'); i >= 0 {
		g, v = apiVersion[:i], apiVersion[i+1:]
	}
	return ObjectRef{Group: g, Version: v, Kind: kind, Namespace: ns, Name: name}, true
}

// Manifest is one raw object to apply.
type Manifest struct {
	Ref ObjectRef      `json:"ref"`
	Doc map[string]any `json:"-"`
	// Target is the render target the object came from; Product marks the
	// targets that deploy the product (ok pairs), as opposed to consumer
	// overlays (not-applicable pairs).
	Target  string `json:"target"`
	Product bool   `json:"product,omitempty"`
	// Hook marks a Helm hook (re-created on every apply, like helm and the
	// GitOps controllers do with the default before-hook-creation policy).
	Hook bool `json:"hook,omitempty"`
	// Namespace is the namespace a namespaced object without one is applied
	// to (the target's release namespace).
	Namespace string `json:"namespace,omitempty"`
}

// YAML encodes the raw object.
func (m Manifest) YAML() ([]byte, error) { return yaml.Marshal(m.Doc) }

// parseManifests splits a raw rendered stream into objects. Lists are
// expanded; documents without apiVersion/kind are skipped; generateName
// objects cannot be applied idempotently and are reported.
func parseManifests(stream []byte, target, namespace string, product bool) (out []Manifest, skipped []string, err error) {
	dec := yaml.NewDecoder(strings.NewReader(string(stream)))
	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, nil, fmt.Errorf("render %s: %w", target, err)
		}
		if doc == nil {
			continue
		}
		docs := []map[string]any{doc}
		if k, _ := doc["kind"].(string); strings.HasSuffix(k, "List") && doc["items"] != nil {
			docs = nil
			items, _ := doc["items"].([]any)
			for _, it := range items {
				if m, ok := it.(map[string]any); ok {
					docs = append(docs, m)
				}
			}
		}
		for _, d := range docs {
			if d["apiVersion"] == nil && d["kind"] == nil {
				continue
			}
			ref, ok := refOf(d)
			if !ok {
				skipped = append(skipped, fmt.Sprintf("%s: an object without apiVersion, kind or name (generateName objects cannot be applied idempotently)", target))
				continue
			}
			ann := annotations(d)
			hook := ann["helm.sh/hook"]
			if strings.Contains(hook, "test") {
				skipped = append(skipped, fmt.Sprintf("%s: %s is a helm test hook (never part of an install or upgrade)", target, ref))
				continue
			}
			out = append(out, Manifest{Ref: ref, Doc: d, Target: target, Product: product, Hook: hook != "", Namespace: namespace})
		}
	}
	return out, skipped, nil
}

func annotations(obj map[string]any) map[string]string {
	meta, _ := obj["metadata"].(map[string]any)
	raw, _ := meta["annotations"].(map[string]any)
	out := map[string]string{}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// manifestSet is the environment at one version: every object, by render id.
type manifestSet struct {
	byID  map[string]Manifest
	notes []string
}

func newManifestSet() *manifestSet { return &manifestSet{byID: map[string]Manifest{}} }

func (s *manifestSet) add(ms []Manifest) {
	for _, m := range ms {
		id := m.Ref.String()
		if prev, dup := s.byID[id]; dup {
			a, _ := json.Marshal(prev.Doc)
			b, _ := json.Marshal(m.Doc)
			if string(a) != string(b) {
				s.notes = append(s.notes, fmt.Sprintf("%s is rendered by %s and differently by %s; the latter is applied (last wins, as with two GitOps applications owning one object)", id, prev.Target, m.Target))
			}
			if prev.Product && !m.Product {
				m.Product = true
			}
		}
		s.byID[id] = m
	}
}

// list returns the manifests in a stable order.
func (s *manifestSet) list() []Manifest {
	out := make([]Manifest, 0, len(s.byID))
	for _, m := range s.byID {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref.String() < out[j].Ref.String() })
	return out
}

// isCRD reports whether a manifest defines a custom resource type.
func isCRD(r ObjectRef) bool {
	return r.Group == "apiextensions.k8s.io" && r.Kind == "CustomResourceDefinition"
}

// crdTypes returns "<group>|<kind>" of every type a CRD manifest defines.
func crdType(m Manifest) (string, bool) {
	spec, _ := m.Doc["spec"].(map[string]any)
	group, _ := spec["group"].(string)
	names, _ := spec["names"].(map[string]any)
	kind, _ := names["kind"].(string)
	if group == "" || kind == "" {
		return "", false
	}
	return group + "|" + kind, true
}

// Redaction markers the bundle and render lanes put in place of sensitive
// names and values; graph node keys can carry them (a Secret named by a
// field whose key looks sensitive). A marker is resolved against live object
// names by recomputing it.
const (
	bundleMarkerPrefix = "<redacted sha256:"  // internal/investigate/bundle: sha256(value)[:12]
	renderMarkerPrefix = "<sensitive sha256:" // internal/render: sha256(json(value))[:16]
)

// markerMatches reports whether name is the value a redaction marker hides.
func markerMatches(marker, name string) bool {
	switch {
	case strings.HasPrefix(marker, bundleMarkerPrefix):
		sum := sha256.Sum256([]byte(name))
		return marker == bundleMarkerPrefix+hex.EncodeToString(sum[:])[:12]+">"
	case strings.HasPrefix(marker, renderMarkerPrefix):
		b, _ := json.Marshal(name)
		sum := sha256.Sum256(b)
		return marker == renderMarkerPrefix+hex.EncodeToString(sum[:])[:16]+">"
	}
	return false
}

func isMarker(s string) bool {
	return strings.HasPrefix(s, bundleMarkerPrefix) || strings.HasPrefix(s, renderMarkerPrefix)
}

// digestValue digests one Secret/ConfigMap value with the run's key:
// observations record keyed digests, never values. Only equality within a
// run matters, so the key is random per run and never stored — a recorded
// digest cannot be checked against a guessed value.
func digestValue(key []byte, v any) string {
	var b []byte
	switch t := v.(type) {
	case string:
		b = []byte(t)
	default:
		b, _ = json.Marshal(t)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(b)
	return "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil))
}
