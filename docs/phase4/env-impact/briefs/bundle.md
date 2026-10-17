# Lane `bundle` — change evidence, environment snapshot, evidence bundle (P1) — Sonnet

Branch `p4ei/bundle`, worktree `.claude/worktrees/p4ei-bundle`. Read `_wave1-common.md` first.

Implement DESIGN §7 "bundle" exactly (owns `internal/investigate/bundle/`, `internal/investigate/store/`):
the content-addressed store (§2.3 layout, atomic writes, digest-verified reads, immutability, audit only
grows), `ChangeCollector` (over `app.Upgrade` + the release-scope chart-default render pair; §1.3 scope
rules incl. the per-item **manifestation** rule if the contract defines it; every `ChangeSourceKind`
stated with a collection status), `SnapshotBuilder` (over `env.Load`, repo ref with commit / dirty / tree
digest, Secret redaction per §2.3), and `BundleAssembler`. The `renderdiff` lane supplies environment
renders through its port — use a fake in your tests.

Done: a bundle that `Validate`s for every groundtruth case's `input.yaml` (live upstream reads go through
the warm cache: `-state /Users/tommydavison/repos/Release-Intelligence-/.ri`; tests use checked-in
fixtures/caches only); determinism test (two runs → same bundle id); docs (a short
`docs/INVESTIGATE.md` section "Evidence bundle" (skeleton exists; edit your section only)).
