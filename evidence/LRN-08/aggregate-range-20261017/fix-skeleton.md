# Ticket136 next numerical slot

Public AggregateCells currently returns Inf,nil for finite successful seed scores even when the mean and population standard deviation are representable. Two public reproductions are preserved in evidence/LRN-08/aggregate-range-20261017. This slot remains separate from the frozen ticket135 source/engineering run.

Production boundary for the later implementation: experiment/continual_math.go AggregateCells only, with a small private numerical helper if needed. Preserve cell dimensions, Total/Succeeded, failed-mask filtering, first-item min/max tie behavior, zero-success result, normal arithmetic order and every ordinary float bit when no representability repair is required. Valid saved models and report schemas must remain compatible. Do not change scoreTwin, bootstrap or learning rules.

Expected supported input: successful values are finite. A nonfinite successful value cannot form a valid finite cell statistic and should return an error with seed/cell context and no result. Failed seed values must be ignored even when NaN/Inf. This is an error-path repair, not a new metric definition.

Numerical expectations from exact finite float64 input values:
- equal large values +/-MaxFloat64: finite same mean, zero Std;
- -1e308 repeated: finite same mean, zero Std;
- -8e307/-7e307: finite mean and population Std half their exact difference;
- {-MaxFloat64,0,0}: finite mean and Std;
- {-MaxFloat64,+MaxFloat64}: zero mean, Std=MaxFloat64;
- {-1e-200,+1e-200}: zero mean, Std=1e-200 despite square underflow;
- {-SmallestNonzeroFloat64,+SmallestNonzeroFloat64}: zero mean, Std=SmallestNonzeroFloat64;
- values with variance below the smallest normal float need a reference which does not round the variance to zero before sqrt;
- ordinary [-8,-7] yields Mean=-7.5, Std=.5 exactly; preserve Min/Max and seed counts;
- nonfinite successful scores error, nonfinite masked scores are ignored;
- empty/all-failed cells and malformed dimensions keep their documented behavior.

Test slot only for now: scratch aggregate_tests.go.txt, external package experiment_test, public API, independent hand-derived or high-precision references. Tests must compile on current source and fail numerically or by missing errors, not a missing API. No formal source/test mutation while ticket135 full engineering is running. Root must review/freeze/run red before a different worker fills the implementation slot. No Go commands by workers.

Candidate algorithm direction for later review: retain the existing normal fast path, fall back for nonfinite or material underflow intermediates to finite input exact moments (rational mean and variance) and high-precision sqrt. Do not merely divide every input before adding, since that loses small cancellation results; do not force current outputs finite by clipping. Final algorithm must meet all expectations above and root-verified ordinary compatibility controls.

Root implementation slot after red verification (ordinary/race each15fail/7pass): collect successful values exactly as before, but reject a nonfinite successful value before statistics with seed/row/column context and nil result. Leave all failed values ignored. Compute normal mean, squared-deviation sum, variance and sqrt in the existing order; record whether any deviation is nonzero. Use an exact-moments fallback if mean/dev/std is nonfinite, variance is positive but below 0x1p-1022, or variance is zero despite a nonzero deviation. The fallback computes exact rational E[x], E[x*x]-E[x]^2 from the original finite float64 values, and sqrt at 256-bit precision before converting Std once to float64. A local helper is sufficient; final finite result checks remain at the public cell boundary. Keep every other function and min/max/count/filter behavior unchanged. Candidate may add only math/big to standard imports. All formal files remain untouched until ticket135's frozen engineering barrier is finished.
