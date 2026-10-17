package render

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/redaction"
)

// The render model applies the shared redaction rule (internal/redaction).

func bodyOf(t *testing.T, doc string) map[string]any {
	t.Helper()
	objs, err := ParseObjects([]byte(doc))
	if err != nil || len(objs) != 1 {
		t.Fatalf("parse: %v %v", objs, err)
	}
	return objs[0].Body
}

func at(t *testing.T, body map[string]any, path ...any) any {
	t.Helper()
	var v any = body
	for _, p := range path {
		switch k := p.(type) {
		case string:
			v = v.(map[string]any)[k]
		case int:
			v = v.([]any)[k]
		}
	}
	return v
}

func sensitive(v any) bool { s, ok := v.(string); return ok && strings.HasPrefix(s, sensitivePrefix) }

func TestRenderRedactsCredentialsKeepsReferences(t *testing.T) {
	b := bodyOf(t, `apiVersion: example.test/v1
kind: Gadget
metadata: {name: g, namespace: lab}
spec:
  secretAccessKey: AKIAEXAMPLESECRET
  accessKey: AKIDEXAMPLE
  clientSecret: s3cr3t
  passphrase: open-sesame
  bearer: tok
  existingSecret: db-creds
  secretName: gadget-tls
  privateKeySecretRef: {name: gadget-key}
  tokenSecretRef: {name: gadget-token, key: token}
  privateKey: {key: lone-key-material}
  credentials:
    key: AKIALONEKEYSECRET
    rotation: Never
    comment: rotate by hand
    size: 256
    enabled: true
    namespace: lab
    caBundleName: ca
  imagePullSecrets: [{name: regcred}]
  tokens: [abc, def]
  template:
    spec:
      containers:
        - name: c
          env:
            - {name: DB_PASSWORD, value: hunter2}
            - {name: LOG_LEVEL, value: debug}
            - name: API_TOKEN
              valueFrom: {secretKeyRef: {name: api, key: token}}
  note: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEsecret
    -----END RSA PRIVATE KEY-----
`)
	spec := at(t, b, "spec").(map[string]any)
	for _, k := range []string{"secretAccessKey", "accessKey", "clientSecret", "passphrase", "bearer", "existingSecret", "note"} {
		if !sensitive(spec[k]) {
			t.Errorf("spec.%s survived: %v", k, spec[k])
		}
	}
	if !sensitive(at(t, b, "spec", "privateKey", "key")) {
		t.Errorf("a lone key below privateKey survived")
	}
	for _, k := range []string{"rotation", "comment", "key"} {
		if !sensitive(at(t, b, "spec", "credentials", k)) {
			t.Errorf("credentials.%s survived", k)
		}
	}
	for _, p := range [][]any{{"tokens", 0}, {"tokens", 1}, {"template", "spec", "containers", 0, "env", 0, "value"}} {
		if !sensitive(at(t, b, append([]any{"spec"}, p...)...)) {
			t.Errorf("spec.%v survived", p)
		}
	}
	for want, p := range map[any][]any{
		"gadget-tls":   {"secretName"},
		"gadget-key":   {"privateKeySecretRef", "name"},
		"gadget-token": {"tokenSecretRef", "name"},
		"token":        {"tokenSecretRef", "key"},
		256:            {"credentials", "size"},
		true:           {"credentials", "enabled"},
		"lab":          {"credentials", "namespace"},
		"ca":           {"credentials", "caBundleName"},
		"regcred":      {"imagePullSecrets", 0, "name"},
		"debug":        {"template", "spec", "containers", 0, "env", 1, "value"},
		"api":          {"template", "spec", "containers", 0, "env", 2, "valueFrom", "secretKeyRef", "name"},
	} {
		if got := at(t, b, append([]any{"spec"}, p...)...); got != want {
			t.Errorf("spec.%v = %v, want %v readable", p, got, want)
		}
	}
}

// A CRD is a schema: credential-named property names do not withhold its
// descriptions, types or enums (the read-back needs them); a private key
// still is.
func TestRenderCRDKeepsSchemaUnderCredentialProperties(t *testing.T) {
	b := bodyOf(t, `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: gadgets.example.test}
spec:
  group: example.test
  names: {kind: Gadget}
  versions:
  - name: v1
    schema:
      openAPIV3Schema:
        properties:
          spec:
            properties:
              privateKey:
                description: the key settings
                properties:
                  rotationPolicy: {type: string, enum: [Never, Always], default: Never}
                  example: "-----BEGIN PRIVATE KEY-----\nMIIE\n-----END PRIVATE KEY-----"
`)
	pk := at(t, b, "spec", "versions", 0, "schema", "openAPIV3Schema", "properties", "spec", "properties", "privateKey").(map[string]any)
	rp := at(t, pk, "properties", "rotationPolicy").(map[string]any)
	if pk["description"] != "the key settings" || rp["type"] != "string" || at(t, rp, "enum", 0) != "Never" || rp["default"] != "Never" {
		t.Fatalf("CRD schema withheld: %#v", pk)
	}
	if !sensitive(at(t, pk, "properties", "example")) {
		t.Errorf("a private key in a CRD survived")
	}
	ix := redaction.NewSchemas()
	ix.AddCRD(b)
	if ix.Len() != 1 {
		t.Fatalf("CRD not indexable after the render model")
	}
}

// A collected file's markers (either form) pass through unchanged, so the
// read-back can match them.
func TestRenderKeepsExistingMarkers(t *testing.T) {
	m := redaction.Marker("Never")
	b := bodyOf(t, "apiVersion: example.test/v1\nkind: Gadget\nmetadata: {name: g}\nspec:\n  credentials:\n    rotation: \""+m+"\"\n  password: \""+m+"\"\n")
	if at(t, b, "spec", "credentials", "rotation") != m || at(t, b, "spec", "password") != m {
		t.Fatalf("markers re-redacted: %#v", b["spec"])
	}
}
