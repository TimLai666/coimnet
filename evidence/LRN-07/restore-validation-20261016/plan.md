# Ticket133 replay restore validation

Root decision 2026-10-16. The preceding turn made authoritative progress: ticket132 was validated, cleaned, committed and pushed, with exact remote readback d38ffae4e0590fd307c0f27e897a2003a4afd556. The goal remains active; no claim of absolute zero bugs.

## Contract and ownership

Current source has848 frozen baseline entries. Restore must reject malformed SHA-256 fingerprints and the confirmed impossible draw counters while leaving caller snapshots untouched. Add/Sample sequencing, seed, stored fields, uppercase valid fingerprints and lawful keep/drop histories remain unchanged. Existing candidate regressions are reused from the verified ticket132 archive. A test-only worker writes an additional proposed test text; a separate implementation worker fills the reviewed production slot. Root reviews all source and runs all Go commands.

Necessary draw invariants: keep/reservoir requires Draws>=Seen-Capacity after overflow. In either retention mode, if reservoir overflow could never occur and no historical sample group could contain multiple items, Draws must be zero. min(Seen,Capacity) is a historical upper bound, not an exact drop history. time_balanced cannot draw at held sizes<=2. These are conservative necessary checks, not arbitrary snapshot reachability reconstruction.

## Acceptance

- [x] Root-read, freeze and run public regressions against unchanged production, covering both causes and additional counter failures.
- [x] Adopt the minimal shared Restore validation and verify legal Add/Sample/Restore continuation across policies, refusal ownership, finite rejection and old model controls.
- [x] Complete required engineering checks against frozen final source and update LRN-07 only with direct evidence. LRN-10 remains specified for ticket134.
- [x] Preserve necessary evidence, remove the owned project work root, commit, push and verify exact remote hash.

## Subtraction review

Reuse existing Restore and validateStored. No new format, arbitrary draw cap, alternative sampler or replay policy. Keep large-draw replay cost and counter-wrap concerns distinct from the confirmed invalid-state checks until their contracts and practical evidence are established.

Root confirmed two more same-cause counter violations through ordinary/race public APIs: all-seen-held drop implies no nonempty sampling ever occurred; all-seen-held keep/task-balanced with unique tasks implies no historical task group could draw. These necessary zero-draw guards and their lawful positive neighbors are included before implementation.
