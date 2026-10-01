package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
)

// Media types of Helm charts stored in OCI registries.
const (
	// HelmConfigMediaType is the media type of a Helm chart's config blob.
	HelmConfigMediaType = "application/vnd.cncf.helm.config.v1+json"
	// HelmChartLayerMediaType is the media type of the chart archive layer.
	HelmChartLayerMediaType = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"
)

// ErrNotHelmChart is returned by ChartConfig when the manifest is not a Helm
// chart manifest (its config blob has another media type).
var ErrNotHelmChart = errors.New("not a Helm chart manifest")

// Descriptor is an OCI content descriptor.
type Descriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Manifest is a fetched image/artifact manifest or index.
type Manifest struct {
	Repository string // as given
	Reference  string // tag or digest requested
	Digest     string // digest of the manifest bytes
	MediaType  string
	URI        string // immutable manifest URL (by digest)

	Config      Descriptor        // image/artifact manifests
	Layers      []Descriptor      // image/artifact manifests
	Manifests   []Descriptor      // indexes / manifest lists
	Annotations map[string]string // manifest-level annotations
	Evidence    domain.Evidence   // the manifest as a registry evidence record
}

// GetManifest fetches and decodes the manifest of reference (tag or digest)
// in repository with GET. Tag-addressed manifests are cached with the
// adapter's manifest TTL, digest-addressed ones forever.
func (a *Adapter) GetManifest(ctx context.Context, repository, reference string) (*Manifest, error) {
	repo, err := a.repository(repository)
	if err != nil {
		return nil, err
	}
	ref, ok := NormalizeTag(reference)
	if !ok {
		return nil, fmt.Errorf("oci: %q is not a valid tag or digest", reference)
	}
	doc, err := a.get(ctx, repo, fetch.Request{
		URL:       repo.manifestURL(ref),
		Method:    http.MethodGet,
		Header:    accept(manifestAccept),
		Immutable: isDigest(ref),
		TTL:       a.manifestTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("oci: get manifest %s: %w", repo.Coordinate(ref), err)
	}
	var raw struct {
		MediaType   string            `json:"mediaType"`
		Config      Descriptor        `json:"config"`
		Layers      []Descriptor      `json:"layers"`
		Manifests   []Descriptor      `json:"manifests"`
		Annotations map[string]string `json:"annotations"`
	}
	if err := json.Unmarshal(doc.Body, &raw); err != nil {
		return nil, fmt.Errorf("oci: decode manifest %s: %w", repo.Coordinate(ref), err)
	}
	digest := doc.Header.Get("Docker-Content-Digest")
	if digest == "" {
		digest = domain.Digest(doc.Body)
	}
	mt := mediaTypeOf(doc)
	if raw.MediaType != "" {
		mt = raw.MediaType
	}
	uri := repo.manifestURL(digest)
	return &Manifest{
		Repository:  repo.Given,
		Reference:   ref,
		Digest:      digest,
		MediaType:   mt,
		URI:         uri,
		Config:      raw.Config,
		Layers:      raw.Layers,
		Manifests:   raw.Manifests,
		Annotations: raw.Annotations,
		Evidence: domain.NewEvidence(domain.EvidenceRegistry, "", uri, "manifests/"+ref,
			"digest="+digest+" mediaType="+mt, digest, doc.RetrievedAt),
	}, nil
}

// ChartConfig is the content of an OCI Helm chart's config blob
// (application/vnd.cncf.helm.config.v1+json), i.e. the chart's Chart.yaml as
// JSON. Only the commonly needed fields are decoded.
type ChartConfig struct {
	APIVersion  string `json:"apiVersion,omitempty"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	AppVersion  string `json:"appVersion,omitempty"`
	KubeVersion string `json:"kubeVersion,omitempty"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Deprecated  bool   `json:"deprecated,omitempty"`
}

// HelmChart is a Helm chart read from an OCI registry.
type HelmChart struct {
	Manifest *Manifest
	Config   ChartConfig
	// ConfigDigest is the digest of the config blob.
	ConfigDigest string
	// Evidence points at the config blob (immutable, digest-addressed).
	Evidence domain.Evidence
}

// ChartConfig reads the Helm chart config of repository:reference: it fetches
// the manifest, checks that its config media type is HelmConfigMediaType and
// downloads and decodes the config blob (digest-addressed, cached forever).
//
// It returns ErrNotHelmChart (wrapped) for manifests that are not Helm
// charts. If the blob cannot be downloaded the error wraps the fetch
// sentinel (for example fetch.ErrUnavailable for ghcr.io, whose blob
// downloads redirect to a host that may be blocked); the manifest-level
// annotations are then still available through GetManifest.
func (a *Adapter) ChartConfig(ctx context.Context, repository, reference string) (*HelmChart, error) {
	m, err := a.GetManifest(ctx, repository, reference)
	if err != nil {
		return nil, err
	}
	if m.Config.MediaType != HelmConfigMediaType {
		return nil, fmt.Errorf("oci: %s:%s: %w (config media type %q)", m.Repository, m.Reference, ErrNotHelmChart, m.Config.MediaType)
	}
	repo, err := a.repository(repository)
	if err != nil {
		return nil, err
	}
	doc, err := a.get(ctx, repo, fetch.Request{
		URL:       repo.blobURL(m.Config.Digest),
		Header:    accept("application/json"),
		Immutable: true,
	})
	if err != nil {
		return nil, fmt.Errorf("oci: chart config blob of %s: %w", repo.Coordinate(m.Reference), err)
	}
	if got := domain.Digest(doc.Body); isSHA256(m.Config.Digest) && got != m.Config.Digest {
		return nil, fmt.Errorf("oci: chart config blob of %s: digest mismatch (want %s, got %s)",
			repo.Coordinate(m.Reference), m.Config.Digest, got)
	}
	var cfg ChartConfig
	if err := json.Unmarshal(doc.Body, &cfg); err != nil {
		return nil, fmt.Errorf("oci: decode chart config of %s: %w", repo.Coordinate(m.Reference), err)
	}
	uri := repo.blobURL(m.Config.Digest)
	return &HelmChart{
		Manifest:     m,
		Config:       cfg,
		ConfigDigest: m.Config.Digest,
		Evidence: domain.NewEvidence(domain.EvidenceStructured, "", uri, "$",
			fmt.Sprintf("name: %s, version: %s, appVersion: %s", cfg.Name, cfg.Version, cfg.AppVersion),
			m.Config.Digest, doc.RetrievedAt),
	}, nil
}

// isSHA256 reports whether d is a sha256 digest (the only algorithm whose
// digest can be recomputed with domain.Digest).
func isSHA256(d string) bool { return len(d) > 7 && d[:7] == "sha256:" }
