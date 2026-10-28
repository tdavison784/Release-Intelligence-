# Lane `groundtruth6`: sealed blind ground truth for Milestone 6 (Opus)

Branch `p6di/groundtruth6`, worktree `.claude/worktrees/p6di-groundtruth6`. Ledger D-0032. **Your branch is sealed.**
It is not merged until the M6 freeze and the pre-registration of GATE-M6. The commander must not see per-case labels
before the runs.

## What you may and may not read

**You may read:**
- `docs/phase6/decision-intel/PLAN-M6.md` (§2 is your spec), `FLEET.md`, `briefs/_common.md`;
- `eval/investigations/FORMAT.md` and `README.md`;
- the existing cases' `input.yaml` and `env/`, **for format only**;
- the helper scripts in `eval/investigations-heldout/` (`observed-lib.sh`, `probes/`, `render-case.sh`,
  `render-diff.py`);
- `products/*.yaml`, to confirm the catalog supports a product and its versions;
- upstream sources: release notes, docs, code at tags, advisories.

**You may not:**
- read any other lane's branch, brief or status, `GATE-M*.md`, `M*-RESULT.md`, or anything under
  `docs/phase6/natural-agent/runs/`;
- read stored run results under any `results/`;
- **run Release Intelligence (`ri`) on your cases.** Labels never come from RI output.

## Build

Build **4 affected/unaffected pairs, 8 environments**, under a new directory `eval/investigations-blind6/`, with the
same layout and format as `eval/investigations/`. Each environment is a realistic GitOps repo (Argo CD / Flux /
helmfile / Kustomize mixes, like the existing cases). Each pair is one upgrade (product, from → to). Its two
environments differ in configuration or consumers so that one is **affected** at the decisive item and the other
**not**. **Coverage across the pairs, each at least once** (PLAN-M6 §2):
1. A product RI has **not** been tuned on: not cert-manager, ingress-nginx or kube-prometheus-stack. It must have a
   catalog definition in `products/` and a real, documented breaking behaviour change on a version transition the
   catalog supports.
2. A cert-manager or ingress-nginx upgrade with a **different mechanism** from rotationPolicy, revisionHistoryLimit
   or #11819 strict path / annotation validation.
3. **Incomplete configuration or missing consumer information** in one environment. Its expected class is
   insufficient-evidence or potentially-affected, with the missing input named.
4. An **apparent mitigation that does not preserve the behaviour**: a setting that looks like a pin but is
   ineffective.
5. **Controller configuration that differs from a default-like setup**: a flag or feature gate that changes the
   behaviour in one environment and not the other.
6. At least one upgrade whose proposed target has a **security regression** versus the current version, per
   GitHub's advisory database. Record the advisory ids and the safer target in `NOTES.md`, as the expected version
   and advisory decision.

**For each environment:**
- `input.yaml` (inputs only, as the existing cases do) and `env/`;
- `expected.yaml`: a per-item class with its basis, plus the decisive item;
- `NOTES.md`: upstream citations with links at tags; the observed procedure and result, with the kind version; why the
  pair differs;
- `observed/`: logs and records.

**Every decisive outcome is observed** in an isolated kind cluster: a real upgrade, or an exercise of the mechanism
with the environment's actual configuration. Clusters are named `ri-groundtruth6-*` and always deleted. Where
observation is impossible, say why in `NOTES.md` and mark the item `basis: upstream-only`; at most 2 of 8 decisive
items may be upstream-only.

**Upstream data:** do not record RI's caches. The commander records them later through RI's online mode.

## Done

- Commit on your branch only; never push and never merge.
- `go`-free work is fine; no repository code changes.
- Your final message starts `LANE DONE:` and gives:
  - the case ids;
  - per pair: the product and versions, and which coverage items it carries;
  - **class counts only** (for example "decisive items: 4 affected, 3 not-affected, 1 insufficient-evidence"), with
    **no per-case labels**;
  - the kind time used;
  - any coverage item you could not meet, and why.
- Clean up the clusters and large files.

FLEET.md applies.
