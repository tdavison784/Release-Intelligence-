package upgrade

// Unit tests of the routine-maintenance detector: the routine positives
// (bump/UI/housekeeping/metrics shapes seen in the real false-positive data)
// and, above all, the safety carve-outs — a security-relevant or
// upgrade-relevant item must never classify routine, however loudly it
// matches a bump pattern.

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// noteChange builds a Change as the note pipeline produces it.
func noteChange(title, detail string, opts ...func(*domain.Change)) domain.Change {
	c := domain.Change{
		Title:      title,
		Detail:     detail,
		Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: noteProducer, Rule: "section:test"},
	}
	for _, o := range opts {
		o(&c)
	}
	return c
}

// The upgrade package keeps its documented dependency set (domain + catalog
// only), so routine.go holds the notes-producer identifier as a literal; this
// is where drift would surface.
func TestNoteProducerConstantMatchesNormalize(t *testing.T) {
	if noteProducer != normalize.ProducerNotes {
		t.Fatalf("noteProducer %q drifted from normalize.ProducerNotes %q", noteProducer, normalize.ProducerNotes)
	}
}

func breakingChange(c *domain.Change) { c.Breaking = true }
func actionChange(c *domain.Change)   { c.ActionRequired = true }
func securityChange(c *domain.Change) { c.Category = domain.CategorySecurity }
func computedChange(c *domain.Change) {
	c.Provenance = domain.Provenance{Method: domain.MethodComputed, Producer: Producer, Rule: "computed:test"}
}
func withRefs(rs ...domain.Reference) func(*domain.Change) {
	return func(c *domain.Change) { c.References = rs }
}

// TestRoutinePositive asserts the shapes that ARE routine, keyed by kind.
func TestRoutinePositive(t *testing.T) {
	cases := []struct {
		title, detail string
		kind          string
	}{
		// Vault's per-plugin dependency bumps (the largest FP mass in the dataset)
		{"auth/alicloud: Update plugin to v0.23.1", "", RoutineDependency},
		{"secrets/kv: Update plugin to v0.26.2", "", RoutineDependency},
		{"database/redis: Update plugin to v0.8.1", "", RoutineDependency},
		// long plugin paths and enterprise version suffixes must not break the shape
		{"database/redis-elasticache: Update plugin to v0.9.1", "", RoutineDependency},
		{"secrets/azure: Update plugin to v0.25.1+ent", "", RoutineDependency},
		{"secrets/keymgmt: Update plugin to v0.19.0+ent", "", RoutineDependency},
		// dependabot / renovate shapes
		{"Bump golang.org/x/net from 0.33.0 to 0.34.0", "", RoutineDependency},
		{"chore(deps): bump github.com/go-git/go-git/v5 from 5.13.2 to 5.16.0", "(#22300)", RoutineDependency},
		{"deps: bump containers/image to v5.34.0", "", RoutineDependency},
		{"Bump OpenTelemetry C++ Contrib", "", RoutineDependency},
		// "Label: Bump …" changelog prefixes
		{"Plugin: Bump `goreleaser` to v2", "", RoutineDependency},
		{"Images: Bump Alpine to v3.21", "", RoutineDependency},
		{"Images: Bump `NGINX_BASE` to v0.0.10", "", RoutineDependency},
		// "Update/upgrade <dep> to vN" (possibly after a label prefix, never
		// with extra prose)
		{"Update quic-go to v0.48.2", "", RoutineDependency},
		{"Helm: Upgrade the chart's crds image to v1.0.1", "", RoutineDependency},
		// UI-only work (Vault's second FP mass)
		{"ui: Fix secrets table pagination when switching page sizes", "", RoutineUI},
		{"ui: Update the sidenav design and add top navbar", "", RoutineUI},
		{"UI: Hashi-Built External Plugin Support: Recognize and support Hashi-built plugins", "", RoutineUI},
		{"ui/pki: Fixes certificate parsing of the `key_usage` extension", "", RoutineUI},
		{"ui/activity (enterprise): Reduce requests to the activity export API", "", RoutineUI},
		{"ui/secrets: Secrets engines url paths renamed from '/secrets' to '/secrets-engines'", "", RoutineUI},
		// CI / docs / build churn
		{"docs: Add deployment for AWS NLB Proxy", "", RoutineHousekeeping},
		{"docs: update OpenSSL Roadmap link", "", RoutineHousekeeping},
		{"chore(appset): simplify cluster list code", "", RoutineHousekeeping},
		{"ci: run codegen as part of version bump job", "", RoutineHousekeeping},
		{"build: pin golangci-lint to v1.64", "", RoutineHousekeeping},
		{"Images: Build `s390x` controller", "", RoutineHousekeeping},
		{"Images: Remove NGINX v1.21", "", RoutineHousekeeping},
		{"Images: Use latest Alpine 3.20 everywhere", "", RoutineHousekeeping},
		{"Prepare release v1.2.3", "", RoutineHousekeeping},
		// metric churn
		{"`envoy_cilium_policymap_<node-ip>_<node-id>_update_duration` -> `envoy_cilium_npds_update_duration`", "", RoutineMetrics},
		{"`cilium_node_health_connectivity_status`", "", RoutineMetrics},
		{"`cilium_node_connectivity_status` is now deprecated", "Please use `cilium_node_health_connectivity_status` instead.", RoutineMetrics},
		{"Changed Metrics: The metrics prefix of all Envoy NPDS metrics has been renamed from `envoy_cilium_policymap_` to `envoy_cilium_npds_`", "", RoutineMetrics},
	}
	for _, tc := range cases {
		routine, kind := ClassifyRoutine(noteChange(tc.title, tc.detail))
		if !routine {
			t.Errorf("expected routine: %q", tc.title)
			continue
		}
		if kind != tc.kind {
			t.Errorf("%q: kind = %q, want %q", tc.title, kind, tc.kind)
		}
	}
}

// TestRoutineCarveouts asserts the safety property: none of these may ever be
// routine.
func TestRoutineCarveouts(t *testing.T) {
	cases := []struct {
		name   string
		change domain.Change
	}{
		// a critical bump citing an advisory stays security
		{"bump with GHSA", noteChange("Bump golang.org/x/crypto to v0.35.0 to fix GHSA-xxxx-xxxx-xxxx", "")},
		{"bump with CVE in detail", noteChange("Update quic-go to v0.48.2", "Addresses CVE-2025-22870.")},
		{"bump with CVE reference", noteChange("Bump golang.org/x/net", "",
			withRefs(domain.Reference{Type: "cve", ID: "CVE-2024-45338"}))},
		{"bump classified security by a product rule", noteChange("Bump base image to v3.21", "", securityChange)},
		{"vulnerability wording", noteChange("Update libxml to v2.13", "Fixes a vulnerability in DTD parsing")},
		// a bump that changes defaults is configuration work
		{"bump of a default", noteChange("Bump default max_connections to 500", "")},
		{"upgrade of a default", noteChange("Upgrade the default termination grace period to v2", "")},
		// a bump that carries more than the bump ("This version upgrades
		// Prometheus-Operator to v0.94.0; the operator's ClusterRole no longer
		// grants wildcard verbs", "review the Dex release notes") is upgrade work
		{"component bump with review pointer", noteChange("Dex was upgraded to v2.43.0", "review the Dex release notes if you rely on custom connectors.")},
		{"upgrade note with behaviour change", noteChange("From 90.x to 91.x: This version upgrades Prometheus-Operator to v0.94.0 The operator's ClusterRole no longer grants wildcard verbs", "")},
		// breaking / directive items
		{"breaking bump", noteChange("Bump the minimum Kubernetes version to v1.25", "", breakingChange)},
		{"bump with operator directive", noteChange("Update the CLI to v2.0.0 before upgrading", "", actionChange)},
		{"migration directive", noteChange("Migrate your existing configuration to the new schema", "You must run the migration script.", actionChange)},
		// capability removal dressed as UI churn
		{"ui removes a feature flag", noteChange("ui: remove the PodSecurityPolicy feature flag", "")},
		{"ui removes feature flag (hyphenated)", noteChange("UI: drop support for the legacy feature-flag X", "")},
		// non-note (computed / lifecycle) changes are never eligible
		{"computed image diff", noteChange("Image `redis` now pulled from `public.ecr.aws/docker/library/redis:7.2.7-alpine` (was `redis:7.2.7-alpine`)", "", computedChange)},
		{"computed values diff", noteChange("Helm values: `replicaCount` default changed from 1 to 2", "", computedChange)},
		// genuinely upgrade-relevant statements that merely mention versions
		{"bundled tool bump in the upgrade guide", noteChange("Argo CD v3.1 upgrades the bundled Helm version to 3.18.4", "")},
		{"default lowered", noteChange("The default value of `hubble.tls.auto.certValidityDuration` has been lowered from 1095 days to 365 days", "")},
		// a deprecation or removal wearing a chore/docs label stays upgrade work
		{"chore deprecation", noteChange("chore: deprecate the `--redis-compress` flag in favour of `redis.compression`", "", func(c *domain.Change) { c.Category = domain.CategoryDeprecation })},
		{"docs removal", noteChange("docs: remove instructions for the disabled basic-auth backend", "", func(c *domain.Change) { c.Category = domain.CategoryRemoval })},
		// metric-lookalikes that are not metrics
		{"values key rename", noteChange("`tls.secretsBackend` -> `tls.readSecretsOnlyFromSecretsNamespace`", "")},
		{"flag rename with sentence", noteChange("The previously deprecated `clustermesh-ip-identities-sync-timeout` flag has been removed in favor of `clustermesh-sync-timeout`", "", breakingChange)},
	}
	for _, tc := range cases {
		if routine, kind := ClassifyRoutine(tc.change); routine {
			t.Errorf("%s: must NOT be routine (kind %q)", tc.name, kind)
		}
	}
}

// TestRoutineSummaryShape checks the edge aggregation end to end.
func TestRoutineSummaryShape(t *testing.T) {
	from := bare("v1.0.0")
	to := bare("v1.1.0").
		note("notes", domain.RoleReleaseNotes, "https://notes", "Dependencies", "auth/x: Update plugin to v0.2.0", domain.CategoryDependency).
		note("notes", domain.RoleReleaseNotes, "https://notes", "Dependencies", "auth/y: Update plugin to v0.3.0", domain.CategoryDependency).
		note("notes", domain.RoleReleaseNotes, "https://notes", "UI", "ui: add a settings page", domain.CategoryFeature).
		note("notes", domain.RoleReleaseNotes, "https://notes", "Dependencies", "Bump golang.org/x/crypto to address CVE-2024-45338", domain.CategorySecurity)
	e := mustBuild(t, simpleInput(from, to))
	if e.Routine == nil {
		t.Fatal("no routine summary")
	}
	if e.Routine.Count != 3 || e.Routine.ByKind[RoutineDependency] != 2 || e.Routine.ByKind[RoutineUI] != 1 {
		t.Errorf("routine summary = %+v", e.Routine)
	}
	if e.Routine.Summary != "2 dependency bumps, 1 UI update" {
		t.Errorf("summary = %q", e.Routine.Summary)
	}
	n := 0
	for _, c := range e.Changes {
		if c.Routine {
			n++
			if c.Breaking || c.ActionRequired || c.Category == domain.CategorySecurity {
				t.Errorf("routine change %q is breaking/action/security", c.Title)
			}
		}
	}
	if n != 3 {
		t.Errorf("routine flag set on %d changes, want 3", n)
	}
	// the security bump keeps its narrative place
	sec := findTitle(e, "CVE-2024-45338")
	if sec == nil || sec.Routine {
		t.Error("the security bump must not be routine")
	}
}
