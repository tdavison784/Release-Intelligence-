// Product inventory: which products, at which versions, run in the
// environment — an explicit, evidence-backed dimension of its own so release
// knowledge can state cross-product conditions ("requires ingress-nginx >=
// 1.12.6") and the deterministic engine can evaluate them.
//
// Entries come from three kinds of source, all kept side by side (a
// disagreement is never resolved here, only made visible):
//
//   - declared: the Kubernetes cluster version (--kubernetes) and an
//     inventory file (--inventory, inventory.yaml in an eval fixture or a repo);
//   - detected from deployment metadata: the existing InstalledProduct
//     detection (Helm annotations, labels, Argo CD, Flux, Helmfile);
//   - detected from images: an image whose repository is a container-image
//     artifact channel of a catalog product (ProductHint), at its tag.
//
// Absence is not knowledge: the products dimension is "absent" unless an
// inventory file was given or something was detected, and a product missing
// from a supplied inventory is "not listed", never "not installed".

package env

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// DimProducts is the product-inventory dimension of Environment.Health.
const DimProducts = "products"

// KubernetesProduct is the inventory id of the cluster itself.
const KubernetesProduct = "kubernetes"

// Sources of inventory entries (ProductInstance.Source).
const (
	ProductDeclared         = "declared"
	ProductDetectedHelm     = "detected-helm"
	ProductDetectedLabel    = "detected-label"
	ProductDetectedArgo     = "detected-argo"
	ProductDetectedFlux     = "detected-flux"
	ProductDetectedHelmfile = "detected-helmfile"
	ProductDetectedImage    = "detected-image"
)

// What a ProductInstance.Version denotes.
const (
	VersionOfApp   = "app"   // the product's own version (declared, label, image tag)
	VersionOfChart = "chart" // the version of the deploying chart/revision, which may differ from the app version
)

// ProductInstance is one inventory entry. Version is the normalized semver
// ("1.12.1") when RawVersion parses, else "" — the raw text is always kept.
type ProductInstance struct {
	// Product is the catalog product id when the entry maps to
	// products/<id>.yaml (Catalog true), else a normalized name.
	Product    string
	Catalog    bool
	Version    string
	RawVersion string
	VersionOf  string // VersionOfApp | VersionOfChart
	Source     string
	Note       string
	// Conflict names the other entries of the same product that state a
	// different version (e.g. "declared 1.12.6"); "" when none do. Both
	// entries stay in the inventory.
	Conflict string
	Evidence []domain.EvidenceID
}

// ProductHint ties a catalog product to the image repositories it publishes,
// so images can be recognised generically (driven by products/*.yaml, never
// by product names in code).
type ProductHint struct {
	ID           string
	Repositories []string
}

// HintsFromCatalog derives hints from every container-image artifact of the
// catalog that publishes through an OCI channel.
func HintsFromCatalog(cat *catalog.Catalog) []ProductHint {
	if cat == nil {
		return nil
	}
	var out []ProductHint
	for _, d := range cat.List() {
		h := ProductHint{ID: d.ID}
		for _, a := range d.Artifacts {
			if a.Type != domain.ArtifactContainerImage {
				continue
			}
			for _, ch := range a.Channels {
				if ch.Kind == "oci" && ch.Repository != "" {
					h.Repositories = append(h.Repositories, ch.Repository)
				}
			}
		}
		out = append(out, h)
	}
	return out
}

// Product returns the inventory entry the engine should read for a product:
// a declared entry wins over a detected one, otherwise the first entry in
// inventory order. Use ProductInstances when conflicts matter.
func (e *Environment) Product(id string) (ProductInstance, bool) {
	all := e.ProductInstances(id)
	if len(all) == 0 {
		return ProductInstance{}, false
	}
	for _, p := range all {
		if p.Source == ProductDeclared {
			return p, true
		}
	}
	return all[0], true
}

// ProductInstances returns every entry of the product (all sources).
func (e *Environment) ProductInstances(id string) []ProductInstance {
	if e == nil {
		return nil
	}
	id = normalizeProductID(id)
	var out []ProductInstance
	for _, p := range e.Products {
		if p.Product == id {
			out = append(out, p)
		}
	}
	return out
}

// ProductRangeCheck is the verdict of ProductInRange.
type ProductRangeCheck struct {
	// Present: at least one entry for the product exists. When false the
	// verdict says nothing about installation unless Inventory is OK
	// (Environment.Health(DimProducts)); the caller decides.
	Present bool
	// Computable: every entry with a parsable version agreed on a verdict.
	// False when no entry has a version, the constraint cannot be evaluated,
	// or entries disagree (Conflict).
	Computable bool
	InRange    bool
	Conflict   bool
	Instance   ProductInstance // the entry Product would return
}

// ProductInRange evaluates "product present with version in range". It reuses
// the repository's range representation (domain.CompatibilityConstraint,
// evaluated by upgrade.EvaluatePlatformConstraint) and, where the constraint
// states patch-level bounds and the entry a full version, compares at full
// semver precision so ">= 1.12.6" does not admit 1.12.1.
func (e *Environment) ProductInRange(id string, c *domain.CompatibilityConstraint) ProductRangeCheck {
	var out ProductRangeCheck
	all := e.ProductInstances(id)
	if len(all) == 0 {
		return out
	}
	out.Present = true
	out.Instance, _ = e.Product(id)
	verdicts := map[bool]bool{}
	for _, p := range all {
		if p.Version == "" {
			continue
		}
		in, ok := versionInRange(c, p.Version)
		if !ok {
			continue
		}
		verdicts[in] = true
		out.InRange = in
	}
	switch len(verdicts) {
	case 1:
		out.Computable = true
	case 2:
		out.Conflict = true
		out.InRange = false
	}
	return out
}

var (
	// bareLineRe finds a "major.minor" that is not followed by a patch segment.
	bareLineRe  = regexp.MustCompile(`(?:^|[^\d.])\d+\.\d+(?:$|[^\d.])`)
	anyVersion  = regexp.MustCompile(`\d`)
	prefixVRe   = regexp.MustCompile(`^[vV]`)
	versionOnly = regexp.MustCompile(`^\d+(\.\d+){0,2}`)
	// lineRe is a release line: one or two numeric segments and nothing else.
	lineRe = regexp.MustCompile(`^\d+(\.\d+)?$`)
)

func versionInRange(c *domain.CompatibilityConstraint, version string) (in, ok bool) {
	if c == nil {
		return false, false
	}
	cs := strings.TrimSpace(c.Constraint)
	// full-precision comparison only for a full version against patch-level bounds
	if cs != "" && !lineRe.MatchString(version) && anyVersion.MatchString(cs) && !bareLineRe.MatchString(cs) {
		if sc, err := semver.NewConstraint(cs); err == nil {
			if v, err := semver.NewVersion(version); err == nil {
				return sc.Check(v), true
			}
		}
	}
	chk := upgrade.EvaluatePlatformConstraint(c, version)
	if !chk.Computable {
		return false, false
	}
	return chk.Admits, true
}

// normalizeProductID lower-cases and trims a product name.
func normalizeProductID(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// normalizeVersion returns the normalized form of raw: full semver without a
// "v" prefix ("v1.12.1" → "1.12.1"), or — for a release line stated without a
// patch ("1.12", "v2") — the line itself, never padded into a patch-level
// claim. It returns "" when raw is not a version.
func normalizeVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	bare := prefixVRe.ReplaceAllString(raw, "")
	if raw == "" || !versionOnly.MatchString(bare) {
		return ""
	}
	if lineRe.MatchString(bare) {
		return bare
	}
	v, err := semver.NewVersion(raw)
	if err != nil {
		return ""
	}
	return v.String()
}

// inventoryBuilder accumulates entries while Load runs.
type inventoryBuilder struct {
	l     *loader
	hints []ProductHint
	byID  map[string]bool
	out   []ProductInstance
	// declaredFile reports an inventory file was supplied (even if empty).
	declaredFile bool
	// complete: the file declares `complete: true` (completeEvidence cites it).
	complete         bool
	completeEvidence domain.EvidenceID
}

func (b *inventoryBuilder) resolve(candidates ...string) (id string, inCatalog bool) {
	for _, c := range candidates {
		c = normalizeProductID(c)
		if c != "" && b.byID[c] {
			return c, true
		}
	}
	for _, c := range candidates {
		if c = normalizeProductID(c); c != "" {
			return c, false
		}
	}
	return "", false
}

func (b *inventoryBuilder) add(p ProductInstance) {
	if p.Product == "" {
		return
	}
	if p.RawVersion != "" && p.Version == "" {
		b.l.warnf(DimProducts, "%s %q: version %q is not a parsable version; kept as stated, it cannot be range-checked", p.Source, p.Product, p.RawVersion)
	}
	b.out = append(b.out, p)
}

// declaredKubernetes adds the --kubernetes cluster version.
func (b *inventoryBuilder) declaredKubernetes() {
	k := b.l.env.Kubernetes
	if k == nil {
		return
	}
	b.add(ProductInstance{
		Product: KubernetesProduct, Version: normalizeVersion(k.Version), RawVersion: k.Version,
		VersionOf: VersionOfApp, Source: ProductDeclared, Note: "from --kubernetes", Evidence: k.Evidence,
	})
}

// declaredFile loads an inventory file: a YAML list of {product, version,
// note?}. Malformed entries are warnings (the dimension turns partial).
func (b *inventoryBuilder) loadInventoryFile(path string) error {
	data, err := b.l.readFile(path)
	if err != nil {
		return fmt.Errorf("env: %w", err)
	}
	b.declaredFile = true
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		b.l.warnf(DimProducts, "%s was not parsed as an inventory: %v", path, err)
		return nil
	}
	if root.Kind == 0 { // empty file
		return nil
	}
	seq := resolveAlias(&root)
	if seq != nil && seq.Kind == yaml.DocumentNode && len(seq.Content) == 1 {
		seq = resolveAlias(seq.Content[0])
	}
	// CONTRACT-CHANGE(applicability): the mapping form {complete: true,
	// products: [...]} declares the inventory complete (DESIGN.md §1.3
	// product-version: "not listed" is false only under a declared-complete,
	// healthy inventory). The list form stays the default: never complete.
	if seq != nil && seq.Kind == yaml.MappingNode {
		complete, cl := scalarLine(seq, "complete")
		switch complete {
		case "true":
			b.complete = true
			b.completeEvidence = b.l.ev(path, fmt.Sprintf("$.complete (L%d)", cl), "complete: true")
		case "", "false":
		default:
			b.l.warnf(DimProducts, "%s: complete must be true or false, got %q; the inventory is not declared complete", path, complete)
		}
		seq = resolveAlias(fieldOf(seq, "products"))
		if seq == nil {
			return nil // a declaration without entries: an empty (possibly complete) inventory
		}
	}
	if seq == nil || seq.Kind != yaml.SequenceNode {
		b.l.warnf(DimProducts, "%s is not a list of {product, version} entries; nothing was read from it", path)
		return nil
	}
	for i, item := range seq.Content {
		name, nl := scalarLine(item, "product")
		raw, vl := scalarLine(item, "version")
		note := scalarOf(item, "note")
		if name == "" {
			b.l.warnf(DimProducts, "%s entry %d has no product; skipped", path, i+1)
			continue
		}
		line := nl
		if line == 0 {
			line = item.Line
		}
		_ = vl
		id, cat := b.resolve(name)
		b.add(ProductInstance{
			Product: id, Catalog: cat, Version: normalizeVersion(raw), RawVersion: raw,
			VersionOf: VersionOfApp, Source: ProductDeclared, Note: note,
			Evidence: []domain.EvidenceID{b.l.ev(path, fmt.Sprintf("$[%d] (L%d)", i, line), "product: "+name+" version: "+raw)},
		})
	}
	return nil
}

// fromInstalled turns the InstalledProduct detections into entries.
func (b *inventoryBuilder) fromInstalled() {
	for _, p := range b.l.env.Installed {
		chartName := chartBase(p.Chart)
		id, cat := b.resolve(chartName, p.Product, p.Instance)
		src, vof := ProductDetectedHelm, VersionOfChart
		switch p.Source {
		case InstalledLabel:
			src, vof = ProductDetectedLabel, VersionOfApp
		case InstalledArgoCD:
			src = ProductDetectedArgo
		case InstalledFlux:
			src = ProductDetectedFlux
		case InstalledHelmfile:
			src = ProductDetectedHelmfile
		}
		b.add(ProductInstance{
			Product: id, Catalog: cat, Version: normalizeVersion(p.Version), RawVersion: p.Version,
			VersionOf: vof, Source: src, Evidence: p.Evidence,
		})
	}
}

// chartBase reduces a chart identity to a bare name: "jetstack/cert-manager"
// → "cert-manager", "cert-manager-v1.17.0" → "cert-manager".
func chartBase(chart string) string {
	if chart == "" {
		return ""
	}
	if i := strings.LastIndexByte(chart, '/'); i >= 0 {
		chart = chart[i+1:]
	}
	if i := strings.LastIndexByte(chart, '-'); i > 0 && chartVersionOf(chart) != "" {
		chart = chart[:i]
	}
	return chart
}

// imagePath drops the registry host: "quay.io/jetstack/x" → "jetstack/x".
func imagePath(repo string) (host, path string) {
	repo = strings.ToLower(repo)
	if i := strings.IndexByte(repo, '/'); i > 0 && (strings.ContainsAny(repo[:i], ".:") || repo[:i] == "localhost") {
		return repo[:i], repo[i+1:]
	}
	return "", repo
}

// fromImages recognises product images through the hints.
func (b *inventoryBuilder) fromImages() {
	if len(b.hints) == 0 {
		return
	}
	type key struct{ product, version, raw, note string }
	merged := map[key]int{}
	for _, img := range b.l.env.Images {
		if img.Tag == "" {
			continue // a digest-only reference states no version
		}
		host, path := imagePath(img.Repository)
		var hit, note string
		for _, h := range b.hints {
			for _, r := range h.Repositories {
				rh, rp := imagePath(r)
				switch {
				case host == rh && path == rp:
					hit = h.ID
				case strings.Count(rp, "/") >= 1 && path == rp: // a mirror keeps the path, changes the host
					hit, note = h.ID, "image repository matched by path; registry host differs from the catalog's"
				}
				if hit != "" {
					break
				}
			}
			if hit != "" {
				break
			}
		}
		if hit == "" {
			continue
		}
		k := key{hit, normalizeVersion(img.Tag), img.Tag, note}
		if i, ok := merged[k]; ok {
			for _, id := range img.Evidence {
				if len(b.out[i].Evidence) < maxInstalledEvidence {
					b.out[i].Evidence = appendUniqueID(b.out[i].Evidence, id)
				}
			}
			continue
		}
		merged[k] = len(b.out)
		ev := img.Evidence
		if len(ev) > maxInstalledEvidence {
			ev = ev[:maxInstalledEvidence]
		}
		b.add(ProductInstance{
			Product: hit, Catalog: true, Version: k.version, RawVersion: img.Tag,
			VersionOf: VersionOfApp, Source: ProductDetectedImage, Note: note, Evidence: ev,
		})
	}
}

// finalize sorts, marks conflicts and stores the inventory on the environment.
func (b *inventoryBuilder) finalize() {
	out := b.out
	// an identical entry stated twice is one entry (evidence merged)
	type key struct{ product, version, raw, source, note string }
	seen := map[key]int{}
	dedup := out[:0:0]
	for _, p := range out {
		k := key{p.Product, p.Version, p.RawVersion, p.Source, p.Note}
		if i, ok := seen[k]; ok {
			for _, id := range p.Evidence {
				if len(dedup[i].Evidence) < maxInstalledEvidence {
					dedup[i].Evidence = appendUniqueID(dedup[i].Evidence, id)
				}
			}
			continue
		}
		seen[k] = len(dedup)
		dedup = append(dedup, p)
	}
	out = dedup
	sort.SliceStable(out, func(i, j int) bool {
		a, c := out[i], out[j]
		if a.Product != c.Product {
			return a.Product < c.Product
		}
		if (a.Source == ProductDeclared) != (c.Source == ProductDeclared) {
			return a.Source == ProductDeclared
		}
		if a.Source != c.Source {
			return a.Source < c.Source
		}
		if a.Version != c.Version {
			return a.Version < c.Version
		}
		return a.RawVersion < c.RawVersion
	})
	// conflicts: same product, same kind of version, different stated versions
	for i := range out {
		var others []string
		for j := range out {
			if i == j || out[i].Product != out[j].Product || out[i].VersionOf != out[j].VersionOf {
				continue
			}
			if out[i].Version == "" || out[j].Version == "" || out[i].Version == out[j].Version {
				continue
			}
			others = append(others, out[j].Source+" "+out[j].Version)
		}
		if len(others) > 0 {
			out[i].Conflict = strings.Join(others, ", ")
		}
	}
	reported := map[string]bool{}
	for _, p := range out {
		if p.Conflict != "" && !reported[p.Product] {
			reported[p.Product] = true
			b.l.warnf(DimProducts, "conflicting versions for %s: %s %s vs %s; every entry is kept", p.Product, p.Source, p.Version, p.Conflict)
		}
	}
	b.l.env.Products = out
}

// loadProducts builds the inventory after every other dimension is loaded.
// invFile is the declared inventory ("" when none).
func (l *loader) loadProducts(invFile string, hints []ProductHint) error {
	b := &inventoryBuilder{l: l, hints: hints, byID: map[string]bool{}}
	for _, h := range hints {
		b.byID[normalizeProductID(h.ID)] = true
	}
	b.declaredKubernetes()
	if invFile != "" {
		if err := b.loadInventoryFile(invFile); err != nil {
			return err
		}
	}
	b.fromInstalled()
	b.fromImages()
	b.finalize()
	if b.complete {
		l.env.InventoryComplete = true
		l.env.InventoryCompleteEvidence = []domain.EvidenceID{b.completeEvidence}
	}
	l.productsSupplied = b.declaredFile
	for _, p := range l.env.Products {
		if p.Product != KubernetesProduct || p.Source != ProductDeclared {
			l.productsSupplied = true
		}
	}
	return nil
}

// repoInventory returns <repo>/inventory.yaml when present (repo mode).
func repoInventory(repo string) string {
	if strings.TrimSpace(repo) == "" {
		return ""
	}
	p := strings.TrimRight(repo, "/") + "/inventory.yaml"
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p
	}
	return ""
}

// ProductsStatus is the state of the products dimension with its warnings
// (unparsable versions, conflicts).
func (e *Environment) ProductsStatus() DimensionStatus {
	s := DimensionStatus{Dimension: DimProducts, Health: e.Health(DimProducts)}
	if e != nil && e.dimWarnings != nil {
		s.Warnings = e.dimWarnings[DimProducts]
	}
	return s
}
