package renderdiff

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/render"
)

// The stored renders are the NORMALIZED objects the diff compared (nulls and
// empty containers dropped; Secret data and credential-named values
// replaced by "<sensitive sha256:…>" digest markers), one YAML document per
// object, sorted by object id, each headed by its identity:
//
//	---
//	# Object: apps/v1/Deployment/cert-manager/cert-manager
//	# Source: cert-manager/templates/deployment.yaml
//	apiVersion: apps/v1
//	...
//
// So every rendered change path resolves in the stored bytes, and no
// sensitive value is ever stored (DESIGN §2.3).

const (
	objectComment = "# Object: "
	sourceComment = "# Source: "
	// sensitiveMarker is how internal/render marks a value replaced by its digest.
	sensitiveMarker = "<sensitive sha256:"
)

// EncodeObjects serialises normalized objects as a YAML stream; redacted
// reports whether any value in it is a digest marker.
func EncodeObjects(objs []render.Object) (stream []byte, redacted bool) {
	var b bytes.Buffer
	for _, o := range objs {
		body, err := marshalYAML(o.Body)
		if err != nil { // a normalized body is plain YAML data; this cannot happen
			body = []byte(fmt.Sprintf("# unencodable object: %v\n", err))
		}
		b.WriteString("---\n" + objectComment + o.ID.String() + "\n")
		if o.Source != "" {
			b.WriteString(sourceComment + o.Source + "\n")
		}
		b.Write(body)
	}
	return b.Bytes(), bytes.Contains(b.Bytes(), []byte(sensitiveMarker))
}

// storedObject is one document of a stored render.
type storedObject struct {
	ID, Source string
	Body       any
}

// lookupObject finds one object of a stored render stream by id.
func lookupObject(stream []byte, id string) (*storedObject, error) {
	var cur *storedObject
	var doc bytes.Buffer
	finish := func() (*storedObject, error) {
		if cur == nil || cur.ID != id {
			return nil, nil
		}
		if err := yaml.Unmarshal(doc.Bytes(), &cur.Body); err != nil {
			return nil, fmt.Errorf("stored object %s: %w", id, err)
		}
		return cur, nil
	}
	sc := bufio.NewScanner(bytes.NewReader(stream))
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "---":
			if o, err := finish(); o != nil || err != nil {
				return o, err
			}
			cur = nil
			doc.Reset()
			continue
		case cur == nil && strings.HasPrefix(line, objectComment):
			cur = &storedObject{ID: strings.TrimPrefix(line, objectComment)}
			continue
		case cur != nil && cur.Source == "" && doc.Len() == 0 && strings.HasPrefix(line, sourceComment):
			cur.Source = strings.TrimPrefix(line, sourceComment)
			continue
		}
		if cur != nil && cur.ID == id {
			doc.WriteString(line + "\n")
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return finish()
}

// pathSeg is one segment of a rendered change path (docs/RENDER.md syntax).
type pathSeg struct {
	key    string // map key
	index  int    // list index (isIndex)
	field  string // keyed-list selector field ("name" of [name=controller])
	value  string // keyed-list selector value
	member string // set-member selector ([=--flag])
	kind   segKind
}

type segKind int

const (
	segKey segKind = iota
	segIndex
	segKeyed
	segMember
)

// parsePath parses "a.b[\"x.y\"].c[name=n][0].args[=--flag]".
func parsePath(p string) ([]pathSeg, error) {
	var out []pathSeg
	for i := 0; i < len(p); {
		switch p[i] {
		case '.':
			i++
		case '[':
			if i+1 < len(p) && p[i+1] == '"' {
				q, err := strconv.QuotedPrefix(p[i+1:])
				if err != nil {
					return nil, fmt.Errorf("path %q: bad quoted key at %d", p, i)
				}
				k, _ := strconv.Unquote(q)
				j := i + 1 + len(q)
				if j >= len(p) || p[j] != ']' {
					return nil, fmt.Errorf("path %q: missing ] at %d", p, j)
				}
				out = append(out, pathSeg{key: k, kind: segKey})
				i = j + 1
				continue
			}
			j := strings.IndexByte(p[i:], ']')
			if j < 0 {
				return nil, fmt.Errorf("path %q: missing ]", p)
			}
			sel := p[i+1 : i+j]
			i += j + 1
			switch {
			case strings.HasPrefix(sel, "#"): // disambiguation suffix of a repeated change: names nothing
			case strings.HasPrefix(sel, "="):
				out = append(out, pathSeg{member: sel[1:], kind: segMember})
			case strings.Contains(sel, "="):
				k := strings.IndexByte(sel, '=')
				out = append(out, pathSeg{field: sel[:k], value: sel[k+1:], kind: segKeyed})
			default:
				n, err := strconv.Atoi(sel)
				if err != nil || n < 0 {
					return nil, fmt.Errorf("path %q: bad selector [%s]", p, sel)
				}
				out = append(out, pathSeg{index: n, kind: segIndex})
			}
		default:
			j := strings.IndexAny(p[i:], ".[")
			if j < 0 {
				j = len(p) - i
			}
			out = append(out, pathSeg{key: p[i : i+j], kind: segKey})
			i += j
		}
	}
	return out, nil
}

// resolve walks a parsed path through a decoded object. A member selector
// narrows a list of container arguments to the tokens of that flag; on any
// other list it names the set the member belongs to (the list itself).
func resolve(v any, segs []pathSeg) (any, bool) {
	listName := ""
	for _, s := range segs {
		switch s.kind {
		case segKey:
			m, ok := v.(map[string]any)
			if !ok {
				return nil, false
			}
			if v, ok = m[s.key]; !ok {
				return nil, false
			}
			listName = s.key
		case segIndex:
			l, ok := v.([]any)
			if !ok || s.index >= len(l) {
				return nil, false
			}
			v = l[s.index]
		case segKeyed:
			l, ok := v.([]any)
			if !ok {
				return nil, false
			}
			var found any
			for _, e := range l {
				if m, ok := e.(map[string]any); ok && m[s.field] != nil && elemKey(listName, s.field, m) == s.value {
					found = e
					break
				}
			}
			if found == nil {
				return nil, false
			}
			v = found
		case segMember:
			l, ok := v.([]any)
			if !ok {
				return nil, false
			}
			if s.member == "(order)" || (listName != "args" && listName != "command") {
				continue
			}
			var toks []any
			for _, e := range l {
				t := fmt.Sprint(e)
				k := t
				if strings.HasPrefix(t, "-") {
					if i := strings.IndexByte(t, '='); i >= 0 {
						k = t[:i]
					}
				}
				if k == s.member {
					toks = append(toks, e)
				}
			}
			if len(toks) == 0 {
				return nil, false
			}
			v = toks
		}
	}
	return v, true
}

// elemKey mirrors internal/render's keyed-list element key (port keys carry
// the protocol).
func elemKey(listName, field string, m map[string]any) string {
	k := fmt.Sprint(m[field])
	if listName == "ports" && field != "name" {
		proto, _ := m["protocol"].(string)
		if proto == "" {
			proto = "TCP"
		}
		k += "/" + proto
	}
	return k
}

// marshalYAML encodes v as YAML with two-space indentation (maps by sorted
// key), the encoding of both the stored renders and Object results.
func marshalYAML(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
