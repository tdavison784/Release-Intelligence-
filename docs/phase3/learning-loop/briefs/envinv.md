# Lane `envinv` — environment product inventory (MISSION Goal 19, environment side)

Read first: `docs/phase3/learning-loop/MISSION.md` (Goal 19), `docs/phase3/learning-loop/FLEET.md`,
`docs/ARCHITECTURE.md` (env section), `docs/IMPACT.md`, `internal/env/env.go` (esp.
`Environment.Installed []InstalledProduct`, the installed-product detection for Helm/Argo/Flux/Helmfile,
`Health`/dimension statuses), `internal/env/repo.go` (`--repo` discovery), `cmd/ri/impact.go`,
`internal/eval` (how environment fixtures under `eval/cases/<id>/environment/` are assembled into impact
inputs).

Branch `p3ll/envinv`, worktree `.claude/worktrees/envinv`.

## Owns

`internal/env/*` (product inventory parts), `cmd/ri/impact.go` flags, the impact-input assembly in
`internal/eval` for fixtures (minimal edit), `internal/domain/impact.go` `ImpactEnvironment` (additive
field only), new `inventory.yaml` files in eval environment fixtures, docs/IMPACT.md section.

## Goal

Make "which products, at which versions, run in this environment" an explicit, evidence-backed,
first-class environment dimension so release knowledge can express and evaluate cross-product
conditions ("requires ingress-nginx >= 1.12.6", "Argo CD manages cert-manager resources").

1. **Model**: a product inventory on `env.Environment` — product id (catalog id when it maps to
   `products/<id>.yaml`, else a normalized name), version (normalized semver where parsable, raw
   kept), source (`declared` | `detected-helm` | `detected-argo` | `detected-flux` | `detected-image`
   | …), and per-entry `domain.Evidence` (file, locator, excerpt, digest) like every other env fact.
   Unify the existing `InstalledProduct` detection into it (don't break existing consumers).
   Kubernetes cluster version appears in the inventory too (source `declared` from `--kubernetes`).
2. **Inputs**: a declared inventory file (`--inventory <file>`, YAML: list of `{product, version,
   note?}`), auto-picked from `inventory.yaml` in an eval environment fixture and from `--repo` mode
   when present; plus best-effort detection from images (an image ref whose repository maps to a known
   product's declared image artifact → that product at that tag; generic, driven by `products/*.yaml`
   artifact declarations, not hard-coded names).
3. **Health**: a `products` dimension with `absent|ok|partial` status (absent ≠ "no other products").
4. **Accessor** for the deterministic engine: e.g. `func (e *Environment) Product(id string)
   (ProductInstance, bool)` and a version-range check reusing the repo's existing range representation
   (`upgrade.versionRangeOf` / semver constraints) — expose whatever the `contract` lane's
   applicability predicate "product present with version in range" needs; keep it generic.
5. **Report**: carry the inventory into `domain.ImpactReport.Environment` (additive) and render it in
   the text output's environment block; regenerate schemas.
6. **Fixtures**: add `inventory.yaml` to eval environment fixtures ONLY with facts already stated in that
   case's `environment.description` or `NOTES.md` (e.g. cert-manager-1.17-1.18: "ingress-nginx
   v1.12.1"). Cite the sentence in a comment per entry. Do not add facts that are not already in the
   fixture's description/notes; do not touch `expectedImpact`/`expectedFindings`.
7. Tests (offline, table-driven) incl. adversarial ones: declared vs detected conflict (both kept,
   conflict visible), unparsable version, product absent vs dimension absent. Existing e2e goldens stay
   byte-identical when no inventory is supplied (or update them with a clear reason if the environment
   block necessarily changes — say so in status).
8. Run the full eval before/after: nothing may regress (`no regressions against eval/results`).

Model: Sonnet. This lane is well-specified; keep it tight and generic.
