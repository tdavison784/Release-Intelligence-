package github

import (
	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// Register creates a Client reading through f (authenticated with token when
// it is non-empty) and registers the GitHub adapters:
//
//	github-releases    VersionLister and DocumentFetcher
//	github-advisories  AdvisorySource
//
// Pass options such as WithBaseURL to point the adapters at GitHub
// Enterprise Server or at a test server.
func Register(reg *sources.Registry, f fetch.Client, token string, opts ...Option) {
	c := NewClient(f, token, opts...)
	releases := NewReleases(c)
	reg.RegisterVersionLister(catalog.LocatorGitHubReleases, releases)
	reg.RegisterDocumentFetcher(catalog.LocatorGitHubReleases, releases)
	reg.RegisterAdvisorySource(catalog.LocatorGitHubAdvisories, NewAdvisories(c))
}
