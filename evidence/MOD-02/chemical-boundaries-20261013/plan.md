# Chemical state and source validation audit

Baseline: dd75a62d152e0066ff04c3c917adbd66e4ebb0d8. Native local APFS, CPU reference. This stage checks three public failure paths against existing finite-value, source-width and atomic-replacement contracts. It does not measure OCR accuracy or establish biological completeness.

| Ticket | Public flow / root cause | Production slot (Root only) | Independent test slot | Expected result |
| --- | --- | --- | --- | --- |
| 78 | NewChemistry → Step / Backward; finite clearance rate plus finite boost can overflow | modulation/chemistry.go | modulation/clearance_overflow_test.go | Derived nonfinite rate rejected by both APIs before any caller state changes; finite/no-boost outputs and gradients unchanged |
| 79 | NeuralActivity / InternalResource Release and SourceSpec.Build; Channel+1 can wrap | modulation/sources.go, modulation/config.go | modulation/source_channel_overflow_test.go | MaxInt channel rejected with error; no panic/negative-width source; finite ordinary channel routing unchanged |
| 80 | EnableChemistry replaces receptor records still referenced by ExpressionGain | learning/chemistry.go | learning/chemistry_expression_replacement_test.go | Invalid replacement rejected with entire old snapshot/report preserved; subsequent inference/training/restoration matches untouched twin; compatible replacement and explicit ClearExpressionGain succeed |

Each ticket owns its root cause and its nil/empty/zero/error cases. The slots are independent and share existing public APIs. No new schema, dependency, or numerical algorithm is introduced. A valid replacement resets chemistry as before; its resources and pending feedback remain as before. Exp underflow from a finite rate remains a valid saturated decay limit.

Verification: independent tests first, save failures against the baseline; freeze tests before Root implements. Then run focused GREEN and race; compare legal baseline/current public outputs and gradients in separate processes; gofmt, build, full uncached unit/race, vet, module verification and existing governance scripts. Fingerprint tested sources. Save commands, environment, logs and outcomes in this directory. Review the final diff, clean only runs/.tmp/chemical-boundaries-20261013 after evidence is retained, commit/push and read back the remote commit.

Delegation: current opencode models failed while opening its existing log; no model request succeeded. Project fallback is gpt-5.6-luna at max. Agents own tests only, cannot fill production slots or edit another worker's checks. Root reviews and runs them.

減法審查：只補既有入口缺少的檢查，移除另建備援運算或自動清除輸出調節的方案。這階段不變更受體生理模型、模型格式或任務訓練設定。
