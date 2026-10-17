# Root review

Scope: the common per-item chemical reset used by fixed scoring, adaptive actions, adaptive scoring and ShuffleInvariance. EnableChemistry already validates the same declaration and retains task resources/pending feedback; disabling is the only divergent operation removed. All remaining DisableChemistry call sites are explicit off-operation tests, not another evaluation reset.

The regression uses public SDK APIs and an independent kinetics value, 2*(1-exp(-.25)), after a nonzero warm concentration. Ordinary/race original versions fail the four ungated reset paths and their parent (14 pass/5 fail). Final related tests each pass 136 events. An overlay which skips chemical initialization is independently rejected (10 pass/9 fail per build), so merely retaining old concentration is not an accepted fix.

The test worker did not alter production code; a different worker supplied the one-line candidate only after Root verified failures. Root inspected the full test and exact byte diff and ran every Go command. The first manually written probe had a compile error from Pending instead of PendingFeedback; that setup error and its log are retained separately and do not count as a framework defect.

The twelve old checkpoint files are actual pre-change saved inputs, loaded in new ordinary/race processes. The driver scores two items with ChemicalAtItemStart, then runs another training episode. Full JSON output and snapshot bytes match original within each build. 64 ordinary no-chemistry reports likewise remain byte-identical.

Chemical reset continues using existing runtime replacement semantics. This patch adds no rollback promise for an entire evaluation and does not decide whether PlasticAtItemStart should retain slow consolidation or existing runtime freeze flags. Those policies need separate contract decisions.

A separate public AggregateCells bug has been reproduced and tracked in ticket136/LRN-08. Extreme finite score aggregation may return infinity; the current generated task range has not been shown to reach those values. Do not claim 91/91 or zero bugs while that ticket remains unverified.

Subtraction review: reuse EnableChemistry and remove the uninstall step. No new API, helper abstraction, dependency or model format is required.

Cleanup audit review: accepted the input/count checks after Root independently hashed the frozen sources, original model/data/module files and foreign files. The audit inferred live status from logs and work-in-progress metadata; only Root actual exec-session polls establish liveness. E136 acceptance need not block E135 temporary-directory cleanup after every E136 candidate/input/log is independently archived and read back. The archive guard, not completion of the next ticket, controls deletion.

The second preservation audit is a snapshot: its39 numerical-candidate counterparts do not include later mutant controls, which Root separately copied and hashed. Its blanket statement that gz content was not read has an archive exception: it did read the22-member preserved-work.tar.gz. Root separately read back all22 members and all5 supplementary baseline members, and verified the four baseline documents equal HEAD bbfd2b3. Final cleanup still requires complete per-file accounting of every non-cache input and no live process handles.
