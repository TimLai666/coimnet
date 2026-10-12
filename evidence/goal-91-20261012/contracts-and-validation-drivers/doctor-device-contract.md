# GPU device trainer discovery

The doctor JSON and its help currently describe only the legacy host AdamW
trainer and incorrectly appear to rule out all device updates. Preserve existing
v1 fields for that default SDK mode. Add `trainer=learning.NewGPUTrainer` and
`device_trainer` details for the opt-in learning.NewGPUDeviceTrainer:

- Constructor: learning.NewGPUDeviceTrainer
- RestoreConstructor: learning.RestoreGPUDeviceTrainer
- Optimizer: device_adamw
- Precision: float32
- ParameterMirror: verified_cpu_mirror
- Snapshot: complete_training_bundle
- UnsupportedOptions in this exact stable order: lif, vector_matrix_edges,
  delays, recompute, persistent_neural_state, ranges, fixed_signs, weight_sharing
- Execution: not_probed
- FullGraph: unverified

GPUDeviceTrainerDetails and nil deviceTrainerCapabilities skeleton are root-owned.
coreCapabilities must return independent owned pointer/slice values per call,
and keep legacy cpu_adamw/not_implemented scoped to its named trainer. The doctor
does not perform actual GPU training and must not claim hardware verification.
Help names both constructors and restore, opt-in float32/resident optimizer,
complete bundle save/restore, restrictions, and retained host neural state,
activations, sparse transfers and Insyra execution limits. Existing cpu_adamw,
not_probed, full_graph=unverified and recompute phrases remain accurate.

Test worker owns new internal/cli/doctor_device_mode_test.go and only the expected
GPU details helper in existing doctor_test.go. New tests: exact JSON descriptors,
owned pointer/slice, public doctor --help discoverability. Helper update is justified
by this additive contract; other existing assertions remain unchanged. Root
reviews and runs all new tests RED before a separate implementation worker fills
doctor.go helper+coreCapabilities and run.go help; it cannot modify tests.

Acceptance: actual doctor command JSON + help, all internal/cli unit/race,
full native checks, backward-compatible existing legacy fields.
