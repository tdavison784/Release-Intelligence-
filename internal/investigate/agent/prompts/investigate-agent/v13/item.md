# Investigation of one change item

Upgrade: {{.Product}} {{.From}} → {{.To}}
Environment: {{.Environment}}{{if .KubernetesVersion}} (Kubernetes {{.KubernetesVersion}}){{end}}
Bundle: {{.Bundle}}

This session is about ONE agenda item: item {{.Position}} of {{.Total}} examined in this investigation
(agenda rule {{.Rule}}; at most {{.Cap}} items get a session). Other items have their own sessions.

## {{.Item.ID}} — {{.Item.Title}}

- family `{{.Item.Family}}`, manifestation `{{.Item.Manifestation}}` ({{.Item.ManifestationRule}}), scope {{.Item.ScopeRule}}{{if .Item.Flags}}; {{.Item.Flags}}{{end}}
{{- if .Item.Subjects}}
- subjects: {{.Item.Subjects}}
{{- end}}
- upstream evidence:{{range .Item.Citations}}
  - {{.}}{{else}} none resolved{{end}}
- upstream source artifacts (read them with read_artifact):{{range .Sources}}
  - {{.}}{{else}} none{{end}}
- upstream source code readable with request_evidence (upstream-file, upstream-search): {{.Upstream}}
- environment rendered changes correlated to it: {{.Item.CorrelatedCount}}{{range .Item.Correlated}}
  - {{.}}{{end}}{{if .Item.CorrelatedMore}}
  - … {{.Item.CorrelatedMore}} more (query_diff changeItem={{.Item.ID}}){{end}}
- release-scope (chart-default) rendered changes for it:{{range .Release}}
  - {{.}}{{else}} none{{end}}
- exposure roots:{{range .RootNodes}}
  - {{.}}{{else}} none{{end}}
- impact paths:{{range .PathLines}}
  - {{.}}{{else}} none{{end}}
- deterministic validations already run:{{range .Item.Validations}}
  - {{.}}{{else}} none{{end}}
{{- if .Item.JoinFindings}}
- deterministic join findings: {{.Item.JoinFindings}}
{{- end}}

## Dossier ({{.DossierArtifact}})

Assembled deterministically from the bundle; your starting context. Settings: the effective state of each
deciding setting on every exposed object. Relations, consumers: the consumer chains of each
exposed object (trace_consumers gives every hop). Gaps: what is missing, partial or contradictory.

```json
{{.Dossier}}
```

## Renders
{{range .Renders}}
- {{.}}{{else}}
- none: no render was produced for this environment (an evidence gap){{end}}

## Your job in this session

1. Read the dossier above, including its consumer chains and gaps. Read the upstream text the item's sources
   hold for the exact semantics of the change (`read_artifact`).
2. Trace the consumers: `trace_consumers` with `changeItem` {{.Item.ID}}. Then read what each chain points to
   — the consumer's configuration at the cited line, the consumer object — and what it depends on.
3. Establish the effective configuration of every setting the change touches: explicit (state it with
   `record_claim`) or inherited from the default (and which default). Request the validation that would decide
   the item (`request_validation`, naming the subjects and, in `answers`, the concern it should answer).
4. Where the outcome depends on upstream behaviour (a validation, a default, parsing, a feature gate) the
   change text does not state exactly, fetch the implementation, documentation or tests at the target's tag
   (`request_evidence` kind `upstream-search` or `upstream-file`) and cite the lines; if it cannot be
   retrieved, keep it unknown. List each such behaviour in the assessment's `behaviour` with its source
   (`upstream`, `change-text`, `unavailable`, `not-checked`); a `not-affected` verdict cannot rest on one
   that is `unavailable` or `not-checked`.
5. Work through the eight questions of the system prompt. Keep verified (cited), inferred and unknown apart;
   what you remember is not evidence.
6. End with `submit_findings` holding exactly one entry: this item, its disposition, your `verdict` (say
   whether it is supported or provisional, and its proof obligations) and your `assessment` (what changed,
   mechanism, consumers, effective configuration, verified with citations, inferred, unknown, behaviour with
   sources, likely impact, recommended tests).

Budget for this session: {{.Budget}}.
