// GVK usage inventory: per group/version/kind, what the environment's
// manifests (and installed CRDs) actually touch. This is the contract the
// GVK-scoped CRD matcher consumes (a later workstream): the split identity,
// who uses it (Names), what fields they set (FieldPaths), where to
// re-inspect the raw document (Documents) and the evidence chain behind
// every claim.

package env

import (
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Caps bounding the per-GVK inventory; hitting one is a warning, never a
// silent truncation.
const (
	maxGVKNames      = 100 // distinct metadata names cited per GVK
	maxGVKFieldPaths = 500 // distinct field paths per GVK
	maxGVKDocs       = 25  // document refs per GVK
)

// ResourceName is the identity of one resource instance a manifest document
// declares (metadata.name / metadata.namespace; either may be empty).
type ResourceName struct {
	Name      string
	Namespace string
}

// DocumentRef points at the raw manifest document so a matcher can
// re-inspect exactly what the using document said.
type DocumentRef struct {
	File      string // path exactly as supplied to Load
	StartLine int    // 1-based line of the document's first content
}

// GVKUsage is one distinct group/version/kind the environment touches.
type GVKUsage struct {
	Group      string              // "cert-manager.io"; "" for the core group (apiVersion "v1")
	Version    string              // "v1"
	Kind       string              // "Certificate"
	Names      []ResourceName      // identities of the using documents (manifests; the CRD name for CRD-served versions)
	FieldPaths []string            // flattened field paths set by documents of this GVK (values-path syntax, sorted, deduplicated)
	Documents  []DocumentRef       // raw document refs, sorted by (file, line)
	Evidence   []domain.EvidenceID // one per using document, capped like APIVersionUse
}

// splitGroupVersion splits a manifest apiVersion into group and version
// ("cert-manager.io/v1" → "cert-manager.io", "v1"; core "v1" → "", "v1";
// the group itself never contains a slash).
func splitGroupVersion(apiVersion string) (group, version string) {
	if i := strings.IndexByte(apiVersion, '/'); i >= 0 {
		return apiVersion[:i], apiVersion[i+1:]
	}
	return "", apiVersion
}

// gvkKey stages one GVK while loading.
type gvkKey struct{ group, version, kind string }

type gvkStage struct {
	names   map[ResourceName]bool
	paths   map[string]bool
	docs    []DocumentRef
	seenDoc map[DocumentRef]bool
	ev      []domain.EvidenceID
}

func (l *loader) gvkStageFor(group, version, kind string) *gvkStage {
	k := gvkKey{group, version, kind}
	st := l.gvk[k]
	if st == nil {
		st = &gvkStage{names: map[ResourceName]bool{}, paths: map[string]bool{}, seenDoc: map[DocumentRef]bool{}}
		l.gvk[k] = st
	}
	return st
}

func (l *loader) gvkAddDoc(st *gvkStage, d doc) {
	ref := DocumentRef{File: d.file, StartLine: d.startLine}
	if st.seenDoc[ref] {
		return
	}
	st.seenDoc[ref] = true
	if len(st.docs) < maxGVKDocs {
		st.docs = append(st.docs, ref)
		return
	}
	l.warnf("more than %d documents per group/version/kind; the rest were not inventoried", maxGVKDocs)
}

func (l *loader) gvkAddName(st *gvkStage, name, namespace string) {
	if name == "" && namespace == "" {
		return
	}
	n := ResourceName{Name: name, Namespace: namespace}
	if st.names[n] {
		return
	}
	if len(st.names) < maxGVKNames {
		st.names[n] = true
		return
	}
	l.warnf("more than %d resource names per group/version/kind; the rest were not inventoried", maxGVKNames)
}

func (l *loader) gvkAddPath(st *gvkStage, path string) {
	if st.paths[path] {
		return
	}
	if len(st.paths) >= maxGVKFieldPaths {
		l.warnf("more than %d field paths per group/version/kind; the rest were not inventoried", maxGVKFieldPaths)
		return
	}
	st.paths[path] = true
}

func (l *loader) gvkAddEvidence(st *gvkStage, id domain.EvidenceID) {
	if len(st.ev) < maxEvidencePerFact {
		st.ev = append(st.ev, id)
	}
}

// finalizeGVK converts the staging state into the sorted, deterministic
// Environment.GVKUsage inventory.
func (l *loader) finalizeGVK() {
	keys := make([]gvkKey, 0, len(l.gvk))
	for k := range l.gvk {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.group != b.group {
			return a.group < b.group
		}
		if a.version != b.version {
			return a.version < b.version
		}
		return a.kind < b.kind
	})
	for _, k := range keys {
		st := l.gvk[k]
		u := GVKUsage{Group: k.group, Version: k.version, Kind: k.kind, Evidence: st.ev}
		for n := range st.names {
			u.Names = append(u.Names, n)
		}
		sort.Slice(u.Names, func(i, j int) bool {
			if u.Names[i].Namespace != u.Names[j].Namespace {
				return u.Names[i].Namespace < u.Names[j].Namespace
			}
			return u.Names[i].Name < u.Names[j].Name
		})
		for p := range st.paths {
			u.FieldPaths = append(u.FieldPaths, p)
		}
		sort.Strings(u.FieldPaths)
		u.Documents = append(u.Documents, st.docs...)
		sort.Slice(u.Documents, func(i, j int) bool {
			if u.Documents[i].File != u.Documents[j].File {
				return u.Documents[i].File < u.Documents[j].File
			}
			return u.Documents[i].StartLine < u.Documents[j].StartLine
		})
		l.env.GVKUsage = append(l.env.GVKUsage, u)
	}
}
