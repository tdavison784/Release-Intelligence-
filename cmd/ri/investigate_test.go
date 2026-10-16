package main

import (
	"slices"
	"testing"

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
