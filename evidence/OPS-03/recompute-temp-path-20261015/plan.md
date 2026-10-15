# Recompute temporary-path correction

The current script creates a private run under the unignored repository-root .recompute-work directory. The resource-document example calls system mktemp without a project prefix. Both conflict with the existing runs/.tmp policy. Do not change the RSS schema, numerical helper, node/step defaults, output overwrite contract or historical evidence.

## Scope and ownership

Root owns scripts/recompute-evidence.sh, docs/resources.md, ticket/status/follow-up updates and final evidence. The test executor owns only new recompute_temp_paths_test.go. Existing 812 Go/module input files and frozen LIF driver/binaries remain unchanged. A new _test.go file is not an input to the already-built LIF programs; record the additive test separately. Original source hashes and model hashes must still match before and after this independent task.

## Test contract written before implementation

Use the repository's existing Go test framework and a copied shell script in an owned temporary fixture repository. A fixture-only fake go command writes an executable which emits the minimal four valid core/mode JSON records, and supports forced build/helper failures or a controlled build wait. It must never invoke real Go or a real training job. Native /usr/bin/time remains in the shell flow. No external network, new dependency or global environment change.

TestRecomputeEvidenceTempPaths(t *testing.T) has six cases:
1. Success with an empty preexisting runs/.tmp retains that shared directory and cleans only its unique work child.
2. Success in a repository path containing spaces preserves sibling files/directories in runs/.tmp and a legacy .recompute-work sentinel.
3. Build failure preserves a preexisting output file and siblings; own work child is removed.
4. Helper failure preserves that output and siblings; own work child is removed.
5. TERM during a controlled fake build exits 143 and removes only owned work; release the fixture child before waiting, and never signal unrelated processes.
6. --help describes runs/.tmp and has no stale .recompute-work description.

For all execution cases capture the binary path from fake go and require it below fixtureRoot/runs/.tmp with basename prefix recompute-rss. Failed old-path checks must remain real failures. Do not depend on fake RSS values as numerical performance evidence.

TestResourceMeasurementExampleTempPath(t *testing.T) extracts the setup of the existing resource measurement code block, executes that setup in an owned fixture repository, and verifies the returned run_dir is newly created below fixtureRoot/runs/.tmp. Do not execute the document's actual coimnet training command, and do not delete its result/checkpoint data automatically.

On unsupported non-Darwin/non-Linux platforms skip only these Unix-script tests. Avoid races on live command output, shared environment mutation, unbounded waits or leaking controlled fake build processes. Existing Go files are not editable by the test executor. Root reviews full source, runs the targeted red check, makes the shell/document correction and reruns the same tests unchanged.

## Verification and delivery

Use project-local temp/cache environment. Run gofmt for the new test, bash -n, targeted normal/race tests, the actual --scale small four-process RSS flow with a new output path, then appropriate repository build/unit/race/vet/dependency/governance checks. Inspect format-index source-line consistency. Preserve source/evidence and all protected models/directories, clean only this task's work after its handles finish, stage only this completed independent fix and verify the remote commit.

The active LIF study must not be restarted or modified. Changes to tracked existing Go sources would violate its input freeze and are outside this task.

## Subtraction review

Reuse the shared runs/.tmp directory and the existing Go test runner. Remove the obsolete dedicated work-root lifecycle rather than adding another directory manager. Keep the established RSS report and numerical helper unchanged. Preserve document results until the user has saved the necessary artifacts.

## Flow, errors and boundary coverage

Caller arguments -> repository/output resolution -> shared runs/.tmp -> unique owned child -> compile -> four timed helper processes -> existing RSS document -> owned-child cleanup. The document example creates a fresh workspace -> records checkpoint/result/time -> retains those files until verified and saved by the user.

| Operation or boundary | Current behavior retained or correction | Owner / check |
| --- | --- | --- |
| Missing/unknown option or unsupported scale | Existing exit 2 before workspace creation | 105; parser unchanged, help regression |
| Empty --out/default output | Existing repository-relative default retained; success may replace its declared output | 105; empty-shared-root success uses default output |
| Output-directory or workspace creation failure | Existing nonzero exit; no deletion of a foreign/shared directory | 105; source review plus owned-prefix cleanup guard |
| Build failure | Nonzero build status; previous output remains and owned work is cleaned | 105; build-failure fixture |
| Helper failure or malformed helper output | Existing nonzero status and captured stdout/stderr; previous output remains | 105; helper-failure fixture and source review |
| TERM while child build is active | Existing exit 143, release fixture child and wait; clean only owned work | 105; controlled termination fixture |
| Empty shared runs/.tmp | Keep shared directory after own work removal | 105; default-output success fixture |
| Spaces and sibling/legacy contents | Quoted paths and existing contents preserved | 105; sibling/space success fixture |
| Final result write failure | Existing replacement/error semantics retained; no new atomic-publication claim | 105; source review, outside the temporary-path behavior change |
| No full model/data input | RSS --scale small is an artificial SDK helper smoke only | 105; actual small invocation, no full-scale benchmark |
| Necessary document checkpoint/results | Retained until verified and saved; no automatic removal | 105; document setup execution and review |

No database migration, dependency change, model-format migration or external service is involved. Additive Go integration tests reuse the existing command boundary with fixture-only tool substitution. They are not imported by the frozen LIF programs. The numerical RSS helper and its schema producer remain unchanged.

All rows are owned by ticket 105; none create an additional requirement on ticket 104. Before delivery, follow the existing repository's full checks and commit procedure, and keep the still-running 104 work out of the independent commit.
