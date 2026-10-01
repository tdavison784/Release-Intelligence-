package discovery

import (
	"regexp"
	"strings"
)

// imageRef is a parsed container image (or OCI artifact) reference.
type imageRef struct {
	Registry string // normalised host, e.g. "docker.io"
	Path     string // "namespace/name" (may have more segments)
	Tag      string
	Digest   string
}

func (r imageRef) Repository() string { return r.Registry + "/" + r.Path }

// Namespace returns the first path segment when there are several.
func (r imageRef) Namespace() string {
	if i := strings.Index(r.Path, "/"); i > 0 {
		return r.Path[:i]
	}
	return ""
}

// Name returns the last path segment.
func (r imageRef) Name() string {
	if i := strings.LastIndex(r.Path, "/"); i >= 0 {
		return r.Path[i+1:]
	}
	return r.Path
}

// RegistryWithNamespace returns host/namespace (or host).
func (r imageRef) RegistryWithNamespace() string {
	if ns := r.Namespace(); ns != "" {
		return r.Registry + "/" + ns
	}
	return r.Registry
}

var knownRegistryHosts = map[string]bool{
	"docker.io": true, "index.docker.io": true, "registry-1.docker.io": true, "quay.io": true, "ghcr.io": true,
	"gcr.io": true, "registry.k8s.io": true, "k8s.gcr.io": true, "public.ecr.aws": true, "mcr.microsoft.com": true,
	"registry.gitlab.com": true, "nvcr.io": true, "cgr.dev": true, "ecr-public.aws.com": true,
}

// isRegistryHost recognises container registry hosts by well-known names
// and generic naming conventions (registry.*, cr.*, *.pkg.dev, …).
func isRegistryHost(h string) bool {
	h = strings.ToLower(h)
	if i := strings.Index(h, ":"); i >= 0 {
		h = h[:i]
	}
	switch {
	case knownRegistryHosts[h]:
		return true
	case strings.HasSuffix(h, ".gcr.io"), strings.HasSuffix(h, ".pkg.dev"), strings.HasSuffix(h, ".azurecr.io"),
		strings.Contains(h, ".dkr.ecr."), strings.HasPrefix(h, "registry."), strings.HasPrefix(h, "cr."),
		strings.HasPrefix(h, "docker."), strings.HasPrefix(h, "containers."):
		return strings.Contains(h, ".")
	}
	return false
}

func normalizeRegistry(h string) string {
	h = strings.ToLower(h)
	switch h {
	case "index.docker.io", "registry-1.docker.io", "registry.hub.docker.com":
		return "docker.io"
	}
	return h
}

const (
	imgSegment = `[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*`
	imgTag     = `(?:[A-Za-z0-9_.%-]|\$\{\{[^}]*\}\}|\$\{[^}]*\}|\$\([^)]*\)|\$[A-Za-z_][A-Za-z0-9_]*|\{\{[^}]*\}\})+`
)

var imageRefRe = regexp.MustCompile(`(?:oci://)?((?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}(?::[0-9]+)?)/(` + imgSegment + `(?:/` + imgSegment + `)*)(?::(` + imgTag + `))?(?:@(sha256:[a-f0-9]{64}))?`)

// foundRef is a reference found in text. Partial is set when the path was
// cut short by an unresolved variable: then only host/namespace is known.
type foundRef struct {
	imageRef
	OCI     bool
	Partial bool
	Raw     string
}

// findImageRefs finds registry references (images, OCI charts) in a line.
func findImageRefs(line string) []foundRef {
	var out []foundRef
	for _, m := range imageRefRe.FindAllStringSubmatchIndex(line, -1) {
		host := line[m[2]:m[3]]
		if !isRegistryHost(host) {
			continue
		}
		if m[0] > 0 {
			c := line[m[0]-1]
			if c == '.' || c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
				continue // part of a longer host or URL path
			}
		}
		r := foundRef{Raw: line[m[0]:m[1]], OCI: strings.HasPrefix(line[m[0]:], "oci://")}
		r.Registry = normalizeRegistry(host)
		r.Path = line[m[4]:m[5]]
		if m[6] >= 0 {
			r.Tag = line[m[6]:m[7]]
		}
		if m[8] >= 0 {
			r.Digest = line[m[8]:m[9]]
		}
		if m[1] < len(line) {
			if c := line[m[1]]; c == '/' || c == '$' || c == '{' || c == '%' {
				r.Partial = true
				r.Tag = ""
			}
		}
		if r.Tag != "" && (strings.HasPrefix(line[m[1]:], "/") || strings.HasPrefix(r.Tag, "//")) {
			continue // host:port-like URL, not an image
		}
		if strings.HasPrefix(line[m[0]:], "http") || (m[0] >= 3 && line[m[0]-3:m[0]] == "://" && !r.OCI) {
			// URLs of web pages under a registry host (e.g. https://quay.io/repository/x)
			// are still registry evidence, but not image references.
			r.Partial = true
			r.Tag = ""
		}
		out = append(out, r)
	}
	return out
}

// parseImageValue parses the value of an `image:` key, which may use Docker
// Hub short names ("redis:7", "istio/pilot:1.2").
func parseImageValue(v string) (imageRef, bool) {
	v = strings.Trim(strings.TrimSpace(v), `"'`)
	if v == "" || strings.ContainsAny(v, " {}$") {
		return imageRef{}, false
	}
	ref := imageRef{}
	if i := strings.Index(v, "@"); i >= 0 {
		ref.Digest = v[i+1:]
		v = v[:i]
	}
	if i := strings.LastIndex(v, ":"); i > strings.LastIndex(v, "/") {
		ref.Tag = v[i+1:]
		v = v[:i]
	}
	segs := strings.Split(v, "/")
	first := segs[0]
	switch {
	case len(segs) > 1 && (strings.ContainsAny(first, ".:") || first == "localhost"):
		ref.Registry = normalizeRegistry(first)
		ref.Path = strings.Join(segs[1:], "/")
	case len(segs) == 1:
		ref.Registry, ref.Path = "docker.io", "library/"+v
	default:
		ref.Registry, ref.Path = "docker.io", v
	}
	if !regexp.MustCompile(`^` + imgSegment + `(?:/` + imgSegment + `)*$`).MatchString(ref.Path) {
		return imageRef{}, false
	}
	return ref, true
}

var (
	hexRunRe      = regexp.MustCompile(`(?:^|[^0-9a-f])[0-9a-f]{7,40}(?:$|[^0-9a-f])`)
	floatingTagRe = regexp.MustCompile(`^(?i:latest|master|main|dev|devel|develop|nightly|edge|canary|head|unstable|testing|test)$`)
)

// Tag classes.
const (
	tagRelease    = "release"    // the release tag / version
	tagSnapshot   = "snapshot"   // commit-based builds
	tagFloating   = "floating"   // latest, master, …
	tagPinned     = "pinned"     // a concrete version unrelated to the scanned ref
	tagUnresolved = "unresolved" // build variable
	tagNone       = ""
)

// hasCommitHash reports whether s contains a 7–40 character hex run with at
// least one letter (a commit hash, not a date or a version number).
func hasCommitHash(s string) bool {
	for _, m := range hexRunRe.FindAllString(s, -1) {
		if strings.ContainsAny(strings.Trim(m, "-_.:/"), "abcdef") {
			return true
		}
	}
	return false
}

// classifyTag classifies an image tag relative to the scanned release.
// It returns the class and, for release tags, the tag template.
func classifyTag(tag, refTag, refVersion string) (string, string) {
	switch {
	case tag == "":
		return tagNone, ""
	case strings.Contains(tag, markGitCommit) || hasCommitHash(tag):
		return tagSnapshot, ""
	case tag == markGitTag || tagVarRe.MatchString(tag) && tagVarRe.FindString(tag) == tag:
		return tagRelease, tmplTag
	case refTag != "" && tag == refTag:
		return tagRelease, tmplTag
	case refVersion != "" && tag == refVersion:
		return tagRelease, tmplVersion
	case strings.Contains(tag, markVersionFile) && strings.Contains(tag, markGitCommit):
		return tagSnapshot, ""
	case floatingTagRe.MatchString(tag):
		return tagFloating, ""
	case strings.ContainsAny(tag, "$%{"):
		return tagUnresolved, ""
	}
	return tagPinned, ""
}
