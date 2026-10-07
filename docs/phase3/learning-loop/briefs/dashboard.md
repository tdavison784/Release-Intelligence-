# Lane `dashboard` — engineering review UI `ri review serve` (G7–G10)

Read `_wave1-common.md` first. Package `internal/reviewui` + `cmd/ri/review.go` per DESIGN.md §9
`dashboard`, stdlib only (`net/http`, `html/template`, `embed`; no JS framework; small vanilla JS only
if needed). Inbox with G7 counts and every G7 filter; item page with every G8 element (upstream
statement + evidence with openable links, structured proposal, per-model proposals side by side with
agreement/disagreement highlighted per aspect, validation results, optional environment context,
previous related decisions, the concrete question); actions ACCEPT / REJECT / CORRECT (an edit form
pre-filled with the proposed assertion, per aspect, with enum dropdowns — correction keeps the original)
/ NEED MORE EVIDENCE / DEFER, each with reason + labels; reviewer name required; StartedAt/DecidedAt
recorded for G24. Evidence-first and fast: keyboard shortcuts for the five actions, the question at the
top, one screen per item. Works on a laptop; readable in light/dark. Develop against a fake
`knowledge.Store`/`Queue` with realistic fixture items (multi-model disagreement, corrections, needs
evidence); switch to the knowledge lane's FileStore once it's on `p3-learning-loop`. httptest-based
tests for every route and every action, incl. invalid decisions (correct without change, proxy kind
from the UI) being refused. Include screenshots or an HTML snapshot in docs. Model: Sonnet.

## Product owner requirements (added 2026-10-01 — these take priority)

- **Modern UI.** Polished, contemporary look: clean type scale, cards, status/severity badges, generous
  spacing, sticky header with inbox counts and filters, smooth expand/collapse, light + dark themes
  (`prefers-color-scheme` + a toggle), responsive down to a narrow laptop window. Still no build step:
  hand-written CSS with design tokens (CSS custom properties), and if you want interactivity beyond vanilla
  JS you may vendor a small library (e.g. htmx or Alpine.js) as a static file under `embed` — no CDN at
  runtime, no npm build, note the license in the status file.
- **Collapsible questions.** The queue renders review items as collapsible rows/cards: collapsed shows the
  question, product/release, question type, severity, priority, model-agreement indicator and status;
  expanding (click or keyboard) reveals the full G8 detail inline (evidence, per-model proposals,
  validations, previous decisions) without leaving the page. Expand all / collapse all. The standalone
  item page stays (deep links).
- **Approve/deny individually or in a group.** Every item has inline Accept / Reject (and Correct / Need
  more evidence / Defer) controls. Multi-select with checkboxes (+ select all in current filter, shift-click
  range) enables bulk **Accept**, **Reject**, **Defer**, **Need more evidence** with one shared reason and
  labels. Correct is individual-only (it needs per-item edits). A bulk action shows a confirmation listing
  the items and counts before submitting.
- **Bulk decisions stay honest records.** A bulk action writes one `ReviewDecision` per item (never one
  decision for many), each carrying a shared batch id (additive contract field `BatchID`, mark it
  `CONTRACT-CHANGE(dashboard)`), the reviewer, the shared reason, and timing: StartedAt = when the batch
  view was opened, DecidedAt = submit, plus the batch size, so the knowledge lane's metrics can report
  batch vs individual decisions separately (G24 review time; rubber-stamping risk). Bulk accept is refused
  for items whose route priority is `high` or whose proposals disagree on any aspect unless each such
  item was expanded first in this session (show which ones block the batch and why). All existing
  decision validations still apply per item; partial failures are reported per item, never silently
  dropped.
- Tests cover the bulk paths (mixed valid/invalid items, the high-priority/disagreement guard, one decision
  per item with the batch id).
