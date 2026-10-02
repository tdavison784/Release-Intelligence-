# Lane `applicability` — fact × environment in the engine (G17, G18, G20, G21)

Read `_wave1-common.md` first. This is the safety-critical lane (you are Opus for this reason). Per
DESIGN.md §9 `applicability` and §4: `EvaluateCondition` (three-valued, absence-is-not-knowledge,
every true with env evidence, every false with an ImpactCheck against a supplied+healthy dimension,
withheld → unknown), bound to the merged env APIs (`internal/env/resources.go`, product inventory);
`ClassifyKnowledge` per the trust ladder; integration into `impact.Build` (supersession of unknown
records, never downgrading a deterministic finding, both evidence chains, fact evidence copied into the
pool, byte-identical output without facts); `ImpactReport.Validate()` hard rules; UNKNOWN reasons
assigned across the existing join (then mandatory, goldens regenerated with the reason stated); the
generic `crd:storage-changed` join rule and joins for the capture lane's new computed CRD changes
(crd:default-changed / enum-changed / field-required / field-type-changed — derive classes from the
contract, document them in docs/IMPACT.md) and the strimzi-style compatibility support-set change;
`InventoryComplete` declaration; `-knowledge <dir>` / `-min-verification` on `ri impact` and `ri eval`;
per-level and transfer reporting in `internal/eval` (no regression with `-knowledge` unset).
Adversarial tests are mandatory: extend `eval/adversarial/` with knowledge traps (proxy fact trying to
reach ACTION; proxy fact trying to clear; fact attaching to the wrong kind; withheld secret value;
partial manifests; inventory listing only a chart version; product missing from a non-complete
inventory; a fact whose condition is true but consequence is deprecation). For testing use hand-built
facts in testdata — NOT facts derived from eval expectations. Model: Opus.
