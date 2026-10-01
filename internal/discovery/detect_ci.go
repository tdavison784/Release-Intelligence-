package discovery

import (
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func isYAMLExt(p string) bool {
	e := strings.ToLower(path.Ext(p))
	return e == ".yaml" || e == ".yml"
}

func isWorkflowFile(p string) bool {
	low := strings.ToLower(p)
	base := path.Base(low)
	switch {
	case strings.HasPrefix(low, ".github/workflows/") && isYAMLExt(low):
		return true
	case low == ".gitlab-ci.yml", strings.HasPrefix(low, ".circleci/") && isYAMLExt(low):
		return true
	case strings.HasPrefix(low, ".tekton/") && isYAMLExt(low):
		return true
	case (strings.Contains(base, "cloudbuild") || strings.HasPrefix(low, "gcb/")) && isYAMLExt(low):
		return true
	}
	return false
}

func isScriptFile(p string) bool {
	low := strings.ToLower(p)
	ext := path.Ext(low)
	if ext == ".sh" || ext == ".bash" {
		return true
	}
	if ext != "" {
		return false
	}
	for _, d := range []string{"hack/", "release/", "scripts/", "build/"} {
		if strings.HasPrefix(low, d) || strings.Contains(low, "/"+d) {
			return true
		}
	}
	return false
}

// Trigger classes of CI pipelines.
const (
	triggerTag      = "tag"      // runs on tag pushes / release events
	triggerBranch   = "branch"   // runs on branch pushes, PRs or schedules only
	triggerReusable = "reusable" // called by other workflows
	triggerManual   = "manual"
	triggerUnknown  = "unknown"
)

type tagPattern struct {
	pattern string
	line    int
}

// workflowTriggers parses GitHub Actions `on:` (and recognises GitLab /
// Cloud Build tag pipelines heuristically).
func workflowTriggers(f *File) (string, []tagPattern) {
	low := strings.ToLower(f.Path)
	if !strings.HasPrefix(low, ".github/workflows/") {
		t := f.Text()
		if strings.Contains(t, "CI_COMMIT_TAG") || strings.Contains(t, "TAG_NAME") || regexp.MustCompile(`(?m)^\s*only:\s*\n\s*-\s*tags`).MatchString(t) {
			return triggerTag, nil
		}
		return triggerUnknown, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(f.Data, &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return triggerUnknown, nil
	}
	root := doc.Content[0]
	var on *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if k := root.Content[i].Value; k == "on" || k == "true" {
			on = root.Content[i+1]
		}
	}
	if on == nil {
		return triggerUnknown, nil
	}
	events := map[string]*yaml.Node{}
	switch on.Kind {
	case yaml.ScalarNode:
		events[on.Value] = nil
	case yaml.SequenceNode:
		for _, n := range on.Content {
			events[n.Value] = nil
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(on.Content); i += 2 {
			events[on.Content[i].Value] = on.Content[i+1]
		}
	}
	var tags []tagPattern
	hasBranch := false
	if push, ok := events["push"]; ok {
		if push == nil || push.Kind != yaml.MappingNode {
			hasBranch = true
		} else {
			for i := 0; i+1 < len(push.Content); i += 2 {
				k, v := push.Content[i].Value, push.Content[i+1]
				switch k {
				case "tags":
					for _, t := range seqOrScalar(v) {
						tags = append(tags, tagPattern{t.Value, t.Line})
					}
				case "branches", "branches-ignore", "paths", "paths-ignore":
					hasBranch = true
				}
			}
		}
	}
	_, release := events["release"]
	switch {
	case len(tags) > 0 || release:
		return triggerTag, tags
	case hasBranch:
		return triggerBranch, nil
	}
	if _, ok := events["workflow_call"]; ok {
		return triggerReusable, nil
	}
	for _, e := range []string{"pull_request", "pull_request_target", "schedule", "merge_group"} {
		if _, ok := events[e]; ok {
			return triggerBranch, nil
		}
	}
	if _, ok := events["workflow_dispatch"]; ok {
		return triggerManual, nil
	}
	return triggerUnknown, nil
}

func seqOrScalar(n *yaml.Node) []*yaml.Node {
	switch n.Kind {
	case yaml.SequenceNode:
		return n.Content
	case yaml.ScalarNode:
		return []*yaml.Node{n}
	}
	return nil
}

var (
	loginRe          = regexp.MustCompile(`\b(?:docker|podman|buildah|helm registry|crane auth|cosign|oras|skopeo|ko)\s+login\s+(?:-[-\w]+(?:[ =]\S+)?\s+)*([A-Za-z0-9.-]+\.[A-Za-z]{2,}(?::\d+)?(?:/[\w./-]+)?)`)
	loginActionRe    = regexp.MustCompile(`uses:\s*docker/login-action@`)
	registryKeyRe    = regexp.MustCompile(`^\s*registry:\s*['"]?([^'"\s#]+)`)
	helmPushRe       = regexp.MustCompile(`helm\S*\)?\s+push\s+(\S+)\s+(oci://\S+)`)
	goreleaserUseRe  = regexp.MustCompile(`goreleaser/goreleaser-action@|\bgoreleaser\s+release\b`)
	koUseRe          = regexp.MustCompile(`\bko\s+(?:build|publish|resolve|apply)\b|ko-build/setup-ko@|imjasonh/setup-ko@`)
	publisherRe      = regexp.MustCompile(`softprops/action-gh-release@|ncipollo/release-action@|actions/create-release@|actions/upload-release-asset@|svenstaro/upload-release-action@|\bgh release (?:create|upload)\b|upload-assets:\s*true|marvinpinto/action-automatic-releases@`)
	chartReleaserRe  = regexp.MustCompile(`helm/chart-releaser-action@`)
	cosignSignRe     = regexp.MustCompile(`\bcosign\s+sign\b|sigstore/cosign-installer@`)
	assetURLRe       = regexp.MustCompile(`https?://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/releases/download/((?:\{\{[^}]*\}\}|\[\[[^\]]*\]\]|[^\s"'<>)\]` + "`" + `])+)`)
	helmRepoAddRe    = regexp.MustCompile(`helm\s+repo\s+add\s+(?:--?[\w-]+(?:[ =]\S+)?\s+)*([A-Za-z0-9_.-]+)\s+(https?://[^\s"'<>)` + "`" + `]+)`)
	helmInstallRe    = regexp.MustCompile(`helm\s+(?:upgrade\s+--install|install|upgrade|template|pull|show\s+\w+)\s+(?:[^\s]+\s+)?([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)`)
	registryVarRe    = regexp.MustCompile(`(?i)(^|_)(registry|hub|image_?repo(sitory)?|image_?namespace|docker_?repo|ko_docker_repo|container_?registry|image_?prefix|oci_?repo)$`)
	ldflagsVersionRe = regexp.MustCompile(`-X\s*=?\s*['"]?[\w./-]+\.(?:[A-Za-z]*[Vv]ersion)=([^\s'"]+)`)
)

func detectWorkflow(st *scanState, f *File) {
	trigger, tags := workflowTriggers(f)
	ctx := pathContext(f.Path)
	if ctx == ctxDocs {
		ctx = ctxNone
	}
	for _, t := range tags {
		if strings.HasPrefix(t.pattern, "!") {
			continue
		}
		st.emit(KindReleaseTrigger, t.pattern, domain.ConfidenceHigh, "workflow.tag-trigger",
			map[string]string{"files": f.Path}, f.Evidence(t.line))
	}
	vars := collectVars(f.Text(), "yaml")
	lines := f.Lines()
	attrs := func(extra map[string]string) map[string]string {
		a := map[string]string{"triggers": trigger, "files": f.Path, "sources": "workflow", "contexts": ctx}
		for k, v := range extra {
			a[k] = v
		}
		return a
	}
	base := domain.ConfidenceMedium
	if trigger == triggerTag {
		base = domain.ConfidenceHigh
	}
	for i, line := range lines {
		n := i + 1
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			continue
		}
		el := vars.expand(line)
		if m := loginRe.FindStringSubmatch(el); m != nil {
			if reg := registryFromValue(m[1]); reg != "" {
				st.emit(KindRegistry, reg, confFor(ctx, base), "workflow.registry-login", attrs(nil), f.Evidence(n))
			}
		}
		if loginActionRe.MatchString(line) {
			reg, at := "docker.io", n
			for j := i + 1; j < len(lines) && j < i+10; j++ {
				if strings.Contains(lines[j], "uses:") || strings.HasPrefix(strings.TrimSpace(lines[j]), "- name:") {
					break
				}
				if m := registryKeyRe.FindStringSubmatch(vars.expand(lines[j])); m != nil {
					reg, at = m[1], j+1
					break
				}
			}
			if r := registryFromValue(reg); r != "" {
				st.emit(KindRegistry, r, confFor(ctx, base), "workflow.registry-login", attrs(nil), f.Evidence(at))
			}
		}
		for _, r := range findImageRefs(el) {
			st.imageRef(f, n, r, "workflow", ctx, trigger, base)
		}
		if m := helmPushRe.FindStringSubmatch(el); m != nil {
			st.emit(KindHelmOCI, strings.TrimSuffix(strings.TrimPrefix(m[2], "oci://"), "/"), confFor(ctx, base), "workflow.helm-push",
				attrs(map[string]string{"prefix": "true", "chartFile": m[1]}), f.Evidence(n))
		}
		if goreleaserUseRe.MatchString(line) {
			st.emit(KindBuildTool, "goreleaser", base, "workflow.goreleaser", attrs(nil), f.Evidence(n))
			if trigger == triggerTag {
				st.emit(KindReleasePublisher, "goreleaser", domain.ConfidenceHigh, "workflow.goreleaser", attrs(nil), f.Evidence(n))
			}
		}
		if koUseRe.MatchString(line) {
			st.emit(KindBuildTool, "ko", base, "workflow.ko", attrs(nil), f.Evidence(n))
		}
		if m := publisherRe.FindString(line); m != "" && trigger != triggerBranch {
			st.emit(KindReleasePublisher, strings.TrimSuffix(strings.TrimSpace(m), "@"), base, "workflow.release-upload", attrs(nil), f.Evidence(n))
		}
		if chartReleaserRe.MatchString(line) {
			st.emit(KindBuildTool, "chart-releaser", base, "workflow.chart-releaser", attrs(nil), f.Evidence(n))
			if st.info.Repo.IsGitHub() {
				slug := st.info.Repo.Slug()
				u := "https://" + strings.ToLower(st.info.Repo.Owner) + ".github.io/" + st.info.Repo.Name
				st.emit(KindHelmRepo, u, domain.ConfidenceMedium, "workflow.chart-releaser-pages", attrs(nil), f.Evidence(n))
				// chart-releaser writes index.yaml to the gh-pages branch; the
				// same index is reachable on raw.githubusercontent.com even
				// when the *.github.io host is not.
				st.emit(KindHelmRepo, "https://raw.githubusercontent.com/"+slug+"/gh-pages", domain.ConfidenceMedium,
					"workflow.chart-releaser-pages-raw", attrs(map[string]string{"pagesIndex": "true"}), f.Evidence(n))
			}
		}
		if cosignSignRe.MatchString(line) {
			st.emit(KindBuildTool, "cosign", base, "workflow.cosign", attrs(nil), f.Evidence(n))
		}
		st.assetURLs(f, n, el, "workflow", ctx)
	}
}

func detectScript(st *scanState, f *File) {
	ctx := pathContext(f.Path)
	if ctx == ctxDocs {
		ctx = ctxNone
	}
	vars := collectVars(f.Text(), "shell")
	vars.merge(st.makeVars)
	low := strings.ToLower(f.Path)
	base := domain.ConfidenceMedium
	if strings.Contains(low, "release") || strings.Contains(low, "publish") {
		base = domain.ConfidenceHigh
	}
	for i, line := range f.Lines() {
		n := i + 1
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		el := vars.expand(line)
		for _, r := range findImageRefs(el) {
			st.imageRef(f, n, r, "script", ctx, "", base)
		}
		if m := helmPushRe.FindStringSubmatch(el); m != nil {
			st.emit(KindHelmOCI, strings.TrimSuffix(strings.TrimPrefix(m[2], "oci://"), "/"), confFor(ctx, base), "script.helm-push",
				map[string]string{"prefix": "true", "files": f.Path, "contexts": ctx}, f.Evidence(n))
		}
		st.helmRepoRefs(f, n, el, ctx, "script")
		st.assetURLs(f, n, el, "script", ctx)
	}
	st.registryVars(f, collectVars(f.Text(), "shell"), ctx, "script")
}

// registryFromValue turns a registry-ish value into "host" or
// "host/namespace". Bare namespaces map to Docker Hub.
func registryFromValue(v string) string {
	v = strings.Trim(strings.TrimSpace(v), `"'`)
	v = strings.TrimPrefix(v, "oci://")
	v = strings.TrimPrefix(v, "https://")
	v = strings.TrimSuffix(v, "/")
	if v == "" || strings.ContainsAny(v, " $%{}()") {
		return ""
	}
	segs := strings.Split(v, "/")
	if isRegistryHost(segs[0]) {
		segs[0] = normalizeRegistry(segs[0])
		return strings.Join(segs, "/")
	}
	if len(segs) == 1 && regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`).MatchString(v) {
		return "docker.io/" + v
	}
	return ""
}

// registryVars emits registry candidates from variables named like
// registries (HUB, IMAGE_REGISTRY, KO_DOCKER_REPO, …).
func (st *scanState) registryVars(f *File, local varTable, ctx, source string) {
	if len(local) == 0 {
		return
	}
	all := varTable{}
	all.merge(local)
	all.merge(st.makeVars)
	lines := f.Lines()
	for name, raw := range local {
		if !registryVarRe.MatchString(name) {
			continue
		}
		val := all.expand(raw)
		reg := registryFromValue(val)
		if reg == "" {
			continue
		}
		line := 0
		re := regexp.MustCompile(`(^|[\s;])(?:export\s+)?` + regexp.QuoteMeta(name) + `\s*(\?=|:=|::=|=|\+=|:)`)
		for i, l := range lines {
			if re.MatchString(l) {
				line = i + 1
				break
			}
		}
		conf := confFor(ctx, domain.ConfidenceMedium)
		attrs := map[string]string{"files": f.Path, "sources": source, "contexts": ctx, "variable": name}
		if !strings.Contains(strings.Split(val, "/")[0], ".") {
			attrs["implicitDockerHub"] = "true"
			conf = domain.ConfidenceLow
		}
		ev := []domain.Evidence{}
		if line > 0 {
			ev = append(ev, f.Evidence(line))
		}
		st.emit(KindRegistry, reg, conf, source+".registry-variable", attrs, ev...)
	}
}

var assignNameRe = regexp.MustCompile(`^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*(?:\?=|:=|::=|=|:)`)

// registryAssignment reports whether line n assigns a registry-named
// variable (its value is a registry, not an image).
func (st *scanState) registryAssignment(f *File, n int) bool {
	lines := f.Lines()
	if n < 1 || n > len(lines) {
		return false
	}
	m := assignNameRe.FindStringSubmatch(lines[n-1])
	return m != nil && registryVarRe.MatchString(m[1])
}

// imageRef turns a found registry reference into candidates.
func (st *scanState) imageRef(f *File, n int, r foundRef, source, ctx, trigger string, base domain.Confidence) {
	conf := confFor(ctx, base)
	attrs := map[string]string{"files": f.Path, "sources": source, "contexts": ctx, "triggers": trigger}
	isChart := r.OCI || strings.Contains("/"+r.Path+"/", "/charts/") || strings.Contains("/"+r.Path+"/", "/helm/")
	if isChart {
		v := r.Repository()
		if r.Partial {
			attrs["prefix"] = "true"
		}
		if r.Tag != "" {
			attrs["tags"] = r.Tag
		}
		st.emit(KindHelmOCI, strings.TrimSuffix(v, "/"), conf, source+".oci-ref", attrs, f.Evidence(n))
		return
	}
	if r.Partial || (!strings.Contains(r.Path, "/") && r.Tag == "" && r.Digest == "") || st.registryAssignment(f, n) {
		st.emit(KindRegistry, r.Registry+"/"+r.Path, confFor(ctx, domain.ConfidenceLow), source+".registry-ref", attrs, f.Evidence(n))
		return
	}
	refTag, refVer := st.refTag()
	class, tmpl := classifyTag(r.Tag, refTag, refVer)
	if class == tagNone && r.Digest != "" {
		class = tagPinned
	}
	if trigger == triggerBranch && (class == tagUnresolved || class == tagNone) {
		class = tagSnapshot
	}
	attrs["classes"] = class
	attrs["tagTemplate"] = tmpl
	if r.Tag != "" {
		attrs["tags"] = r.Tag
	}
	if r.Digest != "" {
		attrs["digest"] = "true"
	}
	st.emit(KindImage, r.Repository(), conf, source+".image-ref", attrs, f.Evidence(n))
}

// assetURLs emits hosted-release asset URLs of the product repository.
func (st *scanState) assetURLs(f *File, n int, line, source, ctx string) {
	for _, m := range assetURLRe.FindAllStringSubmatch(line, -1) {
		if !st.selfRepo(m[1], m[2]) {
			continue
		}
		u := strings.TrimRight(m[0], ".,;:")
		tmpl, ok := templatizeAssetURL(u, st.tagPrefix())
		conf := confFor(ctx, domain.ConfidenceMedium)
		attrs := map[string]string{"files": f.Path, "sources": source, "contexts": ctx, "file": path.Base(tmpl)}
		if !ok {
			attrs["unresolved"] = "true"
			conf = domain.ConfidenceLow
		}
		st.emit(KindReleaseAsset, tmpl, conf, source+".release-asset-url", attrs, f.Evidence(n))
	}
}

// helmRepoRefs records `helm repo add` URLs and the charts installed from
// them in the same file.
func (st *scanState) helmRepoRefs(f *File, n int, line, ctx, source string) {
	if m := helmRepoAddRe.FindStringSubmatch(line); m != nil {
		u := strings.TrimRight(m[2], "/.,;")
		attrs := map[string]string{"alias": m[1], "files": f.Path, "contexts": ctx, "sources": source}
		// charts installed from this alias anywhere in the file
		for _, l := range f.Lines() {
			for _, im := range helmInstallRe.FindAllStringSubmatch(l, -1) {
				if im[1] == m[1] {
					attrs["charts"] = joinSet(attrs["charts"], im[2])
				}
			}
		}
		if dir := path.Dir(f.Path); st.has(dir+"/Chart.yaml") || st.has(dir+"/Chart.template.yaml") {
			attrs["chartDir"] = dir
		}
		st.emit(KindHelmRepo, u, confFor(ctx, domain.ConfidenceMedium), source+".helm-repo-add", attrs, f.Evidence(n))
	}
}
