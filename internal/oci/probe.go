package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// Probe implements sources.ArtifactProbe: it checks whether the manifest of
// tag (or digest) version exists in ch.Repository.
//
// The request is HEAD /v2/<name>/manifests/<tag>; when the registry rejects
// HEAD (405/501/...) or omits the Docker-Content-Digest header, a GET is made
// instead and the digest is computed from the manifest bytes. A 404
// (MANIFEST_UNKNOWN or NAME_UNKNOWN) is not an error: the result has
// Exists=false. A registry that cannot be reached yields an error wrapping
// fetch.ErrUnavailable.
//
// Versions containing "+" are probed under the Helm OCI convention ("+" ->
// "_"). Digest references ("sha256:...") are fetched as immutable.
func (a *Adapter) Probe(ctx context.Context, ch catalog.Locator, version string) (*sources.ProbeResult, error) {
	repo, err := a.repository(ch.Repository)
	if err != nil {
		return nil, err
	}
	ref, ok := NormalizeTag(version)
	if !ok {
		return nil, fmt.Errorf("oci: %q is not a valid tag or digest", version)
	}
	immutable := isDigest(ref)
	req := fetch.Request{
		URL:       repo.manifestURL(ref),
		Method:    http.MethodHead,
		Header:    accept(manifestAccept),
		Immutable: immutable,
		TTL:       a.manifestTTL,
	}

	doc, err := a.get(ctx, repo, req)
	if err != nil && headUnsupported(err) {
		req.Method = http.MethodGet
		doc, err = a.get(ctx, repo, req)
	}
	if err == nil && doc.Header.Get("Docker-Content-Digest") == "" && req.Method == http.MethodHead {
		// The registry did not state the digest on HEAD; compute it from the
		// manifest body instead.
		req.Method = http.MethodGet
		doc, err = a.get(ctx, repo, req)
	}
	switch {
	case errors.Is(err, fetch.ErrNotFound):
		return a.absent(repo, ref, err), nil
	case err != nil:
		return nil, fmt.Errorf("oci: probe %s: %w", repo.Coordinate(ref), err)
	}

	digest := doc.Header.Get("Docker-Content-Digest")
	if digest == "" && len(doc.Body) > 0 {
		digest = domain.Digest(doc.Body) // GET fallback: digest of the exact bytes
	}
	if immutable && digest == "" {
		digest = ref
	}
	mediaType := mediaTypeOf(doc)
	uri := repo.manifestURL(ref)
	if digest != "" {
		uri = repo.manifestURL(digest) // immutable URL of exactly this manifest
	}
	excerpt := "digest=" + digest
	if mediaType != "" {
		excerpt += " mediaType=" + mediaType
	}
	return &sources.ProbeResult{
		Exists:     true,
		Coordinate: repo.Coordinate(ref),
		Digest:     digest,
		URI:        uri,
		Evidence: domain.NewEvidence(domain.EvidenceRegistry, "", uri, "manifests/"+ref,
			excerpt, digest, doc.RetrievedAt),
	}, nil
}

// absent builds the result for a manifest the registry reported as unknown.
func (a *Adapter) absent(repo Repository, ref string, cause error) *sources.ProbeResult {
	uri := repo.manifestURL(ref)
	excerpt := "manifest unknown"
	var fe *fetch.Error
	if errors.As(cause, &fe) && fe.Status != 0 {
		excerpt = fmt.Sprintf("manifest unknown (HTTP %d)", fe.Status)
	}
	return &sources.ProbeResult{
		Exists:     false,
		Coordinate: repo.Coordinate(ref),
		URI:        uri,
		Evidence:   domain.NewEvidence(domain.EvidenceRegistry, "", uri, "manifests/"+ref, excerpt, "", a.now()),
	}
}

// headUnsupported reports whether a failed HEAD should be retried as GET:
// the registry answered, but not with a definitive "exists", "missing" or
// "cannot consult" verdict.
func headUnsupported(err error) bool {
	if errors.Is(err, fetch.ErrNotFound) || errors.Is(err, fetch.ErrUnavailable) || errors.Is(err, fetch.ErrOffline) {
		return false
	}
	var fe *fetch.Error
	return errors.As(err, &fe) && fe.Status >= 400 && fe.Status < 500
}

// mediaTypeOf returns the manifest media type of a response: the
// Content-Type header, or the "mediaType" member of a manifest body.
func mediaTypeOf(doc *fetch.Document) string {
	if ct := doc.Header.Get("Content-Type"); ct != "" {
		if i := strings.Index(ct, ";"); i >= 0 {
			ct = ct[:i]
		}
		return strings.TrimSpace(ct)
	}
	var m struct {
		MediaType string `json:"mediaType"`
	}
	if len(doc.Body) > 0 && json.Unmarshal(doc.Body, &m) == nil {
		return m.MediaType
	}
	return ""
}
