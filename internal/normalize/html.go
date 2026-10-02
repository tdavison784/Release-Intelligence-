package normalize

import (
	"html"
	"strings"
)

// HTMLToMarkdown renders HTML text as markdown so that the markdown
// selectors and parsers apply to it. It exists for products whose release
// notes exist only as HTML: Go's published notes pages (go.dev/doc/go1.N)
// and the in-tree doc/go1.N.html copies of the pre-1.21 era (Tomcat's
// per-line changelog.html, Kafka's downloads-page release notes and Maven's
// docs/history.html are the cross-product cases; the format slot was created
// for docbook and filled for rst and adoc before).
//
// The conversion is line-preserving: output line N renders input line N (tags
// that span lines, comments and multi-line elements leave blank lines), so
// line ranges found in the markdown are line ranges of the original document
// and evidence locators stay valid.
//
//   - <h1>..<h6> become "#".."######" headings (written onto the heading's
//     opening line);
//   - <li> becomes a "- " bullet, nested items and continuation paragraphs
//     are indented (2 spaces per level);
//   - <dt> becomes a "**term**" line, <dd> an indented block;
//   - <pre> lines are indented by 4 spaces (code);
//   - <code> becomes `code`, <strong>/<b> **bold**, <em>/<i> *emphasis*,
//     <a href="u"> [text](u);
//   - <head>, <style>, <script>, <noscript> and <template> contents and
//     comments are dropped; entities are decoded; all other tags are
//     stripped.
//
// It is a renderer, not a validator: malformed input yields best-effort text.
func HTMLToMarkdown(src []byte) []byte {
	s := strings.ReplaceAll(string(src), "\r\n", "\n")
	c := &htmlConv{}
	c.convert(s)
	return []byte(strings.Join(c.lines, "\n"))
}

var (
	htmlVoidElems = setOf("br", "hr", "img", "meta", "link", "input", "area",
		"base", "col", "embed", "source", "track", "wbr", "basefont", "frame", "param")
	htmlSkipElems = setOf("head", "style", "script", "noscript", "template")
	htmlHrefRe    = dbAttrRe("href")
)

type htmlConv struct {
	lines    []string
	cur      strings.Builder
	curText  bool // cur holds non-space content
	listDeep int  // enclosing <li>s
	bullet   bool // next text starts a bullet
	ddDeep   int  // inside <dd>
	// heading text being collected
	inHeading bool
	headLevel int
	headLine  int
	headBuf   strings.Builder
	program   int // inside <pre>
	codeDeep  int // inline code nesting
	skipDeep  int // inside elements whose content is dropped
	urls      []string
	open      []string // open elements (to close unclosed inner tags)
}

func (c *htmlConv) convert(s string) {
	i := 0
	for i < len(s) {
		switch {
		case s[i] == '\n':
			c.newline()
			i++
		case strings.HasPrefix(s[i:], "<!--"):
			end := strings.Index(s[i+4:], "-->")
			body := s[i+4:]
			if end >= 0 {
				body = s[i+4 : i+4+end]
				i += 4 + end + 3
			} else {
				i = len(s)
			}
			for n := strings.Count(body, "\n"); n > 0; n-- {
				c.newline()
			}
		case s[i] == '<':
			end := tagEnd(s, i)
			if end < 0 { // not a tag: keep the rest as text
				c.text(s[i:])
				i = len(s)
				break
			}
			raw := s[i+1 : end]
			c.tag(raw)
			for n := strings.Count(raw, "\n"); n > 0; n-- {
				c.newline()
			}
			i = end + 1
		default:
			j := i
			for j < len(s) && s[j] != '<' && s[j] != '\n' {
				j++
			}
			c.text(s[i:j])
			i = j
		}
	}
	c.newline()
}

func (c *htmlConv) newline() {
	line := strings.TrimRight(c.cur.String(), " \t")
	c.lines = append(c.lines, line)
	c.cur.Reset()
	c.curText = false
}

// write appends s to the current line, emitting the line prefix first.
func (c *htmlConv) write(s string) {
	if s == "" || c.skipDeep > 0 {
		return
	}
	if c.inHeading {
		c.headBuf.WriteString(s)
		return
	}
	if !c.curText {
		if strings.TrimSpace(s) == "" {
			return
		}
		c.cur.Reset()
		c.cur.WriteString(c.prefix())
		c.curText = true
	}
	c.cur.WriteString(s)
}

// prefix is the indentation (and bullet) of a line that starts now.
func (c *htmlConv) prefix() string {
	switch {
	case c.program > 0:
		return strings.Repeat("  ", c.listDeep) + "    "
	case c.ddDeep > 0:
		return strings.Repeat("  ", c.listDeep) + "  "
	case c.listDeep > 0 && c.bullet:
		c.bullet = false
		return strings.Repeat("  ", c.listDeep-1) + "- "
	case c.listDeep > 0:
		return strings.Repeat("  ", c.listDeep)
	}
	return ""
}

func (c *htmlConv) text(t string) {
	if c.skipDeep > 0 {
		return
	}
	t = html.UnescapeString(t)
	if c.program > 0 {
		c.write(t)
		return
	}
	// collapse whitespace; keep one space between words and inline elements
	lead := t != strings.TrimLeft(t, " \t")
	trail := t != strings.TrimRight(t, " \t")
	f := strings.Join(strings.Fields(t), " ")
	if f == "" {
		if (c.curText || (c.inHeading && c.headBuf.Len() > 0)) && t != "" {
			c.write(" ")
		}
		return
	}
	if lead && (c.curText || (c.inHeading && c.headBuf.Len() > 0)) {
		c.write(" ")
	}
	c.write(f)
	if trail {
		c.write(" ")
	}
}

func (c *htmlConv) tag(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] == '!' || raw[0] == '?' {
		return
	}
	closing := strings.HasPrefix(raw, "/")
	if closing {
		raw = strings.TrimSpace(raw[1:])
	}
	selfClose := strings.HasSuffix(raw, "/")
	name := raw
	if k := strings.IndexAny(raw, " \t\n/"); k >= 0 {
		name = raw[:k]
	}
	attrs := raw[len(name):]
	name = strings.ToLower(name)
	// an end tag closes elements opened inside it whose end tag was omitted
	// (HTML allows <li>, <p>, <dt>, <dd> end tags to be omitted)
	if closing {
		idx := len(c.open) - 1
		for idx >= 0 && c.open[idx] != name {
			idx--
		}
		if idx < 0 {
			return
		}
		for k := len(c.open) - 1; k > idx; k-- {
			c.handle(c.open[k], true, "", false)
		}
		c.open = c.open[:idx]
	} else if !selfClose && !htmlVoidElems[name] {
		c.open = append(c.open, name)
	}
	c.handle(name, closing, attrs, selfClose)
}

// handle renders one open or close tag of element name.
func (c *htmlConv) handle(name string, closing bool, attrs string, selfClose bool) {
	switch {
	case len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6':
		if !closing && !selfClose && c.skipDeep == 0 {
			c.inHeading = true
			c.headLevel = int(name[1] - '0')
			c.headLine = len(c.lines)
			c.headBuf.Reset()
			return
		}
		if closing && c.inHeading {
			c.inHeading = false
			heading := strings.Repeat("#", c.headLevel) + " " + strings.TrimSpace(c.headBuf.String())
			if c.headLine < len(c.lines) {
				c.lines[c.headLine] = heading
				return
			}
			c.cur.Reset()
			c.cur.WriteString(heading)
			c.curText = true
		}
	case name == "li":
		if closing {
			if c.listDeep > 0 {
				c.listDeep--
			}
			c.bullet = false
		} else {
			c.listDeep++
			c.bullet = true
		}
	case name == "dd":
		if closing {
			if c.ddDeep > 0 {
				c.ddDeep--
			}
		} else {
			c.ddDeep++
		}
	case htmlSkipElems[name]:
		if closing {
			if c.skipDeep > 0 {
				c.skipDeep--
			}
		} else if !selfClose {
			c.skipDeep++
		}
	case name == "pre":
		if closing {
			if c.program > 0 {
				c.program--
			}
		} else if !selfClose {
			c.program++
		}
	case name == "code", name == "kbd", name == "samp", name == "var":
		if c.program > 0 {
			return
		}
		if closing {
			if c.codeDeep > 0 {
				c.codeDeep--
				if c.codeDeep == 0 {
					c.write("`")
				}
			}
		} else if !selfClose {
			if c.codeDeep == 0 {
				c.write("`")
			}
			c.codeDeep++
		}
	case name == "strong" || name == "b":
		c.write("**")
	case name == "em" || name == "i":
		c.write("*")
	case name == "dt":
		c.write("**")
	case name == "a":
		url := attrValue(htmlHrefRe, attrs)
		switch {
		case closing:
			if n := len(c.urls); n > 0 {
				u := c.urls[n-1]
				c.urls = c.urls[:n-1]
				if u != "" {
					c.write("](" + u + ")")
				}
			}
		case selfClose:
			c.write(url)
		default:
			c.urls = append(c.urls, url)
			if url != "" {
				c.write("[")
			}
		}
	}
}
