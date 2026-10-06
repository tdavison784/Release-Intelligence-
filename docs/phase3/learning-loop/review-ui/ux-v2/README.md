# Review item ux-v2 — "decision card" mockup (step 1, for approval)

Product-owner feedback this answers (2026-10-05): *"very hard to understand … not easy to see what was
recommended by the models, what the issue is that it wasn't able to resolve … a lot of data grouped
together that isn't clear."*

Open the HTML files in a browser (self-contained, CSS inlined; light and dark follow your system):

| File | What it shows |
|---|---|
| `inbox.html` | the redesigned inbox row: plain-English question + the one reason it needs you + suggested answer (no aspect chips) |
| `item-disagreement.html` | **model disagreement** — cert-manager 1.18 `rotationPolicy` default flip, 3 calls, 3 different answers on the consequence |
| `item-thin-evidence.html` | **evidence-thin one-liner** — istio 1.27 ambient note, no model could tell what changed |
| `item-consensus-action.html` | **consensus-ACTION audit** — cert-manager 1.18 rejects RSA keys < 2048; two models agree it is ACTION REQUIRED |

Screenshots (taken with headless Firefox, 1440 px) are in `screenshots/`; `item-disagreement-dark.png`
is the dark theme. Buttons are non-functional — this is a static mockup.

## The card, top to bottom

1. **The question, one sentence in plain English**, generated deterministically from the question type
   and the proposed assertion (no LLM): *"Does cert-manager v1.18.0 change what happens to Certificates
   where `spec.privateKey.rotationPolicy` is left unset — and does that require action?"*
2. **What changed upstream** — the best evidence quote (the record the models cited most) with a link,
   product/release chip, and "+ N more sources in Details".
3. **Recommendation** — "Suggested answer: …" plus the honest reasons to trust it, each ticked
   ✓ / ? / ✕: *"All 2 calls give the same answer"*, *"Checked: the field exists in the v1.18.0 CRD"*,
   *"Evidence is thin: one source, a single line."* One primary button: **Accept suggested answer**
   (on the evidence-thin item it becomes **Send back — need more evidence**).
4. **Why this needs you** — one highlighted line translating the routing signals:
   *"The models disagree on who is exposed and what happens — up to 3 different answers from 3 calls."* /
   *"No model could tell what changed: the note names no concrete field, flag or API."* /
   *"High impact: separate model calls agree this is ACTION REQUIRED — policy sends every such consensus
   to a human audit. This is that audit."*
5. **Where the models differ** — one row per part that actually differs (not all parts), each cell one
   sentence with the agreeing models grouped (*"Opus 5.5 + Sonnet 5.5"*). Identical parts collapse into
   the green line *"All 3 calls agree on: what the thing is, what changed."*
6. **Correct** (collapsed) — editing stays per part, and every part shows a plain-English preview of
   what it currently says ("Currently says: Exposed: Certificates where … is left unset").
7. **Details** (collapsed) — full evidence list, validator checks, raw proposals with call ids, prompt
   digests and provenance, machine ids. For auditing only; no id appears outside it.

Conditions render as English everywhere ("Certificates where `spec.privateKey.rotationPolicy` is left
unset", "ingress-nginx is older than 1.12.0"); enums carry human labels ("workloads fail", "the resource
is rejected"); the few remaining terms (*call*, *aspect*, *validator*) have hover glossary tooltips.

## Everything shown is real fixture data

The pages are generated (`go run ./internal/reviewui/uxv2mock`) from the same demo fixtures the review
UI serves with `-demo`: the wording is produced by deterministic renderers in
`internal/reviewui/uxv2mock/english.go` — nothing is hand-written per item, and no LLM runs at render
time. Those renderers are what step 2 ports into `internal/reviewui` with tests once this direction is
approved. The DEMO DATA banner and fixture tags stay.

## What we need from you

- Does the card answer "what was recommended, and what couldn't the models resolve" within ~30 seconds?
- Is the single **Accept suggested answer** primary button the right default action?
- Anything you still want visible without expanding (currently: rendered-delta evidence appears only in
  step 2's item design — say if it belongs in section 2 instead of Details).

## Step 2 (after approval)

Implement in `internal/reviewui` (stdlib only): the English renderers with tests (every op, every
question type, every routing signal), the redesigned inbox rows, keeping all existing actions, bulk
flow, keyboard shortcuts, guards and the demo banner; re-take screenshots. The `uxv2mock` generator is
deleted then.
