# 設計文件：人工垃圾標記批次清除

## 文件定位與已知契約狀態

需求來源為本目錄 `requirements.md`。`DuplicateMessageFinder` 已提供同群、同人、目標指紋、時間窗口與上限的查詢，正式組裝已注入 PostgreSQL store。`FeedbackActionStore` 已提供處置計畫與個別結果保存。`manual_feedback_actions` 以群組、指令更新 ID、kind 唯一索引；有訊息 ID、狀態與錯誤欄位。不得推定未追蹤訊息可以從 Telegram 查出，也不得從指紋還原原文。

## Bounded Context

包含人工 `/spam` 重複訊息清除、逐訊息稽核、回覆與文件。不包含自動規則、AI、向量模型、跨群清除及新的 migration。

## 設計原則與目標流程

1. 沿用標記先保存與既有授權檢查。
2. 以 `now-48h`、同群、目標人、回覆目標 ID 查詢至多 100 則。目標優先，移除無效及重複 ID，總數最多 100。
3. 在執行前保存完整計畫；查詢或保存失敗時不執行刪除。
4. 每則呼叫 `DeleteMessage` 並保存結果。已不存在訊息沿用冪等成功語義；一般單筆失敗繼續其他目標，結果保存失敗則停止並回覆結果未確認。
5. 有 `ban` 時最後處理，保留封鎖前權限保護檢查。

## 資料與介面

`PlanFeedbackActions` 新增可選訊息 ID 列表；`CompleteFeedbackAction` 新增可選單筆刪除 ID。原目標刪除保留 kind=`delete`，其他目標使用 `delete:<message_id>` 作為唯一動作鍵，ban 維持原鍵。沿用既有欄位與索引，不需 migration。重複保存計畫不得擴大或替換既存目標。

沒有注入 finder 的獨立組裝維持單目標刪除；正式環境已有注入。查詢空結果仍包含回覆目標。

## 受影響檔案計畫

- `internal/command/application/manual_feedback.go`：選取清除目標、逐則處置及數量回覆。
- `internal/command/application/ports.go`：擴充計畫與完成介面。
- `internal/command/application/handler.go`、`internal/command/domain/types.go`：指令說明。
- `internal/detection/infra/postgres/manual_feedback_action_store.go`：逐訊息固定計畫與結果。
- 對應 application、PostgreSQL、delivery 測試：更新介面與覆蓋新情境。
- `README.md`：公開指令行為與限制。

## 關鍵行為與風險

單一 `/spam` 標籤仍針對原回覆訊息；批次刪除不建立額外人工標籤。Telegram 處置與資料庫不能共同交易，回寫失敗須明示結果未確認。時間窗口沿用 `/purge` 的偵測紀錄建立時間。成功數代表已確認刪除目標完成，可能包含已不存在訊息。既有指令冪等行為維持，不增加歷史指令重放機制。
