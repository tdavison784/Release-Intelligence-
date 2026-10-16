package runtime

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Observation is the generic state of the objects of interest at one point
// of the run (no product-specific code): existence and apply errors, status
// conditions, readiness and pod problems, generation vs observedGeneration,
// Secret/ConfigMap data digests (never values), warning events and error
// log lines of the product's controllers that name the object.
type Observation struct {
	Phase string `json:"phase"`
	// Since is the start of the phase's window: events and log lines are
	// those observed since then.
	Since   time.Time              `json:"since"`
	At      time.Time              `json:"at"`
	Objects map[string]ObjectState `json:"objects"` // by live object id
	Notes   []string               `json:"notes,omitempty"`
}

// Condition is one status condition.
type Condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// ObjectState is what was observed about one object.
type ObjectState struct {
	Exists     bool   `json:"exists"`
	ApplyError string `json:"applyError,omitempty"`
	// Unserved: the cluster serves no API for the type (e.g. a GitOps
	// controller's kind in a cluster without that controller).
	Unserved           bool        `json:"unserved,omitempty"`
	Generation         int64       `json:"generation,omitempty"`
	ObservedGeneration int64       `json:"observedGeneration,omitempty"`
	Conditions         []Condition `json:"conditions,omitempty"`
	// Ready is set for kinds with a readiness notion (workloads, jobs, pods,
	// CRDs, objects with a Ready/Available condition).
	Ready          *bool             `json:"ready,omitempty"`
	NotReady       string            `json:"notReady,omitempty"`
	PodProblems    []string          `json:"podProblems,omitempty"`
	Restarts       int               `json:"restarts,omitempty"`
	Data           map[string]string `json:"data,omitempty"` // key -> digest
	Warnings       []string          `json:"warnings,omitempty"`
	Logs           []string          `json:"logs,omitempty"`
	ImagePullIssue bool              `json:"imagePullIssue,omitempty"`
}

// healthy reports whether the state is a usable baseline: the object exists,
// was applied, is not unready and carries no failure condition.
func (s ObjectState) healthy() (bool, string) {
	switch {
	case s.Unserved:
		return false, "its type is not served in an isolated cluster"
	case !s.Exists:
		if s.ApplyError != "" {
			return false, "not applied: " + s.ApplyError
		}
		return false, "does not exist"
	case s.ApplyError != "":
		return false, "apply rejected: " + s.ApplyError
	case s.Ready != nil && !*s.Ready:
		return false, "not ready: " + s.NotReady
	}
	for _, c := range s.Conditions {
		if negativeCondition(c.Type) && c.Status == "True" {
			return false, fmt.Sprintf("condition %s=True (%s)", c.Type, c.Reason)
		}
	}
	return true, ""
}

// negativeCondition reports condition types whose True status is bad (the
// Kubernetes API conventions name most conditions so that True is good;
// these common ones are the exceptions).
func negativeCondition(t string) bool {
	switch strings.ToLower(t) {
	case "degraded", "failed", "failure", "replicafailure", "stalled", "error", "errored", "unhealthy",
		"diskpressure", "memorypressure", "pidpressure", "networkunavailable", "terminating", "invalid":
		return true
	}
	return false
}

// failureReason matches condition reasons that announce a failure.
var failureReason = regexp.MustCompile(`(?i)fail|error|invalid|denied|reject|forbidden|crashloop|backoff`)

// Degradation is one way an object got worse between two observations.
type Degradation struct {
	Object string `json:"object"`
	// Window is "upgrade" (baseline → after the upgrade) or "exercise"
	// (before → after the exercises at the TO version, compared with what
	// the same exercises did at the FROM version).
	Window string `json:"window"`
	// Signature is the value-free kind of change ("rejected",
	// "condition:Ready", "data:tls.key", "warning:<reason>", ...), the unit
	// of "the baseline explains it".
	Signature string `json:"signature"`
	Detail    string `json:"detail"`
	// Infrastructure marks a degradation of the sandbox, not of the
	// environment (an image that could not be pulled): never a failure.
	Infrastructure bool `json:"infrastructure,omitempty"`
	// Inconclusive marks a difference that is not by itself a failure: data
	// that changed under a consumer. It is a plausible impact (an error
	// record), a failure only together with a degraded consumer.
	Inconclusive bool `json:"inconclusive,omitempty"`
}

// compareOptions say what may be compared between two observations.
type compareOptions struct {
	// samePods: the pods are the same in both (an exercise window), so
	// restart counts are comparable.
	samePods bool
	// dataDeclared: the render declares the object's data change (it
	// differs between the FROM and TO renders): the render diff covers it.
	dataDeclared bool
	// consumed: something on the graph reads the object, so a data change
	// lands under a consumer.
	consumed bool
	// removed: the new release no longer renders the object, so its
	// deletion is the upgrade itself (helm upgrade prunes it), not a
	// degradation; the render diff covers it.
	removed bool
	// warnings/logs seen in the baseline windows (normalized), which
	// explain the same lines later.
	knownWarnings, knownLogs map[string]bool
}

// degradations lists how after is worse than before.
func degradations(key, window string, before, after ObjectState, o compareOptions) []Degradation {
	var out []Degradation
	add := func(sig, detail string, infra bool) {
		out = append(out, Degradation{Object: key, Window: window, Signature: sig, Detail: detail, Infrastructure: infra})
	}
	if after.ApplyError != "" && before.ApplyError == "" {
		add("rejected", "apply rejected: "+after.ApplyError, false)
	}
	if before.Exists && !after.Exists && after.ApplyError == "" && !o.removed {
		add("deleted", "the object no longer exists", false)
	}
	if !before.Exists || !after.Exists {
		return out
	}
	prev := map[string]Condition{}
	for _, c := range before.Conditions {
		prev[c.Type] = c
	}
	for _, c := range after.Conditions {
		p, had := prev[c.Type]
		switch {
		case negativeCondition(c.Type):
			if c.Status == "True" && (!had || p.Status != "True") {
				add("condition:"+c.Type, fmt.Sprintf("condition %s became True (%s: %s)", c.Type, c.Reason, c.Message), false)
			}
		case had && p.Status == "True" && c.Status == "False":
			add("condition:"+c.Type, fmt.Sprintf("condition %s went True → False (%s: %s)", c.Type, c.Reason, c.Message), false)
		case !had && c.Status == "False" && failureReason.MatchString(c.Reason):
			add("condition:"+c.Type, fmt.Sprintf("condition %s appeared False (%s: %s)", c.Type, c.Reason, c.Message), false)
		}
	}
	if before.Ready != nil && *before.Ready && after.Ready != nil && !*after.Ready {
		add("not-ready", "became not ready: "+after.NotReady, after.ImagePullIssue)
	}
	if after.ObservedGeneration > 0 && after.ObservedGeneration < after.Generation &&
		!(before.ObservedGeneration > 0 && before.ObservedGeneration < before.Generation) {
		add("not-reconciled", fmt.Sprintf("not reconciled: observedGeneration %d < generation %d", after.ObservedGeneration, after.Generation), false)
	}
	if o.samePods && after.Restarts > before.Restarts {
		add("restarts", fmt.Sprintf("containers restarted (%d → %d restarts)", before.Restarts, after.Restarts), false)
	}
	if o.consumed && !o.dataDeclared {
		var keys []string
		for k, d := range before.Data {
			if a, ok := after.Data[k]; !ok || a != d {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			what := "changed"
			if _, ok := after.Data[k]; !ok {
				what = "removed"
			}
			add("data:"+k, fmt.Sprintf("data key %q %s under a consumer (digest %s → %s; values are never recorded)", k, what, before.Data[k], after.Data[k]), false)
			out[len(out)-1].Inconclusive = true
		}
	}
	for _, w := range after.Warnings {
		if !o.knownWarnings[normalizeLine(w)] && !contains(before.Warnings, w) {
			add("warning:"+warningReason(w), "warning event: "+w, false)
		}
	}
	for _, l := range after.Logs {
		if !o.knownLogs[normalizeLine(l)] && !contains(before.Logs, l) {
			add("log:"+normalizeLine(l), "controller error log naming the object: "+l, false)
		}
	}
	return out
}

func warningReason(w string) string {
	r, _, _ := strings.Cut(w, ":")
	return strings.TrimSpace(r)
}

var (
	digitsRe = regexp.MustCompile(`[0-9a-f]{6,}|\d+`)
	spaceRe  = regexp.MustCompile(`\s+`)
)

// normalizeLine strips numbers, hashes and timestamps so the same message
// at another time compares equal.
func normalizeLine(s string) string {
	s = digitsRe.ReplaceAllString(s, "#")
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// --- building observations from the cluster -----------------------------------------------

// observer reads the cluster into Observations.
type observer struct {
	cl Cluster
	// interest are the object ids to record (live ids); applied the apply
	// outcome of the phase by live id.
	interest map[string]bool
	// markers are graph keys whose name is a redaction marker; they resolve
	// against live names of the same type and namespace.
	markers []ObjectRef
	// controllers are the product's workloads whose pods' logs are read.
	controllers []ObjectRef
	// key digests Secret/ConfigMap data (per run, never stored).
	key []byte
}

// listAll lists every type of interest, by live id. Unserved types are
// reported in the set.
func (ob *observer) listAll(ctx context.Context) (objs map[string]map[string]any, unserved map[string]bool, notes []string, err error) {
	types := map[GVK]bool{}
	for id := range ob.interest {
		r, ok := ParseRef(id)
		if ok {
			types[r.GVK()] = true
		}
	}
	for _, m := range ob.markers {
		types[m.GVK()] = true
	}
	objs, unserved = map[string]map[string]any{}, map[string]bool{}
	gvks := make([]GVK, 0, len(types))
	for g := range types {
		gvks = append(gvks, g)
	}
	sort.Slice(gvks, func(i, j int) bool { return gvks[i].String() < gvks[j].String() })
	for _, g := range gvks {
		items, err := ob.cl.List(ctx, g)
		if errors.Is(err, ErrUnservedKind) {
			unserved[g.String()] = true
			continue
		}
		if err != nil {
			return nil, nil, nil, fmt.Errorf("list %s: %w", g, err)
		}
		for _, it := range items {
			if r, ok := refOf(it); ok {
				r.Version = g.Version // list at the requested version
				objs[r.String()] = it
			}
		}
	}
	return objs, unserved, notes, nil
}

// resolveMarkers maps marker-named refs to the live ids they hide.
func resolveMarkers(markers []ObjectRef, objs map[string]map[string]any) map[string]string {
	out := map[string]string{}
	for _, m := range markers {
		for id := range objs {
			r, _ := ParseRef(id)
			if r.GVK() == m.GVK() && r.Namespace == m.Namespace && markerMatches(m.Name, r.Name) {
				out[m.String()] = id
			}
		}
	}
	return out
}

// observe records one Observation. applyErrors are the final apply errors of
// the phase by live id.
func (ob *observer) observe(ctx context.Context, phase string, since, now time.Time, applyErrors map[string]string) (*Observation, error) {
	objs, unserved, notes, err := ob.listAll(ctx)
	if err != nil {
		return nil, err
	}
	pods, err := ob.cl.List(ctx, GVK{Version: "v1", Kind: "Pod"})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}
	events, err := ob.cl.List(ctx, GVK{Version: "v1", Kind: "Event"})
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	o := &Observation{Phase: phase, Since: since.UTC(), At: now.UTC(), Objects: map[string]ObjectState{}, Notes: notes}
	warnings := warningEvents(events, since)
	logs := ob.controllerLogs(ctx, objs, pods, since, o)
	ids := map[string]bool{}
	for id := range ob.interest {
		ids[id] = true
	}
	for _, live := range resolveMarkers(ob.markers, objs) {
		ids[live] = true
	}
	for id := range ids {
		r, _ := ParseRef(id)
		st := ObjectState{ApplyError: applyErrors[id], Unserved: unserved[r.GVK().String()]}
		if obj, ok := objs[id]; ok {
			st = stateOf(obj, pods, st, ob.key)
			st.Warnings = warnings[r.Kind+"|"+r.Namespace+"|"+r.Name]
			st.Logs = matchLogs(logs, r)
		}
		o.Objects[id] = st
	}
	return o, nil
}

// stateOf extracts the generic state of a live object.
func stateOf(obj map[string]any, pods []map[string]any, st ObjectState, key []byte) ObjectState {
	st.Exists = true
	meta, _ := obj["metadata"].(map[string]any)
	status, _ := obj["status"].(map[string]any)
	st.Generation = toInt(meta["generation"])
	st.ObservedGeneration = toInt(status["observedGeneration"])
	if raw, ok := status["conditions"].([]any); ok {
		for _, c := range raw {
			cm, _ := c.(map[string]any)
			t, _ := cm["type"].(string)
			if t == "" {
				continue
			}
			s, _ := cm["status"].(string)
			reason, _ := cm["reason"].(string)
			msg, _ := cm["message"].(string)
			st.Conditions = append(st.Conditions, Condition{Type: t, Status: s, Reason: reason, Message: clip(msg, 300)})
		}
		sort.Slice(st.Conditions, func(i, j int) bool { return st.Conditions[i].Type < st.Conditions[j].Type })
	}
	r, _ := refOf(obj)
	switch {
	case r.Kind == "Secret" && r.Group == "":
		st.Data = dataDigests(key, obj, "data", "stringData")
	case r.Kind == "ConfigMap" && r.Group == "":
		st.Data = dataDigests(key, obj, "data", "binaryData")
	}
	ready, why := readiness(r, obj)
	if ready != nil {
		st.Ready, st.NotReady = ready, why
	}
	if isWorkload(r) {
		sel := selectorOf(obj)
		for _, p := range pods {
			pr, _ := refOf(p)
			if pr.Namespace != r.Namespace || !selects(sel, labelsOf(p)) {
				continue
			}
			problems, restarts, pull := podProblems(p)
			st.Restarts += restarts
			st.PodProblems = append(st.PodProblems, problems...)
			st.ImagePullIssue = st.ImagePullIssue || pull
		}
		sort.Strings(st.PodProblems)
		if st.Ready != nil && !*st.Ready && len(st.PodProblems) > 0 {
			st.NotReady += " (" + strings.Join(st.PodProblems, "; ") + ")"
		}
	}
	return st
}

func isWorkload(r ObjectRef) bool {
	return r.Group == "apps" && (r.Kind == "Deployment" || r.Kind == "StatefulSet" || r.Kind == "DaemonSet")
}

// readiness decides readiness for kinds with a readiness notion.
func readiness(r ObjectRef, obj map[string]any) (*bool, string) {
	spec, _ := obj["spec"].(map[string]any)
	status, _ := obj["status"].(map[string]any)
	meta, _ := obj["metadata"].(map[string]any)
	b := func(ok bool, why string) (*bool, string) { return &ok, why }
	switch {
	case r.Group == "apps" && (r.Kind == "Deployment" || r.Kind == "StatefulSet"):
		want := int64(1)
		if v, ok := spec["replicas"]; ok {
			want = toInt(v)
		}
		gen, og := toInt(meta["generation"]), toInt(status["observedGeneration"])
		ready := toInt(status["readyReplicas"])
		updated := toInt(status["updatedReplicas"])
		if r.Kind == "Deployment" {
			ready = toInt(status["availableReplicas"])
		}
		if og < gen {
			return b(false, fmt.Sprintf("the controller has not observed generation %d", gen))
		}
		if ready < want || updated < want {
			return b(false, fmt.Sprintf("%d/%d ready, %d/%d updated", ready, want, updated, want))
		}
		return b(true, "")
	case r.Group == "apps" && r.Kind == "DaemonSet":
		want, ready, updated := toInt(status["desiredNumberScheduled"]), toInt(status["numberReady"]), toInt(status["updatedNumberScheduled"])
		if ready < want || updated < want {
			return b(false, fmt.Sprintf("%d/%d ready, %d/%d updated", ready, want, updated, want))
		}
		return b(true, "")
	case r.Group == "batch" && r.Kind == "Job":
		if toInt(status["succeeded"]) > 0 {
			return b(true, "")
		}
		for _, c := range conditionsOf(status) {
			if c.Type == "Failed" && c.Status == "True" {
				return b(false, "job failed: "+c.Reason+": "+c.Message)
			}
		}
		return b(false, "job not complete")
	case r.Group == "" && r.Kind == "Pod":
		for _, c := range conditionsOf(status) {
			if c.Type == "Ready" {
				return b(c.Status == "True", c.Reason)
			}
		}
		return b(false, "no Ready condition")
	case r.Group == "" && r.Kind == "PersistentVolumeClaim":
		phase, _ := status["phase"].(string)
		return b(phase == "Bound", "phase "+phase)
	case isCRD(r):
		for _, c := range conditionsOf(status) {
			if c.Type == "Established" {
				return b(c.Status == "True", c.Reason)
			}
		}
		return b(false, "not established")
	}
	for _, c := range conditionsOf(status) {
		if c.Type == "Ready" || c.Type == "Available" {
			return b(c.Status == "True", c.Type+"="+c.Status+" "+c.Reason)
		}
	}
	return nil, ""
}

func conditionsOf(status map[string]any) []Condition {
	var out []Condition
	raw, _ := status["conditions"].([]any)
	for _, c := range raw {
		cm, _ := c.(map[string]any)
		t, _ := cm["type"].(string)
		s, _ := cm["status"].(string)
		reason, _ := cm["reason"].(string)
		msg, _ := cm["message"].(string)
		out = append(out, Condition{Type: t, Status: s, Reason: reason, Message: msg})
	}
	return out
}

// podProblems lists waiting/terminated container problems of a pod.
func podProblems(p map[string]any) (problems []string, restarts int, imagePull bool) {
	r, _ := refOf(p)
	status, _ := p["status"].(map[string]any)
	for _, field := range []string{"initContainerStatuses", "containerStatuses"} {
		raw, _ := status[field].([]any)
		for _, c := range raw {
			cm, _ := c.(map[string]any)
			name, _ := cm["name"].(string)
			restarts += int(toInt(cm["restartCount"]))
			state, _ := cm["state"].(map[string]any)
			if w, ok := state["waiting"].(map[string]any); ok {
				reason, _ := w["reason"].(string)
				switch reason {
				case "", "ContainerCreating", "PodInitializing":
				default:
					problems = append(problems, fmt.Sprintf("pod %s container %s: %s", r.Name, name, reason))
					if reason == "ErrImagePull" || reason == "ImagePullBackOff" || reason == "InvalidImageName" {
						imagePull = true
					}
				}
			}
		}
	}
	return problems, restarts, imagePull
}

func selectorOf(obj map[string]any) map[string]string {
	spec, _ := obj["spec"].(map[string]any)
	sel, _ := spec["selector"].(map[string]any)
	ml, _ := sel["matchLabels"].(map[string]any)
	out := map[string]string{}
	for k, v := range ml {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func labelsOf(obj map[string]any) map[string]string {
	meta, _ := obj["metadata"].(map[string]any)
	raw, _ := meta["labels"].(map[string]any)
	out := map[string]string{}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func selects(sel, labels map[string]string) bool {
	if len(sel) == 0 {
		return false
	}
	for k, v := range sel {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func dataDigests(key []byte, obj map[string]any, fields ...string) map[string]string {
	out := map[string]string{}
	for _, f := range fields {
		m, _ := obj[f].(map[string]any)
		for k, v := range m {
			out[k] = digestValue(key, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// warningEvents indexes Warning events since a time by "<kind>|<ns>|<name>"
// of the involved object, as "Reason: message (count)".
func warningEvents(events []map[string]any, since time.Time) map[string][]string {
	out := map[string][]string{}
	seen := map[string]bool{}
	for _, e := range events {
		if t, _ := e["type"].(string); t != "Warning" {
			continue
		}
		last := eventTime(e)
		if !last.IsZero() && last.Before(since) {
			continue
		}
		inv, _ := e["involvedObject"].(map[string]any)
		if inv == nil {
			inv, _ = e["regarding"].(map[string]any)
		}
		kind, _ := inv["kind"].(string)
		ns, _ := inv["namespace"].(string)
		name, _ := inv["name"].(string)
		reason, _ := e["reason"].(string)
		msg, _ := e["message"].(string)
		key := kind + "|" + ns + "|" + name
		line := reason + ": " + clip(strings.TrimSpace(msg), 300)
		if seen[key+"|"+line] {
			continue
		}
		seen[key+"|"+line] = true
		out[key] = append(out[key], line)
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

func eventTime(e map[string]any) time.Time {
	for _, f := range []string{"lastTimestamp", "eventTime", "firstTimestamp"} {
		if s, _ := e[f].(string); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return t
			}
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				return t
			}
		}
	}
	if meta, _ := e["metadata"].(map[string]any); meta != nil {
		if s, _ := meta["creationTimestamp"].(string); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// errorLine matches error-level log lines: klog ("E1016 12:00:00..."),
// logfmt/JSON level fields, and Go log "level=error".
var errorLine = regexp.MustCompile(`^E\d{4} |"level":"(error|fatal)"|level=(error|fatal)|"severity":"(ERROR|error)"| ERROR | error `)

// controllerLogs reads the error-level lines of the product's controller pods
// since a time.
func (ob *observer) controllerLogs(ctx context.Context, objs map[string]map[string]any, pods []map[string]any, since time.Time, o *Observation) []string {
	var lines []string
	for _, w := range ob.controllers {
		wobj, ok := objs[w.String()]
		if !ok {
			continue
		}
		sel := selectorOf(wobj)
		for _, p := range pods {
			pr, _ := refOf(p)
			if pr.Namespace != w.Namespace || !selects(sel, labelsOf(p)) {
				continue
			}
			text, err := ob.cl.Logs(ctx, pr.Namespace, pr.Name, since)
			if err != nil {
				o.Notes = append(o.Notes, fmt.Sprintf("logs of pod %s/%s: %v", pr.Namespace, pr.Name, err))
				continue
			}
			for _, l := range strings.Split(text, "\n") {
				if errorLine.MatchString(l) {
					lines = append(lines, clip(strings.TrimSpace(l), 500))
				}
			}
		}
	}
	return lines
}

// matchLogs returns the lines naming the object ("<ns>/<name>", or the name
// with a namespace field), deduplicated, at most 20.
func matchLogs(lines []string, r ObjectRef) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range lines {
		named := r.Namespace != "" && containsToken(l, r.Namespace+"/"+r.Name)
		if !named && r.Namespace != "" {
			named = strings.Contains(l, `"`+r.Name+`"`) && strings.Contains(l, `"`+r.Namespace+`"`)
		}
		if !named && r.Namespace == "" {
			named = strings.Contains(l, `"`+r.Name+`"`) || strings.Contains(l, "="+r.Name+" ")
		}
		if named && !seen[normalizeLine(l)] {
			seen[normalizeLine(l)] = true
			out = append(out, l)
			if len(out) == 20 {
				break
			}
		}
	}
	return out
}

// containsToken reports whether tok occurs in s delimited by characters that
// cannot be part of a Kubernetes name (so "shop/web" does not match
// "shop/webby").
func containsToken(s, tok string) bool {
	nameChar := func(b byte) bool {
		return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' || b == '.' || b == '/'
	}
	for i := 0; ; {
		j := strings.Index(s[i:], tok)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(tok)
		if (start == 0 || !nameChar(s[start-1])) && (end == len(s) || !nameChar(s[end])) {
			return true
		}
		i = start + 1
	}
}

func toInt(v any) int64 {
	switch t := v.(type) {
	case int:
		return int64(t)
	case int64:
		return t
	case float64:
		return int64(t)
	case uint64:
		return int64(t)
	}
	return 0
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
