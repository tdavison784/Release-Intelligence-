// Package app wires the catalog, fetch cache, source adapters, ingestion
// pipeline, store and upgrade engine together into the use cases exposed by
// the CLI. It contains orchestration only; domain logic lives elsewhere.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/ingest"
	"github.com/tdavison784/release-intelligence/internal/sources"
	"github.com/tdavison784/release-intelligence/internal/store"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// Config configures an App.
type Config struct {
	ProductsDir string // product definitions (default "products")
	StateDir    string // local state root (default ".ri"): cache/ and store/ live below it
	Offline     bool   // serve only from cache
	Refresh     bool   // bypass caches (network + stored releases)
	GitHubToken string // optional; enables the GitHub API adapters
	// LLMCacheDir overrides the enrichment answer cache location (default
	// StateDir/llm-cache); used by replay fixtures and `ri eval -enriched`.
	LLMCacheDir string
	Logf        func(format string, args ...any)
	// Now is the clock used for the ingester and for the edge's generation
	// time (default time.Now). Tests and golden runs inject a fixed clock so
	// that outputs are byte-for-byte reproducible.
	Now func() time.Time
}

// App is the composition root.
type App struct {
	cfg      Config
	Catalog  *catalog.Catalog
	Fetch    fetch.Client
	Registry *sources.Registry
	Ingester *ingest.Ingester
	Store    *store.Store
	now      func() time.Time
}

// New builds an App from cfg.
func New(cfg Config) (*App, error) {
	if cfg.ProductsDir == "" {
		cfg.ProductsDir = "products"
	}
	if cfg.StateDir == "" {
		cfg.StateDir = ".ri"
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cat, err := catalog.LoadDir(cfg.ProductsDir)
	if err != nil {
		return nil, fmt.Errorf("load product definitions: %w", err)
	}
	// A malformed product file does not stop the others from loading; it is
	// logged here and named by Product when its id is requested.
	loadErrs := cat.LoadErrors()
	for _, p := range cat.LoadErrorPaths() {
		cfg.Logf("product definition %s was not loaded: %v", p, loadErrs[p])
	}
	mode := fetch.ModeOnline
	switch {
	case cfg.Offline:
		mode = fetch.ModeOffline
	case cfg.Refresh:
		mode = fetch.ModeRefresh
	}
	cacheDir := filepath.Join(cfg.StateDir, "cache")
	fc := fetch.NewHTTPClient(fetch.NewCache(cacheDir), mode)
	reg := sources.NewRegistry()
	if err := registerAdapters(reg, fc, cfg, cacheDir); err != nil {
		return nil, err
	}
	a := &App{
		cfg:      cfg,
		Catalog:  cat,
		Fetch:    fc,
		Registry: reg,
		Ingester: ingest.New(reg),
		Store:    store.New(filepath.Join(cfg.StateDir, "store")),
		now:      cfg.Now,
	}
	a.Ingester.Clock = cfg.Now
	return a, nil
}

// Product returns a validated product definition.
func (a *App) Product(id string) (*catalog.ProductDefinition, error) {
	d, ok := a.Catalog.Get(id)
	if !ok {
		var ids []string
		for _, p := range a.Catalog.List() {
			ids = append(ids, p.ID)
		}
		msg := fmt.Sprintf("unknown product %q (known: %s)", id, strings.Join(ids, ", "))
		if errs := a.Catalog.LoadErrors(); len(errs) > 0 {
			var failed []string
			for _, p := range a.Catalog.LoadErrorPaths() {
				failed = append(failed, fmt.Sprintf("%s: %v", p, errs[p]))
			}
			msg += fmt.Sprintf("; %d product file(s) failed to load, one of them may define it: %s", len(failed), strings.Join(failed, "; "))
		}
		return nil, errors.New(msg)
	}
	if rep := catalog.Validate(d); !rep.OK() {
		return nil, fmt.Errorf("product definition %s is invalid: %v", d.Path(), rep.Errors())
	}
	return d, nil
}

// Versions lists the product's canonical versions.
func (a *App) Versions(ctx context.Context, def *catalog.ProductDefinition) (*ingest.VersionList, error) {
	vl, err := a.Ingester.ListVersions(ctx, def)
	if err != nil {
		return nil, err
	}
	if len(vl.Versions) == 0 {
		return vl, fmt.Errorf("no versions found for %s (sources: %s)", def.ID, describeStatuses(vl.Sources))
	}
	return vl, nil
}

// ResolveVersion finds a version by tag or semver ("v1.18.0" or "1.18.0").
func ResolveVersion(vl *ingest.VersionList, s string) (domain.Version, error) {
	s = strings.TrimSpace(s)
	for _, v := range vl.Versions {
		if v.Tag == s || v.Semver == s || v.Semver == strings.TrimPrefix(s, "v") {
			return v, nil
		}
	}
	return domain.Version{}, fmt.Errorf("version %q is not a known release of %s", s, vl.Product)
}

// Release ingests one release and records the result in the store.
//
// Ingestion is always re-run: it is deterministic and served from the fetch
// cache, so it is cheap, and re-running guarantees that results reflect the
// current code as well as the current definition. The store is an output
// record (inspectable JSON), not a cache.
func (a *App) Release(ctx context.Context, def *catalog.ProductDefinition, v domain.Version, vl *ingest.VersionList) (*domain.Release, error) {
	a.cfg.Logf("ingesting %s %s", def.ID, v)
	r, err := a.Ingester.IngestRelease(ctx, def, v, vl)
	if err != nil {
		return nil, fmt.Errorf("ingest %s %s: %w", def.ID, v, err)
	}
	if err := a.Store.SaveRelease(r); err != nil {
		a.cfg.Logf("warning: could not store release: %v", err)
	}
	return r, nil
}

// UpgradeOptions tunes Upgrade.
type UpgradeOptions struct {
	// Policy overrides the path policy ("" = from the definition's lineage).
	Policy string
}

// Upgrade builds the UpgradeEdge for product from → to.
func (a *App) Upgrade(ctx context.Context, productID, from, to string, opts UpgradeOptions) (*domain.UpgradeEdge, error) {
	def, err := a.Product(productID)
	if err != nil {
		return nil, err
	}
	vl, err := a.Versions(ctx, def)
	if err != nil {
		return nil, err
	}
	fv, err := ResolveVersion(vl, from)
	if err != nil {
		return nil, err
	}
	tv, err := ResolveVersion(vl, to)
	if err != nil {
		return nil, err
	}
	lineage := def.Versioning.Lineage
	switch opts.Policy {
	case "":
	case upgrade.PolicyAll:
		lineage = catalog.LineageLinear
	case upgrade.PolicyMinorLineage:
		lineage = catalog.LineageMinor
	default:
		return nil, fmt.Errorf("unknown path policy %q", opts.Policy)
	}
	sel, err := upgrade.SelectPath(vl.Versions, fv, tv, lineage)
	if err != nil {
		return nil, err
	}
	fromRel, err := a.Release(ctx, def, fv, vl)
	if err != nil {
		return nil, err
	}
	path := make([]*domain.Release, 0, len(sel.Path))
	for _, v := range sel.Path {
		r, err := a.Release(ctx, def, v, vl)
		if err != nil {
			return nil, err
		}
		path = append(path, r)
	}
	toRel := path[len(path)-1]
	advs, advEv, advStatus, err := a.Ingester.Advisories(ctx, def)
	if err != nil {
		a.cfg.Logf("warning: advisories: %v", err)
	}
	extra := append([]domain.SourceStatus{}, vl.Sources...)
	extra = append(extra, advStatus...)
	edge, err := upgrade.Build(upgrade.Input{
		Definition:       def,
		From:             fromRel,
		To:               toRel,
		Path:             path,
		Selection:        sel,
		Advisories:       advs,
		AdvisoryEvidence: advEv,
		ExtraSources:     extra,
		Now:              a.now().UTC(),
	})
	if err != nil {
		return nil, err
	}
	if err := a.Store.SaveEdge(edge); err != nil {
		a.cfg.Logf("warning: could not store edge: %v", err)
	}
	return edge, nil
}

// CheckRelationships validates the definition against historical releases:
// either the given versions, or the latest release of each of the most
// recent n lines.
func (a *App) CheckRelationships(ctx context.Context, productID string, versions []string, n int) (*ingest.RelationshipReport, error) {
	def, err := a.Product(productID)
	if err != nil {
		return nil, err
	}
	vl, err := a.Versions(ctx, def)
	if err != nil {
		return nil, err
	}
	var rels []domain.Version
	if len(versions) > 0 {
		for _, s := range versions {
			v, err := ResolveVersion(vl, s)
			if err != nil {
				return nil, err
			}
			rels = append(rels, v)
		}
	} else {
		rels = SampleReleases(vl.Versions, n)
	}
	return a.Ingester.CheckRelationships(ctx, def, rels, vl)
}

// SampleReleases picks the historical releases used for relationship
// validation: for each of the n most recent lines, the line's first release
// (X.Y.0, which exercises minor-only sources such as upgrade guides) and its
// latest patch.
func SampleReleases(vs []domain.Version, n int) []domain.Version {
	latest := LatestPerLine(vs, n)
	lines := map[string]bool{}
	for _, v := range latest {
		lines[v.Line()] = true
	}
	first := map[string]domain.Version{}
	for _, v := range vs {
		if !lines[v.Line()] {
			continue
		}
		if cur, ok := first[v.Line()]; !ok || v.Less(cur) {
			first[v.Line()] = v
		}
	}
	seen := map[string]bool{}
	var out []domain.Version
	for _, v := range latest {
		for _, c := range []domain.Version{first[v.Line()], v} {
			if !seen[c.Semver] {
				seen[c.Semver] = true
				out = append(out, c)
			}
		}
	}
	domain.SortVersions(out)
	return out
}

// LatestPerLine returns the newest release of each of the n most recent
// release lines, ascending.
func LatestPerLine(vs []domain.Version, n int) []domain.Version {
	if n <= 0 {
		n = ingest.MinValidations
	}
	byLine := map[string]domain.Version{}
	var lines []string
	for _, v := range vs {
		cur, ok := byLine[v.Line()]
		if !ok {
			lines = append(lines, v.Line())
		}
		if !ok || cur.Less(v) {
			byLine[v.Line()] = v
		}
	}
	var out []domain.Version
	for _, l := range lines {
		out = append(out, byLine[l])
	}
	domain.SortVersions(out)
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

func describeStatuses(ss []domain.SourceStatus) string {
	var parts []string
	for _, s := range ss {
		p := fmt.Sprintf("%s=%s", s.SourceID, s.State)
		if s.Detail != "" {
			p += " (" + s.Detail + ")"
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "; ")
}

// GitHubTokenFromEnv returns GITHUB_TOKEN or GH_TOKEN.
func GitHubTokenFromEnv() string {
	for _, k := range []string{"RI_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// ErrUsage marks invalid CLI usage.
var ErrUsage = errors.New("usage")
