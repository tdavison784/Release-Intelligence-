package relations

import (
	"fmt"
	"path"
	"sort"
	"strings"

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

var placeholderVersion = map[string]string{"IngressClass": "v1"}

// resolve finds the object a reference names. It returns the node and how
// well the reference is established; an absent object becomes a placeholder
// node, with a producing custom resource when one is known.
func (g *gb) resolve(group string, groupStated bool, kind, name, ns string) (*gnode, domain.EdgeStatus, string) {
	cluster := clusterScoped[kind]
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
	// Absent: placeholder.
	version := "v1"
	if !groupStated || group != "" {
		version = "*"
	}
	if v, ok := placeholderVersion[kind]; ok {
		version = v
	}
	id := render.ObjectID{Group: group, Version: version, Kind: kind, Namespace: ns, Name: name}
	n := g.node(classify(id), id, id.String(), "")
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
	return n, n.phStatus, n.phReason
}

func producesDetail(p producer) string { return p.detail }

// link adds a references edge target → referrer.
func (g *gb) link(referrer *gnode, group, kind, name, ns, rule, detail string) {
	groupStated := true
	target, status, reason := g.resolve(group, groupStated, kind, name, ns)
	if target == nil {
		return
	}
	art := ""
	if referrer.obj != nil {
		art = referrer.obj.artifact
	}
	g.edge(domain.EdgeReferences, target, referrer, status, rule, detail, reason, art)
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
	ref := func(kind, name, rule, detail string) {
		if name != "" {
			g.link(n, "", kind, name, ns, ruleRefs+rule, prefix+"."+detail)
		}
	}
	for i, v := range asSeq(spec["volumes"]) {
		vn := str(v, "name")
		at := func(s string) string { return fmt.Sprintf("volumes[%d:%s].%s", i, vn, s) }
		ref("Secret", str(v, "secret", "secretName"), "volumes.secret", at("secret.secretName"))
		ref("ConfigMap", str(v, "configMap", "name"), "volumes.configMap", at("configMap.name"))
		for j, src := range asSeq(get(v, "projected", "sources")) {
			ref("Secret", str(src, "secret", "name"), "volumes.projected", at(fmt.Sprintf("projected.sources[%d].secret.name", j)))
			ref("ConfigMap", str(src, "configMap", "name"), "volumes.projected", at(fmt.Sprintf("projected.sources[%d].configMap.name", j)))
		}
	}
	for _, field := range []string{"initContainers", "containers", "ephemeralContainers"} {
		for i, c := range asSeq(spec[field]) {
			cn := str(c, "name")
			for j, e := range asSeq(asMap(c)["env"]) {
				at := fmt.Sprintf("%s[%d:%s].env[%d:%s].valueFrom", field, i, cn, j, str(e, "name"))
				ref("Secret", str(e, "valueFrom", "secretKeyRef", "name"), "env.secretKeyRef", at+".secretKeyRef.name")
				ref("ConfigMap", str(e, "valueFrom", "configMapKeyRef", "name"), "env.configMapKeyRef", at+".configMapKeyRef.name")
			}
			for j, e := range asSeq(asMap(c)["envFrom"]) {
				at := fmt.Sprintf("%s[%d:%s].envFrom[%d]", field, i, cn, j)
				ref("Secret", str(e, "secretRef", "name"), "envFrom.secretRef", at+".secretRef.name")
				ref("ConfigMap", str(e, "configMapRef", "name"), "envFrom.configMapRef", at+".configMapRef.name")
			}
		}
	}
	sa := str(spec, "serviceAccountName")
	if sa == "" {
		sa = str(spec, "serviceAccount")
	}
	ref("ServiceAccount", sa, "serviceAccountName", "serviceAccountName")
	for i, s := range asSeq(spec["imagePullSecrets"]) {
		ref("Secret", str(s, "name"), "imagePullSecrets", fmt.Sprintf("imagePullSecrets[%d].name", i))
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
					if name != "" && kind != "" {
						ns := str(m, "namespace")
						if ns == "" {
							ns = n.effNS
						}
						g.link(n, str(m, "group"), kind, name, ns, ruleRefs+"ref-field", strings.TrimPrefix(child, "."))
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

// indexProducers records which custom resources name objects they create:
// declared rules first, then the generic `spec.secretName` convention.
func (g *gb) indexProducers(rules []ProducesRule) {
	for _, n := range g.sortedNodes() {
		o := n.obj
		if o == nil || n.n.Kind != domain.NodeCustomResource {
			continue
		}
		declared := false
		for _, r := range rules {
			if r.Group != o.id.Group || r.Kind != o.id.Kind {
				continue
			}
			name, _ := get(o.body, strings.Split(r.NamePath, ".")...).(string)
			if name == "" {
				continue
			}
			declared = true
			g.addProducer(r.ProducedKind, n.effNS, name, producer{node: n, rule: "relationship:" + r.Name, declared: true, detail: r.NamePath})
		}
		if !declared {
			if name := str(o.body, "spec", "secretName"); name != "" {
				g.addProducer("Secret", n.effNS, name, producer{node: n, rule: ruleProduces + "spec.secretName", detail: "spec.secretName"})
			}
		}
	}
}

func (g *gb) addProducer(kind, ns, name string, p producer) {
	k := kind + "|" + ns + "|" + name
	g.producers[k] = append(g.producers[k], p)
}

// --- finish -----------------------------------------------------------------------------------

func (g *gb) finish() *domain.RelationshipGraph {
	out := &domain.RelationshipGraph{Bundle: g.bundle.ID, Builder: BuilderName, Nodes: []domain.GraphNode{}, Edges: []domain.GraphEdge{}}
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
