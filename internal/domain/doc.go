// Package domain defines the vendor-neutral core model of the release
// intelligence platform: products, releases, artifacts, sources, evidence,
// facts, changes and upgrade edges.
//
// The package has no knowledge of GitHub, Helm, OCI registries or Kubernetes
// specifics beyond plain data. Everything here is serialisable to JSON and is
// intended to be the stable contract between ingestion, normalisation,
// upgrade analysis and presentation.
//
// Two kinds of knowledge are kept strictly apart:
//
//   - Facts and Changes are produced by deterministic code. Every Change
//     carries a Provenance whose Method is Declared, Computed or Heuristic.
//   - Enrichments are AI-derived. They always carry Method AI together with
//     the model and model version, the prompt version and digest, the
//     evidence used as input and the evidence they cite. They only refer to
//     Changes by id and are never merged into Changes.
package domain
