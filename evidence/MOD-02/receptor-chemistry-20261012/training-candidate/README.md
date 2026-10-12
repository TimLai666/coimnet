# Chemical training candidate checks

These are historical root-executed checks from ticket 65 implementation. Their original receipts and source hashes remain unchanged. See `../candidate-source-audit.json` for current-source differences and the parent `verification.json` for final acceptance.

- Coupled dynamics, stateful chemistry and continuous/LIF chemical recompute: 64 top-level tests passed. The tests compare outputs, gradients, parameter updates, accumulation and optimizer state with full history.
- New LIF trainer checks fail on the empty recompute implementation; the fixture control passes.
- Checkpoint validation rejects alias, null, and Unicode errors after a failing regression run.
- Episode chemistry in a model package previously saved successfully and then failed to load at `config.chemistry.part.resources`. Model packages now reject episode chemistry before publication; episode training checkpoints remain the supported route.
- The checkpoint package passes 133 top-level tests after that boundary fix, including the parent test that launches a fresh process. Counts are raw top-level Go test passes, not a count of independent biological demonstrations.

`verification.json` names the commands, logs and immutable receipts. Receipts retain the original local command paths and source fingerprints. No full-suite, quantitative biology or OCR-quality completion is claimed here.

Physiological scope: receptor regions select local concentrations. Anatomical compartment labels do not create separate membrane voltages. Chemical release is currently broadcast to every region. Effects alter declared gain/offset/threshold and may gate local learning. General training propagates chemical feedback to prior neural activity while controller and receptor coefficients remain fixed. The ecdysone/NPF examples are engineering hypotheses, not quantitative hormone models.

The early training and checkpoint candidates differ from the current source in more than the two final allocation test files. Their results are implementation history, not current-source acceptance.
