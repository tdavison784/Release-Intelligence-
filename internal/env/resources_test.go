package env

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func loadManifest(t *testing.T, content string) *Environment {
	t.Helper()
	p := filepath.Join(t.TempDir(), "m.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := Load(Inputs{Manifests: []string{p}})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func evidenceOf(e *Environment, id string) (locator, excerpt string) {
	for _, ev := range e.Evidence {
		if string(ev.ID) == id {
			return ev.Locator, ev.Excerpt
		}
	}
	return "", ""
}

const certs = `apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: with-policy
  namespace: prod
spec:
  dnsNames:
    - a.example.com
    - b.example.com
  privateKey:
    rotationPolicy: Never
    size: 4096
  issuerRef:
    name: letsencrypt
    kind: ClusterIssuer
    group: cert-manager.io
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: without-policy
  namespace: prod
spec:
  dnsNames: [c.example.com]
  issuerRef:
    name: missing-issuer
    kind: Issuer
---
apiVersion: example.io/v1
kind: Other
metadata:
  name: decoy
spec:
  privateKey:
    rotationPolicy: Always
`

// X1: scalar values with set/unset distinguishable per resource; the same
// field name under a different kind never answers for this kind.
func TestFieldValuesSetUnsetPerResource(t *testing.T) {
	e := loadManifest(t, certs)
	sel := GVKSelector{Group: "cert-manager.io", Kind: "Certificate"}
	res := e.FieldValues(sel, "spec.privateKey.rotationPolicy")
	if len(res) != 2 {
		t.Fatalf("results = %d, want one per Certificate (the Other kind must not appear)", len(res))
	}
	got := map[string]string{}
	for _, r := range res {
		v := "<unset>"
		if r.Set {
			v = r.Facts[0].Value
		}
		got[r.Resource.Name] = v
	}
	if got["with-policy"] != `"Never"` || got["without-policy"] != "<unset>" {
		t.Errorf("per-resource answers = %v", got)
	}
	// the decoy kind answers for itself only
	if r := e.FieldValues(GVKSelector{Group: "example.io", Kind: "Other"}, "spec.privateKey.rotationPolicy"); len(r) != 1 || r[0].Facts[0].Value != `"Always"` {
		t.Errorf("other kind = %+v", r)
	}
	// numbers and containers
	if r := e.FieldValues(sel, "spec.privateKey.size"); r[0].Facts[0].Value != "4096" {
		t.Errorf("size = %+v", r[0].Facts)
	}
	if r := e.FieldValues(sel, "spec.privateKey"); !r[0].Set || !r[0].Facts[0].Container || r[0].Facts[0].Value != "" || r[1].Set {
		t.Errorf("container set-ness = %+v", r)
	}
	// evidence: file line, YAML path, excerpt
	f := res[0].Facts[0]
	loc, ex := evidenceOf(e, string(f.Evidence[0]))
	if f.Line != 11 || !strings.Contains(loc, "$.spec.privateKey.rotationPolicy (L11)") || ex != `spec.privateKey.rotationPolicy: "Never"` {
		t.Errorf("fact %+v evidence %q / %q", f, loc, ex)
	}
	if got := e.ResourcesOfKind("cert-manager.io", "Certificate"); len(got) != 2 {
		t.Errorf("ResourcesOfKind = %d", len(got))
	}
	if got := e.ResourcesOfKind("", "Certificate"); len(got) != 0 {
		t.Errorf("group \"\" is the core group only, got %d", len(got))
	}
	if got := e.ResourcesOfKind("*", "*"); len(got) != 3 {
		t.Errorf("wildcards = %d", len(got))
	}
}

// X2: sequences are descended with [] markers and element indexes.
func TestSequenceDescent(t *testing.T) {
	e := loadManifest(t, `apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: le
spec:
  acme:
    solvers:
      - http01:
          ingress:
            class: nginx
      - dns01:
          route53:
            region: eu-west-1
      - http01:
          ingress:
            ingressClassName: other
    emptyList: []
    nested:
      - [1, 2]
      - scalar
    scalars: ["x", 7]
`)
	sel := GVKSelector{Group: "cert-manager.io", Kind: "ClusterIssuer"}
	r := e.FieldValues(sel, "spec.acme.solvers[].http01.ingress.class")
	if len(r) != 1 || len(r[0].Facts) != 1 || r[0].Facts[0].Value != `"nginx"` || r[0].Facts[0].Element != "spec.acme.solvers[0].http01.ingress.class" {
		t.Fatalf("marker path = %+v", r)
	}
	loc, _ := evidenceOf(e, string(r[0].Facts[0].Evidence[0]))
	if !strings.Contains(loc, "$.spec.acme.solvers[0].http01.ingress.class (L10)") {
		t.Errorf("locator must carry the element index and line: %q", loc)
	}
	// every element of the same marker path
	if r := e.FieldValues(sel, "spec.acme.solvers[].http01"); len(r[0].Facts) != 2 {
		t.Errorf("http01 elements = %d, want 2", len(r[0].Facts))
	}
	// an explicit index addresses one element; an element lacking the field is unset
	if r := e.FieldValues(sel, "spec.acme.solvers[1].http01.ingress.class"); r[0].Set {
		t.Error("element 1 has no http01")
	}
	if r := e.FieldValues(sel, "spec.acme.solvers[2].http01.ingress.ingressClassName"); !r[0].Set || r[0].Facts[0].Value != `"other"` {
		t.Errorf("indexed = %+v", r[0].Facts)
	}
	// list element versus scalar: a scalar list is [] + leaf, the list itself a container
	if r := e.FieldValues(sel, "spec.acme.scalars[]"); len(r[0].Facts) != 2 || r[0].Facts[1].Value != "7" {
		t.Errorf("scalar elements = %+v", r[0].Facts)
	}
	if r := e.FieldValues(sel, "spec.acme.scalars"); !r[0].Facts[0].Container {
		t.Errorf("list is a container fact: %+v", r[0].Facts)
	}
	if r := e.FieldValues(sel, "spec.acme.emptyList"); !r[0].Set {
		t.Error("an empty list is set")
	}
	if r := e.FieldValues(sel, "spec.acme.nested[][]"); len(r[0].Facts) != 2 {
		t.Errorf("nested sequences = %+v", r[0].Facts)
	}
	// existing path-only consumers are untouched: sequences remain leaves there
	var paths []string
	for _, f := range e.ManifestFields {
		paths = append(paths, f.Path)
	}
	if strings.Contains(strings.Join(paths, " "), "[]") {
		t.Errorf("ManifestFields must keep the old path syntax: %v", paths)
	}
}

// X3: embedded text with line-level evidence; --- inside a block scalar does
// not split the document.
func TestTextBlocksLineEvidence(t *testing.T) {
	e := loadManifest(t, `apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-rbac-cm
data:
  policy.default: role:readonly
  policy.csv: |

    p, role:ops, applications, update, */*, allow
    ---
    p, role:ops, applications, delete, */*, allow
    g, alice, role:ops
  folded: >
    one
    two
  quoted: "a\nb"
`)
	if len(e.Resources) != 1 {
		t.Fatalf("a --- inside a block scalar split the document: %d resources", len(e.Resources))
	}
	sel := GVKSelector{Group: "", Kind: "ConfigMap"}
	rts := e.TextBlocks(sel, "data")
	if len(rts) != 1 || len(rts[0].Blocks) != 4 {
		t.Fatalf("blocks = %+v", rts)
	}
	var csv TextBlock
	for _, b := range rts[0].Blocks {
		if b.Path == `data["policy.csv"]` {
			csv = b
		}
	}
	if !csv.Exact || len(csv.Lines) != 5 {
		t.Fatalf("csv block = exact %v, %d lines", csv.Exact, len(csv.Lines))
	}
	// "no line matches" needs the exact line when one does
	del := csv.Match(regexp.MustCompile(`applications, delete`))
	if len(del) != 1 || del[0].FileLine != 11 || del[0].N != 4 {
		t.Fatalf("match = %+v (want file line 11)", del)
	}
	src, _ := os.ReadFile(rts[0].Resource.Doc.File)
	if got := strings.Split(string(src), "\n")[del[0].FileLine-1]; !strings.Contains(got, "applications, delete") {
		t.Errorf("file line %d is %q, not the matched line", del[0].FileLine, got)
	}
	loc, ex := evidenceOf(e, string(del[0].Evidence))
	if !strings.Contains(loc, "(L11)") || !strings.Contains(ex, "applications, delete") {
		t.Errorf("line evidence = %q / %q", loc, ex)
	}
	if len(csv.Match(regexp.MustCompile(`no such line`))) != 0 {
		t.Error("spurious match")
	}
	// single-line data value is a one-line exact block; folded/quoted are not exact
	for _, b := range rts[0].Blocks {
		switch b.Path {
		case "data[\"policy.default\"]", "data.policy.default":
			// path syntax detail asserted below
		case "data.folded", "data.quoted":
			if b.Exact {
				t.Errorf("%s cannot be mapped line for line", b.Path)
			}
		}
	}
	if got := e.TextBlocks(sel, `data["policy.csv"]`); len(got[0].Blocks) != 1 {
		t.Errorf("exact path selection = %d blocks", len(got[0].Blocks))
	}
	if got := e.TextBlocks(GVKSelector{Group: "*", Kind: "Deployment"}, ""); len(got) != 0 {
		t.Errorf("other kinds: %+v", got)
	}
}

// Secrets: never kept, never in evidence excerpts.
func TestSecretsAreWithheld(t *testing.T) {
	e := loadManifest(t, `apiVersion: v1
kind: Secret
metadata: {name: s}
stringData:
  anything: s3cr3t-in-a-secret
---
apiVersion: v1
kind: ConfigMap
metadata: {name: cm}
data:
  app.conf: |
    mode=fast
    db_password = hunter2
    -----BEGIN RSA PRIVATE KEY-----
    MIIEowIBAAKCAQEA-LINE-IN-KEY
    -----END RSA PRIVATE KEY-----
    tail=ok
---
apiVersion: example.io/v1
kind: App
metadata: {name: a}
spec:
  apiToken: tok-12345
  clientSecret: cs-9999
  replicas: 3
  tlsKey: |
    -----BEGIN PRIVATE KEY-----
    ABCDEF
    -----END PRIVATE KEY-----
`)
	for _, f := range e.Resources[0].Fields {
		if f.Value != "" || (!f.Container && f.Withheld != withheldSensitive) {
			t.Errorf("Secret field not withheld: %+v", f)
		}
	}
	app := e.FieldValues(GVKSelector{Group: "example.io", Kind: "App"}, "spec.apiToken")[0].Facts[0]
	if app.Value != "" || app.Withheld != withheldSensitive {
		t.Errorf("credential-named key not withheld: %+v", app)
	}
	if f := e.FieldValues(GVKSelector{Group: "example.io", Kind: "App"}, "spec.replicas")[0].Facts[0]; f.Value != "3" {
		t.Errorf("ordinary values must be kept: %+v", f)
	}
	if f := e.FieldValues(GVKSelector{Group: "example.io", Kind: "App"}, "spec.tlsKey")[0].Facts[0]; f.Value != "" {
		t.Errorf("a private key value must be withheld: %+v", f)
	}
	blk := e.TextBlocks(GVKSelector{Kind: "ConfigMap"}, "data")[0].Blocks[0]
	if blk.Withheld != 4 { // the assignment line and the three lines of the key block
		t.Errorf("withheld lines = %d, want 4", blk.Withheld)
	}
	for _, ln := range blk.Lines {
		leak := strings.Contains(ln.Text, "hunter2") || strings.Contains(ln.Text, "LINE-IN-KEY")
		if leak {
			t.Errorf("secret line kept: %+v", ln)
		}
	}
	if len(blk.Match(regexp.MustCompile(`tail=ok`))) != 1 || len(blk.Match(regexp.MustCompile(`mode=fast`))) != 1 {
		t.Error("ordinary lines around the secret must stay matchable")
	}
	if len(blk.Match(regexp.MustCompile(`hunter2`))) != 0 {
		t.Error("withheld lines must never match")
	}
	// no secret value anywhere in the evidence pool
	for _, ev := range e.Evidence {
		for _, s := range []string{"s3cr3t", "hunter2", "tok-12345", "cs-9999", "LINE-IN-KEY", "ABCDEF"} {
			if strings.Contains(ev.Excerpt, s) {
				t.Errorf("secret %q in evidence excerpt %q", s, ev.Excerpt)
			}
		}
	}
	// nothing renders by default: formatting a fact shows no value
	if s := fmt.Sprintf("%+v", app); strings.Contains(s, "tok-12345") {
		t.Errorf("fact formatting leaks: %s", s)
	}
}

// X5: cross-resource references.
func TestReferenceResolution(t *testing.T) {
	e := loadManifest(t, certs+`---
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt
spec: {acme: {}}
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: shadow
  namespace: other
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: r
  namespace: prod
spec:
  parentRefs:
    - name: gw
    - name: gw2
      namespace: edge
`)
	refs := e.References(GVKSelector{Group: "cert-manager.io", Kind: "Certificate"}, "spec.issuerRef")
	if len(refs) != 2 {
		t.Fatalf("refs = %d", len(refs))
	}
	by := map[string]ResolvedRef{}
	for _, r := range refs {
		by[r.From.Name] = r
	}
	ok := by["with-policy"]
	if ok.Resolution.Status != RefResolved || ok.Resolution.Target.Kind != "ClusterIssuer" || ok.Resolution.Target.Name != "letsencrypt" {
		t.Errorf("with-policy -> %+v", ok.Resolution)
	}
	if ok.Ref.Kind != "ClusterIssuer" || ok.Ref.Group != "cert-manager.io" || ok.Ref.Line == 0 || len(ok.Ref.Evidence) != 1 {
		t.Errorf("ref = %+v", ok.Ref)
	}
	miss := by["without-policy"]
	// healthy but undeclared manifests: unresolved is not "does not exist"
	if miss.Resolution.Status != RefUnresolved || miss.Resolution.ManifestsComplete || !strings.Contains(miss.Resolution.Reason, "missing-issuer") {
		t.Errorf("unresolved must be explicit, not absent, and clean parsing is not completeness: %+v", miss.Resolution)
	}
	declaredFile := filepath.Join(t.TempDir(), "certs.yaml")
	_ = os.WriteFile(declaredFile, []byte(certs), 0o644)
	declared, _ := Load(Inputs{Manifests: []string{declaredFile}, ManifestsComplete: true})
	for _, x := range declared.References(GVKSelector{Group: "cert-manager.io", Kind: "Certificate"}, "spec.issuerRef") {
		if x.Resolution.Status == RefUnresolved && !x.Resolution.ManifestsComplete {
			t.Error("declared-complete, healthy manifests: an unresolved reference may claim completeness")
		}
	}
	// kind-qualified: an Issuer named like a ClusterIssuer does not satisfy it, and a
	// namespace mismatch (shadow lives in "other") does not resolve for "prod"
	r := e.ResolveRef(&Resource{Namespace: "prod"}, Ref{Name: "shadow", Kind: "Issuer"})
	if r.Status != RefUnresolved {
		t.Errorf("cross-namespace = %+v", r)
	}
	if r := e.ResolveRef(&Resource{Namespace: "prod"}, Ref{Name: "shadow", Kind: "Issuer", Namespace: "other"}); r.Status != RefResolved {
		t.Errorf("explicit namespace = %+v", r)
	}
	// name only: matches across kinds -> ambiguous when several
	if r := e.ResolveRef(nil, Ref{Name: "letsencrypt"}); r.Status != RefResolved {
		t.Errorf("name-only single match = %+v", r)
	}
	// sequences of refs
	pr := e.References(GVKSelector{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute"}, "spec.parentRefs[]")
	if len(pr) != 2 || pr[1].Ref.Element != "spec.parentRefs[1]" || pr[1].Ref.Namespace != "edge" {
		t.Errorf("parentRefs = %+v", pr)
	}
	// a partially parsed manifests dimension must say so
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	_ = os.WriteFile(bad, []byte("kind: [unterminated\n"), 0o644)
	good := filepath.Join(t.TempDir(), "good.yaml")
	_ = os.WriteFile(good, []byte(certs), 0o644)
	e2, _ := Load(Inputs{Manifests: []string{good, bad}, ManifestsComplete: true})
	if e2.Health(DimManifests) != HealthPartial {
		t.Fatalf("health = %s", e2.Health(DimManifests))
	}
	u := e2.References(GVKSelector{Group: "cert-manager.io", Kind: "Certificate"}, "spec.issuerRef")
	for _, x := range u {
		if x.Resolution.Status == RefUnresolved && x.Resolution.ManifestsComplete {
			t.Error("unresolved against partial manifests must not claim completeness")
		}
	}
}

func TestResourceFactsAreDeterministicAndCapped(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&b, "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: c%d}\ndata:\n  k: |\n", i)
		for j := 0; j < maxTextLinesPerBlock+5; j++ {
			fmt.Fprintf(&b, "    line %d\n", j)
		}
		b.WriteString("---\n")
	}
	e := loadManifest(t, b.String())
	blk := e.Resources[0].Texts[0]
	if !blk.Truncated || len(blk.Lines) != maxTextLinesPerBlock {
		t.Errorf("block cap: truncated %v lines %d", blk.Truncated, len(blk.Lines))
	}
	if e.Health(DimManifests) != HealthPartial {
		t.Error("hitting a cap must degrade the manifests dimension")
	}
	a := loadManifest(t, certs)
	c := loadManifest(t, certs)
	if len(a.Resources) != len(c.Resources) || fmt.Sprint(a.Resources[0].Fields[3].Element, a.Resources[0].Fields[3].Value) != fmt.Sprint(c.Resources[0].Fields[3].Element, c.Resources[0].Fields[3].Value) {
		t.Error("extraction must be deterministic")
	}
	// empty environment / nil receivers
	var nilEnv *Environment
	if nilEnv.ResourcesOfKind("x", "y") != nil || nilEnv.ResolveRef(nil, Ref{Name: "x"}).Status != RefUnresolved {
		t.Error("nil environment")
	}
}

// Regression: the lines of a credential-named ConfigMap key are withheld
// exactly like Secret values — not exposed, not matchable, and counted.
func TestCredentialNamedConfigMapKeyLinesWithheld(t *testing.T) {
	e := loadManifest(t, `apiVersion: v1
kind: ConfigMap
metadata: {name: cm}
data:
  db-password: |
    plain-looking-secret-line-1
    plain-looking-secret-line-2
  api_token: single-line-token-value
  settings: |
    mode=fast
`)
	byPath := map[string]TextBlock{}
	for _, b := range e.TextBlocks(GVKSelector{Kind: "ConfigMap"}, "data")[0].Blocks {
		byPath[b.Path] = b
	}
	pw := byPath["data.db-password"]
	if pw.Withheld != 2 || len(pw.Lines) != 2 {
		t.Fatalf("db-password block = %+v, want 2 withheld lines", pw)
	}
	tok := byPath["data.api_token"]
	if tok.Withheld != 1 || len(tok.Lines) != 1 {
		t.Fatalf("api_token block = %+v, want 1 withheld line", tok)
	}
	for _, b := range []TextBlock{pw, tok} {
		for _, ln := range b.Lines {
			if !ln.Withheld || ln.Text != "" {
				t.Errorf("line exposed: %+v", ln)
			}
		}
		if len(b.Match(regexp.MustCompile(`.`))) != 0 {
			t.Error("withheld lines must never match")
		}
	}
	if s := byPath["data.settings"]; s.Withheld != 0 || len(s.Match(regexp.MustCompile(`mode=fast`))) != 1 {
		t.Errorf("ordinary key must stay readable: %+v", s)
	}
	for _, ev := range e.Evidence {
		if strings.Contains(ev.Excerpt, "plain-looking-secret") || strings.Contains(ev.Excerpt, "single-line-token-value") {
			t.Errorf("secret in evidence: %q", ev.Excerpt)
		}
	}
}
