# Lane backdoor: status (Milestone 4 safety fix; ledger D-0029)

Branch `p6di/backdoor` (from `p6-decision-intel` @ 66e239d3). Brief: `briefs/backdoor.md`. Design, tests, stored
records and known limits: `M4-IMPLEMENTATION.md`, lane `backdoor`.

## Done

1. **Every route names the other facts** (11a5b5bf): `FactFromDecision` refuses an accept / correct / duplicate that
   mints or changes a fact while another active fact of its product, release and subject is unnamed, or kept
   (`coexists`) although incompatible (`domain.FactIncompatibility`). Ordinary and hypothesis items alike.
2. **Contradiction reporting** (a3424f68): loader warning naming both; the join keeps only affected findings of a
   contradicted fact and records an `impact:knowledge-contradiction` gap naming the other. UI choice on ordinary
   items; `ri knowledge decide -supersedes/-coexists`.
3. **Tests** for all of the brief's §4, each checked to fail without the fix; full suite green.
4. **Stored records**: everything loads; two proxy-level stored pairs reported (not changed); real-data route B on a
   scratch K1 copy refused. `m4rescore` 1004/1004 identical.
5. **Docs**: `docs/IMPACT.md` ("Contradicting facts"), `docs/INVESTIGATE.md` (promotion gate), README command line,
   `M4-IMPLEMENTATION.md` lane section. Schemas regenerated.

## In progress (exact next step)

- Nothing: lane done; awaiting the commander's merge.

## Decisions

- **Scope (product, release, subject)** for ordinary routes, as the brief says; the hypothesis gate keeps its wider
  product-wide scope (narrowing it would weaken it).
- **Compatible = same change, same exposure, same consequence kind/class.** The overlap, severity and prose may
  differ; an unstated before/after is no disagreement. Without this the existing explicit-coexistence test
  (`TestGatePromotesProseOnlyCorrectionAndExplicitCoexistence`: a narrower rule that adds only an overlap) and three
  stored duplicate pairs would count as contradictions. Conditions compare as written (fail closed).
- **Join: contradiction only among usable facts** (those the join evaluates at the requested level): only they can
  clear. The loader reports every kept pair. A contradicted fact's affected finding stands (stricter wins); the gap
  is recorded beside any verdict, deterministic included.
- **Reason `release-knowledge-gap`** for the gap: "no trusted fact says what the change means", routed to review.

## Files touched outside ownership

- `internal/domain/` (additive: `contradiction.go`, `RuleKnowledgeContradiction`, a relaxed `coexists` check for
  duplicates), `internal/domain/schemagen/meta.go`, `schemas/impact-report.schema.json`.
- `internal/impact/knowledge.go`, `internal/app/knowledge.go`, `internal/reviewui/*`, `cmd/ri/knowledge.go`.

## Contract changes

- `ReviewDecision.coexists` is now also valid on a reject with `duplicateOf` (never `supersedes`).
- New impact rule `impact:knowledge-contradiction` (class `unknown`, carries `knowledge`).

## Open questions (commander)

- Should auto-approval (`ri knowledge route`) also refuse to mint a fact that contradicts an active one? Out of the
  brief's routes; today it is reported and blocked in the join.

## Test status

- `go build ./... && go vet ./... && go test ./...` green; `go test -tags m4rescore -run TestM4Rescore ./cmd/ri` green.
