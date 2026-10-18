package runtime

// The fake gizmo controller also issues Badges: real x509 certificates. A
// Badge with spec.signer: true is a CA; spec.issuer names the Badge that
// signs it ("" = self-signed). Re-issuing a Badge keeps its key at v1; at v2
// it generates a new key unless spec.keyPolicy: Keep — the same kind of
// compiled-in default change as Widgets, on certificates a consumer verifies.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
)

type badgePKI struct {
	key  *ecdsa.PrivateKey
	cert *x509.Certificate
	der  []byte
}

func pemOf(typ string, b []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: b}))
}

func badgeSecretID(ns, name string) string {
	return ObjectRef{Version: "v1", Kind: "Secret", Namespace: ns, Name: name}.String()
}

// issueBadge (re-)issues a Badge's Secret; rotate generates a new key.
func (c *fakeCluster) issueBadge(ns string, spec map[string]any, rotate bool) bool {
	if c.pki == nil {
		c.pki = map[string]*badgePKI{}
	}
	secret, _ := spec["secretName"].(string)
	sid := badgeSecretID(ns, secret)
	var parent *badgePKI
	if iss, _ := spec["issuer"].(string); iss != "" {
		io := c.objs[ObjectRef{Group: "gizmo.io", Version: "v1", Kind: "Badge", Namespace: ns, Name: iss}.String()]
		if io == nil {
			return false
		}
		is, _ := io["spec"].(map[string]any)
		isec, _ := is["secretName"].(string)
		if parent = c.pki[badgeSecretID(ns, isec)]; parent == nil {
			return false // the issuer is not issued yet
		}
	}
	p := c.pki[sid]
	if p == nil {
		p = &badgePKI{}
	}
	if p.key == nil || rotate {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			panic(err)
		}
		p.key = k
	}
	c.serial++
	signer, _ := spec["signer"].(bool)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(int64(c.serial)), Subject: pkix.Name{CommonName: secret},
		NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2027, 12, 31, 0, 0, 0, 0, time.UTC),
		BasicConstraintsValid: true, IsCA: signer, KeyUsage: x509.KeyUsageDigitalSignature}
	if signer {
		tmpl.KeyUsage |= x509.KeyUsageCertSign
	}
	pc, pk := tmpl, p.key
	if parent != nil {
		pc, pk = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, pc, &p.key.PublicKey, pk)
	if err != nil {
		panic(err)
	}
	p.der = der
	p.cert, _ = x509.ParseCertificate(der)
	c.pki[sid] = p
	caDER := der
	if parent != nil {
		caDER = parent.der
	}
	kb, _ := x509.MarshalECPrivateKey(p.key)
	ca := pemOf("CERTIFICATE", caDER)
	if c.keyLeaked {
		ca = pemOf("EC PRIVATE KEY", kb) + ca
	}
	c.objs[sid] = map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": secret, "namespace": ns},
		"data": map[string]any{"tls.crt": b64(pemOf("CERTIFICATE", der)), "ca.crt": b64(ca), "tls.key": b64(pemOf("EC PRIVATE KEY", kb))}}
	return true
}

func (c *fakeCluster) reconcileBadge(r ObjectRef, o map[string]any) {
	if c.version() == "" {
		return
	}
	spec, _ := o["spec"].(map[string]any)
	secret, _ := spec["secretName"].(string)
	if _, ok := c.objs[badgeSecretID(r.Namespace, secret)]; ok {
		return
	}
	if c.issueBadge(r.Namespace, spec, false) {
		st, _ := o["status"].(map[string]any)
		if st == nil {
			st = map[string]any{}
		}
		st["conditions"] = []any{map[string]any{"type": "Ready", "status": "True", "reason": "Issued"}}
		o["status"] = st
	}
}

func (c *fakeCluster) reactBadge(ref ObjectRef, obj map[string]any) {
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
	spec, _ := obj["spec"].(map[string]any)
	if !c.issueBadge(ref.Namespace, spec, c.version() == "v2" && spec["keyPolicy"] != "Keep") {
		return
	}
	st["conditions"] = keep
	st["revision"] = toInt(st["revision"]) + 1
	c.renewals++
}

const badgeCRD = `---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: badges.gizmo.io
spec:
  group: gizmo.io
  names: {kind: Badge, plural: badges}
  scope: Namespaced
  versions: [{name: v1, served: true, storage: true}]
`

// two arms: a signer that keeps its key, a signer that does not; each signs a
// leaf that leaves keyPolicy unset
const badgeObjects = `apiVersion: gizmo.io/v1
kind: Badge
metadata: {name: arm-present}
spec: {secretName: arm-present-tls, signer: true, keyPolicy: Keep}
---
apiVersion: gizmo.io/v1
kind: Badge
metadata: {name: arm-present-leaf}
spec: {secretName: arm-present-leaf-tls, issuer: arm-present}
---
apiVersion: gizmo.io/v1
kind: Badge
metadata: {name: arm-absent}
spec: {secretName: arm-absent-tls, signer: true}
---
apiVersion: gizmo.io/v1
kind: Badge
metadata: {name: arm-absent-leaf}
spec: {secretName: arm-absent-leaf-tls, issuer: arm-absent}
`

func reissueBadges() DeclaredExercise {
	return DeclaredExercise{Product: "gizmo", Digest: "sha256:x", Exercise: catalog.Exercise{ID: "reissue", Summary: "re-issue every Badge",
		Target: catalog.ObjectType{Group: "gizmo.io", Kind: "Badge"}, Action: catalog.ActionStatusCondition,
		Condition:     &catalog.ExerciseCondition{Type: "Issuing", Status: "True", Reason: "ManuallyTriggered"},
		Reaction:      &catalog.ExerciseReaction{Kind: catalog.ReactionFieldChanged, Path: "status.revision"},
		SettleSeconds: 30, Families: []string{"behavior"}, References: []catalog.ConfigReference{{URL: "https://example.io/gizmo/reissue", Quote: "reissue"}}}}
}

func badgeCheck(arm string) ConsumerCheck {
	b := func(n string) string { return "gizmo.io/v1/Badge/fixture/" + n }
	return ConsumerCheck{ID: arm, Exercise: "gizmo/reissue", Trust: SecretKey{Secret: badgeSecretID("fixture", arm+"-tls"), Key: "ca.crt"},
		Reissue: []string{b(arm), b(arm + "-leaf")}, Verify: SecretKey{Secret: badgeSecretID("fixture", arm+"-leaf-tls"), Key: "tls.crt"}}
}

func badgeSpec() FixtureSpec {
	var observe []string
	for _, n := range []string{"arm-present-tls", "arm-present-leaf-tls", "arm-absent-tls", "arm-absent-leaf-tls"} {
		observe = append(observe, badgeSecretID("fixture", n))
	}
	return FixtureSpec{ID: "fx-badge0123456", Product: "gizmo", From: "1.0.0", To: "2.0.0",
		FromController: RawRender{Manifests: []byte(controllerYAML("v1") + badgeCRD)}, ToController: RawRender{Manifests: []byte(controllerYAML("v2") + badgeCRD)},
		Objects: []byte(badgeObjects), Namespace: "fixture", Observe: observe, Consumers: []ConsumerCheck{badgeCheck("arm-present"), badgeCheck("arm-absent")}}
}

func consumerResults(r *Run) map[string]ConsumerResult {
	out := map[string]ConsumerResult{}
	for _, c := range r.Consumers {
		out[c.Check+"@"+c.Phase] = c
	}
	return out
}

// The consumer of the signer that keeps its key still verifies leaves issued
// after its re-issuance at both versions — although the leaf's own key
// rotates at v2; the consumer of the signer that rotates at v2 verifies at
// v1 and fails at v2. Only digests are recorded: no key and no certificate
// bytes reach a stored artifact.
func TestConsumerCheckObservesTheChainNotTheData(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	drv := &fakeDriver{free: 100 << 30}
	v := New(WithDriver(drv), WithClock(newFakeClock()), WithStore(st), WithExercises(reissueBadges()))
	res, err := v.RunFixture(context.Background(), badgeSpec())
	if err != nil {
		t.Fatal(err)
	}
	r := res.Run
	if r.Status != RunCompleted || !r.UpgradeCompleted {
		t.Fatalf("run %s %s %v", r.Status, r.Reason, r.UpgradeProblems)
	}
	got := consumerResults(r)
	for key, want := range map[string]string{"arm-present@from": ConsumerVerified, "arm-present@to": ConsumerVerified,
		"arm-absent@from": ConsumerVerified, "arm-absent@to": ConsumerFailed} {
		c, ok := got[key]
		if !ok || c.Outcome != want || len(c.Reissued) != 2 {
			t.Fatalf("%s: %+v (want %s)", key, c, want)
		}
		if !strings.HasPrefix(c.Certificate, "sha256:") || !strings.HasPrefix(c.CertificateKey, "sha256:") || len(c.TrustKeys) != 1 {
			t.Fatalf("%s: digests %+v", key, c)
		}
	}
	if !strings.Contains(got["arm-absent@to"].Detail, "does not verify") {
		t.Fatalf("detail: %s", got["arm-absent@to"].Detail)
	}
	// the shielded arm's leaf rotated its own key at v2: invisible to the consumer
	if got["arm-present@from"].CertificateKey == got["arm-present@to"].CertificateKey {
		t.Fatal("the leaf's key did not rotate at v2; the fixture does not exercise the claim")
	}
	// no key material, no certificate bytes in anything stored
	var secrets []string
	for _, p := range drv.cluster.pki {
		kb, _ := x509.MarshalECPrivateKey(p.key)
		k := pemOf("EC PRIVATE KEY", kb)
		secrets = append(secrets, k, b64(k), base64.StdEncoding.EncodeToString(kb), pemOf("CERTIFICATE", p.der), b64(pemOf("CERTIFICATE", p.der)))
	}
	n := 0
	err = filepath.Walk(st.Dir(), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		n++
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, s := range secrets {
			if strings.Contains(string(b), strings.TrimSpace(s)) {
				return fmt.Errorf("%s holds key or certificate material", path)
			}
		}
		if strings.Contains(string(b), "PRIVATE KEY") {
			return fmt.Errorf("%s holds a PEM private key", path)
		}
		return nil
	})
	if err != nil || n == 0 {
		t.Fatalf("stored artifacts (%d): %v", n, err)
	}
}

// A consumer check never decides on what it could not observe: an object
// that is not re-issued, or a trust key holding anything but certificates.
func TestConsumerCheckUndeterminedNeverFails(t *testing.T) {
	for name, setup := range map[string]func(*fakeCluster){
		"no re-issuance at v2":    func(c *fakeCluster) { c.ignoreAtV2 = true },
		"key material in the key": func(c *fakeCluster) { c.keyLeaked = true },
	} {
		drv := &fakeDriver{free: 100 << 30, setup: setup}
		v := New(WithDriver(drv), WithClock(newFakeClock()), WithExercises(reissueBadges()))
		res, err := v.RunFixture(context.Background(), badgeSpec())
		if err != nil {
			t.Fatal(err)
		}
		got := consumerResults(res.Run)
		to := got["arm-absent@to"]
		if to.Outcome != ConsumerUndetermined {
			t.Fatalf("%s: %+v", name, to)
		}
		if name == "key material in the key" && (!strings.Contains(to.Detail, "refused") || len(to.TrustKeys) != 0) {
			t.Fatalf("%s: %+v", name, to)
		}
	}
}

func TestRunFixtureRefusesAnInvalidConsumerCheck(t *testing.T) {
	for name, mutate := range map[string]func(*ConsumerCheck){
		"undeclared exercise": func(c *ConsumerCheck) { c.Exercise = "gizmo/other" },
		"not a Secret":        func(c *ConsumerCheck) { c.Trust.Secret = "gizmo.io/v1/Badge/fixture/arm-present" },
		"nothing re-issued":   func(c *ConsumerCheck) { c.Reissue = nil },
		"not a target":        func(c *ConsumerCheck) { c.Reissue = []string{"gizmo.io/v1/Widget/fixture/x"} },
	} {
		spec := badgeSpec()
		mutate(&spec.Consumers[0])
		v := New(WithDriver(&fakeDriver{free: 100 << 30}), WithClock(newFakeClock()), WithExercises(reissueBadges()))
		if _, err := v.RunFixture(context.Background(), spec); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
