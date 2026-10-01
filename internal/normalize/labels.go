package normalize

import (
	"regexp"
	"strings"
)

// Label paragraphs.
//
// Some changelogs (the HashiCorp family: Vault, Consul, Nomad, Terraform and
// its providers) group the bullets of a release under uppercase label
// paragraphs instead of markdown headings:
//
//	## 2.1.1
//	### September 16, 2026
//
//	SECURITY:
//
//	* core: Update golang.org/x/crypto ...
//
//	BUG FIXES:
//
//	* ui: Fix ...
//
// Without help the labels are plain paragraphs, so every bullet loses its
// category. DocInput.LabelPattern names a regex; a standalone paragraph line
// matching it is treated as a heading ONE LEVEL BELOW the last real heading,
// so the generic section classification (SECURITY, BUG FIXES, FEATURES,
// DEPRECATIONS, BREAKING CHANGES, ...) applies unchanged.

// promoteLabels turns every label paragraph of d into a heading. A line is a
// label when it is ordinary text, starts in column 0..3, directly follows a
// blank line, a heading or the start of the document (so list continuations
// and wrapped paragraphs are never mistaken for labels) and its trimmed text
// matches re. The trailing colon is dropped from the heading text.
func (d *document) promoteLabels(re *regexp.Regexp) {
	if re == nil {
		return
	}
	cur := 0 // level of the last real heading
	for i := range d.lines {
		l := &d.lines[i]
		if l.kind == lkHeading {
			if !l.label {
				cur = l.level
			}
			continue
		}
		if l.kind != lkText || leadingSpaces(expandLeadingTabs(l.raw)) > 3 {
			continue
		}
		if i > 0 {
			if p := d.lines[i-1]; p.kind != lkBlank && p.kind != lkHeading {
				continue
			}
		}
		t := strings.TrimSpace(l.vis)
		if t == "" || !re.MatchString(t) {
			continue
		}
		h := strings.TrimSpace(strings.TrimSuffix(t, ":"))
		l.kind = lkHeading
		l.label = true
		l.level = cur + 1
		if l.level > 6 {
			l.level = 6
		}
		l.hraw = h
		l.head = normalizeHeading(h)
	}
}
