package normalize

import "testing"

// The structural reading (ListItems) on plain markdown: a section that opens
// with a callout (Karpenter's upgrade-guide sections open with a Hugo warning
// alert) must yield its bullet list as one item per bullet plus the callout
// prose as an item of its own, instead of folding the whole section into a
// single prose item.
func TestParseNotesListItemsMarkdown(t *testing.T) {
	body := []byte(`### Upgrading to ` + "`1.14.0`" + `+

{{% alert title="Warning" color="warning" %}}
Karpenter ` + "`1.1.0`" + ` drops the support for ` + "`v1beta1`" + ` APIs.
{{% /alert %}}

* This version graduates the [Capacity Buffers]({{<ref "../concepts/capacitybuffers.md">}}) API to ` + "`v1beta1`" + `.
* This version adds a [Balanced consolidation policy](https://example.com/pull/2962).
* No breaking changes
`)
	in := DocInput{SourceID: "upgrade-guide", URI: "test", Content: body, ListItems: true}
	items, _, err := ParseNotes(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, it := range items {
		texts = append(texts, it.Text)
	}
	want := []string{
		"Upgrading to `1.14.0`+: Karpenter `1.1.0` drops the support for `v1beta1` APIs.",
		"This version graduates the Capacity Buffers API to `v1beta1`.",
		"This version adds a Balanced consolidation policy.",
		"No breaking changes",
	}
	if len(texts) != len(want) {
		t.Fatalf("items: %q", texts)
	}
	for i := range want {
		if texts[i] != want[i] {
			t.Fatalf("item %d = %q, want %q (all: %q)", i, texts[i], want[i], texts)
		}
	}
}
