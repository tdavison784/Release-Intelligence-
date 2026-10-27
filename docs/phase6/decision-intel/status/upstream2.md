# Lane upstream2: status (Milestone 2 fix, ledger D-0013)

Branch `p6di/upstream2` (from `p6-decision-intel` @ d9770bfa). Brief: `briefs/upstream2.md`. Design, tests and
known limits: `M2-IMPLEMENTATION.md`, section "Fix: behaviour dependencies, prompt v12".

## Done

- **Prompt v12** = v11 + feature `behaviour` (needs `upstream`). v1–v11 frozen by digest (v11 added); the default
  stays v9 (`pinnedDefaultPrompt`).
- **Structured behaviour dependencies:** `AssessmentInput.Behaviour` / `domain.AgentAssessment.Behaviour`, each
  `{behaviour, source}` with source `upstream` (citations), `change-text` (quote), `unavailable` (request) or
  `not-checked`. Schemas regenerated.
- **Checks on `submit_findings`** (`agent/behaviour.go`):
  - every time: not-affected with an `unavailable` / `not-checked` entry; malformed sources (non-upstream artifact,
    unresolved quote, citations that only quote the change text, a forged change-text quote, an `unavailable` id
    that is not an unavailable upstream request of the item);
  - once (own flag): not-affected with an empty list on a behavioural item;
  - recorded: `behaviour "<b>" at <tag>: <source>` gaps in `unknown`.
- **Record checks:** `Investigation.Validate` (an `unavailable` entry names an unavailable request of its item),
  `ValidateAgainst` (behaviour citations resolve).
- **Tests:** `agent/prompt_v12_test.go` (all of brief §4, plus the once-flags' independence and the record check);
  mutation checks fail them.
- **Docs:** `M2-IMPLEMENTATION.md`, `docs/INVESTIGATE.md` (v12 entry, prompt list), `-agent-prompt` help text.

## In progress (exact next step)

- Nothing: lane done. Next is the commander's: the scored D-EU check with `-agent-prompt investigate-agent/v12`.

## Decisions (with why)

1. **"Behavioural" = manifestation `controller-behaviour` / `unknown`, or family `behavior`.** That is the bundle's
   existing declared classification (`bundle/scope.go`): controller-behaviour is "runtime/default semantics of the
   shipped software", unknown fails closed, and family `behavior` keeps a prose behaviour note behavioural even
   when a rendered change is correlated to it (rule `manifest:rendered-correlated` makes it rendered-config).
   Values, schema, image, compatibility, rendered and procedure items in rendered configuration are decided by
   the render. Documentation-only changes are out of scope already (`scope:documentation`).
2. **Change text** = item title, the change's title and detail (upgrade edge), and the excerpts of the item's
   upstream evidence; each piece matched separately with `QuoteOccurs`. It is what the bundle records as the
   item's own text; the whole release-note artifact also holds other items.
3. **`upstream` needs at least one quote beyond the change text** (not all): otherwise relabelling the release
   note's sentence as `upstream` would bypass the check, while mixing it with a fetched line stays allowed.
4. **The empty-list check has its own once-flag**, so the consumer completion check cannot consume it (tested).
5. **Earlier prompts refuse `behaviour` with a new message** rather than the strict decoder's, following the
   scoped lane's precedent; verified that no stored v9–v11 transcript sent it (412 `submit_findings` calls, 31
   transcripts read from 19 `cases.tar.gz`; no label-bearing file opened).
6. **Feature `behaviour` needs `upstream`**: the `unavailable` source refers to upstream requests, and fetching
   is the remedy the rejection names.

## Files touched outside ownership

- `internal/domain/assessment.go`, `internal/domain/investigation.go` (additive), `internal/investigate/api.go`
  (additive `BehaviourInput`), `schemas/investigation.schema.json` (regenerated).
- `cmd/ri/eval_investigations.go` (help text).
- `internal/investigate/agent/prompt_v8_test.go` (v11 added to the frozen digests).

## Contract changes (additive; `CONTRACT-CHANGE(upstream2)`)

- `AgentAssessment.behaviour` (`AssessmentBehaviour`, sources `upstream`, `change-text`, `unavailable`,
  `not-checked`); `AssessmentInput.behaviour` (`BehaviourInput`); prompt meta `behaviour`.

## Open questions

- None blocking. For the commander: v12 is selected with `-agent-prompt investigate-agent/v12`; recorded states
  need the session's upstream fetches as for v11.

## Test status

`go build ./... && go vet ./... && go test ./...` green: 46 packages.

## Cleanup

No `bin/`, scratch state, kind cluster or server was created; no live run was made.
