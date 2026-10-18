package relations

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Object references below credential-named keys stay readable through the
// render model (redaction rule c/d, reference-key exception), so the graph
// still resolves them; the credential next to them does not survive.
func TestReferencesBelowCredentialKeysResolve(t *testing.T) {
	const acct = `apiVersion: signing.gizmo.example.io/v1
kind: Account
metadata: {name: acme, namespace: apps}
spec:
  secretName: acme-tls
  privateKeySecretRef: {name: acme-key}
  credentials: {secretAccessKey: AKIAEXAMPLESECRET, accessKeyID: AKIDEXAMPLE}
---
apiVersion: v1
kind: Secret
metadata: {name: acme-key, namespace: apps}
---
apiVersion: v1
kind: Secret
metadata: {name: acme-tls, namespace: apps}
`
	f := newFixture(t)
	f.pair("argocd:apps/acme.yaml", acct, acct)
	rels := WithRelationships("test",
		catalog.Relationship{ID: "account-key", Kind: catalog.RelReferences,
			From: catalog.ObjectType{Group: "signing.gizmo.example.io", Kind: "Account"}, To: catalog.ObjectType{Kind: "Secret"}, Path: "spec.privateKeySecretRef.name"},
		catalog.Relationship{ID: "account-tls", Kind: catalog.RelReferences,
			From: catalog.ObjectType{Group: "signing.gizmo.example.io", Kind: "Account"}, To: catalog.ObjectType{Kind: "Secret"}, Path: "spec.secretName"})
	v := f.view(SideTo, rels)
	const acctKey = "signing.gizmo.example.io/v1/Account/apps/acme"
	wantRes(t, ref(t, v, acctKey, "spec.privateKeySecretRef.name"), RefResolved, "")
	wantRes(t, ref(t, v, acctKey, "spec.secretName"), RefResolved, "")
	g := f.build(rels)
	view{t, g}.wantEdge(domain.EdgeReferences, "core/v1/Secret/apps/acme-key", acctKey, domain.EdgeEstablished)
	view{t, g}.wantEdge(domain.EdgeReferences, "core/v1/Secret/apps/acme-tls", acctKey, domain.EdgeEstablished)
	for _, o := range v.Objects {
		if o.Key == acctKey {
			creds := o.Body["spec"].(map[string]any)["credentials"].(map[string]any)
			for k, val := range creds {
				if s, _ := val.(string); !strings.HasPrefix(s, "<sensitive sha256:") {
					t.Errorf("credentials.%s survived the render model: %v", k, val)
				}
			}
			return
		}
	}
	t.Fatal("account object not in the view")
}
