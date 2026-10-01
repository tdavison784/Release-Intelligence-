package normalize

import (
	"html"
	"regexp"
	"strings"
)

// DocBookToMarkdown renders DocBook (SGML or XML) text as markdown so that the
// markdown selectors and parsers apply to it. It exists for products whose
// release notes are DocBook sources in the repository (PostgreSQL's
// doc/src/sgml/release-NN.sgml: one <sect1> per release, "Migration to Version
// X" and "Changes" sub-sections, one <listitem> per change).
//
// The conversion is line-preserving: output line N renders input line N (tags
// that span lines, comments and multi-line elements leave blank lines), so
// line ranges found in the markdown are line ranges of the original document
// and evidence locators stay valid.
//
//   - <sect1>..<sect5> + their first <title> become "#".."#####" headings;
//   - <listitem> becomes a "- " bullet, nested items and continuation
//     paragraphs are indented (2 spaces per level);
//   - <programlisting>/<screen> lines are indented by 4 spaces (code);
//   - inline code elements (<literal>, <command>, <function>, <varname>, ...)
//     become `code`; <emphasis> *emphasis*; <quote> "quotes"; <ulink url=..>
//     [text](url); <xref linkend=x/> `x`;
//   - commit links (<ulink url=..>&sect;</ulink>) and comments are dropped;
//   - entities are decoded, all other tags are stripped.
//
// It is a renderer, not a validator: malformed input yields best-effort text.
func DocBookToMarkdown(src []byte) []byte {
	s := strings.ReplaceAll(string(src), "\r\n", "\n")
	s = sectLinkRe.ReplaceAllString(s, "")
	c := &dbConv{}
	c.convert(s)
	return []byte(strings.Join(c.lines, "\n"))
}

// sectLinkRe matches PostgreSQL's per-item commit links "<ulink url=..>&sect;</ulink>".
var sectLinkRe = regexp.MustCompile(`<ulink[ \t]+url="[^"]*"[ \t]*>[ \t]*&sect;[ \t]*</ulink>`)

var (
	dbCodeElems = setOf("literal", "command", "function", "varname", "filename", "application", "option",
		"type", "structname", "structfield", "envar", "parameter", "replaceable", "classname", "symbol",
		"constant", "token", "systemitem", "sgmltag", "userinput", "computeroutput", "database", "returnvalue",
		"property", "productnumber", "prompt", "keycap", "optional")
	dbListElems = setOf("itemizedlist", "orderedlist", "variablelist", "simplelist", "segmentedlist")
	dbSkipElems = setOf("indexterm", "footnote")
	// empty elements: written without an end tag in SGML ("<xref linkend=x>")
	dbVoidElems    = setOf("xref", "anchor", "colspec", "spanspec", "co", "sbr", "footnoteref", "void", "area")
	dbProgramElems = setOf("programlisting", "screen", "synopsis", "literallayout")
	dbAttrRe       = func(name string) *regexp.Regexp {
		return regexp.MustCompile(`(?s)\b` + name + `\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	}
	dbURLRe     = dbAttrRe("url")
	dbLinkendRe = dbAttrRe("linkend")
)

func setOf(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

type dbConv struct {
	lines    []string
	cur      strings.Builder
	curText  bool // cur holds non-space content
	listDeep int  // enclosing <listitem>s
	bullet   bool // next text starts a bullet
	pendSect int  // level of a <sectN> whose <title> has not been seen
	// heading title being collected
	inHeading bool
	headLevel int
	headLine  int
	headBuf   strings.Builder
	codeDeep  int      // inline code nesting
	program   int      // inside <programlisting> etc.
	skipDeep  int      // inside elements whose content is dropped
	urls      []string // open <ulink> urls
	open      []string // open elements (to resolve "</>")
}

func (c *dbConv) convert(s string) {
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

// tagEnd returns the index of the '>' closing the tag opened at s[i] ('<'),
// ignoring '>' inside quoted attribute values, or -1.
func tagEnd(s string, i int) int {
	var quote byte
	for j := i + 1; j < len(s); j++ {
		ch := s[j]
		switch {
		case quote != 0:
			if ch == quote {
				quote = 0
			}
		case ch == '"' || ch == '\'':
			quote = ch
		case ch == '>':
			return j
		}
	}
	return -1
}

func (c *dbConv) newline() {
	line := strings.TrimRight(c.cur.String(), " \t")
	c.lines = append(c.lines, line)
	c.cur.Reset()
	c.curText = false
}

// write appends s to the current line, emitting the line prefix first.
func (c *dbConv) write(s string) {
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
func (c *dbConv) prefix() string {
	switch {
	case c.program > 0:
		return strings.Repeat("  ", c.listDeep) + "    "
	case c.listDeep > 0 && c.bullet:
		c.bullet = false
		return strings.Repeat("  ", c.listDeep-1) + "- "
	case c.listDeep > 0:
		return strings.Repeat("  ", c.listDeep)
	}
	return ""
}

func (c *dbConv) text(t string) {
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

func (c *dbConv) tag(raw string) {
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
	// SGML shorthand "</>" closes the innermost open element; an end tag also
	// closes elements opened inside it whose end tag was omitted.
	if closing {
		idx := len(c.open) - 1
		if name != "" {
			for idx >= 0 && c.open[idx] != name {
				idx--
			}
		}
		if idx < 0 {
			if name == "" {
				return
			}
		} else {
			for k := len(c.open) - 1; k > idx; k-- {
				c.handle(c.open[k], true, "", false)
			}
			name = c.open[idx]
			c.open = c.open[:idx]
		}
	} else if !selfClose && !dbVoidElems[name] {
		c.open = append(c.open, name)
	}
	c.handle(name, closing, attrs, selfClose)
}

// handle renders one open or close tag of element name.
func (c *dbConv) handle(name string, closing bool, attrs string, selfClose bool) {
	switch {
	case len(name) == 5 && strings.HasPrefix(name, "sect") && name[4] >= '1' && name[4] <= '5':
		if closing {
			c.pendSect = 0
		} else {
			c.pendSect = int(name[4] - '0')
		}
	case name == "title":
		c.title(closing)
	case name == "listitem":
		if closing {
			if c.listDeep > 0 {
				c.listDeep--
			}
			c.bullet = false
		} else {
			c.listDeep++
			c.bullet = true
		}
	case dbListElems[name] || name == "para" || name == "formalpara" || name == "simpara":
		// block boundaries are expressed by the (blank) lines of the source
	case dbSkipElems[name]:
		if closing {
			if c.skipDeep > 0 {
				c.skipDeep--
			}
		} else if !selfClose {
			c.skipDeep++
		}
	case dbProgramElems[name]:
		if closing {
			if c.program > 0 {
				c.program--
			}
		} else if !selfClose {
			c.program++
		}
	case dbCodeElems[name]:
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
	case name == "emphasis" || name == "firstterm":
		c.write("*")
	case name == "quote":
		c.write(`"`)
	case name == "term":
		c.write("**")
	case name == "ulink":
		url := attrValue(dbURLRe, attrs)
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
	case name == "xref":
		if id := attrValue(dbLinkendRe, attrs); id != "" {
			c.write("`" + id + "`")
		}
	}
}

// title handles <title>: the first one after a <sectN> is the heading of that
// section; any other (formalpara, table, ...) renders in bold.
func (c *dbConv) title(closing bool) {
	if !closing {
		if c.pendSect > 0 && !c.inHeading {
			c.inHeading = true
			c.headLevel = c.pendSect
			c.pendSect = 0
			c.headLine = len(c.lines)
			c.headBuf.Reset()
			return
		}
		c.write("**")
		return
	}
	if !c.inHeading {
		c.write("**")
		return
	}
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

func attrValue(re *regexp.Regexp, attrs string) string {
	m := re.FindStringSubmatch(attrs)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}
