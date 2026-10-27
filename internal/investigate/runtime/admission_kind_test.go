package runtime

// REAL admission dry runs on kind fixtures (lane semverify). A minimal
// validating webhook (stdlib Go, below) is compiled into a FROM-scratch image
// (no image pull), loaded into a throwaway cluster ri-semverify-*, and one
// ConfigMap is dry-run against it twice: once accepted, once rejected with
// the webhook's message. Opt-in (it creates two kind clusters, ~2-4 min):
//
//	RI_ADMISSION_KIND=1 go test ./internal/investigate/runtime -run AdmissionKind -v -timeout 20m
//
// It skips with a message when kind, kubectl, docker or the docker daemon is
// missing. Every cluster is deleted, and the image removed, afterwards.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
)

const kindWebhookSource = `package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

func main() {
	http.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
		var review struct {
			Request struct {
				UID    string ` + "`json:\"uid\"`" + `
				Object struct {
					Data map[string]string ` + "`json:\"data\"`" + `
				} ` + "`json:\"object\"`" + `
			} ` + "`json:\"request\"`" + `
		}
		if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := map[string]any{"uid": review.Request.UID, "allowed": true}
		for k, v := range review.Request.Object.Data {
			for _, c := range v {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._/-", c)) {
					resp["allowed"] = false
					resp["status"] = map[string]any{"code": 403, "message": "data." + k + " contains " + string(c) + ": only [A-Za-z0-9._/-] is allowed"}
				}
			}
		}
		log.Printf("validated %s: %v", review.Request.UID, resp["allowed"])
		json.NewEncoder(w).Encode(map[string]any{"apiVersion": "admission.k8s.io/v1", "kind": "AdmissionReview", "response": resp})
	})
	log.Fatal(http.ListenAndServeTLS(":8443", "/tls/tls.crt", "/tls/tls.key", nil))
}
`

const kindWebhookImage = "ri-semverify-webhook:test"

// imageLoadingDriver loads the fixture image into each cluster it creates.
type imageLoadingDriver struct {
	*KindDriver
	image string
}

func (d imageLoadingDriver) Create(ctx context.Context, spec ClusterSpec) (Cluster, error) {
	cl, err := d.KindDriver.Create(ctx, spec)
	if err != nil {
		return cl, err
	}
	if _, err := run(ctx, nil, d.Kind, "load", "docker-image", d.image, "--name", spec.Name); err != nil {
		return cl, fmt.Errorf("load %s: %w", d.image, err)
	}
	return cl, nil
}

func TestAdmissionKind(t *testing.T) {
	if os.Getenv("RI_ADMISSION_KIND") == "" {
		t.Skip("set RI_ADMISSION_KIND=1 to run real admission dry runs in kind (creates two clusters ri-semverify-*)")
	}
	for _, bin := range []string{"kind", "kubectl", "docker", "go"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is not on PATH; a real admission dry run needs it", bin)
		}
	}
	t.Setenv("DO_NOT_TRACK", "1")
	ctx := context.Background()
	if out, err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}").CombinedOutput(); err != nil {
		t.Skipf("the docker daemon is not reachable: %v %s", err, out)
	}

	// the webhook image: a static binary FROM scratch (nothing is pulled)
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", kindWebhookSource)
	write("Dockerfile", "FROM scratch\nCOPY webhook /webhook\nENTRYPOINT [\"/webhook\"]\n")
	build := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(dir, "webhook"), filepath.Join(dir, "main.go"))
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+goruntime.GOARCH)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build webhook: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(ctx, "docker", "build", "-q", "-t", kindWebhookImage, dir).CombinedOutput(); err != nil {
		t.Fatalf("docker build: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rmi", "-f", kindWebhookImage).Run() })

	ctrl := kindWebhookController(t)
	x := DeclaredExercise{Product: "fixture", Exercise: catalog.Exercise{ID: "configmap-admission", Summary: "dry-run a ConfigMap",
		Target: catalog.ObjectType{Kind: "ConfigMap"}, Action: catalog.ActionAdmissionDryRun, Families: []string{"behavior"},
		References: []catalog.ConfigReference{{URL: "https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/", Quote: "dry run"}}}}
	v := New(WithDriver(imageLoadingDriver{KindDriver: NewKindDriver(), image: kindWebhookImage}), WithExercises(x),
		WithTimeout(10*time.Minute), WithLogf(t.Logf))
	object := func(value string) []byte {
		return []byte(fmt.Sprintf("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: routes\n  namespace: ri-fixture\n  labels: {ri.dev/admission-fixture: \"true\"}\ndata:\n  path: %q\n", value))
	}
	for _, c := range []struct {
		id, value string
		want      AdmissionOutcome
	}{
		{"adm-kind-accepted", "/docs/a", AdmissionAccepted},
		{"adm-kind-rejected", "/docs/*", AdmissionRejected},
	} {
		t.Run(string(c.want), func(t *testing.T) {
			ar, err := v.DryRunAdmission(ctx, AdmissionSpec{ID: c.id, Product: "fixture", Version: "1.0.0", KubernetesVersion: "1.31",
				Controller: RawRender{Manifests: ctrl, Namespace: "ri-hook"}, Object: object(c.value), Exercise: "fixture/configmap-admission",
				ClusterPrefix: "ri-semverify-"})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: %s — %s %s (webhooks %+v, %d attempts, %.0fs)", ar.Object, ar.Outcome, ar.Detail, ar.Message, ar.Webhooks, ar.Attempts, ar.Run.DurationSeconds)
			if ar.Run.Cleanup == nil || !ar.Run.Cleanup.Deleted || !ar.Run.Cleanup.Verified {
				t.Fatalf("cluster %s not deleted: %+v", ar.Run.Cluster, ar.Run.Cleanup)
			}
			if ar.Run.Status != RunCompleted || ar.Outcome != c.want || ar.Level != "observed" {
				t.Fatalf("run %s (%s): outcome %s, want %s", ar.Run.Status, ar.Run.Reason, ar.Outcome, c.want)
			}
			if c.want == AdmissionRejected && (ar.RejectedBy != "webhook validate.fixture.ri.dev" || !strings.Contains(ar.Message, "contains *")) {
				t.Errorf("rejection: by %q, message %q", ar.RejectedBy, ar.Message)
			}
		})
	}
}

// kindWebhookController is the fixture "controller": the webhook Deployment,
// its Service, its serving certificate and the ValidatingWebhookConfiguration
// (CA bundle from a throwaway CA).
func kindWebhookController(t *testing.T) []byte {
	t.Helper()
	caKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ri-semverify-test-ca"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(24 * time.Hour), IsCA: true, KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "ri-hook.ri-hook.svc"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(24 * time.Hour), DNSNames: []string{"ri-hook.ri-hook.svc"}, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	b64pem := func(typ string, der []byte) string {
		return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}))
	}
	return []byte(fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata: {name: ri-hook-tls, namespace: ri-hook}
type: kubernetes.io/tls
data: {tls.crt: %s, tls.key: %s}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: ri-hook, namespace: ri-hook}
spec:
  replicas: 1
  selector: {matchLabels: {app: ri-hook}}
  template:
    metadata: {labels: {app: ri-hook}}
    spec:
      containers:
      - name: webhook
        image: %s
        imagePullPolicy: Never
        ports: [{containerPort: 8443}]
        readinessProbe: {tcpSocket: {port: 8443}, periodSeconds: 2}
        volumeMounts: [{name: tls, mountPath: /tls, readOnly: true}]
      volumes: [{name: tls, secret: {secretName: ri-hook-tls}}]
---
apiVersion: v1
kind: Service
metadata: {name: ri-hook, namespace: ri-hook}
spec: {selector: {app: ri-hook}, ports: [{port: 443, targetPort: 8443}]}
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata: {name: ri-fixture-admission}
webhooks:
- name: validate.fixture.ri.dev
  sideEffects: None
  failurePolicy: Fail
  timeoutSeconds: 5
  admissionReviewVersions: [v1]
  objectSelector: {matchLabels: {ri.dev/admission-fixture: "true"}}
  clientConfig:
    service: {namespace: ri-hook, name: ri-hook, path: /validate}
    caBundle: %s
  rules: [{apiGroups: [""], apiVersions: [v1], operations: [CREATE, UPDATE], resources: [configmaps]}]
`, b64pem("CERTIFICATE", leafDER), b64pem("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key)), kindWebhookImage, b64pem("CERTIFICATE", caDER)))
}
