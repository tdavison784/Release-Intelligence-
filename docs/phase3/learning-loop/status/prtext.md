# Lane `prtext` status

**State: done** (branch `p3ll/prtext`). `go build ./... && go vet ./... && go test ./...` pass; schemas regenerated.
No LLM calls; GitHub API only (core quota 5000/h, never throttled; wall time 10m38s for all 15 edges).

## Built (see docs/LINKED_EVIDENCE.md)
- `internal/github/linked.go`: `Client.LinkedItem` (issues/N → PR or issue; pulls/N/files first page; commits/SHA), through
  the cached fetch client; description ≤ 8 000 B, ≤ 100 files; digest over shown content, not volatile fields.
- `internal/domain`: evidence kinds `linked-pr`, `linked-commit` (+ schemagen enum, schemas regenerated).
- `internal/linkedev`: `TargetsOf` (pull-request/issue/bare #N/commit URL on github.com; deduped, ranked), `Collect`
  (≤ 4 items per change, ≤ 2000 per edge, one fetch per item; **a throttle stops the run**, remaining targets reported
  `throttled`, Retry-After surfaced), `EvidenceFor` (title+description, description part 2, changed files; each ≤
  `domain.MaxExcerpt`; content-derived ids), `Apply` (stored candidates ↔ edge changes by change id, falling back to the
  statement anchor), `CodeReposOf`.
- `knowledge.ExtendCandidateEvidence` + `mutableChange` for candidates (**CONTRACT-CHANGE(prtext)**): a candidate's
  evidence may only grow (existing records kept, in order; everything else identical). Idempotent; ids do not move.
- `ri evidence link [-dir] [-dry-run] [-edges FILE] [-report FILE] [<product> <from> <to>]`.
- Tests (offline, checked-in fixtures `internal/github/testdata/linked_*.json`): fetcher (PR/issue/commit/404/429/bounds/
  hostile input/offline replay), targets/collect/throttle/caps/bare refs/evidence shape+ids, store extension (idempotent,
  refusals), Apply end to end.

## Finding fixed during the run
Bare `#N` in notes read from a docs repo (cert-manager website) carried the docs repo, so 34 of 73 items on cert-manager
1.17→1.18 were "not found". Bare references are now tried in the product's version-source repositories (catalog
`versions` sources) first, then the extraction repo; explicit URLs / `owner/repo#N` are never re-targeted; the evidence
says "(cited in the notes as #N)". Heuristic: the URI shows where it resolved.

## Results, all 15 environment edges (`runs/prtext/report.json`, `run.txt`)
Changes 2 267; **977 changes carry a PR/issue/commit reference and all 977 gained linked evidence** (1 196 distinct items
fetched, 2 977 evidence records, 0 not-found / 0 errors / 0 capped / 0 throttled).

| edge | changes | with refs | candidates matched | gained | gained with needs-evidence items |
|---|---|---|---|---|---|
| argo-cd 2.14.5→3.0.0 | 460 | 417 | 183 | 164 | 127 |
| cert-manager 1.16→1.17 | 41 | 26 | 37 | 25 | 7 |
| cert-manager 1.17→1.18 | 52 | 36 | 42 | 32 | 13 |
| cilium 1.15.6→1.17 | 275 | 0 | 118 | 0 | 0 |
| cilium 1.16.1→1.17 | 144 | 0 | 72 | 0 | 0 |
| crossplane 1.20→2.0 | 201 | 117 | 144 | 84 | 68 |
| external-secrets 0.15→0.16 | 108 | 74 | 59 | 37 | 22 |
| flux 2.6.4→2.7 | 115 | 44 | 88 | 42 | 30 |
| istio 1.23.4→1.24 | 170 | 52 | 91 | 52 | 15 |
| karpenter 0.37.8→1.0 | 56 | 28 | 42 | 27 | 24 |
| kyverno 1.12.6→1.13 | 107 | 0 | 30 | 0 | 0 |
| loki 2.9.6→3.0 | 291 | 100 | 224 | 90 | 62 |
| prometheus-operator 0.85→0.86.2 | 38 | 19 | 15 | 1 | 1 |
| strimzi 0.45→0.46 | 42 | 0 | 31 | 0 | 0 |
| traefik 2.11.2→3.0 | 167 | 64 | 94 | 58 | 41 |

**Distinct candidates: 612 of 1 193 matched gained linked-PR evidence; 410 of those 612 have at least one review item
whose status is `needs-evidence` (526 such items), by `ReviewItem.Status` only — no decision, review item or eval link was
read or changed.** 1 900 evidence records were added to the store (612 candidate files modified; nothing else under
`knowledge/`). Re-running is a no-op ("+0 new", verified on cert-manager 1.17→1.18).

## Not covered / notes
- cilium, kyverno, strimzi: the notes carry no PR/issue/commit references (advisory ids only); nothing to link.
  prometheus-operator: 19 changes cite PRs but only 1 candidate gained (most changes are not candidates).
- 977 changes are not 977 candidates: restatement clusters and skipped members (routine/umbrella) mean 612 candidates.
- Candidate prompts that show these records are longer and digests change for the 612 gained candidates; re-proposal /
  re-review is a commander decision. The prompt shortens each excerpt to 700 chars and the set is bounded by the existing
  prompt-length halving, so very evidence-rich candidates may show shortened excerpts.
- The fetch cache for these calls is in the primary checkout's `.ri/cache` (I used `-state` there for its warm edges).

## Outside ownership
`internal/knowledge/filestore.go` (candidate mutability, CONTRACT-CHANGE), `internal/domain/evidence.go` + schemagen,
`cmd/ri/main.go` (command registration), `knowledge/**/candidates/*.json` (612 files, evidence appended), `README.md`.
