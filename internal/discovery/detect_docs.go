package discovery

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func isDocFile(p string) bool {
	base := strings.ToUpper(path.Base(p))
	return docExtRe.MatchString(p) || strings.HasPrefix(base, "README") || strings.HasPrefix(base, "CHANGELOG") ||
		strings.HasPrefix(base, "SECURITY") || strings.HasPrefix(base, "CHANGES")
}

var (
	changelogNameRe   = regexp.MustCompile(`(?i)^(changelog|changes|history|news|releases?)(\.[a-z]+)?$`)
	versionHeadingRe  = regexp.MustCompile(`^#{1,4}\s+\[?` + "`?" + `v?(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)`)
	securityNameRe    = regexp.MustCompile(`(?i)^security(\.md|\.rst|\.txt)?$`)
	advisoriesLinkRe  = regexp.MustCompile(`(?i)github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/security/advisories|security advisor(y|ies)`)
	repoURLRe         = regexp.MustCompile(`github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?(?:/(?:tree|blob)/[^/\s)]+/([^\s)#"'` + "`" + `\]]+))?(?:[\s)#"'` + "`" + `\]]|$)`)
	backtickRepoRe    = regexp.MustCompile("`([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)`")
	docsRepoNameRe    = regexp.MustCompile(`(?i)(website|docs?$|^docs|documentation|site$|www|homepage|\.io$|\.dev$|\.org$)`)
	chartRepoNameRe   = regexp.MustCompile(`(?i)(helm|charts?$)`)
	markdownLinkRe    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)|\[([^\]]*)\]\[[^\]]*\]`)
	tableSeparatorRe  = regexp.MustCompile(`^:?-{3,}:?$`)
	headingRe         = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	frontMatterTitle  = regexp.MustCompile(`^title:\s*(.+)$`)
	kubeHeaderRe      = regexp.MustCompile(`(?i)kubernetes|k8s`)
	securityWebLinkRe = regexp.MustCompile(`https?://[^\s)\]]*(security|vulnerab|advisor|bulletin|support)[^\s)\]]*`)
)

func detectDocs(st *scanState, f *File) {
	ctx := pathContext(f.Path)
	base := path.Base(f.Path)
	stem := strings.TrimSuffix(base, path.Ext(base))
	if changelogNameRe.MatchString(base) || changelogNameRe.MatchString(stem) {
		detectChangelog(st, f, ctx)
	}
	if securityNameRe.MatchString(base) && ctx != ctxTest && ctx != ctxSample {
		detectSecurityPolicy(st, f)
	}
	inFence := false
	for i, line := range f.Lines() {
		n := i + 1
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		st.repoRefs(f, n, line, ctx)
		st.helmRepoRefs(f, n, line, ctx, "docs")
		st.assetURLs(f, n, line, "docs", ctx)
		for _, r := range findImageRefs(line) {
			isChart := r.OCI || strings.Contains("/"+r.Path+"/", "/charts/")
			if !isChart && !st.productRelated(r.Repository()) {
				continue
			}
			st.imageRef(f, n, r, "docs", ctx, "", domain.ConfidenceMedium)
		}
		for _, rr := range rawRefs(line) {
			if rr.owner != "" && st.selfRepo(rr.owner, rr.name) {
				st.emit(KindManifest, rr.path, confFor(ctx, domain.ConfidenceMedium), "docs.raw-url",
					map[string]string{"served": "raw-at-tag", "files": f.Path, "contexts": ctx}, f.Evidence(n))
			}
		}
	}
	if ctx != ctxTest && ctx != ctxSample {
		detectCompatTables(st, f)
	}
}

func detectChangelog(st *scanState, f *File, ctx string) {
	if ctx == ctxTest || ctx == ctxSample {
		return
	}
	var newest *semver.Version
	newestLine, entries := 0, 0
	for i, l := range f.Lines() {
		m := versionHeadingRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		entries++
		v, err := semver.NewVersion(m[1])
		if err != nil {
			continue
		}
		if newest == nil || v.GreaterThan(newest) {
			newest, newestLine = v, i+1
		}
		if entries > 40 {
			break
		}
	}
	if entries < 2 {
		return
	}
	attrs := map[string]string{"entries": itoa(entries), "newest": newest.Original(), "contexts": ctx}
	if hl := f.Lines()[newestLine-1]; hl != "" {
		attrs["headingSample"] = strings.TrimSpace(hl)
	}
	st.emit(KindChangelog, f.Path, domain.ConfidenceHigh, "docs.changelog", attrs, f.Evidence(newestLine))
}

func detectSecurityPolicy(st *scanState, f *File) {
	var links []string
	advisoryLine := 0
	for i, l := range f.Lines() {
		if m := advisoriesLinkRe.FindStringSubmatch(l); m != nil && advisoryLine == 0 {
			if m[1] == "" || st.selfRepo(m[1], m[2]) {
				advisoryLine = i + 1
			}
		}
		for _, u := range securityWebLinkRe.FindAllString(l, -1) {
			links = append(links, strings.TrimRight(u, ".,;:"))
		}
		for _, m := range repoURLRe.FindAllStringSubmatch(l, -1) {
			if m[3] != "" && strings.Contains(strings.ToUpper(m[3]), "SECURITY") {
				links = append(links, "https://github.com/"+m[1]+"/"+m[2]+"/blob/…/"+m[3])
			}
		}
	}
	attrs := map[string]string{"links": strings.Join(dedupe(links), " ")}
	st.emit(KindSecurityPolicy, f.Path, domain.ConfidenceHigh, "docs.security-policy", attrs, f.Evidence(1))
	if advisoryLine > 0 && st.in.Main.IsGitHub() {
		st.emit(KindAdvisories, st.in.Main.Slug(), domain.ConfidenceHigh, "docs.security-advisories-link",
			map[string]string{"files": f.Path}, f.Evidence(advisoryLine))
	}
}

// repoRefs records references to sibling repositories of the product
// (documentation websites, Helm chart repositories).
func (st *scanState) repoRefs(f *File, n int, line, ctx string) {
	type ref struct{ owner, name, sub string }
	var refs []ref
	for _, m := range repoURLRe.FindAllStringSubmatch(line, -1) {
		refs = append(refs, ref{m[1], m[2], m[3]})
	}
	for _, m := range backtickRepoRe.FindAllStringSubmatch(line, -1) {
		refs = append(refs, ref{m[1], m[2], ""})
	}
	for _, r := range refs {
		name := strings.TrimSuffix(r.name, ".git")
		if st.selfRepo(r.owner, name) || !strings.EqualFold(r.owner, st.in.Main.Owner) {
			continue
		}
		value := "github.com/" + r.owner + "/" + name
		conf := confFor(ctx, domain.ConfidenceMedium)
		switch {
		case chartRepoNameRe.MatchString(name):
			attrs := map[string]string{"files": f.Path}
			if r.sub != "" {
				p := strings.TrimSuffix(r.sub, "/")
				for _, suffix := range []string{"/values.yaml", "/Chart.yaml", "/README.md"} {
					p = strings.TrimSuffix(p, suffix)
				}
				attrs["paths"] = p
			}
			st.emit(KindChartRepo, value, conf, "docs.chart-repo-ref", attrs, f.Evidence(n))
		case docsRepoNameRe.MatchString(name):
			if strings.Contains(line, "/issues") && !strings.Contains(strings.ToLower(line), "repo") {
				conf = domain.ConfidenceLow
			}
			st.emit(KindDocsRepo, value, conf, "docs.docs-repo-ref", map[string]string{"files": f.Path}, f.Evidence(n))
		}
	}
}

// mdTable is a parsed markdown table.
type mdTable struct {
	header  []string
	rows    [][]string
	line    int // 1-based line of the header
	heading string
}

func splitRow(l string) []string {
	l = strings.TrimSpace(l)
	l = strings.TrimPrefix(l, "|")
	l = strings.TrimSuffix(l, "|")
	l = strings.ReplaceAll(l, `\|`, "\x00")
	cells := strings.Split(l, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(strings.ReplaceAll(cells[i], "\x00", "|"))
	}
	return cells
}

func parseMarkdownTables(lines []string) []mdTable {
	var out []mdTable
	heading := ""
	for i := 0; i < len(lines); i++ {
		if m := headingRe.FindStringSubmatch(lines[i]); m != nil {
			heading = strings.TrimSpace(m[2])
		}
		if !strings.Contains(lines[i], "|") || i+1 >= len(lines) {
			continue
		}
		sep := splitRow(lines[i+1])
		ok := len(sep) > 0
		for _, c := range sep {
			if !tableSeparatorRe.MatchString(strings.ReplaceAll(c, " ", "")) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		t := mdTable{header: splitRow(lines[i]), line: i + 1, heading: heading}
		j := i + 2
		for ; j < len(lines) && strings.Contains(lines[j], "|"); j++ {
			t.rows = append(t.rows, splitRow(lines[j]))
		}
		out = append(out, t)
		i = j - 1
	}
	return out
}

// cleanCell strips markdown links and emphasis.
func cleanCell(s string) string {
	s = markdownLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		sm := markdownLinkRe.FindStringSubmatch(m)
		return sm[1] + sm[2]
	})
	return strings.TrimSpace(strings.Trim(s, "*_`"))
}

// detectCompatTables finds markdown tables keyed by product versions with a
// Kubernetes column. The key column is the first of the leading columns whose
// cells are product version lines — some projects put a status icon or the
// supported flag before the version (ingress-nginx "Supported | Ingress-NGINX
// version | k8s supported version | …").
func detectCompatTables(st *scanState, f *File) {
	tables := parseMarkdownTables(f.Lines())
	if len(tables) == 0 {
		return
	}
	keyCols, supported, tested := map[string]bool{}, map[string]bool{}, map[string]bool{}
	sepPart := map[string]int{}
	firstLine, rows := 0, 0
	var headings []string
	for _, t := range tables {
		keyCol := -1
		versionKeys := 0
		scanCols := len(t.header)
		if scanCols > 3 {
			scanCols = 3
		}
		for c := 0; c < scanCols; c++ {
			n := 0
			for _, r := range t.rows {
				if len(r) > c && st.isProductLineCell(cleanCell(r[c])) {
					n++
				}
			}
			if n > versionKeys {
				keyCol, versionKeys = c, n
			}
		}
		if keyCol < 0 {
			continue // keyed by something else (vendors, other products)
		}
		found := false
		for i, h := range t.header {
			hc := cleanCell(h)
			if i <= keyCol || !kubeHeaderRe.MatchString(hc) {
				continue
			}
			found = true
			low := strings.ToLower(hc)
			if strings.Contains(low, "tested") && !strings.Contains(low, "not supported") {
				tested[hc] = true
			} else {
				supported[hc] = true
			}
			if strings.Contains(hc, "/") {
				for pi, part := range strings.Split(hc, "/") {
					if kubeHeaderRe.MatchString(part) {
						sepPart[hc] = pi
					}
				}
			}
		}
		if !found {
			continue
		}
		keyCols[cleanCell(t.header[keyCol])] = true
		rows += versionKeys
		if firstLine == 0 {
			firstLine = t.line
		}
		if t.heading != "" {
			headings = append(headings, t.heading)
		}
	}
	if len(keyCols) == 0 {
		return
	}
	attrs := map[string]string{
		"format": "markdown-table", "keyColumns": joinKeys(keyCols), "supportedHeaders": joinKeys(supported),
		"testedHeaders": joinKeys(tested), "rows": itoa(rows), "headings": strings.Join(dedupe(headings), "; "),
	}
	var parts []string
	for h, p := range sepPart {
		parts = append(parts, h+"="+itoa(p))
	}
	sort.Strings(parts)
	if len(parts) > 0 {
		attrs["separatorParts"] = strings.Join(parts, ";")
	}
	conf := domain.ConfidenceMedium
	if rows >= 2 {
		conf = domain.ConfidenceHigh
	}
	if docRole(f.Path) == roleCompat {
		attrs["pathRole"] = "true"
	}
	st.emit(KindCompatibility, f.Path, conf, "compat.markdown-table", attrs, f.Evidence(firstLine))
}

// isProductLineCell reports whether a table key cell names a release line of
// the product (when tags are known; any version-like token otherwise).
func (st *scanState) isProductLineCell(cell string) bool {
	toks := findVersionTokens(cell)
	if len(toks) == 0 {
		return false
	}
	if st.in.Tags == nil || len(st.in.Tags.stable) == 0 {
		return true
	}
	t := toks[0]
	return st.in.Tags.HasLine(t.major, t.minor)
}

// titleEvidence returns evidence for the title of a document (front matter
// title or first heading).
func titleEvidence(f *File) (domain.Evidence, string, bool) {
	for i, l := range f.Lines() {
		if i > 40 {
			break
		}
		if m := frontMatterTitle.FindStringSubmatch(l); m != nil {
			return f.Evidence(i + 1), strings.Trim(m[1], `"'`), true
		}
		if m := headingRe.FindStringSubmatch(l); m != nil {
			return f.Evidence(i + 1), m[2], true
		}
	}
	return domain.Evidence{}, "", false
}
