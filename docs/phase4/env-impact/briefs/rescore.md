# Lane `rescore` — score with the product owner's human-verified facts — Sonnet

Branch `p4ei/rescore`, worktree `.claude/worktrees/p4ei-rescore`. Read FLEET.md, `EVAL-REPORT.md` (§2e/§2f),
`status/eval.md` (how runs are made, stored and packed), `docs/KNOWLEDGE.md`. You own `eval/investigations/results/`
(new runs only), `EVAL-REPORT.md` (a new section), your status file.

The product owner human-verified 5 pin-source review items (commit e62355fc): three cert-manager 1.18.0 facts are
now `human` (rotationPolicy Never→Always; two revisionHistoryLimit nil→1); two ingress-nginx 1.12.0 accepts produced
no fact.

1. Score **arm A** and **arm C** (runtime; delete kind clusters) on the current p4-env-impact with the knowledge
   store as input (`-knowledge knowledge`, minimum verification `human`), new run ids, keep every earlier run, pack
   results like the existing ones. Report all gates vs A₅ / C₅ and every decision that moved, with the rule and the
   validation that decided it (pipeline output only in anything you hand to other lanes). Do not change pipeline
   code, prompts, policy, matcher, FORMAT or expectations.
2. Explain (do not change the store) why the two ingress-nginx accepts (`knowledge/ingress-nginx/1.12.0/decisions/
   rd-4c1f0155a7eb.json`, `rd-ff94bf6fdbf8.json`) composed no fact: which other questions of those candidates are
   still open, and whether the review UI's default view shows them. Report it for the commander.
3. EVAL-REPORT: new section "Human-verified facts (2026-10-16)".

Done: runs committed, report section, build/vet/test green, cleanup, `LANE DONE:` with the gate table.
