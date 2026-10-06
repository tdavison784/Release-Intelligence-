# Lane `reviewux` — make a review item understandable in 30 seconds

Product owner feedback (2026-10-05): "very hard to understand … not easy to see what was recommended by the
models, what the issue is that it wasn't able to resolve … a lot of data grouped together that isn't clear."

Read: FLEET.md, docs/REVIEW_UI.md, internal/reviewui (server, view, templates, static), knowledge.ReviewContext,
DESIGN.md §2 (aspects), §6 (routing signals), §1.3 (condition language). Branch `p3ll/reviewux`.

## Redesign the item (expanded card + item page) around a "decision card", top to bottom
1. **The question, in plain English** (one sentence, generated deterministically from the item's question type
   + proposed assertion — no LLM): e.g. "Does cert-manager 1.18 change how Certificates that leave
   `rotationPolicy` unset behave — and does that require action?"
2. **What changed upstream** — one short quote (the best evidence excerpt) + link; product/release chip.
3. **Recommendation** — "Suggested answer: <plain-English statement>" + how confident and why: "3 of 4
   models agree", "validator confirmed the field exists and the default changed", "rendered diff shows …".
   One primary button: **Accept suggested answer**.
4. **Why this needs you** — a single highlighted line translating the routing signals: "Models disagree on
   whether this requires action (Opus: action, Sonnet: review)", "No model could tell which config key this
   is", "Evidence is a one-line changelog entry — PR #1234 attached", "High impact: a model requested ACTION
   REQUIRED".
5. **Where the models differ** — a compact plain-language comparison table, ONE row per aspect that differs
   (not all aspects), each cell a sentence ("Exposed when: any Certificate omits spec.privateKey.rotationPolicy"),
   agreeing models grouped; identical aspects collapsed into "All models agree on: what changed, …".
6. **Correct** — editing stays per aspect but each field shows a plain-English preview of what it means.
7. **Details (collapsed)**: full evidence list, validation records, raw proposals, provenance/digests — for
   auditing only.
- Render every condition tree as English ("Clusters where all of: ingress-nginx is older than 1.12.6; any
  ClusterIssuer sets solvers[].http01.ingress"), every enum with a human label, every id hidden behind details.
- Inbox rows: the plain-English question + the "why this needs you" line + suggested answer, instead of
  aspect chips.
- Glossary tooltip for the few remaining terms.

## Process
1. FIRST: a static mockup (HTML snapshot via the demo fixtures) of 3 representative items — a model
   disagreement, an evidence-thin one-liner, a consensus-ACTION audit — screenshot them with headless Firefox
   (`/Applications/Firefox.app/Contents/MacOS/firefox --headless --screenshot …`), commit under
   `docs/phase3/learning-loop/review-ui/ux-v2/`, and STOP with a message 'MOCKUP READY' so the product owner
   can approve.
2. After approval: implement, keep all existing actions/bulk/keyboard/guards, tests for the English renderers
   (every op, every question type, every routing signal), re-take screenshots. No new JS framework; stdlib.
Reply `LANE DONE: reviewux`.
