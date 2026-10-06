# 任務文件：單筆刪除已不存在訊息的冪等處理

Status: Implemented，待部署驗證

## Execution Context

- 意圖：讓 Telegram 單筆刪除的明確「訊息已不存在」回覆視為完成，避免重複處置卡在相同步驟。
- 非目標：不修改其他 400 的重試策略、不回放歷史事件、不新增 migration 或設定。
- 已定決策：僅 HTTP 400、Telegram 400、完整描述相符才轉為成功；批次刪除不變。
- 邊界：只改 Telegram client 與測試，及本目錄 SDD 文件。
- 關鍵檔案：`internal/detection/infra/telegram/client.go`、`internal/detection/infra/telegram/client_test.go`。
- 完成條件：需求文件三類情境通過測試；全量測試及 vet 通過；文件與實作一致。

### Protected Behavior

- 非目標錯誤必須維持錯誤，不得因包含相似描述而被吞掉。
- `DeleteMessages`、正常單筆刪除、上層處置流程維持既有行為。

### 邊界

#### Allowed Changes

- `internal/detection/infra/telegram/client.go`
- `internal/detection/infra/telegram/client_test.go`
- `.specs/2026-10-06-15-50_BugFix-idempotent-delete-message/`

#### Forbidden

- `internal/detection/application/repeat_enforcement.go`、`internal/detection/delivery/telegram/webhook.go` 的流程修改。
- 資料庫 schema、migration、設定與其他處置規則修改。

## 任務依賴

| 任務 | Depends | 狀態 | 備註 |
|------|---------|------|------|
| 單筆刪除冪等判斷 | 無 | Complete | 已先於 SDD 文件完成 |
| 成功及負向案例測試 | 單筆刪除冪等判斷 | Complete | 包含批次刪除保護 |
| 驗證與文件對齊 | 前兩項 | Complete | 尚待正式環境部署觀察 |

## 實作任務

- [x] 單筆刪除冪等判斷
  - Status: Complete
  - Boundary: Allowed Changes 中的 `client.go`；不得修改 Forbidden 範圍。
  - Depends: 無
  - Context: 原有 `APIError` 未保存 HTTP 狀態；要同時確認 HTTP 與 Telegram 錯誤碼。
  - Verify:
    - `go test -race ./internal/detection/infra/telegram ./internal/detection/application ./internal/detection/delivery/telegram`

- [x] 成功及負向案例測試
  - Status: Complete
  - Boundary: Allowed Changes 中的 `client_test.go`；不得修改 Forbidden 範圍。
  - Depends: 單筆刪除冪等判斷
  - Context: 測試精確回覆、其他 400、403、5xx、HTTP/Telegram 狀態不一致及批次刪除。
  - Verify:
    - `go test -race ./internal/detection/infra/telegram ./internal/detection/application ./internal/detection/delivery/telegram`

- [x] 驗證與文件對齊
  - Status: Complete
  - Boundary: Allowed Changes；不得修改 Forbidden 範圍。
  - Depends: 前兩項
  - Context: 驗收以本目錄需求及設計文件為準；記錄先實作後補文件的順序。
  - Verify:
    - `make test`
    - `make vet`
    - `golangci-lint run ./internal/detection/infra/telegram`
    - `git diff --check`

## 驗證任務

- [x] 驗收情境覆蓋
  - Verify: `TestClientDeleteMessageAlreadyMissing` 涵蓋精確成功、相近錯誤與批次刪除。

- [x] 回歸驗證
  - Verify: `make test`、`make vet` 已通過。

- [x] 品質檢查清單
  - 格式檢查通過：已執行 `gofmt`。
  - 測試通過：`make test` 已通過。
  - 文件一致性已確認：三份 SDD 文件同屬單筆刪除邊界。
  - 主要驗收情境已覆蓋。
  - Protected Behavior 回歸驗證通過。
  - 風險項目已記錄；正式環境部署觀察尚未執行。
  - `git diff --stat` 已檢查。
  - `git diff --check` 已通過。

## 實作中斷恢復

恢復時先讀本文件的 `Execution Context`、未完成任務、`Protected Behavior` 與 `Implementation Notes`，不需掃描整個 `.specs` 目錄。

## Implementation Notes

- 2026-10-06：程式修補先於 SDD 文件；本文件依使用者要求事後補建，不宣稱已事前審核。
- 正式環境觀察到同一單筆刪除因訊息已不存在而累積 22 次失敗；根因在單筆刪除回傳錯誤，讓既有上層流程無法前進。
- 全量 `make lint` 仍有 11 項未修改舊程式的警告；本次目標 package 執行 `golangci-lint run ./internal/detection/infra/telegram` 為 0 項。
- 未連線正式環境，也未直接修正舊資料列；部署後須觀察新更新與步驟狀態。

## 驗證結果摘要

- 新行為驗證：通過，`go test -race ./internal/detection/infra/telegram ./internal/detection/application ./internal/detection/delivery/telegram`。
- 回歸驗證：通過，`make test`、`make vet`；全量 lint 因 11 項既有警告未通過。
- 文件一致性：已確認。
- 剩餘風險：其他不可重試錯誤及已停止重送的舊更新未處理；尚待正式環境部署驗證。

## 後續改善

- [ ] 另案釐清其他永久性 Telegram 4xx 錯誤的步驟狀態與 Webhook 回覆策略。
- [ ] 部署後確認已不存在訊息的重複處置能完成，且不再對同一步驟持續重試。
