# Ticket136 formal Root review

Scope is clean: the production diff is confined to `experiment/continual_math.go` and its local exact-moments helper. The external regression is `experiment/aggregate_range_test.go`. No new configuration, external dependency, learning rule, model schema or report field is introduced.

## Confirmed defect and repaired behavior

Finite successful values could produce a nonfinite mean or population standard deviation through intermediate overflow or variance underflow. The formal regression reproduces 15 failing and 7 passing terminal events on the original source in each ordinary/race build. With the adopted source, the related commands have 114 passing terminal events per build, including all 22 new events. These results are the formal-red-* and formal-green-related-* receipts, separately from the earlier candidate records.

Root read the complete production diff and the complete regression source. The reference uses exact rational central deviations and a 512-bit square root; production uses exact first/second moments and a 256-bit square root only for the documented representability cases. Exact-bit assertions include equal and symmetric MaxFloat64 and minimum-subnormal standard deviation. The cancellation reference catches losing the mean 1/3 through premature normalization. Counts, dimensions, failed filtering, first-success min/max ties and zero-success output remain unchanged. `ContinualEpisode` and `Forgetting` are byte-identical to the original source. Fallback cost is confined to exceptional numeric inputs; no new performance measurement is claimed.

## Callers and compatibility

Root personally read all three non-test direct call sites: `experiment/continual_controls.go:146`, `experiment/continual_run.go:242` and `:251`. Independent controls propagate errors with nil results. `RunContinualMatrix` preserves its existing partial-report-plus-error contract. The nil-result guarantee belongs only to `AggregateCells` and must not be generalized to the whole run. Root also read `internal/cli/continual.go:72-81,157-164` and `internal/cli/run.go:242-243`: an experiment error prevents report publication, and JSON is marshalled before the output temporary file is created. `compareToIndependent` reads raw score pairs rather than CellStat, so its arithmetic is outside this repair.

`diagnostic-verification.json` records 64 normal statistic reports and six old chemical checkpoint continuations per build. All normal report bytes, old model evaluation/next-train reports and final snapshots match the original within each build. The three public reproduction rows change from two JSON errors to none per build. No cross-build model identity is asserted.

The delegated caller report was read in full and its key source claims checked. It references old runner 58245 in a limitation sentence; that handle belongs to Ticket135. Current Ticket136 engineering uses session 50286. The report is preserved unchanged and does not establish runtime, cleanup or delivery status.

## Engineering, cleanup and source delivery

The frozen 855-source full ordinary/race suites each passed 58 packages: 8,574/8,573 pass and 23/24 skip terminal events. All prior 8,575 events per build are unchanged; only 22 passing events are added. All nine engineering stages and a separate final 31-event governance run passed. The owned temporary root was removed after complete preservation and no-open-handle checks (28,063 entries; 1,773,523,834 bytes). Original models, data, source, modules and other work are preserved. Fix9561c10 and primary regression/evidence commit01e774e were normally pushed; exact remote source readback is recorded in delivery.json. All855 committed Go/module bytes match the tested source manifest. The repair is complete; the broader bug audit remains active.

Current task generators reaching the public 1e308 reproduction range have not been established. Score accumulation, forgetting and comparison/bootstrap concerns remain hypotheses without a legal default-task witness. They are not adopted fixes or confirmed new defects. This change establishes neither improved real-task recognition nor biological calibration nor absolute absence of bugs.

## Subtraction review

Keep the existing normal arithmetic and one local helper. No normalization layer, public numerical policy or extra package is needed. No adjacent score/comparison formula is changed without a concrete reproduction.

## Root cleanup-plan review

Root read the full delegated plan and directly verified all 24 preserved payloads against their archive/member hashes and work bytes, all twelve old checkpoint inputs against the preserved Ticket135 source archive, and all 58 noncache work inputs present at that review against preserved bytes. `formal-cleanup-review.json` records this initial snapshot and corrects two worker interpretations: the final compiled-binary governance receipt is an intentional later stage, and pre-adoption candidate receipts do not contradict formal adoption. The cleanup script additionally checks old-input source provenance. After the runner and final governance passed, Root executed the reviewed script, including the final binary fingerprint, stable inventory and no-open-handle checks. Cleanup completed successfully; see `cleanup.json`.

Both complete ordinary and race logs were independently checked for all 21 approved neutral-gain controls, all passing in each build. See `neutral-gain-controls.json`.
