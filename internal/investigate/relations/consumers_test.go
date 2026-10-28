package relations

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// An invented environment: a credential custom resource whose controller
// produces a Secret; a "billing" Deployment mounts that Secret (subPath and
// items on different containers), a ConfigMap with its configuration, and a
// projected volume; its configuration names paths under those mounts, under
// no mount, and under an emptyDir. A second Deployment ("audit") mounts
// another Secret at a path billing's configuration also names: billing's
// path must not resolve to it.
const consumerEnv = `apiVersion: example.dev/v1
kind: Credential
metadata: {name: billing-tls, namespace: shop}
spec: {secretName: billing-tls}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: billing, namespace: shop}
spec:
  template:
    spec:
      volumes:
        - name: tls
          secret: {secretName: billing-tls}
        - name: conf
          configMap: {name: billing-config}
        - name: bundle
          projected:
            sources:
              - secret:
                  name: billing-ca
                  items: [{key: ca.crt, path: roots/ca.pem}]
              - configMap: {name: billing-extra}
        - name: scratch
          emptyDir: {}
      containers:
        - name: app
          volumeMounts:
            - {name: tls, mountPath: /etc/billing/tls}
            - {name: conf, mountPath: /etc/billing/conf}
            - {name: bundle, mountPath: /etc/billing/bundle}
            - {name: scratch, mountPath: /var/scratch}
        - name: sidecar
          volumeMounts:
            - {name: tls, mountPath: /certs/ca.crt, subPath: ca.crt}
---
apiVersion: v1
kind: ConfigMap
metadata: {name: billing-config, namespace: shop}
data:
  app.ini: |
    [tls]
    ca_file = /etc/billing/tls/ca.crt
    roots = /etc/billing/bundle/roots/ca.pem
    extra = /etc/billing/bundle/limits.json
    audit_key = /etc/audit/keys/signing.key
    spool = /var/scratch/spool/out
    log = /var/log/billing/app.log
    route = /healthz
    self = /etc/billing/conf/other.ini
    see https://example.dev/docs/ca for details
---
apiVersion: v1
kind: ConfigMap
metadata: {name: billing-extra, namespace: shop}
data:
  limits.json: "{}"
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: audit, namespace: shop}
spec:
  template:
    spec:
      volumes:
        - name: keys
          secret: {secretName: audit-keys}
      containers:
        - name: audit
          volumeMounts:
            - {name: keys, mountPath: /etc/audit/keys}
`

const (
	cBilling = "apps/v1/Deployment/shop/billing"
	cConfig  = "core/v1/ConfigMap/shop/billing-config"
	cTLS     = "core/v1/Secret/shop/billing-tls"
)

func consumerGraph(t *testing.T, f *fixture) *domain.RelationshipGraph {
	t.Helper()
	g, err := BuildConsumerGraph(context.Background(), f.st, f.b, SideTo)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("consumer graph does not validate: %v", err)
	}
	if g.Builder != ConsumerBuilderName {
		t.Fatalf("builder %q", g.Builder)
	}
	return g
}

// readsFile finds the reads-file edge naming path p.
func readsFile(t *testing.T, g *domain.RelationshipGraph, p string) []domain.GraphEdge {
	t.Helper()
	var out []domain.GraphEdge
	for _, e := range g.Edges {
		if e.Kind == domain.EdgeReadsFile && e.File != nil && e.File.Path == p {
			out = append(out, e)
		}
	}
	return out
}

func TestConsumerMountDetail(t *testing.T) {
	f := newFixture(t)
	f.file("apps/billing/all.yaml", consumerEnv)
	v := view{t, consumerGraph(t, f)}

	e := v.wantEdge(domain.EdgeReferences, cTLS, cBilling, domain.EdgeInferred) // produced by convention, not declared
	m := e.Mount
	if m == nil || m.Volume != "tls" || m.Source != -1 || len(m.Items) != 0 {
		t.Fatalf("mount detail: %+v", m)
	}
	want := []domain.VolumeMount{{Container: "app", MountPath: "/etc/billing/tls"}, {Container: "sidecar", MountPath: "/certs/ca.crt", SubPath: "ca.crt"}}
	if len(m.Mounts) != 2 || m.Mounts[0] != want[0] || m.Mounts[1] != want[1] {
		t.Fatalf("mounts (subPath on the sidecar): %+v", m.Mounts)
	}
	if e.Line == nil || !strings.Contains(e.Line.Text, "secretName: billing-tls") {
		t.Fatalf("line of the reference: %+v", e.Line)
	}
	// projected: the source index and its items (key → path)
	p := v.wantEdge(domain.EdgeReferences, "core/v1/Secret/shop/billing-ca", cBilling, domain.EdgeMissing)
	if p.Mount == nil || p.Mount.Source != 0 || len(p.Mount.Items) != 1 || p.Mount.Items[0] != (domain.KeyToPath{Key: "ca.crt", Path: "roots/ca.pem"}) {
		t.Fatalf("projected mount: %+v", p.Mount)
	}
	if x := v.wantEdge(domain.EdgeReferences, "core/v1/ConfigMap/shop/billing-extra", cBilling, domain.EdgeEstablished); x.Mount == nil || x.Mount.Source != 1 {
		t.Fatalf("projected configMap source: %+v", x.Mount)
	}
	// the produces edge carries the producer's line
	pr := v.wantEdge(domain.EdgeProduces, "example.dev/v1/Credential/shop/billing-tls", cTLS, domain.EdgeInferred)
	if pr.Line == nil || !strings.Contains(pr.Line.Text, "secretName") {
		t.Fatalf("produces line: %+v", pr.Line)
	}
}

func TestConsumerFilePathReferences(t *testing.T) {
	f := newFixture(t)
	f.file("apps/billing/all.yaml", consumerEnv)
	g := consumerGraph(t, f)
	v := view{t, g}

	// A match: the configuration names a key of the mounted Secret.
	e := v.wantEdge(domain.EdgeReadsFile, cTLS, cConfig, domain.EdgeEstablished)
	if e.File == nil || e.File.Key != "ca.crt" || e.File.ConfigKey != "app.ini" || e.File.Container != "app" || e.File.MountPath != "/etc/billing/tls" || e.File.Via != "volume" {
		t.Fatalf("file detail: %+v", e.File)
	}
	if e.Line == nil || e.Line.Text != "ca_file = /etc/billing/tls/ca.crt" || e.Line.Line != 43 {
		t.Fatalf("line and excerpt: %+v", e.Line)
	}
	// items: a projected source maps ca.crt → roots/ca.pem
	ca := readsFile(t, g, "/etc/billing/bundle/roots/ca.pem")
	if len(ca) != 1 || ca[0].File.Key != "ca.crt" || ca[0].Status != domain.EdgeEstablished || g.Edges == nil {
		t.Fatalf("items mapping: %+v", ca)
	}
	if n := v.mustNode("core/v1/Secret/shop/billing-ca"); ca[0].From != n.ID {
		t.Fatalf("items mapping resolves to %s", ca[0].From)
	}
	// a projected source without items, holding the key
	if x := readsFile(t, g, "/etc/billing/bundle/limits.json"); len(x) != 1 || x[0].Status != domain.EdgeEstablished || x[0].From != v.mustNode("core/v1/ConfigMap/shop/billing-extra").ID {
		t.Fatalf("projected source without items: %+v", x)
	}
	// No mount: unresolved (missing), never dropped.
	log := readsFile(t, g, "/var/log/billing/app.log")
	if len(log) != 1 || log[0].Status != domain.EdgeMissing || !strings.Contains(log[0].Reason, "under no volume mount") {
		t.Fatalf("unmatched path: %+v", log)
	}
	// Negative control: the path is under another pod's mount (audit's Secret), not billing's.
	audit := readsFile(t, g, "/etc/audit/keys/signing.key")
	if len(audit) != 1 || audit[0].Status != domain.EdgeMissing {
		t.Fatalf("a path under another pod's mount: %+v", audit)
	}
	if keys := v.mustNode("core/v1/Secret/shop/audit-keys"); audit[0].From == keys.ID {
		t.Fatal("billing's path resolved to the audit pod's Secret")
	}
	// A volume that is not a Secret or ConfigMap: unresolved with the volume named.
	if s := readsFile(t, g, "/var/scratch/spool/out"); len(s) != 1 || s[0].Status != domain.EdgeMissing || !strings.Contains(s[0].Reason, "emptyDir") {
		t.Fatalf("emptyDir path: %+v", s)
	}
	// Not file paths: a one-segment route, a URL; its own volume: resolved within itself.
	for _, p := range []string{"/healthz", "/docs/ca", "/etc/billing/conf/other.ini"} {
		if x := readsFile(t, g, p); len(x) != 0 {
			t.Errorf("%s: %+v", p, x)
		}
	}
	// The default build is unchanged: no mount detail, no reads-file edges.
	plain := f.build()
	if plain.Builder != BuilderName {
		t.Fatalf("builder %q", plain.Builder)
	}
	for _, e := range plain.Edges {
		if e.Kind == domain.EdgeReadsFile || e.Mount != nil || e.Line != nil || e.File != nil {
			t.Fatalf("the default graph carries consumer detail: %+v", e)
		}
	}
	b, _ := json.Marshal(plain.Edges)
	if strings.Contains(string(b), `"mount"`) {
		t.Fatal("the default graph's JSON changed")
	}
}

// subPath: a container that mounts one key at a file path reads it there; a
// configuration consumed through env names a path under the same mount.
func TestConsumerSubPathAndEnv(t *testing.T) {
	f := newFixture(t)
	f.file("apps/web/all.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: web, namespace: shop}
spec:
  template:
    spec:
      volumes:
        - name: trust
          secret:
            secretName: web-trust
            items: [{key: bundle.pem, path: pem/bundle.pem}]
      containers:
        - name: web
          env:
            - name: CA_FILE
              valueFrom: {configMapKeyRef: {name: web-env, key: ca}}
          volumeMounts:
            - {name: trust, mountPath: /etc/ssl/trust.pem, subPath: pem/bundle.pem}
---
apiVersion: v1
kind: ConfigMap
metadata: {name: web-env, namespace: shop}
data:
  ca: /etc/ssl/trust.pem
  other: /etc/ssl/unused/x.pem
---
apiVersion: v1
kind: Secret
metadata: {name: web-trust, namespace: shop}
data: {bundle.pem: eA==}
`)
	g := consumerGraph(t, f)
	v := view{t, g}
	e := v.wantEdge(domain.EdgeReadsFile, "core/v1/Secret/shop/web-trust", "core/v1/ConfigMap/shop/web-env", domain.EdgeEstablished)
	if e.File.Key != "bundle.pem" || e.File.Via != "env" || e.Line == nil || e.Line.Text != "ca: /etc/ssl/trust.pem" {
		t.Fatalf("subPath through items, read through env: %+v %+v", e.File, e.Line)
	}
	if x := readsFile(t, g, "/etc/ssl/unused/x.pem"); len(x) != 0 {
		t.Fatalf("a key the container does not read (configMapKeyRef names one key): %+v", x)
	}
}

// A ConfigMap generated by kustomize exists only as generator input: its file
// is read from the environment file the generator names.
func TestConsumerGeneratedConfigMap(t *testing.T) {
	f := newFixture(t)
	f.file("apps/api/kustomization.yaml", `namespace: shop
resources: [deploy.yaml]
configMapGenerator:
  - name: api-proxy
    files: [proxy.conf]
    literals: ["LOG=/var/log/api/proxy.log"]
`)
	f.file("apps/api/proxy.conf", "server {\n  listen 8443 ssl;\n  ssl_client_certificate /etc/api/ca/ca.crt;\n}\n")
	f.file("apps/api/deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: api}
spec:
  template:
    spec:
      volumes:
        - {name: ca, secret: {secretName: api-ca}}
        - {name: proxy, configMap: {name: api-proxy}}
      containers:
        - name: proxy
          volumeMounts:
            - {name: ca, mountPath: /etc/api/ca}
            - {name: proxy, mountPath: /etc/nginx/conf.d}
`)
	g := consumerGraph(t, f)
	v := view{t, g}
	e := v.wantEdge(domain.EdgeReadsFile, "core/v1/Secret/shop/api-ca", "core/v1/ConfigMap/shop/api-proxy", domain.EdgeEstablished)
	var conf string
	for _, a := range f.b.Manifest.Artifacts {
		if a.Path == "env/apps/api/proxy.conf" {
			conf = a.ID
		}
	}
	if e.File.ConfigKey != "proxy.conf" || e.Line == nil || e.Line.Artifact != conf || e.Line.Line != 3 || e.Line.Text != "ssl_client_certificate /etc/api/ca/ca.crt;" {
		t.Fatalf("generated config: %+v %+v", e.File, e.Line)
	}
	if x := readsFile(t, g, "/var/log/api/proxy.log"); len(x) != 1 || x[0].Status != domain.EdgeMissing || x[0].File.ConfigKey != "LOG" {
		t.Fatalf("a literal's path: %+v", x)
	}
}

func TestConsumerGraphIsDeterministic(t *testing.T) {
	f := newFixture(t)
	f.file("apps/billing/all.yaml", consumerEnv)
	a := consumerGraph(t, f)
	b := consumerGraph(t, f)
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	if string(x) != string(y) {
		t.Fatal("consumer graphs differ between builds")
	}
	if a.ID == f.build().ID {
		t.Fatal("the consumer graph shares the default graph's id")
	}
}
