# TSK-01 glyph contract 證據審查

## Problem

Ticket 95 目前仍是 `in_progress`。本審查核對 red、reference-red、green-final、green-current、來源 manifest、manifest-adjudication，以及 ticket 95 的五項完成條件，確認哪一些收據可作為證據，並避免把歷史檔名重用誤判成新的程式缺陷。

## Done

只讀檢查了 `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/` 的四組結果、`source-red.json`、`source-reference-v2.json`、`source-final-v1.json`、`source-final.json`、`reference-red-overlay.json`、原始收據文字檔、`manifest-adjudication.json`、probe 收據與結果、工程收據，以及 `/Users/timlai/Developer/coimnet/docs/tickets/95-ocr-generate-representability.md`。沒有執行 Go、編譯、測試、race、vet 或清理。

## Confirmed Findings

沒有確認的 P1/P2 缺陷。

可核對的證據如下：

- `red.json:11-22` 的預期與實際退出碼都是 1，收據計數為測試 pass 9、fail 11、skip 0、package pass 1。重新讀取 `red.log` 後計數相同，收據中的 log SHA 與檔案實際 SHA 相同。
- `reference-red.json:13-24` 的預期與實際退出碼都是 1，計數為 20、26、0、1。`reference-red-overlay.json:2-5` 明確指出兩個舊實作覆蓋檔，與 `render-legacy-skeleton.go.txt`、`fixture-before.go.txt` 的內容可重建；重新讀取 `reference-red.log` 後計數相同。
- `green-final.json:11-22` 的預期與實際退出碼都是 0，計數為 46、0、0、3，使用 `source-reference-v2.json`。`green-current.json:11-22` 的預期與實際退出碼也都是 0，計數同樣為 46、0、0、3，使用 `source-final.json`。兩個 log SHA 都與實際檔案相同。
- `source-reference-v2.json` 的實際檔案 SHA 為 `22ee19aab98cafb8c385b400be12eee2cf6a287fbc1ec08b2496ed6db570ad84`，`source-final.json` 的實際檔案 SHA 為 `9217c926d1edb59900c0101cbaccffc8c22662de42532a545c761d51382f6622`，均與 `manifest-adjudication.json:8-10`、兩組 finalized 收據相符。
- `source-final.json` 共 804 個來源項目，逐項對目前工作樹重算後為 804/804 存在且 0 項不符。它可支持 `green-current`、build、unit、format、module-verify 使用目前來源。
- `source-reference-v2.json` 與目前工作樹只有 `tasks/ocr/glyphs/render.go` 一項差異，差異是 API 說明註解；`source-final-v1.json` 是較早的中間 manifest，除該註解外還保留舊版 `generate_capacity_test.go`。因此 `green.json` 的 20 個 pass 不能取代 46 個 pass 的 `green-final`／`green-current`，但其自身 SHA 收據仍是內部一致的。
- `manifest-adjudication.json:2-10` 所述的重建邏輯可信：舊 SHA 確實對應 `source-reference-v2.json`，新 SHA 確實對應 `source-final.json`，且目前來源的唯一語意差異與文件所述相符。這只能證明收據與 manifest 的內容一致，不能把收據提升為我親自重跑的工程結果。

## Needs investigation

1. `reference-red-receipt-original.json.txt:19-20` 與 `green-final-receipt-original.json.txt:17-18` 保留歷史檔名 `source-final.json`，但其 SHA 是舊的 `22ee...`。`manifest-adjudication.json` 已把正式收據改指向 `source-reference-v2.json`，所以目前沒有確認的執行缺陷；後續若要把原始文字檔單獨當權威證據，必須明確標成歷史收據或再補 overlay 與 manifest 的 SHA 對照。
2. `source-final-v1.json` 與 `green.json` 是未被 `manifest-adjudication.json` 列入 earlier receipts 的中間證據，且混合了舊回歸測試與新 fixture/render。需要由主 agent 決定是否只保留為歷史參考，避免驗收者誤採 20-pass 的舊收據。安全確認方式是以 `green-final.json`、`green-current.json` 及其各自 manifest SHA 作唯一完成判準。
3. 完整工程收據尚未足以勾選 ticket 第 3 項。`build.json:7-13`、`unit.json:9-15`、`format.json:806-812`、`module-verify.json:7-13` 都是目前 `source-final.json` 且退出碼為 0，但目錄內沒有 `go vet ./...` 收據；`race.log` 在 `TestRunReportsSeenAndUnseenSeparately` 開始後即結束，沒有完成收據。依委託要求，這個未完成 receipt 忽略、不判成失敗。安全確認方式是等待完整 race receipt，另補明確的 vet 命令與退出碼。
4. Ticket 第 4 項沒有清理完成證據。`runs/.tmp/glyph-contract-20261014/` 仍有 `compatibility.go`、`probe.go`、`go-cache/`、`go-tmp/` 與本審查報告；本輪未清理，也沒有清理收據。這是未完成證據，不是本審查新增的程式缺陷。
5. Ticket 第 5 項沒有 commit、push、遠端提交或來源讀回收據。這輪未做任何 Git 寫入或遠端操作，因此不能勾選。

## Ticket 95 完成項證據矩陣

| Ticket 項目 | 證據判定 | 依據 |
|---|---|---|
| Root 固定樣本容器、字長、影像位元組及正常／空批次 | 證據足以支持，尚未代勾 | `green-final.json:19-22`、`green-current.json:19-22` 的 46 pass；`probe-observed.json:2-25` 的正常與空寬範圍結果 |
| 凍結公開失敗回歸並確認實際崩潰 | 證據足以支持，尚未代勾 | `red.json:11-22`、`reference-red.json:13-24`；`probe-receipt.json:7-15` 明載三個 recovered public API panic，且 `large_allocation_performed` 為 false |
| 共用尺寸驗證、公開入口、正常接續及完整工程檢查 | 部分證據，不能勾選 | targeted green、build、unit、format、module verify 通過；vet 缺收據，race receipt 未完成 |
| 同步 TSK-01 證據及暫存清理 | 未證實，不能勾選 | 無 cleanup receipt，暫存仍存在 |
| 拆分提交、推送、遠端及來源讀回 | 未證實，不能勾選 | 無 Git／遠端 readback receipt，本輪未執行 |

## Reviewed files

- `/Users/timlai/Developer/coimnet/docs/tickets/95-ocr-generate-representability.md`
- `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/red.json`
- `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/reference-red.json`
- `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/green-final.json`
- `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/green-current.json`
- `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/manifest-adjudication.json`
- `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/source-red.json`
- `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/source-reference-v2.json`
- `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/source-final-v1.json`
- `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/source-final.json`
- `/Users/timlai/Developer/coimnet/evidence/TSK-01/glyph-contract-20261014/reference-red-overlay.json`
- 四組結果的 `.log`、原始收據文字檔、probe／工程收據與目前 overlay 快照

## 未驗證項目

- 本報告沒有親自執行 Go 或重跑任何收據命令。
- 沒有親自驗證當時歷史工作樹以外的 804 項來源內容，只重算目前工作樹與 manifest 的對照，以及收據／log／快照中可見的 SHA。
- 沒有把 `int` 表示範圍推論成 runtime `maxAlloc`、physical RAM 或所有大型宣告都安全；這些仍沿用程式審查報告的 Needs investigation。
- Invert 字距語意矛盾維持 ticket 既定待釐清，不列為本次完成或缺陷判定。

## Next

主 agent 應等待完整工程 receipt，補 `go vet ./...` 的可核對收據，並決定是否把 `green.json`／`source-final-v1.json` 僅標為歷史證據，再依清理與遠端讀回證據更新 ticket 狀態。
