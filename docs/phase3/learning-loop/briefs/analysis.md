# Lane `analysis` — why is applicability UNKNOWN? (MISSION Goals 17, 18, 22 input)

You are a research analyst. Read-only on code and data; you write one document. Read first:
`docs/phase3/learning-loop/MISSION.md`, `docs/phase3/learning-loop/FLEET.md`, `docs/phase3/OUTCOMES.md`,
`eval/REPORT.md` (Phase 3 sections), `eval/FORMAT.md`, `docs/ACTION_CLASSIFICATION.md`, `docs/IMPACT.md`,
`internal/eval/compare.go` (link-hit semantics: a link is hit only when an AFFECTED-class finding joins
a change matching the expected item).

Branch `p3ll/analysis`, worktree `.claude/worktrees/analysis`. Owns only
`docs/phase3/learning-loop/UNKNOWN-ANALYSIS.md` (+ your status file).

Get the data: `go build -o bin/ri ./cmd/ri && GITHUB_TOKEN=$(gh auth token) bin/ri -state
/Users/tommydavison/repos/Release-Intelligence-/.ri eval -o json > /tmp/...` (warm cache; check flag
placement with `bin/ri -h` / `bin/ri eval -h`). Also run `bin/ri impact ...` per environment case to read
the actual findings (fixtures under `eval/cases/<id>/environment/`).

## Produce UNKNOWN-ANALYSIS.md

1. **Per applicability link** (all 21 `expectedImpact` links across the 8 environment cases; mark the 2
   currently hit): expected item, relevance, the change(s) that match it, the finding(s) the join emits
   for them today and their class/rule/neededToDetermine, and:
   - **cause class** (Goal 18): release-knowledge-gap, environment-visibility-gap,
     cross-product-context-gap, runtime-behavior-gap, evidence-gap, semantic-ambiguity (more than one
     if true; name the dominant one);
   - **missing semantic structure**, written as the structured fact that would close it (subject
     family + identity, change type, before/after, applicability condition, consequence) — release-level,
     generic, no product-specific code;
   - **environment evidence that decides it** — is it already present in the fixture (which file/path)?
     present but not parsed by `internal/env`? absent (needs a new input, e.g. other products' versions)?
   - **deterministic validation possible?** — can the subject/change be proven from artifacts the
     pipeline already ingests (CRD schemas, values.yaml/schema, compat tables, images), or only from
     prose (→ needs human review)?
   - **reachable class** under an honest trust ladder: with a human-verified fact, what class should the
     deterministic engine produce, and is that an AFFECTED class (hit)? Flag any link where the honest
     answer is still UNKNOWN (genuinely undecidable from supplied env) — do not force it.
2. **The ACTION→UNKNOWN confusion cells** (28) and the 5 ACTION→NOT-AFFECTED cells: same treatment in a
   compact table; call out the NOT-AFFECTED ones especially (those are the dangerous cells).
3. **Aggregates**: counts by cause class and subject family; which env parsing extensions and which
   validators would unlock the most links; a ceiling estimate of applicabilityAccuracy reachable with
   (a) deterministic validation only, (b) + verified facts, (c) + cross-product inventory — with the
   assumptions stated. Be honest if 0.80 looks out of reach without new environment inputs.
4. **Dataset observations** — expectations whose `relevance`/`why` encode operator knowledge the
   supplied environment cannot support. Do NOT edit any case; just list them with reasoning for the
   `groundtruth` lane and the commander.

Integrity: you may read case expectations (you are the analyst, not a fact author), but you must not
author knowledge facts or edit eval data. Commit the document early (partial is fine — the `contract`
architect is waiting on it) and update it as you go; tell the commander via your final message.

Model: Opus.
