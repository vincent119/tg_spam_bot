# 任務文件：人工垃圾標記批次清除

Status: Complete

## Execution Context

- 意圖：讓 `/spam` 同時清除同群同人相同文案。
- 非目標：自動偵測、AI、跨群跨人及 schema 變更。
- 已定決策：48 小時、最多 100 則、目標優先、先標記與保存計畫、逐則記錄結果。
- 關鍵檔案：command 的 manual_feedback 與 PostgreSQL manual_feedback_action_store。
- 邊界：本 spec 的 design 所列檔案；保留先前尚未提交的規則修改。
- 完成條件：需求驗收、回歸測試、文件更新與差異檢查完成。

## Protected Behavior

- 非管理員、Bot、管理員目標、可信任成員保護。
- `/ham` 無處置、`ban` 明示、完成指令不重做、向量停用可標記。
- `/purge` 現有範圍不變。

## 實作任務

- [x] T1 固定逐訊息處置計畫
  - Boundary: Allowed Changes 為 ports、PostgreSQL action store 及其測試；Forbidden 為 migration、AI、其他資料表。
  - Depends: 無
  - Context: 原目標 delete 鍵保持相容；額外刪除以訊息 ID 區分；再次保存不得擴大目標。
  - Verify: PostgreSQL 計畫固定性與逐訊息結果測試；application、delivery 編譯回歸。
- [x] T2 `/spam` 批次清除
  - Boundary: Allowed Changes 為 manual_feedback.go、handler.go、types.go 與 application、delivery 測試；Forbidden 為自動偵測及 `/ham`、`/purge` 政策修改。
  - Depends: T1
  - Context: 查詢空結果包含目標；去重、100 上限；回覆成功失敗數；結果保存失敗停止。
  - Verify: `TestSpamFeedbackDuplicateCleanup`、`TestSpamFeedbackDuplicateCleanupFailures`、權限與封鎖回歸。
- [x] T3 文件與品質驗證
  - Boundary: Allowed Changes 為 README、本 spec 與上述目標檔案格式；Forbidden 為 unrelated 修改及正式環境部署。
  - Depends: T1、T2
  - Context: 明示僅清除追蹤紀錄、範圍、部分失敗及不需要 AI。
  - Verify: `go test -race -count=1 ./...`、`go vet ./...`、目標套件 lint、`git diff --stat`、`git diff --check`。

## 驗證任務與品質檢查清單

- [x] 驗收情境與 Protected Behavior 已覆蓋。
- [x] 測試與 vet 通過；lint 結果如實記錄。
- [x] 文件與程式範圍一致。
- [x] `git diff --stat`、`git diff --check` 已確認。

## Implementation Notes

- 原分支有已驗證但尚未提交的漏判文案規則，本次保留。
- 使用者已明確要求增加功能，建立規格後直接依任務實作。
- T1、T2 已完成；command、PostgreSQL 與 delivery 套件的 race 測試通過。PostgreSQL 真實整合測試需要 `TEST_DATABASE_URL`，未提供時依既有機制跳過。
- 全量 `go test -race -count=1 ./...`、`go vet ./...` 通過。目標套件 lint 有一項未修改 `delivery/telegram/types.go` 的既有 gocritic 警告；以 `--new-from-rev=HEAD` 檢查本次差異為 0 項。
- `TestManualFeedbackActionAndAuditTargetIntegration` 確認因未設定 `TEST_DATABASE_URL` 跳過；未宣稱真實 PostgreSQL 整合驗證完成。
- 已更新 README 的管理指令表、操作說明及 BotFather 描述；未提交、推送或部署。
