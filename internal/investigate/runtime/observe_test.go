package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/renderdiff"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/render"
)

func yes() *bool { b := true; return &b }
func no() *bool  { b := false; return &b }

func TestDegradations(t *testing.T) {
	healthy := ObjectState{Exists: true, Ready: yes(), Generation: 2, ObservedGeneration: 2,
		Conditions: []Condition{{Type: "Ready", Status: "True"}}, Data: map[string]string{"a": "d1", "b": "d2"},
		Warnings: []string{"Old: seen before"}, Logs: []string{"E1016 old line ns/x"}}
	mut := func(f func(*ObjectState)) ObjectState {
		s := healthy
		s.Conditions = append([]Condition(nil), healthy.Conditions...)
		s.Data = map[string]string{"a": "d1", "b": "d2"}
		f(&s)
		return s
	}
	consumed := compareOptions{consumed: true}
	cases := map[string]struct {
		after ObjectState
		opt   compareOptions
		want  []string // signatures
	}{
		"unchanged":              {healthy, consumed, nil},
		"apply rejected":         {mut(func(s *ObjectState) { s.ApplyError = "denied" }), consumed, []string{"rejected"}},
		"deleted":                {ObjectState{}, consumed, []string{"deleted"}},
		"deleted by the upgrade": {ObjectState{}, compareOptions{removed: true}, nil},
		"condition True to False": {mut(func(s *ObjectState) {
			s.Conditions = []Condition{{Type: "Ready", Status: "False", Reason: "Broken"}}
			s.Ready = no()
		}), consumed, []string{"condition:Ready", "not-ready"}},
		"condition Unknown":          {mut(func(s *ObjectState) { s.Conditions = []Condition{{Type: "Ready", Status: "Unknown"}} }), consumed, nil},
		"negative condition appears": {mut(func(s *ObjectState) { s.Conditions = append(s.Conditions, Condition{Type: "Degraded", Status: "True"}) }), consumed, []string{"condition:Degraded"}},
		"new failing condition": {mut(func(s *ObjectState) {
			s.Conditions = append(s.Conditions, Condition{Type: "Synced", Status: "False", Reason: "SyncFailed"})
		}), consumed, []string{"condition:Synced"}},
		"new benign False condition": {mut(func(s *ObjectState) {
			s.Conditions = append(s.Conditions, Condition{Type: "Issuing", Status: "False", Reason: "Done"})
		}), consumed, nil},
		"not ready (image pull)":      {mut(func(s *ObjectState) { s.Ready, s.ImagePullIssue = no(), true }), consumed, []string{"not-ready"}},
		"not reconciled":              {mut(func(s *ObjectState) { s.Generation = 3 }), consumed, []string{"not-reconciled"}},
		"restarts, same pods":         {mut(func(s *ObjectState) { s.Restarts = 2 }), compareOptions{samePods: true}, []string{"restarts"}},
		"restarts, new pods":          {mut(func(s *ObjectState) { s.Restarts = 2 }), consumed, nil},
		"data changed under consumer": {mut(func(s *ObjectState) { s.Data["a"] = "d9" }), consumed, []string{"data:a"}},
		"data removed":                {mut(func(s *ObjectState) { delete(s.Data, "b") }), consumed, []string{"data:b"}},
		"data change declared":        {mut(func(s *ObjectState) { s.Data["a"] = "d9" }), compareOptions{consumed: true, dataDeclared: true}, nil},
		"data change, no consumer":    {mut(func(s *ObjectState) { s.Data["a"] = "d9" }), compareOptions{}, nil},
		"new warning":                 {mut(func(s *ObjectState) { s.Warnings = []string{"Old: seen before", "Rejected: bad field"} }), consumed, []string{"warning:Rejected"}},
		"known warning (baseline)": {mut(func(s *ObjectState) { s.Warnings = []string{"Rejected: bad field 12"} }),
			compareOptions{knownWarnings: map[string]bool{normalizeLine("Rejected: bad field 34"): true}}, nil},
		"new error log": {mut(func(s *ObjectState) { s.Logs = []string{"E1016 new failure ns/x"} }), consumed, []string{"log:E# new failure ns/x"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := degradations("x", "upgrade", healthy, c.after, c.opt)
			var sigs []string
			for _, d := range got {
				sigs = append(sigs, d.Signature)
				if d.Signature == "not-ready" && name == "not ready (image pull)" && !d.Infrastructure {
					t.Fatal("an image pull problem is the sandbox's, marked infrastructure")
				}
				if strings.Contains(d.Detail, "d9") == false && strings.HasPrefix(d.Signature, "data:") && !strings.Contains(d.Detail, "digest") {
					t.Fatal("data degradations cite digests")
				}
			}
			if strings.Join(sigs, ",") != strings.Join(c.want, ",") {
				t.Fatalf("signatures %v, want %v", sigs, c.want)
			}
		})
	}
}

func TestHealthyBaseline(t *testing.T) {
	for name, c := range map[string]struct {
		s  ObjectState
		ok bool
	}{
		"exists":             {ObjectState{Exists: true}, true},
		"absent":             {ObjectState{}, false},
		"rejected":           {ObjectState{ApplyError: "denied"}, false},
		"unserved":           {ObjectState{Unserved: true}, false},
		"not ready":          {ObjectState{Exists: true, Ready: no()}, false},
		"failure condition":  {ObjectState{Exists: true, Conditions: []Condition{{Type: "Stalled", Status: "True"}}}, false},
		"ready with Ready=T": {ObjectState{Exists: true, Ready: yes(), Conditions: []Condition{{Type: "Ready", Status: "True"}}}, true},
	} {
		if ok, _ := c.s.healthy(); ok != c.ok {
			t.Errorf("%s: healthy=%v", name, ok)
		}
	}
}

func TestReadiness(t *testing.T) {
	obj := func(s string) map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	dep := ObjectRef{Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "a", Name: "d"}
	for name, c := range map[string]struct {
		ref  ObjectRef
		obj  string
		want *bool
	}{
		"deployment ready":     {dep, `{"metadata":{"generation":2},"spec":{"replicas":2},"status":{"observedGeneration":2,"availableReplicas":2,"updatedReplicas":2}}`, yes()},
		"deployment rolling":   {dep, `{"metadata":{"generation":2},"spec":{"replicas":2},"status":{"observedGeneration":2,"availableReplicas":1,"updatedReplicas":2}}`, no()},
		"deployment stale gen": {dep, `{"metadata":{"generation":3},"status":{"observedGeneration":2,"availableReplicas":1,"updatedReplicas":1}}`, no()},
		"job failed":           {ObjectRef{Group: "batch", Version: "v1", Kind: "Job"}, `{"status":{"conditions":[{"type":"Failed","status":"True"}]}}`, no()},
		"job done":             {ObjectRef{Group: "batch", Version: "v1", Kind: "Job"}, `{"status":{"succeeded":1}}`, yes()},
		"crd established":      {ObjectRef{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}, `{"status":{"conditions":[{"type":"Established","status":"True"}]}}`, yes()},
		"cr Ready False":       {ObjectRef{Group: "x.io", Version: "v1", Kind: "Thing"}, `{"status":{"conditions":[{"type":"Ready","status":"False"}]}}`, no()},
		"cr no conditions":     {ObjectRef{Group: "x.io", Version: "v1", Kind: "Thing"}, `{"status":{}}`, nil},
		"configmap":            {ObjectRef{Version: "v1", Kind: "ConfigMap"}, `{}`, nil},
	} {
		got, _ := readiness(c.ref, obj(c.obj))
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("%s: readiness %v, want %v", name, got, c.want)
		}
	}
}

func TestWarningEventsAndLogMatching(t *testing.T) {
	since := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	events := []map[string]any{
		{"type": "Warning", "reason": "Rejected", "message": "bad", "lastTimestamp": "2026-10-16T12:05:00Z", "involvedObject": map[string]any{"kind": "Ingress", "namespace": "shop", "name": "web"}},
		{"type": "Warning", "reason": "Old", "message": "before the window", "lastTimestamp": "2026-10-16T11:00:00Z", "involvedObject": map[string]any{"kind": "Ingress", "namespace": "shop", "name": "web"}},
		{"type": "Normal", "reason": "Sync", "message": "ok", "lastTimestamp": "2026-10-16T12:05:00Z", "involvedObject": map[string]any{"kind": "Ingress", "namespace": "shop", "name": "web"}},
	}
	w := warningEvents(events, since)
	if got := w["Ingress|shop|web"]; len(got) != 1 || got[0] != "Rejected: bad" {
		t.Fatalf("warnings %v", got)
	}
	lines := []string{"E1016 12:00 shop/web: annotation rejected", `{"level":"error","msg":"x","namespace":"shop","name":"web"}`, "E1016 other/web: nope", "E1016 shop/webby: nope"}
	got := matchLogs(lines, ObjectRef{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress", Namespace: "shop", Name: "web"})
	if len(got) != 2 {
		t.Fatalf("log lines naming shop/web: %v", got)
	}
	for _, l := range []string{"E1016 12:00:00.000000 1 x.go:1] boom", `{"level":"error"}`, "time=x level=error msg=y"} {
		if !errorLine.MatchString(l) {
			t.Errorf("not an error line: %q", l)
		}
	}
	for _, l := range []string{"I1016 12:00:00 info", `{"level":"info"}`} {
		if errorLine.MatchString(l) {
			t.Errorf("an error line: %q", l)
		}
	}
}

func TestRefsAndMarkers(t *testing.T) {
	r, ok := ParseRef("core/v1/Secret/ns/name")
	if !ok || r.Group != "" || r.String() != "core/v1/Secret/ns/name" || r.APIVersion() != "v1" {
		t.Fatalf("%+v", r)
	}
	if _, ok := ParseRef("cert-manager.io/v1/Certificate"); ok {
		t.Fatal("an api-type key is not an object")
	}
	if r, ok := ParseRef("rbac.authorization.k8s.io/v1/ClusterRole//x"); !ok || r.Namespace != "" {
		t.Fatal("cluster-scoped key")
	}
	if !markerMatches(bundleMarkerPrefix+sha256Hex("w1-tls")[:12]+">", "w1-tls") || markerMatches(bundleMarkerPrefix+sha256Hex("w1-tls")[:12]+">", "other") {
		t.Fatal("bundle marker")
	}
	b, _ := json.Marshal("pw")
	if !markerMatches(renderMarkerPrefix+sha256Hex(string(b))[:16]+">", "pw") {
		t.Fatal("render marker")
	}
	k1, k2 := []byte("run-key-one"), []byte("run-key-two")
	d := digestValue(k1, "secret")
	if strings.Contains(d, "secret") || !strings.HasPrefix(d, "hmac-sha256:") || len(d) != len("hmac-sha256:")+64 {
		t.Fatalf("digest %s: a full-length keyed digest", d)
	}
	if digestValue(k1, "secret") != d || digestValue(k1, "other") == d {
		t.Fatal("within a run, equal values digest equal and different values differ")
	}
	if digestValue(k2, "secret") == d {
		t.Fatal("another run's key gives another digest: a recorded digest cannot be matched against a guessed value")
	}
	if strings.Contains(d, sha256Hex("secret")[:16]) {
		t.Fatal("the digest is keyed, not a plain hash")
	}
}

func TestExercisePatch(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	obj := map[string]any{"metadata": map[string]any{"generation": 4}, "status": map[string]any{"conditions": []any{
		map[string]any{"type": "Ready", "status": "True"}, map[string]any{"type": "Issuing", "status": "False"}}}}
	x := renewExercise().Exercise
	sub, p, err := exercisePatch(x, obj, "tok", now)
	if err != nil || sub != "status" {
		t.Fatal(sub, err)
	}
	var got struct {
		Status struct {
			Conditions []map[string]any `json:"conditions"`
		} `json:"status"`
	}
	_ = json.Unmarshal(p, &got)
	if len(got.Status.Conditions) != 2 || got.Status.Conditions[0]["type"] != "Issuing" || got.Status.Conditions[0]["status"] != "True" ||
		got.Status.Conditions[0]["observedGeneration"] != float64(4) || got.Status.Conditions[1]["type"] != "Ready" {
		t.Fatalf("a merge patch replaces the list: other conditions must be kept, the exercised one replaced: %s", p)
	}
	ann := catalog.Exercise{Action: catalog.ActionAnnotate, Annotation: "ri.dev/runtime-exercise"}
	if sub, p, _ := exercisePatch(ann, obj, "tok", now); sub != "" || !strings.Contains(string(p), `"ri.dev/runtime-exercise":"tok"`) {
		t.Fatalf("annotate: %s %s", sub, p)
	}
	rr := catalog.Exercise{Action: catalog.ActionRolloutRestart}
	if _, p, _ := exercisePatch(rr, obj, "tok", now); !strings.Contains(string(p), "kubectl.kubernetes.io/restartedAt") {
		t.Fatalf("rollout restart: %s", p)
	}
}

// The exercise families the catalog accepts are exactly the domain's.
func TestExerciseFamiliesMatchDomain(t *testing.T) {
	d := renewExercise()
	for _, f := range domain.ChangeItemFamilies {
		if d.covers(f) {
			t.Fatalf("an exercise without families covers no family (%s)", f)
		}
	}
	d = renewExercise("behavior")
	if !d.covers(domain.FamilyBehavior) || d.covers(domain.FamilyRendered) {
		t.Fatal("families restrict coverage")
	}
}

func TestVerifyRender(t *testing.T) {
	ctx := context.Background()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	raw := controllerYAML("v1") + "---\napiVersion: v1\nkind: Secret\nmetadata: {name: s, namespace: gizmo}\ndata: {password: c2VjcmV0}\n"
	objs, _ := render.ParseObjects([]byte(raw))
	stored, redacted := renderdiff.EncodeObjects(objs)
	if !redacted || strings.Contains(string(stored), "c2VjcmV0") {
		t.Fatal("the stored render never holds secret values")
	}
	ref, err := st.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactRenderedManifests, Path: "derived/render/env/x/from.yaml", Origin: domain.OriginDerived,
		MediaType: "application/yaml", URI: "ri:test", Producer: "test@v1"}, stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyRender(ctx, st, ref, &RawRender{Manifests: []byte(raw)}); err != nil {
		t.Fatalf("the raw render (with empty maps and secret values) normalizes to the stored artifact: %v", err)
	}
	changed := strings.Replace(raw, "c2VjcmV0", "b3RoZXI=", 1)
	if err := verifyRender(ctx, st, ref, &RawRender{Manifests: []byte(changed)}); err == nil || !strings.Contains(err.Error(), "core/v1/Secret/gizmo/s differs") {
		t.Fatalf("a different secret value is a different render: %v", err)
	}
	if err := verifyRender(ctx, st, ref, &RawRender{Manifests: []byte(changed), Nondeterministic: []string{"core/v1/Secret/gizmo/s|data.password"}}); err != nil {
		t.Fatalf("nondeterministic objects are tolerated: %v", err)
	}
	if err := verifyRender(ctx, st, ref, &RawRender{Manifests: []byte(controllerYAML("v1"))}); err == nil || !strings.Contains(err.Error(), "missing from the re-render") {
		t.Fatalf("a missing object: %v", err)
	}
}

func TestNodeImagePinning(t *testing.T) {
	v := New(WithDriver(&fakeDriver{}))
	for kube, want := range map[string]string{"1.31": kindNodeImages["1.31"], "v1.30.2": kindNodeImages["1.30"], "": ""} {
		if img, why := v.nodeImage(kube); img != want || why != "" {
			t.Errorf("%q: %q %q", kube, img, why)
		}
	}
	if _, why := v.nodeImage("1.40"); !strings.Contains(why, "no pinned kind node image") {
		t.Fatal(why)
	}
	if img, _ := New(WithDriver(&fakeDriver{}), WithNodeImage("kindest/node:custom")).nodeImage("1.40"); img != "kindest/node:custom" {
		t.Fatal(img)
	}
}
