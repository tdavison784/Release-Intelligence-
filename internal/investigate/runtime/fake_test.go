package runtime

// Fakes for the runtime validator's tests: a fake driver and an in-memory
// cluster that simulates a fictional product "gizmo" (never an evaluation
// case). Its controller (Deployment gizmo/gizmo-controller, image tag v1 or
// v2) issues a Secret for every Widget (spec.secretName: keys crt and key)
// and re-issues it when the Widget's status gets Issuing=True; at v2 it also
// rotates the key unless the Widget pins spec.rotation: Never — a behaviour
// default compiled into the new binary, invisible in any render.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/renderdiff"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// --- clock --------------------------------------------------------------------------------

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
	return nil
}

// --- driver -------------------------------------------------------------------------------

type fakeDriver struct {
	mu         sync.Mutex
	preflight  error
	free       uint64
	existing   []string
	createErr  error
	deleteErr  error
	keepOnDel  bool // Delete "succeeds" but the cluster stays listed
	cluster    *fakeCluster
	creates    int
	deletes    []string
	setup      func(c *fakeCluster)
	lastCreate ClusterSpec
}

func (d *fakeDriver) Preflight(context.Context) (map[string]string, error) {
	return map[string]string{"kind": "fake"}, d.preflight
}

func (d *fakeDriver) FreeDisk(context.Context) (uint64, error) { return d.free, nil }

func (d *fakeDriver) Clusters(context.Context) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.existing...), nil
}

func (d *fakeDriver) Create(ctx context.Context, spec ClusterSpec) (Cluster, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.creates++
	d.lastCreate = spec
	d.existing = append(d.existing, spec.Name)
	if d.createErr != nil {
		return nil, d.createErr
	}
	d.cluster = newGizmoCluster(spec.Name)
	if d.setup != nil {
		d.setup(d.cluster)
	}
	return d.cluster, nil
}

func (d *fakeDriver) Delete(_ context.Context, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deletes = append(d.deletes, name)
	if d.deleteErr != nil {
		return d.deleteErr
	}
	if !d.keepOnDel {
		var keep []string
		for _, n := range d.existing {
			if n != name {
				keep = append(keep, n)
			}
		}
		d.existing = keep
	}
	return nil
}

// --- cluster ------------------------------------------------------------------------------

type fakeCluster struct {
	mu      sync.Mutex
	name    string
	objs    map[string]map[string]any
	events  []map[string]any
	logs    map[string]string // "<ns>/<pod>" -> text
	serial  int
	applies int
	// behaviour switches
	crashAtV1      bool                   // the v1 controller crash-loops (no baseline)
	crashAtV2      bool                   // the v2 controller crash-loops
	pullFailAtV2   bool                   // the v2 controller image cannot be pulled
	rejectAtV2     func(m Manifest) error // the v2 admission webhook's verdict
	patchErr       error
	patchRejectV2  error // the v2 admission webhook refuses the exercise's update
	appBreaksOnKey bool  // the consuming app crash-loops once its key rotated
	ignoreAtV2     bool  // the v2 controller accepts the Issuing patch but never re-issues
	ignoreAlways   bool  // no version re-issues
	keyRotated     bool
	block          bool // Apply blocks until the context ends
	warnAtV2Renew  bool // the v2 controller emits a warning event on renewal
	logAtV2Renew   bool
	renewals       int
	consumerFailed bool
}

func newGizmoCluster(name string) *fakeCluster {
	return &fakeCluster{name: name, objs: map[string]map[string]any{}, logs: map[string]string{}}
}

func (c *fakeCluster) Name() string { return c.name }

var clusterScoped = map[string]bool{"Namespace": true, "CustomResourceDefinition": true, "ClusterRole": true, "ClusterRoleBinding": true}

var builtin = map[string]bool{"": true, "apps": true, "batch": true, "apiextensions.k8s.io": true, "rbac.authorization.k8s.io": true, "networking.k8s.io": true}

func deepCopy(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func (c *fakeCluster) served(g GVK) bool {
	if builtin[g.Group] {
		return true
	}
	for _, o := range c.objs {
		r, _ := refOf(o)
		if isCRD(r) {
			spec, _ := o["spec"].(map[string]any)
			names, _ := spec["names"].(map[string]any)
			if spec["group"] == g.Group && names["kind"] == g.Kind {
				return true
			}
		}
	}
	return false
}

func (c *fakeCluster) version() string {
	d := c.objs["apps/v1/Deployment/gizmo/gizmo-controller"]
	if d == nil {
		return ""
	}
	b, _ := json.Marshal(d)
	if strings.Contains(string(b), "gizmo:v2") {
		return "v2"
	}
	return "v1"
}

func (c *fakeCluster) Apply(ctx context.Context, m Manifest, namespace string) (map[string]any, error) {
	if c.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.applies++
	if !c.served(m.Ref.GVK()) {
		return nil, fmt.Errorf(`resource mapping not found for name: %q namespace: "" from "STDIN": no matches for kind %q in version %q`, m.Ref.Name, m.Ref.Kind, m.Ref.APIVersion())
	}
	if c.rejectAtV2 != nil && c.version() == "v2" {
		if err := c.rejectAtV2(m); err != nil {
			return nil, err
		}
	}
	obj := deepCopy(m.Doc)
	r := m.Ref
	if r.Namespace == "" && !clusterScoped[r.Kind] {
		r.Namespace = namespace
		if r.Namespace == "" {
			r.Namespace = "default"
		}
		obj["metadata"].(map[string]any)["namespace"] = r.Namespace
	}
	id := r.String()
	gen := int64(1)
	if prev, ok := c.objs[id]; ok {
		meta := prev["metadata"].(map[string]any)
		gen = toInt(meta["generation"])
		pb, _ := json.Marshal(prev["spec"])
		nb, _ := json.Marshal(obj["spec"])
		if string(pb) != string(nb) {
			gen++
		}
		obj["status"] = prev["status"]
		if ann := annotations(prev); len(ann) > 0 {
			meta := obj["metadata"].(map[string]any)
			merged, _ := meta["annotations"].(map[string]any)
			if merged == nil {
				merged = map[string]any{}
			}
			for k, v := range ann {
				if _, set := merged[k]; !set && strings.HasPrefix(k, "ri.dev/") {
					merged[k] = v
				}
			}
			meta["annotations"] = merged
		}
	}
	obj["metadata"].(map[string]any)["generation"] = gen
	c.objs[id] = obj
	return deepCopy(obj), nil
}

func (c *fakeCluster) Delete(_ context.Context, ref ObjectRef) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.objs, ref.String())
	return nil
}

func (c *fakeCluster) List(_ context.Context, g GVK) ([]map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g.Kind == "Event" {
		var out []map[string]any
		for _, e := range c.events {
			out = append(out, deepCopy(e))
		}
		return out, nil
	}
	if !c.served(g) {
		return nil, ErrUnservedKind
	}
	c.reconcile()
	var ids []string
	for id := range c.objs {
		r, _ := ParseRef(id)
		if r.Group == g.Group && r.Kind == g.Kind {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	var out []map[string]any
	for _, id := range ids {
		out = append(out, deepCopy(c.objs[id]))
	}
	return out, nil
}

func (c *fakeCluster) Patch(_ context.Context, ref ObjectRef, sub string, patch []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.patchErr != nil {
		return c.patchErr
	}
	if c.patchRejectV2 != nil && c.version() == "v2" {
		return c.patchRejectV2
	}
	obj, ok := c.objs[ref.String()]
	if !ok {
		return fmt.Errorf("%s not found", ref)
	}
	var p map[string]any
	if err := json.Unmarshal(patch, &p); err != nil {
		return err
	}
	if sub == "status" {
		p = map[string]any{"status": p["status"]}
	}
	mergePatch(obj, p)
	c.react(ref, obj)
	return nil
}

func mergePatch(dst, p map[string]any) {
	for k, v := range p {
		pm, isMap := v.(map[string]any)
		dm, dstMap := dst[k].(map[string]any)
		if isMap && dstMap {
			mergePatch(dm, pm)
			continue
		}
		dst[k] = v
	}
}

func (c *fakeCluster) Logs(_ context.Context, ns, pod string, _ time.Time) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logs[ns+"/"+pod], nil
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// reconcile is the simulated controllers: statuses, pods, issued Secrets.
func (c *fakeCluster) reconcile() {
	ver := c.version()
	for id, o := range c.objs {
		r, _ := ParseRef(id)
		meta := o["metadata"].(map[string]any)
		switch {
		case isCRD(r):
			o["status"] = map[string]any{"conditions": []any{map[string]any{"type": "Established", "status": "True"}}}
		case r.Group == "apps" && r.Kind == "Deployment":
			gen := toInt(meta["generation"])
			avail := int64(1)
			waiting := ""
			if r.Name == "gizmo-controller" && ((ver == "v2" && c.crashAtV2) || (ver == "v1" && c.crashAtV1)) {
				avail, waiting = 0, "CrashLoopBackOff"
			}
			if r.Name == "app" && c.appBreaksOnKey && c.keyRotated {
				avail, waiting = 0, "CrashLoopBackOff"
			}
			if r.Name == "gizmo-controller" && ver == "v2" && c.pullFailAtV2 {
				avail, waiting = 0, "ImagePullBackOff"
			}
			o["status"] = map[string]any{"observedGeneration": gen, "replicas": 1, "availableReplicas": avail, "readyReplicas": avail, "updatedReplicas": 1}
			sel := selectorOf(o)
			labels := map[string]any{}
			for k, v := range sel {
				labels[k] = v
			}
			pod := map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": r.Name + "-abc", "namespace": r.Namespace, "labels": labels},
				"status": map[string]any{"containerStatuses": []any{map[string]any{"name": "c", "restartCount": 0, "state": map[string]any{}}}}}
			if waiting != "" {
				pod["status"].(map[string]any)["containerStatuses"] = []any{map[string]any{"name": "c", "restartCount": 3,
					"state": map[string]any{"waiting": map[string]any{"reason": waiting}}}}
			}
			c.objs[ObjectRef{Version: "v1", Kind: "Pod", Namespace: r.Namespace, Name: r.Name + "-abc"}.String()] = pod
		case r.Group == "gizmo.io" && r.Kind == "Widget":
			if ver == "" {
				continue
			}
			spec, _ := o["spec"].(map[string]any)
			secret, _ := spec["secretName"].(string)
			conds := []any{map[string]any{"type": "Ready", "status": "True", "reason": "Issued"}}
			if c.consumerFailed {
				conds = []any{map[string]any{"type": "Ready", "status": "False", "reason": "IssueFailed", "message": "the issuer refused"}}
			}
			st, _ := o["status"].(map[string]any)
			if st == nil {
				st = map[string]any{}
			}
			st["conditions"] = conds
			o["status"] = st
			sid := ObjectRef{Version: "v1", Kind: "Secret", Namespace: r.Namespace, Name: secret}.String()
			if _, ok := c.objs[sid]; !ok && secret != "" {
				c.serial++
				c.objs[sid] = map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": secret, "namespace": r.Namespace},
					"data": map[string]any{"crt": b64(fmt.Sprint("crt-", c.serial)), "key": b64(fmt.Sprint("key-", c.serial))}}
			}
		}
	}
}

// react is the controllers' response to a patch: a Widget marked Issuing is
// re-issued (crt always; key too at v2 unless pinned to Never).
func (c *fakeCluster) react(ref ObjectRef, obj map[string]any) {
	if ref.Kind != "Widget" {
		return
	}
	st, _ := obj["status"].(map[string]any)
	conds, _ := st["conditions"].([]any)
	issuing := false
	var keep []any
	for _, x := range conds {
		m := x.(map[string]any)
		if m["type"] == "Issuing" && m["status"] == "True" {
			issuing = true
			continue
		}
		keep = append(keep, m)
	}
	if !issuing || c.ignoreAlways || (c.ignoreAtV2 && c.version() == "v2") {
		return
	}
	st["conditions"] = keep
	st["revision"] = toInt(st["revision"]) + 1
	c.renewals++
	spec, _ := obj["spec"].(map[string]any)
	sec := c.objs[ObjectRef{Version: "v1", Kind: "Secret", Namespace: ref.Namespace, Name: spec["secretName"].(string)}.String()]
	if sec == nil {
		return
	}
	data := sec["data"].(map[string]any)
	c.serial++
	data["crt"] = b64(fmt.Sprint("crt-", c.serial))
	if c.version() == "v2" && spec["rotation"] != "Never" {
		data["key"] = b64(fmt.Sprint("key-", c.serial))
		c.keyRotated = true
	}
	if c.version() == "v2" && c.warnAtV2Renew {
		c.events = append(c.events, map[string]any{"type": "Warning", "reason": "BadConfig", "message": "renewal used defaults",
			"involvedObject": map[string]any{"kind": "Widget", "namespace": ref.Namespace, "name": ref.Name},
			"lastTimestamp":  "2099-01-01T00:00:00Z"})
	}
	if c.version() == "v2" && c.logAtV2Renew {
		c.logs["gizmo/gizmo-controller-abc"] += fmt.Sprintf("E1016 12:00:00.000000 1 widget.go:42] failed to sync %s/%s: bad\n", ref.Namespace, ref.Name)
	}
}

// --- renderer -----------------------------------------------------------------------------

type fakeRenderer struct {
	raw   map[string]*RawRender // "<target>|<side>"
	calls int
	err   error
}

func (r *fakeRenderer) RawRender(_ context.Context, _ *domain.EvidenceBundle, target, side string) (*RawRender, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	x, ok := r.raw[target+"|"+side]
	if !ok {
		return nil, errors.New("no such target")
	}
	return x, nil
}

// --- the gizmo environment ----------------------------------------------------------------

const (
	productTarget  = "argocd:apps/gizmo.yaml"
	consumerTarget = "kustomize:apps/shop"
)

func controllerYAML(tag string) string {
	return `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.gizmo.io
spec:
  group: gizmo.io
  names: {kind: Widget, plural: widgets}
  scope: Namespaced
  versions: [{name: v1, served: true, storage: true}]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gizmo-controller
  namespace: gizmo
spec:
  selector: {matchLabels: {app: gizmo}}
  template:
    metadata: {labels: {app: gizmo}}
    spec:
      containers: [{name: c, image: "gizmo:` + tag + `"}]
      volumes: [{name: tmp, emptyDir: {}}]
`
}

func consumerYAML(rotation string) string {
	pin := ""
	if rotation != "" {
		pin = "\n  rotation: " + rotation
	}
	return `apiVersion: gizmo.io/v1
kind: Widget
metadata:
  name: w1
  namespace: shop
spec:
  secretName: w1-tls` + pin + `
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
  namespace: shop
spec:
  selector: {matchLabels: {app: shop}}
  template:
    metadata: {labels: {app: shop}}
    spec:
      containers: [{name: c, image: "shop:1"}]
      volumes: [{name: tls, secret: {secretName: w1-tls}}]
`
}

// Node keys of the gizmo graph.
const (
	keyCRD        = "apiextensions.k8s.io/v1/CustomResourceDefinition//widgets.gizmo.io"
	keyAPIType    = "gizmo.io/v1/Widget"
	keyWidget     = "gizmo.io/v1/Widget/shop/w1"
	keySecret     = "core/v1/Secret/shop/w1-tls"
	keyApp        = "apps/v1/Deployment/shop/app"
	keyController = "apps/v1/Deployment/gizmo/gizmo-controller"
)

var (
	itemBehaviour = domain.ChangeItemID("chg-rotation", "", "")
	itemTwin      = domain.ChangeItemID("chg-rotation-guide", "", "")
	itemRendered  = domain.ChangeItemID("", "", "rc-controller")
	itemNoPath    = domain.ChangeItemID("chg-metrics", "", "")
)

type testEnv struct {
	t         *testing.T
	st        investigate.ArtifactStore
	b         *domain.EvidenceBundle
	g         *domain.RelationshipGraph
	tr        *domain.TraversalResult
	rnd       *fakeRenderer
	drv       *fakeDriver
	clock     *fakeClock
	exercises []DeclaredExercise
	opts      []Option
}

type envOptions struct {
	rotation      string // the consumer's pin ("" = unset)
	twin          bool   // a second behaviour item reaching the same objects
	secretMarker  bool   // the Secret node key carries a redaction marker
	partialRender bool
}

func renewExercise(families ...string) DeclaredExercise {
	return DeclaredExercise{Product: "gizmo", Digest: "sha256:x", Exercise: catalog.Exercise{ID: "renew", Summary: "re-issue every Widget",
		Target: catalog.ObjectType{Group: "gizmo.io", Kind: "Widget"}, Action: catalog.ActionStatusCondition,
		Condition: &catalog.ExerciseCondition{Type: "Issuing", Status: "True", Reason: "ManuallyTriggered"},
		Reaction:  &catalog.ExerciseReaction{Kind: catalog.ReactionFieldChanged, Path: "status.revision"},
		Families:  families, References: []catalog.ConfigReference{{URL: "https://example.io/gizmo/renew", Quote: "renew"}}}}
}

func newEnv(t *testing.T, o envOptions) *testEnv {
	t.Helper()
	ctx := context.Background()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := &testEnv{t: t, st: st, clock: newFakeClock(), drv: &fakeDriver{free: 100 << 30}, rnd: &fakeRenderer{raw: map[string]*RawRender{}}}
	raws := map[string]string{
		productTarget + "|from":  controllerYAML("v1"),
		productTarget + "|to":    controllerYAML("v2"),
		consumerTarget + "|from": consumerYAML(o.rotation),
	}
	var manifest []domain.ArtifactRef
	ids := map[string]string{}
	for key, raw := range raws {
		target, side, _ := strings.Cut(key, "|")
		e.rnd.raw[key] = &RawRender{Manifests: []byte(raw)}
		objs, err := render.ParseObjects([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		stored, _ := renderdiff.EncodeObjects(objs)
		slug := strings.NewReplacer(":", "-", "/", "-").Replace(target)
		ref, err := st.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactRenderedManifests, Path: "derived/render/env/" + slug + "/" + side + ".yaml",
			Origin: domain.OriginDerived, MediaType: "application/yaml", URI: "ri:test", Producer: "test@v1"}, stored)
		if err != nil {
			t.Fatal(err)
		}
		manifest = append(manifest, ref)
		ids[key] = ref.ID
	}
	sort.Slice(manifest, func(i, j int) bool { return manifest[i].Path < manifest[j].Path })
	rcController := domain.RenderedChange{ID: domain.RenderedChangeID(domain.RenderEnvironment, productTarget, keyController, "spec.template.spec.containers[name=c].image", "image-changed"),
		Class: "image-changed", Object: keyController, Path: "spec.template.spec.containers[name=c].image", Before: `"gizmo:v1"`, After: `"gizmo:v2"`,
		ChangeItems: []string{itemRendered}, Correlation: "release-anchor"}
	productStatus := domain.CollectionOK
	reason := ""
	var changes []domain.RenderedChange
	if o.partialRender {
		productStatus, reason = domain.CollectionPartial, "values-incomplete: valuesFrom outside the repository"
	} else {
		changes = []domain.RenderedChange{rcController}
	}
	items := []domain.ChangeItem{
		{ID: itemBehaviour, ChangeID: "chg-rotation", Family: domain.FamilyBehavior, Title: "rotation default is now Always", Breaking: true,
			Manifestation: domain.ManifestControllerBehaviour, ManifestationRule: "manifest:declared-behaviour", InScope: true, ScopeRule: "scope:breaking"},
		{ID: itemRendered, RenderedChange: "rc-controller", Family: domain.FamilyRendered, Title: "controller image changed",
			Manifestation: domain.ManifestRenderedConfig, ManifestationRule: "manifest:rendered-anchor", InScope: true, ScopeRule: "scope:rendered-unexplained"},
		{ID: itemNoPath, ChangeID: "chg-metrics", Family: domain.FamilyBehavior, Title: "a metric was removed", Breaking: true,
			Manifestation: domain.ManifestControllerBehaviour, ManifestationRule: "manifest:declared-behaviour", InScope: true, ScopeRule: "scope:breaking"},
	}
	if o.twin {
		items = append(items, domain.ChangeItem{ID: itemTwin, ChangeID: "chg-rotation-guide", Family: domain.FamilyProcedure, Title: "upgrade note: rotation",
			Manifestation: domain.ManifestControllerBehaviour, ManifestationRule: "manifest:declared-behaviour", InScope: true, ScopeRule: "scope:breaking"})
	}
	e.b = &domain.EvidenceBundle{ID: "bnd-gizmo" + fmt.Sprint(o.rotation, o.twin, o.secretMarker, o.partialRender),
		Change:   domain.ChangeEvidenceRecord{ID: "cer-gizmo", Product: domain.ProductRef{ID: "gizmo"}, From: domain.Version{Tag: "v1"}, To: domain.Version{Tag: "v2"}, Items: items},
		Snapshot: domain.EnvironmentSnapshot{ID: "snap-gizmo", Environment: "prod", KubernetesVersion: "1.31"},
		Renders: []domain.ManifestPair{
			{Scope: domain.RenderEnvironment, Target: consumerTarget, From: ids[consumerTarget+"|from"], Status: domain.CollectionNotApplicable, Reason: "version-independent: consumer overlay"},
			{Scope: domain.RenderEnvironment, Target: productTarget, From: ids[productTarget+"|from"], To: ids[productTarget+"|to"], Status: productStatus, Reason: reason, Changes: changes},
		},
		Manifest: domain.BundleManifest{Artifacts: manifest}}
	secretKey := keySecret
	if o.secretMarker {
		secretKey = "core/v1/Secret/shop/" + bundleMarkerPrefix + sha256Hex("w1-tls")[:12] + ">"
	}
	node := func(kind domain.NodeKind, key string) domain.GraphNode {
		return domain.GraphNode{ID: domain.NodeID(kind, key), Kind: kind, Key: key}
	}
	nCRD, nAPI, nWidget, nSecret, nApp, nCtl := node(domain.NodeResource, keyCRD), node(domain.NodeAPIType, keyAPIType), node(domain.NodeCustomResource, keyWidget),
		node(domain.NodeConfigObject, secretKey), node(domain.NodeWorkload, keyApp), node(domain.NodeWorkload, keyController)
	edge := func(kind domain.EdgeKind, a, b domain.GraphNode) domain.GraphEdge {
		return domain.GraphEdge{ID: domain.EdgeID(kind, a.ID, b.ID, ""), Kind: kind, From: a.ID, To: b.ID, Status: domain.EdgeEstablished}
	}
	eServes, eConsumes, eProduces, eRefs := edge(domain.EdgeServesAPI, nCRD, nAPI), edge(domain.EdgeConsumesAPI, nAPI, nWidget),
		edge(domain.EdgeProduces, nWidget, nSecret), edge(domain.EdgeReferences, nSecret, nApp)
	e.g = &domain.RelationshipGraph{ID: "grf-gizmo", Nodes: []domain.GraphNode{nCRD, nAPI, nWidget, nSecret, nApp, nCtl},
		Edges: []domain.GraphEdge{eServes, eConsumes, eProduces, eRefs}}
	path := func(item string, nodes []domain.GraphNode, edges []domain.GraphEdge) domain.ImpactPath {
		var ns, es []string
		for _, n := range nodes {
			ns = append(ns, n.ID)
		}
		for _, x := range edges {
			es = append(es, x.ID)
		}
		return domain.ImpactPath{ID: domain.ImpactPathID(item, ns, es), ChangeItem: item, Nodes: ns, Edges: es, Status: domain.EdgeEstablished}
	}
	full := []domain.GraphNode{nCRD, nAPI, nWidget, nSecret, nApp}
	fullEdges := []domain.GraphEdge{eServes, eConsumes, eProduces, eRefs}
	e.tr = &domain.TraversalResult{ID: "trv-gizmo", Graph: e.g.ID,
		Roots: []domain.ExposureRoot{{ChangeItem: itemBehaviour, Node: nCRD.ID, Rule: "test"}, {ChangeItem: itemRendered, Node: nCtl.ID, Rule: "test"}},
		Paths: []domain.ImpactPath{path(itemBehaviour, full, fullEdges), path(itemRendered, []domain.GraphNode{nCtl}, nil)}, Complete: true}
	if o.twin {
		e.tr.Roots = append(e.tr.Roots, domain.ExposureRoot{ChangeItem: itemTwin, Node: nCRD.ID, Rule: "test"})
		e.tr.Paths = append(e.tr.Paths, path(itemTwin, full, fullEdges))
	}
	e.exercises = []DeclaredExercise{renewExercise("behavior", "procedure")}
	return e
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// setRender replaces one side of a render: raw for the renderer, the stored
// (normalized) artifact in the bundle.
func (e *testEnv) setRender(target, side, raw string) {
	e.t.Helper()
	objs, err := render.ParseObjects([]byte(raw))
	if err != nil {
		e.t.Fatal(err)
	}
	stored, _ := renderdiff.EncodeObjects(objs)
	slug := strings.NewReplacer(":", "-", "/", "-").Replace(target)
	ref, err := e.st.Put(context.Background(), investigate.ArtifactSpec{Kind: domain.ArtifactRenderedManifests, Path: "derived/render/env/" + slug + "/" + side + ".yaml",
		Origin: domain.OriginDerived, MediaType: "application/yaml", URI: "ri:test", Producer: "test@v1"}, stored)
	if err != nil {
		e.t.Fatal(err)
	}
	e.rnd.raw[target+"|"+side] = &RawRender{Manifests: []byte(raw)}
	for i := range e.b.Renders {
		p := &e.b.Renders[i]
		if p.Target != target {
			continue
		}
		old := p.From
		if side == "to" {
			old, p.To = p.To, ref.ID
		} else {
			p.From = ref.ID
		}
		arts := e.b.Manifest.Artifacts[:0]
		for _, a := range e.b.Manifest.Artifacts {
			if a.ID != old {
				arts = append(arts, a)
			}
		}
		e.b.Manifest.Artifacts = append(arts, ref)
	}
}

func (e *testEnv) validator(extra ...Option) *Validator {
	opts := []Option{WithStore(e.st), WithRenderer(e.rnd), WithDriver(e.drv), WithClock(e.clock), WithExercises(e.exercises...)}
	opts = append(opts, e.opts...)
	return New(append(opts, extra...)...)
}

func (e *testEnv) validate(v *Validator, item string) domain.ValidationRecord {
	e.t.Helper()
	out, err := v.Validate(context.Background(), investigate.ValidationInput{Bundle: e.b, Graph: e.g, Traversal: e.tr, ChangeItem: item})
	if err != nil {
		e.t.Fatalf("validate %s: %v", item, err)
	}
	if len(out.Records) != 1 {
		e.t.Fatalf("validate %s: %d records", item, len(out.Records))
	}
	arts := map[string]bool{}
	for _, a := range e.b.Manifest.Artifacts {
		arts[a.ID] = true
	}
	for _, a := range out.Artifacts {
		if err := a.Validate(); err != nil {
			e.t.Fatalf("artifact: %v", err)
		}
		arts[a.ID] = true
	}
	conforms(e.t, out.Records[0], arts, out.Evidence)
	return out.Records[0]
}
