package env

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func evURI(e Environment, id domain.EvidenceID) domain.Evidence {
	for _, x := range e.Evidence {
		if x.ID == id {
			return x
		}
	}
	return domain.Evidence{}
}

const valuesYAML = `# customer overrides
replicaCount: 2
image:
  repository: quay.io/jetstack/cert-manager-controller
  tag: v1.17.0
webhook:
  config: true
  extra:
    - a
    - b
"odd.key": 1
`

const manifestsYAML = `apiVersion: v1
kind: Namespace
metadata:
  name: cert-manager
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: root-ca
spec:
  secretTemplate:
    labels:
      team: plat
  dnsNames:
    - example.com
`

const crdYAML = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: certificates.cert-manager.io
spec:
  group: cert-manager.io
  preserveUnknownFields: false
  names:
    kind: Certificate
    plural: certificates
  versions:
    - name: v1
      served: true
      storage: true
    - name: v1beta1
      served: false
      storage: false
      deprecated: true
      deprecationWarning: use v1
`

func TestLoadValues(t *testing.T) {
	dir := t.TempDir()
	val := write(t, dir, "values.yaml", valuesYAML)
	e, err := Load(Inputs{ValuesFiles: []string{val}})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]ValuesKey{}
	for _, k := range e.ValuesKeys {
		paths[k.Path] = k
	}
	for _, want := range []string{"replicaCount", "image.repository", "image.tag", "webhook.config", "webhook.extra", `["odd.key"]`} {
		if _, ok := paths[want]; !ok {
			t.Errorf("values key %q missing (have %v)", want, keys(e.ValuesKeys, func(k ValuesKey) string { return k.Path }))
		}
	}
	rc := paths["replicaCount"]
	if rc.Value != "2" || rc.Line != 2 {
		t.Errorf("replicaCount = %+v", rc)
	}
	ev := evURI(*e, rc.Evidence[0])
	if ev.URI != val || ev.Locator != "$.replicaCount (L2)" || ev.Excerpt != "replicaCount: 2" || ev.Kind != domain.EvidenceLocalFile {
		t.Errorf("replicaCount evidence = %+v", ev)
	}
	if ev.ContentDigest == "" {
		t.Error("evidence must carry the file digest")
	}
	// list leaf keeps JSON encoding
	if got := paths["webhook.extra"].Value; got != `["a","b"]` {
		t.Errorf("webhook.extra = %q", got)
	}
}

func TestLoadValuesImages(t *testing.T) {
	dir := t.TempDir()
	val := write(t, dir, "values.yaml", `image:
  repository: quay.io/jetstack/cert-manager-controller
  tag: v1.17.0
hub: docker.io/istio/pilot
tag: 1.29.2
direct: ghcr.io/example/other:1.0
noref: just-a-name
`)
	e, err := Load(Inputs{ValuesFiles: []string{val}})
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]ImageUse{}
	for _, im := range e.Images {
		refs[im.Reference] = im
	}
	for _, want := range []string{
		"quay.io/jetstack/cert-manager-controller:v1.17.0",
		"docker.io/istio/pilot:1.29.2",
	} {
		if _, ok := refs[want]; !ok {
			t.Errorf("image %q not extracted (have %v)", want, keys(e.Images, func(i ImageUse) string { return i.Reference }))
		}
	}
	if len(e.Images) != 2 {
		t.Errorf("repository/hub conventions only; got %v", keys(e.Images, func(i ImageUse) string { return i.Reference }))
	}
}

func TestLoadManifestsMultiDoc(t *testing.T) {
	dir := t.TempDir()
	man := write(t, dir, "manifests.yaml", manifestsYAML)
	e, err := Load(Inputs{Manifests: []string{man}})
	if err != nil {
		t.Fatal(err)
	}
	if e.ManifestDocCount != 2 {
		t.Fatalf("docs = %d", e.ManifestDocCount)
	}
	var cert APIVersionUse
	seen := map[string]bool{}
	for _, u := range e.APIVersions {
		seen[u.GroupVersion+"/"+u.Kind] = true
		if u.GroupVersion == "cert-manager.io/v1" && u.Kind == "Certificate" {
			cert = u
		}
	}
	if !seen["v1/Namespace"] || cert.Kind == "" {
		t.Fatalf("apiVersions = %+v", e.APIVersions)
	}
	ev := evURI(*e, cert.Evidence[0])
	if ev.Locator != "L6" { // second document starts after the separator
		t.Errorf("certificate evidence locator = %q", ev.Locator)
	}
	fields := map[string]ManifestField{}
	for _, f := range e.ManifestFields {
		fields[f.Path] = f
	}
	for _, want := range []string{"spec.secretTemplate.labels.team", "spec.dnsNames"} {
		if _, ok := fields[want]; !ok {
			t.Errorf("manifest field %q missing (have %v)", want, keys(e.ManifestFields, func(f ManifestField) string { return f.Path }))
		}
	}
	if f := fields["spec.secretTemplate.labels.team"]; f.APIVersion != "cert-manager.io/v1" || f.Kind != "Certificate" {
		t.Errorf("field carries no document identity: %+v", f)
	}
}

func TestLoadCRDs(t *testing.T) {
	dir := t.TempDir()
	crd := write(t, dir, "crds/certificates.yaml", crdYAML)
	e, err := Load(Inputs{CRDs: []string{filepath.Dir(crd)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.CRDs) != 1 {
		t.Fatalf("crds = %+v", e.CRDs)
	}
	c := e.CRDs[0]
	if c.Name != "certificates.cert-manager.io" || c.Group != "cert-manager.io" || c.Kind != "Certificate" {
		t.Fatalf("crd = %+v", c)
	}
	if !c.HasPreserveUnknownFields || c.PreserveUnknownFields {
		t.Errorf("preserveUnknownFields = %v (stated %v)", c.PreserveUnknownFields, c.HasPreserveUnknownFields)
	}
	if len(c.Versions) != 2 || !c.Versions[0].Served || c.Versions[1].Served || !c.Versions[1].Deprecated ||
		c.Versions[1].DeprecationWarning != "use v1" {
		t.Errorf("versions = %+v", c.Versions)
	}
	// the CRD's own served versions feed the apiVersion inventory
	var found bool
	for _, u := range e.APIVersions {
		if u.GroupVersion == "cert-manager.io/v1beta1" {
			found = true
		}
	}
	if !found {
		t.Errorf("CRD versions must appear in the apiVersion inventory: %+v", e.APIVersions)
	}
}

func TestLoadManifestImagesAndList(t *testing.T) {
	dir := t.TempDir()
	man := write(t, dir, "deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: cm
spec:
  template:
    spec:
      containers:
        - name: cm
          image: quay.io/jetstack/cert-manager-controller:v1.17.0
      initContainers:
        - name: init
          image: registry.example/k8s/init:2.0@sha256:1234567890123456789012345678901234567890123456789012345678901234
`)
	e, err := Load(Inputs{Manifests: []string{man}, Images: []string{"docker.io/library/busybox:1.36"}})
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]ImageUse{}
	for _, im := range e.Images {
		refs[im.Reference] = im
	}
	if _, ok := refs["quay.io/jetstack/cert-manager-controller:v1.17.0"]; !ok {
		t.Errorf("container image missing: %v", keys(e.Images, func(i ImageUse) string { return i.Reference }))
	}
	init, ok := refs["registry.example/k8s/init:2.0@sha256:1234567890123456789012345678901234567890123456789012345678901234"]
	if !ok || init.Digest == "" {
		t.Errorf("init container image = %+v", init)
	}
	bb, ok := refs["docker.io/library/busybox:1.36"]
	if !ok || bb.Source != "list" {
		t.Errorf("list image = %+v", bb)
	}
	if bb.Evidence[0] == "" || evURI(*e, bb.Evidence[0]).Kind != domain.EvidenceInput {
		t.Error("list images must cite input evidence")
	}
}

func TestLoadKubernetesFlag(t *testing.T) {
	e, err := Load(Inputs{KubernetesVersion: "1.31"})
	if err != nil {
		t.Fatal(err)
	}
	if e.Kubernetes == nil || e.Kubernetes.Version != "1.31" {
		t.Fatalf("kubernetes = %+v", e.Kubernetes)
	}
	ev := evURI(*e, e.Kubernetes.Evidence[0])
	if ev.Kind != domain.EvidenceInput || ev.URI != "flag:--kubernetes" || ev.Excerpt != "1.31" {
		t.Errorf("kubernetes evidence = %+v", ev)
	}
}

func TestLoadWarningsAndErrors(t *testing.T) {
	dir := t.TempDir()
	junk := write(t, dir, "broken.yaml", "a: [unclosed\n")
	e, err := Load(Inputs{Manifests: []string{junk}})
	if err != nil {
		t.Fatalf("per-file problems are warnings, not errors: %v", err)
	}
	if len(e.Warnings) == 0 || !strings.Contains(e.Warnings[0], "not parsed") {
		t.Errorf("warnings = %v", e.Warnings)
	}
	if _, err := Load(Inputs{Manifests: []string{filepath.Join(dir, "nope")}}); err == nil {
		t.Error("a missing input path is an error")
	}
	// a non-CRD document inside a --crds input is a warning, not silently kept
	plain := write(t, dir, "plain/x.yaml", manifestsYAML)
	e2, err := Load(Inputs{CRDs: []string{plain}})
	if err != nil {
		t.Fatal(err)
	}
	if len(e2.CRDs) != 0 || len(e2.Warnings) == 0 {
		t.Errorf("crds = %d, warnings = %v", len(e2.CRDs), e2.Warnings)
	}
}

func TestLoadDeterministic(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.yaml", valuesYAML)
	b := write(t, dir, "b.yaml", manifestsYAML)
	c := write(t, dir, "c.yaml", crdYAML)
	in := Inputs{KubernetesVersion: "1.31.5", ValuesFiles: []string{a}, Manifests: []string{b}, CRDs: []string{c},
		Images: []string{"docker.io/library/busybox:1.36"}}
	first, err := Load(in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Load(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("Load must be deterministic")
	}
	if in.Empty() {
		t.Fatal("inputs with content are not empty")
	}
	if !(&Inputs{}).Empty() {
		t.Fatal("zero inputs must be Empty")
	}
}

func TestSplitKeyPath(t *testing.T) {
	for path, want := range map[string][]string{
		"a":              {"a"},
		"a.b.c":          {"a", "b", "c"},
		`a["x.y"].c`:     {"a", "x.y", "c"},
		`["only.key"]`:   {"only.key"},
		"spec.foo[].bar": {"spec", "foo[]", "bar"},
	} {
		if got := SplitKeyPath(path); !reflect.DeepEqual(got, want) {
			t.Errorf("SplitKeyPath(%q) = %v, want %v", path, got, want)
		}
	}
}

// keys lists a path-ish field of any fact slice, for failure messages.
func keys[T any, K ~string](xs []T, get func(T) K) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = string(get(x))
	}
	return out
}
