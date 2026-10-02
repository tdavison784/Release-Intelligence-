# Benchmark brief — you are the model under test: claude-haiku-4-5

Worktree: /Users/tommydavison/repos/ri-wt/p3-main (branch p3-model-comparison). Bench dir: B=/Users/tommydavison/repos/ri-wt/p3-main/.ri/bench/claude-haiku-4-5
Do NOT git commit, do NOT modify anything outside $B (never touch products/, eval/, internal/, docs/).

## Rules (read carefully)
- You answer every enrichment request YOURSELF, as claude-haiku-4-5, by reading and reasoning about it.
- Use ONLY that request file's `request.system`, `request.messages` and `request.jsonSchema`.
- Do NOT read repo source code, docs, other models' answers (anything under .ri/bench* other than $B), the GLM cache
  (internal/app/testdata/impact-llm-cache), or the web. Do not call other LLMs (no `claude -p`, no API calls, no sub-agents).
- Answer each request independently. Do not tune answers to ri's validator. Never generate answer content with a script,
  template or heuristic — every answer's content is your own judgment for that request. (Writing the JSON file with a
  shell heredoc or the Write tool is fine.)
- If ri rejects one of your answers on ingest, that is data: do NOT re-answer, edit, or retry it. Never edit request files.

## Steps
1. T1 — from /Users/tommydavison/repos/ri-wt/p3-main/internal/app run exactly:
   ../../bin/ri -products ../../products -offline -state $B/r4-state impact cert-manager v1.17.0 v1.18.0 --kubernetes 1.28 --values testdata/e2e/env/cert-manager/values.yaml --manifests testdata/e2e/env/cert-manager/manifests --crds testdata/e2e/env/cert-manager/crds --images testdata/e2e/env/cert-manager/images.txt -enrich -model claude-haiku-4-5 -enrich-max 20 -llm-exchange $B/r4-exchange -o json > $B/r4.json
   This writes one <hex>.request.json per prompt into $B/r4-exchange.
2. For every <hex>.request.json, write <hex>.response.json next to it:
   {"format":"ri.dev/llm-exchange/response/v1","promptDigest":"<copied verbatim from the request>","model":"claude-haiku-4-5","modelVersion":"claude-haiku-4-5","generatedAt":"<RFC3339 UTC now>","output":<your JSON answer conforming to request.jsonSchema>}
3. Re-run the exact T1 command to ingest. Capture stderr: append `2> $B/r4.stderr` and save the "impact enrich: ..." summary line to $B/r4.summary.
4. T2 — from the repo root /Users/tommydavison/repos/ri-wt/p3-main run:
   bin/ri -offline -state $B/smoke-state impact cert-manager v1.17.0 v1.18.0 --repo internal/env/testdata/customer-repo --kubernetes 1.28 -enrich -model claude-haiku-4-5 -enrich-max 30 -llm-exchange $B/smoke-exchange -o json > $B/smoke.json
   Answer every request in $B/smoke-exchange as in step 2, re-run the same command with `2> $B/smoke.stderr`, save the summary line to $B/smoke.summary.
5. T3 — from the repo root run:
   bin/ri eval cert-manager-1.17-1.18 -enriched -model claude-haiku-4-5 -llm-cache $B/r4-state/llm-cache -o json > $B/eval.json 2> $B/eval.stderr
   Non-zero gate/regression exit codes are expected; keep the output.
6. Reply with ONLY: the T1 and T2 summary lines, the eval suggestion-scoring numbers (suggestionPrecision/suggestionRecall and their n), and the count of requests you answered (T1 + T2).
