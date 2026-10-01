# Research notes — strimzi 0.45.0 → 0.46.0

Authored blind on 2026-10-01 from the CHANGELOG.md at the 0.46.0 tag (the
channel the definition ingests: "Major changes, deprecations and removals"
sections exist on N.M.0 releases).

## Sources read

- CHANGELOG.md at 0.46.0, top section + "Major changes, deprecations and
  removals": ZooKeeper removal with the pre-upgrade check (E1), MirrorMaker 1
  removal (E2), storage overrides removal (E3), plugin removals from the
  images (E4), Kafka 4.0/3.8 support window (E5), OPA deprecation (E6),
  pod-name label removal (E7), Log4j2 switch (E8).
- The picked items cover the removal, CRD-schema, compatibility, deprecation
  and behavioural change shapes of this minor; the changelog's
  dependency/fix bullets are covered by the notExpected entry.

## Judgement calls

- E1/E2/E4 carry action-required because the upstream text itself demands
  pre-upgrade migration ("Please make sure … before upgrading"). A KRaft-only
  fixture like ours would NOT need E1 — the dataset expectation is about the
  upgrade knowledge, and the environment links only claim what the fixture
  asserts (our Kafka CR is already KRaft-shaped, so no E1 link).
- E5 starts as review-required in the abstract but the fixture pins
  metadataVersion 3.8-IV0, which makes the Kafka-version window a concrete
  action for this environment — the link says action-required.
- F1's range claim is grounded in the 0.47 changelog note ("0.47.0 is the
  last version with support for Kubernetes 1.25 and 1.26; from 0.48.0 only
  1.27+"), which implies 0.46 still supports 1.25+ — 1.29 is comfortably in
  range.
