// Package reportui is the per-upgrade investigation view (lane report): every
// stored investigation with what was checked, every verdict and the rule that
// decided it, the evidence behind it, AI output labelled and kept apart from
// validated results, and the human review of the run (approve / reject /
// request more evidence through the ReviewApplier, with the hash-chained
// audit trail). It also exports one investigation as a self-contained HTML
// report plus its run-record JSON for a release ticket.
//
// It is stdlib only (net/http, html/template, embed), reads the investigation
// store (writable) or stored evaluation runs (read-only), and is mounted by
// `ri review serve` under /investigations and served alone by
// `ri investigate serve`.
package reportui
