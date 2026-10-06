# Independent adversarial review

Reviewer: `/root/sequence_numerical_adversarial_luna`, `gpt-5.6-luna`, effort `max`, read-only. Review covers both implementations, three new tests, ENG numerical contract, ticket 42 and unchanged OCR/ASR training consumers.

Scope: CLEAN. No confirmed findings. Reviewed source SHA-256 values: distribution `affac62481edd50b890b0d7a1bcc51b208980fa59a44aace378367c416b2f5c1`; CTC `8c9aa1b04eb50c7969b8dd0023483817928c64f5fd246511e862b90b77427058`.

The reviewer reported exit 0 for `git diff --check`, scoped numerical tests and `gofmt -d`. Root independently ran the tests, formatting and diff checks; see their command receipts. The reviewer did not run full suite, race, build or vet.

## Investigated concern and Root disposition

The reviewer asked whether an impossible target may return zero before normalization when finite logits have an unrepresentable difference. Root confirmed that the entire validation/feasibility prefix is byte-identical to the base; its digest is in `impossible-precedence-review.json`. All input shape, label and finite-value validation occurs first. With explicit `ZeroOnImpossible`, a structurally impossible target then returns zero without evaluating unused normalization.

`impossible-precedence-probe.log` confirms finite extremes return a zero gradient with `Impossible=true`; NaN is rejected before the skip. This preserves the existing explicit policy. Numeric errors from calculations that do run, including the demonstrated valid-target posterior precision failure, are not skipped. The concern is closed as unchanged, intentional behavior, not a confirmed defect. No extra option or API change was introduced.
