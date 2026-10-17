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
deciding setting on every exposed object. Gaps: what is missing, partial or contradictory.

```json
{{.Dossier}}
```

## Renders
{{range .Renders}}
- {{.}}{{else}}
- none: no render was produced for this environment (an evidence gap){{end}}

## Your job in this session

1. Read the dossier above. Work through the questions of the system prompt for this item; spend tool calls only
   on what the dossier does not settle (the exact upstream semantics in its sources with `read_artifact`, a
   specific object or file, a gap you can close).
2. State what the environment sets or does not set as verified claims (`record_claim`), record what you find
   with verbatim citations, and request the validation that would decide it
   (`request_validation`, naming the subjects and, in `answers`, the concern it should answer).
3. If an upstream source states that a setting or condition preserves the old behaviour and the environment
   meets it, record the mitigation (`record_hypothesis` with `mitigation`), scoped to the objects it is about.
4. End with `submit_findings` holding exactly one entry: this item, with its disposition and your `verdict`
   (class, mechanism, environment reasoning, verified facts vs assumptions, mitigations, uncertainty, proposed
   validations, and what would overturn it).

Budget for this session: {{.Budget}}.
