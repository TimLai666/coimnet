# Root implementation slot

Only learning/consolidation.go may change. Frozen tests: frozen-checks.json. Valid RED: red-verification.json (129 events, 74 pass and 55 boundary failures). Source baseline has 844 unchanged old files plus one new test.

Keep Consolidate's validation, gate/already/budget, candidate finite-value error, delayed effective-weight error and mutation order unchanged. Do not return a new error earlier. The report-norm error must remain before the second ctx.Err and effectiveErr return. Keep normal two-pass operation and existing arrays.

Use one private helper with signature `func refineConsolidationNorm(norm float64, values, added []float64, scale float64) float64`. The arguments are internal validated arrays; added is nil or the same length as values. With added nil, the rounded component is scale*values[k]; otherwise it is values[k]+scale*added[k]. Base uses weights,nil,1; slow uses slow.Values,plastic,Rate; retained plastic uses plastic,nil,Retain. Preserve the exact candidate/retained expression used by the existing scan.

Keep the original sequential Hypot reductions. For len(values)<2 or norm <= MaxFloat64/float64(len(values)), return the old result unchanged. Hypot is at least maxabs, so in the latter case the true norm is far below the overflow boundary even for long vectors. Otherwise compute the exact sum of squared rounded components, with precision 2*(1024+1074)+64 (4260 bits), classify against squared nearest-even overflow midpoint (2^1024-2^970)^2, return +Inf when at/above it. When below it, sqrt and convert to float64. This rare path must correct both false finite and false +Inf results, including legitimate MaxFloat64 reported one ULP too low. Finite guards inside this rare path may return nonfinite for the original final norm refusal; do not add normal-path duplicated validation or public policy.

Integrate into l2's final return for Base (private l2 only has one caller) and after the existing candidate scan for Slow and Plastic, before the original final norm finite check. Do not allocate candidate vectors or build an entire model. Preserve snapshots, counters, caches, formats, domains and dependencies. math/big is standard library.

Root alone runs focused ordinary/race, public probes, full engineering and semantic mutations. The worker may only use gofmt and read-only diff. Never edit test files. If a test seems wrong, report it and stop that change.
