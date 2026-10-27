package verify

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/runtime"
)

// "The role cannot form" (PD-6, lane verify4, D-0027). A default probe in
// the role (probe.go) builds the role's consumer relationship around an
// object that leaves the role field unset, or sets it to another value.
// When such an object cannot act in the role, the dependent output the
// consumer verifies is never issued, and the consumer check stops
// undetermined. Undetermined also covers timeouts, controller failures and
// objects missing for other reasons, so the probe's consumer outcome at one
// version is role-cannot-form only when the run's object observations of
// that version (the observation after it settled and, when the exercises
// ran, the one after them) show all of:
//
//   - the controller healthy and reconciled: the run completed and, at TO,
//     the upgrade completed;
//   - the subject: it exists, was applied, is ready, and nothing in the
//     sandbox failed for it; the consumer captured its trust output and, if
//     it re-issued the subject, the subject answered;
//   - the dependent output not produced: the consumer check could not
//     re-issue a consuming object or could not read the verified output (an
//     x509 error on an output that was read is not this);
//   - the refusal stated in object state: every consuming object exists and
//     was applied, and in every observation of the version at least one of
//     them carries a failure its controller reports (a failure condition
//     that holds, or a condition that does not hold for a reason naming a
//     failure, such as an invalid key pair). "Not ready" while the
//     controller is still working on it (a Ready=False whose reason names
//     no failure) is what a late output looks like too, so it is recorded
//     as context and never suffices; nor does a consuming object the
//     controller never reported on.
//
// The evidence is the observed states, conditions and warning events of the
// arm's objects; log lines are never read for it. The outcome is recorded
// on the probe and never decides the method; it can only establish an
// observed role-membership default (RoleMembershipDefaults).

// versionPhases are the observation phases of one version, the settled
// observation first.
func versionPhases(version string) []string {
	if version == "to" {
		return []string{runtime.PhaseTo, runtime.PhaseToExercised}
	}
	return []string{runtime.PhaseFrom, runtime.PhaseFromExercised}
}

// RoleCannotForm decides whether an in-role default probe's consumer
// observation at one version ("from" or "to") is role-cannot-form (see the
// comment above). It returns the object-state evidence when it is, and why
// not otherwise.
func RoleCannotForm(res *runtime.FixtureResult, p armPlan, version string, c *runtime.ConsumerResult) (evidence []string, why string) {
	r := res.Run
	switch {
	case r == nil || r.Status != runtime.RunCompleted:
		return nil, "the run did not complete"
	case version == "to" && !r.UpgradeCompleted:
		return nil, "the upgrade did not complete"
	case p.consumer == nil || c == nil:
		return nil, "no consumer observation was made"
	case c.Outcome != runtime.ConsumerUndetermined:
		return nil, "the consumer observation decided (" + c.Outcome + ")"
	case len(c.TrustCertificates) == 0:
		return nil, "the subject's trust output was not captured (" + c.Detail + ")"
	case c.Certificate != "":
		return nil, "the verified output was read, and x509 could not decide (" + c.Detail + ")"
	}
	if len(c.Unanswered) > 0 && c.Unanswered[0] == p.subject {
		return nil, "the subject itself did not answer its re-issue (" + c.Detail + ")"
	}
	in := map[string]bool{}
	for _, o := range p.objects {
		in[o] = true
	}
	for _, d := range r.Degradations {
		if d.Infrastructure && in[d.Object] {
			return nil, "the sandbox could not run " + d.Object
		}
	}
	evidence = append(evidence, fmt.Sprintf("%s: the dependent output was not produced: %s", version, c.Detail))
	seen := 0
	for _, phase := range versionPhases(version) {
		o := res.Observations[phase]
		if o == nil {
			continue
		}
		seen++
		st, ok := o.Objects[p.subject]
		switch {
		case !ok || !st.Exists || st.Unserved:
			return nil, fmt.Sprintf("%s: the subject %s does not exist", phase, p.subject)
		case st.ApplyError != "":
			return nil, fmt.Sprintf("%s: the subject %s was not applied: %s", phase, p.subject, st.ApplyError)
		case st.Ready == nil || !*st.Ready:
			return nil, fmt.Sprintf("%s: the subject %s is not ready: %s", phase, p.subject, st.NotReady)
		}
		evidence = append(evidence, fmt.Sprintf("%s: subject %s ready%s", phase, p.subject, conditionsText(st.Conditions)))
		refusing := 0
		for _, id := range p.consuming {
			st, ok := o.Objects[id]
			switch {
			case !ok || !st.Exists || st.Unserved:
				return nil, fmt.Sprintf("%s: the consuming object %s does not exist", phase, id)
			case st.ApplyError != "":
				return nil, fmt.Sprintf("%s: the consuming object %s was not applied: %s", phase, id, st.ApplyError)
			}
			switch failure := refusal(st); {
			case failure != "":
				refusing++
				evidence = append(evidence, fmt.Sprintf("%s: %s reports a failure, %s%s%s", phase, id, failure, conditionsText(st.Conditions), eventsText(st.Warnings)))
			case st.Ready != nil && !*st.Ready:
				evidence = append(evidence, fmt.Sprintf("%s: %s not ready: %s%s", phase, id, st.NotReady, conditionsText(st.Conditions)))
			}
		}
		if refusing == 0 {
			return nil, fmt.Sprintf("%s: no consuming object reports a failure: the output may only be late", phase)
		}
		if v, ok := o.Objects[p.consumer.Verify.Secret]; ok {
			if _, has := v.Data[p.consumer.Verify.Key]; !v.Exists || !has {
				evidence = append(evidence, fmt.Sprintf("%s: the verified output %s[%s] does not exist", phase, p.consumer.Verify.Secret, p.consumer.Verify.Key))
			}
		}
	}
	if seen == 0 {
		return nil, "no object observation of the version was stored"
	}
	return evidence, ""
}

// refusal is how a controller reports that an object failed to reach its
// purpose ("" when it does not): a failure condition that holds, or a
// condition that does not hold for a reason naming a failure.
func refusal(st runtime.ObjectState) string {
	for _, c := range st.Conditions {
		if failureCondition(c) {
			return fmt.Sprintf("condition %s=%s (%s)", c.Type, c.Status, c.Reason)
		}
	}
	return ""
}

// failureReason matches condition reasons that name a failure (the
// runtime's degradation rules use the same words).
var failureReason = regexp.MustCompile(`(?i)fail|error|^err|invalid|denied|reject|forbidden`)

// failureCondition reports a condition that announces a failure: a
// negative condition type that holds, or a condition that does not hold
// for a reason that names one.
func failureCondition(c runtime.Condition) bool {
	switch strings.ToLower(c.Type) {
	case "degraded", "failed", "failure", "stalled", "error", "errored", "invalid":
		return c.Status == "True"
	}
	return c.Status != "True" && failureReason.MatchString(c.Reason)
}

func conditionsText(cs []runtime.Condition) string {
	if len(cs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		s := c.Type + "=" + c.Status
		if c.Reason != "" {
			s += " (" + c.Reason + ")"
		}
		if c.Message != "" {
			s += ": " + c.Message
		}
		parts = append(parts, s)
	}
	return "; conditions " + strings.Join(parts, "; ")
}

func eventsText(ws []string) string {
	if len(ws) == 0 {
		return ""
	}
	ws = append([]string(nil), ws...)
	sort.Strings(ws)
	return "; warning events " + strings.Join(ws, "; ")
}

// formed reports whether a consumer observation formed the relationship at
// both versions (the consumer decided: verified or failed).
func formed(a domain.ProofArm) bool {
	if a.Consumer == nil || a.Consumer.From == nil || a.Consumer.To == nil {
		return false
	}
	d := func(o string) bool { return o == runtime.ConsumerVerified || o == runtime.ConsumerFailed }
	return d(a.Consumer.From.Outcome) && d(a.Consumer.To.Outcome)
}

// cannotForm reports whether a probe could not form the relationship at
// both versions.
func cannotForm(a domain.ProofArm) bool {
	return a.Consumer != nil && a.Consumer.From != nil && a.Consumer.To != nil &&
		a.Consumer.From.Outcome == domain.ConsumerRoleCannotForm && a.Consumer.To.Outcome == domain.ConsumerRoleCannotForm
}

// formedEither reports whether the relationship formed at either version.
func formedEither(a domain.ProofArm) bool {
	if a.Consumer == nil {
		return false
	}
	for _, r := range []*domain.ProofConsumerResult{a.Consumer.From, a.Consumer.To} {
		if r != nil && (r.Outcome == runtime.ConsumerVerified || r.Outcome == runtime.ConsumerFailed) {
			return true
		}
	}
	return false
}

// consumerOutcomes writes an arm's consumer outcomes.
func consumerOutcomes(a domain.ProofArm) string {
	o := func(r *domain.ProofConsumerResult) string {
		if r == nil {
			return runtime.ConsumerUndetermined
		}
		return r.Outcome
	}
	if a.Consumer == nil {
		return "no consumer observation"
	}
	return "FROM " + o(a.Consumer.From) + ", TO " + o(a.Consumer.To)
}
