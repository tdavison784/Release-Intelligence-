# Investigation

Upgrade: {{.Product}} {{.From}} → {{.To}}
Environment: {{.Environment}}{{if .KubernetesVersion}} (Kubernetes {{.KubernetesVersion}}){{end}}
Bundle: {{.Bundle}} · snapshot {{.Snapshot}}

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

1. For each in-scope item, call `get_change_item` and read the upstream text. Decide where it takes
   effect and which explicit setting controls it.
2. Examine the environment's rendered changes for the item (`query_diff`, `get_rendered_object`) and
   the environment's own files (`search_manifest`, `read_artifact`). For `controller-behaviour` and
   `unknown` items, render silence proves nothing: find the setting that pins the old behaviour, or cite
   that the environment does not set it.
3. Explain the uncorrelated rendered changes if you can: which change item, if any, they belong to.
4. Record what you find (`record_hypothesis`, `record_assertion`, with verbatim citations) and request
   the validations that would decide each item (`request_validation`; `request_evidence` for what is
   missing).
5. Finish with `submit_findings`: one closing summary per item you investigated.

Budget: {{.Budget}}.
