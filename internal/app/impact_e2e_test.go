package app

// End-to-end test of `ri impact`: the recorded cert-manager upgrade of
// e2e_test.go joined with a small checked-in environment
// (testdata/e2e/env/cert-manager), replayed offline with the same fixed
// clock. The goldens live in testdata/e2e/golden/impact/ and are re-derived
// with -update, exactly like the edge goldens.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
)

const envFixture = "testdata/e2e/env/cert-manager"

// impactEnvironment is the environment every impact e2e case parses. Paths
// are relative to the package directory, so evidence URIs in the goldens are
// stable regardless of where the test runs from. The image list is resolved
// the way the CLI resolves --images (a file with one reference per line).
func impactEnvironment() env.Inputs {
	return env.Inputs{
		KubernetesVersion: "1.28",
		ValuesFiles:       []string{filepath.Join(envFixture, "values.yaml")},
		Manifests:         []string{filepath.Join(envFixture, "manifests")},
		CRDs:              []string{filepath.Join(envFixture, "crds")},
		Images:            ImagesList(filepath.Join(envFixture, "images.txt")),
	}
}

// ImagesList reads an images file (one reference per line, # comments).
func ImagesList(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var out []string
	for _, ln := range strings.Split(string(b), "\n") {
		if i := strings.IndexByte(ln, '#'); i >= 0 {
			ln = ln[:i]
		}
		if ln = strings.TrimSpace(ln); ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

func mustImpact(t *testing.T, a *App) *domain.ImpactReport {
	t.Helper()
	rep, err := a.Impact(context.Background(), "cert-manager", "v1.17.0", "v1.18.0", ImpactOptions{Environment: impactEnvironment()})
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	if err := rep.Validate(); err != nil {
		t.Fatalf("report does not validate: %v", err)
	}
	return rep
}

func TestE2EImpact(t *testing.T) {
	rep := mustImpact(t, newReplayApp(t, fixedNow))

	// The funnel: most upstream changes do not touch this environment.
	if rep.Summary.UpstreamChanges != 51 {
		t.Errorf("upstream changes = %d, want 51 (the recorded edge)", rep.Summary.UpstreamChanges)
	}
	if rep.Summary.AffectEnvironment == 0 || rep.Summary.AffectEnvironment > rep.Summary.UpstreamChanges {
		t.Errorf("findings = %d", rep.Summary.AffectEnvironment)
	}

	byRule := map[string][]domain.ImpactFinding{}
	for _, f := range rep.Findings {
		byRule[f.Rule] = append(byRule[f.Rule], f)
	}

	// cluster 1.28 is below the recorded support range 1.29-1.33
	fs := byRule[impact.RuleKubernetesBelow]
	if len(fs) != 1 || fs[0].Classification != domain.ImpactActionRequired {
		t.Fatalf("kubernetes-below findings = %+v", fs)
	}
	if !strings.Contains(fs[0].Title, "1.28") || !strings.Contains(fs[0].Title, "1.29–1.33") {
		t.Errorf("kubernetes finding title = %q", fs[0].Title)
	}
	if len(fs[0].UpstreamEvidence) == 0 || len(fs[0].EnvironmentEvidence) == 0 {
		t.Error("compat finding must cite both chains")
	}

	// the customer pins prometheus.servicemonitor.targetPort, whose default
	// changed 9402 -> "http-metrics": their pin keeps winning
	fs = byRule[impact.RuleValuesPinned]
	if len(fs) != 1 || fs[0].Matches[0].Subject != "prometheus.servicemonitor.targetPort" ||
		fs[0].Classification != domain.ImpactInformational {
		t.Fatalf("values-pinned findings = %+v", fs)
	}

	// global.rbac.disableHTTPChallengesRole is new in v1.18.0 and already set
	fs = byRule[impact.RuleValuesNewKey]
	if len(fs) != 1 || !slices.Contains(matchSubjectsOf(fs[0]), "global.rbac.disableHTTPChallengesRole") {
		t.Fatalf("values-new-key findings = %+v", fs)
	}

	// the deployment pins the old controller image and the mirror list pins
	// the old webhook image; the release ships v1.18.0 of both
	var controllerMoves, webhookMoves int
	for _, f := range byRule[impact.RuleImageChanged] {
		if strings.Contains(f.Title, "quay.io/jetstack/cert-manager-controller:v1.17.0") {
			controllerMoves++
			if f.Classification != domain.ImpactReview {
				t.Errorf("controller image move = %s", f.Classification)
			}
		}
		if strings.Contains(f.Title, "quay.io/jetstack/cert-manager-webhook") {
			webhookMoves++
			if f.Classification != domain.ImpactReview {
				t.Errorf("webhook image finding = %+v", f)
			}
		}
	}
	if controllerMoves != 1 || webhookMoves != 1 {
		t.Errorf("image findings: controller = %d, webhook = %d (findings %+v)", controllerMoves, webhookMoves, byRule[impact.RuleImageChanged])
	}

	// every finding cites environment evidence pointing into the fixture
	up := map[domain.EvidenceID]bool{}
	for _, e := range rep.EnvironmentEvidence {
		up[e.ID] = true
		if e.Kind != domain.EvidenceLocalFile && e.Kind != domain.EvidenceInput {
			t.Errorf("environment evidence kind = %s", e.Kind)
		}
	}
	for _, f := range rep.Findings {
		if len(f.UpstreamEvidence) == 0 || len(f.EnvironmentEvidence) == 0 {
			t.Errorf("finding %s lacks a chain: %+v", f.ID, f)
		}
		for _, id := range f.EnvironmentEvidence {
			if !up[id] {
				t.Errorf("finding %s cites unknown environment evidence %s", f.ID, id)
			}
		}
	}

	var text bytes.Buffer
	if err := impact.RenderText(&text, rep, impact.RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	checkImpactGolden(t, "cert-manager_v1.17.0_v1.18.0.txt", text.Bytes())
	checkImpactGolden(t, "cert-manager_v1.17.0_v1.18.0.json", []byte(impactJSON(t, rep)))
}

// TestE2EImpactDeterminism: the report is a pure function of the recording,
// the definitions, the environment files and the clock.
func TestE2EImpactDeterminism(t *testing.T) {
	a := newReplayApp(t, fixedNow)
	first := impactJSON(t, mustImpact(t, a))
	if got := impactJSON(t, mustImpact(t, a)); got != first {
		t.Fatalf("second run differs:\n%s", firstDiff([]byte(got), []byte(first)))
	}
	later := fixedNow.Add(24 * time.Hour)
	rep := mustImpact(t, newReplayApp(t, later))
	rep.GeneratedAt = fixedNow
	if got := impactJSON(t, rep); got != first {
		t.Fatalf("output depends on the clock beyond generatedAt:\n%s", firstDiff([]byte(got), []byte(first)))
	}
}

// TestE2EImpactGoldensMatchSchema: the golden impact reports conform to the
// published JSON Schema (same contract check as the edge goldens).
func TestE2EImpactGoldensMatchSchema(t *testing.T) {
	schemaPath, err := filepath.Abs(filepath.Join("..", "..", "schemas", "impact-report.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := jsonschema.NewCompiler().Compile(schemaPath)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(e2eDir, "golden", "impact", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden impact reports found: %v", err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(inst); err != nil {
				t.Fatalf("%s does not match schemas/impact-report.schema.json: %v", f, err)
			}
		})
	}
}

func impactJSON(t *testing.T, rep *domain.ImpactReport) string {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rep); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func checkImpactGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join(e2eDir, "golden", "impact", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s (%d bytes)", path, len(got))
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (create it with -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs: %s\n(if the change is intended, run: go test ./internal/app -run TestE2EImpact -update)", path, firstDiff(got, want))
	}
}

// matchSubjectsOf lists the matched environment subjects (test helper).
func matchSubjectsOf(f domain.ImpactFinding) []string {
	var out []string
	for _, m := range f.Matches {
		out = append(out, m.Subject)
	}
	return out
}
