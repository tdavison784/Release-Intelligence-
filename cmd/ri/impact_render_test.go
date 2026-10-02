package main

// `ri impact --render`: the minimum workflow of RENDER-MISSION — the impact
// report, then the rendered delta of the customer's configuration against the
// offline recording. Offline the chart packages are not in the cache, so the
// renders fail explicitly (R13: an evidence gap, never "no change"); the
// structure — targets, whys, per-target verdicts — is what this pins down.

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestImpactRenderCommandOffline(t *testing.T) {
	args := []string{
		"-products", filepath.Join("..", "..", "products"), "-offline", "-state", recordedState(t),
		"impact", "cert-manager", "v1.17.0", "v1.18.0",
		"--repo", filepath.Join("..", "..", "internal", "env", "testdata", "customer-repo"),
		"--kubernetes", "1.30", "--render",
	}
	out, _, err := runCLI(t, args...)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Rendered diff: cert-manager v1.17.0 → v1.18.0",
		"Deployment · argocd", "Deployment · flux", "Deployment · helmfile", "Deployment · kustomize",
		"UNKNOWN / RENDER FAILED: chart-unavailable", // offline: not in cache, never "no change"
		"values INCOMPLETE: spec.valuesFrom references cluster objects",
		"why:", // every target says why it was chosen
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in the rendered section:\n%s", want, out[strings.Index(out, "Rendered diff:"):])
		}
	}
	// the kustomize pair fails explicitly too — with which reason depends on
	// the tools this machine has (recordedState scrubs PATH, so usually
	// renderer-unavailable; with kubectl it is the load restrictor).
	if !strings.Contains(out, "renderer-unavailable") && !strings.Contains(out, "kustomize-dependency") {
		t.Errorf("the kustomize target must fail explicitly, never silently:\n%s", out[strings.Index(out, "Deployment · kustomize"):])
	}
}

func TestImpactRenderCommandJSON(t *testing.T) {
	args := []string{
		"-products", filepath.Join("..", "..", "products"), "-offline", "-state", recordedState(t),
		"impact", "cert-manager", "v1.17.0", "v1.18.0",
		"--repo", filepath.Join("..", "..", "internal", "env", "testdata", "customer-repo"),
		"--kubernetes", "1.30", "--render", "-o", "json",
	}
	out, _, err := runCLI(t, args...)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		SchemaVersion string `json:"schemaVersion"`
		Summary       struct {
			AffectEnvironment int `json:"affectEnvironment"`
		} `json:"summary"`
		Render struct {
			Product     string `json:"product"`
			Deployments []struct {
				Status  string `json:"status"`
				Failure *struct {
					Reason string `json:"reason"`
				} `json:"failure"`
				Target struct {
					ID string `json:"id"`
				} `json:"target"`
			} `json:"deployments"`
		} `json:"render"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("impact --render -o json must stay one JSON document (impact fields + render): %v\n%s", err, out[:400])
	}
	if doc.SchemaVersion == "" || doc.Summary.AffectEnvironment == 0 {
		t.Errorf("the impact report must be intact beside the render section: %+v", doc)
	}
	if doc.Render.Product != "cert-manager" {
		t.Errorf("render.product = %q", doc.Render.Product)
	}
	if len(doc.Render.Deployments) < 4 {
		t.Errorf("render.deployments = %d, want the detected deployments", len(doc.Render.Deployments))
	}
	for _, d := range doc.Render.Deployments {
		if d.Status == "failed" && d.Failure == nil {
			t.Errorf("a failed pair must carry its failure: %+v", d)
		}
	}
}
