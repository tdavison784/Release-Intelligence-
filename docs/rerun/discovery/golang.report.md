# Discovery report: go

- Repository: `github.com/golang/go` at `go1.27.1` (commit `862c888e612a…`)
- Generated: 2026-10-02T12:34:51Z
- LLM: not used (deterministic resolver only)
- Tags: 496 tags: prefix "go", 269 stable, 0 prereleases (), 227 junk; latest stable go1.27.1; lineage minor
- Scanned `github.com/golang/go@go1.27.1` (source profile): 14542 files listed, 1631 read
- Validation: done against go1.25.0, go1.25.14, go1.26.0, go1.26.8, go1.27.0, go1.27.1

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 1 | git-tags github.com/golang/go |
| release-notes | not-found | 0 |  |
| changelog | not-found | 0 |  |
| helm-charts | not-found | 0 |  |
| registries | not-found | 0 |  |
| images | not-found | 0 |  |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 1 | github-advisories golang/go |
| version-relations | not-found | 0 |  |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| source:advisories | discovered | security.github-hosted-default | https://github.com/golang/go/blob/go1.27.1/SECURITY.md#L1 |
| source:tags | discovered | versions.git-tags | https://github.com/golang/go/releases/tag/go1.27.1#refs/tags/go1.27.1 |
| versioning | discovered | tags.scheme | https://github.com/golang/go/releases/tag/go1.27.1#refs/tags/go1.27.1 |

Statuses: 3 discovered.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: golang
name: go
homepage: https://github.com/golang/go
versioning:
  scheme: semver
  tagPrefix: go
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/golang/go
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: golang/go
    notes: 'Security policy: SECURITY.md → https://go.dev/security/policy'
artifacts: []
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - go1.25.0
    - go1.25.14
    - go1.26.0
    - go1.26.8
    - go1.27.0
    - go1.27.1
  notes: Proposed by automated discovery; relationship checks run against go1.25.0, go1.25.14, go1.26.0, go1.26.8, go1.27.0, go1.27.1.
```

## Validation matrix

| Element | Verdict | go1.25.0 | go1.25.14 | go1.26.0 | go1.26.8 | go1.27.0 | go1.27.1 |
|---|---|---|---|---|---|---|---|

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 496 tags: prefix "go", 269 stable, 0 prereleases (), 227 junk; latest stable go1.27.1; lineage minor |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (496 tags: prefix "go", 269 stable, 0 prereleases (), 227 junk; latest stable go1.27.1; lineage minor). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |

## Candidates


### security-policy (1)

- `SECURITY.md` (high; docs.security-policy) — [golang/go:SECURITY.md L1](https://github.com/golang/go/blob/go1.27.1/SECURITY.md#L1): `# Security Policy`

### tag-scheme (1)

- `^go(?P<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$` (high; tags.ls-remote) — [golang/go/releases/tag/go1.27.1 refs/tags/go1.27.1](https://github.com/golang/go/releases/tag/go1.27.1#refs/tags/go1.27.1): `862c888e612ac346c7c4d99c9392bdfd265f33b0 refs/tags/go1.27.1`
