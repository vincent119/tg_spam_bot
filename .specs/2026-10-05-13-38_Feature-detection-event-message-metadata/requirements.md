# 需求文件：偵測事件保存發送者與訊息時間

## 來源

- Draft: 無
- Type: Feature
- Owner: 待確認
- Status: Complete

## 文件定位

接續使用者對 Telegram 訊息延遲處置的調查。本 spec 僅補足往後偵測事件的發送者快照與原始訊息時間，不重寫垃圾判定、違規階梯、人工指令或 Webhook 投遞機制。

參考來源：使用者對 `detection_events` 查詢的需求；既有 `internal/detection/delivery/telegram/types.go`、`internal/detection/domain/types.go`、`internal/detection/infra/postgres/store.go`。

## 背景與問題

Webhook DTO 已有 `username`、`first_name`、`date`，但偵測事件只存 `user_id` 與判定時間 `created_at`。管理員無法由 SQL 看出事件發生當時的名稱或訊息原始發送時間，也無法計算進站延遲。

## 目標

1. 新偵測事件保存 Telegram 當時提供的 `username`、`first_name`、原始訊息時間。
2. 管理員可用 SQL 對照 `message_sent_at` 與 `created_at`；Webhook 日誌顯示接收延遲、`user_id` 與有提供時的 `username`，偵測判定日誌也可依相同欄位關聯。
3. 舊資料保持可讀且未知欄位為 `NULL`。

## 非目標

- 不回填舊事件的歷史名稱或發送時間。
- 不更改自動刪除、警告、禁言、封鎖或舊更新是否處置的決策。
- 不以使用者現時 Telegram 名稱覆寫事件快照。

## 已定決策

- `username` 可缺省且可能改名，欄位僅為事件當時的快照。
- `first_name` 保存 Telegram 提供的原值；不新增 `last_name`。
- 原始時間使用 `TIMESTAMPTZ` UTC；未知時間保持 `NULL`。

## 待確認項目

- 無；已處理歷史事件的原始發送時間仍無資料來源。

## 現有行為與新行為

- 現有：`detection_events` 有 `user_id`、`created_at`，沒有名稱與原始發送時間。
- 新增：新事件可查 `username`、`first_name`、`message_sent_at`；Webhook 收件日誌可觀察延遲秒數與發送者，偵測判定日誌可直接辨識發送者。

## 影響範圍

- 使用者：管理員與事故調查者。
- 功能：Telegram Webhook 轉換與偵測事件保存。
- API / CLI：無對外 API 變更；新增 SQL 可查欄位。
- Data / Storage：`detection_events` 增加三個可空欄位。
- 文件 / 安裝 / 發布：資料庫 migration 與 README 查詢範例。

## 使用情境

- 作為管理員，我想依 `chat_id`、`message_id` 查當時名稱與發送時間，以確認誤判及延遲。

## 驗收情境

### 情境：新事件含完整快照

- 場景：一般使用者發出文字訊息，Telegram 提供名稱與有效 `date`。
- 測試：`TestDomainMessageSenderMetadata`、`TestToEventMessageMetadata`
- 假設：Webhook 更新包含 `from` 與 `date`。
- 當：事件經轉換並保存。
- 那麼：資料列含原始 `username`、`first_name`、UTC 發送時間；判定時間仍獨立記錄。

### 情境：缺少 username 或時間

- 場景：使用者沒有公開 username，或測試事件沒有有效時間。
- 測試：`TestToEventMessageMetadata`
- 假設：對應值空白或零值。
- 當：保存偵測事件。
- 那麼：未知欄位為 `NULL`，不捏造名稱或 1970 年時間。

### 情境：日誌包含發送者

- 場景：Webhook 收到附帶 username 的群組訊息。
- 測試：`TestWebhookUpdateLogFields`
- 假設：`from.id`、`from.username` 可用。
- 當：記錄接收與判定摘要。
- 那麼：日誌有 `user_id` 與 `username`；沒有 username 時僅記錄 `user_id`，不記錄 `first_name`。

### 情境：既有處置維持不變

- 場景：垃圾訊息在 enforce 模式被判定。
- 測試：`TestProcessorModes`、`TestStoreIntegration`
- 假設：既有規則與模式不變。
- 當：執行處置。
- 那麼：處置種類、違規計數與冪等行為不變。

## 驗收條件與驗證需求

1. 上述測試通過，`go test ./internal/detection/...` 通過。
2. `git diff --check` 通過；migration 可對既有表新增 nullable 欄位。
3. README SQL 範例清楚區分 `message_sent_at` 與 `created_at`。

## 風險與假設

| 類型 | 內容 | 處理方式 |
|------|------|----------|
| 風險 | 名稱屬個人資料且會變更 | 僅存 Telegram 當時回傳值，不將名稱當身分主鍵 |
| 風險 | 舊資料無原始日期 | 可空欄位，不做推測性回填 |
| 假設 | Webhook `date` 為原始訊息時間 | 保留其 UTC 時間戳，與進站時間分開 |

## 摘要

- 關鍵決策：三個可空欄位及收件延遲日誌，不更動處置。
- 待確認項目：無。
- 風險：名稱快照涉及個資、舊事件無法回補。
- 下一步：依 `tasks.md` 實作與驗證。
