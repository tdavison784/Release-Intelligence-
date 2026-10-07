# Learning-loop fleet: rules every lane follows

The learning-loop work (docs/phase3/learning-loop/MISSION.md) is built by several agents in parallel,
each in its own git worktree and Herdr tab, coordinated by a commander session. This file is the
contract between lanes.

## Branches and worktrees

- Integration branch: `p3-learning-loop` (base: `main` @ c341555). Only the commander merges into it.
- Each lane works on its own branch `p3ll/<lane>` in `.claude/worktrees/<lane>`, branched from
  `p3-learning-loop`. Commit early and often on your lane branch. Never push, never merge into other
  branches, never rebase other lanes' branches. If you need newer integration work, the commander will
  tell you to `git merge p3-learning-loop` into your branch.
- Commit messages: imperative summary line, body explaining why; end every commit message with
  `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (use your own model name if you are not Opus).

## File ownership

Each lane brief lists the files/packages it owns. Edit outside your ownership only when unavoidable,
keep such edits minimal (additive hunks), and list them in your status file so the commander can
anticipate merge conflicts. `internal/domain/semantic.go` and `docs/phase3/learning-loop/DESIGN.md` are
owned by the `contract` lane; once merged, other lanes propose changes to them via their status file
instead of editing them unilaterally (small additive fields are OK, note them).

## Engineering conventions (from docs/ARCHITECTURE.md; non-negotiable)

- Deterministic first. LLMs only at extraction/proposal/review-assist time, never required for a
  runtime impact decision.
- Facts ≠ conclusions ≠ AI. Every AI output carries full provenance (model, provider, modelVersion,
  promptVersion, promptDigest, inputEvidence, generatedAt, confidence).
- Evidence everywhere; content-derived IDs; producer strings `component@vN`.
- **Declarative, generic constructs only. No product-specific Go logic** (no `if product == "cert-manager"`).
- Tests use no network (httptest, fakes, checked-in caches). `go build ./... && go vet ./... && go test ./...`
  must pass on your branch before you report done. If you change `internal/domain` types, regenerate
  schemas: `go run ./internal/domain/schemagen`.
- Update the relevant docs (docs/*.md, README command list) for anything user-visible.
- No new third-party Go dependencies without stating why in your status file (stdlib preferred:
  `net/http`, `html/template`, `embed`, `encoding/json`).

## Evaluation integrity (the most important rules)

- `eval/gates.yaml` thresholds are pre-registered. Never edit them.
- Never edit existing `eval/cases/*/case.yaml` expectations or `eval/results/*` to move a number.
  Expectations are never derived from pipeline output. New ground truth (the `groundtruth` lane) is
  authored blind from upstream sources, with citations, and recorded in NOTES.md.
- Knowledge facts are **release-level** ("cert-manager v1.18.0 changed X"), never tuned to a specific
  eval environment. Whoever authors or reviews a fact must not consult `eval/cases/*/case.yaml`
  `expectedImpact`/`expectedFindings` while doing so.
- Verification provenance is honest: `deterministic` (a validator proved it from artifacts),
  `human` (a named human reviewer), `proxy` (an AI acting as reviewer — always labelled, never
  presented as human). Results are reported per verification level.
- The ACTION REQUIRED contract (docs/ACTION_CLASSIFICATION.md, MISSION Goal 21) is never weakened. A model
  output can never directly produce or suggest `action-required`.
- A failing gate is a finding to report, not something to hide.

## Running things

- Build: `go build -o bin/ri ./cmd/ri` (bin/ is gitignored, per worktree).
- Eval (live, populates `.ri/cache`): `GITHUB_TOKEN=$(gh auth token) bin/ri eval`. The primary checkout's
  cache is warm; reuse it with `-state /Users/tommydavison/repos/Release-Intelligence-/.ri` rather than
  re-fetching. Baseline (main @ c341555, 17 entries): all gates pass except applicabilityAccuracy 0.095
  (2/21 links). Current p3-learning-loop (28 entries incl. the groundtruth expansion, no -knowledge):
  applicabilityAccuracy 0.476 (0.514 with -render) and falseActionRate 0.059 (kyverno E9, pending a
  verified 'superseded' fact) FAIL; the other gates pass. Re-check with `ri eval` before quoting numbers.
- LLM calls: no API keys are set in the environment. Use the file exchange (`-llm-exchange <dir>`) and
  answer requests with stateless `claude -p` calls in the style of
  `docs/phase3/model-comparison/raw-arm.sh` (Claude models: claude-opus-5-5, claude-sonnet-5-5,
  claude-haiku-4-5). GLM-5.3-Flash needs the user's Z.AI key (ANTHROPIC_API_KEY + ANTHROPIC_BASE_URL);
  treat it as optional. Codex models are unavailable until 2026-10-03.

## Reporting

- Keep `docs/phase3/learning-loop/status/<lane>.md` current on your branch: what's done, what's
  next, decisions taken (with why), files touched outside ownership, open questions, test status.
- If blocked on a decision only the commander/user can make, write it in the status file, state it as
  your final message, and stop. Don't guess on contract questions; do pick sensible defaults on local ones.
- When finished: commit everything, then end with a final message that starts with `LANE DONE:` followed
  by a ≤15-line summary (branch, commits, what works, test results, known gaps).
