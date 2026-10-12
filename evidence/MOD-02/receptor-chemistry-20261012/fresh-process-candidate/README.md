# Chemical recompute fresh-process candidate

These are historical root-executed checks from ticket 65 implementation. Their original receipts and source hashes remain unchanged. See `../candidate-source-audit.json` for current-source differences and the parent `verification.json` for final acceptance.

The new public checkpoint test covers CPU continuous and LIF cores. Each case processes seven independent eight-row training episodes. After the first call, one gradient is pending in a three-call accumulation cycle. The real checkpoint is loaded in a new os.Executable process, restored through RestoreTrainer, and continued. The child result matches uninterrupted segmented recompute and full-history training, including parameter and optimizer float bits, accumulated gradients, predictions, and the declared chemical initial conditions. The LIF case includes adaptation, homeostasis, refractory state configuration and a trainable threshold.

Chemical source clock, kinetics counter, controller window, resources, pending feedback, receptor location and readout expression are saved as the initial conditions for independent training episodes. This test does not advance a persistent individual across episode boundaries. The test caller supplies the fixture Step offset. NextSample follows the existing delayed-pulse checkpoint contract and equals Updates; this is not evidence that arbitrary dataset cursors are checkpointed.

With the LIF recompute slot removed by the recorded Go overlay, the continuous control passes and the LIF acceptance case fails with the expected missing implementation error. With the actual implementation, both resume cases pass. The entire checkpoint package reports 135 top-level pass events. These counts include the helper test that returns without a marker during the normal parent run.

All three command receipts fingerprint the same 219 relevant Go/module files. Raw logs and the exact tested resume source are retained. The archived overlay uses original local paths and must be rewritten to a local copy of the archived empty slot when replayed elsewhere.

Full repository acceptance is recorded separately by the parent verification file. These scoped checks make no biological fidelity or task-quality claim.
