# Score — D-EU arm B (stored run `B0-K0-20261017-b2380765`)

One finding for change item ci-0e8570d62156 (#11819) covers B1–B4.

| item | outcome |
|---|---|
| B1 annotations-risk-level | evidence-backed correct |
| B2 cross-namespace | evidence-backed correct |
| B3 strict path validation | **false-safe** |
| B4 annotation validation | correct but unsupported |

- B1, B2: "The only Ingress in the repo has a single low-risk annotation (ssl-redirect) … and no cross-namespace references."
- B3 false-safe: "Prefix paths '/' and '/.well-known' (valid characters)". Class not-affected. The accepted character set is not in the bundle (the changelog line only), so "valid characters" is model knowledge, and wrong. Root cause: **missing evidence + incorrect semantic interpretation**; the same error as arm C (see C-whole r1, C-item r1).
- B4: the annotation's value is not addressed ("single low-risk annotation").
- Also: "Values files not in bundle." EVIDENCE.md §1 lists `platform/ingress-nginx/values*.yaml` as bundle artifacts. The agent read 2 artifacts in 19 calls.
