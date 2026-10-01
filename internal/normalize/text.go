package normalize

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Inline markdown helpers: link stripping, emphasis removal, whitespace
// collapsing and bounded truncation. All functions are pure.

var (
	imageRe    = regexp.MustCompile(`!\[([^\]]*)\]\((?:[^()\s]|\([^()\s]*\))*(?:\s+"[^"]*")?\s*\)`)
	inlineLink = regexp.MustCompile(`\[((?:[^\[\]]|\[[^\[\]]*\])*)\]\(\s*<?(?:[^()\s<>]|\([^()\s]*\))*>?(?:\s+(?:"[^"]*"|'[^']*'))?\s*\)`)
	refLinkRe  = regexp.MustCompile(`\[((?:[^\[\]]|\[[^\[\]]*\])+)\]\[[^\]]*\]`)
	autolinkRe = regexp.MustCompile(`<((?:https?|mailto):[^>\s]+)>`)
	boldStarRe = regexp.MustCompile(`\*\*(\S(?:.*?\S)?)\*\*`)
	boldUndRe  = regexp.MustCompile(`(^|[^\w])__(\S(?:.*?\S)?)__([^\w]|$)`)
	htmlTagRe  = regexp.MustCompile(`</?(?:br|p|div|span|details|summary|kbd|sub|sup|b|i|em|strong|a|img|ul|ol|li|h[1-6]|hr|table|thead|tbody|tr|td|th|center)(?:\s[^>]*)?/?>`)
	escapeRe   = regexp.MustCompile("\\\\([!-/:-@\\[-`{-~])")
	spaceRe    = regexp.MustCompile(`\s+`)
)

// collapseSpace collapses every run of whitespace to one space and trims.
func collapseSpace(s string) string {
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// protectCode replaces every inline code span by a placeholder and returns
// the spans so they can be restored by restoreCode.
func protectCode(s string) (string, []string) {
	if !strings.Contains(s, "`") {
		return s, nil
	}
	var (
		out   strings.Builder
		spans []string
	)
	i := 0
	for i < len(s) {
		if s[i] != '`' {
			out.WriteByte(s[i])
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] == '`' {
			j++
		}
		run := s[i:j]
		// find the closing run of the same length
		k := j
		closeAt := -1
		for k < len(s) {
			if s[k] != '`' {
				k++
				continue
			}
			m := k
			for m < len(s) && s[m] == '`' {
				m++
			}
			if m-k == len(run) {
				closeAt = k
				break
			}
			k = m
		}
		if closeAt < 0 {
			out.WriteString(run)
			i = j
			continue
		}
		spans = append(spans, s[i:closeAt+len(run)])
		out.WriteString("\x01" + strconv.Itoa(len(spans)-1) + "\x02")
		i = closeAt + len(run)
	}
	return out.String(), spans
}

var placeholderRe = regexp.MustCompile("\x01(\\d+)\x02")

func restoreCode(s string, spans []string) string {
	if len(spans) == 0 {
		return s
	}
	return placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		n, err := strconv.Atoi(m[1 : len(m)-1])
		if err != nil || n < 0 || n >= len(spans) {
			return m
		}
		return spans[n]
	})
}

// convertLinks replaces markdown links, reference links and images by their
// text, leaving inline code untouched.
func convertLinks(s string) string {
	s, spans := protectCode(s)
	s = imageRe.ReplaceAllString(s, "$1")
	s = inlineLink.ReplaceAllString(s, "$1")
	s = refLinkRe.ReplaceAllString(s, "$1")
	s = autolinkRe.ReplaceAllString(s, "$1")
	return restoreCode(s, spans)
}

// cleanInline converts inline markdown to readable plain text: links become
// their text, bold markers and simple HTML tags are dropped, entities and
// backslash escapes are resolved and whitespace is collapsed. Inline code is
// kept verbatim (including its backticks).
func cleanInline(s string) string {
	s, spans := protectCode(s)
	s = imageRe.ReplaceAllString(s, "$1")
	s = inlineLink.ReplaceAllString(s, "$1")
	s = refLinkRe.ReplaceAllString(s, "$1")
	s = autolinkRe.ReplaceAllString(s, "$1")
	s = boldStarRe.ReplaceAllString(s, "$1")
	s = boldUndRe.ReplaceAllString(s, "$1$2$3")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = escapeRe.ReplaceAllString(s, "$1")
	s = restoreCode(s, spans)
	return collapseSpace(s)
}

// stripMarkdown is cleanInline plus removal of backticks and single emphasis
// markers; it is used to compare table headers and keys.
func stripMarkdown(s string) string {
	s = cleanInline(s)
	s = strings.ReplaceAll(s, "`", "")
	s = trimSurroundingMarks(s)
	return collapseSpace(s)
}

// truncateText bounds s to roughly max bytes on a rune boundary, preferring
// to cut at a space, and appends an ellipsis when it cut.
func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	if sp := strings.LastIndexByte(s[:cut], ' '); sp > cut-60 && sp > 0 {
		cut = sp
	}
	return strings.TrimRight(s[:cut], " ,;:-") + "…"
}
