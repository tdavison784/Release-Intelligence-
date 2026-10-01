package normalize

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// parseReleaseNoteYAML implements ParseReleaseNoteYAML.
//
// Every file is one YAML document (apiVersion release-notes/v2):
//
//	kind: feature | bug-fix | security-fix | test | promotion | ...
//	area: traffic-management
//	issue: [1234]
//	releaseNotes:  ["**Fixed** ..."]
//	upgradeNotes:  [{title: ..., content: ...}]
//	securityNotes: ["..."]
//	docs:          ["[usage] https://..."]
//
// Keys are matched leniently (case, singular/plural and one-character typos
// such as "upgradeNodes"); unknown keys are ignored. Files that fail to parse
// are skipped and reported in the returned error together with the items of
// the files that did parse.
func parseReleaseNoteYAML(files []DocInput, rules []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error) {
	cls, err := newClassifier(rules)
	if err != nil {
		return nil, nil, err
	}
	var (
		items []domain.NoteItem
		evs   []domain.Evidence
		errs  []error
		seen  = map[string]int{}
	)
	for _, f := range files {
		if len(strings.TrimSpace(string(f.Content))) == 0 {
			continue
		}
		b := newNoteBuilder(f, cls)
		if err := parseOneNoteFile(b); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", f.URI, err))
			continue
		}
		// merge per-file results, de-duplicating IDs across files
		for _, it := range b.items {
			if idx, dup := seen[it.ID]; dup {
				ex := &items[idx]
				for _, e := range it.Evidence {
					has := false
					for _, x := range ex.Evidence {
						if x == e {
							has = true
						}
					}
					if !has {
						ex.Evidence = append(ex.Evidence, e)
					}
				}
				continue
			}
			seen[it.ID] = len(items)
			items = append(items, it)
		}
		evs = appendEvidenceUnique(evs, b.evidence)
	}
	return items, evs, errors.Join(errs...)
}

func appendEvidenceUnique(dst, src []domain.Evidence) []domain.Evidence {
	have := map[domain.EvidenceID]bool{}
	for _, e := range dst {
		have[e.ID] = true
	}
	for _, e := range src {
		if !have[e.ID] {
			have[e.ID] = true
			dst = append(dst, e)
		}
	}
	return dst
}

// canonical key names (singular stems, lower case, letters only)
var noteKeyStems = map[string]string{
	"kind":         "kind",
	"area":         "area",
	"issue":        "issue",
	"releasenote":  "releaseNotes",
	"upgradenote":  "upgradeNotes",
	"securitynote": "securityNotes",
}

// canonicalNoteKey maps a (possibly misspelled) key to its canonical name or "".
func canonicalNoteKey(k string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(k) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	s = strings.TrimSuffix(s, "s")
	if c, ok := noteKeyStems[s]; ok {
		return c
	}
	if len(s) >= 9 { // tolerate one-character typos in the long keys
		for _, stem := range []string{"releasenote", "upgradenote", "securitynote"} {
			if editDistanceAtMost1(s, stem) {
				return noteKeyStems[stem]
			}
		}
	}
	return ""
}

func editDistanceAtMost1(a, b string) bool {
	if a == b {
		return true
	}
	la, lb := len(a), len(b)
	if la < lb {
		a, b, la, lb = b, a, lb, la
	}
	if la-lb > 1 {
		return false
	}
	i, j, diff := 0, 0, 0
	for i < la && j < lb {
		if a[i] == b[j] {
			i++
			j++
			continue
		}
		diff++
		if diff > 1 {
			return false
		}
		if la == lb {
			j++
		}
		i++
	}
	return diff+(la-i)+(lb-j) <= 1
}

type noteList struct {
	name string
	node *yaml.Node
}

func parseOneNoteFile(b *noteBuilder) error {
	in := b.in
	var doc yaml.Node
	if err := yaml.Unmarshal(in.Content, &doc); err != nil {
		return err
	}
	if len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return errors.New("release note file is not a YAML mapping")
	}
	var (
		kind, area string
		issues     []string
		lists      []noteList // in document order
	)
	for i := 0; i+1 < len(root.Content); i += 2 {
		switch canonicalNoteKey(root.Content[i].Value) {
		case "kind":
			kind = strings.TrimSpace(root.Content[i+1].Value)
		case "area":
			area = strings.TrimSpace(root.Content[i+1].Value)
		case "issue":
			issues = scalarList(root.Content[i+1])
		case "releaseNotes", "upgradeNotes", "securityNotes":
			lists = append(lists, noteList{canonicalNoteKey(root.Content[i].Value), root.Content[i+1]})
		}
	}
	if k := yamlKindKey(kind); k == "test" || k == "tests" {
		return nil
	}

	var issueRefs []domain.Reference
	for _, is := range issues {
		issueRefs = mergeReferences(issueRefs, issueReferences(is, in.Repository))
	}

	base := path.Base(uriPath(in.URI))
	var pathElems []string
	if area != "" {
		pathElems = []string{area}
	}
	for _, list := range lists {
		listName, n := list.name, list.node
		entries := n.Content
		if n.Kind == yaml.ScalarNode {
			entries = []*yaml.Node{n}
		}
		for _, e := range entries {
			title, body := noteEntry(e)
			if title == "" && body == "" {
				continue
			}
			var text, raw, label string
			switch {
			case title != "":
				flat, flatRaw, _ := flattenMarkdown(body)
				text = joinHeading(title, flat)
				raw = collapseSpace(title + " " + flatRaw)
				label = title
			default:
				text, raw, label = flattenMarkdown(body)
			}
			text = truncateText(cleanInline(text), maxItemText)
			start := e.Line
			end := lastNodeLine(e)
			locator := fmt.Sprintf("%s L%d-L%d", base, start+in.LineOffset, end+in.LineOffset)
			ci := classInput{yamlKind: kind, yamlList: listName}
			b.addResolved(pathElems, text, raw, label, locator, verbatimLines(in.Content, start, end), ci, issueRefs)
		}
	}
	return nil
}

// issueReferences turns one "issue" entry into references: a bare number is
// an issue of the document's repository, anything else (URLs, "owner/repo#N")
// goes through ExtractReferences.
func issueReferences(s, repository string) []domain.Reference {
	s = strings.TrimSpace(s)
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		repo := normalizeRepo(repository)
		if repo == "" {
			return nil
		}
		return []domain.Reference{{Type: "issue", ID: repo + "#" + s, URL: "https://github.com/" + repo + "/issues/" + s}}
	}
	return ExtractReferences(s, repository)
}

func scalarList(n *yaml.Node) []string {
	if n.Kind == yaml.ScalarNode {
		if n.Value == "" {
			return nil
		}
		return []string{n.Value}
	}
	var out []string
	for _, c := range n.Content {
		if c.Kind == yaml.ScalarNode && c.Value != "" {
			out = append(out, c.Value)
		}
	}
	return out
}

// noteEntry extracts (title, body) of one notes entry: a plain string, or a
// mapping with title/content.
func noteEntry(e *yaml.Node) (title, body string) {
	switch e.Kind {
	case yaml.ScalarNode:
		return "", strings.TrimRight(e.Value, "\n")
	case yaml.MappingNode:
		for i := 0; i+1 < len(e.Content); i += 2 {
			k := strings.ToLower(strings.TrimSpace(e.Content[i].Value))
			v := e.Content[i+1]
			switch k {
			case "title", "name", "summary":
				title = strings.TrimSpace(cleanInline(v.Value))
			case "content", "body", "text", "description":
				body = strings.TrimRight(v.Value, "\n")
			}
		}
	}
	return title, body
}

// lastNodeLine estimates the last source line occupied by n.
func lastNodeLine(n *yaml.Node) int {
	last := n.Line
	var rec func(x *yaml.Node)
	rec = func(x *yaml.Node) {
		l := x.Line
		if x.Kind == yaml.ScalarNode {
			extra := strings.Count(strings.TrimRight(x.Value, "\n"), "\n")
			if x.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
				l += 1 + extra
			} else {
				l += extra
			}
		}
		if l > last {
			last = l
		}
		for _, c := range x.Content {
			rec(c)
		}
	}
	rec(n)
	return last
}

func verbatimLines(content []byte, start, end int) string {
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}

// uriPath returns the path component of a URI (or the string itself when it
// is not a URL).
func uriPath(s string) string {
	if u, err := url.Parse(s); err == nil && u.Path != "" {
		return u.Path
	}
	return s
}
