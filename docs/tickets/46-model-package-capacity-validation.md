# 46 — 使用者可以拒絕容量資訊與模型不符的模型包

**Epic:** STA-01／COR-06 模型保存與容量
**User Story:** 使用者可以載入、保存及使用容量資訊與設定、參數相符的模型包，並繼續讀取沒有容量欄位的舊模型包。
**Blocked by:** 無
**Status:** verified_pending_delivery

## Root 決策（2026-10-07）

根因是共同驗證入口只複製 Capacity，沒有核對六個整數欄位。SHA-256 只能檢查文件完整性，重算 checksum 後仍須拒絕與模型不符的容量資訊。

沿用共同驗證入口，不新增 API、格式、依賴或資料遷移。Capacity 存在時，使用已建立且驗證的 Trainer 容量報告，將 FreeParameterCount 設為 ParameterCount 後逐欄精確比對。模型包沒有最佳化器遮罩，全數參數的語意與 Network.Capacity 相同。LIF 的 theta 也包含在內。這樣不必為驗證再建立一份網路。Capacity 為 nil 時不重算、不補寫報告。

## Root 骨架與分工

| 責任 | 可修改檔案 | 預期結果 |
| --- | --- | --- |
| OpenCode 測試執行者 | checkpoint/package_capacity_validation_test.go | 公開入口重現容量不符，checksum 正確的檔案也拒絕。只寫測試 |
| Root | 測試審查、失敗紀錄與指紋 | 手算六欄報告，修正前負向案例失敗、正向案例通過，凍結測試 |
| 另一個 OpenCode 實作執行者 | checkpoint/package.go | 共同入口驗證非 nil 報告，不得修改測試 |
| Root | 文件、證據與 Git 交付 | 完整審查、驗證、原件指紋及遠端提交核對 |

公開簽名保持：LoadModelPackage／SaveModelPackage／LoadModelPackageBundle／SaveModelPackageBundle／NewIndividualFromPackage。檔案及 bundle 載入回傳零值與容量錯誤，建立個體回傳 nil 與錯誤。保存不建立輸出、不改既有檔案。錯誤包含 capacity，原有錯誤及取消契約保持。

## 使用流程與錯誤驗收

| 流程 | 預期結果 | 證據 |
| --- | --- | --- |
| 純量連續／LIF／向量模型建立、保存、讀取 | 正確六欄報告保持。手算分別為 (2,1,1,8,8,3)、(3,3,1,16,16,6)、(2,1,3,26,26,15) | 新回歸及既有容量測試 |
| 六欄各自修改，包括零／負值，重算 envelope checksum | 載入與同格式 migration 拒絕，不回傳部分模型或發布輸出 | 新回歸 |
| 不符報告經保存、bundle 保存、建立個體 | 拒絕且不產生輸出。呼叫端資料保持 | 新回歸 |
| 以底層寫入工具建立校驗正確、容量不符的 bundle | 公開 bundle 載入拒絕 | 新回歸 |
| Capacity 為 nil、已提交舊 fixture | 讀取及再保存不補寫欄位，原件指紋保持 | 新回歸及既有容量測試 |
| 正確報告的副本及往返 | 不共享容量指標、不改參數及序列化結果 | 新回歸及既有往返測試 |
| 非法設定、取消、既有目的地、錯 schema／checksum | 原有失敗流程保持 | checkpoint 套件與完整測試 |

欄位順序為 Nodes、Edges、StateDimension、ParameterCount、FreeParameterCount、MultAddsPerStep。測試以既有模型 fixture、獨立常數與公開入口驗收，不新增生物或學習能力宣稱。

## 固定完成條件

- [x] Root 確認容量竄改在修正前失敗，正向控制通過，測試指紋凍結。
- [x] 六欄不符在共同入口拒絕，單檔、bundle 及建立個體均有覆蓋。
- [x] 三種核心的合法容量、nil 舊格式、資料所有權及既有原件保持。
- [x] gofmt、build、完整一般／race、vet、相依性及治理檢查通過。
- [ ] 完整 diff 審查、需求證據、提交掃描、推送與遠端核對完成。

## 減法審查

共用既有驗證入口與已建立的模型，不增加驗證服務、註冊表或格式版本。參數數值是否學會任務、其他容量估算與 GPU 全圖能力不屬本票。

## 來源

- [模型包](../../checkpoint/package.go)
- [容量報告](../../learning/capacity.go)
- [既有容量驗收](../../checkpoint/package_capacity_test.go)
- [原模型包契約](16-lif-individual-and-model-package.md)
- [工程決策](../../ENG.md)
- [驗證資料目錄](../../evidence/STA-01/model-capacity-20261007/)

## 驗證結果

五個新增頂層回歸、九個 CLI 流程、Mac 完整一般與 race 各 57 個套件、build、vet、格式、相依性與八個格式／治理頂層檢查全部通過。648 份 Go 來源凍結，3,053 份範圍外既有檔案保持。證據文件必要欄位已依既有治理契約補齊。原失敗與修復後日誌保留，沒有修改測試。

[工程證據](../../evidence/STA-01/model-capacity-20261007/verification.json)。[Root 審查與派工限制](../../evidence/STA-01/model-capacity-20261007/root-review.md)。本票只有軟體正確性證據，需求狀態維持 89／91。
