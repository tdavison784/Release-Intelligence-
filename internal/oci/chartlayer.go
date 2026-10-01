package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/helm"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// ReadChartPackage implements sources.ChartPackageReader for the "oci" locator
// kind: it pulls a Helm chart published as an OCI artifact. The chart archive
// is the manifest layer with media type HelmChartLayerMediaType; it is
// downloaded as a digest-addressed (immutable) blob and its digest verified
// against the manifest before unpacking, so the package is exactly the bytes
// the registry serves for the tag.
//
// The representation is published-oci-chart. Evidence records the manifest
// (registry kind) and the chart archive (the layer, digest-pinned). A
// repository that holds no Helm chart yields an error wrapping
// ErrNotHelmChart; errors keep the fetch sentinels (ErrNotFound for an
// unknown tag, ErrUnavailable for an unreachable registry or a blob download
// a network policy blocks).
func (a *Adapter) ReadChartPackage(ctx context.Context, loc catalog.Locator, version string) (*sources.ChartPackage, error) {
	if loc.Repository == "" {
		return nil, fmt.Errorf("oci: locator has no repository")
	}
	ref, ok := NormalizeTag(version)
	if !ok {
		return nil, fmt.Errorf("oci: %q is not a valid tag or digest", version)
	}
	m, err := a.GetManifest(ctx, loc.Repository, ref)
	if err != nil {
		return nil, err
	}
	if m.Config.MediaType != HelmConfigMediaType {
		return nil, fmt.Errorf("oci: %s:%s: %w (config media type %q)", m.Repository, m.Reference, ErrNotHelmChart, m.Config.MediaType)
	}
	var layer *Descriptor
	for i := range m.Layers {
		if m.Layers[i].MediaType == HelmChartLayerMediaType {
			layer = &m.Layers[i]
			break
		}
	}
	if layer == nil {
		return nil, fmt.Errorf("oci: %s:%s: manifest has no layer of type %s",
			m.Repository, m.Reference, HelmChartLayerMediaType)
	}
	repo, err := a.repository(loc.Repository)
	if err != nil {
		return nil, err
	}
	doc, err := a.get(ctx, repo, fetch.Request{URL: repo.blobURL(layer.Digest), Immutable: true})
	if err != nil {
		return nil, fmt.Errorf("oci: chart layer of %s: %w", repo.Coordinate(m.Reference), err)
	}
	if got := domain.Digest(doc.Body); isSHA256(layer.Digest) && got != layer.Digest {
		return nil, fmt.Errorf("oci: chart layer of %s: digest mismatch (want %s, got %s)",
			repo.Coordinate(m.Reference), layer.Digest, got)
	}
	arc, err := helm.ReadArchive(bytes.NewReader(doc.Body))
	if err != nil {
		return nil, fmt.Errorf("oci: chart layer of %s: %w", repo.Coordinate(m.Reference), err)
	}
	pkg := &sources.ChartPackage{
		Representation: domain.RepresentationOCIChart,
		URI:            m.URI,
		Coordinate:     repo.Coordinate(m.Reference),
		Digest:         layer.Digest,
		RetrievedAt:    doc.RetrievedAt,
		ChartDir:       arc.ChartDir,
		ChartYAML:      arc.ChartYAML,
		Values:         arc.Values,
	}
	for _, f := range arc.CRDs {
		pkg.CRDs = append(pkg.CRDs, sources.PackageFile{Path: f.Path, Content: f.Content})
	}
	for _, f := range arc.Templates {
		pkg.Templates = append(pkg.Templates, sources.PackageFile{Path: f.Path, Content: f.Content})
	}
	excerpt := fmt.Sprintf("chart layer %s", layer.Digest)
	if arc.ChartMeta != nil {
		excerpt = fmt.Sprintf("chart %s %s (appVersion %s) as layer %s",
			arc.ChartMeta.Name, arc.ChartMeta.Version, arc.ChartMeta.AppVersion, layer.Digest)
	}
	layerEv := domain.NewEvidence(domain.EvidenceRegistry, "", repo.blobURL(layer.Digest),
		"blobs/"+layer.Digest, excerpt, layer.Digest, doc.RetrievedAt).
		WithRepresentation(domain.RepresentationOCIChart)
	pkg.Evidence = []domain.Evidence{m.Evidence.WithRepresentation(domain.RepresentationOCIChart), layerEv}
	return pkg, nil
}

// ImageConfig is the config blob of a container image manifest (the
// registry-manifest representation of an image: digests, labels, environment).
type ImageConfig struct {
	// Manifest the config belongs to.
	Manifest *Manifest
	// Labels are the image's config labels (org.opencontainers.image.* and
	// build metadata), possibly empty.
	Labels map[string]string
	// Env is the image's container environment, verbatim, possibly nil.
	Env []string
	// Created is the "created" timestamp of the config, verbatim.
	Created string
	// Architecture and OS name the image's platform.
	Architecture string
	OS           string
	// ConfigDigest is the digest of the config blob.
	ConfigDigest string
	// Evidence points at the config blob (immutable, digest-addressed).
	Evidence domain.Evidence
}

// ReadImage implements sources.ImageManifestReader: it resolves the manifest
// of repository:version and downloads its config blob, surfacing labels and
// digests as evidence for image-refs facts (representation
// registry-manifest). Errors follow the fetch sentinels.
func (a *Adapter) ReadImage(ctx context.Context, loc catalog.Locator, version string) (*sources.Image, error) {
	ic, err := a.ImageConfig(ctx, loc.Repository, version)
	if err != nil {
		return nil, err
	}
	m := ic.Manifest
	img := &sources.Image{
		Repository:  loc.Repository,
		Reference:   m.Reference,
		Digest:      m.Digest,
		URI:         m.URI,
		Labels:      ic.Labels,
		Env:         ic.Env,
		RetrievedAt: ic.Evidence.RetrievedAt,
	}
	if img.RetrievedAt.IsZero() {
		img.RetrievedAt = a.now()
	}
	img.Evidence = []domain.Evidence{
		m.Evidence.WithRepresentation(domain.RepresentationRegistryManifest),
		ic.Evidence,
	}
	return img, nil
}

// ImageConfig fetches and decodes the config blob of repository:reference.
// The blob is downloaded digest-addressed (immutable) and its digest verified
// when it is a sha256. Helm chart manifests are rejected with ErrNotHelmChart
// (use ReadChartPackage for those). An index (manifest list) is resolved
// deterministically: the linux/amd64 platform manifest when present, else the
// first linux one, else the first entry — the digest and labels of an image
// config do not depend on the platform choice in practice, but the selection
// rule is fixed so runs are reproducible.
func (a *Adapter) ImageConfig(ctx context.Context, repository, reference string) (*ImageConfig, error) {
	m, err := a.GetManifest(ctx, repository, reference)
	if err != nil {
		return nil, err
	}
	if m.Config.MediaType == HelmConfigMediaType {
		return nil, fmt.Errorf("oci: %s:%s: %w", m.Repository, m.Reference, ErrNotHelmChart)
	}
	if len(m.Layers) == 0 && len(m.Manifests) > 0 {
		sel := selectPlatformManifest(m.Manifests)
		if sel == nil {
			return nil, fmt.Errorf("oci: %s:%s is an empty manifest list", m.Repository, m.Reference)
		}
		index := m
		m, err = a.GetManifest(ctx, repository, sel.Digest)
		if err != nil {
			return nil, err
		}
		if m.Config.MediaType == HelmConfigMediaType {
			return nil, fmt.Errorf("oci: %s@%s: %w", m.Repository, m.Reference, ErrNotHelmChart)
		}
		_ = index
		// record the selection in the manifest evidence (rebuilt, so the
		// content-derived id stays consistent with the excerpt)
		ev := m.Evidence
		m.Evidence = domain.NewEvidence(ev.Kind, ev.SourceID, ev.URI, ev.Locator,
			fmt.Sprintf("%s (selected platform manifest %s)", ev.Excerpt, sel.Digest), ev.ContentDigest, ev.RetrievedAt)
	}
	repo, err := a.repository(repository)
	if err != nil {
		return nil, err
	}
	doc, err := a.get(ctx, repo, fetch.Request{URL: repo.blobURL(m.Config.Digest), Immutable: true})
	if err != nil {
		return nil, fmt.Errorf("oci: config blob of %s: %w", repo.Coordinate(m.Reference), err)
	}
	if got := domain.Digest(doc.Body); isSHA256(m.Config.Digest) && got != m.Config.Digest {
		return nil, fmt.Errorf("oci: config blob of %s: digest mismatch (want %s, got %s)",
			repo.Coordinate(m.Reference), m.Config.Digest, got)
	}
	var raw struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
		Created      string `json:"created"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
			Env    []string          `json:"Env"`
		} `json:"config"`
	}
	if err := json.Unmarshal(doc.Body, &raw); err != nil {
		return nil, fmt.Errorf("oci: decode config of %s: %w", repo.Coordinate(m.Reference), err)
	}
	ev := domain.NewEvidence(domain.EvidenceStructured, "", repo.blobURL(m.Config.Digest), "blobs/"+m.Config.Digest,
		fmt.Sprintf("image config: architecture=%s os=%s labels=%d", raw.Architecture, raw.OS, len(raw.Config.Labels)),
		m.Config.Digest, doc.RetrievedAt).WithRepresentation(domain.RepresentationRegistryManifest)
	return &ImageConfig{
		Manifest:     m,
		Labels:       raw.Config.Labels,
		Env:          raw.Config.Env,
		Created:      raw.Created,
		Architecture: raw.Architecture,
		OS:           raw.OS,
		ConfigDigest: m.Config.Digest,
		Evidence:     ev,
	}, nil
}

// selectPlatformManifest picks one platform manifest of an index,
// deterministically: linux/amd64, else the first linux, else the first.
func selectPlatformManifest(ms []Descriptor) *Descriptor {
	for i := range ms {
		p := ms[i].Platform
		if p != nil && p.OS == "linux" && p.Architecture == "amd64" {
			return &ms[i]
		}
	}
	for i := range ms {
		if p := ms[i].Platform; p != nil && p.OS == "linux" {
			return &ms[i]
		}
	}
	return &ms[0]
}
