# Test Report: Doctor GPU capabilities (ticket 44)

Tests run: 13 selected top-level cases | Passed: 13 | Failed: 0 | Fixed: 1 diagnostic defect.
The full ordinary and race suites each passed all 57 tested packages. These counts overlap and are not added together.

## Changes tested

- Public Doctor and JSON report the six constrained GPU paths and all 16 details fields, preserve zero delay and the CPU fields, and separate hardware inventory from core execution.
- Newly built CLI: normal hardware probe, missing optional tools, command help, overview, positional-argument rejection and unknown-flag rejection passed.
- Nil/canceled context, failed output and independent details allocation passed.
- Build, vet, gofmt, module verification, tidy diff and target-flow checks passed. Frozen tests, all 646 Go sources and 2,953 unrelated existing files matched their fingerprints.

## Bug fixed

The six GPU capabilities were fixed as not_implemented despite the existing constrained WebGPU scalar continuous adapter and episode trainer. Root reproduced four changed-feature failures before implementation; all 13 selected cases now pass.

## Regression coverage

`internal/cli/doctor_test.go` adds TestDoctorCoreCapabilitiesDetailsAreIndependent, TestGPUProbeMissingOptionalToolDoesNotClaimCoreExecution, TestDoctorRejectsNilContext, TestDoctorRunWritesV1JSON, TestDoctorHelpDescribesGPUConstraintsAndExecutionSeparation, TestDoctorRunRejectsInvalidArguments and TestDoctorRunPropagatesOutputFailure.
The existing complete-report and cancellation checks were strengthened.
`cli-workflow.py` independently verifies the real binary and removes its temporary artifacts.

## Remaining limits

OPS-05 and TSK-11 remain specified. This correction does not add GPU core execution, full-graph GPU validation, cross-platform execution, navigation learning or biological evidence. See verification.json for commands, logs, environment and fingerprints.

Recommendation: SHIP.
