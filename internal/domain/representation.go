package domain

// Representation records which published form of an artifact a fact was
// extracted from. The same content (chart defaults, CRDs, metadata) usually
// exists in more than one representation — the source tree at a tag and one
// or more artifacts users actually consume — and they are not always equal
// (publishers rewrite values at packaging time). Every fact, snapshot and
// piece of evidence produced from a published artifact carries its
// representation so a consumer can tell which form of the artifact a
// statement is about; see docs/ARTIFACTS.md.
//
// A representation is orthogonal to Provenance.Method: the method states how
// knowledge was derived (declared / computed / heuristic), the representation
// states which bytes it was derived from.
type Representation string

const (
	// RepresentationSourceTree: a file read from the source repository
	// (repo-file / repo-dir at a ref, a git checkout).
	RepresentationSourceTree Representation = "source-tree"
	// RepresentationChartTGZ: a packaged Helm chart tarball downloaded from a
	// chart repository (the .tgz URL of a helm repository index entry).
	RepresentationChartTGZ Representation = "published-chart-tgz"
	// RepresentationOCIChart: a packaged Helm chart pulled as an OCI artifact
	// (the chart content layer of a helm chart manifest).
	RepresentationOCIChart Representation = "published-oci-chart"
	// RepresentationReleaseAsset: a file attached to a release and downloaded
	// from a release-asset URL (install manifests, chart tarballs, archives).
	RepresentationReleaseAsset Representation = "release-asset"
	// RepresentationHTTPDocument: a plain HTTP document that is none of the
	// above (a website file, a bare values.yaml).
	RepresentationHTTPDocument Representation = "http-document"
	// RepresentationRegistryManifest: an OCI manifest or config blob read from
	// a container registry (digests, labels, config).
	RepresentationRegistryManifest Representation = "registry-manifest"
)

// ValidRepresentation reports whether r is one of the known representations.
func ValidRepresentation(r Representation) bool {
	switch r {
	case RepresentationSourceTree, RepresentationChartTGZ, RepresentationOCIChart,
		RepresentationReleaseAsset, RepresentationHTTPDocument, RepresentationRegistryManifest:
		return true
	}
	return false
}
