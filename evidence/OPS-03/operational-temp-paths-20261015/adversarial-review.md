## Diff Inspector

Scope: **CLEAN within the requested scope**. The five before/after comparisons contain only the operational temporary-path changes, the three retained-artifact README setup changes, and the one documentation line correction described by `evidence/OPS-03/operational-temp-paths-20261015/plan.md`. The private test is the frozen additive probe; formal integration remains pending ticket 105.

Reviewed:

- `scripts/clean-env-verify.sh` against `clean-env-verify.sh.before.txt`.
- `scripts/graph-evidence.sh` against `graph-evidence.sh.before.txt`.
- `README.md`, `examples/realsubgraph/README.md`, and `docs/INDEX.md` against their recorded before files.
- `plan.md`, `baseline.json`, `applied-fix.json`, `test-freeze.json`, and `operational_temp_paths_test.go.txt`.
- Fixed-source hashes at review time matched `applied-fix.json`: `clean-env-verify.sh` `d75d0ff0dd1a1ae07f8376806c8e24aa3cdc0f8a7101c3477cde99327d5f7508`, `graph-evidence.sh` `fbfe52d3e873a2581bbd391eccdebf32a377b48711a318decbc792ce3d922255`, `README.md` `52c4b91171036e1ce8838306d4a63efe95dd0b58bb1030b9415b30e18e3b49f3`, `examples/realsubgraph/README.md` `6a8ac149c62ce3a749a38ed88660131e090b181516d30c42c2d2aa353222d474`, and `docs/INDEX.md` `5fd50e239760f65d79d6f445c666332a9c1f1a82ab045d5eb0b11605900ec4c2`. The private test hash matched `4019f8afa1a05a34ee1208a41ca01342893a0f7c4735b3b7ac2ad54a7a12443e`.

Adversarial review: **RUN** — Trigger: explicit request to check consumer paths, cleanup and retention, custom workdirs, output/store refusal, documentation setup, and fixture isolation.

### Findings

No confirmed findings.

The clean-environment consumer receives the new default workspace through `workdir` and `src` (`scripts/clean-env-verify.sh:166-174`), while explicit `--workdir` bypasses `mktemp` and remains caller-owned. The only source hunk changes default creation from bare `mktemp -d` to the absolute repository-root `runs/.tmp/clean-env.XXXXXX` template; final removal remains the existing exact `rm -rf "$workdir"` path (`:417-422`). The private probe asserts default placement and retention on the pre-step archive failure, explicit-workdir identity/content preservation, output preservation, and no fake Go invocation (`operational_temp_paths_test.go.txt:377-426`).

The graph consumer passes the new workspace directly to the import command as `--temp-dir "$tmp"` (`scripts/graph-evidence.sh:35-45`). Its pre-existing empty-directory `rmdir`, nonempty inspection message, import status recording, output/store refusal, and validate path are unchanged (`:18-24`, `:46-75`). The private probe covers successful empty cleanup, failed empty cleanup, failed nonempty retention with the sentinel and inspection message, sibling preservation, and existing output/store refusal (`operational_temp_paths_test.go.txt:428-550`).

The retained tutorial paths are distinct from both cleanup prefixes: delayed training uses `runs/delayed-example.XXXXXX` (`README.md:116-125`), downloads use `data/downloads/malecns.XXXXXX` (`README.md:161-171`), and the real-subgraph example uses `runs/realsubgraph.XXXXXX` (`examples/realsubgraph/README.md:9-24`). Neither operational script removes these names. The private probe executes only the `mkdir` and assignment setup, with fake `mktemp`, and verifies each direct child without invoking Go, data import, training, or download (`operational_temp_paths_test.go.txt:301-375`, `:552-610`).

The schema source link correction is exact: the producer is `scripts/clean-env-verify.sh:424`, where the `coimnet-clean-env-run/v1` JSON begins (`docs/INDEX.md:193`).

### Needs investigation

- **[NEEDS INVESTIGATION]** `scripts/clean-env-verify.sh:417-422`: this review did not execute a successful clean-env run, so default-workspace removal and the emitted `workdir_removed` value remain runtime-unverified. The source preserves the prior exact cleanup branch, and no changed line establishes a defect. Root’s permitted normal/race receipts are required for closure.
- **[NEEDS INVESTIGATION]** `scripts/graph-evidence.sh:50-75`: this review did not execute import success, import failure, or store validation. The private source probe statically exercises fake CLI statuses and checks the resulting paths, but it is not evidence of a real MaleCNS import or model result. Runtime receipts are still required for the actual command contract.
- **[NEEDS INVESTIGATION]** `README.md:118-171` and `examples/realsubgraph/README.md:11-24`: only setup extraction is covered by the private probe. The actual training, resume, prediction, download, inspect, graph import, and probe commands were intentionally not run, so their real artifact contents and external-data behavior remain unverified.
- **[NEEDS INVESTIGATION]** `operational_temp_paths_test.go.txt:1-610`: the private probe is not formally integrated; `test-freeze.json` explicitly records `formal_test_not_yet_integrated: true`. Missing evidence is the ticket-105 helper delivery followed by the required formal test/build/race checks on that exact source set.

### Scope differences

None found within the reviewed five source files and private probe. The before/after diffs independently show no unrelated source or schema changes. Whole-repository status and formal test integration were not audited because they are outside this review’s permitted mutation and execution scope.

### Compatibility

- Existing `--workdir`, `--data`, `--out`, output/store refusal, import/validate, and cleanup branches retain their pre-fix source behavior.
- The only changed consumers are path creation sites and the corresponding help text or documentation setup.
- The project-root requirement is explicit for graph and README commands; the clean script still resolves its repository root through Git.
- No Go/module, model, protocol, dependency, API, or data-format change is present in the reviewed diff.

### Regression coverage

The private probe has bounded fixture cases for clean default/custom paths, graph success and both error-directory states, help, output/store refusal, and all three retained README setup paths. Fake `git`, `go`, and `mktemp` are injected only through the child environment; the fake CLI uses `{}` as an artificial manifest and does not perform real import, training, download, or network access. The probe source does not signal processes or mutate global environment state. These are source-level coverage claims; this review did not run the probe.

### Problem

Two operational scripts and three user-facing setup examples used bare temporary-directory creation. The correction must route disposable operational work below project `runs/.tmp` while keeping caller-owned workdirs, existing outputs/stores, retained tutorial artifacts, and graph inspection behavior intact.

### Done

Reviewed the complete before/after diff and frozen private probe. No confirmed P0, P1, or P2 defect was found. The fixed source hashes and private-test hash match the recorded acceptance evidence.

### Changed

| File | Change |
| --- | --- |
| `runs/.tmp/operational-temp-paths-20261015/review.md` | Added this read-only adversarial review. |

### Actions

| Action | Target | Result |
| --- | --- | --- |
| Read-only `git diff` | Five requested source files | Only expected operational path, retained-artifact, and source-line changes found. |
| Read-only `bash -n` | `scripts/clean-env-verify.sh`, `scripts/graph-evidence.sh` | Both passed. |
| Read-only hash comparison | Applied source files and private test | All recorded after/private hashes matched. |

### Verified

- Default clean work enters the consumer through `workdir`/`src`; explicit custom workdirs bypass default creation.
- Graph import receives the project-local temp path through `--temp-dir`; empty and nonempty cleanup branches remain unchanged.
- Existing output/store refusal occurs before graph work is created or Go is invoked.
- README and real-subgraph retained artifacts use distinct project-local prefixes and are not targeted by either cleanup path.
- The `docs/INDEX.md` source link points to the current schema producer line 424.
- The private probe’s fake tools prevent real Go, data, network, training, and download work in the tested seams.

### Notes

No Go build, test, race run, real operational script, import, training, download, network, signal, service, or full-data command was executed. The private red/green and race records were not treated as runtime evidence in this review. The formal additive test remains blocked on ticket-105 helper delivery by the recorded freeze contract.

### Next

Root should finish the permitted race run, restore/confirm the fixed hashes after any preimage replay, and integrate the additive test only after ticket 105 delivers its helper as required by `test-freeze.json`.

Skills：diff-inspector
