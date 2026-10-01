package main

// The `ri impact --repo` flag: directory mode driven exactly the way a user
// would, against the offline recording, with inputs discovered in the
// miniature customer repository.

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestImpactRepoCommand(t *testing.T) {
	args := []string{
		"-products", filepath.Join("..", "..", "products"), "-offline", "-state", recordedState(t),
		"impact", "cert-manager", "v1.17.0", "v1.18.0",
		"--repo", filepath.Join("..", "..", "internal", "env", "testdata", "customer-repo"),
		"--kubernetes", "1.28",
	}
	out, _, err := runCLI(t, args...)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"51 upstream changes · 5 affect this environment",
		"Cluster Kubernetes 1.28 is below the supported range 1.29–1.33",
		"customer-repo/clusters/prod/values-cert-manager.yaml",
		"is not built with kustomize",
		"repo mode found 2 values files",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
