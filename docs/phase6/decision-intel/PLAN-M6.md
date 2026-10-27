# Milestone 6: blind comparative evaluation, plan

Mission §10 M6 and §11. PD-7: M6 comes before M5. **Standing order: stop sprawl early.** M6 judges the design; it is
not a tuning loop. No code changes during M6 except fixes to harness bugs, which are disclosed.

## 1. Freeze (before any blind run)

- **Code:** tag the integration commit `m6-freeze` and build every run from it.
- **Prompts:** `investigate-agent/v13` (default), `synthesize/v1`, `formalize/v2`.
- **Knowledge, `KF`:** the M4 K1, which is K0 plus the product owner's promoted CA-scoped fact. Its digest is
  recorded. Nothing is promoted into `KF` during the blind runs, except inside arm E (§3).
- **Scoring:** `GATE-M6.md` is written by the commander **after** the blind cases are authored and **before** the
  first blind run, then never edited. Amendments are appended, dated, with why.

## 2. New blind cases (lane `groundtruth6`, separate from every implementation lane)

**Goal:** 4 affected/unaffected **pairs**, 8 environments, none seen before by any phase. Each pair:
- one upgrade (product, from, to), with two environments that differ in configuration so that one is affected and one
  is not, by an **independently observed** outcome;
- authored blind from upstream sources;
- outcome **observed** in kind: the real upgrade, or an exercise of the mechanism, recorded per the existing
  `eval/investigations/FORMAT.md`;
- never derived from RI output.

**Coverage**, at least one pair each, combined where natural:
1. **A product RI has not been tuned on.** Not cert-manager, ingress-nginx or kube-prometheus-stack. Choose from the
   catalog products where a real breaking behaviour change is documented.
2. **A cert-manager or ingress-nginx upgrade with a different mechanism** from the development cases (generalisation
   within a product).
3. **Incomplete configuration or missing consumer information** in one environment (the correct answer is
   uncertainty, not a class).
4. **An apparent mitigation that does not preserve the behaviour** (a pin that looks right but is ineffective).
5. **Controller configuration that differs from the verifier fixture** (a flag or gate that changes a default). This
   is the PD-5 risk: an observed default must not carry over.
6. **A proposed target with a security regression** in at least one upgrade (a version and advisory decision).

**Inputs only**, in `input.yaml` + `env/`. Labels go in `expected.yaml` + `NOTES.md` + `observed/`, under
`eval/investigations-blind6/`. No other lane and no investigator reads the labels.

## 3. Arms (mission §11), on the 8 blind environments

| Arm | What runs | Repeats |
|---|---|---|
| A | Deterministic RI (no agent), `KF` | 1 |
| B | Phase 5 structured agent (`investigate-agent/v8`), `KF` | 3 |
| C | Consumer-aware investigation (`v13`), `KF` | 3 |
| D | C + upgrade synthesis (`synthesize/v1`) | 3 (synthesis on the C runs) |
| E | D + formalize (`formalize/v2`) + `verify -runtime`. **The product owner reviews each confirmed rule** in the review UI (promote, reject or defer). Arm A is then re-run with the promoted knowledge, on the affected and unaffected sides | 1 |

All arms use the same model (`claude-sonnet-5-5` for the agent, `claude-opus-5-5` for synthesis and formalization),
offline after one online recording of each case's upstream data.

## 4. Metrics (mission §11, pre-registered numbers in GATE-M6)

**Quality**, per arm:
- correct affected and correct unaffected classifications;
- **false-safe agent assessments**, and **unsafe authoritative clearances** (must be 0);
- confirmed breaking mechanisms;
- relevant consumers identified;
- mitigation hypotheses: correct, and verified;
- legitimate safe clearances (arm E);
- unsupported assumptions stated as fact;
- appropriate uncertainty on incomplete inputs;
- correct security-advisory and target-version decisions (arms D and E);
- **recommendations requiring correction**, judged by the product owner on a sample of D judgements.

**Operational:** tool calls, files and artifacts read, tokens, cost, wall time, evidence requests satisfied, human
review decisions required (arm E), and knowledge created.

**Known limitations to watch** (PD-7):
- the semantic-check gap (does a misread recur on blind cases?);
- observed-default carry-over (coverage item 5);
- auto-approval naming.

## 5. Order and budget

1. **Lane `groundtruth6`:** author and observe 4 pairs. About 1–1.5 days, with kind runs.
2. **The commander** pre-registers GATE-M6 and tags `m6-freeze`.
3. **Runs.** About 8 environments × (1 + 3 + 3 + 3) agent or deterministic runs, plus arm E.
   - **Model spend:** about **$80–130** (agent about $1/case, synthesis and formalization a few cents to tens of cents
     per case).
   - **Wall time:** about half a day.
4. **Scoring:** the commander scores; the product owner audits the scores and grades a sample of judgements.
   `M6-RESULT.md`.
5. **Product owner:** the arm-E review sessions, about 15–30 minutes total.

**Gate:** "meaningful improvement over the existing RI baseline without introducing unsafe approval behaviour"
(mission §10 M6). The exact thresholds go in GATE-M6, before the runs.
