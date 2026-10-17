# Lane `fixes5` — status

Branch `p5ll/fixes5`. Brief: `briefs/fixes5.md`. Ledger W-0024, W-0025.

## Done
- **W-0024 run hijack** (`internal/investigate/run/keys.go`). Root cause, broader than reported: a memo hit
  returns the first call's whole `StageOutput`, refs included, but keys did not cover the refs a stage passes
  on. build-graph's key (bundle, builder) ⇒ second config continued the first investigation; snapshot's key
  omitted the change ref ⇒ a second change on the same env would have continued with the first change;
  collect's key omitted `Config.Render`. Fix: every key adds `passed(rf)` = change, snapshot, bundle,
  investigation, previous, and the pre-record audit (`Pending`) by content with times zeroed; assemble
  drops `Pending` (it consumes it; on a hit the record exists); build-graph also covers investigation +
  record; collect adds `:render` when on.
  - Tests (each fails on the old keys, checked): `run.TestSecondRunWithOtherConfigIsItsOwnInvestigation`
    (both orders, shared store/DirWorkspace; each record completed with its own config/agent status, equal
    id+digest to a fresh-store run), `run.TestStageKeysCoverPassedRefs` (every stage × every passed ref,
    record, Render; audit times not identity), `orchestrate.TestSecondRunWithOtherConfigIsItsOwnInvestigation`
    (Temporal testsuite, both orders, ids + record digests equal the in-process runner's).
  - Historical: keys are workspace-only (never in a record), so no id/digest can change. Proved:
    `ri eval -investigations -rescore` on all 21 stored runs, JSON output byte-identical before/after; new
    `cmd/ri.TestStoredRunsReverifyTheirDigests` re-derives every stored investigation id
    (InvestigationID(bundle, config)) and content digest against run.json (160 case records). The existing
    Temporal history replay still passes.

- **W-0025 verdict citation friction** — prompt `investigate-agent/v7` (now the default) = v6 + meta feature
  `verdictInheritsCitations` + the tool-schema text that states it (submit_findings description sentence and a
  `verdict.citations` description). system.md / task.md / item.md are v6's byte for byte (tested).
  - Behaviour (`tools_record.go` submitFindings, `verdict.go` verdictOf): when the feature is on and the
    verdict states no citations (omitted or `[]`), the entry's citations are checked with the same resolver
    first; on failure the call is rejected with that error (nothing unchecked is ever inherited); on success the
    verdict takes them and is marked `citationsFromEntry` (new omitempty field on `domain.AgentVerdict`,
    schemas regenerated). Citations the verdict does state are never replaced; if none resolves it is still
    rejected (no fallback).
  - Static digest sha256(system ‖ tools.json ‖ meta.json): v6 still
    `sha256:d9b926aeadb8d433ca4295d12157a35e15c2b308b9496af40d9566082f666bff` (unchanged); v7
    `sha256:4d844504e48da1bb3a26c2471c8435943661dd15c4ac58be33aad63106810891`; item.md `94b7d44f9b6e…` both.
  - Replay under the recorded version: the CLI always used the default prompt, so after a bump an earlier
    transcript could not be replayed under its own version. Added `-agent-prompt <version>` (invFlags:
    `ri investigate`, `ri worker`; default the newest; unknown ⇒ usage error) and `ri eval -investigations
    -agent-prompt` (a replay arm defaults to the replayed run's `config.agent.promptVersion`).
  - Tests (fixture + replay, no live model): `TestPromptV7` (features, unchanged prompt files, tools.json
    differs only in the two strings, generic words); `TestV7VerdictInheritsCheckedEntryCitations` (inherits
    checked; `[]` too; unchecked entry citation ⇒ rejected, no verdict; own citations kept; unresolvable own ⇒
    rejected; **v6 still rejects** the same call); `TestV6AndV7TranscriptsReplayUnderTheirVersion` (a live-run
    shaped v6 transcript, rejected-then-accepted, replays with 0 mismatches and equal verdicts under v6, is
    refused under v7; a v7 inherited-verdict transcript replays under v7); `TestInvFlagsAgentPrompt`;
    `TestReplayPromptVersionIsTheReplayedRuns`. Existing v6 tests (incl. its replay) unchanged and green.
  - CLI check on the real case inputs (scratch state, cache copies): `ri investigate -agent-driver replay
    -agent-replay runs/verdict-live-1/transcript.jsonl -agent-prompt investigate-agent/v6 …` gives the same
    investigation id (`inv-182e1930ef28`), the same per-item v6 prompt digests and a record identical apart
    from timestamps and `agent.wallMillis` to the lane-base binary (7cb8d6ff) with the v6 default. Both are
    refused by the replay driver — see findings.

## In progress (exact next step)
- none: lane done.

## Decisions
- Keys include pass-through refs rather than re-deriving them on a memo hit: smallest change, one function,
  same on both runners (the workflow computes `run.Key` too).
- No producer/policy version bump for W-0024 (keys are not identity).
- W-0025: the bump is the prompt version only (v7). Producer `investigate.agent@v1` unchanged. Because the
  prompt version is in the config digest (even with the agent off: A0's config says v5), new runs, deterministic
  ones included, get new investigation ids under the default. Stored runs are unaffected. Pin `-agent-prompt` to
  reproduce an earlier config.
- Inheritance only when the verdict states none (brief: "when it states none"), not as a fallback for
  unresolvable stated citations: a fallback would silently swap what the model said it rested on.

## Findings for the commander (not fixed, outside the brief)
- The two A″ runs `A-20261016T231142Z-5d4da37635` and `A-20261016T231217Z-5d4da37635` do **not** re-verify
  their pinned `recordDigest` at this lane's base (7cb8d6ff, before any change of mine); their ids re-derive.
  Pre-existing; the new test allowlists exactly these two with this explanation. Rescore output is unaffected.
- **verdict-live-1 no longer replays on the integration branch** (pre-existing, same at 7cb8d6ff): its stored
  dossiers list `validate.override-inspection@v1` / `render-diff@v1` records; the current validators are `@v2`
  (pins/coverage merges), so each item's dossier, and so its prompt digest, differs and the replay driver
  refuses ("recorded under prompt …"). The v6 *code path* is unchanged (same digests as the base binary); the
  transcript is tied to the validator versions it ran with.
- `AgentRun.WallMillis` is wall-clock but inside `run.Digest` / `ContentDigest`, so two identical replays get
  different record digests (110 vs 112 ms here). Not touched (identity contract; would move digests).
- The config digest includes `agent.promptVersion` even when the agent is disabled, so every prompt bump moves
  deterministic-arm investigation ids (A0 at v5, now v7 by default). Harmless for scoring (cases are keyed by
  case id); worth knowing when comparing ids across arms.
- Existing state dirs: memos under the old keys are not hit, so resubmitting an old run in an old state dir
  re-runs its stages once (assemble reuses the existing record and appends audit events). Fresh state dirs
  (eval) are unaffected.

## Files touched outside ownership
- Within the brief: run/keys.go; agent prompts v7, prompts.go, verdict.go, tools_record.go; tests in run/,
  orchestrate/, agent/, cmd/ri; docs/INVESTIGATE.md.
- Additive, needed for the v6 replay requirement: `cmd/ri/investigate.go` (`-agent-prompt`),
  `cmd/ri/eval_investigations.go` (`-agent-prompt`, replay default), `README.md` (one command line).
- `internal/domain/verdict.go` + `schemas/investigation.schema.json`: one omitempty field.

## Contract changes
- `CONTRACT-CHANGE(fixes5)`: `domain.AgentVerdict.CitationsFromEntry` (omitempty, additive).
- Prompt default `investigate-agent/v6` → `v7` (new config digests for runs using the default).

## Tests
- `go build ./... && go vet ./... && go test ./...` green after W-0024 and after W-0025.
