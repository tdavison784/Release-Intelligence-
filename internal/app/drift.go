package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/drift"
	"github.com/tdavison784/release-intelligence/internal/ingest"
)

// DefaultDriftBaselineDir is where onboarding saves `ri check -o json`
// reports; drift uses them as the "previously held" baseline when present.
const DefaultDriftBaselineDir = "docs/onboarding/checks"

// DriftOptions tunes Drift.
type DriftOptions struct {
	// N is how many of the newest releases NEWER than the baseline cutoff to
	// re-validate (default 3).
	N int
	// Versions checks these explicit releases instead of the newest N.
	Versions []string
	// BaselinePath loads the baseline from this saved `ri check -o json`
	// report. "" uses docs/onboarding/checks/<id>.json when it exists; "-"
	// forces no saved report (the definition's validatedAgainst lists are the
	// baseline then).
	BaselinePath string
}

// Drift re-validates the definition's declared relationships against the
// newest upstream releases and reports structured differences (see
// internal/drift). It never writes to products/.
func (a *App) Drift(ctx context.Context, productID string, opts DriftOptions) (*drift.Report, error) {
	def, err := a.Product(productID)
	if err != nil {
		return nil, err
	}
	vl, err := a.Ingester.ListVersions(ctx, def)
	if err != nil {
		if errors.Is(err, ingest.ErrNoVersions) {
			// The canonical channel itself is broken (unreachable host, moved
			// repository, changed tag convention): that is a drift finding,
			// not a crash. VersionListFailed turns the source statuses into a
			// report; unreachable stays unverifiable.
			return drift.VersionListFailed(def, vl, a.now()), nil
		}
		return nil, err
	}

	base, basePath, err := a.loadDriftBaseline(def, opts.BaselinePath)
	if err != nil {
		return nil, err
	}
	cutoff := drift.BaselineCutoff(def, base)

	var rels []domain.Version
	if len(opts.Versions) > 0 {
		for _, s := range opts.Versions {
			v, err := ResolveVersion(vl, s)
			if err != nil {
				return nil, err
			}
			rels = append(rels, v)
		}
	} else {
		rels = drift.SelectReleases(vl.Versions, cutoff, opts.N)
	}

	if len(rels) == 0 {
		// Nothing newer than the baseline: an empty report says so instead of
		// failing, so a cron job can distinguish "up to date" from "broken".
		return drift.Analyze(ctx, drift.Input{
			Definition:   def,
			Versions:     vl,
			Current:      &ingest.RelationshipReport{Product: domain.ProductID(def.ID)},
			Releases:     nil,
			Baseline:     base,
			BaselinePath: basePath,
			Registry:     a.Registry,
			Now:          a.now(),
		})
	}

	cur, releases, err := a.Ingester.IngestChecked(ctx, def, rels, vl)
	if err != nil {
		return nil, fmt.Errorf("drift %s: %w", def.ID, err)
	}
	return drift.Analyze(ctx, drift.Input{
		Definition:   def,
		Versions:     vl,
		Current:      cur,
		Releases:     releases,
		Baseline:     base,
		BaselinePath: basePath,
		Registry:     a.Registry,
		Now:          a.now(),
	})
}

// loadDriftBaseline loads the saved check report to compare against. path ""
// falls back to DefaultDriftBaselineDir/<id>.json when it exists; path "-"
// means no saved report.
func (a *App) loadDriftBaseline(def *catalog.ProductDefinition, path string) (*ingest.RelationshipReport, string, error) {
	if path == "-" {
		return nil, "", nil
	}
	if path == "" {
		candidate := filepath.Join(DefaultDriftBaselineDir, def.ID+".json")
		if _, err := os.Stat(candidate); err != nil {
			return nil, "", nil
		}
		path = candidate
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var rep ingest.RelationshipReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, "", fmt.Errorf("%s: not a relationship report: %w", path, err)
	}
	if len(rep.Summary) == 0 && len(rep.Checks) == 0 {
		return nil, "", fmt.Errorf("%s: report has no summary and no checks", path)
	}
	if rep.Product != "" && rep.Product != domain.ProductID(def.ID) {
		return nil, "", fmt.Errorf("%s: report is for product %q, not %q", path, rep.Product, def.ID)
	}
	return &rep, path, nil
}
