package gitsrc

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// Options configures the git adapters.
type Options struct {
	// CacheDir is the root of the cache; adapters write below <CacheDir>/git.
	// Empty means a "release-intelligence" directory inside os.UserCacheDir
	// (os.TempDir when that is unavailable).
	CacheDir string
	// Offline serves every network git operation from the cache and never
	// starts git against a remote. A request that is not cached fails with an
	// error wrapping fetch.ErrOffline. It has the same meaning as
	// fetch.ModeOffline; repo-file requests are served by the fetch.Client
	// passed to New, which must be configured consistently.
	Offline bool
	// Refresh ignores cached results of mutable requests and goes to the
	// network (like fetch.ModeRefresh). It has no effect when Offline is set.
	Refresh bool
	// TTL bounds the age of cached results of mutable requests (branches,
	// tag listings); zero means fetch.DefaultTTL. Results addressed by an
	// immutable ref never expire.
	TTL time.Duration
	// Runner executes git; nil means ExecRunner{}.
	Runner Runner
}

// Git is the shared core of the git adapters. Use Tags, Files, Dirs and Log
// to obtain the ports, or Register to wire them all into a registry.
type Git struct {
	fetch  fetch.Client
	opts   Options
	runner Runner
	root   string // <CacheDir>/git
	now    func() time.Time
	locks  keyedMutex
}

// New returns the git adapters. f is used for repo-file requests only; all
// git operations go through opts.Runner.
func New(f fetch.Client, opts Options) *Git {
	g := &Git{fetch: f, opts: opts, runner: opts.Runner, now: time.Now}
	if g.runner == nil {
		g.runner = ExecRunner{}
	}
	dir := opts.CacheDir
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil || base == "" {
			base = os.TempDir()
		}
		dir = filepath.Join(base, "release-intelligence")
	}
	g.root = filepath.Join(dir, "git")
	return g
}

// Register creates the git adapters and registers them under the locator
// kinds git-tags (versions), repo-file (documents), repo-dir (directories)
// and git-log (documents).
func Register(reg *sources.Registry, f fetch.Client, opts Options) {
	New(f, opts).Register(reg)
}

// Register wires the adapters of g into reg.
func (g *Git) Register(reg *sources.Registry) {
	reg.RegisterVersionLister(catalog.LocatorGitTags, g.Tags())
	reg.RegisterDocumentFetcher(catalog.LocatorRepoFile, g.Files())
	reg.RegisterDirectoryFetcher(catalog.LocatorRepoDir, g.Dirs())
	reg.RegisterDocumentFetcher(catalog.LocatorGitLog, g.Log())
}

// Tags returns the git-tags adapter.
func (g *Git) Tags() sources.VersionLister { return tagsAdapter{g} }

// Files returns the repo-file adapter.
func (g *Git) Files() sources.DocumentFetcher { return filesAdapter{g} }

// Dirs returns the repo-dir adapter.
func (g *Git) Dirs() sources.DirectoryFetcher { return dirsAdapter{g} }

// Log returns the git-log adapter.
func (g *Git) Log() sources.DocumentFetcher { return logAdapter{g} }

type (
	tagsAdapter  struct{ g *Git }
	filesAdapter struct{ g *Git }
	dirsAdapter  struct{ g *Git }
	logAdapter   struct{ g *Git }
)

func (a tagsAdapter) ListReleases(ctx context.Context, loc catalog.Locator) ([]sources.ReleaseRef, error) {
	return a.g.listTags(ctx, loc)
}

func (a filesAdapter) FetchDocument(ctx context.Context, loc catalog.Locator) (*sources.Document, error) {
	return a.g.fetchFile(ctx, loc)
}

func (a dirsAdapter) FetchDirectory(ctx context.Context, loc catalog.Locator) ([]sources.Document, error) {
	return a.g.fetchDir(ctx, loc)
}

func (a logAdapter) FetchDocument(ctx context.Context, loc catalog.Locator) (*sources.Document, error) {
	return a.g.fetchLog(ctx, loc)
}

// ttl returns the effective TTL for mutable cache entries.
func (g *Git) ttl() time.Duration {
	if g.opts.TTL > 0 {
		return g.opts.TTL
	}
	return fetch.DefaultTTL
}

// lookup returns the cached entry at path when it may be served now: always
// in offline mode, otherwise when it is immutable or younger than the TTL
// (and no refresh was requested).
func (g *Git) lookup(path string) (*entry, bool) {
	if g.opts.Refresh && !g.opts.Offline {
		return nil, false
	}
	e, ok := readEntry(path)
	if !ok {
		return nil, false
	}
	if g.opts.Offline || e.fresh(g.now(), g.ttl()) {
		return e, true
	}
	return nil, false
}

// run executes a git command and maps its failure onto the fetch sentinels.
// repoURL and op only decorate the error.
func (g *Git) run(ctx context.Context, repoURL, op string, c Command) ([]byte, error) {
	out, err := g.runner.Run(ctx, c)
	if err != nil {
		return nil, wrapErr(repoURL, op, err)
	}
	return out, nil
}

// inRepo builds a command that operates on the repository stored in dir.
func inRepo(dir string, args ...string) Command {
	return Command{Args: append([]string{"--git-dir", dir}, args...)}
}
