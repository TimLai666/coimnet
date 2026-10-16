# Root implementation slot133

Only output runs/.tmp/replay-restore-20261016/restore-store-candidate.go.txt. Root created an exact copy of current replay/store.go as the skeleton. Public signatures remain Restore(Buffer,State)(*Store,error), RestoreSnapshot(Snapshot)(*Store,error) and validateStored(Experience,bool)(Experience,error). Never edit any test, existing candidate test text or formal file.

## Required changes

- Add standard encoding/hex. After the existing64-character checks in validateStored for hashed-only entries, reject either non-hex InputHash or TargetHash. Preserve valid uppercase bytes exactly, raw-array validation and existing fields.
- Before constructing/replaying a store, reject keep_until_evicted/reservoir when Seen>Capacity and Draws<Seen-Capacity. Use uint64 after positive capacity validation, avoid narrowing Seen or subtraction before the guard. Never impose this lower bound on drop_after_sample.
- Derive conservative canAddDraw=(EvictionReservoir && Seen>uint64(Capacity)). Historical maximum held upper bound is min(Seen,uint64(Capacity)). canSampleDraw requires upper bound>=2, and for time_balanced upper bound>=3. If neither source could ever draw, require Draws0.
- Strengthen historical inference only where every accepted item remains held (Seen==uint64(len(Items))): no Add overflow ever happened. Under drop retention, no nonempty Sample could have happened, so canSampleDraw=false. Under keep/task_balanced, if all current TaskIDs are unique every historical task group had size<=1, so canSampleDraw=false; a repeated task allows a sample draw. Keep the same shared zero-draw guard. A local set is sufficient, no inverse-history reconstruction or second sampling engine.
- Reject without mutating caller data, return nil store+error. No arbitrary upper draw cap, change to PRNG, counter format, Add, Sample, schema, public API or dependency version. Invalid MaxUint64 empty-store Draws must return before the existing replay loop. Valid huge draw replay cost and counter-wrap concerns remain separate investigation, not a reason to alter this contract.

Root has prepared frozen public tests for all three validation causes, checkpoint Validate/Save/Load, lawful positive neighbors and24 combinations of actual old checkpoint continuation. Implementation is dispatched only after current production has actually failed the frozen tests. Root reviews exact diff and runs Go; implementation worker must not run Go.
