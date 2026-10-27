# Milestone 2 implementation: upstream behavioural evidence (lane upstream)

`PLAN-M2.md` lane `upstream` changes 1–6 with the §4 tests, on branch `p6di/upstream` (ledger D-0009). Changed
from the plan by the brief: prompt v11 builds on **v9**, the default, with `scopedVerdict` off. v1–v10 are
byte-frozen (digest-tested; v10 added to the list) and the default stays v9. The security advisory work of
the same milestone (lanes advisories, advisories2, advisories3) is documented in `docs/SECURITY_ADVISORIES.md`.

## What changed, and where

| # | Change | Where |
|---|---|---|
| 1 | **Fulfilment.** `request_evidence` kinds `upstream-file` (`{repo, ref, path}`, or a blob URL at the tag in `target`) and `upstream-search` (`{repo, ref, dir, pattern, glob}`). Only in the product's own code repositories (`linkedev.CodeReposOf`) and only at an immutable ref (`gitsrc.IsImmutableRef`) that is the target's tag (`TagFor`, the default when `ref` is omitted) or the current version's tag (for comparison). The fetch port is `agent.Upstream` (`agent.WithUpstream`): the registered `repo-file` / `repo-dir` adapters (gitsrc `Files` / `Dirs`) plus the product's repositories and tag function, wired in `cmd/ri` for `ri investigate` and `ri eval -investigations`. The search is an in-memory RE2 match, line by line, over the directory listing. | `agent/upstream.go`, `tools_record.go` `requestEvidence`, `session.go` (dispatch, per-item state), `investigator.go`, `cmd/ri/investigate.go` `upstreamPort` |
| 2 | **Recording.** Each file read is stored as an `upstream-document` run artifact: `OriginUpstream`, path `upstream/<product>/<tag>/source/<owner>/<name>/<file>`, URI = the blob URL at the tag, producer `evidence-fetch@v1`, digest of the bytes. The request is `fulfilled` with the artifact ids in `Fulfilled` and the resolved query in the additive `EvidenceRequest.Upstream`. The artifact is retrieved for the item (it counts for the per-item concern rule), citable verbatim at once, and readable with `read_artifact` / `search_manifest`. The bundle is never changed; the runner merges the artifacts into the run record (`Investigation.Artifacts`), `ValidateAgainst` checks that `Fulfilled` resolves, and the `requested` audit event names how each upstream request ended. | `agent/upstream.go` `storeUpstream`, `domain/investigation.go`, `run/pipeline.go` `upstreamOutcome` |
| 3 | **Unavailable.** A request outside the scope (another repository, a mutable ref, a tag that is neither of the upgrade's) is **refused without a fetch**. Refusals, a spent budget, not found, an offline cache miss, a binary file and a search without a match are all recorded as `RequestUnavailable` with the reason. The item's assessment then states each in `unknown` as `investigation gap (recorded by the tool layer): behaviour at <tag> unavailable: <reason> (<kind> <target>, request <id>)` and lists the request in the additive `AgentAssessment.UnavailableEvidence`. One exception: a refusal whose question (same kind, repository, file or directory + pattern + glob) a later request of the item fulfilled is not a gap. A malformed request is rejected, so nothing is recorded and the model fixes it (missing fields, `..` segments, the repository root, an invalid regex or glob, upstream fields on another kind). | `agent/upstream.go` `refusal`, `upstreamGaps`; `agent/assessment.go` |
| 4 | **Bounds** (v11 `meta.json` `upstream`). Per item session: `fetches` 6 (a request that reaches the cache or network counts; refusals and re-reads of the same file or search do not) and `bytes` 384 KiB of stored files (capped by the item's `MaxReadBytes`, 1 MiB by default). Per search: `searchFiles` 1500 files scanned (and at most `MaxGrepScan` bytes), `searchMatches` 30 lines, `searchContext` 2 lines each side, `searchStore` 6 matching files stored. A file result shows `headLines` 40 lines. The result the model reads is also metered against the session's read bytes, as every tool result is. | `prompts.go` `UpstreamBounds`, `checkSurface` |
| 5 | **Prompt v11** = v9 + the tool's new behaviour + one generic rule. `system.md`: section "Upstream behaviour at the target's tag" (when the outcome depends on upstream behaviour — what a validation accepts, which default applies, how a value is parsed, what a feature gate enables — and the change text does not state it exactly, fetch the implementation, documentation or tests at the target's tag before concluding; cite them, or the behaviour stays unknown; remembered behaviour is never evidence). The `request_evidence` bullets point at it. `item.md`: one step, and a line naming the repositories and tags that may be read (`renderItemTaskWith`). `tools.json`: only `request_evidence` (kinds, fields, description). `task.md` is v9's; `meta.json` is v9's plus `upstream`. Generic wording (tested). | `prompts/investigate-agent/v11/` |
| 6 | **Reproducibility.** Fetches go through the existing caches: `repo-file` through the `fetch.Client` cache, which also stores 404s; `repo-dir` through `<state>/cache/git/dir`. An **additive gitsrc hunk** makes an absent directory at an immutable tag a cached not-found, so it replays offline as not-found and not as a cache miss. A missing ref, or an absent directory on a branch, is never cached. The not-found and offline reasons name only the request, never the transport's error text, so the same answer read online and replayed from the cache reads alike (the offline replay test found that they differed). | `gitsrc/dir.go`, `gitsrc/repo.go` `TreeURL`, `agent/upstream.go` `fetchReason` |

**Earlier prompts are unchanged.** Without feature `upstream`, `request_evidence` decodes into the arguments it
always had (`legacyEvidenceRequest`). So `repo` & co. are refused with the same `json: unknown field` message as
before, and the upstream kinds with the same `kind …: want artifact, source, environment-input or
live-observation` message. v1–v10 transcripts therefore replay byte for byte, and their item tasks are unchanged
(the new template field is used only by v11's `item.md`).

**Policy isolation.** The classifier never reads evidence requests or assessments. Fetched artifacts and evidence
reach the record, not the policy input. `TestUpstreamNeverReachesThePolicy` checks that findings, decision and
eligibility are byte-identical with and without the upstream records, an adversarial not-affected verdict
included.

## How to run v11

```sh
go build -o bin/ri ./cmd/ri
cp -Rc .ri "$SCRATCH/state"                      # never write the shared cache
scripts/phase5/snapshot.sh 03b6ff43 "$SCRATCH/K0"
# record (online: upstream fetches populate $SCRATCH/state/cache)
GITHUB_TOKEN=$(gh auth token) DO_NOT_TRACK=1 bin/ri -state "$SCRATCH/state" eval -investigations -case <case> \
  -knowledge "$SCRATCH/K0" -work "$SCRATCH/work" -agent -agent-model claude-sonnet-5-5 \
  -agent-prompt investigate-agent/v11 -run-id <run-id> -save
# replay offline from the recorded state
bin/ri -state "$SCRATCH/state" -offline eval -investigations -case <case> -knowledge "$SCRATCH/K0" \
  -agent -agent-driver replay -agent-replay-dir eval/investigations/results/<run-id> -run-id <new-id>
```

The evaluation state must contain the upstream fetches the recorded session made. A replay from a state without
them makes the same requests unavailable, so its result digests differ from the recording: a correct replay
mismatch, not a silent "no evidence".

## Tests

- **Prompt** (`agent/prompt_v11_test.go`):
  - `TestPromptV11`: features are v9's plus the bounds; only `request_evidence` differs from v9's tools;
    `task.md` and `meta.json` as above; every v9 system section but "Verified, inferred, unknown" is unchanged,
    plus the new one; the default is v9; feature `upstream` needs `assessment` and complete bounds.
  - `TestPromptV11IsGeneric`: v9's word list plus case-specific terms.
  - `TestEarlierPromptVersionsAreFrozen`: v1–v10.
- **File and search at a tag:**
  - `TestUpstreamFileAtTargetTag`: the artifact's origin, kind, URI, producer and id; the blob URL form; the
    re-read is not fetched; a verified statement cites the fetched line, which becomes document evidence at the
    blob URL; the record validates against the unchanged bundle; a `Fulfilled` id naming no run artifact fails.
  - `TestUpstreamSearch`: matches with context, a tree-URL target, stored files readable with `read_artifact`;
    the caps (30 of 40 matches, 6 of 8 files stored, the rest listed as not stored).
- **Refusals** (`TestUpstreamRefusals`):
  - another repository, a mutable ref, another tag and a commit outside the upgrade are refused without a fetch;
  - the current tag is allowed;
  - an answered refusal is not a gap, an unanswered one is;
  - no port means unavailable, never silent.
- **Malformed requests:** `TestUpstreamMalformedRequestsAreRejected`.
- **Budget and unavailable gaps** (`TestUpstreamBudgetAndUnavailableGaps`):
  - the bytes cap and the fetch cap;
  - not found, as a file and as a directory;
  - no match;
  - each one is in `unknown` and `unavailableEvidence`, after the model's own entries.
- **Earlier prompts:** `TestEarlierPromptsDecodeRequestEvidenceAsBefore`, with exact v9 messages.
- **Item task:** `TestUpstreamLineInItemTask` (v9's task is unchanged).
- **Policy:** `TestUpstreamNeverReachesThePolicy`; `run/upstream_test.go` `TestUpstreamEvidenceIsRecordedInTheRun`
  (run artifacts, not bundle; `Fulfilled` resolves; audit outcome; nothing in the classifier input).
- **Replay** (`agent/upstream_replay_test.go` `TestUpstreamReplaysOfflineFromTheRecordedState`):
  - real gitsrc and fetch caches (the git runner and the HTTP transport are fakes);
  - a v11 session recorded online (a file, a 404, a search, an absent directory) replays offline with no git
    and no network: no mismatch, and identical requests, assessments and artifact ids;
  - offline without the recorded state, the requests are unavailable ("offline: not in the recorded cache") and
    the recording does not replay.
- **gitsrc:** `TestFetchDirAbsentDirectoryReplaysOffline`.

## Smoke (deterministic, no model, unscored)

Case `ingress-nginx-1.11-1.12--shop-nl` (non-development; only `input.yaml` and `env/` were read). Cloned state, K0,
`ri investigate … --agent --agent-driver scripted --agent-prompt investigate-agent/v11`, online for the
upstream fetches only (`GITHUB_TOKEN` in that command's environment). The script made three requests and never
submitted, so every session ended "without submit_findings", which is expected.

- `upstream-search` in `kubernetes/ingress-nginx` `pkg/flags`, pattern `enable-metrics`, glob `*.go`: fulfilled
  at `controller-v1.12.0` (the catalog's `tagPrefix` + the target version), through gitsrc's blobless work
  repository.
- `upstream-file` `pkg/flags/flags.go`: fulfilled through the raw file endpoint. It is the same artifact
  (`art-10324b189ba9`, 21056 bytes) the search stored, so both paths read identical bytes.
- `upstream-file` at `main`: refused (not an immutable ref), recorded unavailable; no fetch.
- The run record has the artifact (`OriginUpstream`, blob URL at the tag, `evidence-fetch@v1`), and the
  audit names each outcome.
- **Offline replay** (`-offline`, a fresh state clone carrying the recorded `cache/` only, replay driver on the
  recorded transcript): the same transcript artifact id, so every result digest is identical; requests are
  identical apart from wall-clock `generatedAt`, and so are the fetched artifacts.

No development case was run, recorded or replayed (brief: the commander records the evaluation state and runs
the scored D-EU check).

## Known limits

- **Scope of what is readable.** Only the version-source repositories (`github-releases` / `git-tags` locators on
  github.com) count as the product's code repositories. A product whose code lives elsewhere, or whose chart repo
  tags differ from its canonical tags, gets refusals or not-found results, recorded as gaps. Only the
  upgrade's two tags are readable: no commit, no other release.
- **Search cost online.** `repo-dir` fetches every blob below the directory before the in-memory match, so the
  file cap bounds what is scanned and returned, not what is downloaded. A narrow `dir` and `glob` are what keep
  a search cheap (the prompt says so). The repository root is refused.
- **Gaps are per request.** A refusal is dropped only when a later request reads the same file or runs the same
  search, regardless of tag. Not-found, offline and budget gaps always stay. Nothing checks that a gap's
  question was answered by a *different* file.
- **Network failures are not cached.** A session that met a transient network error replays it as an offline
  cache miss, and so mismatches. That is honest, but such a recording is not reproducible.
- **The fetched text is upstream data, not a validation.** A verified statement that cites it says what the
  quote says (checked verbatim), not that the model read the code correctly. A conclusion built on it stays
  AI prose; the classifier never reads it.
- `ri investigate mcp` serves no fetch port, so upstream requests there are refused as from a read-only session.
