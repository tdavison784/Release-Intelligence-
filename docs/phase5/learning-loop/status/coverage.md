# Lane `coverage` — status

Branch `p5ll/coverage` (from `p5-learning-loop` @ 73d72aef). Brief: `briefs/coverage.md`. **State: done.**

## Done
1. **Documentation-only rendered changes count for R2.2, never as a clearance** (cee09993).
   - `validate.render-diff@v2`: for a `controller-behaviour`/`unknown` item, when every pair is complete and every
     correlated rendered change is CRD description text in a CRD that changes nothing else in that pair, a pass with
     rule `render-diff:documentation-only` and `documentationOnly: true`, naming exactly those rendered changes (no
     subjects, paths or answers). A description change beside any other change of its CRD (correlated to anything or
     not) ⇒ `not-run` naming the disqualifying change. Reuses the render layer's class `crd-description-changed`.
   - `classify.policy@v2`: such a record covers its rendered changes for R2.2 only, after re-checking each against the
     bundle; never counted for R2.4 (paths/roots/"a passing validation exists"), never answers a concern (R2.6), never
     `ClearsBehaviour()`. A not-affected finding cites it beside the pin.
2. **Cluster-scoped name references resolved through a documented controller argument** (d428af81).
   - Catalog `Relationship.namespaceFrom {container, arg, default, configFileArg, references}` (references kind,
     format name; makes `from` cluster-scoped). Validation + hand-written product-definition schema.
   - Relations builder (graph and per-side view): every container of that name in the environment's workloads; flag
     value (`--f=v` / `--f v`), `$(VAR)` from a literal env value or `fieldRef metadata.namespace`; else the default
     when no config file can set it. Decided ⇒ resolve there (established, or missing when absent — never name-only);
     edge cites the controller render. Undecided (no citation, no container, configMapKeyRef/secretKeyRef/envFrom,
     undefined/duplicate var, `$$`, config file without flag, no default, bare flag, conflicting values, non-namespace
     value, controllers disagree, pod namespace unknown) ⇒ inferred with the reason appended.
   - `products/cert-manager.yaml` `clusterissuer-ca-secret`: `--cluster-resource-namespace` of container
     `cert-manager-controller`, default `kube-system`, `configFileArg: --config`; six citations, quotes verified
     verbatim against v1.18.0 source/chart and the pinned website commit.
3. **v1 reproducibility** (00d89479): a run whose config asks for `classify.policy@v1` is classified under v1 (same
   decisions; v1 stamp); a documentation-only record under v1 is refused.
4. Text report labels documentation-only records; docs in `docs/INVESTIGATE.md` (RenderDiff, Relationships
   "Cluster-scoped name references", "Policy v2", Evaluation reproducibility).

## Before/after policy trace (synthetic behaviour item; `TestDocumentationOnlyCoverageClosesR2_2`)
Fixture: fictional product, `controller-behaviour` item; complete environment render whose only correlated change is
`crd-description-changed` on `CustomResourceDefinition//widgets.gizmo.example.io …mode.description`; one established
impact path from the controller; an override-inspection pass with a pinned setting covering the path.

- **Before** (pin only — what v1 could reach): `potentially-affected` / `classify:plausible-impact`. Only gap:
  `environment rendered change rc-c42d4eedf62e (crd-description-changed …mode.description) is shown harmless by no
  clearing validation (a pinned setting or an observed outcome)`.
- **After** (pin + `validate.render-diff@v2` documentation-only record): `not-affected` / `classify:validated-clear`,
  basis deterministic, producer `classify.policy@v2`, validations = {pin, documentation record}. Reasoning: `R2.2 1
  correlated environment rendered change(s) shown harmless (documentation-only CRD description changes accounted for
  by validate.render-diff@v2 [vr-…] — documentation, not a clearance); … R2.4 1 impact path(s) covered …`.
- **Never** (`TestDocumentationOnlyNeverClears`): documentation record alone, + render silence, + a pinless pass, or a
  rendered-config item with only the documentation record ⇒ not `not-affected` (R2.4 gaps on path and item).
- Same trace with an `unknown` manifestation.

## Policy invariants re-tested
- No new path to `not-affected` without a clearing pass (above; mutation: counting documentation records for R2.4
  fails `TestDocumentationOnlyNeverClears` and `…ClosesR2_2`).
- Claim re-checked (mutation: trusting the flag fails `TestDocumentationOnlyClaimIsRechecked`); `ValidateAgainst`
  rejects a false claim.
- Agent output never an input: hypotheses/explanations leave class, validations and basis identical; an unanswered
  concern still blocks (`TestDocumentationOnlyAgentRecordsAreNoInput`).
- Namespace resolution: mutation "fall back to the default when a container is undecided" fails 13 fail-closed
  cases + the undecided traversal case.

## Decisions
- Version bumps: `classify.policy@v1` → `@v2`, `validate.render-diff@v1` → `@v2`.
- Documentation-only coverage is all-or-nothing per validation call; never carries `Answers` (tests no hypothesis).
- The rendered-config path of render-diff is unchanged (it already passes description text as harmless).
- cert-manager default: upstream website says the cluster resource namespace defaults to `cert-manager` (that is
  the chart's `$(POD_NAMESPACE)`); the binary's v1.18.0 default is `kube-system` (`defaults.go`). The rule declares
  `kube-system` for the no-flag case, with both citations; chart installs always pass the flag.
- Duplicate env definitions are undecided rather than "last wins".

## Files touched outside ownership (all additive)
- `internal/domain/investigation.go`: `ValidationRecord.DocumentationOnly`, `EvidenceBundle.DocumentationOnlyChange`,
  `RenderedClassCRDDescription`, record-shape check, `ValidateAgainst` check, R2.2 hunk in `validateFindingRefs`;
  `schemas/investigation.schema.json` regenerated.
- `internal/catalog/relationships.go` (+ test): `NamespaceSource`, validation, shared `relReferences` helper;
  `schemas/product-definition.schema.json` (hand-maintained).
- `internal/investigate/validate/common.go`: `draft.docOnly`, `NameRenderDiff` @v2.
- `internal/investigate/agent/{answers,claims,peritem}_test.go`: literal policy string → `classify.Policy`.
- `internal/app/testdata/e2e/golden/…cert-manager…json` (3 files): only `definitionDigest` changed.
- `docs/INVESTIGATE.md`.

## Contract changes (CONTRACT-CHANGE(coverage); DESIGN.md is commander-owned — proposed text)
- DESIGN §2.6: "`DocumentationOnly`: a deterministic render-diff pass naming only rendered changes that are CRD
  description text in an otherwise unchanged CRD; it accounts for them (R2.2) for any manifestation and covers
  nothing else; `ClearsBehaviour()` is false."
- DESIGN §3.1 R2.4 last paragraph: "…only clearing validations count — for coverage here, for R2.2 and for R2.6",
  add: "except that a documentation-only render-diff pass accounts for the rendered changes it names in R2.2
  (`classify.policy@v2`)."
- DESIGN §2.5: add `namespaceFrom` as a source of `established` for a cluster-scoped name reference.

## Open questions
- None blocking. Note for eval5/commander: stored run `C-20261016T234455Z-aa06720656` re-classifies 6/8 — the same at
  the base commit 73d72aef (pre-existing, not caused by this lane). All other stored runs reproduce 8/8 under this
  branch (checked by re-classifying only; no scoring, no labels read).

## Tests
- `go build ./... && go vet ./... && go test ./...` green at the final commit.
