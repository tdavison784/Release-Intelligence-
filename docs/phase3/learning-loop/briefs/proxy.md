# Lane `proxy` — blind AI proxy review of the knowledge store (product owner decision 2026-10-02)

Decision: a blind Opus proxy reviews every non-high-priority review item, labelled `proxy`; the product
owner reviews the high-priority items himself on the dashboard; afterwards the proxy shadow-reviews those
high items in a COPY so proxy-vs-human agreement can be measured without biasing the human.

Read: `docs/phase3/learning-loop/FLEET.md` (binding), `MISSION.md` (Goals 6–15), `DESIGN.md` (§2.4–2.6,
§4, §6, §7), `DECISIONS.md` (PO-1…PO-4), `docs/KNOWLEDGE.md`, `docs/REVIEW_UI.md`. Branch `p3ll/proxy`,
worktree `.claude/worktrees/proxy`.

**BLINDNESS (hard rule):** you and every prompt you build must never read or include anything under
`eval/` (cases, results, adjudications, render evals), `docs/phase3/learning-loop/UNKNOWN-ANALYSIS.md`,
`TRUSTFIX.md`, or any eval report. Review decisions are about upstream release semantics only. Review items
are environment-free; do not add environment context.

## Build
1. A proxy-review runner (script + small Go/CLI glue if needed) that, for each PENDING review item whose
   route priority is NOT `high`, builds a self-contained prompt from `knowledge.ReviewContext` (question,
   upstream statement + evidence excerpts with URIs, the proposed assertion, every model proposal side by
   side, validation results, previous related decisions) and asks ONE stateless
   `claude -p --model claude-opus-5-5` call (structured JSON output; no tools; neutral cwd — follow
   `scripts/semantic-exchange.sh` for the pattern) to decide: accept | reject | correct (with the corrected
   aspects, enum-valid) | need-more-evidence | defer, with labels per DESIGN §2.5 and a reason that cites
   evidence ids. The proxy may consult only the evidence in the prompt.
2. Record each decision with `ri knowledge decide --reviewer-kind proxy` and complete `ProxyProvenance`
   (model, provider anthropic, modelVersion from the CLI envelope, promptVersion `proxy-review/v1`,
   promptDigest, call id, inputEvidence, generatedAt), StartedAt/DecidedAt = call start/end. One decision
   per item, never batched. Failures/refusals recorded, never dropped, never retried blindly.
3. Self-review bias: record per decision which model families authored the proposals under review, so
   metrics can split "proxy reviewing its own family" vs not.
4. Run it on all non-high pending items in `knowledge/` (≈ 670 + 268 evidence-sufficiency; you may
   batch evidence-sufficiency items more cheaply only if each still gets its own call). Mind the account
   usage (the fleet guard pauses the fleet at 90%). Commit the store changes with a provenance message.
5. Shadow pass (ONLY after the commander says the product owner has finished the high items): run the same
   runner on the high items against a COPY of the store at `docs/phase3/learning-loop/proxy-shadow/` (never
   the real store), then compute proxy-vs-human agreement per question type and aspect.
6. Report: decisions by action/label/question type, accept/correct/reject rates per proposing model,
   self-family vs other-family rates, cost, time. `ri knowledge metrics` output. Facts minted at proxy level.

Integrity test must pass. End with `LANE DONE: proxy` (and later `LANE DONE: proxy-shadow`). Model: Opus.
