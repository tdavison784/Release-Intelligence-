# SAME-MODEL experiment store — not cross-model, not part of the live knowledge/ tree

Two separate call-sets of **one model** (claude-sonnet-5-5 × 2, stateless `claude -p` calls, distinct
call ids) over strimzi 0.45.0→0.46.0, prompt `semantic-full/v1`, 2026-10-02, $2.46. Consensus scope
of any agreement in here is `same-model` (domain.ConsensusScopeOf). Kept outside `knowledge/` so the
loop never routes it; load it explicitly (`knowledge.NewFileStore(<this dir>)`) to study same-model
self-consistency (results: ../README.md § Same-model).
