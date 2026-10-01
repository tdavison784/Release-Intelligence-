package normalize

import (
	"regexp"
	"strings"
)

// MDX dialect handled by the HashiCorp documentation platform
// (web-unified-docs: Vault, Consul, Nomad, Boundary, Terraform ...).
//
//	@include '../../../global/partials/x.mdx'     another file's content, not prose
//	### Title ((#anchor-id))                      explicit heading anchor
//	### Title <EnterpriseAlert inline="true" />   JSX component inside a heading
//
// An include line is not text of this document (its content lives in another
// file, which is not resolved), so it must not turn a heading section into
// "prose" or become part of an item; anchors and components are not part of a
// heading's title.

var (
	mdxIncludeRe   = regexp.MustCompile(`^\s*@include\s+['"][^'"]+['"]\s*$`)
	mdxAnchorRe    = regexp.MustCompile(`\s*\(\(#[^()\s]*\)\)`)
	mdxJSXInlineRe = regexp.MustCompile(`</?[A-Z][A-Za-z0-9.]*(?:\s[^<>]*?)?/?>`)
)

// stripMDXHeadingNoise removes explicit anchors and JSX components from a
// heading's text.
func stripMDXHeadingNoise(h string) string {
	if !strings.Contains(h, "((#") && !strings.Contains(h, "<") {
		return h
	}
	h = mdxAnchorRe.ReplaceAllString(h, "")
	h = mdxJSXInlineRe.ReplaceAllString(h, "")
	return strings.TrimSpace(h)
}
