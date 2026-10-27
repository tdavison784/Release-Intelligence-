# Lane scoped: status (prompt v10, scoped verdicts; ledger D-0007)

Branch `p6di/scoped` (from `p6-decision-intel` @ bfdef58c). Brief: `briefs/scoped.md`. Design, tests and limits:
`M1-IMPLEMENTATION.md`, section "Prompt v10: scoped verdicts".

## Done

- **Prompt v10** (`internal/investigate/agent/prompts/investigate-agent/v10/`), copied from v9 with only the verdict
  rules changed:
  - `system.md`: "Provisional versus supported" becomes "Scoped verdicts", and the closing section names the new
    fields;
  - `item.md`: one step for sorting unknowns and committing within scope;
  - `tools.json`: `submit_findings` only;
  - `meta.json`: `scopedVerdict: true`;
  - `task.md`: unchanged.
- **Feature `scopedVerdict`** (`prompts.go`; it needs `assessment`). Assessment fields `headline` (required under
  v10), `scope` and `preconditions` are added to `investigate.AssessmentInput` and `domain.AgentAssessment`.
  Schemas regenerated.
- **Checks** (`agent/assessment.go` `scopedChecks`, `scopedCompletion`; wired in `tools_record.go`
  `submitFindings`):
  - a committed class without a scope is rejected, every time;
  - `not-affected` with in-scope unknowns (the `unknown` entries plus untraced consumers) is rejected once, with
    the list, sharing the v9 once-flag;
  - preconditions alone are accepted;
  - earlier versions refuse the fields.
- **Default stays v9**: `pinnedDefaultPrompt = "investigate-agent/v9"` (`claims_test.go`). v9 is added to the
  frozen digests.
- **Tests**: `agent/prompt_v10_test.go` (prompt, generic wording, checks, policy isolation, replay) and
  `run/assessment_test.go` (the classifier input carries no scoped field). A mutation check (both checks
  disabled) fails the v10 tests.
- **Replay safety**: none of the 265 `submit_findings` calls in the stored v9 transcripts sends a scoped field
  (parsed from `cases.tar.gz`; no report, score or gate file opened).
- **Docs**: `M1-IMPLEMENTATION.md` (v10 section, plus the stale known-limit line about the default),
  `docs/INVESTIGATE.md` (v10 entry), and the `-agent-prompt` help text in `cmd/ri/eval_investigations.go`.

## In progress (exact next step)

- Nothing. The lane is done. Next is the commander's: the v10 check on the cert-manager pair
  (`-agent-prompt investigate-agent/v10`). If it passes, set `DefaultPromptVersion` to v10 and
  `pinnedDefaultPrompt` to "".

## Decisions (with why)

1. **A feature flag, not a version check.** All v9 behaviour is gated by features, so v10 is `scopedVerdict`. The
   v9 code path is untouched, and v9 transcripts replay as before.
2. **One rejection per item.** The scoped completion check shares `rejectedNA` with the v9 consumer check. One
   rejection lists the unknowns and the untraced consumers together, which matches the brief's "rejected once,
   with the list".
3. **The scope check is hard and runs first.** It runs in the entry loop, before the once-check, so a call that
   lacks a scope never uses up the item's single rejection.
4. **The headline is required under v10, for every class.** The brief asks for one sentence with the class, scope
   and preconditions. Its content is not parsed, because any generic check of the wording would be brittle.
5. **"In-scope unknowns" means the model's `unknown` entries plus the untraced consumers.** The prompt tells the
   model that `unknown` holds only in-scope facts and that preconditions are listed separately. The tool layer
   cannot tell the two kinds apart by itself.
6. **`affected` with in-scope unknowns is accepted.** The brief rejects only `not-affected`.

## Files touched outside ownership

- `internal/domain/assessment.go`, `internal/investigate/api.go`: additive fields.
- `schemas/investigation.schema.json`: regenerated.
- `cmd/ri/eval_investigations.go`: help text only.
- `internal/investigate/run/assessment_test.go`: extended.
- `internal/investigate/agent/{claims_test,prompt_v8_test}.go`: default pin, v9 digest.
- `docs/INVESTIGATE.md`.

## Contract changes (additive; `CONTRACT-CHANGE(scoped)`)

- `AgentAssessment.headline/scope/preconditions`.
- `AssessmentInput.headline/scope/preconditions`.
- Prompt meta `scopedVerdict`.

## Open questions

- None blocking. A note for the commander: assessments, and so headlines, are not rendered in `classify.WriteText`
  (as in v9). If engineers are to read the headline in the text report, that is a follow-up.

## Test status

`go build ./... && go vet ./... && go test ./...` green (final commit).
