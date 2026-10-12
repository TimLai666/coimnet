//go:build race

package dynamics

// Allocation profiles are checked in ordinary builds. Race instrumentation
// randomly drops fmt printer pool entries, so its counts are not deterministic.
const checkCoupledAllocationProfile = false
