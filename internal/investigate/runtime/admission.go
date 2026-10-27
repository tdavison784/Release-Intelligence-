package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// The admission dry run (Phase 6 Milestone 4, mission E5, lane semverify):
// does the target version's validating admission webhook accept one
// environment object? The product's controller at the target version is
// installed in a throwaway kind cluster, the webhooks that cover the object
// are found and waited for, and the object is submitted as a server-side dry
// run. The API server's answer is the observation: accepted, or rejected with
// its message. It settles questions static evidence cannot (a parser's
// actual character class, a validation the documentation does not state).
//
// An "accepted" means something only when a webhook really was in the path:
// at least one validating webhook whose rules and selectors cover the object
// (CREATE) and that is effective — its service has a ready endpoint, or its
// failure policy is Fail (a call failure then rejects, it never silently
// admits). Otherwise, and whenever the API server cannot be asked or answers
// with an infrastructure error, the outcome is undetermined, never accepted.
//
// Opt-in and bounded like the isolated upgrade (runtime.isolated-upgrade@v1):
// the same preflight (tools, disk, one isolated cluster at a time), budget,
// apply and readiness rules, and the cluster is deleted on every exit path.

// AdmissionName is the admission dry run's producer (component@vN).
const AdmissionName = "runtime.admission-dry-run@v1"

// AdmissionClusterPrefix names the clusters of admission dry runs.
const AdmissionClusterPrefix = "ri-admission-"

// AdmissionSchemaVersion versions the admission record.
const AdmissionSchemaVersion = "ri.dev/runtime-admission/v1alpha1"

// AdmissionOutcome is what the API server answered.
type AdmissionOutcome string

const (
	AdmissionAccepted     AdmissionOutcome = "accepted"
	AdmissionRejected     AdmissionOutcome = "rejected"
	AdmissionUndetermined AdmissionOutcome = "undetermined"
)

// AdmissionSpec is one admission dry run.
type AdmissionSpec struct {
	// ID is the run's content id: it names the cluster (prefix + 8 hex) and
	// the stored record.
	ID               string
	Product, Version string
	// KubernetesVersion selects the pinned node image ("" = kind's default).
	KubernetesVersion string
	// Controller is the product at Version (raw manifests: its webhook).
	Controller RawRender
	// Setup are objects applied for real before the dry run (the objects the
	// submitted one refers to, e.g. its class); optional.
	Setup []byte
	// Object is exactly one environment object of the exercise's target
	// kind, applied into Namespace when it names none.
	Object    []byte
	Namespace string
	// Exercise is the declared admission-dry-run exercise (<product>/<id>).
	Exercise string
	// ClusterPrefix overrides AdmissionClusterPrefix (tests name their
	// clusters after their lane).
	ClusterPrefix string
}

// AdmissionWebhook is a validating webhook whose rules match the object.
type AdmissionWebhook struct {
	Configuration string `json:"configuration"`
	Name          string `json:"name"`
	// Service is "<namespace>/<name>" of a service-backed webhook; URL marks
	// a webhook called by URL (its reachability is not checked).
	Service       string `json:"service,omitempty"`
	URL           bool   `json:"url,omitempty"`
	FailurePolicy string `json:"failurePolicy"`
	SideEffects   string `json:"sideEffects"`
	// Covers: the rules and selectors include the object (CREATE).
	Covers bool `json:"covers"`
	// Ready: the service has a ready endpoint.
	Ready  bool   `json:"ready"`
	Detail string `json:"detail,omitempty"`
}

// effective reports whether a covering webhook certainly decides the
// request: it can be called, or a failed call rejects.
func (w AdmissionWebhook) effective() bool {
	return w.Covers && (w.Ready || w.FailurePolicy == "Fail")
}

// AdmissionRun is the record of one admission dry run (level observed).
type AdmissionRun struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Validator     string                 `json:"validator"`
	Exercise      string                 `json:"exercise"`
	Product       string                 `json:"product"`
	Version       string                 `json:"version"`
	Object        string                 `json:"object"`
	Namespace     string                 `json:"namespace,omitempty"`
	Level         domain.ValidationLevel `json:"level"`
	Outcome       AdmissionOutcome       `json:"outcome"`
	// Detail says what the outcome rests on (why undetermined).
	Detail string `json:"detail"`
	// Message is the API server's rejection text; RejectedBy names who
	// rejected: "webhook <name>" or "api-server" (schema or built-in
	// validation, before any validating webhook).
	Message    string             `json:"message,omitempty"`
	RejectedBy string             `json:"rejectedBy,omitempty"`
	Webhooks   []AdmissionWebhook `json:"webhooks,omitempty"`
	Attempts   int                `json:"attempts"`
	// Run is the cluster run: status, cluster, node image, tools, the apply
	// phase and the cleanup.
	Run *Run `json:"run"`
	// Artifact is the stored record (none without a store).
	Artifact *domain.ArtifactRef `json:"-"`
}

var (
	webhookDenied = regexp.MustCompile(`admission webhook "([^"]+)" denied the request`)
	serverInvalid = regexp.MustCompile(`(?i)\bis invalid\b|\bInvalid value\b|\bRequired value\b|\bUnsupported value\b`)
)

// DryRunAdmission runs one admission dry run. Problems are recorded in the
// run (undetermined, with why); the error is for invalid input and
// cancellation only.
func (v *Validator) DryRunAdmission(ctx context.Context, spec AdmissionSpec) (*AdmissionRun, error) {
	if spec.ID == "" || spec.Product == "" || spec.Version == "" {
		return nil, fmt.Errorf("runtime: admission dry run needs an id, product and version")
	}
	var ex *DeclaredExercise
	for i := range v.exercises {
		if v.exercises[i].Key() == spec.Exercise {
			ex = &v.exercises[i]
		}
	}
	switch {
	case ex == nil:
		return nil, fmt.Errorf("runtime: admission dry run: exercise %q is not declared", spec.Exercise)
	case !ex.admission():
		return nil, fmt.Errorf("runtime: admission dry run: exercise %s is a %s exercise, not an admission dry run", ex.Key(), ex.Exercise.Action)
	case ex.Product != spec.Product:
		return nil, fmt.Errorf("runtime: admission dry run: exercise %s is declared for %s, not %s", ex.Key(), ex.Product, spec.Product)
	}
	objs, _, err := parseManifests(spec.Object, "object", spec.Namespace, false)
	if err != nil {
		return nil, fmt.Errorf("runtime: admission dry run: %w", err)
	}
	if len(objs) != 1 {
		return nil, fmt.Errorf("runtime: admission dry run submits exactly one object, got %d", len(objs))
	}
	obj := objs[0]
	if !ex.targets(obj.Ref) {
		return nil, fmt.Errorf("runtime: admission dry run: %s is not a target of %s (%s/%s)", obj.Ref, ex.Key(), ex.Exercise.Target.Group, ex.Exercise.Target.Kind)
	}
	ctrlName := "controller@" + spec.Version
	ctrl, skipped, err := parseManifests(spec.Controller.Manifests, ctrlName, spec.Controller.Namespace, true)
	if err != nil {
		return nil, fmt.Errorf("runtime: admission dry run: %w", err)
	}
	if len(ctrl) == 0 {
		return nil, fmt.Errorf("runtime: admission dry run: %s has no objects", ctrlName)
	}
	setup, skipped2, err := parseManifests(spec.Setup, "setup", spec.Namespace, false)
	if err != nil {
		return nil, fmt.Errorf("runtime: admission dry run: %w", err)
	}
	prefix := spec.ClusterPrefix
	if prefix == "" {
		prefix = AdmissionClusterPrefix
	}
	b := &domain.EvidenceBundle{ID: spec.ID}
	b.Snapshot.KubernetesVersion = spec.KubernetesVersion
	s := &session{v: v, b: b, g: &domain.RelationshipGraph{ID: "admission", Bundle: spec.ID, Builder: AdmissionName}, prefix: prefix, prepared: true,
		allConsumed: true, run: &Run{SchemaVersion: RunSchemaVersion, Validator: AdmissionName, Bundle: spec.ID, Product: spec.Product, From: spec.Version, To: spec.Version}}
	for _, sk := range append(skipped, skipped2...) {
		s.note("%s", sk)
	}
	s.from, s.to = newManifestSet(), newManifestSet()
	ns := obj.Ref.Namespace
	if ns == "" {
		ns = obj.Namespace
	}
	if ns != "" && ns != "default" && !strings.HasPrefix(ns, "kube-") {
		s.to.add([]Manifest{{Ref: ObjectRef{Version: "v1", Kind: "Namespace", Name: ns}, Target: "(namespace of the object)",
			Doc: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": ns}}}})
	}
	s.to.add(ctrl)
	s.to.add(setup)
	s.run.Targets = append(s.run.Targets,
		TargetUse{Target: ctrlName, Status: domain.CollectionOK, Product: true, To: spec.Version, Namespace: spec.Controller.Namespace, ObjectsTo: len(ctrl), Verified: true},
		TargetUse{Target: "setup", Status: domain.CollectionNotApplicable, Namespace: spec.Namespace, Objects: len(setup), Verified: true})
	ar := &AdmissionRun{SchemaVersion: AdmissionSchemaVersion, Validator: AdmissionName, Exercise: ex.Key(), Product: spec.Product, Version: spec.Version,
		Object: obj.Ref.String(), Namespace: ns, Level: domain.LevelObserved, Outcome: AdmissionUndetermined}
	s.body = func(ctx context.Context, cl Cluster) error { return s.admit(ctx, cl, ar, obj, ns) }
	v.logf("runtime: admission dry run of %s against %s %s (%s)", obj.Ref, spec.Product, spec.Version, spec.ID)
	s.execute(ctx)
	if s.cancelled != nil {
		return nil, s.cancelled
	}
	ar.Run = s.run
	if s.run.Status != RunCompleted {
		// a run that did not complete decides nothing, whatever it saw
		// (also when the cluster could not be deleted afterwards)
		switch {
		case ar.Outcome != AdmissionUndetermined:
			ar.Detail = fmt.Sprintf("the API server answered %s (%s), but the run did not complete: %s", ar.Outcome, ar.Detail, s.run.Reason)
		case ar.Detail == "":
			ar.Detail = fmt.Sprintf("the run did not complete (%s): %s", s.run.Status, s.run.Reason)
		default:
			ar.Detail += fmt.Sprintf(" (run %s: %s)", s.run.Status, s.run.Reason)
		}
		ar.Outcome = AdmissionUndetermined
	}
	if err := s.storeAdmission(context.WithoutCancel(ctx), ar); err != nil {
		return nil, fmt.Errorf("runtime: admission dry run: %w", err)
	}
	v.logf("runtime: admission dry run %s: %s (%s) %s", spec.ID, ar.Outcome, s.run.Status, clip(ar.Detail, 200))
	return ar, nil
}

// admit is the body of an admission run inside its cluster.
func (s *session) admit(ctx context.Context, cl Cluster, ar *AdmissionRun, obj Manifest, ns string) error {
	ac, ok := cl.(AdmissionCluster)
	if !ok {
		ar.Detail = "the cluster driver cannot submit dry-run requests"
		return nil
	}
	s.obs, s.applyErr, s.exRuns = map[string]*Observation{}, map[string]map[string]string{}, map[string]map[string]ExerciseRun{}
	if err := s.apply(ctx, cl, PhaseTo, s.to, nil); err != nil {
		return err
	}
	resource, namespaced, err := discoverResource(ctx, ac, obj.Ref.GVK())
	if err != nil {
		ar.Detail = fmt.Sprintf("the cluster does not serve %s: %v", obj.Ref.GVK(), err)
		return nil
	}
	if !namespaced {
		ns = ""
	} else if ns == "" {
		ns = "default"
	}
	ar.Namespace = ns
	// the covering webhooks, once their services are ready (or the ready timeout)
	deadline := s.v.clock.Now().Add(s.v.readyTimeout)
	for {
		hooks, err := webhooksFor(ctx, cl, obj, ns, resource, namespaced)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			ar.Detail = fmt.Sprintf("the validating webhook configurations could not be read: %v", err)
			return nil
		}
		ar.Webhooks = hooks
		waiting := false
		for _, h := range hooks {
			waiting = waiting || (h.Covers && h.Service != "" && !h.Ready)
		}
		if !waiting || !s.v.clock.Now().Before(deadline) {
			break
		}
		if err := s.v.clock.Sleep(ctx, s.v.pollInterval); err != nil {
			return err
		}
	}
	var effective []string
	for _, h := range ar.Webhooks {
		if !h.Covers {
			continue
		}
		if h.SideEffects != "None" && h.SideEffects != "NoneOnDryRun" {
			ar.Detail = fmt.Sprintf("webhook %s declares sideEffects %s: the API server refuses a dry run that would call it", h.Name, h.SideEffects)
			return nil
		}
		if h.effective() {
			effective = append(effective, h.Name)
		}
	}
	if len(effective) == 0 {
		ar.Detail = fmt.Sprintf("no effective validating webhook of %s %s covers %s (CREATE): a dry run would show only the API server's own validation", s.run.Product, s.run.To, obj.Ref.GVK())
		if len(ar.Webhooks) > 0 {
			ar.Detail += fmt.Sprintf(" (%d webhooks match its type but are excluded by a selector or not ready with failure policy Ignore)", len(ar.Webhooks))
		}
		return nil
	}
	// the dry run, retried while the webhook cannot be called yet
	deadline = s.v.clock.Now().Add(s.v.applyTimeout)
	for {
		ar.Attempts++
		_, err := ac.DryRunApply(ctx, obj, ns)
		if err == nil {
			ar.Outcome = AdmissionAccepted
			ar.Detail = fmt.Sprintf("the API server admitted the dry run of %s with validating webhook(s) %s in its path", obj.Ref, strings.Join(effective, ", "))
			return nil
		}
		msg := strings.TrimSpace(err.Error())
		if m := webhookDenied.FindStringSubmatch(msg); m != nil {
			ar.Outcome, ar.RejectedBy, ar.Message = AdmissionRejected, "webhook "+m[1], clip(msg, 2000)
			ar.Detail = fmt.Sprintf("validating webhook %s denied the dry run of %s", m[1], obj.Ref)
			return nil
		}
		if serverInvalid.MatchString(msg) && !transientApply.MatchString(msg) {
			ar.Outcome, ar.RejectedBy, ar.Message = AdmissionRejected, "api-server", clip(msg, 2000)
			ar.Detail = fmt.Sprintf("the API server's own validation rejected %s before any validating webhook", obj.Ref)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !transientApply.MatchString(msg) || !s.v.clock.Now().Before(deadline) {
			ar.Message = clip(msg, 2000)
			ar.Detail = fmt.Sprintf("the dry run was answered with an error that is neither an admission decision nor a validation failure (after %d attempts)", ar.Attempts)
			return nil
		}
		if err := s.v.clock.Sleep(ctx, s.v.retryInterval); err != nil {
			return err
		}
	}
}

// discoverResource returns the resource name (plural) of a type and
// whether it is namespaced, from the API server's discovery.
func discoverResource(ctx context.Context, ac AdmissionCluster, g GVK) (string, bool, error) {
	path := "/apis/" + g.Group + "/" + g.Version
	if g.Group == "" {
		path = "/api/" + g.Version
	}
	raw, err := ac.Raw(ctx, path)
	if err != nil {
		return "", false, err
	}
	var list struct {
		Resources []struct {
			Name       string `json:"name"`
			Kind       string `json:"kind"`
			Namespaced bool   `json:"namespaced"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return "", false, fmt.Errorf("discovery %s: %w", path, err)
	}
	for _, r := range list.Resources {
		if r.Kind == g.Kind && !strings.Contains(r.Name, "/") {
			return r.Name, r.Namespaced, nil
		}
	}
	return "", false, fmt.Errorf("discovery %s lists no %s", path, g.Kind)
}

var (
	vwcType        = GVK{Group: "admissionregistration.k8s.io", Version: "v1", Kind: "ValidatingWebhookConfiguration"}
	namespaceType  = GVK{Version: "v1", Kind: "Namespace"}
	endpointsType  = GVK{Group: "discovery.k8s.io", Version: "v1", Kind: "EndpointSlice"}
	serviceNameKey = "kubernetes.io/service-name"
)

// webhooksFor lists the validating webhooks whose rules match the object's
// CREATE, with whether their selectors include it and their services are
// ready.
func webhooksFor(ctx context.Context, cl Cluster, obj Manifest, ns, resource string, namespaced bool) ([]AdmissionWebhook, error) {
	cfgs, err := cl.List(ctx, vwcType)
	if err != nil {
		return nil, err
	}
	var nsLabels map[string]string
	if namespaced {
		nss, err := cl.List(ctx, namespaceType)
		if err != nil {
			return nil, err
		}
		for _, n := range nss {
			if r, ok := refOf(n); ok && r.Name == ns {
				nsLabels = labelsOf(n)
			}
		}
		if nsLabels == nil {
			nsLabels = map[string]string{}
		}
		if _, set := nsLabels["kubernetes.io/metadata.name"]; !set {
			nsLabels["kubernetes.io/metadata.name"] = ns
		}
	}
	slices, err := cl.List(ctx, endpointsType)
	if err != nil && err != ErrUnservedKind {
		return nil, err
	}
	ready := map[string]bool{}
	for _, sl := range slices {
		r, _ := refOf(sl)
		svc := labelsOf(sl)[serviceNameKey]
		eps, _ := sl["endpoints"].([]any)
		for _, e := range eps {
			em, _ := e.(map[string]any)
			cond, _ := em["conditions"].(map[string]any)
			if rd, set := cond["ready"].(bool); !set || rd {
				ready[r.Namespace+"/"+svc] = true
			}
		}
	}
	var out []AdmissionWebhook
	for _, cfg := range cfgs {
		cr, _ := refOf(cfg)
		hooks, _ := cfg["webhooks"].([]any)
		for _, x := range hooks {
			h, _ := x.(map[string]any)
			if !rulesMatch(h["rules"], obj.Ref.GVK(), resource, namespaced) {
				continue
			}
			w := AdmissionWebhook{Configuration: cr.Name, Name: str(h["name"]), FailurePolicy: str(h["failurePolicy"]), SideEffects: str(h["sideEffects"])}
			if w.FailurePolicy == "" {
				w.FailurePolicy = "Fail" // the admissionregistration/v1 default
			}
			cc, _ := h["clientConfig"].(map[string]any)
			if svc, ok := cc["service"].(map[string]any); ok {
				w.Service = str(svc["namespace"]) + "/" + str(svc["name"])
				w.Ready = ready[w.Service]
			} else if str(cc["url"]) != "" {
				w.URL = true
				w.Detail = "called by URL: its reachability is not checked"
			}
			covers, why := true, []string{}
			if namespaced {
				if ok, decided := selectorMatches(h["namespaceSelector"], nsLabels); !decided || !ok {
					covers = false
					why = append(why, selectorWhy("namespaceSelector", decided))
				}
			}
			if ok, decided := selectorMatches(h["objectSelector"], labelsOf(obj.Doc)); !decided || !ok {
				covers = false
				why = append(why, selectorWhy("objectSelector", decided))
			}
			if mc, _ := h["matchConditions"].([]any); len(mc) > 0 {
				covers = false
				why = append(why, "it has matchConditions (CEL), which are not evaluated here")
			}
			w.Covers = covers
			if len(why) > 0 {
				w.Detail = strings.TrimPrefix(w.Detail+"; "+strings.Join(why, "; "), "; ")
			}
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Configuration+"/"+out[i].Name < out[j].Configuration+"/"+out[j].Name })
	return out, nil
}

func selectorWhy(field string, decided bool) string {
	if !decided {
		return field + " cannot be evaluated"
	}
	return field + " excludes the object"
}

func str(v any) string { s, _ := v.(string); return s }

func strs(v any) []string {
	raw, _ := v.([]any)
	var out []string
	for _, x := range raw {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// rulesMatch reports whether any rule covers a CREATE of the type.
func rulesMatch(raw any, g GVK, resource string, namespaced bool) bool {
	rules, _ := raw.([]any)
	has := func(xs []string, want ...string) bool {
		for _, x := range xs {
			for _, w := range want {
				if x == w {
					return true
				}
			}
		}
		return false
	}
	for _, x := range rules {
		r, _ := x.(map[string]any)
		scope := str(r["scope"])
		if scope == "Namespaced" && !namespaced || scope == "Cluster" && namespaced {
			continue
		}
		if has(strs(r["apiGroups"]), g.Group, "*") && has(strs(r["apiVersions"]), g.Version, "*") &&
			has(strs(r["operations"]), "CREATE", "*") && has(strs(r["resources"]), resource, "*", "*/*") {
			return true
		}
	}
	return false
}

// selectorMatches evaluates a label selector (matchLabels and
// matchExpressions with In, NotIn, Exists, DoesNotExist); an absent or empty
// selector matches everything. decided is false for an unknown operator.
func selectorMatches(raw any, labels map[string]string) (match, decided bool) {
	sel, _ := raw.(map[string]any)
	if len(sel) == 0 {
		return true, true
	}
	ml, _ := sel["matchLabels"].(map[string]any)
	for k, v := range ml {
		if labels[k] != str(v) {
			return false, true
		}
	}
	exprs, _ := sel["matchExpressions"].([]any)
	for _, x := range exprs {
		e, _ := x.(map[string]any)
		key, vals := str(e["key"]), strs(e["values"])
		val, set := labels[key]
		in := false
		for _, v := range vals {
			in = in || (set && v == val)
		}
		switch str(e["operator"]) {
		case "In":
			if !in {
				return false, true
			}
		case "NotIn":
			if in {
				return false, true
			}
		case "Exists":
			if !set {
				return false, true
			}
		case "DoesNotExist":
			if set {
				return false, true
			}
		default:
			return false, false
		}
	}
	return true, true
}

// storeAdmission stores the record (with its run) as an artifact.
func (s *session) storeAdmission(ctx context.Context, ar *AdmissionRun) error {
	if s.v.store == nil {
		return nil
	}
	b, err := json.MarshalIndent(ar, "", "  ")
	if err != nil {
		return err
	}
	path := "derived/runtime-admission/" + s.b.ID + "/admission.json"
	ref, err := s.v.store.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactValidationOutput, Path: path, Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:" + AdmissionName + "/" + path, Producer: AdmissionName,
		Tags: []string{"runtime", "observed", "admission", s.run.Product, string(ar.Outcome)}}, b)
	if err != nil {
		return fmt.Errorf("store %s: %w", path, err)
	}
	ar.Artifact = &ref
	return nil
}
