# Lane consumer2: status (Milestone 1 fix, ledger D-0005)

Branch `p6di/consumer2` (from `p6-decision-intel` @ 09676a58). Brief: `briefs/consumer2.md`. The design and the
known limits are in `M1-IMPLEMENTATION.md`, section "Fix: generated ConfigMap data".

## Cause (verified; reported to the commander)

The commander's description is right in outline. Three parts:

1. **Not in the snapshot.** The snapshot collects only YAML files that hold Kubernetes documents
   (`internal/env/repo.go` `classifyRepoFile`). A `configMapGenerator`'s non-YAML source is not collected, and
   neither is a YAML source without `apiVersion`/`kind`.
2. **Dropped without a trace.** `relations/consumers.go` `generatorData` skipped the absent `files:` source with a
   `continue`, yet still stored the generator's (empty) entries. So the data counted as *known and empty*.
   `readsFile` therefore found nothing to scan: no edge and no gap. The `continue // its data is not in the
   snapshot` in `readsFile` fires only for a ConfigMap the snapshot does not define at all; it dropped silently too.
   `keysOf` also took the empty set for the full key set.
3. **Not among the graph's objects.** The overlay that builds the generator is version-independent: a
   `not-applicable` pair with one render. `relations.load` reads only complete pairs. So the generated object (its
   data, its hash-suffixed name) exists only in a render artifact. The graph's workload is the environment file,
   which names the unhashed generator.

How this was checked: the snapshot code was read. The payments-us *inputs* and the bundle manifest from the B0 run's
`cases.tar.gz` (allowed inputs) show the overlay pair `not-applicable` and the artifact tagged
`ledger-nginx-dm8g5bfhbf`. `kubectl kustomize` of the input overlay shows the same name. The non-development smoke
(shop-nl, below) reproduces parts 1 and 3. No label-bearing file was opened.

## Done

- Consumer rules v2 (`relations.WithConsumerRules`, default v2, producer `…+consumers@v2`):
  - render fallback for generated data, cited by render artifact, line and text, plus `EdgeFile.DataFrom`;
  - unresolved `reads-file` edges for unknown data (`file.path` ""), covering both absent generator sources and
    undefined ConfigMaps;
  - `secretGenerator` keys, and `inferred` for a key of a generated Secret whose keys are unknown;
  - matching that fails closed when renders disagree.
- Trace: unknown data appears in `gaps`, for the configuration and for the reading container's traced mounts.
  The dossier shows the same gaps, and its hop lines mark data that came from the render.
- Replay: v9 transcript headers carry `consumerRules` (from v2 on). A replay uses its transcript's rules, and no
  field means v1. `agent.WithConsumerRules` pins them explicitly.
- Tests: `relations/consumers_generated_test.go` (7 tests) and `agent/consumers_generated_test.go` (3 tests).
  `TestV9Replay` is unchanged and green.
- v1 byte-stability: six fixture graphs (plain, generator, generator + render, both sides) were built with the
  pre-fix code (`git archive 09676a58`) and with v1 rules. They are identical.
- Real replay: the pre-fix transcript `M1s-v9-shop-de-20261020-f6c23796` replayed with this code gives 65/65
  identical calls, `submitted`, and a header with no rules. stdout went to /dev/null; no report, score or gates
  file was opened.
- Smoke (deterministic, no model): `ingress-nginx-1.11-1.12--shop-nl` (non-development), cloned state, `-offline`,
  K0, a scripted v9 session. The `otel-collector` generator source (`config.yaml`, no Kubernetes document) is not
  in the snapshot. The v2 graph took the data from the overlay render `kustomize:platform/otel-collector`. That
  config names no absolute path, so it correctly yields no edge.
- Schemas regenerated. Docs updated: `M1-IMPLEMENTATION.md` (fix, tests, smoke, known limits) and
  `docs/INVESTIGATE.md` (consumer rules).

## In progress (exact next step)

- Nothing. The lane is done. Next step is the commander's: M1 attempt 2 (`scripts/phase6/run-m1-gate.sh`, prompt v9,
  which now builds v2 graphs).

## Decisions (with why)

1. **The fix lives in the session-built consumer graph only.** The snapshot, the stored graph and the policy inputs
   are untouched (brief, and the D-0001 design).
2. **Rules are versioned and the version goes in the transcript header.** Replay compares result digests, and the
   dossier is part of the item prompt digest. A changed consumer graph would therefore break replay of every
   pre-fix v9 transcript whose environment has generated data, including M1 attempt 1. The header field is absent
   for v1, so every existing transcript replays as recorded, and new runs (prompt v9) get v2 without a prompt
   change. The gate runner still uses v9. I did not add a v10 prompt, because the prompt text does not change.
3. **Fallback only when a source is missing.** Complete snapshot data is used as before (brief: "when … not
   available from the environment snapshot").
4. **Kustomize hash alphabet.** The suffix must be ten characters of `[2456789bcdfghkmt]`, so a ConfigMap named
   like `<name>-production` is never taken for the generated one.
5. **Runtime-created placeholder Secrets are unchanged.** Their key edges stay established. The trace already
   reports them as "created at runtime"; marking them inferred would only duplicate that gap.

## Files touched outside ownership

- `internal/domain/investigation.go`: `EdgeFile.DataFrom` (additive) and its doc comment.
- `internal/investigate/agent/{consumers,dossier,drivers,investigator,session}.go`: trace gaps, hop line, transcript
  rules, the investigator option, and a new parameter on `PrepareWith` (in-package callers only).
- `docs/INVESTIGATE.md`.

## Contract changes (additive; `CONTRACT-CHANGE(consumer2)`)

- `GraphEdge.File.dataFrom`.
- `reads-file` edges with an empty `file.path` (unknown data).
- Transcript header field `consumerRules`.
- Rule ids `graph.reads-file@v2:{rendered-data,unknown-data,unknown-keys}:<via>`.
- Producer `relations.builder@v1+consumers@v2`.

## Open questions

- None blocking. One note for the commander: M1 attempt 2 runs under v2 and attempt 1 ran under v1. A comparison
  should name the rules each attempt used; the transcript headers record them.

## Test status

`go build ./... && go vet ./... && go test ./...` green (see the final commit).
