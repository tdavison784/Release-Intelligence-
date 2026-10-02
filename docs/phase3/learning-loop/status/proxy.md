# Lane `proxy` — status

Brief: [briefs/proxy.md](../briefs/proxy.md). Branch `p3ll/proxy`. Design and usage: [proxy/README.md](../proxy/README.md).

## Done

- Step 1 (runner): `internal/proxyreview` builds the prompt and verdict schema and turns a verdict into a decision;
  it also holds the ledger and the report. `ri knowledge proxy-prompt` writes the requests, `scripts/proxy-review.sh`
  makes one stateless `claude -p --model claude-opus-5-5` call per item, and `ri knowledge decide -reviewer-kind
  proxy -proxy-request R -proxy-response S -proxy-ledger L` records one decision per item.
- Step 2 (provenance): complete `ProxyProvenance` (model, modelVersion from the CLI envelope, promptVersion
  `proxy-review/v1`, promptDigest, callId = CLI session id, inputEvidence, generatedAt); StartedAt/DecidedAt = call
  start/end. Failures and refusals go to the ledger; none are retried blindly.
- Step 3 (self-review bias): the request and each ledger line record the proposal authors' models and families and
  the self-model / self-family flags. `ri knowledge proxy-report` splits outcomes by who authored the proposed
  assertion and per proposing model (self-model / self-family / other-family).
- Pilot: 8 items on a scratch copy of the store, all recorded, about $0.06 and 6 s per call.

## Next

- Step 4: run on all pending non-high items (952: 684 + 268 evidence-sufficiency) → `proxy/run-1/`.
- Step 6: report.
- Step 5 (shadow pass on high items in `proxy-shadow/`): **waiting for the commander**.

## Decisions taken (local)

- **Proposals are anonymised (P1…Pn) in the prompt.** Model names are recorded in the request and ledger only.
  This removes overt self-preference; stylistic self-recognition cannot be removed.
- **The proxy sees no proxy decisions and no proxy-level facts.** One proxy error must not seed the next, and
  each prompt depends only on human/validator knowledge. It sees earlier *human* decisions on other items (as the
  brief asks) and never a decision on the item under review. `-no-human-context` exists for the shadow pass.
- **Evidence-sufficiency items:** these verify no aspect, so they cannot be accepted. The verdict
  `evidence-sufficient` is recorded as `defer` with the reason prefixed "evidence sufficient …"; the item stays
  open for re-proposal or a human. `need-more-evidence` is recorded as `need-more-evidence`.
- **Thinking off** (`MAX_THINKING_TOKENS=0`), as in `scripts/semantic-exchange.sh`; this is cost parity with the
  proposers.
- **Provider** is recorded as `Provenance.Rule = "provider:anthropic"`, because `domain.Provenance` has no provider
  field. Proposal to the contract owner: add `Provenance.Provider` (additive).
- **Wrong-* labels on corrections** are derived from the aspects that actually changed (`wrong-classification`
  when the exposed class changed), plus the model's own labels, kept only if they name a verified aspect.

## Files touched outside ownership (additive)

- `cmd/ri/knowledge.go`: usage lines; the `proxy-prompt` and `proxy-report` subcommands; on `decide`, the flags
  `-proxy-call-id` (the manual proxy path previously had no way to record a call id), `-proxy-request`,
  `-proxy-response` and `-proxy-ledger`.
- `internal/semantic/proxy_export.go` (new file): `semantic.Vocabulary(aspects)`, which exports the proposers'
  vocabulary text.
- `docs/KNOWLEDGE.md`: one CLI sentence linking the proxy README.

## Test status

`go build ./... && go vet ./... && go test ./...`: see the latest commit message.

## Run-1 state (2026-10-02 08:05 CDT): PAUSED at the account session limit

- About 560 of 952 non-high items were decided as proxy. The ledger is `proxy/run-1/ledger.jsonl`; the exchange
  directory `.ri/proxy-run-1` is gitignored and holds a `STOP` file.
- About 8 calls failed with "session limit" (`<item>.failed`, ledger stage `call`). These calls never reached the
  model, so re-running them is not a blind retry. To resume after the 10:50 CDT reset: delete the `*.failed`
  markers whose text says "session limit", `rm .ri/proxy-run-1/STOP`, and re-run `scripts/proxy-review.sh` with
  the same arguments.
- Refusals so far (recorded, not retried):
  - **Prose-only corrections** (≈5): the proxy fixed a consequence statement or remediation (for example an
    invented `--log-format` flag) but kept the kind. The aspect digests exclude prose, so `ReviewDecision.Validate`
    refuses them as no-op corrections. A human on the dashboard would hit the same wall. **Contract question
    for the commander:** should a prose-only correction be allowed?
  - 1 statement longer than 400 characters: the v1 prompt does not state the length limits. Fix in a v2 prompt.
- Still to do: finish step 4, write the step 6 REPORT.md (`ri knowledge proxy-report`), commit
  responses/requests. Step 5 waits for the commander.
