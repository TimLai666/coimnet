# Diff Inspector

Scope: CLEAN. Reviewed the two new experiment test files, independent verifier and negative tests, fixed diagnostic ticket, analysis, and linked engineering/requirements documents against base 67c4c5bbed368507310c39b6b4619d2969cfb6af.

Adversarial review: RUN. Luna/max performed read-only reviews of the collection/update path and final diagnostics. Spark/xhigh was actually attempted first and rejected as an unknown model. No delegated writes or Git mutations occurred.

## Findings

No confirmed findings remain. Root reviewed the actual code and artifact comparisons, including independent replay, policy/bootstrap replay for all 600 training episodes, caller snapshot isolation, untouched RNG, update/optimizer progression, fixed probes, exclusive output, and failure checks. Production code and all prior tracked Go sources remain unchanged. Requirements only append an LRN-09 supplemental evidence reference.

## Needs investigation

- Policy/value gradient interference and exploration causality are not isolated by these observations. A diagnostic gradient decomposition using the same fixed models and collected observations is needed before choosing an intervention.
- Python independently recomputes physics, GAE and policy/value loss terms, but not neural forward, entropy or Go PCG reset. Go separately verifies recorded probabilities, values and timeout bootstrap through a second model copy. The complete legacy report also equals ticket 37.

## Error and compatibility checks

The diagnostic is opt-in and bounded to the declared 600 training/198 probe episodes. It adds no public API, dependency, algorithm or model parameter. The caller creates the output directory. Existing report files are refused before training and publishing uses O_EXCL. Write/close failures fail the test; atomic recovery of a failed diagnostic file is not promised. No model package or private input is included in the evidence.

Hand-computed episode and GAE checks, negative mutation tests, two fresh processes, and existing-output rejection are saved with hashes in verification.json. Full Go checks and delivery are recorded separately after their actual results.
