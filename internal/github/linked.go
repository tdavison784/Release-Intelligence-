package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Bounds on what one linked item reads. The PR description is cut at
// MaxLinkedBody bytes and the changed-file list at MaxLinkedFiles names (one
// API page): the point is to give a reviewer the context a one-line note
// lacks, not to mirror the PR.
const (
	MaxLinkedBody  = 8000
	MaxLinkedFiles = 100
)

// Linked item kinds.
const (
	LinkedPullRequest = "pull-request"
	LinkedIssue       = "issue"
	LinkedCommit      = "commit"
)

// LinkedFile is one changed file of a pull request or commit.
type LinkedFile struct {
	Name      string
	Status    string // added | modified | removed | renamed | …
	Additions int
	Deletions int
}

// LinkedItem is what GitHub states about a pull request, issue or commit that
// a release note references: title, description and (for PRs and commits) the
// changed files.
type LinkedItem struct {
	Kind        string // LinkedPullRequest | LinkedIssue | LinkedCommit
	Owner, Repo string
	Number      int    // PR / issue number (0 for a commit)
	SHA         string // full commit id (commits only)
	URL         string // the human-readable page
	Title       string
	Body        string // description (commit: the message after the subject), bounded
	State       string // open | closed ("" for commits)
	Labels      []string
	Files       []LinkedFile
	// FilesKnown is the total number of changed files when the API says so
	// (0 when it does not); Files may hold fewer.
	FilesKnown    int
	BodyTruncated bool
	// Digest identifies exactly the content shown (title, state, labels,
	// body, files), not volatile API fields, so re-fetching an unchanged item
	// yields the same evidence id.
	Digest string
}

// LinkedItem fetches one referenced item through the cached fetch client.
// kind is the reference type ("pull-request", "issue", "commit", or the
// ambiguous "github-ref" for a bare #N, which GitHub's issues endpoint
// resolves to an issue or a pull request). A missing item is
// fetch.ErrNotFound; rate limiting is fetch.ErrThrottled.
func (c *Client) LinkedItem(ctx context.Context, owner, repo, kind string, number int, sha string) (*LinkedItem, error) {
	if !validRepoPart(owner) || !validRepoPart(repo) {
		return nil, fmt.Errorf("github: invalid repository %s/%s", owner, repo)
	}
	if kind == LinkedCommit {
		return c.linkedCommit(ctx, owner, repo, sha)
	}
	if number <= 0 {
		return nil, fmt.Errorf("github: invalid issue/pull number %d", number)
	}
	return c.linkedIssueOrPull(ctx, owner, repo, number)
}

var shaRe = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

func (c *Client) linkedCommit(ctx context.Context, owner, repo, sha string) (*LinkedItem, error) {
	if !shaRe.MatchString(sha) {
		return nil, fmt.Errorf("github: invalid commit id %q", sha)
	}
	doc, err := c.get(ctx, c.endpoint("/repos/"+owner+"/"+repo+"/commits/"+strings.ToLower(sha), nil))
	if err != nil {
		return nil, err
	}
	var raw struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message string `json:"message"`
		} `json:"commit"`
		Files []struct {
			Filename  string `json:"filename"`
			Status    string `json:"status"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		} `json:"files"`
	}
	if err := json.Unmarshal(doc.Body, &raw); err != nil {
		return nil, fmt.Errorf("github: decode commit %s/%s@%s: %w", owner, repo, sha, err)
	}
	subject, body, _ := strings.Cut(strings.ReplaceAll(raw.Commit.Message, "\r\n", "\n"), "\n")
	it := &LinkedItem{Kind: LinkedCommit, Owner: owner, Repo: repo, SHA: raw.SHA, URL: raw.HTMLURL, Title: strings.TrimSpace(subject)}
	if it.URL == "" {
		it.URL = fmt.Sprintf("https://github.com/%s/%s/commit/%s", owner, repo, raw.SHA)
	}
	it.Body, it.BodyTruncated = boundBody(body)
	for i, f := range raw.Files {
		if i == MaxLinkedFiles {
			break
		}
		it.Files = append(it.Files, LinkedFile{Name: f.Filename, Status: f.Status, Additions: f.Additions, Deletions: f.Deletions})
	}
	it.FilesKnown = len(raw.Files)
	it.Digest = it.contentDigest()
	return it, nil
}

func (c *Client) linkedIssueOrPull(ctx context.Context, owner, repo string, n int) (*LinkedItem, error) {
	doc, err := c.get(ctx, c.endpoint(fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, n), nil))
	if err != nil {
		return nil, err
	}
	var raw struct {
		HTMLURL string `json:"html_url"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		State   string `json:"state"`
		Labels  []struct {
			Name string `json:"name"`
		} `json:"labels"`
		PullRequest *json.RawMessage `json:"pull_request"`
	}
	if err := json.Unmarshal(doc.Body, &raw); err != nil {
		return nil, fmt.Errorf("github: decode %s/%s#%d: %w", owner, repo, n, err)
	}
	it := &LinkedItem{Kind: LinkedIssue, Owner: owner, Repo: repo, Number: n, URL: raw.HTMLURL, Title: strings.TrimSpace(raw.Title), State: raw.State}
	if raw.PullRequest != nil {
		it.Kind = LinkedPullRequest
	}
	if it.URL == "" {
		seg := "issues"
		if it.Kind == LinkedPullRequest {
			seg = "pull"
		}
		it.URL = fmt.Sprintf("https://github.com/%s/%s/%s/%d", owner, repo, seg, n)
	}
	for _, l := range raw.Labels {
		it.Labels = append(it.Labels, l.Name)
	}
	it.Body, it.BodyTruncated = boundBody(strings.ReplaceAll(raw.Body, "\r\n", "\n"))
	if it.Kind == LinkedPullRequest {
		files, err := c.get(ctx, c.endpoint(fmt.Sprintf("/repos/%s/%s/pulls/%d/files", owner, repo, n), url.Values{"per_page": {fmt.Sprint(MaxLinkedFiles)}}))
		if err != nil {
			return nil, err
		}
		var fs []struct {
			Filename  string `json:"filename"`
			Status    string `json:"status"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		}
		if err := json.Unmarshal(files.Body, &fs); err != nil {
			return nil, fmt.Errorf("github: decode files of %s/%s#%d: %w", owner, repo, n, err)
		}
		for _, f := range fs {
			it.Files = append(it.Files, LinkedFile{Name: f.Filename, Status: f.Status, Additions: f.Additions, Deletions: f.Deletions})
		}
		it.FilesKnown = len(fs)
		if files.Header.Get("Link") != "" && nextLink(files.Header.Get("Link")) != "" {
			it.FilesKnown = 0 // more than one page: the total is not known from here
		}
	}
	it.Digest = it.contentDigest()
	return it, nil
}

func boundBody(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) <= MaxLinkedBody {
		return s, false
	}
	cut := MaxLinkedBody
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut], true
}

func (it *LinkedItem) contentDigest() string {
	parts := []string{it.Kind, it.Owner + "/" + it.Repo, it.Title, it.State, strings.Join(it.Labels, ","), it.Body}
	for _, f := range it.Files {
		parts = append(parts, fmt.Sprintf("%s|%s|%d|%d", f.Name, f.Status, f.Additions, f.Deletions))
	}
	return domain.Digest([]byte(strings.Join(parts, "\x00")))
}
