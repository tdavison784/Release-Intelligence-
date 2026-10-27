# Investigation

Upgrade: {{.Product}} {{.From}} → {{.To}}
Environment: {{.Environment}}{{if .KubernetesVersion}} (Kubernetes {{.KubernetesVersion}}){{end}}
Bundle: {{.Bundle}} · snapshot {{.Snapshot}}

## Agenda ({{len .Agenda}} items, in priority order)

Close every one of these in `submit_findings` (raises-concern, claims-mitigation, explains,
examined-no-concern or not-examined). Items not listed were already decided by a failing deterministic
validation; you may still comment on them.
{{range .Agenda}}
{{.}}{{else}}
(none: every in-scope item was decided by the deterministic validations){{end}}

## Change items in scope ({{len .Items}})
{{range .Items}}
### {{.ID}} — {{.Title}}

- family `{{.Family}}`, manifestation `{{.Manifestation}}` ({{.ManifestationRule}}), scope {{.ScopeRule}}{{if .Flags}}; {{.Flags}}{{end}}
{{- if .Subjects}}
- subjects: {{.Subjects}}
{{- end}}
- upstream evidence:{{range .Citations}}
  - {{.}}{{else}} none resolved{{end}}
- environment rendered changes correlated to it: {{.CorrelatedCount}}{{range .Correlated}}
  - {{.}}{{end}}{{if .CorrelatedMore}}
  - … {{.CorrelatedMore}} more (query_diff changeItem={{.ID}}){{end}}
- exposure roots: {{.Roots}}; impact paths: {{.Paths}} ({{.UnresolvedPaths}} unresolved)
- deterministic validations already run:{{range .Validations}}
  - {{.}}{{else}} none{{end}}
{{- if .JoinFindings}}
- deterministic join findings: {{.JoinFindings}}
{{- end}}
{{end}}
## Renders
{{range .Renders}}
- {{.}}{{else}}
- none: no render was produced for this environment (an evidence gap){{end}}

## Uncorrelated environment rendered changes ({{.UncorrelatedCount}})
{{range .Uncorrelated}}
- {{.}}{{else}}
- none{{end}}{{if .UncorrelatedMore}}
- … {{.UncorrelatedMore}} more (query_diff uncorrelated=true){{end}}

## Unresolved relationships ({{.UnresolvedCount}})
{{range .Unresolved}}
- {{.}}{{else}}
- none{{end}}{{if .UnresolvedMore}}
- … {{.UnresolvedMore}} more (list_paths unresolved=true){{end}}
{{if .OutOfScope}}
{{.OutOfScope}} further change items are out of scope (routine or not breaking); they are not classified.
{{end}}
## Your job

1. Work down the agenda. For each item, call `get_change_item` and read the upstream text: where does
   it take effect, and which explicit setting controls it?
2. Examine the environment's rendered changes for the item (`query_diff`, `get_rendered_object`) and
   the environment's own files (`search_manifest`, `read_artifact`). For `controller-behaviour` and
   `unknown` items, render silence proves nothing: find the setting that pins the old behaviour, or cite
   that the environment does not set it.
3. Record what you find (`record_hypothesis`, `record_assertion`, with verbatim citations) and request
   the validations that would decide the item (`request_validation`, naming subjects as the evidence
   does; `request_evidence` for what is missing).
4. Finish with `submit_findings`, closing every agenda item with a disposition.

Budget: {{.Budget}}.
