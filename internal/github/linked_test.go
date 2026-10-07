package github

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/fetch"
)

func TestLinkedPullRequest(t *testing.T) {
	api := newFakeAPI(t)
	api.serve("/repos/cert-manager/cert-manager/issues/7601", fixture(t, "linked_issue_pr.json"), "")
	api.serve("/repos/cert-manager/cert-manager/pulls/7601/files?per_page=100", fixture(t, "linked_pr_files.json"), "")
	c, _ := newClient(t, api, t.TempDir(), "tok", fetch.ModeOnline)

	// a bare #N is ambiguous: GitHub's issues endpoint says it is a PR, and the files follow
	it, err := c.LinkedItem(context.Background(), "cert-manager", "cert-manager", "github-ref", 7601, "")
	if err != nil {
		t.Fatal(err)
	}
	if it.Kind != LinkedPullRequest || it.Title != "Change default of privateKey.rotationPolicy to Always" || it.State != "closed" {
		t.Errorf("item = %+v", it)
	}
	if strings.Contains(it.Body, "\r") || !strings.Contains(it.Body, "The default rotationPolicy was Never") {
		t.Errorf("body = %q", it.Body)
	}
	if len(it.Labels) != 2 || len(it.Files) != 3 || it.Files[2].Status != "removed" {
		t.Errorf("labels/files = %v / %+v", it.Labels, it.Files)
	}
	if it.Digest == "" {
		t.Error("no digest")
	}
	// the token goes to the API only, and exactly two requests were made
	if got := api.paths(); len(got) != 2 {
		t.Errorf("requests = %v", got)
	}
	for _, r := range api.requests() {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
	}
	// the same content hashes the same
	again, _ := c.LinkedItem(context.Background(), "cert-manager", "cert-manager", "pull-request", 7601, "")
	if again.Digest != it.Digest {
		t.Error("digest is not stable")
	}
}

func TestLinkedIssueCommitAndMissing(t *testing.T) {
	api := newFakeAPI(t)
	api.serve("/repos/kubernetes/ingress-nginx/issues/11176", fixture(t, "linked_issue_only.json"), "")
	api.serve("/repos/kubernetes/ingress-nginx/commits/618aae18515213bcf3fb820e6f8c234703d844b2", fixture(t, "linked_commit.json"), "")
	c, _ := newClient(t, api, t.TempDir(), "", fetch.ModeOnline)
	ctx := context.Background()

	iss, err := c.LinkedItem(ctx, "kubernetes", "ingress-nginx", LinkedIssue, 11176, "")
	if err != nil || iss.Kind != LinkedIssue || iss.Body != "" || len(iss.Files) != 0 || len(api.paths()) != 1 {
		t.Fatalf("issue = %+v err %v paths %v (an issue has no files request)", iss, err, api.paths())
	}
	cm, err := c.LinkedItem(ctx, "kubernetes", "ingress-nginx", LinkedCommit, 0, "618aae18515213bcf3fb820e6f8c234703d844b2")
	if err != nil {
		t.Fatal(err)
	}
	if cm.Kind != LinkedCommit || cm.Title != "Enable strict path type validation by default" ||
		cm.Body != "Paths containing dots are rejected for Exact path types." || len(cm.Files) != 1 {
		t.Errorf("commit = %+v", cm)
	}
	if _, err := c.LinkedItem(ctx, "kubernetes", "ingress-nginx", LinkedIssue, 1, ""); !errors.Is(err, fetch.ErrNotFound) {
		t.Errorf("missing item error = %v, want ErrNotFound", err)
	}
	// hostile input never reaches the network
	n := len(api.paths())
	for _, bad := range []func() error{
		func() error { _, e := c.LinkedItem(ctx, "../x", "y", LinkedIssue, 1, ""); return e },
		func() error { _, e := c.LinkedItem(ctx, "a", "b", LinkedIssue, 0, ""); return e },
		func() error { _, e := c.LinkedItem(ctx, "a", "b", LinkedCommit, 0, "not-a-sha/../x"); return e },
	} {
		if bad() == nil {
			t.Error("invalid input accepted")
		}
	}
	if len(api.paths()) != n {
		t.Error("invalid input caused requests")
	}
}

func TestLinkedBoundsAndThrottle(t *testing.T) {
	big := `{"number":1,"title":"t","state":"open","labels":[],"body":"` + strings.Repeat("é", MaxLinkedBody) + `"}`
	api := newFakeAPI(t)
	api.serve("/repos/o/r/issues/1", big, "")
	api.handlers["/repos/o/r/issues/2"] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}
	c, _ := newClient(t, api, t.TempDir(), "", fetch.ModeOnline)
	it, err := c.LinkedItem(context.Background(), "o", "r", LinkedIssue, 1, "")
	if err != nil || !it.BodyTruncated || len(it.Body) > MaxLinkedBody || strings.ContainsRune(it.Body, '�') {
		t.Fatalf("bounded body: truncated %v len %d err %v", it != nil && it.BodyTruncated, len(it.Body), err)
	}
	_, err = c.LinkedItem(context.Background(), "o", "r", LinkedIssue, 2, "")
	if !errors.Is(err, fetch.ErrThrottled) {
		t.Fatalf("error = %v, want ErrThrottled", err)
	}
	var fe *fetch.Error
	if !errors.As(err, &fe) || fe.RetryAfter <= 0 {
		t.Errorf("throttle must carry the server's Retry-After: %+v", fe)
	}
}

func TestLinkedOfflineReplay(t *testing.T) {
	api := newFakeAPI(t)
	api.serve("/repos/kubernetes/ingress-nginx/issues/11176", fixture(t, "linked_issue_only.json"), "")
	cache := t.TempDir()
	c, _ := newClient(t, api, cache, "", fetch.ModeOnline)
	if _, err := c.LinkedItem(context.Background(), "kubernetes", "ingress-nginx", LinkedIssue, 11176, ""); err != nil {
		t.Fatal(err)
	}
	n := len(api.paths())
	off, _ := newClient(t, api, cache, "", fetch.ModeOffline)
	if it, err := off.LinkedItem(context.Background(), "kubernetes", "ingress-nginx", LinkedIssue, 11176, ""); err != nil || it.Title != "Validate path type" {
		t.Fatalf("offline replay = %+v, %v", it, err)
	}
	if len(api.paths()) != n {
		t.Error("offline replay touched the network")
	}
}
