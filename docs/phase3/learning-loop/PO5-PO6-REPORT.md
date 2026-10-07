# PO-5 / PO-6: per-level panels before and after (contract-6, 2026-10-05)

Runs: `bin/ri -offline -state <primary>/.ri eval -knowledge <view> -render`, the TRUST-AUDIT setup.
- **Before:** the `p3-learning-loop` binary at 43949651 on the stores as committed.
- **After:** this branch's binary, after
  - `ri knowledge route -auto-approve-general` on `knowledge/` (the loop-run-4 flags), and
  - `scripts/proxy-shadow-merge.py --base-git=HEAD …` for the view.

No case, label, gate or result was edited.

## Real store (`knowledge/`)

Before:
```
level          facts  applicability      transfer           class unknown  ACTION (false/unsupp.)   knowledge findings
none               0  0.564 (28/71)     0.564 (-0)      0.40    0.80    17 (1/0)               0
deterministic      0  0.564 (28/71)     0.564 (-0)      0.40    0.80    17 (1/0)               0
human (gate)       0  0.564 (28/71)     0.564 (-0)      0.40    0.80    17 (1/0)               0
consensus (*)     36  0.584 (30/71)     0.584 (-0)      0.38    0.79    21 (2/0)               40 (4 ACTION · model consensus)
proxy (*)        216  0.634 (36/71)     0.634 (-0)      0.38    0.77    21 (2/0)               291 (4 ACTION · model consensus)
```
After:
```
level          facts  applicability      transfer           class unknown  ACTION (false/unsupp.)   knowledge findings
none               0  0.564 (28/71)     0.564 (-0)      0.40    0.80    17 (1/0)               0
deterministic      0  0.564 (28/71)     0.564 (-0)      0.40    0.80    17 (1/0)               0
human (gate)       0  0.564 (28/71)     0.564 (-0)      0.40    0.80    17 (1/0)               0
consensus (*)     36  0.584 (30/71)     0.584 (-0)      0.39    0.79    17 (1/0)               39
proxy (*)        216  0.634 (36/71)     0.634 (-0)      0.39    0.77    17 (1/0)               290
```

## proxy-incl-shadow view (`docs/phase3/learning-loop/proxy-shadow/eval-knowledge/`)

Before:
```
level          facts  applicability      transfer           class unknown  ACTION (false/unsupp.)   knowledge findings
none               0  0.564 (28/71)     0.564 (-0)      0.40    0.80    17 (1/0)               0
deterministic      0  0.564 (28/71)     0.564 (-0)      0.40    0.80    17 (1/0)               0
human (gate)       0  0.564 (28/71)     0.564 (-0)      0.40    0.80    17 (1/0)               0
consensus (*)     33  0.584 (30/71)     0.584 (-0)      0.39    0.79    20 (1/0)               39 (3 ACTION · model consensus)
proxy (*)        339  0.673 (40/71)     0.673 (-0)      0.39    0.77    20 (1/0)               432 (3 ACTION · model consensus)
```
After:
```
level          facts  applicability      transfer           class unknown  ACTION (false/unsupp.)   knowledge findings
none               0  0.564 (28/71)     0.564 (-0)      0.40    0.80    17 (1/0)               0
deterministic      0  0.564 (28/71)     0.564 (-0)      0.40    0.80    17 (1/0)               0
human (gate)       0  0.564 (28/71)     0.564 (-0)      0.40    0.80    17 (1/0)               0
consensus (*)     33  0.584 (30/71)     0.584 (-0)      0.39    0.79    17 (1/0)               39
proxy (*)        339  0.673 (40/71)     0.673 (-0)      0.39    0.77    17 (1/0)               432
```

## Reading

- **Model-consensus ACTIONs** go 4 → 0 in the real store and 3 → 0 in the shadow view. At the
  consensus and proxy levels, ACTION goes **21 (2 false) → 17 (1 false)** in the real store and
  **20 (1 false) → 17 (1 false)** in the view. The remaining false ACTION is kyverno E9: the
  deterministic `values-removed` label dispute (TRUSTFIX §1b), not knowledge.
- **What PO-5 cost:**
  - **Removed one false ACTION:** flux E6, which is REVIEW now and matches its label.
  - **Removed three true ACTION findings:** external-secrets E3 (one Sonnet call dissented) and
    strimzi-edge E2 ×2 (two Sonnet calls dissented). These are REVIEW now, still applicability hits,
    and come back as ACTION once a human verifies the consequence; open review items are queued.
    That is the cost the commander anticipated.
- **Applicability is unchanged** (REVIEW is an affected class):
  - consensus 0.584 (30/71) in both views;
  - proxy 0.634 (real) / 0.673 (view).

  Class accuracy is 0.38 → 0.39 in the real store and 0.39 in the view.
- **Human (gate) level, aggregate, gate panel and stored-results comparison: identical before and
  after.** No knowledge is used there. The one stored-results regression is the existing `-render`
  vs render-free-baseline difference (cilium plant-edge `notAffectedViolations` 0 → 1), present in
  the before run too. A plain `ri eval` shows **no regressions** on this branch.
- **PO-6 split:** after PO-5 no consensus-ACTION finding remains, so audited/unaudited are both 0.
  The panel prints `N ACTION · model consensus: a audited, x false / u unaudited, y false` whenever
  N > 0. Two shadow-only external-secrets facts (vf-4856fecff791, vf-2a1d3520d913) stay
  consensus-action correctly: all four calls on each requested action, with no dissent. Neither
  yields an ACTION finding in the eval cases.

## The view rebuild: three field-wise merges

Re-routing the real store changed only `consensusAction` on six facts. Three of them the shadow pass
had already **superseded** through proxy corrections:
- vf-d2375ef40b74 (flux E6);
- vf-6703cd4a7629 (flux);
- vf-cec2d4a87881 (karpenter).

The script's per-file rule (real wins every conflict) would have revived those three as active
facts in the view. I added `--base-git=<ref>` to `scripts/proxy-shadow-merge.py`:
- it recovers the base from git, used only if its sha256 equals the manifest digest;
- it merges the record field-wise when the two sides changed different fields;
- anything else stays real-wins.

Result: 0 conflicts, 3 `merged-fields` (status superseded from the shadow, `consensusAction` cleared
from the real store). The LABEL.md stats record it.
