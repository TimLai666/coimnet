# Ticket 106 clean-export VCS metadata review

## Scope

Reviewed the metadata-fix diff only:

- `scripts/clean-env-verify.sh` against `evidence/OPS-03/operational-temp-paths-20261015/clean-env-before-metadata-fix.sh.txt`.
- `docs/INDEX.md` against `INDEX-before-metadata-fix.md.txt`, including the `coimnet-clean-env-run/v1` producer link.
- Frozen private `runs/.tmp/operational-temp-paths-20261015/probe/clean_export_isolation_test.go` and the unchanged original path-test/helper copy.
- `applied-metadata-fix.json`, `clean-export-test-freeze.json`, `clean-export-isolation-plan.md`, `go126-vcs-source-excerpts.txt`, and the recorded red/green probe receipts.

This is narrower than the previous `review.md`: that review covered the five operational path/document diffs, graph cleanup, retained README artifacts, and output/store refusal. This review covers only the later clean-export VCS-stamping correction and rechecks the unchanged default/custom workspace and schema boundaries.

## Confirmed findings

**No confirmed findings.**

The production diff has only two functional additions in `scripts/clean-env-verify.sh`:

- `:256` exports `GOFLAGS="${GOFLAGS:+$GOFLAGS }-buildvcs=false"` after `git archive HEAD` and before every `go` child command.
- `:42-45` updates help text to state that Go VCS stamping is disabled while other `GOFLAGS` are retained.

This handles unset and empty `GOFLAGS` by producing `-buildvcs=false`, and appends rather than replaces existing flags. The frozen private test deliberately supplies `GOFLAGS=-ldflags=-s -buildvcs=true` (`clean_export_isolation_test.go:218-226`), invokes one real dependency-free Go build through the fake `mod download` child (`:21-30`), preserves `-ldflags=-s` (`:295-309`), and rejects every `vcs`/`vcs.*` build setting (`:302-305`). The fixed-source normal and race receipts for the combined original plus metadata test both exit 0: `private-green-v2-local.json` and `private-green-v2-race-local.json`. The pre-fix red receipts record the concrete VCS failure at `clean_export_isolation_test.go:304`: `vcs=git`.

The archive and HEAD contract is unchanged. The script still resolves `repo_root` and `head` (`scripts/clean-env-verify.sh:132-133`), archives `HEAD` into `src` (`:251-253`), and records the same schema fields. The test checks the fixture HEAD in console output (`clean_export_isolation_test.go:239-240`), `run.json` (`:274-289`), tracked files/status, and HEAD after execution (`:326-341`). No numeric Go source, model, protocol, dependency, or formal path-test file changed. `applied-metadata-fix.json` records `original_numeric_Go_sources_unchanged=true`; `clean-export-test-freeze.json` records private test SHA `d6dc26b68e71891c652b349bf6361033c42c470c164a3698a55a3f00b13a3e04` and unchanged original path-test SHA `4019f8afa1a05a34ee1208a41ca01342893a0f7c4735b3b7ac2ad54a7a12443e`.

Default and explicit workdir ownership is unchanged. Default creation remains `runs/.tmp/clean-env.XXXXXX` (`scripts/clean-env-verify.sh:167-174`), and only a default workspace reaches the final `rm -rf` branch (`:421-425`). An explicit `--workdir` remains caller-owned. The unchanged original path test covers explicit-workdir identity/content preservation, while the new metadata test covers default workspace removal and sibling retention. The candidate hunk does not alter cleanup, output paths, or `workdir_removed`.

The schema consumer is corrected exactly. Current `docs/INDEX.md:193` links `coimnet-clean-env-run/v1` to `scripts/clean-env-verify.sh:428`, where the producer writes the schema. The before copy linked to line 424; `applied-metadata-fix.json` records producer line 428 and the expected after hash.

The fixture isolates its intended Git inputs: it removes inherited Git directory/config variables and sets `GIT_CONFIG_NOSYSTEM=1`, `GIT_CONFIG_GLOBAL=/dev/null`, and noninteractive prompts (`clean_export_isolation_test.go:97-120`); the fixture commit uses `core.hooksPath=/dev/null` and fixed commit identity (`:149-155`). The test makes the tracked source dirty after committing (`:157-164`) and still checks unchanged source/status/HEAD after the run. No evidence shows a hook or host configuration affecting the result.

## Needs investigation

- **Combined explicit-workdir plus VCS metadata runtime coverage.** The source proves that the process-local `GOFLAGS` export is before all Go children and does not depend on `workdir_is_default`; the original path test covers explicit `--workdir` with the old fake archive-failure seam. The new metadata test exercises an actual child build only with the default workspace. Missing evidence is one fixed-source fixture run combining explicit `--workdir` with the real minimal build, if that combination is required by the acceptance contract. This is a coverage gap, not a confirmed defect.

- **GOFLAGS variants beyond the tested case.** Source inspection covers unset/empty and append behavior. The frozen test covers an existing linker flag plus `-buildvcs=true`. It does not separately execute unset, explicitly empty, pre-existing `-buildvcs=false`, or several quoting/spacing forms. Missing evidence is a bounded child-env probe for those variants; no source-level failure is established.

- **Host Git template/config isolation.** The test controls global/system Git configuration and the commit hook path, but does not assert a `GIT_TEMPLATE_DIR` or host HOME/XDG fixture. No hook execution or host-dependent failure is present in the recorded logs. Missing evidence would be an explicit host-config/template isolation assertion if that is a release requirement.

- **Full clean-export/OPS-10 coverage.** The new regression intentionally performs one dependency-free native Go build and stubs all heavy framework commands, as stated in `clean-export-isolation-plan.md:7-11`. It does not prove real Go builds for every script step, real data import, network use, or full OPS-10 completion. The earlier `clean-export-vcs-probe-verification.json` field `product_script_runtime_coverage_pending_new_regression=true` is an earlier receipt; later `private-green-v2-local.json` and `private-green-v2-race-local.json` provide the requested product-script regression. Neither should be expanded into a full OPS-10 claim.

## Verification limits

Only read-only Python parsing, hashes, source diff, and recorded JSON/log inspection were used here. I did not run Go, build, test, race, shell scripts, real import/training, download, network, signal, service operation, Git writes, or cleanup. The private green receipts are existing evidence, not executions performed by this review.

## Result

No confirmed P0, P1, or P2 defect. The metadata fix is source-consistent with the stated contract and the frozen red/green test evidence. The remaining items are explicit coverage/acceptance questions only.

Skills: diff-inspector
