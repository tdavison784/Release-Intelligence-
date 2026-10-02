# Lane `groundtruth` — status

**State: in progress.**

## Done
- Format (step 1): `internal/eval/labels.go` (new) — `semantics` per expected item, `exposure` / `overlap` /
  `environmentEvidence` per link, `environment.undecidedImpact`; strict decoding into the domain types;
  validation. `Case.TransferOf` (transfer environments, G12) in `case.go` + `transfer.go`. Tests:
  `labels_test.go`. Docs: `eval/FORMAT.md` ("Semantic labels", "Transfer environments").

## Next
- Labels on the 21 existing links; corrections (CHANGELOG.md); transfer environments; new cases; one eval run.
