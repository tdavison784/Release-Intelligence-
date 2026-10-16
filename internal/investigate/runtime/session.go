package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// RunStatus is the outcome of a cluster run as a whole.
type RunStatus string

const (
	// RunCompleted: the environment was installed at FROM, upgraded to TO and
	// observed (the upgrade itself may have failed: UpgradeCompleted).
	RunCompleted RunStatus = "completed"
	// RunNotRun: a precondition was missing (tool, docker, disk, complete
	// renders, a pinned node image) — nothing was observed.
	RunNotRun RunStatus = "not-run"
	// RunError: the run broke (cluster creation, the FROM version did not
	// come up, a timeout, the cluster could not be deleted).
	RunError RunStatus = "error"
)

// Observation phases.
const (
	PhaseFrom          = "from"
	PhaseFromExercised = "from-exercised"
	PhaseTo            = "to"
	PhaseToExercised   = "to-exercised"
)

// RunSchemaVersion versions the run artifact.
const RunSchemaVersion = "ri.dev/runtime-run/v1alpha1"

// ClusterPrefix names every cluster this validator creates.
const ClusterPrefix = "ri-runtime-"

// Run is the record of one isolated upgrade of a bundle's environment (the
// run artifact). Every in-scope item's validation record is derived from it.
type Run struct {
	SchemaVersion     string            `json:"schemaVersion"`
	Validator         string            `json:"validator"`
	Bundle            string            `json:"bundle"`
	Product           string            `json:"product"`
	From              string            `json:"from"`
	To                string            `json:"to"`
	Status            RunStatus         `json:"status"`
	Reason            string            `json:"reason,omitempty"`
	Cluster           string            `json:"cluster,omitempty"`
	NodeImage         string            `json:"nodeImage,omitempty"`
	KubernetesVersion string            `json:"kubernetesVersion,omitempty"`
	Tools             map[string]string `json:"tools,omitempty"`
	FreeDiskBytes     uint64            `json:"freeDiskBytes,omitempty"`
	StartedAt         time.Time         `json:"startedAt"`
	FinishedAt        time.Time         `json:"finishedAt"`
	DurationSeconds   float64           `json:"durationSeconds"`
	BudgetSeconds     float64           `json:"budgetSeconds"`
	Targets           []TargetUse       `json:"targets,omitempty"`
	Notes             []string          `json:"notes,omitempty"`
	Phases            []PhaseLog        `json:"phases,omitempty"`
	Exercises         []ExerciseRun     `json:"exercises,omitempty"`
	// UpgradeCompleted: every object of the product's targets was applied at
	// TO and every product workload became ready.
	UpgradeCompleted bool          `json:"upgradeCompleted"`
	UpgradeProblems  []string      `json:"upgradeProblems,omitempty"`
	Degradations     []Degradation `json:"degradations,omitempty"`
	Cleanup          *CleanupLog   `json:"cleanup,omitempty"`
}

// TargetUse is one render target applied to the cluster.
type TargetUse struct {
	Target string                  `json:"target"`
	Status domain.CollectionStatus `json:"status"`
	// Product: the target deploys the product (an ok pair); otherwise it is a
	// version-independent consumer overlay (not-applicable pair) whose FROM
	// render is applied at both versions.
	Product   bool   `json:"product"`
	From      string `json:"from"`
	To        string `json:"to,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Objects   int    `json:"objects"`
	ObjectsTo int    `json:"objectsTo,omitempty"`
	// Verified: the raw re-renders normalize to exactly the stored artifacts.
	Verified bool `json:"verified"`
}

// PhaseLog records one apply phase.
type PhaseLog struct {
	Phase    string            `json:"phase"`
	Started  time.Time         `json:"started"`
	Finished time.Time         `json:"finished"`
	Applied  int               `json:"applied"`
	Pruned   []string          `json:"pruned,omitempty"`
	Hooks    []string          `json:"hooks,omitempty"`
	Errors   map[string]string `json:"errors,omitempty"`
	Unserved []string          `json:"unserved,omitempty"`
	Retries  int               `json:"retries,omitempty"`
	NotReady []string          `json:"notReady,omitempty"`
	// Reconciled: the second apply pass (a GitOps sync of the unchanged
	// desired state) after the upgrade settled.
	Reconciled bool `json:"reconciled,omitempty"`
}

// CleanupLog records the deletion of the cluster.
type CleanupLog struct {
	Cluster   string   `json:"cluster"`
	Deleted   bool     `json:"deleted"`
	Verified  bool     `json:"verified"` // absent from the driver's cluster list afterwards
	Error     string   `json:"error,omitempty"`
	Remaining []string `json:"remaining,omitempty"`
}

// one cluster at a time, across every validator in the process.
var clusterMu sync.Mutex

// session is the working state of one run.
type session struct {
	v        *Validator
	b        *domain.EvidenceBundle
	g        *domain.RelationshipGraph
	run      *Run
	from, to *manifestSet
	// liveOf maps render ids to the live ids the API server stored them under.
	liveOf    map[string]string
	interest  map[string]bool
	markers   []ObjectRef
	nodeLive  map[string]string // graph node id → live id
	obs       map[string]*Observation
	applyErr  map[string]map[string]string // phase → live id → error
	exercises []DeclaredExercise           // those with a target in the environment
	exRuns    map[string]map[string]ExerciseRun
	inputs    []string
	arts      []domain.ArtifactRef
	runArt    domain.ArtifactRef
	obsArt    domain.ArtifactRef
	exArt     domain.ArtifactRef
	cancelled error
	consumed  map[string]bool
}

func (s *session) note(format string, args ...any) {
	s.run.Notes = append(s.run.Notes, fmt.Sprintf(format, args...))
}

func (s *session) stop(status RunStatus, format string, args ...any) {
	s.run.Status, s.run.Reason = status, fmt.Sprintf(format, args...)
}

// execute runs the whole session; the outcome is in s.run.
func (s *session) execute(parent context.Context) {
	r := s.run
	r.StartedAt = s.v.clock.Now().UTC()
	r.BudgetSeconds = s.v.timeout.Seconds()
	defer func() {
		r.FinishedAt = s.v.clock.Now().UTC()
		r.DurationSeconds = r.FinishedAt.Sub(r.StartedAt).Seconds()
	}()
	ctx, cancel := context.WithTimeout(parent, s.v.timeout)
	defer cancel()
	if !s.prepare(ctx) {
		return
	}
	clusterMu.Lock()
	defer clusterMu.Unlock()
	if !s.preflight(ctx) {
		return
	}
	r.Cluster = ClusterPrefix + domain.ShortHash(s.b.ID)[:8]
	cl, err := s.v.driver.Create(ctx, ClusterSpec{Name: r.Cluster, NodeImage: r.NodeImage, Timeout: s.v.createTimeout})
	defer s.cleanup(parent, r.Cluster)
	if err != nil {
		s.stopFor(ctx, parent, "create cluster", fmt.Errorf("cluster %s could not be created: %w", r.Cluster, err))
		return
	}
	if err := s.phases(ctx, cl); err != nil {
		s.stopFor(ctx, parent, "run", err)
		return
	}
	r.Status = RunCompleted
}

// stopFor records an infrastructure failure, distinguishing the budget and
// the caller's cancellation.
func (s *session) stopFor(ctx, parent context.Context, what string, err error) {
	switch {
	case parent.Err() != nil:
		s.cancelled = parent.Err()
		s.stop(RunError, "cancelled during %s: %v", what, err)
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		s.stop(RunError, "timed out after %s (budget %s) during %s: %v", s.v.clock.Now().Sub(s.run.StartedAt).Round(time.Second), s.v.timeout, what, err)
	default:
		s.stop(RunError, "%v", err)
	}
}

// cleanup deletes the cluster on every exit path — also after a timeout or
// cancellation (it uses a fresh bounded context) — and verifies it is gone.
func (s *session) cleanup(parent context.Context, name string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), s.v.deleteTimeout)
	defer cancel()
	c := &CleanupLog{Cluster: name}
	s.run.Cleanup = c
	if err := s.v.driver.Delete(ctx, name); err != nil {
		c.Error = err.Error()
	} else {
		c.Deleted = true
	}
	names, err := s.v.driver.Clusters(ctx)
	if err != nil {
		c.Error = strings.TrimSpace(c.Error + "; verify: " + err.Error())
	} else {
		c.Verified = !contains(names, name)
		for _, n := range names {
			if strings.HasPrefix(n, ClusterPrefix) {
				c.Remaining = append(c.Remaining, n)
			}
		}
	}
	if !c.Deleted || !c.Verified {
		s.stop(RunError, "the isolated cluster %s could not be deleted (%s) — delete it with `kind delete cluster --name %s`; prior state: %s %s",
			name, c.Error, name, s.run.Status, s.run.Reason)
	}
}

// --- preparation -------------------------------------------------------------------------

// prepare collects the manifests of both versions from verified raw
// re-renders of the bundle's environment render targets.
func (s *session) prepare(ctx context.Context) bool {
	b := s.b
	if s.v.store == nil {
		s.stop(RunNotRun, "no artifact store configured: the stored renders cannot be verified and observations cannot be stored")
		return false
	}
	if s.v.renderer == nil {
		s.stop(RunNotRun, "no renderer configured: the bundle stores normalized renders, and only a verified raw re-render can be applied")
		return false
	}
	if len(b.Renders) == 0 {
		s.stop(RunNotRun, "the bundle has no environment render (renders disabled, or no deployment of %s found): there is nothing to install", s.run.Product)
		return false
	}
	var incomplete []string
	product := 0
	for _, p := range b.Renders {
		switch p.Status {
		case domain.CollectionOK:
			product++
		case domain.CollectionNotApplicable:
		default:
			incomplete = append(incomplete, fmt.Sprintf("%s is %s (%s)", p.Target, p.Status, p.Reason))
		}
	}
	if len(incomplete) > 0 {
		s.stop(RunNotRun, "environment render incomplete — only complete renders can be installed: %s", strings.Join(incomplete, "; "))
		return false
	}
	if product == 0 {
		s.stop(RunNotRun, "no complete environment render of %s: every render target is not-applicable", s.run.Product)
		return false
	}
	s.from, s.to = newManifestSet(), newManifestSet()
	for _, p := range b.Renders {
		tu := TargetUse{Target: p.Target, Status: p.Status, Product: p.Status == domain.CollectionOK, From: p.From, To: p.To}
		sides := []string{"from", "to"}
		if !tu.Product {
			sides = []string{"from"} // version-independent: the same render at both versions
		}
		if p.From == "" || (tu.Product && p.To == "") {
			if p.Status == domain.CollectionNotApplicable && p.From == "" {
				s.note("%s: not-applicable target without a stored render; skipped", p.Target)
				continue
			}
			s.stop(RunNotRun, "render target %s lacks its stored from/to render artifacts", p.Target)
			return false
		}
		for _, side := range sides {
			art := p.From
			if side == "to" {
				art = p.To
			}
			ref, ok := b.Manifest.Lookup(art)
			if !ok {
				s.stop(RunNotRun, "render artifact %s of %s is not in the bundle manifest", art, p.Target)
				return false
			}
			raw, err := s.v.renderer.RawRender(ctx, b, p.Target, side)
			if err != nil {
				s.stop(RunNotRun, "cannot re-render %s (%s) to applyable manifests: %v", p.Target, side, err)
				return false
			}
			if err := verifyRender(ctx, s.v.store, ref, raw); err != nil {
				s.stop(RunNotRun, "%v — the cluster would not run what the bundle's evidence describes", err)
				return false
			}
			ms, skipped, err := parseManifests(raw.Manifests, p.Target, raw.Namespace, tu.Product)
			if err != nil {
				s.stop(RunNotRun, "%v", err)
				return false
			}
			for _, sk := range skipped {
				s.note("%s", sk)
			}
			tu.Namespace = raw.Namespace
			s.inputs = append(s.inputs, art)
			if side == "from" {
				tu.Objects = len(ms)
				s.from.add(ms)
				if !tu.Product {
					s.to.add(ms)
				}
			} else {
				tu.ObjectsTo = len(ms)
				s.to.add(ms)
			}
		}
		tu.Verified = true
		s.run.Targets = append(s.run.Targets, tu)
	}
	for _, n := range append(s.from.notes, s.to.notes...) {
		s.note("%s", n)
	}
	s.inputs = uniqStrings(s.inputs)
	return true
}

// preflight checks tools, disk, the node image and that no other runtime
// cluster exists (one at a time, also across processes).
func (s *session) preflight(ctx context.Context) bool {
	r := s.run
	r.KubernetesVersion = s.b.Snapshot.KubernetesVersion
	img, why := s.v.nodeImage(r.KubernetesVersion)
	if why != "" {
		s.stop(RunNotRun, "%s", why)
		return false
	}
	r.NodeImage = img
	tools, err := s.v.driver.Preflight(ctx)
	r.Tools = tools
	if err != nil {
		s.stop(RunNotRun, "the isolated cluster cannot be created here: %v", err)
		return false
	}
	free, err := s.v.driver.FreeDisk(ctx)
	if err != nil {
		s.stop(RunNotRun, "free disk space could not be determined (%v); refusing to create a cluster", err)
		return false
	}
	r.FreeDiskBytes = free
	if free < s.v.minFreeDisk {
		s.stop(RunNotRun, "only %.1f GiB free disk, below the %.1f GiB threshold: refusing to create a cluster", gib(free), gib(s.v.minFreeDisk))
		return false
	}
	names, err := s.v.driver.Clusters(ctx)
	if err != nil {
		s.stop(RunNotRun, "existing clusters could not be listed: %v", err)
		return false
	}
	for _, n := range names {
		if strings.HasPrefix(n, ClusterPrefix) {
			s.stop(RunNotRun, "another runtime validation cluster exists (%s): one isolated cluster at a time — delete it with `kind delete cluster --name %s` if it was left behind", n, n)
			return false
		}
	}
	return true
}

func gib(b uint64) float64 { return float64(b) / (1 << 30) }

// --- the cluster phases ------------------------------------------------------------------

func (s *session) phases(ctx context.Context, cl Cluster) error {
	s.obs = map[string]*Observation{}
	s.applyErr = map[string]map[string]string{}
	s.exRuns = map[string]map[string]ExerciseRun{}
	// FROM: the environment as it runs today.
	start := s.v.clock.Now()
	if err := s.apply(ctx, cl, PhaseFrom, s.from, nil); err != nil {
		return err
	}
	s.buildInterest()
	ob := s.observer(cl)
	if err := s.settleAndObserve(ctx, ob, PhaseFrom, start, s.v.settle); err != nil {
		return err
	}
	if problems := s.productProblems(PhaseFrom); len(problems) > 0 {
		return fmt.Errorf("the FROM version (%s) did not come up in the isolated cluster, so there is no baseline: %s", s.run.From, strings.Join(problems, "; "))
	}
	s.selectExercises()
	if len(s.exercises) > 0 {
		start = s.v.clock.Now()
		settle := s.exerciseAll(ctx, cl, "from")
		if err := s.settleAndObserve(ctx, ob, PhaseFromExercised, start, settle); err != nil {
			return err
		}
	}
	// TO: the upgrade (helm upgrade / a GitOps sync of the new release).
	start = s.v.clock.Now()
	if err := s.apply(ctx, cl, PhaseTo, s.to, s.from); err != nil {
		return err
	}
	s.buildInterest()
	ob = s.observer(cl)
	if err := s.settleAndObserve(ctx, ob, PhaseTo, start, s.v.settle); err != nil {
		return err
	}
	s.run.UpgradeProblems = s.productProblems(PhaseTo)
	s.run.UpgradeCompleted = len(s.run.UpgradeProblems) == 0
	if len(s.exercises) > 0 {
		start = s.v.clock.Now()
		settle := s.exerciseAll(ctx, cl, "to")
		if err := s.settleAndObserve(ctx, ob, PhaseToExercised, start, settle); err != nil {
			return err
		}
	}
	s.run.Degradations = s.degradations()
	return nil
}

func (s *session) settleAndObserve(ctx context.Context, ob *observer, phase string, since time.Time, settle time.Duration) error {
	if err := s.v.clock.Sleep(ctx, settle); err != nil {
		return err
	}
	o, err := ob.observe(ctx, phase, since, s.v.clock.Now(), s.applyErr[phase])
	if err != nil {
		return fmt.Errorf("observe %s: %w", phase, err)
	}
	s.obs[phase] = o
	return nil
}

// transientApply matches apply errors that a retry can cure while the
// environment comes up (webhooks not serving yet, CRDs not established, a
// namespace terminating). An admission webhook's denial is not among them.
var transientApply = regexp.MustCompile(`(?i)connection refused|no endpoints available|failed calling webhook|context deadline exceeded|i/o timeout|tls handshake|x509: certificate|ensure crds are installed first|no matches for kind|could not find the requested resource|is being terminated|the server is currently unable|etcdserver: request timed out|service unavailable|connection reset|eof$|timeout`)

// unservedApply matches "the type does not exist".
var unservedApply = regexp.MustCompile(`(?i)no matches for kind|could not find the requested resource|ensure crds are installed first|resource mapping not found`)

// apply installs (FROM) or upgrades to (TO) a manifest set: namespaces, CRDs
// (established), then everything else with retries for transient errors;
// at TO, objects the new release no longer renders are pruned and Helm hooks
// re-created, and after the upgrade settled every object is applied once
// more (a GitOps sync of the unchanged desired state: admission re-validates
// existing objects against the new controller).
func (s *session) apply(ctx context.Context, cl Cluster, phase string, set, prev *manifestSet) error {
	pl := PhaseLog{Phase: phase, Started: s.v.clock.Now().UTC(), Errors: map[string]string{}}
	defer func() {
		pl.Finished = s.v.clock.Now().UTC()
		s.run.Phases = append(s.run.Phases, pl)
	}()
	if s.liveOf == nil {
		s.liveOf = map[string]string{}
	}
	deadline := s.v.clock.Now().Add(s.v.applyTimeout)
	ms := set.list()
	// prune what the new release no longer renders (helm upgrade deletes it)
	if prev != nil {
		for _, m := range prev.list() {
			if _, still := set.byID[m.Ref.String()]; still {
				continue
			}
			live := s.live(m.Ref.String())
			r, _ := ParseRef(live)
			if err := cl.Delete(ctx, r); err != nil && ctx.Err() == nil {
				pl.Errors[m.Ref.String()] = "prune: " + err.Error()
			}
			pl.Pruned = append(pl.Pruned, live)
		}
		for _, m := range ms {
			if m.Hook {
				if err := cl.Delete(ctx, s.refOfLive(m)); err != nil && ctx.Err() == nil {
					pl.Errors[m.Ref.String()] = "re-create hook: " + err.Error()
				}
				pl.Hooks = append(pl.Hooks, m.Ref.String())
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	// namespaces first, then CRDs, then the rest
	var nss, crds, rest []Manifest
	defined := map[string]bool{}
	for _, m := range ms {
		switch {
		case m.Ref.Group == "" && m.Ref.Kind == "Namespace":
			nss = append(nss, m)
			defined[m.Ref.Name] = true
		case isCRD(m.Ref):
			crds = append(crds, m)
		default:
			rest = append(rest, m)
		}
	}
	for _, ns := range namespacesOf(ms) {
		if !defined[ns] && ns != "default" && !strings.HasPrefix(ns, "kube-") {
			nss = append(nss, Manifest{Ref: ObjectRef{Version: "v1", Kind: "Namespace", Name: ns},
				Doc: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": ns}}, Target: "(namespace of the environment)"})
		}
	}
	crdTypes := map[string]bool{}
	for _, m := range crds {
		if t, ok := crdType(m); ok {
			crdTypes[t] = true
		}
	}
	errs := map[string]string{}
	for _, group := range [][]Manifest{nss, crds} {
		for k, v := range s.applyRounds(ctx, cl, group, deadline, crdTypes, &pl) {
			errs[k] = v
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if len(crds) > 0 {
		if err := s.waitEstablished(ctx, cl, crds, deadline); err != nil {
			return err
		}
	}
	// Helm's install order, coarsened: what workloads and hooks need
	// (service accounts, RBAC, config, services) before the workloads,
	// jobs, custom resources and webhooks themselves.
	var first, then []Manifest
	for _, m := range rest {
		if installFirst(m.Ref) {
			first = append(first, m)
		} else {
			then = append(then, m)
		}
	}
	for _, group := range [][]Manifest{first, then} {
		for k, v := range s.applyRounds(ctx, cl, group, deadline, crdTypes, &pl) {
			errs[k] = v
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	notReady, err := s.waitReady(ctx, cl, ms)
	if err != nil {
		return err
	}
	pl.NotReady = notReady
	if prev != nil {
		// the reconcile pass: every non-hook object once more, against the
		// upgraded controllers and webhooks
		var again []Manifest
		for _, m := range ms {
			if !m.Hook && !strings.HasPrefix(errs[m.Ref.String()], "unserved: ") {
				again = append(again, m)
			}
		}
		reconcileDeadline := s.v.clock.Now().Add(s.v.reconcileTimeout)
		recon := s.applyRounds(ctx, cl, again, reconcileDeadline, crdTypes, &pl)
		for _, m := range again {
			id := m.Ref.String()
			if e, failed := recon[id]; failed {
				errs[id] = e
			} else {
				delete(errs, id)
			}
		}
		pl.Reconciled = true
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	byLive := map[string]string{}
	for id, e := range errs {
		pl.Errors[id] = e
		byLive[s.live(id)] = e
	}
	if len(pl.Errors) == 0 {
		pl.Errors = nil
	}
	s.applyErr[phase] = byLive
	return nil
}

// refOfLive returns the live ref of a manifest (its render ref until applied).
func (s *session) refOfLive(m Manifest) ObjectRef {
	r, _ := ParseRef(s.live(m.Ref.String()))
	return r
}

func (s *session) live(renderID string) string {
	if l, ok := s.liveOf[renderID]; ok {
		return l
	}
	return renderID
}

// applyRounds applies manifests in parallel, retrying transient failures
// until they succeed or the deadline passes. It returns the final error of
// every manifest that did not apply, by render id; unserved types are
// prefixed "unserved: ".
func (s *session) applyRounds(ctx context.Context, cl Cluster, ms []Manifest, deadline time.Time, crdTypes map[string]bool, pl *PhaseLog) map[string]string {
	errs := map[string]string{}
	pending := ms
	for len(pending) > 0 {
		type res struct {
			m      Manifest
			stored map[string]any
			err    error
		}
		out := make([]res, len(pending))
		sem := make(chan struct{}, s.v.parallelism)
		var wg sync.WaitGroup
		for i, m := range pending {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int, m Manifest) {
				defer wg.Done()
				defer func() { <-sem }()
				stored, err := cl.Apply(ctx, m, m.Namespace)
				out[i] = res{m, stored, err}
			}(i, m)
		}
		wg.Wait()
		var retry []Manifest
		for _, r := range out {
			id := r.m.Ref.String()
			if r.err == nil {
				pl.Applied++
				delete(errs, id)
				if ref, ok := refOf(r.stored); ok {
					ref.Version = r.m.Ref.Version
					s.liveOf[id] = ref.String()
				}
				continue
			}
			msg := strings.TrimSpace(r.err.Error())
			if unservedApply.MatchString(msg) && !crdTypes[r.m.Ref.typeKey()] {
				errs[id] = "unserved: " + clip(msg, 300)
				pl.Unserved = append(pl.Unserved, id)
				continue
			}
			errs[id] = clip(msg, 600)
			if transientApply.MatchString(msg) {
				retry = append(retry, r.m)
			}
		}
		if len(retry) == 0 || ctx.Err() != nil || !s.v.clock.Now().Before(deadline) {
			break
		}
		pl.Retries++
		if err := s.v.clock.Sleep(ctx, s.v.retryInterval); err != nil {
			break
		}
		pending = retry
	}
	return errs
}

// installFirst reports kinds Helm installs before workloads (its
// InstallOrder: quotas, service accounts, secrets, config, storage, RBAC,
// services).
func installFirst(r ObjectRef) bool {
	switch r.Group {
	case "":
		switch r.Kind {
		case "ResourceQuota", "LimitRange", "ServiceAccount", "Secret", "ConfigMap", "PersistentVolume", "PersistentVolumeClaim", "Service":
			return true
		}
	case "rbac.authorization.k8s.io":
		return true
	case "storage.k8s.io", "policy", "networking.k8s.io":
		return r.Kind == "StorageClass" || r.Kind == "PodDisruptionBudget" || r.Kind == "NetworkPolicy"
	}
	return false
}

func namespacesOf(ms []Manifest) []string {
	set := map[string]bool{}
	for _, m := range ms {
		if m.Ref.Namespace != "" {
			set[m.Ref.Namespace] = true
		}
		if m.Namespace != "" {
			set[m.Namespace] = true
		}
	}
	return sortedKeys(set)
}

// waitEstablished waits until every CRD is established (or the deadline).
func (s *session) waitEstablished(ctx context.Context, cl Cluster, crds []Manifest, deadline time.Time) error {
	want := map[string]bool{}
	for _, m := range crds {
		want[m.Ref.Name] = true
	}
	for {
		items, err := cl.List(ctx, GVK{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"})
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		missing := 0
		got := map[string]bool{}
		for _, it := range items {
			r, _ := refOf(it)
			if ok, _ := readiness(r, it); ok != nil && *ok {
				got[r.Name] = true
			}
		}
		for n := range want {
			if !got[n] {
				missing++
			}
		}
		if missing == 0 || !s.v.clock.Now().Before(deadline) {
			return nil
		}
		if err := s.v.clock.Sleep(ctx, s.v.pollInterval); err != nil {
			return err
		}
	}
}

// waitReady waits until every object with a readiness notion is ready, or
// until the product's workloads are ready and the rest has not changed for
// the stability window, or the ready timeout. It returns what is not ready.
func (s *session) waitReady(ctx context.Context, cl Cluster, ms []Manifest) ([]string, error) {
	deadline := s.v.clock.Now().Add(s.v.readyTimeout)
	types := map[GVK]bool{}
	for _, m := range ms {
		types[m.Ref.GVK()] = true
	}
	var lastSet string
	stableSince := s.v.clock.Now()
	for {
		live := map[string]map[string]any{}
		for g := range types {
			items, err := cl.List(ctx, g)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				continue
			}
			for _, it := range items {
				if r, ok := refOf(it); ok {
					r.Version = g.Version
					live[r.String()] = it
				}
			}
		}
		var notReady []string
		productReady := true
		for _, m := range ms {
			id := s.live(m.Ref.String())
			obj, ok := live[id]
			if !ok {
				continue // not applied: reported as an apply error
			}
			if ready, why := readiness(m.Ref, obj); ready != nil && !*ready {
				notReady = append(notReady, id+": "+why)
				if m.Product && isWorkload(m.Ref) {
					productReady = false
				}
			}
		}
		sort.Strings(notReady)
		if len(notReady) == 0 {
			return nil, nil
		}
		if cur := strings.Join(notReady, "\n"); cur != lastSet {
			lastSet, stableSince = cur, s.v.clock.Now()
		}
		now := s.v.clock.Now()
		if !now.Before(deadline) || (productReady && now.Sub(stableSince) >= s.v.stableWindow) {
			return notReady, nil
		}
		if err := s.v.clock.Sleep(ctx, s.v.pollInterval); err != nil {
			return nil, err
		}
	}
}

// buildInterest sets the objects to observe: every applied object and every
// Kubernetes object the relationship graph names (incl. objects controllers
// produce, e.g. a Certificate's Secret), mapped to live ids.
func (s *session) buildInterest() {
	s.interest, s.nodeLive, s.markers, s.consumed = map[string]bool{}, map[string]string{}, nil, map[string]bool{}
	for _, set := range []*manifestSet{s.from, s.to} {
		for id := range set.byID {
			s.interest[s.live(id)] = true
		}
	}
	if s.g == nil {
		return
	}
	out := map[string]bool{}
	for _, e := range s.g.Edges {
		out[e.From] = true
	}
	for _, n := range s.g.Nodes {
		r, ok := ParseRef(n.Key)
		if !ok {
			continue
		}
		if isMarker(r.Name) {
			s.markers = append(s.markers, r)
			s.nodeLive[n.ID] = r.String() // resolved after an observation
			continue
		}
		live := s.live(n.Key)
		s.interest[live] = true
		s.nodeLive[n.ID] = live
		if out[n.ID] {
			s.consumed[live] = true
		}
	}
	sort.Slice(s.markers, func(i, j int) bool { return s.markers[i].String() < s.markers[j].String() })
}

// resolveNode returns the live id of a graph node ("" when it names no
// Kubernetes object, or a marker no live object matches).
func (s *session) resolveNode(id string) string {
	live := s.nodeLive[id]
	if r, ok := ParseRef(live); ok && isMarker(r.Name) {
		for _, phase := range []string{PhaseFrom, PhaseTo} {
			if o := s.obs[phase]; o != nil {
				for lid := range o.Objects {
					lr, _ := ParseRef(lid)
					if lr.GVK() == r.GVK() && lr.Namespace == r.Namespace && markerMatches(r.Name, lr.Name) {
						return lid
					}
				}
			}
		}
		return ""
	}
	return live
}

func (s *session) observer(cl Cluster) *observer {
	ob := &observer{cl: cl, interest: s.interest, markers: s.markers}
	for _, set := range []*manifestSet{s.from, s.to} {
		for _, m := range set.list() {
			if m.Product && isWorkload(m.Ref) {
				r, _ := ParseRef(s.live(m.Ref.String()))
				ob.controllers = append(ob.controllers, r)
			}
		}
	}
	seen := map[string]bool{}
	var ctrls []ObjectRef
	for _, c := range ob.controllers {
		if !seen[c.String()] {
			seen[c.String()] = true
			ctrls = append(ctrls, c)
		}
	}
	ob.controllers = ctrls
	return ob
}

// productProblems lists why the product is not up at a phase: objects of the
// product's targets that were not applied (unserved types aside) and product
// workloads that are not ready.
func (s *session) productProblems(phase string) []string {
	set := s.from
	if phase == PhaseTo {
		set = s.to
	}
	o := s.obs[phase]
	var out []string
	for _, m := range set.list() {
		if !m.Product {
			continue
		}
		id := s.live(m.Ref.String())
		st := o.Objects[id]
		switch {
		case st.Unserved || strings.HasPrefix(st.ApplyError, "unserved: "):
		case st.ApplyError != "":
			out = append(out, fmt.Sprintf("%s not applied: %s", id, st.ApplyError))
		case isWorkload(m.Ref) && st.Ready != nil && !*st.Ready:
			out = append(out, fmt.Sprintf("%s not ready: %s", id, st.NotReady))
		}
	}
	return out
}

// selectExercises keeps the declared exercises with a target in the
// environment.
func (s *session) selectExercises() {
	o := s.obs[PhaseFrom]
	for _, d := range s.v.exercises {
		for id, st := range o.Objects {
			r, _ := ParseRef(id)
			if st.Exists && d.targets(r) {
				s.exercises = append(s.exercises, d)
				break
			}
		}
	}
}

// exerciseAll runs every selected exercise at one version and returns the
// settle window.
func (s *session) exerciseAll(ctx context.Context, cl Cluster, version string) time.Duration {
	live := map[string]map[string]any{}
	phase := PhaseFrom
	if version == "to" {
		phase = PhaseTo
	}
	o := s.obs[phase]
	// the targets as listed now (a status patch needs the current conditions)
	types := map[GVK]bool{}
	for id := range o.Objects {
		if r, ok := ParseRef(id); ok {
			for _, d := range s.exercises {
				if d.targets(r) {
					types[r.GVK()] = true
				}
			}
		}
	}
	for g := range types {
		items, err := cl.List(ctx, g)
		if err != nil {
			continue
		}
		for _, it := range items {
			if r, ok := refOf(it); ok {
				r.Version = g.Version
				if o.Objects[r.String()].Exists {
					live[r.String()] = it
				}
			}
		}
	}
	settle := time.Duration(0)
	token := ClusterPrefix + version + "-" + domain.ShortHash(s.b.ID, version)[:8]
	runs := map[string]ExerciseRun{}
	for _, d := range s.exercises {
		er := runExercise(ctx, cl, d, version, token, live, s.v.clock.Now())
		st := time.Duration(d.Exercise.SettleSeconds) * time.Second
		if st == 0 {
			st = s.v.settle
		}
		if st > settle {
			settle = st
		}
		er.Settle = st.Seconds()
		runs[d.Key()] = er
		s.run.Exercises = append(s.run.Exercises, er)
	}
	s.exRuns[version] = runs
	if settle == 0 {
		settle = s.v.settle
	}
	return settle
}

// --- degradations ------------------------------------------------------------------------

// degradations compares the observations: the upgrade window (the last FROM
// observation → TO) and the exercise window (what the exercises did at TO
// that the same exercises did not do at FROM).
func (s *session) degradations() []Degradation {
	before := s.obs[PhaseFrom]
	if o := s.obs[PhaseFromExercised]; o != nil {
		before = o
	}
	after := s.obs[PhaseTo]
	var out []Degradation
	ids := sortedKeys(keysOf(after.Objects))
	for _, id := range ids {
		known := s.knownLines(id, PhaseFrom, PhaseFromExercised)
		opt := compareOptions{dataDeclared: s.dataDeclared(id), consumed: s.isConsumed(id), removed: s.removed(id), knownWarnings: known.warnings, knownLogs: known.logs}
		out = append(out, degradations(id, "upgrade", before.Objects[id], after.Objects[id], opt)...)
	}
	// an exercise the TO version refuses on an object the FROM version
	// accepted it on (an admission webhook denying the update)
	for key, er := range s.exRuns["to"] {
		from := s.exRuns["from"][key]
		for _, id := range sortedKeys(keysOf(er.Failed)) {
			if contains(from.Executed, id) {
				out = append(out, Degradation{Object: id, Window: "exercise", Signature: "exercise-rejected:" + key,
					Detail: fmt.Sprintf("the exercise %s was refused at the TO version (%s); the same exercise was accepted at the FROM version", key, er.Failed[id])})
			}
		}
	}
	fe, te := s.obs[PhaseFromExercised], s.obs[PhaseToExercised]
	if fe != nil && te != nil {
		for _, id := range sortedKeys(keysOf(te.Objects)) {
			consumed := s.isConsumed(id)
			fromKnown := s.knownLines(id, PhaseFrom)
			baseline := degradations(id, "exercise", s.obs[PhaseFrom].Objects[id], fe.Objects[id],
				compareOptions{samePods: true, consumed: consumed, knownWarnings: fromKnown.warnings, knownLogs: fromKnown.logs})
			explained := map[string]bool{}
			for _, d := range baseline {
				explained[d.Signature] = true
			}
			known := s.knownLines(id, PhaseFrom, PhaseFromExercised, PhaseTo)
			for _, d := range degradations(id, "exercise", after.Objects[id], te.Objects[id],
				compareOptions{samePods: true, consumed: consumed, knownWarnings: known.warnings, knownLogs: known.logs}) {
				if !explained[d.Signature] {
					d.Detail += " — during the exercises at the TO version; the same exercises at the FROM version did not do this"
					out = append(out, d)
				}
			}
		}
	}
	return out
}

type lines struct{ warnings, logs map[string]bool }

func (s *session) knownLines(id string, phases ...string) lines {
	l := lines{warnings: map[string]bool{}, logs: map[string]bool{}}
	for _, p := range phases {
		if o := s.obs[p]; o != nil {
			st := o.Objects[id]
			for _, w := range st.Warnings {
				l.warnings[normalizeLine(w)] = true
			}
			for _, x := range st.Logs {
				l.logs[normalizeLine(x)] = true
			}
		}
	}
	return l
}

// removed: the FROM render has the object and the TO render does not.
func (s *session) removed(live string) bool {
	for id := range s.from.byID {
		if s.live(id) == live {
			_, still := s.to.byID[id]
			return !still
		}
	}
	return false
}

// dataDeclared: the object's data differs between the FROM and TO renders
// (or it is rendered at one version only), so the render diff covers it.
func (s *session) dataDeclared(live string) bool {
	for id := range s.from.byID {
		if s.live(id) == live {
			return renderedDataDiffers(s.from.byID[id], s.to.byID[id], s.to.byID[id].Doc != nil)
		}
	}
	for id := range s.to.byID {
		if s.live(id) == live {
			return true // rendered at TO only
		}
	}
	return false // produced by a controller: the render cannot show it
}

func renderedDataDiffers(a, b Manifest, bOK bool) bool {
	if !bOK {
		return true
	}
	pick := func(m Manifest) string {
		x := map[string]any{}
		for _, f := range []string{"data", "stringData", "binaryData"} {
			if v, ok := m.Doc[f]; ok {
				x[f] = v
			}
		}
		j, _ := json.Marshal(x)
		return string(j)
	}
	return pick(a) != pick(b)
}

// isConsumed: something on the graph reads the object (without a graph,
// every object counts — conservative).
func (s *session) isConsumed(live string) bool {
	if s.g == nil {
		return true
	}
	if s.consumed[live] {
		return true
	}
	for nid, l := range s.nodeLive {
		if r, ok := ParseRef(l); ok && isMarker(r.Name) && s.resolveNode(nid) == live {
			for _, e := range s.g.Edges {
				if e.From == nid {
					return true
				}
			}
		}
	}
	return false
}

func keysOf[V any](m map[string]V) map[string]bool {
	out := map[string]bool{}
	for k := range m {
		out[k] = true
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func uniqStrings(xs []string) []string {
	m := map[string]bool{}
	for _, x := range xs {
		if x != "" {
			m[x] = true
		}
	}
	if len(m) == 0 {
		return nil
	}
	return sortedKeys(m)
}

// --- artifacts ---------------------------------------------------------------------------

// store writes the run, observation and exercise-declaration artifacts.
func (s *session) store(ctx context.Context) error {
	if s.v.store == nil {
		return nil
	}
	dir := "derived/runtime/" + s.b.ID
	put := func(name string, v any, tags ...string) (domain.ArtifactRef, error) {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return domain.ArtifactRef{}, err
		}
		path := dir + "/" + name
		ref, err := s.v.store.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactValidationOutput, Path: path, Origin: domain.OriginDerived,
			MediaType: "application/json", URI: "ri:" + Name + "/" + path, Producer: Name,
			Tags: append([]string{"runtime", "observed", s.run.Product}, tags...)}, b)
		if err != nil {
			return ref, fmt.Errorf("store %s: %w", path, err)
		}
		s.arts = append(s.arts, ref)
		return ref, nil
	}
	var err error
	if s.exArt, err = put("exercises.json", s.v.exercises, "exercises"); err != nil {
		return err
	}
	if s.obs != nil {
		var list []*Observation
		for _, p := range []string{PhaseFrom, PhaseFromExercised, PhaseTo, PhaseToExercised} {
			if o := s.obs[p]; o != nil {
				list = append(list, o)
			}
		}
		if s.obsArt, err = put("observations.json", list, "observations"); err != nil {
			return err
		}
	}
	s.runArt, err = put("run.json", s.run, "run", string(s.run.Status))
	return err
}
