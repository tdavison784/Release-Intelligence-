# Lane `prompt8` — agent prompt v8: structured mitigation hypotheses from named overturning conditions — Opus

Branch `p5ll/prompt8`, worktree `.claude/worktrees/p5ll-prompt8`. Ledger W-0029; PROTOCOL amendment 2026-10-17
10:55 is your mandate (one revision, no iteration on the dev pair). Read `_common.md`, then
`internal/investigate/agent/prompts/investigate-agent/v7/` (all files), `prompts.go`, `tools_record.go`
(`record_hypothesis`, `mitigationSpec`, `mitigationState`), `mitigation.go`, `verdict.go`, `docs/INVESTIGATE.md`
(agent + verdict + mitigation sections). Smallest correct change: a prompt revision, not a tool or policy change.

## What the runs show (transcript level only — this is all you get, by design)

Run `B0-K0-20261017-6d2ae5bf` (prompt v7, 8 cases, ~80 item sessions): **zero `record_hypothesis` calls**. Yet
many verdicts named, in `wouldOverturn` / `proposedValidations`, conditions under which the change would not
reach an object — "the field set explicitly to the old value on these objects", "the feature gate disabled on the
controller", "evidence that nothing depends on the changed property". Typical shape: of N exposed objects the
environment sets the old value explicitly on some and leaves it unset on others; the agent recorded the explicit
ones as unaffected in prose and recorded nothing structured. Its residual uncertainty was usually "whether any
consumer depends on the changed property; I did not check this" — it did not trace the consumers with its tools
(one static-reference validation could not run because the object it pointed at is produced at runtime by a
controller and is not in the snapshot).

Reading of v7 (`system.md` "Mitigations:"): the trigger "when you believe a setting or condition in the
environment preserves the old behaviour" is read as "only when the environment already meets the condition for
the whole item". The mitigation is a **release-level rule**; whether this environment meets it is decided per
object by the deterministic validator afterwards, so a rule is worth recording even when the environment meets
it for some, or none, of the exposed objects.

## Required

1. New prompt set `investigate-agent/v8` (copy v7, edit), `DefaultPromptVersion` → v8. v1–v7 stay loadable and
   byte-identical (stored runs re-verify their prompt digests). Feature flags unchanged unless you need one.
2. Mitigation trigger: whenever upstream text supports a condition under which an exposed object is **not**
   impacted, record it with `record_hypothesis` + structured `mitigation` — whether the environment meets it for
   all, some or none of the exposed objects (`environmentBasis` lists the objects/claims that do). Consistency:
   every environment-checkable condition named in `wouldOverturn` that upstream text supports is either a
   recorded mitigation (named in the verdict's `mitigations`) or carries a one-line reason it is not expressible
   in the fact grammar.
3. Change vs consequence: tell the model to distinguish the objects whose setting changes from the objects where
   that change has a consequence a consumer can observe, and to scope the rule's applicability/consequence to the
   latter as a decidable predicate over fields and relationships, naming when it does NOT hold. No domain examples
   from any real product; a synthetic or abstract example is fine.
4. Consumer uncertainty: when the open question is whether something depends on the changed property, trace the
   consumers with the tools (what references the produced object, what it reads from it) and record what you find
   as claims before settling on `potentially-affected`; if a referenced object is runtime-produced and absent,
   say so and reason from what references it.
5. Do not change tool semantics, validators, policy, budgets or the verdict schema. If the tool layer rejects a
   mitigation whose environment meets the rule for only some objects (or none), do not fix it: report it in status
   and message the commander — that is a contract question.
6. Tests (synthetic only): v8 loads and is the default; v1–v7 digests unchanged; v8 text contains no product or
   case name (test with a list: cert-manager, ingress-nginx, kube-prometheus-stack, payments, shop, obs, and every
   kind/field name that appears in `eval/investigations/` directory names — not their contents); a fake-model
   session that records a mitigation with partial environment basis is stored (not rejected).
7. Smoke: at most **one** live session-set on one Phase 4 regression case that is **not** a cert-manager case
   (`ri investigate ... -agent -agent-model claude-sonnet-5-5`, ≤150k tokens), only to confirm v8's calls are
   well-formed. **Never run the agent on either cert-manager case** — that is the scored run (W-0030).
8. Docs: the v8 entry in `docs/INVESTIGATE.md` (what changed and why, citing the run id above).

Done per `_common.md`.
