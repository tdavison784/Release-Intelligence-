package discovery

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// releaseImageSources are scanner sources that describe how images are
// built and published (as opposed to prose mentions in documentation).
var releaseImageSources = map[string]bool{"workflow": true, "helm-values": true, "manifest": true, "kustomize": true, "goreleaser": true,
	"make": true, "script": true, "security-insights": true, "build-list": true}

var (
	devSegmentRe   = regexp.MustCompile(`(?i)^(testing|test|tests|dev|devel|nightly|nightlies|snapshots?|staging|ci|e2e|sandbox|experimental|unstable|pr|preview|scratch)$`)
	devSubstringRe = regexp.MustCompile(`(?i)nightly|snapshot|testing|staging`)
)

// hasDevSegment reports whether a registry path names a development,
// testing or nightly repository ("ghcr.io/x/testing", "gcr.io/x-testing",
// "docker.io/xnightly").
func hasDevSegment(repo string) bool {
	for _, seg := range strings.Split(repo, "/")[1:] {
		if devSegmentRe.MatchString(seg) || devSubstringRe.MatchString(seg) {
			return true
		}
		for _, tok := range strings.FieldsFunc(seg, func(r rune) bool { return r == '-' || r == '_' || r == '.' }) {
			if devSegmentRe.MatchString(tok) {
				return true
			}
		}
	}
	return false
}

func setOf(s string) map[string]bool {
	m := map[string]bool{}
	for _, p := range splitList(s) {
		m[p] = true
	}
	return m
}

func onlyContexts(c Candidate, allowed ...string) bool {
	ctxs := setOf(c.Attr("contexts"))
	if len(ctxs) == 0 {
		return false
	}
	for k := range ctxs {
		if !containsStr(allowed, k) {
			return false
		}
	}
	return true
}

// productRelated reports whether a registry path names the product.
func (r *resolver) productRelated(s string) bool {
	s = strings.ToLower(s)
	for _, t := range r.hints.Tokens() {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

type imageInfo struct {
	c        Candidate
	ref      imageRef
	score    int
	template string
	release  bool
}

func (r *resolver) artifacts() {
	charts := r.helmCharts()
	r.externalCharts()
	manifests := r.inRepoManifests()
	r.releaseAssets()
	r.crds()
	r.images(charts, manifests)
}

// images groups product images by name; channels are the registries that
// publish them.
func (r *resolver) images(chartTemplate string, manifests map[string]Candidate) {
	byName := map[string][]imageInfo{}
	var names []string
	for _, c := range r.byK[KindImage] {
		ref, ok := parseImageValue(c.Value)
		if !ok {
			continue
		}
		srcs := setOf(c.Attr("sources"))
		releaseSrc := false
		for s := range srcs {
			if releaseImageSources[s] {
				releaseSrc = true
			}
		}
		classes := setOf(c.Attr("classes"))
		releaseClass := classes[tagRelease] || classes["chart-appversion"]
		strongSrc := srcs["helm-values"] || srcs["goreleaser"] || srcs["kustomize"] || srcs["security-insights"] ||
			(srcs["workflow"] && setOf(c.Attr("triggers"))[triggerTag])
		switch {
		case !r.productRelated(c.Value):
			r.exclude(c, "image.third-party", "Image is not named after the product (dependency or tooling image).")
			continue
		case !releaseSrc:
			r.exclude(c, "image.docs-only", "Only mentioned in documentation; no build or publish configuration produces it.")
			continue
		case onlyContexts(c, ctxTest, ctxSample, ctxDocs):
			r.exclude(c, "image.test-only", "Only referenced from test, sample or documentation files.")
			continue
		case hasDevSegment(c.Value):
			r.exclude(c, "image.dev-registry", "Registry path denotes a development/testing repository.")
			continue
		case !releaseClass && !strongSrc:
			r.exclude(c, "image.no-release-evidence", "No reference with the release tag and no release publication config (tag classes: "+c.Attr("classes")+").")
			continue
		case !releaseClass && (classes[tagSnapshot] || classes[tagFloating]) && !classes[tagUnresolved]:
			r.exclude(c, "image.non-release-tags", "Only snapshot/floating tags ("+c.Attr("classes")+") are produced, e.g. by branch builds.")
			continue
		}
		info := imageInfo{c: c, ref: ref, template: c.Attr("tagTemplate"), release: classes[tagRelease]}
		info.score = confRank(c.Confidence)*10 + atoiDefault(c.Attr("occurrences"), 1)
		if info.release {
			info.score += 30
		}
		if setOf(c.Attr("triggers"))[triggerTag] {
			info.score += 40
		}
		if srcs["helm-values"] || srcs["goreleaser"] {
			info.score += 20
		}
		name := ref.Name()
		if _, ok := byName[name]; !ok {
			names = append(names, name)
		}
		byName[name] = append(byName[name], info)
	}
	// image names produced by build lists, joined with product registries
	regs := r.productRegistries()
	for _, c := range r.byK[KindImageName] {
		if c.Confidence == domain.ConfidenceLow || onlyContexts(c, ctxTest, ctxSample) {
			r.exclude(c, "image-name.test-or-weak", "Image name only from test/sample builds or a lone Dockerfile suffix.")
			continue
		}
		if len(regs) == 0 {
			r.exclude(c, "image-name.no-registry", "The repository builds this image but no publication registry was found.")
			continue
		}
		if _, ok := byName[c.Value]; !ok {
			names = append(names, c.Value)
		}
		for i, reg := range regs {
			ref, ok := parseImageValue(reg.Value + "/" + c.Value)
			if !ok {
				continue
			}
			byName[c.Value] = append(byName[c.Value], imageInfo{c: c, ref: ref, score: 15 - i + confRank(reg.Confidence)})
		}
	}
	sort.Strings(names)
	for _, name := range names {
		infos := byName[name]
		sort.SliceStable(infos, func(i, j int) bool { return infos[i].score > infos[j].score })
		var channels []catalog.Locator
		var cands []Candidate
		seen := map[string]bool{}
		tmpl := ""
		for _, in := range infos {
			if !seen[in.ref.Repository()] {
				seen[in.ref.Repository()] = true
				channels = append(channels, catalog.Locator{Kind: catalog.LocatorOCI, Repository: in.ref.Repository()})
			}
			cands = append(cands, in.c)
			if tmpl == "" && in.template != "" {
				tmpl = in.template
			}
		}
		rule, conf, why := "image.tag-from-release", domain.ConfidenceHigh, "Image tags equal the release tag in the build/publish configuration."
		switch {
		case tmpl != "":
		case setOf(infos[0].c.Attr("classes"))["chart-appversion"] && chartTemplate != "":
			tmpl, rule, why = chartTemplate, "image.tag-from-chart-appversion", "The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release."
		default:
			tmpl, rule, conf, why = tmplTag, "image.tag-assumed-release", domain.ConfidenceLow, "No explicit tag evidence; assuming images are tagged with the release tag."
		}
		a := catalog.Artifact{ID: name, Type: domain.ArtifactContainerImage, Name: name,
			Version: catalog.VersionRelation{Strategy: catalog.VersionTemplate, Template: tmpl}, Channels: channels}
		for id, m := range manifests {
			for _, img := range splitList(m.Attr("images")) {
				if strings.HasPrefix(img, channels[0].Repository+":") && strings.HasSuffix(img, ":"+r.in.ScanRef.Tag) && r.in.ScanRef.Tag != "" {
					a.References = append(a.References, catalog.ArtifactReference{Artifact: id, Pattern: channels[0].Repository + ":" + tmpl})
					break
				}
			}
		}
		sort.Slice(a.References, func(i, j int) bool { return a.References[i].Artifact < a.References[j].Artifact })
		if len(channels) > 1 {
			why += fmt.Sprintf(" Published to %d registries; ordered by evidence strength.", len(channels))
		}
		r.addArtifact(a, rule, conf, []string{TargetImages, TargetRegistries, TargetVersionRelations}, why, cands...)
	}
}

// productRegistries returns product registry candidates (host/namespace),
// strongest first.
func (r *resolver) productRegistries() []Candidate {
	var out []Candidate
	for _, c := range r.byK[KindRegistry] {
		if !strings.Contains(c.Value, "/") || !r.productRelated(c.Value) || hasDevSegment(c.Value) || onlyContexts(c, ctxTest, ctxSample) {
			continue
		}
		if strings.Contains(c.Value, "/charts") {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return registryScore(out[i]) > registryScore(out[j]) })
	return out
}

var (
	pushVarRe = regexp.MustCompile(`(?i)(^|_)(hub|registry|ko_docker_repo|docker_repo|image_repo(sitory)?|push)(_|$)`)
	baseVarRe = regexp.MustCompile(`(?i)base|builder|tools`)
)

// registryScore ranks registry candidates as publication targets: variables
// naming the push hub weigh most; base-image registries least.
func registryScore(c Candidate) int {
	s := confRank(c.Confidence)*10 + minInt(atoiDefault(c.Attr("occurrences"), 1), 10)
	for _, v := range splitList(c.Attr("variable")) {
		switch {
		case baseVarRe.MatchString(v):
			s -= 20
		case pushVarRe.MatchString(v):
			s += 25
		}
	}
	if setOf(c.Attr("triggers"))[triggerTag] {
		s += 30
	}
	return s
}

// helmCharts adds in-repo charts with publication evidence and returns the
// version template of the main chart ("" when none).
func (r *resolver) helmCharts() string {
	type chart struct {
		c         Candidate
		name, dir string
		oci       []string
		prefixOCI []string
		repos     []Candidate
		published bool
		explicit  bool
	}
	var charts []*chart
	for _, c := range r.byK[KindHelmChart] {
		name, dir := c.Value, c.Attr("dir")
		if i := strings.Index(name, "@"); i >= 0 {
			name = name[:i]
		}
		if _, _, main := r.repoOf(c); !main {
			continue
		}
		if onlyContexts(c, ctxTest, ctxSample) || c.Attr("type") == "library" {
			r.exclude(c, "helm.test-or-library", "Chart lives in a test/sample directory or is a library chart.")
			continue
		}
		ch := &chart{c: c, name: name, dir: dir}
		for _, o := range r.byK[KindHelmOCI] {
			if hasDevSegment(o.Value) || onlyContexts(o, ctxTest, ctxSample) && o.Attr("prefix") == "" && path.Base(o.Value) != name {
				continue
			}
			switch {
			case path.Base(o.Value) == name:
				ch.oci = append(ch.oci, o.Value)
			case o.Attr("prefix") == "true" || strings.HasSuffix(o.Value, "/charts"):
				ch.prefixOCI = append(ch.prefixOCI, strings.TrimSuffix(o.Value, "/")+"/"+name)
			}
		}
		for _, h := range r.byK[KindHelmRepo] {
			// chart-releaser publishes every chart of the repository it runs
			// in, so its pages candidates match the repo's own charts even
			// without naming them.
			releaser := c.Attr("repo") == "" && h.Attr("repo") == "" &&
				(containsStr(h.Rules, "workflow.chart-releaser-pages") || containsStr(h.Rules, "workflow.chart-releaser-pages-raw"))
			if setOf(h.Attr("charts"))[name] || h.Attr("chartDir") == dir || releaser {
				ch.repos = append(ch.repos, h)
			}
		}
		ch.explicit = len(ch.oci) > 0 || len(ch.repos) > 0
		charts = append(charts, ch)
	}
	anyExplicit := false
	for _, ch := range charts {
		if ch.explicit {
			anyExplicit = true
		}
	}
	// A chart-registry prefix ("oci://host/charts/") applies to charts that
	// are published explicitly elsewhere, or — when no chart is — to the
	// product's own charts.
	for _, ch := range charts {
		if ch.explicit || (!anyExplicit && r.productRelated(ch.name+" "+ch.dir)) {
			ch.oci = append(ch.oci, ch.prefixOCI...)
		}
		ch.published = len(ch.oci) > 0 || len(ch.repos) > 0
	}
	anyPublished := false
	for _, ch := range charts {
		if ch.published {
			anyPublished = true
		}
	}
	mainTemplate := ""
	for _, ch := range charts {
		if !ch.published && (anyPublished || !r.productRelated(ch.name+" "+ch.dir)) {
			r.exclude(ch.c, "helm.unpublished", "No publication channel (OCI push/reference or Helm repository) mentions this chart.")
			continue
		}
		if !ch.published {
			r.exclude(ch.c, "helm.no-channel", "Product chart found but no publication channel; cannot be located per release.")
			r.draft.Open = append(r.draft.Open, fmt.Sprintf("Chart %s (%s) has no discovered publication channel.", ch.name, ch.dir))
			continue
		}
		var channels []catalog.Locator
		cands := []Candidate{ch.c}
		seen := map[string]bool{}
		for _, o := range dedupe(ch.oci) {
			if !seen[o] {
				seen[o] = true
				channels = append(channels, catalog.Locator{Kind: catalog.LocatorOCI, Repository: o})
			}
		}
		for _, o := range r.byK[KindHelmOCI] {
			for _, x := range ch.oci {
				if x == o.Value || strings.HasPrefix(x, strings.TrimSuffix(o.Value, "/")+"/") {
					cands = append(cands, o)
				}
			}
		}
		// Canonical *.github.io repository first, then the same index on
		// raw.githubusercontent.com (reachable where the pages host is not).
		sort.SliceStable(ch.repos, func(i, j int) bool {
			return ch.repos[i].Attr("pagesIndex") == "" && ch.repos[j].Attr("pagesIndex") != ""
		})
		for _, h := range ch.repos {
			channels = append(channels, catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: h.Value, Chart: ch.name})
			cands = append(cands, h)
		}
		// chart-releaser also tags every chart release ("<chart>-X.Y.Z" or
		// another dedicated family); the chart directory at those tags is a
		// git channel that needs no HTTP host at all.
		if r.hasBuildTool("chart-releaser") {
			if f := chartTrainFamily(r.in.Tags, ch.name); f != nil {
				pat := `^` + regexp.QuoteMeta(f.Prefix) + `(?P<version>\d+\.\d+\.\d+)$`
				channels = append(channels, catalog.Locator{Kind: catalog.LocatorHelmGit, Repository: r.in.Repo.String(), Path: ch.dir, TagPattern: pat})
				cands = append(cands, tagFamilyCandidate(r.in.Tags, *f))
			}
		}
		vr, rule, conf, why := r.chartVersion(ch.c, ch.dir)
		chartFile := ch.c.Attr("chartFile")
		a := catalog.Artifact{ID: ch.name + "-chart", Type: domain.ArtifactHelmChart, Name: ch.name, Version: vr, Channels: channels,
			Contents: []catalog.Content{
				{Kind: catalog.ContentHelmValues, Locator: &catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: r.in.Repo.String(), Path: ch.dir + "/values.yaml"}},
				{Kind: catalog.ContentChartMetadata, Locator: &catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: r.in.Repo.String(), Path: chartFile}},
			}}
		if path.Base(chartFile) != "Chart.yaml" {
			a.Notes = "Chart metadata file is " + path.Base(chartFile) + " at the scanned ref; the file name may differ at other releases."
		}
		r.addArtifact(a, rule, conf, []string{TargetHelmCharts, TargetVersionRelations},
			why+fmt.Sprintf(" Published via %d channel(s); OCI locations first.", len(channels)), cands...)
		if mainTemplate == "" && vr.Strategy == catalog.VersionTemplate {
			mainTemplate = vr.Template
		}
	}
	return mainTemplate
}

// chartVersion derives the relation between chart and release versions.
// Evidence priority: an explicit build stamp (make/goreleaser) beats the
// Chart.yaml appVersion relation, which beats a chart version that happens
// to equal the release version.
func (r *resolver) chartVersion(c Candidate, dir string) (catalog.VersionRelation, string, domain.Confidence, string) {
	for _, vr := range r.byK[KindVersionRelation] {
		if vr.Attr("subject") == "chart.version" && vr.Attr("template") != "" && vr.Attr("chart") == "" {
			return catalog.VersionRelation{Strategy: catalog.VersionTemplate, Template: vr.Attr("template")}, "helm.version-from-build", domain.ConfidenceHigh,
				fmt.Sprintf("The build sets the chart version from the release tag (%s).", vr.Value)
		}
	}
	for _, vr := range r.byK[KindVersionRelation] {
		if vr.Attr("subject") == "chart.appVersion" && vr.Attr("template") != "" && (vr.Attr("chart") == "" || strings.HasPrefix(c.Value, vr.Attr("chart")+"@")) {
			return catalog.VersionRelation{Strategy: catalog.VersionLookup, Field: "appVersion", Match: vr.Attr("template"), Select: "latest"},
				"helm.appversion-lookup", domain.ConfidenceHigh,
				fmt.Sprintf("Chart.yaml sets appVersion %q, which equals the scanned release tag; the chart release for a product release is looked up by appVersion == %s (the chart's own version moves independently).", vr.Attr("appVersion"), vr.Attr("template"))
		}
	}
	for _, vr := range r.byK[KindVersionRelation] {
		if vr.Attr("subject") == "chart.version" && vr.Attr("template") != "" && (vr.Attr("chart") == "" || strings.HasPrefix(c.Value, vr.Attr("chart")+"@")) {
			return catalog.VersionRelation{Strategy: catalog.VersionTemplate, Template: vr.Attr("template")}, "helm.version-matches-release", domain.ConfidenceHigh,
				fmt.Sprintf("Chart.yaml at the scanned release carries version %q, the release version; chart versions follow the release (%s).", c.Attr("version"), vr.Attr("template"))
		}
	}
	if c.Attr("placeholder") == "true" {
		t := tmplVersion
		if c.Attr("versionPrefix") == "v" {
			t = tmplTag
		}
		return catalog.VersionRelation{Strategy: catalog.VersionTemplate, Template: t}, "helm.placeholder-version-follows-release", domain.ConfidenceMedium,
			fmt.Sprintf("Chart.yaml carries placeholder version %q replaced at release time; assumed to equal the release (%s).", c.Attr("version"), t)
	}
	return catalog.VersionRelation{Strategy: catalog.VersionIndependent}, "helm.independent-version", domain.ConfidenceLow,
		fmt.Sprintf("Chart has its own version %q unrelated to the release.", c.Attr("version"))
}

// externalCharts adds charts maintained in a separate repository.
func (r *resolver) externalCharts() {
	for _, c := range r.byK[KindChartRepo] {
		cr, err := ParseRepo(c.Value)
		if err != nil {
			continue
		}
		var best string
		for _, p := range splitList(c.Attr("paths")) {
			if r.productRelated(path.Base(p)) && (best == "" || path.Base(p) == r.in.Repo.Name) {
				best = p
			}
		}
		if best == "" {
			r.exclude(c, "chart-repo.no-product-chart", "No chart path for this product is referenced in that repository.")
			continue
		}
		name := path.Base(best)
		var channels []catalog.Locator
		cands := []Candidate{c}
		for _, h := range r.byK[KindHelmRepo] {
			if setOf(h.Attr("charts"))[name] {
				channels = append(channels, catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: h.Value, Chart: name})
				cands = append(cands, h)
			}
		}
		if len(channels) == 0 && cr.IsGitHub() {
			channels = append(channels, catalog.Locator{Kind: catalog.LocatorHelmRepo, URL: "https://" + strings.ToLower(cr.Owner) + ".github.io/" + cr.Name, Chart: name})
		}
		tp, tpNote := chartTagPattern(name, r.in.ChartRepoTags[c.Value])
		channels = append(channels, catalog.Locator{Kind: catalog.LocatorHelmGit, Repository: cr.String(), Path: best, TagPattern: tp})
		a := catalog.Artifact{ID: name + "-chart", Type: domain.ArtifactHelmChart, Name: name,
			Version:  catalog.VersionRelation{Strategy: catalog.VersionLookup, Field: "appVersion", Match: tmplTag, Select: "latest"},
			Channels: channels,
			Notes:    "Chart maintained in " + cr.String() + " with its own version; chart releases are found by appVersion. " + tpNote,
		}
		r.addArtifact(a, "chart.external-lookup-appversion", domain.ConfidenceMedium, []string{TargetHelmCharts, TargetVersionRelations},
			"The chart lives in a separate repository with independent versions; the chart release for a product release is looked up by appVersion == release tag (GitHub Pages URL by the chart-releaser convention).", cands...)
	}
}

// chartTagPattern infers the chart-releaser tag pattern ("<chart>-X.Y.Z").
func chartTagPattern(chart string, tags []RemoteTag) (string, string) {
	pat := `^` + regexp.QuoteMeta(chart) + `-(?P<version>\d+\.\d+\.\d+)$`
	if len(tags) == 0 {
		return pat, "Tag pattern assumed from the chart-releaser convention (chart repository tags not listed)."
	}
	re := regexp.MustCompile(pat)
	n := 0
	for _, t := range tags {
		if re.MatchString(t.Name) {
			n++
		}
	}
	return pat, fmt.Sprintf("%d of %d chart-repository tags match %s.", n, len(tags), pat)
}

var (
	checksumAssetRe = regexp.MustCompile(`(?i)checksum|sha256sum|\.sha(256|512)$|sums\.txt$`)
	sigAssetRe      = regexp.MustCompile(`(?i)\.(sig|pem|cert|asc|intoto\.jsonl|spdx|sbom|bom\.json)$`)
	manifestAssetRe = regexp.MustCompile(`(?i)\.(ya?ml|json)$`)
	crdAssetRe      = regexp.MustCompile(`(?i)crd`)
	platformRe      = regexp.MustCompile(`(?i)[-_.](linux|darwin|osx|windows|win|freebsd)([-_.](amd64|arm64|armv7|arm|386|s390x|ppc64le|x86_64))?`)
)

// releaseAssets adds artifacts downloaded from hosted releases.
func (r *resolver) releaseAssets() {
	groups := map[string][]Candidate{}
	var order []string
	for _, c := range r.ownAssets() {
		if !r.assetEvidenceOK(c) {
			r.exclude(c, "asset.only-in-versioned-docs", "Only referenced from documentation of old releases.")
			continue
		}
		file := c.Attr("file")
		switch {
		case checksumAssetRe.MatchString(file), sigAssetRe.MatchString(file):
			r.decide("candidate:"+c.ID, "annotate", "asset.integrity-file", "Checksum/signature/provenance asset; recorded but not modelled as an artifact.", c.Confidence, "", c)
			continue
		}
		key := file
		if !manifestAssetRe.MatchString(file) {
			key = platformRe.ReplaceAllString(file, "")
			key = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(key, ".exe"), ".zip"), ".tar.gz")
			key = strings.TrimSuffix(strings.TrimSuffix(key, "-"+tmplVersion), "-"+tmplTag)
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], c)
	}
	sort.Strings(order)
	for _, key := range order {
		cs := groups[key]
		sort.SliceStable(cs, func(i, j int) bool { return assetRank(cs[i].Attr("file")) < assetRank(cs[j].Attr("file")) })
		rep := cs[0]
		file := rep.Attr("file")
		var files []string
		for _, c := range cs {
			files = append(files, c.Attr("file"))
		}
		stem := strings.NewReplacer(tmplVersion, "", tmplTag, "").Replace(key)
		stem = strings.Trim(strings.TrimSuffix(strings.TrimSuffix(stem, path.Ext(stem)), "."), "-_.")
		a := catalog.Artifact{Name: file, Version: catalog.VersionRelation{Strategy: catalog.VersionTemplate, Template: tmplTag},
			Channels: []catalog.Locator{{Kind: catalog.LocatorHTTP, URL: rep.Value}}}
		targets := []string{TargetReleaseSource, TargetVersionRelations}
		switch {
		case manifestAssetRe.MatchString(file) && crdAssetRe.MatchString(file):
			a.ID, a.Type = stem, domain.ArtifactCRD
			a.Contents = []catalog.Content{{Kind: catalog.ContentCRDs}}
		case manifestAssetRe.MatchString(file):
			a.ID, a.Type = stem+"-manifest", domain.ArtifactManifest
			a.Contents = []catalog.Content{{Kind: catalog.ContentImageRefs}}
		default:
			a.ID, a.Type, a.Name = stem+"-binary", domain.ArtifactBinary, stem
			if len(files) > 1 {
				a.Description = "Platform builds: " + strings.Join(files, ", ")
			}
		}
		r.addArtifact(a, "asset.hosted-release-download", maxConfOf(cs), targets,
			fmt.Sprintf("Downloaded from the hosted release of the tag (%d reference(s)).", len(cs)), cs...)
	}
}

func maxConfOf(cs []Candidate) domain.Confidence {
	c := domain.ConfidenceLow
	for _, x := range cs {
		c = maxConf(c, x.Confidence)
	}
	return c
}

func assetRank(file string) int {
	l := strings.ToLower(file)
	switch {
	case strings.Contains(l, "linux-amd64") || strings.Contains(l, "linux_amd64") || strings.Contains(l, "linux-x86_64"):
		return 0
	case strings.Contains(l, "linux"):
		return 1
	}
	return 2
}

// assetEvidenceOK requires a reference from the product repository or from
// current (non-versioned) documentation.
func (r *resolver) assetEvidenceOK(c Candidate) bool {
	if c.Attr("repo") == "" {
		return true
	}
	for _, f := range splitList(c.Attr("files")) {
		if len(findVersionTokens(f)) == 0 {
			return true
		}
	}
	return false
}

// inRepoManifests adds install manifests served from the tagged repository
// and returns them keyed by artifact id.
func (r *resolver) inRepoManifests() map[string]Candidate {
	out := map[string]Candidate{}
	type m struct {
		c     Candidate
		score int
	}
	var ms []m
	for _, c := range r.byK[KindManifest] {
		if _, _, main := r.repoOf(c); !main {
			continue
		}
		served := c.Attr("served") != ""
		release := false
		for _, img := range splitList(c.Attr("images")) {
			if r.in.ScanRef.Tag != "" && strings.HasSuffix(img, ":"+r.in.ScanRef.Tag) && r.productRelated(img) {
				release = true
			}
		}
		if !release && !(served && c.Attr("images") != "") {
			if c.Attr("images") != "" {
				r.exclude(c, "manifest.not-release-tagged", "No product image tagged with the release in this manifest.")
			}
			continue
		}
		score := 0
		if served {
			score += 50
		}
		if containsStr(c.Rules, "goreleaser.release-header-raw-url") {
			score += 100 // linked from the release notes themselves
		}
		if strings.Contains(path.Base(c.Value), "install") {
			score += 20
		}
		score -= strings.Count(c.Value, "/")*3 + len(c.Value)/10
		ms = append(ms, m{c, score})
	}
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].score > ms[j].score })
	for i, x := range ms {
		if i >= 4 {
			r.exclude(x.c, "manifest.limit", "More install manifest variants exist; only the four best ranked are modelled.")
			continue
		}
		stem := strings.TrimSuffix(strings.TrimSuffix(x.c.Value, ".yaml"), ".yml")
		a := catalog.Artifact{ID: strings.ReplaceAll(stem, "/", "-"), Type: domain.ArtifactManifest, Name: path.Base(x.c.Value),
			Version:  catalog.VersionRelation{Strategy: catalog.VersionTemplate, Template: tmplTag},
			Channels: []catalog.Locator{{Kind: catalog.LocatorRepoFile, Repository: r.in.Repo.String(), Path: x.c.Value}},
			Contents: []catalog.Content{{Kind: catalog.ContentImageRefs}}}
		if x.c.Attr("crds") == "true" {
			a.Description = "Install manifest (also bundles the CRDs)."
		}
		id := r.addArtifact(a, "manifest.in-repo-install", x.c.Confidence, []string{TargetImages, TargetVersionRelations},
			"Static install manifest in the tagged tree referencing the product image with the release tag.", x.c)
		out[id] = x.c
	}
	return out
}

// inRepoManifestCandidates returns manifest candidates of the main
// repository (docs-referenced YAML files), most evidenced first.
func (r *resolver) inRepoManifestCandidates() []Candidate {
	var out []Candidate
	for _, c := range r.byK[KindManifest] {
		if _, _, main := r.repoOf(c); main {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return atoiDefault(out[i].Attr("occurrences"), 1) > atoiDefault(out[j].Attr("occurrences"), 1)
	})
	return out
}

var crdBundleBaseRe = regexp.MustCompile(`(?i)^(?:[a-z0-9_.-]*-)?crds?\.ya?ml$|^crd\.ya?ml$`)

// isCRDBundlePath reports whether a path names a single CRD bundle file
// (deploy/crds/bundle.yaml, something-crds.yaml) rather than one CRD of a
// directory of many.
func isCRDBundlePath(p string) bool {
	base := strings.ToLower(path.Base(p))
	dir := strings.ToLower(path.Dir(p))
	if crdBundleBaseRe.MatchString(base) {
		return true
	}
	return (base == "bundle.yaml" || base == "bundle.yml") && (strings.Contains(dir, "crd"))
}

// hasBuildTool reports whether a build-tool candidate names tool.
func (r *resolver) hasBuildTool(tool string) bool {
	for _, c := range r.byK[KindBuildTool] {
		if c.Value == tool {
			return true
		}
	}
	return false
}

// tagFamilyCandidate turns a tag family into a pseudo-candidate so that the
// chart's helm-git channel can cite the tag list as evidence.
func tagFamilyCandidate(ta *TagAnalysis, f TagFamily) Candidate {
	c := Candidate{Kind: KindTagScheme, Value: f.Prefix, Confidence: domain.ConfidenceMedium, Rules: []string{"tags.family"},
		Attributes: map[string]string{"prefix": f.Prefix, "stable": itoa(f.Count), "latest": f.Latest}}
	c.ID = candidateID(c.Kind, "family:"+f.Prefix)
	if ta != nil && f.Latest != "" {
		c.Evidence = append(c.Evidence, ta.Evidence(f.Latest, ta.ListedAt))
	}
	return c
}

// crds adds CRD artifacts: release-asset CRD bundles are preferred; else a
// docs-referenced CRD bundle file; else dedicated in-repo CRD files of the
// product's API groups.
func (r *resolver) crds() {
	for _, a := range r.def.Artifacts {
		if a.Type == domain.ArtifactCRD {
			for _, c := range r.byK[KindCRD] {
				r.exclude(c, "crd.prefer-release-asset", "A CRD bundle is attached to hosted releases ("+a.Name+"); in-repo CRD sources are not modelled.")
			}
			return
		}
	}
	// A single CRD bundle file referenced from the repository's own install
	// documentation (e.g. deploy/crds/bundle.yaml linked from the README) is
	// what non-Helm users are told to apply: prefer it over the raw CRD
	// directory tree.
	for _, m := range r.inRepoManifestCandidates() {
		if !isCRDBundlePath(m.Value) {
			continue
		}
		a := catalog.Artifact{ID: "crds", Type: domain.ArtifactCRD, Name: path.Base(m.Value),
			Version:  catalog.VersionRelation{Strategy: catalog.VersionTemplate, Template: tmplTag},
			Channels: []catalog.Locator{r.repoFile(m, m.Value)},
			Contents: []catalog.Content{{Kind: catalog.ContentCRDs}},
			Notes:    "Single CRD bundle file; the repository's install documentation references it."}
		r.addArtifact(a, "crd.docs-referenced-bundle", m.Confidence, []string{TargetVersionRelations},
			"The install documentation references this CRD bundle directly; preferred over the CRD source directory.", m)
		for _, c := range r.byK[KindCRD] {
			r.exclude(c, "crd.prefer-docs-bundle", "A docs-referenced CRD bundle file covers the CRDs; the directory layout is not modelled.")
		}
		return
	}
	dirs := map[string][]Candidate{}
	for _, c := range r.byK[KindCRD] {
		if _, _, main := r.repoOf(c); !main {
			continue
		}
		if c.Attr("bundled") == "true" || onlyContexts(c, ctxTest, ctxSample) || !r.productRelated(c.Attr("groups")+" "+c.Value) {
			r.exclude(c, "crd.not-dedicated", "CRDs bundled in another manifest, test data, or of third-party API groups.")
			continue
		}
		dirs[c.Attr("dir")] = append(dirs[c.Attr("dir")], c)
	}
	best, bestN := "", -1
	for d, cs := range dirs {
		n := 0
		for _, c := range cs {
			n += atoiDefault(c.Attr("count"), 1)
		}
		if n > bestN || (n == bestN && d < best) {
			best, bestN = d, n
		}
	}
	if best == "" {
		return
	}
	cs := dirs[best]
	var loc catalog.Locator
	if len(cs) == 1 {
		loc = catalog.Locator{Kind: catalog.LocatorRepoFile, Repository: r.in.Repo.String(), Path: cs[0].Value}
	} else {
		loc = catalog.Locator{Kind: catalog.LocatorRepoDir, Repository: r.in.Repo.String(), Path: best, Glob: commonGlob(cs)}
	}
	a := catalog.Artifact{ID: "crds", Type: domain.ArtifactCRD, Name: "CustomResourceDefinitions",
		Version: catalog.VersionRelation{Strategy: catalog.VersionTemplate, Template: tmplTag}, Channels: []catalog.Locator{loc},
		Contents: []catalog.Content{{Kind: catalog.ContentCRDs}}}
	r.addArtifact(a, "crd.in-repo", domain.ConfidenceHigh, []string{TargetVersionRelations},
		fmt.Sprintf("%d CRDs of the product's API groups in %s at the release tag.", bestN, best), cs...)
}

func commonGlob(cs []Candidate) string {
	var names []string
	for _, c := range cs {
		names = append(names, path.Base(c.Value))
	}
	suffix := names[0]
	for _, n := range names[1:] {
		for !strings.HasSuffix(n, suffix) {
			suffix = suffix[1:]
		}
	}
	if i := strings.IndexAny(suffix, "-_"); i >= 0 && strings.HasSuffix(suffix, ".yaml") {
		return "*" + suffix[i:]
	}
	return "*" + path.Ext(names[0])
}

// questions records ambiguities for the optional LLM resolver.
func (r *resolver) questions() {
	var regCands []Candidate
	hosts := map[string]bool{}
	for _, a := range r.def.Artifacts {
		if a.Type != domain.ArtifactContainerImage {
			continue
		}
		for _, ch := range a.Channels {
			hosts[strings.SplitN(ch.Repository, "/", 2)[0]] = true
		}
	}
	for _, c := range r.byK[KindRegistry] {
		if r.productRelated(c.Value) || hosts[strings.SplitN(c.Value, "/", 2)[0]] {
			regCands = append(regCands, c)
		}
	}
	if len(hosts) > 1 || len(regCands) > 1 {
		r.draft.Questions = append(r.draft.Questions, Question{Kind: QuestionRegistryRoles,
			Summary:    fmt.Sprintf("%d registry hosts/namespaces are referenced for product images; which are release channels?", len(regCands)+len(hosts)),
			Candidates: append(regCands, r.imageCandidates()...)})
	}
	// a role is "missing" when no high-confidence source serves it (a
	// generic fallback such as hosted release bodies does not count)
	missing := []string{}
	for _, role := range []domain.SourceRole{domain.RoleReleaseNotes, domain.RoleUpgradeGuide, domain.RoleCompatibility} {
		strong := false
		for _, s := range r.def.SourcesWithRole(role) {
			if e := r.draft.Elements["source:"+s.ID]; e != nil && e.Confidence == domain.ConfidenceHigh {
				strong = true
			}
		}
		if !strong {
			missing = append(missing, string(role))
		}
	}
	if docs := r.byK[KindDocsRepo]; len(docs) > 0 && len(missing) > 0 {
		q := Question{Kind: QuestionDocsSources, Summary: "Documentation lives in another repository; no " + strings.Join(missing, ", ") + " source was derived from it.",
			Candidates: docs, Facts: map[string]string{"missing": strings.Join(missing, ",")}}
		for repo, paths := range r.in.DocsListings {
			q.Paths = append(q.Paths, prefixAll(repo+":", firstN(paths, 60))...)
		}
		r.draft.Questions = append(r.draft.Questions, q)
	}
	var charts []catalog.Artifact
	for _, a := range r.def.Artifacts {
		if a.Type == domain.ArtifactHelmChart {
			charts = append(charts, a)
		}
	}
	if len(charts) > 1 {
		r.draft.Questions = append(r.draft.Questions, Question{Kind: QuestionMainChart,
			Summary: fmt.Sprintf("%d charts are published; which one installs the product?", len(charts)), Candidates: r.byK[KindHelmChart]})
	}
	// one question per distinct uncertain relation, covering every chart
	// that shares it
	groups := map[string][]catalog.Artifact{}
	var order []string
	for _, a := range charts {
		e := r.draft.Elements["artifact:"+a.ID]
		if e == nil || e.Confidence == domain.ConfidenceHigh {
			continue
		}
		k := a.Version.Strategy + "|" + a.Version.Template + "|" + a.Version.Field + "|" + a.Version.Match
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], a)
	}
	for _, k := range order {
		as := groups[k]
		var ids, names []string
		var cs []Candidate
		for _, a := range as {
			ids = append(ids, a.ID)
			names = append(names, a.Name)
			for _, c := range r.in.Candidates {
				if containsStr(r.draft.Elements["artifact:"+a.ID].Candidates, c.ID) && c.Kind == KindHelmChart {
					cs = append(cs, c)
				}
			}
		}
		for _, c := range r.in.Candidates {
			if c.Kind == KindVersionRelation || c.Kind == KindChartRepo || c.Kind == KindHelmRepo || c.Kind == KindHelmOCI {
				cs = append(cs, c)
			}
		}
		a := as[0]
		r.draft.Questions = append(r.draft.Questions, Question{Kind: QuestionChartVersion, Subject: "artifact:" + a.ID,
			Summary: "How do the versions of chart(s) " + strings.Join(names, ", ") + " relate to the release version?", Candidates: cs,
			Facts: map[string]string{"chart": strings.Join(names, ", "), "artifacts": strings.Join(ids, ","), "strategy": a.Version.Strategy, "template": a.Version.Template + a.Version.Match}})
	}
}

func (r *resolver) imageCandidates() []Candidate {
	var out []Candidate
	for _, c := range r.byK[KindImage] {
		if r.productRelated(c.Value) {
			out = append(out, c)
		}
	}
	return out
}

func prefixAll(p string, xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = p + x
	}
	return out
}
