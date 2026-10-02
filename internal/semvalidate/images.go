package semvalidate

import (
	"context"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// imageValidator proves image subjects and changes from the image-refs
// snapshots of both releases.
type imageValidator struct{}

func (imageValidator) Name() string { return ProducerImage }

// normRepo lowercases and strips the implicit registry and library prefixes
// so "docker.io/library/nginx" and "nginx" are the same repository.
func normRepo(r string) string {
	r = strings.ToLower(strings.TrimSpace(r))
	for _, p := range []string{"index.docker.io/", "docker.io/", "registry-1.docker.io/"} {
		r = strings.TrimPrefix(r, p)
	}
	return strings.TrimPrefix(r, "library/")
}

// imageTags returns the tags of a repository across the image snapshots of a
// release (an empty tag set with found true: referenced by digest only).
func imageTags(r *domain.Release, repo string) (tags []string, found bool, present bool, ev []domain.Evidence) {
	if r == nil {
		return
	}
	want := normRepo(repo)
	for _, s := range r.Snapshots {
		if s.Kind != domain.SnapshotImages || s.Images == nil {
			continue
		}
		present = true
		hit := false
		for _, im := range s.Images.Images {
			if normRepo(im.Repository) != want {
				continue
			}
			hit, found = true, true
			if im.Tag != "" && !contains(tags, im.Tag) {
				tags = append(tags, im.Tag)
			}
		}
		if hit {
			ev = append(ev, resolve(s.Evidence, r.Evidence)...)
		}
	}
	return
}

func (imageValidator) Validate(_ context.Context, in knowledge.ValidationInput) ([]domain.ValidationResult, error) {
	if !applies(in, domain.SubjectImage) {
		return nil, nil
	}
	subj, chg := in.Assertion.Subject, in.Assertion.Change
	fTags, fFound, fPresent, fEv := imageTags(in.From, subj.Name)
	tTags, tFound, tPresent, tEv := imageTags(in.To, subj.Name)
	if !fPresent || !tPresent {
		v := inconclusive("image:snapshot", "no image-refs snapshot on both sides")
		return build(in, ProducerImage, map[domain.Aspect]verdict{domain.AspectSubject: v, domain.AspectChange: v}, evidenceSet{}), nil
	}
	var ev evidenceSet
	ev.add(fEv...)
	ev.add(tEv...)
	// Manifests reference only the images they run: an image absent from both
	// snapshots may still exist, so absence is inconclusive, not a refutation.
	vs := map[domain.Aspect]verdict{}
	if fFound || tFound {
		vs[domain.AspectSubject] = confirmed("image:referenced", "%s is referenced by %s", subj.Name, sides(fFound, tFound))
	} else {
		vs[domain.AspectSubject] = inconclusive("image:referenced", "%s is referenced by neither release's manifests", subj.Name)
	}
	switch chg.Type {
	case domain.ChangeKindRemoved:
		switch {
		case !fFound:
			vs[domain.AspectChange] = refuted("image:removed", "%s was not referenced at the source", subj.Name)
		case tFound:
			vs[domain.AspectChange] = refuted("image:removed", "%s is still referenced at the target", subj.Name)
		default:
			vs[domain.AspectChange] = confirmed("image:removed", "%s is referenced at the source and not at the target", subj.Name)
		}
	case domain.ChangeKindAdded:
		switch {
		case fFound:
			vs[domain.AspectChange] = refuted("image:added", "%s was already referenced at the source", subj.Name)
		case !tFound:
			vs[domain.AspectChange] = refuted("image:added", "%s is not referenced at the target", subj.Name)
		default:
			vs[domain.AspectChange] = confirmed("image:added", "%s is referenced at the target only", subj.Name)
		}
	case domain.ChangeKindValueChanged:
		b, a := strings.Trim(canonJSON(ptr(chg.Before)), `"`), strings.Trim(canonJSON(ptr(chg.After)), `"`)
		switch {
		case !fFound || !tFound:
			vs[domain.AspectChange] = refuted("image:tag", "%s must be referenced on both sides for a tag change", subj.Name)
		case !contains(fTags, b):
			vs[domain.AspectChange] = refuted("image:tag", "the source tags of %s are %s, not %s", subj.Name, strings.Join(fTags, ", "), b)
		case !contains(tTags, a):
			vs[domain.AspectChange] = refuted("image:tag", "the target tags of %s are %s, not %s", subj.Name, strings.Join(tTags, ", "), a)
		case contains(tTags, b) && contains(fTags, a):
			vs[domain.AspectChange] = refuted("image:tag", "%s is referenced with both tags on both sides", subj.Name)
		default:
			vs[domain.AspectChange] = confirmed("image:tag", "%s moves from tag %s to %s", subj.Name, b, a)
		}
	default:
		vs[domain.AspectChange] = inconclusive("image:"+string(chg.Type), "a %s change is not decidable from image references", chg.Type)
	}
	return build(in, ProducerImage, vs, ev), nil
}
