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

## Renders
{{range .Renders}}
- {{.}}{{else}}
- none: no render was produced for this environment (an evidence gap){{end}}

## Your job in this session

1. `get_change_item {{.Item.ID}}` and read the upstream text: where does it take effect, which explicit
   setting controls it?
2. Retrieve this item's own evidence: its rendered changes and objects (`query_diff changeItem={{.Item.ID}}`,
   `get_rendered_object`), the environment files that would set the deciding setting (`search_manifest`,
   `read_artifact`), its relations (`expand_graph`, `list_paths`).
3. State what the environment sets or does not set as verified claims (`record_claim`), record what you find
   with verbatim citations, and request the validation that would decide it
   (`request_validation`, naming the subjects and, in `answers`, the concern it should answer).
4. End with `submit_findings` holding exactly one entry: this item, with its disposition.

Budget for this session: {{.Budget}}.
