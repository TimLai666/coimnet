# 生物證據登錄

本文件登錄 CoImNet 模型設計所引用的生物來源（主規格 3.1「已核對的依據」與 11.7「生物命名設定的完成方式」），
記錄每項設計「採用」了什麼、「不宣稱」什麼。登錄內容與 `docs/evidence-registry.json`（`data/` 是原始資料目錄且被 .gitignore 排除，登錄檔改放 docs）（schema
`coimnet-evidence-registry/v1`）一致。人工數值只能用規格允許的假設名稱，不得標成定量生物重現；
工程數值不因寫進程式就升格為生物定律（主規格 3.3）。

### [S14] Ecdysone signaling regulates the formation of long-term courtship memory in adult Drosophila melanogaster

#### 原文主張

成年果蠅的 20E 與長期求偶記憶相關，給藥時機影響記憶結局（主規格 3.1；查核範圍：摘要與圖說，適用於成年雄性果蠅）。

#### 本專案採用的設計

ExternalTimeline 脈衝時機 + 受體驅動的 Consolidate 觸發，量測 slow 幅度與重測成績隨脈衝時機的曲線；協定名稱 ecdysone_inspired。

#### 明示不宣稱的事項

不是 20E 的定量模型、濃度越高不等於記憶越好、數值是工程數值。未據此設定每細胞濃度與參數（主規格 S14 適用限制）。

#### 對應主規格章節

3.1、11.7、14.5

### [S15] A neural circuit mechanism integrating motivational state with memory expression in Drosophila

#### 原文主張

NPF／多巴胺迴路使內在狀態影響記憶表現，暫時不表現記憶不一定等同遺忘（主規格 3.1；查核範圍：摘要與圖說）。

#### 本專案採用的設計

SetExpressionGain 受體驅動的表現抑制與中性狀態下重評，三次評估間 Parameters、slow、plastic 逐位不變；協定名稱 npf_memory_expression_hypothesis。

#### 明示不宣稱的事項

不是 NPF／多巴胺迴路的定量模型；暫時不表現不等於遺忘。記憶測試需使用固定調節狀態，另設狀態切換測試，避免把暫時抑制表現當成遺忘（主規格 14.5）。

#### 對應主規格章節

3.1、11.7、14.5

### [S20] Proximal Policy Optimization Algorithms

#### 原文主張

以行動回饋學習策略的強化學習演算法：用 GAE 估計優勢並以裁切代理目標更新（主規格 10.1；查核範圍：摘要與方法定位）。

#### 本專案採用的設計

learning/rl 的 GAE、裁切代理目標、價值與熵項、burn-in、終止與時間上限分開、策略版本與初始狀態一致性檢查。

#### 明示不宣稱的事項

不宣稱與論文的實驗結果可比；只做小型 fixture 的可重現改善。行動回饋學習需實作適用的數值方法，不是加上名稱即完成（主規格 3.1）。

#### 對應主規格章節

3.1、10.1、10.5

---

## 命名規則

協定名稱只能有 `ecdysone_inspired` 與 `npf_memory_expression_hypothesis` 兩個。任何要標成 `ecdysone`／`npf`
定量模型的請求，需要 `QuantitativeEvidence{Method, Data, Fit, HeldOutIntervention}` 四項齊全：原研究方法、
原始或可追溯量測資料、參數估計、未參與擬合的干預測試與結果（主規格 11.7），否則拒絕。只讓曲線看起來類似
不夠；資料若不可取得，保留明確的研究限制，而非補造數字（主規格 11.7）。本段是 ticket 26 第三階段的實作契約。