# 任務文件：偵測事件保存發送者與訊息時間

Status: Complete

## Execution Context

- 意圖：讓新偵測事件可查當時 username、first_name、原始發送時間，並可排查 Webhook 延遲。
- 非目標：不改垃圾判定或處置、不回填舊資料、不實作過期更新忽略。
- 已定決策：三個 nullable 欄位；沿用 `ReceivedAt` 承載 Telegram `message.date`。
- 邊界：僅偵測輸入、事件保存、DEBUG 日誌、migration、對應測試及 README。
- 關鍵檔案：`types.go`、`webhook.go`、`store.go`、`detection_events`。
- 完成條件：驗收測試與 Go 回歸測試通過，SQL 範例可查三欄。

### Protected Behavior

- 原有 `user_id`、`created_at`、違規階梯與處置順序。
- 重送冪等及無 username 使用者的正常處理。

### 邊界

#### Allowed Changes

- 本 spec 的三個文件。
- `internal/detection/domain/types.go`
- `internal/detection/delivery/telegram/types.go`、`webhook.go` 及相應測試。
- `internal/detection/application/processor.go` 及相應測試。
- `internal/detection/infra/postgres/store.go` 及相應測試。
- `migrations/*detection_event_message_metadata*.sql`
- `README.md`

#### Forbidden

- 不更動判定規則、違規階梯、Telegram 處置、人工管理指令、其他既有 spec。

## 任務依賴

| 任務 | Depends | 狀態 | 備註 |
|------|---------|------|------|
| 輸入與日誌 | 無 | Complete | 新事件的來源 |
| 儲存與 migration | 輸入與日誌 | Complete | 可查詢 |
| 文件與驗證 | 前兩項 | Complete | 回歸與查詢範例 |
| 日誌發送者欄位 | 輸入與日誌 | Complete | 使用者補充要求 |

## 實作任務

- [x] 傳遞 Telegram 發送者名稱並記錄收件延遲
  - Status: Complete
  - Boundary: Allowed Changes 中的 domain、delivery 檔案；禁止變動處置策略。
  - Depends: 無
  - Context: `message.date` 已進 `ReceivedAt`；`from.username`、`from.first_name` 尚未轉送。
  - Verify: `go test ./internal/detection/delivery/telegram ./internal/detection/domain`

- [x] 保存事件快照與 migration
  - Status: Complete
  - Boundary: Allowed Changes 中的 store、migration、測試；禁止回填舊資料。
  - Depends: 輸入與日誌
  - Context: `toEvent` 同時供觀測及處置事件使用。
  - Verify: `go test ./internal/detection/infra/postgres`

- [x] 更新查詢文件與全量驗證
  - Status: Complete
  - Boundary: Allowed Changes 中的 README 與測試；禁止變更部署流程。
  - Depends: 前兩項
  - Context: SQL 須明確標示台灣時區及兩種時間意義。
  - Verify: `go test ./internal/detection/...`、`git diff --check`

- [x] Webhook 與偵測判定日誌加上發送者識別
  - Status: Complete
  - Boundary: Allowed Changes 中的 `webhook.go`、`processor.go` 及相應測試；禁止記錄 `first_name` 或原文。
  - Depends: 輸入與日誌
  - Context: 使用者要求日誌也直接顯示 username；無 username 時以 `user_id` 關聯。
  - Verify: `go test ./internal/detection/delivery/telegram ./internal/detection/application`、`go test -race -count=1 ./...`、`git diff --check`

## 驗證任務

- [x] 驗收情境覆蓋：名稱快照、缺值、原始時間、處置回歸。
- [x] 品質檢查清單：`gofumpt`、`go vet`、全量 `-race` 測試、文件一致性、`git diff --stat`、`git diff --check`。

## Implementation Notes

- 2026-10-05：舊資料保持 `NULL`；不以目前 `getChatMember` 回覆回填歷史事件。
- 2026-10-05：本機無 `TEST_DATABASE_URL` 且無可用 PostgreSQL，資料庫整合測試自動略過；migration 未對實際資料庫執行。
- 2026-10-05：使用者補充 DEBUG 日誌需顯示 username；為調查用途修改已完成的日誌 task，仍不記錄 first_name 或訊息原文。

## 驗證結果摘要

- 新行為驗證：`TestDomainMessageSenderMetadata`、`TestToEventMessageMetadata`、`TestWebhookUpdateLogFields`、`TestDetectionResultLogFields` 通過；PostgreSQL 整合測試因未設定 `TEST_DATABASE_URL` 而略過。
- 回歸驗證：`go test -race -count=1 ./...`、`go vet ./...` 通過；全量 lint 剩 11 項既有警告，`--new-from-rev=main` 為 0 項。
- 文件一致性：README 查詢範例已更新。
- 剩餘風險：舊事件無法回推原始發送時間；migration 尚未在實際資料庫驗證。

## 後續改善

- [ ] 如需避免歷史更新在封鎖後仍觸發警告，另行設計過期訊息處置門檻；本次僅記錄可觀測資料。
