# Engineering review UI (`ri review serve`)

The human half of the learning loop (MISSION G7–G10). Models propose, validators check, and an
engineer settles what is left: *"does this release note mean X, who is exposed, what happens?"*
The UI shows the evidence first, records the decision with full attribution and timing, and never
lets a proxy pose as a human.

```
ri review serve -demo                 # fixture items, in-memory (nothing persisted)
ri review serve -addr 127.0.0.1:8484 -reviewer dana -demo
ri review serve -knowledge knowledge/ # the learning loop's file-backed queue (knowledge.NewQueue over NewFileStore)
```

Stdlib only (`net/http`, `html/template`, `embed`); hand-written CSS with design tokens and about 200
lines of vanilla JS, vendored inside the binary — no CDN, no npm, no third-party code. There is no
authentication (a non-goal): it binds to localhost and refuses cross-origin POSTs; do not expose it.
Without JavaScript the inbox lists the items, each links to its page, and every decision is a plain form.

## Pages

| Route | What |
|---|---|
| `GET /` | inbox: the G7 counters (clickable) and every G7 filter (status, product, release, subject type, question type, severity, confidence, model, model agreement, evidence source, reviewer) |
| `GET /items/{id}` | the item page (deep link): everything of G8 |
| `GET /items/{id}/detail` | the same detail as an HTML fragment (inline expansion) |
| `POST /items/{id}/decision` | one decision: `accept`, `reject`, `correct`, `need-more-evidence`, `defer` |
| `POST /bulk` | a bulk action over the selected items (see below) |

## The inbox

Questions are collapsible cards. Collapsed, a card shows the question, product/release, question type,
severity, priority, a model-agreement indicator (disagree / agree · N models / single model) and the
status. Clicking (or `Enter`) expands it **inline** with the full detail, fetched on demand; *Expand all /
Collapse all* are in the toolbar. Light and dark themes follow `prefers-color-scheme` with a toggle.

## The item (G8), top to bottom

1. **The concrete question**, then the upstream statement.
2. **Evidence**: each record with an openable link (`http`/`https` only; GitHub `L12-L30` locators become
   line anchors), verbatim excerpt, digest, and which models cited it.
3. **Proposed assertion** (what you accept or correct), aspect by aspect, the ones the question verifies
   marked. Applicability conditions are rendered as a readable tree.
4. **Proposals by model**, side by side, one row per aspect. Cells that give the same answer carry the same
   letter; the row says *agree · N models*, *disagree · N variants*, *single model* or *no model committed*;
   abstentions show their reason. Proposals are never merged.
5. **Rendered delta** (only when the item has render evidence): object identity, change class, path, source
   vs target values side by side, renderer + version + chart digests, and the render relation
   (`confirmed-by-render | not-visible-in-render | contradicted-by-render | render-not-applicable`).
   Absence from a render is not refutation. The data comes through the optional `RenderedDeltaSource`
   port (see "Contract changes" in the lane status) until the render lane adds it to `knowledge.ReviewContext`.
6. **Validation results**: per validator, per aspect: confirmed / refuted / inconclusive, rule, detail.
7. **Environment context**, only if recorded on the item, labelled *illustration only*.
8. **Previous related decisions and facts** about the same subject.
9. **Decide**.

## Decisions

Reviewer name is **required** (remembered in a cookie). `ReviewerKind` is always `human`: the UI refuses
any attempt to send another kind or proxy provenance (HTTP 400) — proxy decisions come from the CLI.
`StartedAt` is when the item (page or inline detail) was served, `DecidedAt` is the submit time; the
server parses the page's start time and refuses a missing or future one.

| Action | Needs | Labels (G13) |
|---|---|---|
| accept | – | `accepted` |
| reject | reason | `rejected` (or `duplicate` + the fact id) + optional `wrong-*` |
| correct | reason, ≥ 1 changed aspect | `corrected` + `wrong-*` (derived from the changed aspects unless ticked) |
| need more evidence | reason | `insufficient-evidence` |
| defer | – | none |

**Correct** opens an edit form pre-filled with the proposed assertion, per aspect: subject (family
dropdown + identity fields), change (type dropdown, before/after, replaced-by), applicability (exposure
and overlap conditions as JSON, validated strictly) and consequence (kind dropdown — *the class that
follows the kind is shown and computed server-side* — severity, statement, remediation). The original
assertion is kept in the decision; a correction that changes nothing, or drops an aspect, is refused by
the domain validation (`ReviewDecision.Validate`) before anything reaches the queue.

**Keyboard**: `j`/`k` move, `Enter` expands, `x` selects, `a r c e d` arm the five actions (nothing is
submitted by a single key: accept/defer then `Enter`, reject/evidence: type the reason then `Ctrl+Enter`),
`/` filters, `?` help.

## Bulk review

Select cards (checkbox, shift-click range, *select all in this filter*) and the bar offers **Accept,
Reject, Defer, Need more evidence** with one shared reason and labels. **Correct is individual-only.**

1. `POST /bulk` shows a **confirmation**: every item with its priority, agreement and outcome
   (*record* / *blocked* / *skip + why*), and the counts. Nothing is recorded.
2. Confirming submits **one decision per item** (never one decision for many) in **one `Queue.Decide`
   call**, all with the same `BatchID` (`domain.BatchID`) and `BatchSize`, the reviewer, the shared reason
   and labels, `StartedAt` = when the batch view (the inbox page) was opened, `DecidedAt` = submit. The
   knowledge lane's metrics therefore report batch and individual decisions separately (G24; rubber-stamping risk).
3. **Guard**: bulk *accept* is refused for items whose routing priority is `high` or whose proposals
   disagree on any aspect, **unless that item was opened in this browser session** (its page or its inline
   detail). The confirmation lists the blockers and why; the confirm button is disabled; *Re-check* after
   opening them, or *Exclude blocked and continue*. A direct confirm with blockers returns 409 and records nothing.
4. Every per-item validation still applies. Items that cannot be decided (already decided, not found,
   reason missing…) are shown as *skip + why*, excluded from the batch (batch size = the decidable items,
   at least two), and reported again after submitting — never silently dropped. If the queue refuses the
   call, nothing was written (`Decide` validates all before writing) and the page says so.

## Develop and test

`reviewui.NewDemoQueue()` is an in-memory `knowledge.Queue` with realistic fixtures (a three-model
disagreement on rotationPolicy's consequence with an environment illustration, a relationship question with render
evidence, a mapping the models disagree on, a needs-evidence gate, a deferred item, a decided item, six
routine items the models agree on). `go test ./internal/reviewui` drives every route and action through `httptest`,
including the refusals (correct without change, proxy kind, missing reviewer, blocked bulk accept, atomicity).

HTML snapshots with the CSS inlined (open in a browser, no server needed):
`docs/phase3/learning-loop/review-ui/{inbox,item-disagreement,item-rendered-delta,bulk-confirm}.html`
(the inbox snapshot is the collapsed list; the cards expand only with the live server's JS).

## Browser verification

The JS behaviour is exercised in real headless Firefox with Selenium (not part of `go test`, which needs no browser):

```
python3 -m venv /tmp/riv && /tmp/riv/bin/pip install selenium
ri review serve -demo -addr 127.0.0.1:18485 &
MOZ_NO_REMOTE=1 /tmp/riv/bin/python docs/phase3/learning-loop/review-ui/e2e_firefox.py http://127.0.0.1:18485 /tmp/shots
```

`e2e_firefox.py` asserts: expand/collapse (click, Enter), expand/collapse all, select, shift-click range, select-all, the bulk bar,
the bulk confirmation → record → flash, the bulk-accept guard, inline reject arming (no submit without a reason), `j`/`k`/`Enter`/`x`/`a`/`r`/`c`/`?`
shortcuts, the live class-follows-kind readout, theme toggle. `shots_firefox.py` captures the item sections. Screenshots (light and dark,
inbox, expanded card, bulk bar, confirmation, guard, item page, proposals matrix, rendered delta, correct form, narrow window) are in
`docs/phase3/learning-loop/review-ui/screenshots/`. `MOZ_NO_REMOTE=1` keeps the test Firefox from attaching to a running one.
