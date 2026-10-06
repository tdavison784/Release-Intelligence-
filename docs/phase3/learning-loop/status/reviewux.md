# Lane `reviewux` — status

Branch `p3ll/reviewux`. Goal: make a review item understandable in 30 seconds (PO feedback
2026-10-05). Brief: `briefs/reviewux.md`.

## Done — step 1: the mockup (awaiting product-owner approval)

- `internal/reviewui/uxv2mock/` — throwaway generator (`go run ./internal/reviewui/uxv2mock`) that
  renders the step-1 mockup from `reviewui.NewDemoQueue()` fixtures. Everything shown is derived
  deterministically from fixture data (no LLM, no hand-written per-item prose):
  - `english.go` — the plain-English renderers this lane is really about: `plainQuestion` (question
    type + proposed assertion → one sentence), condition trees as English (`condEnglish` — every op in
    DESIGN §1.3 the fixtures use, generic fallback for the rest), `changeSentence` /
    `consequenceSentence` / `aspectSentence`, human labels for every enum (consequence kinds, change
    kinds, severities, classes, statuses, question types), `whyYou` (routing signals → the one
    highlighted line, incl. the PO-2 consensus-action audit phrasing), `whyConfident` (agreement
    counts + validator outcomes + evidence breadth, each ticked ✓/?/✕), `consensusAction` mirroring
    `itemView.noteConsequenceConsensus` (PO-2/PO-5).
  - `mock.html` / `mock.css` — the decision card: 1 question · 2 what changed upstream (best-cited
    evidence quote + link) · 3 recommendation (suggested answer + trust reasons + ONE primary button;
    on gate items the primary becomes "Send back — need more evidence") · 4 why this needs you (single
    highlighted line) · 5 where the models differ (one row per differing part, agreeing models grouped,
    identical parts collapsed to a green "All N calls agree on: …") · 6 correct (collapsed, per-part
    edit with plain-English previews) · 7 details (collapsed: evidence, checks, raw proposals with
    call ids + digests, machine ids — no id outside it) · decide form (all five actions kept).
    Glossary tooltips (CSS-only) for call/aspect/validator. Light+dark via the same tokens as app.css.
  - Output (committed): `docs/phase3/learning-loop/review-ui/ux-v2/{inbox,item-disagreement,
    item-thin-evidence,item-consensus-action}.html` + `screenshots/*.png` (headless Firefox 152,
    dedicated `-profile` because the user's Firefox holds the single-instance lock; absolute
    `--screenshot` paths — relative ones silently no-op).
- The three representative items: `rotationPolicy` fixture (3-model disagreement), istio ambient
  (evidence-thin one-liner, all aspects undetermined), RSA < 2048 (PO-2 consensus-ACTION audit).
- Inbox rows redesigned per the brief: plain-English question + why-line + suggested answer.

## Done — step 1b: visual polish pass (PO approved structure, asked for flavor only)

PO feedback 2026-10-05: *"format looks good … keep the structure as is … just make it more visually
appealing."* Structure, section order, wording and content generation are untouched; the pass is
visual only:

- Priority-colored gradient strip across the card top (danger→warn for high, muted for low).
- Numbered stepper: circles 1–7 + a green ✓ on Decide, on a rail down the card's left edge.
- Serif question headline; chips carry tone dots; recommendation panel gets a gradient wash and a
  gradient primary button with hover lift; "why this needs you" gets a circular "!" badge; differ
  cells get per-group left-border colors and model avatars (initial in a hue hashed from the model
  id — deterministic); agree line gets a ✓ chip; folds get rotating chevrons; inbox rows get a
  priority stripe and hover lift. Dark-mode variants for all of it.
- Fixed while re-screenshotting: the step-1 pages had a dead stylesheet — html/template's CSS value
  filter collapses a plain-string `{{.CSS}}` in `<style>` to `ZgotmplZ`, and the step-1 screenshots
  had silently been rendered from Firefox's cached earlier copies of the pages (same file:// URLs,
  stale profile), so the mismatch went unnoticed. Now: stylesheet passed as `template.CSS`
  (inlines verbatim — verified byte-identical to mock.css), screenshots always taken with a fresh
  `-profile` (no cache), and the four light shots pin `data-theme="light"` in a sed'd copy because
  this machine runs macOS dark mode (otherwise `prefers-color-scheme` renders both variants dark).

## Done — step 2: ux-v2 is the real review UI (PO: "make this the new standard", 2026-10-05)

- `internal/reviewui/english.go` — the renderers, ported from the mock and extended:
  `plainQuestion` (every QuestionType + nil-subject variants + fallback to the recorded question),
  `condEnglish` (every condition op incl. field states in-range/out-of-range/exists/none that the mock
  missed, version ranges, undecidable, generic fallback), `subjectPhrase` (every family), `changeSentence`,
  `consequenceSentence`, `aspectSentence`, `exposurePhrase`/`overlapPhrase` (shielded-when), `suggestedAnswer`
  (normal + gate with the models' undetermined reason), `gateReason`, `whyYou` (every routing signal, incl.
  the PO-2 consensus-action audit phrasing), `whyYouBrief` (signals-only, for inbox rows), `whyTone`,
  `whyConfident` (per-aspect agree/disagree/abstain, validator outcomes, evidence breadth), `buildDiffer`
  (one cell per answer group, biggest-first + model-name tie-break, consensus labels per PO-1,
  "= the suggested answer" marker, single-group aspects → chips, undetermined reason), `modelShort`
  (claude-opus-5-5 → Opus 5.5; title-cases unknown tokens) + `avatarOf` (hue hashed from the raw id).
- `view.go` — `rowView` gains Question/Why/WhyTone/Suggested/ClassLabel; `itemView` rebuilt for the
  card (UpQuote/UpLink/UpSource/UpMore best-cited evidence, Conf, PrimaryLabel/PrimaryAction,
  WhyLine/WhyTone, Differ/Agrees/Undetermined, raw per-call `Calls` with avatars); the matrix
  (`buildMatrix`) is deleted; `noteConsequenceConsensus` (PO-2) and `distinctCalls` kept; correction
  view gains plain-English "Currently says:" previews per fieldset.
- Templates rewritten: `detail.html` is the 7-section decision card (rendered-delta section between 5
  and 6 when present; decide form and correct form markup preserved verbatim — only wrapped);
  `item.html` slimmed to bar + card; `inbox.html` rows lead with the plain question + why-line +
  suggested answer (fixture title kept as `.qtext` so title-based tests/filters still work).
- `app.css`: tokens extended (glow/dot/rail/serif/group hues), layered background, priority stripes,
  the whole dcard system (stepper rail, quote, gradient primary, differ cells, avatars, folds,
  glossary tooltips, narrow + print). Dead matrix CSS removed; demo-banner/fixture/req-*/agree-*
  badge styles kept (test-asserted).
- `app.js`: two changes only — the delegated click handler also resolves `b.form` (the §3 primary
  button sits outside `form.actions` and references it via the `form` attribute), and `showCorrect`
  opens the §6 fold.
- Tests: `english_test.go` (new) — every condition op and field state, version range shapes, every
  question type incl. fallbacks, every routing signal for `whyYou`/`whyYouBrief`/`whyTone`,
  `suggestedAnswer` normal/gate/statement-only, differ grouping/ordering/chips/undetermined,
  conf-line tones, model names/avatars, sentences, truncate. `server_test.go` layout assertions
  updated to the new rendering (behavior tests untouched; the recorded question stays verbatim in
  Details; a "models disagree" badge marks §5).
- Browser verification re-run, all three scripts green against the new DOM: `e2e_firefox.py` (every
  behavior assertion passes; script updated — `steady()` pins transitions/transforms because the
  ux-v2 hover lift and fixed bulk bar made coordinate clicks racy, targets scrolled into view, and the
  inline-reason selector scoped to `form.actions` because the §6 correct fold's hidden `reason`
  textarea now precedes the decide form), `shots_firefox.py` (selectors `table.matrix` → `#differ`;
  `os.makedirs` added), `shot_priority_firefox.py` (unchanged). Each e2e run needs a fresh server —
  the suite records decisions and the demo queue is in-memory.
- Screenshots re-taken from the live server (fresh Firefox profiles, light pinned via sed):
  `docs/phase3/learning-loop/review-ui/screenshots/` — the ux-v2 design set (0x/1x: inbox, item,
  details fold, correct form, consensus-action, rendered delta, gate, narrow) plus the verified
  behaviors from the e2e and section scripts (2x). No-server snapshots regenerated as
  `docs/phase3/learning-loop/review-ui/ux2-*.html` (+ `inbox-truncated.html`); old pre-ux-v2
  snapshots removed. `internal/reviewui/uxv2mock/` deleted; its README keeps the design record with
  an "implemented" banner. `docs/REVIEW_UI.md` rewritten around the 7-section card.

## Decisions taken (local)

- Mockup is generated, not hand-written HTML: proves the "deterministic, no LLM" wording is actually
  derivable from the data, and step 2 becomes a port, not a rewrite.
- Primary button label varies by item shape (accept vs need-more-evidence on gate items) — the
  suggestion is what you accept, and on a gate the honest suggestion is "not enough evidence".
- Model display names shortened deterministically (claude-opus-5-5 → Opus 5.5); raw ids stay in
  tooltips and Details.
- Differ rows ordered: largest agreeing group first, then model name (deterministic tie-break).
- Model avatars (differ cells + raw-proposal table) hash the raw model id to a hue class av0–av5 —
  same model, same color, always; initials from the shortened display name.
- Light screenshots on a dark-mode host require pinning `data-theme="light"`; never trust a
  screenshot taken with a reused Firefox profile (file:// cache serves stale documents).

## Files touched outside ownership

- Step 1/1b: none. Step 2 works in this lane's owned surface: `internal/reviewui/` (all of it:
  english.go, view.go, templates, static, server_test.go, english_test.go — the review UI is this
  lane's file list per FLEET.md), `docs/REVIEW_UI.md` (the review UI's own doc), and the lane
  artifacts under `docs/phase3/learning-loop/review-ui/`.

## Test status

- `go build ./...`, `go vet ./...` clean; `go test ./...` green (2026-10-05), including the new
  english_test.go and the updated server_test.go.
- No new third-party dependencies (stdlib + existing module packages only).

## Lane complete

Both steps done; the approved ux-v2 is the review UI standard. Nothing blocked, nothing deferred.
