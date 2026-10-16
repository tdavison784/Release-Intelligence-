package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// DeclaredExercise is a catalog exercise with the product that declares it.
type DeclaredExercise struct {
	Product  string           `json:"product"`
	File     string           `json:"-"`                // the declaring file, as loaded (not recorded: machine-specific)
	Digest   string           `json:"digest,omitempty"` // of the declaring file
	Exercise catalog.Exercise `json:"exercise"`
}

// Key is "<product>/<id>".
func (d DeclaredExercise) Key() string { return d.Product + "/" + d.Exercise.ID }

// covers reports whether the exercise exercises items of a family.
func (d DeclaredExercise) covers(f domain.ChangeItemFamily) bool {
	if len(d.Exercise.Families) == 0 {
		return true
	}
	for _, x := range d.Exercise.Families {
		if x == string(f) {
			return true
		}
	}
	return false
}

// targets reports whether an object is a target of the exercise.
func (d DeclaredExercise) targets(r ObjectRef) bool {
	return r.Group == d.Exercise.Target.Group && r.Kind == d.Exercise.Target.Kind
}

// FromSets flattens loaded exercise files, in product then id order.
func FromSets(sets ...*catalog.ExerciseSet) []DeclaredExercise {
	var out []DeclaredExercise
	for _, s := range sets {
		if s == nil {
			continue
		}
		for _, x := range s.Exercises {
			out = append(out, DeclaredExercise{Product: s.Product, File: s.Path(), Digest: s.Digest(), Exercise: x})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

// ExerciseRun records one exercise at one version.
type ExerciseRun struct {
	Exercise string `json:"exercise"` // <product>/<id>
	Phase    string `json:"phase"`    // "from" | "to"
	Action   string `json:"action"`
	// Targets are the live objects of the target kind; Executed those the
	// action was applied to; Failed why it failed on the others.
	Targets  []string          `json:"targets"`
	Executed []string          `json:"executed,omitempty"`
	Failed   map[string]string `json:"failed,omitempty"`
	Settle   float64           `json:"settleSeconds"`
}

// runExercise applies one exercise to every target among the live objects.
func runExercise(ctx context.Context, cl Cluster, d DeclaredExercise, phase, token string, live map[string]map[string]any, now time.Time) ExerciseRun {
	er := ExerciseRun{Exercise: d.Key(), Phase: phase, Action: string(d.Exercise.Action)}
	var ids []string
	for id := range live {
		if r, ok := ParseRef(id); ok && d.targets(r) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	er.Targets = ids
	for _, id := range ids {
		r, _ := ParseRef(id)
		sub, patch, err := exercisePatch(d.Exercise, live[id], token, now)
		if err == nil {
			err = cl.Patch(ctx, r, sub, patch)
		}
		if err != nil {
			if er.Failed == nil {
				er.Failed = map[string]string{}
			}
			er.Failed[id] = clip(err.Error(), 400)
			continue
		}
		er.Executed = append(er.Executed, id)
	}
	return er
}

// exercisePatch builds the merge patch (and subresource) of an action.
func exercisePatch(x catalog.Exercise, obj map[string]any, token string, now time.Time) (string, []byte, error) {
	switch x.Action {
	case catalog.ActionAnnotate:
		b, err := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": map[string]string{x.Annotation: token}}})
		return "", b, err
	case catalog.ActionRolloutRestart:
		b, err := json.Marshal(map[string]any{"spec": map[string]any{"template": map[string]any{"metadata": map[string]any{
			"annotations": map[string]string{"kubectl.kubernetes.io/restartedAt": now.UTC().Format(time.RFC3339)}}}}})
		return "", b, err
	case catalog.ActionStatusCondition:
		c := x.Condition
		meta, _ := obj["metadata"].(map[string]any)
		status, _ := obj["status"].(map[string]any)
		cond := map[string]any{"type": c.Type, "status": c.Status, "reason": c.Reason, "message": c.Message,
			"lastTransitionTime": now.UTC().Format(time.RFC3339)}
		if g := toInt(meta["generation"]); g > 0 {
			cond["observedGeneration"] = g
		}
		// A merge patch replaces the list: keep every other condition.
		conds := []any{cond}
		raw, _ := status["conditions"].([]any)
		for _, e := range raw {
			if m, ok := e.(map[string]any); ok && m["type"] != c.Type {
				conds = append(conds, m)
			}
		}
		b, err := json.Marshal(map[string]any{"status": map[string]any{"conditions": conds}})
		return "status", b, err
	}
	return "", nil, fmt.Errorf("unknown exercise action %q", x.Action)
}
