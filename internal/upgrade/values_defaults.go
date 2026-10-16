package upgrade

import (
	"encoding/json"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// FromDefaults returns the source-release default of each subject of a
// computed values change (values:removed, values:section-removed,
// values:default-changed), JSON-encoded exactly as the values snapshot holds
// it, read back from the Detail diffValues writes. A subject whose default
// the Detail does not carry (a section enumeration truncated at
// maxDetailLines) is absent from the result: its default is unknown, which
// callers must not read as empty. Other changes yield nil.
func FromDefaults(c domain.Change) map[string]string {
	if c.Provenance.Method != domain.MethodComputed {
		return nil
	}
	out := map[string]string{}
	switch c.Provenance.Rule {
	case RuleValuesRemoved, RuleValuesSectionRemoved:
		if len(c.Subjects) == 1 {
			// "… Default in <from>: <json>." on the first line
			line, _, _ := strings.Cut(c.Detail, "\n")
			if _, rest, ok := strings.Cut(line, ". Default in "); ok {
				if _, v, ok := strings.Cut(rest, ": "); ok {
					if v = strings.TrimSuffix(v, "."); json.Valid([]byte(v)) {
						out[c.Subjects[0]] = v
						return out
					}
				}
			}
		}
		// "<key> (default <json>)" per removed key of a section
		lines := map[string]string{}
		for _, l := range strings.Split(c.Detail, "\n") {
			lines[l] = ""
		}
		for _, k := range c.Subjects {
			prefix := k + " (default "
			for l := range lines {
				if v, ok := strings.CutPrefix(l, prefix); ok {
					if v, ok = strings.CutSuffix(v, ")"); ok && json.Valid([]byte(v)) {
						out[k] = v
						break
					}
				}
			}
		}
	case RuleValuesDefaultChanged:
		// "<old json> → <new json>"; the arrow may occur inside a string, so
		// take the split whose both sides are valid JSON
		if len(c.Subjects) != 1 {
			return nil
		}
		const arrow = " → "
		for i := 0; ; {
			j := strings.Index(c.Detail[i:], arrow)
			if j < 0 {
				break
			}
			old, nv := c.Detail[:i+j], c.Detail[i+j+len(arrow):]
			if json.Valid([]byte(old)) && json.Valid([]byte(nv)) {
				out[c.Subjects[0]] = old
				break
			}
			i += j + len(arrow)
		}
	default:
		return nil
	}
	return out
}

// EmptyDefault reports whether a JSON-encoded default sets nothing: null,
// "", {} or []. A key with such a default contributes nothing a customer
// could have relied on.
func EmptyDefault(v string) bool {
	switch strings.TrimSpace(v) {
	case "null", `""`, "{}", "[]":
		return true
	}
	return false
}
