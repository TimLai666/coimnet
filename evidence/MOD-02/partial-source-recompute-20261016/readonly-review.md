# Read-only contract and source review

Two independent native reviewers inspected current source and existing tests. Neither executed Go nor modified files. They found no confirmed production defect; runtime behavior is a separate gate.

Existing full-history Continuous stateful gradients cover a partial 3/4 controller window and one chemical channel (`learning/chemical_stateful_test.go:325-383`). Existing ordinary/recomputed episode tests cover a full four-row window and three channels for both Continuous and LIF (`learning/chemical_recompute_test.go:16-253,364-696`). These do not establish the new combined partial-window, five-source, long-delay matrix.

`ChemistryConfig.Validate` requires exactly one source per channel in channel-index order (`modulation/config.go:199-220`). The original Root draft proposed five sources with three channels; the diagnostic worker correctly refused that invalid fixture before writing code or running it. Root read the validation and corrected the diagnostic contract to five channels. This was a diagnostic authoring mistake, not a framework bug. Source ablations preserve dimensions and replace only one source with a legal zero-release timeline.

The controller's oldest-to-newest window has an all-zero unfilled prefix, and reverse returns zero for those rows (`modulation/controller_state.go:5-62,253-286`). Every restart clones the boundary and advances the absolute source clock while preserving the independent kinetics clock (`learning/chemical_recompute.go:28-47`). Controller and NeuralActivity have neural activity derivatives. Replay, resource and external timeline inputs remain fixed episode inputs, as declared.

Reverse truncation uses fixed windows of local episode row indexes. It does not use absolute SourceStep, and it is not reset at recomputation segment joins (`learning/chemical_sequence.go:492-510`, `dynamics/coupled_reverse.go:47-58,112-185`). Delay history is retained independently of segment length. No concrete indexing/ownership mismatch was established from reading those producers.

Root narrowed inaccurate worker wording: PredictAll publishes readout rows, not membrane voltages or spikes. The public recomputed Trainer path does not publish all observation/initial-state derivatives. The new audit compares complete trainable parameter gradients through the first pending accumulator, plus all public update state. Full-history observation gradients receive an independent finite-difference reference. Unsupported Mixed/stateful/GPU/vector chemical recomputation remains refused, not reclassified as a defect.

These findings locate coverage and support the chosen diagnostic. They do not prove absence of defects, biological calibration or task-quality improvement.
