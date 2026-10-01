package upgrade

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Artifact and image diff rules.
const (
	RuleArtifactAdded    = "artifacts:added"
	RuleArtifactRemoved  = "artifacts:removed"
	RuleImageAdded       = "images:added"
	RuleImageRemoved     = "images:removed"
	RuleImageMoved       = "images:moved"
	RuleImageTagsChanged = "images:tag-changed"
)

var statusRank = map[domain.ArtifactStatus]int{
	domain.ArtifactVerified:      5,
	domain.ArtifactReferenced:    4,
	domain.ArtifactExpected:      3,
	domain.ArtifactMissing:       2,
	domain.ArtifactNotApplicable: 1,
}

// pickInstances selects one representative instance per artifact id: the
// best-confirmed one, and among equals the highest version.
func pickInstances(insts []domain.ArtifactInstance) map[string]*domain.ArtifactInstance {
	out := map[string]*domain.ArtifactInstance{}
	for i := range insts {
		cand := insts[i]
		cur, ok := out[cand.ArtifactID]
		if !ok || better(cand, *cur) {
			c := cand
			out[cand.ArtifactID] = &c
		}
	}
	return out
}

func better(a, b domain.ArtifactInstance) bool {
	if statusRank[a.Status] != statusRank[b.Status] {
		return statusRank[a.Status] > statusRank[b.Status]
	}
	va, ea := semver.NewVersion(a.Version)
	vb, eb := semver.NewVersion(b.Version)
	if ea == nil && eb == nil {
		return va.GreaterThan(vb)
	}
	return false
}

func applicable(i *domain.ArtifactInstance) bool {
	return i != nil && i.Status != domain.ArtifactNotApplicable
}

func sameInstance(a, b *domain.ArtifactInstance) bool {
	if a.Coordinate != b.Coordinate || a.Version != b.Version {
		return false
	}
	return a.Digest == "" || b.Digest == "" || a.Digest == b.Digest
}

func typeLabel(t domain.ArtifactType) string {
	switch t {
	case domain.ArtifactContainerImage:
		return "container image"
	case domain.ArtifactHelmChart:
		return "Helm chart"
	case domain.ArtifactSourceRelease:
		return "source release"
	case domain.ArtifactCRD:
		return "CRD bundle"
	case "":
		return "artifact"
	}
	return string(t)
}

func statusNote(s domain.ArtifactStatus) string {
	switch s {
	case domain.ArtifactVerified:
		return "verified at its channel"
	case domain.ArtifactReferenced:
		return "referenced by another artifact of the release, not observed directly"
	case domain.ArtifactExpected:
		return "expected from the product definition, not verified"
	case domain.ArtifactMissing:
		return "missing at its channel"
	}
	return string(s)
}

// artifactChanges compares artifact instances of From and To.
func (b *builder) artifactChanges() {
	fromBy, toBy := pickInstances(b.from.Artifacts), pickInstances(b.to.Artifacts)
	union := map[string]bool{}
	for id := range fromBy {
		union[id] = true
	}
	for id := range toBy {
		union[id] = true
	}
	ids := sortedKeys(union)
	b.artifactOrder(ids)
	fromTag, toTag := b.from.Version.String(), b.to.Version.String()

	for _, id := range ids {
		f, t := fromBy[id], toBy[id]
		fp, tp := applicable(f), applicable(t)
		if !fp && !tp {
			continue
		}
		ac := domain.ArtifactChange{ArtifactID: id, From: f, To: t}
		ref := t
		if ref == nil || !tp {
			ref = f
		}
		ac.Type, ac.Name = ref.Type, ref.Name
		var ev []domain.EvidenceID
		if f != nil {
			ev = append(ev, f.Evidence...)
		}
		if t != nil {
			ev = append(ev, t.Evidence...)
		}
		ac.Evidence = b.resolve(ev...)
		switch {
		case !fp:
			ac.Change = domain.ChangeAdded
		case !tp:
			ac.Change = domain.ChangeRemoved
		case sameInstance(f, t):
			ac.Change = domain.ChangeUnchanged
		default:
			ac.Change = domain.ChangeUpdated
		}
		b.edge.Artifacts = append(b.edge.Artifacts, ac)

		name := ac.Name
		if name == "" {
			name = id
		}
		switch ac.Change {
		case domain.ChangeAdded:
			b.addChange(domain.Change{
				Category: domain.CategoryArtifact,
				Title:    fmt.Sprintf("New %s %s published: %s", typeLabel(t.Type), code(name), t.Coordinate),
				Detail: fmt.Sprintf("Not part of %s; %s ships it (status: %s — %s). If you mirror artifacts into a private registry, add it to your mirror list.",
					fromTag, toTag, t.Status, statusNote(t.Status)),
				Subjects:   []string{t.Coordinate},
				Provenance: computed(RuleArtifactAdded),
				Evidence:   ev,
			}, RuleArtifactAdded, id)
		case domain.ChangeRemoved:
			reason := "not part of " + toTag
			if t != nil && t.Status == domain.ArtifactNotApplicable {
				reason = "not applicable to " + toTag + " according to the product definition"
				if t.Detail != "" {
					reason += " (" + t.Detail + ")"
				}
			}
			b.addChange(domain.Change{
				Category:       domain.CategoryArtifact,
				ActionRequired: true,
				Title:          fmt.Sprintf("%s %s is no longer published", capitalize(typeLabel(f.Type)), code(name)),
				Detail: fmt.Sprintf("Present in %s as %s (%s); %s. Remove references to it (image mirrors, overrides, automation) and switch to its replacement if one is documented.",
					fromTag, f.Coordinate, f.Status, reason),
				Subjects:   []string{f.Coordinate},
				Provenance: computed(RuleArtifactRemoved),
				Evidence:   ev,
			}, RuleArtifactRemoved, id)
		}
		if tp && t.Status == domain.ArtifactMissing {
			detail := ""
			if t.Detail != "" {
				detail = ": " + t.Detail
			}
			b.warnf("%s %s of %s is missing at its channel (%s)%s", capitalize(typeLabel(t.Type)), name, toTag, t.Coordinate, detail)
		}
	}

	// honest about unverified target artifacts
	var expected []string
	total := 0
	for _, id := range ids {
		t := toBy[id]
		if !applicable(t) {
			continue
		}
		total++
		if t.Status == domain.ArtifactExpected {
			expected = append(expected, id)
		}
	}
	switch {
	case total > 0 && len(expected) == total:
		b.warnf("None of the %d artifacts of %s could be verified; all are only expected from the product definition (channels unreachable)", total, toTag)
	case len(expected) > 0:
		b.warnf("%s of %s only expected (not observed or referenced): %s", plural(len(expected), "artifact", "artifacts"), toTag, strings.Join(expected, ", "))
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// splitCoordinate splits "repo:tag" / "repo@digest" into base and version part.
func splitCoordinate(c string) (base, ver string) {
	if i := strings.LastIndexByte(c, '@'); i > 0 {
		return c[:i], c[i+1:]
	}
	slash := strings.LastIndexByte(c, '/')
	if i := strings.LastIndexByte(c, ':'); i > slash && i > 0 {
		return c[:i], c[i+1:]
	}
	return c, ""
}

// imageSide aggregates image references of one endpoint.
type imageSide struct {
	tags map[string]map[string]bool // repository → tag labels
}

func refLabel(r domain.ImageRef) string {
	switch {
	case r.Tag != "":
		return r.Tag
	case r.Digest != "":
		return "@" + shorten(r.Digest, 19)
	}
	return "(untagged)"
}

func collectImages(snaps map[string]*domain.Snapshot, ids []string) imageSide {
	s := imageSide{tags: map[string]map[string]bool{}}
	for _, id := range ids {
		for _, r := range snaps[id].Images.Images {
			if s.tags[r.Repository] == nil {
				s.tags[r.Repository] = map[string]bool{}
			}
			s.tags[r.Repository][refLabel(r)] = true
		}
	}
	return s
}

func tagList(m map[string]bool) []string { return sortedKeys(m) }

func lastSegment(repo string) string {
	if i := strings.LastIndexByte(repo, '/'); i >= 0 {
		return repo[i+1:]
	}
	return repo
}

// imageChanges diffs third-party image references (image-refs snapshots).
// Repositories of the product's own artifacts are covered by artifactChanges.
func (b *builder) imageChanges() {
	ids, from, to := b.snapshotPairs(domain.SnapshotImages, "Image references", func(s *domain.Snapshot) bool { return s.Images != nil })
	if len(ids) == 0 {
		return
	}
	var evidence []domain.EvidenceID
	for _, id := range ids {
		evidence = append(evidence, from[id].Evidence...)
		evidence = append(evidence, to[id].Evidence...)
	}
	product := map[string]bool{}
	for _, r := range []*domain.Release{b.from, b.to} {
		for _, a := range r.Artifacts {
			if a.Coordinate == "" {
				continue
			}
			base, _ := splitCoordinate(a.Coordinate)
			product[base] = true
		}
	}
	releaseTags := func(r *domain.Release) map[string]bool {
		return map[string]bool{r.Version.Tag: true, r.Version.Semver: true}
	}
	fromTags, toTags := releaseTags(b.from), releaseTags(b.to)
	onlyRelease := func(tags map[string]bool, rel map[string]bool) bool {
		for t := range tags {
			if !rel[t] {
				return false
			}
		}
		return len(tags) > 0
	}

	fs, ts := collectImages(from, ids), collectImages(to, ids)
	var removed, added []string
	for _, repo := range sortedKeys(fs.tags) {
		if product[repo] {
			continue
		}
		if _, ok := ts.tags[repo]; !ok {
			removed = append(removed, repo)
		}
	}
	for _, repo := range sortedKeys(ts.tags) {
		if product[repo] {
			continue
		}
		if _, ok := fs.tags[repo]; !ok {
			added = append(added, repo)
		}
	}

	// pair removed/added repositories with the same image name (registry move)
	moved := map[string]string{}
	usedAdded := map[string]bool{}
	for _, r := range removed {
		var match []string
		for _, a := range added {
			if lastSegment(a) == lastSegment(r) {
				match = append(match, a)
			}
		}
		var rivals int
		for _, r2 := range removed {
			if lastSegment(r2) == lastSegment(r) {
				rivals++
			}
		}
		if len(match) == 1 && rivals == 1 {
			moved[r] = match[0]
			usedAdded[match[0]] = true
		}
	}

	add := func(c domain.Change, rule string, parts ...string) {
		c.Provenance = computed(rule)
		c.Evidence = evidence
		b.addChange(c, append([]string{rule}, parts...)...)
	}
	for _, r := range removed {
		ft := tagList(fs.tags[r])
		if a, ok := moved[r]; ok {
			tt := tagList(ts.tags[a])
			add(domain.Change{
				Category: domain.CategoryDependency,
				Title:    fmt.Sprintf("Image %s now pulled from %s (was %s)", code(lastSegment(r)), code(a+":"+strings.Join(tt, ",")), code(r+":"+strings.Join(ft, ","))),
				Detail:   "The registry/repository of this third-party image changed. Update image mirrors, pull-through caches and registry allow-lists.",
				Subjects: []string{r, a},
			}, RuleImageMoved, r, a)
			continue
		}
		if onlyRelease(fs.tags[r], fromTags) {
			add(domain.Change{
				Category: domain.CategoryArtifact,
				Title:    fmt.Sprintf("Manifests no longer reference image %s", code(r)),
				Subjects: []string{r},
			}, RuleImageRemoved, r)
			continue
		}
		add(domain.Change{
			Category: domain.CategoryDependency,
			Title:    fmt.Sprintf("Third-party image %s no longer referenced", code(r+":"+strings.Join(ft, ","))),
			Subjects: []string{r},
		}, RuleImageRemoved, r)
	}
	for _, a := range added {
		if usedAdded[a] {
			continue
		}
		tt := tagList(ts.tags[a])
		if onlyRelease(ts.tags[a], toTags) {
			add(domain.Change{
				Category: domain.CategoryArtifact,
				Title:    fmt.Sprintf("Manifests now reference image %s", code(a+":"+strings.Join(tt, ","))),
				Detail:   "If you mirror images into a private registry, add it to your mirror list.",
				Subjects: []string{a},
			}, RuleImageAdded, a)
			continue
		}
		add(domain.Change{
			Category: domain.CategoryDependency,
			Title:    fmt.Sprintf("New third-party image %s", code(a+":"+strings.Join(tt, ","))),
			Detail:   "If you mirror images into a private registry, add it to your mirror list.",
			Subjects: []string{a},
		}, RuleImageAdded, a)
	}
	var both []string
	for _, repo := range sortedKeys(fs.tags) {
		if _, ok := ts.tags[repo]; ok && !product[repo] {
			both = append(both, repo)
		}
	}
	for _, repo := range both {
		ft, tt := tagList(fs.tags[repo]), tagList(ts.tags[repo])
		if strings.Join(ft, ",") == strings.Join(tt, ",") {
			continue
		}
		if onlyRelease(fs.tags[repo], fromTags) && onlyRelease(ts.tags[repo], toTags) {
			continue // the product's own image following the release tag
		}
		add(domain.Change{
			Category: domain.CategoryDependency,
			Title:    fmt.Sprintf("Third-party image %s: %s → %s", code(repo), strings.Join(ft, ", "), strings.Join(tt, ", ")),
			Subjects: []string{repo},
		}, RuleImageTagsChanged, repo)
	}
}
