// Package env models a Kubernetes environment from LOCAL inputs only (no
// cluster access): a cluster version, Helm values files, manifests,
// installed CustomResourceDefinitions and container image references.
//
// Everything is parsed deterministically and every extracted fact carries
// domain.Evidence pointing at the file (and line / YAML path) it came from,
// so the impact join can cite the environment chain exactly as the upgrade
// edge cites upstream sources.
//
// CONTRACT NOTE: Load is the API used by the orchestration layer (app).
package env

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/normalize"
)

// Producer identifies the parser in warnings.
const Producer = "env@v1"

// The input dimensions of the environment model, as named by
// Environment.Health and Environment.Statuses.
const (
	DimKubernetes = "kubernetes"
	DimValues     = "values"
	DimManifests  = "manifests"
	DimCRDs       = "crds"
	DimImages     = "images"
	// DimProducts (the product inventory) is declared in inventory.go.
)

// dimensionOrder is the fixed reporting order of the dimensions.
var dimensionOrder = []string{DimKubernetes, DimValues, DimManifests, DimCRDs, DimImages}

// Health states what one input dimension of the environment can support.
// Absence is NOT knowledge: a dimension the caller did not supply must never
// be read by the join as "the environment is not affected" — there is
// nothing to be not-affected about. A partial dimension (parse failures,
// documents dropped at a cap, a stream that stopped midway) supports only
// weakened conclusions.
type Health string

const (
	// HealthAbsent: no input was supplied for the dimension.
	HealthAbsent Health = "absent"
	// HealthOK: supplied, and every file parsed completely.
	HealthOK Health = "ok"
	// HealthPartial: supplied, but parsing was incomplete. The dimension's
	// warnings (in Environment.Warnings, or DimensionStatus.Warnings) say
	// exactly what is missing.
	HealthPartial Health = "partial"
)

// DimensionStatus is the state of one input dimension: what was supplied,
// how trustworthy the extracted facts are, and which warnings belong to it.
type DimensionStatus struct {
	Dimension string   `json:"dimension"`
	Health    Health   `json:"health"`
	Warnings  []string `json:"warnings,omitempty"`
}

// Caps that bound extraction on hostile inputs; hitting one is a warning,
// never a silent truncation.
const (
	maxValuesKeys      = 10000 // flattened keys across all values files
	maxManifestDocs    = 2000  // documents across all manifests/CRDs inputs
	maxManifestPaths   = 8000  // flattened manifest field paths
	maxEvidencePerFact = 5     // occurrences cited per apiVersion/image use
)

// Inputs are the environment sources to load. Every field is optional; the
// caller decides how much of the join applies.
type Inputs struct {
	// KubernetesVersion is the cluster version ("1.31" or "1.31.5").
	KubernetesVersion string
	// ValuesFiles are Helm values files, in the order -f would apply them.
	ValuesFiles []string
	// Manifests are files or directories of Kubernetes manifests
	// (multi-document YAML streams).
	Manifests []string
	// CRDs are files or directories of installed CustomResourceDefinitions.
	// CRD documents inside Manifests are picked up as well.
	CRDs []string
	// RenderedCRDs are CRD documents extracted from the FROM render of the
	// customer's install (PO-7a addendum item 6): helm template with their
	// values — honouring CRD gates such as crds.enabled / installCRDs exactly
	// as they set them — or kustomize build of their overlay. Rendered CRDs
	// are render evidence, never observed state: an observed CRDs input wins
	// over them (Load loads them only without one), they never mark the
	// dimension supplied for absence conclusions, and a render without CRD
	// documents says nothing (a gate or a separate install path), so the
	// dimension stays absent/partial — never "no CRDs installed".
	RenderedCRDs []RenderedCRDSource
	// Images are explicit image references (mirror lists and the like).
	Images []string
	// Repo enables directory mode: the tree below Repo is walked (bounded
	// depth, VCS/vendor directories skipped) and environment inputs are
	// discovered by convention — values files, k8s-shaped manifests, Argo CD
	// Applications, Flux HelmReleases, kustomizations, workflow/terraform
	// image references. Explicit flags COMPOSE with discovery: discovered
	// files are loaded first, explicit entries after them (so an explicit
	// values file wins per-key), and a file named by both is loaded once.
	Repo string
	// Inventory is a declared product inventory file (YAML list of {product,
	// version, note?}). In repo mode <Repo>/inventory.yaml is picked up when
	// Inventory is empty.
	Inventory string
	// ManifestsComplete declares that Manifests (and repo-discovered
	// manifests) are every object the environment runs — workloads and the
	// resources they reference — so an object missing from them is genuinely
	// absent (--manifests-complete). Without it a missing workload or an
	// unresolved reference is "not shown", never "not there".
	ManifestsComplete bool
	// ProductHints let images be recognised as catalog products (see
	// HintsFromCatalog); without them only declared and Helm/Argo/Flux
	// detections populate the inventory.
	ProductHints []ProductHint
}

// Empty reports whether no input was given at all.
func (in Inputs) Empty() bool {
	return in.KubernetesVersion == "" && len(in.ValuesFiles) == 0 && len(in.Manifests) == 0 && len(in.CRDs) == 0 && len(in.Images) == 0 && strings.TrimSpace(in.Repo) == "" && strings.TrimSpace(in.Inventory) == ""
}

// SuppliedInputs records which environment inputs were given on the command
// line, independently of whether they yielded facts. The impact join needs
// the distinction: "checked the values you supplied and found no overlap"
// (not-affected) is a different claim from "no values were supplied, so
// applicability cannot be decided" (unknown).
type SuppliedInputs struct {
	Kubernetes bool
	Values     bool
	Manifests  bool
	CRDs       bool
	Images     bool
}

// File is one parsed input file.
type File struct {
	Path   string
	Digest string
}

// KubernetesVersion is the cluster version with the evidence of where it was
// declared (a direct input, recorded as evidence of kind input).
type KubernetesVersion struct {
	Version  string
	Evidence []domain.EvidenceID
}

// ValuesKey is one key a user's values file sets, flattened with exactly the
// same path syntax as domain.ValuesSnapshot entries (lists are leaves).
type ValuesKey struct {
	Path     string
	Value    string // JSON-encoded leaf
	Line     int    // 1-based line of the key in its file
	Evidence []domain.EvidenceID
}

// APIVersionUse is one distinct apiVersion (+kind) that manifests use.
type APIVersionUse struct {
	GroupVersion string // "cert-manager.io/v1"
	Kind         string
	Evidence     []domain.EvidenceID // one per using document, capped
}

// CRDVersion is one version of an installed CRD.
type CRDVersion struct {
	Name               string
	Served             bool
	Storage            bool
	Deprecated         bool
	DeprecationWarning string
}

// InstalledCRD is a CustomResourceDefinition found in the inputs.
type InstalledCRD struct {
	Name     string // "certificates.cert-manager.io"
	Group    string // "cert-manager.io"
	Kind     string // "Certificate"
	Versions []CRDVersion
	// PreserveUnknownFields is spec.preserveUnknownFields when stated.
	HasPreserveUnknownFields bool
	PreserveUnknownFields    bool
	// Rendered marks a CRD extracted from the render of the customer's
	// install (Inputs.RenderedCRDs): its evidence is render provenance, and
	// it is not observed cluster state (PO-7a addendum item 6).
	Rendered bool
	Evidence []domain.EvidenceID
}

// RenderedCRDSource is one render's worth of CRD documents for
// Inputs.RenderedCRDs: the CustomResourceDefinition objects the FROM render
// of the customer's install produced, with the provenance of that render.
type RenderedCRDSource struct {
	// Label names the render in evidence URIs and warnings, e.g.
	// "chart example@1.0.0" or "kustomize overlay overlays/prod".
	Label string
	// Tool is the renderer ("helm" | "kustomize").
	Tool string
	// ChartDigest and ValuesDigest carry the render's artifact/values
	// provenance ("" when the renderer states none).
	ChartDigest  string
	ValuesDigest string
	// Docs is the rendered CustomResourceDefinition documents (a YAML
	// stream; non-CRD documents are ignored).
	Docs []byte
}

// ManifestField is one flattened field path set by a manifest document
// (same path syntax as values keys), remembering the document's identity so
// removed CRD fields can be matched against what the environment really sets.
type ManifestField struct {
	Path       string // "spec.secretTemplate.labels"
	APIVersion string
	Kind       string
	Line       int
	Evidence   []domain.EvidenceID
}

// RenderedCRDsPresent reports whether any installed CRD came from the
// render of the customer's install (Inputs.RenderedCRDs, PO-7a addendum 6):
// render evidence for CRD questions when no observed CRDs input exists —
// never observed state (a name is never both: an observed input wins).
func (e *Environment) RenderedCRDsPresent() bool {
	if e == nil {
		return false
	}
	for _, c := range e.CRDs {
		if c.Rendered {
			return true
		}
	}
	return false
}

// ImageUse is one container image the environment references.
type ImageUse struct {
	Reference  string // as written, e.g. "quay.io/jetstack/cert-manager-controller:v1.17.0"
	Repository string
	Tag        string
	Digest     string
	Source     string // "manifest" | "values" | "list" | "workflow" | "terraform" | "kustomization"
	Line       int    // 1-based line in the file (0 for list entries)
	Evidence   []domain.EvidenceID
}

// Environment is the parsed environment: a value type plus the local
// evidence pool backing every fact.
type Environment struct {
	Kubernetes *KubernetesVersion

	ValuesKeys       []ValuesKey
	APIVersions      []APIVersionUse
	CRDs             []InstalledCRD
	ManifestFields   []ManifestField
	Images           []ImageUse
	ManifestDocCount int

	// GVKUsage is the per group/version/kind inventory of what manifests (and
	// installed CRDs) use — the contract the GVK-scoped CRD matcher consumes.
	GVKUsage []GVKUsage
	// Installed is the best-effort identification of the installed
	// product/chart(s), each with the mechanism that grounded it.
	Installed []InstalledProduct
	// Products is the product inventory (declared + detected, conflicts kept
	// visible); see ProductInstance and Environment.Product.
	Products []ProductInstance
	// InventoryComplete: the declared inventory file states `complete: true`
	// — every product running here is listed, so a product it does not list
	// is not installed (while the products dimension is healthy).
	// InventoryCompleteEvidence cites the declaration. False without a
	// declaration: absence from an inventory is never proof of absence.
	// CONTRACT-CHANGE(applicability): requested by DESIGN.md §1.3 (envinv follow-up).
	InventoryComplete         bool
	InventoryCompleteEvidence []domain.EvidenceID
	// ManifestsDeclaredComplete: the caller declared the supplied manifests
	// complete (Inputs.ManifestsComplete, with manifests supplied);
	// ManifestsCompleteEvidence cites the declaration. Distinct from parse
	// health: healthy manifests are fully parsed, not necessarily everything.
	ManifestsDeclaredComplete bool
	ManifestsCompleteEvidence []domain.EvidenceID
	// Resources are the per-document resource facts (field values with "[]"
	// sequence paths, embedded text lines, references); see resources.go and
	// the query API (ResourcesOfKind, FieldValues, TextBlocks, ResolveRef).
	Resources []Resource

	// RepoRoot is the repository directory of repo mode ("" otherwise) and
	// Discovered lists every file the walk classified, with its evidence.
	RepoRoot   string
	Discovered []RepoDiscovery

	// Supplied records which inputs were given (see SuppliedInputs).
	Supplied SuppliedInputs

	Files    []File
	Evidence []domain.Evidence
	Warnings []string

	// health and dimWarnings back Health and Statuses; built by Load.
	health      map[string]Health
	dimWarnings map[string][]string
}

// Health reports the state of one input dimension (see Health). Unknown
// dimension names report HealthAbsent. The impact layer consults this before
// drawing any "not affected"-style conclusion from missing facts: absent
// means "nothing was supplied", not "nothing is there"; partial means the
// facts that exist are a subset of what the files state.
func (e *Environment) Health(dimension string) Health {
	if e == nil || e.health == nil {
		return HealthAbsent
	}
	if h, ok := e.health[dimension]; ok {
		return h
	}
	return HealthAbsent
}

// Statuses returns the state of every input dimension in fixed order.
//
// The products dimension (DimProducts) is deliberately not listed: it is read
// through Health/ProductsStatus, so the enrichment prompts that print these
// statuses (and their committed answer caches) stay byte-identical.
func (e *Environment) Statuses() []DimensionStatus {
	out := make([]DimensionStatus, 0, len(dimensionOrder))
	for _, d := range dimensionOrder {
		s := DimensionStatus{Dimension: d, Health: e.Health(d)}
		if e != nil && e.dimWarnings != nil {
			s.Warnings = e.dimWarnings[d]
		}
		out = append(out, s)
	}
	return out
}

// loader accumulates state while Load runs.
type loader struct {
	env         *Environment
	evidence    map[domain.EvidenceID]bool
	files       map[string]string // path → digest
	warned      map[string]bool
	dimWarnings map[string][]string
	capped      map[string]int
	gvk         map[gvkKey]*gvkStage
	installed   []InstalledProduct
	repoFiles   int
	// productsSupplied: an inventory file or a detected product backs the
	// products dimension.
	productsSupplied bool
	// resFacts / resLines count the resource-fact store against its caps.
	resFacts, resLines int
	// renderedCRDs counts CRD documents loaded from Inputs.RenderedCRDs
	// (PO-7a addendum 6): they reach the loader, so the dimension is not
	// absent — but they are render output, so it is partial, never supplied.
	renderedCRDs int
}

// warnf records a warning on the Environment and attributes it to the input
// dimension it degrades. Identical warnings are recorded once.
func (l *loader) warnf(dim, format string, args ...any) {
	w := fmt.Sprintf(format, args...)
	if !l.warned[w] {
		l.warned[w] = true
		l.env.Warnings = append(l.env.Warnings, w)
	}
	for _, x := range l.dimWarnings[dim] {
		if x == w {
			return
		}
	}
	l.dimWarnings[dim] = append(l.dimWarnings[dim], w)
}

// ev records one piece of local-file evidence and returns its id.
func (l *loader) ev(uri, locator, excerpt string) domain.EvidenceID {
	return l.record(domain.NewEvidence(domain.EvidenceLocalFile, "", uri, locator, domain.TruncateExcerpt(excerpt), l.files[uri], zeroTime))
}

// evInput records evidence of a directly supplied value (a flag).
func (l *loader) evInput(uri, value string) domain.EvidenceID {
	return l.record(domain.NewEvidence(domain.EvidenceInput, "", uri, "", value, domain.Digest([]byte(uri+":"+value)), zeroTime))
}

func (l *loader) record(e domain.Evidence) domain.EvidenceID {
	if !l.evidence[e.ID] {
		l.evidence[e.ID] = true
		l.env.Evidence = append(l.env.Evidence, e)
	}
	return e.ID
}

// Load parses the inputs. It returns an error only when nothing can be done
// at all (an input path does not exist / cannot be read); per-document
// problems are warnings. Every dimension the inputs name gets a health
// status on the result: supplied-and-healthy, supplied-with-warnings
// (partial) or absent.
func Load(in Inputs) (*Environment, error) {
	l := &loader{
		env: &Environment{}, evidence: map[domain.EvidenceID]bool{},
		files: map[string]string{}, warned: map[string]bool{}, dimWarnings: map[string][]string{},
		gvk: map[gvkKey]*gvkStage{},
	}
	if v := strings.TrimSpace(in.KubernetesVersion); v != "" {
		id := l.evInput("flag:--kubernetes", v)
		l.env.Kubernetes = &KubernetesVersion{Version: v, Evidence: []domain.EvidenceID{id}}
		l.env.Supplied.Kubernetes = true
	}
	var discVals, discMans []string
	if strings.TrimSpace(in.Repo) != "" {
		vals, mans, err := l.discoverRepo(in.Repo)
		if err != nil {
			return nil, err
		}
		discVals, discMans = vals, mans
		if len(discVals) > 1 {
			l.warnf(DimValues, "repo mode found %d values files; all are applied, the last one wins per key", len(discVals))
		}
		if len(discVals)+len(discMans) == 0 {
			l.warnf("", "repo mode found no values or manifest files in %s", in.Repo)
		}
	}

	for _, input := range in.ValuesFiles {
		l.warnEmptyInput(input, DimValues)
	}
	for _, input := range in.CRDs {
		l.warnEmptyInput(input, DimCRDs)
	}
	for _, input := range in.Manifests {
		l.warnEmptyInput(input, DimManifests)
	}
	vals, err := mergeFiles(discVals, in.ValuesFiles)
	if err != nil {
		return nil, err
	}
	if err := l.loadValuesFiles(vals); err != nil {
		return nil, err
	}
	crds, err := mergeFiles(nil, in.CRDs)
	if err != nil {
		return nil, err
	}
	if err := l.loadFiles(crds, true); err != nil {
		return nil, err
	}
	mans, err := mergeFiles(discMans, in.Manifests)
	if err != nil {
		return nil, err
	}
	if err := l.loadFiles(mans, false); err != nil {
		return nil, err
	}
	// PO-7a addendum 6: without an observed --crds input, the CRDs the FROM
	// render of the customer's install produced populate the dimension as
	// render evidence (partial; absence in a render is a gate or a separate
	// install path, never knowledge that nothing is installed). Loaded after
	// every observed input — a CRD name an observed file or manifest already
	// states keeps its observed provenance.
	if len(crds) == 0 {
		l.loadRenderedCRDs(in.RenderedCRDs)
	}
	for _, ref := range in.Images {
		l.addImageAt(ref, "list", "", 0)
	}

	sort.Slice(l.env.APIVersions, func(i, j int) bool {
		a, b := l.env.APIVersions[i], l.env.APIVersions[j]
		if a.GroupVersion != b.GroupVersion {
			return a.GroupVersion < b.GroupVersion
		}
		return a.Kind < b.Kind
	})
	sort.Slice(l.env.CRDs, func(i, j int) bool { return l.env.CRDs[i].Name < l.env.CRDs[j].Name })
	sort.Slice(l.env.Images, func(i, j int) bool {
		if l.env.Images[i].Repository != l.env.Images[j].Repository {
			return l.env.Images[i].Repository < l.env.Images[j].Repository
		}
		return l.env.Images[i].Reference < l.env.Images[j].Reference
	})
	l.finalizeGVK()
	l.finalizeInstalled()
	invFile := in.Inventory
	if strings.TrimSpace(invFile) == "" {
		invFile = repoInventory(in.Repo)
	}
	if err := l.loadProducts(invFile, in.ProductHints); err != nil {
		return nil, err
	}

	// Supplied reflects anything that reached the loader: explicit inputs or
	// repo-discovered files (the impact layer reads it for visibility rules).
	l.env.Supplied.Values = len(in.ValuesFiles) > 0 || len(discVals) > 0
	l.env.Supplied.Manifests = len(in.Manifests) > 0 || len(discMans) > 0
	l.env.Supplied.CRDs = len(in.CRDs) > 0
	l.env.Supplied.Images = len(in.Images) > 0 || len(l.env.Images) > 0
	if in.ManifestsComplete && l.env.Supplied.Manifests {
		l.env.ManifestsDeclaredComplete = true
		l.env.ManifestsCompleteEvidence = []domain.EvidenceID{l.evInput("flag:--manifests-complete", "the supplied manifests are every object this environment runs")}
	}

	// A dimension is "supplied" when anything reached the loader for it —
	// explicit inputs or repo-discovered files (vals/mans already merged).
	supplied := map[string]bool{
		DimKubernetes: strings.TrimSpace(in.KubernetesVersion) != "",
		DimValues:     len(vals) > 0 || len(in.ValuesFiles) > 0,
		DimManifests:  len(mans) > 0 || len(in.Manifests) > 0,
		DimCRDs:       len(crds) > 0 || len(in.CRDs) > 0 || l.renderedCRDs > 0,
		DimImages:     len(in.Images) > 0 || len(l.env.Images) > 0,
	}
	l.env.health = map[string]Health{}
	l.env.dimWarnings = l.dimWarnings
	supplied[DimProducts] = l.productsSupplied
	for _, d := range append(append([]string{}, dimensionOrder...), DimProducts) {
		switch {
		case !supplied[d]:
			l.env.health[d] = HealthAbsent
		case len(l.dimWarnings[d]) > 0:
			l.env.health[d] = HealthPartial
		default:
			l.env.health[d] = HealthOK
		}
	}
	return l.env, nil
}

// warnEmptyInput flags an explicitly supplied input that expanded to no YAML
// files: the caller told us to look there, so the dimension is supplied but
// degraded (partial), never silently absent.
func (l *loader) warnEmptyInput(input, dim string) {
	files, err := expand(input)
	if err != nil || len(files) > 0 {
		return
	}
	l.warnf(dim, "%s contains no .yaml/.yml/.json files", input)
}

// mergeFiles combines discovered files with explicit entries. Explicit
// entries (files or directories) are expanded and appended AFTER the
// discovered ones — in an ordered application (values files) an explicit
// file therefore wins per-key over a discovered one. A file reached twice is
// loaded once, at its last position.
func mergeFiles(discovered, explicit []string) ([]string, error) {
	out := append([]string{}, discovered...)
	for _, e := range explicit {
		files, err := expand(e)
		if err != nil {
			return nil, err
		}
		out = append(out, files...)
	}
	last := map[string]int{}
	for i, f := range out {
		last[f] = i
	}
	final := make([]string, 0, len(last))
	for i, f := range out {
		if last[f] == i {
			final = append(final, f)
		}
	}
	return final, nil
}

// zeroTime keeps environment evidence timeless: it describes files, not a
// retrieval. (Evidence ids never included the time anyway.)
var zeroTime time.Time

// readFile records the file digest and returns its bytes.
func (l *loader) readFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if _, seen := l.files[path]; !seen {
		d := domain.Digest(b)
		l.files[path] = d
		l.env.Files = append(l.env.Files, File{Path: path, Digest: d})
	}
	return b, nil
}

// expand turns an input entry into the list of files it names: a directory
// becomes its .yaml/.yml/.json files (sorted), a file names itself.
func expand(path string) ([]string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("env: %w", err)
	}
	if !fi.IsDir() {
		return []string{path}, nil
	}
	var out []string
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".yaml", ".yml", ".json":
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("env: walk %s: %w", path, err)
	}
	sort.Strings(out)
	return out, nil
}

// loadValuesFiles parses Helm values files (already expanded and
// deduplicated, discovered ones first and explicit ones last).
func (l *loader) loadValuesFiles(files []string) error {
	for _, f := range files {
		b, err := l.readFile(f)
		if err != nil {
			return fmt.Errorf("env: %w", err)
		}
		if len(bytes.TrimSpace(b)) == 0 {
			continue
		}
		flat, err := normalize.FlattenValues(b)
		if err != nil {
			l.warnf(DimValues, "%s was not parsed as a Helm values mapping: %v", f, err)
			continue
		}
		// yaml.Unmarshal decodes only a stream's FIRST document and silently
		// drops the rest; a Helm values file must be a single document, so
		// make any extra ones an explicit, dimension-degrading warning.
		if n := countDocs(b); n > 1 {
			l.warnf(DimValues, "%s holds %d YAML documents; a Helm values file is one document — the rest was ignored", f, n)
		}
		for _, kv := range flat {
			if !l.addValuesKey(ValuesKey{
				Path: kv.Path, Value: kv.Value, Line: kv.Line,
				Evidence: []domain.EvidenceID{l.ev(f, fmt.Sprintf("$.%s (L%d)", kv.Path, kv.Line), kv.Path+": "+kv.Value)},
			}) {
				l.warnf(DimValues, "more than %d values keys; the rest of %s was ignored", maxValuesKeys, f)
				break
			}
		}
		// images set through the usual chart conventions
		byPath := make(map[string]normalize.FlattenedValue, len(flat))
		for _, kv := range flat {
			byPath[kv.Path] = kv
		}
		for _, kv := range flat {
			base := lastSegment(kv.Path)
			if base != "repository" && base != "hub" && base != "image" {
				continue
			}
			ref := strings.Trim(strings.Trim(kv.Value, `"`), `'`)
			if !strings.Contains(ref, "/") {
				continue // a bare name or a non-reference value
			}
			tag := ""
			if base != "image" {
				if sib, ok := byPath[siblingPath(kv.Path, "tag")]; ok {
					tag = strings.Trim(strings.Trim(sib.Value, `"`), `'`)
				}
			}
			full := ref
			if tag != "" && base != "image" {
				full = ref + ":" + tag
			}
			if _, err := normalize.ParseImageRef(full); err != nil {
				continue
			}
			l.addImageAt(full, "values", f, kv.Line)
		}
	}
	return nil
}

// addValuesKey appends a values key under the global cap; false when the cap
// is hit.
func (l *loader) addValuesKey(vk ValuesKey) bool {
	if len(l.env.ValuesKeys) >= maxValuesKeys {
		return false
	}
	l.env.ValuesKeys = append(l.env.ValuesKeys, vk)
	return true
}

// doc is one YAML document of a stream.
type doc struct {
	node      *yaml.Node // resolved root node (nil: empty document)
	file      string
	startLine int // 1-based line of the document's first content in the file
}

// loadFiles parses already-expanded manifest/CRD files. Only CRD documents
// are kept from crdOnly inputs; manifest inputs keep everything. Documents
// come from parseDocs (a real stream decoder), so separators inside block
// scalars never split documents; a stream that stops midway keeps its prefix
// and warns about the rest.
func (l *loader) loadFiles(files []string, crdOnly bool) error {
	dim := DimManifests
	if crdOnly {
		dim = DimCRDs
	}
	for _, f := range files {
		b, err := l.readFile(f)
		if err != nil {
			return fmt.Errorf("env: %w", err)
		}
		if len(bytes.TrimSpace(b)) == 0 {
			continue
		}
		docs, perr := parseDocs(b, func(key string, line int) {
			l.warnf(dim, "%s L%d: duplicate key %s; the last value wins", f, line, key)
		})
		switch {
		case perr != nil && len(docs) == 0:
			l.warnf(dim, "%s was not parsed: %v", f, perr)
			continue
		case perr != nil:
			l.warnf(dim, "%s: parsing stopped after %d document(s): %v", f, len(docs), perr)
		}
		for i, d := range docs {
			d.file = f
			if d.node == nil {
				continue
			}
			if l.env.ManifestDocCount >= maxManifestDocs {
				l.warnf(dim, "more than %d manifest documents; the rest was ignored", maxManifestDocs)
				return nil
			}
			l.env.ManifestDocCount++
			kind := scalarOf(d.node, "kind")
			if kind == "CustomResourceDefinition" {
				l.loadCRD(d, nil)
				continue
			}
			if crdOnly {
				l.warnf(dim, "%s doc %d is a %q document, not a CustomResourceDefinition", f, i+1, kind)
				continue
			}
			l.loadManifest(d)
		}
	}
	return nil
}

func (l *loader) loadCRD(d doc, rs *RenderedCRDSource) {
	// The CRD document is itself a resource (apiextensions.k8s.io
	// CustomResourceDefinition): conditions over its own fields
	// (status.storedVersions, spec.versions[].served) read it like any other
	// resource. Without this a "resource CustomResourceDefinition [...]"
	// predicate found no resource of the kind and decided false from never
	// looking.
	if g, v := splitGroupVersion(scalarOf(d.node, "apiVersion")); g == "apiextensions.k8s.io" {
		l.collectResource(d, g, v, "CustomResourceDefinition")
	}
	spec := fieldOf(d.node, "spec")
	if spec == nil {
		l.warnf(DimCRDs, "%s L%d: CustomResourceDefinition without spec", d.file, d.startLine)
		return
	}
	group := scalarOf(spec, "group")
	names := fieldOf(spec, "names")
	kind := ""
	if names != nil {
		kind = scalarOf(names, "kind")
	}
	name := scalarOf(d.node, "metadata", "name")
	if name == "" && group != "" {
		plural := ""
		if names != nil {
			plural = scalarOf(names, "plural")
		}
		if plural != "" {
			name = plural + "." + group
		}
	}
	if name == "" || group == "" {
		l.warnf(DimCRDs, "%s L%d: CustomResourceDefinition without name or group", d.file, d.startLine)
		return
	}
	crd := InstalledCRD{Name: name, Group: group, Kind: kind, Rendered: rs != nil}
	if pv := fieldOf(spec, "preserveUnknownFields"); pv != nil && pv.Tag == "!!bool" {
		crd.HasPreserveUnknownFields = true
		crd.PreserveUnknownFields = pv.Value == "true"
	}
	if versions := fieldOf(spec, "versions"); versions != nil {
		for _, vn := range itemsOf(versions) {
			v := CRDVersion{Name: scalarOf(vn, "name")}
			if s := scalarOf(vn, "served"); s != "" {
				v.Served = s == "true"
			}
			if s := scalarOf(vn, "storage"); s != "" {
				v.Storage = s == "true"
			}
			if s := scalarOf(vn, "deprecated"); s != "" {
				v.Deprecated = s == "true"
			}
			v.DeprecationWarning = scalarOf(vn, "deprecationWarning")
			crd.Versions = append(crd.Versions, v)
		}
	}
	var ev domain.EvidenceID
	if rs != nil { // PO-7a addendum 6: render provenance, distinct from observed
		ev = l.evRendered(rs, name)
	} else {
		ev = l.ev(d.file, fmt.Sprintf("L%d", d.startLine), "CustomResourceDefinition "+name)
	}
	crd.Evidence = []domain.EvidenceID{ev}
	// A CRD document also states the group/version pairs it serves.
	for _, v := range crd.Versions {
		l.useAPIVersion(group+"/"+v.Name, kind, d, 1)
		st := l.gvkStageFor(group, v.Name, kind)
		l.gvkAddDoc(st, d)
		l.gvkAddName(st, name, "")
		l.gvkAddEvidence(st, ev)
	}
	l.env.CRDs = append(l.env.CRDs, crd)
}

// evRendered records the evidence of one rendered CRD: an EvidenceRendered
// record whose URI names the render and whose Render provenance carries the
// chart and values digests — "rendered (chart X@from, values digest …)",
// distinct from every observed input (PO-7a addendum item 6).
func (l *loader) evRendered(rs *RenderedCRDSource, name string) domain.EvidenceID {
	uri := "render:" + rs.Label
	e := domain.NewEvidence(domain.EvidenceRendered, "render@v1", uri, "CustomResourceDefinition "+name,
		fmt.Sprintf("rendered CustomResourceDefinition %s (render of %s with your configuration)", name, rs.Label),
		domain.Digest([]byte(uri+"\x00"+name)), zeroTime)
	rp := domain.RenderProvenance{Scope: domain.RenderEnvironment, Tool: rs.Tool,
		ChartDigest: rs.ChartDigest, ValuesDigest: rs.ValuesDigest, Change: "crd-rendered"}
	e.Render = &rp
	return l.record(e)
}

// loadRenderedCRDs loads Inputs.RenderedCRDs (PO-7a addendum item 6): the
// CustomResourceDefinition documents the FROM render of the customer's
// install produced, with their gates exactly as set. Rendered CRDs never
// override an observed input — a CRD name already loaded from --crds or a
// manifest is skipped, and Load calls this only without --crds files — and
// they never mark the dimension supplied for absence conclusions; the
// dimension stays partial (render output is not observed state).
func (l *loader) loadRenderedCRDs(srcs []RenderedCRDSource) {
	seen := map[string]bool{}
	for _, c := range l.env.CRDs {
		seen[c.Name] = true
	}
	var labels []string
	for _, src := range srcs {
		if len(bytes.TrimSpace(src.Docs)) == 0 {
			continue
		}
		docs, perr := parseDocs(src.Docs, nil)
		switch {
		case perr != nil && len(docs) == 0:
			l.warnf(DimCRDs, "rendered CRDs of %s were not parsed: %v", src.Label, perr)
			continue
		case perr != nil:
			l.warnf(DimCRDs, "rendered CRDs of %s: parsing stopped after %d document(s): %v", src.Label, len(docs), perr)
		}
		for _, d := range docs {
			if d.node == nil || scalarOf(d.node, "kind") != "CustomResourceDefinition" {
				continue // the extractor keeps CRD documents; anything else is noise
			}
			if n := scalarOf(d.node, "metadata", "name"); n != "" && seen[n] {
				continue // an observed input already states this CRD
			}
			d.file = "render:" + src.Label // warnings name the render, not a file
			n0 := len(l.env.CRDs)
			l.loadCRD(d, &src)
			if len(l.env.CRDs) > n0 {
				seen[l.env.CRDs[n0].Name] = true
				l.renderedCRDs++
				if len(labels) == 0 || labels[len(labels)-1] != src.Label {
					labels = append(labels, src.Label)
				}
			}
		}
	}
	if l.renderedCRDs > 0 {
		l.warnf(DimCRDs, "%d rendered CRD(s) from %s: rendered with your configuration, not observed state — the CRDs dimension stays partial",
			l.renderedCRDs, strings.Join(labels, ", "))
	}
}

func (l *loader) loadManifest(d doc) {
	apiVersion := scalarOf(d.node, "apiVersion")
	kind := scalarOf(d.node, "kind")
	var st *gvkStage
	if apiVersion != "" && kind != "" {
		l.useAPIVersion(apiVersion, kind, d, maxEvidencePerFact)
		group, version := splitGroupVersion(apiVersion)
		st = l.gvkStageFor(group, version, kind)
		l.gvkAddDoc(st, d)
		l.gvkAddName(st, scalarOf(d.node, "metadata", "name"), scalarOf(d.node, "metadata", "namespace"))
		l.gvkAddEvidence(st, l.ev(d.file, fmt.Sprintf("L%d", d.startLine), "apiVersion: "+apiVersion+" / kind: "+kind))
		l.detectInstalled(d)
		l.collectResource(d, group, version, kind)
	}
	// images: any mapping key "image" with a reference value
	walkMappings(d.node, func(key string, val *yaml.Node, path string) {
		if key != "image" || val.Kind != yaml.ScalarNode {
			return
		}
		ref := val.Value
		if strings.ContainsAny(ref, "{}$") || !strings.Contains(ref, ":") && !strings.Contains(ref, "/") {
			return
		}
		l.addImageAt(ref, "manifest", d.file, val.Line)
	})
	// flattened field paths, for removed-CRD-field matching
	if apiVersion == "" || kind == "" {
		return
	}
	walkFieldPaths(d.node, "", func(path string, val *yaml.Node) {
		if len(l.env.ManifestFields) >= maxManifestPaths {
			l.warnf(DimManifests, "more than %d manifest field paths; the rest of %s was ignored", maxManifestPaths, d.file)
			return
		}
		line := 0
		if val != nil {
			line = val.Line
		}
		if st != nil {
			l.gvkAddPath(st, path)
		}
		l.env.ManifestFields = append(l.env.ManifestFields, ManifestField{
			Path: path, APIVersion: apiVersion, Kind: kind, Line: line,
			Evidence: []domain.EvidenceID{l.ev(d.file, fmt.Sprintf("$.%s (L%d)", path, line), path)},
		})
	})
}

func (l *loader) useAPIVersion(gv, kind string, d doc, cap int) {
	for i := range l.env.APIVersions {
		u := &l.env.APIVersions[i]
		if u.GroupVersion != gv || u.Kind != kind {
			continue
		}
		if len(u.Evidence) < cap {
			u.Evidence = append(u.Evidence, l.ev(d.file, fmt.Sprintf("L%d", d.startLine), "apiVersion: "+gv+" / kind: "+kind))
		}
		return
	}
	l.env.APIVersions = append(l.env.APIVersions, APIVersionUse{
		GroupVersion: gv, Kind: kind,
		Evidence: []domain.EvidenceID{l.ev(d.file, fmt.Sprintf("L%d", d.startLine), "apiVersion: "+gv+" / kind: "+kind)},
	})
}

func (l *loader) addImageAt(ref, source, file string, line int) {
	parsed, err := normalize.ParseImageRef(ref)
	if err != nil {
		if source != "list" {
			return // manifest/values fields are often not references
		}
		l.warnf(DimImages, "%q is not a parsable image reference", ref)
		return
	}
	var evidence []domain.EvidenceID
	if file != "" {
		evidence = []domain.EvidenceID{l.ev(file, fmt.Sprintf("L%d", line), "image: "+ref)}
	} else {
		evidence = []domain.EvidenceID{l.evInput("flag:--images", ref)}
	}
	for i := range l.env.Images {
		u := &l.env.Images[i]
		if u.Reference == ref && u.Source == source {
			if len(u.Evidence) < maxEvidencePerFact {
				u.Evidence = append(u.Evidence, evidence[0])
			}
			return
		}
	}
	l.env.Images = append(l.env.Images, ImageUse{
		Reference: ref, Repository: parsed.Repository, Tag: parsed.Tag, Digest: parsed.Digest,
		Source: source, Line: line, Evidence: evidence,
	})
}
