package main

import (
	"reflect"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// This file holds everything the generator cannot derive from the Go types
// by reflection: the closed enum sets, hand-written descriptions and the few
// constraints that go beyond the shape of the types.
//
// Every key below is checked: generate() fails when a description, patch or
// enum names a type or field that no longer exists, so this file cannot rot
// silently when the domain types change.

// enumSet lists the allowed values of a typed string whose constants form a
// closed set. Go has no way to enumerate constants by reflection, so the sets
// are listed here explicitly; TestEnumsMatchGoConstants parses the domain
// sources and fails when a constant is added, removed or renamed without
// updating this list.
type enumSet struct {
	typ    reflect.Type
	values []string
}

func enumOf[T ~string](values ...T) enumSet {
	var zero T
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return enumSet{typ: reflect.TypeOf(zero), values: out}
}

var enums = []enumSet{
	enumOf(domain.MethodDeclared, domain.MethodComputed, domain.MethodHeuristic, domain.MethodAI),
	enumOf(domain.ConfidenceHigh, domain.ConfidenceMedium, domain.ConfidenceLow),
	enumOf(
		domain.CategoryFeature, domain.CategoryBugfix, domain.CategoryDeprecation, domain.CategoryRemoval,
		domain.CategoryMigration, domain.CategoryConfiguration, domain.CategoryHelmValues, domain.CategoryCRDSchema,
		domain.CategoryAPI, domain.CategoryCompatibility, domain.CategorySecurity, domain.CategoryArtifact,
		domain.CategoryDependency, domain.CategoryOther,
	),
	enumOf(
		domain.ArtifactVerified, domain.ArtifactReferenced, domain.ArtifactExpected,
		domain.ArtifactMissing, domain.ArtifactNotApplicable,
	),
	enumOf(
		domain.SourceOK, domain.SourcePartial, domain.SourceNotFound,
		domain.SourceUnavailable, domain.SourceThrottled, domain.SourceSkipped, domain.SourceError,
	),
	enumOf(domain.ChangeAdded, domain.ChangeRemoved, domain.ChangeUpdated, domain.ChangeUnchanged),
	enumOf(domain.EnrichmentCluster, domain.EnrichmentMigrationSummary, domain.EnrichmentDiffExplanation, domain.EnrichmentRelated,
		domain.EnrichmentPlausiblyApplies, domain.EnrichmentNotApplicable, domain.EnrichmentUndetermined),
	enumOf(
		domain.EvidenceDocument, domain.EvidenceGitRef, domain.EvidenceRegistry, domain.EvidenceReleaseAsset,
		domain.EvidenceStructured, domain.EvidenceAdvisory, domain.EvidenceRepoFile,
		domain.EvidenceLocalFile, domain.EvidenceInput, domain.EvidenceRenderedDiff,
		domain.EvidenceLinkedPR, domain.EvidenceLinkedCommit,
	),
	enumOf(domain.SnapshotHelmValues, domain.SnapshotCRDs, domain.SnapshotImages, domain.SnapshotLines),
	enumOf(
		domain.ArtifactSourceRelease, domain.ArtifactHelmChart, domain.ArtifactContainerImage, domain.ArtifactOperator,
		domain.ArtifactBinary, domain.ArtifactPackage, domain.ArtifactManifest, domain.ArtifactCRD,
		domain.ArtifactDocumentation,
	),
	enumOf(
		domain.RoleVersions, domain.RoleReleaseNotes, domain.RoleChangelog, domain.RoleUpgradeGuide,
		domain.RoleCompatibility, domain.RoleSecurity,
	),
	// Not in the original brief, but the same shape (typed string constants
	// with a closed set) and it appears in every Fact.
	enumOf(
		domain.FactReleasePublished, domain.FactArtifactPublished, domain.FactDocumentRetrieved,
		domain.FactCompatibility, domain.FactSnapshot, domain.FactAdvisory, domain.FactRelationship,
		domain.FactRepresentationDivergence,
	),
	enumOf(
		domain.RepresentationSourceTree, domain.RepresentationChartTGZ, domain.RepresentationOCIChart,
		domain.RepresentationReleaseAsset, domain.RepresentationHTTPDocument, domain.RepresentationRegistryManifest,
	),
	enumOf(
		domain.ImpactActionRequired, domain.ImpactReviewRequired, domain.ImpactInformational,
		domain.ImpactNotAffected, domain.ImpactUnknown,
	),
	enumOf(domain.SeverityCritical, domain.SeverityHigh, domain.SeverityMedium, domain.SeverityLow),
	enumOf(
		domain.DimensionValues, domain.DimensionManifests, domain.DimensionCRDs,
		domain.DimensionImages, domain.DimensionCluster, domain.DimensionProducts, domain.DimensionFromVersion, domain.DimensionRender,
	),
	enumOf(
		domain.MatchValuesKey, domain.MatchAPIVersion, domain.MatchCRD, domain.MatchCRDVersion,
		domain.MatchManifestField, domain.MatchImage, domain.MatchKubernetes,
		domain.MatchProduct, domain.MatchTextLine, domain.MatchReference, domain.MatchFromVersion, domain.MatchAbsence,
		domain.MatchRenderedChange,
	),
	// --- semantic knowledge (internal/domain/semantic.go) ---
	enumOf(domain.SubjectFamilies...),
	enumOf(domain.ChangeKinds...),
	enumOf(domain.ConditionOps...),
	enumOf(domain.FieldStates...),
	enumOf(domain.ConsequenceKinds...),
	enumOf(domain.Aspects...),
	enumOf(domain.UnknownReasons...),
	enumOf(domain.VerificationLevels...),
	enumOf(domain.ReviewerHuman, domain.ReviewerProxy),
	enumOf(domain.ProposalTasks...),
	enumOf(domain.OutcomeConfirmed, domain.OutcomeRefuted, domain.OutcomeInconclusive),
	enumOf(domain.QuestionTypes...),
	enumOf(domain.ReviewPending, domain.ReviewNeedsEvidence, domain.ReviewDeferred, domain.ReviewDecided, domain.ReviewSuperseded),
	enumOf(domain.RouteAutoVerify, domain.RouteReview, domain.RouteMissingEvidence),
	enumOf(domain.PriorityHigh, domain.PriorityNormal, domain.PriorityLow),
	enumOf(domain.RoutingSignals...),
	enumOf(domain.ActionAccept, domain.ActionReject, domain.ActionCorrect, domain.ActionNeedMoreEvidence, domain.ActionDefer),
	enumOf(domain.FeedbackLabels...),
	enumOf(domain.FactActive, domain.FactRetracted, domain.FactSuperseded),
	enumOf(domain.RenderRelease, domain.RenderEnvironment),
	enumOf(domain.ConsensusCrossModel, domain.ConsensusSameModel),
	enumOf(domain.RenderAttributableChange, domain.RenderNoAttributableChange, domain.RenderUnavailable),
	enumOf(domain.RenderConfirmed, domain.RenderNotVisible, domain.RenderContradicted, domain.RenderNotApplicable),
	enumOf(domain.RenderVerifiable, domain.RenderPartiallyVerifiable, domain.RenderNotVerifiable),
	enumOf(domain.RecordCandidate, domain.RecordProposal, domain.RecordValidation, domain.RecordReviewItem, domain.RecordDecision, domain.RecordFact),
}

// notSerialised names typed-constant sets that never appear in the JSON
// output, so they have no enum in the schema.
var notSerialised = map[string]bool{
	"VersionScheme": true,
}

// Names of the generated provenance variants (see variantDefs).
const (
	defProvenance  = "Provenance"
	defDeterminist = "DeterministicProvenance"
	defAI          = "AIProvenance"
)

// fieldPatches are merged into the schema generated for one struct field,
// keyed "Type.jsonName". They encode constraints that do not follow from the
// Go type alone.
var fieldPatches = map[string]obj{
	// The envelope version is a constant, not free text.
	"UpgradeEdge.schemaVersion": o("const", domain.UpgradeEdgeSchemaVersion),

	// Provenance invariant: a Change is deterministic knowledge, an
	// Enrichment is AI output. The two never share a provenance shape.
	"Change.provenance":                  o("$ref", "#/$defs/"+defDeterminist),
	"Enrichment.provenance":              o("$ref", "#/$defs/"+defAI),
	"NoteItem.classification":            o("$ref", "#/$defs/"+defDeterminist),
	"CompatibilityConstraint.provenance": o("$ref", "#/$defs/"+defDeterminist),

	// UpgradeEdge.Validate(): every change carries at least one evidence id,
	// every provenance names its producer.
	"Change.evidence":     o("minItems", 1),
	"Provenance.producer": o("minLength", 1),

	// Enrichments never appear as Changes: the id spaces are disjoint.
	"Change.id":     o("not", o("pattern", "^"+domain.EnrichmentIDPrefix)),
	"Enrichment.id": o("pattern", "^"+domain.EnrichmentIDPrefix),
	// An enrichment says something (non-blank), about at least one change,
	// and cites at least one piece of evidence. That the ids resolve (changes
	// in `changes`, citations ⊆ provenance.inputEvidence ⊆ `evidence`) is
	// referential and checked by domain.UpgradeEdge.Validate().
	"Enrichment.content":   o("pattern", `\S`),
	"Enrichment.relatesTo": o("minItems", 1, "uniqueItems", true),
	"Enrichment.citations": o("minItems", 1, "uniqueItems", true),

	// The impact report is a deterministic document too: its findings carry
	// computed provenance and the class-specific provenance rules of
	// docs/ACTION_CLASSIFICATION.md. That the evidence ids resolve is
	// referential and checked by domain.ImpactReport.Validate().
	"ImpactReport.schemaVersion":        o("const", domain.ImpactReportSchemaVersion),
	"ImpactFinding.provenance":          o("$ref", "#/$defs/"+defDeterminist),
	"ImpactFinding.upstreamEvidence":    o("minItems", 1, "uniqueItems", true),
	"ImpactFinding.environmentEvidence": o("uniqueItems", true),
	"ImpactFinding.title":               o("pattern", `\S`),
	"ImpactMatch.evidence":              o("minItems", 1),
	"ImpactCheck.subjects":              o("minItems", 1),
}

// typePatches are appended to the schema generated for a whole struct,
// keyed by type name.
var typePatches = map[string]obj{
	"Enrichment": o("allOf", enrichmentKindRules()),
	// The action-classification contract (docs/ACTION_CLASSIFICATION.md):
	// which provenance each class must carry.
	"ImpactFinding": o("allOf", impactClassRules()),
	// "Exactly one of the typed payloads is set", and it is the one that
	// matches kind.
	"Snapshot": o(
		"oneOf", []any{
			o("required", []string{"values"}),
			o("required", []string{"crds"}),
			o("required", []string{"images"}),
			o("required", []string{"lines"}),
		},
		"allOf", []any{
			snapshotKindRule(domain.SnapshotHelmValues, "values"),
			snapshotKindRule(domain.SnapshotCRDs, "crds"),
			snapshotKindRule(domain.SnapshotImages, "images"),
			snapshotKindRule(domain.SnapshotLines, "lines"),
		},
	),
}

// impactClassRules encodes the per-class provenance rules of
// docs/ACTION_CLASSIFICATION.md as JSON Schema conditionals:
//   - affected classes carry at least one environment match and cite the
//     environment chain — except the impact:security-fix informational rule
//     (internal/impact.RuleSecurityFix; the literal is pinned by its tests),
//     which applies to every environment that upgrades and therefore carries
//     the upstream chain only;
//   - action-required carries high confidence only (below-high is demoted to
//     review-required);
//   - not-affected carries the evaluation record (checks) and no
//     neededToDetermine;
//   - unknown carries neededToDetermine; affected classes carry neither
//     checks nor neededToDetermine.
func impactClassRules() []any {
	classIs := func(classes ...domain.ImpactClass) obj {
		vals := make([]any, len(classes))
		for i, c := range classes {
			vals[i] = string(c)
		}
		return o("properties", o("classification", o("enum", vals)), "required", []string{"classification"})
	}
	// ruleIs: a conditional on the join rule (rule is required on every
	// finding); v is the rule's schema, e.g. o("const", "impact:security-fix")
	// or o("not", o("const", "impact:security-fix")).
	ruleIs := func(v any) obj {
		return o("properties", o("rule", v), "required", []string{"rule"})
	}
	all := func(cs ...obj) []any {
		out := make([]any, len(cs))
		for i, c := range cs {
			out[i] = c
		}
		return out
	}
	affected := []domain.ImpactClass{domain.ImpactActionRequired, domain.ImpactReviewRequired, domain.ImpactInformational}
	securityFix := "impact:security-fix"
	return []any{
		o("if", o("allOf", all(classIs(affected...), ruleIs(o("not", o("const", securityFix))))),
			"then", o("required", []string{"matches", "environmentEvidence"},
				"properties", o("matches", o("minItems", 1), "environmentEvidence", o("minItems", 1),
					"checks", o("maxItems", 0), "neededToDetermine", o("maxItems", 0)))),
		// impact:security-fix: universal applicability by construction — no
		// environment chain, no matches, informational only
		o("if", o("allOf", all(classIs(domain.ImpactInformational), ruleIs(o("const", securityFix)))),
			"then", o("properties", o("matches", o("maxItems", 0), "environmentEvidence", o("maxItems", 0),
				"checks", o("maxItems", 0), "neededToDetermine", o("maxItems", 0)))),
		o("if", classIs(domain.ImpactActionRequired),
			"then", o("properties", o("provenance", o("properties", o("confidence", o("const", string(domain.ConfidenceHigh))))))),
		o("if", classIs(domain.ImpactNotAffected),
			"then", o("required", []string{"checks"},
				"properties", o("checks", o("minItems", 1), "neededToDetermine", o("maxItems", 0), "matches", o("maxItems", 0)))),
		o("if", classIs(domain.ImpactUnknown),
			"then", o("required", []string{"neededToDetermine"},
				"properties", o("neededToDetermine", o("minItems", 1), "matches", o("maxItems", 0)))),
		// unknownReason is unknown-only, and every unknown states one
		o("if", o("required", []string{"unknownReason"}),
			"then", classIs(domain.ImpactUnknown)),
		o("if", classIs(domain.ImpactUnknown),
			"then", o("required", []string{"unknownReason"})),
		// the trust ladder (docs/phase3/learning-loop/DESIGN.md §4): a finding
		// evaluated from verified knowledge reaches not-affected only from a
		// trusted (deterministic/human) fact, and action-required from a
		// trusted fact or a consensus-action fact (PO-2), never from proxy
		o("if", o("allOf", all(classIs(domain.ImpactNotAffected), o("required", []string{"knowledge"}))),
			"then", o("properties", o("knowledge", o("properties", o("verification",
				o("enum", []any{string(domain.VerifiedDeterministic), string(domain.VerifiedHuman)})))))),
		o("if", o("allOf", all(classIs(domain.ImpactActionRequired), o("required", []string{"knowledge"}))),
			"then", o("properties", o("knowledge", o("anyOf", []any{
				o("properties", o("verification", o("enum", []any{string(domain.VerifiedDeterministic), string(domain.VerifiedHuman)}))),
				o("properties", o("verification", o("const", string(domain.VerifiedConsensus)), "consensusAction", o("const", true)),
					"required", []string{"consensusAction"}),
			})))),
		// PO-3: the values-default verdict rules have fixed classes
		o("if", ruleIs(o("const", domain.RuleValuesDefaultApplies)),
			"then", classIs(domain.ImpactReviewRequired)),
		o("if", ruleIs(o("enum", []any{domain.RuleValuesDefaultNoEffect, domain.RuleValuesDefaultUnrendered})),
			"then", classIs(domain.ImpactNotAffected)),
		// PO-4: a refinement comes from a trusted fact, stays affected, and
		// refinedFrom is carried exactly by impact:knowledge-refined
		o("if", o("required", []string{"refinedFrom"}),
			"then", o("allOf", all(
				ruleIs(o("const", domain.RuleKnowledgeRefined)),
				classIs(affected...),
				o("required", []string{"knowledge"}, "properties", o("knowledge", o("properties", o("verification",
					o("enum", []any{string(domain.VerifiedDeterministic), string(domain.VerifiedHuman)}))))))),
			"else", ruleIs(o("not", o("const", domain.RuleKnowledgeRefined)))),
		// knowledge is carried exactly by impact:knowledge-* rules
		o("if", o("required", []string{"knowledge"}),
			"then", ruleIs(o("pattern", "^"+domain.KnowledgeRulePrefix)),
			"else", ruleIs(o("not", o("pattern", "^"+domain.KnowledgeRulePrefix)))),
	}
}

// enrichmentKindRules: clusters and related changes connect at least two
// changes; "related" (and only "related") is a hypothesis marked unverified.
func enrichmentKindRules() []any {
	kindIs := func(kinds ...domain.EnrichmentKind) obj {
		vals := make([]any, len(kinds))
		for i, k := range kinds {
			vals[i] = string(k)
		}
		return o("properties", o("kind", o("enum", vals)), "required", []string{"kind"})
	}
	return []any{
		o("if", kindIs(domain.EnrichmentCluster, domain.EnrichmentRelated),
			"then", o("properties", o("relatesTo", o("minItems", 2)))),
		// the impact-only applicability kinds are about exactly one finding
		o("if", kindIs(domain.EnrichmentPlausiblyApplies, domain.EnrichmentNotApplicable, domain.EnrichmentUndetermined),
			"then", o("properties", o("relatesTo", o("minItems", 1, "maxItems", 1)))),
		o("if", kindIs(domain.EnrichmentRelated),
			"then", o("required", []string{"unverified"}, "properties", o("unverified", o("const", true))),
			"else", o("properties", o("unverified", o("const", false)))),
	}
}

func snapshotKindRule(kind domain.SnapshotKind, payload string) obj {
	return o(
		"if", o("properties", o("kind", o("const", string(kind))), "required", []string{"kind"}),
		"then", o("required", []string{payload}),
	)
}

// descriptions are the hand-maintained documentation of the schema, keyed
// "Type" for a type and "Type.jsonName" for a field. Only the fields whose
// meaning is not obvious from the name are documented.
var descriptions = map[string]string{
	// --- envelopes ---------------------------------------------------------
	"UpgradeEdge": "Everything relevant to upgrading a product from one version to another: the release " +
		"path that was traversed, the deterministic changes found along it (each with provenance and " +
		"evidence), compatibility and artifact deltas, the status of every consulted source, and " +
		"optional AI enrichments, which are kept apart from changes. Referential integrity (every " +
		"evidence/fact id resolves within this document, from < to) is checked by " +
		"domain.UpgradeEdge.Validate() in Go and cannot be expressed in JSON Schema.",
	"KnowledgeRecord": "One file of the learning loop's knowledge store (knowledge/<product>/<release>/<kind>/<id>.json): " +
		"exactly one entity — a semantic candidate, a model proposal (AI, never collapsed with other models), a deterministic " +
		"validation result, a review item, a review decision (human or proxy, labelled) or a verified release-level fact. " +
		"Cross-record integrity (a fact's per-aspect verification resolves to confirming validations or to decisions by a " +
		"reviewer of the claimed kind, or to separate agreeing model calls for consensus) is checked by domain.ValidateFactRecords in Go. See docs/phase3/learning-loop/DESIGN.md.",
	"Release": "Everything deterministically known about one product release, as produced by ingestion " +
		"from the sources of a product definition. UpgradeEdges are computed from Releases. Contains no AI output.",

	"UpgradeEdge.schemaVersion":    "Serialisation version of this document.",
	"UpgradeEdge.pathPolicy":       "How the traversed releases were chosen, e.g. \"minor-lineage\" (the X.Y.0 release of each line after `from`, then the patches of the target line), \"all\" (every release in (from, to]) or \"unspecified\".",
	"UpgradeEdge.path":             "Releases traversed by the upgrade, ascending; excludes `from`, ends with `to`.",
	"UpgradeEdge.skippedReleases":  "Releases between the endpoints deliberately not traversed, e.g. backport patches of intermediate lines.",
	"UpgradeEdge.sources":          "What happened when each source was consulted. Gaps are reported here (state unavailable, not-found, ...) instead of being hidden.",
	"UpgradeEdge.changes":          "Deterministic conclusions (declared, computed or heuristic), most important first. Never AI-derived.",
	"UpgradeEdge.routine":          "Summary of the routine-maintenance changes (Changes with routine set): the count and breakdown a brief shows instead of the items. Absent when nothing classifies routine.",
	"UpgradeEdge.enrichments":      "AI-derived additions (clusters, migration summaries, diff explanations, related changes). Always labelled method \"ai\" and never mixed into `changes`.",
	"UpgradeEdge.facts":            "Deterministic statements extracted from sources, without interpretation.",
	"UpgradeEdge.evidence":         "Every piece of source evidence referenced by id anywhere in this document, de-duplicated.",
	"UpgradeEdge.warnings":         "Human-readable notes about gaps or degraded input that a consumer should surface.",
	"UpgradeEdge.enrichmentRun":    "Present when enrichment was attempted: how the AI enrichments were produced and what the validator rejected.",
	"UpgradeEdge.generatedAt":      "When the edge was assembled (UTC).",
	"UpgradeEdge.definitionDigest": "Digest of the product definition revision the edge was built from.",

	"Release.product":          "Product id, e.g. \"cert-manager\".",
	"Release.publishedAt":      "Publication time reported by the canonical release channel, when known.",
	"Release.artifacts":        "Concrete artifacts (images, charts, manifests, ...) of this release. Their versions need not equal the release version.",
	"Release.notes":            "Atomic release-note / changelog / upgrade-guide entries with their deterministic classification.",
	"Release.compatibility":    "Platform compatibility constraints stated for this release.",
	"Release.snapshots":        "Structured snapshots (Helm values, CRDs, image references) used to diff releases.",
	"Release.sources":          "Outcome of consulting each source for this release.",
	"Release.ingestedAt":       "When the release was ingested (UTC).",
	"Release.definitionDigest": "Digest of the product definition revision used for ingestion; a stored release is only reusable with the same digest.",

	// --- impact ---------------------------------------------------------------
	"ImpactReport": "Which of an upgrade edge's changes matter to ONE environment, produced by joining " +
		"an UpgradeEdge with locally parsed environment inputs (values files, manifests, installed CRDs, " +
		"image references, a cluster version). Every finding cites two provenance chains: upstream evidence " +
		"copied from the edge and environment evidence pointing at the user's files. Deterministic only; " +
		"referential integrity (both chains resolve within this document) is checked by " +
		"domain.ImpactReport.Validate() in Go and cannot be expressed in JSON Schema.",
	"ImpactReport.schemaVersion":       "Serialisation version of this document.",
	"ImpactReport.environment":         "What the join ran against: the supplied cluster version, the parsed input files with digests, counts of extracted facts and parsing warnings.",
	"ImpactReport.summary":             "The funnel: all upstream changes, those with affected findings, and the explicit count of every verdict class — unknowns are counted, never folded into not-affected.",
	"ImpactReport.findings":            "One verdict per analyzed unit: affected overlaps first (action-required, review-required, informational), then unknown, then the not-affected evaluation records.",
	"ImpactReport.evidence":            "Chain 1: upstream Evidence records cited by findings, copied from the UpgradeEdge the report was built from.",
	"ImpactReport.environmentEvidence": "Chain 2: Evidence records of kind local-file / input pointing at the user's environment inputs.",
	"ImpactReport.enrichments": "AI-derived additions from the optional impact enrichment step " +
		"(`ri impact … -enrich`): applicability suggestions on unknown findings, duplicate clusters and " +
		"migration summaries. Always labelled method \"ai\" and never mixed into `findings`; the referential " +
		"checks (finding ids resolve, citations ⊆ inputEvidence ⊆ the evidence pools) are " +
		"domain.ImpactReport.Validate() in Go.",
	"ImpactReport.enrichmentRun":    "Present when impact enrichment was attempted: how the AI enrichments were produced and what the validator rejected.",
	"ImpactReport.generatedAt":      "When the report was built (UTC).",
	"ImpactReport.definitionDigest": "Digest of the product definition revision the underlying edge was built from.",
	"KnowledgeRef":                  "The verified fact a knowledge finding (rule impact:knowledge-*) was evaluated from, and the fact's verification level (its weakest aspect). Only deterministic/human facts yield not-affected; action-required needs a deterministic/human fact or a consensus fact with consensusAction (PO-2, rendered \"ACTION REQUIRED · model consensus\"); proxy facts yield neither.",
	"ImpactFinding.unknownReason":   "Why an unknown finding is unknown: release-knowledge-gap, environment-visibility-gap, cross-product-context-gap, runtime-behavior-gap, evidence-gap or semantic-ambiguity (docs/phase3/learning-loop/DESIGN.md §1.5). Unknown-only.",
	"ImpactFinding": "One deterministic verdict of the applicability engine: an upstream change (or compatibility " +
		"constraint, or moved image artifact) met the environment — or could not be evaluated. Affected classes " +
		"(action-required / review-required / informational) cite both evidence chains; not-affected carries the " +
		"evaluation record (`checks`); unknown carries `neededToDetermine`. See docs/ACTION_CLASSIFICATION.md.",
	"ImpactFinding.classification":      "What to do (docs/ACTION_CLASSIFICATION.md): action-required — the environment must change to avoid concrete failure, evidenced on both chains, high confidence only; review-required — credible overlap, applicability not deterministically provable; informational — evidenced overlap with no action implied; not-affected — checked against a supplied environment dimension and clear; unknown — applicability undeterminable, `neededToDetermine` says why. The impact:security-fix informational findings are the one affected shape with no environment chain: the fix ships with the target, so applicability is universal and configuration-independent.",
	"ImpactFinding.severity":            "How bad if it bites (independent axis): critical (upgrade fails outright), high (concrete degradation), medium (needs a look), low (confirmed no-action overlap). Absent for not-affected/unknown.",
	"ImpactFinding.rule":                "Join rule that fired, e.g. \"impact:values-removed\"; verdict records use impact:values-unset / impact:not-joined / impact:insufficient-visibility / ... .",
	"ImpactFinding.detail":              "Prose explaining the verdict: what upstream changed and which environment fact matched — or what could not be checked, and why.",
	"ImpactFinding.changeId":            "Id of the upstream Change in the UpgradeEdge this report was built from; absent when the finding comes from a compatibility constraint or artifact move alone.",
	"ImpactFinding.changeTitle":         "Copy of the upstream change's title, so the report renders standalone.",
	"ImpactFinding.changeCategory":      "Copy of the upstream change's category.",
	"ImpactFinding.changeBreaking":      "Copy of the upstream change's breaking flag.",
	"ImpactFinding.matches":             "The environment facts that made the finding fire, each with its own environment evidence (affected classes only).",
	"ImpactFinding.upstreamEvidence":    "Chain 1: Evidence ids resolving in `evidence` (every class).",
	"ImpactFinding.environmentEvidence": "Chain 2: Evidence ids resolving in `environmentEvidence` (affected classes; check evidence for verdict records).",
	"ImpactFinding.checks":              "Evaluation record of a not-affected verdict (and the partial record of an unknown one): which environment dimension was consulted, how many facts were compared, which upstream subjects were compared.",
	"ImpactFinding.neededToDetermine":   "What evidence was missing for an unknown verdict, e.g. \"Helm values files (--values) not supplied\"; unknown-only by contract.",
	"ImpactFinding.suggestedClassification": "The AI layer's review suggestion for an unknown finding (\"review-required\" only; never " +
		"action-required). The deterministic classification is not overwritten: the finding stays as the join " +
		"produced it and the suggestion is carried by a plausibly-applies enrichment with full AI provenance.",
	"ImpactCheck":                   "One entry of an evaluation record: what was checked against what, so a not-affected verdict is auditable without re-running anything.",
	"ImpactCheck.dimension":         "The environment input class consulted: values, manifests, crds, images, or cluster-version (platform names it).",
	"ImpactCheck.platform":          "Cluster platform of a cluster-version check (\"kubernetes\", \"openshift\", ...).",
	"ImpactCheck.facts":             "How many environment facts of that dimension were compared (0 when the input was supplied but yielded none).",
	"ImpactCheck.subjects":          "The upstream subjects compared against those facts.",
	"ImpactCheck.evidence":          "Environment evidence proving a directly-supplied input (the --kubernetes / --images flag); file-backed dimensions are audited through environment.files digests.",
	"ImpactMatch":                   "One environment fact that matched: what it is (subject) and the local evidence that proves the environment has it.",
	"ImpactMatch.kind":              "What kind of environment fact: a set values key, an apiVersion in use, an installed CRD or one of its versions, a manifest field path, an image in use, or the cluster Kubernetes version.",
	"ImpactMatch.subject":           "The fact itself: a values key path, \"group/version Kind\", a CRD name, a field path, an image reference or a version string.",
	"ImpactMatch.evidence":          "Environment evidence ids backing this match.",
	"ImpactSummary":                 "Counts of the impact funnel; must equal the findings (checked by Validate). Unknowns are counted explicitly, never folded into not-affected.",
	"ImpactSummary.suggestedReview": "How many unknown findings carry an AI review suggestion; they still count under unknown. 0 without -enrich.",
	"ImpactFile":                    "One environment input file with the digest of the bytes that were parsed.",
	"ImpactFile.path":               "Path exactly as supplied on the command line (evidence URIs use the same form).",
	"ImpactEnvironment":             "Summary of the environment inputs the join consumed.",
	"ImpactEnvironment.kubernetes":  "Cluster Kubernetes version as supplied (e.g. \"1.31\" or \"1.31.5\").",

	// --- versions ---------------------------------------------------------
	"Version":         "A release version of a product.",
	"Version.tag":     "Identifier exactly as published by the canonical release channel, e.g. \"v1.18.0\".",
	"Version.version": "Normalised semantic version without any tag prefix, e.g. \"1.18.0\".",
	"PathStep.reason": "Why the release is traversed, e.g. \"release\", \"patch\", \"major-release\", \"minor-release\", \"line-entry\" or \"target-line-patch\".",

	// --- provenance (the AI / deterministic boundary) -----------------------
	"Provenance": "How a Change, note classification, compatibility constraint or Enrichment was produced. " +
		"This is the generic shape; documents use DeterministicProvenance where only deterministic methods are " +
		"allowed (Change.provenance, NoteItem.classification, CompatibilityConstraint.provenance) and " +
		"AIProvenance for Enrichment.provenance.",
	defDeterminist: "Provenance of deterministic knowledge: method is declared, computed or heuristic, and the " +
		"AI-only fields (model, modelVersion, promptVersion, promptDigest, inputEvidence, generatedAt) must be absent. " +
		"A Change can never be method \"ai\".",
	defAI: "Provenance of AI output: method is \"ai\" and model, modelVersion, promptVersion, promptDigest, " +
		"inputEvidence (at least one evidence id) and generatedAt are required, so every enrichment can be traced " +
		"to the model that answered, the exact prompt and the evidence it was given.",
	"Provenance.method": "How the knowledge was derived. " +
		"declared: upstream stated it explicitly in a structured or labelled way (an item under a \"Breaking Changes\" heading, " +
		"a release-note YAML with \"action required\", a published support matrix). " +
		"computed: deterministic computation over source data (a diff of two values.yaml files or two CRD sets). " +
		"heuristic: deterministic but pattern-based interpretation (keyword matching such as \"deprecated\" inside a bug-fix bullet); " +
		"treat with more caution than declared or computed. " +
		"ai: produced by a language model; only valid on Enrichments.",
	"Provenance.producer":      "Component and version that produced the record, e.g. \"normalize.notes@v1\".",
	"Provenance.rule":          "Identifier of the rule that fired, e.g. \"section:/breaking/i\".",
	"Provenance.confidence":    "Coarse confidence in the classification.",
	"Provenance.model":         "AI only: identifier of the model that answered, as reported by the API response or the exchange response file (never assumed from the request).",
	"Provenance.modelVersion":  "AI only: the most precise version identifier of that model reported with the answer (a pinned snapshot id when the provider reports no separate version).",
	"Provenance.promptVersion": "AI only: version of the prompt templates, e.g. \"enrich/v1\"; promptDigest pins the exact rendered prompt.",
	"Provenance.promptDigest":  "AI only: digest of the exact prompt, so the generation can be reproduced or audited.",
	"Provenance.inputEvidence": "AI only: ids of the Evidence records the model was given as input.",
	"Provenance.generatedAt":   "AI only: when the output was generated.",

	// --- changes -------------------------------------------------------------
	"Change": "A deterministic conclusion about the upgrade: from a release-note item (declared or heuristic) or " +
		"from a computed diff of two releases (values, CRDs, images, compatibility, advisories).",
	"Change.category": "Primary classification of the change.",
	"Change.breaking": "True when upgrading across this change can break existing deployments or integrations, " +
		"as declared upstream or computed (for example a removed Helm value or CRD version). Omitted when false.",
	"Change.actionRequired": "True when operators must take a manual step before, during or after the upgrade " +
		"(migration, config change, manifest edit). Omitted when false. A change can be action-required without being breaking.",
	"Change.title":    "One-line summary.",
	"Change.release":  "Version (semver) of the release that introduced the change; omitted for diffs between the two endpoints.",
	"Change.subjects": "What the change affects: Helm value paths, CRD names, image repositories, API versions.",
	"Change.facts":    "Ids of the Facts this conclusion is based on.",
	"Change.evidence": "Ids of the Evidence records that support the change; at least one, each resolving within the edge's `evidence`.",
	"Change.routine": "True for release-note maintenance churn (dependency bumps, UI-only work, CI/docs/test/build " +
		"noise, metric renames) found by the deterministic routine detector. Routine changes are never breaking, " +
		"security-relevant or directive; they stay in `changes` with their evidence, but are kept out of the default " +
		"upgrade narrative and summarised in `routine`. Omitted when false.",
	"Change.routineKind": "Why the change is routine: dependency (version bumps), ui, housekeeping (CI/docs/test/build " +
		"churn) or metrics (renames/deprecations). Set only when routine is true.",
	"RoutineSummary":         "Count and breakdown of the routine-maintenance changes of an edge.",
	"RoutineSummary.byKind":  "Routine changes per kind (dependency, ui, housekeeping, metrics).",
	"RoutineSummary.summary": "Human-readable breakdown, e.g. \"27 dependency bumps, 14 UI/docs/CI churn\".",
	"Reference":              "An external identifier mentioned by a source.",
	"Reference.type":         "Kind of identifier, e.g. \"cve\", \"ghsa\", \"pull-request\", \"issue\" or \"url\".",

	// --- AI ------------------------------------------------------------------
	"Enrichment": "AI-derived information that groups, summarises, connects or explains deterministic Changes. " +
		"It never replaces or modifies a Change or a source fact: it refers to Changes by id and cites the evidence " +
		"it relies on. It carries AIProvenance (model, model version, prompt version and digest, input evidence, " +
		"generation time). Consumers must present it as AI-generated and verify it against the cited evidence.",
	"Enrichment.id": "Identifier starting with \"enr-\"; Change ids never use that prefix.",
	"Enrichment.kind": "cluster: semantically equivalent Changes from different sources consolidated into one conclusion " +
		"(≥2 changes). migration-summary: one migration requirement summarised from one or more Changes. " +
		"diff-explanation: why a computed diff matters, citing the release-note statement that explains it. " +
		"related: Changes that are potentially related; a hypothesis that requires verification (≥2 changes, unverified).",
	"Enrichment.relatesTo":  "Ids of the Changes the enrichment is about; each resolves within the edge's `changes`.",
	"Enrichment.citations":  "Evidence ids the enrichment relies on: a subset of provenance.inputEvidence, which is a subset of the edge's `evidence`.",
	"Enrichment.unverified": "True exactly for kind \"related\": the relation was suggested by the model and is not established by the citations.",
	"EnrichmentRun": "How the enrichments of this edge were produced: deterministic candidate groups, prompts, answers " +
		"pending or rejected by the validator, and the duplicate metric. Metadata about AI output, never a conclusion.",
	"EnrichmentRun.candidateGroups":        "Deterministic candidate groups (changes sharing subjects, references, wording, a diffed key, or an upgrade-guide/release-note pairing) found before any model was asked.",
	"EnrichmentRun.pending":                "Requests written to the file exchange that have no response yet.",
	"EnrichmentRun.rejected":               "Model proposals refused by the validator (unknown change ids, citations outside the input, empty content, ...).",
	"EnrichmentRun.duplicatesConsolidated": "clusteredChanges - clusters: how many Changes duplicate another Change of the same cluster.",

	// --- evidence and facts ------------------------------------------------
	"Evidence": "A verifiable pointer to source material supporting a fact or conclusion. `uri` is something a " +
		"human can open; `locator` narrows it down; `excerpt` and `contentDigest` pin what was read.",
	"EvidenceID":        "Stable identifier derived from the evidence content (\"ev-\" followed by a short hash).",
	"Evidence.sourceId": "Id of the product-definition source or artifact that produced the evidence.",
	"Evidence.uri":      "Location a human or a later run can open to check the claim.",
	"Evidence.locator":  "Position within the document, e.g. a line range (\"L120-L131\"), a heading (\"## Breaking Changes\") or a JSONPath (\"$.spec.versions[1]\").",
	"Evidence.excerpt":  "Short verbatim excerpt of the source, truncated.",
	"Representation": "Which published form of an artifact a fact came from. The same content (chart defaults, " +
		"CRDs, metadata) can exist in the source tree and in several artifacts users actually consume, and publishers " +
		"rewrite content at packaging time; the representation keeps the two apart. Orthogonal to " +
		"Provenance.method (how knowledge was derived).",
	"Evidence.contentDigest": "sha256 of the complete retrieved document, so the exact bytes the conclusion was drawn from can be identified later.",
	"Evidence.representation": "Which published form of the artifact these bytes came from: source-tree (a file in the " +
		"source repository), published-chart-tgz (a chart tarball from a chart repository), published-oci-chart (a chart " +
		"pulled as an OCI artifact), release-asset (a file attached to a release), http-document (a plain HTTP document) " +
		"or registry-manifest (an OCI manifest/config). Orthogonal to Provenance.method, which states how knowledge was " +
		"derived, not from which bytes.",
	"Evidence.retrievedAt": "When the source was fetched (UTC).",
	"Fact": "A deterministic statement extracted from sources, with evidence. Facts hold no interpretation; " +
		"interpretation lives in Changes.",
	"FactID":         "Stable identifier derived from the fact content (\"fact-\" followed by a short hash).",
	"Fact.subject":   "What the fact is about, e.g. \"cert-manager@1.18.0\" or an artifact coordinate.",
	"Fact.release":   "Semver of the release the fact is about.",
	"Fact.extractor": "Component and version that extracted the fact, e.g. \"normalize.notes@v1\".",

	// --- sources -------------------------------------------------------------
	"SourceStatus":         "What happened when a source was consulted; lets consumers tell \"nothing found\" from \"could not look\".",
	"SourceStatus.kind":    "Locator kind, e.g. \"github-releases\", \"repo-file\" or \"oci\".",
	"SourceStatus.version": "Release the status refers to; omitted for product-level sources.",
	"SourceStatus.state":   "ok; partial (some data missing); not-found (reachable, nothing for this version); unavailable (unreachable, blocked or auth required); throttled (reachable but rate limited; retrying later is expected to work); skipped (not applicable or disabled); error.",

	// --- artifacts -----------------------------------------------------------
	"ArtifactInstance":            "One concrete artifact (image, chart, manifest, ...) belonging to a release.",
	"ArtifactInstance.artifactId": "Id of the artifact in the product definition.",
	"ArtifactInstance.coordinate": "Where to find it, e.g. \"quay.io/jetstack/cert-manager-controller:v1.18.0\".",
	"ArtifactInstance.version":    "The artifact's own version; a chart version may differ from the application version.",
	"ArtifactInstance.channel":    "Publication channel that confirmed the artifact.",
	"ArtifactInstance.status": "How strongly the artifact is confirmed: verified (observed at its channel), referenced " +
		"(named by another verified artifact of the same release), expected (predicted by the definition only; channel " +
		"unreachable), missing (channel reachable, artifact absent), not-applicable (does not exist for this release).",
	"ArtifactChange":        "Comparison of one artifact between the two endpoints of an edge.",
	"ArtifactChange.change": "added, removed, updated or unchanged between `from` and `to`.",

	// --- compatibility -------------------------------------------------------
	"CompatibilityConstraint":            "A platform requirement stated for a release, e.g. Kubernetes \"1.29 - 1.33\".",
	"CompatibilityConstraint.platform":   "Platform name, e.g. \"kubernetes\", \"openshift\" or \"helm\".",
	"CompatibilityConstraint.constraint": "Normalised semver constraint when derivable, e.g. \">=1.29.0-0 <=1.33.x\".",
	"CompatibilityConstraint.versions":   "Explicit platform versions when the source enumerates them.",
	"CompatibilityConstraint.raw":        "Verbatim value from the source.",
	"CompatibilityConstraint.kind":       "e.g. \"supported\", \"tested\", \"minimum\" or \"chart-kubeVersion\".",
	"CompatibilityChange":                "Comparison of one platform constraint between the two endpoints of an edge.",
	"CompatibilityChange.narrowed":       "True when the target supports a strict subset of the platform versions the source supported (support for an older platform version was dropped).",

	// --- release contents ----------------------------------------------------
	"NoteItem":                "One atomic entry of a release note, changelog or upgrade guide as published, with its deterministic classification.",
	"NoteItem.release":        "Semver of the release that introduced the entry.",
	"NoteItem.section":        "Heading path the entry was found under, e.g. \"Breaking Changes\".",
	"NoteItem.breaking":       "Entry is declared or classified as breaking. Omitted when false.",
	"NoteItem.actionRequired": "Entry requires a manual step. Omitted when false.",
	"NoteItem.classification": "How category, breaking and actionRequired were determined.",
	"Snapshot": "A deterministic, structured view of an artifact used to diff two releases. Exactly one payload " +
		"(values, crds or images) is present and it matches `kind`.",
	"ValuesSnapshot":             "Flattened Helm values: dotted path to JSON-encoded default value. Lists are leaf values.",
	"ValuesSnapshot.entries":     "Dotted value path to its default value, JSON-encoded as a string: the Helm value 3 is stored as \"3\", the string x as \"\\\"x\\\"\".",
	"ValuesSnapshot.comments":    "Doc comment directly above a key, when present.",
	"CRDVersionInfo.schemaPaths": "Dotted property paths of the openAPIV3Schema (e.g. \"spec.secretTemplate.labels\"); used to detect removed or added fields.",
}
