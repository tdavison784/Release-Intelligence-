package render

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// ChangeClass is the semantic class of a rendered change (RENDER-MISSION R4).
type ChangeClass string

const (
	ResourceAdded         ChangeClass = "resource-added"
	ResourceRemoved       ChangeClass = "resource-removed"
	APIVersionChanged     ChangeClass = "api-version-changed"
	FieldAdded            ChangeClass = "field-added"
	FieldRemoved          ChangeClass = "field-removed"
	FieldChanged          ChangeClass = "field-changed"
	ImageChanged          ChangeClass = "image-changed"
	RBACPermissionAdded   ChangeClass = "rbac-permission-added"
	RBACPermissionRemoved ChangeClass = "rbac-permission-removed"
	ServicePortAdded      ChangeClass = "service-port-added"
	ServicePortRemoved    ChangeClass = "service-port-removed"
	ServicePortChanged    ChangeClass = "service-port-changed"
	ContainerArgAdded     ChangeClass = "container-arg-added"
	ContainerArgRemoved   ChangeClass = "container-arg-removed"
	ContainerArgChanged   ChangeClass = "container-arg-changed"
	EnvVarAdded           ChangeClass = "env-var-added"
	EnvVarRemoved         ChangeClass = "env-var-removed"
	EnvVarChanged         ChangeClass = "env-var-changed"
	LabelAdded            ChangeClass = "label-added"
	LabelRemoved          ChangeClass = "label-removed"
	LabelChanged          ChangeClass = "label-changed"
	AnnotationAdded       ChangeClass = "annotation-added"
	AnnotationRemoved     ChangeClass = "annotation-removed"
	AnnotationChanged     ChangeClass = "annotation-changed"
)

// ChangeClasses lists every class.
var ChangeClasses = []ChangeClass{
	ResourceAdded, ResourceRemoved, APIVersionChanged, FieldAdded, FieldRemoved, FieldChanged, ImageChanged,
	RBACPermissionAdded, RBACPermissionRemoved, ServicePortAdded, ServicePortRemoved, ServicePortChanged,
	ContainerArgAdded, ContainerArgRemoved, ContainerArgChanged, EnvVarAdded, EnvVarRemoved, EnvVarChanged,
	LabelAdded, LabelRemoved, LabelChanged, AnnotationAdded, AnnotationRemoved, AnnotationChanged,
}

// Presence is the contract-level shape of a change at its path: what the
// rendered-change predicate's states test.
type Presence string

const (
	PresenceAdded   Presence = "added"   // only in the target render
	PresenceRemoved Presence = "removed" // only in the source render
	PresenceChanged Presence = "changed" // in both, value differs
)

// Presence maps a class onto added/removed/changed.
func (c ChangeClass) Presence() Presence {
	s := string(c)
	switch {
	case strings.HasSuffix(s, "-added"):
		return PresenceAdded
	case strings.HasSuffix(s, "-removed"):
		return PresenceRemoved
	}
	return PresenceChanged
}

// Permission is one RBAC permission tuple (apiGroup × resource × verb, with
// an optional resourceName; or a nonResourceURL × verb).
type Permission struct {
	Group          string `json:"group,omitempty"`
	Resource       string `json:"resource,omitempty"`
	ResourceName   string `json:"resourceName,omitempty"`
	NonResourceURL string `json:"nonResourceURL,omitempty"`
	Verb           string `json:"verb"`
}

func (p Permission) String() string {
	if p.NonResourceURL != "" {
		return p.NonResourceURL + ":" + p.Verb
	}
	g := p.Group
	if g == "" {
		g = "core"
	}
	s := g + "/" + p.Resource
	if p.ResourceName != "" {
		s += "/" + p.ResourceName
	}
	return s + ":" + p.Verb
}

// Change is one semantic difference between two renders.
//
// Path syntax (docs/RENDER.md): dotted keys; keys holding "." or "/" are
// quoted (labels["app.kubernetes.io/name"]); elements of lists Kubernetes
// merges by key are addressed by that key (containers[name=controller],
// ports[port=443/TCP]); Pattern is the same path with every element
// selector reduced to "[]" — the env/CRD SchemaPath syntax the condition
// language uses ("spec.template.spec.containers[].args").
type Change struct {
	Class ChangeClass `json:"class"`
	// Object is the target-render identity (the source identity for removals).
	Object ObjectID `json:"object"`
	// FromObject is the source identity when it differs (api-version-changed).
	FromObject *ObjectID `json:"fromObject,omitempty"`
	Path       string    `json:"path,omitempty"`
	Pattern    string    `json:"pattern,omitempty"`
	// Name is the class-specific subject name: the image repository, the
	// flag ("--foo"), the env var, the port, the label/annotation key, the
	// permission ("certificates/update").
	Name string `json:"name,omitempty"`
	// Container names the container for container-scoped classes.
	Container  string      `json:"container,omitempty"`
	Permission *Permission `json:"permission,omitempty"`
	// Before/After are JSON-encoded values (absent on the missing side).
	// Sensitive values are digests (Sensitive=true).
	Before    *string `json:"before,omitempty"`
	After     *string `json:"after,omitempty"`
	Sensitive bool    `json:"sensitive,omitempty"`
	// Source is the template that rendered the object.
	Source string `json:"source,omitempty"`
}

// Summary renders the change on one line; values only when showValues.
func (c Change) Summary(showValues bool) string {
	var b strings.Builder
	b.WriteString(string(c.Class))
	b.WriteString(" ")
	b.WriteString(c.Object.Kind + "/" + c.Object.Name)
	if c.Object.Namespace != "" {
		b.WriteString(" (ns " + c.Object.Namespace + ")")
	}
	if c.Path != "" {
		b.WriteString(" " + c.Path)
	}
	if c.Class == APIVersionChanged && c.FromObject != nil {
		b.WriteString(" " + c.FromObject.APIVersion() + " → " + c.Object.APIVersion())
		return b.String()
	}
	if c.Permission != nil {
		b.WriteString(" " + c.Permission.String())
		return b.String()
	}
	if c.Before == nil && c.After == nil {
		return b.String()
	}
	if showValues && !c.Sensitive {
		b.WriteString(": " + deref(c.Before) + " → " + deref(c.After))
	} else if c.Sensitive {
		b.WriteString(" (sensitive value; digest only)")
	}
	return b.String()
}

func deref(s *string) string {
	if s == nil {
		return "∅"
	}
	return *s
}

// DiffOptions tunes Diff.
type DiffOptions struct {
	// Nondeterministic lists "<object>|<pattern>" pairs to suppress (fields
	// that differ between renders of identical inputs).
	Nondeterministic []string
	// KeepNoise disables noise suppression (testing / --show-noise).
	KeepNoise bool
}

// DiffResult is the semantic diff of two renders.
type DiffResult struct {
	Changes []Change `json:"changes"`
	// Suppressed counts changes removed as non-semantic noise, per rule.
	Suppressed  map[string]int `json:"suppressed,omitempty"`
	FromObjects int            `json:"fromObjects"`
	ToObjects   int            `json:"toObjects"`
}

// noiseRule suppresses non-semantic differences (R15). Rules are data.
type noiseRule struct {
	name string
	// match reports whether a change at pattern (of the object kind) is noise.
	match func(kind, pattern, name string) bool
}

// chartMetadataKeys are labels/annotations stamped with the chart or app
// version on every object: the version moved, not the deployment.
var chartMetadataKeys = map[string]bool{"helm.sh/chart": true, "app.kubernetes.io/version": true, "chart": true}

var noiseRules = []noiseRule{
	{"chart-metadata", func(_, pattern, name string) bool {
		return chartMetadataKeys[name] && (strings.HasSuffix(pattern, `labels["`+name+`"]`) || strings.HasSuffix(pattern, "labels."+name) ||
			strings.HasSuffix(pattern, `annotations["`+name+`"]`) || strings.HasSuffix(pattern, "annotations."+name))
	}},
	{"checksum-annotation", func(_, pattern, name string) bool {
		return strings.HasPrefix(name, "checksum/") && strings.Contains(pattern, "annotations[")
	}},
}

// Diff computes the semantic diff between a source and a target render.
func Diff(from, to []Object, opts DiffOptions) DiffResult {
	res := DiffResult{FromObjects: len(from), ToObjects: len(to), Suppressed: map[string]int{}}
	nondet := map[string]bool{}
	for _, k := range opts.Nondeterministic {
		nondet[k] = true
	}
	index := func(objs []Object) (map[string]*Object, []string) {
		m := map[string]*Object{}
		var keys []string
		for i := range objs {
			k := objs[i].ID.key()
			if _, dup := m[k]; dup {
				k = objs[i].ID.String() // same object at two versions in one render
			}
			m[k] = &objs[i]
			keys = append(keys, k)
		}
		return m, keys
	}
	fm, fk := index(from)
	tm, tk := index(to)
	keys := map[string]bool{}
	for _, k := range fk {
		keys[k] = true
	}
	for _, k := range tk {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	var out []Change
	for _, k := range sorted {
		a, b := fm[k], tm[k]
		switch {
		case a == nil:
			out = append(out, Change{Class: ResourceAdded, Object: b.ID, Source: b.Source})
		case b == nil:
			out = append(out, Change{Class: ResourceRemoved, Object: a.ID, Source: a.Source})
		default:
			d := &differ{obj: b.ID, kind: b.ID.Kind, source: b.Source}
			if a.ID.Version != b.ID.Version {
				fromID := a.ID
				d.emit(Change{Class: APIVersionChanged, FromObject: &fromID,
					Before: ptr(encode(a.ID.APIVersion())), After: ptr(encode(b.ID.APIVersion()))})
			}
			d.object(a.Body, b.Body)
			out = append(out, d.out...)
		}
	}
	for _, c := range out {
		if !opts.KeepNoise {
			if rule := noiseOf(c); rule != "" {
				res.Suppressed[rule]++
				continue
			}
			if nondet[c.Object.String()+"|"+c.Pattern] || (c.FromObject != nil && nondet[c.FromObject.String()+"|"+c.Pattern]) {
				res.Suppressed["nondeterministic"]++
				continue
			}
		}
		res.Changes = append(res.Changes, c)
	}
	if len(res.Suppressed) == 0 {
		res.Suppressed = nil
	}
	return res
}

func noiseOf(c Change) string {
	for _, r := range noiseRules {
		if r.match(c.Object.Kind, c.Pattern, c.Name) {
			return r.name
		}
	}
	return ""
}

type differ struct {
	obj    ObjectID
	kind   string
	source string
	out    []Change
}

func (d *differ) emit(c Change) {
	c.Object = d.obj
	c.Source = d.source
	if c.Before != nil && strings.Contains(*c.Before, sensitivePrefix) || c.After != nil && strings.Contains(*c.After, sensitivePrefix) {
		c.Sensitive = true
	}
	d.out = append(d.out, c)
}

func ptr(s string) *string { return &s }

// object diffs two object bodies; apiVersion/kind are identity, not fields.
func (d *differ) object(a, b map[string]any) {
	a, b = without(a, "apiVersion", "kind"), without(b, "apiVersion", "kind")
	if isRBACRole(d.kind) {
		d.rbac(a["rules"], b["rules"])
		a, b = without(a, "rules"), without(b, "rules")
	}
	d.value("", "", a, b)
}

func without(m map[string]any, keys ...string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	for _, k := range keys {
		delete(out, k)
	}
	return out
}

func isRBACRole(kind string) bool { return kind == "Role" || kind == "ClusterRole" }

// joinKey appends a map key to a path, quoting keys that hold separators.
func joinKey(path, key string) string {
	if strings.ContainsAny(key, "./[]\"") {
		return path + "[" + strconv.Quote(key) + "]"
	}
	if path == "" {
		return key
	}
	return path + "." + key
}

// value diffs a at path/pattern against b.
func (d *differ) value(path, pattern string, a, b any) {
	if reflect.DeepEqual(a, b) {
		return
	}
	switch {
	case a == nil:
		d.added(path, pattern, b)
		return
	case b == nil:
		d.removed(path, pattern, a)
		return
	}
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if aok && bok {
		keys := map[string]bool{}
		for k := range am {
			keys[k] = true
		}
		for k := range bm {
			keys[k] = true
		}
		ks := make([]string, 0, len(keys))
		for k := range keys {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			d.keyed(joinKey(path, k), joinKey(pattern, k), k, am[k], bm[k])
		}
		return
	}
	al, alok := a.([]any)
	bl, blok := b.([]any)
	if alok && blok {
		d.list(path, pattern, al, bl)
		return
	}
	d.changed(path, pattern, a, b)
}

// keyed handles a map entry: label/annotation classes, container images,
// then the generic recursion.
func (d *differ) keyed(path, pattern, key string, a, b any) {
	if reflect.DeepEqual(a, b) {
		return
	}
	parent := lastSeg(trimLastSeg(pattern))
	if parent == "labels" || parent == "annotations" {
		var class ChangeClass
		switch {
		case a == nil:
			class = map[string]ChangeClass{"labels": LabelAdded, "annotations": AnnotationAdded}[parent]
		case b == nil:
			class = map[string]ChangeClass{"labels": LabelRemoved, "annotations": AnnotationRemoved}[parent]
		default:
			class = map[string]ChangeClass{"labels": LabelChanged, "annotations": AnnotationChanged}[parent]
		}
		d.emit(Change{Class: class, Path: path, Pattern: pattern, Name: key, Before: enc(a), After: enc(b)})
		return
	}
	if key == "labels" || key == "annotations" {
		// a labels map that appears or disappears is still per-key changes
		am, _ := a.(map[string]any)
		bm, _ := b.(map[string]any)
		if (am != nil || a == nil) && (bm != nil || b == nil) {
			if am == nil {
				am = map[string]any{}
			}
			if bm == nil {
				bm = map[string]any{}
			}
			d.value(path, pattern, am, bm)
			return
		}
	}
	if key == "image" && isContainerPattern(trimLastSeg(pattern)) && a != nil && b != nil && isScalar(a) && isScalar(b) {
		d.emit(Change{Class: ImageChanged, Path: path, Pattern: pattern, Name: imageRepository(fmt.Sprint(b)),
			Container: containerOf(path), Before: enc(a), After: enc(b)})
		return
	}
	if (key == "args" || key == "command") && isContainerPattern(trimLastSeg(pattern)) {
		d.args(path, pattern, toList(a), toList(b))
		return
	}
	d.value(path, pattern, a, b)
}

func enc(v any) *string {
	if v == nil {
		return nil
	}
	return ptr(encode(v))
}

func toList(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	if v == nil {
		return nil
	}
	return []any{v}
}

func (d *differ) added(path, pattern string, b any) {
	d.emit(Change{Class: FieldAdded, Path: path, Pattern: pattern, After: enc(b)})
}

func (d *differ) removed(path, pattern string, a any) {
	d.emit(Change{Class: FieldRemoved, Path: path, Pattern: pattern, Before: enc(a)})
}

func (d *differ) changed(path, pattern string, a, b any) {
	d.emit(Change{Class: FieldChanged, Path: path, Pattern: pattern, Before: enc(a), After: enc(b)})
}

// containerLists are the pod-spec lists whose elements are containers.
var containerLists = map[string]bool{"containers": true, "initContainers": true, "ephemeralContainers": true}

func isContainerPattern(pattern string) bool {
	return strings.HasSuffix(pattern, "[]") && containerLists[lastSeg(strings.TrimSuffix(pattern, "[]"))]
}

// containerOf extracts the container name from a keyed path.
func containerOf(path string) string {
	for _, l := range []string{"containers", "initContainers", "ephemeralContainers"} {
		marker := l + "[name="
		if i := strings.LastIndex(path, marker); i >= 0 {
			rest := path[i+len(marker):]
			if j := strings.IndexByte(rest, ']'); j >= 0 {
				return rest[:j]
			}
		}
	}
	return ""
}

func lastSeg(p string) string {
	p = strings.TrimSuffix(p, "[]")
	if strings.HasSuffix(p, "\"]") {
		if i := strings.LastIndex(p, "[\""); i >= 0 {
			s, err := strconv.Unquote(p[i+1 : len(p)-1])
			if err == nil {
				return s
			}
		}
	}
	if i := strings.LastIndexByte(p, '.'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func trimLastSeg(p string) string {
	if strings.HasSuffix(p, "\"]") {
		if i := strings.LastIndex(p, "[\""); i >= 0 {
			return p[:i]
		}
	}
	if i := strings.LastIndexByte(p, '.'); i >= 0 {
		return p[:i]
	}
	return ""
}

// listKeyFields: lists Kubernetes merges by key, by list name, in the order
// tried. A list is keyed only when every element has a unique value for the
// field; otherwise it is compared as a whole (ordered).
var listKeyFields = map[string][]string{
	"containers": {"name"}, "initContainers": {"name"}, "ephemeralContainers": {"name"},
	"env": {"name"}, "volumes": {"name"}, "imagePullSecrets": {"name"},
	"volumeMounts": {"mountPath"}, "volumeDevices": {"devicePath"},
	"ports":    {"name", "port", "containerPort"},
	"webhooks": {"name"}, "versions": {"name"}, "subjects": {"name"},
	"hostAliases": {"ip"}, "topologySpreadConstraints": {"topologyKey"},
	"conditions": {"type"}, "tolerations": {"key"},
}

// unorderedLists are compared as sets when no key applies.
var unorderedLists = map[string]bool{
	"tolerations": true, "subjects": true, "finalizers": true, "envFrom": true, "imagePullSecrets": true,
	"accessModes": true, "verbs": true, "resources": true, "apiGroups": true,
}

func (d *differ) list(path, pattern string, a, b []any) {
	name := lastSeg(pattern)
	if field := keyField(name, a, b); field != "" {
		d.keyedList(path, pattern, name, field, a, b)
		return
	}
	if unorderedLists[name] && sameMultiset(a, b) {
		return
	}
	d.changed(path, pattern, a, b)
}

func keyField(listName string, a, b []any) string {
	fields := listKeyFields[listName]
	if len(fields) == 0 {
		return ""
	}
	for _, f := range fields {
		ok := true
		for _, l := range [][]any{a, b} {
			seen := map[string]bool{}
			for _, e := range l {
				m, isMap := e.(map[string]any)
				if !isMap || m[f] == nil {
					ok = false
					break
				}
				k := elemKey(listName, f, m)
				if seen[k] {
					ok = false
					break
				}
				seen[k] = true
			}
			if !ok {
				break
			}
		}
		if ok {
			return f
		}
	}
	return ""
}

// elemKey is an element's key value; port keys include the protocol.
func elemKey(listName, field string, m map[string]any) string {
	k := fmt.Sprint(m[field])
	if listName == "ports" && field != "name" {
		proto, _ := m["protocol"].(string)
		if proto == "" {
			proto = "TCP"
		}
		k += "/" + proto
	}
	return k
}

func (d *differ) keyedList(path, pattern, listName, field string, a, b []any) {
	am, bm := map[string]map[string]any{}, map[string]map[string]any{}
	var order []string
	for _, e := range a {
		m := e.(map[string]any)
		k := elemKey(listName, field, m)
		am[k] = m
		order = append(order, k)
	}
	for _, e := range b {
		m := e.(map[string]any)
		k := elemKey(listName, field, m)
		bm[k] = m
		if _, ok := am[k]; !ok {
			order = append(order, k)
		}
	}
	sort.Strings(order)
	ep := pattern + "[]"
	for _, k := range order {
		x, y := am[k], bm[k]
		p := path + "[" + field + "=" + k + "]"
		switch {
		case listName == "env":
			d.env(p, ep, k, x, y)
		case listName == "ports" && d.kind == "Service":
			d.servicePort(p, ep, k, x, y)
		case x == nil:
			d.added(p, ep, y)
		case y == nil:
			d.removed(p, ep, x)
		default:
			d.value(p, ep, x, y)
		}
	}
}

func (d *differ) env(path, pattern, name string, a, b map[string]any) {
	if reflect.DeepEqual(a, b) {
		return
	}
	c := Change{Path: path, Pattern: pattern, Name: name, Container: containerOf(path)}
	switch {
	case a == nil:
		c.Class, c.After = EnvVarAdded, enc(envValue(b))
	case b == nil:
		c.Class, c.Before = EnvVarRemoved, enc(envValue(a))
	default:
		c.Class, c.Before, c.After = EnvVarChanged, enc(envValue(a)), enc(envValue(b))
	}
	d.emit(c)
}

// envValue is an env element without its name: the value or valueFrom.
func envValue(m map[string]any) any {
	if v, ok := m["value"]; ok && len(m) == 2 {
		return v
	}
	return without(m, "name")
}

func (d *differ) servicePort(path, pattern, key string, a, b map[string]any) {
	if reflect.DeepEqual(a, b) {
		return
	}
	c := Change{Path: path, Pattern: pattern, Name: key}
	switch {
	case a == nil:
		c.Class, c.After = ServicePortAdded, enc(b)
	case b == nil:
		c.Class, c.Before = ServicePortRemoved, enc(a)
	default:
		c.Class, c.Before, c.After = ServicePortChanged, enc(a), enc(b)
	}
	d.emit(c)
}

// args diffs container args/command. Ordering is semantic for positional
// arguments, so the comparison keys each token by its flag ("--foo=bar" →
// "--foo") and reports added/removed/changed flags; a pure reordering is
// one container-arg-changed with both lists.
func (d *differ) args(path, pattern string, a, b []any) {
	if reflect.DeepEqual(a, b) {
		return
	}
	container := containerOf(path)
	type tok struct{ key, raw string }
	toks := func(l []any) ([]tok, map[string][]string) {
		var out []tok
		m := map[string][]string{}
		for _, x := range l {
			s := fmt.Sprint(x)
			k := s
			if strings.HasPrefix(s, "-") {
				if i := strings.IndexByte(s, '='); i >= 0 {
					k = s[:i]
				}
			}
			out = append(out, tok{k, s})
			m[k] = append(m[k], s)
		}
		return out, m
	}
	at, am := toks(a)
	bt, bm := toks(b)
	var keys []string
	seen := map[string]bool{}
	for _, t := range append(append([]tok{}, at...), bt...) {
		if !seen[t.key] {
			seen[t.key] = true
			keys = append(keys, t.key)
		}
	}
	sort.Strings(keys)
	emitted := false
	for _, k := range keys {
		x, y := am[k], bm[k]
		if reflect.DeepEqual(x, y) {
			continue
		}
		emitted = true
		c := Change{Path: path, Pattern: pattern, Name: k, Container: container}
		switch {
		case len(x) == 0:
			c.Class, c.After = ContainerArgAdded, ptr(encodeTokens(y))
		case len(y) == 0:
			c.Class, c.Before = ContainerArgRemoved, ptr(encodeTokens(x))
		default:
			c.Class, c.Before, c.After = ContainerArgChanged, ptr(encodeTokens(x)), ptr(encodeTokens(y))
		}
		d.emit(c)
	}
	if !emitted { // same tokens, different order
		d.emit(Change{Class: ContainerArgChanged, Path: path, Pattern: pattern, Name: "(order)", Container: container,
			Before: ptr(encode(a)), After: ptr(encode(b))})
	}
}

// encodeTokens encodes one token as a JSON string, several as a JSON list.
func encodeTokens(ts []string) string {
	if len(ts) == 1 {
		return encode(ts[0])
	}
	return encode(ts)
}

// rbac diffs Role/ClusterRole rules as permission sets: reordering rules,
// splitting or merging them changes nothing; a verb removed from a rule is
// one rbac-permission-removed per (group, resource, verb) tuple.
func (d *differ) rbac(a, b any) {
	ap, bp := permissions(a), permissions(b)
	keys := map[string]bool{}
	for k := range ap {
		keys[k] = true
	}
	for k := range bp {
		keys[k] = true
	}
	ks := make([]string, 0, len(keys))
	for k := range keys {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		pa, inA := ap[k]
		pb, inB := bp[k]
		switch {
		case inA && !inB:
			p := pa
			d.emit(Change{Class: RBACPermissionRemoved, Path: "rules", Pattern: "rules", Name: permName(p), Permission: &p, Before: ptr(encode(p.String()))})
		case inB && !inA:
			p := pb
			d.emit(Change{Class: RBACPermissionAdded, Path: "rules", Pattern: "rules", Name: permName(p), Permission: &p, After: ptr(encode(p.String()))})
		}
	}
}

// permName is the rbac-permission subject name: "resource/verb" (or
// "url:verb" for non-resource URLs).
func permName(p Permission) string {
	if p.NonResourceURL != "" {
		return p.NonResourceURL + ":" + p.Verb
	}
	r := p.Resource
	if p.ResourceName != "" {
		r += "/" + p.ResourceName
	}
	return r + "/" + p.Verb
}

func permissions(rules any) map[string]Permission {
	out := map[string]Permission{}
	l, _ := rules.([]any)
	strs := func(m map[string]any, k string) []string {
		var s []string
		for _, x := range toList(m[k]) {
			s = append(s, fmt.Sprint(x))
		}
		return s
	}
	for _, r := range l {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		verbs := strs(m, "verbs")
		for _, u := range strs(m, "nonResourceURLs") {
			for _, v := range verbs {
				p := Permission{NonResourceURL: u, Verb: v}
				out[p.String()] = p
			}
		}
		groups := strs(m, "apiGroups")
		if len(groups) == 0 && m["resources"] != nil {
			groups = []string{""}
		}
		names := strs(m, "resourceNames")
		if len(names) == 0 {
			names = []string{""}
		}
		for _, g := range groups {
			for _, res := range strs(m, "resources") {
				for _, n := range names {
					for _, v := range verbs {
						p := Permission{Group: g, Resource: res, ResourceName: n, Verb: v}
						out[p.String()] = p
					}
				}
			}
		}
	}
	return out
}

func sameMultiset(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	count := map[string]int{}
	for _, x := range a {
		count[encode(x)]++
	}
	for _, x := range b {
		count[encode(x)]--
	}
	for _, n := range count {
		if n != 0 {
			return false
		}
	}
	return true
}

// imageRepository strips the tag and digest from an image reference.
func imageRepository(ref string) string {
	if i := strings.IndexByte(ref, '@'); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndexByte(ref, ':'); i >= 0 && !strings.Contains(ref[i:], "/") {
		ref = ref[:i]
	}
	return ref
}

// Unquote decodes a Before/After value.
func Unquote(s *string) any {
	if s == nil {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(*s), &v); err != nil {
		return *s
	}
	return v
}
