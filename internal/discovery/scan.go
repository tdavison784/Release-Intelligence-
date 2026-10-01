package discovery

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Profile selects which detectors run on a tree.
type Profile string

const (
	// ProfileSource scans the product repository (every detector).
	ProfileSource Profile = "source"
	// ProfileDocs scans an external documentation repository: listing-based
	// detectors plus documentation detectors on a narrow file selection.
	ProfileDocs Profile = "docs"
)

// ProductHints are tokens that identify the product (repository owner and
// name, product id), used to tell product artifacts from third-party ones.
type ProductHints struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
	ID    string `json:"id"`
}

// Tokens returns distinctive lowercase tokens.
func (h ProductHints) Tokens() []string {
	set := map[string]bool{}
	for _, s := range []string{h.Owner, h.Name, h.ID} {
		s = strings.ToLower(s)
		if len(s) >= 3 {
			set[s] = true
		}
		for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == '.' }) {
			if len(p) >= 4 && !genericWords[p] {
				set[p] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

var genericWords = map[string]bool{"project": true, "operator": true, "manager": true, "server": true, "controller": true, "helm": true, "charts": true, "docs": true}

// ScanInput parameterises one scan.
type ScanInput struct {
	Profile Profile
	Product ProductHints
	// Main is the product repository (used to recognise self references
	// when scanning other trees).
	Main RepoRef
	// Tags of the product (may be nil); used for templating and coverage.
	Tags *TagAnalysis
	// RefVersion is the release the main tree was checked out at (zero
	// when scanning a branch).
	RefVersion domain.Version
}

// ScanResult is the outcome of scanning one tree.
type ScanResult struct {
	Tree       TreeInfo    `json:"tree"`
	Profile    Profile     `json:"profile"`
	Listed     int         `json:"filesListed"`
	Read       int         `json:"filesRead"`
	Candidates []Candidate `json:"-"`
}

// Scanner runs detectors over a tree.
type Scanner struct {
	// MaxFileSize skips larger files (default 8 MiB).
	MaxFileSize int64
	// MaxFiles bounds the number of files read (default 30000 for source
	// trees, 2500 for docs trees).
	MaxFiles int
}

// File is a file handed to detectors.
type File struct {
	Path   string
	Data   []byte
	Digest string
	info   TreeInfo
	lines  []string
}

// Text returns the content as a string.
func (f *File) Text() string { return string(f.Data) }

// Lines returns the content split into lines (without terminators).
func (f *File) Lines() []string {
	if f.lines == nil {
		f.lines = strings.Split(strings.ReplaceAll(string(f.Data), "\r\n", "\n"), "\n")
	}
	return f.lines
}

// URI returns the blob URI of the file pinned to the scanned ref.
func (f *File) URI() string { return f.info.Repo.BlobURL(f.info.PinRef, f.Path) }

// Evidence returns repo-file evidence for 1-based line n.
func (f *File) Evidence(n int) domain.Evidence {
	lines := f.Lines()
	excerpt := ""
	if n >= 1 && n <= len(lines) {
		excerpt = lines[n-1]
	}
	return domain.NewEvidence(domain.EvidenceRepoFile, "", f.URI(), "L"+itoa(n), excerpt, f.Digest, f.info.RetrievedAt)
}

// fileEvidence is evidence for a whole file known from the tree listing.
func fileEvidence(info TreeInfo, p, note string) domain.Evidence {
	return domain.NewEvidence(domain.EvidenceRepoFile, "", info.Repo.BlobURL(info.PinRef, p), "", note, "", info.RetrievedAt)
}

// scanState is the per-scan working state shared by detectors.
type scanState struct {
	in        ScanInput
	info      TreeInfo
	files     []FileEntry
	fileSet   map[string]bool
	cands     map[string]*Candidate
	order     []string
	makeVars  varTable
	callbacks map[string][]func(*File)
	extra     map[string]bool // paths listing detectors want read
}

func (st *scanState) has(p string) bool { return st.fileSet[p] }

// emit records a candidate (merging with an existing one of the same kind
// and value). Candidates from trees other than the main repository carry
// the repository and ref they come from.
func (st *scanState) emit(kind CandidateKind, value string, conf domain.Confidence, rule string, attrs map[string]string, ev ...domain.Evidence) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	a := map[string]string{}
	for k, v := range attrs {
		if v != "" {
			a[k] = v
		}
	}
	if v, ok := attrs["contexts"]; ok && v == "" {
		a["contexts"] = "none" // record it so that mixed contexts are visible
	}
	if !st.info.Repo.Same(st.in.Main) {
		a["repo"] = st.info.Repo.String()
		a["ref"] = st.info.Ref
	}
	c := Candidate{Kind: kind, Value: value, Attributes: a, Confidence: conf, Rules: []string{rule}, Evidence: nil}
	for _, e := range ev {
		if e.URI != "" {
			c.Evidence = append(c.Evidence, e)
		}
	}
	key := string(kind) + "\x00" + value
	if cur, ok := st.cands[key]; ok {
		cur.merge(c)
		return
	}
	c.ID = candidateID(kind, value)
	c.Attributes["occurrences"] = "1"
	if len(c.Evidence) > maxEvidencePerCandidate {
		c.Evidence = c.Evidence[:maxEvidencePerCandidate]
	}
	st.cands[key] = &c
	st.order = append(st.order, key)
}

// requestRead asks for a file's content to be read in the content phase
// and passes it to fn.
func (st *scanState) requestRead(p string, fn func(*File)) {
	st.extra[p] = true
	if fn != nil {
		st.callbacks[p] = append(st.callbacks[p], fn)
	}
}

// Context classes of a path.
const (
	ctxNone   = ""
	ctxTest   = "test"
	ctxSample = "sample"
	ctxVendor = "vendor"
	ctxDocs   = "docs"
)

var (
	testWords   = map[string]bool{"test": true, "tests": true, "e2e": true, "testdata": true, "test-data": true, "fixtures": true, "fixture": true, "integration": true, "testing": true, "mocks": true, "mock": true, "benchmark": true, "benchmarks": true, "conformance": true}
	sampleWords = map[string]bool{"samples": true, "sample": true, "examples": true, "example": true, "demo": true, "demos": true, "tutorials": true, "tutorial": true, "quickstart": true, "playground": true}
	devDirWords = map[string]bool{"e2e": true, "test": true, "tests": true, "dev": true, "tilt": true, "local": true, "testdata": true, "fixtures": true}
	vendorWords = map[string]bool{"vendor": true, "third_party": true, "third-party": true, "node_modules": true, "licenses": true, ".git": true}
	docExtRe    = regexp.MustCompile(`(?i)\.(md|mdx|markdown|rst|adoc|txt|html)$`)
)

// pathContext classifies a path as test, sample, vendor, docs or none.
func pathContext(p string) string {
	segs := strings.Split(strings.ToLower(p), "/")
	for _, s := range segs[:len(segs)-1] {
		switch {
		case vendorWords[s]:
			return ctxVendor
		case testWords[s]:
			return ctxTest
		case sampleWords[s]:
			return ctxSample
		}
		for _, tok := range strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == '.' }) {
			if devDirWords[tok] {
				return ctxTest
			}
		}
	}
	base := segs[len(segs)-1]
	for _, tok := range strings.FieldsFunc(base, func(r rune) bool { return r == '-' || r == '_' || r == '.' }) {
		if testWords[tok] || strings.HasSuffix(tok, "test") {
			return ctxTest
		}
	}
	if docExtRe.MatchString(base) || containsStr(segs, "docs") || containsStr(segs, "doc") || containsStr(segs, "design") {
		return ctxDocs
	}
	return ctxNone
}

// confFor lowers a confidence for test/sample contexts.
func confFor(ctx string, base domain.Confidence) domain.Confidence {
	switch ctx {
	case ctxTest, ctxSample:
		return domain.ConfidenceLow
	case ctxDocs:
		if base == domain.ConfidenceHigh {
			return domain.ConfidenceMedium
		}
	}
	return base
}

// detector is a content detector.
type detector struct {
	name     string
	profiles []Profile
	match    func(p string) bool
	run      func(st *scanState, f *File)
}

// listingDetector works on the file listing only.
type listingDetector struct {
	name     string
	profiles []Profile
	run      func(st *scanState)
}

func (d detector) enabled(p Profile) bool        { return containsProfile(d.profiles, p) }
func (d listingDetector) enabled(p Profile) bool { return containsProfile(d.profiles, p) }

func containsProfile(ps []Profile, p Profile) bool {
	for _, x := range ps {
		if x == p {
			return true
		}
	}
	return false
}

var both = []Profile{ProfileSource, ProfileDocs}
var sourceOnly = []Profile{ProfileSource}

// detectors returns the content detectors in execution order.
func detectors() []detector {
	return []detector{
		{"workflow", sourceOnly, isWorkflowFile, detectWorkflow},
		{"script", sourceOnly, isScriptFile, detectScript},
		{"make", sourceOnly, isMakeFile, detectMake},
		{"dockerfile", sourceOnly, isDockerfile, detectDockerfile},
		{"goreleaser", sourceOnly, isGoreleaserFile, detectGoreleaser},
		{"ko", sourceOnly, isKoFile, detectKo},
		{"chart", sourceOnly, isChartFile, detectChart},
		{"values", sourceOnly, isValuesFile, detectValues},
		{"yaml", both, isYAMLFile, detectYAML},
		{"docs", both, isDocFile, detectDocs},
		{"version-file", sourceOnly, func(p string) bool { return p == "VERSION" }, detectVersionFile},
	}
}

// listingDetectors returns detectors that only need the file listing.
func listingDetectors() []listingDetector {
	return []listingDetector{
		{"versioned-docs", both, detectVersionedDocs},
		{"notes-dir", sourceOnly, detectNotesDirs},
		{"security-bulletins", both, detectBulletinDirs},
	}
}

// Scan runs the detectors of in.Profile over tree.
func (s *Scanner) Scan(ctx context.Context, tree Tree, in ScanInput) (*ScanResult, error) {
	info := tree.Info()
	st := &scanState{
		in: in, info: info, files: tree.Files(), fileSet: map[string]bool{},
		cands: map[string]*Candidate{}, makeVars: varTable{}, callbacks: map[string][]func(*File){}, extra: map[string]bool{},
	}
	maxSize := s.MaxFileSize
	if maxSize <= 0 {
		maxSize = 8 << 20
	}
	maxFiles := s.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 30000
		if in.Profile == ProfileDocs {
			maxFiles = 2500
		}
	}
	var kept []FileEntry
	for _, f := range st.files {
		if pathContext(f.Path) == ctxVendor {
			continue
		}
		kept = append(kept, f)
	}
	st.files = kept
	if in.Profile == ProfileDocs {
		st.files = filterLocalizations(st.files)
	}
	for _, f := range st.files {
		st.fileSet[f.Path] = true
	}
	for _, ld := range listingDetectors() {
		if ld.enabled(in.Profile) {
			ld.run(st)
		}
	}
	dets := detectors()
	var want []string
	for _, f := range st.files {
		if f.Size > maxSize {
			continue
		}
		if st.extra[f.Path] {
			want = append(want, f.Path)
			continue
		}
		if in.Profile == ProfileDocs && !docsWorthReading(f.Path) {
			continue
		}
		for _, d := range dets {
			if d.enabled(in.Profile) && d.match(f.Path) {
				want = append(want, f.Path)
				break
			}
		}
	}
	if len(want) > maxFiles {
		want = want[:maxFiles]
	}
	contents, err := tree.ReadFiles(ctx, want)
	if err != nil {
		return nil, err
	}
	files := make([]*File, 0, len(want))
	for _, p := range want {
		data, ok := contents[p]
		if !ok || looksBinary(data) {
			continue
		}
		files = append(files, &File{Path: p, Data: data, Digest: domain.Digest(data), info: info})
	}
	// Make variables are global across included makefiles: collect first.
	for _, f := range files {
		if isMakeFile(f.Path) && pathContext(f.Path) != ctxTest {
			st.makeVars.merge(collectVars(f.Text(), "make"))
		}
	}
	for _, f := range files {
		for _, fn := range st.callbacks[f.Path] {
			fn(f)
		}
		for _, d := range dets {
			if d.enabled(in.Profile) && d.match(f.Path) && (in.Profile != ProfileDocs || docsWorthReading(f.Path)) {
				d.run(st, f)
			}
		}
	}
	res := &ScanResult{Tree: info, Profile: in.Profile, Listed: len(st.files), Read: len(files)}
	for _, k := range st.order {
		res.Candidates = append(res.Candidates, *st.cands[k])
	}
	return res, nil
}

func looksBinary(b []byte) bool {
	n := len(b)
	if n > 8000 {
		n = 8000
	}
	for _, c := range b[:n] {
		if c == 0 {
			return true
		}
	}
	return false
}

var docsKeywordRe = regexp.MustCompile(`(?i)(release|upgrad|migrat|compat|support|security|advisor|bulletin|install|helm|changelog|changes|version|getting-started|readme)`)

// docsWorthReading limits content reads in documentation repositories to
// files whose path suggests release information.
func docsWorthReading(p string) bool {
	low := strings.ToLower(p)
	ext := path.Ext(low)
	switch ext {
	case ".md", ".mdx", ".markdown", ".rst", ".adoc", ".yaml", ".yml", ".json", ".toml":
	default:
		return false
	}
	return docsKeywordRe.MatchString(low)
}

var localeRe = regexp.MustCompile(`^[a-z]{2}(?:[-_][a-z]{2,4})?$`)

// filterLocalizations keeps only the English variant of localised content
// trees (a directory whose children include "en" and other locale codes).
func filterLocalizations(files []FileEntry) []FileEntry {
	children := map[string]map[string]bool{}
	for _, f := range files {
		segs := strings.Split(f.Path, "/")
		for i := 0; i < len(segs)-1; i++ {
			parent := strings.Join(segs[:i], "/")
			if children[parent] == nil {
				children[parent] = map[string]bool{}
			}
			children[parent][segs[i]] = true
		}
	}
	localeRoots := map[string]bool{}
	for parent, kids := range children {
		if !kids["en"] {
			continue
		}
		others := 0
		for k := range kids {
			if k != "en" && localeRe.MatchString(k) {
				others++
			}
		}
		if others > 0 {
			localeRoots[parent] = true
		}
	}
	if len(localeRoots) == 0 {
		return files
	}
	var out []FileEntry
	for _, f := range files {
		segs := strings.Split(f.Path, "/")
		drop := false
		for i := 0; i < len(segs)-1; i++ {
			parent := strings.Join(segs[:i], "/")
			if localeRoots[parent] && segs[i] != "en" && localeRe.MatchString(segs[i]) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, f)
		}
	}
	return out
}

// selfRepo reports whether host/owner/name refers to the product repository.
func (st *scanState) selfRepo(owner, name string) bool {
	return strings.EqualFold(owner, st.in.Main.Owner) && strings.EqualFold(strings.TrimSuffix(name, ".git"), st.in.Main.Name)
}

// productRelated reports whether s mentions a product token.
func (st *scanState) productRelated(s string) bool {
	s = strings.ToLower(s)
	for _, t := range st.in.Product.Tokens() {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

// refTag returns the scanned release tag and version ("" for branches).
func (st *scanState) refTag() (string, string) {
	if st.in.RefVersion.IsZero() || !st.info.Repo.Same(st.in.Main) {
		return "", ""
	}
	return st.in.RefVersion.Tag, st.in.RefVersion.Semver
}

func (st *scanState) tagPrefix() string {
	if st.in.Tags != nil {
		return st.in.Tags.Prefix
	}
	return "v"
}
