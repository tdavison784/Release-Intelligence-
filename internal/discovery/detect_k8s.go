package discovery

import (
	"bytes"
	"errors"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// isYAMLFile selects YAML files for the generic YAML detector (manifests,
// CRDs, kustomizations, image build lists, compatibility data).
func isYAMLFile(p string) bool {
	if !isYAMLExt(p) || isWorkflowFile(p) || isChartFile(p) || isGoreleaserFile(p) || isKoFile(p) {
		return false
	}
	if strings.HasPrefix(strings.ToLower(path.Base(p)), "values") {
		return false
	}
	return !strings.Contains("/"+p, "/templates/")
}

var (
	crdKindRe      = regexp.MustCompile(`(?m)^kind:\s*CustomResourceDefinition\s*$`)
	workloadKindRe = regexp.MustCompile(`(?m)^kind:\s*(Deployment|DaemonSet|StatefulSet|Job|CronJob|Pod|ReplicaSet)\s*$`)
	imageLineRe    = regexp.MustCompile(`^\s*-?\s*image:\s*['"]?([^'"\s#]+)`)
	imageArgRe     = regexp.MustCompile(`--[\w-]*image[\w-]*=([^\s'"]+)`)
	manifestDirRe  = regexp.MustCompile(`(?i)(^|/)(manifests?|deploy|deployments?|install|config|dist|bundle)(/|$)`)
	compatKeyRe    = regexp.MustCompile(`(?i)k8s|kubernetes`)
)

func detectYAML(st *scanState, f *File) {
	base := strings.ToLower(path.Base(f.Path))
	ctx := pathContext(f.Path)
	if ctx == ctxDocs {
		ctx = ctxNone
	}
	text := f.Text()
	if compatKeyRe.MatchString(text) {
		detectCompatYAML(st, f)
	}
	if st.in.Profile == ProfileDocs {
		return
	}
	switch base {
	case "kustomization.yaml", "kustomization.yml":
		detectKustomize(st, f, ctx)
		return
	case "security-insights.yml", "security-insights.yaml":
		detectSecurityInsights(st, f)
		return
	}
	if strings.Contains(text, "dockerfile:") {
		detectImageBuildList(st, f)
	}
	if crdKindRe.MatchString(text) {
		detectCRDs(st, f, ctx)
	}
	if strings.Contains(text, "image:") || strings.Contains(text, "image=") {
		detectManifestImages(st, f, ctx)
	}
}

func detectManifestImages(st *scanState, f *File, ctx string) {
	refTag, refVer := st.refTag()
	var images []string
	workload := workloadKindRe.MatchString(f.Text())
	base := domain.ConfidenceMedium
	if workload && ctx == ctxNone && manifestDirRe.MatchString(path.Dir(f.Path)) {
		base = domain.ConfidenceHigh
	}
	for i, line := range f.Lines() {
		var refs []imageRef
		if m := imageLineRe.FindStringSubmatch(line); m != nil {
			if r, ok := parseImageValue(m[1]); ok {
				refs = append(refs, r)
			}
		}
		for _, m := range imageArgRe.FindAllStringSubmatch(line, -1) {
			for _, r := range findImageRefs(m[1]) {
				if !r.Partial {
					refs = append(refs, r.imageRef)
				}
			}
		}
		for _, r := range refs {
			class, tmpl := classifyTag(r.Tag, refTag, refVer)
			conf := confFor(ctx, base)
			if class != tagRelease && conf == domain.ConfidenceHigh {
				conf = domain.ConfidenceMedium
			}
			attrs := map[string]string{"files": f.Path, "sources": "manifest", "contexts": ctx, "classes": class, "tagTemplate": tmpl, "tags": r.Tag}
			st.emit(KindImage, r.Repository(), conf, "manifest.image", attrs, f.Evidence(i+1))
			img := r.Repository()
			if r.Tag != "" {
				img += ":" + r.Tag
			}
			images = append(images, img)
		}
	}
	if len(images) == 0 || !workload || ctx != ctxNone {
		return
	}
	conf := domain.ConfidenceMedium
	if manifestDirRe.MatchString(path.Dir(f.Path)) || strings.Contains(strings.ToLower(path.Base(f.Path)), "install") {
		conf = domain.ConfidenceHigh
	}
	sort.Strings(images)
	line := 1
	if loc := workloadKindRe.FindStringIndex(f.Text()); loc != nil {
		line = strings.Count(f.Text()[:loc[0]], "\n") + 1
	}
	st.emit(KindManifest, f.Path, conf, "manifest.workloads",
		map[string]string{"images": strings.Join(dedupe(images), ","), "crds": boolStr(crdKindRe.MatchString(f.Text()))}, f.Evidence(line))
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return ""
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func detectCRDs(st *scanState, f *File, ctx string) {
	dec := yaml.NewDecoder(bytes.NewReader(f.Data))
	var names, groups []string
	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Spec struct {
				Group string `yaml:"group"`
			} `yaml:"spec"`
		}
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			break
		}
		if doc.Kind == "CustomResourceDefinition" && doc.Metadata.Name != "" {
			names = append(names, doc.Metadata.Name)
			if doc.Spec.Group != "" && !containsStr(groups, doc.Spec.Group) {
				groups = append(groups, doc.Spec.Group)
			}
		}
	}
	if len(names) == 0 {
		return
	}
	line := 1
	if loc := crdKindRe.FindStringIndex(f.Text()); loc != nil {
		line = strings.Count(f.Text()[:loc[0]], "\n") + 1
	}
	bundled := ""
	if workloadKindRe.MatchString(f.Text()) {
		bundled = "true"
	}
	st.emit(KindCRD, f.Path, confFor(ctx, domain.ConfidenceHigh), "crd.file",
		map[string]string{"names": strings.Join(names, ","), "groups": strings.Join(groups, ","), "count": itoa(len(names)), "dir": path.Dir(f.Path), "bundled": bundled, "contexts": ctx},
		f.Evidence(line))
}

func detectKustomize(st *scanState, f *File, ctx string) {
	var k struct {
		Images []struct {
			Name    string `yaml:"name"`
			NewName string `yaml:"newName"`
			NewTag  string `yaml:"newTag"`
		} `yaml:"images"`
	}
	if yaml.Unmarshal(f.Data, &k) != nil {
		return
	}
	refTag, refVer := st.refTag()
	for _, im := range k.Images {
		name := im.NewName
		if name == "" {
			name = im.Name
		}
		r, ok := parseImageValue(name)
		if !ok {
			continue
		}
		line := 1
		for i, l := range f.Lines() {
			if strings.Contains(l, name) {
				line = i + 1
			}
			if im.NewTag != "" && strings.Contains(l, "newTag: "+im.NewTag) {
				line = i + 1
				break
			}
		}
		class, tmpl := classifyTag(im.NewTag, refTag, refVer)
		st.emit(KindImage, r.Repository(), confFor(ctx, domain.ConfidenceHigh), "kustomize.image",
			map[string]string{"files": f.Path, "sources": "kustomize", "contexts": ctx, "classes": class, "tagTemplate": tmpl, "tags": im.NewTag},
			f.Evidence(line))
	}
}

// detectImageBuildList recognises YAML lists describing the images a repo
// builds: items with both a "name" and a "dockerfile" key.
func detectImageBuildList(st *scanState, f *File) {
	var doc yaml.Node
	if yaml.Unmarshal(f.Data, &doc) != nil || len(doc.Content) == 0 {
		return
	}
	var visit func(n *yaml.Node, parentKey string)
	visit = func(n *yaml.Node, parentKey string) {
		switch n.Kind {
		case yaml.MappingNode:
			name, df := scalar(mapGet(n, "name")), scalar(mapGet(n, "dockerfile"))
			if name != "" && df != "" {
				if scalar(mapGet(n, "base")) == "true" || parentKey == "example" || parentKey == "examples" {
					return
				}
				ctx := pathContext(df)
				conf := domain.ConfidenceMedium
				if ctx == ctxTest || ctx == ctxSample {
					conf = domain.ConfidenceLow
				}
				st.emit(KindImageName, name, conf, "build.image-list",
					map[string]string{"dockerfile": df, "files": f.Path, "sources": "build-list", "contexts": ctx}, f.Evidence(mapKeyLine(n, "name")))
				return
			}
			for i := 0; i+1 < len(n.Content); i += 2 {
				visit(n.Content[i+1], n.Content[i].Value)
			}
		case yaml.SequenceNode, yaml.DocumentNode:
			for _, c := range n.Content {
				visit(c, parentKey)
			}
		}
	}
	visit(&doc, "")
}

// detectCompatYAML recognises support matrices kept as YAML records, e.g.
// [{version: "1.31", k8sVersions: [...]}].
func detectCompatYAML(st *scanState, f *File) {
	var recs []map[string]any
	if yaml.Unmarshal(f.Data, &recs) != nil || len(recs) < 2 {
		return
	}
	keyField := ""
	supported, tested := map[string]bool{}, map[string]bool{}
	for _, r := range recs {
		for k := range r {
			lk := strings.ToLower(k)
			switch {
			case compatKeyRe.MatchString(k):
				if strings.Contains(lk, "test") {
					tested[k] = true
				} else {
					supported[k] = true
				}
			case keyField == "" && (lk == "version" || lk == "release" || lk == "minor"):
				keyField = k
			}
		}
	}
	versionRows := 0
	for _, r := range recs {
		if v, ok := r[keyField]; ok && len(findVersionTokens(strings.TrimSpace(toString(v)))) > 0 {
			versionRows++
		}
	}
	if keyField == "" || len(supported)+len(tested) == 0 || versionRows < 2 {
		return
	}
	line := 1
	for i, l := range f.Lines() {
		if compatKeyRe.MatchString(l) {
			line = i + 1
			break
		}
	}
	st.emit(KindCompatibility, f.Path, domain.ConfidenceHigh, "compat.yaml-records",
		map[string]string{"format": "yaml-records", "keyColumns": keyField, "supportedHeaders": joinKeys(supported), "testedHeaders": joinKeys(tested), "rows": itoa(len(recs))},
		f.Evidence(line))
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	}
	b, _ := yaml.Marshal(v)
	return strings.TrimSpace(string(b))
}

func joinKeys(m map[string]bool) string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

var (
	quayRepoURLRe   = regexp.MustCompile(`https?://quay\.io/repository/([a-z0-9_.-]+)/([a-z0-9_.-]+)`)
	dockerHubURLRe  = regexp.MustCompile(`https?://hub\.docker\.com/r/([a-z0-9_.-]+)/([a-z0-9_.-]+)`)
	ghReleasesURLRe = regexp.MustCompile(`https?://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/releases\b`)
)

// detectSecurityInsights reads OpenSSF SECURITY-INSIGHTS distribution
// points and policy links.
func detectSecurityInsights(st *scanState, f *File) {
	for i, line := range f.Lines() {
		n := i + 1
		if m := quayRepoURLRe.FindStringSubmatch(line); m != nil {
			st.emit(KindImage, "quay.io/"+m[1]+"/"+m[2], domain.ConfidenceMedium, "openssf.distribution-point",
				map[string]string{"sources": "security-insights", "files": f.Path}, f.Evidence(n))
		}
		if m := dockerHubURLRe.FindStringSubmatch(line); m != nil {
			st.emit(KindImage, "docker.io/"+m[1]+"/"+m[2], domain.ConfidenceMedium, "openssf.distribution-point",
				map[string]string{"sources": "security-insights", "files": f.Path}, f.Evidence(n))
		}
		if m := ghReleasesURLRe.FindStringSubmatch(line); m != nil && st.selfRepo(m[1], m[2]) {
			st.emit(KindReleasePublisher, "github-releases", domain.ConfidenceMedium, "openssf.distribution-point",
				map[string]string{"files": f.Path}, f.Evidence(n))
		}
	}
}
