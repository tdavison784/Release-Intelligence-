package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
)

// --- a fake cluster with a validating webhook ----------------------------------------------

type admCluster struct {
	mu       sync.Mutex
	name     string
	objs     map[string]map[string]any
	ready    bool                                // the webhook service has a ready endpoint
	verdict  func(attempt int, m Manifest) error // the API server's answer to a dry run
	attempts int
	dryRuns  []string
	noDryRun bool // Raw/DryRunApply unsupported (a plain Cluster)
}

func (c *admCluster) Name() string { return c.name }

func (c *admCluster) Apply(_ context.Context, m Manifest, namespace string) (map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	obj := deepCopy(m.Doc)
	r := m.Ref
	if r.Namespace == "" && !clusterScoped[r.Kind] && r.Kind != "ValidatingWebhookConfiguration" {
		r.Namespace = namespace
		obj["metadata"].(map[string]any)["namespace"] = namespace
	}
	if r.Group == "apps" && r.Kind == "Deployment" {
		obj["metadata"].(map[string]any)["generation"] = 1
		obj["status"] = map[string]any{"observedGeneration": 1, "availableReplicas": 1, "updatedReplicas": 1}
	}
	c.objs[r.String()] = obj
	return deepCopy(obj), nil
}

func (c *admCluster) Delete(_ context.Context, ref ObjectRef) error { return nil }

func (c *admCluster) List(_ context.Context, g GVK) ([]map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []map[string]any
	if g == endpointsType {
		for id := range c.objs {
			if r, _ := ParseRef(id); r.Kind == "Service" {
				out = append(out, map[string]any{"apiVersion": "discovery.k8s.io/v1", "kind": "EndpointSlice",
					"metadata":  map[string]any{"name": r.Name + "-abc", "namespace": r.Namespace, "labels": map[string]any{serviceNameKey: r.Name}},
					"endpoints": []any{map[string]any{"conditions": map[string]any{"ready": c.ready}}}})
			}
		}
		return out, nil
	}
	for id, o := range c.objs {
		if r, _ := ParseRef(id); r.Group == g.Group && r.Kind == g.Kind {
			out = append(out, deepCopy(o))
		}
	}
	return out, nil
}

func (c *admCluster) Patch(context.Context, ObjectRef, string, []byte) error { return nil }

func (c *admCluster) Logs(context.Context, string, string, time.Time) (string, error) { return "", nil }

func (c *admCluster) DryRunApply(_ context.Context, m Manifest, _ string) (map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attempts++
	c.dryRuns = append(c.dryRuns, m.Ref.String())
	if c.verdict != nil {
		if err := c.verdict(c.attempts, m); err != nil {
			return nil, err
		}
	}
	return deepCopy(m.Doc), nil
}

func (c *admCluster) Raw(_ context.Context, path string) ([]byte, error) {
	switch path {
	case "/apis/networking.k8s.io/v1":
		return []byte(`{"resources":[{"name":"ingresses","kind":"Ingress","namespaced":true},{"name":"ingresses/status","kind":"Ingress","namespaced":true},{"name":"ingressclasses","kind":"IngressClass","namespaced":false}]}`), nil
	}
	return nil, fmt.Errorf("the server could not find the requested resource")
}

type admDriver struct {
	fakeDriver
	c *admCluster
}

func (d *admDriver) Create(ctx context.Context, spec ClusterSpec) (Cluster, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.creates++
	d.lastCreate = spec
	d.existing = append(d.existing, spec.Name)
	d.c.name = spec.Name
	if d.c.noDryRun {
		return struct{ Cluster }{d.c}, nil
	}
	return d.c, nil
}

func admissionExercise() DeclaredExercise {
	return DeclaredExercise{Product: "gizmo", Exercise: catalog.Exercise{ID: "ingress-admission", Summary: "dry-run an Ingress",
		Target: catalog.ObjectType{Group: "networking.k8s.io", Kind: "Ingress"}, Action: catalog.ActionAdmissionDryRun,
		Families: []string{"behavior"}, References: []catalog.ConfigReference{{URL: "https://example.io/gizmo/admission", Quote: "validates"}}}}
}

const admController = `apiVersion: apps/v1
kind: Deployment
metadata: {name: hook, namespace: gizmo}
spec:
  replicas: 1
  selector: {matchLabels: {app: hook}}
  template:
    metadata: {labels: {app: hook}}
    spec: {containers: [{name: hook, image: "gizmo-hook:v2"}]}
---
apiVersion: v1
kind: Service
metadata: {name: hook, namespace: gizmo}
spec: {selector: {app: hook}, ports: [{port: 443}]}
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata: {name: gizmo-admission}
webhooks:
- name: validate.gizmo.example
  sideEffects: %s
  failurePolicy: %s
  admissionReviewVersions: [v1]
  clientConfig: {service: {namespace: gizmo, name: hook}}
  rules: [{apiGroups: [networking.k8s.io], apiVersions: [v1], operations: [CREATE, UPDATE], resources: [%s]}]
  %s
`

const admObject = `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata: {name: docs, namespace: web, labels: {tier: public}}
spec:
  rules: [{http: {paths: [{path: "/docs/*", pathType: Prefix, backend: {service: {name: docs, port: {number: 80}}}}]}}]
`

type admCase struct {
	sideEffects, failurePolicy, resources, extra string
}

func runAdmission(t *testing.T, c *admCluster, hook admCase, edit func(*AdmissionSpec)) (*AdmissionRun, *admDriver) {
	t.Helper()
	if hook.sideEffects == "" {
		hook = admCase{"None", "Fail", "ingresses", hook.extra}
	}
	c.objs = map[string]map[string]any{}
	d := &admDriver{fakeDriver: fakeDriver{free: 50 << 30}, c: c}
	v := New(WithDriver(d), WithClock(newFakeClock()), WithExercises(admissionExercise()))
	spec := AdmissionSpec{ID: "adm-0123456789", Product: "gizmo", Version: "2.0.0", Exercise: "gizmo/ingress-admission",
		Controller: RawRender{Manifests: []byte(fmt.Sprintf(admController, hook.sideEffects, hook.failurePolicy, hook.resources, hook.extra)), Namespace: "gizmo"},
		Object:     []byte(admObject), ClusterPrefix: "ri-semverify-"}
	if edit != nil {
		edit(&spec)
	}
	ar, err := v.DryRunAdmission(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if ar.Run == nil || ar.Run.Cleanup == nil || !ar.Run.Cleanup.Deleted || !ar.Run.Cleanup.Verified {
		t.Fatalf("the cluster must be deleted: %+v", ar.Run)
	}
	if !strings.HasPrefix(ar.Run.Cluster, "ri-semverify-") || ar.Level != "observed" || ar.Validator != AdmissionName {
		t.Errorf("record: cluster %q level %q validator %q", ar.Run.Cluster, ar.Level, ar.Validator)
	}
	return ar, d
}

func TestAdmissionAccepted(t *testing.T) {
	c := &admCluster{ready: true}
	ar, _ := runAdmission(t, c, admCase{}, nil)
	if ar.Outcome != AdmissionAccepted || ar.Attempts != 1 || len(ar.Webhooks) != 1 || !ar.Webhooks[0].Covers || !ar.Webhooks[0].Ready ||
		ar.Namespace != "web" || ar.Object != "networking.k8s.io/v1/Ingress/web/docs" {
		t.Fatalf("%+v", ar)
	}
	if _, ok := c.objs["networking.k8s.io/v1/Ingress/web/docs"]; ok {
		t.Error("the object must only be dry-run, never stored")
	}
	if _, ok := c.objs["core/v1/Namespace//web"]; !ok {
		t.Errorf("the object's namespace must exist: %v", c.objs)
	}
}

func TestAdmissionRejectedByWebhook(t *testing.T) {
	c := &admCluster{ready: true, verdict: func(_ int, m Manifest) error {
		return errors.New(`Error from server (BadRequest): error when creating "STDIN": admission webhook "validate.gizmo.example" denied the request: path /docs/* cannot be used with pathType Prefix`)
	}}
	ar, _ := runAdmission(t, c, admCase{}, nil)
	if ar.Outcome != AdmissionRejected || ar.RejectedBy != "webhook validate.gizmo.example" || !strings.Contains(ar.Message, "cannot be used with pathType Prefix") {
		t.Fatalf("%+v", ar)
	}
}

func TestAdmissionRejectedByAPIServer(t *testing.T) {
	c := &admCluster{ready: true, verdict: func(int, Manifest) error {
		return errors.New(`The Ingress "docs" is invalid: spec.rules[0].http.paths[0].path: Invalid value: "/docs/*": must be an absolute path`)
	}}
	ar, _ := runAdmission(t, c, admCase{}, nil)
	if ar.Outcome != AdmissionRejected || ar.RejectedBy != "api-server" {
		t.Fatalf("%+v", ar)
	}
}

// A webhook that cannot be called yet is retried; an infrastructure error
// is never a rejection.
func TestAdmissionRetriesUnreachableWebhook(t *testing.T) {
	unreachable := errors.New(`Internal error occurred: failed calling webhook "validate.gizmo.example": Post "https://hook.gizmo.svc:443/": dial tcp 10.96.0.1:443: connect: connection refused`)
	c := &admCluster{ready: true, verdict: func(n int, _ Manifest) error {
		if n < 3 {
			return unreachable
		}
		return nil
	}}
	ar, _ := runAdmission(t, c, admCase{}, nil)
	if ar.Outcome != AdmissionAccepted || ar.Attempts != 3 {
		t.Fatalf("%+v", ar)
	}
	c = &admCluster{ready: true, verdict: func(int, Manifest) error { return unreachable }}
	ar, _ = runAdmission(t, c, admCase{}, nil)
	if ar.Outcome != AdmissionUndetermined || ar.Attempts < 2 || !strings.Contains(ar.Message, "failed calling webhook") {
		t.Fatalf("%+v", ar)
	}
}

// Without a webhook certainly in the path, acceptance would mean nothing.
func TestAdmissionUndeterminedWithoutEffectiveWebhook(t *testing.T) {
	for name, tc := range map[string]struct {
		hook  admCase
		ready bool
		why   string
	}{
		"rules for another resource":           {admCase{"None", "Fail", "ingressclasses", ""}, true, "no effective validating webhook"},
		"objectSelector excludes the object":   {admCase{"None", "Fail", "ingresses", "objectSelector: {matchLabels: {tier: internal}}"}, true, "excluded by a selector"},
		"namespaceSelector cannot be decided":  {admCase{"None", "Fail", "ingresses", "namespaceSelector: {matchExpressions: [{key: a, operator: Gt, values: ['1']}]}"}, true, "no effective"},
		"not ready with failure policy Ignore": {admCase{"None", "Ignore", "ingresses", ""}, false, "not ready with failure policy Ignore"},
		"side effects refuse dry runs":         {admCase{"Unknown", "Fail", "ingresses", ""}, true, "sideEffects Unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			c := &admCluster{ready: tc.ready}
			ar, _ := runAdmission(t, c, tc.hook, nil)
			if ar.Outcome != AdmissionUndetermined || c.attempts != 0 || !strings.Contains(ar.Detail, tc.why) {
				t.Fatalf("outcome %s after %d dry runs: %s", ar.Outcome, c.attempts, ar.Detail)
			}
		})
	}
	// a matching namespaceSelector covers it
	c := &admCluster{ready: true}
	ar, _ := runAdmission(t, c, admCase{"None", "Fail", "ingresses", "namespaceSelector: {matchExpressions: [{key: kubernetes.io/metadata.name, operator: In, values: [web]}]}"}, nil)
	if ar.Outcome != AdmissionAccepted {
		t.Fatalf("%+v", ar)
	}
}

func TestAdmissionNotRunAndInvalidInput(t *testing.T) {
	// another isolated cluster exists: one at a time
	c := &admCluster{ready: true, objs: map[string]map[string]any{}}
	d := &admDriver{fakeDriver: fakeDriver{free: 50 << 30, existing: []string{"ri-verify-12345678"}}, c: c}
	v := New(WithDriver(d), WithClock(newFakeClock()), WithExercises(admissionExercise(), renewExercise("behavior")))
	spec := AdmissionSpec{ID: "adm-1", Product: "gizmo", Version: "2.0.0", Exercise: "gizmo/ingress-admission",
		Controller: RawRender{Manifests: []byte(fmt.Sprintf(admController, "None", "Fail", "ingresses", "")), Namespace: "gizmo"}, Object: []byte(admObject)}
	ar, err := v.DryRunAdmission(context.Background(), spec)
	if err != nil || ar.Outcome != AdmissionUndetermined || ar.Run.Status != RunNotRun || d.creates != 0 || !strings.Contains(ar.Detail, "another runtime validation cluster") {
		t.Fatalf("%v %+v", err, ar)
	}
	// a cluster that cannot dry-run
	c2 := &admCluster{ready: true, noDryRun: true}
	ar, _ = runAdmission(t, c2, admCase{}, nil)
	if ar.Outcome != AdmissionUndetermined || !strings.Contains(ar.Detail, "cannot submit dry-run") {
		t.Fatalf("%+v", ar)
	}
	for name, edit := range map[string]func(*AdmissionSpec){
		"not an admission exercise": func(s *AdmissionSpec) { s.Exercise = "gizmo/renew" },
		"undeclared exercise":       func(s *AdmissionSpec) { s.Exercise = "gizmo/nope" },
		"two objects":               func(s *AdmissionSpec) { s.Object = append([]byte(admObject+"---\n"), admObject...) },
		"not the target kind":       func(s *AdmissionSpec) { s.Object = []byte("apiVersion: v1\nkind: ConfigMap\nmetadata: {name: x}\n") },
		"no controller":             func(s *AdmissionSpec) { s.Controller = RawRender{} },
	} {
		s := spec
		edit(&s)
		if _, err := v.DryRunAdmission(context.Background(), s); err == nil {
			t.Errorf("%s: want an input error", name)
		}
	}
}

// The isolated upgrade never runs an admission exercise.
func TestUpgradeSkipsAdmissionExercises(t *testing.T) {
	x := admissionExercise()
	if !x.admission() || renewExercise("behavior").admission() {
		t.Fatal("admission() misclassifies")
	}
}
