# Lane `dashboard` — status

**State: done** (build, vet and the full test suite green on `p3ll/dashboard`).

## Done
- `internal/reviewui` (stdlib: `net/http`, `html/template`, `embed`; vanilla JS, no vendored library):
  `server.go` (routes, per-browser session, same-origin guard), `decision.go` (decision + correction
  building, proxy refusal), `bulk.go` (plan, guard, confirmation, one `Decide` call), `view.go`, `rendered.go`,
  `demo.go` (`DemoQueue` fixtures), `templates/`, `static/app.{css,js}`.
- `cmd/ri/review.go`: `ri review serve [-addr] [-demo] [-reviewer]` (`-knowledge DIR` reserved, see open items).
- Inbox: G7 counters (clickable) + every G7 filter, sticky header, collapsible cards with inline lazy detail,
  expand/collapse all, light/dark + toggle, responsive. Item page: every G8 element. Actions ACCEPT / REJECT /
  CORRECT (per-aspect pre-filled form with enum dropdowns, class follows kind) / NEED MORE EVIDENCE / DEFER, each
  with reason + labels; reviewer required; StartedAt = served, DecidedAt = submit. Keyboard shortcuts.
- Bulk (product-owner addendum): multi-select + range + select-all, confirmation page, one decision per item with a
  shared `BatchID`/`BatchSize` in one `Queue.Decide` call, high-priority/disagreement guard for bulk accept (lifted per item
  by opening it in the session), exclude-blocked, per-item skip reporting, correct individual-only.
- Render panel (commander note): "Rendered delta" on the item page and expanded card; hidden when absent.
- Docs: `docs/REVIEW_UI.md`, README command line, ARCHITECTURE package entry, HTML snapshots under
  `docs/phase3/learning-loop/review-ui/`.

## Decisions (and why)
- **No vendored JS library.** Vanilla JS (~200 lines) covers expand, selection, shortcuts; nothing to license.
- **Lazy inline detail** (`/items/{id}/detail`): keeps the inbox cheap for hundreds of items, and the fetch is what
  marks an item as "opened" for the bulk-accept guard (server-side session, not client trust).
- **Bulk needs ≥ 2 decidable items** (contract invariant `BatchID ⇔ BatchSize ≥ 2`); a lone item uses its own controls.
  The batch is the decidable items only; skipped ones are reported per item.
- **Bulk confirm is two-step** (`POST /bulk` previews, `confirm=1` records). `BatchID = domain.BatchID(ids, reviewer, decidedAt)`.
- **Applicability correction is JSON** (strictly decoded, then `Validate`d): the condition tree is the one thing a
  form cannot sensibly flatten. Everything else is dropdowns/fields.
- **`wrong-*` labels on correct are derived** from the aspects whose digest changed, unless the reviewer ticks labels.
- **Defaults**: pending-only view; `Decide` is only offered for pending / needs-evidence / deferred items.
- No auth (non-goal); cross-origin POSTs refused; redirects limited to local paths.

## Contract changes
- `BatchID` / `BatchSize` were already in `domain.ReviewDecision` (contract lane) — nothing added.
- `CONTRACT-CHANGE(dashboard)` (in `internal/reviewui/rendered.go`, not in `internal/knowledge`): the render panel reads
  `reviewui.RenderedDelta` through the optional `RenderedDeltaSource` port. When the render lane adds render evidence to
  `knowledge.ReviewContext`, replace `renderedDelta()` in `server.go` to map that field into `RenderedDelta` (the view/templates
  need no change). Fields mirror RENDER-MISSION goal 18: object identity, change class, path, source/target, renderer+version,
  chart digests, relation.

## Files touched outside ownership (all additive)
- `cmd/ri/main.go`: `review` command + usage lines. `README.md`, `docs/ARCHITECTURE.md`: one line each.

## Open items / for the commander

- The UI does not see `ValidationResult`s for a *correction* before submit (queue-side concern).


## Test status
`go build ./... && go vet ./... && go test ./...` green. `internal/reviewui`: fixtures validate against the domain rules;
every route; every filter; every action; refusals (correct without change, proxy kind/provenance, no reviewer, bad
started, outcome label by hand, cross-origin, already-decided); bulk (confirm step, one decision per item + batch id +
timing, shared reason/labels, mixed valid/invalid, high-priority/disagreement guard incl. per-item lifting and per-session
scope, exclude-blocked, atomic on queue refusal, correct/proxy refused).

## Pending contract follow-ups (PO-1 / PO-2, from commander; not implemented until `p3ll/contract-3` is merged)
- Show the consensus label per aspect in the proposals matrix: cross-model vs same-model (separate calls), using the call id each proposal will carry.
- Show the requested class (`SuggestedClass`) per proposal, including a model's request for action-required, and the "ACTION REQUIRED · model consensus" outcome that auto-sampled review items will carry.
- Nothing built so far contradicts these: the matrix groups by aspect digest and never merges proposals, and the UI never sets or caps a class.

## dashboard-2 follow-up (commander request)
- **`-knowledge DIR` wired**: `ri review serve -knowledge DIR` uses `knowledge.NewQueue(knowledge.NewFileStore(DIR), nil)` (a missing dir = empty store).
  `TestAgainstTheFileStoreQueue` runs inbox, item page, an individual decision, a bulk batch and a correction through the real queue.
- **Atomicity of `Queue.Decide` (findings)**: it validates every decision, computes all facts/follow-ups against an in-memory view and dry-runs the
  fact basis *before the first write*, so invalid input anywhere in a call (unknown item, bad batch, bad fact) writes nothing — pinned by
  `TestDecideRefusedBatchWritesNothing`. It is not transactional against a *store failure mid-write* (JSON files cannot roll back): but decisions are
  content-addressed and the call is idempotent, so retrying the same decisions completes it exactly once — `TestDecideRetryAfterAStoreFailureCompletesOnce`
  (marked `CONTRACT-CHANGE(dashboard)`, tests only, no production change). One gap, handled in the UI: `Decide` does not refuse a *second* decision on an
  already-`decided` item (only superseded); the UI refuses these per item (409 / "skip"). The knowledge lane may want to refuse them too.
- **Real-browser verification (Firefox 152 headless + Selenium in a venv; Chrome is not installed)**: 28 assertions, all pass (list in docs/REVIEW_UI.md).
  Screenshots reviewed critically; fixed from them: sticky header was ~220px (filters moved into a collapsible, non-sticky panel; tiles compacted and
  aligned to the page width), the item-page question rendered tiny (CSS class collision), magenta checkboxes (`accent-color`), a raw timestamp in the
  Decide heading, a redundant reviewer field in every inline form (now the header's), and inline forms now refuse early when no reviewer name is set.
  Screenshots: `docs/phase3/learning-loop/review-ui/screenshots/` (light + dark, narrow window).
- Still untested in a browser: Safari/Chrome specifics, touch input, very large inboxes (hundreds of cards).


## dashboard-3: PO-1 / PO-2 (contract-3 merged)
- **Fixtures fixed**: every demo proposal now has its own `Provenance.CallID` (`call-<model>-<n>`, part of the proposal id), a test asserts all
  call ids are distinct. New examples: **same-model consensus** (txt-prefix: two separate Opus calls), **cross-model consensus** (opus + glm; note
  `ModelFamily` makes opus + sonnet the same family, so that pair is labelled same-model), and a **consensus-ACTION audit item** (RSA < 2048 rejected:
  opus + glm both `requested action-required`, action-eligible kind, no refutation, high priority).
- **UI**: proposals matrix is now "by call" with call id and requested class per column, consensus scope per aspect row (and a legend per
  answer group when they disagree), and a "Consensus requests ACTION REQUIRED · model consensus" banner (reports PO-2 condition (b)+(d) from the
  proposals/validations; (a) and (c) are the pipeline's). Inbox badge: "consensus · cross-model|same-model · N calls" / "single call" / "models disagree".
- **CONTRACT-CHANGE(dashboard)**: `knowledge.InboxRow.Calls int` (additive; filled in `reviewQueue.Inbox` from distinct `Provenance.CallID`s) so the inbox
  can label consensus by separate calls rather than distinct models (two Opus calls are a consensus; `Models` alone says "1 model").
- Decisions are untouched: a consensus-ACTION audit decision is a normal human decision; the UI never sets or caps a class.
- Firefox script updated (10 pending cards), screenshots refreshed incl. `23-consensus-action-*`, `24-same-model-consensus-*`.
