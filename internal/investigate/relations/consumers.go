package relations

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// Consumer mode (WithConsumers; docs/phase6/decision-intel/PLAN-M1.md
// changes 1 and 2). Everything here is generic Kubernetes semantics: how a
// pod mounts a Secret or ConfigMap (volumes, projected sources, items,
// subPath), and plain path matching of the absolute file paths a mounted or
// env-consumed ConfigMap's data names (rules v3: or the container's own
// command, args and literal env values) against the other mounts of the same
// container. No rule knows a product, an application, a file or a key.

const (
	ruleReadsFile   = "graph.reads-file@v1:"
	ruleReadsFileV2 = "graph.reads-file@v2:" // derivations new in consumer rules v2
	ruleReadsFileV3 = "graph.reads-file@v3:" // derivations new in consumer rules v3
	maxLineText     = 300
)

// saTokenDir is where the kubelet mounts the pod's service account token
// unless automountServiceAccountToken is false (Kubernetes, not the spec).
const saTokenDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// volumeMount is the mount detail of one volume (src: projected source index
// or -1; items: that source's or the volume's items).
func volumeMount(spec map[string]any, volume string, src int, items []any) *domain.EdgeMount {
	m := &domain.EdgeMount{Volume: volume, Source: src}
	for _, it := range items {
		if k, p := str(it, "key"), str(it, "path"); k != "" && p != "" {
			m.Items = append(m.Items, domain.KeyToPath{Key: k, Path: p})
		}
	}
	for _, c := range podContainers(spec) {
		for _, vm := range asSeq(c.body["volumeMounts"]) {
			if str(vm, "name") == volume && str(vm, "mountPath") != "" {
				m.Mounts = append(m.Mounts, domain.VolumeMount{Container: c.name, MountPath: str(vm, "mountPath"), SubPath: str(vm, "subPath")})
			}
		}
	}
	return m
}

// container is one container of a pod spec, at index idx of field.
type container struct {
	name, field string
	idx         int
	body        map[string]any
}

func podContainers(spec map[string]any) []container {
	var out []container
	for _, field := range []string{"initContainers", "containers", "ephemeralContainers"} {
		for i, c := range asSeq(spec[field]) {
			if m := asMap(c); m != nil && str(m, "name") != "" {
				out = append(out, container{name: str(m, "name"), field: field, idx: i, body: m})
			}
		}
	}
	return out
}

// dataEntry is one key of a ConfigMap's data with where its text lives: an
// artifact and the line its text starts at (line 0: locate the key in the
// artifact's YAML instead, in object obj when it is not the node's own: a
// rendered generated object). from says where data that is not the
// snapshot's came from (EdgeFile.DataFrom).
type dataEntry struct {
	key, text, artifact string
	line                int
	obj                 *render.ObjectID
	from                string
}

// genData is what the bundle says about the data of one kustomize-generated
// ConfigMap or Secret (a Secret only under rules v2, for its keys).
type genData struct {
	entries []dataEntry // a ConfigMap's keys with their text
	// v2: keys are the keys known to exist; unknown the data the bundle does
	// not hold (key "": which keys exist is not known); from the rendered
	// object the data was completed from.
	keys    map[string]bool
	unknown []unknownData
	from    string
}

// unknownData is configuration data the bundle does not hold, with why and
// where the generator names its source.
type unknownData struct {
	key, reason, artifact string
	line                  *domain.EdgeLine
}

// consumerEdges runs the consumer pass after every other edge exists.
func (g *gb) consumerEdges() error {
	if err := g.generatorData(); err != nil {
		return err
	}
	for _, n := range g.sortedNodes() {
		if n.obj != nil && g.objNode[n.obj] == n && n.n.Kind == domain.NodeWorkload {
			if err := g.readsFile(n); err != nil {
				return err
			}
		}
	}
	return g.lines()
}

// --- generated ConfigMaps ----------------------------------------------------------------

// generatorData reads the data of every kustomize configMapGenerator the
// snapshot defines (files, literals, envs) for ConfigMaps that are not
// objects of the snapshot themselves. Under rules v2 it also reads the keys
// of every secretGenerator, records each source the snapshot lacks, and
// completes the data from the generated object of a render of the same side.
func (g *gb) generatorData() error {
	files := map[string]string{} // logical path → artifact id (every environment file)
	for _, a := range g.bundle.Manifest.Artifacts {
		if a.Kind == domain.ArtifactEnvironmentFile {
			files[a.Path] = a.ID
		}
	}
	v2 := g.rules >= ConsumerRulesV2
	for _, k := range sortedKusts(g.env) {
		for _, kind := range []string{"configMapGenerator", "secretGenerator"} {
			objKind := "ConfigMap"
			if kind == "secretGenerator" {
				if !v2 {
					continue
				}
				objKind = "Secret"
			}
			for _, gen := range asSeq(k.body[kind]) {
				name := str(gen, "name")
				if name == "" {
					continue // kustomize rejects a generator without a name
				}
				ns := g.env.dirNS[k.dir]
				if ns == "" {
					ns = str(gen, "namespace")
				}
				key := render.ObjectID{Version: "v1", Kind: objKind, Namespace: ns, Name: name}.String()
				if n := g.byKey[key]; n == nil || n.obj != nil {
					continue // an object of the snapshot states its own data
				}
				gd := &genData{keys: map[string]bool{}}
				lacks := func(dataKey, spec, what string) {
					gd.unknown = append(gd.unknown, unknownData{key: dataKey, artifact: k.artifact,
						reason: fmt.Sprintf("%s %s is generated by %s of %s from %s, which is not in the environment snapshot", objKind, name, kind, k.file, what),
						line:   g.lineAt(k.artifact, spec)})
				}
				for _, f := range asSeq(asMap(gen)["files"]) {
					spec, _ := f.(string)
					dataKey, rel := path.Base(spec), spec
					if i := strings.Index(spec, "="); i > 0 {
						dataKey, rel = spec[:i], spec[i+1:]
					}
					gd.keys[dataKey] = true
					id, ok := files[path.Join(k.dir, rel)]
					if !ok {
						lacks(dataKey, spec, "file "+path.Join(k.dir, rel))
						continue
					}
					if objKind == "Secret" {
						continue // its content is withheld; the key is what a consumer needs
					}
					b, err := g.read(id)
					if err != nil {
						return err
					}
					gd.entries = append(gd.entries, dataEntry{key: dataKey, text: string(b), artifact: id, line: 1})
				}
				for _, l := range asSeq(asMap(gen)["literals"]) {
					lit, _ := l.(string)
					if i := strings.Index(lit, "="); i > 0 {
						gd.keys[lit[:i]] = true
						if objKind == "ConfigMap" {
							gd.entries = append(gd.entries, dataEntry{key: lit[:i], text: lit[i+1:], artifact: k.artifact, line: g.lineOf(k.artifact, lit)})
						}
					}
				}
				envs := asSeq(asMap(gen)["envs"])
				if e, ok := asMap(gen)["env"].(string); ok {
					envs = append(envs, e)
				}
				for _, e := range envs {
					rel, _ := e.(string)
					id, ok := files[path.Join(k.dir, rel)]
					if !ok {
						lacks("", rel, "env file "+path.Join(k.dir, rel))
						continue
					}
					b, err := g.read(id)
					if err != nil {
						return err
					}
					for i, line := range strings.Split(string(b), "\n") {
						if j := strings.Index(line, "="); j > 0 && !strings.HasPrefix(strings.TrimSpace(line), "#") {
							gd.keys[strings.TrimSpace(line[:j])] = true
							if objKind == "ConfigMap" {
								gd.entries = append(gd.entries, dataEntry{key: strings.TrimSpace(line[:j]), text: line[j+1:], artifact: id, line: i + 1})
							}
						}
					}
				}
				if v2 && len(gd.unknown) > 0 {
					if err := g.renderedData(k, gen, objKind, name, ns, gd); err != nil {
						return err
					}
				}
				g.gen[key] = gd
			}
		}
	}
	return nil
}

// configData is a ConfigMap node's data: its object's data (located in its
// artifact), else its generator's (gd). ok is false when the snapshot does
// not hold the data (a placeholder).
func (g *gb) configData(n *gnode) (entries []dataEntry, gd *genData, ok bool) {
	if n.obj != nil {
		var out []dataEntry
		data := asMap(n.obj.body["data"])
		keys := make([]string, 0, len(data))
		for k := range data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if s, ok := data[k].(string); ok {
				out = append(out, dataEntry{key: k, text: s, artifact: n.obj.artifact})
			}
		}
		return out, nil, true
	}
	if gd, ok := g.gen[n.n.Key]; ok {
		return gd.entries, gd, true
	}
	return nil, nil, false
}

// keysOf is the set of keys a provider object is known to hold (ok false:
// not known — a placeholder, a generated object whose sources the bundle
// lacks, a generated Secret under rules v1).
func (g *gb) keysOf(n *gnode) (map[string]bool, bool) {
	if n.obj == nil {
		gd, ok := g.gen[n.n.Key]
		if !ok {
			return nil, false
		}
		out := map[string]bool{}
		if g.rules < ConsumerRulesV2 {
			for _, e := range gd.entries {
				out[e.key] = true
			}
			return out, true
		}
		for _, u := range gd.unknown {
			if u.key == "" {
				return nil, false
			}
		}
		for k := range gd.keys {
			out[k] = true
		}
		return out, true
	}
	out := map[string]bool{}
	for _, f := range []string{"data", "stringData", "binaryData"} {
		for k := range asMap(n.obj.body[f]) {
			out[k] = true
		}
	}
	return out, true
}

// --- generated objects in the renders (rules v2) ----------------------------------------------

// hashSuffix is the name suffix kustomize gives a generated object: ten
// characters of its content hash, hex digits 0, 1, 3, a and e mapped to g, h,
// k, m and t.
var hashSuffix = regexp.MustCompile(`^[2456789bcdfghkmt]{10}$`)

// renderedObj is a generated object found in a render.
type renderedObj struct {
	id               render.ObjectID
	body             map[string]any
	artifact, target string
	overlay          bool
}

// renderedData completes the data of generator gen of kustomization k (kind,
// name, ns: the object it generates) from the generated object a render of
// the graph's side holds: the render of an overlay whose kustomization tree
// includes k, an object of the kind named as kustomize names it (the name
// prefixes and suffixes of the tree, then the hash suffix unless disabled),
// in the namespace. With no such object, or several with different data, the
// unknown data stays unknown and its reason says so.
func (g *gb) renderedData(k *kustomization, gen any, kind, name, ns string, gd *genData) error {
	found, err := g.renderedGenerated(k, gen, kind, name, ns)
	if err != nil {
		return err
	}
	why := ""
	switch {
	case len(found) == 0:
		why = fmt.Sprintf("; no render of the %s side holds the generated object", g.side)
	default:
		for _, o := range found[1:] {
			if !sameData(kind, o.body, found[0].body) {
				why = fmt.Sprintf("; %d rendered objects match it with different data (%s in %s, %s in %s), so which one it is is not decided", len(found), found[0].id, found[0].target, o.id, o.target)
				break
			}
		}
	}
	if why != "" {
		for i := range gd.unknown {
			gd.unknown[i].reason += why
		}
		return nil
	}
	o := found[0]
	note := ""
	if o.overlay {
		note = ", a version-independent overlay: the same objects at both sides"
	}
	gd.from = fmt.Sprintf("rendered object %s of render %s (artifact %s%s); %s", o.id, o.target, o.artifact, note, gd.unknown[0].reason)
	known := map[string]bool{}
	for _, e := range gd.entries {
		known[e.key] = true
	}
	for _, field := range []string{"data", "binaryData", "stringData"} {
		data := asMap(o.body[field])
		keys := make([]string, 0, len(data))
		for key := range data {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			gd.keys[key] = true
			if kind != "ConfigMap" || field != "data" || known[key] {
				continue
			}
			if s, ok := data[key].(string); ok {
				id := o.id
				gd.entries = append(gd.entries, dataEntry{key: key, text: s, artifact: o.artifact, obj: &id, from: gd.from})
			}
		}
	}
	gd.unknown = nil
	if n := g.byKey[render.ObjectID{Version: "v1", Kind: kind, Namespace: ns, Name: name}.String()]; n != nil {
		n.arts[o.artifact] = true
	}
	return nil
}

// renderedGenerated finds the objects generator gen of kustomization k
// generates in the renders of the graph's side: complete environment pairs
// (the side's render) and version-independent overlays (not-applicable
// pairs whose one render states the objects at both sides).
func (g *gb) renderedGenerated(k *kustomization, gen any, kind, name, ns string) ([]renderedObj, error) {
	hashed := true
	if b, ok := get(k.body, "generatorOptions", "disableNameSuffixHash").(bool); ok {
		hashed = !b
	}
	if b, ok := get(gen, "options", "disableNameSuffixHash").(bool); ok {
		hashed = !b
	}
	var out []renderedObj
	for _, p := range g.bundle.Renders {
		if p.Scope != domain.RenderEnvironment {
			continue
		}
		art, overlay := "", false
		switch {
		case p.Status == domain.CollectionOK && g.side == SideFrom:
			art = p.From
		case p.Status == domain.CollectionOK:
			art = p.To
		case p.Status == domain.CollectionNotApplicable && p.From != "":
			art, overlay = p.From, true
		}
		if art == "" {
			continue
		}
		prefix, suffix, ok := g.renderedBy(p.Target, k.dir)
		if !ok {
			continue
		}
		objs, err := g.renderObjects(art)
		if err != nil {
			return nil, err
		}
		want := prefix + name + suffix
		for _, o := range objs {
			if o.ID.Group != "" || o.ID.Kind != kind || (ns != "" && o.ID.Namespace != "" && o.ID.Namespace != ns) {
				continue
			}
			if hashed && !(strings.HasPrefix(o.ID.Name, want+"-") && hashSuffix.MatchString(o.ID.Name[len(want)+1:])) || !hashed && o.ID.Name != want {
				continue
			}
			out = append(out, renderedObj{id: o.ID, body: o.Body, artifact: art, target: p.Target, overlay: overlay})
		}
	}
	return out, nil
}

// renderedBy reports whether render target t builds kustomization directory
// dir (a kustomize target, or a GitOps application whose source path is a
// kustomization including it), with the name prefix and suffix the
// kustomizations from t's root down to dir give dir's objects.
func (g *gb) renderedBy(t, dir string) (prefix, suffix string, ok bool) {
	i := strings.Index(t, ":")
	if i < 0 {
		return "", "", false
	}
	var roots []string
	if t[:i] == "kustomize" {
		roots = []string{"env/" + strings.TrimPrefix(path.Clean(t[i+1:]), "./")}
	} else {
		for _, a := range g.env.appsOfTarget(t) {
			roots = append(roots, appSourceDirs(a)...)
		}
	}
	for _, r := range roots {
		if prefix, suffix, ok := g.kustChain(r, dir, map[string]bool{}); ok {
			return prefix, suffix, true
		}
	}
	return "", "", false
}

// kustChain follows resources/bases/components from kustomization from to
// kustomization to, accumulating namePrefix (outer first) and nameSuffix
// (outer last).
func (g *gb) kustChain(from, to string, seen map[string]bool) (prefix, suffix string, ok bool) {
	k := g.env.kustomizations[from]
	if k == nil || seen[from] {
		return "", "", false
	}
	seen[from] = true
	p, s := str(k.body, "namePrefix"), str(k.body, "nameSuffix")
	if from == to {
		return p, s, true
	}
	for _, key := range []string{"resources", "bases", "components"} {
		for _, r := range asSeq(k.body[key]) {
			if rs, _ := r.(string); rs != "" {
				if ip, is, ok := g.kustChain(path.Join(from, rs), to, seen); ok {
					return p + ip, is + s, true
				}
			}
		}
	}
	return "", "", false
}

func (g *gb) renderObjects(art string) ([]render.Object, error) {
	if objs, ok := g.renders[art]; ok {
		return objs, nil
	}
	b, err := g.read(art)
	if err != nil {
		return nil, err
	}
	objs, err := render.ParseObjects(b)
	if err != nil {
		objs = nil // an unparsable render holds no object (as in load)
	}
	g.renders[art] = objs
	return objs, nil
}

// sameData compares the data of two generated objects: a ConfigMap's keys
// and text, a Secret's keys (its values are withheld).
func sameData(kind string, a, b map[string]any) bool {
	for _, field := range []string{"data", "binaryData", "stringData"} {
		x, y := asMap(a[field]), asMap(b[field])
		if len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || kind == "ConfigMap" && fmt.Sprint(v) != fmt.Sprint(w) {
				return false
			}
		}
	}
	return true
}

// --- reads-file edges --------------------------------------------------------------------

// mountRef is one container mount with its volume.
type mountRef struct {
	volume, mountPath, subPath string
	vol                        map[string]any
}

// reader is a ConfigMap whose data a container reads: through a volume
// (keys: the keys visible in the mount; nil = all) or through env.
type reader struct {
	name, via, volume string
	keys              map[string]bool
}

// absPath finds absolute file paths in configuration text. A path starts
// after a delimiter (not after a word character, a '/', '$' or '}', so URLs
// and ${VAR}/x are not paths) and has segments of portable file-name
// characters.
var absPath = regexp.MustCompile(`(?:^|[^A-Za-z0-9._~/$}\-])(/[A-Za-z0-9._@+\-]+(?:/[A-Za-z0-9._@+\-]+)*)`)

func filePaths(text string) []string {
	text = strings.ReplaceAll(text, "file://", " ")
	var out []string
	seen := map[string]bool{}
	for _, m := range absPath.FindAllStringSubmatch(text, -1) {
		p := strings.TrimRight(m[1], ".")
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func under(p, mountPath string) (rel string, ok bool) {
	mp := strings.TrimSuffix(mountPath, "/")
	switch {
	case mp == "":
		return "", false
	case p == mp:
		return "", true
	case strings.HasPrefix(p, mp+"/"):
		return p[len(mp)+1:], true
	}
	return "", false
}

func volumeType(v map[string]any) string {
	var ks []string
	for k := range v {
		if k != "name" {
			ks = append(ks, k)
		}
	}
	sort.Strings(ks)
	if len(ks) == 0 {
		return "unknown"
	}
	return ks[0]
}

// readsFile adds the reads-file edges of one workload: for each container,
// each ConfigMap it reads (mounted or through env), each absolute path in the
// visible data, the provider the path resolves to through the container's
// other mounts — or an unresolved edge saying why none. Under rules v3 the
// paths the container's own spec names follow (podSpecFiles).
func (g *gb) readsFile(w *gnode) error {
	spec, _, prefix := podTemplate(w.obj)
	if spec == nil {
		return nil
	}
	vols := map[string]map[string]any{}
	for _, v := range asSeq(spec["volumes"]) {
		if m := asMap(v); m != nil && str(m, "name") != "" {
			vols[str(m, "name")] = m
		}
	}
	automount := true
	if b, ok := spec["automountServiceAccountToken"].(bool); ok {
		automount = b
	}
	for _, c := range podContainers(spec) {
		var mounts []mountRef
		for _, vm := range asSeq(c.body["volumeMounts"]) {
			if v := vols[str(vm, "name")]; v != nil && str(vm, "mountPath") != "" {
				mounts = append(mounts, mountRef{volume: str(vm, "name"), mountPath: str(vm, "mountPath"), subPath: str(vm, "subPath"), vol: v})
			}
		}
		for _, r := range containerReaders(c, mounts) {
			rn, _, _ := g.resolve("", true, "ConfigMap", r.name, w.effNS)
			if rn == nil {
				continue
			}
			data, gd, ok := g.configData(rn)
			if !ok {
				// Its data is not in the snapshot: the references edge says so;
				// under rules v2 a reads-file edge says what is unknown.
				if g.rules >= ConsumerRulesV2 {
					g.unknownData(w, c.name, mounts, r, rn, unknownData{reason: fmt.Sprintf("%s is not an object of the snapshot (%s), so its data is not known", rn.n.Key, rn.phReason)})
				}
				continue
			}
			if gd != nil && g.rules >= ConsumerRulesV2 {
				for _, u := range gd.unknown {
					if u.key == "" || r.keys == nil || r.keys[u.key] {
						g.unknownData(w, c.name, mounts, r, rn, u)
					}
				}
			}
			for _, d := range data {
				if r.keys != nil && !r.keys[d.key] {
					continue
				}
				for _, p := range filePaths(d.text) {
					g.fileEdge(w, c.name, mounts, r, rn, d, p, automount)
				}
			}
		}
		if g.rules >= ConsumerRulesV3 {
			g.podSpecFiles(w, prefix, c, mounts, automount)
		}
	}
	return nil
}

// containerReaders are the ConfigMaps a container reads, in a fixed order.
func containerReaders(c container, mounts []mountRef) []reader {
	var out []reader
	visible := func(items []any, subPath string) map[string]bool {
		if len(items) == 0 && subPath == "" {
			return nil
		}
		keys := map[string]bool{}
		for _, it := range items {
			if subPath == "" || str(it, "path") == subPath || strings.HasPrefix(subPath, str(it, "path")+"/") {
				keys[str(it, "key")] = true
			}
		}
		if len(items) == 0 {
			keys[strings.SplitN(subPath, "/", 2)[0]] = true
		}
		return keys
	}
	for _, m := range mounts {
		if name := str(m.vol, "configMap", "name"); name != "" {
			out = append(out, reader{name: name, via: "volume", volume: m.volume, keys: visible(asSeq(get(m.vol, "configMap", "items")), m.subPath)})
		}
		for _, src := range asSeq(get(m.vol, "projected", "sources")) {
			if name := str(src, "configMap", "name"); name != "" {
				out = append(out, reader{name: name, via: "volume", volume: m.volume, keys: visible(asSeq(get(src, "configMap", "items")), m.subPath)})
			}
		}
	}
	for _, e := range asSeq(c.body["env"]) {
		if name, key := str(e, "valueFrom", "configMapKeyRef", "name"), str(e, "valueFrom", "configMapKeyRef", "key"); name != "" && key != "" {
			out = append(out, reader{name: name, via: "env", keys: map[string]bool{key: true}})
		}
	}
	for _, e := range asSeq(c.body["envFrom"]) {
		if name := str(e, "configMapRef", "name"); name != "" {
			out = append(out, reader{name: name, via: "env"})
		}
	}
	return out
}

// candidate is one provider a matched path may come from.
type candidate struct {
	kind, name, key string
}

// fileSite is where a matched path is named: the node its edges point to
// (the configuration that names it, or under rules v3 the workload whose pod
// spec does), what is known of the match before it resolves, and the
// edges' detail, line, artifacts and rules (keysRule: rules v2, a key the
// bundle does not know).
type fileSite struct {
	to             *gnode
	file           *domain.EdgeFile
	detail         string
	line           *domain.EdgeLine
	arts           []string
	rule, keysRule string
}

// longestMount is the mount of mounts p is under, the longest mountPath
// winning, and p relative to it (nil: under none).
func longestMount(mounts []mountRef, p string) (*mountRef, string) {
	var best *mountRef
	var rel string
	for i := range mounts {
		if x, ok := under(p, mounts[i].mountPath); ok && (best == nil || len(mounts[i].mountPath) > len(best.mountPath)) {
			best, rel = &mounts[i], x
		}
	}
	return best, rel
}

// fileEdge resolves one path p named in data entry d of reader rn, read by
// container c of workload w, and adds its reads-file edge(s).
func (g *gb) fileEdge(w *gnode, c string, mounts []mountRef, r reader, rn *gnode, d dataEntry, p string, automount bool) {
	best, rel := longestMount(mounts, p)
	segments := strings.Count(p, "/")
	if best == nil && segments < 2 {
		return // a one-segment path under no mount (a URL route, a command) is not taken for a file
	}
	if best != nil && r.via == "volume" && best.volume == r.volume {
		return // the configuration names a file of its own volume: resolved within itself
	}
	rule := ruleReadsFile + r.via
	if d.from != "" {
		rule = ruleReadsFileV2 + "rendered-data:" + r.via
	}
	g.resolveFile(w, c, best, rel, p, automount, fileSite{
		to:       rn,
		file:     &domain.EdgeFile{ConfigKey: d.key, Path: p, Workload: w.n.Key, Container: c, Via: r.via, DataFrom: d.from},
		detail:   fmt.Sprintf("%s %s data[%s] names %s (container %s of %s)", r.via, rn.n.Key, d.key, p, c, w.n.Key),
		line:     g.dataLine(rn, d, p),
		arts:     []string{d.artifact, w.obj.artifact},
		rule:     rule,
		keysRule: ruleReadsFileV2 + "unknown-keys:" + r.via,
	})
}

// resolveFile adds the reads-file edge(s) of path p, named at site s and
// under mount best (rel: p relative to it; nil: under none) of container c
// of workload w: from the provider the path resolves to, or from a
// placeholder with the reason none does.
func (g *gb) resolveFile(w *gnode, c string, best *mountRef, rel, p string, automount bool, s fileSite) {
	file := s.file
	unresolved := func(status domain.EdgeStatus, reason string) {
		ext := g.node(domain.NodeExternal, render.ObjectID{Kind: "File", Name: p}, "file:"+w.n.Key+"#"+c+":"+p, "")
		ext.n.Placeholder = true
		g.fileEdgeTo(ext, s.to, status, s.rule, s.detail, reason, file, s.line, s.arts)
	}
	if best == nil {
		if _, ok := under(p, saTokenDir); ok && automount {
			unresolved(domain.EdgeInferred, fmt.Sprintf("%s is under %s, where Kubernetes mounts the pod's service account token (automountServiceAccountToken is not false); no volume of the spec mounts it", p, saTokenDir))
			return
		}
		unresolved(domain.EdgeMissing, fmt.Sprintf("%s is under no volume mount of container %s of %s: what provides the file is not in the snapshot", p, c, w.n.Key))
		return
	}
	file.Volume, file.MountPath = best.volume, best.mountPath
	vrel := rel
	if best.subPath != "" {
		vrel = strings.Trim(best.subPath+"/"+rel, "/")
	}
	var cands []candidate
	var why string
	switch vt := volumeType(best.vol); vt {
	case "secret":
		cands, why = sourceCandidate("Secret", str(best.vol, "secret", "secretName"), asSeq(get(best.vol, "secret", "items")), vrel)
	case "configMap":
		cands, why = sourceCandidate("ConfigMap", str(best.vol, "configMap", "name"), asSeq(get(best.vol, "configMap", "items")), vrel)
	case "projected":
		cands, why = g.projectedCandidates(asSeq(get(best.vol, "projected", "sources")), vrel, w.effNS)
	default:
		unresolved(domain.EdgeMissing, fmt.Sprintf("%s is under mountPath %s of volume %s (%s), which is not a Secret or ConfigMap: its content is not in the snapshot", p, best.mountPath, best.volume, vt))
		return
	}
	if len(cands) == 0 {
		unresolved(domain.EdgeMissing, fmt.Sprintf("%s is under mountPath %s of volume %s, but %s", p, best.mountPath, best.volume, why))
		return
	}
	for _, cd := range cands {
		pn, _, _ := g.resolve("", true, cd.kind, cd.name, w.effNS)
		if pn == nil {
			continue
		}
		f := *file
		f.Key = cd.key
		status, reason := domain.EdgeEstablished, ""
		if len(cands) > 1 {
			status, reason = domain.EdgeInferred, fmt.Sprintf("%s is under mountPath %s of projected volume %s, whose sources without items (%d) may each hold key %q; which one does is not decided from the snapshot", p, best.mountPath, best.volume, len(cands), cd.key)
		}
		rule := s.rule
		keys, known := g.keysOf(pn)
		switch {
		case known && cd.key != "" && !keys[cd.key]:
			status, reason = domain.EdgeMissing, fmt.Sprintf("%s maps to key %q of %s, which holds no such key", p, cd.key, pn.n.Key)
		case !known && cd.key != "" && !pn.n.Placeholder && g.rules >= ConsumerRulesV2:
			// A generated object whose keys the bundle does not state (a
			// placeholder's absence is its references edge's gap).
			why := fmt.Sprintf("%s maps to key %q of %s, whose keys are not in the snapshot: that it holds the key is not established", p, cd.key, pn.n.Key)
			if reason != "" {
				why = reason + "; " + why
			}
			status, reason, rule = domain.EdgeInferred, why, s.keysRule
		}
		g.fileEdgeTo(pn, s.to, status, rule, s.detail, reason, &f, s.line, s.arts)
	}
}

// --- pod-spec paths (rules v3) -------------------------------------------------------------

// kubeVar is a Kubernetes variable reference, $(NAME), which the kubelet
// expands in command, args and env values: the path it starts is not known
// from the spec, so it is masked like a shell ${VAR} before matching.
var kubeVar = regexp.MustCompile(`\$\([A-Za-z_][A-Za-z0-9_.\-]*\)`)

// podSpecFiles adds the reads-file edges of the absolute paths container c's
// own spec names (rules v3): its command and args (shell scripts passed to
// `sh -c` included) and its literal env values. valueFrom env entries are
// references edges already. Each path resolves through the same container's
// mounts, as configuration's paths do, and its edge points to the workload
// w, citing the field and the line that state it. The program a container
// runs (command[0]) under no mount comes from its image, so it is not taken
// for a file it reads.
func (g *gb) podSpecFiles(w *gnode, prefix string, c container, mounts []mountRef, automount bool) {
	at := fmt.Sprintf("%s.%s[%d:%s]", prefix, c.field, c.idx, c.name)
	type stated struct{ field, kind, text string }
	var fields []stated
	for _, kind := range []string{"command", "args"} {
		for i, v := range asSeq(c.body[kind]) {
			if t, ok := v.(string); ok {
				fields = append(fields, stated{fmt.Sprintf("%s.%s[%d]", at, kind, i), kind, t})
			}
		}
	}
	for j, e := range asSeq(c.body["env"]) {
		if t, ok := asMap(e)["value"].(string); ok && asMap(e)["valueFrom"] == nil {
			fields = append(fields, stated{fmt.Sprintf("%s.env[%d:%s].value", at, j, str(e, "name")), "env", t})
		}
	}
	for i, f := range fields {
		for _, p := range filePaths(kubeVar.ReplaceAllString(f.text, "${}")) {
			best, rel := longestMount(mounts, p)
			if best == nil && (strings.Count(p, "/") < 2 || i == 0 && f.kind == "command") {
				continue // a route, or the program the container runs, from its image
			}
			g.resolveFile(w, c.name, best, rel, p, automount, fileSite{
				to:       w,
				file:     &domain.EdgeFile{Path: p, Workload: w.n.Key, Container: c.name, Via: "pod-spec", Field: f.field},
				detail:   fmt.Sprintf("pod spec %s names %s (container %s of %s)", f.field, p, c.name, w.n.Key),
				line:     g.fieldLine(w, f.field, p),
				arts:     []string{w.obj.artifact},
				rule:     ruleReadsFileV3 + "pod-spec:" + f.kind,
				keysRule: ruleReadsFileV3 + "unknown-keys:pod-spec",
			})
		}
	}
}

// fieldLine locates path p in the pod-spec field of workload w (the field's
// first line when p is not found on one line of it).
func (g *gb) fieldLine(w *gnode, field, p string) *domain.EdgeLine {
	pa := g.parsed(w.obj.artifact)
	segs, ok := parseFieldPath(field)
	if pa == nil || !ok {
		return nil
	}
	start, end := pa.span(w.obj.id, segs)
	if start == 0 {
		return nil
	}
	return pa.find(w.obj.artifact, start, end, p)
}

// sourceCandidate maps a volume-relative path to the key of one Secret or
// ConfigMap source ("" = the volume's directory itself).
func sourceCandidate(kind, name string, items []any, vrel string) ([]candidate, string) {
	if name == "" {
		return nil, "the volume names no " + kind
	}
	if vrel == "" {
		return []candidate{{kind: kind, name: name}}, ""
	}
	if len(items) > 0 {
		for _, it := range items {
			if str(it, "path") == vrel {
				return []candidate{{kind: kind, name: name, key: str(it, "key")}}, ""
			}
		}
		return nil, fmt.Sprintf("no item of %s %s maps a key to %q", kind, name, vrel)
	}
	if strings.Contains(vrel, "/") {
		return nil, fmt.Sprintf("%s %s is mounted without items: it has no nested path %q", kind, name, vrel)
	}
	return []candidate{{kind: kind, name: name, key: vrel}}, ""
}

func (g *gb) projectedCandidates(sources []any, vrel, ns string) ([]candidate, string) {
	var withItems, without []candidate
	for _, src := range sources {
		for _, kind := range []string{"secret", "configMap"} {
			name := str(src, kind, "name")
			if name == "" {
				continue
			}
			k := "Secret"
			if kind == "configMap" {
				k = "ConfigMap"
			}
			items := asSeq(get(src, kind, "items"))
			if len(items) > 0 {
				if c, _ := sourceCandidate(k, name, items, vrel); len(c) > 0 {
					withItems = append(withItems, c...)
				}
				continue
			}
			if c, _ := sourceCandidate(k, name, nil, vrel); len(c) > 0 {
				without = append(without, c...)
			}
		}
	}
	if len(withItems) > 0 {
		return withItems[:1], ""
	}
	if len(without) > 1 { // narrow by the keys the snapshot knows the sources hold
		var known []candidate
		for _, c := range without {
			if pn, _, _ := g.resolve("", true, c.kind, c.name, ns); pn != nil {
				if keys, ok := g.keysOf(pn); ok && keys[c.key] {
					known = append(known, c)
				}
			}
		}
		if len(known) == 1 {
			return known, ""
		}
	}
	if len(without) == 0 {
		return nil, fmt.Sprintf("no Secret or ConfigMap source of the projected volume provides %q", vrel)
	}
	return without, ""
}

// unknownData adds the unresolved reads-file edge of configuration data u of
// ConfigMap rn that container c of workload w reads but the bundle does not
// hold (rules v2): from a placeholder standing for the files it may name.
func (g *gb) unknownData(w *gnode, c string, mounts []mountRef, r reader, rn *gnode, u unknownData) {
	shown := u.key
	if shown == "" {
		shown = "*"
	}
	ext := g.node(domain.NodeExternal, render.ObjectID{Kind: "ConfigData", Name: rn.n.Key + "[" + shown + "]"}, "data:"+w.n.Key+"#"+c+":"+rn.n.Key+"["+shown+"]", "")
	ext.n.Placeholder = true
	file := &domain.EdgeFile{ConfigKey: u.key, Workload: w.n.Key, Container: c, Via: r.via}
	for _, m := range mounts {
		if r.via == "volume" && m.volume == r.volume {
			file.Volume, file.MountPath = m.volume, m.mountPath
			break
		}
	}
	detail := fmt.Sprintf("%s %s data[%s] is not known (container %s of %s)", r.via, rn.n.Key, shown, c, w.n.Key)
	reason := u.reason + ": the files it names are not known"
	g.fileEdgeTo(ext, rn, domain.EdgeMissing, ruleReadsFileV2+"unknown-data:"+r.via, detail, reason, file, u.line, []string{u.artifact, w.obj.artifact})
}

func (g *gb) fileEdgeTo(from, to *gnode, status domain.EdgeStatus, rule, detail, reason string, f *domain.EdgeFile, line *domain.EdgeLine, arts []string) {
	g.edge(domain.EdgeReadsFile, from, to, status, rule, detail, reason, arts...)
	id := domain.EdgeID(domain.EdgeReadsFile, from.n.ID, to.n.ID, detail)
	if e, ok := g.edges[id]; ok {
		e.File, e.Line = f, line
		g.edges[id] = e
	}
}

// --- lines -------------------------------------------------------------------------------

// dataLine locates path p in data entry d of ConfigMap n.
func (g *gb) dataLine(n *gnode, d dataEntry, p string) *domain.EdgeLine {
	pa := g.parsed(d.artifact)
	if pa == nil {
		return nil
	}
	if d.line > 0 { // a generator file or env line: the text starts there
		for i := d.line; i <= len(pa.lines); i++ {
			if strings.Contains(pa.lines[i-1], p) {
				return pa.at(d.artifact, i)
			}
		}
		return pa.at(d.artifact, d.line)
	}
	id := n.id
	if d.obj != nil {
		id = *d.obj
	}
	start, end := pa.span(id, []seg{{key: "data"}, {key: d.key}})
	if start == 0 {
		return nil
	}
	return pa.find(d.artifact, start, end, p)
}

// find is the first line from start to end containing s, else start.
func (pa *parsedArtifact) find(artifact string, start, end int, s string) *domain.EdgeLine {
	for i := start; i <= end && i <= len(pa.lines); i++ {
		if strings.Contains(pa.lines[i-1], s) {
			return pa.at(artifact, i)
		}
	}
	return pa.at(artifact, start)
}

// lines gives references edges of pod specs and custom resource fields, and
// produces edges, the line of the referrer's (producer's) field.
func (g *gb) lines() error {
	ids := make([]string, 0, len(g.edges))
	for id := range g.edges {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		e := g.edges[id]
		if e.Line != nil {
			continue
		}
		var holder *gnode
		switch e.Kind {
		case domain.EdgeReferences:
			holder = g.nodes[e.To]
		case domain.EdgeProduces:
			holder = g.nodes[e.From]
		default:
			continue
		}
		if holder == nil || holder.obj == nil {
			continue
		}
		segs, ok := parseFieldPath(e.Detail)
		if !ok {
			continue
		}
		pa := g.parsed(holder.obj.artifact)
		if pa == nil {
			continue
		}
		if l, _ := pa.span(holder.obj.id, segs); l > 0 {
			e.Line = pa.at(holder.obj.artifact, l)
			g.edges[id] = e
		}
	}
	return nil
}

// lineAt is the first line of an artifact containing s, located (nil: none).
func (g *gb) lineAt(artifact, s string) *domain.EdgeLine {
	if l := g.lineOf(artifact, s); l > 0 {
		return g.parsed(artifact).at(artifact, l)
	}
	return nil
}

// lineOf is the first line of an artifact containing s (0: none).
func (g *gb) lineOf(artifact, s string) int {
	if pa := g.parsed(artifact); pa != nil {
		for i, l := range pa.lines {
			if strings.Contains(l, s) {
				return i + 1
			}
		}
	}
	return 0
}

// parsedArtifact is an artifact's lines and YAML documents.
type parsedArtifact struct {
	lines []string
	docs  []*yaml.Node
}

func (g *gb) parsed(id string) *parsedArtifact {
	if id == "" {
		return nil
	}
	if pa, ok := g.docs[id]; ok {
		return pa
	}
	var pa *parsedArtifact
	if b, err := g.read(id); err == nil {
		pa = &parsedArtifact{lines: strings.Split(string(b), "\n")}
		dec := yaml.NewDecoder(bytes.NewReader(b))
		for {
			var n yaml.Node
			if err := dec.Decode(&n); err != nil {
				if !errors.Is(err, io.EOF) {
					pa.docs = nil // not YAML (a configuration file): lines only
				}
				break
			}
			pa.docs = append(pa.docs, &n)
		}
	}
	g.docs[id] = pa
	return pa
}

func (pa *parsedArtifact) at(artifact string, line int) *domain.EdgeLine {
	if line < 1 || line > len(pa.lines) {
		return nil
	}
	text := strings.TrimSpace(pa.lines[line-1])
	if len(text) > maxLineText {
		cut := maxLineText
		for cut > 0 && (text[cut]&0xC0) == 0x80 {
			cut--
		}
		text = text[:cut]
	}
	return &domain.EdgeLine{Artifact: artifact, Line: line, Text: text}
}

// seg is one step of a field path: a mapping key, then optional sequence
// indices.
type seg struct {
	key string
	idx []int
}

// parseFieldPath parses an edge detail of the form
// "spec.template.spec.volumes[0:tls].secret.secretName" (indices "[i]" or
// "[i:name]"). Details that are not field paths do not parse.
func parseFieldPath(s string) ([]seg, bool) {
	if s == "" || strings.ContainsAny(s, " ") {
		return nil, false
	}
	var out []seg
	for _, part := range strings.Split(s, ".") {
		sg := seg{key: part}
		if i := strings.IndexByte(part, '['); i >= 0 {
			sg.key = part[:i]
			rest := part[i:]
			for rest != "" {
				end := strings.IndexByte(rest, ']')
				if rest[0] != '[' || end < 0 {
					return nil, false
				}
				num := rest[1:end]
				if j := strings.IndexByte(num, ':'); j >= 0 {
					num = num[:j]
				}
				n, err := strconv.Atoi(num)
				if err != nil || n < 0 {
					return nil, false
				}
				sg.idx = append(sg.idx, n)
				rest = rest[end+1:]
			}
		}
		if sg.key == "" {
			return nil, false
		}
		out = append(out, sg)
	}
	return out, true
}

// span returns the first and last line of the node at segs in the document
// of the object id (0, 0 when the object or the path is not found). For a
// block scalar the last line covers its text.
func (pa *parsedArtifact) span(id render.ObjectID, segs []seg) (int, int) {
	for _, d := range pa.docs {
		root := d
		if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
			root = root.Content[0]
		}
		if root.Kind != yaml.MappingNode || scalarAt(root, "kind") != id.Kind || scalarAt(mapAt(root, "metadata"), "name") != id.Name {
			continue
		}
		if ns := scalarAt(mapAt(root, "metadata"), "namespace"); ns != "" && id.Namespace != "" && ns != id.Namespace {
			continue
		}
		cur, start := root, root.Line
		for _, s := range segs {
			var next *yaml.Node
			if cur.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(cur.Content); i += 2 {
					if cur.Content[i].Value == s.key {
						start, next = cur.Content[i].Line, cur.Content[i+1]
						break
					}
				}
			}
			if next == nil {
				return 0, 0
			}
			cur = next
			for _, i := range s.idx {
				if cur.Kind != yaml.SequenceNode || i >= len(cur.Content) {
					return 0, 0
				}
				cur = cur.Content[i]
				start = cur.Line
			}
		}
		end := cur.Line
		if cur.Kind == yaml.ScalarNode && (cur.Style == yaml.LiteralStyle || cur.Style == yaml.FoldedStyle) {
			end += strings.Count(strings.TrimRight(cur.Value, "\n"), "\n") + 1
		}
		return start, end
	}
	return 0, 0
}

func mapAt(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func scalarAt(n *yaml.Node, key string) string {
	if v := mapAt(n, key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}
