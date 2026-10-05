# 需求文件：獨立重複文案偵測窗口

## 來源

- Type: Feature
- Owner: 專案維護者
- Status: Complete
- Draft: 無
- 建立時間：2026-10-05 10:04，Asia/Taipei

## 文件定位

本 spec 接續使用者回報每隔 4～9 分鐘重複發送廣告仍未命中，以及後續「保留 1 分鐘發送頻率、另外新增 30 分鐘重複文案窗口、第 3 則標記可疑」的討論。先建立 SDD 文件，再依使用者「task go」指示完成實作與驗證；Complete 不代表已提交、推送或部署。

範圍是既有垃圾訊息偵測模組的重複觀測與訊號安全邊界，不重寫規則分類、AI classifier、語意記憶、管理員指令、違規階梯或 Telegram adapter。

參考來源：

- 既有規格：`openspec/specs/telegram-spam-detection/spec.md`。
- AI 契約：`openspec/changes/add-ai-spam-detection/specs/ai-spam-detection/spec.md`；以目前程式碼確認實際模式行為，不宣稱該 OpenSpec change 已封存。
- 既有程式碼：`cmd/tg-spam-bot/main.go`、`internal/detection/infra/redis/behavior.go`、`internal/detection/infra/memory/store.go`、`internal/detection/domain/rules.go`、`internal/detection/application/processor.go`、`internal/detection/application/ai_policy.go`、`internal/detection/application/ai_processor.go`。
- 先前已完成的詞彙補強：保留 `configs/rules/money_laundering_recruitment.yaml` 與 `internal/detection/rules/loader_test.go` 的工作區變更；本 spec 不重做或擴大這些修改。

## 背景與問題陳述

變更前的啟動組裝把 Redis 發送頻率、重複內容與跨帳號協同狀態共用同一個 1 分鐘窗口。廣告間隔超過 1 分鐘時，前一則相同內容已離開窗口，無法產生 `repeated_content`。此結論依據儲存庫程式碼，不代表已查驗線上部署版本。

現有 AI 已接受 `repeated_content` 作為弱可疑訊號，不需要新增模型或 prompt。然而變更前規則引擎也接收全部行為訊號；若規則的 `require_any` 引用重複訊號，原先可增加規則分數或滿足封鎖必要條件。新長窗口已將重複訊號隔離出規則評分，避免以重複次數直接處罰。

## 目標

1. 在預設 30 分鐘內，同群組、同使用者、相同原始文案累計第 3 則唯一訊息時產生 `repeated_content`。
2. 發送頻率與跨帳號協同維持原有 1 分鐘設定及現行計數契約。
3. 重複訊號只能提供 AI 與稽核線索，不可替規則加分、滿足封鎖必要條件或自行建立違規。
4. 已保留的同一訊息重送不增加重複計數，也不刷新其首次觀測時間；多副本共享 Redis 時保持原子性。不承諾超過保留期的永久去重。
5. 提供可調整的重複窗口與門檻，沿用既有設定優先順序及安全驗證。

## 非目標

- 不合併語意相近、金額替換、繁簡變體、大小寫或空白不同的文案指紋。
- 不把所有行為窗口都延長為 30 分鐘，也不修正既有 Redis／Memory 協同訊號差異。
- 不變更 AI provider、prompt、快取鍵、信心門檻或處置模式政策。
- 不掃描歷史 Telegram 訊息、不自動回溯刪除、不修改 `/purge`。
- 不修改真實部署設定、秘密值、資料庫 schema 或新增外部套件。
- 不進行 Git 提交、推送、映像發布或線上部署。

## 已確認設計基準

- 設計採前一輪建議的 `30m`、`3` 作為預設；3 則包含當前訊息，不是歷史 3 則加上當前第 4 則。
- 使用處理時刻，而非 Telegram 訊息原始日期；排除首次時間 `<= now - repeat_window` 的紀錄。正常有序觀測為左開窗口；並行計數以原子交易已提交紀錄為準，不承諾相對每個 client 取樣 now 的嚴格右界。
- 「相同文案」沿用原始 `message.Text` 的 HMAC-SHA256 指紋。所有副本必須使用相同內容雜湊金鑰。
- 重複訊號不直接處置，不代表重複訊息一律豁免；明確規則命中或既有 AI 政策允許的結果仍可處理。
- 使用者以「task go」確認進入實作；30 分鐘、3 則是起始政策，不是已完成的誤判率校準結果。

## 現有行為與新行為

| 項目 | 變更前行為 | 新行為 |
|------|----------|--------|
| 發送頻率 | 1 分鐘，歷史同使用者 4 則後的第 5 則產生 `high_frequency` | 不變 |
| 重複內容 | 1 分鐘，第 2 則同內容可產生訊號 | 獨立 30 分鐘，含當前第 3 則唯一訊息起產生訊號 |
| 跨帳號協同 | 沿用各 adapter 現行 1 分鐘邏輯 | 不變，不重新定義成嚴格逐成員滑動窗口 |
| 重送 | Redis member 含每次觀測時間，可能再次累計 | 原始紀錄仍保留時以穩定訊息識別去重，不刷新首次時間；過期後不提供永久去重 |
| 規則評分 | `require_any` 相符訊號可加 20 分 | `repeated_content` 不參與加分或封鎖組合條件 |
| AI 候選 | 低分或弱可疑訊號可進 AI | 保留；零分、非垃圾且有重複訊號也可進 AI |

## 影響範圍與使用情境

- 使用者：允許群組內、通過既有支援與豁免檢查的成員。
- 設定：新增 `behavior.repeat_window`、`behavior.repeat_threshold`；未提供時使用預設。
- 介面：`BehaviorStore.Observe(ctx, message, fingerprint)` 不變；不新增 HTTP、Telegram 指令或 Webhook 欄位。
- 儲存：Redis 新重複 key namespace 與 Memory 獨立重複歷史；不建立資料庫 migration。
- 發布：程式已完成、尚未部署；部署時需同步副本政策並重啟。新計數從上線後逐步累積，不回補舊畫面訊息。
- 作為群組管理者，我希望固定間隔廣告能進入既有 AI 判定，而正常短句重複不會只因計數就受到處罰。

## 驗收情境

### R1：每隔 9 分鐘重複文案

- 場景：同群組、同使用者於第 0、9、18 分鐘發送相同文案，訊息 ID 不同。
- 測試：`TestBehaviorStoreRepeatWindow`、`TestStoreRepeatWindow`。
- 假設：預設政策為 30 分鐘、3 則，歷史未被容量降級丟棄。
- 當：分別觀測三則訊息。
- 那麼：前兩則沒有 `repeated_content`，第 3 則有；三則不因長重複窗口產生 `high_frequency`。

### R2：群組、使用者與內容隔離

- 場景：不同群組、不同使用者或不同內容指紋。
- 測試：`TestBehaviorStoreRepeatIsolation`、`TestStoreRepeatIsolation`。
- 假設：其他範圍已觀測兩則相同內容。
- 當：在任一不同範圍發送一則訊息。
- 那麼：不得借用其他範圍的計數達到重複門檻；變更文案不做語意合併。

### R3：滑動窗口邊界與過期

- 場景：在第 0、15、30 分鐘發送三則；另驗證停止觀測後的 TTL。
- 測試：`TestBehaviorStoreRepeatBoundaryAndExpiry`、`TestStoreRepeatBoundaryAndExpiry`。
- 假設：依序觀測，排除首次時間 `<= now - 30m` 的紀錄；並行情境的上界限制另依 R4 與 design 約定。
- 當：第 30 分鐘觀測第三則。
- 那麼：第 0 分鐘的紀錄排除，只剩兩則，不產生重複訊號；無新觀測超過 TTL 後 Redis key 回收。Memory 的重複窗口使用相同邊界，短窗口契約不變。

### R4：重送與多副本

- 場景：相同 message ID 重送；或 Observe 成功後下游失敗再重試；兩個副本並行觀測。
- 測試：`TestBehaviorStoreRepeatRetryDoesNotRecount`、`TestBehaviorStoreRepeatConcurrent`、`TestBehaviorStoreRepeatLateRetryAndRequestOrdering`、`TestStoreRepeatRetryDoesNotRecount`、`TestStoreRepeatLateRetry`、`TestProcessorRepeatedContentRetryDoesNotRecount`。
- 假設：副本共用 Redis、同一內容雜湊金鑰及相同政策，時鐘同步；去重情境的首次紀錄尚未被窗口、TTL 或容量降級清除。並行第三則測試注入相同取樣時間。
- 當：重送同一則，或並行觀測三則唯一訊息。
- 那麼：仍保留的 MessageID 重送不增加次數、不刷新 score；並行測試按原子交易順序，含當前第 3 則達門檻。未完成訊息若在原紀錄已過期後才重試，可成為新的觀測，不能宣稱 NX 永久記住首次時間。已完成更新仍由既有 UpdateStore 去重。

### R5：短窗口與既有豁免不變

- 場景：1 分鐘內快速發送、跨帳號協同、管理員／可信任成員與已完成更新重送。
- 測試：`TestBehaviorStoreShortWindowRegression`、`TestProcessorRepeatedContentExemption`、`TestProcessorRepeatedContentDuplicate`；既有 `TestStoreObserve`、`TestStoreCoordinatedContentAndExpiry`、`TestProcessorExemption`、`TestProcessorCriticalAndDuplicate`。
- 假設：既有短窗口、豁免、UpdateStore 契約維持不變。
- 當：執行對應流程。
- 那麼：頻率第 5 則契約不變；協同不改成 30 分鐘；豁免與已完成重送不進入行為偵測／AI。

### R6：重複訊號不能替規則加分或滿足封鎖條件

- 場景：以自訂規則明確設定 `require_any: [repeated_content]`。
- 測試：`TestDetectorRepeatedContentObservationOnly`、`TestProcessorRepeatedContentDoesNotAffectRules`。
- 假設：測試同時涵蓋低於門檻的 progressive 規則，以及依賴必要訊號的 critical／ban 規則。
- 當：加入重複訊號後執行偵測。
- 那麼：規則分數不增加，重複訊號不滿足 ban 必要條件；訊號仍保留於結果供 AI 與稽核使用。訊息本身已充分命中的其他規則仍維持原判定。

### R7：既有 AI 安全政策

- 場景：非垃圾、規則零分且含重複訊號。
- 測試：`TestAITriggerPolicyRepeatedContent`、`TestProcessorRepeatedContentAIModes`、`TestProcessorRepeatedContentWithoutAI`；既有 `TestAIDetectionProcessorModes`、`TestAIDetectionProcessorSafetyPolicies`、`TestAIDetectionProcessorSafeDegrade`。
- 假設：依個案設定 AI 停用、正常、uncertain、失敗、低信心或不同模式。
- 當：評估重複訊號。
- 那麼：AI 啟用時符合既有弱可疑候選條件，可使用既有快取；其餘處置沿用已存在的模式／信心政策，重複訊號本身不建立違規或呼叫 Telegram 處置。高信心 spam 仍可在既有允許模式下刪除或走一般階梯。

### R8：設定相容與非法值

- 場景：舊設定沒有 behavior 區塊，或以 YAML／ENV 調整新設定。
- 測試：`TestLoadBehaviorDefaults`、`TestLoadBehaviorEnvironmentOverrides`、`TestValidateBehavior`、`TestNewBehaviorStoreRepeatPolicy`。
- 假設：原有必要設定有效。
- 當：載入預設、合法覆寫、零值、負值或超出資源上限的政策。
- 那麼：未提供新欄位仍可啟動；合法 ENV 優先於 YAML；非法值在啟動與 adapter 建構驗證失敗，不靜默套用危險政策。

### R9：容量與儲存錯誤

- 場景：Memory 重複歷史超過有限容量；Redis 命令失敗。
- 測試：`TestStoreRepeatCapacityDegradesSafely`、`TestBehaviorStoreObserveError`、`TestProcessorRepeatedContentBehaviorError` 與既有處理器錯誤路徑回歸測試。
- 假設：Memory 容量可不足以保留完整窗口。
- 當：超過容量或儲存失敗。
- 那麼：Memory 只減少重複偵測能力，不因容量壓力製造處罰；Redis 錯誤沿用中止處理、允許 Webhook 重送的契約，不假造正常結果。

## 驗收條件與驗證需求

- R1～R9 需有對應測試，並包含原始截圖文案的重複序列；不得把「含當前 3 則」實作為第 4 則。
- Redis 原子操作、窗口／TTL 與跨副本測試使用既有 miniredis 依賴；正式 Redis 支援的交易語意不得由非原子的讀取拼接。
- 新測試名稱已透過 `go test -list` 核對，並執行完整回歸，未以空 selector 當作通過。
- 完成條件以 `tasks.md` 的 T1～T6、V1～V3 的 `Verify:` 為準；包含定向測試、完整 `-race`、格式、vet、lint 與文件一致性。
- 本次已執行完整 `-race`、`go vet`、格式與差異檢查，實際證據與 lint／覆蓋率限制見 [tasks.md](tasks.md) 的驗證結果摘要。

## 風險與假設

| 類型 | 內容 | 處理方式 |
|------|------|----------|
| 風險 | 相似文案交替發送，不一定在單一指紋下達到 3 則 | 明確不承諾語意合併；仍依規則與既有語意／AI 流程判定 |
| 風險 | 長窗口增加 Redis 保留量與 AI 候選量 | 有效範圍驗證、滑動清除、TTL、沿用 AI 快取；先觀察後校準 |
| 風險 | 重複正常短句仍可成為候選 | 不直接加分或處罰，交由既有 AI 安全政策 |
| 風險 | 滾動部署期間新舊副本使用不同窗口／namespace | 發布時同步政策，新 key 冷啟動，不宣稱回補舊歷史 |
| 風險 | 未完成訊息在去重資料過期後延遲重試 | 有限窗口不承諾永久去重；測試明確區分保留期內重送與過期後新觀測 |
| 風險 | client 時間取樣順序與交易提交順序不同 | 原子計數以已提交紀錄為準；不宣稱嚴格右界或已採 server time |
| 假設 | 原始文字指紋與觀測時鐘可跨副本一致 | 不更改 HMAC 契約；確認部署金鑰及時鐘同步，不在文件保存秘密值 |

## 摘要

- 關鍵決策：獨立長重複窗口，保持短窗口，重複訊號只供 AI／稽核。
- 已確認項目：使用者以「task go」授權設計與 30m／3 則起始值進入實作。
- 主要風險：長窗口資源量、精確文案限制、重送誤計及觀察模式誤解。
- 下一步：待使用者另外指示提交或部署；部署前確認副本政策與 AI 模式，上線後觀察資源量與誤判率。
