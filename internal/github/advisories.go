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
	UpdatedAt   *time.Time `json:"updated_at"`
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
//	            converted by ConvertVersionRange and united with "||"; empty
//	            when any of them cannot be converted
//	Patched     the patched_versions of those entries
//	SourceID    left empty: the adapter does not know the source id and the
//	            caller fills it in
//	Evidence    the ID of one advisory-evidence record
//
// and one domain.EvidenceAdvisory record is returned per advisory (in the
// same order), pointing at the advisory page, with the digest of the
// advisory's JSON as returned by the API. Withdrawn advisories are skipped.
// Advisories are ordered newest first (ties by ID). If any range of an
// advisory cannot be converted into a constraint, Vulnerable is left empty
// (a partial constraint could claim a version is unaffected while the
// unconverted range still covers it); the original ranges are kept in the
// evidence excerpt.
func (a *Advisories) ListAdvisories(ctx context.Context, loc catalog.Locator) ([]domain.Advisory, []domain.Evidence, error) {
	owner, name, err := parseRepository(loc.Repository)
	if err != nil {
		return nil, nil, err
	}
	first := a.c.endpoint("/repos/"+owner+"/"+name+"/security-advisories",
		url.Values{"state": {"published"}, "per_page": {fmt.Sprint(perPage)}})

	var pairs []advisoryPair
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
			pairs = append(pairs, advisoryPair{adv, ev})
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return sortedAdvisories(pairs)
}

// advisoryPair is one converted advisory and its evidence record.
type advisoryPair struct {
	adv domain.Advisory
	ev  domain.Evidence
}

// sortedAdvisories orders advisories newest first (ties by ID) and splits
// the pairs.
func sortedAdvisories(pairs []advisoryPair) ([]domain.Advisory, []domain.Evidence, error) {
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

// convertAdvisory maps a repository advisory of owner/name (and its raw
// JSON, which is digested for the evidence) to the domain model.
func convertAdvisory(owner, name string, ga advisory, raw []byte, retrievedAt time.Time) (domain.Advisory, domain.Evidence) {
	var ranges, patched []string
	for _, v := range ga.Vulnerabilities {
		ranges = append(ranges, v.VulnerableVersionRange)
		patched = append(patched, ParsePatchedVersions(v.PatchedVersions)...)
	}
	uri := ga.HTMLURL
	if uri == "" {
		uri = "https://github.com/" + owner + "/" + name + "/security/advisories/" + ga.GHSAID
	}
	return convertAdvisoryRanges(ga, uri, ranges, patched, raw, retrievedAt)
}

// convertAdvisoryRanges maps an API advisory to the domain model: uri is its
// human page, rawRanges the vulnerable_version_range of every vulnerability
// entry that applies, rawPatched the patched versions they name, and raw the
// advisory's JSON (digested for the evidence).
func convertAdvisoryRanges(ga advisory, uri string, rawRanges, rawPatched []string, raw []byte, retrievedAt time.Time) (domain.Advisory, domain.Evidence) {
	var ranges, patched []string
	var patchedSeen = map[string]bool{}
	for _, r := range rawRanges {
		if r = strings.TrimSpace(r); r != "" {
			ranges = append(ranges, r)
		}
	}
	for _, p := range rawPatched {
		if !patchedSeen[p] {
			patchedSeen[p] = true
			patched = append(patched, p)
		}
	}
	// A range that cannot be converted may still cover the target version, so
	// a partial constraint would turn "maybe affected" into a confident "fixed"
	// (or "not affected"). If any range fails, Vulnerable stays empty: the
	// upgrade engine then asks for a manual check instead of making a claim.
	// The raw ranges stay in the evidence excerpt below.
	vulnerable, skipped := CombineVersionRanges(ranges)
	if len(skipped) > 0 {
		vulnerable = ""
	}

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

	excerpt := ga.GHSAID
	if ga.Severity != "" {
		excerpt += " (" + ga.Severity + ")"
	}
	excerpt += ": " + strings.TrimSpace(ga.Summary)
	if len(ranges) > 0 {
		excerpt += "; vulnerable: " + strings.Join(ranges, " | ")
		if len(skipped) > 0 {
			excerpt += " (not machine-readable)"
		}
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
	if ga.UpdatedAt != nil {
		t := ga.UpdatedAt.UTC()
		adv.UpdatedAt = &t
	}
	return adv, ev
}

// globalAdvisory is the subset of a GitHub global advisory database object
// (GET /advisories) used here. Unlike a repository advisory, every
// vulnerability entry names its package, and the fix is one
// first_patched_version per entry.
type globalAdvisory struct {
	advisory
	Type            string `json:"type"`
	Vulnerabilities []struct {
		Package struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
		} `json:"package"`
		VulnerableVersionRange string          `json:"vulnerable_version_range"`
		FirstPatchedVersion    json.RawMessage `json:"first_patched_version"`
	} `json:"vulnerabilities"`
}

// firstPatched reads first_patched_version, which the API writes as a
// string ("1.12.1"), as an object ({"identifier": "1.12.1"}) in older
// responses, or as null.
func firstPatched(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct {
		Identifier string `json:"identifier"`
	}
	if json.Unmarshal(raw, &o) == nil {
		return o.Identifier
	}
	return ""
}

// ListGlobal lists the reviewed advisories of GitHub's global advisory
// database that affect one package (loc.Ecosystem, loc.Package): all pages
// of GET /advisories?ecosystem=…&affects=…&type=reviewed&per_page=100.
//
// Many projects publish no repository advisories, so their advisories exist
// only here. Only the vulnerability entries whose package equals the
// locator's (ecosystem case-insensitive, name exact) contribute ranges and
// patched versions; an advisory with no such entry is dropped (the API's
// affects filter may also match other packages). Conversion is the one of
// ListAdvisories: Vulnerable is the union of the entries' ranges, empty when
// any of them cannot be converted, Patched the entries'
// first_patched_version, URL the advisory's html_url
// (https://github.com/advisories/GHSA-…), and one advisory-evidence record
// per advisory digests its JSON. Withdrawn advisories are skipped.
func (a *Advisories) ListGlobal(ctx context.Context, loc catalog.Locator) ([]domain.Advisory, []domain.Evidence, error) {
	eco, pkg := strings.ToLower(strings.TrimSpace(loc.Ecosystem)), strings.TrimSpace(loc.Package)
	if eco == "" || pkg == "" {
		return nil, nil, fmt.Errorf("github-global-advisories: locator needs ecosystem and package (got %q, %q)", loc.Ecosystem, loc.Package)
	}
	first := a.c.endpoint("/advisories", url.Values{
		"ecosystem": {eco}, "affects": {pkg}, "type": {"reviewed"}, "per_page": {fmt.Sprint(perPage)},
	})
	var pairs []advisoryPair
	seen := map[string]bool{}
	err := a.c.paginate(ctx, first, func(doc *fetch.Document) error {
		var page []json.RawMessage
		if err := json.Unmarshal(doc.Body, &page); err != nil {
			return fmt.Errorf("github-global-advisories %s %s: decoding %s: %w", eco, pkg, doc.URL, err)
		}
		for _, raw := range page {
			var ga globalAdvisory
			if err := json.Unmarshal(raw, &ga); err != nil {
				return fmt.Errorf("github-global-advisories %s %s: decoding advisory: %w", eco, pkg, err)
			}
			if ga.GHSAID == "" || ga.WithdrawnAt != nil || seen[ga.GHSAID] {
				continue
			}
			var ranges, patched []string
			for _, v := range ga.Vulnerabilities {
				if !strings.EqualFold(v.Package.Ecosystem, eco) || v.Package.Name != pkg {
					continue
				}
				ranges = append(ranges, v.VulnerableVersionRange)
				patched = append(patched, ParsePatchedVersions(firstPatched(v.FirstPatchedVersion))...)
			}
			if len(ranges) == 0 {
				continue
			}
			seen[ga.GHSAID] = true
			uri := ga.HTMLURL
			if uri == "" {
				uri = "https://github.com/advisories/" + ga.GHSAID
			}
			adv, ev := convertAdvisoryRanges(ga.advisory, uri, ranges, patched, raw, doc.RetrievedAt)
			pairs = append(pairs, advisoryPair{adv, ev})
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return sortedAdvisories(pairs)
}

// GlobalAdvisories implements sources.AdvisorySource for the
// github-global-advisories locator kind (Advisories.ListGlobal).
type GlobalAdvisories struct{ a *Advisories }

// NewGlobalAdvisories returns the github-global-advisories adapter.
func NewGlobalAdvisories(c *Client) *GlobalAdvisories {
	return &GlobalAdvisories{a: NewAdvisories(c)}
}

// ListAdvisories implements sources.AdvisorySource.
func (g *GlobalAdvisories) ListAdvisories(ctx context.Context, loc catalog.Locator) ([]domain.Advisory, []domain.Evidence, error) {
	return g.a.ListGlobal(ctx, loc)
}
