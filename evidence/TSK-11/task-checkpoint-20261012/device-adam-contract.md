# Root device optimizer contract

Scope: real hardware WebGPU float32 AdamW primitive, no CPU fallback. This unit alone DOES NOT complete OPS-05 or full GPU training. Existing CPU/GPUTrainer unchanged.

API fixed in backend/webgpu/device_adam.go. Config finite Beta1/Beta2 in [0,1), Epsilon positive finite, WeightDecay >=0 finite, MaxBytes positive hard total GPU buffer budget. New requires nonempty finite params; all failures validate before allocating hardware.

State interleaves p/m/v/step (16 bytes/value), incoming gradient/mask (8 bytes/value), private shadow for atomic commit. Every step validates gradient length/finites, mask length, lr positive finite. Only mask=true values change p/m/v/step, including decay. Non-enabled params/moments/step stay bit-identical. Per-parameter counters guard uint32 overflow before commit. Gradient accum/clip/schedule belong to learning caller, not this primitive. Formula m=b1*m+(1-b1)*g; v=b2*v+(1-b2)*g*g; p=p-lr*(m/(1-b1^step))/(sqrt(v/(1-b2^step))+eps)-lr*decay*p.

Chunk ALL values internally to satisfy device storage/buffer/workgroup limits without reducing declared params. Resource estimate includes live+shadow state,input,status,uniform,readback buffers. Reject insufficient MaxBytes before buffers. Resident parameters and moments uploaded ONCE; steps upload gradients/masks/settings only, readback finite/error flag only, no state roundtrip. Snapshot explicitly reads all states; RestoreDeviceAdamW validates schema, shapes, finite values, v>=0 and step/moment consistency then uploads complete state. Mask all false is legal no-op.

No CPU update calculation permitted on GPU path. WGSL writes shadow candidate and checks non-finite intermediates/result by float exponent bits; failure/cancellation anywhere leaves EVERY chunk at previous parameter/moment/step. Commit swaps all device buffers only after all flags validated and ctx checked. Snapshot/Step/Close serialized and resource-safe. Context nil/canceled, closed/nil receiver, invalid config/shape/non-finites and over-budget return error. Close idempotent.

Metrics include hardware identity, constructor phases and actual allocated/upload/readback bytes. One step upload <= 8*N + 32*chunks + status-reset*chunks; readback <=4*chunks. Snapshot may read 16*N explicitly. Each hardware test must Skip only ErrUnavailable; formal evidence demands at least one non-skipped hardware test, records skip separately.

Tests owned by test agent: backend/webgpu/device_adam_test.go only. Root owns implementation splitfiles device_adam.go/device_adam_buffers.go/device_adam_execute.go/device_adam.wgsl, cannot edit tests except independently proven test errors.

Acceptance: hand/intrinsic-free CPU f32 multi-update reference+mask+decay; 130 values boundary; save+restore/new object continuation; canceled/NaN/Inf/overflow rejects without mutation; corrupted snapshot/preflight fail; output allocation/transfer metrics prove resident state. GPU fullgraph/integration remains later mandatory work.
