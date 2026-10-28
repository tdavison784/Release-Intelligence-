package main

import (
	"context"
	"io"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	runpkg "github.com/tdavison784/release-intelligence/internal/investigate/run"
	"github.com/tdavison784/release-intelligence/internal/llm"
	"github.com/tdavison784/release-intelligence/internal/synthesize"
)

// synthFlags are the upgrade-level synthesis flags (Milestone 3). They are
// not part of the request configuration: the investigation id and its
// findings are the same with or without a judgement.
type synthFlags struct {
	on       *bool
	model    *string
	exchange *string
}

func addSynthFlags(fs interface {
	String(name, value, usage string) *string
	Bool(name string, value bool, usage string) *bool
}) synthFlags {
	return synthFlags{
		on:       fs.Bool("synthesize", false, "after classification, make the upgrade-level judgement (one cached model call; advisory, never a policy input)"),
		model:    fs.String("synthesize-model", synthesize.DefaultModel, "model of the upgrade-level synthesis"),
		exchange: fs.String("synthesize-exchange", "", "answer the synthesis through request/response files in `DIR` (scripts/semantic-exchange.sh answers them) instead of `claude -p`"),
	}
}

func (s synthFlags) enabled() bool { return s.on != nil && *s.on }

// synthesizer is the synthesis port: llm.Cache over the file exchange or a
// stateless `claude -p` call (nothing but the cache with -offline), and the
// release facts of the latest-patch rule from the app's cached ingestion.
func (c *cli) synthesizer(a *app.App, s synthFlags, claudeBin string) runpkg.Synthesizer {
	var inner llm.Client
	switch {
	case s.exchange != nil && *s.exchange != "":
		inner = &llm.Exchange{Dir: *s.exchange}
	case c.g.offline:
	default:
		inner = &synthesize.ClaudePrint{Binary: claudeBin}
	}
	model := synthesize.DefaultModel
	if s.model != nil && *s.model != "" {
		model = *s.model
	}
	return synthPort{a: a, s: &synthesize.Synthesizer{Client: &llm.Cache{Dir: a.LLMCacheDir(), Inner: inner}, Model: model}}
}

type synthPort struct {
	a *app.App
	s *synthesize.Synthesizer
}

func (p synthPort) Synthesize(ctx context.Context, in runpkg.SynthesisInput) (*domain.UpgradeJudgement, error) {
	return p.s.Synthesize(ctx, synthesize.Input{Investigation: in.Investigation, Bundle: in.Bundle, Report: in.Report, Edge: in.Edge,
		Releases: p.releases(ctx, in.Edge)})
}

// releases are the product's known releases and advisories as the edge was
// built from them, with the chart versions of the latest known patch on the
// target's line and on each candidate's line. Nil when they cannot be read:
// the judgement then recommends the minimum and says why.
func (p synthPort) releases(ctx context.Context, e *domain.UpgradeEdge) *synthesize.Releases {
	if e == nil {
		return nil
	}
	def, err := p.a.Product(string(e.Product.ID))
	if err != nil {
		return nil
	}
	vl, err := p.a.Versions(ctx, def)
	if err != nil {
		return nil
	}
	advs, advEv, _, err := p.a.Ingester.Advisories(ctx, def)
	if err != nil {
		return nil
	}
	r := &synthesize.Releases{Versions: vl.Versions, Advisories: advs, Evidence: advEv, Charts: map[string][]domain.CandidateChart{}}
	lines := map[string]bool{e.To.Line(): true}
	if e.Security != nil {
		for _, c := range e.Security.Candidates {
			if c.Version != nil && c.Kind != domain.CandidateCurrentLine {
				lines[c.Version.Line()] = true
			}
		}
	}
	for line := range lines {
		var latest *domain.Version
		for _, v := range vl.Versions {
			if !v.IsPrerelease() && v.Line() == line && (latest == nil || v.Compare(*latest) > 0) {
				v := v
				latest = &v
			}
		}
		if latest == nil {
			continue
		}
		charts, _, err := p.a.Ingester.ChartVersions(ctx, def, *latest, vl)
		if err != nil {
			continue
		}
		for _, ch := range charts {
			if len(ch.Evidence) == 0 {
				continue // a template guess is never cited
			}
			cc := domain.CandidateChart{Artifact: ch.Artifact, Name: ch.Name, Version: ch.Version}
			for _, ev := range ch.Evidence {
				cc.Evidence = append(cc.Evidence, ev.ID)
				r.ChartEvidence = append(r.ChartEvidence, ev)
			}
			r.Charts[latest.Semver] = append(r.Charts[latest.Semver], cc)
		}
	}
	return r
}

// printJudgement writes the run's judgement section (nothing without one).
func (c *cli) printJudgement(w io.Writer, st *store.Store, inv *domain.Investigation) error {
	if inv.Judgement == nil {
		return nil
	}
	b, err := st.GetBundle(c.ctx, inv.Bundle)
	if err != nil {
		b = nil
	}
	return synthesize.RenderJudgement(w, inv, b)
}
