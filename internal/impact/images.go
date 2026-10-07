package impact

// CONTRACT-CHANGE(renderfirst): product-owner decision PO-7a (briefs/
// renderfirst.md) — for an image/tag change whose repository the environment
// references, the customer's own From→To render (with their configuration)
// decides what the reference alone cannot: a referenced repository whose image
// changes in their rendered delta (attributable, rendered evidence on chain 2)
// versus a reference that pins the image so nothing changes (values-unset of
// the image family: impact:image-render-unchanged, not-affected with a render
// check). Without --render — or when no render decides — today's
// impact:image-changed verdict stands, byte-identical.

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// RuleImageRenderUnchanged is the not-affected verdict of the image family
// (alias of the domain constant, like every Rule*).
const RuleImageRenderUnchanged = domain.RuleImageRenderUnchanged

// ImageOutcome is what the customer's render said about one image repository
// of an image change.
type ImageOutcome string

const (
	// ImageAttributable: the customer's rendered upgrade delta contains a
	// change for the repository.
	ImageAttributable ImageOutcome = "attributable"
	// ImageNoEffect: every rendered deployment succeeded with complete values
	// and none contains a change for the repository.
	ImageNoEffect ImageOutcome = "no-effect"
	// ImageUndecided: no render decided.
	ImageUndecided ImageOutcome = "undecided"
)

// ImageResult is the outcome of one repository of an image change.
type ImageResult struct {
	Outcome ImageOutcome
	// Matches are the rendered image changes of the customer's upgrade delta
	// (Outcome attributable), each with environment evidence.
	Matches []domain.ImpactMatch
	// Checks is the no-image-change render check (Outcome no-effect).
	Checks []domain.ImpactCheck
	// Records are evidence records the evaluation created.
	Records []domain.Evidence
	// Needed says why no render decided (Outcome undecided).
	Needed []string
}

// ImageRenderEvaluator decides one repository of an image change against the
// customer's rendered upgrade delta (PO-7a): does their From→To render change
// the image. The render lane implements it (internal/render.RenderImages over
// the environment's pairs, `ri impact --render`); without one the verdict is
// today's image-changed.
type ImageRenderEvaluator interface {
	// EvaluateImage reports the customer's rendered delta for one repository
	// of the change.
	EvaluateImage(c domain.Change, repo string) ImageResult
}

// imageVerdict emits the PO-7a verdict of one referenced repository: without
// an evaluator (no --render), or when no render decided, exactly today's
// review-required image-changed; a decisive render replaces or backs it.
func (b *builder) imageVerdict(ev ImageRenderEvaluator, c domain.Change, repo string, matches []domain.ImpactMatch, detail, title string) {
	var r ImageResult
	if ev != nil {
		r = ev.EvaluateImage(c, repo)
	}
	for _, rec := range r.Records {
		if _, ok := b.extraLocal[rec.ID]; !ok {
			b.extraLocal[rec.ID] = rec
			b.extraLocalOrder = append(b.extraLocalOrder, rec.ID)
		}
	}
	switch r.Outcome {
	case ImageNoEffect:
		toTag := b.edge.To.String()
		detail := fmt.Sprintf("Your environment references %s, but rendering the upgrade with your configuration shows no change to it — you pin the reference (or the value that selects it), so %s's image change for this repository never reaches your rendered deployment. No action is needed for this image; review your pin against the new reference when you next change it.",
			repo, toTag)
		if refs := imageRefs(matches); refs != "" {
			detail += "\n" + refs
		}
		b.verdict(RuleImageRenderUnchanged, domain.ImpactNotAffected, c.ID+":"+repo,
			fmt.Sprintf("You reference %s but your render does not change it: %s's image change does not reach you", code(repo), toTag),
			detail, c, c.Evidence, append([]domain.ImpactCheck{b.imagesCheck([]string{repo})}, r.Checks...))
	default: // attributable, undecided or no evaluator: today's verdict
		all := matches
		if len(r.Matches) > 0 { // the render attributed the change
			all = append(append([]domain.ImpactMatch{}, matches...), r.Matches...)
		}
		b.add(RuleImageChanged, domain.ImpactReviewRequired, domain.SeverityMedium, domain.ConfidenceHigh, title, detail, c, all, c.Evidence...)
	}
}

func imageRefs(matches []domain.ImpactMatch) string {
	var out []string
	for _, m := range matches {
		out = append(out, "You reference it as "+code(m.Subject)+".")
	}
	return strings.Join(out, "\n")
}
