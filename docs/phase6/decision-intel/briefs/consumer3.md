# Lane `consumer3`: Milestone 1 fix, file paths in container command, args and env (Opus)

Branch `p6di/consumer3`, worktree `.claude/worktrees/p6di-consumer3`. Ledger D-0006. Read `_common.md` first, then
`PLAN-M1.md`, `M1-IMPLEMENTATION.md`, and `status/consumer.md` and `status/consumer2.md` (merged at 6f5de003).

## The limit (documented in M1-IMPLEMENTATION.md "Known limits")

`reads-file` edges are derived only from paths in ConfigMap data. A workload often names the mounted files it reads
directly in its pod spec, for example a TLS client flag in the container's `command`, or an env var holding a file
path. Those relationships are invisible today, so `trace_consumers` reports "mounted, what it reads is not
established" even when the pod spec says exactly which files it reads.

## Required

1. **Pod-spec paths.** For each container, find absolute paths in `command`, `args` and literal `env[].value` strings
   (including shell scripts passed as `sh -c` arguments) with the existing `filePaths` matcher. Match them against the
   **same container's** mounts, and emit `reads-file` edges from the provider object, and its key when it resolves, to
   the workload.
   - Record which field and line state the path, with artifact and line citations like the existing edges.
   - Paths that match no mount become unresolved edges, as today.
   - Leave `valueFrom` references alone: those are already `references` edges.
2. **Same rules versioning as consumer rules v2.** Bump to v3. Old transcripts replay with the rules they recorded,
   and v1 and v2 rebuild their graphs byte for byte (extend the existing comparison test).
3. **Tests.**
   - A command with a multi-line `sh -c` script naming two files under a Secret mount, which gives two edges with
     keys.
   - An args path under another container's mount, which gives no edge.
   - An env value path under no mount, which gives an unresolved edge.
   - A URL-like or `${VAR}/x` string, which gives no path.
   - `trace_consumers` returns the new hops with citations.
   - Replay pinning.
4. **No live runs** on the development cases. A live smoke on a non-development case is optional.
5. **Docs:** update `M1-IMPLEMENTATION.md` (known limits) and your status file.

FLEET.md applies, including generic logic only and the files you never open (`GATE-M*.md`, `M*-RESULT.md`).
