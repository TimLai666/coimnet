# Diff Inspector

Scope: CLEAN. Reviewed ticket120 diagnostic sources, the entire new permanent test file, affected mixing explanation and evidence drivers. Production Go, dependencies, public APIs and model formats remain unchanged. Original834 source entries are identical; formal-source.json adds one test, total835. Unrelated AGENTS.md and two pre-existing evidence trees are excluded.

Adversarial review: RUN. Independent Luna/max source, diagnostic and cleanup reviews supplement Root's own line-by-line review. Root dispositions and limitations are in readonly-review.md; pending artifacts and hypothetical future changes are not classified as production defects.

## Findings

No confirmed production findings. Ticket21's mathematical equivalence sentence was wrong for negative coefficients and has been corrected to the already explicit raw-occupancy rule. The permanent hand oracle checks occupancy .8/.2 with scale-.5: gain .6 and upstream2 occupancy derivative [-1,0].

Root caught two incorrect Network.PredictAll calls before runtime. The first focused run additionally exposed a new test's wrong empty-initial-gradient assertion; existing coupled_reverse.go:45 allocates one initial derivative per node. The corrected oracle requires Nodes2, with all108 reference coordinates and the predeclared tolerance retained. The original failure and original manifest are preserved. Root also ensured restored trainers are owned by the deferred Close before subsequent checks can fail.

## Evidence and scope

Both diagnostic builds pass336 complete/recompute comparisons,36 controls,56 first-step chemistry proofs,432 informative Continuous parameter/input reference coordinates and28 separate-process pending-snapshot resumes. Key and coordinate sets are checked independently by verify-diagnostic.py.txt. The largest observed absolute gradient error is2.032082964920079e-5. This is a two-node artificial fixture, not a full-brain or real-task quality measurement.

The permanent tests reuse existing three-channel fixtures. Both builds pass65 terminal test events:56 training leaves, six18-coordinate reference profiles and the three top-level tests. Existing direct effect rules pass39 events per build. All108 permanent reference coordinates are informative at the fixed tolerance.

Both isolated mutations compile and fail assertions. Consistently replacing max with sum is rejected by58 test events, including the independent hand result and every training control. Ignoring the clamp derivative guard is rejected by14 events, including both gain-bound independent reference profiles and existing exact/outside-bound oracles. Formal sources remain unchanged by the Go overlays.

No hard-spike finite differences are claimed for LIF. Its independent rule oracles supplement full/recompute comparisons, but this ticket does not provide a new independent full LIF surrogate reference. First-step clamp proofs do not establish the clamp status of every later row. No biological calibration, OCR improvement or absolute absence of bugs follows from these results.

Full framework engineering now passes:58 packages each build,6520/6519 pass and22/23 skip. All6477 prior terminal events remain unchanged and exactly65 permanent test events are added in each build. Format/build/vet/module selection and verification,31 engineering governance events and the fresh closure binary pass. Final metadata governance, evidence preservation, owned cleanup and Git delivery remain separate closure conditions.

## Subtraction review

Only tests and the inaccurate explanation are added or corrected. Reused fixtures avoid another production abstraction or duplicated chemistry declaration. Build/module caches and diagnostic binaries will be removed after evidence preservation. The final evidence keeps the public probe and resume inputs in one compressed archive.

## Independent permanent-test review

The final test SHA1e8ebebb9f62a5447542c9ad8ed912e04b6cef32960fb570d73b920d92c63f12 was independently reviewed with the unchanged Root contract36726690a6b571c0ee26439f34a8831b10ff2718a448081831665a37b7e63e33. No confirmed defect remained. The review checked all10 parameter coordinate mappings, eight input coordinates, the exact signed profiles, complete snapshot normalization and every trainer Close path. The scope limit on a new independent full LIF surrogate oracle is retained, not reclassified as a confirmed bug.

The first full engineering attempt failed because the sandbox denied httptest loopback binds, with six package panics. Unchanged-source engineering-r2.py.txt is executed outside that sandbox under the existing local validation authorization. The original compressed log remains available; no test or production source is changed to bypass the denial.

## Closure checks

Final metadata governance caught the missing required test_log key in the new evidence JSON. Existing logs were indexed, the original failure retained, and all31 governance events then passed (final-governance-r2.json). No validator or Go source was weakened. The67 necessary work files are byte-verified in the archive. Cleanup removed28022 entries/1668319814 logical bytes, preserving835 formal sources,18 original models,six data files,15046 original module files,32 foreign paths and both pre-existing work roots. Git delivery is still pending.
