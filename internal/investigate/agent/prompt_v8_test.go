package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/investigate"
)

const promptV8 = "investigate-agent/v8"

// promptSetDigest is sha256 over every file of an embedded prompt version,
// in name order (name, NUL, bytes, NUL).
func promptSetDigest(t *testing.T, version string) string {
	t.Helper()
	dir := "prompts/" + version
	entries, err := promptFS.ReadDir(dir)
	must(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		b, err := promptFS.ReadFile(dir + "/" + n)
		must(t, err)
		h.Write([]byte(n))
		h.Write([]byte{0})
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Stored runs re-verify their prompt digests, so every earlier version is
// frozen byte for byte.
func TestEarlierPromptVersionsAreFrozen(t *testing.T) {
	want := map[string]string{
		"investigate-agent/v1": "94ab8d1d90c9d8b5203ffb29ee98a03ff38632f17eebfa195341ace933154d57",
		"investigate-agent/v2": "5fd7268490d08c866615771fea77962bc0b8a3eb2eed5d3a2fd9bd2b601b9e18",
		"investigate-agent/v3": "2fc7522e1aa0b9a8bbc65ef529b2afce0dbce185ebdfc900e72e0c621018b40e",
		"investigate-agent/v4": "cac2682aa6bbfbbd8a281846918520f2c9e45e3557a6572fbabcd470c38c4436",
		"investigate-agent/v5": "85331d84ddadaa6588aeaf43f8810f1e38cabac9c4d2b37f7689f18ee829272c",
		"investigate-agent/v6": "15cff2638ce18cccafc245b0557e47d953b15088aad0005744865045393772f8",
		"investigate-agent/v7": "18e8cdba89cc7ff73dfd824b42361f8d296ce9644f1a3c889f140d8629e0db43",
	}
	for v, d := range want {
		if _, err := LoadPrompt(v); err != nil {
			t.Errorf("%s: %v", v, err)
		}
		if got := promptSetDigest(t, v); got != d {
			t.Errorf("%s changed (files digest %s, want %s): earlier prompt versions are frozen", v, got, d)
		}
	}
}

// v8 is v7 with a revised system prompt and item task: same features, same
// tool schemas, same whole-agenda task; it is the default.
func TestPromptV8(t *testing.T) {
	v7, err := LoadPrompt(promptV7)
	must(t, err)
	v8, err := LoadPrompt(promptV8)
	must(t, err)
	if v8.Features != v7.Features || !v8.Features.Mitigation || !v8.Features.Claims || !v8.Features.PerItem {
		t.Fatalf("features: v7 %+v, v8 %+v", v7.Features, v8.Features)
	}
	for _, name := range []string{"tools.json", "meta.json", "task.md"} {
		a, err := promptFS.ReadFile("prompts/" + promptV7 + "/" + name)
		must(t, err)
		b, err := promptFS.ReadFile("prompts/" + promptV8 + "/" + name)
		must(t, err)
		if !bytes.Equal(a, b) {
			t.Errorf("v8 %s differs from v7: v8 changes no tool, feature or whole-agenda task", name)
		}
	}
	for _, name := range []string{"system.md", "item.md"} {
		a, _ := promptFS.ReadFile("prompts/" + promptV7 + "/" + name)
		b, _ := promptFS.ReadFile("prompts/" + promptV8 + "/" + name)
		if bytes.Equal(a, b) {
			t.Errorf("v8 %s is v7's", name)
		}
	}
	// the revision's instructions are present
	for _, want := range []string{
		"whether the environment meets\nthe condition on all, some or none of the exposed objects", // mitigation trigger
		"when no exposed object meets it",                    // basis when it holds nowhere
		"consequence a consumer can observe",                 // change vs consequence
		"name the condition under\nwhich it does NOT hold",   // scoped rule
		"Every\ncondition named there that upstream text supports", // wouldOverturn consistency
		"Trace the consumers with the tools before you settle on `potentially-affected`",
		"produced at runtime by a controller and is absent from the snapshot",
	} {
		if !strings.Contains(v8.System, want) {
			t.Errorf("v8 system prompt lacks %q", want)
		}
	}
	if v7.Digest("t") == v8.Digest("t") {
		t.Error("v7 and v8 share a prompt digest")
	}
	if DefaultPromptVersion != promptV8 {
		t.Fatalf("default %s, want %s", DefaultPromptVersion, promptV8)
	}
	if p, err := LoadPrompt(""); err != nil || p.Version != promptV8 {
		t.Fatalf("LoadPrompt(\"\"): %v %v", p, err)
	}
}

// The v8 text names no product, environment, case, kind or field of the
// evaluation: the listed words (case-sensitive, so the English "never" is
// not the enum value "Never"), and every token of the evaluation case
// directory names in any case (names only, never their contents).
func TestPromptV8IsGeneric(t *testing.T) {
	words := []string{"cert-manager", "ingress-nginx", "kube-prometheus-stack", "payments", "shop", "obs",
		"Certificate", "rotationPolicy", "privateKey", "Issuer", "ClusterIssuer", "Ingress", "IngressClass",
		"Prometheus", "Alertmanager", "ServiceMonitor", "PodMonitor", "PrometheusRule", "Never", "Always"}
	anyCase := len(words)
	entries, err := os.ReadDir("../../../eval/investigations")
	must(t, err)
	cases := 0
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "results" {
			continue
		}
		cases++
		for _, tok := range regexp.MustCompile(`[-.]+`).Split(e.Name(), -1) {
			if tok != "" && !regexp.MustCompile(`^[0-9]+$`).MatchString(tok) {
				words = append(words, tok)
			}
		}
	}
	if cases == 0 {
		t.Fatal("no evaluation case directories found")
	}
	files := map[string]string{}
	for _, name := range []string{"system.md", "item.md", "task.md", "tools.json", "meta.json"} {
		b, err := promptFS.ReadFile("prompts/" + promptV8 + "/" + name)
		must(t, err)
		files[name] = string(b)
	}
	for i, w := range words {
		flags := ""
		if i >= anyCase {
			flags = "(?i)"
		}
		re := regexp.MustCompile(flags + `(^|[^A-Za-z0-9])` + regexp.QuoteMeta(w) + `($|[^A-Za-z0-9])`)
		for name, text := range files {
			if loc := re.FindStringIndex(text); loc != nil {
				t.Errorf("v8 %s names %q (%q): prompts are generic", name, w, text[loc[0]:loc[1]])
			}
		}
	}
}

// Under v8 a mitigation whose condition this environment meets on only some
// exposed objects is stored, with the basis naming those objects; one that
// holds on none is stored when its basis names the objects whose state was
// established. An empty basis is still rejected by the unchanged tool layer
// (the hypothesis stays free text and the model is told why).
func TestV8MitigationWithPartialBasisIsStored(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{})
	if len(f.explicit) == 0 || len(f.unset) == 0 {
		t.Fatalf("fixture must mix explicit and unset widgets: %v / %v", f.explicit, f.unset)
	}
	good := f.upstreamCite()
	ref := func(id string) string { // group/version/Kind/ns/name → Kind/ns/name
		p := strings.Split(id, "/")
		return strings.Join(p[len(p)-3:], "/")
	}
	var explicit []string
	for _, id := range f.explicit {
		explicit = append(explicit, ref(id))
	}
	hyp := func(stmt string, basis ...string) Step {
		return Step{Call: "call-8", Tool: investigate.ToolRecordHypothesis, Args: map[string]any{"changeItem": f.item,
			"statement": stmt, "citations": good, "mitigation": widgetMitigation(basis...)}}
	}
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{f.item: {
		hyp("some widgets set the policy explicitly", explicit...),
		hyp("no widget meets the condition", ref(f.unset[0])),
		hyp("nothing named"),
		{Call: "call-8", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.item, "disposition": "claims-mitigation",
			"summary": "explicit widgets keep their storage", "citations": good,
			"verdict": map[string]any{"class": "potentially-affected", "confidence": "medium", "mechanism": "the default flips",
				"environmentReasoning": "some widgets set the old value, one inherits the new default", "wouldOverturn": "w"}})},
	}}}
	cfg := v6Config()
	cfg.PromptVersion = promptV8
	out, err := New(f.st, WithDriver(DriverScripted, d), WithClock(fixedNow)).Investigate(context.Background(), f.input(cfg))
	must(t, err)
	for _, r := range d.Results {
		if r.Err != nil {
			t.Fatalf("%s: %v", r.Tool, r.Err)
		}
	}
	if out.Run.PromptVersion != promptV8 || len(out.Hypotheses) != 3 {
		t.Fatalf("run %s, hypotheses %+v", out.Run.PromptVersion, out.Hypotheses)
	}
	byStmt := map[string]int{}
	for i, h := range out.Hypotheses {
		byStmt[h.Statement] = i
	}
	partial := out.Hypotheses[byStmt["some widgets set the policy explicitly"]]
	if partial.Mitigation == nil || partial.MitigationRejected != "" {
		t.Fatalf("a partial-basis mitigation must be stored: %+v", partial)
	}
	sort.Strings(explicit)
	if strings.Join(partial.Mitigation.EnvironmentBasis, ",") != strings.Join(explicit, ",") {
		t.Fatalf("basis %v, want the explicit widgets %v", partial.Mitigation.EnvironmentBasis, explicit)
	}
	if none := out.Hypotheses[byStmt["no widget meets the condition"]]; none.Mitigation == nil || none.MitigationRejected != "" {
		t.Fatalf("a mitigation whose basis names an object that does not meet it is stored by the tool layer: %+v", none)
	}
	if empty := out.Hypotheses[byStmt["nothing named"]]; empty.Mitigation != nil || !strings.Contains(empty.MitigationRejected, "environmentBasis is required") {
		t.Fatalf("an empty basis must stay rejected (contract unchanged): %+v", empty)
	}
}
