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

## Next — step 2 (blocked on PO approval of the mockup)

- Port the renderers into `internal/reviewui` (view.go + templates) with tests: every condition op,
  every question type, every routing signal, undetermined/abstention, gate items.
- Keep all existing behaviour: five actions, bulk flow + guard, keyboard, reviewer/timing contract,
  demo banner + fixture tags, no-JS operation. No new dependencies (stdlib only).
- Rendered-delta section (RenderedDeltaSource) needs a ux-v2 treatment too — not in the three
  representative items; will design with the http01 fixture during implementation.
- Re-take the real screenshots; delete `internal/reviewui/uxv2mock`.

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

- None. Everything new: `internal/reviewui/uxv2mock/` (owned), `docs/phase3/learning-loop/review-ui/ux-v2/`
  (lane artifact), this status file.

## Test status

- `go build ./...`, `go vet ./...` clean; `go test ./...` green (2026-10-05) — the mockup generator
  adds no tests (throwaway; the renderers get tests when they land in internal/reviewui in step 2).
- No new third-party dependencies (stdlib + existing module packages only).
