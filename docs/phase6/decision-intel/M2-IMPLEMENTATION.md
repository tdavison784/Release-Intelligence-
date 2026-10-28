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

## Fix: behaviour dependencies, prompt v12 (lane upstream2, ledger D-0013)

**Defect** (found by the commander on development runs, described generically). Under v11, fetching upstream
behaviour is a model choice. With the same prompt, dossier and first tool calls, some sessions of one item
fetched the validation code at the target's tag; others never called `request_evidence` and concluded
`not-affected` from remembered behaviour. The assessment's own `unknown` said the behaviour was not fetched.
Nothing deterministic connected that admitted gap to the verdict.

**Fix: prompt v12** = v11 + feature `behaviour` (`meta.json` `"behaviour": true`; needs `upstream`). v1–v11 are
byte-frozen (v11 added to the digest list) and the default stays v9.

| # | Change | Where |
|---|---|---|
| 1 | **`behaviour` on the assessment** (additive): the upstream behaviours the verdict relies on, each `{behaviour, source}` with `source` `upstream` (`citations`), `change-text` (`quote`), `unavailable` (`request`) or `not-checked`. Stored as `domain.AssessmentBehaviour` (`AgentAssessment.Behaviour`). `Validate` checks the shape; `Investigation.Validate` checks that an `unavailable` entry names an unavailable request of its item, and `ValidateAgainst` that the citations resolve. | `domain/assessment.go`, `domain/investigation.go`, `investigate/api.go` `BehaviourInput` |
| 2 | **Source checks** (every time, any verdict). `upstream`: each citation resolves verbatim, names an `OriginUpstream` artifact (a fetched `upstream-document` or an upstream source of the bundle), and at least one quote is not in the item's change text. `change-text`: the quote is in the item's change text: its title, the change's title and detail, and the excerpts of its upstream evidence (`QuoteOccurs`, per piece). `unavailable`: the id of an upstream request of this item with status `unavailable`. `not-checked`: no other field. A field of another source, an unknown source or a blank behaviour is rejected. | `agent/behaviour.go` `behaviourOf` |
| 3 | **Verdict check.** A `not-affected` verdict with any `unavailable` / `not-checked` entry is rejected every time; the message lists them and names the two ways out (fetch with `request_evidence`, or submit `potentially-affected` / `insufficient-evidence` with the gap). | `behaviourVerdictCheck` |
| 4 | **Empty list, once.** A `not-affected` verdict with no behaviour on a behavioural item is rejected once, with its own once-flag (independent of the consumer completion check, so each can fire once). Behavioural is the bundle's declared classification (`bundle/scope.go`): manifestation `controller-behaviour` or `unknown` (runtime or default semantics of the shipped software; unknown fails closed), or family `behavior` (runtime behaviour stated in prose, even when correlated to a rendered change). Values, schema, image, platform-bound, chart-rendered and procedure items in rendered configuration are not; documentation-only changes are out of scope and get no session. | `behaviouralChange`, `behaviourCompletion` |
| 5 | **Gaps.** Each `not-checked` entry adds `investigation gap (recorded by the tool layer): behaviour "<b>" at <target tag>: not-checked` to `unknown`; each `unavailable` entry `… at <request ref>: unavailable (request <id>: <reason>)`. Upstream citations become run evidence and inputs of the assessment's provenance. | `behaviourGaps`, `agent/assessment.go` |
| 6 | **Prompt v12.** `system.md`: section "Behaviour the verdict relies on" (the four sources, what the tool layer checks, the gap format) and one Closing bullet; every other section is v11's (tested). `item.md`: step 4 names the list and the rule, step 6 the field. `tools.json`: only `submit_findings` (the `behaviour` schema and one description sentence). `task.md` is v11's. Generic wording (tested). | `prompts/investigate-agent/v12/` |

**No product logic.** The checks read only the structured fields, the item's family and manifestation, the
request records and citation resolution. They never inspect the behaviour's wording.

**Earlier prompts.** Without feature `behaviour`, `behaviour` is refused (`this prompt version … takes no
behaviour`). Before, the strict decoder refused the unknown field with another message, so a stored call that sent
it would no longer replay byte for byte. None did: the 412 `submit_findings` calls in the 31 transcripts of the
stored v9–v11 runs (19 `cases.tar.gz`, transcripts only; no label-bearing file opened) carry no `behaviour`.
The canonical arguments of earlier calls are unchanged (`omitempty`), and the assessments of v9–v11 sessions
are built exactly as before.

**Policy isolation.** The classifier never reads assessments. `TestBehaviourNeverReachesThePolicy`: findings,
decision and eligibility are byte-identical without the agent records, with the behaviour list stripped, and with
an adversarial not-affected verdict resting on a fetched citation and a change-text quote.

**How to run.** As v11 ("How to run v11" above), with `-agent-prompt investigate-agent/v12`.

### Tests (`agent/prompt_v12_test.go`)

- `TestPromptV12`: features are v11's plus `behaviour`; only `submit_findings` differs from v11's tools;
  `task.md` and `meta.json` as above; every v11 system section but Closing is unchanged, Closing gains only the
  bullet, plus the new section; the default is v9; `behaviour` without `upstream` is refused.
  `TestPromptV12IsGeneric`: v11's word list plus "strict". `TestEarlierPromptVersionsAreFrozen`: v1–v11.
- `TestV12NotCheckedBehaviourBlocksNotAffected`: not-affected with a `not-checked` behaviour is rejected, twice;
  resubmitted as `potentially-affected` it is accepted, with the gap in `unknown`.
- `TestV12UpstreamBehaviourAccepted`: not-affected citing the fetched artifact is accepted at once; the citation is
  run evidence and a provenance input; the record validates against the bundle. A bundle upstream source is
  accepted when its quote goes beyond the change text, rejected when every quote is the change text.
- `TestV12UpstreamSourceIsChecked`: an environment artifact, a forged quote, no citations, a stray `request`, an
  unknown source and a blank behaviour are rejected.
- `TestV12ChangeTextQuote`: a forged quote and a quote from the notes outside the item's text are rejected, as
  are citations on `change-text`; the change's detail and an evidence excerpt are accepted, and support
  not-affected.
- `TestV12UnavailableNeedsTheRequest`: an unknown id and a fulfilled request's id are rejected; a real
  unavailable request blocks not-affected and is accepted with `potentially-affected`, with the gap;
  `Investigation.Validate` rejects a record whose `unavailable` names a fulfilled request.
- `TestV12EmptyBehaviourList`: a behavioural item's not-affected with no behaviour is rejected once, then
  accepted; other classes are not asked; a non-behavioural item (values, rendered-config) is accepted at once;
  the `behaviouralChange` table.
- `TestV12BehaviourAndConsumerChecksRejectOnceEach`: the consumer check, then the behaviour check, then accepted.
- `TestEarlierPromptsRefuseBehaviour`: v11 refuses the field with the exact message; v11's not-affected is not
  asked for behaviour.
- `TestBehaviourNeverReachesThePolicy`, as above.
- `TestV12Replay`: a scripted v12 run (a fetch, the not-checked rejection, the empty-list rejection, the accepted
  resubmission) replays with no mismatch; transcript, prompt digest and assessments identical.

Mutation checks: disabling the verdict check, the once-check, or the change-text quote check each fails v12
tests.

### Known limits

- **The change text is the bundle's record of the item:** its title, the change's title and detail, and the
  (truncated) excerpts of its upstream evidence. A quote from the same release note outside those excerpts
  counts as upstream text beyond the change text. A quote that spans two pieces does not match.
- **Whether a quote states the behaviour** (rather than only that it changed) is not checked; only that it is
  verbatim and of the right kind of source. A `change-text` or `upstream` entry can still be cited for a
  behaviour the text does not establish; it stays AI prose, as every verified statement does.
- **Completeness of the list is not checked** beyond "non-empty on a behavioural not-affected verdict, once". A
  model can omit a behaviour it relies on; the second submission of an empty list is accepted, as with the
  consumer completion check.
- **Only not-affected is gated.** `affected`, `potentially-affected` and `insufficient-evidence` record their gaps
  but are never rejected for them.
- No live run on a development case (brief). The scored check of v12 is the commander's.


## Fix: fetch first, prompt v13 (lane upstream3, ledger D-0014)

**Gap** (found by the commander on development runs, described generically). Under v12, `not-affected` can no
longer rest on unestablished upstream behaviour. But with fetching configured and working, the agent often
declared the deciding behaviour `not-checked` and took `potentially-affected` or `insufficient-evidence` without
ever calling `request_evidence`. The verdict was safe; the evidence, one call away, was never gathered.

**Fix: prompt v13** = v12 + feature `fetchFirst` (`meta.json` `"fetchFirst": true`; needs `behaviour`). v1–v12
are byte-frozen (v12 added to the digest list) and the default stays v9.

| # | Change | Where |
|---|---|---|
| 1 | **The check** on `submit_findings`, whatever the verdict. It applies while the session can fetch for the item: a fetch port (`WithUpstream`), at least one code repository of the product (`upstreamScope`), and the session budget not spent. The item must be behavioural (`behaviouralChange`, as v12). **No upstream request for the item:** a `not-checked` entry is rejected **once** (own once-flag `rejectedNoFetch`), with "Upstream fetching is available for <repos> at <target> (target) and <current> (current): request the behaviour, or state in the entry why upstream source cannot answer it." The resubmission is accepted. **After a request** (any upstream request of the item, fulfilled or unavailable, refusals included): a `not-checked` entry is rejected **every time**; the entry is `upstream` (cited) or `unavailable` (the request id), under v12's source checks. The message lists the item's requests with their status. | `agent/behaviour.go` `fetchFirst`, `tools_record.go` (before v12's verdict check) |
| 2 | **`reason`** (additive, on `not-checked` only): why the upstream source cannot answer the behaviour. It does not skip the once-rejection; it is accepted on the resubmission and recorded in the gap: `behaviour "<b>" at <tag>: not-checked (reason: <r>)`. A reason on another source is rejected (tool layer and `AgentAssessment.Validate`). | `domain/assessment.go` `AssessmentBehaviour.Reason`, `investigate/api.go` `BehaviourInput.Reason`, `behaviourList`, `behaviourGaps` |
| 3 | **Prompt v13.** `system.md`: one sentence at the end of "Behaviour the verdict relies on" (fetch first, then conclude; the once-rejection; `reason`; after a request, `upstream` or `unavailable`). `tools.json`: only `submit_findings` (the `reason` property and one description sentence). `item.md`, `task.md` are v12's. Generic wording (tested). | `prompts/investigate-agent/v13/` |
| 4 | **Earlier prompts.** Without `fetchFirst` the check never runs. A behaviour `reason` is refused with exactly the message the strict decoder gave before the field existed (`invalid arguments for submit_findings: json: unknown field "reason"`, the key as sent), through the same path, so the transcript records the call as before. Checked against the stored v12 runs (6 transcripts from the commander's `cases.tar.gz`, transcripts only): 75 `submit_findings` calls, 82 behaviour entries, none with a `reason` key. | `agent/session.go` `legacyBehaviourReason` |

**No product logic.** The check reads the sources, the item's family and manifestation, the request records, the
port and the budget. It never reads the behaviour's or the reason's wording.

**Policy isolation.** The classifier never reads assessments. `TestFetchFirstNeverReachesThePolicy`: findings,
decision and eligibility are byte-identical without the agent records, with the behaviour list stripped, and with a
reasoned `not-checked`.

**How to run.** As v11 ("How to run v11" above), with `-agent-prompt investigate-agent/v13`.

### Tests (`agent/prompt_v13_test.go`)

- `TestPromptV13`: features are v12's plus `fetchFirst`; only `submit_findings` differs (the `reason` property;
  the description is v12's plus one sentence); `item.md`, `task.md` and `meta.json` as above; the system prompt is
  v12's plus the one sentence; the default is v9; `fetchFirst` without `behaviour` is refused.
  `TestPromptV13IsGeneric`. `TestEarlierPromptVersionsAreFrozen`: v1–v12.
- `TestV13NotCheckedWithoutAttemptRejectedOnce`: with a port and no request, `not-checked` is rejected once for
  `potentially-affected`, `insufficient-evidence` and `affected`, then accepted with the gap. With `not-affected`,
  the fetch-first message comes first, then v12's verdict check every time.
- `TestV13ReasonAcceptedOnResubmission`: a reason does not skip the once-rejection; a reason on `upstream` is
  rejected; the resubmission is accepted with the reason in the gap; the record validates; a reason on another
  source fails `Validate`.
- `TestV13AfterAttemptUpstreamOrUnavailable`: after a fulfilled request, `not-checked` is rejected every time (a
  reason included), and the cited `upstream` entry is accepted; after an unavailable request, `not-checked` is
  rejected and `unavailable` naming it is accepted.
- `TestV13NotCheckedAcceptedWhenTheCheckDoesNotApply`: no port; a port with no repository for the product; a
  non-behavioural item. Each is accepted at once.
- `TestV13SpentBudgetAcceptsNotChecked`: a driver that keeps calling past the budget; the closing submission is
  accepted (no request can be made).
- `TestEarlierPromptsRefuseBehaviourReason`: v12 refuses `reason` and `Reason` with the decoder's exact message,
  and the transcript keeps the call as sent. v12 accepts `not-checked` with a port and no request. The Go
  interface refuses it with "takes no reason".
- `TestV12ReplayUnchangedByFetchFirst`: v12 accepts `not-checked` with no request and after one, and replays
  with no mismatch; `TestV12Replay` is unchanged and passes.
- `TestV13Replay`: two scripted v13 runs replay with no mismatch, with identical transcripts, prompt digests and
  assessments. The first has the once-rejection, a fetch, the after-request rejection and the cited
  resubmission; the second is accepted with a reason.
- `TestFetchFirstNeverReachesThePolicy`, as above.

Mutation checks: each of these, when disabled, fails at least one test:
- the check;
- the after-request rejection;
- the once-flag;
- the spent-budget, no-repository and behavioural exemptions;
- the feature gate (v12's tests fail);
- the legacy `reason` refusal;
- the reason in the gap.

### Known limits

- **The attempt is per item, not per behaviour.** Without reading wording, a request cannot be matched to the
  behaviour it was meant to answer. Any upstream request of the item counts. After one, each `not-checked`
  behaviour needs its own `upstream` citation or `unavailable` request, so one fetch does not excuse the others.
  But an `unavailable` entry can name any unavailable request of the item, as in v12.
- **After a request, `reason` is not an exit.** A behaviour the product's source cannot answer then needs a
  request that ends unavailable: for example a search with no match, or a request outside the readable scope,
  which is refused and recorded. The gap names that request's reason. This follows the brief ("after an attempt,
  the entry must be upstream or unavailable"). It costs one tool call and can leave refusals in `unknown`.
- **A model can still skip the fetch.** It gives a reason on the resubmission, or submits a second time. The
  once-rejection asks; it does not force. The reason is recorded, never checked.
- **The spent-budget exemption** covers the session's tool-call or read budget (`Exhausted`). An exhausted
  upstream fetch budget still allows a request, which is recorded unavailable (`budget`) and gives the
  `unavailable` exit.
- **Decoder parity for earlier prompts** holds for a string `reason` (any case). A non-string `reason` on a pre-v13
  prompt now fails decoding with a type error rather than "unknown field". No stored transcript sends a `reason` at
  all.
- No live run on a development case (brief). The scored check of v13 is the commander's.
