# Lane `verdict` — item dossier, independent provisional verdict, structured mitigation hypotheses — Opus

Branch `p5ll/verdict`, worktree `.claude/worktrees/p5ll-verdict`. Read `_common.md`, DESIGN §2.7 (agent),
`docs/INVESTIGATE.md` "Investigation agent", `internal/investigate/agent` (prompt v5, tools, replay),
`internal/investigate/api.go:620-720`. You own `internal/investigate/agent` (incl. prompts), the additive fields
in `internal/investigate/api.go` and `internal/domain` named in PROTOCOL §8 (verdict + mitigation), and a new
`cmd/ri` subcommand for export.

## Build (in this order; commit after each)

1. **Item dossier** (MISSION §1) as the agent's *initial* per-item context, assembled deterministically from the
   bundle (no new store): the change item (versions, upstream text + links, change type), the exposed
   objects with their effective setting state and where it comes from (explicit / inherited default / unknown),
   correlated rendered changes (before/after excerpts) and relevant **unchanged** configuration, existing
   knowledge facts for the subject with level/status/release bounds, prior validations, and an explicit
   **gaps** list (missing / partial / contradictory evidence). Bounded size; everything else stays behind the
   existing MCP tools. Content-addressed artifact in the run, reproducible; also exposed as a tool and via
   `ri investigate dossier <run> <item>` (read-only) so a human can see what the agent started from.
2. **Provisional verdict** per investigated item via `submit_findings` (prompt v6): the PROTOCOL §8
   `AgentVerdict` fields, each citation and each "verified fact" must resolve (existing verification), unresolved
   ones are dropped and counted. Stored as `Investigation.AgentVerdicts`; **never** read by `classify` — add a test
   that findings, decision and eligibility are byte-identical with and without verdicts, and with an adversarial
   "not-affected" verdict on an affected fixture item. The agent may say `not-affected` when policy cannot.
3. **Mitigation hypotheses**: extend `record_hypothesis` with the optional structured `MitigationSpec`
   (PROTOCOL §8): a proposed rule in the existing fact grammar (subject, change, applicability conditions,
   consequence, release bounds) + the environment basis (claim ids/object refs) + requested verification.
   Validate the shape deterministically at record time (well-formed grammar, subject exists in the bundle,
   release bounds parse, environment-basis refs resolve); malformed ⇒ recorded as free-text hypothesis only.
   Do **not** wire `SettingClaim` or hypotheses into pins: the agent never chooses the pin rule.
4. **Export** `ri investigate export-hypotheses <run> -knowledge <dir>`: writes each well-formed mitigation as a
   `SemanticProposal` (provenance ai) + a whole-fact `fact-review` item in the given knowledge dir (existing
   knowledge APIs; never a fact; idempotent by content id). The `verify` lane attaches proof records to these.
5. Prompt v6 is generic: it asks the agent to reason per item through MISSION §2's questions, to prefer the dossier
   then targeted tools, to propose a mitigation only with upstream citations, and to say what would overturn it.
   No product/case words. Prompt digest recorded; replay stays byte-identical.

Cost metrics per investigation (tokens, wall time, sessions, tool calls, dropped citations) recorded in
`AgentRun` for lane `eval5`. Live smoke test on **one** Phase 4 case of your choice is allowed (stateless
`claude -p`, see FLEET.md); keep its transcript under `docs/phase5/learning-loop/runs/` and do not tune the prompt
to it — tuning is on synthetic fixtures only.

## Done

Per `_common.md`; status lists the prompt digest and a sample dossier + verdict + hypothesis (from a fixture).
