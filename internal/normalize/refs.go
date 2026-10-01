package normalize

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

var (
	cveRe  = regexp.MustCompile(`(?i)\bCVE-\d{4}-\d{4,}\b`)
	ghsaRe = regexp.MustCompile(`(?i)\bGHSA(?:-[2-9cfghjmpqrvwx]{4}){3}\b`)
	// https://github.com/owner/repo/pull/123 and .../issues/123
	ghURLRe = regexp.MustCompile(`(?i)https?://(?:www\.)?github\.com/([A-Za-z0-9][A-Za-z0-9-]*)/([A-Za-z0-9_.-]+)/(pull|issues)/(\d+)`)
	// owner/repo#123
	crossRefRe = regexp.MustCompile(`(?:^|[^\w/.#-])([A-Za-z][A-Za-z0-9-]*)/([A-Za-z0-9][A-Za-z0-9_.-]*)#(\d{1,7})\b`)
	// bare #123, not part of a word, an HTML entity, a URL path or fragment
	bareRefRe = regexp.MustCompile(`(?:^|[^\w&/#=])#(\d{1,7})\b`)
)

type foundRef struct {
	pos int
	ref domain.Reference
}

// normalizeRepo reduces "owner/name", "github.com/owner/name" and
// "https://github.com/owner/name.git" to "owner/name"; any other host yields
// "".
func normalizeRepo(repo string) string {
	r := strings.TrimSpace(repo)
	if r == "" {
		return ""
	}
	r = strings.TrimPrefix(strings.TrimPrefix(r, "https://"), "http://")
	r = strings.TrimSuffix(strings.TrimSuffix(r, "/"), ".git")
	parts := strings.Split(r, "/")
	if len(parts) == 3 && strings.EqualFold(parts[0], "github.com") {
		parts = parts[1:]
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(parts[0], ".") {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

func extractReferences(text, repository string) []domain.Reference {
	if text == "" {
		return nil
	}
	var found []foundRef
	seen := map[string]bool{}
	add := func(pos int, r domain.Reference) {
		k := r.Type + "\x00" + strings.ToLower(r.ID)
		if seen[k] {
			return
		}
		seen[k] = true
		found = append(found, foundRef{pos, r})
	}

	for _, m := range cveRe.FindAllStringIndex(text, -1) {
		id := strings.ToUpper(text[m[0]:m[1]])
		add(m[0], domain.Reference{Type: "cve", ID: id, URL: "https://www.cve.org/CVERecord?id=" + id})
	}
	for _, m := range ghsaRe.FindAllStringIndex(text, -1) {
		raw := text[m[0]:m[1]]
		id := "GHSA" + strings.ToLower(raw[4:])
		add(m[0], domain.Reference{Type: "ghsa", ID: id, URL: "https://github.com/advisories/" + id})
	}

	// GitHub URLs first, so that "#123" mentions of the same item are folded.
	covered := map[string]bool{}
	for _, m := range ghURLRe.FindAllStringSubmatchIndex(text, -1) {
		owner, repo := text[m[2]:m[3]], strings.TrimSuffix(text[m[4]:m[5]], ".git")
		kind, num := strings.ToLower(text[m[6]:m[7]]), text[m[8]:m[9]]
		typ := "issue"
		if kind == "pull" {
			typ = "pull-request"
		}
		id := owner + "/" + repo + "#" + num
		covered[strings.ToLower(id)] = true
		add(m[0], domain.Reference{Type: typ, ID: id, URL: text[m[0]:m[1]]})
	}

	for _, m := range crossRefRe.FindAllStringSubmatchIndex(text, -1) {
		owner, repo, num := text[m[2]:m[3]], text[m[4]:m[5]], text[m[6]:m[7]]
		id := owner + "/" + repo + "#" + num
		if covered[strings.ToLower(id)] {
			continue
		}
		covered[strings.ToLower(id)] = true
		add(m[2], domain.Reference{Type: "github-ref", ID: id, URL: "https://github.com/" + owner + "/" + repo + "/issues/" + num})
	}

	if repo := normalizeRepo(repository); repo != "" {
		for _, m := range bareRefRe.FindAllStringSubmatchIndex(text, -1) {
			num := text[m[2]:m[3]]
			if n, err := strconv.Atoi(num); err != nil || n == 0 {
				continue
			}
			id := repo + "#" + num
			if covered[strings.ToLower(id)] {
				continue
			}
			covered[strings.ToLower(id)] = true
			add(m[2]-1, domain.Reference{Type: "github-ref", ID: id, URL: "https://github.com/" + repo + "/issues/" + num})
		}
	}

	if len(found) == 0 {
		return nil
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].pos < found[j].pos })
	out := make([]domain.Reference, len(found))
	for i, f := range found {
		out[i] = f.ref
	}
	return out
}
