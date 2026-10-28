package github

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

const globalURI = "/advisories?affects=example.com%2Facme&ecosystem=go&per_page=100&type=reviewed"

// globalAPI serves two pages of the global advisory database, linked by a
// cursor the way GitHub paginates GET /advisories.
func globalAPI(t *testing.T) *fakeAPI {
	api := newFakeAPI(t)
	api.serve(globalURI, fixture(t, "global_advisories_page1.json"),
		`<{base}`+globalURI+`&after=Y3Vyc29yOnYyOpK0>; rel="next"`)
	api.serve(globalURI+"&after=Y3Vyc29yOnYyOpK0", fixture(t, "global_advisories_page2.json"),
		`<{base}`+globalURI+`&before=Y3Vyc29yOnYyOpK1>; rel="prev"`)
	return api
}

func globalLoc() catalog.Locator {
	return catalog.Locator{Kind: catalog.LocatorGitHubGlobalAdvisories, Ecosystem: "go", Package: "example.com/acme"}
}

func TestListGlobal(t *testing.T) {
	api := globalAPI(t)
	c, _ := newClient(t, api, t.TempDir(), "tok", fetch.ModeOnline)
	advs, evs, err := NewAdvisories(c).ListGlobal(context.Background(), globalLoc())
	if err != nil {
		t.Fatal(err)
	}
	if got := api.paths(); len(got) != 2 || got[0] != globalURI || !strings.Contains(got[1], "after=") {
		t.Errorf("requests = %v (both pages, cursor followed)", got)
	}
	// Withdrawn dropped; an advisory of another package only is dropped;
	// newest first.
	var ids []string
	for _, a := range advs {
		ids = append(ids, a.ID)
	}
	if want := "GHSA-g333-eeee-ffff,GHSA-g111-aaaa-bbbb,GHSA-g555-jjjj-kkkk"; strings.Join(ids, ",") != want {
		t.Fatalf("ids = %v, want %s", ids, want)
	}
	if len(evs) != 3 || advs[1].Evidence[0] != evs[1].ID {
		t.Fatalf("evidence %v", evs)
	}
	pseudo, multi, old := advs[0], advs[1], advs[2]

	// Only the entries of the located package contribute; the other
	// package's "< 9.0.0" must not widen the range.
	if multi.Vulnerable != "< 1.11.5 || >= 1.12.0-beta.0, < 1.12.1" || strings.Join(multi.Patched, ",") != "1.11.5,1.12.1" {
		t.Errorf("multi = %q %v", multi.Vulnerable, multi.Patched)
	}
	if multi.URL != "https://github.com/advisories/GHSA-g111-aaaa-bbbb" || multi.Severity != "critical" ||
		strings.Join(multi.Aliases, ",") != "CVE-2025-10001" {
		t.Errorf("multi = %+v", multi)
	}
	if multi.PublishedAt == nil || multi.UpdatedAt == nil || !multi.UpdatedAt.Equal(time.Date(2025, 4, 2, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("timestamps = %v %v", multi.PublishedAt, multi.UpdatedAt)
	}
	if evs[1].Kind != domain.EvidenceAdvisory || evs[1].URI != multi.URL || !strings.Contains(evs[1].Excerpt, ">= 1.12.0-beta.0, < 1.12.1") {
		t.Errorf("evidence = %+v", evs[1])
	}
	// Ecosystem matched case-insensitively; patched as an object or null.
	if old.Vulnerable != "< 1.2.0 || >= 1.3.0, < 1.3.2" || strings.Join(old.Patched, ",") != "1.2.0" {
		t.Errorf("old = %q %v", old.Vulnerable, old.Patched)
	}
	// The prerelease lower bound matches the stable release; the
	// pseudo-version bound matches no ordinary release.
	check := func(cons, v string) bool {
		c, err := semver.NewConstraint(cons)
		if err != nil {
			t.Fatalf("%q: %v", cons, err)
		}
		return c.Check(semver.MustParse(v))
	}
	for v, want := range map[string]bool{"1.11.4": true, "1.11.5": false, "1.12.0": true, "1.12.1": false, "1.13.0": false} {
		if got := check(multi.Vulnerable, v); got != want {
			t.Errorf("%s affected = %v, want %v", v, got, want)
		}
	}
	// A pseudo-version bound is a commit: it matches no release, and it is
	// not read as "no release is affected" either. The advisory stays, with
	// no machine-readable range (a manual check), and its raw range in the
	// evidence.
	if pseudo.Vulnerable != "" || !strings.Contains(evs[0].Excerpt, "0.0.0-20260319175635-5183b7d86137 (not machine-readable)") {
		t.Errorf("pseudo = %q, excerpt %q", pseudo.Vulnerable, evs[0].Excerpt)
	}
}

func TestListGlobalOfflineReplayAndMiss(t *testing.T) {
	api := globalAPI(t)
	dir := t.TempDir()
	c, _ := newClient(t, api, dir, "", fetch.ModeOnline)
	first, _, err := NewAdvisories(c).ListGlobal(context.Background(), globalLoc())
	if err != nil {
		t.Fatal(err)
	}
	api.Close()
	off, _ := newClient(t, api, dir, "", fetch.ModeOffline)
	again, _, err := NewAdvisories(off).ListGlobal(context.Background(), globalLoc())
	if err != nil || len(again) != len(first) {
		t.Fatalf("replay: %d, %v", len(again), err)
	}
	// An offline cache miss is unavailable, never an empty list.
	miss, _ := newClient(t, api, t.TempDir(), "", fetch.ModeOffline)
	advs, _, err := NewAdvisories(miss).ListGlobal(context.Background(), globalLoc())
	if !errors.Is(err, fetch.ErrOffline) || fetch.StateFor(err) != domain.SourceUnavailable || advs != nil {
		t.Errorf("offline miss: %v %v", advs, err)
	}
}

func TestListGlobalErrors(t *testing.T) {
	ctx := context.Background()
	api := newFakeAPI(t)
	api.handlers[globalURI] = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTooManyRequests) }
	c, _ := newClient(t, api, t.TempDir(), "", fetch.ModeOnline, WithTTL(time.Minute))
	if _, _, err := NewAdvisories(c).ListGlobal(ctx, globalLoc()); fetch.StateFor(err) != domain.SourceThrottled {
		t.Errorf("throttled: %v", err)
	}
	if _, _, err := NewAdvisories(c).ListGlobal(ctx, catalog.Locator{Kind: catalog.LocatorGitHubGlobalAdvisories, Ecosystem: "go"}); err == nil {
		t.Error("no package: want error")
	}
	bad := newFakeAPI(t)
	bad.serve(globalURI, `{"message":"x"}`, "")
	cb, _ := newClient(t, bad, t.TempDir(), "", fetch.ModeOnline)
	if _, _, err := NewAdvisories(cb).ListGlobal(ctx, globalLoc()); err == nil {
		t.Error("object instead of list: want error")
	}
}

func TestRegisterGlobalAdvisories(t *testing.T) {
	reg := sources.NewRegistry()
	Register(reg, fetch.NewHTTPClient(fetch.NewCache(t.TempDir()), fetch.ModeOffline), "")
	if _, err := reg.AdvisorySource(catalog.LocatorGitHubGlobalAdvisories); err != nil {
		t.Fatal(err)
	}
}

func TestConvertVersionRangeListsAndBounds(t *testing.T) {
	for in, want := range map[string]string{
		"v1.2.0,v1.2.1":               "= 1.2.0 || = 1.2.1",
		"v1.18.0, v1.18.1,v1.19.0":    "= 1.18.0 || = 1.18.1 || = 1.19.0",
		"= 1.2.0, = 1.2.1":            "= 1.2.0 || = 1.2.1",
		">= 1.12.0-beta.0, < 1.12.1":  ">= 1.12.0-beta.0, < 1.12.1",
		">= 1.13.0-alpha.0, < 1.15.4": ">= 1.13.0-alpha.0, < 1.15.4",
	} {
		if got, err := ConvertVersionRange(in); err != nil || got != want {
			t.Errorf("ConvertVersionRange(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"< 0.0.0-20260319175635-5183b7d86137", "< 1.3.3-0.20260331202714-6800989bdc12",
		">= 0.0.0-20220724015452, < 0.0.0-20260414213245", "<v1.12.14,<v1.15.4,<v1.16.2", ">= 1.0.0, > 1.1.0, < 2.0.0", "1.2.0, < 1.3.0", "= 1.2.0, >= 1.0.0"} {
		if got, err := ConvertVersionRange(in); err == nil {
			t.Errorf("ConvertVersionRange(%q) = %q, want error (not an interval)", in, got)
		}
	}
	// An explicit list matches exactly its versions.
	c, _ := ConvertVersionRange("v1.18.0,v1.18.1,v1.19.2")
	cons := semver.MustParse
	cs, err := semver.NewConstraint(c)
	if err != nil {
		t.Fatal(err)
	}
	for v, want := range map[string]bool{"1.18.0": true, "1.18.1": true, "1.18.2": false, "1.19.2": true, "1.19.3": false} {
		if got := cs.Check(cons(v)); got != want {
			t.Errorf("%s in %q = %v, want %v", v, c, got, want)
		}
	}
	// Prerelease lower bounds match the stable release, and not the earlier prereleases.
	cs, _ = semver.NewConstraint(">= 1.12.0-beta.0, < 1.12.1")
	for v, want := range map[string]bool{"1.12.0": true, "1.12.0-beta.1": true, "1.12.0-alpha.1": false, "1.11.9": false} {
		if got := cs.Check(cons(v)); got != want {
			t.Errorf("%s = %v, want %v", v, got, want)
		}
	}
}
