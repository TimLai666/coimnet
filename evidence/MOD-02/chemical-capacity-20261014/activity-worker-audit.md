# Chemical entry audit

## Problem

查驗公開 `NewIndividual -> EnableChemistry -> Advance` 在第一列使用
`SourceNeuralActivity` 或 `SourceController` 時的 nil prehistory 契約，並對照 later-enable、
`ResetNeural`、fresh/warm snapshot restore、`ChemicalEpisode` 與 `TrainEpisode` 的生命週期。

## Confirmed findings

1. **Fresh persistent neural source 的第一列拒絕是既定契約，不是已確認 bug。**
   - 公開觸發：`NewIndividual`、`EnableChemistry(neural)`、第一次 `Advance`。
   - 實際錯誤：`individual chemistry: source 0 at step 0: modulation: neural source has no activity for step 0`。
   - `learning/chemistry.go:161-164` 將 fresh row 的 `activity` 定義為 nil；
     `learning/chemistry.go:474-509` 在 `Steps=0` 明確回 nil；
     `learning/individual.go:678-701` 將它交給來源並在錯誤時不提交候選狀態。
   - `modulation/sources.go:129-151` 的公開 `NeuralActivity.Release` 明確拒絕 nil。
   - `learning/chemistry_test.go:579-611` 已把這個拒絕寫成測試契約，並驗證 `Steps` 不增加。
   - 小圖結果：before/after snapshot digest 都是
     `bdbd85fe6ec472a629e8831652e2755a011f92e7a234e75814e28799c95f0fa4`，
     chemical/neural steps 仍為 0，`ChemistryReport` 不變。

2. **Fresh persistent controller source 接受同一個 nil boundary，因其契約把 nil 當全零列。**
   - `modulation/controller.go:196-224` 明確將 nil activity 觀察成 node-count zeros。
   - 第一列成功提交 neural/chemical steps=1，release=`0.6931471805599453`，concentration 每區為
     `0.5454643277139397`。
   - 這排除了「所有 EnableChemistry 第一列都會被 nil 卡住」的說法。

3. **先暖機再 Enable neural source 成功，且讀的是 raw neural history，不是 float32 readout。**
   - 公開流程先執行一列無化學 `Advance`，再 `EnableChemistry(neural)`，下一列成功。
   - warm readout 是 `0.5595106482505798`，持續神經 history 的上一列是
     `0.5595106570525964`；化學 `ReleaseTotal` 精確為後者，concentration 為
     `0.44030057822847224`。
   - 這符合 `learning/chemistry.go:474-509` 與 `learning/chemistry_test.go:594-610` 的 previous-output 契約。

4. **`ResetNeural` 會重新建立沒有 observed neural history 的來源邊界，同時保留化學狀態。**
   - 暖機、啟用 neural source 並成功推進後呼叫 `ResetNeural(..., []float64{0,0})`。
   - reset 後 neural `Steps=0`、chemical `Steps=1`，chemical snapshot digest 與 report 保持不變。
   - reset 後第一列再次得到同一 nil-activity refusal，且 snapshot/report 沒有變更。
   - 這與 `learning/individual.go:862-880` 及 `ENG.md:227` 的「重設神經與來源時鐘，保留化學濃度及化學更新步數」一致。

5. **Fresh snapshot restore 保留 refusal；warm snapshot restore 保留成功接續。**
   - fresh enabled neural snapshot 經 JSON round trip、`RestoreIndividual` 後，第一列同樣拒絕且 digest 不變。
   - warm enabled neural snapshot restore 後，下一列輸出及完整 snapshot 與不中斷原個體相同。
   - `learning/individual.go:394-426` 保存 chemical/source state；`learning/chemistry_test.go:663-716` 已有 warm neural mid-run continuation regression。

6. **普通 `ChemicalEpisode` 的 nil `InitialActivity` 不會把 nil 傳給 `NeuralActivity.Release`。**
   - `learning/chemical_episode.go:14-22` 定義 nil `InitialActivity` 使用 fresh core 的 initial output。
   - `dynamics/coupled_modulation.go:146-173` 先建立 activated initial output，再以 node-width vector 呼叫 sequence。
   - `learning/chemical_sequence.go:131-167` 只在顯式提供時替換第一列 boundary，後續一律接收上一列 raw output。
   - 小圖 `PredictAll`：neural nil 與顯式 zero output 都是 `[0.5595106482505798]`；顯式 active `[0.75,0]` 變為 `[0.7506017088890076]`；controller nil 也成功。這是獨立 episode 契約，不應拿 persistent `Advance` 的 nil refusal 類比。

7. **一般 `NeuralActivity` nil 輸入檢查保持不變。**
   - 直接呼叫公開 `NeuralActivity.Release(0, SourceContext{})` 仍得到
     `modulation: neural source has no activity for step 0`；顯式 zero activity 才成功。
   - 沒有弱化或繞過來源層的 nil 檢查。

## Controls

- `fresh-neural-source-refuses-without-prehistory`：拒絕及 transaction no-op。
- `fresh-controller-source-accepts-zero-prehistory`：同一 fresh 邊界的 controller 對照。
- `later-enable-neural-source-reads-previous-output`：暖機後啟用及 raw history release 對照。
- `reset-neural-recreates-no-prehistory-boundary`：reset 後拒絕及化學狀態保留。
- `fresh-neural-snapshot-restore-preserves-refusal`：fresh restore 對照。
- `warm-neural-snapshot-restore-preserves-success`：warm restore continuation 對照。
- `ordinary-chemical-episode-uses-explicit-initial-activity-contract`：普通 episode nil/zero/active 初始活動對照。
- `neural-activity-general-nil-check-remains`：來源層一般 nil 檢查對照。

8/8 小圖 controls 通過。Root 已另行完成既有基準專項 45 頂層／124 pass events，包含
`TestNeuralSourceReadsThePreviousRowOutputs`；本 activity 沒有擴大執行完整測試。

## Needs investigation

- 沒有證據支持修改實作或放寬 `NeuralActivity` nil 檢查。若要改善 API 使用體驗，只能另行決定是否在公開 `EnableChemistry` 文件補充「persistent neural source 必須先有一列 observed output」的操作提示，這是契約呈現問題，不是本次已確認的實作缺陷。
- 正式 regression 已覆蓋 fresh refusal、later-enable、warm neural snapshot continuation；目前沒有單獨的正式測試同時鎖定 `ResetNeural + persistent SourceNeuralActivity` 的 refusal/no-op，fresh neural restore refusal 也由本 activity 補充驗證。這是測試覆蓋缺口，未在本輪修改正式測試。

## Commands and results

Environment for every Go command:

```text
GOPROXY=off
GOSUMDB=off
GOTOOLCHAIN=local
GOMODCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-mod-cache
GOCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-cache
GOTMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-tmp
TMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-tmp
```

The `GOTMPDIR` line above is recorded exactly in `command-env.txt`; the canonical value used by commands is
`/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-tmp`.

| Command | Exit | Evidence |
|---|---:|---|
| `env GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOMODCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-mod-cache GOCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-cache GOTMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-tmp TMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-tmp go version` | 0 | `go-version.txt`: `go version go1.26.5 darwin/arm64` |
| `gofmt -w runs/.tmp/chemical-entry-audit-20261014/activity/main.go` | 0 | no output |
| `env GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOMODCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-mod-cache GOCACHE=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-cache GOTMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-tmp TMPDIR=/Users/timlai/Developer/coimnet/runs/.tmp/chemical-entry-audit-20261014/go-tmp go run ./runs/.tmp/chemical-entry-audit-20261014/activity/` | 0 | complete JSON: `run-stdout.json`; stderr empty: `run-stderr.log` |
| `python3 runs/.tmp/chemical-entry-audit-20261014/activity/fingerprint.py` | 0 | `fingerprint-result.txt`: `source_count=788 mismatch_count=0` |

Pre/post source manifests are `source-fingerprint-before-all.txt` and
`source-fingerprint-after-all.txt`. Both use baseline commit
`57cc92d2864bd19f2424a4daa57edad60ad2763d`; all 788 baseline source hashes match after the run.

## Changed files

Only the allowed activity directory was touched by this audit:

- `main.go`：public API bounded diagnostic program。
- `fingerprint.py`：baseline/current source hash comparator。
- `command-env.txt`、`go-version.txt`、`go-version.stderr`：Go environment/version record。
- `run-stdout.json`、`run-stderr.log`、`run-stdout-attempt-2.json`、`run-stderr-attempt-2.log`：complete diagnostic output。
- `source-fingerprint-before.txt`、`source-fingerprint-after.txt`、`source-fingerprint-before-all.txt`、`source-fingerprint-after-all.txt`：pre/post source fingerprints。
- `fingerprint-result.txt`、`fingerprint-stderr.log`：fingerprint command output。
- `report.md`：this audit report。

This audit did not modify formal Go source, formal tests, documentation, models, caches, or other temporary paths. No commit, stash, reset, remote call, or app cross-thread message was used.

## Unverified scope

- No full `go test`, race suite, build, vet, large graph, LIF/mixed persistent neural-source matrix, or biological interpretation was run here.
- The result is limited to the bounded continuous two-node public fixture and the public lifecycle operations listed above.

## Next

Root 可採用本報告的 fresh refusal、controller zero-prehistory、warm/reset/restore、ordinary episode 及 nil general control 證據；不應因 fresh neural refusal 修改 `NeuralActivity` 或把它列為已確認 bug。
