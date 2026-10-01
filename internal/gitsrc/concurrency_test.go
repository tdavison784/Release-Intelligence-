package gitsrc

import (
	"context"
	"sync"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
)

// Requests for the same repository are serialised, so concurrent callers (an
// ingestion that fans out over releases) must neither fail nor corrupt the
// shared work repository.
func TestConcurrentFetchesShareOneWorkRepository(t *testing.T) {
	r := istioLikeRepo(t)
	g := newTestGit(t, t.TempDir(), nil, nil, nil, Options{})
	locs := []catalog.Locator{
		dirLocator(r, "1.0.0", "", ""),
		dirLocator(r, "1.1.0", "", ""),
		dirLocator(r, "1.1.0", "1.0.0", ""),
		dirLocator(r, "main", "1.0.0", "*.yaml"),
		dirLocator(r, "1.1.0", "", "*.yaml"),
	}
	var wg sync.WaitGroup
	errs := make(chan error, 3*len(locs))
	for round := 0; round < 3; round++ {
		for _, loc := range locs {
			wg.Add(1)
			go func(loc catalog.Locator) {
				defer wg.Done()
				if _, err := g.Dirs().FetchDirectory(context.Background(), loc); err != nil {
					errs <- err
				}
			}(loc)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestConcurrentLogRequestsShareOneMirror(t *testing.T) {
	r := newTestRepo(t)
	r.commit("one", map[string]string{"a": "1"})
	r.git("tag", "v1.0.0")
	r.commit("two", map[string]string{"a": "2"})
	r.git("tag", "v1.1.0")
	r.commit("three", map[string]string{"a": "3"})
	r.git("tag", "v1.2.0")

	g := newTestGit(t, t.TempDir(), nil, nil, nil, Options{})
	ranges := []string{"v1.0.0..v1.1.0", "v1.1.0..v1.2.0", "v1.0.0..v1.2.0"}
	var wg sync.WaitGroup
	errs := make(chan error, 3*len(ranges))
	for round := 0; round < 3; round++ {
		for _, rng := range ranges {
			wg.Add(1)
			go func(rng string) {
				defer wg.Done()
				_, err := g.Log().FetchDocument(context.Background(),
					catalog.Locator{Kind: catalog.LocatorGitLog, Repository: r.URL, Ref: rng})
				if err != nil {
					errs <- err
				}
			}(rng)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
