# Prompt levers from the semantic store (feeds LOOP-DIAGNOSIS-2 L1/L4/L5)

GLM handoff analysis, 2026-10-03, over the committed store at this branch
(1 092 candidates / 2 683 proposals = semantic-2 + semantic-3 runs). Deterministic,
offline, no LLM calls, no eval data consulted. Reproduce with
`python3 analyze_undecidable.py <repo>/knowledge` and the `semantic candidates`
commands below.

## 1. Undecidable exposure leaves (L1/L5 input)

LOOP-DIAGNOSIS-2: 72 % of UNKNOWN exposure evaluations come from an `undecidable`
leaf a proposal wrote; `cli-flag` leaves account for 50 of 178 UNKNOWNs. Store-level
numbers behind that:

- 995 of 2 683 proposals assert an exposure condition. **157 undecidable leaves on
  152 proposals (15 % of exposure-bearing); 106 exposures are pure undecidable**
  (no decidable leaf at all).
- **Not a weak-model artifact**: GLM 47, Haiku 45, Opus 39, Sonnet 26. The prompt,
  not the model, is the variable.
- Reasons: environment-visibility-gap 86, runtime-behavior-gap 55, evidence-gap 13.
- Leaf census on exposure-bearing proposals: `gvk-in-use` 305, `field` 296,
  `values-key` 196, `cli-flag` 184, `undecidable` 157, `edge-from-version` 76 …
  `text-line` **13**. The operator that reads config embedded in ConfigMaps is
  nearly unused — exactly the diagnosis's L1 observation, now measured at the
  proposal layer.
- By subject family: protocol-behavior 64, migration 22, product-relationship 16,
  config-key 13. By product: cert-manager 36, flux 24, strimzi 17.

The `needed` texts (full list from the analyzer) split into three honest classes:

1. **Decidable with an operator the model did not know about** (most of the 86
   environment-visibility leaves): "whether the Loki configuration files set
   shared_store; config file content is not visible as a static input",
   "whether the Loki configuration file **or command-line flags** set
   experimental.ruler.enable-api", "whether the environment has already explicitly
   configured tsdb schema and v13 schema versions" (a `field` on the CRD's
   `status.storedVersions`), "whether the YAML shared_store_key_prefix setting is
   configured". The model names the object it needs; it does not know `text-line`,
   `field`-on-CRD, or the product's config channel can test it. This is L1's
   per-product channel context, and it is the majority class.
2. **Genuinely runtime** (the 55 runtime-behavior leaves): "which metrics are
   currently being scraped", "whether log endpoints are accessed at runtime and by
   which principals", "whether the environment's Karpenter scheduling ever hit the
   intermittent nodepool-readiness flake". These SHOULD stay `undecidable`; L1 must
   not teach models to force a predicate where none can exist.
3. Evidence-shape gaps (13): "exact field paths not stated in the excerpt" — L2's
   richer evidence, not vocabulary.

Prompt-design consequence (for the next prompt version, which the brief reserves
to the Claude agent): the applicability/full prompts need a per-product channel
block (where this product reads configuration: Helm values paths, ConfigMap-embedded
files with their keys, CLI args — the addendum's rendered evidence already proved
models cite what they are shown), plus vocabulary examples that map "config file
sets X" → `text-line`/`field` and "CRD status lists vN" → `field`. Abstention stays
cheap for class 2.

## 2. L4 (candidates for filtered notes and `lines:*` diffs) is blocked on integration

LOOP-DIAGNOSIS-2 assigns L4 to this lane (crossplane E8, flux E5 = `lines:*`
capture diffs; ES E7 = `cc:feat` note; prom-op E8 = heuristic note; kyverno E8 =
joined values change). On **this branch** (base p3-learning-loop @ pre-loop-diagnosis-2
merges) none of it is reproducible:

- No `lines:*` change kinds exist here (capture lane's `contents: lines` is on
  integration only).
- No `cc:feat` classification exists here (`internal/normalize/classify*.go` is
  integration-only). Measured: the 7 v3 edges skip **0** routine notes on
  external-secrets and prometheus-operator — the E7/E8 stage-a causes cannot even
  arise.

**Action for the commander: merge p3-learning-loop into this lane before L4 work
starts.** The cost side to weigh once it lands (measured today, this branch,
`bin/ri -state <primary>/.ri semantic candidates <p> <f> <t> -o json` on the 7 edges):

| edge | candidates | routine skips (kind) |
|---|---|---|
| loki 2.9.6→3.0.0 | 157 | 77 (metrics 68, dependency 9) |
| crossplane 1.20.1→2.0.0 | 135 | 33 (dependency 25, housekeeping 8) |
| traefik 2.11.2→3.0.0 | 94 | 6 (housekeeping) |
| flux 2.6.4→2.7.0 | 84 | 1 (dependency) |
| external-secrets 0.15.0→0.16.0 | 59 | 0 |
| kyverno 1.12.6→1.13.0 | 30 | 0 (22 multi-subject-computed, 1 security-fix) |
| prometheus-operator 0.85.0→0.86.2 | 14 | 0 |

Admitting every routine note would grow the run from 573 to ≈690 candidates
(+20 %), dominated by obvious noise (loki's 68 metrics notes). The L4 design after
the merge should admit selectively — by routine kind and/or only notes naming a
subject — and re-measure this table first, because integration's classifier
changes both the kinds and the counts.

## 3. Controlled with/without rendered-evidence run (the addendum's open step)

Not started this handoff: it needs live LLM calls (same 51 rendered candidates
re-proposed without render evidence, into a separate store). The selection is
deterministic — candidates in `knowledge/` whose evidence contains a
`Render.scope = release` record — and the driver pattern is `run3.sh` with
`-render` dropped. Budget note: restricting to the 51 candidates × the models that
answered them with-render (Sonnet+Haiku everywhere, Opus+GLM on the 4 SUB edges)
is at most ≈210 calls rather than a full re-run; `semantic propose` currently writes
requests for every candidate, so it needs an `-only` filter (small, in-lane change)
before the run is cheap to prepare.
