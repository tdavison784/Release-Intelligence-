package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// SynthesisInput is what the upgrade-level synthesis reads from a classified
// run (Milestone 3): the record, its bundle, its impact report and the
// upgrade edge. Report and Edge may be nil.
type SynthesisInput struct {
	Investigation *domain.Investigation
	Bundle        *domain.EvidenceBundle
	Report        *domain.ImpactReport
	Edge          *domain.UpgradeEdge
}

// Synthesizer makes the upgrade judgement (internal/synthesize, wired in
// cmd/ri). It must not change its input.
type Synthesizer interface {
	Synthesize(ctx context.Context, in SynthesisInput) (*domain.UpgradeJudgement, error)
}

// SynthesizeInput is the payload of the synthesize activity.
type SynthesizeInput struct {
	Investigation string `json:"investigation"`
	Round         int    `json:"round"`
}

// SynthesizeOutput says what the synthesis recorded.
type SynthesizeOutput struct {
	Judgement      string                 `json:"judgement"`
	Status         domain.JudgementStatus `json:"status"`
	Recommendation domain.Recommendation  `json:"recommendation,omitempty"`
	Detail         string                 `json:"detail,omitempty"`
	Digest         string                 `json:"digest"`
}

// SynthesizeKey is the idempotency key of a synthesis: the record's content
// before it, so a retry or a second worker dedupes the model call.
func SynthesizeKey(in SynthesizeInput, record string) string {
	return "synthesize:" + domain.ShortHash(in.Investigation, itoa(in.Round), record)
}

// Synthesize runs the upgrade-level synthesis after classify and records the
// judgement on the run record with its audit events. It never touches the
// findings, the decision, the review or knowledge: only Judgement and the
// audit trail change. A synthesis that produces no judgement (no answer, a
// pending exchange) is recorded as a failed stage and returned as an error;
// callers carry on to the review without a judgement.
func (p *Pipeline) Synthesize(ctx context.Context, in SynthesizeInput) (SynthesizeOutput, error) {
	if p.d.Synthesizer == nil {
		return SynthesizeOutput{}, investigate.PermanentError(domain.StageSynthesize, errors.New("no synthesizer is configured"))
	}
	rc, err := p.load(ctx, in.Investigation, false)
	if err != nil {
		return SynthesizeOutput{}, err
	}
	if rc.inv.Decision == nil {
		return SynthesizeOutput{}, investigate.PermanentError(domain.StageSynthesize, fmt.Errorf("investigation %s is not classified", in.Investigation))
	}
	before := *rc.inv
	before.Judgement = nil
	key := SynthesizeKey(in, ContentDigest(&before))
	var cached SynthesizeOutput
	if ok, err := p.W().Memo("out:"+key, &cached); err != nil {
		return SynthesizeOutput{}, err
	} else if ok {
		return cached, nil
	}
	edge, err := p.edge(ctx, rc)
	if err != nil {
		return SynthesizeOutput{}, err
	}
	if _, err := p.update(ctx, domain.StageSynthesize, in.Investigation, func(inv *domain.Investigation) error {
		inv.AppendAudit(domain.AuditEvent{At: p.now(), Stage: domain.StageSynthesize, Event: "started", Actor: "system"})
		return nil
	}); err != nil {
		return SynthesizeOutput{}, err
	}
	beat(ctx, "synthesize")
	j, serr := p.d.Synthesizer.Synthesize(ctx, SynthesisInput{Investigation: rc.inv, Bundle: rc.bundle, Report: rc.report, Edge: edge})
	if serr != nil {
		if _, uerr := p.update(ctx, domain.StageSynthesize, in.Investigation, func(inv *domain.Investigation) error {
			inv.AppendAudit(domain.AuditEvent{At: p.now(), Stage: domain.StageSynthesize, Event: "failed", Actor: "system", Detail: Trunc(serr.Error())})
			return nil
		}); uerr != nil {
			serr = errors.Join(serr, uerr)
		}
		return SynthesizeOutput{}, serr
	}
	out := SynthesizeOutput{Judgement: j.ID, Status: j.Status, Recommendation: j.Recommendation,
		Detail: fmt.Sprintf("judgement %s: %s %s", j.ID, j.Status, j.Recommendation)}
	inv, err := p.update(ctx, domain.StageSynthesize, in.Investigation, func(inv *domain.Investigation) error {
		inv.Judgement = j
		inv.AppendAudit(domain.AuditEvent{At: p.now(), Stage: domain.StageSynthesize, Event: "completed", Actor: "synthesize:" + j.Provenance.Model,
			Refs: []string{j.ID}, Detail: out.Detail})
		if err := inv.ValidateAgainst(rc.bundle); err != nil {
			return investigate.PermanentError(domain.StageSynthesize, err)
		}
		return nil
	})
	if err != nil {
		return SynthesizeOutput{}, err
	}
	out.Digest = Digest(inv)
	if err := p.W().SetMemo("out:"+key, out); err != nil {
		return SynthesizeOutput{}, err
	}
	return out, nil
}

// edge reads the run's upgrade-edge artifact (the bundle's change record
// names it); nil when the bundle names none.
func (p *Pipeline) edge(ctx context.Context, rc *runCtx) (*domain.UpgradeEdge, error) {
	id := rc.bundle.Change.Edge
	if id == "" {
		return nil, nil
	}
	for _, a := range rc.bundle.Manifest.Artifacts {
		if a.ID == id {
			var e domain.UpgradeEdge
			if err := p.readArtifact(ctx, a, &e); err != nil {
				return nil, fmt.Errorf("upgrade edge %s: %w", id, err)
			}
			return &e, nil
		}
	}
	return nil, investigate.PermanentError(domain.StageSynthesize, fmt.Errorf("upgrade edge %s is not a bundle artifact", id))
}
