# Discovery report: PostgreSQL

- Repository: `github.com/postgres/postgres` at `master` (commit `45277ca0d1cb…`)
- Generated: 2026-10-01T11:50:46Z
- LLM: not used (deterministic resolver only)
- Tags: 693 tags: prefix "", 0 stable, 0 prereleases (), 693 junk; latest stable ; lineage linear
- Scanned `github.com/postgres/postgres@master` (source profile): 7699 files listed, 521 read
- Validation: skipped (only 0 stable releases available, 3 required)

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 1 | git-tags github.com/postgres/postgres |
| release-notes | not-found | 0 |  |
| changelog | not-found | 0 |  |
| helm-charts | not-found | 0 |  |
| registries | not-found | 0 |  |
| images | candidates-only | 3 |  |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 1 | github-advisories postgres/postgres |
| version-relations | not-found | 0 |  |

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: postgresql
name: PostgreSQL
homepage: https://github.com/postgres/postgres
versioning:
  scheme: semver
  lineage: linear
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/postgres/postgres
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: postgres/postgres
    notes: 'Security policy: .github/SECURITY.md → https://www.postgresql.org/support/security/>'
artifacts: []
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-01"
  notes: Proposed by automated discovery; NOT validated against releases (only 0 stable releases available, 3 required). Review before use.
```

## Open questions for the reviewer

- No stable release tags were found; the tag scheme could not be inferred.
- Run the relationship checks (validation) before adopting this definition.

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 693 tags: prefix "", 0 stable, 0 prereleases (), 693 junk; latest stable ; lineage linear |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (693 tags: prefix "", 0 stable, 0 prereleases (), 693 junk; latest stable ; lineage linear). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |

<details><summary>3 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| image `ghcr.io/anarazel/pg-vm-images/main` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/anarazel/pg-vm-images/main/linux_debian_trixie_ci` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/anarazel/pg-vm-images/main/linux_debian_trixie_ci_docs` | image.third-party | Image is not named after the product (dependency or tooling image). |

</details>

## Candidates


### image (3)

- `ghcr.io/anarazel/pg-vm-images/main` (medium; workflow.image-ref) — [postgres/postgres:.github/workflows/pg-ci.yml L125](https://github.com/postgres/postgres/blob/45277ca0d1cb6dbe722a01b3b5461c30abd40eee/.github/workflows/pg-ci.yml#L125): `CONTAINER_REPO: ghcr.io/anarazel/pg-vm-images/main`
- `ghcr.io/anarazel/pg-vm-images/main/linux_debian_trixie_ci` (medium; workflow.image-ref) — [postgres/postgres:.github/workflows/pg-ci.yml L207](https://github.com/postgres/postgres/blob/45277ca0d1cb6dbe722a01b3b5461c30abd40eee/.github/workflows/pg-ci.yml#L207): `container_linux_ci: ${{ env.CONTAINER_REPO }}/${{ env.CONTAINER_LINUX_CI }}`
- `ghcr.io/anarazel/pg-vm-images/main/linux_debian_trixie_ci_docs` (medium; workflow.image-ref) — [postgres/postgres:.github/workflows/pg-ci.yml L208](https://github.com/postgres/postgres/blob/45277ca0d1cb6dbe722a01b3b5461c30abd40eee/.github/workflows/pg-ci.yml#L208): `container_linux_ci_docs: ${{ env.CONTAINER_REPO }}/${{ env.CONTAINER_LINUX_CI_DOCS }}`

### security-policy (1)

- `.github/SECURITY.md` (high; docs.security-policy) — [postgres/postgres:.github/SECURITY.md L1](https://github.com/postgres/postgres/blob/45277ca0d1cb6dbe722a01b3b5461c30abd40eee/.github/SECURITY.md#L1): `For information about reporting security issues, see`

### tag-scheme (1)

- `^(?P<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$` (low; tags.ls-remote) — [postgres/postgres/releases/tag/PG95-1_01 refs/tags/PG95-1_01](https://github.com/postgres/postgres/releases/tag/PG95-1_01#refs/tags/PG95-1_01): `d31084e9d1118b25fd16580d9d8c2924b5740dff refs/tags/PG95-1_01`
