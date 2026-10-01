# PROXY REVIEW — SRE

> **PROXY REVIEW — performed by an AI agent simulating the persona; NOT human
> validation.** It exists to de-risk the questionnaire and the packet before
> real reviewers see them. The "reviewer" shares an origin with the packet's
> authors, so its judgments validate process (are questions answerable, do
> citations resolve, does scoring flow), not human usability or real-world
> value. Every number below must be re-measured with real engineers.

Reviewer: proxy-sre (AI-simulated)
Persona: SRE on an on-call rotation for a production fleet; cares about blast
radius, security exposure, alert surface, and not getting paged because an
upgrade note went unread; optimizes for "what can I safely skip".
Date: 2026-10-01 · Time spent: ~35 minutes · Blind: reports reviewed cold from
`report.txt` + `example-env/` inputs only; source code not read; citations
spot-checked by fetching them.

---

## How I reviewed

I read the reports looking for two things: (1) would this thing page me for
nothing, and (2) would it stay silent about something that pages me at 3am.
I read R1, R2, R3, R4 in that order, then checked three citations end-to-end,
then answered the questionnaire.

## Cold-read notes

**R1.** The thing I check first in any upgrade doc is the security section.
Here it does not exist as a section: five dependency/CVE bumps in this edge
are classified UNKNOWN ("not evaluated for this environment"), mixed in
alphabetically with feature notes about ACME profiles and Pebble test
tooling. The report tells me they exist — nothing is hidden — but the
presentation gives a CVE-2025-22868 the same visual weight as "Upgraded
Pebble to v2.7.0". For my role that is a false-negative trap: the correct
answer for the CVEs is not "UNKNOWN", it is "in the images you run after
upgrade — review before shipping". The offline warnings say advisory matching
was unavailable, so the tool knows it is missing inputs here; fine, but then
the security-adjacent rows need to be louder, not quieter.

What R1 gets right: the ACTION REQUIRED is the correct single item and the
"what will fail" question is answered (unsupported cluster). The two image
findings are mirror-work I already do; the values-key finding
(`imp-0dae408f0c17`) is the one I would have missed, and it is exactly the
kind of thing that produces a behavior change nobody planned. UNKNOWN items
are honest — each says what evidence was missing — I just want them collapsed.

**R2.** This is the report I judge hardest, because "you're safe" is the
claim that gets people paged when it's wrong. Both informational items hold
up: cluster 1.31 is inside the range; the pinned `targetPort` means the
default change genuinely does not reach this environment. The detail text
even tells me what to re-check at next edit. I tried to break the pinned-
default finding and could not: the citation points at my own values file line
(`targetPort: 9402`) and the upstream change id is the same one R1 shows. No
false ACTION REQUIRED, no false reassurance. This is the strongest report of
the four.

**R3.** Repo mode inventories my kind of repo correctly and — more important
— prints its own blind spots as warnings (vendored chart skipped, helmfile
literal, kustomize not applied, last-values-wins, valuesFrom unreadable).
Every one of those is a place where a silent tool would have lied to me. My
complaint is different: `--kubernetes 1.28` had to be passed by hand while
everything else was discovered. A cluster-version fact sitting in a README or
a flux label would remove the last manual input; until then the headline
finding in a repo-mode run depends on the operator remembering to type it.
The tool does say what it would need (UNKNOWN rows name it), so this is a
workflow gap, not an honesty gap.

**R4.** The AI layer pulls the five CVE bumps out of the UNKNOWN pile and
gives each a "review before completing the upgrade" suggestion with the
advisory id in the text. That is the most useful thing in any of the four
reports *for my role* — and it bothers me that it lives in the optional
layer: without `-enrich` (or offline without a cache), a security bump is one
undistinguished line among 50. Suggestions stay suggestions, notes that say
"probably not applicable" stay hedged, and two machine-rejected answers are
printed rather than dropped — all good. The `ciss` and annotation-flag
suggestions are triage noise for an upgrade review; fine, they're cheap.

One overreach candidate I checked: the x/crypto suggestion says the bump
"ships inside components this cluster deploys" — true (it's in the
controller image) but the leap from "in the image" to "review your exposure"
is the model being appropriately cautious, not the tool claiming impact. No
verdict was moved. OK.

## Citation spot-checks (3)

| Report | Finding | Evidence | What I did | Result |
|---|---|---|---|---|
| R4 | imp-fa8df652362e | ev-695a6d7c7c6e | Opened `release-notes-1.18.md` L350 | **Holds up.** "Bump `github.com/golang-jwt/jwt` to patch `GHSA-mh63-6h87-95cp`" verbatim |
| R4/R1 | imp-123e6594ecd5 | ev-f3c0b5db4aa8 | Environment citation: opened `example-env/report4-env/manifests/deployment.yaml` L12 (packet copy of the fixture path in the report) | **Holds up.** `image: quay.io/jetstack/cert-manager-controller:v1.17.0` at that line; sha256 in the report header matched my `shasum` of the packet copy |
| R4 | imp-2577b5434545 (env evidence for the CRD) | ev-c70daaf7f996 | Opened `example-env/report4-env/crds/certificates.yaml` L4 | **Survives with a caveat.** L4 starts the CRD object; the excerpt quotes `kind: CustomResourceDefinition` (L5) and the name (L7) — right object, right fields, but the line number points at the object start, not the quoted lines. I found it in seconds; a stricter reader would call the locator imprecise |

3/3 survive (1 with the locator caveat).

## Answer sheet (filled)

```yaml
meta:
  reviewer: proxy-sre
  role: SRE (simulated)
  date: "2026-10-01"
  minutes_spent: 35

A1:
  R1: "no security-advisory section; CVE bumps surfaced only as UNKNOWN rows
    (advisory source unavailable offline - the warning says so)"
  R2: "none"
  R3: "no cluster-version discovery (had to be passed via --kubernetes)"
  R4: "none beyond R1's"
A2: "security-relevant changes need their own bucket or at minimum their own
  section; depending on an optional AI step for them is the wrong dependency
  direction"

B1:
  R1: "none factually wrong; wording of imp-7e9a11c2834d ('requires') vs the
    report's own not-affected kubeVersion row (>=1.22) is a presentational
    contradiction worth fixing"
  R2: "none"
  R3: "none"
  R4: "none; rejected model answers shown, not hidden"
B2:
  R1: "no false action required"
  R3: "no false action required"

C1:
  R1: { acceptable: "partially", noise_fraction: "65" }
  R2: { acceptable: "yes", noise_fraction: "50" }
  R3: { acceptable: "partially", noise_fraction: "60" }
  R4: { acceptable: "partially", noise_fraction: "50" }
C2: "collapse - group the 50 UNKNOWNs by their neededToDetermine reason; five
  groups, not fifty blocks"

D1:
  R1: "yes - neededToDetermine is the right field and it is filled honestly"
  R2: "yes"
  R3: "yes"
  R4: "yes"
D2: "mostly yes - suggestions never became verdicts; my criticism is
  architectural (security triage in an optional layer), not behavioral"

E1:
  - { report: R4, finding: imp-fa8df652362e, evidence_id: ev-695a6d7c7c6e,
      uri: "github.com/cert-manager/website/.../release-notes-1.18.md", locator: L350, held_up: yes }
  - { report: R4, finding: imp-123e6594ecd5, evidence_id: ev-f3c0b5db4aa8,
      uri: "example-env/report4-env/manifests/deployment.yaml (packet copy)", locator: L12, held_up: yes }
  - { report: R4, finding: imp-2577b5434545, evidence_id: ev-c70daaf7f996,
      uri: "example-env/report4-env/crds/certificates.yaml (packet copy)", locator: L4, held_up: "yes (node-start locator, imprecise)" }
E2:
  R1: "mostly - ids force a JSON lookup; excerpts are not in the text report"
  R2: "mostly - same"
  R3: "yes - warnings section made the discovery contract clear"
  R4: "yes"

F1: { minutes_saved_or_no: "for this edge: 25-35 min; the savings concentrate
  in not reading 51 release-note bullets and the chart values diff myself" }
F2: "only if security findings stop living in the optional AI layer"
F3:
  R1: [imp-7e9a11c2834d]
  R2: [imp-e43eb9c2af2b]
  R3: [imp-7e9a11c2834d]
  R4: [imp-fa8df652362e, imp-50009880b265, imp-82d81560bbc8, imp-88e0a1b18609]
F4: "surface security bumps deterministically (own section), even when the
  advisory source is offline"

G_R1:
  - { finding: imp-7e9a11c2834d, agree: yes, corrected_class: "",
      comment: "untestable combo on my fleet = action required; wording caveat noted" }
  - { finding: imp-123e6594ecd5, agree: yes, corrected_class: "",
      comment: "review not action: the pin lives in our mirror job, nothing breaks silently" }
  - { finding: imp-a137dded3ed6, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-0dae408f0c17, agree: yes, corrected_class: "",
      comment: "would have been 'action-required' in a stricter tool; review is the right call" }
  - { finding: imp-6cae808def98, agree: yes, corrected_class: "",
      comment: "verified against my own values line; correct" }
G_R2:
  - { finding: imp-123e6594ecd5, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-a137dded3ed6, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-0dae408f0c17, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-e43eb9c2af2b, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-6cae808def98, agree: yes, corrected_class: "",
      comment: "the report I'd trust; 'applies to you, you appear safe' is earned here" }
G_R3:
  - { finding: imp-7e9a11c2834d, agree: yes, corrected_class: "",
      comment: "but the input was hand-typed; discovery should find it" }
  - { finding: imp-123e6594ecd5, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-a137dded3ed6, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-0dae408f0c17, agree: yes, corrected_class: "", comment: "" }
  - { finding: imp-6cae808def98, agree: yes, corrected_class: "", comment: "" }
G_R3_discovery: "nothing spurious; the skipped/readonly warnings are the
  honest part; cluster version should be discoverable"
G_R4_suggestions:
  - { finding: imp-eef2b0d0d72f, worth_look: yes, comment: "breaking + HTTP01 is our solver" }
  - { finding: imp-80754ea19305, worth_look: maybe, comment: "" }
  - { finding: imp-16163701185e, worth_look: yes, comment: "" }
  - { finding: imp-6432f469b85a, worth_look: yes, comment: "new metrics = new alert surface" }
  - { finding: imp-2577b5434545, worth_look: no, comment: "alias, zero blast radius" }
  - { finding: imp-62fb5ea91b14, worth_look: yes, comment: "" }
  - { finding: imp-d09df14ed530, worth_look: maybe, comment: "" }
  - { finding: imp-fa8df652362e, worth_look: yes, comment: "this is the point of the whole report" }
  - { finding: imp-50009880b265, worth_look: yes, comment: "" }
  - { finding: imp-82d81560bbc8, worth_look: yes, comment: "" }
  - { finding: imp-88e0a1b18609, worth_look: yes, comment: "" }
  - { finding: imp-4abadd43d378, worth_look: no, comment: "" }
  - { finding: imp-d1ff8cfc0c11, worth_look: no, comment: "" }
G_R4_notes: "hedged notes stayed hedged; nothing overconfident"

H1: "packet itself was fine; the txt-to-JSON evidence indirection is the one
  friction I'd remove"
H2: yes
```

## Scored per SCORING.md

| Metric | Value (this reviewer, n=1) |
|---|---|
| False-action count (B2) | **0** of 2 ACTION REQUIRED findings shown |
| Adjudication agreement (G, 15 rows) | **15/15 = 100%** (one comment-grade caveat on wording, no class disputes) |
| Citation survival (E1) | **3/3** (1 with the node-start-locator caveat; 0 failures) |
| UNKNOWN honesty (D1) | 1.0 (honest, all four reports) |
| Suggestion precision proxy (R4, 13) | 8 yes / 3 no / 2 maybe → **62% "worth a look"** |
| Adoption intent (F2) | conditional yes (condition: security out of the optional layer) |
| Self-reported time saved (F1) | 25–35 min for this edge |

## Verdict

No false actions, no broken citations, no dishonest uncertainty — the packet
survives the two failure modes I was hunting for. The escalation is
architectural, not clerical: **security-relevant changes must be visible
deterministically**, not via an optional AI pass, or the tool's worst-case
failure mode is a silent CVE. Both of my conditions for real use are
addressable without touching the classification contract.
