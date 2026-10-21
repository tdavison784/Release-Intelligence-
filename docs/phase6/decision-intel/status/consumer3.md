# Lane consumer3: status (Milestone 1 fix, ledger D-0006)

Branch `p6di/consumer3` (from `p6-decision-intel` @ 9296a213). Brief: `briefs/consumer3.md`. The design, tests,
smoke and known limits are in `M1-IMPLEMENTATION.md`, section "Fix: file paths in container command, args and env".

## Done

- **Golden graphs first** (commit edd511a9): v1 and v2 graphs of three environments (plain, generated data, pod-spec
  paths), both sides. They were written by the code before any v3 change. `TestConsumerRulesGolden` checks every
  later build against them byte for byte.
- **Consumer rules v3** (default; producer `relations.builder@v1+consumers@v3`):
  - absolute paths in `command`, `args` (`sh -c` scripts included) and literal `env[].value` strings;
  - matched against the same container's mounts with the existing resolution;
  - `reads-file` edges provider → workload, carrying `file.via: pod-spec`, `file.field` and the cited line;
  - unresolved edges for paths under no mount;
  - `$(VAR)` masked, `command[0]` under no mount skipped, `valueFrom` untouched.
- **Trace and dossier.**
  - Pod-spec hops and gaps are reported.
  - The mount-unread gap names the pod spec too, on v3 graphs only (`cgraph.podSpec`, via `relations.RulesOf`), so
    v1/v2 trace text is unchanged and old transcripts replay.
- **Replay.**
  - The header records `consumerRules: 3`.
  - The existing v2 check now pins v2 explicitly (`TestReplayPinsConsumerRules`), because the default moved to v3.
  - `TestReplayPinsConsumerRulesV3` is new.
- **Tests:** `relations/consumers_rules_test.go` and `agent/consumers_podspec_test.go`. They cover every case the
  brief lists.
- **Smoke** (deterministic, no model, non-development `ingress-nginx-1.11-1.12--shop-nl`):
  - v2 has 0 `reads-file` edges.
  - v3 resolves the controller's webhook cert/key args to Secret `ingress-nginx-admission` keys `cert`/`key`, and
    otel-collector's `--config` to its ConfigMap.
  - One image path (`LD_PRELOAD`) is unresolved.
  - stdout was discarded, and no report, score or gates file was opened.
- **Schemas** regenerated (`EdgeFile.field`).
- **Docs:** `M1-IMPLEMENTATION.md` (fix section, known limits) and `docs/INVESTIGATE.md` (consumer rules).

## In progress (exact next step)

- Nothing. The lane is done. Next step is the commander's: M1 attempt 3 (prompt v9, which now builds v3 graphs).

## Decisions (with why)

1. **The edge points to the workload.** The brief asks for provider → workload. `file.configKey` is empty and the
   new `file.field` names the pod-spec field. An empty `file.path` still means unknown data, so the two kinds of
   edge cannot be confused.
2. **`$(VAR)` is masked, not expanded.** Expansion would yield paths that appear on no line, so they could not be
   cited. Masking keeps the matcher from taking `/certs/ca` out of `$(DIR)/certs/ca` (a false unresolved edge).
3. **`command[0]` under no mount is skipped.** Otherwise every `sh -c` container gets a `/bin/sh` gap. Under a mount
   it is kept (a mounted script). Other image paths stay unresolved edges: honest, if noisy. This is documented as a
   limit.
4. **v1/v2 wording is frozen.** Trace results are digest-compared on replay, so new gap wording applies to v3 graphs
   only.
5. **Golden files rather than a v1-vs-v2 comparison.** The brief asked to extend the comparison test. Goldens
   written before the change pin both versions against the pre-v3 code itself, not just against each other.

## Files touched outside ownership

- `internal/domain/investigation.go`: `EdgeFile.Field` (additive) and doc comments.
- `schemas/investigation-graph.schema.json`, `schemas/investigation.schema.json` (regenerated).
- `internal/investigate/agent/{consumers,dossier}.go`, and in agent tests `consumer_fixture_test.go` (a new fixture
  mode) and `consumers_generated_test.go` (v2 pinned explicitly, `replayRun` helper).
- `docs/INVESTIGATE.md`.

## Contract changes (additive; `CONTRACT-CHANGE(consumer3)`)

- `GraphEdge.File.field`; `file.via` value `pod-spec`; `reads-file` edges whose `To` is a workload.
- Rule ids `graph.reads-file@v3:pod-spec:{command,args,env}` and `graph.reads-file@v3:unknown-keys:pod-spec`.
- Producer `relations.builder@v1+consumers@v3`; transcript header `consumerRules: 3`.
- `relations.RulesOf(builder)` (exported helper).

## Open questions

- None blocking. A note for the commander: M1 attempt 3 runs under v3, attempt 2 under v2. The transcript headers
  record the rules.

## Test status

`go build ./... && go vet ./... && go test ./...` green (final commit).
