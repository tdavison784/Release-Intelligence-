package main

// `ri impact --tier-policy`: offline the renders fail explicitly, so the
// shipped policy must say REVIEW (a render failure is never a pass) and carry
// the rule/invariant that decided it, in text and in JSON.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func policyArgs(t *testing.T, extra ...string) []string {
	return append([]string{
		"-products", filepath.Join("..", "..", "products"), "-offline", "-state", recordedState(t),
		"impact", "cert-manager", "v1.17.0", "v1.18.0",
		"--repo", filepath.Join("..", "..", "internal", "env", "testdata", "customer-repo"),
		"--kubernetes", "1.30", "--render",
	}, extra...)
}

func TestImpactTierPolicyOffline(t *testing.T) {
	out, _, err := runCLI(t, policyArgs(t, "--tier-policy", "default")...)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`Upgrade policy "default"`, "REVIEW", "render-not-complete"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out[strings.Index(out, "Upgrade policy"):])
		}
	}
}

func TestImpactTierPolicyJSON(t *testing.T) {
	out, _, err := runCLI(t, policyArgs(t, "--tier-policy", "default", "-o", "json")...)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Policy struct {
			Tier   string `json:"tier"`
			Policy string `json:"policy"`
			Items  []struct {
				Rule      string `json:"rule"`
				Invariant string `json:"invariant"`
			} `json:"items"`
		} `json:"policy"`
		Render json.RawMessage `json:"render"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("one JSON document expected: %v", err)
	}
	if doc.Policy.Tier != "review" && doc.Policy.Tier != "block" {
		t.Errorf("an offline render failure must never auto-pass: tier = %q", doc.Policy.Tier)
	}
	if doc.Policy.Policy != "default" || len(doc.Policy.Items) == 0 || len(doc.Render) == 0 {
		t.Errorf("policy verdict and render must sit beside the report: %+v", doc.Policy)
	}
}

func TestImpactTierPolicyBadFile(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(bad, []byte("apiVersion: ri.dev/upgrade-policy/v1alpha1\nname: x\ndefault: {tier: maybe, reason: r}\nrules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, policyArgs(t, "--tier-policy", bad)...); err == nil || !strings.Contains(err.Error(), "policy") {
		t.Fatalf("a malformed policy must be refused, got %v", err)
	}
}

func TestImpactWithoutTierPolicyUnchanged(t *testing.T) {
	out, _, err := runCLI(t, policyArgs(t)...)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Upgrade policy") {
		t.Fatal("the verdict section is opt-in")
	}
}
