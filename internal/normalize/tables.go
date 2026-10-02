package normalize

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// mdTable is a GFM pipe table found in a document.
type mdTable struct {
	headers   []string // markdown stripped
	headerRaw string   // verbatim header line
	rows      []mdRow
	headings  []heading // heading path above the table, outermost first
}

type mdRow struct {
	cells []string // raw cell text, trimmed, padded to the header count
	line  int      // 1-based line number
	raw   string   // verbatim line
}

type heading struct {
	level int
	text  string // normalised
	raw   string // without the leading/trailing '#'
}

// scanTables finds every pipe table outside fenced code and front matter.
func scanTables(md []byte) []mdTable {
	d := scanDocument(md)
	var (
		out   []mdTable
		stack []heading
		n     = len(d.lines)
	)
	for i := 0; i < n; i++ {
		l := d.lines[i]
		if l.kind == lkHeading {
			for len(stack) > 0 && stack[len(stack)-1].level >= l.level {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, heading{level: l.level, text: l.head, raw: l.hraw})
			continue
		}
		if l.kind != lkText || !d.tableStart(i, n) {
			continue
		}
		hdrCells := splitTableRow(l.vis)
		t := mdTable{headerRaw: l.raw, headings: append([]heading(nil), stack...)}
		for _, c := range hdrCells {
			t.headers = append(t.headers, stripMarkdown(c))
		}
		j := i + 2
		for j < n && d.lines[j].kind == lkText && strings.TrimSpace(d.lines[j].vis) != "" && strings.Contains(d.lines[j].vis, "|") {
			cells := splitTableRow(d.lines[j].vis)
			row := make([]string, len(t.headers))
			copy(row, cells)
			t.rows = append(t.rows, mdRow{cells: row, line: d.lines[j].n, raw: d.lines[j].raw})
			j++
		}
		out = append(out, t)
		i = j - 1
	}
	return out
}

func (t mdTable) underHeading(re *regexp.Regexp) bool {
	for _, h := range t.headings {
		if re.MatchString(h.text) || re.MatchString(h.raw) {
			return true
		}
	}
	return false
}

func columnIndex(headers []string, names []string) int {
	for _, name := range names {
		want := strings.ToLower(stripMarkdown(name))
		for i, h := range headers {
			if strings.ToLower(h) == want {
				return i
			}
		}
	}
	return -1
}

func extractTableRow(md []byte, sel TableSelector) (*TableRow, error) {
	if len(sel.KeyColumns) == 0 {
		return nil, ErrNoMatch
	}
	for _, t := range scanTables(md) {
		if sel.TableHeading != nil && !t.underHeading(sel.TableHeading) {
			continue
		}
		idx := columnIndex(t.headers, sel.KeyColumns)
		if idx < 0 {
			continue
		}
		if len(sel.ValueColumns) > 0 && columnIndex(t.headers, sel.ValueColumns) < 0 {
			continue
		}
		for _, r := range t.rows {
			key := stripMarkdown(r.cells[idx])
			if sel.KeyRe != nil && !sel.KeyRe.MatchString(key) {
				continue
			}
			row := &TableRow{
				Headers: append([]string(nil), t.headers...),
				Cells:   map[string]string{},
				Line:    r.line,
				Excerpt: strings.TrimSpace(t.headerRaw) + "\n" + strings.TrimSpace(r.raw),
			}
			for i, h := range t.headers {
				if _, dup := row.Cells[h]; !dup {
					row.Cells[h] = r.cells[i]
				}
			}
			return row, nil
		}
	}
	return nil, ErrNoMatch
}

// ---- YAML / JSON records ----------------------------------------------------

func extractRecord(content []byte, sel TableSelector) (*TableRow, error) {
	if sel.Collect {
		return collectRecords(content, sel)
	}
	if len(sel.KeyColumns) == 0 {
		return nil, ErrNoMatch
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("normalize: parse records: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, ErrNoMatch
	}
	for _, rec := range recordNodes(doc.Content[0]) {
		var headers []string
		cells := map[string]string{}
		for i := 0; i+1 < len(rec.Content); i += 2 {
			k := rec.Content[i].Value
			headers = append(headers, k)
			if _, dup := cells[k]; !dup {
				cells[k] = yamlValueString(rec.Content[i+1])
			}
		}
		key, ok := "", false
		for _, kc := range sel.KeyColumns {
			for _, h := range headers {
				if strings.EqualFold(h, strings.TrimSpace(kc)) {
					key, ok = cells[h], true
					break
				}
			}
			if ok {
				break
			}
		}
		if !ok {
			continue
		}
		if len(sel.ValueColumns) > 0 && columnIndex(headers, sel.ValueColumns) < 0 {
			continue
		}
		if sel.KeyRe != nil && !sel.KeyRe.MatchString(stripMarkdown(key)) {
			continue
		}
		if !recordPasses(headers, cells, sel.Where) {
			continue
		}
		excerpt, _ := yaml.Marshal(rec)
		return &TableRow{
			Headers: headers,
			Cells:   cells,
			Line:    rec.Line,
			Excerpt: strings.TrimSpace(string(excerpt)),
		}, nil
	}
	return nil, ErrNoMatch
}

// recordPasses reports whether every Where field of the record matches.
func recordPasses(headers []string, cells map[string]string, where map[string]*regexp.Regexp) bool {
	for field, re := range where {
		matched := false
		for _, h := range headers {
			if strings.EqualFold(h, field) {
				matched = re.MatchString(cells[h])
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// collectRecords merges every record passing sel.Where into one row: each
// header's cell is the distinct non-empty values of the passing records,
// joined with ", " in document order. Only the ValueColumns headers are
// merged (and quoted in the excerpt), so unrelated bulky fields (checksums)
// stay out of the evidence. ErrNoMatch when no record qualifies.
func collectRecords(content []byte, sel TableSelector) (*TableRow, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("normalize: parse records: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, ErrNoMatch
	}
	var (
		headers []string
		order   = map[string][]string{}
		seen    = map[string]map[string]bool{}
		first   int
		n       int
	)
	for _, rec := range recordNodes(doc.Content[0]) {
		var hs []string
		cells := map[string]string{}
		for i := 0; i+1 < len(rec.Content); i += 2 {
			k := rec.Content[i].Value
			hs = append(hs, k)
			if _, dup := cells[k]; !dup {
				cells[k] = yamlValueString(rec.Content[i+1])
			}
		}
		if !recordPasses(hs, cells, sel.Where) {
			continue
		}
		if n == 0 {
			first = rec.Line
		}
		n++
		for _, h := range hs {
			if len(sel.ValueColumns) > 0 && columnIndex([]string{h}, sel.ValueColumns) < 0 {
				continue
			}
			v := strings.TrimSpace(cells[h])
			if v == "" {
				continue
			}
			if seen[h] == nil {
				seen[h] = map[string]bool{}
				headers = append(headers, h)
			}
			if !seen[h][v] {
				seen[h][v] = true
				order[h] = append(order[h], v)
			}
		}
	}
	if n == 0 || len(headers) == 0 {
		return nil, ErrNoMatch
	}
	row := &TableRow{Headers: headers, Cells: map[string]string{}, Line: first}
	var lines []string
	for _, h := range headers {
		row.Cells[h] = strings.Join(order[h], ", ")
		lines = append(lines, h+": "+row.Cells[h])
	}
	row.Excerpt = fmt.Sprintf("%d records", n)
	for f := range sel.Where {
		row.Excerpt += " where " + f + " matches"
		break
	}
	row.Excerpt += "\n" + strings.Join(lines, "\n")
	return row, nil
}

func resolveAlias(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

// recordNodes returns the mapping nodes of a records document: a sequence of
// mappings, or a mapping whose first list-valued field holds them.
func recordNodes(root *yaml.Node) []*yaml.Node {
	root = resolveAlias(root)
	if root == nil {
		return nil
	}
	collect := func(seq *yaml.Node) []*yaml.Node {
		var out []*yaml.Node
		for _, c := range seq.Content {
			if c = resolveAlias(c); c != nil && c.Kind == yaml.MappingNode {
				out = append(out, c)
			}
		}
		return out
	}
	switch root.Kind {
	case yaml.SequenceNode:
		return collect(root)
	case yaml.MappingNode:
		for i := 0; i+1 < len(root.Content); i += 2 {
			if v := resolveAlias(root.Content[i+1]); v != nil && v.Kind == yaml.SequenceNode {
				if recs := collect(v); len(recs) > 0 {
					return recs
				}
			}
		}
	}
	return nil
}

// yamlValueString renders a YAML value for a record cell: scalars verbatim
// (so 1.30 stays "1.30"), lists of scalars joined with ", ".
func yamlValueString(n *yaml.Node) string {
	n = resolveAlias(n)
	if n == nil {
		return ""
	}
	switch n.Kind {
	case yaml.ScalarNode:
		if n.Tag == "!!null" {
			return ""
		}
		return n.Value
	case yaml.SequenceNode:
		parts := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			if s := yamlValueString(c); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case yaml.MappingNode:
		parts := make([]string, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			parts = append(parts, n.Content[i].Value+": "+yamlValueString(n.Content[i+1]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return ""
}

// ---- compatibility ------------------------------------------------------------

func compatibilityFromRow(in DocInput, row *TableRow, columns []catalog.ColumnSpec) ([]domain.CompatibilityConstraint, []domain.Evidence) {
	if row == nil {
		return nil, nil
	}
	var (
		out []domain.CompatibilityConstraint
		ev  *domain.Evidence
	)
	for _, col := range columns {
		header := findHeader(row, col.Headers)
		if header == "" {
			continue
		}
		cell := strings.TrimSpace(row.Cells[header])
		if col.Separator != "" {
			parts := strings.Split(cell, col.Separator)
			if col.Part < 0 || col.Part >= len(parts) {
				continue
			}
			cell = strings.TrimSpace(parts[col.Part])
		}
		if cell == "" {
			continue
		}
		cell = reduceVersions(cell, col.Reduce)
		if ev == nil {
			kind := domain.EvidenceDocument
			if first, _, _ := strings.Cut(row.Excerpt, "\n"); !strings.Contains(first, "|") {
				kind = domain.EvidenceStructured
			}
			e := domain.NewEvidence(kind, in.SourceID, in.URI, fmt.Sprintf("L%d", row.Line+in.LineOffset), row.Excerpt, in.Digest, in.RetrievedAt)
			ev = &e
		}
		kind := col.Kind
		if kind == "" {
			kind = "supported"
		}
		cc := domain.CompatibilityConstraint{
			Platform: col.Platform,
			Raw:      cell,
			Kind:     kind,
			SourceID: in.SourceID,
			Provenance: domain.Provenance{
				Method:     domain.MethodDeclared,
				Producer:   ProducerTable,
				Rule:       "column:" + header,
				Confidence: domain.ConfidenceHigh,
			},
			Evidence: []domain.EvidenceID{ev.ID},
		}
		if c, vs, err := ParseVersionRange(stripMarkdown(cell)); err == nil {
			cc.Constraint, cc.Versions = c, vs
		}
		out = append(out, cc)
	}
	if ev == nil {
		return out, nil
	}
	return out, []domain.Evidence{*ev}
}

// findHeader returns the row header matching the first name present
// (case-insensitive, markdown stripped).
func findHeader(row *TableRow, names []string) string {
	for _, name := range names {
		want := strings.ToLower(stripMarkdown(name))
		for _, h := range row.Headers {
			if strings.ToLower(h) == want {
				return h
			}
		}
	}
	return ""
}

// reduceVersions coarsens each ", "-separated version of a cell to its
// "major" or "minor" line, dropping duplicates (order kept). Items that do not
// start with a numeric major[.minor] are kept untouched.
func reduceVersions(cell, reduce string) string {
	if reduce == "" {
		return cell
	}
	var out []string
	seen := map[string]bool{}
	for _, item := range strings.Split(cell, ",") {
		item = strings.TrimSpace(item)
		if m := reduceRe.FindStringSubmatch(item); m != nil {
			if reduce == "major" || m[2] == "" {
				item = m[1]
			} else {
				item = m[1] + "." + m[2]
			}
		}
		if item != "" && !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return strings.Join(out, ", ")
}

var reduceRe = regexp.MustCompile(`^v?(\d+)(?:\.(\d+))?(?:\.\d+)?(?:[-+].*)?$`)
