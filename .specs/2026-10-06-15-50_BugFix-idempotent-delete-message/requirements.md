# 需求文件：單筆刪除已不存在訊息的冪等處理

## 來源

- Draft: 無
- Type: BugFix
- Owner: 專案維護者
- Status: Implemented，待部署驗證

## 文件定位

本文件接續 `.specs/2026-10-05-11-14_Feature-moderation-feedback-workflow/` 的重複文案處置流程，只處理單筆 `deleteMessage` 遇到目標訊息已不存在時的結果判定。不重寫重複文案門檻、處置快照、資料庫狀態機或 Webhook 重試策略。

參考來源：

- 需求來源：2026-10-06 正式環境記錄顯示，同一筆重複文案清除步驟因 Telegram 回覆 `Bad Request: message to delete not found` 連續失敗；使用者要求將「訊息已不存在」視為刪除完成。
- 既有文件：上述重複文案處置規格的 R4 與重試設計。
- 既有程式碼：`internal/detection/infra/telegram/client.go`、`internal/detection/application/repeat_enforcement.go`、`internal/detection/delivery/telegram/webhook.go`。

## 背景

重複文案處置會依已保存的目標逐則刪除訊息。若目標先由 Bot 或群組管理員刪掉，Telegram 單筆刪除回覆 400 與 `message to delete not found`。目前此結果被當成步驟失敗，造成原 Webhook 更新回覆失敗並反覆重送；事件中相同步驟已累計 22 次嘗試。

## 問題陳述

刪除目標實際上已不存在，但流程仍把該步驟視為失敗，阻止後續處置完成。重送同一事件不會改變目標已不存在的狀態。

## 目標

1. 單筆 `deleteMessage` 明確收到「訊息已不存在」時回傳成功，讓既有處置流程繼續並記錄步驟完成。
2. 其他 Telegram 錯誤維持失敗，不因文字相似或狀態不符而誤判成功。
3. 保護批次刪除與一般刪除既有行為。

## 非目標

1. 修改其他不可重試錯誤的分類、Webhook 回應策略或重送機制。
2. 主動重播 Telegram 已停止重送的舊更新，或直接修改正式環境的 `repeat_action_steps`。
3. 新增資料庫 migration、設定項或儲存訊息原文。

## 已定決策

- 僅在單筆刪除的 HTTP 狀態為 400、Telegram `error_code` 為 400，且描述為 `Bad Request: message to delete not found` 時，視為刪除目標已達成。
- 對描述允許前後空白及大小寫差異；不以部分字串比對，也不套用到 `deleteMessages`。
- `DeleteMessage` 是共用介面，因此此冪等語義同時適用於重複文案清除與其他單筆刪除呼叫者。

## 待確認項目

- 無。本次程式修補已先完成；本文件依使用者要求事後補建，不宣稱曾於實作前審核。

## 現有行為

Telegram 回覆上述 400 時，`DeleteMessage` 將錯誤向上傳遞。重複文案步驟保存為失敗，Webhook 回覆失敗後可再次收到同一更新。

## 新行為

同一明確回覆被視為單筆刪除完成。既有應用層可繼續執行後續處置，並依正常成功路徑更新步驟狀態；其他錯誤仍向上傳遞。

## 影響範圍

- 使用者：群組管理員及被處置訊息的發送者。
- 功能：所有使用 `Client.DeleteMessage` 的單筆刪除流程，包含重複文案清除。
- API / CLI：不新增對外介面；調整 `DeleteMessage` 的結果語義。
- Data / Storage：不變；沿用既有步驟狀態保存機制。
- 文件 / 安裝 / 發布：部署新版本後，須由新到達或重送的事件觸發處理；無自動回放。

## 使用情境

- 作為群組管理員，我想要已消失的舊垃圾訊息被視為清除完成，以便後續處置不因同一訊息卡住。

## 驗收情境

### 情境：單筆刪除目標已不存在

- 場景：先前清除步驟重試時，目標訊息已不存在。
- 測試：`TestClientDeleteMessageAlreadyMissing/單筆訊息已不存在`
- 假設：Telegram `deleteMessage` 回覆 HTTP 400、`error_code=400`、描述為 `Bad Request: message to delete not found`。
- 當：呼叫 `DeleteMessage`。
- 那麼：回傳 `nil`，上層可按成功步驟繼續。

### 情境：相近但不同的錯誤不得忽略

- 場景：刪除權限不足、訊息無法刪除、伺服器錯誤或 HTTP 與 Telegram 狀態不一致。
- 測試：`TestClientDeleteMessageAlreadyMissing` 的其他單筆案例。
- 假設：Telegram 回覆並非三條件同時符合。
- 當：呼叫 `DeleteMessage`。
- 那麼：保留錯誤，不回傳成功。

### 情境：既有批次刪除不被破壞

- 場景：批次刪除收到相同描述。
- 測試：`TestClientDeleteMessageAlreadyMissing/批次刪除不可套用單筆規則`、`TestClientDeleteMessages`
- 假設：呼叫介面是 `DeleteMessages`。
- 當：Telegram 回覆 400。
- 那麼：批次刪除仍回傳錯誤。

## 驗收條件

1. 上述三類驗收情境均由測試覆蓋且通過。
2. `go test -race -count=1 ./...` 與 `go vet ./...` 通過。
3. 變更不含 migration、設定檔或應用層重試流程修改。

## 驗證需求

- Unit / Integration：`go test -race ./internal/detection/infra/telegram ./internal/detection/application ./internal/detection/delivery/telegram`
- CLI / Dry-run：無。
- 文件檢查：本文件、`design.md`、`tasks.md` 的範圍一致。
- 回歸驗證：`go test -race -count=1 ./...`、`go vet ./...`。

## 風險與假設

| 類型 | 內容 | 處理方式 |
|------|------|----------|
| 風險 | 其他 400 錯誤若仍被上層當成可重試，可能繼續重送。 | 本次僅修正已不存在訊息；另案處理錯誤分類。 |
| 風險 | Telegram 已停止重送的舊更新不會因部署而自動補處理。 | 部署後檢查新事件與既有失敗步驟；必要時另訂安全回復程序。 |
| 假設 | 生產事故中的錯誤描述與記錄一致。 | 以現有日誌為依據，測試固定該回覆形態。 |

## 摘要

- 關鍵決策：僅將單筆刪除的精確「訊息已不存在」回覆視為完成。
- 待確認項目：無。
- 風險：其他永久錯誤與停止重送的舊更新不在本次修補範圍。
- 下一步：檢查程式與測試、部署後觀察步驟狀態。
