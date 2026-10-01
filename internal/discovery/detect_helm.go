package discovery

import (
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func isChartFile(p string) bool {
	b := path.Base(p)
	return b == "Chart.yaml" || b == "Chart.yml" || b == "Chart.template.yaml"
}

func isValuesFile(p string) bool {
	b := path.Base(p)
	return b == "values.yaml" || b == "values.yml"
}

func chartDirHas(st *scanState, dir string) bool {
	return st.has(dir+"/Chart.yaml") || st.has(dir+"/Chart.yml") || st.has(dir+"/Chart.template.yaml")
}

type chartMeta struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	AppVersion  string `yaml:"appVersion"`
	KubeVersion string `yaml:"kubeVersion"`
	Type        string `yaml:"type"`
}

var (
	placeholderVersionRe = regexp.MustCompile(`^v?0\.0\.0(-.*)?$|\{\{|\$`)
	trivialVersionRe     = regexp.MustCompile(`^v?(0\.0\.0|0\.0\.1|0\.1\.0|1\.0\.0)$`)
	stampCommentRe       = regexp.MustCompile(`(?i)(set|replaced?|overwritten|stamped|updated|filled|injected)\b.*\b(automatically|at build|build[- ]time|by the release|release tool|during release|by ci|by make)|never (actually )?shipped|placeholder`)
)

func detectChart(st *scanState, f *File) {
	var c chartMeta
	if err := yaml.Unmarshal(f.Data, &c); err != nil || c.Name == "" {
		return
	}
	dir := path.Dir(f.Path)
	ctx := pathContext(f.Path)
	if ctx == ctxDocs {
		ctx = ctxNone
	}
	lines := f.Lines()
	versionLine := 1
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "version:") {
			versionLine = i + 1
			break
		}
	}
	placeholder := placeholderVersionRe.MatchString(c.Version) || (c.Version == c.AppVersion && trivialVersionRe.MatchString(c.Version))
	for i := versionLine - 4; i < versionLine+2 && !placeholder; i++ {
		if i >= 0 && i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "#") && stampCommentRe.MatchString(lines[i]) {
			placeholder = true
		}
	}
	attrs := map[string]string{
		"dir": dir, "chartFile": f.Path, "version": c.Version, "appVersion": c.AppVersion,
		"kubeVersion": c.KubeVersion, "type": c.Type, "contexts": ctx,
	}
	if placeholder {
		attrs["placeholder"] = "true"
	}
	if strings.HasPrefix(c.Version, "v") {
		attrs["versionPrefix"] = "v"
	}
	conf := confFor(ctx, domain.ConfidenceHigh)
	if c.Type == "library" {
		conf = domain.ConfidenceLow
	}
	ev := []domain.Evidence{f.Evidence(versionLine)}
	for i, l := range lines {
		if strings.HasPrefix(l, "name:") {
			ev = append([]domain.Evidence{f.Evidence(i + 1)}, ev...)
			break
		}
	}
	// One candidate per chart directory: charts may share a name across
	// directories (published chart vs sample).
	st.emit(KindHelmChart, c.Name+"@"+dir, conf, "helm.chart", attrs, ev...)
	if !placeholder {
		if refTag, refVer := st.refTag(); refTag != "" {
			if _, t := classifyTag(c.Version, refTag, refVer); t != "" {
				st.emit(KindVersionRelation, "chart.version = "+t, conf, "helm.chart-version-matches-ref",
					map[string]string{"subject": "chart.version", "template": t, "chart": c.Name, "files": f.Path}, f.Evidence(versionLine))
			}
		}
	}
}

// valuesCtx carries registry defaults through a values.yaml walk.
type valuesCtx struct {
	registry, namespace string
}

func detectValues(st *scanState, f *File) {
	dir := path.Dir(f.Path)
	if !chartDirHas(st, dir) {
		return
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(f.Data, &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return
	}
	ctx := pathContext(f.Path)
	if ctx == ctxDocs {
		ctx = ctxNone
	}
	root := doc.Content[0]
	vc := valuesCtx{}
	globalSrc := []*yaml.Node{root}
	if g := mapGet(root, "global"); g != nil && g.Kind == yaml.MappingNode {
		globalSrc = append(globalSrc, g)
		if gi := mapGet(g, "image"); gi != nil && gi.Kind == yaml.MappingNode {
			globalSrc = append(globalSrc, gi)
		}
	}
	for _, n := range globalSrc {
		if v := scalar(mapGet(n, "imageRegistry")); v != "" && vc.registry == "" {
			vc.registry = v
		}
		if v := scalar(mapGet(n, "imageNamespace")); v != "" && vc.namespace == "" {
			vc.namespace = v
		}
	}
	walkValues(st, f, root, "", vc, ctx, dir)
}

func mapGet(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func mapKeyLine(n *yaml.Node, key string) int {
	if n == nil || n.Kind != yaml.MappingNode {
		return 0
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i].Line
		}
	}
	return 0
}

func scalar(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return strings.TrimSpace(n.Value)
}

func walkValues(st *scanState, f *File, n *yaml.Node, parentKey string, vc valuesCtx, ctx, dir string) {
	switch n.Kind {
	case yaml.SequenceNode:
		for _, c := range n.Content {
			walkValues(st, f, c, parentKey, vc, ctx, dir)
		}
		return
	case yaml.MappingNode:
	default:
		return
	}
	attrs := func() map[string]string {
		return map[string]string{"sources": "helm-values", "chartDir": dir, "files": f.Path, "contexts": ctx}
	}
	repo, reg, name, tag := scalar(mapGet(n, "repository")), scalar(mapGet(n, "registry")), scalar(mapGet(n, "name")), scalar(mapGet(n, "tag"))
	full, line := "", 0
	switch {
	case repo != "" && (strings.Contains(repo, "/") || isRegistryHost(strings.Split(repo, "/")[0])):
		full, line = repo, mapKeyLine(n, "repository")
		if reg != "" && !isRegistryHost(strings.Split(repo, "/")[0]) {
			full = reg + "/" + repo
		}
	case parentKey == "image" && name != "" && (reg != "" || vc.registry != ""):
		r := reg
		if r == "" {
			r = vc.registry
		}
		full, line = r, mapKeyLine(n, "name")
		if vc.namespace != "" {
			full += "/" + vc.namespace
		}
		full += "/" + name
	}
	if full != "" {
		if ref, ok := parseImageValue(full); ok && isRegistryHost(ref.Registry) {
			a := attrs()
			refTag, refVer := st.refTag()
			class, tmpl := classifyTag(tag, refTag, refVer)
			if tag == "" {
				class = "chart-appversion"
				a["tagDefault"] = "appVersion"
			}
			a["classes"], a["tagTemplate"] = class, tmpl
			if tag != "" {
				a["tags"] = tag
			}
			st.emit(KindImage, ref.Repository(), confFor(ctx, domain.ConfidenceHigh), "helm.values-image", a, f.Evidence(line))
		}
	}
	if hub := scalar(mapGet(n, "hub")); hub != "" {
		if r := registryFromValue(hub); r != "" {
			a := attrs()
			a["variable"] = "hub"
			st.emit(KindRegistry, r, confFor(ctx, domain.ConfidenceMedium), "helm.values-hub", a, f.Evidence(mapKeyLine(n, "hub")))
		}
	}
	if img := scalar(mapGet(n, "image")); img != "" && !strings.ContainsAny(img, "{$") {
		line := mapKeyLine(n, "image")
		if ref, ok := parseImageValue(img); ok && strings.Contains(img, "/") && isRegistryHost(ref.Registry) && strings.Contains(strings.Split(img, "/")[0], ".") {
			st.emit(KindImage, ref.Repository(), confFor(ctx, domain.ConfidenceMedium), "helm.values-image", attrs(), f.Evidence(line))
		} else if !strings.ContainsAny(img, "/:") {
			st.emit(KindImageName, img, confFor(ctx, domain.ConfidenceMedium), "helm.values-image-name", attrs(), f.Evidence(line))
		}
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		walkValues(st, f, n.Content[i+1], n.Content[i].Value, vc, ctx, dir)
	}
}
