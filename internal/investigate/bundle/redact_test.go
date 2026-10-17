package bundle

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/redaction"
	"github.com/tdavison784/release-intelligence/internal/render"
)

func TestRedactSecretDocumentKeepsKeys(t *testing.T) {
	in := []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: db\ndata:\n  username: YWRtaW4=\n  tls.key: c3VwZXJzZWNyZXQ=\nstringData:\n  note: hunter2\n")
	out, red := redactYAML(in)
	if !red {
		t.Fatal("not redacted")
	}
	s := string(out)
	for _, leak := range []string{"YWRtaW4=", "c3VwZXJzZWNyZXQ=", "hunter2"} {
		if strings.Contains(s, leak) {
			t.Errorf("%q leaked:\n%s", leak, s)
		}
	}
	for _, keep := range []string{"username:", "tls.key:", "note:", "name: db", "<redacted sha256:"} {
		if !strings.Contains(s, keep) {
			t.Errorf("%q missing:\n%s", keep, s)
		}
	}
}

func TestRedactValuesCredentialKeys(t *testing.T) {
	in := []byte("replicas: 2\ndb:\n  password: hunter2\n  host: db.internal\nauth:\n  apiKey: abc123\n  createSecret: true\nimagePullSecrets:\n  - name: regcred\n")
	out, red := redactYAML(in)
	if !red {
		t.Fatal("not redacted")
	}
	s := string(out)
	for _, leak := range []string{"hunter2", "abc123"} {
		if strings.Contains(s, leak) {
			t.Errorf("%q leaked", leak)
		}
	}
	for _, keep := range []string{"replicas: 2", "host: db.internal", "createSecret: true"} {
		if !strings.Contains(s, keep) {
			t.Errorf("%q should stay readable:\n%s", keep, s)
		}
	}
}

func TestRedactNoopIsByteExact(t *testing.T) {
	in := []byte("# comment\nreplicas:   2   # keep\nimage:\n    tag: v1\n---\nkind: ConfigMap\ndata:\n  a: b\n")
	out, red := redactYAML(in)
	if red || string(out) != string(in) {
		t.Fatalf("untouched input must be returned byte for byte (redacted=%v):\n%s", red, out)
	}
}

func TestRedactMultiDocOnlyChangesSecretDocuments(t *testing.T) {
	in := []byte("kind: ConfigMap\ndata:\n  a:   b\n---\napiVersion: v1\nkind: Secret\ndata:\n  k: dg==\n---\nkind: Service\n")
	out, red := redactYAML(in)
	if !red {
		t.Fatal("expected redaction")
	}
	s := string(out)
	if !strings.Contains(s, "  a:   b\n") {
		t.Errorf("ConfigMap document must be verbatim:\n%s", s)
	}
	if strings.Contains(s, "dg==") || !strings.Contains(s, "kind: Service") {
		t.Errorf("bad output:\n%s", s)
	}
}

func TestRedactUnparsableKeptVerbatim(t *testing.T) {
	in := []byte("foo: [unterminated\npassword: x\n")
	out, red := redactYAML(in)
	if red || string(out) != string(in) {
		t.Fatalf("unparsable input is kept verbatim, got redacted=%v", red)
	}
}

func TestRedactedValueIsStableAndOpaque(t *testing.T) {
	a, b := redactedValue("hunter2"), redactedValue("hunter2")
	if a != b || strings.Contains(a, "hunter2") || redactedValue("other") == a {
		t.Fatalf("bad redaction token %q", a)
	}
}

// gadgetDoc is a synthetic custom resource: a credential-named map holding
// an enum setting, free text, a number, a flag, a password-like value and a
// private key.
const gadgetDoc = `apiVersion: example.test/v1
kind: Gadget
metadata:
  name: g
spec:
  replicas: 2
  credentials:
    rotation: Never
    comment: rotate by hand, ask ops
    size: 256
    enabled: true
    pin: "0420"
    password: hunter2
    bundle: |
      -----BEGIN EC PRIVATE KEY-----
      MHcCAQEEIGadgetKeyMaterial
      -----END EC PRIVATE KEY-----
`

const gadgetCRD = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: gadgets.example.test
spec:
  group: example.test
  names: {kind: Gadget, plural: gadgets}
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              credentials:
                type: object
                properties:
                  rotation: {type: string, enum: [Never, Always]}
                  comment: {type: string}
                  pin: {type: string}
                  password: {type: string, enum: [hunter2, letmein]}
`

func TestRedactCredentialMapKeepsOnlyNonStrings(t *testing.T) {
	out, red := redactYAML([]byte(gadgetDoc))
	if !red {
		t.Fatal("not redacted")
	}
	s := string(out)
	// readable: what is provably not a credential
	for _, keep := range []string{"replicas: 2", "size: 256", "enabled: true", "name: g"} {
		if !strings.Contains(s, keep) {
			t.Errorf("%q should stay readable:\n%s", keep, s)
		}
	}
	// withheld: every string below the map (no schema is known at collection)
	for _, leak := range []string{"Never", "rotate by hand", "0420", "hunter2", "PRIVATE KEY", "GadgetKeyMaterial"} {
		if strings.Contains(s, leak) {
			t.Errorf("%q leaked:\n%s", leak, s)
		}
	}
	for _, m := range []string{"Never", "rotate by hand, ask ops", "0420", "hunter2"} {
		if !strings.Contains(s, redactedValue(m)) {
			t.Errorf("no marker for %q:\n%s", m, s)
		}
	}
}

// TestRedactReadBackOnlySchemaEnum runs the whole chain the validators and
// the agent see: the stored (redacted) file, parsed, then read back against
// the bundle's CRD. Only the enum setting comes back.
func TestRedactReadBackOnlySchemaEnum(t *testing.T) {
	stored, _ := redactYAML([]byte(gadgetDoc))
	crds, err := render.ParseObjects([]byte(gadgetCRD))
	if err != nil {
		t.Fatal(err)
	}
	ix := redaction.NewSchemas()
	for _, c := range crds {
		ix.AddCRD(c.Body)
	}
	objs, err := render.ParseObjects(stored)
	if err != nil || len(objs) != 1 {
		t.Fatalf("parse: %v %v", objs, err)
	}
	got := ix.Recover(objs[0].ID.Group, objs[0].ID.Version, objs[0].ID.Kind, objs[0].Body)
	if len(got) != 1 || got[0].Path != "spec.credentials.rotation" || got[0].Value != "Never" {
		t.Fatalf("read back %v, want only spec.credentials.rotation = Never", got)
	}
	cred := objs[0].Body["spec"].(map[string]any)["credentials"].(map[string]any)
	if cred["rotation"] != "Never" {
		t.Errorf("rotation = %v", cred["rotation"])
	}
	for _, k := range []string{"comment", "pin", "password", "bundle"} {
		if v, _ := cred[k].(string); !redaction.IsMarker(v) && !strings.HasPrefix(v, "<sensitive sha256:") {
			t.Errorf("%s was re-exposed: %v", k, cred[k])
		}
	}
}

func TestRedactPrivateKeyAnywhere(t *testing.T) {
	in := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: c\ndata:\n  ca.pem: |\n    -----BEGIN CERTIFICATE-----\n    MIIBpublic\n    -----END CERTIFICATE-----\n  key.pem: |\n    -----BEGIN PRIVATE KEY-----\n    MIIEsecret\n    -----END PRIVATE KEY-----\n")
	out, red := redactYAML(in)
	s := string(out)
	if !red || strings.Contains(s, "MIIEsecret") || strings.Contains(s, "PRIVATE KEY") {
		t.Fatalf("private key kept:\n%s", s)
	}
	if !strings.Contains(s, "MIIBpublic") {
		t.Errorf("a certificate is not a credential:\n%s", s)
	}
}

// TestRedactSecretAndOwnKeyUnchanged pins the v1 values for what v3 does not
// change (Secret data, scalar credentials) and the reference-key exception:
// a credential-named list is walked, its items' names are object names.
func TestRedactSecretAndOwnKeyUnchanged(t *testing.T) {
	in := []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: db\ndata:\n  password: aHVudGVyMg==\n  port: 5432\nstringData:\n  flag: true\n---\ndb:\n  password: hunter2\n  apiToken: 123456\n  createSecret: true\nimagePullSecrets:\n  - name: regcred\n")
	want := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: db\ndata:\n  password: \"" + redactedValue("aHVudGVyMg==") + "\"\n  port: \"" + redactedValue("5432") + "\"\nstringData:\n  flag: \"" + redactedValue("true") + "\"\n" +
		"---\ndb:\n  password: \"" + redactedValue("hunter2") + "\"\n  apiToken: \"" + redactedValue("123456") + "\"\n  createSecret: true\nimagePullSecrets:\n  - name: regcred\n"
	out, _ := redactYAML(in)
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestWithheldFact(t *testing.T) {
	for _, c := range []struct {
		path, value string
		crd         bool
		want        bool
		named       bool // a "name" fact beside it
	}{
		{"replicas", "2", false, false, false},
		{"db.host", `"db.internal"`, false, false, false},
		{"db.password", `"hunter2"`, false, true, false},
		{"db.password", "true", false, false, false}, // a flag, as in the files
		{"spec.secretAccessKey", `"AKIAEXAMPLE"`, false, true, false},
		{"spec.secretName", `"tls"`, false, false, false}, // an object reference
		{"spec.privateKeySecretRef.name", `"ca-key"`, false, false, false},
		{"spec.tokenSecretRef.key", `"token"`, false, false, true},    // {name, key}
		{"spec.credentials.key", `"AKIAEXAMPLE"`, false, true, false}, // a lone key
		{"spec.credentials.rotation", `"Never"`, false, true, false},
		{"spec.credentials.size", "256", false, false, false},
		{"spec.credentials.enabled", "false", false, false, false},
		{"spec.credentials.note", "null", false, false, false},
		{"spec.privateKey.algorithm", `"ECDSA"`, false, true, false},
		{"imagePullSecrets[0].name", `"regcred"`, false, false, false}, // a credential-named list is walked like a map
		{"tokens[0]", `"abc"`, false, true, false},
		{"tokens[0].weight", "3", false, false, false},
		{"tokens[0].owner", `"ops"`, false, true, false},
		{"ca", `"-----BEGIN PRIVATE KEY-----\nMIIE\n-----END PRIVATE KEY-----"`, false, true, false},
		{"spec.x", "not json", false, true, false},
		// a CRD is a schema: only private keys
		{"spec.versions[].schema.openAPIV3Schema.properties.privateKey.properties.rotationPolicy.enum[]", `"Never"`, true, false, false},
		{"spec.versions[].schema.openAPIV3Schema.properties.privateKey.default", `"-----BEGIN PRIVATE KEY-----\nx"`, true, true, false},
	} {
		if got := withheldFact(c.path, c.value, c.crd, c.named); got != c.want {
			t.Errorf("withheldFact(%q, %s, crd=%v, named=%v) = %v, want %v", c.path, c.value, c.crd, c.named, got, c.want)
		}
	}
}

func TestRedactKeepsDocumentSeparators(t *testing.T) {
	in := []byte("---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n---\napiVersion: v1\nkind: Secret\nmetadata:\n  name: b\ndata:\n  k: dg==\n---\napiVersion: example.test/v1\nkind: Gadget\nmetadata:\n  name: c\nspec:\n  token: abc\n")
	out, red := redactYAML(in)
	if !red || strings.Count(string(out), "---\n") != 3 {
		t.Fatalf("separators lost:\n%s", out)
	}
	objs, err := render.ParseObjects(out)
	if err != nil || len(objs) != 3 {
		t.Fatalf("the redacted stream must keep its 3 objects, got %d (%v):\n%s", len(objs), err, out)
	}
}

// The collector applies the same rule as the render model: credentials by
// the union of names, references readable, CRDs are schemas, credential env
// values withheld.
func TestRedactSharedRule(t *testing.T) {
	in := []byte(`apiVersion: example.test/v1
kind: Gadget
metadata: {name: g}
spec:
  secretAccessKey: AKIAEXAMPLESECRET
  passphrase: open-sesame
  secretName: gadget-tls
  privateKeySecretRef: {name: gadget-key}
  tokenSecretRef: {name: gadget-token, key: token}
  credentials: {key: AKIALONEKEYSECRET}
  privateKey: {key: lone-key-material}
  env:
    - {name: DB_PASSWORD, value: hunter2}
    - {name: LOG_LEVEL, value: debug}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: gadgets.example.test}
spec:
  versions:
  - name: v1
    schema:
      openAPIV3Schema:
        properties:
          privateKey:
            description: key settings
            properties:
              rotationPolicy: {type: string, enum: [Never, Always]}
`)
	out, _ := redactYAML(in)
	s := string(out)
	for _, leak := range []string{"AKIAEXAMPLESECRET", "open-sesame", "hunter2", "AKIALONEKEYSECRET", "lone-key-material"} {
		if strings.Contains(s, leak) {
			t.Errorf("%q leaked:\n%s", leak, s)
		}
	}
	for _, keep := range []string{"secretName: gadget-tls", "name: gadget-key", "name: gadget-token", "key: token", "value: debug",
		"description: key settings", "enum: [Never, Always]"} {
		if !strings.Contains(s, keep) {
			t.Errorf("%q should stay readable:\n%s", keep, s)
		}
	}
}

// The facts artifact: a key is a reference key only beside a "name" fact of
// the same mapping (the {name, key} selector shape).
func TestRedactedFactsKeyNeedsNameSibling(t *testing.T) {
	tree := map[string]any{"Resources": []any{map[string]any{"Kind": "Gadget", "Fields": []any{
		map[string]any{"Path": "spec.tokenSecretRef.name", "Element": "spec.tokenSecretRef.name", "Value": `"gadget-token"`},
		map[string]any{"Path": "spec.tokenSecretRef.key", "Element": "spec.tokenSecretRef.key", "Value": `"token"`},
		map[string]any{"Path": "spec.credentials.key", "Element": "spec.credentials.key", "Value": `"AKIALONEKEYSECRET"`},
		map[string]any{"Path": "spec.credentials.region", "Element": "spec.credentials.region", "Value": `"eu-1"`},
	}}}}
	walkFacts(tree, false, nil)
	fields := tree["Resources"].([]any)[0].(map[string]any)["Fields"].([]any)
	val := func(i int) string { return fields[i].(map[string]any)["Value"].(string) }
	if val(0) != `"gadget-token"` || val(1) != `"token"` {
		t.Errorf("selector {name, key} withheld: %s %s", val(0), val(1))
	}
	for i := 2; i < 4; i++ {
		if !strings.HasPrefix(val(i), "<redacted sha256:") {
			t.Errorf("%s kept: %s", fields[i].(map[string]any)["Path"], val(i))
		}
	}
}
