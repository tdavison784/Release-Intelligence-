package relations

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// BuilderName is the producer string of graphs built here.
const BuilderName = "relations.builder@v1"

// ConsumerBuilderName is the producer string of graphs built in consumer
// mode (WithConsumers) under the default consumer rules: the same graph plus
// mount detail, lines and reads-file edges. A graph built under other rules
// names them ("+consumers@v<rules>").
const ConsumerBuilderName = BuilderName + "+consumers@v3"

// Consumer rules (WithConsumerRules). v1 is the Milestone 1 consumer graph
// (ledger D-0001). v2 (D-0005) also reads the data of a kustomize-generated
// ConfigMap whose generator sources the snapshot lacks from its rendered
// object, knows the keys of generated Secrets, and records configuration
// data and keys it cannot know as unresolved edges instead of skipping them.
// v3 (D-0006) also matches the paths a container's own spec names (command,
// args, literal env values) against that container's mounts: reads-file
// edges from the provider to the workload. A transcript recorded under v1
// replays only against a v1 graph, so the rules a session used are part of
// its transcript header.
const (
	ConsumerRulesV1 = 1
	ConsumerRulesV2 = 2
	ConsumerRulesV3 = 3
	// ConsumerRules is the default.
	ConsumerRules = ConsumerRulesV3
)

func consumerBuilderName(rules int) string {
	return fmt.Sprintf("%s+consumers@v%d", BuilderName, rules)
}

// RulesOf is the consumer rules a graph's builder name states (0: not a
// consumer-mode graph).
func RulesOf(builder string) int {
	var n int
	if _, err := fmt.Sscanf(strings.TrimPrefix(builder, BuilderName), "+consumers@v%d", &n); err != nil || !strings.HasPrefix(builder, BuilderName+"+consumers@v") {
		return 0
	}
	return n
}

// ProducesRule is a declarative relationship: a custom resource of
// Group/Kind names, at NamePath (dotted, e.g. "spec.secretName"), an object
// of ProducedGroup/ProducedKind that its controller creates. A match yields
// an established `produces` edge. Rules are data (to come from the product
// catalog's relationships construct), never product code; with no rule a
// generic convention still proposes the edge, as inferred.
type ProducesRule struct {
	Name          string `json:"name"` // edge rule id: "relationship:<name>"
	Group         string `json:"group"`
	Kind          string `json:"kind"`
	NamePath      string `json:"namePath"`
	ProducedGroup string `json:"producedGroup,omitempty"`
	ProducedKind  string `json:"producedKind"`
}

// rel is a declarative relationship with the rule id its edges carry.
type rel struct {
	id string
	r  catalog.Relationship
}

func (b *builder) add(id string, r catalog.Relationship) { b.rels = append(b.rels, rel{id, r}) }

// Option configures the builder.
type Option func(*builder)

// WithProducesRules adds declarative produces rules.
func WithProducesRules(rules ...ProducesRule) Option {
	return func(b *builder) {
		for _, r := range rules {
			b.add("relationship:"+r.Name, catalog.Relationship{
				ID: r.Name, Kind: catalog.RelProduces,
				From: catalog.ObjectType{Group: r.Group, Kind: r.Kind},
				To:   catalog.ObjectType{Group: r.ProducedGroup, Kind: r.ProducedKind}, Path: r.NamePath,
			})
		}
	}
}

// WithRelationships adds the relationships a product definition declares
// (catalog `relationships:`); their edges carry the rule id
// "relationship:<product>/<id>". Invalid relationships (catalog.Validate reports them)
// are skipped.
func WithRelationships(product string, rs ...catalog.Relationship) Option {
	return func(b *builder) {
		for _, r := range rs {
			b.add("relationship:"+product+"/"+r.ID, r)
		}
	}
}

// WithCatalog adds the relationships of every product in the catalog, in
// product id order. Rules of products the environment does not use match no
// object and cost nothing.
func WithCatalog(c *catalog.Catalog) Option {
	return func(b *builder) {
		if c == nil {
			return
		}
		defs := c.List()
		sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })
		for _, d := range defs {
			WithRelationships(d.ID, d.Relationships...)(b)
		}
	}
}

type builder struct {
	st   investigate.ArtifactStore
	rels []rel
	// consumers: consumer mode (WithConsumers), under consumerRules (0: the
	// default).
	consumers     bool
	consumerRules int
}

// WithConsumers switches on consumer mode (docs/phase6/decision-intel/PLAN-M1.md
// changes 1 and 2): references edges of pod volumes carry their mounts
// (GraphEdge.Mount), references and produces edges the line that states them
// (GraphEdge.Line), and reads-file edges link a mounted Secret/ConfigMap to the
// ConfigMap whose configuration names a path under its mount (consumers.go).
// Graphs built without it are unchanged, so the pipeline's graph (a policy
// input) never depends on it.
func WithConsumers() Option { return func(b *builder) { b.consumers = true } }

// WithConsumerRules selects the consumer rules of a consumer-mode build
// (ConsumerRulesV1 … ConsumerRulesV3; 0 or unknown: the default).
func WithConsumerRules(rules int) Option {
	return func(b *builder) { b.consumerRules = rules }
}

// BuildConsumerGraph builds the consumer-mode graph of a bundle at one side
// of the upgrade (the graph a TO-side build gives, plus mounts, lines and
// reads-file edges). opts are the graph builder's (WithCatalog, …).
func BuildConsumerGraph(ctx context.Context, st investigate.ArtifactStore, b *domain.EvidenceBundle, side Side, opts ...Option) (*domain.RelationshipGraph, error) {
	if b == nil {
		return nil, fmt.Errorf("relations: nil bundle: %w", investigate.ErrRejected)
	}
	if side != SideFrom && side != SideTo {
		return nil, fmt.Errorf("relations: side %q: %w", side, investigate.ErrRejected)
	}
	bd := &builder{st: st}
	for _, o := range opts {
		o(bd)
	}
	bd.consumers = true
	g, err := bd.build(ctx, b, side, nil)
	if err != nil {
		return nil, err
	}
	return g.finish(), nil
}

// NewBuilder returns the GraphBuilder. st serves the bundle's artifacts
// (rendered manifests and environment files).
func NewBuilder(st investigate.ArtifactStore, opts ...Option) investigate.GraphBuilder {
	b := &builder{st: st}
	for _, o := range opts {
		o(b)
	}
	return b
}

// --- classification ---------------------------------------------------------------------

var builtinGroups = map[string]bool{
	"": true, "apps": true, "batch": true, "policy": true, "autoscaling": true,
	"networking.k8s.io": true, "rbac.authorization.k8s.io": true, "storage.k8s.io": true,
	"apiextensions.k8s.io": true, "admissionregistration.k8s.io": true, "coordination.k8s.io": true,
	"scheduling.k8s.io": true, "discovery.k8s.io": true, "node.k8s.io": true, "certificates.k8s.io": true,
	"apiregistration.k8s.io": true, "events.k8s.io": true, "flowcontrol.apiserver.k8s.io": true,
}

var clusterScoped = map[string]bool{
	"Namespace": true, "Node": true, "PersistentVolume": true, "StorageClass": true, "IngressClass": true,
	"ClusterRole": true, "ClusterRoleBinding": true, "CustomResourceDefinition": true, "PriorityClass": true,
	"ValidatingWebhookConfiguration": true, "MutatingWebhookConfiguration": true, "APIService": true,
}

const (
	groupArgo      = "argoproj.io"
	groupFluxHelm  = "helm.toolkit.fluxcd.io"
	groupFluxKust  = "kustomize.toolkit.fluxcd.io"
	groupKustomize = "kustomize.config.k8s.io"
)

func classify(id render.ObjectID) domain.NodeKind {
	switch id.Group {
	case groupArgo:
		if id.Kind == "Application" || id.Kind == "ApplicationSet" {
			return domain.NodeGitOpsApplication
		}
	case groupFluxHelm:
		if id.Kind == "HelmRelease" {
			return domain.NodeGitOpsApplication
		}
	case groupFluxKust:
		if id.Kind == "Kustomization" {
			return domain.NodeGitOpsApplication
		}
	case groupKustomize:
		if id.Kind == "Kustomization" {
			return domain.NodeKustomization
		}
	case "gateway.networking.k8s.io":
		return domain.NodeIngress
	}
	switch id.Kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job", "CronJob", "Pod":
		if builtinGroups[id.Group] {
			return domain.NodeWorkload
		}
	case "Service":
		if id.Group == "" {
			return domain.NodeService
		}
	case "Ingress", "IngressClass":
		if id.Group == "networking.k8s.io" {
			return domain.NodeIngress
		}
	case "ConfigMap", "Secret":
		if id.Group == "" {
			return domain.NodeConfigObject
		}
	case "ServiceAccount":
		if id.Group == "" {
			return domain.NodeRBAC
		}
	case "Role", "ClusterRole", "RoleBinding", "ClusterRoleBinding":
		if id.Group == "rbac.authorization.k8s.io" {
			return domain.NodeRBAC
		}
	}
	if !builtinGroups[id.Group] {
		return domain.NodeCustomResource
	}
	return domain.NodeResource
}

// --- graph under construction ---------------------------------------------------------------

type gnode struct {
	n     domain.GraphNode
	id    render.ObjectID
	effNS string
	arts  map[string]bool
	obj   *object
	// phStatus/phReason: how well a placeholder's absence is explained.
	phStatus domain.EdgeStatus
	phReason string
}

type producer struct {
	node     *gnode
	rule     string
	declared bool
	detail   string
}

type gb struct {
	bundle    *domain.EvidenceBundle
	nodes     map[string]*gnode // by node id
	edges     map[string]domain.GraphEdge
	byName    map[string][]*gnode // kind|name → nodes (resolution index)
	byKey     map[string]*gnode
	producers map[string][]producer
	objNode   map[*object]*gnode
	env       *environment
	rels      []rel
	// view, when set, records every reference as it is resolved (View).
	view *refView
	// nsCache holds each namespaceFrom resolution by rule id (one per build).
	nsCache map[string]nsResolution
	// Consumer mode (WithConsumers): rules are the consumer rules; side the
	// side built; read reads an artifact by id; gen holds what the snapshot
	// says about kustomize-generated objects by node key; docs caches parsed
	// artifacts for line lookups; renders caches parsed render artifacts.
	consumers bool
	rules     int
	side      Side
	read      func(id string) ([]byte, error)
	gen       map[string]*genData
	docs      map[string]*parsedArtifact
	renders   map[string][]render.Object
}

func (g *gb) node(kind domain.NodeKind, id render.ObjectID, key string, artifact string) *gnode {
	nid := domain.NodeID(kind, key)
	n := g.nodes[nid]
	if n == nil {
		n = &gnode{n: domain.GraphNode{ID: nid, Kind: kind, Key: key}, id: id, effNS: id.Namespace, arts: map[string]bool{}}
		g.nodes[nid] = n
	}
	if artifact != "" {
		n.arts[artifact] = true
	}
	return n
}

func (g *gb) objectNode(o *object) *gnode {
	n := g.node(classify(o.id), o.id, o.key(), o.artifact)
	n.effNS = o.effNS
	if n.obj == nil {
		n.obj = o
	}
	g.objNode[o] = n
	return n
}

func (g *gb) edge(kind domain.EdgeKind, from, to *gnode, status domain.EdgeStatus, rule, detail, reason string, arts ...string) {
	if from == nil || to == nil || from.n.ID == to.n.ID {
		return
	}
	id := domain.EdgeID(kind, from.n.ID, to.n.ID, detail)
	if _, dup := g.edges[id]; dup {
		return
	}
	e := domain.GraphEdge{ID: id, Kind: kind, From: from.n.ID, To: to.n.ID, Status: status, Rule: rule, Detail: detail}
	if status != domain.EdgeEstablished {
		e.Reason = reason
	}
	set := map[string]bool{}
	for _, a := range arts {
		if a != "" {
			set[a] = true
		}
	}
	e.Artifacts = sortedKeys(set)
	g.edges[id] = e
}

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// --- build ------------------------------------------------------------------------------

func (bd *builder) BuildGraph(ctx context.Context, b *domain.EvidenceBundle) (*domain.RelationshipGraph, error) {
	if b == nil {
		return nil, fmt.Errorf("relations: nil bundle: %w", investigate.ErrRejected)
	}
	g, err := bd.build(ctx, b, SideTo, nil)
	if err != nil {
		return nil, err
	}
	return g.finish(), nil
}

// build reads the environment at one side and adds every node and edge. A
// reference view (v non-nil) sees only objects that exist at that side: the
// graph's extra nodes for objects of rendered changes (removed objects, so
// they can be roots) would make references to them resolve.
func (bd *builder) build(ctx context.Context, b *domain.EvidenceBundle, side Side, v *refView) (*gb, error) {
	env, err := load(ctx, b, bd.st, side)
	if err != nil {
		return nil, err
	}
	g := &gb{bundle: b, nodes: map[string]*gnode{}, edges: map[string]domain.GraphEdge{}, byName: map[string][]*gnode{}, byKey: map[string]*gnode{}, producers: map[string][]producer{}, objNode: map[*object]*gnode{}, env: env, rels: bd.rels, view: v, nsCache: map[string]nsResolution{}}
	if bd.consumers && v == nil {
		g.consumers, g.gen, g.docs, g.renders = true, map[string]*genData{}, map[string]*parsedArtifact{}, map[string][]render.Object{}
		g.rules, g.side = bd.consumerRules, side
		if g.rules < ConsumerRulesV1 || g.rules > ConsumerRulesV3 {
			g.rules = ConsumerRules
		}
		g.read = func(id string) ([]byte, error) {
			ref, ok := b.Manifest.Lookup(id)
			if !ok {
				return nil, fmt.Errorf("relations: artifact %s is not in the manifest: %w", id, investigate.ErrNotFound)
			}
			return readArtifact(ctx, bd.st, ref)
		}
	}

	env.cluster = clusterKinds(env.objects, bd.rels)
	applyNamespaces(env)
	objs := dedupe(env.objects)

	// Nodes: every object, then the objects the environment renders changed.
	for _, o := range objs {
		g.objectNode(o)
	}
	if v == nil {
		for _, p := range b.Renders {
			for _, c := range p.Changes {
				if id, ok := parseObjectKey(c.Object); ok && g.find(id.String()) == nil {
					g.node(classify(id), id, id.String(), p.Diff)
				}
			}
		}
	}
	for _, k := range sortedKusts(env) {
		g.node(domain.NodeKustomization, render.ObjectID{Group: groupKustomize, Version: "v1beta1", Kind: "Kustomization", Name: k.dir}, k.key(), k.artifact)
	}
	g.generatorNodes()
	for _, n := range g.sortedNodes() {
		g.index(n)
	}
	g.indexProducers(bd.rels)

	g.kustomizeEdges()
	g.gitopsEdges()
	g.renderEdges()
	g.referenceEdges()
	g.selectorEdges()
	g.relationshipSelectEdges()
	g.apiEdges()
	if g.consumers {
		if err := g.consumerEdges(); err != nil {
			return nil, err
		}
	}
	return g, nil
}

// clusterKinds is the set of kinds known to be cluster-scoped: the built-in
// ones, kinds a declared relationship says are cluster-scoped, and kinds of
// CRDs in the snapshot with spec.scope: Cluster.
func clusterKinds(objs []*object, rels []rel) map[string]bool {
	out := map[string]bool{}
	for k := range clusterScoped {
		out[k] = true
	}
	for _, r := range rels {
		if r.r.Scope == "cluster" {
			out[r.r.To.Kind] = true
		}
		if r.r.NamespaceFrom != nil { // the rule states its referrer is cluster-scoped
			out[r.r.From.Kind] = true
		}
	}
	for _, o := range objs {
		if o.id.Kind == "CustomResourceDefinition" && str(o.body, "spec", "scope") == "Cluster" {
			if k := str(o.body, "spec", "names", "kind"); k != "" {
				out[k] = true
			}
		}
	}
	return out
}

func (g *gb) index(n *gnode) {
	k := n.id.Kind + "|" + n.id.Name
	g.byName[k] = append(g.byName[k], n)
	g.byKey[n.n.Key] = n
}

func (g *gb) find(key string) *gnode {
	for _, n := range g.nodes {
		if n.n.Key == key {
			return n
		}
	}
	return nil
}

func (g *gb) sortedNodes() []*gnode {
	out := make([]*gnode, 0, len(g.nodes))
	for _, n := range g.nodes {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].n.ID < out[j].n.ID })
	return out
}

func sortedKusts(env *environment) []*kustomization {
	out := make([]*kustomization, 0, len(env.kustomizations))
	for _, k := range env.kustomizations {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].dir < out[j].dir })
	return out
}

func parseObjectKey(s string) (render.ObjectID, bool) {
	parts := strings.SplitN(s, "/", 5)
	if len(parts) != 5 || parts[2] == "" || parts[4] == "" {
		return render.ObjectID{}, false
	}
	group := parts[0]
	if group == "core" {
		group = ""
	}
	return render.ObjectID{Group: group, Version: parts[1], Kind: parts[2], Namespace: parts[3], Name: parts[4]}, true
}

// --- namespaces and de-duplication -----------------------------------------------------------

// applyNamespaces gives file objects the namespace kustomize (outermost
// `namespace:` wins) or the deploying Application's destination would give
// them, and rendered objects that leave it unset the destination of the
// Application that rendered them. Resolution of references needs it; keys of
// rendered objects stay as rendered so rendered changes keep naming them.
func applyNamespaces(env *environment) {
	env.dirNS = map[string]string{}
	env.dirDest = map[string]string{}
	fileNS := map[string]string{}
	fileDest := map[string]string{}
	visited := map[string]bool{}

	var walk func(dir, inherited, dest string)
	walk = func(dir, inherited, dest string) {
		k := env.kustomizations[dir]
		if k == nil || visited[dir] {
			return
		}
		visited[dir] = true
		eff := inherited
		if eff == "" {
			eff = str(k.body, "namespace")
		}
		env.dirNS[dir], env.dirDest[dir] = eff, dest
		for _, key := range []string{"resources", "bases", "components"} {
			for _, r := range asSeq(k.body[key]) {
				rs, _ := r.(string)
				if rs == "" {
					continue
				}
				p := path.Join(dir, rs)
				if _, ok := env.files[p]; ok {
					if _, set := fileNS[p]; !set && eff != "" {
						fileNS[p] = eff
					}
					if _, set := fileDest[p]; !set && dest != "" {
						fileDest[p] = dest
					}
				} else {
					walk(p, eff, dest)
				}
			}
		}
	}

	// Applications first (they carry a destination), then kustomizations no
	// other kustomization includes.
	for _, o := range env.objects {
		if o.file == "" || !isGitOps(o.id) {
			continue
		}
		dest := appDestination(o)
		for _, dir := range appSourceDirs(o) {
			walk(dir, "", dest)
			markPlain(env, dir, dest, fileDest)
		}
	}
	referenced := map[string]bool{}
	for _, k := range env.kustomizations {
		for _, key := range []string{"resources", "bases", "components"} {
			for _, r := range asSeq(k.body[key]) {
				if rs, _ := r.(string); rs != "" {
					referenced[path.Join(k.dir, rs)] = true
				}
			}
		}
	}
	for _, k := range sortedKusts(env) {
		if !referenced[k.dir] {
			walk(k.dir, "", "")
		}
	}

	for _, o := range env.objects {
		if o.file != "" && !env.cluster[o.id.Kind] {
			if ns, ok := fileNS[o.file]; ok {
				o.id.Namespace, o.effNS = ns, ns
			} else if o.id.Namespace == "" {
				if d, ok := fileDest[o.file]; ok {
					o.id.Namespace, o.effNS = d, d
				}
			}
		}
	}
	// Rendered objects: the destination of the Application that rendered them.
	targets := make([]string, 0, len(env.pairObjects))
	for t := range env.pairObjects {
		targets = append(targets, t)
	}
	sort.Strings(targets)
	for _, t := range targets {
		dest := ""
		for _, a := range env.appsOfTarget(t) {
			if dest = appDestination(a); dest != "" {
				break
			}
		}
		if dest == "" {
			continue
		}
		for _, o := range env.pairObjects[t] {
			if o.id.Namespace == "" && !env.cluster[o.id.Kind] {
				o.effNS = dest
			}
		}
	}
}

// markPlain applies an Application's destination to plain manifest files
// under a directory that has no kustomization.
func markPlain(env *environment, dir, dest string, fileDest map[string]string) {
	if dest == "" || env.kustomizations[dir] != nil {
		return
	}
	for p := range env.files {
		if strings.HasPrefix(p, dir+"/") && path.Dir(p) == dir {
			if _, set := fileDest[p]; !set {
				fileDest[p] = dest
			}
		}
	}
}

func isGitOps(id render.ObjectID) bool { return classify(id) == domain.NodeGitOpsApplication }

func appDestination(o *object) string {
	if ns := str(o.body, "spec", "destination", "namespace"); ns != "" {
		return ns
	}
	return str(o.body, "spec", "targetNamespace")
}

// appSources returns the Application's source blocks (spec.source and
// spec.sources[]).
func appSources(o *object) []map[string]any {
	var out []map[string]any
	if m := asMap(get(o.body, "spec", "source")); m != nil {
		out = append(out, m)
	}
	for _, s := range asSeq(get(o.body, "spec", "sources")) {
		if m := asMap(s); m != nil {
			out = append(out, m)
		}
	}
	return out
}

// appSourceDirs are the logical env directories a GitOps object deploys
// from (Argo source path, Flux Kustomization spec.path), when it names one.
func appSourceDirs(o *object) []string {
	var out []string
	add := func(p string) {
		p = strings.TrimPrefix(path.Clean(p), "./")
		if p == "." || p == "" || strings.HasPrefix(p, "..") {
			return
		}
		out = append(out, "env/"+p)
	}
	if o.id.Group == groupFluxKust {
		if p := str(o.body, "spec", "path"); p != "" {
			add(p)
		}
		return out
	}
	for _, s := range appSources(o) {
		if str(s, "chart") == "" {
			if p := str(s, "path"); p != "" {
				add(p)
			}
		}
	}
	return out
}

func (env *environment) appsOfTarget(target string) []*object {
	i := strings.Index(target, ":")
	if i < 0 {
		return nil
	}
	scheme, rest := target[:i], target[i+1:]
	if j := strings.Index(rest, "#"); j >= 0 {
		rest = rest[:j]
	}
	if scheme != "argocd" && scheme != "flux" {
		return nil
	}
	var out []*object
	for _, o := range env.objects {
		if o.file == "env/"+strings.TrimPrefix(path.Clean(rest), "./") && isGitOps(o.id) {
			out = append(out, o)
		}
	}
	return out
}

// dedupe drops file objects a render already supplies (same group, kind and
// name, namespace equal or unstated in the file) and repeated keys.
func dedupe(objs []*object) []*object {
	rendered := map[string][]string{}
	for _, o := range objs {
		if o.rendered {
			k := o.id.Group + "|" + o.id.Kind + "|" + o.id.Name
			rendered[k] = append(rendered[k], o.effNS)
		}
	}
	seen := map[string]bool{}
	var out []*object
	for _, o := range objs {
		if !o.rendered {
			dup := false
			for _, ns := range rendered[o.id.Group+"|"+o.id.Kind+"|"+o.id.Name] {
				if ns == o.effNS || o.effNS == "" || ns == "" {
					dup = true
				}
			}
			if dup {
				continue
			}
		}
		if seen[o.key()] {
			continue
		}
		seen[o.key()] = true
		out = append(out, o)
	}
	return out
}
