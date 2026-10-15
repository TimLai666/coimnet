# Ticket 105 final engineering acceptance review

## Scope

Read-only review of the final E105 engineering receipts, normal/race JSONL terminal events, source manifests, module/gofmt/governance receipts, and the three new document-checkpoint/shared-document receipts. No Go, product script, training, network, Git write, or cleanup command was executed by this review.

## 已完成證據

### 工程檢查

`evidence/OPS-03/recompute-temp-path-20261015/engineering-pipeline.json` records all eight required commands with `exit_code: 0` and `all_required_commands_completed: true`, `all_passed: true`:

- `full-build`
- `full-unit-local`
- `full-race-local`
- `full-vet`
- `module-verify`
- `module-selection`
- `go-format`
- `final-governance`

The independent receipts agree: `full-race-local.json`, `full-vet.json`, `module-verify.json`, `module-selection.json`, `go-format.json`, and `final-governance.json` all have `exit_code: 0`. `full-race-local.json` reports `3531.079510666` seconds.

The final governance JSON selects `TestGovernance|TestFormatIndex|TestTargetFlow` and exits 0. Parsing `final-governance.log` gives 21 test terminal passes plus one package pass and no fail/skip terminal event. The diagnostic `no Root 決策 section` lines are output from a passing governance test, not failed events.

### Normal/race JSONL counts

Independent parsing of the final logs gives:

| Log | Package terminal events | Test pass | Test skip | Test fail | Log SHA-256 |
| --- | ---: | ---: | ---: | ---: | --- |
| `full-unit-local.log` | 58 pass, 3 skip | 5,826 | 22 | 0 | `69822d6777fab582a55ddfc559f81f3aed786d76bcea2a9f2b0959716b8d98b2` |
| `full-race-local.log` | 58 pass, 3 skip | 5,825 | 23 | 0 | `8252aca226fbcdf4da9e3c48d9106af57d373a9c2a2c97367b9f6b01d7135f99` |

The one normal/race test-count difference is expected and source-proven: `checkpoint/bundle_kinds_test.go:274-276` skips `TestBundleLargerThanSingleFileLimit` when `bundleRaceEnabled` because race instrumentation makes the large fixture too memory intensive. This is not a failure or a new E105 defect.

### Frozen source and formal test

`go-source-before.json` has 812 entries; `go-source-final.json` has 813. Set comparison shows no missing or changed pre-existing entry and exactly one added entry, `recompute_temp_paths_test.go`, with SHA-256 `e56e559cf1ef1f0e4a665be32103ca0e9447e1c97d87d574c62afd078e8e02ca`. This matches `test-freeze.json` and the engineering-pipeline fingerprint. The protected-input receipts still report 812 original Go sources, 18 models, 9 frozen drivers, 2 binaries, and unchanged protocol.

`go.mod:6` fixes `github.com/HazelnutParadise/insyra v0.3.4`; `module-selection.log` contains the same version and `module-verify.json` exits 0. `go-format.json` runs `gofmt -l recompute_temp_paths_test.go` and exits 0.

### 教學快照缺口已關閉

The earlier checkpoint-byte gap is now covered by actual saved content:

- `document-example-checkpoint.json` is a 1,177-byte `coimnet-episode-checkpoint/v1` JSON file with `payload.next_sample=600`, `training.updates=600`, and file SHA-256 `559dbe3b08b944d0ef34756489c48dbf4c4818d5e8d682463568ff30a7a691f3`.
- `document-checkpoint-preservation.json` reports the same SHA matches all three original document checkpoint hashes and `command_receipt=document-checkpoint-reproduction.json`.
- `document-checkpoint-reproduction.json` reports the deterministic 600-step command exited 0.
- `document-example-verification.json` records all three document runs creating the same checkpoint bytes plus result/time logs, each with exit 0.

This is sufficient to close the previous “only checkpoint hash, no checkpoint bytes” evidence gap. The receipt explicitly says the checkpoint was reproduced deterministically rather than copied from the original fixture; the byte hash equality is the available proof.

`concurrent-document-ownership.json` correctly records that AGENTS, delivery-status, and docs/INDEX changes are Root-owned shared-document changes and are outside the 812 frozen numerical/test inputs.

## 確認問題

### P2 — Ticket/status evidence still reports the pre-final engineering state

- **Files/lines:** `docs/tickets/105-recompute-project-temp.md:6,10,25`; `delivery-status.md:7`.
- **Evidence:** the ticket still says `Status: in progress`, leaves the engineering-check checkbox unchecked, and says “目前 2／4” with race and subsequent engineering checks pending. `delivery-status.md:7` likewise says race and later checks are still running. This contradicts `engineering-pipeline.json:all_required_commands_completed=true, all_passed=true` and the final full-race/vet/module/gofmt/governance exit-0 receipts.
- **Impact:** current acceptance/status documents under-report the completed engineering milestone and can cause a reviewer to treat the final race as pending.
- **Minimum correction:** after the owned-temp cleanup decision, check the engineering milestone and update the progress text to reflect 3/4 while leaving evidence/cleanup/commit delivery pending until those steps actually finish. This is a delivery-document defect, not a runtime/code defect.

### P2 — Shared-document ownership receipt has stale final hashes

- **File/fields:** `evidence/OPS-03/recompute-temp-path-20261015/concurrent-document-ownership.json:documents.*.observed_sha256` and `final_hashes_must_be_checked_after_status_updates=true`.
- **Evidence:** current read-only hashes are:
  - `AGENTS.md`: observed `18788eefd011d0292aed00740dc27dc65088fdd448bfa1574f11095dd9b37728`, current identical.
  - `delivery-status.md`: observed `850b86ac15b73ddffddb22c2f0ad194442d9d82668bd1b822ab623d58fb40f12`, current `f6b57e8b61d8ea1e3ed2317e941f3ba8f816dff89178c2513a58754df91195a5`.
  - `docs/INDEX.md`: observed `5fd50e239760f65d79d6f445c666332a9c1f1a82ab045d5eb0b11605900ec4c2`, current `e042c3759b57f61e8cc73c3fc40c151e4f9266f0de585a9d4c70069e0df0f42f`.
- **Impact:** the shared-document receipt does not fingerprint the files that would be staged now. The differences are plausibly the later final status update and the metadata producer-line correction, but the receipt itself is stale.
- **Minimum correction:** refresh the shared-document ownership/final hash receipt after the last status and INDEX edits, then verify the scoped commit against those final hashes.

### P2 — Earlier aggregate unit receipt contains a stale race-pending flag

- **File/field:** `evidence/OPS-03/recompute-temp-path-20261015/full-unit-verification.json:full_race_pipeline_still_running`.
- **Evidence:** the field remains `true`, while the later `full-race-local.json:exit_code` is 0 and `engineering-pipeline.json` records `full-race-local` completed and passed.
- **Impact:** consuming this older aggregate as final evidence gives the wrong current race status.
- **Minimum correction:** mark this receipt historical/superseded or publish a final aggregate that includes the race exit code, parsed event counts, and final log SHA. Do not alter its historical provenance silently.

## 待補項

1. Clean only the owned `runs/.tmp/recompute-temp-path-20261015` work after confirming no active process uses it, while preserving the allowed final review artifact and all evidence required by the delivery policy. No final cleanup receipt is present yet.
2. Refresh the shared-document hashes after the current `delivery-status.md` and `docs/INDEX.md` edits.
3. Update ticket/status text and checkboxes to separate completed engineering checks from still-pending cleanup and delivery.
4. Produce the scoped commit and remote readback. No E105 commit or remote receipt is present in the reviewed evidence directory.
5. If the final preservation policy requires a post-race input receipt, rerun the read-only protected-input manifest after the last source/status changes. Existing phase-7 evidence proves the 812/18/9/2 protected set before the final engineering run; `go-source-final.json` independently proves the 812+1 Go-source set.

Not required for ticket 105: a complete OPS-10 real-data/long-running workload. The plan and evidence identify the RSS/doc fixture as path-flow validation rather than a full model or performance claim.

## 最短交付順序

1. Finish/confirm only E105-owned temp cleanup and preserve final evidence.
2. Refresh final governance/status/shared-document fingerprints and supersede stale aggregate receipts.
3. Update ticket 105 and delivery status to 3/4 with cleanup/commit still pending, then close line 11 only after actual commit and remote readback.
4. Stage only ticket 105 files and verify the scoped commit contains no ticket 104/106 work.

## Result

The engineering acceptance is proven, normal/race count difference is an intentional documented race skip, the frozen-source count is exactly 812 unchanged plus one formal test, Insyra remains pinned at v0.3.4, gofmt and governance pass, and the checkpoint preservation gap is closed. The confirmed issues are stale acceptance/status metadata and stale shared-document/aggregate receipts; no runtime defect was found.

Skills: diff-inspector
