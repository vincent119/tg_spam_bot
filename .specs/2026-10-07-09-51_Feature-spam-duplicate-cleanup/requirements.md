# 需求文件：人工垃圾標記批次清除

## 文件定位

接續 `.specs/2026-10-05-11-14_Feature-moderation-feedback-workflow/` 的 `/spam` 人工標記與處置流程，沿用 `/purge` 的查詢範圍與既有逐項稽核。不改寫自動偵測、語意記憶或分數政策。

## 背景與現有行為

目前 `/spam` 保存本群垃圾標記後，只刪除回覆的訊息。管理員需另外使用 `/purge` 才能清掉已追蹤的重複文案。

## 目標與新行為

管理員回覆訊息執行 `/spam [分類] [delete|ban]` 後，先保存人工標籤，再清除同群、同人、相同內容指紋的近期訊息。沿用 48 小時窗口，包含回覆目標且最多 100 則。只查詢既有偵測紀錄，不取得 Telegram 歷史訊息。未追蹤到相同訊息時，仍刪除回覆目標。無需 AI；向量停用或向量化失敗不影響刪除。

## 非目標

- 不跨群、不跨發送者、不刪除相似但不同的文案。
- 不變更規則分數、門檻、自動處置、`/ham` 或 `/purge`。
- 不新增 migration，不自動補償已完成的舊指令。

## 影響範圍與使用情境

管理員可用一次 `/spam` 完成標記及重複訊息清除。`ban` 仍須明示，且所有處置保留管理員、Bot、可信任成員保護。刪除目標在 Telegram 呼叫前持久化，各訊息分別記錄結果。

## 驗收情境

### 同群同人重複文案

- 場景：目標與多則相同文案已被追蹤。
- 測試：`TestSpamFeedbackDuplicateCleanup`
- 假設：查詢回傳同群同人的相同指紋訊息 ID。
- 當：管理員使用 `/spam`。
- 那麼：標記先保存，目標與重複文案去重後刪除，回覆清除數量。

### 空結果、上限及異常

- 場景：未追蹤目標、重複 ID、查詢失敗或單筆刪除失敗。
- 測試：`TestSpamFeedbackDuplicateCleanup`、`TestSpamFeedbackDuplicateCleanupFailures`
- 假設：查詢或 Telegram 回覆不同結果。
- 當：執行 `/spam`。
- 那麼：空結果仍刪目標；最多 100 則且目標優先；查詢失敗回覆標記已保存但未處置；單筆失敗不阻止其他刪除，稽核與回覆反映部分失敗。

### 權限與封鎖

- 場景：無權限、受保護目標、`/spam ban` 或重送指令。
- 測試：`TestSpamFeedbackActionsAndAuthorization`、`TestSpamFeedbackRechecksProtectionBeforeBan`、`TestHandlerReturnsExistingResultWithoutRepeatingEffect`
- 假設：沿用現有權限及冪等機制。
- 當：執行或重送指令。
- 那麼：權限不足不處置；明示 ban 才封鎖；封鎖前再次檢查保護；已完成指令不重做。

## 驗收條件與驗證需求

- 上述情境與既有 `/ham`、`/purge` 測試通過。
- `go test -race -count=1 ./...`、`go vet ./...` 與變更套件 lint；記錄既有警告及環境限制。
- README 與指令說明列明範圍、上限與部分失敗回覆。
- 正式環境未部署，本地驗證不得宣稱線上完成。
