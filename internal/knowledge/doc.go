// Package knowledge is the learning loop's knowledge layer
// (docs/phase3/learning-loop/DESIGN.md): the durable store of semantic
// candidates, model proposals, validation results, review items, review
// decisions and verified facts; the review queue that routes open aspects to
// engineers; the conversion of decisions into release-level facts; the
// human-feedback dataset; and the loop's metrics.
//
// api.go holds the PORTS only — the interfaces and their data types that the
// wave-1 lanes implement and consume (semantic: Proposer; validate: Validator;
// knowledge: Store, Queue; dashboard: Queue consumer; applicability: Snapshot
// consumer). The entity types themselves live in internal/domain/semantic.go.
//
// The package depends on internal/domain only. It never imports the impact
// engine, and the impact engine never imports it: impact.Build receives
// []domain.VerifiedFact, so model proposals can never reach a classification.
package knowledge
