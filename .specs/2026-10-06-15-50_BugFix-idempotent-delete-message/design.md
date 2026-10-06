# 設計文件：單筆刪除已不存在訊息的冪等處理

## 設計摘要

在 Telegram 基礎設施層的 `DeleteMessage` 邊界判斷特定 API 回覆，而不修改上層處置狀態機。此回覆代表刪除後的目標狀態已成立，因此回傳成功；其餘錯誤原樣傳遞。透過同時檢查 HTTP 狀態、Telegram 錯誤碼和完整描述縮小誤判範圍。

## 文件定位

對應本目錄的 `requirements.md`，接續 `.specs/2026-10-05-11-14_Feature-moderation-feedback-workflow/` 的重複處置設計。只調整 Telegram 單筆刪除結果語義，不重寫重複文案規則、應用層步驟快照、Webhook、migration 或批次刪除。

## 已知契約狀態

- 需求來源：`requirements.md` 的「目標」及三個驗收情境；2026-10-06 正式環境回報。
- API / CLI / Hook contract：內部 `DeleteMessage(ctx, chatID, messageID) error`；Telegram 單筆刪除失敗回覆包含 HTTP 狀態、`error_code`、`description`。
- Data contract：現有 `repeat_action_steps` 保存步驟結果；本次不變更結構。
- 既有實作：`Client.callResult` 解析 Telegram 回應；`executeRepeat` 按 `DeleteMessage` 的錯誤決定成功或失敗；Webhook 依處理結果回覆。
- 不可假造：不能推定任意 400 都表示訊息已刪除，也不能推定過往失敗事件一定會再次投遞。

## Bounded Context

包含：

- Telegram 單筆刪除對「目標已不存在」的冪等判斷。
- 單筆與批次刪除的回歸測試。

不包含：

- Telegram 更新重送、重複內容偵測、資料庫步驟重放及正式環境人工補償。

## 設計原則

- 在外部 API 邊界轉換結果，不讓應用層依賴 Telegram 原始錯誤字串。
- 僅匹配已觀測到的完整回覆形態；未知錯誤保持失敗。
- 不新增狀態、設定或 schema；沿用既有成功路徑。

## 需求對應

| 需求 / 驗收情境 | 設計處理方式 | 驗證方式 |
|-----------------|--------------|----------|
| 已不存在訊息視為完成 | `DeleteMessage` 對三條件相符的 `APIError` 回傳 `nil` | `TestClientDeleteMessageAlreadyMissing` |
| 其他錯誤不誤判 | HTTP、Telegram 碼及完整描述需同時符合 | 同上各負向案例 |
| 批次刪除不變 | 不修改 `DeleteMessages` 的錯誤傳遞 | `TestClientDeleteMessages` 及新增批次案例 |

## 受影響檔案計畫

| 檔案 | 預期變更 | 原因 | 風險 |
|------|----------|------|------|
| `internal/detection/infra/telegram/client.go` | `APIError` 保留 HTTP 狀態；`DeleteMessage` 判斷目標已不存在 | 精確分辨單筆刪除結果 | 共用單筆刪除呼叫者皆採新語義 |
| `internal/detection/infra/telegram/client_test.go` | 加入成功及負向表格測試 | 防止過度吞錯 | 無外部依賴 |

## 目標結構或流程

`DeleteMessage` 呼叫原有 `call`。`callResult` 在錯誤物件中保留 HTTP 狀態，並繼續使用原本的 Telegram 錯誤碼與已遮蔽描述。只有錯誤型別為 `APIError` 且 HTTP 400、Telegram 400、描述完整符合時，`DeleteMessage` 回傳 `nil`。`executeRepeat` 無須修改，會依原成功路徑標記步驟並繼續後續動作；其他情況仍沿原錯誤路徑處理。

## Mermaid Diagrams

不需要；此變更是單一 API 邊界判斷，流程已於上節說明。

## 介面與資料契約

### API / CLI / Hook

- Input：`DeleteMessage(ctx, chatID, messageID)`。
- Output：目標已不存在的明確回覆與正常刪除成功皆為 `nil`。
- Error：其他 API 錯誤、網路錯誤、解碼錯誤維持原有錯誤。

### Data / Config

- 新增資料：無。
- 既有資料相容性：無 migration；舊失敗步驟只有在相同處置重新執行時才可能進入新的成功路徑。

## 關鍵行為

- 比對前修剪描述前後空白並忽略大小寫，但要求完整字串一致。
- HTTP 狀態與 Telegram `error_code` 必須都是 400；不以其中一項替代另一項。
- `DeleteMessages` 不套用單筆規則。

## 前後端或跨模組設計

基礎設施層將目標已不存在解讀為單筆刪除完成。應用層的 `executeRepeat`、步驟持久化與 Webhook 回應流程不變；成功是否能寫入步驟仍由既有流程負責。

## Protected Behavior

- 刪除權限不足、無法刪除、5xx 及 HTTP/Telegram 狀態不一致仍回傳錯誤。
- 一般成功刪除與批次刪除行為維持不變。
- 錯誤描述對外仍依既有遮蔽邏輯處理。

## 替代方案

| 方案 | 優點 | 缺點 | 結論 |
|------|------|------|------|
| 在 `DeleteMessage` 邊界轉為成功 | 範圍小、所有單筆刪除一致 | 共用呼叫者皆受影響 | 採用；符合冪等刪除語義 |
| 在重複處置流程比對錯誤字串 | 只影響重複處置 | 應用層耦合 Telegram 回覆，其他單筆刪除仍有相同問題 | 不採用 |
| 所有 400 都當成功 | 實作簡單 | 會吞掉權限及其他真實失敗 | 不採用 |

## 風險與處理方式

| 風險 | 影響 | 處理方式 | 驗證 |
|------|------|----------|------|
| 錯誤回覆誤判 | 真正刪除失敗卻標記完成 | 三條件完整匹配；負向測試 | `TestClientDeleteMessageAlreadyMissing` |
| 其他不可重試 400 持續重送 | 仍可能卡住 Webhook | 留待獨立改善，不擴大本次修補 | 監看部署後錯誤記錄 |
| 舊失敗事件不再重送 | 舊步驟不會自動完成 | 不宣稱部署可回補歷史資料；另訂人工修復程序 | 部署後查步驟狀態 |

## 實作注意事項

- 本設計是程式修補後依使用者要求補建的規格，任務狀態須如實記錄。
- 無須修改 `repeat_enforcement.go`、`webhook.go`、資料庫 schema 或設定。
