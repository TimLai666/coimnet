# Ticket133 public regression slot

Root decision 2026-10-16. Ticket132's 848 formal source files are frozen while its full engineering gate runs. This slot writes only one proposed text file in the owned work root; it must not add a formal Go file yet.

Public signatures: replay.New(Buffer) (*Store, error), (*Store).Add(Experience) error, (*Store).Sample(int) ([]Experience, SampleReport, error), (*Store).Snapshot() Snapshot, replay.Restore(Buffer, State) (*Store, error), replay.RestoreSnapshot(Snapshot) (*Store, error).

Root read store.go, policy.go and sampling.go. Restore checks only hash lengths in the hashed-only path. It also checks retained item count, but accepts keep/reservoir states with fewer draws than any valid Add sequence could produce. Add uses one or more random raw draws for every item added beyond capacity; Sample and rejection also consume raw draws. Drop retention can free capacity and therefore cannot use the keep/reservoir lower bound.

## Expected results

1. A hashed-only snapshot with one 64-byte non-hex input or target fingerprint must return nil store and error. Use actual Add snapshots as the base, modify one hash at a time, include invalid first and last characters. Do not confuse a 64-character SHA-256 digest with valid hexadecimal text.
2. Keep/reservoir capacity2 states with Seen3/Items2/Draws0 and Seen6/Items2/Draws3 must reject. Construct the items through valid Add calls, then corrupt only the counters needed for the case. A SeenMaxUint64/Items2/Draws0 control must reject without narrowing to int. Refused Restore must leave its input Snapshot unchanged.
3. Valid Add/Sample/Restore continuation must retain exact samples, reports, state and policies. Cover the three sampling modes, two eviction modes, both retention modes and raw/hashed storage with small fixtures; helpers may keep the file compact. Include initial empty and a naturally emptied drop snapshot where useful.
4. A drop/reservoir capacity2 store built by three Add-one/Sample-one cycles has Seen3, no held items and Draws0. That is valid and must restore and continue; it rejects an over-broad draw lower bound. A valid hashed-only uppercase hex digest accepted by the previous code must remain accepted and preserved.

No tests that invoke Restore with enormous valid Draws and hang a process. Large-draw complexity and draw-counter wrap are separate investigation items. No fairness, duplicate-content ID, schema, PRNG or Add/Sample algorithm change. Existing replay tests stay untouched.

Test writer uses external package replay_test and writes only runs/.tmp/feedback-permission-20261016/future-replay-tests.go.txt. Root alone reviews and executes tests via an overlay; a different worker later writes the implementation candidate. Invalid cases must fail on original production; valid continuation controls should pass before and after repair. Do not mirror the proposed validation expression in a test oracle.
