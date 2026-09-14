# SIG-04 implementation review

Scope: CLEAN. Reviewed against the original SIG-04 fixture acceptance and specification section 6.4, including persisted input/output projections, metadata selection, deterministic reconstruction, binding/replacement, selected-input gradients and checkpoint consumers.

Adversarial review: RUN with GPT-5.6-Luna at max effort after the requested Spark call reported a usage limit. The primary agent independently inspected the network/trainer changes, new public APIs, tests and documentation. The final review found no remaining confirmed defects.

## Findings resolved before delivery

- Metadata-selected neuron order could be changed and its hash recomputed, allowing decoding even though graph binding would later fail. Projection validation now rejects noncanonical metadata order during decoding. Explicit selections preserve the caller's order. A failed load leaves the original projection unchanged.
- Go's JSON decoder replaced invalid UTF-8 and unpaired UTF-16 escapes with U+FFFD before field validation. The signal package now checks the original text before decoding, and NeuronID validates UTF-8 before construction/marshaling. Regression coverage includes the legacy Signal, Mapping and NeuronID entry points, valid replacement runes, paired escapes and literal backslashes.

Both were reproduced before fixes; see [root causes and test fingerprints](red-reproduction.json), [projection failures](review-red.log) and [legacy signal failures](unicode-red.log). Luna subsequently reviewed the fixes and reported no additional confirmed defects or outstanding investigation items.

## Contract checks

`Spec()` provides constructor settings. Passing those settings to NewProjection deliberately constructs a new mapping from current candidates; DecodeProjection plus Bind restores and validates the original resolved selection. ValidateChannelShape checks rank before flattening; BindProjections checks the flattened model widths. These distinct contracts are documented in the [usage guide](../../docs/signal-projections.md).

InputNodes uses a compact encoder, scatters the forward result into the core and gathers its reverse gradient. A dense expansion and finite differences cover all parameter groups and observations. Replacement owns its configuration/parameters, preserves the supplied core and creates a fresh optimizer through NewTrainer. Original trainer state remains unchanged.

The primary agent also reviewed strict-input limits, malformed values, output data ownership, fixed-random reconstruction, graph identity assumptions and documentation. No external services, authentication policies or database migrations are involved in this change.

## Final verification

[The v22 report](../cpu-reference-20260914/verification-v22.json) records identical 127-file sources on macOS arm64 and Ubuntu amd64. Both passed formatting, module verification, build, full tests, race, vet and CLI checkpoint comparisons. Each platform also passed 20 matching projection/input-selection/Unicode tests and examples, including selected-input checkpoint resume in a separate process. Windows crosscompilation passed. Windows runtime, GPU execution and biological mapping efficacy were not validated.

The initial v21 test run is preserved as historical evidence and superseded by v22 after the review findings. InputNodes' initial missing-field red test was observed by the implementation agent but not saved as a standalone log; it is not represented as a preserved artifact here.
