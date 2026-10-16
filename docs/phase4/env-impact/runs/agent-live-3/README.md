# Live agent run 3 — `kube-prometheus-stack-89-90--obs-north` (inputs only, prompt v3)

The third live run of the investigation agent: the product neither earlier run touched, and the first run under
prompt `investigate-agent/v3` (agenda, subject resolution, answer links, depth rules). This page records **what the
agent did: coverage, depth, validation requests, answer links**. Correctness is the eval lane's call: lane agent
never opened the case's `expected.yaml`, `NOTES.md` or `observed/`.

| | |
|---|---|
| Date | 2026-10-16 |
| Case | `eval/investigations/kube-prometheus-stack-89-90--obs-north/input.yaml` + `env/` |
| Upgrade | kube-prometheus-stack chart 89.2.4 → 90.2.0 (prometheus-operator v0.93.1 both), Kubernetes 1.31 |
| Driver | `claude-cli` (`claude -p`, logged in), MCP bridge `ri investigate mcp --attach` |
| Model | requested `claude-sonnet-5-5`; envelope reported `claude-sonnet-5-5` |
| Prompt | `investigate-agent/v3`, digest `sha256:8db9e388…b91f` |
| Agenda | 26 of 26 in-scope items: no item was decided by a failing deterministic validation |
| Budget | `agenda@v1 n=26` → 168 tool calls, 88 turns, 930 000 tokens, 6.2 MB, 1860 s |
| Outcome | **submitted** after 13 turns, 12 tool calls (2 rejected), 57 785 tokens |
| Coverage | **26 / 26 closed**: 10 raises-concern, 16 not-examined; 0 explains, 0 examined-no-concern, 0 claims-mitigation |
| Depth | 2 items examined (`get_change_item`); inspections per item: 1, 1, 0 × 24 (`coverage.md`) |
| Validation requests | 1 (override-inspection on `ci-194d5b487655`) → record `not-run` |
| Answer links | 0 (the request named no assertion; a `not-run` record could not lift a concern anyway) |
| Deterministic pass | lane validate's five validators, 92 records |

Bundle facts:

- one complete environment render (5 rendered changes, 0 uncorrelated);
- one not-applicable consumer overlay;
- the chart-default release render, 10 changes;
- traversal complete: 5 roots, 5 paths.

**Replay check.** The transcript was re-issued against a bundle rebuilt from the same inputs, at the same commit and
path. All 12 result digests reproduced; the transcript and the coverage table were byte-identical. The run record
(outcome plus the deterministic pass, answer links applied) passes `Investigation.ValidateAgainst(bundle)`.

## Files

- `task.md` — the rendered task, incl. the agenda (26 items).
- `transcript.jsonl` — every call, with arguments, result digests, counters and the CLI envelope.
- `outcome.json` — `AgentOutcome`, incl. `run.coverage` (with `examined` / `inspections` / `validationRequests`)
  and `run.budgetRule`.
- `coverage.md` — the per-item coverage and depth table, tool-call counts, validation requests, answer links.
- `bundle-summary.json` — the in-scope items, renders and traversal.

## What the agent did (12 calls)

1. `get_change_item` on `ci-194d5b487655`: the chart's "From 89.x to 90.x" upgrade-guide item, a
   controller-behaviour item.
2. `query_diff` on the environment render, `get_rendered_object` (the kubelet ServiceMonitor's `spec.endpoints`,
   to side) and `read_artifact` (lines 3–40 of the upgrade guide).
3. `search_manifest` ×2, greps over the environment files:
   - the first, for the new settings, found nothing;
   - the second, for `ignoreNamespaceSelectors|namespaceOverride|serviceMonitor`, found the prod values.
4. `record_assertion`, **rejected**: the quote `ignoreNamespaceSelectors: true` was cited from `values.yaml`; it
   occurs in `values-prod.yaml`. The agent re-cited it from the right file, and the assertion was accepted as a
   **raises-concern** on `ci-194d5b487655`.
5. `request_validation`: `override-inspection` on `ci-194d5b487655`, naming two values keys as subjects
   (`prometheus.serviceAccount.createTokenSecret`, `kubelet.serviceMonitor.authorization`).
   - Both resolved (v3 subject resolution) to the product's values-source nodes.
   - The validator returned `not-run`: no declarative pin rule names a setting that pins the old behaviour.
   - The request named no assertion in `answers`, so no answer link was made.
6. `get_change_item` on `ci-0f8475f72a68` (a removed values key).
7. `submit_findings`, **rejected** by depth rule `depth:examined-after-get-change-item`: it closed `ci-aaeb79bed47e`
   (the operator `--secret-field-selector` change) as an explanation without having retrieved it. The resubmission
   (accepted) closed all 26:
   - **raises-concern ×10.** The upgrade-guide item, plus nine `scope:rendered-unexplained` items: seven
     ServiceMonitor endpoint changes, the operator flag and the new token Secret. Their common claim: the
     environment sets `ignoreNamespaceSelectors: true`, so the ServiceMonitors in `kube-system` / `default`
     reference a token Secret rendered in `monitoring`.
     - Nine of the ten concerns have zero inspections of their own: they rest on the one item it examined. Four of
       the seven ServiceMonitor items are raised "by analogy" (its words): their changes are not in the
       environment render.
     - It gave each a `noValidationReason`. The rule requires one only for behaviour items; these are
       rendered-config.
   - **not-examined ×16.**
     - 14 removed values keys: the deterministic join says the environment does not set them.
     - The Grafana `grafana.ini` change: release render only.
     - The `secretFieldSelector` default: covered, it says, by the operator-flag item.

## What the depth rules changed (against run 2)

- **Run 2 (v2, no depth rules):** 8 of 14 items were closed `examined-no-concern` after 4 inspection calls.
- **Run 3:** that disposition was **never used**. Every item the agent did not inspect is honestly `not-examined`.
  The agent said why in its closing message: an `explains` / `examined-no-concern` needs a citation no other item
  shares, "which would have cost about one extra lookup per item; I chose not to spend that". It had used 12 of
  168 calls.
- **D1 caught one shortcut** (the explanation of an unretrieved item).
- **D2 did not fire.** The one behaviour item with a concern had a validation request, and the agent added reasons
  to the rendered-config concerns too.

## Observations for the commander (generic; not tuned to this case)

1. **The rules make coverage honest; they do not make the agent spend its budget.** Depth is still the agent's
   choice: 7 % of the call budget, 2 of 26 items examined. The next lever is the budget's *floor* rather than its
   ceiling. One generic option is a v4 rule: the first `submit_findings` is refused while fewer than k
   inspections were spent per priority-0 item, unless the item is closed `not-examined` with a reason. That is
   your call; I did not add it.
2. **Concerns are not subject to D1.** Of the 10 concerns, 9 rest on a single examined item ("same as …"). This
   is fail-safe by design (a concern can only add caution; DESIGN §3.3) and was not in the decision's scope. But a
   blocked `explains` can be relabelled `raises-concern` (it happened once). That inflates concerns, not
   clearances, and the eval lane's false-positive metric will show it.
3. **Answer links went unused.** The agent requested a validation but did not name the assertion in `answers`;
   the validator could not decide anyway (`not-run`: no pin rule for these keys). The links are tested, but have
   no live exercise yet.
4. **Question B (bundle reproducibility)** is with orchestrate2, per your decision. This run's replay worked
   because it ran at the recording's commit and path.
