# Lane verdict live smoke run 1: `ingress-nginx-1.11-1.12--shop-de`, inputs only, prompt v6

This is a smoke test of the v6 surface (item dossier, provisional verdicts, structured mitigations) on one
Phase 4 case. The case is deliberately **not** the PROTOCOL development pair. This page records what the
agent did. Lane verdict never opened the case's `expected.yaml`, `NOTES.md` or `observed/`, so correctness of
the verdicts is lane eval5's call. The prompt was **not** changed after this run.

| | |
|---|---|
| Date | 2026-10-17, commit `d53c639b` (bin built at `db65b899`; no prompt or agent change since) |
| Case | `eval/investigations/ingress-nginx-1.11-1.12--shop-de/input.yaml` + `env/` |
| Command | `ri -state <scratch> investigate -agent -agent-model claude-sonnet-5-5 -repo <case>/env -env-name shop-prod-de -kubernetes 1.31 -knowledge knowledge ingress-nginx controller-v1.11.5 controller-v1.12.0` |
| Knowledge | the worktree's `knowledge/` (= 03b6ff43, PROTOCOL K0 content) |
| Driver / model | `claude-cli`, requested and reported `claude-sonnet-5-5` |
| Prompt | `investigate-agent/v6`, run digest `sha256:a760d2d19483…` (the digest of the 10 item digests; each item digest includes its dossier) |
| Agenda | 27 items, cap 10 → 10 sessions; 17 items closed `not-examined: beyond cap 10` by the runner |
| Outcome | **submitted** (all 10 sessions). 70 turns, 60 tool calls, **137 737 tokens**, wall time 210.5 s |
| Investigation | `inv-8b78ca6db274`, bundle `bnd-82092377020c`; the policy decision is `human-review-required` |

## What the v6 features did

- **Dossiers.** Each session started from a stored dossier (`dossiers/<item>.json`, 4.5–7 KB). `ri investigate
  dossier inv-8b78ca6db274 <item>` rebuilt two of them read-only from the stored run, and both **matched** the
  dossier the session started from. For the security-defaults item the dossier named the four settings from the
  upstream text's code spans (`--enable-annotation-validation`, `allow-cross-namespace-resources`,
  `annotations-risk-level`, `strict-validate-path-type`). Each was `inherited-default`: it occurs in no complete
  render or environment file.
- **Depth with fewer calls.** Every session made `get_change_item` and 0–3 targeted reads; 12 `record_claim`
  calls were all verified. 60 calls / 137 737 tokens for 10 sessions, against run agent-live-5 (v5, a different
  case): 167 metered calls / 253 869 tokens for 10 sessions. This is not a controlled comparison: different
  case, and no cost gate.
- **Verdicts.** 10 verdicts, one per examined item, none dropped (0 dropped citations or facts). They are
  recorded beside the policy classes, and the classifier never saw them:

| item | agent verdict | policy class |
|---|---|---|
| `ci-0e8570d62156` security features on by default (#11819) | potentially-affected (medium) | potentially-affected |
| 9 other behaviour items (removed metric, metrics default, Lua plugins, PSP, inline Lua, OpenTelemetry image, test files, dashboard) | not-affected (8 medium, 1 high) | potentially-affected |

  The disagreement on 9 items is what PROTOCOL B-arm scoring measures. Which side is right is eval5's
  question, not this lane's. For example, the potentially-affected verdict names a mechanism (the new
  annotation risk level rejects a critical snippet annotation an overlay uses). It rests on two verified claims
  and says what would overturn it.
- **Mitigations.** The agent recorded no hypothesis in this run. `ri investigate export-hypotheses` therefore
  wrote nothing (`"hypotheses": []`).

## Observation: a schema-usability cost, recorded and not tuned

In **all 10** sessions the first `submit_findings` was rejected with `verdict: a … verdict needs at least one
citation that resolves (0 dropped)`. The model put its citations on the closing entry and left
`verdict.citations` empty. The second attempt succeeded every time. That is 10 of 60 calls (17%) spent on one
avoidable rejection.

A generic fix would be to make `verdict.citations` required in the schema, or to let the verdict default to the
entry's checked citations. Either is a prompt or semantics change for a later version. It is not applied here:
the brief forbids tuning on a live run, and the change would break this run's replay.

## Replay check

The transcript was replayed (`-agent-driver replay`) in a fresh state from the same inputs. The replayed
transcript's digest is **identical** (`sha256:d0ee1b20d1dd…`), the run is `submitted`, and the verdicts
(provenance aside) and the findings are equal to the original.

## Files

- `transcript.jsonl`: the agent transcript (81 lines).
- `outcome.json`: the agent run with coverage and cost, verdicts, claims, assertions, the policy findings and
  the decision.
- `dossiers/`: the 10 stored dossiers.
