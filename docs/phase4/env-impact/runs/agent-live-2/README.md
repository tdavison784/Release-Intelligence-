# Live agent run 2 — `cert-manager-1.17-1.18--payments-eu` (inputs only, prompt v2)

The second live run of the investigation agent: another product than run 1, the first run under prompt
`investigate-agent/v2` (deterministic agenda, dispositions, coverage, subject resolution). This page records
**what the agent did and how far it covered the agenda**. Correctness is the eval lane's call: lane agent
never opened the case's `expected.yaml`, `NOTES.md` or `observed/`.

| | |
|---|---|
| Date | 2026-10-16 |
| Case | `eval/investigations/cert-manager-1.17-1.18--payments-eu/input.yaml` + `env/` |
| Upgrade | cert-manager v1.17.2 → v1.18.0, Kubernetes 1.31 |
| Driver | `claude-cli` (`claude -p`, logged in), MCP bridge `ri investigate mcp --attach` |
| Model | requested `claude-sonnet-5-5`; envelope reported `claude-sonnet-5-5` |
| Prompt | `investigate-agent/v2`, digest `sha256:918a68a7…ebfd` |
| Agenda | 14 of 16 in-scope items (the 2 removed RBAC objects, `scope:rendered-structural`, were decided by failing deterministic validations) |
| Budget | `agenda@v1 n=14` → 96 tool calls, 52 turns, 600 000 tokens, 4 MiB, 1200 s (`AgentRun.BudgetRule`) |
| Outcome | **submitted** after 6 turns, 5 tool calls, 32 710 tokens |
| Coverage | **14 / 14 closed**: 4 raises-concern, 2 explains, 8 examined-no-concern, 0 claims-mitigation, 0 not-examined (`coverage.md`) |
| Deterministic pass | lane validate's five validators, 49 records. All five were available to `request_validation`; the agent made no requests |

Bundle facts:

- one complete environment render (49 rendered changes, 0 uncorrelated after the §1.3 amendment);
- two not-applicable consumer overlays;
- the chart-default release render, 14 changes;
- traversal: 15 roots, 24 paths, 1 unresolved edge (incomplete).

**Replay check.** The transcript was re-issued against a bundle rebuilt from the same inputs, through the
`replay` driver. All 5 result digests reproduced; the replayed transcript and the coverage table were
byte-identical. The run record (outcome plus deterministic pass) passes `Investigation.ValidateAgainst(bundle)`.

## Files

- `task.md` — the rendered task, incl. the agenda in priority order.
- `transcript.jsonl` — every call, with arguments, result digests, counters and the CLI envelope.
- `outcome.json` — `AgentOutcome`, incl. `run.coverage` and `run.budgetRule`.
- `coverage.md` — the per-item coverage table.
- `bundle-summary.json` — the in-scope items, renders and traversal.

## What the agent did (5 calls)

1. `query_diff` (environment Deployments).
2. `search_manifest`: one grep over the environment files for
   `rotationPolicy|revisionHistoryLimit|dns01|http01|ValidateCAA|featureGates|servicemonitor|ClusterIssuer`.
3. `read_artifact` ×2: the payments `Certificate` manifest and the mesh CA `ClusterIssuer`.
4. `submit_findings`, closing all 14 agenda items in one call:
   - **raises-concern** ×4: the two `RotationPolicy` default items (`Never` → `Always`) and the two
     `RevisionHistoryLimit` default items (`nil` → `1`).
     - On rotation it cited the Certificate's `privateKey:` block, which sets an algorithm and size but no
       `rotationPolicy`. It named the setting that would pin the old behaviour and said which consumers it
       would affect (mesh peers that pin keys).
     - On revision history it cited that `revisionHistoryLimit` is unset.
   - **explains** ×2: the webhook readiness and liveness probe port changes (6080 → named port
     `healthcheck`, rendered-unexplained). It said explicitly that it did not verify the container declares
     that port.
   - **examined-no-concern** ×8:
     - the four new dns01/http01 ClusterRoles and ClusterRoleBindings, the ACME HTTP01 `PathType` item and
       the DNS01 advisory, all on the grounds that the environment's issuers are CA / self-signed, with no
       ACME. For the advisory it added that this "is not proof of safety";
     - the `ValidateCAA` feature-gate removal: not set in the environment;
     - the ServiceMonitor `targetPort` default: unset, and the deterministic validations passed.

It made no `get_change_item`, `get_rendered_object`, `expand_graph`, `list_paths`, `request_validation` or
`request_evidence` calls. Every citation passed the verbatim-quote check (6 distinct citations).

## Observations for the commander (generic; not tuned to this case)

1. **The agenda fixed coverage; it did not fix depth.** v2 made the agent close every agenda item: 14/14
   here, against 3/27 in run 1 under v1. But it closed all 14 after only 4 inspection calls, with 8
   `examined-no-concern` entries resting on one grep and two file reads. The citations are short generic
   quotes (`kind: ClusterIssuer`, `secretName`) shared across items.
   - Policy-wise this is safe: `examined-no-concern` is an `explains` assertion and can never lower a class.
   - But the coverage numbers overstate examination.
   - A generic, deterministic depth floor would be a v3 rule: `examined-no-concern` (and `explains`) only
     for an item whose `get_change_item` was called in the session, with at least one citation that is not
     reused for another item. It is enforceable in the tool layer and does not depend on the case. I did not
     change v2 after its live run: a change means a new version.
2. **No validation requests**, so the v2 subject resolution was not exercised live (it is covered by tests:
   values keys, flags, object references, rendered changes, files). The agent named the deciding settings
   (`rotationPolicy`, `revisionHistoryLimit`) in prose, but never asked for the override inspection that
   would decide them. v3 could require a `request_validation` (or a stated reason) for every `raises-concern`
   on a `controller-behaviour` item.
3. **Environment evidence URIs are absolute local paths** (`/Users/…/eval/investigations/…/env/…`). That is how
   the snapshot builder records them. Together with the snapshot pinning the HEAD of the enclosing git tree
   and the input path as supplied, a groundtruth bundle id changes with every commit and checkout location.
   A recorded run then replays only at its commit and path; elsewhere it is refused for a prompt-digest
   mismatch, never silently accepted. This is for the bundle owner (now lane renderdiff).
