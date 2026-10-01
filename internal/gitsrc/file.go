package gitsrc

import (
	"context"
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// defaultRef is used when a repo-file locator carries no ref. It is mutable.
const defaultRef = "HEAD"

// fetchFile implements the repo-file locator kind: one file of a repository
// at a ref, read from the host's raw content endpoint through the
// fetch.Client (so it is cached and replayable like every other HTTP read).
//
// Requests whose ref is immutable (see IsImmutableRef) are marked
// fetch.Request.Immutable and cached forever; branch refs use Options.TTL (the
// fetch client's default TTL when zero).
// Hosts without a known raw endpoint fail with an error wrapping
// fetch.ErrUnavailable.
func (g *Git) fetchFile(ctx context.Context, loc catalog.Locator) (*sources.Document, error) {
	repo, err := ParseRepo(loc.Repository)
	if err != nil {
		return nil, fmt.Errorf("repo-file: %w", err)
	}
	p, err := cleanRepoPath(loc.Path)
	if err != nil {
		return nil, fmt.Errorf("repo-file %s: %w", repo, err)
	}
	if p == "" {
		return nil, fmt.Errorf("repo-file %s: path is required", repo)
	}
	ref := loc.Ref
	if ref == "" {
		ref = defaultRef
	}
	if err := checkRef(ref); err != nil {
		return nil, fmt.Errorf("repo-file %s: %w", repo, err)
	}
	raw, err := repo.RawURL(ref, p)
	if err != nil {
		return nil, err
	}
	doc, err := g.fetch.Do(ctx, fetch.Request{URL: raw, Immutable: IsImmutableRef(ref), TTL: g.opts.TTL})
	if err != nil {
		return nil, err
	}
	return &sources.Document{
		Locator:     loc,
		URI:         repo.BlobURL(ref, p),
		FetchURL:    raw,
		Path:        p,
		Content:     doc.Body,
		Format:      FormatFromPath(p),
		Digest:      doc.Digest,
		RetrievedAt: doc.RetrievedAt,
	}, nil
}
