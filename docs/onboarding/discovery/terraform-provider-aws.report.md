# Discovery report: terraform-provider-aws

- Repository: `github.com/hashicorp/terraform-provider-aws` at `v6.67.0` (commit `e449e05eb24b…`)
- Generated: 2026-10-01T15:36:32Z
- LLM: not used (deterministic resolver only)
- Tags: 521 tags: prefix "v", 518 stable, 3 prereleases (betaN×3), 0 junk; latest stable v6.67.0; lineage linear
- Scanned `github.com/hashicorp/terraform-provider-aws@v6.67.0` (source profile): 21055 files listed, 3581 read
- Validation: done against v6.64.0, v6.65.0, v6.66.0, v6.67.0

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 2 | git-tags github.com/hashicorp/terraform-provider-aws |
| release-notes | not-found | 0 |  |
| changelog | found | 1 | repo-file github.com/hashicorp/terraform-provider-aws CHANGELOG.md |
| helm-charts | not-found | 0 |  |
| registries | candidates-only | 218 |  |
| images | candidates-only | 3 |  |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 0 | github-advisories hashicorp/terraform-provider-aws |
| version-relations | not-found | 0 |  |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| source:changelog | historically-validated | changelog.file | https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/CHANGELOG.md#L1 |
| source:advisories | discovered | security.github-hosted-default |  |
| source:tags | discovered | versions.git-tags | https://github.com/hashicorp/terraform-provider-aws/releases/tag/v6.67.0#refs/tags/v6.67.0 |
| versioning | discovered | tags.scheme | https://github.com/hashicorp/terraform-provider-aws/releases/tag/v6.67.0#refs/tags/v6.67.0 (+1) |

Statuses: 1 historically-validated, 3 discovered.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: terraform-provider-aws
name: terraform-provider-aws
homepage: https://github.com/hashicorp/terraform-provider-aws
versioning:
  scheme: semver
  tagPrefix: v
  lineage: linear
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/hashicorp/terraform-provider-aws
  - id: changelog
    roles:
      - changelog
    locator:
      kind: repo-file
      repository: github.com/hashicorp/terraform-provider-aws
      path: CHANGELOG.md
    extract:
      type: markdown-section
      heading: ^\[?v?{{regexQuote .Version}}\]?(\s|$)
    validatedAgainst:
      - 6.64.0
      - 6.65.0
      - 6.66.0
      - 6.67.0
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: hashicorp/terraform-provider-aws
artifacts: []
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-01"
  validatedReleases:
    - v6.64.0
    - v6.65.0
    - v6.66.0
    - v6.67.0
  notes: Proposed by automated discovery; relationship checks run against v6.64.0, v6.65.0, v6.66.0, v6.67.0.
```

## Open questions for the reviewer

- Ambiguity (registry-roles): 218 registry hosts/namespaces are referenced for product images; which are release channels?

## Validation matrix

| Element | Verdict | v6.64.0 | v6.65.0 | v6.66.0 | v6.67.0 |
|---|---|---|---|---|---|
| source:changelog | validated |  |  |  |  |

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 521 tags: prefix "v", 518 stable, 3 prereleases (betaN×3), 0 junk; latest stable v6.67.0; lineage linear |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (521 tags: prefix "v", 518 stable, 3 prereleases (betaN×3), 0 junk; latest stable v6.67.0; lineage linear). |
| source:changelog | include | changelog.file | heuristic | Changelog with 41 version sections, newest 6.67.0 (heading "## 6.67.0 (September 30, 2026)"). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |

<details><summary>3 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| image `ghcr.io/tcort/markdown-link-check` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/yakdriver/md-check-links` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.terraform.io/hashicorp/aws` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |

</details>

## Candidates


### changelog (1)

- `CHANGELOG.md` (high; docs.changelog) — [hashicorp/terraform-provider-aws:CHANGELOG.md L1](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/CHANGELOG.md#L1): `## 6.67.0 (September 30, 2026)`

### image (3)

- `ghcr.io/tcort/markdown-link-check` (medium; make.image-ref, script.image-ref) — [hashicorp/terraform-provider-aws:.ci/scripts/markdown-link-check.sh L8](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/.ci/scripts/markdown-link-check.sh#L8): `link_check_container="ghcr.io/tcort/markdown-link-check"`
- `ghcr.io/yakdriver/md-check-links` (medium; make.image-ref) — [hashicorp/terraform-provider-aws:GNUmakefile L317](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/GNUmakefile#L317): `ghcr.io/yakdriver/md-check-links:2.2.0 \`
- `registry.terraform.io/hashicorp/aws` (medium; docs.image-ref) — [hashicorp/terraform-provider-aws:docs/design-decisions/provider_meta.md L49](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/design-decisions/provider_meta.md#L49): `│ Error while loading schemas for plugin components: Failed to obtain provider schema: Could not load the sc…`

### registry (218)

- `registry.terraform.io/hashicorp/aws/99.99.99` (low; make.registry-ref, workflow.registry-ref) — [hashicorp/terraform-provider-aws:.github/workflows/examples.yml L88](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/.github/workflows/examples.yml#L88): `mkdir -p ~/.terraform.d/plugins/registry.terraform.io/hashicorp/aws/99.99.99/"$(go env GOOS)"_"$(go env GOARCH…`
- `registry.terraform.io/providers` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/design-decisions/exclusive-relationship-management-resources.md L18](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/design-decisions/exclusive-relationship-management-resources.md#L18): `During early development of the Terraform AWS provider, resources sometimes represented "one-to-many" relation…`
- `registry.terraform.io/providers/hashicorp/ad/latest/docs` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/provider-design.md L16](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/provider-design.md#L16): `* Active Directory or other protocol clients. See the [Terraform Active Directory Provider](https://registry.t…`
- `registry.terraform.io/providers/hashicorp/aws/latest` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/add-a-new-datasource.md L98](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/add-a-new-datasource.md#L98): `This documentation will appear on the [Terraform Registry](https://registry.terraform.io/providers/hashicorp/a…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:.agents/skills/review-docs/SKILL.md L90](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/.agents/skills/review-docs/SKILL.md#L90): `* 'tags_all' - Map of tags assigned to the resource, including those inherited from the provider ['default_tag…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/ami` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/running-and-writing-acceptance-tests.md L1338](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/running-and-writing-acceptance-tests.md#L1338): `- __Uses aws_ami Data Source__: Any hardcoded AMI ID configuration, e.g. 'ami-12345678', should be replaced wi…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/availability_zones` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/running-and-writing-acceptance-tests.md L1363](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/running-and-writing-acceptance-tests.md#L1363): `- __Uses aws_availability_zones Data Source__: Any hardcoded AWS Availability Zone configuration, e.g. 'us-wes…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/billing_service_account` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/running-and-writing-acceptance-tests.md L1318](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/running-and-writing-acceptance-tests.md#L1318): `- ['aws_billing_service_account' data source](https://registry.terraform.io/providers/hashicorp/aws/latest/doc…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/caller_identity` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/running-and-writing-acceptance-tests.md L1316](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/running-and-writing-acceptance-tests.md#L1316): `- ['aws_caller_identity' data source](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-s…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/canonical_user_id` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/running-and-writing-acceptance-tests.md L1317](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/running-and-writing-acceptance-tests.md#L1317): `- ['aws_canonical_user_id' data source](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/cloudtrail_service_account` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:internal/service/cloudtrail/README.md L12](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/internal/service/cloudtrail/README.md#L12): `* AWS Provider Docs: [One of the CloudTrail data sources](https://registry.terraform.io/providers/hashicorp/aw…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/docdb_engine_version` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/running-and-writing-acceptance-tests.md L1385](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/running-and-writing-acceptance-tests.md#L1385): `- ['aws_docdb_engine_version' data source](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/d…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/docdb_orderable_db_instance` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/running-and-writing-acceptance-tests.md L1435](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/running-and-writing-acceptance-tests.md#L1435): `- ['aws_docdb_orderable_db_instance' data source](https://registry.terraform.io/providers/hashicorp/aws/latest…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/dx_locations` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/running-and-writing-acceptance-tests.md L1412](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/running-and-writing-acceptance-tests.md#L1412): `- __Uses aws_dx_locations Data Source__: Hardcoded AWS Direct Connect locations, e.g., 'EqSe2', should be repl…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/ec2_instance_type_offering` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/running-and-writing-acceptance-tests.md L1430](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/running-and-writing-acceptance-tests.md#L1430): `- ['aws_ec2_instance_type_offering' data source](https://registry.terraform.io/providers/hashicorp/aws/latest/…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/ec2_managed_prefix_list` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:website/docs/d/prefix_list.html.markdown L19](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/website/docs/d/prefix_list.html.markdown#L19): `The [aws_ec2_managed_prefix_list](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sourc…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/ec2_spot_price` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/running-and-writing-acceptance-tests.md L1543](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/running-and-writing-acceptance-tests.md#L1543): `- __Uses aws_ec2_spot_price Data Source__: Any hardcoded spot prices, e.g., '0.05', should be replaced with th…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/ec2_transit_gateway_attachment` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:website/docs/d/ec2_transit_gateway_attachments.html.markdown L52](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/website/docs/d/ec2_transit_gateway_attachments.html.markdown#L52): `* 'ids' A list of all attachments ids matching the filter. You can retrieve more information about the attachm…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/ec2_transit_gateway_peering_attachment` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:website/docs/d/ec2_transit_gateway_peering_attachments.html.markdown L53](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/website/docs/d/ec2_transit_gateway_peering_attachments.html.markdown#L53): `* 'ids' A list of all attachments ids matching the filter. You can retrieve more information about the attachm…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/ec2_transit_gateway_vpc_attachment` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:website/docs/d/ec2_transit_gateway_vpc_attachments.html.markdown L47](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/website/docs/d/ec2_transit_gateway_vpc_attachments.html.markdown#L47): `* 'ids' A list of all attachments ids matching the filter. You can retrieve more information about the attachm…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/iam_policy_document` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:website/docs/r/kms_key.html.markdown L322](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/website/docs/r/kms_key.html.markdown#L322): `* 'policy' - (Optional) Valid policy JSON document. Although this is a key policy, not an IAM policy, an ['aws…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/identitystore_group` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:internal/service/identitystore/README.md L12](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/internal/service/identitystore/README.md#L12): `* AWS Provider Docs: [One of the IdentityStore data sources](https://registry.terraform.io/providers/hashicorp…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/lambda_invocation` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:website/docs/r/lambda_invocation.html.markdown L13](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/website/docs/r/lambda_invocation.html.markdown#L13): `~> **Note:** By default this resource _only_ invokes the function when the arguments call for a create or repl…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/neptune_engine_version` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/running-and-writing-acceptance-tests.md L1386](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/running-and-writing-acceptance-tests.md#L1386): `- ['aws_neptune_engine_version' data source](https://registry.terraform.io/providers/hashicorp/aws/latest/docs…`
- `registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/neptune_orderable_db_instance` (low; docs.registry-ref) — [hashicorp/terraform-provider-aws:docs/running-and-writing-acceptance-tests.md L1434](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/docs/running-and-writing-acceptance-tests.md#L1434): `- ['aws_neptune_orderable_db_instance' data source](https://registry.terraform.io/providers/hashicorp/aws/late…`
- … 193 more

### release-trigger (1)

- `v[0-9]+.[0-9]+.[0-9]+*` (high; workflow.tag-trigger) — [hashicorp/terraform-provider-aws:.github/workflows/maintainer_helpers.yml L27](https://github.com/hashicorp/terraform-provider-aws/blob/v6.67.0/.github/workflows/maintainer_helpers.yml#L27): `- "v[0-9]+.[0-9]+.[0-9]+*"`

### tag-scheme (1)

- `^v(?P<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$` (high; tags.ls-remote) — [hashicorp/terraform-provider-aws/releases/tag/v6.67.0 refs/tags/v6.67.0](https://github.com/hashicorp/terraform-provider-aws/releases/tag/v6.67.0#refs/tags/v6.67.0): `e449e05eb24b58213c3c4b2bd07d22b0ca9be443 refs/tags/v6.67.0`
