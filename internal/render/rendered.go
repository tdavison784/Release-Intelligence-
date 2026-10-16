package render

import (
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// RenderedPath is the path a rendered change is recorded under as a
// domain.RenderedChange (both render scopes use it, so a release-scope and an
// environment-scope record of the same chart change share a path). It is the
// change's Path, except for container arguments and RBAC rules: those are
// sets addressed by one path, and Diff emits one change per member, so the
// member is appended as a selector — "…args[=--flag]",
// "rules[=apps/deployments:get]" — keeping (scope, target, object, path,
// class) unique per change.
func RenderedPath(c Change) string {
	switch c.Class {
	case ContainerArgAdded, ContainerArgRemoved, ContainerArgChanged:
		if c.Name != "" {
			return c.Path + "[=" + c.Name + "]"
		}
	case RBACPermissionAdded, RBACPermissionRemoved:
		if c.Permission != nil {
			return c.Path + "[=" + c.Permission.String() + "]"
		}
		if c.Name != "" {
			return c.Path + "[=" + c.Name + "]"
		}
	}
	return c.Path
}

// SelfArtifactBump reports whether a rendered change is only the version bump
// of a container image the product definition itself publishes (DESIGN.md
// §1.3 scope:routine-self-artifact): an image-changed (or an argument
// carrying an image reference, "--flag=<image>") whose before and after are
// the same repository — one of the edge's container-image artifacts, which
// come from the product definition — at exactly that artifact's from and to
// coordinates' tags. It returns the repository. Declarative: the decision
// reads the edge's artifacts only; a mirrored repository, another tag or any
// other field is not a self-artifact bump.
func SelfArtifactBump(c Change, edge *domain.UpgradeEdge) (string, bool) {
	if edge == nil || c.Before == nil || c.After == nil {
		return "", false
	}
	if c.Class != ImageChanged && c.Class != ContainerArgChanged {
		return "", false
	}
	ref := func(s *string) string {
		v, _ := Unquote(s).(string)
		if c.Class == ContainerArgChanged {
			i := strings.IndexByte(v, '=')
			if i < 0 || !strings.HasPrefix(v, "-") {
				return ""
			}
			v = v[i+1:]
		}
		return v
	}
	before, after := ref(c.Before), ref(c.After)
	repo := imageRepository(after)
	if before == "" || after == "" || repo == after || imageRepository(before) != repo {
		return "", false
	}
	for _, a := range edge.Artifacts {
		if a.Type != domain.ArtifactContainerImage || a.From == nil || a.To == nil {
			continue
		}
		if imageRepository(a.To.Coordinate) != repo || imageRepository(a.From.Coordinate) != repo {
			continue
		}
		if imageTag(before) == imageTag(a.From.Coordinate) && imageTag(after) == imageTag(a.To.Coordinate) && imageTag(before) != imageTag(after) {
			return repo, true
		}
	}
	return "", false
}

// imageTag is the tag (or "@digest") part of an image reference.
func imageTag(ref string) string {
	return strings.TrimPrefix(strings.TrimPrefix(ref, imageRepository(ref)), ":")
}
