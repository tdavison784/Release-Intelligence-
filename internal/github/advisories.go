package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/fetch"
)

// advisory is the subset of the GitHub repository security advisory object
// used here.
type advisory struct {
	GHSAID      string     `json:"ghsa_id"`
	CVEID       string     `json:"cve_id"`
	HTMLURL     string     `json:"html_url"`
	Summary     string     `json:"summary"`
	Severity    string     `json:"severity"`
	State       string     `json:"state"`
	PublishedAt *time.Time `json:"published_at"`
	WithdrawnAt *time.Time `json:"withdrawn_at"`
	Identifiers []struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"identifiers"`
	Vulnerabilities []struct {
		VulnerableVersionRange string `json:"vulnerable_version_range"`
		PatchedVersions        string `json:"patched_versions"`
	} `json:"vulnerabilities"`
}

// Advisories implements sources.AdvisorySource for the github-advisories
// locator kind.
type Advisories struct {
	c *Client
}

// NewAdvisories returns the github-advisories adapter.
func NewAdvisories(c *Client) *Advisories { return &Advisories{c: c} }

// ListAdvisories lists the published security advisories of loc.Repository
// ("owner/name"): all pages of
// GET /repos/{owner}/{repo}/security-advisories?state=published&per_page=100.
//
// Each advisory becomes a domain.Advisory:
//
//	ID          ghsa_id
//	Aliases     cve_id and the other identifiers (sorted, without the ID itself)
//	Summary     summary
//	Severity    severity (critical, high, medium, low)
//	URL         html_url
//	PublishedAt published_at
//	Vulnerable  the vulnerable_version_range of every entry of "vulnerabilities",
//	            converted by ConvertVersionRange and united with "||"
//	Patched     the patched_versions of those entries
//	SourceID    left empty: the adapter does not know the source id and the
//	            caller fills it in
//	Evidence    the ID of one advisory-evidence record
//
// and one domain.EvidenceAdvisory record is returned per advisory (in the
// same order), pointing at the advisory page, with the digest of the
// advisory's JSON as returned by the API. Withdrawn advisories are skipped.
// Advisories are ordered newest first (ties by ID). Ranges that cannot be
// converted into a constraint are left out of Vulnerable; the original
// ranges are kept in the evidence excerpt.
func (a *Advisories) ListAdvisories(ctx context.Context, loc catalog.Locator) ([]domain.Advisory, []domain.Evidence, error) {
	owner, name, err := parseRepository(loc.Repository)
	if err != nil {
		return nil, nil, err
	}
	first := a.c.endpoint("/repos/"+owner+"/"+name+"/security-advisories",
		url.Values{"state": {"published"}, "per_page": {fmt.Sprint(perPage)}})

	type pair struct {
		adv domain.Advisory
		ev  domain.Evidence
	}
	var pairs []pair
	seen := map[string]bool{}
	err = a.c.paginate(ctx, first, func(doc *fetch.Document) error {
		var page []json.RawMessage
		if err := json.Unmarshal(doc.Body, &page); err != nil {
			return fmt.Errorf("github-advisories %s/%s: decoding %s: %w", owner, name, doc.URL, err)
		}
		for _, raw := range page {
			var ga advisory
			if err := json.Unmarshal(raw, &ga); err != nil {
				return fmt.Errorf("github-advisories %s/%s: decoding advisory: %w", owner, name, err)
			}
			if ga.GHSAID == "" || ga.WithdrawnAt != nil || seen[ga.GHSAID] {
				continue
			}
			seen[ga.GHSAID] = true
			adv, ev := convertAdvisory(owner, name, ga, raw, doc.RetrievedAt)
			pairs = append(pairs, pair{adv, ev})
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	sort.SliceStable(pairs, func(i, j int) bool {
		pi, pj := pairs[i].adv.PublishedAt, pairs[j].adv.PublishedAt
		switch {
		case pi != nil && pj != nil && !pi.Equal(*pj):
			return pi.After(*pj)
		case (pi == nil) != (pj == nil):
			return pi != nil
		}
		return pairs[i].adv.ID < pairs[j].adv.ID
	})
	advs := make([]domain.Advisory, len(pairs))
	evs := make([]domain.Evidence, len(pairs))
	for i, p := range pairs {
		advs[i], evs[i] = p.adv, p.ev
	}
	return advs, evs, nil
}

// convertAdvisory maps an API advisory (and its raw JSON, which is digested
// for the evidence) to the domain model.
func convertAdvisory(owner, name string, ga advisory, raw []byte, retrievedAt time.Time) (domain.Advisory, domain.Evidence) {
	var ranges, patched []string
	var patchedSeen = map[string]bool{}
	for _, v := range ga.Vulnerabilities {
		if r := strings.TrimSpace(v.VulnerableVersionRange); r != "" {
			ranges = append(ranges, r)
		}
		for _, p := range ParsePatchedVersions(v.PatchedVersions) {
			if !patchedSeen[p] {
				patchedSeen[p] = true
				patched = append(patched, p)
			}
		}
	}
	vulnerable, _ := CombineVersionRanges(ranges)

	aliasSet := map[string]bool{}
	addAlias := func(a string) {
		if a = strings.TrimSpace(a); a != "" && !strings.EqualFold(a, ga.GHSAID) {
			aliasSet[a] = true
		}
	}
	addAlias(ga.CVEID)
	for _, id := range ga.Identifiers {
		addAlias(id.Value)
	}
	var aliases []string
	for a := range aliasSet {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)

	uri := ga.HTMLURL
	if uri == "" {
		uri = "https://github.com/" + owner + "/" + name + "/security/advisories/" + ga.GHSAID
	}

	excerpt := ga.GHSAID
	if ga.Severity != "" {
		excerpt += " (" + ga.Severity + ")"
	}
	excerpt += ": " + strings.TrimSpace(ga.Summary)
	if len(ranges) > 0 {
		excerpt += "; vulnerable: " + strings.Join(ranges, " | ")
	}
	if len(patched) > 0 {
		excerpt += "; patched: " + strings.Join(patched, ", ")
	}
	ev := domain.NewEvidence(domain.EvidenceAdvisory, "", uri, ga.GHSAID, excerpt, domain.Digest(raw), retrievedAt)

	adv := domain.Advisory{
		ID:         ga.GHSAID,
		Aliases:    aliases,
		Summary:    strings.TrimSpace(ga.Summary),
		Severity:   strings.ToLower(strings.TrimSpace(ga.Severity)),
		URL:        uri,
		Vulnerable: vulnerable,
		Patched:    patched,
		Evidence:   []domain.EvidenceID{ev.ID},
	}
	if ga.PublishedAt != nil {
		t := ga.PublishedAt.UTC()
		adv.PublishedAt = &t
	}
	return adv, ev
}
