# Frozen stimulus-history check — 2026-10-03

The trained models respond to past delivered stimulus, but this check does not establish a navigation benefit. Clearing stimulus before relocation changed later outputs in all eight rewarded test trials for each trained delivered and shuffled model. The five non-rewarded trials, no-stimulus models and untrained zero-readout models had exactly zero difference.

| Seed | Control | Maximum recorded-output difference (cm) | Maximum path separation (cm) | Erased minus original test MSE (cm²) |
| --- | --- | ---: | ---: | ---: |
| 20261003 | delivered | 0.00551190 | 1.06816204 | +0.000000827620 |
| 20261003 | shuffled | 0.00923888 | 1.00804751 | +0.000000661816 |
| 20261004 | delivered | 0.05122368 | 6.59741407 | -0.000004425708 |
| 20261004 | shuffled | 0.06770313 | 7.72915777 | -0.000006221511 |
| 20261005 | delivered | 0.01861945 | 2.95416667 | +0.000010228816 |
| 20261005 | shuffled | 0.02235079 | 3.25400186 | +0.000010197149 |

Every original and erased model group remains 0/13 hits. The MSE effect changes sign across seeds. These are engineering results from an artificial eight-node network on previously evaluated test trials, not evidence of a fly neural mechanism.

The [preregistered protocol](preregistration.json) fixes all settings before the paired runs. It transforms the original stimulus control first, then zeroes only model-visible stimulus before the first source relocation row. Recorded non-stimulus inputs and all later stimulus stay identical. Autonomous paths may diverge through model actions while using the original raw blocks for subsequent shuffled stimulus.

[report.json](report.json) retains all 234 pairs across 18 groups, with no exclusions, and [summary.json](summary.json) reports every group. Complete paired traces are hashed and omitted from this compact report. Source and model bundles remain in the external directories recorded by [evaluation.json](evaluation.json); neither was changed or retrained. The fixed 1e-6 cm reporting scale does not define statistical significance or navigation success.

Two independent processes produced byte-identical paired reports. Every original rollout summary and complete trace fingerprint matches ticket 34; weighted original MSE matches within 1e-15 cm². The prior summary used Python JSON trace hashing. [legacy-trace-hashes.go.txt](legacy-trace-hashes.go.txt) independently normalizes the original complete traces with the same typed Go JSON encoding used by the new report, and [analyze.py](analyze.py) verifies every pair without selecting trials. Repeat the normalization by copying that utility to a temporary .go file, then run it with the legacy complete-report path from the preceding evidence. Run `python3 evidence/TSK-11/stimulus-counterfactual-20261003/analyze.py` from the repository root to regenerate the summary.

[checks.json](checks.json) records successful formatting, build, full normal and race suites, a fresh example race run, vet and module checks. Normal tests passed in 57 packages without cache; race passed in 57 packages, 51 cached, followed by fresh race coverage of the changed example. [commands.json](commands.json) records paired runs and CLI failure checks. Existing and competing output writes, wrong source/model and cancellation all failed without modifying a prior report or publishing partial output. [red.log](red.log) records the failing behavior tests before implementation.

The [independent review](independent-review.txt) and [root review](root-review.md) found no confirmed defect in the final diff. [verification.json](verification.json) links the source fingerprints, environment and acceptance evidence. TSK-11 and OPS-05 stay specified; 89 of 91 requirements remain passed.

The implementation reuses the existing model and rollout scorer. Duplicate full stdout reports were removed after exact comparison with the retained report; their size and SHA-256 remain in the command receipts. No new model, reward mechanism or training settings were added.
