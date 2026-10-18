// Package verify independently verifies agent-proposed mitigation rules
// (Phase 5 PROTOCOL §7–§8): for each open hypothesis-sourced review item in a
// knowledge store it produces a domain.ProofRecord from
//
//   - a citation check (deterministic, citations.go): every cited upstream
//     text resolves in the evidence cache, contains the cited passage and
//     names only versions inside the rule's release bounds;
//   - a rule exercise (observed, opt-in, exercise.go): a synthetic minimal
//     fixture built from the rule — the subject object with the rule's
//     mitigating condition present and absent — installed with the
//     controller at FROM in a throwaway kind cluster, upgraded to TO, the
//     product's declared exercise run at both versions, both arms observed.
//
// The outcome is confirmed only when every method passed; any failure
// refutes; anything else is inconclusive. The proof gates promotion
// (internal/knowledge promote.go); it never decides whether an environment
// meets the rule's conditions — the deterministic validators do that at
// investigation time. The package is plain Go: no product names, no case
// shapes; products contribute declarative fixtures
// (products/exercises/fixtures/<product>.yaml).
package verify
