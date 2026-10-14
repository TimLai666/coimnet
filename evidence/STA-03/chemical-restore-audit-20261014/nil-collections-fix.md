# STA-03 chemical nil collections compatibility fix

## Problem

舊版 `SaveTrainingBundle` 合法寫出的化學訓練文件可將 `config.chemistry.part.resources` 與 `pending_feedback` 寫成 `null`。前一版共用檢查沿用了個體單檔的嚴格 null 規則，因此新 loader 誤拒這些舊保存物。

## 修正

只修改 `checkpoint/chemical_episode.go`：

- `checkEpisodeRequiredFields` 明確以 `allowNilCollections=false`，保留 standalone episode 的原 null 契約。
- direct `TrainingSnapshot`／task bundle 以 `allowNilCollections=true` 驗證化學 part。
- 只在 validation-only copy 中把已存在且為 `null` 的 `resources` 替成 `{}`、`pending_feedback` 替成 `[]`，不改原始文件、不改 decode 後的 nil 語意。
- 欄位缺少仍拒絕，非空 map／feedback 的 null element 或錯誤型別仍交由既有 chemical checker 拒絕；來源窗口、步數及其他欄位規則未變。

## 驗證

執行指定命令，使用 audit-local cache/temp：

```text
env GOCACHE=$PWD/runs/.tmp/chemical-restore-audit-20261014/go-cache GOMODCACHE=$PWD/runs/.tmp/chemical-restore-audit-20261014/go-mod-cache GOTMPDIR=$PWD/runs/.tmp/chemical-restore-audit-20261014/go-tmp TMPDIR=$PWD/runs/.tmp/chemical-restore-audit-20261014/go-tmp GOPROXY=file:///Users/timlai/go/pkg/mod/cache/download GOSUMDB=off GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS='-p=2 -timeout=60m' go test -count=1 -run '^Test(BundleChemical|EpisodeChemicalNullCollections|BundleRejectsIncomplete|BundleTaskChemicalTraining|BundlePresencePreserves|ChemicalEpisodeCheckpoint)' ./checkpoint
```

結果：`ok github.com/TimLai666/coimnet/checkpoint 1.358s`，exit 0。

`gofmt` 只處理 `checkpoint/chemical_episode.go`，`git diff --check` 通過。

凍結測試 SHA 保持：

- `checkpoint/bundle_chemical_nil_test.go`: `a579f204724b75c9aa348fb903c802ec3e97491b8719f9417f8f3d043c51607e`
- `checkpoint/bundle_presence_test.go`: `6a89dc00a4a5a80f1cc9979faf6ff05daff96873dc2b6a5fd461fe6edadb0114`

## 未驗證

未執行完整 repository suite、race、Root 的 6 個舊 OCR 模型讀回／輸出／續訓比對；不宣稱全框架 zero bug。未 commit、stash 或 reset，也未修改任何測試檔。
