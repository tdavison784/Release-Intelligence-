package app

import (
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/semvalidate"
)

// Validators is the single registry of the learning loop's deterministic
// validators (DESIGN.md §2.3): the validate lane's (helm-values, crd-schema,
// compatibility, image, restatement, canonical-applicability) followed by the
// render lane's rendered-diff validator over release-level (chart-default)
// renders at kubeVersion ("" = helm's default capabilities). Callers that
// validate proposals (`ri knowledge validate`) take the list from here.
func (a *App) Validators(kubeVersion string) []knowledge.Validator {
	return append(semvalidate.Validators(), a.RenderValidator(kubeVersion))
}
