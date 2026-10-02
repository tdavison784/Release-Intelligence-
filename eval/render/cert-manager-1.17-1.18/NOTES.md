# NOTES — cert-manager-1.17-1.18 (render case)

## How this was researched

Source material, in the order read (2026-10-01):

1. `git diff v1.17.0 v1.18.0 -- deploy/charts/cert-manager` on
   github.com/cert-manager/cert-manager (partial clone, both tags fetched). 11 files:
   rbac.yaml, webhook-deployment.yaml, servicemonitor.yaml, deployment.yaml,
   cainjector-deployment.yaml, serviceaccount.yaml, startupapicheck-job.yaml,
   NOTES.txt, values.yaml, values.schema.json, README.template.md.
2. `git diff v1.17.0 v1.18.0 -- deploy/crds` (5 CRDs changed).
3. https://cert-manager.io/docs/releases/release-notes/release-notes-1.18
   (rotationPolicy default change; disableHTTPChallengesRole context).

## Judgement calls

- **R6–R12 (RBAC):** the template diff renames `-controller-challenges` to
  `-http01-controller-challenges` and adds a `-dns01-controller-challenges` role with
  its rules spelled out; the http01 role additionally gains
  `challenges/finalizers update`. Object identity is by name, so the rename is
  remove+add of ClusterRole and ClusterRoleBinding (R6–R11) plus one permission add
  (R12). Authored from the template names/rules directly.
- **R13/R14 (probe ports):** the diff collapses the `healthzPort`-or-6080 branch to
  the named port `healthcheck` for BOTH probes; the port *number* behind the name is
  template context, so only the field change is asserted, not a numeric value.
- **R15 (targetPort default):** values.yaml diff `targetPort: 9402` →
  `targetPort: http-metrics`. The Service (always rendered) carries it, so the
  expectation is on the Service; the variant additionally covers the ServiceMonitor.
- **R16 (CRD field):** from the crd-certificates.yaml diff (new signatureAlgorithm
  property with enum). Marked "only if CRDs are rendered" because the chart packages
  CRDs under `crds/` and renderer inclusion of that directory is a renderer property,
  not an upstream fact — the comparison run decides it.
- **R17 (not-render-verifiable):** release notes say the default changed from Never to
  Always; the CRD at both tags carries no schema `default:` for rotationPolicy (only
  description prose), and the notes document the
  `DefaultPrivateKeyRotationPolicyAlways` controller feature gate. So the semantic
  default must NOT be claimable from a render — an R12 boundary case.
- **No env-var or API-version case:** the template diff between these two tags
  contains no env var change and no apiVersion change of rendered objects or served
  CRD versions. Rather than invent one, this is recorded here; a future case on an
  edge that has such changes should cover those categories.
- **v1.18.2 reverted `disableHTTPChallengesRole`** (release notes). This case pins
  v1.18.0 exactly; the revert does not affect the v1.18.0 chart, but the gate
  existing only in 1.18.0/1.18.1 is why the http01/dns01 split appears here.

## Comparison learnings (recorded before results were committed)

The first comparison run surfaced three expectation-granularity mistakes and one
real modeling fact. All four are recorded here; the fixes were made before any
results file was committed.

1. **R5/R15:** upstream sources name the arg flag (`--acme-http01-solver-image`) and
   the values key (`targetPort`); the diff encodes them in the change's subject name
   and before/after values, not its path. The runner's path filter now also looks
   there. No expectation content changed.
2. **R12 (dropped):** originally asserted class `rbac-permission-added` for the
   http01 role's new `challenges/finalizers update` permission. Two comparison runs
   showed why it cannot match: the diff models a wholly-new ClusterRole as ONE
   `resource-added` change (per-permission decomposition happens only for roles
   present on both sides), and a `resource-added` change carries only the object
   identity — no field content — so the permission is not visible in the change
   record at all. Both are defensible modeling choices, not renderer bugs; the
   permission IS visible in the rendered object (the inventory/validator layer reads
   it). R12 was therefore dropped as a change-stream expectation (redundant with R7)
   and this note is its record.
3. **V1:** originally filtered on path `targetPort` — the values key, which never
   appears in the rendered ServiceMonitor (the rendered field is `endpoints[].port`).
   Re-scoped to `endpoints`, the field that actually carries the changed default.
4. **R16 (known gap, still open):** the Certificate CRD's new `signatureAlgorithm`
   field did not appear in the rendered delta even though the renderer passes
   `--include-crds`. Marked `knownGap` at authoring time; the results file records
   what the run showed.

## Blind-authoring caveat (honest disclosure)

Authoring was **not fully blind** in two ways, both recorded rather than hidden:

1. The lane's status file (written before this case, by the lane's previous agent)
   already summarizes a live render of this same edge in prose (14 changes, the RBAC
   split, the probe ports, the targetPort default, image bumps).
2. While verifying that the live renderer path works at all, one `ri render diff`
   run of this edge was executed before the case file was written; its output was
   seen.

Mitigations: every expectation was re-derived from the upstream diffs above, and the
matchers deliberately stay at upstream granularity — object names as the templates
spell them, field areas (`livenessProbe`, `targetPort`, `signatureAlgorithm`), diff
classes — without copying any renderer path spelling (e.g. no
`spec.ports[name=tcp-prometheus-servicemonitor]`, no change-class wording from the
tool). R16's purpose (measure the tool, not mirror it) is served by that discipline,
and the caveat is here so a reviewer can weigh it.
