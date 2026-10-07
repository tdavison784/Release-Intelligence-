# Lane `policy` — status (PO-7b tiered upgrade policy)

Branch `p3ll/policy`, brief `briefs/policy.md`, user doc `docs/POLICY.md`. Model: Sonnet 5.5.

## Done

- `internal/policy`: declarative policy (YAML, JSON-schema'd by `schemas/policy.schema.json`, embedded through
  `schemas/schemas.go`), `Parse`/`Load`/`Validate`, `Evaluate(policy, Input{Pairs, Edge, Report}) → Verdict`,
  `WriteText`. Subjects: rendered changes, non-routine upstream changes, findings, render states. First match wins,
  explicit default tier; image predicates (same repository, repository globs, patch/minor/major/prerelease/
  downgrade/digest/unknown bump from strict semver tags).
- Upgrade verdict: auto-pass only if every item is auto-pass, else the strictest tier; JSON/text show per-item
  rule, policy tier, invariant, evidence (render digests, change/finding ids), grouped reasons. No rendered values leak.
- Safety invariants (`action-required-finding`, `security-advisory-affects-target`, `render-not-complete`,
  `unknown-on-non-routine-change`, `breaking-or-directive-change`) enforced in `Evaluate`; `Validate` rejects
  auto-pass rules that contradict them. Tested with an auto-pass-everything policy.
- Shipped `policies/default.yaml` (embedded via `policies/embed.go`), rationale in `docs/POLICY.md`.
- CLI: `ri impact --tier-policy <file|default> [--tier-policy-all]`; `ri eval -render` reports the default
  policy's verdict per environment case (measurement only, no gate).
- Tests: `internal/policy/evaluate_test.go` (table-driven incl. adversarial: image bump + hidden RBAC add/remove,
  major bump, downgrade, prerelease, different repository/registry, non-semver, digest-only, render unavailable /
  failed / rejects / incomplete values, hostile policies, file validation), `cmd/ri/impact_policy_test.go`.

## Verdict distribution (ri eval -render, 19 environment cases, warm cache, 2026-10-05)

auto-pass 0 · review 12 · block 7. The environment cases are breaking upgrades by construction. Blocks (by rule):
cert-manager 1.17→1.18 resource-removed; cilium 1.15→1.17 render-rejects-config; crossplane rbac-permission-removed;
external-secrets crd storage/served; karpenter ×2 and kyverno rbac-permission-removed + resource-removed + crd
storage/served. 7 of the 12 reviews have no render pair at all (argo-cd, flux, loki, prometheus-operator,
strimzi ×2, traefik: no chart-visible customer values), so `render-not-complete` refuses to auto-pass them whatever
else is true. A real patch upgrade
(cert-manager v1.17.0→v1.17.2 with the eval values) shows the intended behaviour: 4 image bumps + 11 other
auto-pass items, REVIEW only for the acmesolver arg change and three unevaluated findings.

## Decisions

- **`--policy` was taken** by `ri impact` (path policy `minor-lineage|all`). The tier policy flag is `--tier-policy`
  (+ `--tier-policy-all`); the old flag is untouched. Commander: rename if you prefer a different spelling.
- Verdict is opt-in (no output change without the flag) and does not modify `domain.ImpactReport` or its schema:
  it is a sibling `policy` key in `-o json` (like `render`).
- A non-routine upstream change that has joined findings is decided through them (otherwise every changed values key
  would need its own rule even when the customer render and findings already cleared it); one with no findings falls
  to the rules/default. Advisory-affecting changes and breaking/directive changes keep their own floor.
- `breaking-or-directive-change` is stricter than the brief's invariant list (a policy may not wave through a
  breaking/action-required upstream change that no finding decided).
- "security advisory affecting the target" = change from `advisory:affects-target`; a security fix shipped with the
  target (`impact:security-fix`) is informational and may auto-pass.
- No new dependencies (jsonschema/yaml/semver already in go.mod).

## Files touched outside ownership (additive)

- `cmd/ri/impact.go` (flag, verdict, JSON sibling key; local var `policy` → `pathPolicy` to free the package name)
- `cmd/ri/eval_render.go` (two fields on `renderCaseStat`, a distribution section after the render total)
- `README.md` (one command example)
- new top-level dirs `policies/` and `schemas/schemas.go` (embed only; generated schemas untouched)

## Open / known gaps

- Changelog correlation (documented vs undocumented rendered change) is not a predicate.
- `ri render diff` has no verdict (only `ri impact` and the eval); easy to add if wanted.
- Cases without a render yield review for every one of their upstream items too; only the render-state floor matters.

## Test status

`go build ./... && go vet ./... && go test ./...` all pass on this branch (2026-10-05).
