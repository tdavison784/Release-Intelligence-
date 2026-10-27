# Lane upstream: status (Milestone 2 A1/A3/B6, ledger D-0009)

Branch `p6di/upstream` (from `p6-decision-intel` @ f0521952). Brief: `briefs/upstream.md`; plan: `PLAN-M2.md` lane
`upstream`, §4. Design, tests, smoke and known limits: `M2-IMPLEMENTATION.md`.

## Done

- **Changes 1–6 of PLAN-M2 lane upstream, each with its §4 tests:**
  - **Fulfilment.** `request_evidence` kinds `upstream-file` / `upstream-search`; fetch port `agent.Upstream`
    (`WithUpstream`), wired in `cmd/ri` from the registered `repo-file` / `repo-dir` adapters,
    `linkedev.CodeReposOf` and `TagFor`. Allowed: the product's code repositories, at the target's tag (the
    default) or the current version's tag, both immutable. Anything else is refused with the reason.
  - **Recording.** `upstream-document` run artifacts (`OriginUpstream`, blob URL at the tag, `evidence-fetch@v1`),
    `Fulfilled`, the resolved query in `EvidenceRequest.Upstream`. They are citable at once and the bundle is
    never changed.
  - **Unavailable.** Refused, budget, not found, offline cache miss, binary, no match: `RequestUnavailable` with
    the reason. The assessment states each in `unknown` ("behaviour at <tag> unavailable: <reason>") and in
    `unavailableEvidence`.
  - **Bounds.** In v11 `meta.json` `upstream`: 6 fetches and 384 KiB per item (≤ `MaxReadBytes`); 1500 files,
    30 matches, 2 context lines and 6 stored files per search; 40 head lines.
  - **Prompt v11** = v9 + the tool + one generic rule + the item line naming the readable repositories and tags.
    v1–v10 are frozen and the default stays v9.
  - **Reproducibility.** Through the fetch and gitsrc caches. The offline replay test uses real caches, and
    deterministic not-found and offline reasons (the test caught the transport's 404 text differing between
    online and cached).
- **Run recording.** The pipeline merges the artifacts (already did) and names each upstream outcome in the
  `requested` audit event. `ValidateAgainst` checks that `Fulfilled` resolves.
- **gitsrc (additive).** An absent directory at an immutable tag is a cached not-found; `Repo.TreeURL`.
- **Schemas** regenerated (`EvidenceRequest.upstream`, `AgentAssessment.unavailableEvidence`, request kinds).
- **Smoke** (deterministic, scripted, unscored; non-development `ingress-nginx-1.11-1.12--shop-nl`):
  - online, a search and a file at `controller-v1.12.0` were fulfilled, both reaching the same artifact
    (identical bytes through both paths), and `main` was refused;
  - the offline replay from the recorded cache gives the same transcript artifact id (every result digest
    identical) and identical requests and artifacts.
- **Docs:** `M2-IMPLEMENTATION.md` (new), `docs/INVESTIGATE.md` (tool table, v11 entry, prompt list), and the
  `-agent-prompt` help text in `cmd/ri/eval_investigations.go`.
- **Cleanup:** `bin/` and the scratch state clones removed; no kind cluster or server was started.

## In progress (exact next step)

- Nothing: lane done. Next is the commander's: record the evaluation state with upstream fetches (online v11
  session) and run the scored D-EU check (`-agent-prompt investigate-agent/v11`). See "How to run v11" in
  `M2-IMPLEMENTATION.md`.

## Decisions (with why)

1. **Refusals are recorded unavailable, not rejected.** The brief lists "refused" under Unavailable, and a recorded
   refusal becomes a gap the engineer sees ("never silent"). Malformed requests, which the model can fix, are
   rejected. To keep refusals from cluttering `unknown`, a refusal is dropped when a later request of the item
   reads the same file or runs the same search.
2. **Only the upgrade's two tags are readable** (not every immutable ref). The brief's "allowed only: immutable
   refs, preferring the target's tag, with the current version's tag also allowed". I read that as the target's
   tag by default plus the current tag, with immutability checked too. This fails closed and keeps every read
   tied to the transition being assessed.
3. **Earlier prompts decode the old argument struct.** New fields on `EvidenceRequestInput` would otherwise change
   v1–v10's strict-decoding error for an unknown field. With the legacy struct, their messages are byte-identical
   (tested), so no stored transcript's replay can change.
4. **A file result shows a head, not the file.** The fetched file is a run artifact; the model reads it with the
   existing metered `read_artifact` / `search_manifest`. Stored bytes have their own per-item cap, within
   `MaxReadBytes`.
5. **Searches store each matching file** (up to 6) as its own blob-URL artifact, so every match line is citable
   verbatim. A synthetic "search result" document would not have a blob URL at the tag.
6. **Deterministic failure reasons.** Not-found and offline reasons name only the request. Transport error text
   differs between a live 404 and its cached replay; the replay test proved it.
7. **Negative cache in gitsrc for absent directories at immutable tags only.** Otherwise a search of a
   non-existent directory could never replay. A missing ref (a tag not yet pushed) or a branch is never cached.
8. **Upstream needs feature assessment** (checkSurface), because the gaps land in the assessment.

## Files touched outside ownership

- `internal/gitsrc/dir.go` (additive: `dirPayload.Missing`, negative cache), `internal/gitsrc/repo.go`
  (`TreeURL`), `internal/gitsrc/dir_test.go` (new test). Reuses `Files` / `Dirs`; nothing network-facing added.
- `internal/investigate/api.go` (additive `EvidenceRequestInput` fields).
- `cmd/ri/investigate.go` (`upstreamPort`, one option), `cmd/ri/eval_investigations.go` (help text).
- `internal/investigate/agent/prompt_v8_test.go` (v10 added to the frozen digests).
- `internal/investigate/run/upstream_test.go` (new).

## Contract changes (additive; `CONTRACT-CHANGE(upstream)`)

- Request kinds `upstream-file`, `upstream-search`; `EvidenceRequest.upstream` (`UpstreamQuery`).
- `AgentAssessment.unavailableEvidence`.
- `EvidenceRequestInput.repo/ref/path/dir/pattern/glob`.
- Prompt meta `upstream` (`UpstreamBounds`).
- Producer `evidence-fetch@v1`.
- Artifact path `upstream/<product>/<tag>/source/<owner>/<name>/<file>`.
- `ValidateAgainst` checks `EvidenceRequest.Fulfilled`.

## Open questions

- None blocking. For the commander:
  1. The recorded evaluation state must include the v11 session's upstream fetches (`<state>/cache`), or its
     replay mismatches (correctly).
  2. The development cases' repositories come from the catalog's version sources; for the D-EU check, the
     readable scope is the item line in each item's task.

## Test status

`go build ./... && go vet ./... && go test ./...` green: 46 packages (final commit).
