# Product onboarding protocol

Onboarding is measured, so every product follows the same steps and leaves the
same record. The point is not the product count: it is to find out whether the
definition model can represent each product's release topology, and how much
of onboarding is configuration rather than software development.

## Steps

1. **Start the clock.** `date -u +%FT%TZ` → `onboarding.startedAt`.
2. **Automatic discovery first (before any manual research).**
   ```
   go build -o $BIN ./cmd/ri
   $BIN -state $STATE discover <repository> -id <id> \
       -out docs/onboarding/discovery/<id>.proposed.yaml \
       -report docs/onboarding/discovery/<id>.report.md
   ```
   No LLM (there is no API key in the sandbox). Keep both files: they are the
   baseline for the "discovered automatically" metrics and for measuring later
   discovery improvements.
3. **Research the release topology.** Clone or browse the repository (partial
   clones are cheap) and find: the canonical version channel and tag
   convention, release notes / changelog, upgrade guide, compatibility matrix,
   security channel, and every published artifact (images, charts, CRDs,
   manifests, binaries, packages) with its version relationship to the
   release. Write a short channel map to `docs/research/<id>.md` (same shape as
   the existing ones). Note which hosts are unreachable from the sandbox.
4. **Author `products/<id>.yaml`.** Start from the proposal where it is right.
   Declare the canonical channel first and reachable alternatives after it.
   Prefer existing constructs; read `docs/ARCHITECTURE.md` and the three
   existing definitions first.
5. **Validate statically:** `ri validate` and
   `go test ./internal/catalog/` (checks every definition against the JSON
   Schema and the Go validator).
6. **Validate historically:** `ri check <id>` (6 releases by default; use
   `-n` for more). Record the failing subjects of the *first* run of your first
   complete draft (`initialFailures`). Fix definitions, not data: a failure is
   either a wrong assumption (fix the relationship) or an upstream anomaly
   (`exceptions` with a reason). Save the final run:
   `ri check <id> -o json > docs/onboarding/checks/<id>.json`.
7. **Exercise an upgrade edge** across a recent minor (or major) release:
   `ri upgrade <id> A B` and `-o json`. Read the output critically: garbage
   notes, misclassified items, missing breaking changes, noise. Tune
   `classify` rules. Record the summary line in `sampleEdge`.
8. **Generic constructs.** If the topology cannot be represented, decide
   whether a generic construct is justified: it must have value beyond this
   product (name the other products, onboarded or well known, that need it).
   If justified, implement it generically (catalog field + JSON Schema +
   validator + ingest/normalize + unit tests) and record it under
   `constructs.new`. If not, record the gap under `gaps` and represent what
   you can. **Never** add product-specific Go code.
9. **Stop the clock** → `onboarding.finishedAt`, `minutes`.
10. **Write `docs/onboarding/records/<id>.yaml`** (format below) and run
    `go test ./...`.

## Record format (`docs/onboarding/records/<id>.yaml`)

```yaml
product: cilium                 # definition id
order: 4                        # onboarding order (see docs/phase2/PLAN.md)
wave: 1
repository: github.com/cilium/cilium
onboarding:
  startedAt: "2026-10-01T12:00:00Z"
  finishedAt: "2026-10-01T13:40:00Z"
  minutes: 100                  # wall-clock, agent time
discovery:
  proposal: docs/onboarding/discovery/cilium.proposed.yaml
  report: docs/onboarding/discovery/cilium.report.md
# Every source and artifact of the FINAL definition, with where it came from:
#   discovered           – proposed by `ri discover` and kept essentially as proposed
#   discovered-modified  – proposed, but the locator/version relation had to be corrected
#   manual               – not proposed; found by research
sources:
  - {id: git-tags, origin: discovered}
  - {id: upgrade-guide, origin: manual, note: "rst file under Documentation/"}
artifacts:
  - {id: agent-image, origin: discovered-modified, note: "quay.io → also docker.io"}
discoveryRejected:              # proposals that were wrong or useless
  - {id: docs-index, reason: "not release-specific"}
relationships:
  historicalReleasesChecked: [1.15.0, 1.16.0, 1.16.12, 1.17.0, 1.17.6, 1.18.1]
  initialFailures: 3            # failing subjects on the first complete draft
  finalFailures: 0
  manualInterventions:          # definition edits made because history disagreed
    - {subject: cli-archive, change: "availability >= 1.15.0", reason: "asset renamed in 1.15"}
  unverifiable:
    - {subject: agent-image, reason: "quay.io blocked by sandbox egress policy"}
constructs:
  # names as printed by `ri stats <id> -o json` (usedConstructs /
  # unaccountedConstructs), e.g. field:sources.fallbackGroup, extract:yaml-records
  new: []                       # [{name, kind, justification, otherProducts: [...]}]
  considered: []                # constructs considered and rejected, with reason
goChanges:
  productSpecific: 0            # must be 0
  generic: []                   # [{file, purpose}]
unreachableSources:
  - {host: helm.cilium.io, reason: "egress policy 403"}
ambiguousRelationships:
  - "chart version == app version, but charts are only published for some patch releases?"
representability: full          # full | partial | poor
gaps: []                        # what the definition cannot express (and why it matters)
sampleEdge:
  from: 1.17.6
  to: 1.18.1
  summary: "4 breaking · 2 action required · 85 changes · 1 warnings"
  evidenceRecords: 120
notes: ""
```

Computed metrics (definition size and complexity, constructs used, validated
relationships, evidence coverage) are derived from the definition and the
saved check report by `ri stats`; do not copy them into the record by hand.
