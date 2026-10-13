# 化學組成多通道稽核報告

稽核基準：`c43ff4051ed3e80b3988ee37335353579f0c82d5`。範圍是多通道 source ordering、受體 record 的 location/channel、MixSum/MixMax，以及 effect clamp 的前向與反向。以下只記錄可由 live source、既有證據或既有測試內容核對的結論。既有測試與 probe 證據未在本輪重新執行。

## Problem

化學層的契約要求 occupancy 為零時 sensitivity effect 的 gain、offset、threshold 與未宣告 effect 完全中性。需確認多通道來源與受體定位沒有把 channel 或 region 對錯，也需確認 MixSum/MixMax 及 clamp 在前向、反向使用相同的 ownership 與邊界語意。

## Done

- 已核對 source 宣告順序、runtime source row、kinetics input、chemical reverse 的 release gradient，以及 source feedback dispatch。
- 已核對 receptor record 的 channel、顯式 location、region mapping、Occupancies 與 BackwardOccupancies。
- 已核對 MixSum/MixMax 的 occupancy aggregate、winner、tie 行為與反向 gradient；目前未找到可重現差異。
- 已確認一項 P2 契約差異：`checkedEffect` 接受不包含 1 的 gamma interval，造成零 occupancy 時 gain 被 clamp 成非中性值。

## CONFIRMED findings

### P2-01：`checkedEffect` 允許 gamma bounds 排除中性值 1

- **位置與觸發**：`modulation/effects.go:186-208` 對 `GammaMin`、`GammaMax` 只檢查有限、正值與 `GammaMin <= GammaMax`，沒有檢查 `GammaMin <= 1 <= GammaMax`。例如 sensitivity effect `GammaScale: 0, GammaMin: 1.25, GammaMax: 1.5` 搭配 `{Receptor: 0, Cell: 0, Occupancy: 0}` 會被接受。`GammaMin: 0.5, GammaMax: 0.75` 也同樣被接受。
- **實際結果**：`modulation/effects.go:148-168` 先以 gain 1 初始化，再以 `clamped64(1+GammaScale*total, GammaMin, GammaMax)` 套用 gain；`clamped64` 位於 `modulation/effects.go:239-247`。既有 `evidence/MOD-02/chemical-composition-20261014/neutral-probe.log` 顯示上述兩種 interval 在 `mix: ""`、`sum`、`max` 均回傳 `error: null`，且零 occupancy gain 分別為 `[1.25]`、`[0.75]`，`clamp.gamma: 1`，`matches_declared_neutral: false`。
- **契約差異**：`modulation/effects.go:76-78` 明定零 occupancy 必須與未宣告 effect bit-for-bit 相同。相同要求也在 `docs/tickets/21-chemical-state-receptors-and-effects.md:47-58`、`docs/model-and-mechanisms.md:54` 與 `docs/handoff/CoImNet_Implementation_Plan.zh-TW.md:539-550`；ticket 明確寫出 `occ=0 => gamma=1`。因此這不是單純缺少測試，而是可由公開入口重現的前向契約違反。
- **反向影響**：`modulation/receptor_gradient.go:213-267` 以相同 gamma clamp 條件計算 sensitivity gradient，`interiorBound` 在 `modulation/receptor_gradient.go:285-287` 對邊界與區間外值回傳 false。這使反向對該零 occupancy clamp 的導數為零，與實際前向被 clamp 的函式一致，但沒有修復前向的中性語意。`learning/chemistry.go:235-275` 會把 `ApplyEffects` 的 gain 直接交給 core modulation，故此差異可進入完整化學 forward。
- **根因**：合法性條件與中性證明的前提不一致。實作允許任意正且有序的 gamma interval，文件則假設 interval 包含中性值 1。
- **影響**：任何目前或外部 caller 使用有效但排除 1 的 gamma interval，在 occupancy 為零時仍會改變 core gain。現有 source、test、example、model 搜尋未找到依賴這種設定的項目，但這只降低目前相容性證據，不會消除公開設定的錯誤行為。
- **最小修正建議**：先由規則決定要維持既有 `occ=0` 中性契約，或改寫該契約。若維持，最小方向是在 `checkedEffect` 加上 `GammaMin <= 1 <= GammaMax` 的合法性條件，並補 `mix=""`、`MixSum`、`MixMax` 的公開入口回歸案例。若改規則，則需同步更新 effects 文件、ticket、模型文件、既有 neutral 測試與 reverse 邊界定義。本輪未修改合法性檢查。

本範圍沒有 P0 或 P1 confirmed finding。

## 已核對但目前未見可確認差異

### 多通道 source ordering

- `modulation/config.go:212-221` 要求 source 數量等於 channel 數，並要求 `Sources[k].Channel == k`。
- `modulation/sources.go:68-81,135-157,174-205,222-233` 的各 source 只在自己的 channel 寫入 rate；`learning/chemistry.go:197-214` 逐一以 runtime index `k` 檢查非本 channel 的非零 rate，並把 `rates[k]` broadcast 到每個 region。
- `learning/chemical_sequence.go:530-544` 反向依同一個 config index 逐 channel 累計 `chemGradient.Release[region][channel]`，再 dispatch 至相同 channel 的 controller 或 neural source。
- `modulation/config_test.go:89-102` 與 `modulation/source_channel_validation_test.go:17-90` 已有順序及 channel 寫入邊界案例。讀過的來源歷史中，順序契約由 `beb1956` 引入，反向同序 dispatch 位於 `9ed31a5` 後續變更；基準到目前的 relevant diff 為空。未發現可重現的 index 置換或 region/channel 交叉。

### Receptors record location/channel

- `modulation/receptors.go:97-167` 檢查 cell 順序與範圍、channel 範圍、status、evidence、measurement kind、mapping version 及 status 對應 coefficients。
- `modulation/receptors.go:182-248` 建立 occupancy record 時保留 record 的 channel 與解析後的 region；`modulation/receptor_location.go:22-68` 使顯式 location region 優先於 node region mapping，nil location 才使用 mapping。
- `modulation/config.go:238-246` 檢查顯式 location 對 declared region 的合法性。`modulation/receptor_gradient.go:13-87` 的 reverse 依 `record.Region` 與 `record.Channel` 回寫 concentration gradient。
- `modulation/receptor_location_test.go:40-151` 與 `modulation/receptor_gradient_test.go:114-141` 已涵蓋顯式 region、同 cell 不同 location、nil legacy mapping、channel/location gradient。未發現 record location 或 channel 被改用成 node mapping 的可重現差異。

### MixSum/MixMax 與 clamp 前後向

- 前向 `modulation/effects.go:62-83,123-181` 對同一 receptor/cell 以 MixSum 加總、MixMax 取最大 occupancy，之後以單一 coefficient set 套用 gamma、beta、theta clamp。
- 反向 `modulation/receptor_gradient.go:142-197,213-287` 重建相同 totals；MixSum 對所有 records 累加，MixMax 只回傳 winner，tie 依相同的嚴格 `>` 與 declaration traversal 保留先者。clamp 邊界的導數為零，與 forward 的 closed interval clamp 一致。
- `modulation/effects_test.go:45-156,178-366` 與 `modulation/receptor_gradient_test.go:314-496` 含 zero neutral、手算 MixSum/MixMax、tie、finite difference 及 clamp gradient 案例。ticket 的「raw occupancy aggregate 後套一組 coefficients、衝突 coefficients 拒絕」要求也與 `modulation/effects.go:80-83` 相符。除 P2-01 外未找到可重現的 aggregate、winner 或反向 clamp 不一致。

## NEEDS INVESTIGATION

1. 需由規格負責人裁決 gamma interval 是否必須包含 1。現有契約文件一致要求零 occupancy 中性，但沒有一條明確的 `GammaMin <= 1 <= GammaMax` 合法性條文；本報告不自行改規則。
2. 既有 `neutral-probe` 是公開入口的歷史證據，沒有在本輪重新執行 Go。仍缺一個納入現行測試套件的回歸測試，直接覆蓋合法性檢查、三種 mix、zero occupancy forward，以及對應 reverse gradient。
3. 既有 `evidence/MOD-03/verification.json` 記錄過 zero neutral、MixSum/MixMax 與 clamp 的測試結果，但只覆蓋 default 或包含 1 的 bounds，不能證明排除 1 的設定應被接受，也不能取代本輪動態重跑。
4. 搜尋目前 tree 的 tests、examples、models、JSON/YAML 未找到 `GammaMin > 1` 或 `GammaMax < 1` 的既有依賴。這是目前 tree 的證據，不是對外 caller 或未追蹤模型的相容性證明。
5. 多通道 ordering、受體 location/channel、MixSum/MixMax 只完成 source、diff、歷史與既有證據核對；本輪依限制未進行全套 Go、race、跨程序或完整模型執行驗證。

## 已核對契約與檔案 SHA

### 契約與證據

| 檔案 | SHA-256 |
|---|---|
| `evidence/MOD-02/chemical-composition-20261014/plan.md` | `632577097066c95647cbb039a6ed1d135dce581bd34cd900d5db684312a24fb3` |
| `evidence/MOD-02/chemical-composition-20261014/baseline.json` | `4fa4847815affbfd46a762976517ebda5f13c0080ed39121cc71f1fae454ec47` |
| `ENG.md` | `3c2e550fb165d3d17fe97ee555c64e009cefc7945454dc73172e5820f80b45e5` |
| `docs/handoff/CoImNet_Implementation_Plan.zh-TW.md` | `b92126883a10cdb9fb852276796e0521111520709bb7ca82d06f98dbab8935ea` |
| `docs/tickets/21-chemical-state-receptors-and-effects.md` | `ccedfbcaf6185ac14a0ee2183796b0e82a8aea6d7e3c530455f0b58c46c79179` |
| `docs/model-and-mechanisms.md` | `43d9e559e9c25ea8767d0e65e9bb920406258cb3975912b21d9490b347184d61` |
| `evidence/MOD-02/chemical-composition-20261014/neutral-probe.log` | `03ede1580f078804c3164e240444e73e0e6341e851c8ed5c76a79f0e078e3e88` |
| `evidence/MOD-02/chemical-composition-20261014/neutral-probe.json` | `13bbebfb59f132f024ffa1b14fcd409ac2dacd2e41fe955bb3241bad2dfd7fe6` |
| `evidence/MOD-03/verification.json` | `683162dab66e0c900370921bae057e560158b508afc9aee1ef5bf59ec708eb06` |

### Live source 與相關測試

| 檔案 | SHA-256 |
|---|---|
| `modulation/effects.go` | `8cfaa586fd6ad6b85b2adb16604c93b6850a1b67a2a82cd51ad5f96b816ae7b7` |
| `modulation/receptors.go` | `de4b3b2926bdeaa53c1805cab7898d815b79d81015d809542f16fdd12fb8120d` |
| `modulation/receptor_location.go` | `79bd05c0e34dd913502ee41fa44324719db3836436470e34c9d456d22bc467b1` |
| `modulation/receptor_gradient.go` | `c87110f8a628c171a908c38695acd06a522634c7600a98f945618482bfb5c953` |
| `modulation/config.go` | `888441fa39463afe1afe0fb28c8e008d23c55f5e6b8e6aab171e4fd434928a01` |
| `modulation/sources.go` | `5280342db13372aff9ec3f3f540961135e762d97f0af4821a47e6986b85883af` |
| `learning/chemistry.go` | `4e6164cbafd843c56c357558e9b8a580598b607daeccffbfc74a714052a52032` |
| `learning/chemical_sequence.go` | `c73b9c655e451ac5dd882547a260c69e48ea5379b4b08c53c4dd2cf3b5ac9305` |
| `modulation/effects_test.go` | `570b49b61efaad8d66cecff78a29388dc69d2acfc58eef11b47b7de249b4a6aa` |
| `modulation/source_channel_validation_test.go` | `8e1ee27fd1af8f046ec6d86148c9a62e5c65674cbe73df1264faf8ef199fc281` |
| `modulation/receptor_location_test.go` | `682589668dd52d17b24e032008ebf74710a0d2c8f247c69facc3cf0cb1e29099` |
| `modulation/receptor_gradient_test.go` | `c0742bd8e7427fd37c71be847b4cf928b361f61c8c8a286ebf4f1b5ced24aeb3` |
| `modulation/config_test.go` | `9eea2c4175719e9a635328c7a39c1f7e2faf39d54b8a76210c6c1921151aebad` |
| `modulation/receptors_test.go` | `c5e5aca53a7122f21f05df642833fc8cfab390caba55b0f74b4afbf0c68a8301` |
| `modulation/sources_test.go` | `cc8c6ab0b82e6aec6ca390c710cb510a6be9e9b2a888966bd6d20e6411ab28d8` |
| `learning/chemical_multichannel_test.go` | `d59435ce5b0440e70c3af33c36946ad098fbce40c2faac01ac6c27c87d75b790` |
| `learning/chemical_mixed_reference_test.go` | `09da8868555df9f45804aa18c4865ce376d0c06d7c66b7e2286ec6d0145b7242` |
| `learning/chemical_recompute_test.go` | `2086215c6c242503cdd49566394fdd8ec53ee5b59dadf420a2f9719d50ed8947` |

## Actions

| 動作 | 對象 | 結果 |
|---|---|---|
| 唯讀 source、diff、歷史與既有 evidence 核對 | CoImNet baseline `c43ff4051ed3e80b3988ee37335353579f0c82d5` | 完成；relevant source diff 為空 |
| 建立本稽核報告 | `runs/.tmp/chemical-composition-audit-20261014/multi-review/report.md` | 完成 |
| Go、race、網路、下載、cache、commit、stash、reset | 全部 | 本輪未執行 |

## Verified

- 以 live source 行號核對了 source ordering、record location/channel、MixSum/MixMax、forward clamp、reverse clamp，以及 chemical row 的接線順序。
- 讀取既有 `neutral-probe.json` metadata 與 `neutral-probe.log`，確認公開入口對排除 1 的 gamma bounds 回傳成功且產生 `[1.25]`／`[0.75]`；這是 P2-01 的最小重現證據，本輪沒有重跑該 command。
- 讀取既有 MOD-03 test evidence、相關測試 source 與 baseline hashes。這些資料支持預設及包含 1 的案例，但不支持排除 1 的合法性決策。
- 沒有宣稱整體零 bug、模型品質提升或測試通過。

## Notes

本報告只涵蓋指定化學組成稽核。P2-01 的修正涉及公開合法性規則與相容性，需先完成規格裁決；在裁決前不應自行放寬或收緊 `checkedEffect`。

## Next

先裁決 gamma bounds 是否必須包含 1。若維持現有零 occupancy 中性契約，再由實作者補 legality check、三種 mix 的公開入口回歸測試與完整 forward/reverse 驗證。

Skills：無
