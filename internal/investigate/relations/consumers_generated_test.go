package relations

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Generated configuration whose source the snapshot lacks (consumer rules
// v2, ledger D-0005). The snapshot collects YAML files only, so a
// configMapGenerator's application config file is absent; the generated
// object, with its data, is in the render of the overlay that builds it.

// overlay adds a version-independent overlay render (a not-applicable pair
// whose one render states the objects at both sides).
func (f *fixture) overlay(target, rendered string) domain.ArtifactRef {
	f.t.Helper()
	ref := f.put(domain.ArtifactRenderedManifests, domain.OriginDerived, "derived/render/env/"+target+"/from.yaml", rendered)
	f.b.Renders = append(f.b.Renders, domain.ManifestPair{Scope: domain.RenderEnvironment, Target: target, From: ref.ID,
		Status: domain.CollectionNotApplicable, Reason: "version-independent: the overlay does not deploy the product"})
	return ref
}

const (
	genBase = `namespace: shop
resources: [deploy.yaml]
configMapGenerator:
  - name: api-proxy
    files: [proxy.conf]
    literals: ["LOG=/var/log/api/proxy.log"]
`
	genOverlay = `namePrefix: eu-
resources: [../../base]
`
	genDeploy = `apiVersion: apps/v1
kind: Deployment
metadata: {name: api}
spec:
  template:
    spec:
      volumes:
        - {name: ca, secret: {secretName: api-ca}}
        - {name: proxy, configMap: {name: api-proxy}}
        - {name: side, secret: {secretName: side-keys}}
      containers:
        - name: proxy
          volumeMounts:
            - {name: ca, mountPath: /etc/api/ca}
            - {name: proxy, mountPath: /etc/nginx/conf.d}
        - name: sidecar
          volumeMounts:
            - {name: side, mountPath: /etc/side/keys}
`
	// The overlay's render: the generated ConfigMap under its prefixed,
	// hash-suffixed name, and two that are not it — a ConfigMap whose
	// name only starts like it, and one of the same name in another namespace.
	genRender = `apiVersion: v1
kind: ConfigMap
metadata:
  name: eu-api-proxy-production
  namespace: shop
data:
  x.conf: "ca /etc/api/ca/unrelated.pem"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: eu-api-proxy-7k9b2mdhg5
  namespace: other
data:
  proxy.conf: "ca /etc/api/ca/other-namespace.pem"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: eu-api-proxy-5g7k9b2mdh
  namespace: shop
data:
  LOG: /var/log/api/proxy.log
  proxy.conf: |
    server {
      listen 8443 ssl;
      ssl_client_certificate /etc/api/ca/ca.crt;
      ssl_certificate_key /etc/side/keys/side.pem;
    }
`
	gProxy = "core/v1/ConfigMap/shop/api-proxy"
	gCA    = "core/v1/Secret/shop/api-ca"
)

// genFixture is the environment without the generator's source file.
func genFixture(t *testing.T) *fixture {
	f := newFixture(t)
	f.file("apps/api/base/kustomization.yaml", genBase)
	f.file("apps/api/base/deploy.yaml", genDeploy)
	f.file("apps/api/overlays/eu/kustomization.yaml", genOverlay)
	return f
}

func TestConsumerGeneratedDataFromRender(t *testing.T) {
	f := genFixture(t)
	r := f.overlay("kustomize:apps/api/overlays/eu", genRender)
	g := consumerGraph(t, f)
	v := view{t, g}

	e := v.wantEdge(domain.EdgeReadsFile, gCA, gProxy, domain.EdgeEstablished)
	if e.File.Key != "ca.crt" || e.File.ConfigKey != "proxy.conf" || e.File.Container != "proxy" || e.File.MountPath != "/etc/api/ca" {
		t.Fatalf("file detail: %+v", e.File)
	}
	// every hop cited: the render's artifact, line and text, and the object read
	if e.Line == nil || e.Line.Artifact != r.ID || e.Line.Line != 27 || e.Line.Text != "ssl_client_certificate /etc/api/ca/ca.crt;" {
		t.Fatalf("render line: %+v", e.Line)
	}
	if !strings.Contains(e.File.DataFrom, "v1/ConfigMap/shop/eu-api-proxy-5g7k9b2mdh") || !strings.Contains(e.File.DataFrom, "kustomize:apps/api/overlays/eu") ||
		!strings.Contains(e.File.DataFrom, "version-independent overlay") || !strings.Contains(e.File.DataFrom, "env/apps/api/base/proxy.conf, which is not in the environment snapshot") {
		t.Fatalf("data source: %q", e.File.DataFrom)
	}
	if !strings.HasPrefix(e.Rule, ruleReadsFileV2+"rendered-data:") || !contains(e.Artifacts, r.ID) {
		t.Fatalf("rule %s, artifacts %v", e.Rule, e.Artifacts)
	}
	if n := v.mustNode(gProxy); !contains(n.Artifacts, r.ID) {
		t.Fatalf("the generated node does not cite the render: %v", n.Artifacts)
	}
	// The snapshot's own data (a literal) stays the snapshot's.
	if x := readsFile(t, g, "/var/log/api/proxy.log"); len(x) != 1 || x[0].File.DataFrom != "" || x[0].Line == nil || x[0].Line.Artifact == r.ID {
		t.Fatalf("the literal's path: %+v", x)
	}
	// Negative control: a path under another container's mount (the sidecar's
	// Secret) does not resolve for the container that reads the configuration.
	side := readsFile(t, g, "/etc/side/keys/side.pem")
	if len(side) != 1 || side[0].Status != domain.EdgeMissing || side[0].From == v.mustNode("core/v1/Secret/shop/side-keys").ID {
		t.Fatalf("a path under another container's mount: %+v", side)
	}
	// Negative controls: ConfigMaps that are not the generated one.
	for _, p := range []string{"/etc/api/ca/unrelated.pem", "/etc/api/ca/other-namespace.pem"} {
		if x := readsFile(t, g, p); len(x) != 0 {
			t.Errorf("%s read from a ConfigMap that is not the generator's: %+v", p, x)
		}
	}
	for _, e := range g.Edges {
		if e.File != nil && e.File.Path == "" {
			t.Errorf("unknown data with the render present: %+v", e)
		}
	}
	// the FROM side reads the same overlay render
	from, err := BuildConsumerGraph(context.Background(), f.st, f.b, SideFrom)
	if err != nil {
		t.Fatal(err)
	}
	if fe, ok := (view{t, from}).edge(domain.EdgeReadsFile, gCA, gProxy); !ok || fe.Line == nil || fe.Line.Artifact != r.ID {
		t.Fatalf("from side: %+v", fe)
	}
}

func TestConsumerGeneratedDataUnknown(t *testing.T) {
	f := genFixture(t)
	g := consumerGraph(t, f)
	v := view{t, g}
	var unknown []domain.GraphEdge
	for _, e := range g.Edges {
		if e.Kind == domain.EdgeReadsFile && e.File != nil && e.File.Path == "" {
			unknown = append(unknown, e)
		}
	}
	if len(unknown) != 1 {
		t.Fatalf("unknown-data edges: %+v", unknown)
	}
	u := unknown[0]
	if u.To != v.mustNode(gProxy).ID || u.Status != domain.EdgeMissing || u.File.ConfigKey != "proxy.conf" || u.File.Container != "proxy" || u.File.MountPath != "/etc/nginx/conf.d" {
		t.Fatalf("unknown data: %+v %+v", u, u.File)
	}
	for _, want := range []string{"generated by configMapGenerator of env/apps/api/base/kustomization.yaml from file env/apps/api/base/proxy.conf, which is not in the environment snapshot",
		"no render of the to side holds the generated object", "the files it names are not known"} {
		if !strings.Contains(u.Reason, want) {
			t.Errorf("reason lacks %q: %s", want, u.Reason)
		}
	}
	if u.Line == nil || u.Line.Text != "files: [proxy.conf]" || !strings.HasPrefix(u.Rule, ruleReadsFileV2+"unknown-data:") {
		t.Fatalf("line %+v rule %s", u.Line, u.Rule)
	}
	for _, n := range g.Nodes {
		if n.ID == u.From && (!n.Placeholder || n.Kind != domain.NodeExternal) {
			t.Fatalf("from: %+v", n)
		}
	}
	if _, ok := v.edge(domain.EdgeReadsFile, gCA, gProxy); ok {
		t.Fatal("a reads-file edge from data nobody has")
	}
	// The literal the snapshot does hold still yields its edge.
	if x := readsFile(t, g, "/var/log/api/proxy.log"); len(x) != 1 {
		t.Fatalf("the literal's path: %+v", x)
	}
}

// A ConfigMap the snapshot does not define at all: its data is unknown too.
func TestConsumerUndefinedConfigMapData(t *testing.T) {
	f := newFixture(t)
	f.file("apps/api/deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: api, namespace: shop}
spec:
  template:
    spec:
      volumes:
        - {name: conf, configMap: {name: api-conf, items: [{key: app.yaml, path: app.yaml}]}}
      containers:
        - name: api
          volumeMounts: [{name: conf, mountPath: /etc/api}]
`)
	g := consumerGraph(t, f)
	n := 0
	for _, e := range g.Edges {
		if e.Kind == domain.EdgeReadsFile && e.File != nil && e.File.Path == "" {
			n++
			if e.Status != domain.EdgeMissing || e.File.ConfigKey != "" || !strings.Contains(e.Reason, "is not an object of the snapshot") ||
				!strings.Contains(e.Detail, "data[*]") {
				t.Fatalf("undefined ConfigMap: %+v", e)
			}
		}
	}
	if n != 1 {
		t.Fatalf("%d unknown-data edges", n)
	}
}

// secretGenerator: the keys are known from the generator even though the
// files are not in the snapshot; an env source the snapshot lacks leaves the
// keys unknown, so a key is inferred, never established.
func TestConsumerGeneratedSecretKeys(t *testing.T) {
	f := newFixture(t)
	f.file("apps/api/kustomization.yaml", `namespace: shop
resources: [deploy.yaml, config.yaml]
secretGenerator:
  - name: api-ca
    files: [ca.crt]
  - name: api-env
    envs: [secret.env]
`)
	f.file("apps/api/config.yaml", `apiVersion: v1
kind: ConfigMap
metadata: {name: api-conf}
data:
  app.ini: |
    ca = /etc/api/ca/ca.crt
    other = /etc/api/ca/other.pem
    token = /etc/api/env/TOKEN
`)
	f.file("apps/api/deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: api}
spec:
  template:
    spec:
      volumes:
        - {name: ca, secret: {secretName: api-ca}}
        - {name: env, secret: {secretName: api-env}}
        - {name: conf, configMap: {name: api-conf}}
      containers:
        - name: api
          volumeMounts:
            - {name: ca, mountPath: /etc/api/ca}
            - {name: env, mountPath: /etc/api/env}
            - {name: conf, mountPath: /etc/api/conf}
`)
	g := consumerGraph(t, f)
	if x := readsFile(t, g, "/etc/api/ca/ca.crt"); len(x) != 1 || x[0].Status != domain.EdgeEstablished || x[0].File.Key != "ca.crt" {
		t.Fatalf("a key the generator names: %+v", x)
	}
	if x := readsFile(t, g, "/etc/api/ca/other.pem"); len(x) != 1 || x[0].Status != domain.EdgeMissing || !strings.Contains(x[0].Reason, "holds no such key") {
		t.Fatalf("a key the generator does not name: %+v", x)
	}
	if x := readsFile(t, g, "/etc/api/env/TOKEN"); len(x) != 1 || x[0].Status != domain.EdgeInferred || !strings.Contains(x[0].Reason, "whose keys are not in the snapshot") {
		t.Fatalf("a key of a Secret whose keys are unknown: %+v", x)
	}
	// Rules v1 checked no key of a generated Secret.
	v1, err := BuildConsumerGraph(context.Background(), f.st, f.b, SideTo, WithConsumerRules(ConsumerRulesV1))
	if err != nil {
		t.Fatal(err)
	}
	if x := readsFile(t, v1, "/etc/api/ca/other.pem"); len(x) != 1 || x[0].Status != domain.EdgeEstablished {
		t.Fatalf("v1: %+v", x)
	}
}

// Two overlays render the generated object with different data (one merges
// into it): which is the generator's is not decided, so its data stays
// unknown — fail closed, never a guess.
func TestConsumerGeneratedDataAmbiguous(t *testing.T) {
	f := genFixture(t)
	f.file("apps/api/overlays/us/kustomization.yaml", "namePrefix: eu-\nresources: [../../base]\n")
	f.overlay("kustomize:apps/api/overlays/eu", genRender)
	f.overlay("kustomize:apps/api/overlays/us", strings.Replace(genRender, "/etc/api/ca/ca.crt", "/etc/api/ca/us.crt", 1))
	g := consumerGraph(t, f)
	if _, ok := (view{t, g}).edge(domain.EdgeReadsFile, gCA, gProxy); ok {
		t.Fatal("data read from one of two differing renders")
	}
	found := false
	for _, e := range g.Edges {
		if e.File != nil && e.File.Path == "" && strings.Contains(e.Reason, "2 rendered objects match it with different data") {
			found = true
		}
	}
	if !found {
		t.Fatal("no unknown-data edge naming the ambiguity")
	}
}

// With the hash suffix disabled, the generated object keeps its name.
func TestConsumerGeneratedDataNoHash(t *testing.T) {
	f := newFixture(t)
	f.file("apps/api/base/kustomization.yaml", strings.Replace(genBase, "configMapGenerator:", "generatorOptions: {disableNameSuffixHash: true}\nconfigMapGenerator:", 1))
	f.file("apps/api/base/deploy.yaml", genDeploy)
	f.file("apps/api/overlays/eu/kustomization.yaml", genOverlay)
	f.overlay("kustomize:apps/api/overlays/eu", genRender)
	g := consumerGraph(t, f)
	if _, ok := (view{t, g}).edge(domain.EdgeReadsFile, gCA, gProxy); ok {
		t.Fatal("a hash-suffixed object taken for an unhashed generator")
	}
	f.overlay("kustomize:apps/api/overlays/eu", strings.Replace(genRender, "eu-api-proxy-5g7k9b2mdh", "eu-api-proxy", 1))
	f.b.Renders = f.b.Renders[1:]
	if _, ok := (view{t, consumerGraph(t, f)}).edge(domain.EdgeReadsFile, gCA, gProxy); !ok {
		t.Fatal("the unhashed generated object is not found")
	}
}

// Rules v1 build the Milestone 1 graph: generated data the snapshot lacks is
// skipped (the defect v2 fixes). Graphs without such data are the same under
// both rules but for the builder name, so earlier transcripts of such
// environments replay under either.
func TestConsumerRulesV1(t *testing.T) {
	f := genFixture(t)
	f.overlay("kustomize:apps/api/overlays/eu", genRender)
	g, err := BuildConsumerGraph(context.Background(), f.st, f.b, SideTo, WithConsumerRules(ConsumerRulesV1))
	if err != nil {
		t.Fatal(err)
	}
	if g.Builder != BuilderName+"+consumers@v1" {
		t.Fatalf("builder %q", g.Builder)
	}
	for _, e := range g.Edges {
		if e.Kind == domain.EdgeReadsFile && (e.File.Path == "" || e.File.DataFrom != "" || strings.HasPrefix(e.Rule, ruleReadsFileV2)) {
			t.Fatalf("a v2 edge under v1: %+v", e)
		}
	}
	if _, ok := (view{t, g}).edge(domain.EdgeReadsFile, gCA, gProxy); ok {
		t.Fatal("v1 read the render")
	}

	plain := newFixture(t)
	plain.file("apps/billing/all.yaml", consumerEnv)
	a, err := BuildConsumerGraph(context.Background(), plain.st, plain.b, SideTo, WithConsumerRules(ConsumerRulesV1))
	if err != nil {
		t.Fatal(err)
	}
	b := consumerGraph(t, plain)
	x, _ := json.Marshal(a.Nodes)
	y, _ := json.Marshal(b.Nodes)
	xe, _ := json.Marshal(a.Edges)
	ye, _ := json.Marshal(b.Edges)
	if string(x) != string(y) || string(xe) != string(ye) {
		t.Fatal("a graph without generated data differs between rules v1 and v2")
	}
}
