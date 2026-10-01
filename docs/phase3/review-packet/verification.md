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
