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
3. If the open question is whether a consumer depends on the changed property, trace the consumers with the
   tools (what references the object that changes or what it produces, and what each reads from it) and
   record what you find as claims and assertions.
4. If an upstream source states a condition under which an exposed object is not impacted, record the
   mitigation (`record_hypothesis` with `mitigation`) whether the environment meets it on all, some or none
   of the exposed objects: scope it to the objects where the change has a consequence a consumer can
   observe, and list in `environmentBasis` the verified evidence of the environment's state with respect to
   its conditions (the claims or objects that meet it, and those that do not).
5. End with `submit_findings` holding exactly one entry: this item, with its disposition and your `verdict`
   (class, mechanism, environment reasoning, verified facts vs assumptions, mitigations, uncertainty, proposed
   validations, and what would overturn it). Every upstream-supported, environment-checkable condition in
   `wouldOverturn` is a recorded mitigation named in `mitigations`, or carries a one-line reason it cannot be
   expressed in the fact grammar.

Budget for this session: {{.Budget}}.
