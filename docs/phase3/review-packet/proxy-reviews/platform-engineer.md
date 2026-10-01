# PROXY REVIEW — platform engineer

> **PROXY REVIEW — performed by an AI agent simulating the persona; NOT human
> validation.** It exists to de-risk the questionnaire and the packet before
> real reviewers see them. The "reviewer" shares an origin with the packet's
> authors, so its judgments validate process (are questions answerable, do
> citations resolve, does scoring flow), not human usability or real-world
> value. Every number below must be re-measured with real engineers.

Reviewer: proxy-platform-engineer (AI-simulated)
Persona: senior platform engineer; owns Helm charts, image mirroring and the
upgrade calendar for ~40 clusters; has never seen this tool or its code.
Date: 2026-10-01 · Time spent: ~40 minutes · Blind: reports reviewed cold from
`report.txt` + `example-env/` inputs only; source code not read; citations
spot-checked by fetching them.

---

## How I reviewed

Read R1 → R4 cold as rendered text, opened the environment inputs alongside
each, then spot-checked three citations by URL + line + excerpt. Then filled
the answer sheet. Notes below are what I wrote while reading.

## Cold-read notes

**R1 (compat action-required).** The funnel is legible in ten seconds: one
ACTION REQUIRED, and it's the one I'd have found myself — the cluster is 1.28,
v1.18.0 supports 1.29–1.33. The three REVIEW REQUIRED items are the two image
pins and a values key that becomes live. The two image findings are things I
would have caught in a chart diff; the `global.rbac.disableHTTPChallengesRole`
one is the interesting one — a key we set that was previously ignored becomes
live. That is a genuine "go look at it" item and I would not have thought to
check it.

But the ACTION REQUIRED wording bothers me: it says "v1.18.0 **requires**
Kubernetes 1.29–1.33". Two findings later the same report says the chart's
`kubeVersion` constraint is `>= 1.22` and that my 1.28 **satisfies** it
(`imp-d55868f8741c`, NOT AFFECTED). Both are cited and both are true, but the
report simultaneously tells me the upgrade requires ≥1.29 and that 1.28
satisfies the chart constraint. What actually happens if I run it on 1.28?
Presumably: it installs, and it's just unsupported/untested. That distinction
matters for how I schedule it (hard blocker vs. calendar preference). The
class is defensible — I would not upgrade cert-manager on an unsupported
cluster — but the detail text should say "outside the tested/supported
range" not "requires", and ideally reconcile the two constraints in one
place. As shipped, a junior following the letter of the report would think
Helm will refuse.

**R2 (informational).** Same edge, cluster 1.31, pinned default. The
informational class reads correctly to me: "you pin it, the default change
doesn't apply, you're safe" — with a nudge to re-check the pinned value
against new defaults later. Correct and useful; this is exactly the item that
would otherwise cost me a chart-diff archaeology session. No false ACTION
REQUIRED here.

**R3 (repo mode).** Discovery found what I'd expect from the miniature repo
(two values files, manifests, workflow images, Terraform, Argo CD inline
values) and the warnings section told me what it did NOT do (vendored chart
skipped, helmfile read literally, kustomization not applied, last-values-wins,
valuesFrom unreadable). That honesty is the difference between "interesting
toy" and "usable"; with those warnings I know what to double-check by hand.
My discovery gripe: two values files applied last-wins is stated, but the
report doesn't show me which keys collided between dev and prod values
(`replicaCount` differs; `targetPort` doesn't). A collision list would cost
nothing and prevent a real class of mistake.

**R4 (AI-enriched).** The deterministic content is unchanged, the suggestion
lines are visibly weaker than verdicts, and the two schema-rejected answers
are printed at the bottom instead of being hidden — that last point raised my
trust. The suggestions are mostly reasonable triage prompts. Weak ones: the
`ciss` short-name suggestion (an additive alias — nobody's upgrade plan
changes) and the annotation-copy flag (opt-in feature, "review the release
notes" is what I was already doing). The dependency-CVE suggestions are the
valuable ones — but they lead me to my biggest complaint, see A2 below.

## Citation spot-checks (3)

| Report | Finding | Evidence | What I did | Result |
|---|---|---|---|---|
| R1 | imp-7e9a11c2834d | ev-94a8abc4e147 | Opened `github.com/cert-manager/website/blob/master/content/docs/releases/README.md` L306 | **Holds up.** Table row for 1.18: "1.29 → 1.33", matches excerpt verbatim |
| R1 | imp-123e6594ecd5 | ev-87e79b56b7a6 | Fetched release asset `cert-manager.yaml` v1.17.0, line 13047 | **Holds up.** `image: "quay.io/jetstack/cert-manager-controller:v1.17.0"` exactly |
| R1 | imp-6cae808def98 | ev-717ddb6f40cd | Opened chart `values.yaml` at v1.17.0 | **Resolves, weak locator.** The URI is file-level (no line), the excerpt is a `# +docs:section=Global` marker found in the file. I ended up verifying the values claim by reading the file myself. It checked out (the key's default did change in v1.18.0), but this citation form is the weakest of the three |

3/3 survive (one with a locator-precision complaint). One incidental find: the
website citations point at `master`; they work today but a moved line breaks
them silently. Pin them.

## Answer sheet (filled)

```yaml
meta:
  reviewer: proxy-platform-engineer
  role: platform engineer (simulated)
  date: "2026-10-01"
  minutes_spent: 40

A1:
  R1: "none missing for this env; the CRD-apply ordering step of the official
    upgrade guide is outside the report's scope but I knew to look for it"
  R2: "none"
  R3: "values-key collisions between the two discovered values files not shown"
  R4: "none"
A2: "dependency CVE bumps are invisible without the optional AI step (they sit
  in UNKNOWN); that is a gap, not a nicety — see D2"

B1:
  R1: "imp-7e9a11c2834d wording: 'requires 1.29-1.33' overstates; chart kubeVersion >= 1.22
    (the report's own imp-d55868f8741c says 1.28 satisfies it). Class right, reason conflated"
  R2: "none"
  R3: "none"
  R4: "none"
B2:
  R1: "no false action-required; imp-7e9a11c2834d survives triage as
    'unsupported combination, schedule cluster first' — wording fix, not reclassification"
  R3: "no false action-required"

C1:
  R1: { acceptable: "partially", noise_fraction: "60" }
  R2: { acceptable: "yes", noise_fraction: "55" }
  R3: { acceptable: "partially", noise_fraction: "60" }
  R4: { acceptable: "partially", noise_fraction: "55" }
C2: "collapse — one line per distinct missing-evidence reason, expandable; 50
  near-identical blocks is where I stop reading"

D1:
  R1: "yes - UNKNOWNs state the missing input, none overreach"
  R2: "yes"
  R3: "yes - plus the warnings section is the best part of repo mode"
  R4: "yes - notes marked undetermined stay undetermined"
D2: "mostly - suggestions never promote to verdicts and rejections are shown;
  overreach: none observed. But see A2: security triage leaning on an
  optional layer is a design smell even when the layer behaves"

E1:
  - { report: R1, finding: imp-7e9a11c2834d, evidence_id: ev-94a8abc4e147,
      uri: "github.com/cert-manager/website/.../releases/README.md", locator: L306, held_up: yes }
  - { report: R1, finding: imp-123e6594ecd5, evidence_id: ev-87e79b56b7a6,
      uri: "github.com/cert-manager/cert-manager/releases/download/v1.17.0/cert-manager.yaml",
      locator: L13047, held_up: yes }
  - { report: R1, finding: imp-6cae808def98, evidence_id: ev-717ddb6f40cd,
      uri: "github.com/cert-manager/cert-manager/blob/v1.17.0/deploy/charts/cert-manager/values.yaml",
      locator: "file-level", held_up: "yes (weak locator)" }
E2:
  R1: "mostly - had to open report.json to resolve evidence ids; report.txt
    shows ids only. Works, costs a step"
  R2: "same"
  R3: "yes - sha256s in the header made matching the packet inputs trivial"
  R4: "yes"

F1: { minutes_saved_or_no: "30-40 per edge after trust is established; first
  run closer to 10-15 because I verified everything by hand" }
F2: "only if the ACTION REQUIRED wording is fixed and evidence ids render as
  clickable/inline in the text output"
F3:
  R1: [imp-123e6594ecd5, imp-a137dded3ed6, imp-6cae808def98]
  R2: [imp-6cae808def98]
  R3: [imp-123e6594ecd5, imp-a137dded3ed6]
  R4: [imp-0dae408f0c17]
F4: "collapse the UNKNOWN block and render inline citations in text output"

G_R1:
  - { finding: imp-7e9a11c2834d, agree: "partial", corrected_class: "",
      comment: "class defensible; detail text must say 'outside supported range',
      not 'requires' (chart installs on >=1.22 per its own kubeVersion)" }
  - { finding: imp-123e6594ecd5, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-a137dded3ed6, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-0dae408f0c17, agree: yes, corrected_class: "",
      comment: "genuinely useful catch; new key going live under our feet is
      review-worthy, action-required would have been too strong" }
  - { finding: imp-6cae808def98, agree: yes, corrected_class: "",
      comment: "the informational case done right" }
G_R2:
  - { finding: imp-123e6594ecd5, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-a137dded3ed6, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-0dae408f0c17, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-e43eb9c2af2b, agree: yes, corrected_class: "",
      comment: "in-range informational is right; no noise complaint" }
  - { finding: imp-6cae808def98, agree: yes, corrected_class: "",
      comment: "same as R1; correctly NOT action-required" }
G_R3:
  - { finding: imp-7e9a11c2834d, agree: "partial", corrected_class: "",
      comment: "same wording caveat as R1" }
  - { finding: imp-123e6594ecd5, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-a137dded3ed6, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-0dae408f0c17, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-6cae808def98, agree: yes, corrected_class: "", comment: "" }
G_R3_discovery: "no false positives from the vendored chart or templates; missed
  nothing I'd call load-bearing; wants values-collision report"
G_R4_suggestions:
  - { finding: imp-eef2b0d0d72f, worth_look: yes, comment: "breaking + we use HTTP01; best suggestion in the set" }
  - { finding: imp-80754ea19305, worth_look: maybe, comment: "" }
  - { finding: imp-16163701185e, worth_look: yes, comment: "label-based tooling could care" }
  - { finding: imp-6432f469b85a, worth_look: yes, comment: "dashboards" }
  - { finding: imp-2577b5434545, worth_look: no, comment: "additive alias, not upgrade-relevant" }
  - { finding: imp-62fb5ea91b14, worth_look: yes, comment: "overlaps deterministic finding imp-0dae408f0c17; consistent" }
  - { finding: imp-d09df14ed530, worth_look: maybe, comment: "" }
  - { finding: imp-fa8df652362e, worth_look: yes, comment: "security triage, belongs in the deterministic set" }
  - { finding: imp-50009880b265, worth_look: yes, comment: "" }
  - { finding: imp-82d81560bbc8, worth_look: yes, comment: "" }
  - { finding: imp-88e0a1b18609, worth_look: yes, comment: "" }
  - { finding: imp-4abadd43d378, worth_look: no, comment: "internal fork, no operator surface stated" }
  - { finding: imp-d1ff8cfc0c11, worth_look: no, comment: "opt-in feature, no default change" }
G_R4_notes: "none wrong; the 'probably not applicable' Pebble and Vault notes
  are correctly hedged"

H1: "report.txt citing evidence by id only forces the JSON round-trip; state
  that in the README sooner (it does, but as step 4 of a list)"
H2: yes
```

## Scored per SCORING.md

| Metric | Value (this reviewer, n=1) |
|---|---|
| False-action count (B2) | **0** of 2 ACTION REQUIRED findings shown (wording caveat recorded on `imp-7e9a11c2834d`, survives triage) |
| Adjudication agreement (G, 15 rows) | 13 agree, 2 partial (both the same k8s-range wording issue) → **93%** (partial = 0.5) |
| Citation survival (E1) | **3/3** (1 of 3 with a locator-precision complaint; 0 failures) |
| UNKNOWN honesty (D1) | 1.0 (honest, all four reports) |
| Suggestion precision proxy (R4, 13) | 9 yes / 2 no / 1 maybe → **69% "worth a look"** |
| Adoption intent (F2) | conditional yes |
| Self-reported time saved (F1) | 30–40 min/edge after trust; 10–15 min on first run |

## Verdict

The packet is reviewable in the promised time and the reports survive a
hostile reading: zero false actions, citations hold, the one wording problem
("requires" vs "unsupported") is real but bounded. The design issue I'd
escalate before any real-engineer session: **security-relevant changes must
not depend on the optional AI layer** to leave the UNKNOWN bucket.
