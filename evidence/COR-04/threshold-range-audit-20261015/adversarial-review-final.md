# LIF threshold-range adversarial final review

## Problem

Ticket 103 addresses binary64 overflow in the threshold transform for the legal ordered range `ThetaMin=-2^1023`, `ThetaMax=2^1023`, and rejects a non-finite effective threshold before a trace, segment, recomputed result, or persistent state is published.

## Scope

- Complete read-only comparison of `dynamics/lif.go`, `lif_coupled.go`, `lif_segment.go`, `lif_state.go`, `mixed.go`, and `mixed_state.go` against `evidence/COR-04/threshold-range-audit-20261015/before/`.
- Read-only review of `dynamics/lif_threshold_range_test.go`, `dynamics/lif_effective_threshold_range_test.go`, ticket 103, `ENG.md`, the threshold-transform inventory, `experiment/lif_threshold.go`, and `internal/delayedfixture/delayedfixture.go`.
- Validation used source/evidence inspection and Python standard-library `math`/`fractions` reference arithmetic only. No Go command, test, race run, build, training, download, or other runtime execution was performed by this reviewer.

## Resolved finding

The original P1 is resolved by the current diff.

- The prior evidence `evidence/COR-04/threshold-range-audit-20261015/effective-threshold-probe.json` records the old public probe returning finite outputs even though the second-step effective threshold was unrepresentable. `root-effective-red.json` records the corresponding regression failures for ordinary forward, recompute, and persistent paths.
- The overflow-only transform now uses the endpoint-scaled equivalent when `span=ThetaMax-ThetaMin` is `+Inf`: `dynamics/lif.go:331-339`, `lif_coupled.go:91-99`, `lif_segment.go:97-105`, `lif_state.go:378-388`, and `mixed.go:325-350`. The normal finite-span expression is unchanged. `mixed_state.go` uses `mixed.go:baseThreshold`, so it does not duplicate a formula.
- Python binary64 reference arithmetic produced `span=+Inf`, fallback midpoint `+0`, and fallback slope `2^1022` for `[-2^1023,+2^1023]` at `s=0.5`. Endpoint inputs `s=0`, `0.5`, and `1` produced finite bases and slopes. The adaptation fixture produced a finite first step and `theta=+Inf` on the second step while the candidate, voltage, synaptic output, and adaptation values remained finite. The homeostasis fixture likewise produced `theta=+Inf` with a finite homeostasis ceiling. These are the concrete old failure triggers, not hypothetical overflow cases.
- The new final finite checks are present before publication in every ordinary path: `dynamics/lif.go:418-425`, `lif_segment.go:225-233`, `lif_state.go:335-357`, `mixed.go:499-506`, and `mixed_state.go:420-438`. A failure returns before trace/segment rows or persistent state are committed. Recompute callers propagate segment failure as nil/empty results in `dynamics/forward_recompute.go:190-198` and `lif_recompute.go:82-84,121-123`; the new regression assertions cover this at `dynamics/lif_effective_threshold_range_test.go:165-218`.
- Coupled paths already had the equivalent effective-threshold rejection at `dynamics/lif_coupled.go:280-282` and `mixed_coupled.go:226-228`, with non-finite shifted thresholds rejected earlier at `lif_coupled.go:249-252` and `mixed_coupled.go:193-203`. The current change aligns the ordinary and persistent paths with that existing final boundary without changing their normal arithmetic.
- The persistent regression cases check first-step success, two-step failure, zero/nil failure returns, and preservation of the caller state and arguments at `dynamics/lif_effective_threshold_range_test.go:220-294` and `:296-374`. Adaptation and homeostasis variants are covered at `:377-413`. The source also preserves the existing batch and empty-slice state-copy behavior covered by the new tests.
- Public construction still requires finite ordered bounds and `VReset < ThetaMin < ThetaMax` at `dynamics/lif.go:152-153`; `NewMixed` delegates that validation at `dynamics/mixed.go:197-205`. Existing invalid-boundary cases remain covered at `dynamics/lif_threshold_range_test.go:368-400`, while ordinary finite-range, saturation, state, and gradient cases remain covered at `:74-181` and `:270-365`.
- The only non-core formula found by the supplied inventory is `experiment/lif_threshold.go:263`. Its callers use the fixed `0.05..1` example range (`internal/delayedfixture/delayedfixture.go:66-83`), so it is outside the configurable threshold-range contract and is not a finding.

## CONFIRMED

No confirmed findings.

## NEEDS INVESTIGATION

There is one contract question, not a confirmed defect in this patch:

- `dynamics/modulation.go:95-104` floors `theta+shift` at `theta_min` without first checking whether the finite intermediate became `-Inf`. The coupled implementations reject a non-finite shifted intermediate before flooring (`dynamics/lif_coupled.go:249-252`, `mixed_coupled.go:193-203`). A concrete source-level boundary case is `ThetaMin=-2^1023`, `ThetaMax=2^1023`, `ThetaRaw=-1000` (base `-2^1023`) plus finite shift `-MaxFloat64`, for which the ordinary sum is `-Inf` and the floor returns finite `ThetaMin`, whereas coupled code rejects. The ticket and `ENG.md` specify the lower-floor behavior and final effective-threshold rejection but do not state whether an unrepresentable pre-floor intermediate must be rejected in ordinary paths. A public paired regression case and an explicit contract decision are needed before classifying this as a bug.

## 未驗證事項

- This reviewer did not execute Go tests or any runtime command. The read-only evidence `final-target-summary.json` records the root's targeted 40-event green result, but it is not independent execution by this review and does not establish full-package or runtime compatibility.
- Root's broader runtime compatibility rerun was still in progress at review time.
- Long-term training quality and full-model behavior are outside ticket 103's numerical boundary scope.

## Done

The original P1 was rechecked against the complete source diff, all corresponding ordinary, coupled, recompute, mixed, and persistent paths, endpoint arithmetic, invalid-boundary behavior, and the two permanent regression suites. It is resolved on source and test-contract evidence.

## Changed

| 檔案 | 變更摘要 |
|---|---|
| `runs/.tmp/lif-threshold-range-20261015/adversarial-review-final.md` | 新增本輪唯讀數值審查報告。未修改任何核心程式或測試。 |

## Actions

| 動作 | 對象 | 結果 |
|---|---|---|
| 唯讀 diff／來源／證據審查 | Ticket 103 的指定範圍 | 完成，未提交、未 stash、未 reset。 |

## Verified

已確認溢位專用等價公式只在 `span=+Inf` 分支使用，合法有序端點的中點與斜率有限；五個普通／持久化路徑在發布輸出或提交狀態前拒絕非有限 `theta`；耦合路徑、重算路徑、錯誤返回與狀態保留均有對應來源或回歸斷言；正常有限公式與既有錯誤端點驗證未見本次 diff 改動。

## Notes

唯一待釐清項目是普通調變路徑的負向中間值下溢與耦合路徑拒絕邊界是否應一致，已列為 NEEDS INVESTIGATION，沒有當成確定缺陷。

## Next

Root 可依其完整套件與 runtime compatibility 結果裁決是否另開調變負向中間值契約票。

## Skills

Skills：diff-inspector
