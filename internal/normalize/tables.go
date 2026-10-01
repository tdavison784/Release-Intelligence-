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
		if sel.KeyRe != nil && !sel.KeyRe.MatchString(stripMarkdown(key)) {
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
