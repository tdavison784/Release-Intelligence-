package render

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

var fixedNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// needTool skips the test when a renderer binary is not installed: the
// golden-stream tests cover diff and normalization without the tools.
func needTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s is not on PATH; skipping the live render test (golden-stream tests cover the diff logic)", name)
	}
}

// dirCharts resolves the testdata chart directories demo-<version>.
type dirCharts struct{ root string }

func (d dirCharts) ResolveChart(_ context.Context, product, version string) (*Chart, error) {
	p := filepath.Join(d.root, product+"-"+version)
	if _, err := os.Stat(filepath.Join(p, "Chart.yaml")); err != nil {
		return nil, &Failure{Reason: FailChartUnavailable, Detail: fmt.Sprintf("no chart %s %s", product, version)}
	}
	return &Chart{Name: product, Version: version, Path: p, Digest: "sha256:test-" + product + "-" + version, URI: "testdata/" + product + "-" + version}, nil
}

func testEngine(t *testing.T) *Engine {
	t.Helper()
	h := NewHelm(NewCache(t.TempDir()))
	h.Now = func() time.Time { return fixedNow }
	k := NewKustomize(NewCache(t.TempDir()))
	k.Now = h.Now
	return &Engine{Helm: h, Kustomize: k, Charts: dirCharts{root: "testdata/charts"}}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func findChange(cs []Change, class ChangeClass, name string) *Change {
	for i := range cs {
		if cs[i].Class == class && (name == "" || cs[i].Name == name) {
			return &cs[i]
		}
	}
	return nil
}

func goldenPair(t *testing.T) (from, to []Object) {
	t.Helper()
	f, err := ParseObjects(mustRead(t, "testdata/golden/demo-1.0.0.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := ParseObjects(mustRead(t, "testdata/golden/demo-1.1.0.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return f, g
}

var _ = domain.RenderRelease
