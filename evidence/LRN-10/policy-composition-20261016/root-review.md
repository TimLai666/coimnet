# Root review

Scope: ticket131 holds fast state during fixed scoring and scoring after adaptation. It does not change feedback permissions or reset semantics. The user-approved neutral gain guard remains present at modulation/effects.go:208; this turn does not change it.

## Verified cause and common path

Fixed evaluation and adaptive scoring both enter scoreItems; ShuffleInvariance also uses that function. The old consumer called native Advance, which intentionally tracks eligibility and decays fast state with no gate and may write new changes with a receptor gate. The public FreezePlasticity oracle and main specification 7.2 require the complete fast state to be held instead. All 18 existing-entry reproduction leaves failed on the original 846-file baseline; additional new-operation and concurrent-operation leaves make 35 failing terminal events including parents. No compilation failure was used to prove the original evaluator bug.

Flow after repair: RunAdaptiveEvaluation / ShuffleInvariance -> scoreItems -> AdvanceFixed -> the existing locked validation, stepwise candidate, readout and commit path.

The only production changes are one additive method and the shared scorer consumer/comment. AdvanceFixed holds the same individual lock throughout the temporary internal freeze. The restoration defer runs before the unlock defer. No other public operation can observe the temporary flag. The locked engine does not replace the plastic runtime, so the defer restores the same runtime. Native Advance, AdvanceGated, online130 helpers, model declarations and persisted formats are unchanged. Effective weights retain base, slow and held fast values. Neural/chemical source steps, sensitivity and expression retain the existing shared calculation.

| Failure or interaction | Observable contract | Check |
| --- | --- | --- |
| nil/zero individual, nil/canceled context | error, no output or publication | public fixed-operation tests |
| empty/malformed input | full snapshot unchanged, later native operation unaffected | public refusal tests |
| unfrozen/frozen caller | existing runtime policy restored on success | three-core native-continuation oracle |
| simultaneous fixed/native input | each operation applies its own policy under the same lock | concurrent zero-input fast-decay count |
| fixed/adaptive scoring | fast state held, neural/chemical evolve | three-core retained/reset and receptor controls |
| disabled plasticity | same full snapshot/output as native Advance | disabled control |

## Implementation receipt

OpenCode failed in this turn with `Unknown: FileSystem.open (/Users/timlai/.local/share/opencode/log/opencode.log)`. The project-directed fallback was native gpt-5.6-luna with max effort. Two read-only workers inspected adaptive/replay contracts. A separate worker implemented only learning/individual.go and experiment/adaptive_run.go. Root rejected its first return statement because a three-result private helper cannot be returned by a two-result method; the worker corrected only that adapter before Root's Go run. Tests were Root-authored and retained, apart from Root's separately recorded gofmt alignment.

Root read the entire production diff and verified all receipts directly. General and race focused runs each pass all 35 terminal events. Twelve historical chemical snapshots resume to the exact previous per-build output bytes. A separate adversarial worker found no confirmed defect in the narrow patch. Its report misnamed the online helper file in one reference; Root independently checked the actual unchanged learning/individual_online.go. Static review is not a substitute for the engineering receipts.

The first full unit run passed 8,435 events and skipped 23, with one failure: the format index still named the shifted constant's old line 276. Root updated only docs/INDEX.md to actual line277 and reran the complete gate with separate r2 logs. The original failing evidence is retained. No test expectation was weakened.

## Additional findings, outside this patch

Root verified both workers' replay candidates and the adaptation permission candidate through public APIs, using normal finite inputs. Normal/race ten-row results are byte-identical. Ticket132 owns unpermitted target-derived score updates (P1). Ticket133 owns invalid SHA-256 text and impossible reservoir draw state acceptance (P2). LRN-10 and LRN-07 remain specified until their respective remaining defects are repaired; an engineering pass for this patch cannot restore those requirements by itself.

Resetting fast state discards the slow runtime and resetting chemistry can discard task resources/pending feedback depending on receptor dependencies. These are observations needing an evaluation-specific contract decision, not confirmed algorithm defects in this ticket. The existing timing ambiguity about pre activity remains unchanged. Repeated balanced samples or identical replay content also lack the contract needed to call them bugs.

## Subtraction review

No second evaluator or stepping engine, model flag, persisted field, schema version, dependency or CLI option was added. The common scorer removes the divergent inference policy. Separate pending defects are recorded without folding them into this patch. No task accuracy or biological-calibration conclusion is drawn.

## Final engineering and preservation

Root resumed the existing full-engineering process without starting a second race run. The completed r2 gate passed 58 packages in each mode: 8,436 pass / 23 skip for ordinary and 8,435 pass / 24 skip for race. All 8,424 prior terminal events retained their previous actions; the only additions are the 35 passing regression events. Build, formatting, vet, module verification and the pinned Insyra v0.3.4 check passed. Final governance passed 31 events. The separate archived ten-row public probe and candidate132 overlay do not count as formal production fixes.

The guarded cleanup removed 1,718,722,369 bytes from the owned work root after preserving 21 necessary files in preserved-work.tar.gz. The original 847 source files, 18 model files, six persistent data files, 15,046 module-cache files, 32 foreign files and two protected old work roots remained unchanged. Root verified compressed and uncompressed full-test log hashes, final-governance log hash and archive/cleanup hashes again before staging. Cleanup is complete; commit, push and exact remote readback remain pending at this record.

The approved neutral-range rejection was already delivered by ticket89. Its commit is an ancestor of the current baseline, the guard at modulation/effects.go:208 remains unchanged, and all 21 associated terminal events pass in the current full unit log.

A final independent read-only evidence review found stale cleanup/race progress and an obsolete uncompressed race-log link in the file inventory. Root updated only those closure records. The reviewer used the project-directed gpt-5.6-luna/max fallback after this turn's OpenCode probe failed with `Unknown: FileSystem.open (/Users/timlai/.local/share/opencode/log/opencode.log)`. No worker reran Go or modified formal sources.

The first staged whitespace check flagged only the exact gofmt unified-diff context prefixes in format-new-test.log. Root preserved those raw log bytes and added the repository's existing per-file whitespace attribute pattern for this one log. The production and regression source formatting remains governed by the completed Go formatting checks.
