# Engineering review UI (`ri review serve`)

The human half of the learning loop (MISSION G7–G10). Models propose, validators check, and an
engineer settles what is left: *"does this release note mean X, who is exposed, what happens?"*
The UI shows the evidence first, records the decision with full attribution and timing, and never
lets a proxy pose as a human.

```
ri review serve -demo                 # fixture items, in-memory (nothing persisted); every page carries a DEMO DATA banner
ri review serve -demo -limit 3        # cap the inbox rows, to see the "first N of M" notice
ri review serve -addr 127.0.0.1:8484 -reviewer dana -demo
ri review serve -knowledge knowledge/ # the learning loop's file-backed queue (knowledge.NewQueue over NewFileStore)
ri review serve -knowledge knowledge/ -investigation-store eval/investigations/results   # + the stored evaluation runs under /investigations
```

The same server also shows **investigations** under `/investigations` (top-bar link "Investigations"; back via
"Knowledge review"): every investigated upgrade, what was checked, every verdict, and the human approve / reject /
request-more-evidence decision with its audit trail. `-investigation-store` picks what it shows (default: the
investigation store in the global `-state`; reviews are recorded there; stored evaluation runs are read-only). The
pages and routes are described in docs/INVESTIGATE.md, "Investigation report"; `ri investigate serve` serves them alone.

Stdlib only (`net/http`, `html/template`, `embed`); hand-written CSS with design tokens and about 200
lines of vanilla JS, vendored inside the binary — no CDN, no npm, no third-party code. There is no
authentication (a non-goal): it binds to localhost and refuses cross-origin POSTs; do not expose it.
Without JavaScript the inbox lists the items, each links to its page, and every decision is a plain form.

## Needs your verification: the default view

The product owner verifies knowledge facts here, so `/` lists **only the questions whose human verification would let an
investigation clear an environment**, judged by **fact level**. A fact is as trusted as its weakest aspect
(`VerifiedFact.Level`), so a pin-source fact (`validate.CanBePinSource`, the derivation the OverrideInspection pin rules
use) of an investigated upgrade needs review while **any** aspect is below human, or while no fact is composed yet
because a question of its candidate is open. The view lists every open question needed to make each such fact human,
sibling questions included (the consequence question of a default-change candidate is not a pin source by itself, but
the fact cannot compose without it), **grouped per fact** under a header with the aspect levels and a one-line
"N questions left to verify this fact". Start it and read the counts off the lines it prints:

```
ri review serve -knowledge knowledge/ -reviewer <your name>
# review UI on http://127.0.0.1:8484 (knowledge store knowledge/; Ctrl-C to stop)
# 4 pin-source fact(s) below human verification, 4 question(s) to verify them (listed); 205 other blocking and 631 other open item(s) are one link away
```

**Whole-fact review.** An audit item (a `classification` question on the fact's assertion) verifies only the
consequence, so accepting it can leave a fact at consensus. For a pin-source fact below human, `ri review open-audits
-knowledge DIR -apply` opens ONE **whole-fact review** item (question type `fact-review`): its page shows every aspect
(subject, change, applicability, consequence) with its current level and asks *"Is this whole fact correct?"*.
**Accept** verifies every aspect the page displayed as a human decision (the decision records them in `aspects`); an
aspect a validator confirmed stays deterministic and one a human already verified keeps that earlier decision as its
basis. **Correct** records the corrections per aspect (`wrong-*` labels) and supersedes the fact; **reject** retracts it.
A proxy decision on it never makes an aspect human. Bulk accept of a whole-fact item needs its page opened first.
Earlier decisions are never rewritten: a fact whose audit was accepted is re-opened as a new whole-fact item.

Four views, one link apart, never mixed (header counters cover only the listed view):

| View | URL | Lists |
|---|---|---|
| **Needs your verification** (default) | `/` | open pin-source items, per product in the bar |
| **Other blocking items (N)** | `/?scope=blocking` | open items that block investigations but are not pin sources |
| **Other open items (N)** | `/?scope=other` | the remaining open items |
| **Show all staged data** | `/?scope=all` | the full inbox with every filter, incl. decided / proxy / verified / superseded rows, each labelled why it is not on the review list |

An item is **open** when no human decision is recorded on it, no active fact already holds the aspects it asks about
at `human` or `deterministic` level, it is not superseded, and (for a fact-review audit item) the audited fact is
still active. **A proxy decision never closes an item that blocks investigations** (a proxy is never human; proxy- and
consensus-level facts never pin): such an item stays listed, says an AI proxy already acted on it, and a human can
still decide it (the decision upgrades the verification). Proxy-acted items that do not block stay hidden (under *Show
all*). Consensus-only facts, proxies' proposals and demo fixtures are not review items and never appear.

**Blocking** is computed, not a product list: the item's product is investigated (a product in
`eval/investigations/*/input.yaml`, or an upstream repository one of those products' definitions reads), its release
lies in (from, to] of an investigated upgrade of that product (an unparsable release is never excluded), and its
candidate manifests as controller behaviour (DESIGN §2.2.1): only upstream prose (`manifest:declared-behaviour`), or a
computed `crd:default-changed` diff (`manifest:crd-default`, read from the "computed artifact diff (<rule>)" line the
candidate text records). `-investigations DIR` (default `eval/investigations`) and the global `-products DIR` say where
the scope is read; without them nothing blocks and everything open is under "Other open items".

**Facts with no item.** When consensus auto-approves a fact, its questions are superseded and only some facts are sampled
into audit, so a pin-source fact can sit below human with nothing a human can decide. The default view lists such a fact
in a note above the cards, and the startup line says how many are waiting. `ri review open-audits -knowledge DIR` lists
them (dry run) and with `-apply` opens one whole-fact review item per fact (writes review items only, never decisions or
facts; idempotent), then prints the default view: every pin-source fact below human, its aspect levels and the
questions left.

Decisions are unchanged: reviewer recorded with `reviewerKind: human`, labels and reasons as before; a decided item leaves
the lists at once. Without a snapshot source (a custom `knowledge.Queue` and no `Options.Snapshot`) the inbox lists
everything as before. An item the snapshot does not know is shown, never hidden. Tests: `triage_test.go`.

## Pages

| Route | What |
|---|---|
| `GET /` | inbox (default: pin-source items to verify; `?scope=blocking` other blockers; `?scope=other` other open items; `?scope=all` all staged data): the G7 counters (clickable) and every G7 filter (status, product, release, subject type, question type, severity, confidence, **route priority**, model, model agreement, evidence source, reviewer; `?priority=high|normal|low`). A red **High priority N** chip in the header (pending items of priority high; independent of the status and priority filters) links to `?status=pending&priority=high` |
| `GET /items/{id}` | the item page (deep link): everything of G8 |
| `GET /items/{id}/detail` | the same detail as an HTML fragment (inline expansion) |
| `POST /items/{id}/decision` | one decision: `accept`, `reject`, `correct`, `need-more-evidence`, `defer` |
| `POST /bulk` | a bulk action over the selected items (see below) |
| `GET /investigations…` | the investigation report (list, page, review, export; docs/INVESTIGATE.md "Investigation report") |

## The inbox

Questions are collapsible cards. Collapsed, a card leads with the **plain-English question** (composed
from the item's data — `plainQuestion`), the fixture/upstream title under it, a one-line *why this needs
you* (the strongest routing signal: consensus ACTION, model disagreement, undetermined, refuted,
single call, high priority — `whyYouBrief`), the **suggested answer**, and then product/release, the
class badge (ACTION REQUIRED / REVIEW REQUIRED / …), the agreement badge (models disagree / consensus ·
cross-model|same-model · N calls / single call), priority and status. Clicking (or `Enter`) expands it
**inline** with the full detail, fetched on demand; *Expand all / Collapse all* are in the toolbar. Light
and dark themes follow `prefers-color-scheme` with a toggle.

A filter matching more than 200 items shows the first 200 (priority order) with a *Showing the first N of M
matching items* notice; *Select all* then covers the items shown, so a bulk action never silently skips the
unseen rest — narrow the filters to reach them.

## The item (G8), top to bottom

The item page (and the inline expansion) is a **decision card** with seven numbered sections:

1. **The question**, in plain English (`plainQuestion`: the question type + the proposed assertion →
   "Does cert-manager v1.18.0 change what happens to Certificates where `spec.x` is left unset — and
   does that require action?"). The recorded question, verbatim, stays in Details.
2. **What changed upstream**: the best-cited evidence quote (the one the models actually cite), its
   openable link (`http`/`https` only; GitHub `L12-L30` locators become line anchors), source kind, and
   how many more sources exist (in Details).
3. **Recommendation**: the suggested answer in plain English (`suggestedAnswer`: change sentence +
   consequence + remediation → class · severity), a class badge, *why to trust or doubt it* bullets
   (`whyConfident`: per-aspect agreement, validator outcomes, evidence breadth — ✓ / ? / ✕), and **one
   primary button** — *Accept suggested answer*, or *Send back — need more evidence* on gate items.
4. **Why this needs you**: one line, the strongest routing signal (`whyYou`: consensus ACTION → model
   disagreement → all undetermined → validation refuted → single model → high priority → routine).
   The **Consensus requests ACTION REQUIRED** banner (PO-2) and the **Consensus ACTION blocked by
   dissent** note (PO-5) render here.
5. **Where the models differ**: one row per aspect with ≥ 2 answer groups (PO-1: a proposal is one
   separate stateless call; same-digest answers share a cell). Each cell: the calls (avatar + display
   name), *cross-model/same-model consensus · N calls* (by `domain.ConsensusScopeOf`) or *single call*,
   the answer in one sentence, and * = the suggested answer* when the cell matches the proposal.
   Aspects all calls agree on collapse into chips (*consensus · scope · N calls*); when no call committed,
   the undetermined reason is shown.
6. **Correct the suggestion** (folded): the edit form, each fieldset opening with what the assertion
   *currently says* in plain English.
7. **Details** (folded): the raw record for auditing — upstream statement, every evidence record
   (link, excerpt, digest, cited-by), the proposed assertion aspect by aspect, **raw proposals, one per
   separate call, never merged** (call id, model + version, prompt digest, the class each call suggested
   or **requested** — PO-2, `requests action-required` in red — and abstentions with reasons), validation
   results, environment context (*illustration only*), previous related decisions and facts, and the
   ids/routing line.

A **Rendered delta** section appears between 5 and 6 when the item has render evidence: object
identity, change class, path, source vs target side by side, renderer + version + chart digests, and
the render relation (`confirmed-by-render | not-visible-in-render | contradicted-by-render |
render-not-applicable`). Absence from a render is not refutation.

**Decide** closes the card: the full form with all five actions (see below).

## Decisions

Reviewer name is **required** (remembered in a cookie). `ReviewerKind` is always `human`: the UI refuses
any attempt to send another kind or proxy provenance (HTTP 400) — proxy decisions come from the CLI.
`StartedAt` is when the item (page or inline detail) was served, `DecidedAt` is the submit time; the
server parses the page's start time and refuses a missing or future one.

| Action | Needs | Labels (G13) |
|---|---|---|
| accept | – | `accepted` |
| reject | reason | `rejected` (or `duplicate` + the fact id) + optional `wrong-*` |
| correct | reason, ≥ 1 changed aspect **or** changed consequence statement/remediation | `corrected` + `wrong-*` (derived from the changed aspects unless ticked), plus `improved-statement` when the consequence prose changed too; a **prose-only** correction is exactly `[corrected, improved-statement]` (never `wrong-*`; ticking one is refused) |
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
`docs/phase3/learning-loop/review-ui/ux2-{inbox,item-disagreement,item-rendered-delta,item-consensus-action,item-gate}.html`
plus `inbox-truncated.html` (the "first N of M" notice). The inbox snapshot is the collapsed list; the
cards expand only with the live server's JS.

## Browser verification

The JS behaviour is exercised in real headless Firefox with Selenium (not part of `go test`, which needs no browser):

```
python3 -m venv /tmp/riv && /tmp/riv/bin/pip install selenium
ri review serve -demo -addr 127.0.0.1:18485 &      # FRESH server per run: the suite records decisions
MOZ_NO_REMOTE=1 /tmp/riv/bin/python docs/phase3/learning-loop/review-ui/e2e_firefox.py http://127.0.0.1:18485 /tmp/shots
```

`e2e_firefox.py` asserts: expand/collapse (click, Enter), expand/collapse all, select, shift-click range, select-all, the bulk bar,
the bulk confirmation → record → flash, the bulk-accept guard, inline reject arming (no submit without a reason), `j`/`k`/`Enter`/`x`/`a`/`r`/`c`/`?`
shortcuts, the live class-follows-kind readout, theme toggle. The script pins transitions/transforms (`steady()`) and scrolls targets
into view: the ux-v2 cards lift on hover and the bulk bar is fixed, which otherwise makes coordinate clicks racy. Selectors scope to
`form.actions` — the correct form also carries a `reason` textarea but sits hidden in the §6 fold. `shots_firefox.py` captures the item
sections (`#differ`, `.decide`, `#rendered`, `.consensus-action`) and `shot_priority_firefox.py` the priority filter. Screenshots of the
ux-v2 decision card and the verified behaviors (light and dark; inbox, item, details fold, correct form, consensus-action, rendered
delta, gate item, bulk flow, armed actions, help, narrow window) are in
`docs/phase3/learning-loop/review-ui/screenshots/`. `MOZ_NO_REMOTE=1` keeps the test Firefox from attaching to a running one.

## Demo mode is unmistakable

The fixtures pair invented excerpts with real-looking GitHub URLs, so with `-demo` (`Options.Demo`) every page — inbox, item, bulk confirmation, error, in
both themes, and below the sticky header — carries a red **DEMO DATA — fixtures, not real upstream evidence** banner, and the upstream statement, each
evidence record and each excerpt is tagged **fixture**. Without `-demo` (the knowledge store) none of it renders. `TestDemoModeBannerAndFixtureTags`.

## Proxy decisions

A decision by an AI proxy settles an item like any decision: the item leaves `pending` (and the counters), is found under status *decided*, and its page
lists the decision as `proxy` and offers no further decision. The UI never records a proxy decision itself. (`TestProxyDecidedItemsLeavePendingAndAreMarked`.)
Reviewing a large high-priority queue: the inbox shows at most `-limit` rows (default 200) with a "first N of M" notice, highest priority first.
