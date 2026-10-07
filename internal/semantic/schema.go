package semantic

import (
	"encoding/json"
	"fmt"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// The answer schema. Every enumerated value comes from the domain's own
// lists (families, change kinds, ops, states, consequence kinds, severities,
// unknown reasons), so the schema cannot drift from the contract. Free text
// is confined to the statement, the per-aspect undetermined reason and the
// consequence's statement/remediation.
//
// What the schema cannot express at all: an exposed class (it follows from
// the consequence kind, deterministically), a not-affected suggestion, an
// action-required request outside a consequence task (PO-2), a high confidence, a subject product (it is
// the candidate's), a citation id that was not shown (per-request enum).
//
// Transport notes (docs/phase3/model-comparison/RESULTS.md): the Anthropic
// structured-output dialect supports enum/anyOf/$ref/$defs but no recursive
// schemas and no length bounds, so the condition tree is unrolled to
// maxConditionLevels levels via $defs (each level refers to the next; the
// last level has leaf ops only) and length limits are checked client-side.
// Gateways that do not enforce the schema (GLM via Z.AI) are covered by
// validating every answer against the same schema before use.

// UndecidableReasons are the reasons a model may give an `undecidable` leaf
// (prompt v2, LOOP-DIAGNOSIS-2 L1). `undecidable` means "not statically
// decidable": exposure that depends on runtime behaviour, on evidence that
// does not state the condition, or on an identity that cannot be pinned.
// environment-visibility-gap and cross-product-context-gap are the engine's
// verdicts about one environment's missing inputs or inventory (a decidable
// predicate exists: values-key, field, text-line, cli-flag, env-var,
// feature-gate, product-version); release-knowledge-gap describes the
// absence of a fact, never a condition inside one. Those three are not a
// model's to assert, so the schema does not offer them and the answer
// validator refuses them (avoidableUndecidable).
var UndecidableReasons = []domain.UnknownReason{
	domain.UnknownRuntimeBehaviorGap, domain.UnknownEvidenceGap, domain.UnknownSemanticAmbiguity,
}

// maxConditionLevels bounds the condition tree a model can express (the
// domain allows 8; real conditions need 3–4: all → resource → ref → field).
const maxConditionLevels = 4

// Answer limits checked client-side (structured outputs cannot bound strings).
const (
	maxStatementLen   = 400
	maxReasonLen      = 400
	maxConsequenceLen = 600
	maxCitations      = 12
	maxValues         = 16
)

var leafOps = func() []domain.ConditionOp {
	var out []domain.ConditionOp
	for _, op := range domain.ConditionOps {
		switch op {
		case domain.OpAll, domain.OpAny, domain.OpNot, domain.OpResource, domain.OpRef:
			continue
		}
		out = append(out, op)
	}
	return out
}()

func strEnum[T ~string](xs []T) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = string(x)
	}
	return out
}

type obj = map[string]any

func str() obj { return obj{"type": "string"} }

func enum(xs []string) obj { return obj{"type": "string", "enum": xs} }

// scalar is a literal value as the evidence states it.
func scalar() obj {
	return obj{"anyOf": []any{obj{"type": "string"}, obj{"type": "number"}, obj{"type": "boolean"}, obj{"type": "null"}}}
}

func closed(required []string, props obj) obj {
	if required == nil {
		required = []string{}
	}
	return obj{"type": "object", "additionalProperties": false, "required": required, "properties": props}
}

func subjectIdentity(extra obj, required ...string) obj {
	props := obj{
		"family":    enum(strEnum(domain.SubjectFamilies)),
		"group":     str(),
		"version":   str(),
		"kind":      str(),
		"path":      str(),
		"name":      str(),
		"component": str(),
	}
	for k, v := range extra {
		props[k] = v
	}
	return closed(append(required, "family"), props)
}

func undeterminedSchema() obj {
	return closed([]string{"determination", "reason"}, obj{
		"determination": enum([]string{"undetermined"}),
		"reason":        str(),
	})
}

func asserted() obj { return enum([]string{"asserted"}) }

func conditionLevel(level int) obj {
	props := obj{
		"group": str(), "version": str(), "kind": str(), "name": str(), "path": str(), "component": str(),
		"state":     enum(strEnum(domain.FieldStates)),
		"values":    obj{"type": "array", "items": scalar()},
		"pattern":   str(),
		"separator": str(),
		"range":     str(),
		"reason":    enum(strEnum(UndecidableReasons)),
		"needed":    str(),
	}
	if level < maxConditionLevels {
		props["op"] = enum(strEnum(domain.ConditionOps))
		props["of"] = obj{"type": "array", "items": obj{"$ref": fmt.Sprintf("#/$defs/condition%d", level+1)}}
	} else {
		props["op"] = enum(strEnum(leafOps))
	}
	return closed([]string{"op"}, props)
}

func aspectSchema(a domain.Aspect) obj {
	var as obj
	switch a {
	case domain.AspectSubject:
		as = subjectIdentity(obj{"determination": asserted()}, "determination")
	case domain.AspectChange:
		as = closed([]string{"determination", "type"}, obj{
			"determination": asserted(),
			"type":          enum(strEnum(domain.ChangeKinds)),
			"before":        scalar(),
			"after":         scalar(),
			"replacedBy":    subjectIdentity(nil),
		})
	case domain.AspectApplicability:
		as = closed([]string{"determination", "exposure"}, obj{
			"determination": asserted(),
			"exposure":      obj{"$ref": "#/$defs/condition1"},
			"overlap":       obj{"$ref": "#/$defs/condition1"},
		})
	case domain.AspectConsequence:
		as = closed([]string{"determination", "kind", "statement"}, obj{
			"determination": asserted(),
			"kind":          enum(strEnum(domain.ConsequenceKinds)),
			"statement":     str(),
			"remediation":   str(),
			"severity":      enum(strEnum(domain.AllImpactSeverities)),
		})
	}
	return obj{"anyOf": []any{as, undeterminedSchema()}}
}

// suggestableClasses are the classes a model may suggest for a task. PO-2
// (docs/phase3/learning-loop/DECISIONS.md): action-required is a REQUEST,
// valid only with an asserted action-eligible consequence in the same
// proposal, so it is offered only to tasks that answer the consequence. It
// takes effect only through separate-call consensus or human verification.
// not-affected is never a model's to suggest.
func suggestableClasses(task domain.ProposalTask) []domain.ImpactClass {
	out := []domain.ImpactClass{domain.ImpactReviewRequired, domain.ImpactInformational, domain.ImpactUnknown}
	if containsAspect(domain.TaskAspects(task), domain.AspectConsequence) {
		out = append([]domain.ImpactClass{domain.ImpactActionRequired}, out...)
	}
	return out
}

// answerConfidences are the only confidences a model may report (the
// contract caps model confidence at medium; offering "high" and silently
// capping it would misreport what the model said).
var answerConfidences = []domain.Confidence{domain.ConfidenceMedium, domain.ConfidenceLow}

// answerSchema builds the schema of one task. citations / known, when
// non-nil, enumerate the evidence ids shown and the known fact ids (the
// per-request narrowing); nil leaves them free strings (the generic schema).
func answerSchema(task domain.ProposalTask, citations []string, known []string) obj {
	cite := str()
	if citations != nil {
		cite = enum(citations)
	}
	props := obj{
		"citations":  obj{"type": "array", "items": cite},
		"confidence": enum(strEnum(answerConfidences)),
	}
	required := []string{"citations", "confidence"}
	if task == domain.TaskDuplicate {
		dup := str()
		if known != nil {
			dup = enum(append(append([]string{}, known...), "none"))
		}
		props["duplicateOf"] = dup
		props["reason"] = str()
		required = append(required, "duplicateOf", "reason")
	} else {
		props["statement"] = str()
		props["suggestedClass"] = enum(strEnum(suggestableClasses(task)))
		required = append(required, "statement")
		for _, a := range domain.TaskAspects(task) {
			props[string(a)] = aspectSchema(a)
			required = append(required, string(a))
		}
	}
	s := closed(required, props)
	if task != domain.TaskDuplicate && containsAspect(domain.TaskAspects(task), domain.AspectApplicability) {
		defs := obj{}
		for l := 1; l <= maxConditionLevels; l++ {
			defs[fmt.Sprintf("condition%d", l)] = conditionLevel(l)
		}
		s["$defs"] = defs
	}
	return s
}

// AnswerSchema is the generic JSON Schema of a task's answer: enumerated,
// typed, every enum from the domain. Requests use a narrowed copy whose
// citation (and, for the duplicate task, fact id) enums list exactly what the
// prompt showed. nil for an unknown task.
func AnswerSchema(task domain.ProposalTask) json.RawMessage {
	if domain.TaskAspects(task) == nil {
		return nil
	}
	return mustJSON(answerSchema(task, nil, nil))
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v) // map keys are sorted: the schema bytes are deterministic
	if err != nil {
		panic(err)
	}
	return b
}

func containsAspect(xs []domain.Aspect, a domain.Aspect) bool {
	for _, x := range xs {
		if x == a {
			return true
		}
	}
	return false
}
