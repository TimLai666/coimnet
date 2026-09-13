# Signal resampling review

Scope: new sampled-signal API in ticket 03, compared with 793d98c. Existing Signal, Clock, Observation and Stream contracts remain unchanged. Source files are identified by the final CPU verification manifest.

Adversarial review: Luna (max), after Spark (xhigh) returned a usage-limit error. The reviewer inspected the stream contract and implementation read-only. Root reviewed the code, documentation and regression evidence independently.

## Findings resolved before delivery

- Constant interpolation could move 0.1 to 0.10000000000000002 and violate a constant valid range. Reproduced in `interpolation-red.log`. Equal endpoints now retain their value exactly, and float roundoff cannot extend endpoint bounds.
- The constructor validated exclusive EndStep, rejecting a legal final sample when only the unused endpoint overflowed. Reproduced in `review-red.log`. Nonempty ranges now validate EndStep-1; empty ranges skip endpoint arithmetic. The test covers both Clock and source-ratio overflow.
- Converting large integer time differences separately to float64 rounded the fraction 2^53/(2^53+1) to one. Reproduced in `review-red.log`. Both interpolation weights are formed as exact rationals before conversion to float64. Luna reviewed the endpoint and rational-fraction corrections and found no remaining defect in those patches.
- Root extended the same numerical audit to asymmetric values: subtracting a rounded right weight from one lost a left contribution of 8.673617379884036e+81. Reproduced in `complement-red.log`. Computing the complementary weight independently fixes the failure; `final-green.log` includes the regression. This final three-line correction was reviewed by root.

The v12 formatting gate caught one spacing difference before build/tests. `format-gate.log` retains that failure. Formatting was corrected before the final source snapshot. Earlier full-suite runs are superseded by the final manifest and logs.

## Contract limitations retained

- Batch mode is explicit, while the time mapping remains in ResampleConfig. Callers must preserve the config with outputs.
- Each channel is aligned independently. Joining channels into one Observation needs an explicit shared sequence mapping. This milestone does not mark SIG-02 or SIG-05 passed.
- Input durations must be zero and kinds continuous, activity or modulation. Pulse/interval rules, media decoding, filtering and an end-to-end custom neural adapter remain work in ticket 03.
- Limits account for logical samples and values, not RSS. Values remain float64 approximations; no claim of exact rational-valued output or cross-platform bitwise numerical equality is made.

No unresolved confirmed finding remains in the reviewed scope. There are no HTTP, database, authentication, deployment or data-migration changes.

Subtraction review: reused Signal and Clock, used one processing path for offline and streaming modes, and removed the redundant retained template sample. No new dependency, scheduler, codec layer or model-specific training program was added.
