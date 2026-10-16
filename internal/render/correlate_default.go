package render

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// fromDefaultGone is the second half of the values-default rule (lane
// defaults): a removed or default-changed values key whose source default
// was in effect for the customer shows in their render as that default
// value disappearing — the rendered path is not the values path, so only
// the value and its location tie them. It holds when the rendered change's
// before holds, under a field named like the key's last segment, the key's
// source default (or its redaction digest, for a credential-named field),
// more often than its after does, and the key's top-level section names the
// object (its name, kind or a segment of the rendered path).
//
// Minimum specificity: booleans, null, empty values, strings shorter than 4
// characters, single-digit numbers and tiny maps/lists never link — they
// are everywhere in a render.
func fromDefaultGone(rc Change, c domain.Change) bool {
	if rc.Before == nil || c.Category != domain.CategoryHelmValues {
		return false
	}
	defaults := upgrade.FromDefaults(c)
	if len(defaults) == 0 {
		return false
	}
	before, after := Unquote(rc.Before), Unquote(rc.After)
	for key, raw := range defaults {
		var dv any
		if json.Unmarshal([]byte(raw), &dv) != nil || !specificDefault(dv) {
			continue
		}
		segs := keySegments(key)
		if len(segs) == 0 || !locatesObject(segs, rc) {
			continue
		}
		field := segs[len(segs)-1]
		want := map[string]bool{encode(dv): true, encode(redact(dv)): true}
		root := lastSeg(rc.Pattern)
		if countField(before, root, field, want) > countField(after, root, field, want) {
			return true
		}
	}
	return false
}

// specificDefault is the minimum-specificity guard of fromDefaultGone.
func specificDefault(v any) bool {
	switch t := v.(type) {
	case string:
		return len(strings.TrimSpace(t)) >= 4
	case float64:
		return t <= -10 || t >= 10
	case map[string]any, []any:
		return len(encode(t)) >= 8
	}
	return false // bool, null
}

// countField counts the occurrences of a field named field whose value
// encodes to one of want, anywhere in v; name is the field v itself sits at
// (the rendered change's last path segment), so a scalar before/after
// counts when the change is about that very field.
func countField(v any, name, field string, want map[string]bool) int {
	n := 0
	if name == field && want[encode(v)] {
		n++
	}
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			n += countField(x, k, field, want)
		}
	case []any:
		for _, x := range t {
			n += countField(x, "", field, want)
		}
	}
	return n
}

// locatesObject: the key's top-level section (the component it configures)
// names the rendered object — a key without a section is located by its
// field alone.
func locatesObject(segs []string, rc Change) bool {
	if len(segs) < 2 {
		return true
	}
	top := squash(segs[0])
	if len(top) < 3 {
		return false
	}
	if strings.Contains(squash(rc.Object.Name), top) || squash(rc.Object.Kind) == top {
		return true
	}
	for _, s := range keySegments(rc.Pattern) {
		if squash(s) == top {
			return true
		}
	}
	return false
}

// squash lowercases and drops separators: "kube-dns", "kubeDns" and
// "kube_dns" compare equal.
func squash(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// keySegments splits a dotted values key or rendered path into its
// segments: `a.b["x.y"]` → [a b x.y]; element selectors ("[]",
// "[name=x]") are dropped.
func keySegments(k string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(k); i++ {
		switch k[i] {
		case '.':
			flush()
		case '[':
			flush()
			j := strings.IndexByte(k[i:], ']')
			if j < 0 {
				return append(out, k[i:])
			}
			inner := k[i+1 : i+j]
			if s, err := strconv.Unquote(inner); err == nil {
				out = append(out, s)
			}
			i += j
		default:
			cur.WriteByte(k[i])
		}
	}
	flush()
	return out
}
