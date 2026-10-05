# 需求文件：重複處置、人工回饋與判定預覽

## 來源與文件定位

- Type: Feature
- Status: Complete
- Owner: 專案維護者
- Source draft: [規劃草稿](../drafts/2026-10-05-11-05_Draft-moderation-feedback-workflow/brief.md)
- 建立時間：2026-10-05 11:14，Asia/Taipei。

本規格接續已完成的 [重複文案窗口](../2026-10-05-10-04_Feature-repeat-content-window/requirements.md)。使用者已要求將三項建議加入 SDD，並以「ok go」授權開始實作。此處針對原本只觀察的重複訊號增加獨立政策，並建立人工回饋與只讀預覽；不重寫既有規則、短窗口、AI provider 或 `/feedspam`、`/purge` 的原有契約。完整探索記錄見來源 [需求草稿](../drafts/2026-10-05-11-05_Draft-moderation-feedback-workflow/requirements.md)。

## 背景、現況與新行為

| 功能 | 現況 | 本規格目標 |
|------|------|------------|
| 同文重複 | 同群同人原文 30 分鐘第 3 則只有 `repeated_content` 訊號 | 可選擇觀察、刪除或封鎖；需清除窗口內已追蹤的相同文案，包括當前訊息 |
| 批次清理 | `/purge` 需管理員手動回覆，查詢近期 48 小時已觀測訊息，最多 100 則 | 自動清理只用觸發時窗口內的精確候選快照，不擴大 `/purge` |
| 人工標註 | `/feedspam` 可建立垃圾樣本／向量，沒有正常樣本修正入口 | `/spam` 標記垃圾並可處置；`/ham` 修正正常標籤並保留稽核 |
| 判定診斷 | 需查日誌，正式 Processor 有計數及處置副作用 | 管理員可只讀預覽規則、行為、AI 狀態與政策結果 |

## 目標與使用情境

1. 管理員能明確啟用重複刪文或封鎖；第 3 則命中時，一併處理已追蹤到的前兩則。
2. 管理員能將漏判標垃圾、修正誤判為正常，確認保存、向量化及處置的各別結果。
3. 管理員能查看某則訊息的判定理由，預覽不增加正式計數或處置。
4. 固定中文廣告與正常反例可回歸驗證內容、重複序列及最終政策。

## 非目標

- 不掃描 Telegram 全部歷史，不跨群、跨人或合併不同原文指紋批刪。
- 不訓練貝氏分類或其他新模型；模型選型留待標註資料及離線量測。
- 不建立網頁後台，不自動解封或將 `ham` 當帳號白名單。
- 不常態保存完整原文，不從指紋推算已刪除訊息文字。
- 不直接修改真實設定、正式資料庫或部署群組。

## 契約決策與假設

- `behavior.repeat_action` 允許 `observe`、`delete`、`ban`，預設 `observe`；ENV 為 `BEHAVIOR_REPEAT_ACTION`，非法值拒絕啟動。
- 重複政策是獨立處置來源，不能替規則加分；僅受全域 `app.mode` 上限限制。既有 AI 模式的相對優先順序保持原樣，另以測試覆蓋來源組合。
- `repeat_action=delete` 不新增一般違規階梯；`ban` 直接封鎖同一成員並保留原因，不把已清除的歷史訊息各算一筆違規。
- `/spam [分類] [delete|ban]` 預設刪文，封鎖須明示；`/ham [原因]` 只修正樣本，不自動解封、撤銷警告或放行規則。
- `/check` 預設僅使用規則與可讀歷史資料；本次先不開即時付費 AI 預覽。回覆須明示 AI 未執行，避免誤認為模型放行。
- 已刪除訊息可用 `/ham event:tg:<update_id> [原因]` 回饋，但事件必須在本群已保存且可查證；無原文時可以修正標籤與稽核，不能重建向量。

## 影響範圍

- 使用者：允許群組中的受檢成員，以及具備即時管理權限的管理員。
- 介面：行為設定、Telegram 管理指令與只讀查詢；舊指令相容。
- 資料：原子候選快照、逐項處置目標、人工標註修正與群組範圍；不加入全文常態保存。
- 發布：需提供 sample／ENV／指令說明及資料結構升級方案，避免以本地測試推論真實群已驗證。

## 驗收情境

### R1：三則精確重複的處置及清理

- 場景：同群同人同原文於第 0、9、18 分鐘發送三個不同訊息 ID。
- 測試：`TestRepeatEnforcementThresholdAndCleanup`、`TestChineseModerationRepeatSequence`。
- 假設：全域 `enforce`，管理員權限與候選資料完整。
- 當：第 3 則進入 `observe`、`delete`、`ban` 三種設定。
- 那麼：`observe` 無直接處置；`delete` 清除當前與前兩則；`ban` 清除三則並封鎖，前兩則未達門檻時不預先處罰。

### R2：隔離、窗口與保護對象

- 場景：候選有其他群、其他人、不同指紋、過窗訊息及受保護目標。
- 測試：`TestRepeatCleanupScopeAndExemptions`、`TestRepeatActionStoreIntegration`；回歸 `TestBehaviorStoreRepeatIsolation`、`TestBehaviorStoreRepeatBoundaryAndExpiry`。
- 假設：單次上限 100 則，含當前訊息。
- 當：建立與執行自動處置計畫。
- 那麼：僅處理同範圍有效 ID；管理員／Bot／信任成員受到保護，授權查詢錯誤不封鎖；截斷時顯示未全清。

### R3：模式來源分離

- 場景：全域 `observe`、`delete-only`、`enforce` 與重複策略交叉，另有規則與 AI 判定。
- 測試：`TestRepeatEnforcementModeMatrix`、`TestRepeatEnforcementThresholdAndCleanup`；回歸 `TestProcessorRepeatedContentAIModes`。
- 假設：全域模式限制本次新增的重複自動處置；既有 AI 契約不順手改寫。
- 當：處理單一觸發訊息。
- 那麼：`observe` 不因重複刪文；`delete-only` 不因重複封鎖；`enforce` 可按明示策略處置。repeat observe 不阻止既有規則或 AI 合法處置；repeat 不滿足規則 ban gate。

### R4：重送、並行、部分失敗

- 場景：相同 update 重送、多副本同時達門檻、某一則 Telegram 刪除失敗或成功後本地回寫失敗。
- 測試：`TestRepeatCleanupRetry`、`TestRepeatActionStoreIntegration` 的並行與租約情境。
- 假設：訊息仍在去重保留期；已持久化的處置目標快照不可變。過窗重送不承諾永久去重。
- 當：重試未完成項目。
- 那麼：不擴大清理範圍、不重建同一筆違規；已確認成功項目不重做；部分成功與未知結果分開記錄。

### R5：垃圾回饋與處置

- 場景：管理員回覆漏判訊息，以 `/spam` 標記並選擇刪除或封鎖。
- 測試：`TestSpamFeedbackActionsAndAuthorization`、`TestSpamFeedbackRechecksProtectionBeforeBan`、`TestWebhookFeedbackAndPreviewEndToEnd`。
- 假設：操作者為即時管理員、目標不受保護且有本群證據。
- 當：樣本寫入、向量化與 Telegram 操作各自成功或失敗。
- 那麼：分別回報保存、向量與處置狀態；樣本無法持久化時不開始新處置；一般成員無法執行。`/feedspam` 保持原本無處置語意。

### R6：正常回饋與修正

- 場景：同一群同一訊息由 spam 改 ham，並行修正或無原文的已刪除訊息。
- 測試：`TestHamFeedbackRevisionAndAuditTarget`、`TestManualFeedbackRevisionAndIsolationIntegration`、`TestAIDetectionProcessorRejectsStaleFeedbackEpoch`。
- 假設：群組範圍、版本與稽核識別可驗證。
- 當：管理員提交修正。
- 那麼：保留修正歷史，舊人工 spam 標記不再是該群有效樣本；他群不受影響；舊快取不使用失效標籤。沒有原文或向量時顯示限制，不自動解封。

### R7：只讀判定預覽

- 場景：管理員回覆訊息使用 `/check`，包含已觀測與未觀測、AI 停用、命中與未命中。
- 測試：`TestDetectionPreviewReadOnlyAndReasons`、`TestCheckCommandUsesReadOnlyPreview`。
- 假設：預覽為現行規則重新計算，歷史狀態另標示查詢時點。
- 當：多次預覽同一訊息。
- 那麼：顯示規則分數／門檻／版本／命中、重複訊號與可知計數、AI 未執行原因、模式和預估動作；未知值標示未知。正式行為、違規、樣本、AI 快取及 Telegram 處置無副作用。

### R8：中文案例回歸

- 場景：洗米、收米、日掙等短廣告，及煮飯、正常收款、引用和重複「收到」。
- 測試：`TestChineseModerationRegressionCorpus`、`TestChineseModerationRepeatSequence`、`TestChineseModerationMockedAIPolicy`。
- 假設：文字判定與明示重複處置各自有預期值，AI 答案以 stub 固定。
- 當：對照單則與第 0、9、18 分鐘序列。
- 那麼：每筆結果可重現；不把正常句因單一黑話硬判為垃圾，也不把明示 repeat ban 的風險藏在內容測試內。

## 驗收條件與驗證需求

- R1～R8 有可執行測試，實際 selector 須回填 [tasks.md](tasks.md)，並以 `go test -list` 核對。
- 執行定向與 `go test -race -count=1 ./...`、`go vet ./...`、格式、lint 與 `git diff --check`；有環境限制則如實註明。
- 正式 Telegram 刪除權限及資料庫 schema 需在獲授權測試環境查驗，未執行不標記為實機通過。
- 完成條件依 T1～T6、V1～V2 的 `Verify:`，不能僅以文件已撰寫判定完成。

## 風險與假設

| 類型 | 內容 | 處理方式 |
|------|------|----------|
| 風險 | 正常短句精確重複仍可能觸發明示 ban | 預設 observe，啟用後觀察誤判並可回退 |
| 風險 | 刪文不可復原或已超 Telegram 可刪期限 | 精確 ID 快照、逐項錯誤、不得回報全成功 |
| 風險 | 樣本修正與現有無群組範圍向量衝突 | 新人工回饋需帶群組與版本，舊向量不可被他群覆寫 |
| 假設 | 原始指紋鍵、時鐘與既有審核資料在服務副本一致 | 維持舊契約，多副本與重送回歸測試 |
