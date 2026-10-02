package sources

import (
	"context"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// PackageFile is one member of a published package.
type PackageFile struct {
	// Path inside the package, relative to the chart directory
	// ("crds/acme.yaml", "templates/deployment.yaml").
	Path    string
	Content []byte
}

// ChartPackage is a published Helm chart package (the gzipped tarball users
// install) together with the provenance of where it was retrieved from.
// Reading chart contents from the package — instead of from the source tree —
// records facts about the representation users actually consume; the
// representation field keeps the two apart (see docs/ARTIFACTS.md).
type ChartPackage struct {
	// Representation is how this package reached us (published-chart-tgz,
	// published-oci-chart or release-asset).
	Representation domain.Representation
	// URI is where the package came from: the tarball URL (helm repository
	// index entry, release asset) or the OCI coordinate of the manifest.
	URI string
	// Coordinate is the user-facing coordinate, e.g.
	// "https://charts.example/acme-1.2.3.tgz" or "ghcr.io/acme/charts/acme:1.2.3".
	Coordinate string
	// Digest is the sha256 of the archive bytes: the helm index digest for a
	// chart repository package, the layer digest for an OCI chart, the body
	// digest for a plain download. Verified against the index/manifest when
	// one was consulted.
	Digest string
	// RetrievedAt is when the package bytes were retrieved.
	RetrievedAt time.Time
	// Evidence records the retrieval: the archive (or, for OCI charts, the
	// manifest plus the layer) with its digest.
	Evidence []domain.Evidence
	// ChartDir is the top-level directory of the archive ("acme" of
	// "acme/Chart.yaml"); members' paths are relative to it.
	ChartDir string
	// ChartYAML is the Chart.yaml member (never nil when ReadChartPackage
	// succeeds).
	ChartYAML []byte
	// Values is the values.yaml member; nil when the package has none.
	Values []byte
	// CRDs are the crds/** members, sorted by path (possibly empty).
	CRDs []PackageFile
	// Templates are the templates/** members, sorted by path (possibly empty).
	Templates []PackageFile
	// Archive is the packaged chart (.tgz) exactly as retrieved (Digest is
	// its sha256), for consumers that need the whole package — e.g.
	// `helm template` in internal/render (subcharts, helpers, schema).
	Archive []byte
}

// Member returns the member addressed by a content kind, for callers that
// need the raw bytes: Chart.yaml for chart-metadata, values.yaml for
// helm-values. Image-refs reads several members (values, templates, CRDs).
func (p *ChartPackage) Member(contentKind string) ([]byte, bool) {
	switch contentKind {
	case catalog.ContentChartMetadata:
		return p.ChartYAML, true
	case catalog.ContentHelmValues:
		if p.Values == nil {
			return nil, false
		}
		return p.Values, true
	}
	return nil, false
}

// ChartPackageReader reads a published, packaged Helm chart at one version.
// The locator kinds it serves are registered in the Registry; the version is
// the artifact version (chart version), already resolved.
//
// Errors follow the fetch sentinel conventions: a version the (reachable)
// index or registry does not carry wraps fetch.ErrNotFound, an unreachable
// host wraps fetch.ErrUnavailable, and a repository that does not hold a
// packaged chart (an image manifest, a plain document) yields an error
// identifying that (the oci adapter wraps its ErrNotHelmChart).
type ChartPackageReader interface {
	ReadChartPackage(ctx context.Context, loc catalog.Locator, version string) (*ChartPackage, error)
}

// Image is a container image observed at a registry: its manifest and config
// blob, the registry-manifest representation of an image-refs fact.
type Image struct {
	// Repository is the repository as written in the locator (without tag).
	Repository string
	// Reference is the tag or digest that was resolved.
	Reference string
	// Digest is the manifest digest ("sha256:...").
	Digest string
	// URI is the immutable manifest URL.
	URI string
	// Labels are the config labels of the image (org.opencontainers.image.*,
	// build metadata), possibly empty.
	Labels map[string]string
	// Env is the container environment of the config, verbatim, possibly nil.
	Env []string
	// Evidence records the manifest and the config blob.
	Evidence    []domain.Evidence
	RetrievedAt time.Time
}

// ImageManifestReader reads the manifest and config blob of a container
// image, so image-refs facts can be backed by the registry itself
// (representation registry-manifest) instead of only by documents.
type ImageManifestReader interface {
	ReadImage(ctx context.Context, loc catalog.Locator, version string) (*Image, error)
}
