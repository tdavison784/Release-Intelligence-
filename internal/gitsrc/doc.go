// Package gitsrc implements the git-based, host-agnostic source adapters of
// the release-intelligence pipeline:
//
//	git-tags   sources.VersionLister     tags of a repository (git ls-remote)
//	repo-file  sources.DocumentFetcher   one file at a ref (raw content hosts)
//	repo-dir   sources.DirectoryFetcher  every file in a directory at a ref,
//	                                     optionally only those added since a base ref
//	git-log    sources.DocumentFetcher   commit subjects of a range A..B as markdown
//
// # Reproducibility
//
// Every network git operation is cached under <CacheDir>/git so that a run
// made online can be replayed later with Options.Offline set:
//
//	git/tags/<slug>.json.gz                 ls-remote output
//	git/log/<slug>/<key>.json.gz            rendered git-log documents
//	git/dir/<slug>/<key>.json.gz            directory listings with file contents
//	git/mirrors/<slug>.git                  treeless bare mirror (git-log accelerator)
//	git/work/<slug>.git                     shallow blobless repo (repo-dir accelerator)
//
// The mirror and work repositories are accelerators for online runs only.
// Offline replay is served exclusively from the *.json.gz result files, which
// makes its behaviour independent of what happens to be in a clone.
// Results addressed by an immutable ref (a commit SHA or a version tag, see
// IsImmutableRef) never expire; results addressed by a branch expire after
// Options.TTL (fetch.DefaultTTL when zero). repo-file goes through the
// injected fetch.Client and therefore follows that client's own cache and
// mode; callers that replay offline should configure both consistently.
//
// # Errors
//
// Failures are mapped onto the sentinels of package fetch so callers can use
// fetch.StateFor: an unreachable host, a missing git binary, a timeout or an
// authentication failure wrap fetch.ErrUnavailable; a missing ref or
// repository wraps fetch.ErrNotFound; an uncached request in offline mode
// wraps fetch.ErrOffline. Anything else is returned as a plain error.
package gitsrc
