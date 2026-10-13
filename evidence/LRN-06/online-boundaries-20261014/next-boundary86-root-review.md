# Root review of ticket 86 preparation

This preparation is an unexecuted draft, not a formal regression or a ticket 86 acceptance result. Root read both worker artifacts in full and checked their production fingerprints against the current source.

The draft exercises public Act, Update and TrainEpisode, with public records and complete snapshots as result oracles. Its ordinary-path serial control advances neural state before external training. Score-zero checks preserve legitimate optimizer counters rather than incorrectly requiring the entire optimizer state to stay unchanged.

Before adoption, Root must compile and run the overlay on the frozen ticket 85 baseline. The private mutex token and runtime stack are synchronization diagnostics only; channel fairness is not a language contract. Runtime stability remains unverified. Failure cleanup must release both contexts and join every started goroutine; the current release-only defers are not sufficient evidence of goroutine cleanup after a failed assertion. Chemical, frozen-mask, accumulation and cancellation cases still require coverage.

No production source or formal test was changed for this preparation. Public v3 diagnostics separately confirm the two pre-existing defects, but a diagnostic PASS is not a fix PASS. The useful draft is retained here before removal of the owned work directory.
