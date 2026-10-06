# Lane `policy` — tiered upgrade policy rules (PO-7b)

Product owner decision (2026-10-05, PO-7): engineers want org-configurable rules that classify an upgrade (and
each change) into tiers — e.g. **auto-pass** for an image-tag-only rendered diff — the way they'd wave through a
trivial `helm diff` and stop on anything structural.

Read: FLEET.md, DESIGN.md §4, DECISIONS.md, docs/ACTION_CLASSIFICATION.md, docs/RENDER.md (render diff change
classes R4: resource-added/removed, api-version-changed, field-*, image-changed, rbac-permission-*,
service-port-*, container-arg-*, env-var-*, label/annotation), internal/upgrade/routine.go (routine
classification), internal/impact (findings, classes). Branch `p3ll/policy`. Owns new package
`internal/policy`, `docs/POLICY.md`, `policies/default.yaml`, `cmd/ri` `--policy` flag, a report section.

1. **Declarative policy file** (`--policy <file>`, default shipped `policies/default.yaml`, JSON-schema'd):
   ordered rules matching on rendered-diff change classes (with attribute predicates: same image repository,
   semver bump level patch/minor/major, kinds, namespaces, label-only…), upstream change categories /
   routine kinds / security, finding classes and verification levels; each rule assigns a tier:
   `auto-pass` | `review` | `block`, with a reason. First match wins; an explicit default tier.
2. **Upgrade-level verdict**: the upgrade auto-passes only if EVERY rendered change and every non-routine
   upstream change is covered by auto-pass rules; otherwise the strictest tier present. Report and JSON show
   the verdict, the rule that fired per change, and the evidence.
3. **Safety invariants (enforced, tested)**: a policy can never auto-pass or downgrade an ACTION REQUIRED
   finding, a security advisory affecting the target, a render failure / chart-rejects-config, an UNKNOWN on a
   non-routine change, or anything outside the rendered/known set (absence is not knowledge). Policies only
   decide among review/informational outcomes and the upgrade-level verdict; ACTION still follows the trust
   ladder. A `block` rule may raise attention (never lowers).
4. **Default policy** (conservative, documented with rationale): image tag patch/minor bump of the same
   repository with no other rendered change ⇒ auto-pass; label/annotation-only ⇒ auto-pass; RBAC permission
   removed, CRD storage/api-version change, resource removed, values-schema rejection ⇒ block; everything else ⇒
   review.
5. Tests: table-driven incl. adversarial (image bump + hidden RBAC change ⇒ not auto-pass; major image bump;
   different repository; render unavailable). Run on the eval environments and report verdict distribution.
   Reply `LANE DONE: policy`.

Coordinate with `renderfirst` (it changes how render deltas enter findings): consume render deltas through
`internal/render`'s exported diff types, not internals. Model: Sonnet.
