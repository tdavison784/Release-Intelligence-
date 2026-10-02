# Render evaluation results — 2026-10-11 (GLM handoff)

One live run of `go test ./internal/app -run TestEvalRenderCases -v`
(helm v3.16.1, kubectl v1.34.1 on PATH, network on; charts fetched through the
product's channels into a scratch copy of the e2e recording). Expectations were
authored before this run (see each case's NOTES.md, including the blind-authoring
caveats and the comparison learnings recorded there).

| Case | Pairs rendered / failed | Changes | Recall | Precision |
|---|---|---|---|---|
| cert-manager-1.17-1.18 (release) | 1 / 0 | 14 | 15/16 (R16 known gap) | 14/14 |
| cert-manager-1.17-1.18 / servicemonitor (variant) | 1 / 0 | 15 | 2/2 | — (variant expectations do not aim at the whole delta) |
| kustomize-customer-overlay (environment) | 4 / 1 | 54 | 2/2 | — (expectations aim at the kustomize pair's failure) |

Reading:

- **Release level:** every one of the 14 rendered changes is explained by an
  upstream-authored expectation (precision 14/14), and every decidable expectation
  matched (15/16; the 16th is the marked known gap). The renderer surfaced the RBAC
  role/binding split, both webhook probe ports, the ServiceMonitor targetPort default
  and the five image changes — none of which the changelog entries name individually
  (the renderer's own correlation marked 6 of the 14 documented).
- **Known gap R16 stays open:** the Certificate CRD's new `signatureAlgorithm` field
  did not appear in the rendered delta although the renderer passes `--include-crds`
  (46 → 48 objects; no CustomResourceDefinition among the changes). Whether the
  packaged chart serves its CRDs through `crds/` in a way `helm template` emits — or
  the renderer drops them — is an open question for the lane's next agent.
- **Kustomize:** the overlay fails exactly as authored (`kustomize-dependency`), an
  explicit per-target failure — never "no change" (R13).
- **Not covered here:** env-var and API-version categories (no such change between
  these two tags — recorded in NOTES.md), the pipeline-level R17 metrics (UNKNOWN →
  decided due to render, ACTION strengthened, false ACTION delta, applicability
  before/after) which need the applicability lane's wiring, and a second product
  (the mission asks for live runs on one more product; the lane status records
  cert-manager only).
