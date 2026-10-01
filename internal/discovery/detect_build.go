package discovery

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func isMakeFile(p string) bool {
	base := path.Base(p)
	return base == "Makefile" || base == "GNUmakefile" || strings.HasPrefix(base, "Makefile.") || strings.HasSuffix(base, ".mk")
}

func isDockerfile(p string) bool {
	base := path.Base(p)
	return base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile.") || strings.HasSuffix(strings.ToLower(base), ".dockerfile")
}

func isGoreleaserFile(p string) bool {
	base := strings.ToLower(path.Base(p))
	return regexp.MustCompile(`^\.?goreleaser[\w.-]*\.ya?ml$`).MatchString(base)
}

func isKoFile(p string) bool { return path.Base(p) == ".ko.yaml" || path.Base(p) == ".ko.yml" }

var (
	helmPackageRe   = regexp.MustCompile(`(?i)helm\S*\)?\s+package\b`)
	flagValueRe     = regexp.MustCompile(`--(version|app-version)[= ]+['"]?([^\s'"]+)`)
	yqChartStampRe  = regexp.MustCompile(`\.(version|appVersion)\s*=\s*"([^"]+)"`)
	versionAssignRe = regexp.MustCompile(`^\s*(?:export\s+)?VERSION\s*(\?=|:=|=)`)
)

func detectMake(st *scanState, f *File) {
	ctx := pathContext(f.Path)
	if ctx == ctxDocs {
		ctx = ctxNone
	}
	vars := st.makeVars
	for i, line := range f.Lines() {
		n := i + 1
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		el := vars.expand(line)
		for _, r := range findImageRefs(el) {
			st.imageRef(f, n, r, "make", ctx, "", domain.ConfidenceMedium)
		}
		if m := helmPushRe.FindStringSubmatch(el); m != nil {
			st.emit(KindHelmOCI, strings.TrimSuffix(strings.TrimPrefix(m[2], "oci://"), "/"), confFor(ctx, domain.ConfidenceMedium), "make.helm-push",
				map[string]string{"prefix": "true", "files": f.Path, "contexts": ctx}, f.Evidence(n))
		}
		if helmPackageRe.MatchString(line) {
			for _, m := range flagValueRe.FindAllStringSubmatch(el, -1) {
				subject := "chart.version"
				if m[1] == "app-version" {
					subject = "chart.appVersion"
				}
				st.versionRelation(f, n, subject, m[2], "make.helm-package", ctx)
			}
		}
		if strings.Contains(strings.ToLower(line), "chart") {
			for _, m := range yqChartStampRe.FindAllStringSubmatch(el, -1) {
				st.versionRelation(f, n, "chart."+m[1], m[2], "make.chart-stamp", ctx)
			}
		}
		for _, m := range ldflagsVersionRe.FindAllStringSubmatch(el, -1) {
			st.versionRelation(f, n, "binary.version", m[1], "make.ldflags-version", ctx)
		}
		if versionAssignRe.MatchString(line) && strings.Contains(vars.expand("$(VERSION)"), markGitTag) {
			st.versionRelation(f, n, "build.VERSION", markGitTag, "make.version-from-git", ctx)
		}
		st.assetURLs(f, n, el, "make", ctx)
	}
	st.registryVars(f, collectVars(f.Text(), "make"), ctx, "make")
}

// versionRelation records how a build value relates to the release tag.
func (st *scanState) versionRelation(f *File, n int, subject, value, rule, ctx string) {
	tmpl := ""
	switch {
	case strings.Contains(value, markGitTag):
		tmpl = tmplTag
	case strings.Contains(value, markVersionFile):
		tmpl = tmplVersion
	default:
		if refTag, refVer := st.refTag(); refTag != "" {
			_, tmpl = classifyTag(value, refTag, refVer)
		}
	}
	if tmpl == "" {
		return
	}
	st.emit(KindVersionRelation, subject+" = "+tmpl, confFor(ctx, domain.ConfidenceHigh), rule,
		map[string]string{"subject": subject, "template": tmpl, "files": f.Path, "contexts": ctx}, f.Evidence(n))
}

var genericDockerSuffix = map[string]bool{"dev": true, "tilt": true, "test": true, "ci": true, "local": true, "debug": true, "base": true, "builder": true, "build": true, "e2e": true, "release": true, "distroless": true, "ui": true}

func detectDockerfile(st *scanState, f *File) {
	ctx := pathContext(f.Path)
	base := path.Base(f.Path)
	name := ""
	switch {
	case strings.HasPrefix(base, "Dockerfile."):
		name = strings.TrimPrefix(base, "Dockerfile.")
	case strings.HasSuffix(strings.ToLower(base), ".dockerfile"):
		name = base[:len(base)-len(".dockerfile")]
	}
	name = strings.ToLower(name)
	if name == "" || genericDockerSuffix[name] || strings.Contains(name, "tilt") {
		return
	}
	line := 1
	for i, l := range f.Lines() {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(l)), "FROM") {
			line = i + 1
			break
		}
	}
	st.emit(KindImageName, name, domain.ConfidenceLow, "dockerfile.name",
		map[string]string{"dockerfile": f.Path, "contexts": ctx, "sources": "dockerfile"}, f.Evidence(line))
}

// goreleaser -----------------------------------------------------------------

type grBuild struct {
	ID     string   `yaml:"id"`
	Binary string   `yaml:"binary"`
	Goos   []string `yaml:"goos"`
	Goarch []string `yaml:"goarch"`
	Ignore []struct {
		Goos   string `yaml:"goos"`
		Goarch string `yaml:"goarch"`
	} `yaml:"ignore"`
}

type grConfig struct {
	ProjectName string    `yaml:"project_name"`
	Builds      []grBuild `yaml:"builds"`
	Archives    []struct {
		Builds       []string `yaml:"builds"`
		IDs          []string `yaml:"ids"`
		NameTemplate string   `yaml:"name_template"`
		Format       string   `yaml:"format"`
		Formats      []string `yaml:"formats"`
	} `yaml:"archives"`
	Checksum struct {
		NameTemplate string `yaml:"name_template"`
	} `yaml:"checksum"`
	Release struct {
		GitHub struct {
			Owner string `yaml:"owner"`
			Name  string `yaml:"name"`
		} `yaml:"github"`
		Prerelease string `yaml:"prerelease"`
		Header     string `yaml:"header"`
		Footer     string `yaml:"footer"`
		Disable    any    `yaml:"disable"`
	} `yaml:"release"`
	Changelog struct {
		Use    string `yaml:"use"`
		Skip   any    `yaml:"skip"`
		Groups []struct {
			Title string `yaml:"title"`
		} `yaml:"groups"`
	} `yaml:"changelog"`
	Dockers []struct {
		ImageTemplates []string `yaml:"image_templates"`
	} `yaml:"dockers"`
	DockersV2 []struct {
		Images []string `yaml:"images"`
		Tags   []string `yaml:"tags"`
	} `yaml:"dockers_v2"`
	Kos []struct {
		Repository   string   `yaml:"repository"`
		Repositories []string `yaml:"repositories"`
		Tags         []string `yaml:"tags"`
	} `yaml:"kos"`
}

var grTemplateRe = regexp.MustCompile(`\{\{-?\s*\.(ProjectName|Os|Arch|Tag|Version|Arm|Amd64)\s*-?\}\}`)

func detectGoreleaser(st *scanState, f *File) {
	var cfg grConfig
	if err := yaml.Unmarshal(f.Data, &cfg); err != nil {
		return
	}
	lineOf := func(substr string) int {
		for i, l := range f.Lines() {
			if strings.Contains(l, substr) {
				return i + 1
			}
		}
		return 1
	}
	attrs := map[string]string{"files": f.Path, "sources": "goreleaser"}
	st.emit(KindBuildTool, "goreleaser", domain.ConfidenceHigh, "goreleaser.config", attrs, f.Evidence(1))
	if cfg.Release.Disable == nil || fmt.Sprint(cfg.Release.Disable) == "false" {
		pa := map[string]string{"files": f.Path, "prerelease": cfg.Release.Prerelease}
		st.emit(KindReleasePublisher, "goreleaser", domain.ConfidenceHigh, "goreleaser.release", pa, f.Evidence(lineOf("release:")))
	}
	if cfg.Changelog.Use != "" || len(cfg.Changelog.Groups) > 0 {
		var groups []string
		for _, g := range cfg.Changelog.Groups {
			groups = append(groups, g.Title)
		}
		st.emit(KindReleaseNotes, "github-release-body", domain.ConfidenceHigh, "goreleaser.changelog",
			map[string]string{"generator": "goreleaser", "use": cfg.Changelog.Use, "groups": strings.Join(groups, "; "), "files": f.Path},
			f.Evidence(lineOf("changelog:")))
	}
	project := cfg.ProjectName
	if project == "" {
		project = st.info.Repo.Name
	}
	refTag, refVer := st.refTag()
	expand := func(tmpl, goos, goarch string) string {
		return grTemplateRe.ReplaceAllStringFunc(tmpl, func(m string) string {
			switch grTemplateRe.FindStringSubmatch(m)[1] {
			case "ProjectName":
				return project
			case "Os":
				return goos
			case "Arch":
				return goarch
			case "Tag":
				return tmplTag
			case "Version":
				return tmplVersion
			}
			return ""
		})
	}
	buildsByID := map[string]grBuild{}
	for _, b := range cfg.Builds {
		buildsByID[b.ID] = b
	}
	emitAsset := func(name, rule string, line int) {
		if strings.ContainsAny(name, "{}$") && !strings.Contains(name, tmplTag) && !strings.Contains(name, tmplVersion) {
			return
		}
		u := name
		if st.info.Repo.IsGitHub() {
			u = fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", st.info.Repo.Slug(), tmplTag, name)
		}
		st.emit(KindReleaseAsset, u, domain.ConfidenceHigh, rule, map[string]string{"files": f.Path, "file": name, "sources": "goreleaser"}, f.Evidence(line))
	}
	for _, a := range cfg.Archives {
		formats := a.Formats
		if a.Format != "" {
			formats = append(formats, a.Format)
		}
		if len(formats) == 0 {
			formats = []string{"tar.gz"}
		}
		ids := append(append([]string{}, a.IDs...), a.Builds...)
		var builds []grBuild
		for _, id := range ids {
			if b, ok := buildsByID[id]; ok {
				builds = append(builds, b)
			}
		}
		if len(builds) == 0 {
			builds = cfg.Builds
		}
		tmpl := strings.TrimSpace(a.NameTemplate)
		if tmpl == "" {
			tmpl = "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
		}
		line := lineOf("name_template")
		for _, b := range builds {
			for _, p := range platforms(b) {
				name := expand(tmpl, p[0], p[1])
				switch formats[0] {
				case "binary":
					if p[0] == "windows" {
						name += ".exe"
					}
				case "zip":
					name += ".zip"
				default:
					name += "." + formats[0]
				}
				emitAsset(name, "goreleaser.archive", line)
			}
		}
	}
	if cs := strings.TrimSpace(cfg.Checksum.NameTemplate); cs != "" {
		emitAsset(expand(cs, "", ""), "goreleaser.checksum", lineOf("checksum:"))
	}
	var imgs []string
	for _, d := range cfg.Dockers {
		imgs = append(imgs, d.ImageTemplates...)
	}
	for _, d := range cfg.DockersV2 {
		for _, im := range d.Images {
			for _, t := range d.Tags {
				imgs = append(imgs, im+":"+t)
			}
		}
	}
	for _, k := range cfg.Kos {
		for _, r := range append(k.Repositories, k.Repository) {
			if r != "" {
				imgs = append(imgs, r+":{{ .Tag }}")
			}
		}
	}
	for _, im := range imgs {
		line := lineOf(strings.Split(im, ":")[0])
		ex := expand(im, "", "")
		for _, r := range findImageRefs(strings.ReplaceAll(strings.ReplaceAll(ex, tmplTag, markGitTag), tmplVersion, markGitTag)) {
			if r.Partial {
				continue
			}
			class, tt := classifyTag(r.Tag, refTag, refVer)
			if strings.Contains(im, ".Version") {
				tt = tmplVersion
			}
			st.emit(KindImage, r.Repository(), domain.ConfidenceHigh, "goreleaser.image",
				map[string]string{"files": f.Path, "sources": "goreleaser", "classes": class, "tagTemplate": tt, "triggers": triggerTag}, f.Evidence(line))
		}
	}
	for _, text := range []string{cfg.Release.Header, cfg.Release.Footer} {
		for _, m := range rawRefs(text) {
			if m.owner != "" && !st.selfRepo(m.owner, m.name) {
				continue
			}
			st.emit(KindManifest, m.path, domain.ConfidenceHigh, "goreleaser.release-header-raw-url",
				map[string]string{"served": "raw-at-tag", "files": f.Path, "ref": m.ref}, f.Evidence(lineOf(m.path)))
		}
	}
}

func platforms(b grBuild) [][2]string {
	goos, goarch := b.Goos, b.Goarch
	if len(goos) == 0 {
		goos = []string{"linux", "darwin", "windows"}
	}
	if len(goarch) == 0 {
		goarch = []string{"amd64", "arm64"}
	}
	var out [][2]string
	for _, o := range goos {
		for _, a := range goarch {
			skip := false
			for _, ig := range b.Ignore {
				if ig.Goos == o && (ig.Goarch == "" || ig.Goarch == a) {
					skip = true
				}
			}
			if !skip {
				out = append(out, [2]string{o, a})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0]+out[i][1] < out[j][0]+out[j][1] })
	return out
}

type rawRef struct{ owner, name, ref, path string }

var rawPrefixRe = regexp.MustCompile(`https?://raw\.githubusercontent\.com/((?:\{\{[^}]*\}\}|\[\[[^\]]*\]\]|[^\s"'<>)` + "`" + `])+)`)

// rawRefs parses raw.githubusercontent.com URLs; a templated owner/name
// segment (e.g. {{ .Env.REPO }}) is accepted and reported with owner "".
func rawRefs(text string) []rawRef {
	var out []rawRef
	for _, m := range rawPrefixRe.FindAllStringSubmatch(text, -1) {
		rest := m[1]
		var r rawRef
		if strings.HasPrefix(rest, "{{") {
			end := strings.Index(rest, "}}")
			if end < 0 {
				continue
			}
			rest = strings.TrimPrefix(rest[end+2:], "/")
		} else {
			segs := strings.SplitN(rest, "/", 3)
			if len(segs) < 3 {
				continue
			}
			r.owner, r.name, rest = segs[0], segs[1], segs[2]
		}
		segs := strings.SplitN(rest, "/", 2)
		if len(segs) < 2 {
			continue
		}
		r.ref, r.path = segs[0], strings.TrimRight(segs[1], ".,;:")
		if isYAMLExt(r.path) {
			out = append(out, r)
		}
	}
	return out
}

func detectKo(st *scanState, f *File) {
	st.emit(KindBuildTool, "ko", domain.ConfidenceMedium, "ko.config", map[string]string{"files": f.Path}, f.Evidence(1))
}

func detectVersionFile(st *scanState, f *File) {
	v := strings.TrimSpace(f.Text())
	if v == "" || strings.Contains(v, "\n") {
		return
	}
	_, refVer := st.refTag()
	rel := ""
	switch {
	case refVer != "" && strings.TrimPrefix(v, "v") == refVer:
		rel = "VERSION file = " + tmplVersion
	case refVer != "" && strings.HasPrefix(refVer, strings.TrimPrefix(v, "v")+"."):
		rel = "VERSION file = " + tmplLine
	default:
		return
	}
	st.emit(KindVersionRelation, rel, domain.ConfidenceMedium, "version-file.matches-ref",
		map[string]string{"subject": "VERSION", "value": v, "files": f.Path}, f.Evidence(1))
}
