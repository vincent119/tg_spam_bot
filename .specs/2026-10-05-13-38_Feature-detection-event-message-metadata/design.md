# 設計文件：偵測事件保存發送者與訊息時間

## 設計摘要

在 Telegram Update 轉換時保留 `from.username`、`from.first_name`；沿用目前由 `message.date` 轉成 UTC 的 `ReceivedAt`，於偵測事件持久化時命名為 `message_sent_at`。`created_at` 仍是判定時間，不改處置流程。新欄位可空，舊資料不回填。

## 文件定位與已知契約狀態

- 需求來源：同目錄 `requirements.md`。
- API / CLI / Hook contract：現有 Telegram `message` Webhook；無新增 endpoint。
- Data contract：`detection_events` 目前有 `user_id`、`created_at`，無名稱或原始日期。
- 既有實作：`Update.DomainMessage`、`Processor.Process`、`toEvent`、`AutoMigrate`。
- 不可假造：舊事件的 Telegram `message.date` 與當時 username。
- 不重寫：規則引擎、違規計數、動作規劃與 Telegram API 處置。

## Bounded Context

包含：Webhook 輸入轉換、偵測領域訊息、偵測事件 PostgreSQL 儲存及事故查詢文件。

不包含：Telegram 遞送、CloudFront、Nginx、舊訊息補抓、過期事件處置策略。

## 設計原則與需求對應

- `user_id` 繼續是穩定識別；名稱只做快照與查詢輔助。
- `message_sent_at` 與 `created_at` 明確區分；無效日期為 `NULL`。
- 舊資料與無 username 使用者均可保存；不修改處置結果。

| 需求 | 設計 | 驗證 |
|------|------|------|
| 發送者快照 | DTO 複製名稱至領域訊息，`toEvent` 寫可空欄位 | DTO、`toEvent` 測試 |
| 時間差 | 儲存 UTC 原始日期，Webhook 記錄延遲秒數 | DTO、`toEvent` 測試與日誌檢查 |
| 相容性 | nullable migration 與 GORM model 同步 | 整合測試、SQL 檢查 |

## 受影響檔案計畫

| 檔案 | 預期變更 | 風險 |
|------|----------|------|
| `internal/detection/delivery/telegram/types.go` | 轉送名稱 | 個資流入事件 |
| `internal/detection/delivery/telegram/webhook.go` | 記錄原始時間、延遲與發送者識別 | 日誌個資 |
| `internal/detection/application/processor.go` | 判定摘要記錄發送者識別 | 日誌個資 |
| `internal/detection/domain/types.go` | 增加名稱欄位 | 領域資料面擴大 |
| `internal/detection/infra/postgres/store.go` | GORM 欄位與映射 | schema 需同步 |
| `migrations/*detection_event_message_metadata*.sql` | nullable up/down | 舊資料不可回填 |
| `internal/detection/**/*_test.go` | 映射與回歸測試 | 無 |
| `README.md` | 查詢範例與時間語意 | 無 |

## 目標流程與資料契約

Telegram `message.from` 與 `message.date` → `domain.Message` → `application.Event` → `detection_events`。Webhook 收到時，以應用程式目前時間減 `message.date` 記錄 `delivery_lag_seconds`；負值保留供時鐘偏差排查。

新增 `detection_events.username TEXT NULL`、`first_name TEXT NULL`、`message_sent_at TIMESTAMPTZ NULL`。空名稱與零值時間映射為 `NULL`。原有事件不更新。Webhook 與偵測 DEBUG 日誌僅記錄 `user_id` 及有提供時的 `username`，不記錄 `first_name` 或訊息原文；`user_id` 仍是主識別。

## Protected Behavior

- `user_id`、`created_at` 原有語意與違規動作不變。
- Webhook 支援沒有 username 的使用者及 caption 訊息。
- 舊事件、重送事件仍可查詢且維持冪等。

## 替代方案與風險

| 方案 | 優點 | 缺點 | 結論 |
|------|------|------|------|
| 查詢時呼叫 `getChatMember` | 不增加儲存 | 只能取得現在的名稱，可能已改名或離群 | 不採用於事件稽核 |
| 保存快照 | 可追溯當時資訊 | 增加個資儲存 | 採用，限制欄位與用途 |

## 實作注意事項

- 專案現有 `AutoMigrate` 仍負責啟動 schema 同步；版本化 migration 保留供之後 runner 使用，SQL 使用 `IF NOT EXISTS` 避免重複新增。
- `ReceivedAt` 目前實際承載 Telegram 原始 `message.date`；本次不變更既有欄位名稱，以縮小範圍。
- 舊事件的發送時間無法從此變更回推。
