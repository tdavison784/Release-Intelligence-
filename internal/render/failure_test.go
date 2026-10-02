package render

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// fakeRunner answers `helm version` and fails `helm template` with stderr.
type fakeRunner struct {
	missing bool
	stderr  string
}

func (f fakeRunner) LookPath(name string) (string, error) {
	if f.missing {
		return "", errors.New("not found")
	}
	return "/usr/bin/" + name, nil
}

func (f fakeRunner) Run(_ context.Context, _, _ string, args ...string) ([]byte, []byte, error) {
	if len(args) > 0 && args[0] == "version" {
		return []byte("v3.16.1"), nil, nil
	}
	return nil, []byte(f.stderr), errors.New("exit status 1")
}

// TestFailuresAreExplicit: every R13 failure class becomes a failed result
// and a failed pair — never an empty diff ("no change").
func TestFailuresAreExplicit(t *testing.T) {
	cases := []struct {
		runner fakeRunner
		want   FailureReason
	}{
		{fakeRunner{missing: true}, FailRendererUnavailable},
		{fakeRunner{stderr: "Error: found in Chart.yaml, but missing in charts/ directory: postgresql"}, FailMissingDependency},
		{fakeRunner{stderr: "Error: values don't meet the specifications of the schema(s) in the following chart(s):\ndemo:\n- replicaCount: Invalid type"}, FailInvalidValues},
		{fakeRunner{stderr: "Error: chart requires kubeVersion: >= 1.29.0-0 which is incompatible with Kubernetes v1.27.0"}, FailMissingCapability},
		// a chart-authored fail/required check rejects the configuration
		{fakeRunner{stderr: `Error: execution error at (demo/templates/validate.yaml:25:5): containerRuntime.integration was removed in v1.16`}, FailInvalidValues},
		// a template that renders text that is not YAML is a template error
		{fakeRunner{stderr: "Error: YAML parse error on demo/templates/hook.yaml: error converting YAML to JSON: yaml: line 26: found character that cannot start any token"}, FailTemplateError},
		// a broken template is a template error
		{fakeRunner{stderr: `Error: template: demo/templates/deployment.yaml:12:20: executing "demo/templates/deployment.yaml" at <.Values.x.y>: nil pointer evaluating interface {}.y`}, FailTemplateError},
	}
	for _, c := range cases {
		h := &Helm{Runner: c.runner, Now: func() time.Time { return fixedNow }}
		e := &Engine{Helm: h, Charts: dirCharts{root: "testdata/charts"}}
		p := e.RenderPair(context.Background(), PairRequest{Product: "demo", From: "1.0.0", To: "1.1.0", Target: DefaultsTarget("demo"), Scope: domain.RenderRelease})
		if p.Status != PairFailed || p.Diff != nil || p.Failure == nil || p.Failure.Reason != c.want {
			t.Errorf("%q: got %s %+v, want failed/%s", c.runner.stderr, p.Status, p.Failure, c.want)
		}
		if p.Complete() {
			t.Errorf("a failed pair must never count as complete evidence")
		}
	}
	// an unresolvable chart is chart-unavailable
	h := &Helm{Runner: fakeRunner{}, Now: func() time.Time { return fixedNow }}
	e := &Engine{Helm: h, Charts: dirCharts{root: "testdata/charts"}}
	p := e.RenderPair(context.Background(), PairRequest{Product: "demo", From: "0.9.0", To: "1.1.0", Target: DefaultsTarget("demo")})
	if p.Failure == nil || p.Failure.Reason != FailChartUnavailable || !strings.Contains(p.Failure.Detail, "0.9.0") {
		t.Errorf("unresolvable chart: %+v", p.Failure)
	}
}
