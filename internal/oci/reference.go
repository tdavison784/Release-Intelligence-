package oci

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Repository is a parsed OCI repository reference such as
// "quay.io/jetstack/cert-manager-controller" or "docker.io/istio/pilot".
//
// The user-facing form (Given) is preserved so that coordinates reported to
// the rest of the system look exactly like the locator in the product
// definition ("docker.io/istio/pilot:1.26.0"), while Registry/Name/APIBase say
// how to talk to the registry's Distribution API.
type Repository struct {
	// Given is the repository as written in the locator, without an
	// "oci://" prefix. It is the prefix of every reported coordinate.
	Given string
	// Registry is the registry host as written ("docker.io", "quay.io",
	// "127.0.0.1:5000"); it is "docker.io" for names without a registry.
	Registry string
	// Name is the repository path used in /v2/<Name>/... requests. For Docker
	// Hub, single-segment names are expanded to "library/<name>".
	Name string
	// APIBase is the scheme and host of the Distribution API, without a
	// trailing slash ("https://registry-1.docker.io").
	APIBase string
}

// dockerHubHosts are registry names that all denote Docker Hub.
var dockerHubHosts = map[string]bool{
	"docker.io":               true,
	"index.docker.io":         true,
	"registry-1.docker.io":    true,
	"registry.hub.docker.com": true,
}

// dockerHubAPI is the Distribution API endpoint of Docker Hub.
const dockerHubAPI = "https://registry-1.docker.io"

// pathComponent is one "/"-separated component of a repository name
// (distribution/reference grammar, lower-case only).
var pathComponent = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$`)

// ParseRepository parses a repository reference. Accepted forms:
//
//	quay.io/jetstack/cert-manager-controller
//	docker.io/istio/pilot          (API host registry-1.docker.io)
//	docker.io/nginx                (-> library/nginx)
//	istio/pilot, nginx             (no registry: Docker Hub)
//	oci://ghcr.io/istio/release/charts/istiod
//	127.0.0.1:5000/team/app        (loopback registries are spoken to over plain HTTP)
//
// A tag or digest must not be part of the reference.
func ParseRepository(s string) (Repository, error) {
	given := strings.TrimSpace(s)
	given = strings.TrimPrefix(given, "oci://")
	if given == "" {
		return Repository{}, fmt.Errorf("oci: empty repository")
	}
	if strings.ContainsAny(given, " \t\r\n@?#") {
		return Repository{}, fmt.Errorf("oci: invalid repository %q", s)
	}

	host, rest := "", given
	if i := strings.Index(given, "/"); i > 0 {
		first := given[:i]
		// Same disambiguation as docker/distribution: the first component is
		// a registry host when it looks like a host name.
		if strings.ContainsAny(first, ".:") || first == "localhost" {
			host, rest = first, given[i+1:]
		}
	}
	if host == "" {
		host = "docker.io"
	}
	if rest == "" {
		return Repository{}, fmt.Errorf("oci: repository %q has no name", s)
	}
	for _, c := range strings.Split(rest, "/") {
		if strings.Contains(c, ":") {
			return Repository{}, fmt.Errorf("oci: repository %q must not include a tag or digest", s)
		}
		if !pathComponent.MatchString(c) {
			return Repository{}, fmt.Errorf("oci: invalid repository name component %q in %q", c, s)
		}
	}

	r := Repository{Given: given, Registry: host, Name: rest}
	if dockerHubHosts[strings.ToLower(host)] {
		r.Registry = "docker.io"
		r.APIBase = dockerHubAPI
		if !strings.Contains(rest, "/") {
			r.Name = "library/" + rest
		}
		// Given deliberately stays as written ("docker.io/istio/pilot",
		// "nginx"): coordinates keep the user-facing form.
		return r, nil
	}
	scheme := "https"
	if isLoopback(host) {
		scheme = "http"
	}
	r.APIBase = scheme + "://" + host
	return r, nil
}

// isLoopback reports whether host (with optional port) is a loopback address
// or "localhost". Such registries are contacted over plain HTTP, matching the
// convention of docker and containerd.
func isLoopback(host string) bool {
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	h = strings.Trim(h, "[]")
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// String returns the repository as given.
func (r Repository) String() string { return r.Given }

// Coordinate returns the user-facing coordinate of a tag or digest:
// "<given>:<tag>" or "<given>@<digest>".
func (r Repository) Coordinate(reference string) string {
	if isDigest(reference) {
		return r.Given + "@" + reference
	}
	return r.Given + ":" + reference
}

// manifestURL returns the Distribution API URL of a manifest.
func (r Repository) manifestURL(reference string) string {
	return r.APIBase + "/v2/" + r.Name + "/manifests/" + reference
}

// blobURL returns the Distribution API URL of a blob.
func (r Repository) blobURL(digest string) string {
	return r.APIBase + "/v2/" + r.Name + "/blobs/" + digest
}

// tagsURL returns the first page of the tag listing.
func (r Repository) tagsURL(pageSize int) string {
	return fmt.Sprintf("%s/v2/%s/tags/list?n=%d", r.APIBase, r.Name, pageSize)
}

var (
	digestRe = regexp.MustCompile(`^[a-z0-9]+(?:[+._-][a-z0-9]+)*:[0-9a-fA-F]{32,}$`)
	tagRe    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)
)

// isDigest reports whether s is an OCI digest ("sha256:<hex>").
func isDigest(s string) bool { return digestRe.MatchString(s) }

// NormalizeTag converts a version into an OCI tag. Helm stores charts whose
// semver carries build metadata under a tag with "+" replaced by "_" (OCI
// tags cannot contain "+"); the same mapping is applied here. The second
// result reports whether the outcome is a valid tag (or a digest).
func NormalizeTag(version string) (string, bool) {
	v := strings.TrimSpace(version)
	if isDigest(v) {
		return v, true
	}
	v = strings.ReplaceAll(v, "+", "_")
	return v, tagRe.MatchString(v)
}
