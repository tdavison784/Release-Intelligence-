# Wave 1 — common preamble (every wave-1 lane reads this first)

1. `git merge p3-learning-loop` is already your base (the contract lane is merged). Read, in order:
   `docs/phase3/learning-loop/FLEET.md` (binding), `MISSION.md`, `DESIGN.md` (normative — especially
   §9 for your lane's package, files and signatures), `UNKNOWN-ANALYSIS.md` (§0, §3), then
   `internal/domain/semantic.go` and `internal/knowledge/api.go`.
2. DESIGN.md + semantic.go + api.go are the contract. Implement against them exactly. If something in the
   contract is wrong or missing for your lane, make the smallest additive change, mark it
   `// CONTRACT-CHANGE(<lane>): why` and list it in your status file under "Contract changes" — the
   commander reconciles these across lanes.
3. Other lanes are building their parts concurrently. Never edit another lane's package. For tests, use
   fakes of the ports in `internal/knowledge/api.go`, not other lanes' implementations.
4. Done = `go build ./... && go vet ./... && go test ./...` green on your branch, docs updated, status
   file current, final message `LANE DONE:`.
