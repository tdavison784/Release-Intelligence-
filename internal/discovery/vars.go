package discovery

import (
	"regexp"
	"strings"
)

// Markers substituted for build-time values whose meaning is known.
const (
	markGitTag      = "%GIT_TAG%"      // the pushed / described git tag
	markGitCommit   = "%GIT_COMMIT%"   // a commit sha (snapshot builds)
	markVersionFile = "%VERSION_FILE%" // contents of an in-repo VERSION file
)

// varTable holds variable assignments of one file (make, shell, CI env).
type varTable map[string]string

var (
	makeAssignRe  = regexp.MustCompile(`^\s*(?:export\s+|override\s+)?([A-Za-z_][A-Za-z0-9_.-]*)\s*(\?=|::=|:=|\+=|=)\s*(.*?)\s*$`)
	makeEvalRe    = regexp.MustCompile(`\$\(eval\s+([A-Za-z_][A-Za-z0-9_]*)\s*:?=\s*(.*)\)\s*$`)
	shellAssignRe = regexp.MustCompile(`(?:^|[\s;(])(?:export\s+|local\s+|readonly\s+)?([A-Za-z_][A-Za-z0-9_]*)=("(?:[^"\\]|\\.)*"|'[^']*'|[^\s;#&|)]*)`)
	yamlEnvRe     = regexp.MustCompile(`^\s*-?\s*([A-Z][A-Z0-9_]*)\s*:\s*(.+?)\s*$`)

	actionsDefaultRe = regexp.MustCompile(`\$\{\{[^}]*\|\|\s*'([^']*)'\s*\}\}`)
	gitDescribeRe    = regexp.MustCompile(`\$\((?:shell\s+)?git\s+describe[^)]*\)`)
	gitRevParseRe    = regexp.MustCompile(`\$\((?:shell\s+)?git\s+rev-parse[^)]*\)`)
	catVersionRe     = regexp.MustCompile(`\$\((?:shell\s+)?cat\s+(?:\./)?VERSION[^)]*\)`)
	commitVarRe      = regexp.MustCompile(`\$\{\{\s*github\.sha\s*\}\}|\$\{GITHUB_SHA(?:::\d+)?\}|\$GITHUB_SHA\b`)
)

// collectVars extracts assignments; the first assignment of a name wins
// (defaults such as `?=` usually come first).
func collectVars(text string, style string) varTable {
	vars := varTable{}
	set := func(k, v string) {
		v = normalizeValue(v)
		if style != "make" {
			// command substitutions and unbalanced quotes (multi-line
			// values) cannot be resolved statically
			if strings.Contains(v, "$(") || strings.Contains(v, "`") || strings.Count(v, `"`)%2 == 1 || strings.Count(v, `'`)%2 == 1 {
				return
			}
		}
		if strings.Contains(v, "$"+k) || strings.Contains(v, "${"+k) || strings.Contains(v, "$("+k) {
			return // self reference (e.g. X="${X##*/}")
		}
		if _, ok := vars[k]; !ok && v != "" {
			vars[k] = v
		}
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}
		switch style {
		case "make":
			if m := makeEvalRe.FindStringSubmatch(line); m != nil {
				set(m[1], m[2])
				continue
			}
			if strings.HasPrefix(line, "\t") {
				continue // recipe line
			}
			if m := makeAssignRe.FindStringSubmatch(line); m != nil && !strings.Contains(m[1], " ") {
				set(m[1], stripMakeComment(m[3]))
			}
		default:
			for _, m := range shellAssignRe.FindAllStringSubmatch(line, -1) {
				set(m[1], m[2])
			}
			if style == "yaml" {
				if m := yamlEnvRe.FindStringSubmatch(line); m != nil {
					set(m[1], m[2])
				}
			}
		}
	}
	return vars
}

func stripMakeComment(v string) string {
	if i := strings.Index(v, " #"); i >= 0 {
		return strings.TrimSpace(v[:i])
	}
	return v
}

// normalizeValue strips quotes and rewrites known build-time expressions
// into markers.
func normalizeValue(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		v = v[1 : len(v)-1]
	}
	v = actionsDefaultRe.ReplaceAllString(v, "$1")
	v = gitDescribeRe.ReplaceAllString(v, markGitTag)
	v = gitRevParseRe.ReplaceAllString(v, markGitCommit)
	v = catVersionRe.ReplaceAllString(v, markVersionFile)
	v = commitVarRe.ReplaceAllString(v, markGitCommit)
	v = tagVarRe.ReplaceAllString(v, markGitTag)
	return v
}

var varRefRe = regexp.MustCompile(`\$\(([A-Za-z_][A-Za-z0-9_.-]*)\)|\$\{([A-Za-z_][A-Za-z0-9_]*)(?::?[-=]([^}]*))?\}|\$\{\{\s*(?:env|vars|inputs)\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// expand substitutes known variables (recursively, bounded) and markers.
func (vars varTable) expand(s string) string {
	s = normalizeValue(s)
	for depth := 0; depth < 6; depth++ {
		changed := false
		s = varRefRe.ReplaceAllStringFunc(s, func(ref string) string {
			m := varRefRe.FindStringSubmatch(ref)
			name := m[1] + m[2] + m[4] + m[5]
			if v, ok := vars[name]; ok && v != ref {
				changed = true
				return v
			}
			if m[2] != "" && m[3] != "" { // ${NAME:-default}
				changed = true
				return m[3]
			}
			return ref
		})
		if !changed {
			break
		}
	}
	return s
}

// merge adds entries of o that are not yet defined.
func (vars varTable) merge(o varTable) {
	for k, v := range o {
		if _, ok := vars[k]; !ok {
			vars[k] = v
		}
	}
}
