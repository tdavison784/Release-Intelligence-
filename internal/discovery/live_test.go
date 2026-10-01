package discovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// TestLiveDiscovery runs the real pipeline against GitHub repositories. It
// is skipped unless RI_DISCOVERY_LIVE=1 (unit tests never touch the network).
//
//	RI_DISCOVERY_LIVE=1 RI_DISCOVERY_CACHE=/tmp/ri-cache RI_DISCOVERY_OUT=/tmp/ri-out \
//	  go test ./internal/discovery -run TestLiveDiscovery -v -timeout 30m
//
// Each product yields <id>.yaml and <id>.summary.md; RI_DISCOVERY_FULL=1 also
// writes the full markdown and JSON reports. RI_DISCOVERY_REPOS overrides
// the repository list (comma separated).
// ANTHROPIC_API_KEY, when set, enables the LLM resolver.
func TestLiveDiscovery(t *testing.T) {
	if os.Getenv("RI_DISCOVERY_LIVE") == "" {
		t.Skip("set RI_DISCOVERY_LIVE=1 to run against the network")
	}
	cache := os.Getenv("RI_DISCOVERY_CACHE")
	if cache == "" {
		cache = t.TempDir()
	}
	out := os.Getenv("RI_DISCOVERY_OUT")
	if out == "" {
		out = t.TempDir()
	}
	repos := []string{"cert-manager/cert-manager", "istio/istio", "argoproj/argo-cd"}
	if r := os.Getenv("RI_DISCOVERY_REPOS"); r != "" {
		repos = strings.Split(r, ",")
	}
	gc := NewGitCheckout(cache)
	d := &Discoverer{Checkout: gc, Tags: gc}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		d.LLM = llm.NewAnthropic(key)
	}
	for _, r := range repos {
		start := time.Now()
		res, err := d.Run(context.Background(), Request{Repository: r})
		if err != nil {
			t.Errorf("%s: %v", r, err)
			continue
		}
		if rep := catalog.Validate(res.Definition); !rep.OK() {
			t.Errorf("%s: invalid definition: %v", r, rep.Errors())
		}
		name := res.Definition.ID
		must(t, os.MkdirAll(out, 0o755))
		must(t, os.WriteFile(filepath.Join(out, name+".yaml"), res.YAML, 0o644))
		must(t, os.WriteFile(filepath.Join(out, name+".summary.md"), []byte(res.Report.Summary()), 0o644))
		if os.Getenv("RI_DISCOVERY_FULL") != "" {
			must(t, os.WriteFile(filepath.Join(out, name+".report.md"), []byte(res.Report.Markdown()), 0o644))
			j, _ := json.MarshalIndent(res.Report, "", "  ")
			must(t, os.WriteFile(filepath.Join(out, name+".report.json"), j, 0o644))
		}
		t.Logf("%s: %d sources, %d artifacts, %d candidates in %s", r, len(res.Definition.Sources), len(res.Definition.Artifacts),
			len(res.Report.Candidates), time.Since(start).Round(time.Second))
		for _, c := range res.Report.Coverage {
			t.Logf("  %-18s %-16s %s", c.Target, c.Status, strings.Join(c.Findings, "; "))
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
