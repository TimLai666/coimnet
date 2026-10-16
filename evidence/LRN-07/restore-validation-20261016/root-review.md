# Root review: ticket133

Root re-read current replay Add, Sample, draw and Restore and the original37-event public candidate before reuse. The preceding source delivery is unchanged at848 fingerprints.

A read-only caller audit correctly identified checkpoint canonicalState as the current production save/load consumer. State.Validate, Save and Load all route through it. Existing cross-process training recovery does not carry Replay, so Root adds actual old-checkpoint replay continuation evidence.

One worker simplification is rejected: reservoir + Seen<=2 + time_balanced does not imply zero draws. With capacity1 the second Add already draws. The conservative predicate must first rule out reservoir overflow, independently from sampling. An explicit lawful neighbor is added before implementation. min(Seen,Capacity) is a historical upper bound for drop retention, never its current held size. These are necessary checks, not an inverse history reconstruction.

Root read the complete production diff and all three frozen regression sources before adoption. Candidate and formal ordinary/race results match: all74 new terminal events pass and the original615 package events retain their previous outcomes. The48 pre-change replay checkpoints retain their exact bytes; independent new-process continuations match before/after byte for byte. The independent adversarial worker reported no confirmed narrow-patch defect; Root verified actual source rather than relying on inaccurate worker line citations. The zero-draw task-set allocation is restricted to positive Draws. Full engineering is still pending.

## Diff Inspector

Scope: CLEAN. Reviewed one production file and the three frozen public regression files against d38ffae4e0590fd307c0f27e897a2003a4afd556. All851 formal source fingerprints still match. Add, Sample, RNG and persisted field declarations retain their baseline source. The current producer/consumer boundary is checkpoint canonicalState, reached by Validate, Save and Load.

Adversarial review: RUN, triggered by persistent recovery and rejection behavior. Root read the full returned patch and independently compared formal logs. No confirmed defect in the adopted narrow patch. Sampling source at replay/sampling.go:75-100 confirms singleton task groups draw nothing; lines108-123 confirm held sizes1/2 have only singleton time buckets. The live local source was checked after indexed discovery.

Needs investigation: replay/store.go:113-115 still replays lawful large Draws one at a time; an operational latency bound and counter-wrap policy are not established. The current fix rejects confirmed impossible histories early and does not claim arbitrary history reconstruction. No guessed draw cap was introduced.

Preservation review: Root verified60 necessary inputs including48 old checkpoints,18 original model files,6 persistent data files and32 foreign files. Explicit count guards and the frozen payload manifest hash supplement existing set/hash checks. Actual archive, cleanup and full-engineering receipts remain pending at this review point.

Full engineering has now completed on the same851 source fingerprints: unit8517pass23skip, race8516pass24skip,58pass/4skip packages per build,8466 prior terminal events unchanged plus74 newpass each. All nine steps passed. The restricted sandbox listener failures were archived; the permitted localhost rerun changed no source. The approved neutral-gain21-event controls passed in both full builds. LRN-07 is restored to passed; independent ticket134 keeps LRN-10 specified. Final governance, preservation, cleanup and delivery remain pending.

Final governance initially exposed acceptance-metadata omissions, not a source regression: the new record lacked requirement and the historical diagnostic was still in the passed-evidence list. The original finding remains unchanged and is linked as prior_finding; only complete acceptance records are indexed for passed LRN-07. Corrected final governance has31pass. All60 necessary inputs are archived and verified. The owned work directory was removed (1,739,953,161bytes);851 source entries,18 models,6 data files,15046 original module files and32 foreign files remain unchanged. Delivery is pending.
