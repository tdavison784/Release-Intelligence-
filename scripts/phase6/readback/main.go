// Command readback annotates RI's stored environment files and renders with
// the withheld values RI's own read tools read back (PROTOCOL §3, decision
// D1 at X-0002).
//
//	go run ./scripts/phase6/readback -evidence <dir> -out <dir> -tsv <file> [-authored <env-root>] <path>...
//
// <dir> is a materialised case ($P6_EVIDENCE/<case-id>: files/,
// artifacts.tsv, renders.tsv). Each <path> (relative to files/) is written to
// -out/<path> with RI's bytes unchanged except that every line holding a
// marker RI reads back gets a YAML comment carrying RI's note. The schema
// index and the read-back are RI's: the CRDs bundleSchemas indexes
// (internal/investigate/agent/withheld.go), redaction.Schemas.Recover on the
// objects render.ParseObjects parses, and readBackNote's text. With -plain
// (raw mode) each marker read back is replaced by its value instead, with no
// comment. Markers RI does not read back stay as they are. -tsv lists every marker line (read back or
// not); -authored only fills that list's as-authored column, by matching each
// marker against RI's own marker of every string in the authored env files.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/redaction"
	"github.com/tdavison784/release-intelligence/internal/render"
)

var markerRE = regexp.MustCompile(`<(?:redacted|sensitive) sha256:[0-9a-f]+>`)

func main() {
	evidence := flag.String("evidence", "", "materialised case dir")
	out := flag.String("out", "", "output dir")
	tsv := flag.String("tsv", "", "marker list (TSV)")
	authored := flag.String("authored", "", "the case's authored env root (as-authored column only)")
	plain := flag.Bool("plain", false, "replace each marker read back by its value, with no comment (raw mode)")
	flag.Parse()
	if *evidence == "" || *out == "" || *tsv == "" {
		fmt.Fprintln(os.Stderr, "usage: readback -evidence <dir> -out <dir> -tsv <file> [-authored <env-root>] <path>...")
		os.Exit(2)
	}
	if err := run(*evidence, *out, *tsv, *authored, *plain, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "readback:", err)
		os.Exit(1)
	}
}

func run(evidence, out, tsvPath, authoredRoot string, plain bool, paths []string) error {
	ix, err := schemas(evidence)
	if err != nil {
		return err
	}
	byMarker := map[string]string{}
	if authoredRoot != "" {
		if byMarker, err = authoredValues(authoredRoot); err != nil {
			return err
		}
	}
	var rows []string
	for _, p := range paths {
		data, err := os.ReadFile(filepath.Join(evidence, "files", p))
		if err != nil {
			return err
		}
		annotated, rs, err := annotate(ix, data, plain)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		for _, r := range rs {
			rows = append(rows, strings.Join([]string{p, fmt.Sprint(r.line), r.path, r.marker, r.value, r.note, byMarker[r.marker]}, "\t"))
		}
		dst := filepath.Join(out, p)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, annotated, 0o644); err != nil {
			return err
		}
	}
	var b strings.Builder
	b.WriteString("file\tline\tpath\tmarker\tread-back value\tnote\tas authored\n")
	for _, r := range rows {
		b.WriteString(r + "\n")
	}
	return os.WriteFile(tsvPath, []byte(b.String()), 0o644)
}

// schemas is bundleSchemas over the materialised case: the CRD artifacts,
// the environment files and the environment renders, as RI stored them.
func schemas(evidence string) (*redaction.Schemas, error) {
	var arts []string
	tsv, err := os.ReadFile(filepath.Join(evidence, "artifacts.tsv"))
	if err != nil {
		return nil, err
	}
	for i, line := range strings.Split(strings.TrimRight(string(tsv), "\n"), "\n") {
		f := strings.Split(line, "\t")
		if i > 0 && len(f) > 2 && (f[2] == "crd" || f[2] == "environment-file") {
			arts = append(arts, f[0])
		}
	}
	renders, err := os.ReadFile(filepath.Join(evidence, "renders.tsv"))
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimRight(string(renders), "\n"), "\n") {
		f := strings.Split(line, "\t")
		for _, p := range f[2:] {
			if p != "" {
				arts = append(arts, p)
			}
		}
	}
	ix := redaction.NewSchemas()
	for _, p := range arts {
		if !strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".yml") && !strings.HasSuffix(p, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(evidence, "files", p))
		if err != nil || bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
			continue
		}
		objs, err := render.ParseObjects(data)
		if err != nil {
			continue
		}
		for _, o := range objs {
			if o.ID.Kind == "CustomResourceDefinition" && o.ID.Group == "apiextensions.k8s.io" {
				ix.AddCRD(o.Body)
			}
		}
	}
	return ix, nil
}

// readBackNote is RI's note (internal/investigate/agent/withheld.go).
func readBackNote(r redaction.Recovered) string {
	return fmt.Sprintf("stored redacted as %s: the digest of %q, an enum value its CRD declares for %s (not a credential)", r.Marker, r.Value, r.Path)
}

type markerLine struct {
	line                      int
	path, marker, value, note string
}

// annotate splits data into documents as render.ParseObjects does, reads
// back each document's objects, and comments the lines of the markers read
// back (located by field path and marker in the document's YAML nodes).
func annotate(ix *redaction.Schemas, data []byte, plain bool) ([]byte, []markerLine, error) {
	lines := strings.SplitAfter(string(data), "\n")
	notes := map[int]string{}
	paths := map[int]string{}
	values := map[int]string{}
	marks := map[int]string{}
	start := 0
	flush := func(end int) error {
		doc := strings.Join(lines[start:end], "")
		nodes := markerNodes(doc)
		for _, n := range nodes {
			paths[start+n.line] = n.path
		}
		objs, err := render.ParseObjects([]byte(doc))
		if err != nil {
			return nil // RI reads back nothing from an unparsable artifact
		}
		for _, o := range objs {
			for _, r := range ix.Recover(o.ID.Group, o.ID.Version, o.ID.Kind, o.Body) {
				found := false
				for _, n := range nodes {
					if n.path == r.Path && n.marker == r.Marker {
						notes[start+n.line], values[start+n.line], marks[start+n.line], found = readBackNote(r), r.Value, r.Marker, true
					}
				}
				if !found {
					return fmt.Errorf("read back %s at %s but found no such line", r.Marker, r.Path)
				}
			}
		}
		return nil
	}
	for i, l := range lines {
		t := strings.TrimRight(l, "\r\n")
		if strings.HasPrefix(t, "---") && strings.TrimSpace(strings.TrimLeft(t, "-")) == "" {
			if err := flush(i); err != nil {
				return nil, nil, err
			}
			start = i + 1
		}
	}
	if err := flush(len(lines)); err != nil {
		return nil, nil, err
	}
	var b strings.Builder
	var rs []markerLine
	for i, l := range lines {
		body := strings.TrimRight(l, "\r\n")
		for _, m := range markerRE.FindAllString(body, -1) {
			rs = append(rs, markerLine{line: i + 1, path: paths[i], marker: m, value: values[i], note: notes[i]})
		}
		if n, ok := notes[i]; ok && plain {
			b.WriteString(plainValue(body, marks[i], values[i]) + l[len(body):])
		} else if ok {
			b.WriteString(body + "  # " + n + l[len(body):])
		} else {
			b.WriteString(l)
		}
	}
	return []byte(b.String()), rs, nil
}

// plainValue puts value where line has marker: inside the marker's quotes,
// or as a YAML scalar that reads back as that string.
func plainValue(line, marker, value string) string {
	if q := `"` + marker + `"`; strings.Contains(line, q) {
		return strings.Replace(line, q, strconv.Quote(value), 1)
	}
	y, _ := yaml.Marshal(value)
	return strings.Replace(line, marker, strings.TrimSuffix(string(y), "\n"), 1)
}

type node struct {
	line         int // 0-based, within the document
	path, marker string
}

// markerNodes lists the scalar markers of one YAML document with their field
// paths, in the form redaction.Schemas uses ("a.b", "[]" per sequence item).
func markerNodes(doc string) []node {
	var root yaml.Node
	if yaml.Unmarshal([]byte(doc), &root) != nil {
		return nil
	}
	var out []node
	var walk func(n *yaml.Node, path string)
	walk = func(n *yaml.Node, path string) {
		switch n.Kind {
		case yaml.DocumentNode:
			for _, c := range n.Content {
				walk(c, path)
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				p := n.Content[i].Value
				if path != "" {
					p = path + "." + p
				}
				walk(n.Content[i+1], p)
			}
		case yaml.SequenceNode:
			for _, c := range n.Content {
				walk(c, path+"[]")
			}
		case yaml.ScalarNode:
			if redaction.IsMarker(n.Value) {
				out = append(out, node{line: n.Line - 1, path: path, marker: n.Value})
			}
		}
	}
	walk(&root, "")
	return out
}

// authoredValues maps RI's markers (both forms) of every string scalar in
// the authored env files to that string.
func authoredValues(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !(strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml")) {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		dec := yaml.NewDecoder(bufio.NewReader(f))
		for {
			var n yaml.Node
			if dec.Decode(&n) != nil {
				break
			}
			var walk func(*yaml.Node)
			walk = func(n *yaml.Node) {
				if n.Kind == yaml.ScalarNode {
					out[redaction.Marker(n.Value)] = n.Value
					out[redaction.RenderMarker(n.Value)] = n.Value
				}
				for _, c := range n.Content {
					walk(c)
				}
			}
			walk(&n)
		}
		return nil
	})
	return out, err
}
