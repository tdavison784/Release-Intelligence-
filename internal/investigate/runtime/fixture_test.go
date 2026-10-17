package runtime

import (
	"context"
	"strings"
	"testing"
)

const fixtureObjects = `apiVersion: gizmo.io/v1
kind: Widget
metadata: {name: arm-present}
spec: {secretName: arm-present-tls, rotation: Never}
---
apiVersion: gizmo.io/v1
kind: Widget
metadata: {name: arm-absent}
spec: {secretName: arm-absent-tls}
`

func fixtureSpec() FixtureSpec {
	return FixtureSpec{ID: "fx-0123456789ab", Product: "gizmo", From: "1.0.0", To: "2.0.0",
		FromController: RawRender{Manifests: []byte(controllerYAML("v1"))}, ToController: RawRender{Manifests: []byte(controllerYAML("v2"))},
		Objects: []byte(fixtureObjects), Namespace: "fixture",
		Observe: []string{"core/v1/Secret/fixture/arm-present-tls", "core/v1/Secret/fixture/arm-absent-tls"}}
}

// A fixture run is an environment run over given manifests: both arms are
// exercised at both versions, and only the arm without the pin shows the
// key rotation at TO (a data difference on the produced Secret, counted
// without a consumer). The cluster is named ri-verify-* and deleted.
func TestRunFixtureObservesBothArms(t *testing.T) {
	drv := &fakeDriver{free: 100 << 30}
	v := New(WithDriver(drv), WithClock(newFakeClock()), WithExercises(renewExercise("behavior")))
	res, err := v.RunFixture(context.Background(), fixtureSpec())
	if err != nil {
		t.Fatal(err)
	}
	r := res.Run
	if r.Status != RunCompleted || !r.UpgradeCompleted {
		t.Fatalf("run %s %s %v", r.Status, r.Reason, r.UpgradeProblems)
	}
	if !strings.HasPrefix(r.Cluster, FixtureClusterPrefix) || r.Cleanup == nil || !r.Cleanup.Deleted || !r.Cleanup.Verified || len(drv.existing) != 0 {
		t.Fatalf("cluster %s cleanup %+v left %v", r.Cluster, r.Cleanup, drv.existing)
	}
	var keyChanged []string
	for _, d := range r.Degradations {
		if d.Signature == "data:key" {
			keyChanged = append(keyChanged, d.Object)
		}
	}
	if len(keyChanged) != 1 || keyChanged[0] != "core/v1/Secret/fixture/arm-absent-tls" {
		t.Fatalf("key rotation observed on %v; want only the arm without the pin (degradations %+v)", keyChanged, r.Degradations)
	}
	reacted := map[string]int{}
	for _, er := range r.Exercises {
		for _, id := range er.Reacted {
			reacted[id]++
		}
	}
	for _, w := range []string{"gizmo.io/v1/Widget/fixture/arm-present", "gizmo.io/v1/Widget/fixture/arm-absent"} {
		if reacted[w] != 2 {
			t.Fatalf("%s reacted %d times, want at FROM and TO (%+v)", w, reacted[w], r.Exercises)
		}
	}
	if len(res.Observations) != 4 {
		t.Fatalf("observations: %d", len(res.Observations))
	}
}

// One isolated cluster at a time across environment and fixture runs.
func TestRunFixtureRefusesWhileAnotherIsolatedClusterExists(t *testing.T) {
	for _, existing := range []string{"ri-runtime-deadbeef", "ri-verify-deadbeef"} {
		drv := &fakeDriver{free: 100 << 30, existing: []string{existing}}
		v := New(WithDriver(drv), WithClock(newFakeClock()), WithExercises(renewExercise("behavior")))
		res, err := v.RunFixture(context.Background(), fixtureSpec())
		if err != nil {
			t.Fatal(err)
		}
		if res.Run.Status != RunNotRun || !strings.Contains(res.Run.Reason, existing) || drv.creates != 0 {
			t.Fatalf("with %s: %s %s (creates %d)", existing, res.Run.Status, res.Run.Reason, drv.creates)
		}
	}
}
