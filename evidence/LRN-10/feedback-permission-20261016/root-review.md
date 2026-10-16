# Root review — ticket132

The complete candidate test, fixture patch and three-hunk production diff were read before adoption. Current formal ordinary and race runs each compiled and produced exactly six expected failing terminal events and one passing allowed-score control. The previous candidate and source fingerprints are retained.

RunAdaptiveEvaluation validates every item AllowedFeedback against FeedbackAvailable before the adaptive runner. The guard after successful Act therefore enforces both declarations. Every observation and normal closed-gate decay still runs. itemMSE, score clamp, Feedback construction, Receive and Update remain exactly the same for explicitly allowed score. No other feedback kind has a defined automatic mapping; metadata is retained and is no longer silently converted to score. Fixed scoring continues through ticket131's AdvanceFixed.

The only existing fixture changed is TestRunAdaptiveEvaluationAdaptiveMode: it originally expected target-derived learning without declaring any allowed feedback. Only global/per-item score declarations were added; its state-change assertions remain. New public tests distinguish omitted permissions, other-kind permissions, mixed items, allowed learning, natural decay and unchanged base parameters. No test threshold was weakened.

The public API comment now states the actual synthesized kind and permission conditions. CLI fixture already declares score at both levels; help now says fixed scoring holds fast state. The format index follows the actual shifted constant line. There is no model field, new API, dependency or persistence-format change.

## Subtraction review

One membership check uses existing validation and stepping. No duplicate validation map, feedback registry, callback, second scoring engine or correctness scoring rule was added. Real-task quality and biological calibration are outside this engineering change.

## Next audit

Root read the current replay/store.go and replay/sampling.go in full. Draws counts sampling, reservoir replacement and whole-value rejection through the shared draw method. Keep/drop retention require different invariants, so the initial minimum-draw guard cannot be applied to drop retention. Extremely large Draws restore cost and draw-counter wrap remain investigation items pending a practical reproduction and a contract decision; the worker's static claim alone does not establish a repaired or newly confirmed runtime defect.

## Root verification and adversarial review

Formal ordinary/race permission suites each passed39 events; fixed-scoring controls each passed35. The ten-row public before/after probe is identical across builds. Three forbidden target pairs are now independent, allowed-score rows match the original bytes, and unrelated replay controls remain unchanged. Twelve historical chemical snapshots resume byte-identically. Actual CLI fixed/adaptive reports, eight items each, match original-production overlay reports byte for byte; the updated help was executed and read back.

An independent gpt-5.6-luna/max adversarial reviewer found no confirmed defect in the narrow permission change, including the public API and CLI descriptions. It did not run tests or use spies; Root executed the public state/output controls and CLI comparisons rather than accepting its static conclusion as test evidence. OpenCode remained unavailable with the recorded FileSystem.open error.

The complete ordinary/race engineering suites passed58 packages each, with8,443/8,442 pass and23/24 skip. Each retains all8,459 prior terminal events; only the seven passing permission regression events are new. The original engineering session91954 completed successfully without a replacement race. Format, build, vet, module verification, pinned Insyra v0.3.4,31 governance events and the closure binary all passed. The21 approved neutral-gain regressions also passed in each full build; their exact log-derived receipt is approved-neutral-gain-control.json.

## Confirmed follow-up findings

The public fixed-evaluation MSE probe returned+Inf and an unencodable report for two zero outputs and two finite1e154 targets. The independently squared exact float64 reference rounds to1e308. Both build outputs are identical; the0.5 control is finite0.25. This P2 is ticket134 and keepsLRN-10 specified even after ticket132 engineering passes. It is an extreme finite-value reporting defect, not measured OCR degradation.

Ticket133's initial candidate passed all96 replay terminal events in each build, retaining59 old events and adding37. Root then found four additional unreachable draw states that both original and candidate accept. The candidate is explicitly incomplete and is not to be adopted. These are the same restore-counter cause: derive whether any historical Add/Sample could draw, rather than only checking the lower bound after a full reservoir. Candidate validation results do not satisfy the whole ticket. No formal replay source changed.

A separate read-only follow-up checked the zero-draw necessary condition against Add, Sample and shuffled. Root read the same full paths. If neither reservoir overflow nor historical sample groups could ever draw, a nonzero Draws is impossible for keep or drop retention. min(Seen,Capacity) is only an upper bound on drop history, sufficient for this conservative implication. Exact single-draw examples depend on the stream because rejection can add raw draws. This does not prove arbitrary snapshot reachability, does not complete candidate133, and has not been adopted or executed as a new regression.

Final31 governance events passed using the already-built root test binary. All33 necessary non-cache payloads were archived and checked. Cleanup removed28,199 entries and1,786,439,615 bytes from the owned inode58105910 only. All848 source fingerprints,18 model files,6 persistent data files,15,046 original module files,32 foreign file fingerprints and both older work roots remain unchanged. Git delivery is still pending.

The raw standard Go flag help contains spaces followed by a tab. Its exact receipt hash remains unchanged. A single path-specific .gitattributes whitespace exception preserves this CLI readback, following the existing raw-output convention. No source indentation or general whitespace policy changed.
