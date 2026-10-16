# Status — lane `knowledge` (Claude Sonnet 5.5, agent ri-knowledge)

Branch `p4ei/knowledge`. Brief: `briefs/knowledge.md`. State: **IN PROGRESS** (2026-10-16).

## Done
- Candidates built (warm cache `-state <primary>/.ri`):
  - ingress-nginx `1.11.5 → 1.12.0` (controller train; chart 4.11.5→4.12.0 appVersions): 54 candidates
  - kube-prometheus-stack `89.2.4 → 90.2.0`: 6 candidates
- Pass 1 of `ri semantic propose` (no new prompts, prompt v2) wrote 120 requests (60 candidates x 2 models) and
  candidate records into `knowledge/`.

## In progress
- Answering requests: `scripts/semantic-exchange.sh <dir> "" 8 anthropic` (exchange dirs in the session scratchpad).
- Next: re-run the same propose commands to ingest, `ri knowledge` checks, go build/vet/test, reviewfilter counts.

## Decisions
- Edge versions: ingress-nginx product is the controller train, so chart 4.11.5→4.12.0 is controller 1.11.5→1.12.0.
- Models: claude-opus-5-5 + claude-sonnet-5-5 (claude exchange). No proxy review / consensus / verification.

## Commands
```
ri -state <primary>/.ri semantic propose -model claude-opus-5-5,claude-sonnet-5-5 -llm-exchange $X/in -out knowledge ingress-nginx 1.11.5 1.12.0
ri -state <primary>/.ri semantic propose -model claude-opus-5-5,claude-sonnet-5-5 -llm-exchange $X/kp -out knowledge kube-prometheus-stack 89.2.4 90.2.0
```
