# Lane `report` — status

Branch `p4ei/report`. Brief: `briefs/report.md`. **State: done.**

## Done
- `internal/investigate/reportui` (stdlib only):
  - `source.go` — `Source` over the live investigation store (`StoreSource`, built on `internal/investigate/store`: verified
    reads, `PutInvestigation` for writes, so the audit trail can only grow) and over stored evaluation runs (`RunsSource`,
    read-only; packed `cases.tar.gz` or unpacked `cases/<case>/`; key `<run-id>/<case>`). `Open(dir)` detects the layout
    (state dir, its `investigations/` dir, a results dir of runs, one run dir).
  - `view.go` — pure view model: list rows + filters (product / env / decision / run), newest first; the page (decision in
    plain English, release + collection status, environment + inputs, "what was checked" checklist, items attention-first,
    AI records, review history, audit chain re-verified per event, record digest via `run.Digest`).
  - `server.go` + `templates/` + `static/report.css` — routes under `/investigations`: list, `view/<key>`, `review/<key>`
    (POST), `export/<key>?format=html|json`, `artifact/<key>?digest=` (text/plain, nosniff, CSP none), static CSS.
    Same-origin check on POST.
- `cmd/ri`: `ri investigate serve [-store DIR] [-addr 127.0.0.1:8486] [-reviewer NAME]` (new file
  `investigate_serve.go`, registered via `investigateSubcommands`); `ri review serve -investigation-store DIR` mounts the
  same handler under `/investigations` with cross links; usage text.
- Docs: `docs/INVESTIGATE.md` "Investigation report (per upgrade)…" section; `docs/REVIEW_UI.md` (mount + route row);
  README command line.
- Screenshots (headless Firefox, ports 8497/8498, servers stopped; 8484/8485 untouched):
  `docs/phase4/env-impact/review-ui/10-investigations-list.png` (stored runs, arm B filter),
  `11-investigation-page.png` (kube-prometheus-stack obs-north, arm B), `12-investigation-review.png` (a scratch copy of
  that record in a scratch state store, after a request-more-evidence decision posted through the form route).

## Decisions
- **Human-only form.** The review form never sends a reviewer kind; the server always builds `ReviewerKind: human`
  and a posted `reviewerKind` is ignored. Reviewer and reason are both required (the brief: "with reviewer name and
  reason"); "more" without named inputs records `see note` (the ReviewApplier needs ≥1 request).
- **Status after a decision** mirrors `run.Pipeline.ApplyReview` (what `ri investigate review` does on a stored run): a
  decided review (approved / rejected / more-evidence-requested) sets the run `completed`. The decision itself is
  `classify.NewReviewApplier()`. Temporal-driven runs: reviewed with the `--temporal` signal (documented); the page
  writes the stored record.
- **One-sentence reason** is built from structured fields (failing validation + first line of its detail; passing
  validators + basis; plausible-impact signals; first gap) — never from AI output.
- **AI separation**: every AI record (agent coverage summary, assertions by stance, hypotheses, setting claims,
  validation/evidence requests, investigation-level agent notes) renders only inside a dashed "AI OUTPUT — advisory, not
  a validation" box with model/provider/prompt/call id; validations never render inside it (tested).
- **Export** = the page sections with `Offline` links: inlined CSS, no scripts/forms/server links, every item open, the
  run-record JSON embedded as a `data:` download link (byte-identical to `?format=json`, tested).
- **CSS**: `report.css` copies the review UI's design tokens (so it looks like one product) instead of linking
  `/static/app.css`, because the export must be self-contained and the standalone server does not mount reviewui.
- Knowledge-fact verification level is shown only when the store holds the fact's bytes (stored evaluation runs carry
  no CAS: the page says "not readable here" instead of guessing).
- Stored runs: list rows cached forever (immutable); live store rows cached by run-file size+mtime.

## Files touched outside ownership (additive)
- `internal/reviewui/server.go`: `Options.Links []Link` + `toplinks` template func.
- `internal/reviewui/templates/{inbox,item,bulk}.html`: one `{{range toplinks}}` link in the top bar.
- `internal/reviewui/static/app.css`: `.toplink` (one line).
- `cmd/ri/review.go` (wiring, my ownership per brief), `cmd/ri/main.go` (usage lines), `README.md` (one line).

## Contract changes
- None. No domain types changed; no schema regeneration needed.

## New dependencies
- None (stdlib). `reportui` imports `internal/investigate/run` (for `run.Digest`) and `internal/investigate/store`.

## Open questions / known gaps
- No pagination on the list (128 rows for all 17 stored runs, ~1.3 s cold, cached afterwards).
- Stored evaluation runs keep no artifact store, so render/diff/manifest artifact names show without links there; the
  live store links them. Rendered before/after shows the `RenderedChange` field values (the excerpt the diff index
  holds), not full-object YAML.
- The badge "upstream: action required" also appears on not-affected items whose upstream note is
  action-required but the environment does not use the key — correct data, may read oddly; wording could be refined.

## Tests
- `internal/investigate/reportui/reportui_test.go`: filter + newest-first; list from a real store (runtest fixture);
  attention order + collapsed not-affected + reason text; AI labelling/separation; human review recorded with reason
  (+ refusals: no reason, no reviewer, bad action, second decision; posted reviewerKind ignored); more-evidence requests
  in the audit; proxy decision recorded but never approves (real classify ReviewApplier); read-only stored runs (packed
  and unpacked), cross-origin refusal; self-contained export + embedded JSON identical to the record; artifact
  endpoint text/plain + 404s.
- `go build ./... && go vet ./... && go test ./...` green on this branch (2026-10-16).

## Cleanup
- Servers on 8497/8498 stopped; `bin/` and a stray `ri` binary removed; scratch store/screenshots under the session
  scratchpad removed; no kind clusters created.
