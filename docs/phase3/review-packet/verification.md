# Citation verification record (packet QA)

Run before the reviews, on 2026-10-01, from the repository at commit
`77bed5a` (branch `feat/p3-human`). Method: every distinct evidence record of
kind `document`, `release-asset` or `structured` in the four `report.json`
files was fetched from its live source (raw content for `blob` URLs; the
release asset itself for `releases/download` URLs), and the locator
(line / line-range / file path) was checked to contain the recorded excerpt
(whitespace-normalized). Environment citations (`environmentEvidence`) were
checked against the packet's own `example-env/` copies the same way.

## Upstream citations: 62/62 verified

| Source document | Citations | Result |
|---|---|---|
| `cert-manager/website` `content/docs/releases/README.md` (master) L306–L307 | 2 | OK — supported-releases table shows 1.18: k8s 1.29→1.33 |
| `cert-manager/website` `content/docs/releases/release-notes/release-notes-1.18.md` (master), L16–L371 | 44 | OK — every line locator matches its excerpt |
| `cert-manager/website` `content/docs/releases/upgrading/upgrading-1.17-1.18.md` (master) L8–L14 | 2 | OK |
| `cert-manager/cert-manager` `deploy/charts/cert-manager/Chart.template.yaml` @v1.17.0/@v1.18.0 L23 | 2 | OK — `kubeVersion: ">= 1.22.0-0"` |
| `cert-manager/cert-manager` `deploy/charts/cert-manager/values.yaml` @v1.17.0/@v1.18.0 (file-level) | 2 | OK |
| `cert-manager/cert-manager` release asset `cert-manager.yaml` @v1.17.0 (L12984, L13047, L13053, L13129) and @v1.18.0 (L13088, L13151, L13157, L13233) | 8 | OK — image references at the stated lines |
| `cert-manager.crds.yaml` release assets @v1.17.0/@v1.18.0 (file-level) | 2 | OK |

Known fragility, not a failure today: the three website citations point at
the `master` branch. They resolved on 2026-10-01, but upstream edits can move
lines. Tag-pinned and release-asset citations cannot drift. (Issue filed for
the tool: consider pinning website citations to a commit.)

## Environment citations: 25/25 verified (3 with a precision caveat)

22 match file + exact line + excerpt. 3 (manifest/CRD citations of report 4:
`ev-4b5ee105c929`, `ev-c70daaf7f996`, `ev-fb0a77fa0452`) use a **node-start
locator**: the line number points at the first line of the YAML object and the
excerpt quotes two scalar fields from within that object (e.g. L4 of
`crds/certificates.yaml` starts the CRD document; the excerpt quotes
`kind: CustomResourceDefinition` at L5 and the name at L7). A reviewer lands
on the right object and the quoted fields are visibly there, but not on the
exact excerpt lines. Filed as a minor tool issue (locator should span
L4–L7). None of the 25 failed to resolve.

## Reproducibility

`reproduce.sh` re-runs all four reports offline from the recorded fixtures and
diffs against the shipped copies: all 7 outputs byte-identical (JSON compared
modulo the wall-clock `generatedAt`). The environment copies under
`example-env/` are byte-identical (sha256-verified) to the repository fixtures
the reports were generated from.

## Nothing failed verification

No dead links, no wrong lines, no mismatched excerpts were found. The two
caveats above (master-branch drift risk; node-start locators) are recorded as
issues, not failures.

---

## Addendum — Round 4 trust fixes (2026-10-01, branch `feat/p3-trust-fixes`)

The four filed issues below were addressed AFTER the proxy reviews were
written; the reviews and their numbers above are the round-3 state and are
deliberately untouched. The shipped reports under `reports/` were regenerated
from this branch (same offline commands; `reproduce.sh` passes byte for byte
again).

| Filed issue (reviewer) | What this round did |
|---|---|
| Security-relevant changes must be visible deterministically (SRE A2/D2, platform-engineer A2 — both escalation) | New `impact:security-fix` rule: note-derived changes citing a CVE/GHSA/advisory classify **informational** ("the fix ships with the target; no environment-specific action beyond upgrading") with their upstream evidence and the advisory ids in the detail. The predicate is the routine detector's security carve-out (`upgrade.IsSecurityItem` — one definition). A security item that also breaks or carries an operator directive keeps its stronger class. In report 1 the five dependency-CVE bumps moved UNKNOWN → INFORMATIONAL. |
| Collapse the UNKNOWN wall (both reviewers, C2/F4) | The text report's default view is one line per missing-evidence family with counts (e.g. report 1: 40 note-derived, 4 computed-diff-without-join, 1 unsuppliable cluster platform); `--show-unknown` lists every item. JSON and the funnel counts are unchanged. |
| Evidence ids force the JSON round-trip (both reviewers, E2/H1/F4) | Finding why-blocks now carry inline short-form locators (`evidence: upstream: release-notes-1.18 L350 · environment: values.yaml:L42`), top 2 per chain with "+n more"; the id legend stays. |
| "requires 1.29–1.33" overstates (platform-engineer B1/G, the pilot's §0 caveat) | The below-range detail now states the exclusion is **pre-existing** when the source release's range also excluded the cluster (report 1's case), and appends the reconciliation when the chart's `kubeVersion` admits the cluster: "the narrower supported range is the project's tested-matrix statement (Helm will not refuse the install)". Classes unchanged. |

The AI-suggestion metadata of report 4 changed as a consequence (9 suggestions,
4 prompts pending: the CVE items are no longer UNKNOWN, so the model is no
longer asked about them) — see `reports/report-4-ai-enriched/run.md`.
