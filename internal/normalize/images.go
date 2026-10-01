package normalize

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

var (
	// `image: ref`, `- image: "ref"`, `"image": "ref",`
	imageKeyRe = regexp.MustCompile(`(?:^|[\s{,\[-])["']?image["']?\s*:\s*(?:"([^"]*)"|'([^']*)'|([^\s#,}\]"']+))`)
	// `--foo-image=ref`, `--image=ref`, optionally quoted
	imageFlagRe = regexp.MustCompile(`--(?:[A-Za-z0-9][\w.-]*-)?image=(?:"([^"]*)"|'([^']*)'|([^\s"',}\]]+))`)

	imageRepoRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(?::[0-9]+)?(?:/[A-Za-z0-9][A-Za-z0-9._-]*)*$`)
	imageTagRe    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	imageDigestRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*(?:[+._-][A-Za-z][A-Za-z0-9]*)*:[0-9a-fA-F]{32,}$`)
)

// imageRefs extracts container image references from manifests or values:
// `image: <ref>` (quoted or not, also as a list item or JSON key) and
// `--<name>-image=<ref>` flags. Values containing template or shell
// substitutions ("{{", "${", "$(") and empty values are skipped, as are
// comment lines. The result is de-duplicated and sorted by repository, tag,
// digest.
func imageRefs(content []byte) []domain.ImageRef {
	seen := map[domain.ImageRef]bool{}
	var out []domain.ImageRef
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || strings.Contains(v, "{{") || strings.Contains(v, "${") || strings.Contains(v, "$(") {
			return
		}
		ref, err := parseImageRef(v)
		if err != nil || seen[ref] {
			return
		}
		seen[ref] = true
		out = append(out, ref)
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		if t := strings.TrimSpace(line); t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		for _, m := range imageKeyRe.FindAllStringSubmatch(line, -1) {
			add(firstNonEmpty(m[1], m[2], m[3]))
		}
		for _, m := range imageFlagRe.FindAllStringSubmatch(line, -1) {
			add(firstNonEmpty(m[1], m[2], m[3]))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Repository != b.Repository {
			return a.Repository < b.Repository
		}
		if a.Tag != b.Tag {
			return a.Tag < b.Tag
		}
		return a.Digest < b.Digest
	})
	return out
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// parseImageRef parses "[registry[:port]/]path[:tag][@digest]". The tag colon
// must come after the last "/" (so "localhost:5000/app" has no tag). The
// repository is kept as written (no docker.io defaulting).
func parseImageRef(s string) (domain.ImageRef, error) {
	orig := s
	s = strings.TrimSpace(s)
	if s == "" {
		return domain.ImageRef{}, fmt.Errorf("normalize: empty image reference")
	}
	var ref domain.ImageRef
	if i := strings.Index(s, "@"); i >= 0 {
		ref.Digest = s[i+1:]
		s = s[:i]
		if !imageDigestRe.MatchString(ref.Digest) {
			return domain.ImageRef{}, fmt.Errorf("normalize: image reference %q: invalid digest", orig)
		}
	}
	if c := strings.LastIndex(s, ":"); c > strings.LastIndex(s, "/") {
		ref.Tag = s[c+1:]
		s = s[:c]
		if !imageTagRe.MatchString(ref.Tag) {
			return domain.ImageRef{}, fmt.Errorf("normalize: image reference %q: invalid tag", orig)
		}
	}
	if !imageRepoRe.MatchString(s) {
		return domain.ImageRef{}, fmt.Errorf("normalize: image reference %q: invalid repository", orig)
	}
	ref.Repository = s
	return ref, nil
}
