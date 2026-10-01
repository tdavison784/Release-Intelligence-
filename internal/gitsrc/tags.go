package gitsrc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// tagRef is one tag as advertised by the remote.
type tagRef struct {
	Name string
	// Commit is the commit the tag points to: the peeled "^{}" object of an
	// annotated tag, else the object the ref itself names.
	Commit string
}

// parseLsRemote parses the output of "git ls-remote --tags": lines of
// "<sha>\t<refname>". Only refs/tags/* are returned, each tag once, sorted by
// name. For annotated tags the peeled "<ref>^{}" line supplies the commit.
func parseLsRemote(out []byte) ([]tagRef, error) {
	const prefix = "refs/tags/"
	commits := map[string]string{}
	peeled := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		sha, ref, ok := strings.Cut(line, "\t")
		if !ok || sha == "" || ref == "" {
			return nil, fmt.Errorf("malformed ls-remote line %q", line)
		}
		if !strings.HasPrefix(ref, prefix) {
			continue
		}
		name := strings.TrimPrefix(ref, prefix)
		if base, found := strings.CutSuffix(name, "^{}"); found {
			commits[base] = sha
			peeled[base] = true
			continue
		}
		if !peeled[name] {
			commits[name] = sha
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	tags := make([]tagRef, 0, len(commits))
	for name, sha := range commits {
		tags = append(tags, tagRef{Name: name, Commit: sha})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })
	return tags, nil
}

// tagsPayload is the cached form of an ls-remote result.
type tagsPayload struct {
	Output string `json:"output"`
}

// listTags implements the git-tags locator kind.
func (g *Git) listTags(ctx context.Context, loc catalog.Locator) ([]sources.ReleaseRef, error) {
	repo, err := ParseRepo(loc.Repository)
	if err != nil {
		return nil, fmt.Errorf("git-tags: %w", err)
	}
	var filter *regexp.Regexp
	if loc.TagPattern != "" {
		if filter, err = regexp.Compile(loc.TagPattern); err != nil {
			return nil, fmt.Errorf("git-tags %s: invalid tagPattern %q: %w", repo, loc.TagPattern, err)
		}
	}
	out, retrievedAt, err := g.lsRemoteTags(ctx, repo)
	if err != nil {
		return nil, err
	}
	tags, err := parseLsRemote(out)
	if err != nil {
		return nil, fmt.Errorf("git-tags %s: %w", repo, err)
	}
	refs := make([]sources.ReleaseRef, 0, len(tags))
	for _, t := range tags {
		if filter != nil && !filter.MatchString(t.Name) {
			continue
		}
		line := t.Commit + " refs/tags/" + t.Name
		uri := repo.TagURL(t.Name)
		ev := domain.NewEvidence(domain.EvidenceGitRef, "", uri, "refs/tags/"+t.Name, line,
			domain.Digest([]byte(line)), retrievedAt)
		refs = append(refs, sources.ReleaseRef{
			Tag:      t.Name,
			Commit:   t.Commit,
			URL:      uri,
			Evidence: ev,
		})
	}
	return refs, nil
}

// lsRemoteTags returns the raw "git ls-remote --tags" output of the
// repository together with the time it was obtained, from the cache when
// possible. Tag listings are mutable and expire after the TTL.
func (g *Git) lsRemoteTags(ctx context.Context, repo Repo) ([]byte, time.Time, error) {
	path := filepath.Join(g.root, "tags", repo.slug()+".json.gz")
	if e, ok := g.lookup(path); ok {
		var p tagsPayload
		if err := json.Unmarshal(e.Payload, &p); err == nil {
			return []byte(p.Output), e.RetrievedAt, nil
		}
	}
	if g.opts.Offline {
		return nil, time.Time{}, offlineErr(repo.CloneURL, "git ls-remote --tags is not cached")
	}
	out, err := g.run(ctx, repo.CloneURL, "git ls-remote --tags", Command{
		Args: []string{"ls-remote", "--tags", "--", repo.CloneURL},
	})
	if err != nil {
		return nil, time.Time{}, err
	}
	now := g.now().UTC()
	payload, _ := json.Marshal(tagsPayload{Output: string(out)})
	// A failing cache write must not fail the request: the result is still
	// valid, only its replay is lost.
	_ = writeEntry(path, &entry{
		Kind: "tags", Repo: repo.CloneURL, Request: "ls-remote --tags",
		RetrievedAt: now, Payload: payload,
	})
	return out, now, nil
}
