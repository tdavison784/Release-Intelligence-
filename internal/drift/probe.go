package drift

import (
	"context"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/sources"
)

// foundAt is a declared channel where an artifact version was observed.
type foundAt struct {
	channelIndex int
	channel      string // locator kind
	where        string // human description of the channel
	version      string // the artifact version found
	coordinate   string // canonical coordinate of the observation
	evidence     domain.Evidence
}

// findArtifactAt looks for an artifact of release v at its DECLARED channels
// only (template artifacts are probed per channel; lookup artifacts are
// searched in each channel's version index). Unreachable channels are
// skipped: only a positive observation counts. It returns the first declared
// channel (in definition order) that has the artifact, or nil.
//
// This mirrors ingest's resolution on purpose: a check that failed while a
// later declared channel has the artifact means the publication moved
// between channels the definition already knows about.
func (a *analyzer) findArtifactAt(ctx context.Context, art *catalog.Artifact, v domain.Version) *foundAt {
	if a.reg == nil || art == nil {
		return nil
	}
	rc := a.renderContext(v)
	switch art.Version.Strategy {
	case catalog.VersionTemplate:
		av, err := catalog.Render(art.Version.Template, rc)
		if err != nil || strings.TrimSpace(av) == "" {
			return nil
		}
		for i, ch := range art.Channels {
			probe, err := a.reg.Probe(ch.Kind)
			if err != nil {
				continue
			}
			loc, err := catalog.RenderLocator(ch, rc)
			if err != nil {
				continue
			}
			res, err := probe.Probe(ctx, loc, av)
			if err != nil || res == nil || !res.Exists {
				continue
			}
			coord := res.Coordinate
			if coord == "" {
				coord = coordinateFor(loc, av)
			}
			ev := res.Evidence
			if ev.ID == "" {
				uri := res.URI
				if uri == "" {
					uri = locatorURI(loc)
				}
				ev = domain.NewEvidence(domain.EvidenceRegistry, art.ID, uri, coord, coord, res.Digest, a.now)
			}
			return &foundAt{channelIndex: i, channel: loc.Kind, where: describeChannel(loc), version: av, coordinate: coord, evidence: ev}
		}
		return nil
	case catalog.VersionLookup:
		match, err := catalog.Render(art.Version.Match, rc)
		if err != nil || strings.TrimSpace(match) == "" {
			return nil
		}
		field := art.Version.Field
		for i, ch := range art.Channels {
			idx, err := a.reg.VersionIndex(ch.Kind)
			if err != nil {
				continue
			}
			loc, err := catalog.RenderLocator(ch, rc)
			if err != nil {
				continue
			}
			entries, err := idx.ListArtifactVersions(ctx, loc)
			if err != nil {
				continue
			}
			var matched []sources.ArtifactVersion
			exposes := false
			for _, e := range entries {
				if _, ok := e.Fields[field]; !ok {
					continue
				}
				exposes = true
				if e.Fields[field] == match {
					matched = append(matched, e)
				}
			}
			if !exposes || len(matched) == 0 {
				continue
			}
			sel := latestArtifactVersion(matched)
			coord := coordinateFor(loc, sel.Version)
			ev := sel.Evidence
			if ev.ID == "" {
				ev = domain.NewEvidence(domain.EvidenceRegistry, art.ID, locatorURI(loc), coord,
					fmt.Sprintf("version: %s %s: %s", sel.Version, field, match), sel.Digest, a.now)
			}
			return &foundAt{channelIndex: i, channel: loc.Kind, where: describeChannel(loc), version: sel.Version, coordinate: coord, evidence: ev}
		}
		return nil
	}
	return nil
}

// latestArtifactVersion picks the highest semver entry (ingest's default
// "select: latest" behaviour).
func latestArtifactVersion(vs []sources.ArtifactVersion) sources.ArtifactVersion {
	best := vs[0]
	bv, bErr := semver.NewVersion(best.Version)
	for _, e := range vs[1:] {
		ev, eErr := semver.NewVersion(e.Version)
		switch {
		case bErr != nil && eErr == nil, ev != nil && bv != nil && ev.Compare(bv) > 0:
			best, bv, bErr = e, ev, eErr
		}
	}
	return best
}

// describeChannel renders a channel for humans ("oci ghcr.io/acme/charts/acme").
func describeChannel(loc catalog.Locator) string {
	switch loc.Kind {
	case catalog.LocatorOCI:
		return "oci " + strings.TrimPrefix(loc.Repository, "oci://")
	case catalog.LocatorHelmRepo:
		return loc.URL + " (chart " + loc.Chart + ")"
	case catalog.LocatorHTTP:
		return loc.URL
	}
	s := loc.Repository
	if loc.Ref != "" {
		s += "@" + loc.Ref
	}
	if loc.Path != "" {
		s += ":" + loc.Path
	}
	return loc.Kind + " " + s
}

// coordinateFor is the canonical coordinate of a version at a channel
// (the same mapping ingest uses).
func coordinateFor(loc catalog.Locator, av string) string {
	with := func(base, sep string) string {
		if av == "" {
			return base
		}
		return base + sep + av
	}
	switch loc.Kind {
	case catalog.LocatorOCI:
		return with(strings.TrimPrefix(loc.Repository, "oci://"), ":")
	case catalog.LocatorHelmRepo:
		return with(strings.TrimSuffix(loc.URL, "/")+"/"+loc.Chart, ":")
	case catalog.LocatorHTTP:
		return loc.URL
	case catalog.LocatorGitHubReleases:
		return with("github.com/"+loc.Repository, "@")
	}
	return describeChannel(loc)
}

// locatorURI is a human-facing URI for a channel.
func locatorURI(loc catalog.Locator) string {
	if loc.URL != "" {
		return loc.URL
	}
	if loc.Kind == catalog.LocatorOCI {
		return "oci://" + strings.TrimPrefix(loc.Repository, "oci://")
	}
	if loc.Repository == "" {
		return ""
	}
	s := loc.Repository
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	return s
}
