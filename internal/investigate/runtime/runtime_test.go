package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
)

func expect(t *testing.T, r domain.ValidationRecord, outcome domain.ValidationOutcomeKind, rule string, detail ...string) {
	t.Helper()
	if r.Outcome != outcome || r.Provenance.Rule != rule {
		t.Fatalf("got %s/%s, want %s/%s: %s", r.Outcome, r.Provenance.Rule, outcome, rule, r.Detail)
	}
	for _, d := range detail {
		if !strings.Contains(r.Detail, d) {
			t.Fatalf("detail does not mention %q: %s", d, r.Detail)
		}
	}
}

func (e *testEnv) assertDeleted(v *Validator) {
	e.t.Helper()
	run := v.RunRecord(e.b.ID)
	if run == nil || run.Cleanup == nil || !run.Cleanup.Deleted || !run.Cleanup.Verified {
		e.t.Fatalf("cluster not deleted and verified: %+v", run)
	}
	if len(e.drv.existing) != 0 {
		e.t.Fatalf("clusters left: %v", e.drv.existing)
	}
}

// The behaviour change only manifests when the controller acts: the renewal
// exercise rotates the key at v2 (not at v1) under the consuming Deployment.
// A data change under a consumer that does not degrade is inconclusive
// (error), never a fail and never a clearance.
func TestBehaviourChangeUnderExerciseIsInconclusiveWithoutConsumerDegradation(t *testing.T) {
	e := newEnv(t, envOptions{})
	v := e.validator()
	r := e.validate(v, itemBehaviour)
	expect(t, r, domain.ValidationError, RuleInconclusive, `data key "key" changed under a consumer`, "the same exercises at the FROM version did not do this",
		"core/v1/Secret/shop/w1-tls", "consumers apps/v1/Deployment/shop/app: no degradation observed")
	if r.Level != domain.LevelObserved || r.Kind != domain.ValidationRuntime || len(r.Evidence) == 0 || len(r.Outputs) != 2 {
		t.Fatalf("record shape: %+v", r)
	}
	if strings.Contains(r.Detail, b64("key-")) {
		t.Fatal("a data value leaked into the record")
	}
	e.assertDeleted(v)
	// the key did NOT rotate at FROM: crt-only renewal there is explained
	run := v.RunRecord(e.b.ID)
	for _, d := range run.Degradations {
		if d.Signature == "data:crt" {
			t.Fatalf("crt rotation happens at both versions and must be explained by the baseline: %+v", d)
		}
	}
}

// When the consumer does degrade after the data changed under it, the item
// fails, naming the consumer's failure and the data change.
func TestBehaviourChangeBreakingTheConsumerFails(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.drv.setup = func(c *fakeCluster) { c.appBreaksOnKey = true }
	v := e.validator()
	expect(t, e.validate(v, itemBehaviour), domain.ValidationFail, RuleDegraded,
		"apps/v1/Deployment/shop/app", "CrashLoopBackOff", `Also observed (not failures by themselves)`, `data key "key" changed`)
}

// An item reached only as a kind the product's controller reconciles has no
// item-specific exposure: a healthy exercised run is not a pass for it.
func TestReconciledOnlyExposureNeverPasses(t *testing.T) {
	e := newEnv(t, envOptions{rotation: "Never"})
	for i := range e.tr.Roots {
		if e.tr.Roots[i].ChangeItem == itemBehaviour {
			e.tr.Roots[i].Rule = relations.RuleReconciledKind
		}
	}
	v := e.validator()
	expect(t, e.validate(v, itemBehaviour), domain.ValidationNotRun, RuleReconciledOnly, "not a pass")
}

// A pinned setting keeps the old behaviour: the exercise ran at both
// versions and nothing degraded — the only way to pass.
func TestPinnedBehaviourWithExercisePasses(t *testing.T) {
	e := newEnv(t, envOptions{rotation: "Never"})
	v := e.validator()
	r := e.validate(v, itemBehaviour)
	expect(t, r, domain.ValidationPass, RuleQuiet, "gizmo/renew")
	if !r.ClearsBehaviour() {
		t.Fatal("an observed pass clears a behaviour item")
	}
	if len(r.Inputs) == 0 || len(r.Outputs) == 0 {
		t.Fatal("a pass cites its inputs and outputs")
	}
	e.assertDeleted(v)
}

// The core rule: a quiet window without the relevant exercise is never a
// pass — whatever the objects looked like.
func TestQuietWindowWithoutExerciseNeverPasses(t *testing.T) {
	for name, exs := range map[string][]DeclaredExercise{
		"no exercises declared":       nil,
		"exercise for other families": {renewExercise("values")},
		"exercise targets another kind": {func() DeclaredExercise {
			d := renewExercise()
			d.Exercise.Target.Kind = "Gadget"
			return d
		}()},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, envOptions{rotation: "Never"})
			e.exercises = exs
			v := e.validator()
			r := e.validate(v, itemBehaviour)
			expect(t, r, domain.ValidationNotRun, RuleNoExercise, "never a pass")
			// the rendered item has objects on its path but no exercise either
			expect(t, e.validate(v, itemRendered), domain.ValidationNotRun, RuleNoExercise)
		})
	}
	t.Run("exercise declared but it could not run", func(t *testing.T) {
		e := newEnv(t, envOptions{rotation: "Never"})
		e.drv.setup = func(c *fakeCluster) { c.patchErr = errors.New("status subresource is forbidden") }
		v := e.validator()
		expect(t, e.validate(v, itemBehaviour), domain.ValidationNotRun, RuleExerciseNoRun, "not applied to the item's objects at the FROM version")
	})
}

func TestItemWithoutObjectsIsNotRun(t *testing.T) {
	e := newEnv(t, envOptions{})
	v := e.validator()
	expect(t, e.validate(v, itemNoPath), domain.ValidationNotRun, RuleNoObject, "nothing to observe")
}

// Two items reach the degraded objects: the observation cannot say which
// change caused it — inconclusive (error) for both, never a fail.
func TestSharedDegradationIsInconclusive(t *testing.T) {
	e := newEnv(t, envOptions{twin: true})
	v := e.validator()
	for _, it := range []string{itemBehaviour, itemTwin} {
		expect(t, e.validate(v, it), domain.ValidationError, RuleUnattributed, "cannot be attributed to this item alone", "reached by 2 in-scope items")
	}
}

// An exercise degradation reached by an item whose family the exercise does
// not cover is not attributed to it.
func TestExerciseDegradationNotAttributedOutsideItsFamilies(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.exercises = []DeclaredExercise{renewExercise("procedure")}
	v := e.validator()
	expect(t, e.validate(v, itemBehaviour), domain.ValidationNotRun, RuleOtherFamilies, "other change families than behavior")
}

// After the upgrade the new admission webhook rejects the consumer's Widget
// on the reconcile pass: an upgrade-window degradation only this item reaches.
func TestUpgradeRejectionFails(t *testing.T) {
	e := newEnv(t, envOptions{rotation: "Never"})
	e.drv.setup = func(c *fakeCluster) {
		c.rejectAtV2 = func(m Manifest) error {
			if m.Ref.Kind == "Widget" {
				return errors.New(`admission webhook "validate.gizmo.io" denied the request: spec.rotation is no longer supported`)
			}
			return nil
		}
	}
	v := e.validator()
	r := e.validate(v, itemBehaviour)
	expect(t, r, domain.ValidationFail, RuleDegraded, "apply rejected", "denied the request", "upgrade window")
	run := v.RunRecord(e.b.ID)
	var to PhaseLog
	for _, p := range run.Phases {
		if p.Phase == PhaseTo {
			to = p
		}
	}
	if !to.Reconciled || to.Retries != 0 {
		t.Fatalf("a denial is not transient and the reconcile pass ran: %+v", to)
	}
}

// The new controller crash-loops: items whose paths reach it fail; the
// others are inconclusive because the upgrade did not complete.
func TestUpgradeThatBreaksTheController(t *testing.T) {
	e := newEnv(t, envOptions{rotation: "Never"})
	e.drv.setup = func(c *fakeCluster) { c.crashAtV2 = true }
	v := e.validator()
	expect(t, e.validate(v, itemRendered), domain.ValidationFail, RuleDegraded, "became not ready", "CrashLoopBackOff")
	r := e.validate(v, itemRendered)
	if len(r.RenderedChanges) != 1 {
		t.Fatalf("the fail cites the controller's rendered change: %v", r.RenderedChanges)
	}
	expect(t, e.validate(v, itemBehaviour), domain.ValidationError, RuleUpgradeFailed, "did not complete")
}

// An image that cannot be pulled is the sandbox's problem, never a fail.
func TestImagePullIsNeverAFailure(t *testing.T) {
	e := newEnv(t, envOptions{rotation: "Never"})
	e.drv.setup = func(c *fakeCluster) { c.pullFailAtV2 = true }
	v := e.validator()
	expect(t, e.validate(v, itemRendered), domain.ValidationError, RuleSandbox, "ImagePullBackOff")
}

// A consumer already unhealthy at the FROM version gives no baseline.
func TestUnhealthyBaselineIsNotRun(t *testing.T) {
	e := newEnv(t, envOptions{rotation: "Never"})
	e.drv.setup = func(c *fakeCluster) { c.consumerFailed = true }
	v := e.validator()
	expect(t, e.validate(v, itemBehaviour), domain.ValidationNotRun, RuleNoBaseline, "gizmo.io/v1/Widget/shop/w1 at from: not ready")
}

// Warning events and controller error logs that appear only under the TO
// exercise are degradations (generic observations).
func TestNewWarningsAndErrorLogsUnderExercise(t *testing.T) {
	e := newEnv(t, envOptions{rotation: "Never"})
	e.drv.setup = func(c *fakeCluster) { c.warnAtV2Renew, c.logAtV2Renew = true, true }
	v := e.validator()
	r := e.validate(v, itemBehaviour)
	expect(t, r, domain.ValidationFail, RuleDegraded, "warning event: BadConfig", "controller error log naming the object")
}

// A graph key that hides the Secret's name behind a redaction marker still
// resolves to the live Secret.
func TestRedactedNodeKeyResolves(t *testing.T) {
	e := newEnv(t, envOptions{secretMarker: true})
	v := e.validator()
	expect(t, e.validate(v, itemBehaviour), domain.ValidationError, RuleInconclusive, "core/v1/Secret/shop/w1-tls")
}

// --- preconditions: not-run with the reason, never a cluster ------------------------------

func TestPreconditionsNotRun(t *testing.T) {
	cases := map[string]struct {
		mod  func(e *testEnv)
		opts []Option
		want string
	}{
		"missing tool":      {mod: func(e *testEnv) { e.drv.preflight = errors.New("kind is not on PATH") }, want: "kind is not on PATH"},
		"low disk":          {mod: func(e *testEnv) { e.drv.free = 1 << 30 }, want: "below the 10.0 GiB threshold"},
		"another cluster":   {mod: func(e *testEnv) { e.drv.existing = []string{"ri-runtime-deadbeef"} }, want: "one isolated cluster at a time"},
		"incomplete render": {mod: func(e *testEnv) {}, want: "environment render incomplete"},
		"no renders":        {mod: func(e *testEnv) { e.b.Renders = nil }, want: "no environment render"},
		"unpinned kube":     {mod: func(e *testEnv) { e.b.Snapshot.KubernetesVersion = "1.99" }, want: "no pinned kind node image"},
		"re-render differs": {mod: func(e *testEnv) { e.rnd.raw[productTarget+"|to"] = &RawRender{Manifests: []byte(controllerYAML("v3"))} }, want: "does not reproduce stored render"},
		"re-render fails":   {mod: func(e *testEnv) { e.rnd.err = errors.New("chart unavailable offline") }, want: "cannot re-render"},
		"no store":          {opts: []Option{WithStore(nil)}, want: "no artifact store"},
		"no renderer":       {opts: []Option{WithRenderer(nil)}, want: "no renderer configured"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, envOptions{partialRender: name == "incomplete render"})
			if c.mod != nil {
				c.mod(e)
			}
			v := e.validator(c.opts...)
			out, err := v.Validate(context.Background(), investigate.ValidationInput{Bundle: e.b, Graph: e.g, Traversal: e.tr, ChangeItem: itemBehaviour})
			if err != nil {
				t.Fatal(err)
			}
			r := out.Records[0]
			expect(t, r, domain.ValidationNotRun, RuleRunNotRun, c.want)
			if e.drv.creates != 0 {
				t.Fatalf("a cluster was created despite: %s", r.Detail)
			}
			if name == "another cluster" && len(e.drv.deletes) != 0 {
				t.Fatal("someone else's cluster must not be deleted")
			}
		})
	}
}

// --- cleanup on every exit path -----------------------------------------------------------

func TestClusterDeletedOnEveryExitPath(t *testing.T) {
	t.Run("creation fails", func(t *testing.T) {
		e := newEnv(t, envOptions{})
		e.drv.createErr = errors.New("node image pull failed")
		v := e.validator()
		expect(t, e.validate(v, itemBehaviour), domain.ValidationError, RuleRunError, "could not be created", "node image pull failed")
		e.assertDeleted(v)
	})
	t.Run("FROM never comes up", func(t *testing.T) {
		e := newEnv(t, envOptions{})
		e.drv.setup = func(c *fakeCluster) { c.crashAtV1 = true }
		v := e.validator()
		expect(t, e.validate(v, itemBehaviour), domain.ValidationError, RuleRunError, "did not come up", "no baseline")
		e.assertDeleted(v)
	})
	t.Run("timeout", func(t *testing.T) {
		e := newEnv(t, envOptions{})
		e.drv.setup = func(c *fakeCluster) { c.block = true }
		v := e.validator(WithTimeout(50 * time.Millisecond))
		expect(t, e.validate(v, itemBehaviour), domain.ValidationError, RuleRunError, "timed out")
		e.assertDeleted(v)
	})
	t.Run("cancellation", func(t *testing.T) {
		e := newEnv(t, envOptions{})
		ctx, cancel := context.WithCancel(context.Background())
		e.drv.setup = func(c *fakeCluster) {
			c.block = true
			go func() { time.Sleep(20 * time.Millisecond); cancel() }()
		}
		v := e.validator()
		_, err := v.Validate(ctx, investigate.ValidationInput{Bundle: e.b, Graph: e.g, Traversal: e.tr, ChangeItem: itemBehaviour})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation must surface as the context error, got %v", err)
		}
		if len(e.drv.deletes) != 1 || len(e.drv.existing) != 0 {
			t.Fatalf("cluster not deleted after cancellation: deletes %v, left %v", e.drv.deletes, e.drv.existing)
		}
		if v.RunRecord(e.b.ID) != nil {
			t.Fatal("a cancelled run must not be cached")
		}
	})
	t.Run("deletion fails", func(t *testing.T) {
		e := newEnv(t, envOptions{rotation: "Never"})
		e.drv.deleteErr = errors.New("docker is gone")
		v := e.validator()
		expect(t, e.validate(v, itemBehaviour), domain.ValidationError, RuleRunError, "could not be deleted", "kind delete cluster --name ri-runtime-")
	})
	t.Run("deletion not verified", func(t *testing.T) {
		e := newEnv(t, envOptions{rotation: "Never"})
		e.drv.keepOnDel = true
		v := e.validator()
		r := e.validate(v, itemBehaviour)
		expect(t, r, domain.ValidationError, RuleRunError, "could not be deleted")
		if run := v.RunRecord(e.b.ID); run.Cleanup.Verified {
			t.Fatal("a cluster still listed is not verified deleted")
		}
	})
}

// One cluster run serves every item of the bundle, and its records are
// deterministic for the same observations.
func TestOneRunPerBundleAndDeterministicRecords(t *testing.T) {
	ids := map[string]string{}
	for i := 0; i < 2; i++ {
		e := newEnv(t, envOptions{})
		v := e.validator()
		for _, it := range []string{itemBehaviour, itemRendered, itemNoPath} {
			r := e.validate(v, it)
			if prev, ok := ids[it]; ok && prev != r.ID+string(r.Outcome) {
				t.Fatalf("record of %s differs between runs: %s vs %s", it, prev, r.ID+string(r.Outcome))
			}
			ids[it] = r.ID + string(r.Outcome)
		}
		if e.drv.creates != 1 {
			t.Fatalf("%d clusters created for one bundle", e.drv.creates)
		}
		if e.drv.lastCreate.NodeImage != kindNodeImages["1.31"] {
			t.Fatalf("node image not pinned to the environment's Kubernetes version: %q", e.drv.lastCreate.NodeImage)
		}
	}
}

func TestRequestedCoverageAndAnswers(t *testing.T) {
	e := newEnv(t, envOptions{rotation: "Never"})
	v := e.validator()
	req := &domain.ValidationRequest{ID: "vrq-1", Hypothesis: "hyp-1"}
	widget := domain.NodeID(domain.NodeCustomResource, keyWidget)
	out, err := v.Validate(context.Background(), investigate.ValidationInput{Bundle: e.b, Graph: e.g, Traversal: e.tr, ChangeItem: itemBehaviour,
		Subjects: []string{widget}, Request: req})
	if err != nil {
		t.Fatal(err)
	}
	r := out.Records[0]
	if r.Request != "vrq-1" || len(r.Answers) != 1 || len(r.Subjects) != 1 || r.Subjects[0] != widget {
		t.Fatalf("request/answers/subjects: %+v", r)
	}
	expect(t, r, domain.ValidationPass, RuleQuiet)
	if _, err := v.Validate(context.Background(), investigate.ValidationInput{Bundle: e.b, Graph: e.g, Traversal: e.tr, ChangeItem: itemBehaviour,
		Subjects: []string{"gn-unknown"}}); !errors.Is(err, investigate.ErrRejected) {
		t.Fatalf("unknown subject: %v", err)
	}
	if _, err := v.Validate(context.Background(), investigate.ValidationInput{Bundle: e.b, ChangeItem: "ci-nope"}); !errors.Is(err, investigate.ErrNotFound) {
		t.Fatalf("unknown item: %v", err)
	}
}

func TestParseManifestsHooks(t *testing.T) {
	extra := "---\napiVersion: batch/v1\nkind: Job\nmetadata: {name: migrate, namespace: gizmo, annotations: {helm.sh/hook: pre-upgrade}}\nspec: {template: {spec: {restartPolicy: Never, containers: [{name: m, image: \"gizmo:v1\"}]}}}\n" +
		"---\napiVersion: v1\nkind: Pod\nmetadata: {name: test, namespace: gizmo, annotations: {helm.sh/hook: test}}\nspec: {containers: [{name: t, image: x}]}\n" +
		"---\napiVersion: v1\nkind: List\nitems: [{apiVersion: v1, kind: ConfigMap, metadata: {name: a, namespace: gizmo}}]\n"
	ms, skipped, err := parseManifests([]byte(controllerYAML("v1")+extra), productTarget, "gizmo", true)
	if err != nil {
		t.Fatal(err)
	}
	hooks := 0
	for _, m := range ms {
		if m.Hook {
			hooks++
		}
	}
	if len(ms) != 4 || hooks != 1 || len(skipped) != 1 || !strings.Contains(skipped[0], "helm test hook") {
		t.Fatalf("manifests %d hooks %d skipped %v", len(ms), hooks, skipped)
	}
}

// The upgrade prunes what the new release no longer renders (as helm
// upgrade does) and re-creates hooks.
func TestUpgradePrunesAndRecreatesHooks(t *testing.T) {
	e := newEnv(t, envOptions{rotation: "Never"})
	legacy := "---\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: legacy, namespace: gizmo}\ndata: {a: b}\n"
	hook := "---\napiVersion: batch/v1\nkind: Job\nmetadata: {name: migrate, namespace: gizmo, annotations: {helm.sh/hook: pre-upgrade}}\nspec: {template: {spec: {restartPolicy: Never, containers: [{name: m, image: \"gizmo:TAG\"}]}}}\n"
	e.setRender(productTarget, "from", controllerYAML("v1")+legacy+strings.Replace(hook, "TAG", "v1", 1))
	e.setRender(productTarget, "to", controllerYAML("v2")+strings.Replace(hook, "TAG", "v2", 1))
	v := e.validator()
	e.validate(v, itemBehaviour)
	run := v.RunRecord(e.b.ID)
	var to PhaseLog
	for _, p := range run.Phases {
		if p.Phase == PhaseTo {
			to = p
		}
	}
	if len(to.Pruned) != 1 || to.Pruned[0] != "core/v1/ConfigMap/gizmo/legacy" || len(to.Hooks) != 1 {
		t.Fatalf("pruned %v hooks %v", to.Pruned, to.Hooks)
	}
	for _, d := range run.Degradations {
		if d.Object == "core/v1/ConfigMap/gizmo/legacy" {
			t.Fatalf("a pruned object's deletion is the upgrade itself, not a degradation: %+v", d)
		}
	}
	if _, still := e.drv.cluster.objs["core/v1/ConfigMap/gizmo/legacy"]; still {
		t.Fatal("the pruned object still exists")
	}
}

// The new version's admission webhook refuses the exercise's update that the
// old version accepted: a concrete failure on the item's path; an item whose
// paths reach no object but whose family the exercise covers gets an
// inconclusive record (a plausible impact), never a fail or a pass.
func TestExerciseRefusedAtTO(t *testing.T) {
	e := newEnv(t, envOptions{rotation: "Never"})
	e.drv.setup = func(c *fakeCluster) {
		c.patchRejectV2 = errors.New(`admission webhook "validate.gizmo.io" denied the request: risky field`)
	}
	v := e.validator()
	expect(t, e.validate(v, itemBehaviour), domain.ValidationFail, RuleDegraded, "refused at the TO version", "denied the request", "accepted at the FROM version")
	expect(t, e.validate(v, itemNoPath), domain.ValidationError, RuleUnreached, "the relationship graph does not connect")
	// the rendered item's family is not covered by the exercise
	expect(t, e.validate(v, itemRendered), domain.ValidationNotRun, RuleNoExercise)
}

// (review a) An accepted patch is not an exercise: the controller must show
// the declared reaction at both versions for the item to pass.
func TestExerciseRequiresObservedReaction(t *testing.T) {
	t.Run("no reaction declared", func(t *testing.T) {
		e := newEnv(t, envOptions{rotation: "Never"})
		e.exercises[0].Exercise.Reaction = nil
		v := e.validator()
		expect(t, e.validate(v, itemBehaviour), domain.ValidationNotRun, RuleExerciseNoRun, "declares no observable controller reaction")
	})
	t.Run("no reaction at either version", func(t *testing.T) {
		e := newEnv(t, envOptions{rotation: "Never"})
		e.drv.setup = func(c *fakeCluster) { c.ignoreAlways = true }
		v := e.validator()
		expect(t, e.validate(v, itemBehaviour), domain.ValidationNotRun, RuleExerciseNoRun, "no reaction (field status.revision changes)", "FROM version")
	})
	t.Run("reaction at FROM, none at TO", func(t *testing.T) {
		e := newEnv(t, envOptions{rotation: "Never"})
		e.drv.setup = func(c *fakeCluster) { c.ignoreAtV2 = true }
		v := e.validator()
		expect(t, e.validate(v, itemBehaviour), domain.ValidationError, RuleInconclusive, "did not react to the exercise gizmo/renew at the TO version")
	})
	t.Run("reaction at both", func(t *testing.T) {
		e := newEnv(t, envOptions{rotation: "Never"})
		v := e.validator()
		expect(t, e.validate(v, itemBehaviour), domain.ValidationPass, RuleQuiet, "showed the declared reaction to them at both")
		for _, x := range v.RunRecord(e.b.ID).Exercises {
			if len(x.Reacted) != 1 || len(x.Unanswered) != 0 {
				t.Fatalf("exercise run %+v", x)
			}
		}
	})
}

// (review b) The pass states exactly the observations it checked, including
// the ones after the exercises.
func TestPassChecksHealthAfterExercises(t *testing.T) {
	e := newEnv(t, envOptions{rotation: "Never"})
	v := e.validator()
	expect(t, e.validate(v, itemBehaviour), domain.ValidationPass, RuleQuiet, "healthy in each observation (from, from-exercised, to, to-exercised)")
}

// (review e) Requested subjects are checked against the graph; without a
// graph they cannot be, and are rejected.
func TestSubjectsWithoutGraphRejected(t *testing.T) {
	e := newEnv(t, envOptions{})
	v := e.validator()
	_, err := v.Validate(context.Background(), investigate.ValidationInput{Bundle: e.b, Traversal: e.tr, ChangeItem: itemBehaviour, Subjects: []string{"gn-anything"}})
	if !errors.Is(err, investigate.ErrRejected) {
		t.Fatalf("got %v", err)
	}
	if e.drv.creates != 0 {
		t.Fatal("a rejected request must not create a cluster")
	}
}
