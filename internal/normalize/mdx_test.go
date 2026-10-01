package normalize

import (
	"regexp"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestNormalizeHeadingStripsMDXAnchorsAndComponents(t *testing.T) {
	cases := map[string]string{
		"Precedence change for Azure authentication ((#azure-precedence))":                     "Precedence change for Azure authentication",
		"Unauthenticated endpoints ((#sys-endpoint-auth)) <EnterpriseAlert inline=\"true\" />": "Unauthenticated endpoints",
		"Known issues <EnterpriseAlert inline=\"true\" /> ((#x))":                              "Known issues",
		"Title with <Tag>inline</Tag> component":                                               "Title with inline component",
		"Plain heading":                                                                        "Plain heading",
		"Lowercase <br/> html stays out of the way":                                            "Lowercase <br/> html stays out of the way",
		"Generic type Map<String, int> is not a component":                                     "Generic type Map<String, int> is not a component",
		"Anchor kept when not at hashicorp style {#custom-id}":                                 "Anchor kept when not at hashicorp style",
		"Fragment (#not-an-anchor) in parentheses":                                             "Fragment (#not-an-anchor) in parentheses",
	}
	for in, want := range cases {
		if got := normalizeHeading(in); got != want {
			t.Errorf("normalizeHeading(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSelectSectionMatchesHeadingWithMDXAnchor(t *testing.T) {
	md := "# Important changes\n\n## Breaking changes\n\n### Azure precedence ((#azure-precedence)) <EnterpriseAlert inline=\"true\" />\n\nBody.\n\n### Other\n\nx\n"
	sec, err := SelectSection([]byte(md), regexp.MustCompile(`^Azure precedence$`))
	if err != nil {
		t.Fatal(err)
	}
	if sec.Heading != "Azure precedence" || !strings.Contains(sec.Body, "Body.") || strings.Contains(sec.Body, "Other") {
		t.Fatalf("section: %+v", sec)
	}
}

// An @include directive is another file's content: it must neither become
// prose of the section it sits in (which would fold every sub-section into one
// item) nor be part of an item.
func TestMDXIncludeLinesAreNotProse(t *testing.T) {
	md := `# Important changes

## New behavior

@include '../../../global/partials/new-behavior/ipc_lock-removed.mdx'

### First change ((#first))

| Change | Affected version |
| ------ | ---------------- |
| New behavior | 2.0.0+ |

The first change does a thing.

@include 'tips/a-partial.mdx'

### Second change ((#second))

The second change does another thing.
`
	items, _, err := ParseNotes(testInput(md, domain.RoleUpgradeGuide), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := texts(items)
	if len(got) != 2 {
		t.Fatalf("want one item per sub-section, got %d: %q", len(got), got)
	}
	for _, g := range got {
		if strings.Contains(g, "@include") || strings.Contains(g, "((#") {
			t.Errorf("item text keeps MDX noise: %q", g)
		}
	}
	if !strings.HasPrefix(got[0], "First change:") || !strings.HasPrefix(got[1], "Second change:") {
		t.Errorf("items: %q", got)
	}
}
