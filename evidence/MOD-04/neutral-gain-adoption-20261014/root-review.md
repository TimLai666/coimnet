# Neutral gain adoption review

Scope: CLEAN. The accepted change only tightens the common sensitivity-bound validator in `modulation/effects.go:205-210` and documents the public contract at lines 22-26. Root owns both new regression files. No forward/gradient formula, serialization field, dependency, initialization or task setting changed.

Adversarial review: RUN through the native fallback, `gpt-5.6-luna` with max effort, after the freshly listed OpenCode `opencode/mimo-v2.5-free` returned HTTP 410. The independent report is `native-review.md`; Root checked its conclusions against current source, the complete code diff and actual receipts.

## Findings

No confirmed findings in the adopted patch. The previously confirmed sensitivity-neutrality defect is addressed at the common validator. Defaults apply before the inclusion test, and `[1,1]` remains legal. An invalid declaration cannot enter ApplyEffects, BackwardEffects, ChemistryConfig.Validate, Network/Trainer construction or restoration, Individual replacement/restoration, or checkpoint loading. The authorized compatibility change rejects previously accepted ranges excluding 1.

## Verification and compatibility

The corrected permanent regression fixture uses the documented explicit initial voltage `[]float64{0}`. Against the original production file it produces 20 failed test events and one legal-control pass. The fixed source produces 21 passes with no failures. Assertions were not weakened. The first nil-voltage fixture error and first diagnostic per-row target-shape error are recorded separately as test-program errors, not framework defects.

`compatibility.json` proves that 15 actual old-source individual checkpoints have byte-identical complete loaded state, next Advance, next training result and final state between old and fixed binaries in separate processes. All 15 update real parameters. Three excluded-range old checkpoints are rejected by current LoadIndividual, return an empty result and retain their original bytes. Controls include all three receptor mix declarations, zero-default bounds, `[1,1]` and three other legal ranges.

`persistence-endpoints.json` adds 10 actual old-source episode checkpoint / training bundle controls with complete JSON equality after load, prediction and next update. Ten invalid artifacts, including the two adjacent float64 values around 1, are rejected by Load and LoadTrainingBundle without changing any original file. DecodeIndividual also rejects all three actual invalid individual files. These endpoint checks close the native report's outstanding serialized-entry-point questions.

Root rehashed all 806 source files and 470 frozen test files, plus every regression, program, manifest and compatibility-log receipt used above. No current source or frozen test mismatch occurred. Full engineering and cleanup/delivery are tracked in `verification.json` when their commands finish; this review does not infer their success from focused checks.

## Limits and simplification

The fixtures are artificial one-node models for contract and persistence verification. They do not establish new whole-brain performance, biological calibration, real OCR accuracy improvement or absolute absence of bugs. The lower shared validator covers adjacent float64 bounds; the additional serialized trainer entry-point checks exercise the same boundary values. No duplicated endpoint validator, automatic migration, model-file rewrite or task-specific workaround is needed.
