# Tiered upgrade policy

Product-owner decision PO-7 (2026-10-05): engineers want org-configurable rules that classify an upgrade — and
each change in it — the way they would wave through a trivial `helm diff` and stop on anything structural.
`internal/policy` implements that; `ri impact --tier-policy <file|default>` prints the verdict.

```
ri impact cert-manager v1.17.0 v1.17.2 --repo ./customer-repo --kubernetes 1.31 --render --tier-policy default
ri impact … -o json --tier-policy my-org.yaml        # the verdict is the "policy" key beside the report and "render"
ri eval -render                                      # also prints the default policy's verdict per environment case
```

`--policy` on `ri impact` keeps its older meaning (the upgrade *path* policy: `minor-lineage|all`); the tier
policy is `--tier-policy` (and `--tier-policy-all` lists the auto-pass items too). Without `--tier-policy`
nothing changes in the output.

## Tiers and the verdict

| tier | meaning |
|---|---|
| `auto-pass` | a reviewer would wave this through |
| `review` | a person should look |
| `block` | stop: structural risk |

Every **item** gets a tier from the first rule (in file order) that matches it, or from the policy's `default`:

- **rendered** — each semantic change of each render diff (`internal/render` change classes: `image-changed`,
  `rbac-permission-removed`, `resource-removed`, `api-version-changed`, `label-*`, `annotation-*`, `field-*`,
  `service-port-*`, `container-arg-*`, `env-var-*`, …)
- **upstream** — each non-routine change of the upgrade edge. Routine changes (the deterministic detector in
  `internal/upgrade/routine.go`) are covered without a row unless a rule names `routine: true`; a non-routine
  change that has joined impact findings is decided through those findings (they carry the tier) unless a rule
  matches it explicitly
- **finding** — each impact finding that is not cleared (`not-affected` findings are counted, not listed)
- **render-state** — a render that did not yield a complete diff (`not-rendered`, `failed`, `rejects-config`,
  `not-applicable`, `incomplete-values`)

The **upgrade-level verdict** is `auto-pass` only if *every* item is auto-pass; otherwise the strictest tier
present (`block` > `review`). Text and JSON show the verdict, the grouped reasons, and for each item the rule that
fired, the tier the rules gave (`policyTier`), any safety invariant that applies, and the evidence (render
artifact digests, change/finding ids, upstream evidence ids). Rendered values never appear in the verdict:
predicates see derived attributes (`bump=minor`, `sameRepository=true`) only.

## Policy file

YAML, validated against `schemas/policy.schema.json` (embedded) and then semantically (`Policy.Validate`):

```yaml
apiVersion: ri.dev/upgrade-policy/v1alpha1
name: my-org
default: {tier: review, reason: ...}        # explicit default tier
rules:                                       # ordered; first match wins
  - id: image-tag-same-repository-patch-minor
    tier: auto-pass                          # auto-pass | review | block
    reason: ...                              # required: it is shown in the report
    match:                                   # all set fields must match; list fields match any element
      subject: rendered                      # rendered | upstream | finding | render-state
      classes: [image-changed]
      image: {sameRepository: true, bump: [patch, minor]}
```

Predicates by subject:

| subject | fields |
|---|---|
| `rendered` | `classes` (render change classes), `kinds` (object kind), `namespaces` (`-` = unset), `pathSuffixes`, `image: {sameRepository, repositories (globs), bump}` |
| `upstream` | `categories`, `routine`, `routineKinds`, `breaking`, `actionRequired`, `security` (the shared `upgrade.IsSecurityItem` definition) |
| `finding` | `findingClasses`, `findingRules`, `verification` (`none\|deterministic\|human\|consensus\|proxy`) |
| `render-state` | `states` |

Image `bump` is computed from the before/after reference of the same repository (registry included):
`patch`, `minor`, `major` (strict semver tags, optional `v`), `prerelease` (target is a pre-release),
`downgrade`, `digest` (same tag, new digest), `unknown` (a different repository, a non-semver or two-part tag,
or an identical version). Only `patch`/`minor` of the same repository can be waved through by the default policy.

## Safety invariants

Enforced by the evaluator on top of every policy (tests: `TestInvariantsHoldUnderAutoPassEverything` runs an
"auto-pass everything" policy). A policy only decides among review/informational outcomes; ACTION still follows
the trust ladder (docs/ACTION_CLASSIFICATION.md). A `block` rule may raise attention and is never lowered.

| invariant | floor | applies to |
|---|---|---|
| `action-required-finding` | review | any ACTION REQUIRED finding |
| `security-advisory-affects-target` | review | a change produced by `advisory:affects-target` (the target version is itself affected by an advisory), and any finding joined to it. A security *fix* shipped with the target (`impact:security-fix`, informational) is not an advisory against it |
| `render-not-complete` | review | no render, a failed render, a target that rejects the values, a not-applicable target, or customer values that are not complete: absence of change is only evidence from a complete render |
| `unknown-on-non-routine-change` | review | an UNKNOWN finding on a non-routine change or on no change at all |
| `breaking-or-directive-change` | review | a breaking or action-required upstream change decided on its own (stricter than the brief's minimum) |

`Policy.Validate` additionally rejects an `auto-pass` rule that targets a render state, or the `action-required`
/ `unknown` finding classes, so such a rule fails loudly at load time instead of being silently overridden.
"Anything outside the rendered/known set" is covered structurally: with no complete render there is always an
item that cannot auto-pass, and the verdict never claims coverage of changes the diff does not contain.

## Default policy (`policies/default.yaml`) and its rationale

Conservative: it recognises only what a reviewer would wave through, and stops on what breaks running systems.

| rule | tier | why |
|---|---|---|
| `render-rejects-config` | block | the target chart rejects the customer's values; the upgrade would fail at render |
| `rbac-permission-removed` | block | what relied on the permission starts failing with 403 |
| `resource-removed` | block | an object that exists today is pruned or orphaned |
| `crd-api-version-changed` | block | a CRD's storage/served versions changed; stored objects and clients may need migration |
| `api-version-changed` | block | tooling or clusters that do not serve the new version reject the manifest |
| `image-tag-same-repository-patch-minor` | auto-pass | same repository, patch/minor tag; safe only because the verdict needs *every* other change covered too (an image bump plus a hidden RBAC change is not auto-pass) |
| `label-annotation-only` | auto-pass | does not change what the workload runs (chart-version labels and checksum annotations are already suppressed as noise) |
| `upstream-non-breaking-bugfix` | auto-pass | non-breaking bugfix/dependency items that carry no directive and no security content |
| `finding-informational` | auto-pass | informational findings need no action |
| `image-major-or-other-bump` | review | major, pre-release, downgrade, digest-only, non-semver or other-repository image changes |
| `rbac-permission-added` | review | widens what the workload can do |
| *(default)* | review | everything else |

Measured on the 19 environment cases of `ri eval -render` (2026-10-05): **auto-pass 0, review 12, block 7**. That
is the honest picture for that dataset: the cases were chosen *because* they contain breaking changes (major bumps,
RBAC removals, CRD version moves); the 7 blocks are structural (removed RBAC permissions, removed resources, CRD storage/served moves, one chart that rejects the values), and no case is an image-only
upgrade. The shape of an auto-passable upgrade is demonstrated by the table-driven tests, not by the dataset.

## Not covered

- Policies cannot read values (an `image.tag` allow-list, namespace-scoped exceptions beyond `namespaces`) — add
  predicates when a real policy needs them.
- Correlation with the changelog (documented vs undocumented rendered changes) is not a predicate yet.
