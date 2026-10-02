# Benchmark brief (continuation) — you are the model under test: claude-haiku-4-5

T1 is already finished. Do NOT touch r4-exchange, r4-state, r4.json or r4.stderr.
B=/Users/tommydavison/repos/ri-wt/p3-main/.ri/bench/claude-haiku-4-5

## Rules (strict)
- Answer every request YOURSELF, as claude-haiku-4-5. For EACH request: open it with the Read tool, reason about
  that request only, then create its response file with the Write tool. One request at a time.
- NO scripts, loops, keyword rules or templates to produce answer content. Never use python or bash to write responses.
  A previous scripted attempt was discarded for exactly this reason.
- Use ONLY the request's `request.system`, `request.messages` and `request.jsonSchema`. Do not read repo source, docs,
  other models' answers, the GLM cache, or the web. Do not call other LLMs or sub-agents.
- Citations must be evidence ids that appear in that request's prompt. Never edit request files. If ri rejects an
  answer on ingest, do NOT re-answer it — a rejection is data.
- generatedAt must be the real current UTC time (run `date -u +%Y-%m-%dT%H:%M:%SZ` once at the start and use it).

## Steps
1. If r4.summary does not exist: `grep -m1 '^impact enrich:' $B/r4.stderr > $B/r4.summary`
2. The 30 T2 requests already exist in $B/smoke-exchange. For each <hex>.request.json write <hex>.response.json:
   {"format":"ri.dev/llm-exchange/response/v1","promptDigest":"<copied verbatim>","model":"claude-haiku-4-5","modelVersion":"claude-haiku-4-5","generatedAt":"<RFC3339 UTC>","output":<your JSON answer conforming to request.jsonSchema>}
3. From /Users/tommydavison/repos/ri-wt/p3-main run:
   bin/ri -offline -state $B/smoke-state impact cert-manager v1.17.0 v1.18.0 --repo internal/env/testdata/customer-repo --kubernetes 1.28 -enrich -model claude-haiku-4-5 -enrich-max 30 -llm-exchange $B/smoke-exchange -o json > $B/smoke.json 2> $B/smoke.stderr
   then `grep -m1 '^impact enrich:' $B/smoke.stderr > $B/smoke.summary`
4. From the repo root: bin/ri eval cert-manager-1.17-1.18 -enriched -model claude-haiku-4-5 -llm-cache $B/r4-state/llm-cache -o json > $B/eval.json 2> $B/eval.stderr  (non-zero exit is expected)
5. Reply with ONLY: the T1 and T2 summary lines, suggestionPrecision/suggestionRecall from eval.json, and the number of requests you answered in this session.
