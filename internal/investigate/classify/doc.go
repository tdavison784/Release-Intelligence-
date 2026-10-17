// Package classify is the deterministic four-class policy of an
// environment-aware investigation (DESIGN.md §3, `classify.policy@v1`; MISSION
// §A6, AC6, AC8) and the human-review applier.
//
// New returns the investigate.Classifier: for every in-scope change item it
// applies the rules in order, first match deciding —
//
//	R1 classify:validated-exposure   → affected               (a failing deterministic/observed validation)
//	R2 classify:validated-clear      → not-affected           (R2.1–R2.8 all hold)
//	R3 classify:plausible-impact     → potentially-affected   (an exposure signal, with next steps)
//	R4 classify:insufficient-evidence→ insufficient-evidence  (gaps + next steps)
//
// and then the investigation-level approval decision (§3.4), which runs in
// shadow mode: eligibility is computed and recorded, automated approval is
// never enacted in milestone 1.
//
// Inputs are the evidence bundle, the traversal, the deterministic join's
// findings, the validation records and the agent records — never model
// confidence or model agreement. Agent records can only raise concern
// (§3.3): a raises-concern assertion blocks R2 until a passing validation
// answers it; a claims-mitigation assertion is an unvalidated assumption on
// the finding until a validation establishes it; an explains assertion is
// attached, labelled AI, and changes nothing.
//
// For an item whose manifestation is not rendered-config (a default compiled
// into the controller, server-side defaulting, or unknown), render silence
// proves nothing: only an override inspection that established a pinned
// setting, or an observed validation, counts for R2 (R2.4). When such an item
// ends in R3/R4, the next steps name the setting to look for, taken from the
// item's upstream evidence and the agent's hypotheses.
//
// The classifier is pure: same input ⇒ byte-identical output (input order
// does not matter).
package classify
