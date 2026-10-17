# Wave 1 — common preamble (every wave-1 lane reads this first)

1. Your branch was created from `p4-env-impact` after the `contract` lane merged. Read, in order:
   `FLEET.md` (binding), `MISSION.md` (incl. the product-owner clarification: render **diffs** are the
   centerpiece), `DESIGN.md` (normative — §0 gap map, §2 contracts, §3 policy, and **§7 for your lane's
   package, files and signatures**), then `internal/domain/investigation.go` and `internal/investigate/api.go`.
2. DESIGN.md + investigation.go + api.go are the contract. Implement against them exactly. If something
   is wrong or missing for your lane, make the smallest additive change, mark it
   `// CONTRACT-CHANGE(<lane>): why`, and list it in your status file under "Contract changes".
3. Other wave-1 lanes build their packages concurrently. Never edit another lane's package; use your own
   in-memory fakes of the ports you consume (DESIGN §7).
4. Fixtures: `eval/investigations/<case>/input.yaml` and `env/` are yours to use. **Never open
   `expected.yaml`, `NOTES.md` or `observed/`** (ground truth; FLEET "Evaluation integrity"). Your tests
   assert generic behaviour, not case answers.
5. Reuse the existing packages DESIGN §0 names (`internal/render`, `internal/env`, `internal/impact`,
   `internal/app`, …) — build on them, not beside them. No product-specific Go logic.
6. Done = `go build ./... && go vet ./... && go test ./...` green, docs updated, status file current,
   cleanup done (FLEET), final message `LANE DONE:`.
