package ingest

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

func (i *Ingester) listVersions(ctx context.Context, def *catalog.ProductDefinition) (*VersionList, error) {
	if def == nil {
		return nil, errors.New("ingest: nil product definition")
	}
	if i.Registry == nil {
		return nil, errors.New("ingest: no source registry")
	}
	parser, err := def.VersionParser()
	if err != nil {
		return nil, fmt.Errorf("ingest: %s versioning: %w", def.ID, err)
	}
	vl := &VersionList{Product: domain.ProductID(def.ID), Refs: map[string]sources.ReleaseRef{}}
	srcs := def.SourcesWithRole(domain.RoleVersions)
	if len(srcs) == 0 {
		return vl, fmt.Errorf("%w: %s declares no %s source", ErrNoVersions, def.ID, domain.RoleVersions)
	}
	rc := catalog.RenderContext{Product: def.ID}
	now := i.now()
	for _, src := range srcs {
		st := domain.SourceStatus{SourceID: src.ID, Kind: src.Locator.Kind, Roles: src.Roles}
		if vl.Source != "" {
			st.State = domain.SourceSkipped
			st.Detail = fmt.Sprintf("not consulted: %s answered", vl.Source)
			vl.Sources = append(vl.Sources, st)
			continue
		}
		st.State, st.Detail = i.listFrom(ctx, def, parser, src, rc, now, vl, &st)
		vl.Sources = append(vl.Sources, st)
		if err := ctx.Err(); err != nil {
			return vl, err
		}
	}
	if vl.Source == "" {
		var why []string
		for _, s := range vl.Sources {
			why = append(why, fmt.Sprintf("%s: %s", s.SourceID, s.State))
		}
		return vl, fmt.Errorf("%w for %s (%s)", ErrNoVersions, def.ID, strings.Join(why, "; "))
	}
	return vl, nil
}

// listFrom consults one versions source; on success it fills vl.
func (i *Ingester) listFrom(ctx context.Context, def *catalog.ProductDefinition, parser domain.VersionParser, src catalog.Source, rc catalog.RenderContext, now time.Time, vl *VersionList, st *domain.SourceStatus) (domain.SourceState, string) {
	loc, err := renderLocator(src.Locator, rc)
	if err != nil {
		state, detail, _ := stateFor(err)
		if state != domain.SourceSkipped {
			state = domain.SourceError
		}
		return state, detail
	}
	st.URI = locatorURI(loc)
	lister, err := i.Registry.VersionLister(loc.Kind)
	if err != nil {
		state, detail, _ := stateFor(err)
		return state, detail
	}
	refs, err := lister.ListReleases(ctx, loc)
	if err != nil {
		state, detail, _ := stateFor(err)
		return state, detail
	}
	sel, err := selectReleases(def, parser, src, loc, refs, now)
	if err != nil {
		return domain.SourceError, err.Error()
	}
	if len(sel.versions) == 0 {
		return domain.SourceNotFound, fmt.Sprintf("%d tags listed, none is a release of %s (%s)", len(refs), def.ID, sel.summary())
	}
	vl.Source = src.ID
	vl.Versions = sel.versions
	vl.Refs = sel.refs
	vl.Evidence = sel.evidence
	return domain.SourceOK, fmt.Sprintf("%d releases from %d tags (%s)", len(sel.versions), len(refs), sel.summary())
}

type selection struct {
	versions                                    []domain.Version
	refs                                        map[string]sources.ReleaseRef
	evidence                                    []domain.Evidence
	mismatched, drafts, prereleases, duplicates int
}

func (s selection) summary() string {
	return fmt.Sprintf("ignored: %d non-matching, %d prereleases, %d drafts, %d duplicates",
		s.mismatched, s.prereleases, s.drafts, s.duplicates)
}

// selectReleases turns raw refs into canonical versions: tags must match the
// locator's TagPattern (when set) and parse with the product's version
// parser; drafts are dropped, prereleases too (semver suffix or source flag)
// unless the definition includes them; one tag is kept per semver, preferring
// the canonical tag. Every kept ref carries evidence.
func selectReleases(def *catalog.ProductDefinition, parser domain.VersionParser, src catalog.Source, loc catalog.Locator, refs []sources.ReleaseRef, now time.Time) (selection, error) {
	var locRe *regexp.Regexp
	if loc.TagPattern != "" {
		re, err := regexp.Compile(loc.TagPattern)
		if err != nil {
			return selection{}, fmt.Errorf("invalid tagPattern %q: %w", loc.TagPattern, err)
		}
		locRe = re
	}
	type cand struct {
		v   domain.Version
		ref sources.ReleaseRef
	}
	best := map[string]cand{}
	var sel selection
	for _, ref := range refs {
		tag := strings.TrimSpace(ref.Tag)
		if locRe != nil && !locRe.MatchString(tag) {
			sel.mismatched++
			continue
		}
		v, err := parser.Parse(tag)
		if err != nil && locRe != nil && locRe.SubexpIndex("version") > 0 {
			v, err = domain.VersionParser{Scheme: def.Versioning.Scheme, Pattern: locRe}.Parse(tag)
		}
		if err != nil {
			sel.mismatched++
			continue
		}
		if ref.Draft {
			sel.drafts++
			continue
		}
		if (v.IsPrerelease() || ref.Prerelease) && !def.Versioning.IncludePrereleases {
			sel.prereleases++
			continue
		}
		ref.Tag = tag
		if c, ok := best[v.Semver]; ok {
			sel.duplicates++
			if preferTag(def, v.Semver, tag, c.v.Tag) {
				best[v.Semver] = cand{v, ref}
			}
			continue
		}
		best[v.Semver] = cand{v, ref}
	}
	sel.refs = map[string]sources.ReleaseRef{}
	for semver, c := range best {
		sel.versions = append(sel.versions, c.v)
		ref := c.ref
		if ref.Evidence.ID == "" {
			ref.Evidence = refEvidence(src.ID, loc, ref, now)
		}
		sel.refs[semver] = ref
	}
	domain.SortVersions(sel.versions)
	for _, v := range sel.versions {
		sel.evidence = append(sel.evidence, sel.refs[v.Semver].Evidence)
	}
	return sel, nil
}

// preferTag reports whether tag a should replace tag b for the same semver:
// the canonical tag (TagPrefix + semver) wins, otherwise the smaller string.
func preferTag(def *catalog.ProductDefinition, semver, a, b string) bool {
	canon := def.TagFor(semver)
	if (a == canon) != (b == canon) {
		return a == canon
	}
	return a < b
}

// refEvidence builds evidence for a ref whose adapter did not supply any.
func refEvidence(sourceID string, loc catalog.Locator, ref sources.ReleaseRef, now time.Time) domain.Evidence {
	uri := ref.URL
	if uri == "" {
		uri = locatorURI(loc)
	}
	excerpt := ref.Tag
	if ref.Commit != "" {
		excerpt += " " + ref.Commit
	}
	return domain.NewEvidence(domain.EvidenceGitRef, sourceID, uri, "tag "+ref.Tag, excerpt, "", now)
}
