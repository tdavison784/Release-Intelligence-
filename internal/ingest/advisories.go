package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// advisories consults the security sources in priority order. Sources of the
// same fallback group stop at the first ok; security sources whose kind has
// no AdvisorySource adapter (e.g. a bulletin page) are skipped. Advisories
// are de-duplicated by id (first source wins) and keep adapter order.
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
	seen := map[string]bool{}
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
			if a.ID != "" && seen[a.ID] {
				continue
			}
			seen[a.ID] = true
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
