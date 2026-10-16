# Wave 2 — common preamble (every wave-2 lane reads this first)

Same rules as `_wave1-common.md` (read it too): FLEET.md binding; DESIGN.md + `internal/domain/investigation.go`
+ `internal/investigate/api.go` are the contract (§7 "Wave 2" names your package, files, signatures);
`// CONTRACT-CHANGE(<lane>)` for unavoidable contract edits; never edit another lane's package; never open
`eval/investigations/*/{expected.yaml,NOTES.md,observed/}`.

Difference from wave 1: the wave-1 packages are **merged** — `internal/investigate/{bundle,store,renderdiff,
validate,relations}`. Use the real ones in integration-style tests where cheap, fakes where they make a test
slow or network-bound. Read their status files (`status/<lane>.md`) for known gaps before relying on them.
Done = build/vet/test green, docs (your section of `docs/INVESTIGATE.md` — add a heading for it), status file,
cleanup, `LANE DONE:`.
