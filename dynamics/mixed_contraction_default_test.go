//go:build !race

package dynamics

// lifReferenceContractsLikeMixed records, per build, whether LIF.Forward rounds
// its membrane expression the way this package's membrane helper does.
// TestMixedMembraneContraction checks that this constant still describes the
// build it was compiled into, so it can never go stale unnoticed.
const lifReferenceContractsLikeMixed = true

// lifReferenceTolerance is unused in this build: every comparison is exact.
const lifReferenceTolerance = 0.0
