// Package discovery is the AI-assisted source-discovery workflow. Given an
// upstream repository it proposes a catalog.ProductDefinition together with
// an evidence-backed discovery report. It runs rarely (onboarding a product,
// or when validated patterns drift); recurring ingestion never calls it.
//
// The pipeline is a chain of small, separately testable stages:
//
//  1. Checkout  obtains local trees: the product repository at a ref (by
//     default its latest stable tag), plus lightweight partial clones of
//     external documentation / chart repositories it references.
//  2. Scanner   runs deterministic, product-agnostic detectors over the trees
//     and the tag list and emits Candidates. Every candidate carries
//     domain.Evidence pointing at a file pinned to the scanned ref, a line
//     locator ("L12") and the matching line as excerpt.
//  3. Resolver  turns candidates into a draft definition with ranking and
//     templating rules (concrete versions in paths and tags become
//     {{.Tag}}, {{.Version}}, {{.Major}}.{{.Minor}} …). Each choice is a
//     Decision with a rule id. An optional LLM resolver answers small,
//     focused questions about ambiguity (several registries, docs living in
//     another repository, chart ↔ app versions); every answer becomes a
//     Proposal with AI provenance (model, prompt digest, input evidence).
//  4. Validator checks the draft (and every AI proposal) against at least
//     ingest.MinValidations recent stable releases through a
//     RelationshipChecker (the real one is (*ingest.Ingester).CheckRelationships).
//     An AI proposal enters the definition only when it validates.
//  5. Proposer  emits the definition (it always passes catalog.Validate) and
//     a JSON-serialisable Report with a markdown rendering.
package discovery

// Producer identifiers recorded in provenance.
const (
	ProducerScan     = "discovery.scan@v1"
	ProducerResolve  = "discovery.resolve@v1"
	ProducerLLM      = "discovery.llm@v1"
	ProducerValidate = "discovery.validate@v1"
)
