// Installed product identification: which product/chart is installed in the
// environment and at what version, detected from Helm metadata annotations,
// standard labels, Argo CD Application specs and Flux HelmRelease specs
// (plus Helmfile releases in repo mode). Best-effort by design: a guess is a
// guess, so every entry records the mechanism that grounded it and cites the
// exact fields it came from.

package env

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// Sources of installed-product facts.
const (
	InstalledHelmAnnotation = "helm-annotation"
	InstalledLabel          = "label"
	InstalledArgoCD         = "argocd-application"
	InstalledFlux           = "flux-helmrelease"
	InstalledHelmfile       = "helmfile"
)

// maxInstalledEvidence caps the evidence cited per InstalledProduct entry.
const maxInstalledEvidence = 5

// InstalledProduct is a best-effort identification of an installed product.
// Version and Chart are recorded exactly as stated (a HelmRelease version,
// for example, may be a range such as "1.*"); "" means not stated.
type InstalledProduct struct {
	Product  string // release name, app/chart name or Helmfile release name — the best guess available
	Instance string // app.kubernetes.io/instance when stated
	Version  string // as stated (may be a range or a branch); "" when unknown
	Chart    string // chart identity as stated ("cert-manager-v1.17.0", "jetstack/cert-manager"); "" when unknown
	Source   string // which mechanism grounded this guess (InstalledHelmAnnotation, ...)
	Evidence []domain.EvidenceID
}

// detectInstalled runs every manifest-level detection over one document.
func (l *loader) detectInstalled(d doc) {
	l.detectHelmMetadata(d)
	l.detectAppLabels(d)
	apiVersion := scalarOf(d.node, "apiVersion")
	kind := scalarOf(d.node, "kind")
	group, _ := splitGroupVersion(apiVersion)
	switch {
	case kind == "Application" && group == "argoproj.io":
		l.detectArgoApplication(d)
	case kind == "HelmRelease" && group == "helm.toolkit.fluxcd.io":
		l.detectFluxHelmRelease(d)
	}
}

// scalarLine returns the scalar value at a mapping key and its 1-based line
// (0 when absent).
func scalarLine(n *yaml.Node, key string) (string, int) {
	m := resolveAlias(n)
	if m == nil || m.Kind != yaml.MappingNode {
		return "", 0
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != key {
			continue
		}
		v := resolveAlias(m.Content[i+1])
		if v == nil || v.Kind != yaml.ScalarNode {
			return "", 0
		}
		return v.Value, v.Line
	}
	return "", 0
}

// chartVersionOf peels the version segment off a helm.sh/chart annotation
// value ("cert-manager-v1.17.0" → "v1.17.0",
// "kube-prometheus-stack-70.4.1" → "70.4.1", "1.17.0" → "1.17.0"). Anything
// that does not end in a (optionally v-prefixed) numeric segment yields "".
func chartVersionOf(chart string) string {
	if chart == "" {
		return ""
	}
	if i := strings.LastIndexByte(chart, '-'); i >= 0 {
		s := chart[i+1:]
		if digitStart(s) || (len(s) > 1 && (s[0] == 'v' || s[0] == 'V') && digitStart(s[1:])) {
			return s
		}
		return ""
	}
	// no dash: the annotation may be the bare version
	if digitStart(chart) || (len(chart) > 1 && (chart[0] == 'v' || chart[0] == 'V') && digitStart(chart[1:])) {
		return chart
	}
	return ""
}

func digitStart(s string) bool {
	return len(s) > 0 && s[0] >= '0' && s[0] <= '9'
}

// detectHelmMetadata reads meta.helm.sh/release-name and helm.sh/chart.
func (l *loader) detectHelmMetadata(d doc) {
	ann := fieldOf(d.node, "metadata", "annotations")
	release, rl := scalarLine(ann, "meta.helm.sh/release-name")
	chart, cl := scalarLine(ann, "helm.sh/chart")
	if release == "" && chart == "" {
		return
	}
	product := release
	if product == "" {
		product = chart
	}
	var ev []domain.EvidenceID
	if release != "" {
		ev = append(ev, l.ev(d.file, fmt.Sprintf("$.metadata.annotations[meta.helm.sh/release-name] (L%d)", rl), "meta.helm.sh/release-name: "+release))
	}
	if chart != "" {
		ev = append(ev, l.ev(d.file, fmt.Sprintf("$.metadata.annotations[helm.sh/chart] (L%d)", cl), "helm.sh/chart: "+chart))
	}
	l.installed = append(l.installed, InstalledProduct{
		Product: product, Version: chartVersionOf(chart), Chart: chart,
		Source: InstalledHelmAnnotation, Evidence: ev,
	})
}

// detectAppLabels reads the app.kubernetes.io/* recommendations.
func (l *loader) detectAppLabels(d doc) {
	lbl := fieldOf(d.node, "metadata", "labels")
	name, nl := scalarLine(lbl, "app.kubernetes.io/name")
	instance, il := scalarLine(lbl, "app.kubernetes.io/instance")
	version, vl := scalarLine(lbl, "app.kubernetes.io/version")
	if name == "" && instance == "" {
		return
	}
	product := name
	if product == "" {
		product = instance
	}
	var ev []domain.EvidenceID
	if name != "" {
		ev = append(ev, l.ev(d.file, fmt.Sprintf("$.metadata.labels[app.kubernetes.io/name] (L%d)", nl), "app.kubernetes.io/name: "+name))
	}
	if instance != "" {
		ev = append(ev, l.ev(d.file, fmt.Sprintf("$.metadata.labels[app.kubernetes.io/instance] (L%d)", il), "app.kubernetes.io/instance: "+instance))
	}
	if version != "" {
		ev = append(ev, l.ev(d.file, fmt.Sprintf("$.metadata.labels[app.kubernetes.io/version] (L%d)", vl), "app.kubernetes.io/version: "+version))
	}
	l.installed = append(l.installed, InstalledProduct{
		Product: product, Instance: instance, Version: version,
		Source: InstalledLabel, Evidence: ev,
	})
}

// detectArgoApplication reads spec.source (and spec.sources[]) of an Argo CD
// Application: chart/path identity, targetRevision as the installed version,
// and the inline Helm values — which ARE the customer's values, so they join
// the values inventory exactly like a values file.
func (l *loader) detectArgoApplication(d doc) {
	appName := scalarOf(d.node, "metadata", "name")
	var sources []*yaml.Node
	var helmPaths []string
	if s := fieldOf(d.node, "spec", "source"); mappingOrSequence(s) != nil {
		sources = append(sources, s)
		helmPaths = append(helmPaths, "spec.source.helm")
	}
	for i, s := range itemsOf(fieldOf(d.node, "spec", "sources")) {
		sources = append(sources, s)
		helmPaths = append(helmPaths, fmt.Sprintf("spec.sources[%d].helm", i))
	}
	for i, s := range sources {
		chart := scalarOf(s, "chart")
		path := scalarOf(s, "path")
		revision, rvLine := scalarLine(s, "targetRevision")
		if chart == "" && path == "" && revision == "" {
			continue
		}
		product := appName
		if product == "" {
			product = chart
		}
		if product == "" {
			product = path
		}
		var ev []domain.EvidenceID
		if revision != "" {
			ev = append(ev, l.ev(d.file, fmt.Sprintf("$.%s (L%d)", strings.TrimSuffix(helmPaths[i], ".helm")+".targetRevision", rvLine), "targetRevision: "+revision))
		}
		chartIdentity := chart
		if chartIdentity == "" {
			chartIdentity = path
		}
		l.installed = append(l.installed, InstalledProduct{
			Product: product, Version: revision, Chart: chartIdentity,
			Source: InstalledArgoCD, Evidence: ev,
		})
		if helm := fieldOf(s, "helm"); helm != nil {
			prefix := "$." + helmPaths[i]
			if v := fieldOf(helm, "values"); v != nil {
				l.inlineValues(d.file, prefix+".values", v)
			}
			if v := fieldOf(helm, "valuesObject"); v != nil {
				l.inlineValues(d.file, prefix+".valuesObject", v)
			}
			l.parametersToValues(d.file, prefix+".parameters", fieldOf(helm, "parameters"))
		}
	}
}

// detectFluxHelmRelease reads a Flux HelmRelease: chart identity, the
// (possibly ranged) version pin and spec.values — the customer's values.
func (l *loader) detectFluxHelmRelease(d doc) {
	name := scalarOf(d.node, "metadata", "name")
	chart, chLine := scalarLine(fieldOf(d.node, "spec", "chart", "spec"), "chart")
	version, vLine := scalarLine(fieldOf(d.node, "spec", "chart", "spec"), "version")
	if chart == "" && name == "" {
		return
	}
	product := name
	if product == "" {
		product = chart
	}
	var ev []domain.EvidenceID
	if chart != "" {
		ev = append(ev, l.ev(d.file, fmt.Sprintf("$.spec.chart.spec.chart (L%d)", chLine), "chart: "+chart))
	}
	if version != "" {
		ev = append(ev, l.ev(d.file, fmt.Sprintf("$.spec.chart.spec.version (L%d)", vLine), "version: "+version))
	}
	l.installed = append(l.installed, InstalledProduct{
		Product: product, Version: version, Chart: chart,
		Source: InstalledFlux, Evidence: ev,
	})
	if values := fieldOf(d.node, "spec", "values"); values != nil {
		l.inlineValues(d.file, "$.spec.values", values)
	}
	if vf := fieldOf(d.node, "spec", "valuesFrom"); mappingOrSequence(vf) != nil {
		l.warnf("HelmRelease %s uses spec.valuesFrom, which references cluster objects; those values are not readable from files", name)
	}
}

// inlineValues flattens an inline values mapping (or a values string holding
// YAML) into the values inventory, with the same path syntax and value
// encoding as parsed values files. Lines come from the original nodes where
// possible and from the inline node itself otherwise.
func (l *loader) inlineValues(file, locatorPrefix string, node *yaml.Node) {
	node = resolveAlias(node)
	if node == nil {
		return
	}
	add := func(flat []normalize.FlattenedValue, lines map[string]int) {
		for _, kv := range flat {
			line := node.Line
			if ln, ok := lines[kv.Path]; ok && ln > 0 {
				line = ln
			}
			l.addValuesKey(ValuesKey{
				Path: kv.Path, Value: kv.Value, Line: line,
				Evidence: []domain.EvidenceID{l.ev(file, fmt.Sprintf("%s.%s (L%d)", locatorPrefix, kv.Path, line), kv.Path+": "+kv.Value)},
			})
		}
	}
	switch {
	case node.Kind == yaml.MappingNode:
		b, err := yaml.Marshal(node)
		if err != nil {
			l.warnf("%s: inline values at L%d were not serializable: %v", file, node.Line, err)
			return
		}
		flat, err := normalize.FlattenValues(b)
		if err != nil {
			l.warnf("%s: inline values at L%d were not parsed as a values mapping: %v", file, node.Line, err)
			return
		}
		lines := map[string]int{}
		walkFieldPaths(node, "", func(p string, v *yaml.Node) {
			if v != nil {
				lines[p] = v.Line
			}
		})
		add(flat, lines)
	case node.Kind == yaml.ScalarNode && node.Tag != "!!null":
		flat, err := normalize.FlattenValues([]byte(node.Value))
		if err != nil {
			l.warnf("%s: inline values at L%d were not parsed as a values mapping: %v", file, node.Line, err)
			return
		}
		add(flat, nil)
	}
}

// parametersToValues converts an Argo helm.parameters list
// ([- name: x, value: y]) into values keys.
func (l *loader) parametersToValues(file, locatorPrefix string, parameters *yaml.Node) {
	for _, item := range itemsOf(parameters) {
		name := scalarOf(item, "name")
		if name == "" {
			continue
		}
		// Re-marshal the item so the value gets the exact encoding of a
		// parsed values leaf, then re-map its "value" entry under the
		// parameter name.
		b, err := yaml.Marshal(item)
		if err != nil {
			continue
		}
		flat, err := normalize.FlattenValues(b)
		if err != nil {
			continue
		}
		for _, kv := range flat {
			switch {
			case kv.Path == "value":
				l.addValuesKey(ValuesKey{
					Path: name, Value: kv.Value, Line: item.Line,
					Evidence: []domain.EvidenceID{l.ev(file, fmt.Sprintf("%s[%s] (L%d)", locatorPrefix, name, item.Line), name+": "+kv.Value)},
				})
			case strings.HasPrefix(kv.Path, "value."):
				l.addValuesKey(ValuesKey{
					Path: name + "." + strings.TrimPrefix(kv.Path, "value."), Value: kv.Value, Line: item.Line,
					Evidence: []domain.EvidenceID{l.ev(file, fmt.Sprintf("%s[%s] (L%d)", locatorPrefix, name, item.Line), name+"."+kv.Path+": "+kv.Value)},
				})
			}
		}
	}
}

// mappingOrSequence returns the node when it is a mapping with content or a
// sequence with content, else nil.
func mappingOrSequence(n *yaml.Node) *yaml.Node {
	n = resolveAlias(n)
	if n == nil {
		return nil
	}
	if (n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode) && len(n.Content) > 0 {
		return n
	}
	return nil
}

// finalizeInstalled coalesces identical detections (many resources of the
// same chart carry the same annotations/labels) and sorts deterministically.
func (l *loader) finalizeInstalled() {
	type key struct{ product, instance, version, chart, source string }
	merged := map[key]*InstalledProduct{}
	var order []key
	for _, p := range l.installed {
		k := key{p.Product, p.Instance, p.Version, p.Chart, p.Source}
		if m, ok := merged[k]; ok {
			for _, id := range p.Evidence {
				if len(m.Evidence) >= maxInstalledEvidence {
					break
				}
				m.Evidence = appendUniqueID(m.Evidence, id)
			}
			continue
		}
		cp := p
		if len(cp.Evidence) > maxInstalledEvidence {
			cp.Evidence = cp.Evidence[:maxInstalledEvidence]
		}
		merged[k] = &cp
		order = append(order, k)
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := merged[order[i]], merged[order[j]]
		if a.Product != b.Product {
			return a.Product < b.Product
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Chart != b.Chart {
			return a.Chart < b.Chart
		}
		return a.Version < b.Version
	})
	for _, k := range order {
		l.env.Installed = append(l.env.Installed, *merged[k])
	}
}

func appendUniqueID(xs []domain.EvidenceID, y domain.EvidenceID) []domain.EvidenceID {
	for _, x := range xs {
		if x == y {
			return xs
		}
	}
	return append(xs, y)
}
