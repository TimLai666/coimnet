## Diff Inspector

Scope: **CLEAN within the requested scope**. The two tracked-file diffs contain only the temporary-root and documentation-example changes described by `evidence/OPS-03/recompute-temp-path-20261015/plan.md`; `recompute_temp_paths_test.go` is the additive test covered by `test-freeze.json`. No RSS schema, numerical helper, node/step defaults, output path resolution, or final JSON publication code was changed.

Reviewed:

- `scripts/recompute-evidence.sh` against `evidence/OPS-03/recompute-temp-path-20261015/recompute-evidence.sh.before.txt`.
- `docs/resources.md` against `evidence/OPS-03/recompute-temp-path-20261015/resources.md.before.txt`.
- `recompute_temp_paths_test.go`, the plan, and `test-freeze.json`.
- The fixed-source hashes at review time: script `3a380136cd5071c42db111c1700a9241c9c2832cfe3ef35883391ef1f26547c5`, documentation `0fc8b793f4923ef2137fb4037a7583e588d63735e8453aad5a2e7f8e438d0b52`, and test `e56e559cf1ef1f0e4a665be32103ca0e9447e1c97d87d574c62afd078e8e02ca`.

Adversarial review: **RUN** — Trigger: explicit request to check cleanup, failure, TERM, overwrite, first-use documentation, and test synchronization.

### Findings

No confirmed findings.

The script now creates one owned child under `runs/.tmp` (`scripts/recompute-evidence.sh:99-110`) and removes it only when the recorded path matches the owned `recompute-rss.*` prefix (`:101-104`). It no longer removes the shared root or unrelated entries. The before/after comparison shows no change to argument parsing, `/usr/bin/time`, helper invocation, RSS parsing, JSON assembly, or the declared output replacement behavior (`:31-87`, `:113-169`).

The source test covers the planned success, build-failure, helper-failure, TERM, sibling/legacy-preservation, and help cases (`recompute_temp_paths_test.go:386-540`). Failure cases pre-create an output and assert its bytes remain unchanged (`:444-464`); the TERM case also releases the controlled fake build before waiting and asserts exit 143, child completion, output preservation, and sibling preservation (`:467-525`). The signal test starts the command before launching the `Wait` goroutine and signals the returned `cmd.Process` (`:211-230`, `:491-510`), so the previously identified Start/Wait ordering defect is absent from this source.

The documentation change requires the command to be run from the repository root and explicitly creates `runs/.tmp` before using an absolute `$PWD/runs/.tmp/resource-profile.XXXXXX` template (`docs/resources.md:259-268`). The test extracts and executes only that setup, checks the new directory is below the fixture `runs/.tmp`, and does not run the training command (`recompute_temp_paths_test.go:543-632`).

### Needs investigation

- **[NEEDS INVESTIGATION]** `scripts/recompute-evidence.sh:101-111`, `recompute_temp_paths_test.go:467-525`: this review did not execute the real script or the normal/race test, by the explicit restriction. The source-level TERM test exercises a controlled child and waits for its completion, but actual process-group behavior and residual-process cleanup need the root-owned targeted normal/race receipts. This is an unverified boundary, not a confirmed bug.
- **[NEEDS INVESTIGATION]** `docs/resources.md:261-268`: the full `train delayed` measurement command was not run here. Static setup extraction is covered by the new test, while first-run behavior of the actual built binary, `/usr/bin/time`, and checkpoint/result writes remains for the root-owned check. The known sandbox `/usr/bin/time` `kern.clockrate` limitation is an environment limitation, not evidence of a framework defect.
- **[NEEDS INVESTIGATION]** `scripts/recompute-evidence.sh:158-166`: the final-output write-failure path was not executed here. The diff leaves the prior direct-write and replacement contract unchanged, and the source test covers preservation on build/helper failure; no new failure is established.

### Scope differences

None found in the reviewed files. The evidence record states zero existing Go files were modified, and the before/after diffs independently confirm that the tracked code change is limited to the two requested files. Whole-repository status was not audited because the review was restricted to the specified paths.

### Problem

The recompute helper and the resource example previously created temporary directories outside the project `runs/.tmp` policy. The correction must preserve shared work, old results, and the existing RSS computation contract while cleaning only the current run.

### Done

Reviewed the fixed script/documentation diff and the additive fixture test for path ownership, cleanup, failures, TERM synchronization, output preservation, and documentation setup. No confirmed defect was found.

### Changed

| File | Change |
| --- | --- |
| `runs/.tmp/recompute-temp-path-20261015/review.md` | Added this read-only adversarial review. |

### Actions

| Action | Target | Result |
| --- | --- | --- |
| Read-only `git diff` | `scripts/recompute-evidence.sh`, `docs/resources.md` | Passed; only the expected path/documentation hunks are present. |
| Read-only syntax check | `scripts/recompute-evidence.sh` | `bash -n` passed. |
| Read-only source/hash checks | Fixed script, documentation, test, and before files | Fixed hashes matched the recorded values at review time. |

### Verified

- The script uses `runs/.tmp/recompute-rss.XXXXXX`, preserves the shared root, and guards cleanup to its own child.
- Build/helper failures preserve a pre-existing output in the test source; successful runs retain the declared overwrite behavior.
- The TERM fixture releases its child before waiting and checks exit 143 and cleanup-related invariants.
- The documentation setup creates a project-local unique directory and does not execute the training command in the test.
- No existing RSS computation, schema, core/mode loop, scale defaults, or output resolution hunk changed.

### Notes

No Go build, test, race run, RSS measurement, training, download, signal operation, or service operation was performed, as required. The report therefore does not claim runtime or race completion. The root-owned preimage replay must be restored to the fixed hashes before final acceptance.

### Next

Root should restore the fixed script/documentation versions and use the permitted targeted normal/race receipts to close the two runtime verification items above.

Skills：diff-inspector
