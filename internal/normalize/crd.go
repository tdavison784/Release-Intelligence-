package normalize

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// MaxSchemaPaths bounds CRDVersionInfo.SchemaPaths of one CRD version. When a
// schema has more paths, the first MaxSchemaPaths in depth-first order with
// property names visited alphabetically are kept (deterministic); the final
// list is sorted.
const MaxSchemaPaths = 10000

// maxSchemaDepth bounds the recursion into nested schemas.
const maxSchemaDepth = 64

// crdSnapshot summarises the CustomResourceDefinitions found in the given
// multi-document YAML streams. Documents that are not CRDs are ignored; a
// "List" document is searched for CRDs in .items. Supported: apiextensions.k8s.io
// v1 (spec.versions[].schema.openAPIV3Schema) and v1beta1 (spec.versions[] with
// optional per-version schema, or the legacy spec.version + spec.validation).
// When the same CRD name occurs twice the first occurrence wins. The result is
// sorted by name.
//
// SchemaPaths are dotted property paths of the openAPIV3Schema below the root
// ("spec.issuerRef.name"). An array property carries a "[]" suffix
// ("spec.dnsNames[]") and properties of its items are addressed through it
// ("spec.foo[].bar"). x-kubernetes-* keys are skipped and additionalProperties
// (free-form maps) are not descended into, so a map property is a leaf.
func crdSnapshot(streams ...[]byte) (*domain.CRDSnapshot, error) {
	byName := map[string]domain.CRDSummary{}
	var errs []error
	for si, s := range streams {
		dec := yaml.NewDecoder(bytes.NewReader(s))
		for {
			var doc yaml.Node
			err := dec.Decode(&doc)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("stream %d: %w", si, err))
				break
			}
			if len(doc.Content) == 0 {
				continue
			}
			collectCRDs(doc.Content[0], byName)
		}
	}
	snap := &domain.CRDSnapshot{CRDs: []domain.CRDSummary{}}
	for _, name := range mapKeys(byName) {
		snap.CRDs = append(snap.CRDs, byName[name])
	}
	if len(errs) > 0 && len(snap.CRDs) == 0 {
		return nil, fmt.Errorf("normalize: parse CRDs: %w", errors.Join(errs...))
	}
	return snap, nil
}

func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func collectCRDs(n *yaml.Node, out map[string]domain.CRDSummary) {
	n = resolveAlias(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return
	}
	kind := scalarField(n, "kind")
	if strings.HasSuffix(kind, "List") {
		if items := field(n, "items"); items != nil && items.Kind == yaml.SequenceNode {
			for _, it := range items.Content {
				collectCRDs(it, out)
			}
		}
		return
	}
	if kind != "CustomResourceDefinition" || !strings.HasPrefix(scalarField(n, "apiVersion"), "apiextensions.k8s.io/") {
		return
	}
	meta := field(n, "metadata")
	spec := field(n, "spec")
	if meta == nil || spec == nil {
		return
	}
	name := scalarField(meta, "name")
	if name == "" {
		return
	}
	if _, dup := out[name]; dup {
		return
	}
	sum := domain.CRDSummary{
		Name:     name,
		Group:    scalarField(spec, "group"),
		Scope:    scalarField(spec, "scope"),
		Versions: []domain.CRDVersionInfo{},
	}
	if names := field(spec, "names"); names != nil {
		sum.Kind = scalarField(names, "kind")
	}
	topSchema := openAPISchema(field(spec, "validation"))
	if versions := field(spec, "versions"); versions != nil && versions.Kind == yaml.SequenceNode {
		for _, v := range versions.Content {
			v = resolveAlias(v)
			if v == nil || v.Kind != yaml.MappingNode {
				continue
			}
			info := domain.CRDVersionInfo{
				Name:               scalarField(v, "name"),
				Served:             boolField(v, "served"),
				Storage:            boolField(v, "storage"),
				Deprecated:         boolField(v, "deprecated"),
				DeprecationWarning: scalarField(v, "deprecationWarning"),
			}
			schema := openAPISchema(field(v, "schema"))
			if schema == nil {
				schema = topSchema
			}
			info.SchemaPaths = schemaPaths(schema)
			sum.Versions = append(sum.Versions, info)
		}
	} else if ver := scalarField(spec, "version"); ver != "" {
		// legacy v1beta1 single-version CRD
		sum.Versions = append(sum.Versions, domain.CRDVersionInfo{
			Name: ver, Served: true, Storage: true, SchemaPaths: schemaPaths(topSchema),
		})
	}
	out[name] = sum
}

func openAPISchema(holder *yaml.Node) *yaml.Node {
	if holder == nil {
		return nil
	}
	return field(holder, "openAPIV3Schema")
}

// field returns the value node of key in mapping n (nil when absent).
func field(n *yaml.Node, key string) *yaml.Node {
	n = resolveAlias(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return resolveAlias(n.Content[i+1])
		}
	}
	return nil
}

func scalarField(n *yaml.Node, key string) string {
	if v := field(n, key); v != nil && v.Kind == yaml.ScalarNode && v.Tag != "!!null" {
		return v.Value
	}
	return ""
}

func boolField(n *yaml.Node, key string) bool {
	return strings.EqualFold(scalarField(n, key), "true")
}

func schemaPaths(root *yaml.Node) []string {
	if root == nil {
		return nil
	}
	var paths []string
	var walk func(s *yaml.Node, prefix string, depth int)
	walk = func(s *yaml.Node, prefix string, depth int) {
		if depth > maxSchemaDepth || len(paths) >= MaxSchemaPaths {
			return
		}
		props := field(s, "properties")
		if props == nil || props.Kind != yaml.MappingNode {
			return
		}
		type kv struct {
			name string
			node *yaml.Node
		}
		var list []kv
		for i := 0; i+1 < len(props.Content); i += 2 {
			name := props.Content[i].Value
			if strings.HasPrefix(name, "x-kubernetes-") {
				continue
			}
			list = append(list, kv{name, resolveAlias(props.Content[i+1])})
		}
		sort.SliceStable(list, func(i, j int) bool { return list[i].name < list[j].name })
		for _, p := range list {
			if len(paths) >= MaxSchemaPaths {
				return
			}
			path := p.name
			if prefix != "" {
				path = prefix + "." + p.name
			}
			sub := p.node
			// arrays: "name[]", with the properties of the items below it
			for sub != nil && scalarField(sub, "type") == "array" {
				path += "[]"
				sub = field(sub, "items")
				if sub != nil && sub.Kind != yaml.MappingNode {
					sub = nil
				}
			}
			paths = append(paths, path)
			if sub != nil {
				walk(sub, path, depth+1)
			}
		}
	}
	walk(root, "", 0)
	sort.Strings(paths)
	// de-duplicate (not expected, but cheap)
	out := paths[:0]
	for i, p := range paths {
		if i == 0 || p != paths[i-1] {
			out = append(out, p)
		}
	}
	return out
}
