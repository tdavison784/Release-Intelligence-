package ingest

import (
	"context"
	"fmt"
	"sort"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Sources following a pinned artifact's version.
//
// A source locator (or extract template) may reference another artifact's
// resolved version:
//
//	ref: '{{.ArtifactVersionOf "prometheus-operator-image"}}'
//
// This lets a product that AGGREGATES pinned components read the component's
// own release notes at its pinned tag — kube-prometheus-stack's operator
// notes at the appVersion pin, Flux's per-controller notes at their
// kustomization pins — without any product-specific code. The construct is
// the declarative mirror of version.strategy field/pattern: those read a pin
// out of the release's own files; this renders it into a source locator.
//
// Resolution order: sources and artifacts run in parallel, but the followed
// artifact's version depends only on the release itself (strategy template,
// field or pattern — validated so), so presolvePinned resolves exactly those
// artifacts first, and runArtifact reuses the cached resolution (one fetch,
// statuses and evidence recorded once). There is no circularity by
// construction: artifact version resolution never renders source templates.

// pinnedVersion is the outcome of pre-resolving one followed artifact's
// version for source templates: the version when state is ok, else why the
// following sources cannot render their locators.
type pinnedVersion struct {
	version string
	state   domain.SourceState
	detail  string
}

// presolvePinned resolves, before the parallel fan-out, the versions of every
// artifact a per-release source template follows. Failures are recorded per
// id and surface as the following sources' status; nothing here fails the
// ingestion.
func (i *Ingester) presolvePinned(ctx context.Context, r *run, groups []sourceGroup) {
	ids := map[string]bool{}
	for _, g := range groups {
		for _, s := range g.members {
			for _, id := range catalog.SourceArtifactRefs(s) {
				ids[id] = true
			}
		}
	}
	if len(ids) == 0 {
		return
	}
	r.pinned = map[string]pinnedVersion{}
	for _, id := range sortedIDs(ids) {
		r.pinned[id] = i.pinnedResolve(ctx, r, id)
	}
}

func sortedIDs(ids map[string]bool) []string {
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// pinnedResolve resolves one followed artifact's version, applying the same
// gates runArtifact applies (availability, exceptions) so a source is skipped
// exactly when the artifact it follows is not part of the release.
func (i *Ingester) pinnedResolve(ctx context.Context, r *run, id string) pinnedVersion {
	a, ok := r.def.Artifact(id)
	if !ok {
		// validated definitions cannot get here; in-memory definitions keep
		// working, with the source reporting the gap instead of crashing
		return pinnedVersion{state: domain.SourceError, detail: fmt.Sprintf("artifact %q is not defined", id)}
	}
	if reason, ex := catalog.ExceptionFor(r.v, a.Exceptions); ex {
		return pinnedVersion{state: domain.SourceSkipped, detail: "known exception: " + reason}
	}
	ok, err := catalog.AppliesTo(r.v, a.Availability, nil)
	if err != nil {
		return pinnedVersion{state: domain.SourceError, detail: fmt.Sprintf("availability %q: %v", a.Availability, err)}
	}
	if !ok {
		return pinnedVersion{state: domain.SourceSkipped, detail: notApplicableDetail(r.v, a.Availability, nil)}
	}
	res := i.resolutionFor(ctx, r, a)
	return pinnedOutcome(res)
}

// pinnedOutcome maps a version resolution to what a following source should
// report: the selected (latest) version when it resolved; otherwise the
// artifact's own reason, translated to a source state — the artifact's
// missing-ness is the source's not-found, an unreachable pin document is the
// source's unavailability, and so on.
func pinnedOutcome(res resolution) pinnedVersion {
	if len(res.versions) > 0 {
		// the selected (latest) version, mirroring captureContents
		return pinnedVersion{version: res.versions[len(res.versions)-1].version, state: domain.SourceOK}
	}
	switch res.status {
	case domain.ArtifactMissing:
		return pinnedVersion{state: domain.SourceNotFound, detail: res.detail}
	case domain.ArtifactNotApplicable:
		return pinnedVersion{state: domain.SourceSkipped, detail: res.detail}
	default:
		// ArtifactExpected: the resolution's last status distinguishes an
		// unreachable pin document (unavailable) from a malformed one (error)
		if len(res.statuses) > 0 {
			st := res.statuses[len(res.statuses)-1]
			return pinnedVersion{state: st.State, detail: res.detail}
		}
		return pinnedVersion{state: domain.SourceError, detail: res.detail}
	}
}

// sourceContext returns the render context for one source. Sources that
// follow pinned artifacts get their resolved versions attached; a follow that
// did not resolve is reported as the source's status (never a half-rendered
// locator). The bool is false with state/detail set when the source cannot be
// rendered at all.
func (r *run) sourceContext(src catalog.Source) (catalog.RenderContext, domain.SourceState, string, bool) {
	refs := catalog.SourceArtifactRefs(src)
	if len(refs) == 0 {
		return r.rc, domain.SourceOK, "", true
	}
	for _, id := range refs {
		p, ok := r.pinned[id]
		if !ok {
			// unreachable for validated definitions (persolvePinned covers
			// every per-release source's references)
			return r.rc, domain.SourceError, fmt.Sprintf("follows artifact %s, which was not resolved for this release", id), false
		}
		if p.state != domain.SourceOK {
			return r.rc, p.state, fmt.Sprintf("cannot follow artifact %s: %s", id, p.detail), false
		}
	}
	versions := map[string]string{}
	for id, p := range r.pinned {
		if p.state == domain.SourceOK {
			versions[id] = p.version
		}
	}
	return r.rc.WithArtifactVersions(versions), domain.SourceOK, "", true
}
