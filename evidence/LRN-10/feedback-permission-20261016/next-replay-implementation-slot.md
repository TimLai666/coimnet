# Ticket133 implementation candidate slot

Root read the complete public tests and corrected the two assertion gaps through the test writer: rejected Restore must return nil store, and a valid Sample returning any error must fail its control. Root then compiled the overlay against unchanged production in ordinary and race builds. Each run produced nine failing terminal events from seven malformed-state leaves, plus28 passing valid-state controls. The first uncompiled invocation is separately recorded and excluded from the red proof.

## Allowed output

Write only runs/.tmp/feedback-permission-20261016/future-replay-store.go.txt as a candidate copy of replay/store.go. Do not modify formal source, tests, declarations, docs, module files, evidence or flags. Ticket132's formal848 source remains frozen under full race validation. Root alone runs Go.

## Required behavior

1. In Restore, keep the existing declaration, schema, capacity, seen and retained-count checks. For keep_until_evicted plus reservoir, Seen greater than Capacity requires Draws at least Seen-Capacity. Compare/subtract as uint64 after the greater-than check, without narrowing Seen to int. Refuse the impossible state before rewinding the generator or publishing a store. Do not apply this bound to FIFO or drop_after_sample.
2. In validateStored's hashed-only path, both InputHash and TargetHash must be exactly64 hexadecimal characters. Reuse encoding/hex parsing and retain the existing invalid-fingerprint error style. Valid uppercase hexadecimal remains accepted and preserved. Raw=true array/hash matching remains exactly as before.
3. Do not change Add, Sample, random draw generation/rejection, state formats, raw ownership, retention, metadata, counters or any other validation policy. No arbitrary Draws cap and no RNG replay optimization belongs to this slot.

## Checks owned by Root

All37 candidate terminal events must pass after repair, including24 continuation combinations and the naturally empty drop/reservoir counterexample. Existing replay tests also pass. Root reviews the complete old/new diff, source/test fingerprints and ordinary/race overlay results. Implementation writer must not edit tests or run Go, and must stop if it believes the test contract is wrong.

Subtraction review: two narrow validation checks in existing paths, one standard-library import, no new public type/helper or alternative store implementation.
