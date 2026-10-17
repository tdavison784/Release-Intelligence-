# Status — lane `knowledge` (Claude Sonnet 5.5, agent ri-knowledge)

Branch `p4ei/knowledge`. Brief: `briefs/knowledge.md`. State: **DONE** (2026-10-16).

## Done
- Candidates built (warm cache `-state <primary>/.ri`):
  - ingress-nginx `1.11.5 → 1.12.0` (controller train; chart 4.11.5→4.12.0 appVersions): 54 candidates
  - kube-prometheus-stack `89.2.4 → 90.2.0`: 6 candidates
- Pass 1 of `ri semantic propose` (no new prompts, prompt v2) wrote 120 requests (60 candidates x 2 models) and
  candidate records into `knowledge/`.

## Results
All 120 requests answered (0 pending), ingested, routed. Models claude-opus-5-5 + claude-sonnet-5-5, prompt `semantic-full/v2`, one stateless `claude -p` call each.

| product (edge) | candidates | proposals | refusals | review items |
|---|---|---|---|---|
| ingress-nginx 1.11.5→1.12.0 (chart 4.11.5→4.12.0) | 54 | 107 (opus 54, sonnet 53) | 1 (sonnet, sc-c654561feb5a: value-changed missing before/after; in `knowledge/ingress-nginx/failures/`) | 99 |
| kube-prometheus-stack 89.2.4→90.2.0 | 6 | 12 (6+6) | 0 | 11 |

Routed with `ri knowledge route -auto-approve=false -audit-every 0 -product <p>`: 0 facts auto-verified, 0 audit items.
No proxy review, no consensus acceptance, no `decide`; nothing verified. Agreement is same-model-family only.

Token use / cost (claude -p envelopes): ingress-nginx 108 calls, ~1.27M input (incl. cache) / 63k output tokens, $8.66;
kube-prometheus-stack 12 calls, ~147k input / 7.8k output, $1.01. Total ≈ $9.67.

## Review UI (`ri review serve -knowledge knowledge`, default view)
Before this lane: 641 need review (21 block investigations). After: **751 need review, 128 block investigations**,
i.e. all 110 new items show and 107 of them are blocking-first (their products are investigated by
`eval/investigations/*/input.yaml`, inputs only; expected.yaml never read). Server stopped afterwards.

## Checks
`go build ./... && go vet ./... && go test ./...` green (includes `TestStoreIntegrity`).

## Decisions
- Edge versions: ingress-nginx product is the controller train, so chart 4.11.5→4.12.0 is controller 1.11.5→1.12.0.
- Release-level only; no render (`-render` not used, as the brief did not ask); no eval/investigations read.
- Side effect: the kube-prometheus-stack edge also wrote candidate/review dirs under releases 90.0.0 and 90.1.0
  (the edge spans them); expected.

## Files outside ownership
None (`knowledge/` data and this status file only).

## Cleanup
Removed `bin/`; scratch exchange dirs live in the session scratchpad (outside repo); review server stopped; no kind clusters; shared `.ri` untouched.
