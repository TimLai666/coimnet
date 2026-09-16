//go:build race

package dynamics

// lifReferenceContractsLikeMixed is false under -race: the race instrumented
// build compiles LIF.Forward's membrane expression without the fused
// multiply-add an ordinary build uses. See TestMixedMembraneContraction.
const lifReferenceContractsLikeMixed = false

// lifReferenceTolerance bounds what that one difference can cause. It is
// 1e-16 relative to 1+|want|, which is below one adjacent float64 value at unit
// scale; the largest deviation the fixtures of this package actually produce is
// 1.39e-17, measured by lowering this constant until the comparison failed.
const lifReferenceTolerance = 1e-16
