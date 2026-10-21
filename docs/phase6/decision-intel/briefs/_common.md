# Phase 6 decision intel: common preamble (every lane reads this first)

- Read these in order:
  1. `docs/phase6/decision-intel/FLEET.md` (binding; it points to the Phase 5 FLEET.md for the general rules);
  2. `MISSION.md`;
  3. your milestone's `PLAN-M<n>.md`;
  4. your brief.
- Keep `status/<lane>.md` current as you go (Phase 5 FLEET.md "Reporting and continuity").
- **Done means:**
  - `go build ./... && go vet ./... && go test ./...` green;
  - schemas regenerated if `internal/domain` changed;
  - docs updated;
  - `status/<lane>.md` current;
  - cleanup per FLEET.md;
  - a final message starting `LANE DONE:`, of ≤ 15 lines.
