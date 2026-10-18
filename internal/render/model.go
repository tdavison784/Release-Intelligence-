package render

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/redaction"
)

// ObjectID is the identity of a rendered Kubernetes object,
// "<group>/<version>/<kind>/<namespace>/<name>" with "core" for the core
// group and "" for an unset namespace (cluster-scoped kinds, or namespaced
// templates that rely on the release namespace).
type ObjectID struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

func (id ObjectID) String() string {
	g := id.Group
	if g == "" {
		g = "core"
	}
	return g + "/" + id.Version + "/" + id.Kind + "/" + id.Namespace + "/" + id.Name
}

// key is the version-independent identity: the same object across an API
// version migration (api-version-changed, not removed + added).
func (id ObjectID) key() string {
	return id.Group + "|" + id.Kind + "|" + id.Namespace + "|" + id.Name
}

// APIVersion renders group/version as apiVersion.
func (id ObjectID) APIVersion() string {
	if id.Group == "" {
		return id.Version
	}
	return id.Group + "/" + id.Version
}

// Object is one normalized rendered object.
type Object struct {
	ID ObjectID `json:"id"`
	// Source is the template that produced it (Helm's "# Source:" comment;
	// empty for kustomize output).
	Source string `json:"source,omitempty"`
	// Body is the normalized object (see normalizeTree): nulls and empty
	// containers dropped, sensitive values replaced by digests.
	Body map[string]any `json:"body"`
}

var sourceLine = regexp.MustCompile(`^# Source: (.+)$`)

// ParseObjects parses a rendered YAML stream into normalized objects.
// Documents without apiVersion/kind (empty documents, comments) are skipped;
// a document that is not a mapping is an error (the render is unparsable).
// List kinds (v1 List) are expanded into their items.
func ParseObjects(stream []byte) ([]Object, error) {
	var out []Object
	var cur bytes.Buffer
	source := ""
	flush := func() error {
		defer func() { cur.Reset(); source = "" }()
		if len(bytes.TrimSpace(cur.Bytes())) == 0 {
			return nil
		}
		var raw any
		if err := yaml.Unmarshal(cur.Bytes(), &raw); err != nil {
			return fmt.Errorf("rendered document (source %q): %w", source, err)
		}
		if raw == nil {
			return nil
		}
		m, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("rendered document (source %q) is not a mapping", source)
		}
		objs, err := objectsOf(m, source)
		if err != nil {
			return err
		}
		out = append(out, objs...)
		return nil
	}
	sc := bufio.NewScanner(bytes.NewReader(stream))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "---") && strings.TrimSpace(strings.TrimLeft(line, "-")) == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		if m := sourceLine.FindStringSubmatch(line); m != nil && source == "" {
			source = strings.TrimSpace(m[1])
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if err := flush(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out, nil
}

func objectsOf(m map[string]any, source string) ([]Object, error) {
	apiVersion, _ := m["apiVersion"].(string)
	kind, _ := m["kind"].(string)
	if apiVersion == "" && kind == "" {
		return nil, nil
	}
	if apiVersion == "" || kind == "" {
		return nil, fmt.Errorf("rendered document (source %q) lacks apiVersion or kind", source)
	}
	if kind == "List" || strings.HasSuffix(kind, "List") && m["items"] != nil {
		var out []Object
		items, _ := m["items"].([]any)
		for _, it := range items {
			im, ok := it.(map[string]any)
			if !ok {
				continue
			}
			objs, err := objectsOf(im, source)
			if err != nil {
				return nil, err
			}
			out = append(out, objs...)
		}
		return out, nil
	}
	group, version := splitAPIVersion(apiVersion)
	meta, _ := m["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	if name == "" {
		if gn, _ := meta["generateName"].(string); gn != "" {
			name = gn + "*"
		}
	}
	ns, _ := meta["namespace"].(string)
	id := ObjectID{Group: group, Version: version, Kind: kind, Namespace: ns, Name: name}
	body, _ := normalizeTree(m, kind, "", false).(map[string]any)
	if body == nil {
		body = map[string]any{}
	}
	return []Object{{ID: id, Source: source, Body: body}}, nil
}

func splitAPIVersion(av string) (group, version string) {
	if i := strings.LastIndexByte(av, '/'); i >= 0 {
		return av[:i], av[i+1:]
	}
	return "", av
}

// sensitivePrefix marks a value replaced by its digest
// (redaction.RenderMarker).
const sensitivePrefix = redaction.RenderMarkerPrefix

func redact(v any) string { return redaction.RenderMarker(v) }

// isSensitive reports whether a normalized scalar is a redaction marker.
func isSensitive(v any) bool {
	s, ok := v.(string)
	return ok && strings.HasPrefix(s, sensitivePrefix)
}

// normalizeTree canonicalizes one object (RENDER-MISSION R15):
//   - nulls, empty maps and empty lists are dropped (empty ≡ omitted);
//   - credentials are replaced by a digest marker by the rule of package
//     redaction (shared with the bundle collector): Secret data/stringData,
//     private keys anywhere, scalars whose own key names a credential
//     (object-reference keys such as secretName or …Ref.name excepted),
//     strings below a credential-named mapping or list (booleans, nulls,
//     numbers and reference keys kept) and credential-named env values;
//     changes stay visible, values never leave the renderer. Inside a
//     CustomResourceDefinition (a schema) only private keys are;
//   - map ordering is irrelevant by construction; list ordering is handled
//     by the diff (keyed lists, permission sets, ordered args).
//
// key is the nearest mapping key above v; under reports that a key further
// up names a credential and holds a mapping or list.
func normalizeTree(v any, kind, key string, under bool) any {
	crd := kind == "CustomResourceDefinition"
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		envName, _ := t["name"].(string)
		_, hasName := t["name"]
		for k, x := range t {
			var n any
			switch {
			case kind == "Secret" && (key == "" && (k == "data" || k == "stringData")):
				n = redactAll(x)
			case !crd && k == "value" && envName != "" && redaction.CredentialEnvVar(envName) && isScalar(x):
				n = redact(x)
			case isScalar(x) || x == nil:
				n = leaf(x, k, under, crd, hasName)
			default:
				n = normalizeTree(x, kind, k, under || (!crd && redaction.CredentialContainer(k)))
			}
			if n != nil {
				out[k] = n
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		var out []any
		for _, x := range t {
			var n any
			if isScalar(x) {
				n = leaf(x, key, under, crd, false) // a list item
			} else {
				n = normalizeTree(x, kind, key, under)
			}
			if n != nil {
				out = append(out, n)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case nil:
		return nil
	default:
		return leaf(t, key, under, crd, false)
	}
}

// leaf applies the rule to one scalar (nil is dropped); nameSibling: its
// mapping has a "name" key.
func leaf(x any, key string, under, crd, nameSibling bool) any {
	if x == nil {
		return nil
	}
	s, isString := x.(string)
	switch {
	case isString && redaction.IsMarker(s):
		return s // already a marker (either form: a stored, collected file)
	case crd:
		if isString && redaction.HoldsPrivateKey(s) {
			return redact(s)
		}
		return x
	case !redaction.Keep(key, redaction.KindOf(x), s, under, nameSibling):
		return redact(x)
	}
	return x
}

func redactAll(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		if v == nil {
			return nil
		}
		return redact(v)
	}
	out := map[string]any{}
	for k, x := range m {
		if x != nil {
			out[k] = redact(x)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isScalar(v any) bool {
	switch v.(type) {
	case map[string]any, []any, nil:
		return false
	}
	return true
}

// encode renders a value as compact JSON (the Before/After encoding).
func encode(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprintf("%q", fmt.Sprint(v))
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// nondeterministicPaths compares two renders of identical inputs and
// returns "<object>|<path pattern>" for every field that differs.
func nondeterministicPaths(a, b []Object) []string {
	d := Diff(a, b, DiffOptions{})
	seen := map[string]bool{}
	var out []string
	for _, c := range d.Changes {
		k := c.Object.String() + "|" + c.Pattern
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
