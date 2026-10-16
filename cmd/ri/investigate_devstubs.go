//go:build ri_devstubs

package main

import (
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/run/runtest"
)

// TEMPORARY scaffolding for live checks before lane classify merges:
// `go build -tags ri_devstubs`. It wires the runtest placeholder classifier
// (every in-scope item: insufficient-evidence, human review required — the
// fail-closed answer, never a real classification) and review applier.
// Delete this file when classify.New() is wired in investigate.go.
func init() {
	newClassifier = func() (investigate.Classifier, investigate.ReviewApplier, error) {
		return &runtest.Classifier{}, runtest.Reviewer{}, nil
	}
}
