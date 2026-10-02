# Report 4 — with the optional AI step (`-enrich`): suggestions on UNKNOWN findings, never verdicts

- **Upgrade / environment:** cert-manager v1.17.0 → v1.18.0 with the same environment inputs as report 1 (Kubernetes 1.28, values, manifests, one CRD, images).
- **What is different:** the run had the optional AI enrichment step enabled
  (`-enrich`). The deterministic report is built first and is unchanged; the
  step then asks a language model about the UNKNOWN findings and attaches,
  per finding, at most one **suggestion** ("AI suggests review-required") plus
  notes — each with its own provenance and citations. The AI never rewrites a
  classification: no deterministic verdict moved classes in this report.
- **Run metadata (also in `report.json` → `enrichmentRun`):** model
  `glm-5.3-flash`; 41 candidate groups, 20 prompts asked, 14 answers accepted,
  2 rejected (schema-violating answers — shown at the bottom of the report),
  4 prompts pending (no committed answer for their digest — the deterministic
  UNKNOWN set moved in the round-4 trust fixes: the five dependency-CVE items
  now classify `impact:security-fix`/informational deterministically, so the
  AI layer is no longer asked about them), 9 UNKNOWN findings now carry an AI
  review suggestion.
- **Run mode:** offline replay of recorded model answers. No network, no API
  keys needed to reproduce.

## Files

| File | What it is |
|---|---|
| `report.txt` | the rendered report (see the `AI enrichments` section and the `AI suggests review-required` lines) |
| `report.json` | the same report as JSON (`suggestions` live on the findings; `enrichments` and `enrichmentRun` at top level) |
| `../../example-env/report4-env/` | byte-identical copies of the environment inputs (see path note below) |
| `stderr.txt` | the progress line the tool prints to stderr during the run |

## Exact command

This one is run from `internal/app/` (not the repository root) so that the
environment paths match the recorded model prompts byte for byte — the prompt
digest, and therefore the cached-answer lookup, covers the evidence URIs which
embed the input paths as passed:

```
cd internal/app
../../bin/ri -products ../../products -offline -state <recorded-state-dir> \
  impact cert-manager v1.17.0 v1.18.0 \
  --kubernetes 1.28 \
  --values testdata/e2e/env/cert-manager/values.yaml \
  --manifests testdata/e2e/env/cert-manager/manifests \
  --crds testdata/e2e/env/cert-manager/crds \
  --images testdata/e2e/env/cert-manager/images.txt \
  -enrich -model glm-5.3-flash -enrich-max 20 -o text
```

**Path note for citations:** because of the working directory above, the
environment citations inside `report.txt`/`report.json` read
`testdata/e2e/env/cert-manager/…` (relative to `internal/app/` in the
repository). The identical files are included in this packet at
`example-env/report4-env/` — contents are byte-identical (sha256 in the report
header matches `shasum -a 256` of the packet copies). Only the path prefix
differs.

## What to judge here

- Are the AI suggestions honest about what they do not know (no invented
  versions, no "you must act")?
- Does `[suggests review]` stay clearly weaker than the deterministic
  ACTION REQUIRED of report 1?
- Do the two schema-rejected answers being visible (instead of silently
  dropped) change your trust up or down?
