// Package validate holds the deterministic validators of the Phase 4
// investigation (docs/phase4/env-impact/DESIGN.md §2.6, §3.1 and §7
// "validate"; MISSION §A5, P4). Each is an investigate.Validator:
//
//   - JoinVerdicts wraps the decisive verdicts of the deterministic join
//     (domain.ImpactReport) as validation records;
//   - RenderDiff reads the environment-scope render pairs: a breaking item
//     whose effect demonstrably reaches the customer's rendered deployment
//     fails; a complete render showing nothing attributable passes a
//     rendered-config item (never a controller-behaviour one);
//   - OverrideInspection establishes an explicit environment setting that
//     pins the old behaviour of an item (domain.PinnedSetting) from a
//     declarative, release-level pin rule — the only deterministic way to
//     clear a controller-behaviour item;
//   - SchemaCompat checks the platform version against the target's
//     supported range and environment objects against the target CRDs;
//   - StaticReference checks that references along the item's impact paths
//     still resolve after the upgrade.
//
// Every record is at level deterministic and states its outcome relative to
// the safety property of (item, subjects): pass = demonstrably not reached /
// not broken, fail = demonstrably reached and breaking, error = the validator
// broke, not-run = it could not decide (with the reason). A validator that
// cannot decide never passes. Validators never call a model and never read
// expected outcomes; everything they know comes from the bundle, the
// traversal, the join and declarative knowledge.
package validate
