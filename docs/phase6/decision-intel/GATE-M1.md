# Milestone 1 gate (commander only; label-bearing; lanes never read this file)

Development cases: cert-manager 1.17 → 1.18, payments-us (not affected) and payments-eu (affected).

Facts the gate checks, from the Phase 0 transcripts:
- The `ledger-nginx` ConfigMap holds `ledger.conf`. It exists only inside the render (`art-1df346a0c673`) and
  contains `ssl_client_certificate /tls/ca.crt`, which reads Secret `ledger-tls` key `ca.crt`.
- In arm B, 12 of 45 calls per case went to `submit_findings` and 9–10 to `record_claim`.

## 1. Expected behaviour (the M1 gate)

On both cases, for the rotationPolicy item (and the revisionHistoryLimit item as a control), the RI investigator:

- **(a)** reads the consuming workloads (`ledger`, `checkout`) and the `ledger-nginx` config;
- **(b)** states the CA trust relationship: Certificate → Secret → mounted `ca.crt` → nginx/curl trust;
- **(c)** distinguishes explicit configuration from inherited defaults: the P-US prod overlay pins
  `rotationPolicy: Never` on `mesh-root-ca`, while P-EU inherits the new default;
- **(d)** explains why the two environments have different outcomes;
- **(e)** delivers this as a natural technical assessment (mission B5 sections) without any `SemanticAssertion` or
  `MitigationSpec` being required.

**Gate:** (a)–(e) hold in ≥ 2 of 3 repeats per case, with:
- no new false-safe on any item;
- `classify` output byte-identical in policy terms (verdicts and assessments stay out of the classifier, as
  `verdict_policy_test.go` already proves for verdicts).

The comparison is against the stored Phase 5 run `B0-K0-20261017-edb21ed9-v8`, on the same knowledge snapshot K0, in
offline mode, with the same model (`claude-sonnet-5-5`).

## 2. Regression comparison

```sh
go build -o bin/ri ./cmd/ri
scripts/phase5/snapshot.sh 03b6ff43 <scratch>/K0          # expect sha256:672a7ce0…
bin/ri -state <clone of shared .ri> -offline eval -investigations -arm C1 -run-id <id> \
  -case cert-manager-1.17-1.18--payments-eu,cert-manager-1.17-1.18--payments-us \
  -knowledge <scratch>/K0 -work <empty> -agent -agent-model claude-sonnet-5-5 \
  -agent-prompt investigate-agent/v9 -save           # × 3 repeats
bin/ri eval -investigations -compare base=eval/investigations/results/B0-K0-20261017-edb21ed9-v8,new=<dir>
```

- **Scoring:** the commander scores (a)–(e) per run, quoting the stored assessment, like Phase 0's `score.md`. The
  product owner audits the scores.
- **Also reported:** tool calls, tokens, cost and time against B0 ($0.80 per case, about 160 s, 45 calls).
- **Expected spend:** about $5–8 for 3 × 2 cases, more if the budget increase is used.
- **The shared `.ri`:** cloned, never written.

