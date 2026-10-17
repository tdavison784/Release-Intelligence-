# Status — lane `reviewfilter` (Claude Sonnet 5.5, agent ri-reviewfilter)

Branch `p4ei/reviewfilter`, worktree `.claude/worktrees/p4ei-reviewfilter`. Brief: `briefs/reviewfilter.md`.
State: **DONE** (2026-10-16).

## Done
- `internal/reviewui/triage.go`: per-item verdict (needs review / hidden + reason / blocking), `BlockingScope`
  (`ScopeFromInvestigations` reads `eval/investigations/*/input.yaml` products — inputs only, never expected.yaml —
  and the upstream repos their `products/<p>.yaml` read).
- Server: default `/` = Needs review, `?scope=all` = "Show all staged data"; counts/tiles reflect the default view;
  blocking-first ordering; hidden tally with reasons; server fails open (an item the snapshot doesn't know is shown).
- `validate.CanBePinSource` (additive export in `internal/investigate/validate/pins.go`).

## Also done
- Tests `internal/reviewui/triage_test.go` (each hidden category, consensus-only not hiding, priority/blocking order,
  show-all toggle + labels, counts, decide-leaves-view, scope derivation never reading expected.yaml, fail-open).
- `ri review serve` prints the waiting count; flag `-investigations` (global `-products` reused); docs/REVIEW_UI.md section.
- Browser check: headless Firefox screenshots on port 18601 (server stopped; 8484 untouched) in
  `docs/phase4/env-impact/review-ui/` (`01-needs-review-default.png`, `02-show-all-staged-data.png`). The Selenium
  e2e suite was not re-run: the venv at /tmp/riv has no selenium and the JS is unchanged.
- Real store (`knowledge/`): 2813 items → 641 need review (21 block investigations: cert-manager/prometheus-operator),
  2172 hidden (2139 proxy-acted, 32 superseded/retracted, 1 decided). No ingress-nginx or kube-prometheus-stack
  knowledge exists in the store yet, so no item of those chart/product ids can show up.
- `go build ./... && go vet ./... && go test ./...` green.

## Decisions
- Needs review = status pending ∧ no decision of any kind ∧ no active fact holding the item's aspects at
  human/deterministic level ∧ (audit items) audited fact still active. needs-evidence/deferred are decisions → hidden
  (reachable via show-all), per the brief's "no human decision yet".
- Blocking = item of an investigated product (or of an upstream repo an investigated product reads) whose candidate has
  only prose members (⇒ manifest:declared-behaviour, controller-behaviour) and no trusted fact. Items whose proposed
  assertion `CanBePinSource` (pin rule derivation) rank first, other blockers next, the rest after (stable inside).
  Approximation: computed `crd:default-changed` candidates (also controller-behaviour) are not flagged (rule not on the candidate).
- Existing inbox tests now use `getAll` (full view); behaviour of the full view is unchanged.

## Files outside ownership
- `internal/investigate/validate/pins.go`: additive `CanBePinSource`.
- `cmd/ri/review.go` (+ main.go usage line): flags/wiring.

## Open questions
- Default limit 200 truncates the 641 (notice shown); start with `-limit 700` to see all in one page.

## Follow-up (commander request, 2026-10-16) — DONE
- Default `/` = blocking items only (pin sources first); `/?scope=other` = "Other open items (N)"; `/?scope=all` unchanged.
  Header counters cover the listed view. The old `blocking=yes` filter/tile is gone (the default is that filter).
- Gap closed: `controllerBehaviour` also treats a computed `crd:default-changed` candidate as blocking; the rule is
  derived from the "computed artifact diff (<rule>)" line in the candidate text (no contract change).
- Startup line prints both counts. Real store: 21 block investigations, 620 other open (641 total open); the store
  currently holds no crd:default-changed candidate, so the count did not move.
- Screenshots replaced: `docs/phase4/env-impact/review-ui/01-default-blocking-only.png`, `02-other-open-items.png`.
- Tests: other-view, crd-default rule, updated hidden-category/order/count tests; build/vet/test green.

## Follow-up 2 (commander, proxy + pin sources + narrowing) — DONE
- Proxy rule fixed: a proxy decision / proxy- or consensus-level fact does not close a blocking item (human decision or
  human/deterministic fact only); proxy-acted non-blockers stay hidden. A human can decide a proxy-decided item (`decidable`).
- rotationPolicy (cert-manager 1.18.0): fact vf-9b9cfa5c9f5f is active, consensus-level (subject deterministic, rest
  consensus), auto-approved; its 3 review items are *superseded* (pipeline supersedes questions when a fact is
  auto-approved) and there is no audit item, so no item exists to show. Not a proxy-hiding problem. `ri review
  open-audits` opens audit items for it and 2 revisionHistoryLimit facts (dry run lists 3; not applied to the store).
- Version ranges: scope reads from/to of each investigation input; items outside (from, to] do not block.
- Views: default = pin sources; `?scope=blocking`; `?scope=other`; `?scope=all`.
- Screenshots (taken on a scratch copy of the store after `open-audits -apply`): `review-ui/01-default-pin-sources.png`,
  `02-other-blocking-items.png`.
