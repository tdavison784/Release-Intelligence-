package relations

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/render"
)

const (
	ruleRefs      = "graph.k8s-refs@v1:"
	ruleSelectors = "graph.selectors@v1:"
	ruleKustomize = "graph.kustomize@v1:"
	ruleGitOps    = "graph.gitops@v1:"
	ruleRender    = "graph.render@v1:"
	ruleCRD       = "graph.crd@v1:"
	ruleProduces  = "graph.produces@v1:"
)

// --- kustomize --------------------------------------------------------------------------------

func (g *gb) kustNode(k *kustomization) *gnode {
	return g.byKey[k.key()]
}

// generatorNodes creates the ConfigMaps and Secrets kustomize generates.
func (g *gb) generatorNodes() {
	for _, k := range sortedKusts(g.env) {
		for _, kind := range []string{"configMapGenerator", "secretGenerator"} {
			objKind := "ConfigMap"
			if kind == "secretGenerator" {
				objKind = "Secret"
			}
			for _, gen := range asSeq(k.body[kind]) {
				name := str(gen, "name")
				if name == "" {
					continue
				}
				ns := g.env.dirNS[k.dir]
				if ns == "" {
					ns = str(gen, "namespace")
				}
				id := render.ObjectID{Version: "v1", Kind: objKind, Namespace: ns, Name: name}
				if g.find(id.String()) != nil {
					continue
				}
				n := g.node(classify(id), id, id.String(), k.artifact)
				n.effNS = ns
			}
		}
	}
}

func (g *gb) objectsOfFile(p string) []*gnode {
	var out []*gnode
	for o, n := range g.objNode {
		if o.file == p && n.obj == o {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].n.ID < out[j].n.ID })
	return out
}

func (g *gb) kustomizeEdges() {
	for _, k := range sortedKusts(g.env) {
		kn := g.kustNode(k)
		for _, key := range []string{"resources", "bases", "components"} {
			for _, r := range asSeq(k.body[key]) {
				rs, _ := r.(string)
				if rs == "" {
					continue
				}
				p := path.Join(k.dir, rs)
				detail := key + ": " + rs
				if _, ok := g.env.files[p]; ok {
					for _, n := range g.objectsOfFile(p) {
						g.edge(domain.EdgeRendersTo, kn, n, domain.EdgeEstablished, ruleKustomize+key, detail, "", k.artifact)
					}
				} else if sub := g.env.kustomizations[p]; sub != nil {
					g.edge(domain.EdgeDeploys, kn, g.kustNode(sub), domain.EdgeEstablished, ruleKustomize+key, detail, "", k.artifact)
				}
			}
		}
		for _, kind := range []string{"configMapGenerator", "secretGenerator"} {
			objKind := "ConfigMap"
			if kind == "secretGenerator" {
				objKind = "Secret"
			}
			for _, gen := range asSeq(k.body[kind]) {
				name := str(gen, "name")
				ns := g.env.dirNS[k.dir]
				if ns == "" {
					ns = str(gen, "namespace")
				}
				id := render.ObjectID{Version: "v1", Kind: objKind, Namespace: ns, Name: name}
				if n := g.byKey[id.String()]; n != nil && name != "" {
					g.edge(domain.EdgeRendersTo, kn, n, domain.EdgeEstablished, ruleKustomize+kind, kind+": "+name, "", k.artifact)
				}
			}
		}
	}
}

// --- GitOps -----------------------------------------------------------------------------------

func (g *gb) gitopsEdges() {
	for _, n := range g.sortedNodes() {
		o := n.obj
		if o == nil || o.file == "" || n.n.Kind != domain.NodeGitOpsApplication {
			continue
		}
		for _, dir := range appSourceDirs(o) {
			detail := "source path " + strings.TrimPrefix(dir, "env/")
			if k := g.env.kustomizations[dir]; k != nil {
				g.edge(domain.EdgeDeploys, n, g.kustNode(k), domain.EdgeEstablished, ruleGitOps+"source-path", detail, "", o.artifact)
				continue
			}
			for p := range g.env.files {
				if path.Dir(p) == dir {
					for _, m := range g.objectsOfFile(p) {
						g.edge(domain.EdgeRendersTo, n, m, domain.EdgeEstablished, ruleGitOps+"source-path", detail, "", o.artifact)
					}
				}
			}
		}
		for _, s := range appSources(o) {
			vf := asSeq(get(s, "helm", "valueFiles"))
			for _, v := range vf {
				vs, _ := v.(string)
				if !strings.HasPrefix(vs, "$values/") {
					continue
				}
				p := "env/" + path.Clean(strings.TrimPrefix(vs, "$values/"))
				vn := g.node(domain.NodeValuesSource, render.ObjectID{Kind: "ValuesSource", Name: p}, p, "")
				detail := "helm.valueFiles: " + vs
				if a, ok := g.env.files[p]; ok {
					vn.arts[a.ID] = true
					g.edge(domain.EdgeConfigures, vn, n, domain.EdgeEstablished, ruleGitOps+"values-file", detail, "", a.ID, o.artifact)
				} else {
					vn.n.Placeholder = true
					g.edge(domain.EdgeConfigures, vn, n, domain.EdgeMissing, ruleGitOps+"values-file", detail,
						"values file "+p+" is not in the snapshot", o.artifact)
				}
			}
		}
		if o.id.Group == groupFluxHelm {
			for i, vf := range asSeq(get(o.body, "spec", "valuesFrom")) {
				kind, name := str(vf, "kind"), str(vf, "name")
				if (kind == "ConfigMap" || kind == "Secret") && name != "" {
					g.link(n, "", kind, name, o.effNS, ruleRefs+"valuesFrom", fmt.Sprintf("spec.valuesFrom[%d]", i))
				}
			}
		}
	}
}

// --- renders ----------------------------------------------------------------------------------

func (g *gb) renderEdges() {
	targets := make([]string, 0, len(g.env.pairObjects))
	for t := range g.env.pairObjects {
		targets = append(targets, t)
	}
	sort.Strings(targets)
	for _, t := range targets {
		var sources []*gnode
		for _, a := range g.env.appsOfTarget(t) {
			if n := g.objNode[a]; n != nil {
				sources = append(sources, n)
			}
		}
		if i := strings.Index(t, ":"); len(sources) == 0 && i > 0 && t[:i] == "kustomize" {
			if k := g.env.kustomizations["env/"+strings.TrimPrefix(path.Clean(t[i+1:]), "./")]; k != nil {
				sources = append(sources, g.kustNode(k))
			}
		}
		if len(sources) == 0 {
			key := fmt.Sprintf("%s@%s", g.bundle.Change.Product.ID, g.bundle.Change.To.Tag)
			n := g.node(domain.NodeProductRelease, render.ObjectID{Kind: "ProductRelease", Name: key}, key, "")
			sources = append(sources, n)
		}
		for _, o := range g.env.pairObjects[t] {
			on := g.objNode[o]
			if on == nil || on.obj != o {
				on = g.byKey[o.key()]
			}
			if on == nil {
				continue
			}
			for _, s := range sources {
				g.edge(domain.EdgeRendersTo, s, on, domain.EdgeEstablished, ruleRender+"renders-to", "render target "+t, "", o.artifact)
			}
		}
	}
}

// --- references -------------------------------------------------------------------------------

// anyGroup, as the group of a reference, means the group is not stated: match by kind and name.
const anyGroup = "*"

var placeholderVersion = map[string]string{"IngressClass": "v1"}

// resolve finds the object a reference names. It returns the node and how
// well the reference is established; an absent object becomes a placeholder
// node, with a producing custom resource when one is known.
func (g *gb) resolve(group string, groupStated bool, kind, name, ns string) (*gnode, domain.EdgeStatus, string) {
	cluster := g.env.cluster[kind]
	if cluster {
		ns = ""
	}
	var exact, unknown []*gnode
	for _, c := range g.byName[kind+"|"+name] {
		if groupStated && c.id.Group != group {
			continue
		}
		switch {
		case c.n.Placeholder:
			if cluster || c.effNS == ns {
				return c, c.phStatus, c.phReason
			}
		case cluster || c.effNS == ns:
			exact = append(exact, c)
		case c.effNS == "":
			unknown = append(unknown, c)
		case ns == "":
			unknown = append(unknown, c)
		}
	}
	if len(exact) > 0 {
		return exact[0], domain.EdgeEstablished, ""
	}
	if len(unknown) > 0 {
		reason := fmt.Sprintf("%s %q matched by name only: namespace %q of the referrer or of the object is not known", kind, name, ns)
		if len(unknown) > 1 {
			reason += fmt.Sprintf(" (%d candidates)", len(unknown))
		}
		return unknown[0], domain.EdgeInferred, reason
	}
	// Absent. When the referrer's namespace is unknown (a cluster-scoped
	// object reading a namespaced one), a producer of that name in any
	// namespace explains it — but only as inferred.
	nsUnknown := false
	if ns == "" && !cluster {
		var keys []string
		for k := range g.producers {
			if strings.HasPrefix(k, kind+"|") && strings.HasSuffix(k, "|"+name) {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			sort.Strings(keys)
			ns = strings.Split(keys[0], "|")[1]
			nsUnknown = true
		}
	}
	// Placeholder.
	version := "v1"
	if !groupStated || group != "" {
		version = "*"
	}
	if v, ok := placeholderVersion[kind]; ok {
		version = v
	}
	id := render.ObjectID{Group: group, Version: version, Kind: kind, Namespace: ns, Name: name}
	n := g.node(classify(id), id, id.String(), "")
	if !n.n.Placeholder {
		n.n.Placeholder = true
		n.effNS = ns
		n.phStatus = domain.EdgeMissing
		n.phReason = fmt.Sprintf("%s %q is not defined in the snapshot", kind, name)
		if ns != "" {
			n.phReason = fmt.Sprintf("%s %s/%s is not defined in the snapshot", kind, ns, name)
		}
		for _, p := range g.producers[kind+"|"+ns+"|"+name] {
			st := domain.EdgeInferred
			reason := fmt.Sprintf("%s %q is not defined in the snapshot; %s/%s names it by convention (%s), so its controller is presumed to create it", kind, name, p.node.id.Kind, p.node.id.Name, p.rule)
			if p.declared {
				st, reason = domain.EdgeEstablished, ""
			}
			g.edge(domain.EdgeProduces, p.node, n, st, p.rule, producesDetail(p), reason, p.node.obj.artifact)
			if n.phStatus != domain.EdgeEstablished {
				n.phStatus, n.phReason = st, reason
			}
		}
		g.index(n)
	}
	status, reason := n.phStatus, n.phReason
	if nsUnknown && status == domain.EdgeEstablished {
		status = domain.EdgeInferred
		reason = fmt.Sprintf("%s %q is produced in namespace %q, but the namespace of the referrer is not known", kind, name, ns)
	}
	return n, status, reason
}

func producesDetail(p producer) string { return p.detail }

// link adds a references edge target → referrer.
func (g *gb) link(referrer *gnode, group, kind, name, ns, rule, detail string) {
	g.linkRef(referrer, group, kind, name, ns, rule, detail, false)
}

// linkRef is link for a reference that may be optional (its absence is
// tolerated: a key selector or volume source with optional: true, an image
// pull secret). The graph treats both alike; a reference view records it.
func (g *gb) linkRef(referrer *gnode, group, kind, name, ns, rule, detail string, optional bool) string {
	return g.linkVia(referrer, group, kind, name, ns, rule, detail, optional, nsResolution{})
}

// linkVia is linkRef for a reference whose namespace a declared
// namespaceFrom resolution decided (or failed to decide): a non-established
// edge says how the namespace was decided, or why it could not be, and the
// edge cites the controller configuration it was read from. It returns the
// id of the edge ("" when the reference names nothing).
func (g *gb) linkVia(referrer *gnode, group, kind, name, ns, rule, detail string, optional bool, via nsResolution) string {
	groupStated := group != anyGroup
	target, status, reason := g.resolve(group, groupStated, kind, name, ns)
	if target == nil {
		return ""
	}
	if via.note != "" && status != domain.EdgeEstablished {
		reason += "; " + via.note
	}
	art := ""
	if referrer.obj != nil {
		art = referrer.obj.artifact
	}
	if g.view != nil {
		g.view.record(g, referrer, group, groupStated, kind, name, ns, rule, detail, optional, target, status, reason)
	}
	g.edge(domain.EdgeReferences, target, referrer, status, rule, detail, reason, append([]string{art}, via.arts...)...)
	return domain.EdgeID(domain.EdgeReferences, target.n.ID, referrer.n.ID, detail)
}

// podTemplate returns the pod spec, labels and the path prefix of a
// workload object.
func podTemplate(o *object) (spec map[string]any, labels map[string]any, prefix string) {
	switch o.id.Kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job":
		if m := asMap(get(o.body, "spec", "template")); m != nil {
			return asMap(m["spec"]), asMap(get(m, "metadata", "labels")), "spec.template.spec"
		}
	case "CronJob":
		if m := asMap(get(o.body, "spec", "jobTemplate", "spec", "template")); m != nil {
			return asMap(m["spec"]), asMap(get(m, "metadata", "labels")), "spec.jobTemplate.spec.template.spec"
		}
	case "Pod":
		return asMap(o.body["spec"]), asMap(get(o.body, "metadata", "labels")), "spec"
	}
	return nil, nil, ""
}

func (g *gb) referenceEdges() {
	for _, n := range g.sortedNodes() {
		o := n.obj
		if o == nil {
			continue
		}
		g.relationshipRefs(n)
		switch {
		case n.n.Kind == domain.NodeWorkload:
			g.podRefs(n)
		case o.id.Kind == "Ingress" && o.id.Group == "networking.k8s.io":
			g.ingressRefs(n)
		case n.n.Kind == domain.NodeCustomResource:
			g.refFields(n)
		}
	}
}

func (g *gb) podRefs(n *gnode) {
	spec, _, prefix := podTemplate(n.obj)
	if spec == nil {
		return
	}
	ns := n.effNS
	// ref links the name at keys of v; sel is the selector holding the name
	// (its optional: true makes the reference optional).
	ref := func(kind string, v any, keys []string, rule, detail string) string {
		name := str(v, keys...)
		if name == "" {
			return ""
		}
		opt, _ := get(v, append(keys[:len(keys)-1:len(keys)-1], "optional")...).(bool)
		return g.linkRef(n, "", kind, name, ns, ruleRefs+rule, prefix+"."+detail, opt)
	}
	// mounted records, in consumer mode, how a volume's edge is mounted.
	mounted := func(id string, v any, src int, items []any) {
		if !g.consumers || id == "" {
			return
		}
		if e, ok := g.edges[id]; ok && e.Mount == nil {
			e.Mount = volumeMount(spec, str(v, "name"), src, items)
			g.edges[id] = e
		}
	}
	for i, v := range asSeq(spec["volumes"]) {
		vn := str(v, "name")
		at := func(s string) string { return fmt.Sprintf("volumes[%d:%s].%s", i, vn, s) }
		mounted(ref("Secret", v, []string{"secret", "secretName"}, "volumes.secret", at("secret.secretName")), v, -1, asSeq(get(v, "secret", "items")))
		mounted(ref("ConfigMap", v, []string{"configMap", "name"}, "volumes.configMap", at("configMap.name")), v, -1, asSeq(get(v, "configMap", "items")))
		for j, src := range asSeq(get(v, "projected", "sources")) {
			mounted(ref("Secret", src, []string{"secret", "name"}, "volumes.projected", at(fmt.Sprintf("projected.sources[%d].secret.name", j))), v, j, asSeq(get(src, "secret", "items")))
			mounted(ref("ConfigMap", src, []string{"configMap", "name"}, "volumes.projected", at(fmt.Sprintf("projected.sources[%d].configMap.name", j))), v, j, asSeq(get(src, "configMap", "items")))
		}
	}
	for _, field := range []string{"initContainers", "containers", "ephemeralContainers"} {
		for i, c := range asSeq(spec[field]) {
			cn := str(c, "name")
			for j, e := range asSeq(asMap(c)["env"]) {
				at := fmt.Sprintf("%s[%d:%s].env[%d:%s].valueFrom", field, i, cn, j, str(e, "name"))
				ref("Secret", e, []string{"valueFrom", "secretKeyRef", "name"}, "env.secretKeyRef", at+".secretKeyRef.name")
				ref("ConfigMap", e, []string{"valueFrom", "configMapKeyRef", "name"}, "env.configMapKeyRef", at+".configMapKeyRef.name")
			}
			for j, e := range asSeq(asMap(c)["envFrom"]) {
				at := fmt.Sprintf("%s[%d:%s].envFrom[%d]", field, i, cn, j)
				ref("Secret", e, []string{"secretRef", "name"}, "envFrom.secretRef", at+".secretRef.name")
				ref("ConfigMap", e, []string{"configMapRef", "name"}, "envFrom.configMapRef", at+".configMapRef.name")
			}
		}
	}
	sa := str(spec, "serviceAccountName")
	if sa == "" {
		sa = str(spec, "serviceAccount")
	}
	if sa != "" {
		g.link(n, "", "ServiceAccount", sa, ns, ruleRefs+"serviceAccountName", prefix+".serviceAccountName")
	}
	// The kubelet pulls without a missing pull secret (and warns): optional.
	for i, s := range asSeq(spec["imagePullSecrets"]) {
		if name := str(s, "name"); name != "" {
			g.linkRef(n, "", "Secret", name, ns, ruleRefs+"imagePullSecrets", prefix+"."+fmt.Sprintf("imagePullSecrets[%d].name", i), true)
		}
	}
}

func (g *gb) ingressRefs(n *gnode) {
	o, ns := n.obj, n.effNS
	spec := asMap(o.body["spec"])
	ref := func(group, kind, name, rule, detail string) {
		if name != "" {
			g.link(n, group, kind, name, ns, ruleRefs+rule, detail)
		}
	}
	ref("networking.k8s.io", "IngressClass", str(spec, "ingressClassName"), "ingress.class", "spec.ingressClassName")
	ref("networking.k8s.io", "IngressClass", str(o.body, "metadata", "annotations", "kubernetes.io/ingress.class"), "ingress.class", "metadata.annotations[kubernetes.io/ingress.class]")
	ref("", "Service", str(spec, "defaultBackend", "service", "name"), "ingress.backend", "spec.defaultBackend.service.name")
	for i, r := range asSeq(spec["rules"]) {
		for j, p := range asSeq(get(r, "http", "paths")) {
			ref("", "Service", str(p, "backend", "service", "name"), "ingress.backend", fmt.Sprintf("spec.rules[%d].http.paths[%d].backend.service.name", i, j))
		}
	}
	for i, t := range asSeq(spec["tls"]) {
		ref("", "Secret", str(t, "secretName"), "ingress.tls", fmt.Sprintf("spec.tls[%d].secretName", i))
	}
}

func groupOf(m map[string]any) string {
	if g := str(m, "group"); g != "" {
		return g
	}
	if str(m, "kind") == "" { // implied core kind (secretRef, configMapRef, ...)
		return ""
	}
	return anyGroup
}

var impliedRefKinds = map[string]string{"secretRef": "Secret", "configMapRef": "ConfigMap", "serviceAccountRef": "ServiceAccount"}

// refFields follows the generic `*Ref` convention of custom resources: an
// object {name, kind[, group, namespace]} under a field named "...Ref".
func (g *gb) refFields(n *gnode) {
	var walk func(v any, p string)
	walk = func(v any, p string) {
		switch t := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				child := p + "." + k
				if m := asMap(t[k]); m != nil && strings.HasSuffix(k, "Ref") {
					name, kind := str(m, "name"), str(m, "kind")
					if kind == "" {
						kind = impliedRefKinds[k]
					}
					if name != "" && kind != "" && !g.covered(n.obj, child+".name") {
						ns := str(m, "namespace")
						if ns == "" {
							ns = n.effNS
						}
						g.link(n, groupOf(m), kind, name, ns, ruleRefs+"ref-field", strings.TrimPrefix(child, "."))
						continue
					}
				}
				walk(t[k], child)
			}
		case []any:
			for i, e := range t {
				walk(e, fmt.Sprintf("%s[%d]", p, i))
			}
		}
	}
	walk(n.obj.body["spec"], "spec")
}

// covered reports whether a declared references relationship already
// follows the name field at p (".spec.x.name" form) of the object.
func (g *gb) covered(o *object, p string) bool {
	p = strings.TrimPrefix(p, ".")
	for _, r := range g.rels {
		if r.r.Kind != catalog.RelReferences || !g.matches(r, o) {
			continue
		}
		if r.r.Path == p {
			return true
		}
		for _, ref := range readNames(o, r.r, "") {
			if ref.field == p {
				return true
			}
		}
	}
	return false
}

// --- selectors --------------------------------------------------------------------------------

func (g *gb) selectorEdges() {
	nodes := g.sortedNodes()
	for _, svc := range nodes {
		if svc.obj == nil || svc.n.Kind != domain.NodeService {
			continue
		}
		sel := asMap(get(svc.obj.body, "spec", "selector"))
		if len(sel) == 0 {
			continue
		}
		for _, w := range nodes {
			if w.obj == nil || w.n.Kind != domain.NodeWorkload {
				continue
			}
			_, labels, _ := podTemplate(w.obj)
			match := true
			for k, v := range sel {
				if lv, ok := labels[k]; !ok || fmt.Sprint(lv) != fmt.Sprint(v) {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			switch {
			case w.effNS == svc.effNS:
				g.edge(domain.EdgeSelects, w, svc, domain.EdgeEstablished, ruleSelectors+"service", "spec.selector", "", svc.obj.artifact, w.obj.artifact)
			case w.effNS == "" || svc.effNS == "":
				g.edge(domain.EdgeSelects, w, svc, domain.EdgeInferred, ruleSelectors+"service", "spec.selector",
					"labels match but the namespace of the workload or the Service is not known", svc.obj.artifact, w.obj.artifact)
			}
		}
	}
}

// --- API types --------------------------------------------------------------------------------

func apiKey(group, version, kind string) string { return group + "/" + version + "/" + kind }

func (g *gb) apiEdges() {
	nodes := g.sortedNodes()
	apiNodes := map[string]*gnode{}
	apiNode := func(group, version, kind string) *gnode {
		k := apiKey(group, version, kind)
		if n := apiNodes[k]; n != nil {
			return n
		}
		id := render.ObjectID{Group: group, Version: version, Kind: kind}
		n := g.node(domain.NodeAPIType, id, k, "")
		apiNodes[k] = n
		return n
	}
	for _, n := range nodes {
		o := n.obj
		if o == nil || o.id.Kind != "CustomResourceDefinition" || o.id.Group != "apiextensions.k8s.io" {
			continue
		}
		group, kind := str(o.body, "spec", "group"), str(o.body, "spec", "names", "kind")
		for _, v := range asSeq(get(o.body, "spec", "versions")) {
			if ver := str(v, "name"); ver != "" && kind != "" {
				a := apiNode(group, ver, kind)
				a.arts[o.artifact] = true
				g.edge(domain.EdgeServesAPI, n, a, domain.EdgeEstablished, ruleCRD+"serves", "spec.versions["+ver+"]", "", o.artifact)
			}
		}
	}
	for _, n := range nodes {
		o := n.obj
		if o == nil || n.n.Kind != domain.NodeCustomResource {
			continue
		}
		a := apiNode(o.id.Group, o.id.Version, o.id.Kind)
		if len(a.arts) == 0 && !a.n.Placeholder {
			a.n.Placeholder = true // no CRD in the snapshot defines the type
		}
		g.edge(domain.EdgeConsumesAPI, a, n, domain.EdgeEstablished, ruleCRD+"consumes", "apiVersion/kind", "", o.artifact)
	}
}

// --- producers --------------------------------------------------------------------------------

// readNames reads the names (and namespaces, for namespace/name) a
// relationship field holds in an object, with the concrete field path of
// each and whether the reference is optional (a Kubernetes key selector's
// `optional: true` next to the name).
func readNames(o *object, r catalog.Relationship, defaultNS string) []namedRef {
	keys, err := catalog.SplitListFieldPath(r.Path)
	if err != nil {
		return nil
	}
	var out []namedRef
	for _, fv := range fieldValues(o.body, keys, r.Path) {
		v, _ := fv.v.(string)
		if v == "" {
			continue
		}
		ref := namedRef{field: fv.path, ns: defaultNS, name: v}
		if opt, _ := fv.parent["optional"].(bool); opt {
			ref.optional = true
		}
		switch {
		case r.Format == catalog.RelFormatNamespaceName,
			r.Format == catalog.RelFormatOptionalNamespace && strings.Contains(v, "/"):
			i := strings.Index(v, "/")
			if i <= 0 || i == len(v)-1 || strings.Count(v, "/") != 1 {
				continue
			}
			ref.ns, ref.name = v[:i], v[i+1:]
		}
		out = append(out, ref)
	}
	return out
}

type namedRef struct {
	field, ns, name string
	optional        bool
}

func (g *gb) matches(r rel, o *object) bool {
	if r.r.From.Group != o.id.Group || r.r.From.Kind != o.id.Kind {
		return false
	}
	if r.r.WhenPath != "" {
		p, err := catalog.SplitFieldPath(r.r.WhenPath)
		if err != nil {
			return false
		}
		if v, _ := get(o.body, p...).(string); v != r.r.WhenEquals {
			return false
		}
	}
	return true
}

// indexProducers records which objects name objects they create: declared
// produces relationships first, then the generic `spec.secretName`
// convention for custom resources no declared relationship covers.
func (g *gb) indexProducers(rels []rel) {
	for _, n := range g.sortedNodes() {
		o := n.obj
		if o == nil {
			continue
		}
		declared := false
		for _, r := range rels {
			if r.r.Kind != catalog.RelProduces || !g.matches(r, o) {
				continue
			}
			for _, ref := range readNames(o, r.r, n.effNS) {
				declared = true
				g.addProducer(r.r.To.Kind, ref.ns, ref.name, producer{node: n, rule: r.id, declared: true, detail: ref.field})
			}
		}
		if !declared && n.n.Kind == domain.NodeCustomResource {
			if name := str(o.body, "spec", "secretName"); name != "" {
				g.addProducer("Secret", n.effNS, name, producer{node: n, rule: ruleProduces + "spec.secretName", detail: "spec.secretName"})
			}
		}
	}
}

// relationshipRefs applies declared references relationships to one object.
// A name a cluster-scoped object references through a relationship with a
// namespaceFrom resolution is looked up in the namespace the environment's
// controller configuration gives; when that is not decided, in no namespace
// (matched by name only: never established).
func (g *gb) relationshipRefs(n *gnode) {
	for _, r := range g.rels {
		if r.r.Kind != catalog.RelReferences || !g.matches(r, n.obj) {
			continue
		}
		var via nsResolution
		if r.r.NamespaceFrom != nil {
			via = g.clusterRefNamespace(r)
		}
		for _, ref := range readNames(n.obj, r.r, n.effNS) {
			if r.r.NamespaceFrom != nil {
				ref.ns = via.ns
			}
			g.linkVia(n, r.r.To.Group, r.r.To.Kind, ref.name, ref.ns, r.id, ref.field, ref.optional, via)
		}
	}
}

// nsResolution is where a declared namespaceFrom resolution says the
// controller resolves a cluster-scoped object's name references: ns ("" when
// not decided), a note for the edge reason, and the artifacts of the
// controller configuration it was read from.
type nsResolution struct {
	ns   string
	note string
	arts []string
}

// clusterRefNamespace decides a relationship's namespaceFrom from the
// environment at the graph's side: every container named by the rule in the
// pod templates of the environment's workloads, the rule's flag in its
// command and args (Kubernetes $(VAR) references expanded from the
// container's env: literal values, or the pod's own namespace through
// fieldRef metadata.namespace), else the documented default when no
// configuration file can set it. Fail closed: no citation, no such
// container, a value set through anything the snapshot cannot read
// (configMapKeyRef, secretKeyRef, envFrom, a configuration file), a flag
// without a value, or containers that disagree leave it undecided.
func (g *gb) clusterRefNamespace(r rel) nsResolution {
	if res, ok := g.nsCache[r.id]; ok {
		return res
	}
	src := r.r.NamespaceFrom
	where := fmt.Sprintf("%s of container %q", src.Arg, src.Container)
	undecided := func(why string) nsResolution {
		return nsResolution{note: fmt.Sprintf("the namespace the controller resolves it in (%s, rule %s) is not decided: %s", where, r.id, why)}
	}
	res := func() nsResolution {
		if len(src.References) == 0 {
			return undecided("the rule cites no upstream documentation for it")
		}
		if src.Container == "" || src.Arg == "" {
			return undecided("the rule names no container or argument")
		}
		var found []string
		var arts []string
		seen := map[string]bool{}
		for _, n := range g.sortedNodes() {
			o := n.obj
			if o == nil || g.objNode[o] != n || n.n.Kind != domain.NodeWorkload {
				continue
			}
			spec, _, prefix := podTemplate(o)
			for _, field := range []string{"initContainers", "containers"} {
				for _, c := range asSeq(spec[field]) {
					if str(c, "name") != src.Container {
						continue
					}
					at := fmt.Sprintf("%s %s.%s[%s]", o.key(), prefix, field, src.Container)
					ns, why := containerNamespace(asMap(c), n.effNS, src)
					if ns == "" {
						return undecided(at + ": " + why)
					}
					if !seen[ns] {
						seen[ns] = true
						found = append(found, fmt.Sprintf("%s (%s: %s)", ns, at, why))
					}
					if o.artifact != "" {
						arts = append(arts, o.artifact)
					}
				}
			}
		}
		switch len(seen) {
		case 0:
			return undecided(fmt.Sprintf("no workload of the environment runs a container named %q", src.Container))
		case 1:
			var ns string
			for k := range seen {
				ns = k
			}
			return nsResolution{ns: ns, arts: uniqStrings(arts), note: "namespace " + found[0] + " from " + where + " (rule " + r.id + ")"}
		}
		sort.Strings(found)
		return undecided("the controller containers disagree: " + strings.Join(found, ", "))
	}()
	g.nsCache[r.id] = res
	return res
}

// containerNamespace decides the namespace one controller container gives
// through src: the flag's value, else the documented default. why says how
// (or why not).
func containerNamespace(c map[string]any, podNS string, src *catalog.NamespaceSource) (ns, why string) {
	vals, bare, cfg := flagValues(c, src.Arg), false, false
	for _, v := range vals {
		bare = bare || v == nil
	}
	if src.ConfigFileArg != "" {
		cfg = len(flagValues(c, src.ConfigFileArg)) > 0
	}
	switch {
	case vals == nil && cfg:
		return "", fmt.Sprintf("%s is not given and %s names a configuration file that can set it", src.Arg, src.ConfigFileArg)
	case vals == nil && src.Default == "":
		return "", fmt.Sprintf("%s is not given and the rule documents no default", src.Arg)
	case vals == nil:
		return src.Default, fmt.Sprintf("%s is not given; documented default %s", src.Arg, src.Default)
	case bare:
		return "", fmt.Sprintf("%s is given without a value", src.Arg)
	}
	out := map[string]bool{}
	for _, v := range vals {
		x, why := expandEnv(*v, c, podNS)
		if x == "" {
			return "", why
		}
		if !namespaceName(x) {
			return "", fmt.Sprintf("%s=%s is not a namespace name", src.Arg, x)
		}
		out[x] = true
	}
	if len(out) != 1 {
		return "", fmt.Sprintf("%s is given more than once with different values", src.Arg)
	}
	for x := range out {
		ns = x
	}
	if len(vals) == 1 && *vals[0] == ns {
		return ns, fmt.Sprintf("%s=%s", src.Arg, ns)
	}
	return ns, fmt.Sprintf("%s=%s, expanded from the container's env", src.Arg, *vals[0])
}

// flagValues returns every value the flag takes in the container's command
// and args ("--flag=v" or "--flag v"; nil for a flag without a value; nil
// slice when absent). Tokens after "--" are positional.
func flagValues(c map[string]any, flag string) []*string {
	want := strings.TrimLeft(flag, "-")
	var toks []string
	for _, field := range []string{"command", "args"} {
		for _, x := range asSeq(c[field]) {
			s, _ := x.(string)
			toks = append(toks, s)
		}
	}
	var out []*string
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t == "--" {
			break
		}
		if !strings.HasPrefix(t, "-") {
			continue
		}
		k, v, eq := strings.Cut(t, "=")
		if strings.TrimLeft(k, "-") != want {
			continue
		}
		switch {
		case eq:
			out = append(out, &v)
		case i+1 < len(toks) && !strings.HasPrefix(toks[i+1], "-"):
			i++
			v := toks[i]
			out = append(out, &v)
		default:
			out = append(out, nil)
		}
	}
	return out
}

// expandEnv expands Kubernetes $(VAR) references in a container argument
// from the container's env: a literal value, or the pod's namespace through
// fieldRef metadata.namespace. Anything else is undecided ("" with why).
func expandEnv(v string, c map[string]any, podNS string) (string, string) {
	if strings.Contains(v, "$$") {
		return "", fmt.Sprintf("%q escapes a $(VAR) reference", v)
	}
	var b strings.Builder
	for {
		i := strings.Index(v, "$(")
		if i < 0 {
			b.WriteString(v)
			break
		}
		j := strings.IndexByte(v[i:], ')')
		if j < 0 {
			return "", fmt.Sprintf("%q has an unterminated $(VAR) reference", v)
		}
		name := v[i+2 : i+j]
		val, why := envValue(c, name, podNS)
		if why != "" {
			return "", why
		}
		b.WriteString(v[:i])
		b.WriteString(val)
		v = v[i+j+1:]
	}
	return b.String(), ""
}

// envValue is the value of the container's env var name, when the snapshot
// decides it.
func envValue(c map[string]any, name, podNS string) (string, string) {
	var hit map[string]any
	n := 0
	for _, e := range asSeq(c["env"]) {
		if m := asMap(e); str(m, "name") == name {
			hit, n = m, n+1
		}
	}
	switch {
	case n > 1:
		return "", fmt.Sprintf("env var %s is defined more than once", name)
	case hit == nil && len(asSeq(c["envFrom"])) > 0:
		return "", fmt.Sprintf("env var %s is not defined in env; envFrom may supply it, which the snapshot does not decide", name)
	case hit == nil:
		return "", fmt.Sprintf("env var %s is not defined: $(%s) stays unexpanded", name, name)
	}
	if v, ok := hit["value"].(string); ok && !strings.Contains(v, "$(") {
		return v, ""
	}
	if fp := str(hit, "valueFrom", "fieldRef", "fieldPath"); fp == "metadata.namespace" {
		if podNS == "" {
			return "", fmt.Sprintf("env var %s is the pod's namespace, which is not known", name)
		}
		return podNS, ""
	}
	return "", fmt.Sprintf("env var %s is set through something the snapshot does not decide (%s)", name, envSource(hit))
}

func envSource(m map[string]any) string {
	if vf := asMap(m["valueFrom"]); vf != nil {
		var ks []string
		for k := range vf {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		return "valueFrom." + strings.Join(ks, ",")
	}
	return "a value with a $(VAR) reference"
}

func namespaceName(s string) bool {
	if len(s) == 0 || len(s) > 63 {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-' && i > 0 && i < len(s)-1:
		default:
			return false
		}
	}
	return true
}

func uniqStrings(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

// relationshipSelectEdges applies declared selects relationships: a selected
// Service or Pod-template workload → the selecting object.
func (g *gb) relationshipSelectEdges() {
	nodes := g.sortedNodes()
	for _, r := range g.rels {
		if r.r.Kind != catalog.RelSelects {
			continue
		}
		selPath, err := catalog.SplitFieldPath(r.r.SelectorPath)
		if err != nil {
			continue
		}
		var nsPath []string
		if r.r.NamespaceSelectorPath != "" {
			if nsPath, err = catalog.SplitFieldPath(r.r.NamespaceSelectorPath); err != nil {
				continue
			}
		}
		for _, src := range nodes {
			if src.obj == nil || !g.matches(r, src.obj) {
				continue
			}
			sel := asMap(get(src.obj.body, selPath...))
			if len(asMap(sel["matchLabels"])) == 0 && len(asSeq(sel["matchExpressions"])) == 0 {
				continue // an empty selector is not treated as "everything": no edges, no noise
			}
			anyNS, names := false, []string(nil)
			if nsPath != nil {
				nsSel := asMap(get(src.obj.body, nsPath...))
				anyNS, _ = nsSel["any"].(bool)
				for _, x := range asSeq(nsSel["matchNames"]) {
					if s, ok := x.(string); ok {
						names = append(names, s)
					}
				}
			}
			for _, tgt := range nodes {
				if tgt.obj == nil {
					continue
				}
				var labels map[string]any
				switch {
				case r.r.To.Kind == "Service" && tgt.n.Kind == domain.NodeService:
					labels = asMap(get(tgt.obj.body, "metadata", "labels"))
				case r.r.To.Kind == "Pod" && tgt.n.Kind == domain.NodeWorkload:
					_, labels, _ = podTemplate(tgt.obj)
				default:
					continue
				}
				if !labelSelectorMatches(sel, labels) {
					continue
				}
				status, reason := domain.EdgeEstablished, ""
				switch {
				case anyNS:
				case len(names) > 0 && !containsString(names, tgt.effNS):
					if tgt.effNS != "" {
						continue
					}
					status, reason = domain.EdgeInferred, "namespace of the selected object is not known"
				case len(names) == 0 && tgt.effNS != src.effNS:
					if tgt.effNS != "" && src.effNS != "" {
						continue
					}
					status, reason = domain.EdgeInferred, "namespace of the selecting or selected object is not known"
				}
				g.edge(domain.EdgeSelects, tgt, src, status, r.id, r.r.SelectorPath, reason, src.obj.artifact, tgt.obj.artifact)
			}
		}
	}
}

// labelSelectorMatches evaluates a Kubernetes LabelSelector (matchLabels and
// matchExpressions In/NotIn/Exists/DoesNotExist) against a label set. An
// operator it does not know never matches.
func labelSelectorMatches(sel, labels map[string]any) bool {
	for k, v := range asMap(sel["matchLabels"]) {
		if lv, has := labels[k]; !has || fmt.Sprint(lv) != fmt.Sprint(v) {
			return false
		}
	}
	for _, e := range asSeq(sel["matchExpressions"]) {
		key, op := str(e, "key"), str(e, "operator")
		lv, has := labels[key]
		var vals []string
		for _, x := range asSeq(asMap(e)["values"]) {
			vals = append(vals, fmt.Sprint(x))
		}
		switch op {
		case "In":
			if !has || !containsString(vals, fmt.Sprint(lv)) {
				return false
			}
		case "NotIn":
			if has && containsString(vals, fmt.Sprint(lv)) {
				return false
			}
		case "Exists":
			if !has {
				return false
			}
		case "DoesNotExist":
			if has {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func containsString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func (g *gb) addProducer(kind, ns, name string, p producer) {
	k := kind + "|" + ns + "|" + name
	g.producers[k] = append(g.producers[k], p)
}

// --- finish -----------------------------------------------------------------------------------

func (g *gb) finish() *domain.RelationshipGraph {
	out := &domain.RelationshipGraph{Bundle: g.bundle.ID, Builder: BuilderName, Nodes: []domain.GraphNode{}, Edges: []domain.GraphEdge{}}
	if g.consumers {
		out.Builder = consumerBuilderName(g.rules)
	}
	for _, n := range g.sortedNodes() {
		n.n.Artifacts = sortedKeys(n.arts)
		out.Nodes = append(out.Nodes, n.n)
	}
	for _, e := range g.edges {
		out.Edges = append(out.Edges, e)
	}
	sort.Slice(out.Edges, func(i, j int) bool { return out.Edges[i].ID < out.Edges[j].ID })
	out.ID = out.DerivedID()
	return out
}
