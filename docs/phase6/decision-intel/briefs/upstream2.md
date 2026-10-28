# Lane `upstream2`: Milestone 2 fix, a not-affected verdict cannot rest on unfetched upstream behaviour (Opus)

Branch `p6di/upstream2`, worktree `.claude/worktrees/p6di-upstream2`. Ledger D-0013. Read `_common.md` first, then
`PLAN-M2.md`, `M2-IMPLEMENTATION.md`, `M1-IMPLEMENTATION.md` (the completion check), and `status/upstream.md` and
`status/consumer.md`.

## The defect (found by the commander on development runs; generic description)

Under v11, fetching upstream behaviour is a model choice. In some sessions of one item, the agent fetched the
validation code at the target tag and concluded correctly. In other sessions of the same item, with the same prompt,
the same dossier and the same first tool calls, it never called `request_evidence`. It then concluded `not-affected`
from remembered behaviour ("the paths are valid under the new validation"), a false-safe. Its own assessment `unknown`
said: "Behaviour of the … validation code, which I did not fetch". Nothing deterministic connected that admitted
unknown to the verdict.

## Required: prompt v12 = v11 + the following; v1–v11 stay frozen; the default stays v9

1. **Structured behaviour dependencies.** The assessment gains `behaviour`: a list of the upstream behaviours the
   verdict relies on (validation, defaulting, parsing, feature gates, API semantics). Each entry is
   `{behaviour, source}`, where `source` is one of:
   - **`upstream`**: citations of upstream evidence beyond the item's change text. That is a run artifact fetched
     through `request_evidence`, or an upstream source artifact of the bundle. The quotes must be verbatim, as today.
   - **`change-text`**: a verbatim citation of the item's own change text. Allowed only when that text states the
     behaviour itself, not just that it changed.
   - **`unavailable`**: the id of a `request_evidence` request that was recorded `unavailable`.
   - **`not-checked`**: honestly unestablished.
2. **Deterministic check on `submit_findings`.**
   - **Rejected every time:** a `not-affected` verdict with any `behaviour` entry whose source is `unavailable` or
     `not-checked`. The message lists them, and says that the agent may fetch the behaviour (`request_evidence`) or
     choose `potentially-affected` / `insufficient-evidence` with the gap.
   - **Rejected once:** a `not-affected` verdict with an empty `behaviour` list on an item whose change is
     behavioural. Use RI's existing change classification (find the category or kind that separates
     behaviour/default/validation changes from docs-only or packaging changes). The message asks the agent to declare
     the behaviours it relies on.
   - **Rejected:** a `change-text` source whose quote is not in the item's change text.
   - **Recorded:** gaps such as "behaviour <b> at <tag>: <source>" go into the assessment `unknown`, as M1's
     untraced consumers do.
3. **No product logic.** The check never inspects behaviour wording. It relies only on the structured fields, the
   change classification, the request records and the citation resolution.
4. **Tests.**
   - not-affected with a `not-checked` behaviour: rejected; resubmitted as `potentially-affected`: accepted, with the
     gap.
   - not-affected with an `upstream`-sourced behaviour (a fetched artifact): accepted.
   - `unavailable` without a matching request id: rejected.
   - a forged `change-text` quote: rejected.
   - a behavioural item with an empty list: rejected once.
   - a non-behavioural item with an empty list: accepted.
   - Policy isolation: the classification stays byte-identical.
   - v12 replay.
   - v1–v11 digests frozen.
5. **Docs:** `M2-IMPLEMENTATION.md`, `docs/INVESTIGATE.md` and your status file.

No live runs on development cases. FLEET.md applies, including the files you never open.
