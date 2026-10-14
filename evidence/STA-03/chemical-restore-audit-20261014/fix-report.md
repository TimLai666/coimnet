# STA-03 chemical restore presence fix

## Problem

`readBundle` decoded `document.json` before applying the existing raw presence rules. Missing or `null` individual chemical fields, training scalar/options fields, and model `evidence_registry` could therefore reach `encoding/json` as zero values. A valid chemical `TaskTrainingBundle` was also rejected because the generic struct walker does not support the `ChemicalPart.Resources` map.

## Fix

- `checkpoint/bundle.go`: invoke the presence validator after document checksum and duplicate-key checks, before strict decoding and array restoration. The hook applies to the four public bundle document types while preserving the low-level generic bundle round-trip contract.
- `checkpoint/bundle_presence.go`: reuse the existing individual and model single-file walkers through validation-only envelopes; route training and task-training documents to the chemistry-aware training walker. No public API was added.
- `checkpoint/chemical_episode.go`: validate direct `TrainingSnapshot` chemistry separately, remove only chemistry from a validation copy before the ordinary struct walker, and retain the existing episode outer-state checks. Chemistry resource maps are checked by the existing individual chemical validator.
- `checkpoint/task_bundle.go`: make `TaskTrainingSnapshot.UnmarshalJSON` use the same task boundary and nested training presence validation as bundle loading.

All four kinds are connected:

| Kind | Raw presence validation |
| --- | --- |
| `individual` | `requireIndividualFields` through a validation-only envelope; global null rejection matches `DecodeIndividual` |
| `training` | `checkTrainingSnapshotRequiredFields`, including chemistry map handling |
| `model_package` | `requireModelPackageFields` through a validation-only envelope; global null rejection matches `DecodeModelPackage` |
| `task_training` | `checkTaskTrainingDocumentFields`, including task fields, config, and chemistry-aware training |

## Verification

Required regression command, using only the audit-local caches and temp directory:

```text
env GOCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-restore-audit-20261014/go-cache GOMODCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-restore-audit-20261014/go-mod-cache GOTMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-restore-audit-20261014/go-tmp TMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-restore-audit-20261014/go-tmp GOPROXY=file:///Users/timlai/go/pkg/mod/cache/download GOSUMDB=off GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS='-p=2 -timeout=60m' go test -count=1 -run '^TestBundle(RejectsIncomplete|TaskChemicalTraining|PresencePreserves)' ./checkpoint
```

Result: `ok github.com/TimLai666/coimnet/checkpoint 0.559s`, exit 0.

The existing low-level bundle tests `TestBundleRoundTrip`, `TestBundleKeepsPointersToZero`, `TestBundleExtractsOnlyLargeArrays`, and `TestBundleArrayPrimitiveKinds` also passed (`0.681s`). `gofmt` was run only on the modified formal files, and `git diff --check` passed.

The frozen RED-v2 test SHA remains unchanged: `6a89dc00a4a5a80f1cc9979faf6ff05daff96873dc2b6a5fd461fe6edadb0114`.

The source-with-tests-v2 baseline check covered all 807 listed files and reported `mismatch=0` before editing. No test file was modified. No commit, stash, reset, dependency update, or other repository was touched.

## Limits

The full repository suite, race run, and root's independent 48-fixture byte/report/resume comparison were not run by this worker. This report does not claim framework-wide zero defects.
