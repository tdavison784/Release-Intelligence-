package discovery

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Documentation roles inferred from paths.
const (
	roleUpgrade  = "upgrade-guide"
	roleCompat   = "compatibility"
	roleSecurity = "security"
	roleNotes    = "release-notes"
)

var (
	upgradeRoleRe  = regexp.MustCompile(`(?i)upgrad|migrat`)
	compatRoleRe   = regexp.MustCompile(`(?i)compatib|supported[-_ ]?(releases|versions)|support[-_ ]?(matrix|status|policy)|tested[-_]kubernetes|version[-_]skew|kubernetes[-_]versions|supportstatus`)
	securityRoleRe = regexp.MustCompile(`(?i)security|advisor|bulletin|(^|/)cve`)
	notesRoleRe    = regexp.MustCompile(`(?i)release[-_ ]?notes|releasenotes|change[-_]?notes|changelog|news/releases|announc|whats[-_]?new|(^|/)releases?(/|$)|(^|/)changes(/|$)`)
)

// docRole classifies a documentation path; the most specific role wins.
func docRole(p string) string {
	switch {
	case upgradeRoleRe.MatchString(p):
		return roleUpgrade
	case compatRoleRe.MatchString(p):
		return roleCompat
	case securityRoleRe.MatchString(p):
		return roleSecurity
	case notesRoleRe.MatchString(p):
		return roleNotes
	}
	return ""
}

func roleKind(role string) CandidateKind {
	switch role {
	case roleUpgrade:
		return KindUpgradeGuide
	case roleCompat:
		return KindCompatibility
	case roleSecurity:
		return KindSecurityDocs
	}
	return KindReleaseNotes
}

type pathInstance struct {
	path string
	pt   pathTemplate
}

func instanceLess(a, b pathInstance) bool {
	if a.pt.Major != b.pt.Major {
		return a.pt.Major < b.pt.Major
	}
	if a.pt.Minor != b.pt.Minor {
		return a.pt.Minor < b.pt.Minor
	}
	return a.pt.Patch < b.pt.Patch
}

// detectVersionedDocs groups documentation files whose paths embed release
// versions into path templates (one candidate per template).
func detectVersionedDocs(st *scanState) {
	groups := map[string][]pathInstance{}
	roles := map[string]string{}
	for _, f := range st.files {
		if !docExtRe.MatchString(f.Path) {
			continue
		}
		if c := pathContext(f.Path); c == ctxTest || c == ctxSample {
			continue
		}
		role := docRole(f.Path)
		// security pages are not per-release documents (bulletin directories
		// are found by detectBulletinDirs); archived doc sites duplicate
		// current ones
		if role == "" || role == roleSecurity || archivedDocRe.MatchString(f.Path) {
			continue
		}
		pt, ok := inferPathTemplate(f.Path, st.tagPrefix())
		if !ok {
			continue
		}
		groups[pt.Template] = append(groups[pt.Template], pathInstance{f.Path, pt})
		roles[pt.Template] = role
	}
	tmpls := make([]string, 0, len(groups))
	for t := range groups {
		tmpls = append(tmpls, t)
	}
	sort.Strings(tmpls)
	var recent []string
	if st.in.Tags != nil {
		recent = st.in.Tags.Lines
		if len(recent) > 5 {
			recent = recent[:5]
		}
	}
	for _, tmpl := range tmpls {
		insts := groups[tmpl]
		if len(insts) < 2 {
			continue
		}
		sort.Slice(insts, func(i, j int) bool { return instanceLess(insts[i], insts[j]) })
		lines := map[string]bool{}
		zeroPatch := false
		for _, in := range insts {
			lines[lineOf(in.pt)] = true
			if in.pt.Patch == 0 {
				zeroPatch = true
			}
		}
		covered := 0
		for _, l := range recent {
			if lines[l] {
				covered++
			}
		}
		if len(recent) > 0 && covered == 0 {
			continue // documents versions that are not this product's
		}
		newest := insts[len(insts)-1]
		level := "line"
		switch {
		case newest.pt.Pair:
			level = "pair"
		case newest.pt.Patch >= 0:
			level = "patch"
		}
		var examples []string
		for i := len(insts) - 1; i >= 0 && len(examples) < 3; i-- {
			examples = append(examples, insts[i].path)
		}
		attrs := map[string]string{
			"instances": itoa(len(insts)), "lines": itoa(len(lines)), "covered": itoa(covered) + "/" + itoa(len(recent)),
			"newest": newest.path, "level": level, "examples": strings.Join(examples, " "),
		}
		if zeroPatch {
			attrs["zeroPatch"] = "true"
		}
		conf := domain.ConfidenceMedium
		if len(recent) > 0 && covered >= minInt(3, len(recent)) {
			conf = domain.ConfidenceHigh
		}
		role := roles[tmpl]
		kind := roleKind(role)
		st.emit(kind, tmpl, conf, "paths.versioned-"+role, attrs, fileEvidence(st.info, newest.path, "file "+newest.path))
		newestLine := newest.pt
		st.requestRead(newest.path, func(f *File) {
			ev, _, ok := titleEvidence(f)
			if !ok {
				return
			}
			extra := map[string]string{}
			if kind == KindReleaseNotes && newestLine.Patch < 0 {
				if h := versionSectionHeading(f, newestLine); h != "" {
					extra["sectionHeadings"] = "true"
					extra["headingSample"] = h
				}
			}
			st.emit(kind, tmpl, conf, "paths.versioned-"+role, extra, ev)
		})
	}
}

func lineOf(pt pathTemplate) string {
	return itoa(int(pt.Major)) + "." + itoa(int(pt.Minor))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// versionSectionHeading returns a heading of f naming a patch release of
// the line the file documents ("" when there is none), i.e. a per-line file
// with one section per release.
func versionSectionHeading(f *File, pt pathTemplate) string {
	for _, l := range f.Lines() {
		m := headingRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		for _, t := range findVersionTokens(strings.Trim(m[2], "`* ")) {
			if t.patch >= 0 && t.major == pt.Major && t.minor == pt.Minor {
				return strings.TrimSpace(l)
			}
		}
	}
	return ""
}

var notesDirRe = regexp.MustCompile(`(?i)(^|/)(release-?notes|releasenotes|changelogs?|changes|\.changes|changelog\.d|notes|unreleased|\.changeset)$`)

// detectNotesDirs finds directories of structured per-change note files.
func detectNotesDirs(st *scanState) {
	counts := map[string][]string{}
	for _, f := range st.files {
		if !isYAMLExt(f.Path) {
			continue
		}
		dir := path.Dir(f.Path)
		counts[dir] = append(counts[dir], f.Path)
	}
	dirs := make([]string, 0, len(counts))
	for d := range counts {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		files := counts[dir]
		if len(files) < 5 || !notesDirRe.MatchString(dir) {
			continue
		}
		if c := pathContext(dir + "/x"); c == ctxTest || c == ctxSample || c == ctxVendor {
			continue
		}
		dir, files := dir, files
		sample := files
		if len(sample) > 3 {
			sample = sample[len(sample)-3:]
		}
		attrs := map[string]string{"files": itoa(len(files)), "glob": "*" + path.Ext(files[0])}
		parent := path.Dir(dir)
		for _, cand := range []string{dir + "/template.yaml", parent + "/template.yaml", parent + "/README.md", dir + "/README.md"} {
			if st.has(cand) {
				attrs["docs"] = joinSet(attrs["docs"], cand)
			}
		}
		attrs["fileCount"] = attrs["files"]
		delete(attrs, "files")
		st.emit(KindNotesDir, dir, domain.ConfidenceMedium, "notes.structured-dir", attrs, fileEvidence(st.info, sample[0], "directory "+dir+" ("+itoa(len(files))+" files)"))
		for _, p := range sample {
			st.requestRead(p, func(f *File) {
				for i, l := range f.Lines() {
					if notesKeyRe.MatchString(l) {
						st.emit(KindNotesDir, dir, domain.ConfidenceHigh, "notes.structured-dir", map[string]string{"keys": strings.TrimSpace(strings.Split(l, ":")[0])}, f.Evidence(i+1))
						return
					}
				}
			})
		}
	}
}

var notesKeyRe = regexp.MustCompile(`^(releaseNotes|upgradeNotes|securityNotes|release-note|changelog|changes|breaking|description|summary)\s*:`)

var bulletinNameRe = regexp.MustCompile(`(?i)security-\d{4}-\d+|cve-\d{4}-\d+|ghsa-[a-z0-9]{4}-|advisory-\d+`)

// detectBulletinDirs finds directories of security bulletin pages.
func detectBulletinDirs(st *scanState) {
	counts := map[string]map[string]bool{}
	for _, f := range st.files {
		segs := strings.Split(f.Path, "/")
		for i := len(segs) - 1; i >= 1; i-- {
			if bulletinNameRe.MatchString(segs[i]) {
				dir := strings.Join(segs[:i], "/")
				if counts[dir] == nil {
					counts[dir] = map[string]bool{}
				}
				counts[dir][segs[i]] = true
				break
			}
		}
	}
	dirs := make([]string, 0, len(counts))
	for d := range counts {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		names := counts[d]
		if len(names) < 5 {
			continue
		}
		if c := pathContext(d + "/x"); c == ctxTest || c == ctxSample || archivedDocRe.MatchString(d) {
			continue
		}
		var list []string
		for n := range names {
			list = append(list, n)
		}
		sort.Strings(list)
		newest := list[len(list)-1]
		st.emit(KindSecurityDocs, d, domain.ConfidenceMedium, "paths.security-bulletins",
			map[string]string{"entries": itoa(len(names)), "newest": newest}, fileEvidence(st.info, d+"/"+newest, "bulletin "+newest))
	}
}
