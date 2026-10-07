package render

import (
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
)

// ConditionEvaluator implements impact.RenderedChangeEvaluator over the
// environment's render pairs (`ri impact --render`): rendered-change leaves of
// verified facts are decided by EvaluateRenderedChange, whose result maps
// field for field onto impact.ConditionResult.
type ConditionEvaluator struct {
	Pairs []*Pair
}

var _ impact.RenderedChangeEvaluator = ConditionEvaluator{}

// EvaluateRenderedChange implements impact.RenderedChangeEvaluator.
func (r ConditionEvaluator) EvaluateRenderedChange(c domain.Condition, _ *env.Environment, _ *domain.UpgradeEdge) impact.ConditionResult {
	x := EvaluateRenderedChange(c, r.Pairs)
	out := impact.ConditionResult{Value: impact.Unknown, Reason: x.Reason, Needed: x.Needed}
	switch x.Value {
	case RenderedTrue:
		out = impact.ConditionResult{Value: impact.True, Matches: x.Matches}
	case RenderedFalse:
		out = impact.ConditionResult{Value: impact.False, Checks: x.Checks, Examined: x.Examined}
	default:
		if out.Reason == "" {
			out.Reason = domain.UnknownEnvironmentVisibilityGap
		}
		if len(out.Needed) == 0 && x.Detail != "" {
			out.Needed = []string{x.Detail}
		}
	}
	out.Records = x.Evidence
	return out
}
