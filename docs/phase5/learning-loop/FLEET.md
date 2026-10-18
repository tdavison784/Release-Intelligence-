# Learning-loop fleet: rules every lane follows

Phase 5 (MISSION.md in this directory) is built by several agents in parallel, each in its own git
worktree and Herdr tab, coordinated by a commander session. This file is the contract between lanes.
The commander's live state is `COMMANDER.md`; its role brief is `briefs/commander.md`.

## Branches and worktrees

- Integration branch: `p5-learning-loop` (base: `p4-env-impact` @ 03b6ff43, Phase 4 PR #6 not yet merged), worktree
  `.claude/worktrees/p5-integration`. Only the commander (Claude or GLM) merges into it.
- Each lane works on `p5ll/<lane>` in `.claude/worktrees/p5ll-<lane>`, branched from `p5-learning-loop`.
  Commit early and often on your lane branch. Never push, never merge into other branches, never rebase
  other lanes' branches. If you need newer integration work, the commander tells you to
  `git merge p5-learning-loop` into your branch.
- Herdr agent names: `ri-<lane>` (Claude), `glm-<lane>` (GLM continuation of the same lane during a
  Claude usage-limit window). The fleet guard (`~/.claude/fleet-guard/fleet_guard.py`) relies on these.
- Commit messages: imperative summary line, body explaining why; end with
  `Co-Authored-By: <your model name> <noreply@anthropic.com>` (e.g. `Claude Opus 5.5`, `Claude Sonnet 5.5`).
  GLM agents prefix every subject with `[glm-handoff] ` and use `Co-Authored-By: GLM-5.3 <noreply@z.ai>`.

## File ownership

Each lane brief lists what it owns. Edit outside your ownership only when unavoidable, keep such edits
minimal (additive hunks), and list them in your status file. `docs/phase4/env-impact/DESIGN.md` (still the contract) and `docs/phase5/learning-loop/PROTOCOL.md`
are owned by the commander: other
lanes propose changes via `// CONTRACT-CHANGE(<lane>): why` + their status file (small additive fields
are OK, note them).

## Engineering conventions (from docs/ARCHITECTURE.md; non-negotiable)

- **Facts ≠ conclusions ≠ AI.** Every AI output carries full provenance (model, provider, modelVersion,
  promptVersion, promptDigest, inputEvidence, generatedAt). Agent *assertions* are never stored or shown
  as *validated* results (MISSION P4, AC6).
- **Deterministic first, fail closed.** The investigation agent is a sanctioned LLM use (MISSION §A4),
  but the final classification and any automated approval are computed by deterministic policy over
  evidence + validation results. A model can never alone produce `Not Affected` for auto-approval, nor
  `action-required` (docs/ACTION_CLASSIFICATION.md contract is never weakened). Missing context,
  incomplete traversal or an unsupported assumption ⇒ no automatic safe determination (Principle 7).
- Evidence everywhere; content-derived IDs; producer strings `component@vN`; every artifact referenced
  by digest so an investigation is reproducible from its bundle (Principle 6).
- **Declarative, generic constructs only. No product-specific Go logic** (no `if product == "cert-manager"`).
- **Temporal stays at the edge** (§A7): only `internal/orchestrate` (and `cmd/ri` wiring) may import
  `go.temporal.io/*`. Reasoning, graph, validation and policy packages are plain Go, testable without
  Temporal.
- Tests use no network (httptest, fakes, checked-in fixtures/caches, Temporal's in-process
  `testsuite`). Tests that need an external binary (`helm`, `kubectl`, `kind`, `temporal`) skip with a
  clear message when it is missing. `go build ./... && go vet ./... && go test ./...` must pass on your
  branch before you report done. If you change `internal/domain` types, regenerate schemas:
  `go run ./internal/domain/schemagen`.
- Update docs (docs/*.md, README command list) for anything user-visible.
- No new third-party Go dependencies without stating why in your status file (stdlib preferred). The
  Temporal Go SDK is pre-approved for `internal/orchestrate` only.

## Evaluation integrity (the most important rules)

- `eval/gates.yaml` thresholds are pre-registered. Never edit them. Phase 4's `eval/investigations/gates.yaml` is frozen too.
  New Phase 5 gates/metrics are pre-registered by the commander in `docs/phase5/learning-loop/PROTOCOL.md`
  **before** the first scored run, and never edited after (amendments are appended, dated, with why).
- Never edit existing `eval/cases/*/case.yaml` expectations or `eval/results/*` to move a number.
- **Investigation ground truth** (`eval/investigations/`, `groundtruth` lane) is authored blind from
  upstream sources and, where possible, confirmed by an observed outcome (a real install/upgrade in a
  `kind` cluster, a real render), with citations, recorded in each case's NOTES.md. It is never derived
  from Release Intelligence pipeline or agent output. Only the `groundtruth` lane writes expectations.
- Lanes building the pipeline or the agent must not read `eval/investigations/*/expected.yaml` while
  designing heuristics, prompts or policy (fixtures/env inputs are fine). Prompts and policy are
  generic, never tuned to a case.
- Verification provenance is honest: `deterministic` (a validator proved it from artifacts),
  `observed` (a runtime outcome in an isolated cluster), `human`, `proxy` (AI acting as reviewer, always
  labelled). Results are reported per level. A failing gate is a finding to report, not something to hide.

## Running things

- Shell hygiene: in any `rm` write the variable as `"${VAR:?}"/…` (or use a literal path). An `rm` on a possibly-empty
  variable triggers a Claude Code safety prompt that blocks unattended lanes until it auto-denies.
- Build: `go build -o bin/ri ./cmd/ri` (bin/ is gitignored, per worktree).
- Upstream cache: reuse the primary checkout's warm cache with
  `-state /Users/tommydavison/repos/Release-Intelligence-/.ri` (read-mostly; don't delete it). Live
  fetches need `GITHUB_TOKEN=$(gh auth token)`.
- Local tools available: `helm`, `kubectl` (incl. `kubectl kustomize`), `kind`, `docker`, `temporal`
  (`temporal server start-dev` for a local dev server). Clusters you create are named `ri-<lane>-*` and
  deleted when you are done.
- LLM calls: no API keys are set. Use the file exchange (`-llm-exchange <dir>`) and/or stateless
  `claude -p` calls (models: claude-opus-5-5, claude-sonnet-5-5, claude-haiku-4-5), in the style of
  `docs/phase3/model-comparison/raw-arm.sh`.

## Reporting and continuity (handoffs depend on this)

- Keep `docs/phase5/learning-loop/status/<lane>.md` current on your branch **as you go**, not only at the
  end: done, in progress (exact next step), decisions (with why), files touched outside ownership,
  contract changes, open questions, test status. Commit it with your work. At any moment another agent
  (a GLM continuation, or you after a context compaction) must be able to resume from brief + status +
  `git log` alone.
- If you receive `CONTEXT CHECKPOINT` from the fleet guard: update and commit your status file now,
  then continue.
- If blocked on a decision only the commander/user can make, write it in the status file, state it as
  your final message, and stop. Don't guess on contract questions; do pick sensible defaults on local ones.
- **Clean up before reporting done (disk space is shared):** delete every `kind` cluster you created
  (`kind get clusters`), remove `bin/`, scratch renders, downloaded charts/tarballs and any other large
  untracked or gitignored output in your worktree (`git status --ignored` should show nothing big), and
  stop any server you started (`temporal server start-dev`, port-forwards). Never delete the shared
  `.ri` cache in the primary checkout. Say what you cleaned in your final message. You do not remove
  your own worktree — the commander does that after merging (see `briefs/commander.md`).
- `docs/phase5/learning-loop/LEDGER.yaml` lists your lane as an item; the commander updates it — lanes do not
  edit the ledger (one writer per window avoids merge conflicts), but read it to see what is queued after you.
- When finished: commit everything, then end with a final message starting `LANE DONE:` followed by a
  ≤15-line summary (branch, commits, what works, test results, known gaps).
