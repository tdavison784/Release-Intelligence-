# Lane upstream3: status (Milestone 2 fix, ledger D-0014)

Branch `p6di/upstream3` (from `p6-decision-intel` @ 62705bb7). Brief: `briefs/upstream3.md`. Design, tests and
known limits: `M2-IMPLEMENTATION.md`, section "Fix: fetch first, prompt v13".

## Done

- **Prompt v13** = v12 + feature `fetchFirst` (needs `behaviour`). v1–v12 are frozen by digest (v12 added). The
  default stays v9.
- **The check** (`agent/behaviour.go` `fetchFirst`), on `submit_findings`, whatever the verdict. It applies to a
  behavioural item while the session can fetch: a port, a known repository of the product, budget left.
  - With no upstream request for the item, `not-checked` is rejected once, with the brief's message naming the
    repositories and tags.
  - After a request, `not-checked` is rejected every time: the entry is `upstream` or `unavailable`.
- **`reason`** on a `not-checked` entry (additive: `AssessmentBehaviour.Reason`, `BehaviourInput.Reason`). It is
  accepted on the resubmission and recorded in the gap. Schemas regenerated.
- **Earlier prompts:** the check never runs. A behaviour `reason` is refused with the strict decoder's exact
  earlier message. Stored v12 transcripts were checked: 75 `submit_findings` calls, none with a `reason`.
- **Prompt text:** one sentence on the order, plus the `reason` property and one description sentence in
  `submit_findings`.
- **Tests:** `agent/prompt_v13_test.go`. It covers every case in brief §3, plus the spent-budget exemption, the
  no-repository port, the legacy decoder parity, and replays of v12 and v13. The mutation checks fail them.
- **Docs:** `M2-IMPLEMENTATION.md` (new section), `docs/INVESTIGATE.md` (v13 entry, prompt list), and the
  `-agent-prompt` help text.

## In progress (exact next step)

- Nothing: lane done. Next is the commander's: the scored check with `-agent-prompt investigate-agent/v13`.
  Recorded states need the session's upstream fetches, as for v11 and v12.

## Decisions (with why)

1. **"The session has an upstream port"** means a port is configured, the product has at least one known code
   repository, and the session budget is not spent. Without a repository, or with the budget spent, no request
   can succeed or be made, so asking for one would only spend a call. All three exemptions are tested.
2. **An attempt is any upstream request of the item** (fulfilled or unavailable, refusals included). The check
   cannot match requests to behaviours without reading wording, which it must not do.
3. **After an attempt, `not-checked` is rejected every time, a `reason` included** (brief: "the entry must be
   `upstream` or `unavailable`"). There is always a way out: any request is recorded with an id, even past the
   fetch budget. The one case with no way out is a spent session budget, and that case is exempt.
4. **A `reason` does not skip the once-rejection.** Otherwise a model could give a reason on its first
   submission and never be shown that fetching is available. The brief says it is "accepted on the
   resubmission".
5. **The check runs before v12's verdict check.** With `not-affected` and no fetch, the first message the agent
   reads asks it to fetch, rather than to downgrade. v12's every-time rule still applies afterwards.
6. **Legacy decoder parity.** v12's tests and the stored transcripts must not change, so earlier prompts refuse
   `reason` through the same path and with the same text as the strict decoder did. This follows the upstream
   lane's `legacyEvidenceRequest` precedent. A new message, as upstream2 used, would also have been safe here,
   since no stored call sent a `reason`.

## Files touched outside ownership

- `internal/domain/assessment.go`, `internal/investigate/api.go` (additive `reason`),
  `schemas/investigation.schema.json` (regenerated).
- `cmd/ri/eval_investigations.go` (help text).
- `internal/investigate/agent/prompt_v8_test.go` (v12 added to the frozen digests).

## Contract changes (additive; `CONTRACT-CHANGE(upstream3)`)

- `AgentAssessment.behaviour[].reason` (`AssessmentBehaviour.Reason`, `not-checked` only, checked by
  `Validate`); `AssessmentInput.behaviour[].reason`; prompt meta `fetchFirst`.

## Open questions

- None blocking. For the commander: decision 3 (no `reason` exit after an attempt) follows the brief literally.
  If the scored runs show agents making throwaway requests just to obtain an `unavailable` id, a softer rule
  would accept a `reason` after an attempt as well. That would be a v14 question.

## Test status

`go build ./... && go vet ./... && go test ./...` green: 46 packages.

## Cleanup

No `bin/`, kind cluster or server was created; no live run was made. The stored v12 transcripts extracted for the
`reason` check were in the session scratchpad, outside the worktree, and are removed.
