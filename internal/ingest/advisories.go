package ingest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// advisories consults the security sources in priority order. Sources of the
// same fallback group stop at the first ok; security sources whose kind has
// no AdvisorySource adapter (e.g. a bulletin page) are skipped. Advisories
// are de-duplicated by id and keep adapter order; copies of one id from
// several sources are merged by mergeAdvisory.
//
// A source that cannot be read (an error, throttling, an offline cache miss)
// is a status with that state and contributes nothing: it is never an empty
// list. The upgrade engine turns it into a warning and a security gap.
func (i *Ingester) advisories(ctx context.Context, def *catalog.ProductDefinition) ([]domain.Advisory, []domain.Evidence, []domain.SourceStatus, error) {
	if def == nil {
		return nil, nil, nil, errors.New("ingest: nil product definition")
	}
	if i.Registry == nil {
		return nil, nil, nil, errors.New("ingest: no source registry")
	}
	var (
		advs     []domain.Advisory
		statuses []domain.SourceStatus
		evs      domain.EvidenceSet
	)
	index := map[string]int{}        // advisory id → position in advs
	satisfied := map[string]string{} // fallback group → source that satisfied it
	rc := catalog.RenderContext{Product: def.ID}
	now := i.now()
	for _, src := range def.SourcesWithRole(domain.RoleSecurity) {
		st := domain.SourceStatus{SourceID: src.ID, Kind: src.Locator.Kind, Roles: src.Roles}
		group := fallbackGroup(src)
		if w, ok := satisfied[group]; ok && group != "" {
			st.State, st.Detail = domain.SourceSkipped, fmt.Sprintf("fallback group %s satisfied by %s", group, w)
			statuses = append(statuses, st)
			continue
		}
		loc, err := renderLocator(src.Locator, rc)
		if err != nil {
			st.State, st.Detail, _ = stateFor(err)
			statuses = append(statuses, st)
			continue
		}
		st.URI = locatorURI(loc)
		as, err := i.Registry.AdvisorySource(loc.Kind)
		if err != nil {
			st.State, st.Detail, _ = stateFor(err)
			statuses = append(statuses, st)
			continue
		}
		list, ev, err := as.ListAdvisories(ctx, loc)
		if err != nil {
			st.State, st.Detail, _ = stateFor(err)
			statuses = append(statuses, st)
			if ctx.Err() != nil {
				return nil, nil, nil, ctx.Err()
			}
			continue
		}
		for _, e := range ev {
			if e.ID != "" {
				evs.Add(e)
			}
		}
		added := 0
		for _, a := range list {
			if a.SourceID == "" {
				a.SourceID = src.ID
			}
			if len(a.Evidence) == 0 {
				uri := a.URL
				if uri == "" {
					uri = st.URI
				}
				e := domain.NewEvidence(domain.EvidenceAdvisory, src.ID, uri, a.ID, a.Summary, "", now)
				evs.Add(e)
				a.Evidence = []domain.EvidenceID{e.ID}
			}
			if j, ok := index[a.ID]; ok && a.ID != "" {
				advs[j] = mergeAdvisory(advs[j], a)
				continue
			}
			if a.ID != "" {
				index[a.ID] = len(advs)
			}
			a.Sources = []string{a.SourceID}
			advs = append(advs, a)
			added++
		}
		st.State = domain.SourceOK
		st.Detail = fmt.Sprintf("%d advisories", len(list))
		if added != len(list) {
			st.Detail += fmt.Sprintf(" (%d already listed by an earlier source)", len(list)-added)
		}
		statuses = append(statuses, st)
		if group != "" {
			satisfied[group] = src.ID
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	return advs, evs.List(), statuses, nil
}

// mergeAdvisory merges a later source's copy (next) of an advisory into the
// earlier one (prev). Evidence, aliases, patched versions and sources are
// united, so both copies stay cited. The affected range is chosen to fail
// closed:
//
//   - only one copy is machine-readable: that one (and its source becomes
//     SourceID), so a free-text or list-shaped copy listed first does not
//     degrade the advisory to a manual check;
//   - both are, and they differ: their union, so a version either copy calls
//     affected stays affected (and no "fixed" claim rests on the narrower
//     copy);
//   - neither is: no range.
//
// Severity keeps the higher of the two; UpdatedAt the later.
func mergeAdvisory(prev, next domain.Advisory) domain.Advisory {
	out := prev
	pv, nv := strings.TrimSpace(prev.Vulnerable), strings.TrimSpace(next.Vulnerable)
	switch {
	case pv == "" && nv != "":
		out.Vulnerable, out.SourceID = nv, next.SourceID
		out.Patched = unite(next.Patched, prev.Patched)
	case pv != "" && nv != "" && pv != nv:
		out.Vulnerable = pv + " || " + nv
		out.Patched = unite(prev.Patched, next.Patched)
	default:
		out.Patched = unite(prev.Patched, next.Patched)
	}
	out.Aliases = unite(prev.Aliases, next.Aliases)
	sort.Strings(out.Aliases)
	out.Sources = unite(prev.Sources, []string{next.SourceID})
	ev := append([]domain.EvidenceID(nil), prev.Evidence...)
	for _, id := range next.Evidence {
		if !slices.Contains(ev, id) {
			ev = append(ev, id)
		}
	}
	out.Evidence = ev
	if severityRank[strings.ToLower(next.Severity)] > severityRank[strings.ToLower(out.Severity)] {
		out.Severity = next.Severity
	}
	if out.Summary == "" {
		out.Summary = next.Summary
	}
	if out.URL == "" {
		out.URL = next.URL
	}
	if out.PublishedAt == nil {
		out.PublishedAt = next.PublishedAt
	}
	if next.UpdatedAt != nil && (out.UpdatedAt == nil || next.UpdatedAt.After(*out.UpdatedAt)) {
		out.UpdatedAt = next.UpdatedAt
	}
	return out
}

var severityRank = map[string]int{"low": 1, "medium": 2, "moderate": 2, "high": 3, "critical": 4}

// unite appends the elements of b missing from a (order kept, no
// duplicates, empty strings dropped).
func unite(a, b []string) []string {
	var out []string
	for _, s := range append(append([]string(nil), a...), b...) {
		if s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}
