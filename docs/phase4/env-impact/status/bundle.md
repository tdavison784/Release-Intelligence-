# Status — lane `bundle` (Claude Sonnet 5.5, agent ri-bundle)

Branch `p4ei/bundle`, worktree `.claude/worktrees/p4ei-bundle`. Brief: `briefs/bundle.md`.
State: **DONE** (2026-10-16). Resume from: this file + brief + DESIGN §2/§7 + `git log`.

## Done
- `internal/investigate/store` — `store.New(stateDir)` implementing `ArtifactStore` + `RecordStore`
  (§2.3 layout, atomic temp+rename writes, digest-verified reads, `ErrNotFound`, bundle immutability
  incl. idempotent re-write ignoring `CreatedAt`, bundle refused if a manifest artifact is not in the CAS,
  run-record audit may only grow / history not altered). Tests: round trip, bad specs, tamper detection +
  repair, immutability, audit rules.
- `internal/investigate/bundle`:
  - `scope.go` — declared §1.3 scope rules and §2.2.1 manifestation rules (+ family), pure functions.
  - `change.go` — `NewChangeCollector(app, store)` (+ `NewChangeCollectorOver(Upstream, store)` for tests):
    edge → `upgrade-edge` artifact (wall-clock fields zeroed), `ChangeItem` per non-routine change and per
    moved compatibility constraint (Platform anchor), release-scope chart-default pair (renders + diff as
    artifacts, `RenderedChange` per `render.Change`, correlation → items, unexplained structural changes
    become `scope:rendered-structural` items), all nine `ChangeSourceKind`s stated with status+reason,
    upstream documents fetched through the fetch cache and digest-compared to the ingested evidence.
  - `snapshot.go` — `NewSnapshotBuilder(store)` (+ `…WithHints`) over `env.Load`: files as artifacts
    (redacted before hashing), `environment-facts` JSON (credentials withheld), `SnapshotInput` per input
    with status, `RepoRef` (git commit, dirty, always a tree digest), live request ⇒ `unavailable`.
  - `redact.go`, `repo.go`, `assemble.go` — `NewAssembler(store)`: unions artifacts (unique path check,
    each must be openable) and evidence, `BundleID`, `Validate`.
  - Tests: scope/manifestation/family tables, redaction, snapshot (secrets never in any artifact/facts/
    evidence; git commit/dirty; non-git pin; statuses), change (fake `Upstream`), assemble (determinism,
    refusals), groundtruth inputs.
- `docs/INVESTIGATE.md` "Evidence bundle" section.
- Groundtruth: a validating bundle for all 5 `eval/investigations/*/input.yaml` (only `input.yaml` + `env/`
  read) from the warm cache offline (helm present ⇒ release renders included), stored via
  `store.PutBundle`, two independent runs ⇒ same bundle id.

## In progress / next step
- Nothing. Commander: merge `p4ei/bundle`. Wiring (`internal/investigate/run`) calls
  `store.New(stateDir)`, `bundle.NewChangeCollector(app, st)`, `bundle.NewSnapshotBuilderWithHints(st,
  env.HintsFromCatalog(app.Catalog))`, `bundle.NewAssembler(st)`.

## Decisions (with why)
- `store.New(root)`: `root` = the *state dir*; files live in `<root>/investigations/`.
- Routine changes get **no** item (§2.2 says "non-routine"); a rendered change correlated only to a routine
  change is therefore uncorrelated at release scope. `scope:routine-excluded` still exists for completeness.
  renderdiff must map `render.ChangeLink.ChangeID` to `ChangeItemID(id,"","")` only for ids that have items.
- Computed `compat:*` changes are not itemised: the **Platform item** of the moved `CompatibilityChange`
  represents them (flags: `ActionRequired` if a compat change on that platform needs action). Platform
  anchor is the platform name, or `<platform>/<kind>` when several constraints of one platform moved
  (e.g. `kubernetes/supported`, `kubernetes/chart-kubeVersion`). Unchanged constraints get no item.
- Scope list (`computedBreakingRules`): §1.3's "`crd:version-*`" taken as removed/unserved/deprecated only
  (not `version-added`); `crd:storage-changed`, `compat:changed-raw` (cannot be compared ⇒ assume narrow)
  and `artifacts:removed` added. Manifestation `manifest:computed-crd-structure` also covers
  `crd:storage-changed` and `crd:version-*`; any other computed rule falls to `manifest:unknown` (fail closed).
- `render.ChangeLink`s of rule `artifact` carry no change id; the collector attributes them to computed
  `images:*`/`artifacts:*` items whose title/subjects mention the repository. **renderdiff has the same
  problem for environment renders** — it may reuse this idea (or a shared helper) so image bumps are not
  "uncorrelated" forever.
- Documents: GitHub blob URL → raw.githubusercontent.com URL; the fetched digest must match one of the
  ingested evidence digests, else the source is `partial` ("changed since it was ingested"). Advisory pages
  that are not in the cache are `unavailable` with the fetch error (honest gap, offline).
- Stored edge copy zeroes `generatedAt` and evidence `retrievedAt` (reproducible bundle id).
- Redaction: spec regex on last key segment; booleans/nulls stay readable; unparsable or non-YAML files are
  stored verbatim (flagged by the env loader). Evidence ids are kept when excerpts are redacted (ids stay
  valid), so a redacted file's evidence `contentDigest` is the digest of the *original* bytes.
- Environment files are mapped to `env/<repo-relative path>`; the facts JSON keeps paths as supplied
  (so a bundle id depends on how the repo path was typed).

## Files touched outside ownership
- `docs/INVESTIGATE.md` — "Evidence bundle" section only (allowed by the brief).

## Contract changes
- none

## Open questions
- (commander/renderdiff) Should environment-scope correlation to *routine* changes be dropped like release
  scope? As built, items exist only for non-routine changes.

## Test status
- `go build ./... && go vet ./... && go test ./...` green on `p4ei/bundle`
  (`gofmt -l` flags only `internal/domain/investigation.go`, contract lane's file).
- `TestGroundtruthInputsBundle` skips unless a warm cache exists (`RI_STATE` or the primary checkout's
  `.ri`); it uses the network never (offline app).
- Cleanup: no kind clusters/servers created; no `bin/`; scratch binary lived in the session scratchpad.
