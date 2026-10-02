package render

import (
	"regexp"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ChangeLink ties a rendered change to an upgrade-edge change that restates
// it, with the deterministic rule that matched.
type ChangeLink struct {
	ChangeID string `json:"changeId"`
	Title    string `json:"title"`
	Rule     string `json:"rule"` // subject | mentions-name | mentions-permission | mentions-object | mentions-api-version | mentions-field | artifact
}

// Correlated is one rendered change with the edge changes that restate it.
type Correlated struct {
	Change Change       `json:"change"`
	Links  []ChangeLink `json:"links,omitempty"`
}

// Correlation is the rendered diff read against the changelog (R8): which
// rendered changes an upstream change restates, and which rendered changes
// no changelog entry mentions (undocumented changes → review items).
type Correlation struct {
	Changes      []Correlated `json:"changes"`
	Documented   int          `json:"documented"`
	Undocumented int          `json:"undocumented"`
}

// genericKinds are too common to count as a mention by kind name alone.
var genericKinds = map[string]bool{
	"ConfigMap": true, "Secret": true, "Service": true, "Deployment": true, "ServiceAccount": true, "Role": true,
	"RoleBinding": true, "ClusterRole": true, "ClusterRoleBinding": true, "Pod": true, "Job": true,
}

// Correlate links every rendered change of a diff to the edge changes that
// restate it. Rules are generic and deterministic (no product logic):
//
//   - subject: the change's subjects name the image repository / flag / env
//     var / port / label the rendered change is about;
//   - mentions-name: the change text names it verbatim (a flag "--foo", an env
//     var, a label key, an image repository) — names shorter than 4
//     characters never count;
//   - mentions-permission: the text names the resource and the verb;
//   - mentions-object: the text names the object (or a non-generic kind);
//   - mentions-api-version: the text names the kind and either apiVersion;
//   - mentions-field: the text names the last path segment (an identifier of
//     6+ characters);
//   - values-default: a helm-values change whose subject ends in a key that
//     changed in the rendered object and whose text states the new value;
//   - artifact: an image change (or an argument carrying an image reference) whose repository is an artifact the edge
//     records as changed (a version bump documented by the artifact delta).
func Correlate(d *DiffResult, edge *domain.UpgradeEdge) Correlation {
	var out Correlation
	if d == nil {
		return out
	}
	type doc struct {
		c    domain.Change
		text string
	}
	var docs []doc
	if edge != nil {
		ev := map[domain.EvidenceID]domain.Evidence{}
		for _, e := range edge.Evidence {
			ev[e.ID] = e
		}
		for _, c := range edge.Changes {
			var b strings.Builder
			b.WriteString(c.Title + "\n" + c.Detail + "\n" + strings.Join(c.Subjects, "\n"))
			for _, id := range c.Evidence {
				if e, ok := ev[id]; ok && e.Kind != domain.EvidenceStructured {
					b.WriteString("\n" + e.Excerpt)
				}
			}
			docs = append(docs, doc{c, b.String()})
		}
	}
	artifactRepos := map[string]bool{}
	if edge != nil {
		for _, a := range edge.Artifacts {
			for _, inst := range []*domain.ArtifactInstance{a.From, a.To} {
				if inst != nil && inst.Coordinate != "" {
					artifactRepos[imageRepository(inst.Coordinate)] = true
				}
			}
		}
	}
	for _, rc := range d.Changes {
		cc := Correlated{Change: rc}
		seen := map[string]bool{}
		for _, dc := range docs {
			if rule := match(rc, dc.c, dc.text); rule != "" && !seen[dc.c.ID] {
				seen[dc.c.ID] = true
				cc.Links = append(cc.Links, ChangeLink{ChangeID: dc.c.ID, Title: dc.c.Title, Rule: rule})
			}
		}
		if len(cc.Links) == 0 {
			if repo := artifactImage(rc, artifactRepos); repo != "" {
				cc.Links = append(cc.Links, ChangeLink{Rule: "artifact", Title: "artifact version change of " + repo})
			}
		}
		sort.Slice(cc.Links, func(i, j int) bool { return cc.Links[i].ChangeID < cc.Links[j].ChangeID })
		if len(cc.Links) > 0 {
			out.Documented++
		} else {
			out.Undocumented++
		}
		out.Changes = append(out.Changes, cc)
	}
	return out
}

// artifactImage returns the artifact repository an image change (or an
// argument carrying an image reference) is about, when the edge records that
// artifact as changed.
func artifactImage(rc Change, repos map[string]bool) string {
	switch rc.Class {
	case ImageChanged:
		if repos[rc.Name] {
			return rc.Name
		}
	case ContainerArgChanged:
		if rc.After == nil {
			return ""
		}
		v, _ := Unquote(rc.After).(string)
		if i := strings.IndexByte(v, '='); i >= 0 {
			v = v[i+1:]
		}
		if r := imageRepository(v); r != v && repos[r] {
			return r
		}
	}
	return ""
}

// changedLeaves returns the keys whose values differ inside a change, with
// the new value: for object-valued changes (a Service port) the differing
// members, otherwise the last path segment.
func changedLeaves(rc Change) map[string]any {
	out := map[string]any{}
	b, a := Unquote(rc.Before), Unquote(rc.After)
	bm, bok := b.(map[string]any)
	am, aok := a.(map[string]any)
	if bok && aok {
		for k, v := range am {
			if encode(v) != encode(bm[k]) {
				out[k] = v
			}
		}
		return out
	}
	if seg := lastSeg(rc.Pattern); seg != "" && rc.After != nil {
		out[seg] = a
	}
	return out
}

func match(rc Change, c domain.Change, text string) string {
	if c.Category == domain.CategoryHelmValues && rc.Class.Presence() == PresenceChanged {
		// values-default: the values change names a key whose rendered leaf
		// changed, and states the new rendered value
		for key, val := range changedLeaves(rc) {
			for _, s := range c.Subjects {
				if s == key || strings.HasSuffix(s, "."+key) {
					if vs := strings.Trim(encode(val), `"`); len(vs) >= 2 && strings.Contains(c.Title+" "+c.Detail, vs) {
						return "values-default"
					}
				}
			}
		}
	}
	for _, s := range c.Subjects {
		if rc.Name != "" && (s == rc.Name || imageRepository(s) == rc.Name && rc.Class == ImageChanged) {
			return "subject"
		}
	}
	switch rc.Class {
	case RBACPermissionAdded, RBACPermissionRemoved:
		if p := rc.Permission; p != nil && p.Resource != "" && mentions(text, p.Resource) && mentions(text, p.Verb) {
			return "mentions-permission"
		}
		return ""
	case ResourceAdded, ResourceRemoved:
		if len(rc.Object.Name) >= 6 && mentions(text, rc.Object.Name) || !genericKinds[rc.Object.Kind] && mentions(text, rc.Object.Kind) {
			return "mentions-object"
		}
		return ""
	case APIVersionChanged:
		if mentions(text, rc.Object.Kind) && (mentions(text, rc.Object.APIVersion()) || rc.FromObject != nil && mentions(text, rc.FromObject.APIVersion())) {
			return "mentions-api-version"
		}
		return ""
	case FieldAdded, FieldRemoved, FieldChanged:
		seg := lastSeg(rc.Pattern)
		if len(seg) >= 6 && identifier.MatchString(seg) && mentions(text, seg) {
			return "mentions-field"
		}
		return ""
	}
	if len(rc.Name) >= 4 && rc.Name != "(order)" && mentions(text, rc.Name) {
		return "mentions-name"
	}
	return ""
}

var identifier = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// mentions reports a whole-token occurrence of s in text (case-sensitive
// for identifiers, which is how notes quote them).
func mentions(text, s string) bool {
	if !strings.ContainsFunc(s, func(r rune) bool { return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }) {
		return false // "*", "-", "" name nothing: a wildcard permission or bare punctuation never counts as mentioned
	}
	idx := 0
	for {
		i := strings.Index(text[idx:], s)
		if i < 0 {
			return false
		}
		start, end := idx+i, idx+i+len(s)
		if (start == 0 || !tokenChar(text[start-1])) && (end == len(text) || !tokenChar(text[end])) {
			return true
		}
		idx = start + 1
	}
}

func tokenChar(b byte) bool {
	return b == '_' || b == '-' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}
