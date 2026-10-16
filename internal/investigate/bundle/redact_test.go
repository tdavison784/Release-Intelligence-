package bundle

import (
	"strings"
	"testing"
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

func TestCredentialLastSegment(t *testing.T) {
	for p, want := range map[string]string{"db.password": "password", "a.b[0].token": "token", "x": "x"} {
		if got := lastSegment(p); got != want {
			t.Errorf("lastSegment(%q) = %q, want %q", p, got, want)
		}
	}
}
