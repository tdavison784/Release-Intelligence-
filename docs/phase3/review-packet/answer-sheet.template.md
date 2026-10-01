# Answer sheet (copy this file, fill it in, return it)

Reviewer: _<name or handle>_
Role: _<platform engineer / SRE / other>_
Date: _<yyyy-mm-dd>_
Time spent on the packet (minutes): _

Instructions: every field accepts one line. `n/a` and `don't know` are valid.
The question ids match `questionnaire.md`.

```yaml
meta:
  reviewer: ""
  role: ""
  date: ""
  minutes_spent: ""

# A. Completeness — missing: none | <finding/change description + where you'd look>
A1:
  R1: ""
  R2: ""
  R3: ""
  R4: ""
A2: ""

# B. Correctness — wrong claims: none | <finding id: what is wrong>
B1:
  R1: ""
  R2: ""
  R3: ""
  R4: ""
# falsely ACTION REQUIRED: none | <finding id + one line why not>
B2:
  R1: ""
  R3: ""

# C. Noise — acceptable: yes/no/partially; noise_fraction: <rough %>
C1:
  R1: { acceptable: "", noise_fraction: "" }
  R2: { acceptable: "", noise_fraction: "" }
  R3: { acceptable: "", noise_fraction: "" }
  R4: { acceptable: "", noise_fraction: "" }
# UNKNOWN bucket volume: acceptable | collapse | hide | <other>
C2: ""

# D. Uncertainty — honest: yes/mostly/no + one line
D1:
  R1: ""
  R2: ""
  R3: ""
  R4: ""
# AI layer stays in its lane: yes/mostly/no + one line
D2: ""

# E. Evidence — one row per spot-checked citation
E1:
  - { report: "", finding: "", evidence_id: "", uri: "", locator: "", held_up: "" }
  - { report: "", finding: "", evidence_id: "", uri: "", locator: "", held_up: "" }
  - { report: "", finding: "", evidence_id: "", uri: "", locator: "", held_up: "" }
# evidence followable: yes/mostly/no + what blocked you
E2:
  R1: ""
  R2: ""
  R3: ""
  R4: ""

# F. Value
F1: { minutes_saved_or_no: "" }
F2: ""   # yes | no | only if <condition>
F3:
  R1: []
  R2: []
  R3: []
  R4: []
F4: ""

# G. Per-finding adjudication
# agree: yes | no; corrected_class: "" (only if no); comment: one line
G_R1:
  - { finding: "imp-7e9a11c2834d", agree: "", corrected_class: "", comment: "" }
  - { finding: "imp-123e6594ecd5", agree: "", corrected_class: "", comment: "" }
  - { finding: "imp-a137dded3ed6", agree: "", corrected_class: "", comment: "" }
  - { finding: "imp-0dae408f0c17", agree: "", corrected_class: "", comment: "" }
  - { finding: "imp-6cae808def98", agree: "", corrected_class: "", comment: "" }
G_R2:
  - { finding: "imp-123e6594ecd5", agree: "", corrected_class: "", comment: "" }
  - { finding: "imp-a137dded3ed6", agree: "", corrected_class: "", comment: "" }
  - { finding: "imp-0dae408f0c17", agree: "", corrected_class: "", comment: "" }
  - { finding: "imp-e43eb9c2af2b", agree: "", corrected_class: "", comment: "" }
  - { finding: "imp-6cae808def98", agree: "", corrected_class: "", comment: "" }
G_R3:
  - { finding: "imp-7e9a11c2834d", agree: "", corrected_class: "", comment: "" }
  - { finding: "imp-123e6594ecd5", agree: "", corrected_class: "", comment: "" }
  - { finding: "imp-a137dded3ed6", agree: "", corrected_class: "", comment: "" }
  - { finding: "imp-0dae408f0c17", agree: "", corrected_class: "", comment: "" }
  - { finding: "imp-6cae808def98", agree: "", corrected_class: "", comment: "" }
G_R3_discovery: ""   # picked up anything it shouldn't / missed anything?
G_R4_suggestions:
  # worth_look: yes | no | maybe
  - { finding: "imp-eef2b0d0d72f", worth_look: "", comment: "" }
  - { finding: "imp-80754ea19305", worth_look: "", comment: "" }
  - { finding: "imp-16163701185e", worth_look: "", comment: "" }
  - { finding: "imp-6432f469b85a", worth_look: "", comment: "" }
  - { finding: "imp-2577b5434545", worth_look: "", comment: "" }
  - { finding: "imp-62fb5ea91b14", worth_look: "", comment: "" }
  - { finding: "imp-d09df14ed530", worth_look: "", comment: "" }
  - { finding: "imp-fa8df652362e", worth_look: "", comment: "" }
  - { finding: "imp-50009880b265", worth_look: "", comment: "" }
  - { finding: "imp-82d81560bbc8", worth_look: "", comment: "" }
  - { finding: "imp-88e0a1b18609", worth_look: "", comment: "" }
  - { finding: "imp-4abadd43d378", worth_look: "", comment: "" }
  - { finding: "imp-d1ff8cfc0c11", worth_look: "", comment: "" }
G_R4_notes: ""       # any of the 5 non-suggesting notes wrong or overconfident?

# H. Wrap-up
H1: ""
H2: ""   # yes | no
```
