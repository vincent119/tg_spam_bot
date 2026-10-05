# 任務文件：重複處置、人工回饋與判定預覽

Status: Complete

## Execution Context

- 意圖：完成需求 R1～R8 的三項功能與驗證，來源為已升格的 [草稿](../drafts/2026-10-05-11-05_Draft-moderation-feedback-workflow/brief.md)。
- 非目標：貝氏分類、網頁管理介面、語意近似重複、正式部署與真實群操作。
- 已定決策：repeat 預設 observe；只有明確設定 delete／ban 會觸發獨立政策，受全域 app.mode 上限；`/spam` 預設刪文、ban 明示；`/ham` 不自動解封；`/check` 只讀且不即時呼叫 AI。
- 關鍵檔案：config、behavior Redis／Memory、detection processor／PostgreSQL、command domain／handler、manual sample／embedding、preview、main／DI 及測試。
- 完成條件：R1～R8 全有實際測試並記錄驗證；不以空 selector 或未執行的整合／實群測試宣稱通過。

### Protected Behavior

- 保留舊 Complete spec、1 分鐘頻率／協同、原文 HMAC、同群同人隔離、豁免及 update 冪等。
- `repeated_content` 不替規則加分或滿足 ban gate；未設定 repeat_action 時不直接處置。
- `/feedspam` 不刪文；`/purge` 仍只按其原有人工範圍；既有 AI mode 相對優先順序不順手改寫。
- 不常態保存原文；不得因 `/ham` 自動解封、清警告或豁免帳號。

### 全域 Boundary

#### Allowed Changes

- `internal/config/`、`configs/config.sample.yaml`、`.env.example`、`cmd/tg-spam-bot/main.go`。
- `internal/detection/application/`、`internal/detection/infra/{redis,memory,postgres,telegram}/`、`internal/detection/delivery/telegram/`、必要的 `internal/detection/di.go`。
- `internal/command/{domain,application}/`、必要的 `migrations/`、相關 `*_test.go`／`testdata/`，及本 spec 文件。
- 來源 draft 僅做 promotion 狀態與正式 spec 連結修改，之後不再改歷史文字。

#### Forbidden

- 真實 `configs/config.yaml`、秘密值、正式資料庫／群組、AI provider／prompt、新 Web UI、autoreply、OpenSpec 歷史。
- 未經明確要求的 commit、push、發布及不相關程式整理。

## 任務依賴

| 任務 | Depends | 狀態 |
|------|---------|------|
| T0 草稿升格與契約固定 | 使用者 `ok go` | Complete |
| T1 重複快照與設定 | T0 | Complete |
| T2 自動處置與前文清理 | T1 | Complete |
| T3 人工修正資料與作用範圍 | T0 | Complete |
| T4 `/spam`、`/ham` 指令 | T3 | Complete |
| T5 `/check` 只讀預覽 | T0 | Complete |
| T6 中文回歸案例 | T0；整合驗證依賴 T1～T5 | Complete |
| V1 測試與安全回歸 | T1～T6 | Complete |
| V2 文件、品質與交付 | V1 | Complete |

## 實作任務

### T0：升格與契約

- [x] Status: Complete
- Boundary:
  - Allowed Changes：本正式 spec 三檔及來源 draft promotion 標記。
  - Forbidden：擴大已完成窗口規格與任何真實外部系統操作。
- Depends: 使用者 `ok go`。
- Context: 依現有程式固定保守預設、指令語法、mode 來源隔離及只讀預覽；樣本群組 scope 與逐項持久化在 design 中規範。
- Verify: 本三檔含 R1～R8、受影響範圍及每個 task 邊界；來源 draft 可回溯；`git diff --check`。

### T1：設定與原子候選快照

- [x] Status: Complete
- Boundary:
  - Allowed Changes：config／sample／ENV、detection behavior port、新 repeat policy、Redis／Memory adapters、必要 main 組裝及測試。
  - Forbidden：短窗口契約、AI provider、Telegram 處置及真實設定。
- Depends: T0。
- Context: 結構化 observation 保留舊訊號，提供同群同人同 fingerprint 的 count、窗口與最多 100 個 ID；多副本交易與去重語意維持。
- Verify: 新設定與原子快照定向測試；回歸 `TestBehaviorStoreRepeatWindow`、`TestBehaviorStoreRepeatIsolation`、`TestBehaviorStoreRepeatBoundaryAndExpiry`、`TestBehaviorStoreRepeatConcurrent` 與 Memory 對等測試；`git diff --check`。

### T2：達門檻自動處置與前文清理

- [x] Status: Complete
- Boundary:
  - Allowed Changes：detection policy／processor／ports、PostgreSQL action store、必要 migration、Telegram client 與測試。
  - Forbidden：跨群或跨人清理、以重複訊號偽造規則命中、擴大 `/purge`。
- Depends: T1。
- Context: 觸發時計畫包含已追蹤的前文與當前訊息；先持久化固定清單再執行，成功項目不重做，部分失敗分別記錄。
- Verify: R1～R4 對應新 selector；既有 policy／processor／Telegram adapter／`/purge` 回歸；隔離 DB 的 schema 驗證；`git diff --check`。

### T3：人工樣本修正與隔離

- [x] Status: Complete
- Boundary:
  - Allowed Changes：manual sample service／port／store、人工向量與有效 revision 查詢、必要 migration、相關測試。
  - Forbidden：改 AI provider／prompt、覆寫他群共享向量、常態保存完整原文。
- Depends: T0。
- Context: 同群同訊息修正有版本與稽核；舊向量或快取不能在修正後當有效人工證據；`/feedspam` 保持相容。
- Verify: R6 並行／跨群／舊工作回寫測試；`/feedspam`、AI／語意舊測試；隔離 DB schema 回歸；`git diff --check`。

### T4：人工標記與處置指令

- [x] Status: Complete
- Boundary:
  - Allowed Changes：command domain／handler／ports、Telegram delivery、必要指令稽核、DI／main 與測試。
  - Forbidden：略過即時管理員授權、一般成員投樣本、`/ham` 自動解封或修改 `/feedspam` 原本副作用。
- Depends: T3。
- Context: 保存失敗不開始新處置；向量與處置結果分開回報；已刪除目標只接受可驗證本群稽核資料，不還原原文。
- Verify: R5～R6 指令與 E2E 測試；回歸 `/feedspam`、`/ban`、`/unban`、`/purge` 及匿名管理員拒絕；`git diff --check`。

### T5：只讀判定預覽

- [x] Status: Complete
- Boundary:
  - Allowed Changes：新 preview service／只讀 ports、command `/check`、必要唯讀 store 查詢、DI／main、測試。
  - Forbidden：正式 Processor／Observe 寫入、未明示的 AI 呼叫、Web UI、處置副作用。
- Depends: T0。
- Context: 目前規則重新計算與當時已存事件分開顯示；未知資料明示，輸出截斷且不洩漏他群資料。
- Verify: R7 的正常／垃圾／未觀測／AI停用／豁免測試；以 spy 確認正式行為、違規與 Telegram 處置零呼叫；回歸指令授權與限流；`git diff --check`。

### T6：中文回歸資料

- [x] Status: Complete
- Boundary:
  - Allowed Changes：相關 `*_test.go`／`testdata/`、本 spec 驗證記錄。
  - Forbidden：為測試方便擴大正式規則硬封鎖、使用真人資料、單元測試直接呼叫付費 AI。
- Depends: T0；整合結果依賴 T1～T5。
- Context: 使用者六組短廣告與正常煮飯／收款／引用／重複短句；拆分單則內容、0／9／18 分鐘及政策最終動作預期。
- Verify: R8 table-driven 測試及既有 normalize、rule loader、quote／signal 回歸；不宣稱線上準確率；`git diff --check`。

## 驗證任務

### V1：測試與安全回歸

- [x] Status: Complete
- Boundary:
  - Allowed Changes：核准範圍內的測試與本 spec 證據。
  - Forbidden：對真實群／正式 DB 測試、把 Skip 當通過。
- Depends: T1～T6。
- Context: 依 R1～R8 查詢實際 selector，並跑定向與完整回歸。
- Verify: `go test -list .` 核對新 selector；`go test -race -count=1 ./...`；記錄 `TEST_DATABASE_URL` 是否提供與整合測試是否 Skip；模式、權限、重送、跨群、限流；`git diff --check`。

### V2：品質與交付

- [x] Status: Complete
- Boundary:
  - Allowed Changes：格式、lint 修正僅限本次變更、sample／ENV／指令文件與 spec 結果。
  - Forbidden：不相關重構、commit／push／部署。
- Depends: V1。
- Context: 未查驗真實 Telegram 與資料庫不得寫成成功；原有 lint 限制與新增問題分開。
- Verify: gofmt、`go vet ./...`、`golangci-lint run ./...`、受影響 package 覆蓋率、sample／ENV／指令一致、`git diff --stat`、`git diff --check`、文件連結。

## Implementation Notes

- 2026-10-05：由使用者「ok go」進入實作，既有分支 `docs/moderation-workflow-sdd` 承接未追蹤來源 draft。
- 程式基準 `main@2e16115`；現有 AutoMigrate 與 AI 模式優先序先維持，資料變更需版本化 SQL 與隔離環境驗證。
- T1 已加入設定、Redis／Memory 原子候選與唯讀查詢；T2 使用固定快照、跨事件刪文步驟與租約 fencing；T3／T4 增加群組範圍可修正標記、epoch、`/spam`、`/ham`；T5／T6 增加 `/check` 只讀預覽與中文案例。
- `go test -list` 已核對 R1～R8 的實際 selector，詳見 [需求驗收情境](requirements.md)。最終 `go test -race -count=1 ./...`、`go vet ./...`、`golangci-lint run --new-from-rev=HEAD ./...`（0 issues）與 `git diff --check` 通過；受影響套件 coverage：detection/application 82.9%、command/application 80.0%、detection/infra/redis 91.1%。
- 全量 `golangci-lint run ./...` 仍有 11 項未修改舊程式的既有問題（gocritic 1、gofumpt 1、gosec 4、revive 5）；未將其誤列為本次通過。PostgreSQL 測試在一般回歸因 `TEST_DATABASE_URL` 未設而 Skip，但各組 migration up/down、AutoMigrate 相容與 store 整合已在一次性隔離 PostgreSQL 實跑通過。
- 2026-10-05：補正 [README 管理指令與設定](../../README.md#telegram-管理指令) 及設計契約，列明 `/spam`、`/ham`、`/check` 用法、群組範圍、預覽限制、BotFather 清單與重複處置設定；這是文件補正，未重新宣稱程式驗證或線上部署。
- 未操作正式 Telegram 群組或資料庫；真實 Bot 刪文／封鎖權限、正式 migration 排程與回退仍須部署者於獲授權環境確認。未 commit、push 或部署。
