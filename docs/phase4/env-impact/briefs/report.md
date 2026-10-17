# Lane `report` — per-upgrade investigation view: everything checked, every verdict, every human decision — Opus

Branch `p4ei/report`, worktree `.claude/worktrees/p4ei-report`. Read FLEET.md, `_wave2-common.md`, `SUMMARY.md`,
DESIGN §2.8–§2.9 (finding + run record), §3 (policy), `docs/INVESTIGATE.md`, `docs/REVIEW_UI.md`, and
`internal/reviewui` (look and feel to match). You own a new package `internal/investigate/reportui`, its templates,
the `cmd/ri` wiring, and additive shared-asset edits in `internal/reviewui/static` if needed.

## The request (product owner, 2026-10-16)

"We should be able to see all data per upgrade so humans can see what was checked prior to each release. Keep
humans semi in the loop and be able to see the verdicts and decisions."

## Build

1. **Investigations list** — served by `ri review serve` under `/investigations` (same server, one nav link) and
   standalone via `ri investigate serve -store <dir>`: one row per investigation (product, from → to, environment,
   run date, decision, review state, counts by class), newest first; filter by product/environment/decision.
2. **Investigation page** — top: the decision in plain English (why it needs review or why it is eligible), the
   release, the environment (repo + tree digest), inputs and collection status. Then **a "what was checked"
   checklist** (renders per target with status, validators run, runtime run yes/no, agent run yes/no, knowledge
   facts used with their verification level). Then **items, attention first** (affected → potentially →
   insufficient → not-affected collapsed): each with class, the rule that decided it, manifestation, a one-sentence
   reason, upstream change text with links, rendered before/after excerpts for its correlated changes, every
   validation (validator, outcome, level, detail, evidence links), runtime observations, gaps and next steps.
   AI content (agent concerns, explanations, verified claims) is visibly labelled as AI and separated from
   validated results.
3. **Human in the loop** — on the investigation page: approve / reject / request more evidence with reviewer name and
   reason, through the existing `ReviewApplier` (human-only; proxy never approves; auto-approval stays off). The page
   shows the full review history and the hash-chained audit trail (who, what, when).
4. **Shareable record** — a "download report" that produces a self-contained HTML (and the run-record JSON) for
   one investigation, suitable for attaching to a release ticket.
5. Demo data: the stored evaluation runs (`eval/investigations/results/*`, packed) should be browsable read-only
   (`-store` pointing at an unpacked run or support the packed layout) so the product owner can look at real
   investigations immediately.

## Done

Tests (list/filter, item ordering, AI labelling, review actions recorded as human with reason, proxy cannot
approve, export is self-contained), a browser check with screenshots in `docs/phase4/env-impact/review-ui/`
(list page, one investigation page), docs (`docs/REVIEW_UI.md` + `docs/INVESTIGATE.md`), build/vet/test green,
cleanup, `LANE DONE:` with the exact command to open it on the stored runs.
