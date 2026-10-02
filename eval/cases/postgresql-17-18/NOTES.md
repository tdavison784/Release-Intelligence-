# Research notes: PostgreSQL REL_17_4 → REL_18_0

Researched blind (no Release Intelligence output consulted) on 2026-10-01.

## Sources read

- Official PostgreSQL 18 release notes, section "E.6.2. Migration to
  Version 18":
  https://www.postgresql.org/docs/18/release-18.html
  (same content in-repo at
  https://github.com/postgres/postgres/blob/REL_18_0/doc/src/sgml/release-18.sgml)

## Judgement calls

- The migration section lists ~12 incompatibilities. Kept the six with real
  operator consequences; folded related ones:
  - "COPY FROM \. handling" and "unlogged partitioned tables" were left out:
    both are narrow (CSV copy across versions; unlogged partitioned tables).
  - "AFTER triggers run as the queuing role", "GRANT/REVOKE rule privileges
    removed", "pg_backend_memory_contexts.parent removed / level one-based"
    left out as minor/internal (the last recorded under notExpected).
- E2 is critical: it changes what a NEW cluster is created with AND constrains
  pg_upgrade (checksum settings must match).
- E6 matters specifically for ICU/builtin-collation clusters migrating with
  pg_upgrade (reindex FTS + pg_trgm).
- notExpected: PG18's headline performance features (async I/O etc.) are
  features, not incompatibilities — a tool flagging them as breaking for this
  edge would be miscategorising.
