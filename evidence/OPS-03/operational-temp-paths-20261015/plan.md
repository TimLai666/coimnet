# Operational script temporary-path correction

Confirmed locations: scripts/clean-env-verify.sh default workspace at line 168 and scripts/graph-evidence.sh import temporary directory at line 35 both call mktemp -d without a project template. Existing ticket 53 and AGENTS.md require process workspace below project runs/.tmp. Ticket 105 fixes the recompute consumer and document example; this ticket owns the two other operational scripts.

## Scope, input freeze and ownership

Root owns script changes, copied probe inputs, records, verification and delivery. The test worker may only add runs/.tmp/operational-temp-paths-20261015/probe/operational_temp_paths_test.go. Root has exclusively copied the frozen ticket105 test helpers and the two current scripts to that private probe. Do not change formal Go sources, ticket105 helpers, active LIF sources/binaries/protocol, original models, dependencies or command schemas.

The probe will run explicitly as a private Go package using the existing module and project-local cache/temp environment. It reuses helpers from a byte-identical copy of recompute_temp_paths_test.go; select only TestOperationalScriptsProjectTemp. These private sources are not included in go test ./... and not used by existing binaries. Formal permanent-test integration must wait for ticket105 helper delivery, then run the required project checks on that exact source set.

## Minimal fix skeleton written before implementation

User runs an operational script -> script resolves repository root -> creates shared runs/.tmp if needed -> creates one unique workspace -> performs the unchanged operation -> records existing outputs and follows existing cleanup/preservation behavior.

- clean-env-verify.sh keeps explicit --workdir behavior and final workdir_removed/schema. Only default work creation gains an absolute repoRoot/runs/.tmp/clean-env.XXXXXX template and matching help. Failure before the existing final cleanup keeps the workspace for inspection as today; do not invent another cleanup contract.
- graph-evidence.sh already requires repository-root execution. Create shared runs/.tmp and an absolute $PWD/runs/.tmp/graph-evidence.XXXXXX import workspace. Keep empty-directory rmdir and nonempty-directory inspection behavior, output/store existence refusal, import/store/validate commands, schemas and binary handling.
- No broad deletion, no existing data/model replacement, no package/dependency/API migration. Do not run the full OPS-10 suite or real MaleCNS import merely to test placement.

## Existing command seam and bounded fixtures

Use the established Go stdlib + copied Bash command seam. Each test has a new fixture repository inside t.TempDir, sibling sentinels and private fallback temp inside the test root. Fake PATH tools affect only child env, not global env.

A fixture mktemp wrapper records argument vectors. For bare -d (old bug) it invokes the real mktemp with an explicit private-fallback template, so red never creates a system directory. Explicit templates use the real mktemp unchanged. Require the observed operational workspace to be a direct child of fixtureRoot/runs/.tmp with the precise script-specific prefix; this independent check must fail for old bare-mktemp inputs.

For clean-env use fake git: rev-parse returns fixtureRoot/a fixed head, archive exits 41. This stops before all real Go, training, downloading and archive work. Record the printed workdir, require nonzero exit and preserve a preexisting --out result. A fake go marker must prove no Go invocation occurred.

For graph use fixture fake go that builds a small fake coimnet executable. It supports doctor and data import, records --temp-dir, optionally writes a leftover sentinel and returns an explicit import status. Native /usr/bin/time remains in the script. Provide only an artificial manifest {} for hashing. Never touch real data, model files, existing bin/coimnet or use a real import. These fixtures prove shell flow only.

## Required test contract

TestOperationalScriptsProjectTemp has these external-behavior cases:
1. clean-env default workspace with spaces in repo path and absent shared root: observed workspace directly below runs/.tmp/clean-env.*, initial output unchanged, archive exit surfaced, no Go invoked.
2. clean-env explicit custom --workdir inside fixture: preserved identity/content and printed path; no default workspace created, output unchanged, same early-failure behavior.
3. graph successful empty workspace: direct graph-evidence.* child is removed, shared root/sibling file+directory remain, output temp-dir/result/log artifacts remain.
4. graph import failure with empty workspace: nonzero exit and exit log preserved, own empty workspace removed, sibling contents unchanged.
5. graph import failure with a nonempty workspace: retained below project runs/.tmp with its exact leftover sentinel; inspection message and artifacts preserved, siblings unchanged.
6. Both --help outputs describe runs/.tmp default placement without executing work.
7. Existing graph output/store refusal: no existing user result/store overwritten. Preserve the original refusal/exit contract.

All fixtures must be bounded, avoid goroutine/output races and global environment changes, never signal unrelated processes and keep every test-created directory under its own t.TempDir. Reuse copied helpers rather than duplicate 600 lines. Copy script contents from repositoryRoot(t)/scripts; do not create new runtime seams in production scripts.

## Error, shadow and interaction ownership

| Operation / path | Preserved or corrected behavior | Ticket106 acceptance |
| --- | --- | --- |
| Empty/default workdir | Project-local new unique child | case1 |
| Explicit workdir and spaces | Existing caller directory/content and existing retention contract | case2 |
| Missing/bad option, help | Existing exit/help; help documents new default | source/parser review and case6 |
| Git archive failure | Existing nonzero exit, no Go or overwrite of final output | cases1-2 |
| Graph successful import | Existing output artifacts and own empty workspace removed | case3 |
| Graph upstream import error | Error status/log retained, own empty temp removed | case4 |
| Nonempty graph temp on error | Keep for inspection inside project | case5 |
| Existing output/store | Refuse before overwrite | case7 |
| Directory/build/doctor/hash/time failure | Existing set-e behavior and outputs retained; no broad deletion | source review; no new semantics |
| Simultaneous jobs/sibling work | Unique workspace, shared root and other entries preserved | cases1,3-5 |
| Interruption | Existing signal/inspection behavior unchanged; no new cancellation contract | source review, outside placement change |
| Large/real graph or full clean suite | Not needed for path acceptance, not launched | fixture only |

Each row is owned by106. No database, service, model, API or dependency change is involved. Original Go/module/model/protocol hashes must match before/after; the active104 study and105 engineering pipeline continue without restart.

## Verification, subtraction and delivery

Root reads the complete new test, freezes it, runs old-source local normal/race red with exact test names, makes the minimal script/help correction, refreshes only the two owned copied script inputs after CAS verification, then runs the same frozen tests green. Verify bash -n, real help and source-contract inventory. Integrate the permanent additive test only after105 delivery; run project build/unit/race/vet/module/governance checks and source/model preservation. Save evidence, clean only106-owned work after its handles finish and complete authorized conventional commits/push/readback.

Use the existing shared runs/.tmp and command fixture seam. No directory manager, new framework, full data workload or alternative cache system is needed. Keep data/results and preexisting temp roots.

## Tutorial artifacts retained in the project (scope inventory completed before source edits)

README.md lines119 and164 also use bare mktemp for necessary artifacts. Their intended behavior differs from disposable measurement work: the first block saves training checkpoints and observations; the second downloads an original data file and receipt. Keep them as new unique directories under project runs/delayed-example.XXXXXX and data/downloads/malecns.XXXXXX respectively. Explicitly create parents and state repository-root execution. Do not automatically delete these tutorial results or change their commands, model formats, download URL, limits or resume behavior.

Add cases8-9 to the same private test: extract and execute only mkdir plus run_dir/data_dir assignment setup from the corresponding existing README sh block, use the child mktemp wrapper/private fallback for old bare -d, and require newly created direct children of the correct retained-artifact parents. Do not execute training, data sources or download in this path test. Root will separately validate the local train/resume/predict block without downloading data.

The new source inventory is two operational scripts plus the two README setup locations. Path creation errors preserve existing behavior. All tutorial path/default/space/first-use acceptance belongs to106. Preserve results until separately saved and verified; no production data or original model is removed. This inventory does not change104 or105 milestones or frozen source/test inputs.

## Retained real-subgraph example artifacts

The complete nonhistorical source inventory also found examples/realsubgraph/README.md line13 using bare mktemp. This directory holds a graph store and import/probe reports, so keep it as a unique project runs/realsubgraph.XXXXXX directory with its parent created explicitly. Do not alter import selection, limits, source fingerprints, updates, or real-subgraph profile, and do not rerun the real import for this path correction. Add case10 that executes only mkdir/run_dir setup with the same private mktemp seam and checks a retained direct runs/realsubgraph.* child. Historical /private/tmp references in docs/insyra-integration.md and docs/malecns-source-audit.md are evidence history and remain unchanged.
