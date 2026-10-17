# Lane `prompt8` — status

Branch `p5ll/prompt8`. Brief: `briefs/prompt8.md`. Ledger W-0029; mandate: PROTOCOL amendment 2026-10-17 10:55
(one revision, no iteration on the dev pair).

## Done
- **Prompt `investigate-agent/v8`** (now `DefaultPromptVersion`). A copy of v7 with `system.md` and `item.md`
  revised; `tools.json`, `meta.json`, `task.md` are v7's byte for byte (tested). No tool, validator, policy,
  budget or verdict-schema change; feature flags unchanged. What changed (from the transcript-level failure of
  `B0-K0-20261017-6d2ae5bf`: zero `record_hypothesis` calls while verdicts named overturning conditions in prose):
  - Mitigation trigger: a mitigation is a release-level rule; record it whenever upstream text supports a condition
    under which an exposed object is not impacted, whether the environment meets it on all, some or none of the
    exposed objects (the validator decides per object). `environmentBasis` lists the claims/objects that meet it.
  - Change vs consequence: objects whose setting changes vs objects where a consumer observes the change; scope
    applicability/consequence to the latter as a decidable predicate (`field` / `ref` inside `resource`), name when
    it does not hold. Abstract symbolic example (`K`, `spec.a`, `spec.b`), no product.
  - Consistency: every upstream-supported, environment-checkable condition in `wouldOverturn` /
    `proposedValidations` is a recorded mitigation named in `mitigations`, or carries a one-line reason it is not
    expressible in the fact grammar.
  - Consumers: trace what references the changed/produced object and what it reads (expand_graph out, list_paths,
    search_manifest grep, get_rendered_object, read_artifact), record claims/assertions before settling on
    `potentially-affected`; a runtime-produced object absent from the snapshot is stated absent and reasoned about
    from its referrers.
  - Question list: 7 extended (consumers / observable consequence), new 8 (upstream not-impacted conditions →
    mitigations), old 8 → 9.
- **Tests** (`internal/investigate/agent/prompt_v8_test.go`, synthetic only):
  `TestEarlierPromptVersionsAreFrozen` (v1–v7 load; a sha256 over every file of each version equals a golden
  taken from the committed files), `TestPromptV8` (features = v7's, tools/meta/task byte-identical, system/item
  differ, the new instructions present, distinct digest, default), `TestPromptV8IsGeneric` (word list:
  cert-manager, ingress-nginx, kube-prometheus-stack, payments, shop, obs, kinds/fields/enum values
  case-sensitively, plus every token of the `eval/investigations/` directory names case-insensitively, as whole
  words; mutation-checked by appending a banned word), `TestV8MitigationWithPartialBasisIsStored` (widget fixture:
  basis = only the explicit widgets while an unset one exists → stored; basis = an object that does not meet it →
  stored; empty basis → still rejected, hypothesis kept as free text).
  `verdict_v7_test.go`: dropped the "v7 is the default" assertion (now v8's test); nothing else.
- **Docs**: v8 entry in `docs/INVESTIGATE.md` (agent section, after v7; prompts paragraph).

## In progress
- Smoke (one live session-set, `ingress-nginx-1.11-1.12--shop-fr`, not cert-manager): see "Smoke" below.

## Smoke
(pending)

## Decisions
- tools.json kept identical to v7 (the record_hypothesis description already says "showing this environment
  meets the rule's conditions"); all new guidance lives in system.md/item.md so the change is prompt-only and the
  test can pin tool-schema identity.
- "None meets it" case: the tool layer requires a non-empty `environmentBasis` (`MitigationSpec.Validate`). v8
  tells the model to name the verified claims that establish the condition's state on the exposed objects and to
  say in the statement it holds on none. Not a tool change. See contract question 1.
- Smoke run used a scratch `-state` whose `cache`, `store`, `render`, `llm-cache` symlink the shared `.ri`
  (investigation records stay in scratch), and K0 (`git archive 03b6ff43 knowledge`). `ri investigate`, not
  `ri eval` (which would score against expected and iterate all cases, including cert-manager).

## Contract questions (for the commander)
1. **`environmentBasis` when no exposed object meets the rule.** Partial basis is accepted (tested); an empty
   basis is rejected (`mitigation: environmentBasis is required (… showing this environment meets the
   conditions)`). Under v8 a "holds on none" rule is recorded with basis = claims that establish the condition's
   state (the tool accepts any resolving reference). That stretches the field's documented meaning ("showing this
   environment meets the conditions"). No safety effect: `EnvironmentBasis` has no consumer besides storage
   (not export, verify, policy or UI — grep). Options: accept as is (document the field as "claims/objects that
   establish the condition's state"), or allow an empty basis with an explicit `meetsOn: none` — a tool-layer
   change I did not make (brief §5).

## Files touched outside ownership
- `internal/investigate/agent/verdict_v7_test.go` (one assertion removed: v7 is no longer the default).
- `docs/INVESTIGATE.md` (v8 entry; prompts paragraph; v6 heading wording).

## Versions
- `DefaultPromptVersion` investigate-agent/v7 → v8. No producer or policy version bump.

## Tests
- `go build ./... && go vet ./... && go test ./...` green at 3e36fd91.
