package main

import (
	"errors"
	"slices"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/investigate/agent"
	rtval "github.com/tdavison784/release-intelligence/internal/investigate/runtime"
)

// The arm C wiring is honest in the stored config: opting into -runtime adds
// the observed validator to the run's Validators list (and stays off for
// every other caller of invFlags, which leave the field nil).
func TestInvFlagsRuntimeValidatorListed(t *testing.T) {
	str := func(s string) *string { return &s }
	off := func(v bool) *bool { return &v }
	base := func() invFlags {
		return invFlags{repo: str(t.TempDir()), values: str(""), manifests: str(""), crds: str(""), images: str(""),
			inventory: str(""), kubernetes: str(""), envName: str("t"), knowledge: str(""), minVerification: str(""),
			render: off(true), agent: off(false), driver: str("claude-cli"), model: str(""), exchange: str(""),
			replay: str(""), script: str(""), claudeBin: str("")}
	}
	r, err := base().request()
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(r.Config.Validators, rtval.Name) {
		t.Fatalf("runtime validator enabled without opting in: %v", r.Config.Validators)
	}
	f := base()
	f.runtime = off(true)
	r, err = f.request()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.Config.Validators, rtval.Name) {
		t.Fatalf("runtime validator missing after opting in: %v", r.Config.Validators)
	}
}

// -agent-prompt pins the agent prompt version in the config (so an earlier
// version's transcripts replay under it); unset it is the newest; an unknown
// version is a usage error, never a silent fallback.
func TestInvFlagsAgentPrompt(t *testing.T) {
	str := func(s string) *string { return &s }
	b := func(v bool) *bool { return &v }
	f := invFlags{repo: str(t.TempDir()), values: str(""), manifests: str(""), crds: str(""), images: str(""),
		inventory: str(""), kubernetes: str(""), envName: str("t"), knowledge: str(""), minVerification: str(""),
		render: b(true), agent: b(true), driver: str("replay"), model: str(""), exchange: str(""),
		replay: str(""), script: str(""), claudeBin: str("")}
	r, err := f.request()
	if err != nil || r.Config.Agent.PromptVersion != agent.DefaultPromptVersion {
		t.Fatalf("default: %v %s", err, r.Config.Agent.PromptVersion)
	}
	f.prompt = str("investigate-agent/v6")
	v6, err := f.request()
	if err != nil || v6.Config.Agent.PromptVersion != "investigate-agent/v6" || v6.Config.Digest() == r.Config.Digest() {
		t.Fatalf("v6: %v %s", err, v6.Config.Agent.PromptVersion)
	}
	f.prompt = str("investigate-agent/v99")
	if _, err := f.request(); !errors.Is(err, app.ErrUsage) {
		t.Fatalf("unknown version: %v", err)
	}
}
