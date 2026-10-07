package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Occurrence is one place a subject appears in a render, with its value
// ("" when presence is the value, e.g. a permission).
type Occurrence struct {
	Object ObjectID
	Path   string
	Value  string
}

// Inventory returns where a semantic subject appears in a render, for the
// families a render can show (ok=false otherwise). It is the
// release-artifact counterpart of the env package's resource facts:
//
//   - rbac-permission: Name "resource/verb", "resource/name/verb" or
//     "resource" (any verb), Group and Component (role name) narrowing;
//   - image: container images whose repository is Name (value: the reference);
//   - cli-flag: container args/command tokens whose flag is Name, Component
//     the container (value: the part after "=", "" for a bare flag);
//   - env-var: container env entries named Name, Component the container;
//   - feature-gate: "--feature-gates" tokens whose key is Name (value: on/off);
//   - gvk: objects of Group/Version/Kind, and CRD versions served for the kind.
func Inventory(objs []Object, s domain.Subject) (occ []Occurrence, ok bool) {
	switch s.Family {
	case domain.SubjectRBACPermission:
		for _, o := range objs {
			if !isRBACRole(o.ID.Kind) || s.Component != "" && o.ID.Name != s.Component && !strings.HasSuffix(o.ID.Name, "-"+s.Component) {
				continue
			}
			for _, p := range permissions(o.Body["rules"]) {
				if s.Group != "" && p.Group != s.Group {
					continue
				}
				if permMatches(s.Name, p) {
					occ = append(occ, Occurrence{Object: o.ID, Path: "rules", Value: p.String()})
				}
			}
		}
		return sortOcc(occ), true
	case domain.SubjectImage:
		eachContainer(objs, func(o Object, path, name string, c map[string]any) {
			if img, _ := c["image"].(string); img != "" && imageRepository(img) == s.Name {
				occ = append(occ, Occurrence{Object: o.ID, Path: path + ".image", Value: img})
			}
		})
		return sortOcc(occ), true
	case domain.SubjectCLIFlag:
		want := strings.TrimLeft(s.Name, "-")
		eachContainer(objs, func(o Object, path, name string, c map[string]any) {
			if s.Component != "" && name != s.Component {
				return
			}
			for _, field := range []string{"command", "args"} {
				for _, x := range toList(c[field]) {
					tok := fmt.Sprint(x)
					if !strings.HasPrefix(tok, "-") {
						continue
					}
					k, v, _ := strings.Cut(tok, "=")
					if strings.TrimLeft(k, "-") == want {
						occ = append(occ, Occurrence{Object: o.ID, Path: path + "." + field, Value: v})
					}
				}
			}
		})
		return sortOcc(occ), true
	case domain.SubjectEnvVar:
		eachContainer(objs, func(o Object, path, name string, c map[string]any) {
			if s.Component != "" && name != s.Component {
				return
			}
			for _, e := range toList(c["env"]) {
				m, _ := e.(map[string]any)
				if n, _ := m["name"].(string); n == s.Name {
					occ = append(occ, Occurrence{Object: o.ID, Path: path + ".env[name=" + n + "]", Value: encode(envValue(m))})
				}
			}
		})
		return sortOcc(occ), true
	case domain.SubjectFeatureGate:
		eachContainer(objs, func(o Object, path, name string, c map[string]any) {
			if s.Component != "" && name != s.Component {
				return
			}
			for _, x := range toList(c["args"]) {
				tok := fmt.Sprint(x)
				k, v, found := strings.Cut(tok, "=")
				if !found || strings.TrimLeft(k, "-") != "feature-gates" {
					continue
				}
				for _, g := range strings.Split(v, ",") {
					gk, gv, _ := strings.Cut(strings.TrimSpace(g), "=")
					if gk == s.Name {
						occ = append(occ, Occurrence{Object: o.ID, Path: path + ".args", Value: gv})
					}
				}
			}
		})
		return sortOcc(occ), true
	case domain.SubjectGVK:
		for _, o := range objs {
			if o.ID.Kind == s.Kind && o.ID.Version == s.Version && (s.Group == "" || o.ID.Group == s.Group) {
				occ = append(occ, Occurrence{Object: o.ID, Value: "object"})
			}
			if o.ID.Kind == "CustomResourceDefinition" && str(o.Body, "spec", "names", "kind") == s.Kind && (s.Group == "" || str(o.Body, "spec", "group") == s.Group) {
				for _, v := range toList(get(o.Body, "spec", "versions")) {
					vm, _ := v.(map[string]any)
					if n, _ := vm["name"].(string); n == s.Version {
						if served, ok := vm["served"].(bool); !ok || served {
							occ = append(occ, Occurrence{Object: o.ID, Path: "spec.versions[name=" + n + "]", Value: "served"})
						}
					}
				}
			}
		}
		return sortOcc(occ), true
	}
	return nil, false
}

func permMatches(name string, p Permission) bool {
	if p.NonResourceURL != "" {
		return name == p.NonResourceURL+":"+p.Verb
	}
	full := permName(p)
	if name == full || name == p.Resource {
		return true
	}
	// "resource/verb" matches a resourceName-scoped permission too
	return p.ResourceName != "" && name == p.Resource+"/"+p.Verb
}

// eachContainer visits every container of every pod template in the render.
func eachContainer(objs []Object, visit func(o Object, path, name string, c map[string]any)) {
	for _, o := range objs {
		var walk func(v any, path string)
		walk = func(v any, path string) {
			m, ok := v.(map[string]any)
			if !ok {
				return
			}
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				x := m[k]
				p := joinKey(path, k)
				if containerLists[k] {
					for _, c := range toList(x) {
						cm, _ := c.(map[string]any)
						if n, _ := cm["name"].(string); n != "" {
							visit(o, p+"[name="+n+"]", n, cm)
						}
					}
					continue
				}
				walk(x, p)
			}
		}
		walk(o.Body, "")
	}
}

func sortOcc(occ []Occurrence) []Occurrence {
	sort.Slice(occ, func(i, j int) bool {
		a, b := occ[i], occ[j]
		if a.Object.String() != b.Object.String() {
			return a.Object.String() < b.Object.String()
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Value < b.Value
	})
	return occ
}

// values returns the distinct values of occurrences.
func values(occ []Occurrence) []string {
	seen := map[string]bool{}
	var out []string
	for _, o := range occ {
		if !seen[o.Value] {
			seen[o.Value] = true
			out = append(out, o.Value)
		}
	}
	sort.Strings(out)
	return out
}
