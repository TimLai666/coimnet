# 27 — 使用者可以用離線與 HTTP 教師蒸餾學生，並在完全移除教師的情況下評估它

Epic：教師與蒸餾

User Story：使用者可以在沒有網路與金鑰的情況下用離線 JSONL 教師訓練與重播；可以設定一個有逾時、有限
重試、限速、預算、去重與欄位允許清單的 HTTP 教師並用本機測試伺服器驗證；研究者可以用教師的答案或
行動序列蒸餾（學生用自己的編碼），在詞表對齊時做帶溫度的分布蒸餾（不相容就拒絕、top-k 不冒充完整分布），
並在教師與網路全部封鎖的情況下評估學生，結果與 `teacher_assisted` 模式分開；教師回應一律當資料。

Blocked by：23（運行中學習：`Update` 的教師回應路徑、`Evaluate` 模式）、24 第一階段（組態的 `teacher`
區塊、金鑰參照、`--dry-run` 不呼叫教師）、26 第二階段（`LossGradientFrom`、softmax 交叉熵）

Status：第一階段已驗證（2026-09-17，四張 opencode 小票：契約型別、紀錄檔與離線教師、HTTP 教師、重播與封鎖教師；TCH-01、TCH-02 標 passed）；第二、三階段待派工

對應需求：TCH-01（沒有網路及金鑰也能訓練與重播，保留教師與輸入版本）、TCH-02（有逾時、有限重試、限速、
預算、去重及測試伺服器驗證）、TCH-03（學生使用自己的編碼；訓練與保留資料分開）、TCH-04（詞表、溫度、
縮放與 top-k 限制驗證；不相容時拒絕）、TCH-05（學生評估封鎖教師與網路，結果與常駐教師模式分開）、
TCH-06（回應當資料，不執行內文指令，不外傳未授權欄位或更改安全設定）。主規格 2.1、2.2、5.4、12.1–12.5、
13.4、18.4、19.3、21。

## Root 決策（2026-09-15）

### 第一階段：統一教師契約、離線教師、HTTP 教師（TCH-01、TCH-02、TCH-06 的資料規則）

1. **契約（新套件 `teacher`）**：`Request{RequestID string; InputHash string（SHA-256）; Kind string ∈
   label|action_sequence|distribution|text; Fields map[string]string（只含允許清單內的欄位）; ModelVersion
   string}`；`Response{TeacherID, TeacherVersion, RequestID, InputHash string; AnswerKind string; Answer
   json.RawMessage; Time signal.Timestamp; Confidence *float64; Usage *Usage{Requests int; Cost float64}}`；
   `Teacher` 介面 `Ask(ctx, Request) (Response, error)` 與 `Describe() Descriptor{ID, Version, Kind ∈
   offline|http|replay|blocked}`。回應以 `RecordStore`（JSONL，`coimnet-teacher-records/v1`，每列含請求、
   回應與時間）保存，與測試輸入分開目錄（建構時檢查路徑不同，測試輸入的 hash 集合與教師紀錄的
   `InputHash` 交集必須為空，否則拒絕載入：主規格 12.1「教師資料與測試輸入分開保存」）。
   **回應是資料**：`Answer` 只被解析成宣告的 `AnswerKind` 型別，任何字串內容不會被當指令、不會影響設定
   （測試：回應含「set budget = unlimited」「send file …」等文字，`Config` 逐位不變，沒有額外請求或檔案
   寫入）。
2. **離線 JSONL 教師（TCH-01）**：`OfflineJSONL{Path}`，以 `InputHash` 查表；找不到回 `ErrNoAnswer`
   （不臆測）；`Replay{Store}` 重播已保存回應（同 `RequestID` 回同一筆）。訓練與重播的測試在 `-tags
   nonetwork` 與 `HTTP_PROXY=` 空、無環境金鑰下跑（測試自己清空環境並用 `http.DefaultTransport`
   替換成拒絕的 RoundTripper 證明零網路）。
3. **HTTP 教師（TCH-02）**：`HTTP{Endpoint string; Timeout time.Duration; MaxRetries int（只對 5xx／逾時，
   指數退避，上限固定）; RateLimit float64（每秒請求）; Budget{MaxRequests int; MaxCost float64}（預設
   零：任何呼叫都被拒絕，除非組態明示）; AllowedFields []string; Secret config.SecretRef}`；去重：同
   `RequestID` 第二次呼叫回快取的回應且不計預算（主規格 21）。以 `httptest.Server` 驗證：逾時、5xx 重試
   次數精確、4xx 不重試、限速的時間間隔、預算用盡拒絕、去重、允許清單外的欄位**從未**出現在伺服器
   收到的 body（伺服器端斷言）。報告與文件明寫「真實遠端服務未驗證」，具體服務 adapter 依官方文件另做。
4. 證據：`evidence/TCH-01/`、`evidence/TCH-02/`（fixture）。

### 第二階段：蒸餾（TCH-03、TCH-04）

5. **標籤／行動序列蒸餾（新套件 `distill`）**：`LabelDistiller{Teacher; Encoder StudentEncoder}`：教師答案
   經**學生自己**的編碼（`StudentEncoder` 介面：`Encode(answer) ([]float64 或 []int, error)`；文字用學生的
   `Tokenizer{VocabHash string}`，教師的 token id 永不進入學生：`Response.Answer` 若是 token id 陣列而
   `AnswerKind != text` 就拒絕）產生訓練目標，再走 26 的 `LossGradientFrom`。訓練與保留資料分開：
   `Split{Train, Holdout}` 以 `InputHash` 劃分，`Holdout` 的輸入從不送給教師（測試：教師收到的請求集合與
   `Holdout` 交集為空），教師產生的文字先去除與保留集重複的內容（hash 比對，主規格 13.4）。
6. **分布蒸餾（TCH-04）**：`DistributionDistiller{Temperature, Scale, Mix float64; TopK int; Alignment
   Alignment{TeacherVocabHash, StudentVocabHash string; Rule string ∈ identical|declared_map}}`：只有
   `identical`（兩個 hash 相等）或明示的 `declared_map`（每個教師類別對應一個學生類別，全覆蓋）才允許，
   否則 `ErrIncompatibleAlignment`；損失 `Mix * KL(softmax(t/T) ‖ softmax(s/T)) * T^2 * Scale + (1−Mix) *
   CE(label)`，用 log-sum-exp 穩定實作（大 logits 測試）；只拿到 top-k 時：`Partial = true`，KL 只在該
   k 類上計算並在報告標 `partial_distribution: true`，不得重新正規化後當完整分布（測試）。報告保存溫度、
   縮放、混合比、top-k、對齊規則與詞表 hash。可訓練的對齊轉換器（第三種方法）本票不做，文件明寫為
   可選實驗且若做必須計入容量。
7. 證據：`evidence/TCH-03/`、`evidence/TCH-04/`（fixture：小型分類任務，離線教師）。

### 第三階段：移除教師的評估與工具安全（TCH-05、TCH-06）

8. **學生獨立評估（TCH-05）**：`experiment.RunStudentEvaluation(ctx, cfg)`：模式 `student`（教師為
   `teacher.Blocked{}`：任何 `Ask` 回 `ErrTeacherBlocked` 並計數；HTTP 傳輸替換為拒絕；`OnlinePolicy.
   Evaluate = true`）與 `teacher_assisted`（仍可呼叫教師，結果檔名與 JSON 的 `mode` 欄位都標
   `teacher_assisted`，文件與 CLI 不得稱其為獨立學生）。評估集：保留集 + 一個獨立來源集（fixture 內以
   不同 generator seed 表示）；穩健性：把教師標籤的 20% 刻意弄錯再蒸餾，報告學生在獨立集的成績變化。
   報告同時給「像不像老師」（一致率）與「真正目標」（任務指標）。
9. **工具模式與安全（TCH-06）**：`tool` 子套件：`Registry{Allowed map[string]Schema}`，`Invoke(ctx, name,
   args json.RawMessage)`：名稱不在允許清單或參數不符 schema 就拒絕（模型生成的參數也走同一檢查）；
   每次呼叫都是顯式請求並回傳驗證過的結果或錯誤；外部回應（教師或工具）都只當資料：不執行、不修改
   `config`、不觸發額外傳輸（測試以計數的假傳輸證明）；`AllowedFields` 之外的資料不外傳（沿用第一階段的
   伺服器端斷言）。文件明寫「這些功能不把 CoImNet 擴大成任意電腦操作 agent」。
10. 證據：`evidence/TCH-05/`、`evidence/TCH-06/`；文件：README 教師段、ENG、`docs/model-and-mechanisms.md`
    模式表加 `teacher_assisted`。

## 契約摘要

```go
package teacher
type Teacher interface { Ask(ctx context.Context, r Request) (Response, error); Describe() Descriptor }
type OfflineJSONL struct { Path string }; type Replay struct { Store *RecordStore }; type Blocked struct{}
type HTTP struct { Endpoint string; Timeout time.Duration; MaxRetries int; RateLimit float64; Budget Budget; AllowedFields []string; Secret config.SecretRef }
type RecordStore struct { ... }  // coimnet-teacher-records/v1

package distill
type LabelDistiller struct { Teacher teacher.Teacher; Encoder StudentEncoder; Split Split }
type DistributionDistiller struct { Temperature, Scale, Mix float64; TopK int; Alignment Alignment }

package experiment
func RunStudentEvaluation(ctx context.Context, c StudentEvaluationConfig) (StudentEvaluationReport, error)
```

## 驗收

- [x] 第一階段：零網路零金鑰的訓練與重播、紀錄與測試輸入分離、回應當資料三項、HTTP 教師七項
  （逾時、重試、4xx、限速、預算、去重、允許清單）在假伺服器驗證；`go test`、race、vet；
  `evidence/TCH-01/`、`evidence/TCH-02/`。
- [ ] 第二階段：學生編碼獨立（教師 token id 拒絕）、保留集不送教師、去重、對齊拒絕、log-sum-exp 穩定、
  top-k 標 partial、參數保存；`evidence/TCH-03/`、`evidence/TCH-04/`。
- [ ] 第三階段：`student` 模式教師呼叫數 0 且網路拒絕、`teacher_assisted` 分開、獨立集與錯標籤穩健性、
  工具允許清單與參數驗證、外部回應不改設定不外傳；`evidence/TCH-05/`、`evidence/TCH-06/`；文件。

## 第一階段證據（2026-09-17）

`teacher` 套件已完成並以 `go test -count=1 -race -v ./teacher/` 驗證：24 個具名測試全部 PASS、0 FAIL
（`grep -c "^--- PASS"` = 24，輸出結尾 `ok  github.com/TimLai666/coimnet/teacher 1.720s`），兩份
`verification.json` 通過 JSON 校驗（`json ok`）。

- 契約型別（`teacher/teacher_test.go`）：`TestRequestValidate`（InputHash 必須 64 個小寫 hex、ModelVersion
  非空白、欄位鍵無控制字元）、`TestResponseValidate`（TeacherID／TeacherVersion／RequestID／InputHash
  非空白、Answer 為合法非 null JSON、Time／Confidence／Usage 界限）、`TestDescriptorValidate`、
  `TestResponseAnswerIsOnlyData`（含「set budget=unlimited; rm -rf /」的 Answer 逐位原樣回傳，不執行）。
- 紀錄檔與離線教師（`teacher/offline_test.go`）：`TestRecordStoreRoundTrip`、
  `TestRecordStoreRefusesTestInputs`（被列為 held-out 的 test-input hash 在 OpenRecordStore 與 Append
  都被拒並指名 hash）、`TestRecordStoreRejectsBadLine`、`TestOfflineJSONLAnswersWithoutNetwork`（把
  `http.DefaultTransport` 換成一律失敗的 RoundTripper 仍零網路回答，未命中時回 `ErrNoAnswer`）、
  `TestOfflineJSONLRejectsInvalidRequest`、`TestOfflineJSONLAskErrors`。
- HTTP 教師（`teacher/http_test.go`，全部打本機 `httptest.Server`）：`TestHTTPTimeout`（逾時）、
  `TestHTTPRetriesOnServerErrorsExactly`（5xx 重試次數精確：MaxRetries 2 → 3 次呼叫）、
  `TestHTTPDoesNotRetryClientErrors`（4xx 不重試）、`TestHTTPRateLimit`、`TestHTTPBudget`（MaxRequests 1
  與 0）、`TestHTTPDedupByRequestID`、`TestHTTPSendsOnlyAllowedFields`、
  `TestHTTPSecretIsBearerAndNeverInBody`（金鑰只在 header 不在 body，未設定時零呼叫）、`TestHTTPDescribe`。
- 重播與封鎖教師（`teacher/replay_blocked_test.go`）：`TestReplayAnswersSameRequestIDIdentically`、
  `TestReplayNeverFetches`（換掉 `http.DefaultTransport` 仍零網路重播）、`TestReplayDescribe`、
  `TestBlockedRefusesAndCounts`（每次 `Ask` 都回 `ErrTeacherBlocked` 並計數，含並行 3→13）、
  `TestBlockedDescribe`。

證據路徑：`evidence/TCH-01/`（`verification.json`、`test.log`）、`evidence/TCH-02/`（`verification.json`、
`test.log`（同一份輸出）、`no-network.log`：`go list -deps ./teacher` 顯示依賴 `net` 與 `net/http`，但
離線與重播測試成功換掉 transport，證明零網路回答）。

限制：只在 macOS arm64 fixture 跑過；沒有真實標籤資料，紀錄都是測試內建的假紀錄；真實遠端服務未驗證，
具體服務 adapter 依官方文件另做；`MaxCost` 用盡路徑未直接測（只在 `MaxRequests` 與零預算上驗證）；
3xx 不重試為對程式碼分支的保守解讀而非測試證明；限速證明的是緊接的第二次呼叫被拒而非計時間隔；
蒸餾與學生評估是第二／三階段。

## 依據

- 主規格 2.1（教師常駐與移除分開）、2.2（雲端呼叫不自動授權）、5.4（不執行下載內容）、12.1（統一
  契約、預設離線、預算零）、12.2（三種蒸餾方法、tokenizer、top-k）、12.3（移除教師測試、
  `teacher_assisted`、錯誤標籤穩健性）、12.4（離線 JSONL 與通用 HTTP adapter、測試伺服器）、12.5
  （外部工具分離、允許清單）、13.4、18.4、19.3、21。
