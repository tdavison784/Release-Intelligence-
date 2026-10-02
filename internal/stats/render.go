package stats

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

// Output formats of Render.
const (
	FormatText     = "text"
	FormatJSON     = "json"
	FormatMarkdown = "markdown"
)

// Render writes the report in the given format.
func Render(w io.Writer, rep *Report, format string) error {
	switch format {
	case FormatText, "":
		return renderText(w, rep)
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(rep)
	case FormatMarkdown, "md":
		return renderMarkdown(w, rep)
	default:
		return fmt.Errorf("unknown output format %q (want text, json or markdown)", format)
	}
}

// table is a small grid that renders as aligned text or as a markdown table.
type table struct {
	head []string
	rows [][]string
}

func (t *table) add(cells ...string) { t.rows = append(t.rows, cells) }

func (t *table) text(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(t.head, "\t"))
	for _, r := range t.rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()
}

func (t *table) markdown(w io.Writer) {
	esc := func(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
	row := func(cells []string) {
		out := make([]string, len(cells))
		for i, c := range cells {
			out[i] = esc(c)
		}
		fmt.Fprintf(w, "| %s |\n", strings.Join(out, " | "))
	}
	row(t.head)
	sep := make([]string, len(t.head))
	for i := range sep {
		sep[i] = "---"
	}
	fmt.Fprintf(w, "| %s |\n", strings.Join(sep, " | "))
	for _, r := range t.rows {
		row(r)
	}
}

// ---- cell formatting -------------------------------------------------------

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func orderCell(o int) string {
	if o == 0 {
		return "-"
	}
	return itoa(o)
}

func waveCell(w *int) string {
	if w == nil {
		return "-"
	}
	return itoa(*w)
}

func minutesCell(m *float64) string {
	if m == nil {
		return "n/a"
	}
	if *m == float64(int64(*m)) {
		return fmt.Sprintf("%d", int64(*m))
	}
	return fmt.Sprintf("%.1f", *m)
}

func pctCell(p float64) string {
	if p < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", p)
}

func reusePct(reused, used int) float64 {
	if used == 0 {
		return -1
	}
	return 100 * float64(reused) / float64(used)
}

func intPtrCell(p *int) string {
	if p == nil {
		return "n/a"
	}
	return itoa(*p)
}

func (r ProductRow) newCell() string {
	if !r.HasRecord {
		return "n/a"
	}
	return itoa(r.New)
}

func (r ProductRow) defOr(n int) string {
	if !r.HasDefinition {
		return "-"
	}
	return itoa(n)
}

func discoveryCell(d DiscoveryShare) string {
	if d.Total() == 0 {
		return "n/a"
	}
	s := fmt.Sprintf("%d/%d/%d (%s)", d.Discovered, d.DiscoveredModified, d.Manual+d.Other, pctCell(d.Percent()))
	if d.Unclassified > 0 {
		s += fmt.Sprintf(" +%d unclassified", d.Unclassified)
	}
	return s
}

func checkCell(c *CheckSummary) string {
	if c == nil {
		return "no report"
	}
	return fmt.Sprintf("%d/%d/%d/%d", c.Validated, c.Failing, c.Insufficient, c.Unverifiable)
}

func outcomesCell(c *CheckSummary) string {
	if c == nil {
		return "-"
	}
	return fmt.Sprintf("%d/%d/%d/%d/%d", c.Outcomes["pass"], c.Outcomes["fail"], c.Outcomes["unverifiable"], c.Outcomes["not-applicable"], c.Outcomes["covered"])
}

func failuresCell(r ProductRow) string {
	if r.InitialFailures == nil && r.FinalFailures == nil {
		return "n/a"
	}
	return intPtrCell(r.InitialFailures) + " → " + intPtrCell(r.FinalFailures)
}

func goCell(gen, spec int) string { return fmt.Sprintf("%d / %d", gen, spec) }

// ---- tables ----------------------------------------------------------------

func sizeTable(rep *Report) *table {
	t := &table{head: []string{"Order", "Wave", "Product", "YAML lines (code)", "Sources", "Artifacts", "Channels", "Contents", "Classify rules", "Template exprs", "Exceptions", "Optional artifacts", "Fallback groups", "Availability constraints"}}
	for _, r := range rep.Products {
		lines := "-"
		if r.HasDefinition {
			lines = fmt.Sprintf("%d (%d)", r.YAMLLines, r.YAMLCodeLines)
		}
		t.add(orderCell(r.Order), waveCell(r.Wave), r.Product, lines,
			r.defOr(r.Sources), r.defOr(r.Artifacts), r.defOr(r.Channels), r.defOr(r.Contents),
			r.defOr(r.ClassifyRules), r.defOr(r.TemplateExprs), r.defOr(r.Exceptions),
			r.defOr(r.OptionalArtifacts), r.defOr(r.FallbackGroups), r.defOr(r.AvailabilityConstraints))
	}
	return t
}

func effortTable(rep *Report) *table {
	t := &table{head: []string{"Order", "Product", "Used", "New", "Reused", "Reuse", "Cumulative", "Go generic / specific", "Minutes", "Representability"}}
	for _, r := range rep.Products {
		repr := r.Representability
		if repr == "" {
			repr = "n/a"
		}
		gos := "n/a"
		if r.HasRecord {
			gos = goCell(r.GoGeneric, r.GoProductSpecific)
		}
		reused, reuse := "n/a", "n/a"
		if r.HasRecord && r.HasDefinition {
			reused, reuse = itoa(r.Reused), pctCell(reusePct(r.Reused, r.Used))
		}
		t.add(orderCell(r.Order), r.Product, r.defOr(r.Used), r.newCell(), reused,
			reuse, itoa(r.Cumulative), gos, minutesCell(r.Minutes), repr)
	}
	return t
}

func qualityTable(rep *Report) *table {
	t := &table{head: []string{"Product", "Discovered / modified / manual (share found)", "Validated / failing / insufficient / unverifiable", "Checks pass/fail/unverif./n.a./covered", "Failures initial → final", "Manual interventions", "Unreachable sources"}}
	for _, r := range rep.Products {
		unreach := "n/a"
		if r.HasRecord {
			unreach = itoa(r.UnreachableSources)
		}
		interv := "n/a"
		if r.HasRecord {
			interv = itoa(r.ManualInterventions)
		}
		t.add(r.Product, discoveryCell(r.Discovery), checkCell(r.Check), outcomesCell(r.Check), failuresCell(r), interv, unreach)
	}
	return t
}

func curveTable(rep *Report) *table {
	t := &table{head: []string{"Order", "Product", "New (records)", "Cumulative", "Used", "Reused", "Reuse", "First use (computed)", "Cumulative used (computed)"}}
	for _, c := range rep.Curve {
		reused, reuse := "n/a", "n/a"
		if c.HasRecord {
			reused, reuse = itoa(c.Reused), pctCell(reusePct(c.Reused, c.Used))
		}
		t.add(orderCell(c.Order), c.Product, newOrNA(c), itoa(c.Cumulative), itoa(c.Used), reused,
			reuse, itoa(c.FirstUse), itoa(c.CumulativeUsed))
	}
	return t
}

func newOrNA(c CurvePoint) string {
	if !c.HasRecord {
		return "n/a"
	}
	return itoa(c.New)
}

func waveLabel(w *int) string {
	if w == nil {
		return "unassigned"
	}
	return itoa(*w)
}

func waveTable(rep *Report) *table {
	t := &table{head: []string{"Wave", "Products", "Zero-new products", "New constructs (per product)", "Reuse", "Go generic / specific", "Median minutes", "Discovered / modified / manual (share found)", "Validated / failing / insufficient / unverifiable", "Unreachable sources", "Representability"}}
	for _, w := range rep.Waves {
		zero := fmt.Sprintf("%d of %d", w.ZeroNew, len(w.Products))
		if len(w.ZeroNewProducts) > 0 {
			zero += " (" + strings.Join(w.ZeroNewProducts, ", ") + ")"
		}
		med := minutesCell(w.MedianMinutes)
		if w.MedianMinutes != nil {
			med += fmt.Sprintf(" (n=%d)", w.MinutesKnown)
		}
		val := "no reports"
		if w.Reports > 0 {
			val = fmt.Sprintf("%d/%d/%d/%d (%d of %d reports)", w.Validated, w.Failing, w.Insufficient, w.Unverifiable, w.Reports, len(w.Products))
		}
		t.add(waveLabel(w.Wave), strings.Join(w.Products, ", "), zero,
			fmt.Sprintf("%d (%.1f)", w.NewConstructs, w.NewPerProduct), pctCell(reusePct(w.Reused, w.Used)),
			goCell(w.GoGeneric, w.GoProductSpecific), med, discoveryCell(w.Discovery), val,
			itoa(w.UnreachableSources), reprSummary(w.Representability))
	}
	return t
}

func reprSummary(m map[string]int) string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}

// bars draws one horizontal bar per curve point, scaled to width.
func bars(rep *Report, width int, value func(CurvePoint) int, glyph string) string {
	max := 0
	for _, c := range rep.Curve {
		if v := value(c); v > max {
			max = v
		}
	}
	nameW := 0
	for _, c := range rep.Curve {
		if len(c.Product) > nameW {
			nameW = len(c.Product)
		}
	}
	var b strings.Builder
	for _, c := range rep.Curve {
		v := value(c)
		n := 0
		if max > 0 {
			n = (v*width + max - 1) / max // round up: a non-zero value always shows
		}
		if v == 0 {
			n = 0
		}
		o := orderCell(c.Order)
		fmt.Fprintf(&b, "%3s  %-*s  %4d  %s\n", o, nameW, c.Product, v, strings.Repeat(glyph, n))
	}
	return b.String()
}

func newBars(rep *Report) string {
	return bars(rep, 40, func(c CurvePoint) int {
		if !c.HasRecord {
			return 0
		}
		return c.New
	}, "█")
}

func cumulativeBars(rep *Report) string {
	return bars(rep, 40, func(c CurvePoint) int { return c.Cumulative }, "▒")
}

// groupConstructs groups construct names by their kind prefix.
func groupConstructs(names []string) (kinds []string, byKind map[string][]string) {
	byKind = map[string][]string{}
	for _, n := range names {
		kind, rest := "other", n
		if i := strings.Index(n, ":"); i > 0 {
			kind, rest = n[:i], n[i+1:]
		}
		if _, ok := byKind[kind]; !ok {
			kinds = append(kinds, kind)
		}
		byKind[kind] = append(byKind[kind], rest)
	}
	sort.Strings(kinds)
	return kinds, byKind
}

// ---- text ------------------------------------------------------------------

func renderText(w io.Writer, rep *Report) error {
	s := rep.Summary
	fmt.Fprintf(w, "Onboarding stats: %d definitions, %d records, %d check reports; %d distinct constructs used, %d introduced by records\n",
		s.Definitions, s.Records, s.CheckReports, s.DistinctUsed, s.DistinctIntroduced)
	if len(rep.Products) == 0 {
		fmt.Fprintln(w, "\nno products")
	}
	if len(rep.Products) > 0 {
		fmt.Fprintln(w, "\nSIZE AND COMPLEXITY")
		sizeTable(rep).text(w)
		fmt.Fprintln(w, "\nCONSTRUCTS AND EFFORT   (used: computed from the definition; new: from the record; reuse: used constructs an earlier product introduced)")
		effortTable(rep).text(w)
		fmt.Fprintln(w, "\nDISCOVERY AND RELATIONSHIPS")
		qualityTable(rep).text(w)
		fmt.Fprintln(w, "\nCONSTRUCT-INTRODUCTION CURVE")
		curveTable(rep).text(w)
		fmt.Fprintln(w, "\nnew constructs per product (records):")
		io.WriteString(w, newBars(rep))
		fmt.Fprintln(w, "\ncumulative constructs introduced:")
		io.WriteString(w, cumulativeBars(rep))
	}
	if len(rep.Waves) > 0 {
		fmt.Fprintln(w, "\nPER-WAVE AGGREGATES")
		waveTable(rep).text(w)
	}
	if len(rep.Warnings) > 0 {
		fmt.Fprintln(w, "\nDATA QUALITY")
		for _, m := range rep.Warnings {
			fmt.Fprintln(w, "  - "+m)
		}
	}
	return nil
}

// ---- markdown --------------------------------------------------------------

const markdownHeader = `# Onboarding scalability

> Generated by ` + "`ri stats -o markdown > docs/ONBOARDING.md`" + `. Do not edit by hand: regenerate it whenever a product lands.
> Inputs: ` + "`products/*.yaml`" + ` (definitions), ` + "`docs/onboarding/records/*.yaml`" + ` (one record per product, format in [onboarding/PROTOCOL.md](onboarding/PROTOCOL.md)) and ` + "`docs/onboarding/checks/*.json`" + ` (saved ` + "`ri check <id> -o json`" + ` reports).

## The question

Release Intelligence only becomes a generalised knowledge layer for third-party software if **adding the next product is increasingly configuration and data, not software development**. Configuration means a new ` + "`products/<id>.yaml`" + ` that the existing generic code interprets. Software development means a new *construct*: a new catalog field, locator kind, extract type or version relation, with the validator, schema and ingestion code behind it.

If every product needs new constructs, effort grows with the product count and the approach does not scale. If the number of new constructs per product falls towards zero while the definitions keep growing, it does. This report measures which of the two is happening, product by product, in onboarding order (the order and wave come from each product's record; the plan is in [phase2/PLAN.md](phase2/PLAN.md)).

What convergence looks like:

- new constructs per product trend to zero, and products that introduce none become the norm;
- the share of a definition's constructs that already existed (reuse) approaches 100%;
- generic Go changes per product fall, and product-specific Go changes stay at exactly 0;
- the time to onboard falls and the share of sources and artifacts found by automatic discovery rises;
- relationships keep validating against history, and the unverifiable ones are only those behind unreachable hosts.

## Metrics

**Definition size and complexity** (computed from each ` + "`products/<id>.yaml`" + `)

- *YAML lines (code)*: physical lines of the definition, and lines that are neither blank nor comment-only.
- *Sources*, *Artifacts*: elements declared. *Channels*: artifact publication locations. *Contents*: structured snapshots to diff (Helm values, CRDs, image references, chart metadata).
- *Classify rules*: product-specific classification rules on sources. *Template exprs*: ` + "`{{ ... }}`" + ` expressions in the definition (descriptions and notes excluded).
- *Exceptions*: curated releases where a relationship is known not to hold, each with a reason. *Optional artifacts*: artifacts not published for every release. *Fallback groups*: distinct named sets of alternative sources. *Availability constraints*: version constraints on sources, artifacts and contents.

**Constructs**

- A *construct* is one capability of the definition format. The inventory is computed from the parsed definition by reflection, so constructs added to the catalog later are counted without editing any list. Names carry a kind prefix: ` + "`field:sources.fallbackGroup`" + ` (a catalog field that is set; list elements do not appear in the path), ` + "`locator:git-log`" + `, ` + "`extract:markdown-table`" + `, ` + "`version:lookup`" + `, ` + "`content:helm-values`" + `, ` + "`artifact:…`" + `, ` + "`role:…`" + ` and other enumerated values, ` + "`template-var:PrevTag`" + ` and ` + "`template-func:regexQuote`" + `. Free-text fields (id, name, description, notes, provenance, validatedAgainst) are not constructs.
- *Used*: constructs the definition uses (computed).
- *New*: constructs the product's record lists under ` + "`constructs.new`" + `. This is authoritative: it says which constructs the product made necessary, whether or not the code already existed. A construct is credited to the first product, in onboarding order, that needed it.
- *Reused*: used constructs that an earlier product introduced. *Reuse* is Reused / Used.
- *Cumulative*: distinct constructs introduced up to and including the product. The curve below plots *New* and *Cumulative*.
- *First use (computed)* and *Cumulative used (computed)*: the same curve derived from the definitions alone (a construct is first used by the first product in order whose definition uses it). It needs no records, so it also covers products without one.
- *Zero-new product*: has a record and introduced nothing.
- To name constructs in a record, run ` + "`ri stats <id> -o json`" + `: ` + "`usedConstructs`" + ` lists every construct of the definition and ` + "`unaccountedConstructs`" + ` the ones no record has introduced yet, which are the candidates for ` + "`constructs.new`" + `.

**Effort**

- *Go generic / specific*: Go changes recorded for the product: generic ones (a file and its purpose) and product-specific ones. Product-specific Go code is prohibited, so that number must stay 0.
- *Minutes*: wall-clock agent time to onboard, from the record; ` + "`n/a`" + ` when not measured. The wave aggregate is the median of the known values.

**Discovery**

- *Discovered / modified / manual*: how many of the final definition's sources and artifacts were proposed by ` + "`ri discover`" + ` and kept (discovered), proposed but corrected by hand (modified), or found by research (manual), from the record. *Share found* is (discovered + modified) / all. Elements of the definition that the record does not classify are shown as unclassified.

**Relationships** (from the saved ` + "`ri check`" + ` report)

- *Validated / failing / insufficient / unverifiable*: number of sources, artifacts and contents whose relationship to the release held in at least 3 historical releases and never failed (validated), failed at least once (failing), had too few passes (insufficient), or could not be verified for at least one release because a host was unreachable (unverifiable; independent of the other three).
- *Checks pass/fail/unverif./n.a./covered*: the individual checks by outcome. *Failures initial → final*: failing subjects of the first complete draft and of the final definition, from the record. *Manual interventions*: definition edits made because history disagreed.
- *Unreachable sources*: hosts a record lists as blocked or unreachable from where the product was onboarded.
- *Representability*: whether the definition model can express the product's release topology (full, partial or poor), from the record; gaps are listed there.

**Wave aggregates** cover the products that have a record, grouped by the wave in their record: how many needed zero new constructs, the total and mean of new constructs, reuse pooled over the wave (sum of reused / sum of used), generic and product-specific Go changes, the median of the known minutes (with the number of values), the pooled discovery share, the sums of the relationship counts over the products with a saved report, the unreachable sources, and the representability counts.

**Limits.** A construct is a unit of capability, not of effort: a catalog field is cheaper than an adapter, so read the construct curve together with the Go changes and minutes. The first product necessarily introduces the base set that every definition uses (identity, versioning, sources, artifacts, channels), which also inflates early reuse. Records are written by whoever onboarded the product; the report cross-checks them against the definitions and lists the inconsistencies under *Data quality*.
`

func renderMarkdown(w io.Writer, rep *Report) error {
	io.WriteString(w, markdownHeader)
	s := rep.Summary

	fmt.Fprintln(w, "\n## Summary")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- %d product definitions, %d onboarding records, %d saved relationship reports.\n", s.Definitions, s.Records, s.CheckReports)
	fmt.Fprintf(w, "- %d distinct constructs are used across all definitions; the records introduce %d.\n", s.DistinctUsed, s.DistinctIntroduced)
	if s.Records > 0 {
		fmt.Fprintf(w, "- %d of %d recorded products introduced no new construct.", s.ZeroNewProducts, s.Records)
		if s.LastNewOrder > 0 {
			fmt.Fprintf(w, " The latest product that introduced one has order %d.", s.LastNewOrder)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "- Product-specific Go changes recorded: %d (must be 0).\n", s.GoProductSpecific)

	if len(rep.Products) == 0 {
		fmt.Fprintln(w, "\nNo products yet.")
		return nil
	}

	fmt.Fprintln(w, "\n## Per-product table")
	fmt.Fprintln(w, "\n### Size and complexity")
	fmt.Fprintln(w)
	sizeTable(rep).markdown(w)
	fmt.Fprintln(w, "\n### Constructs and effort")
	fmt.Fprintln(w)
	effortTable(rep).markdown(w)
	fmt.Fprintln(w, "\n### Discovery and relationship validation")
	fmt.Fprintln(w)
	qualityTable(rep).markdown(w)

	fmt.Fprintln(w, "\n## Construct-introduction curve")
	fmt.Fprintln(w)
	curveTable(rep).markdown(w)
	fmt.Fprintln(w, "\nNew constructs per product, in onboarding order (records):")
	fmt.Fprintln(w, "\n```text")
	io.WriteString(w, newBars(rep))
	fmt.Fprintln(w, "```")
	fmt.Fprintln(w, "\nCumulative constructs introduced:")
	fmt.Fprintln(w, "\n```text")
	io.WriteString(w, cumulativeBars(rep))
	fmt.Fprintln(w, "```")

	fmt.Fprintln(w, "\n## Constructs introduced per product")
	haveRecords := false
	for _, r := range rep.Products {
		if !r.HasRecord {
			continue
		}
		haveRecords = true
		fmt.Fprintf(w, "\n### %s (order %s, wave %s): %d new\n\n", r.Product, orderCell(r.Order), waveCell(r.Wave), r.New)
		if r.New == 0 {
			fmt.Fprintln(w, "No new constructs: the definition is configuration only.")
			continue
		}
		var plain []string
		for _, n := range r.NewList {
			if d, ok := r.NewDetails[n]; ok && (d.Justification != "" || len(d.OtherProducts) > 0) {
				line := "- `" + n + "`"
				if d.Justification != "" {
					line += ": " + strings.TrimSpace(d.Justification)
				}
				if len(d.OtherProducts) > 0 {
					line += " (also needed by " + strings.Join(d.OtherProducts, ", ") + ")"
				}
				fmt.Fprintln(w, line)
				continue
			}
			plain = append(plain, n)
		}
		kinds, byKind := groupConstructs(plain)
		for _, k := range kinds {
			names := byKind[k]
			quoted := make([]string, len(names))
			for i, n := range names {
				quoted[i] = "`" + n + "`"
			}
			fmt.Fprintf(w, "- %s (%d): %s\n", k, len(names), strings.Join(quoted, ", "))
		}
	}
	if !haveRecords {
		fmt.Fprintln(w, "\nNo records yet.")
	}

	if len(rep.Waves) > 0 {
		fmt.Fprintln(w, "\n## Per-wave aggregates")
		fmt.Fprintln(w)
		waveTable(rep).markdown(w)
	}

	var notes []ProductRow
	for _, r := range rep.Products {
		if r.Notes != "" {
			notes = append(notes, r)
		}
	}
	if len(notes) > 0 {
		fmt.Fprintln(w, "\n## Notes from the records")
		for _, r := range notes {
			fmt.Fprintf(w, "\n**%s**: %s\n", r.Product, strings.Join(strings.Fields(r.Notes), " "))
		}
	}

	if len(rep.Warnings) > 0 {
		fmt.Fprintln(w, "\n## Data quality")
		fmt.Fprintln(w, "\nObservations about the inputs (missing, inconsistent or later-adopted data). The report is computed regardless.")
		fmt.Fprintln(w)
		for _, m := range rep.Warnings {
			fmt.Fprintln(w, "- "+m)
		}
	}
	io.WriteString(w, markdownCorrections)
	return nil
}

// markdownCorrections is the dated correction log of this generated report.
// It is static text in the generator, not hand edits to docs/ONBOARDING.md,
// so it survives regeneration. Append new entries; never rewrite old ones.
const markdownCorrections = `
## Corrections

Phase 1 and most of Phase 2 ran in a network-restricted sandbox. The statements
below were true of that run only; the original text above and in the records is
kept as the historical record.

**Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):**

- *Introduction ("the unverifiable ones are only those behind unreachable hosts")
  and the "Unreachable sources" / "unverifiable" columns*: recorded from the
  sandbox. Live, unverifiable checks fell to 0 for cert-manager (12), ingress-nginx
  (47) and postgresql (15); remaining unverifiable subjects are falco (a genuine
  digest mismatch: falco-9.0.0.tgz was re-published after its index entry) and
  minio (anonymous pulls of minio/minio are denied). karpenter's HTTP 429s were
  throttling, not unavailability, and its checks all validate on the sequential
  re-run after the throttling fixes. The saved check reports for argo-cd
  (v3.4.1 in place of the never-published v3.4.0 release), cert-manager,
  crossplane, external-secrets, ingress-nginx, karpenter, postgresql,
  redis, terraform-provider-aws, traefik, vault, istio, strimzi and
  kube-prometheus-stack — 14 products — were replaced by the live runs, so
  the validated / unverifiable counts above reflect them.
- *` + "`locator:helm-git`" + ` (vault): "helm.releases.hashicorp.com is blocked from the
  sandbox"*: it answers (366 chart versions) and the helm-repo channel declared
  first validates; helm-git remains as the fallback. Likewise the notes that call
  charts.crossplane.io, charts.external-secrets.io, kubernetes.github.io,
  charts.jetstack.io, quay.io or api.github.com blocked.
- *` + "`field:sources.locator.url`" + ` (postgresql): "the host is blocked"*:
  www.postgresql.org answers; the security page could be read directly.
- *Argo CD "v3.4.0 exception"*: v3.4.0 is a git tag that was never a GitHub
  release; the definition's exception wording was corrected.
- *ingress-nginx*: the live run found that ` + "`controller-chroot:v1.10.0`" + ` was never
  published (exception added to the definition).
`
