package app

import (
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/github"
	"github.com/tdavison784/release-intelligence/internal/gitsrc"
	"github.com/tdavison784/release-intelligence/internal/helm"
	"github.com/tdavison784/release-intelligence/internal/httpsrc"
	"github.com/tdavison784/release-intelligence/internal/oci"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// registerAdapters is the single place that binds locator kinds to concrete
// adapters. Adding a hosting platform or registry means registering another
// adapter here; nothing else in the pipeline changes.
func registerAdapters(reg *sources.Registry, f fetch.Client, cfg Config, cacheDir string) error {
	git := gitsrc.New(f, gitsrc.Options{CacheDir: cacheDir, Offline: cfg.Offline, Refresh: cfg.Refresh})
	git.Register(reg)        // git-tags, repo-file, repo-dir, git-log
	httpsrc.Register(reg, f) // http
	// The GitHub REST API also works unauthenticated for public repositories
	// (rate limited); a token raises the limit.
	github.Register(reg, f, cfg.GitHubToken)       // github-releases, github-advisories
	oci.Register(reg, f)                           // oci
	helm.Register(reg, f, git.Tags(), git.Files()) // helm-repo, helm-git
	return nil
}
