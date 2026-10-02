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
- `-knowledge DIR` returns "not part of this build yet": wire `knowledge`'s FileStore-backed `Queue` into `cmd/ri/review.go`
  (`reviewServe`) once the knowledge lane is on `p3-learning-loop`. `Queue.Decide` must be all-or-nothing, as the contract says
  (the UI relies on it for the atomic bulk).
- The UI does not see `ValidationResult`s for a *correction* before submit (queue-side concern).
- No browser was available here: snapshots are static HTML (CSS inlined); the JS behaviour (expand, selection, shortcuts) is
  not covered by automated tests — all server routes/actions are.

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
