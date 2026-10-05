# 需求文件：版本化資料庫 Migration Runner

Status: Complete

## 文件定位與來源

本規格接續已完成的[重複處置與人工回饋 SDD](../2026-10-05-11-14_Feature-moderation-feedback-workflow/requirements.md)，處理其 `11400` 至 `11800` 版本化 SQL 原本不會自動執行、也沒有逐版日誌的交付缺口。先前僅建立 SDD；使用者後續授權依任務實作。本次只操作一次性隔離資料庫，未修改正式資料庫。

後續 `main` 已合併[偵測事件發送者資料 SDD](../2026-10-05-13-38_Feature-detection-event-message-metadata/requirements.md)，新增 `20261005133800` SQL。本規格的交付範圍因此加入該版本的向前遷移與結構契約，但不修改已發布的 SQL 或發送者資料行為。

既有偵測規則、`/spam`、`/ham`、`/check`、資料內容與 Telegram 處置契約均不在本規格重寫範圍。以[目前的啟動流程](../../cmd/tg-spam-bot/main.go)與[部署說明](../../README.md#postgresql-初始化)作為現況依據。

## 背景與問題

目前 Bot 啟動時由 GORM `AutoMigrate` 同步結構，成功後只記錄一筆「資料庫結構同步完成」。`migrations/` 中五組 `up/down.sql` 不會被啟動流程執行，沒有版本、checksum、執行結果或逐版日誌。單看成功訊息，無法判斷哪個 SQL 已套用。

直接在空資料庫依檔名跑全部 SQL 也不可行：`11800` 依賴先有 `ai_detection_events`，`11500` 依賴 pgvector；目前語意記憶預設關閉，不應因此強制所有部署安裝 pgvector。

## 目標

1. 提供獨立於 Bot 主程式的部署前 runner，依已審核版本執行待套用的 `up.sql`，成功才允許新版 Bot 啟動。
2. 可查詢每個 migration stream 的版本、checksum、套用時間、來源（`applied` 或 `adopted`）與失敗狀態；重跑不重複執行已完成版本，檔案遭修改時拒絕繼續。
3. 每版有開始、成功、失敗的結構化日誌，失敗時非零退出且不宣稱結構同步完成。
4. 支援全新資料庫與既有 `AutoMigrate` 資料庫接管，保留語意記憶可選的現有部署契約。

## 非目標

- 不在 Bot 的 `main` 啟動路徑中偷偷執行版本化 SQL。
- 不自動執行 `down.sql` 或以刪表回退；既有 `down.sql` 可能造成資料遺失。
- 不修改垃圾訊息偵測、人工標籤或 Telegram 管理指令的業務規則。
- 不宣稱已在正式資料庫或真實群組驗證。

## 已定決策與待確認

- 依專案規範，採部署前一次性命令或 job；Bot 啟動時驗證必要 schema 版本、clean 狀態與本發行版 checksum，不再自行做未記錄版本的 DDL。Compose 部署須以 runner 成功作為 app 啟動門檻。
- migration 只自動向前執行。正式回退以備份還原、相容舊版應用與明示人工程序處理。
- 保留 `semantic_memory.enabled=false` 不需 pgvector 的契約。核心與語意 migration 必須有獨立進度，語意未啟用時不能把 `11500` 誤標為已執行。
- 已選 `pressly/goose/v3` v3.27.0；兩張版本表及單版交易已實測。checksum、dirty、來源與跨 stream 互斥由同一 runner 補足，稽核表位於只授予 Bot 讀取權限的 `migration` schema。
- 待部署者確認：既有環境的 schema 基線、備份與正式切換窗口；不得僅憑 `CREATE IF NOT EXISTS` 判定既有物件結構相容。

## 現有行為與新行為

| 範圍 | 現有行為 | 新行為 |
|---|---|---|
| 結構同步 | Bot 啟動時執行多組 GORM `AutoMigrate` | 獨立 runner 執行版本化 SQL；Bot 啟動驗證版本、clean 狀態及 checksum |
| 可觀測性 | 一筆 `auto_migrate` 成功摘要 | 每版開始／完成／失敗與最終版本的結構化日誌 |
| 版本稽核 | 無 SQL 套用紀錄 | 依 stream 保存版本、checksum、來源、時間與 dirty／失敗狀態 |
| 失敗處理 | 啟動時回傳錯誤 | runner 非零退出、阻止部署；不自動 `down` |

## 影響範圍與使用情境

- 使用者：部署者、維運者；一般群組成員的既有行為應保持不變。
- 介面：一次性 migration 命令的 `up`、`status`、`verify`，以及部署編排。
- 資料：migration 版本紀錄、基線 SQL、既有五組 SQL 及後續合併的 `20261005133800`；不得更動業務資料。
- 文件：README、環境準備、備份與回退程序。
- 作為部署者，我想在啟動新版 Bot 前看到已套用版本及失敗原因，以便避免未完成 schema 上線。

## 驗收情境

### R1：全新資料庫建置

- 場景：空 PostgreSQL database 啟用核心功能。
- 測試：待建立 `TestMigrationRunnerFreshDatabase`。
- 假設：具備核准的核心基線與必要 DDL 權限，未啟用語意記憶。
- 當：執行核心 runner，再啟動 Bot。
- 那麼：基礎表先建立，`11400`、`11600`、`11700`、`11800`、`133800` 依序完成；不要求 pgvector，Bot 版本檢查通過。

### R2：既有資料庫接管與重跑

- 場景：既有資料庫曾由 GORM `AutoMigrate` 建表，部分新結構已存在。
- 測試：待建立 `TestMigrationRunnerAdoptAndRerun`。
- 假設：部署者已備份，runner 已驗證必要表、欄位、索引與約束。
- 當：明示接管並執行 `up`，再執行第二次。
- 那麼：既有資料不遺失；已存在且結構相符的版本標為 `adopted`，缺少的版本由 runner 執行後標為 `applied`，不冒稱接管版本的 SQL 曾被執行。checksum 表示對應檔案版本；第二次零 DDL、回報已是最新版本。

### R3：語意記憶可選

- 場景：語意記憶關閉或 pgvector 尚未安裝。
- 測試：待建立 `TestMigrationRunnerSemanticOptional`。
- 假設：核心 stream 與語意 stream 分開記錄。
- 當：先跑核心，再嘗試啟用語意記憶。
- 那麼：核心可完成且 `11500` 保持待執行；語意啟用前檢查 extension，缺失時明確失敗，不把待執行標成成功。

### R4：並行、失敗與檔案漂移

- 場景：兩個 runner 同時執行、SQL 中途失敗、程序中斷或已套用檔案內容改變。
- 測試：待建立 `TestMigrationRunnerLockDirtyAndChecksum`。
- 假設：使用資料庫互斥、單版交易與持久化狀態。
- 當：執行或重試 `up`。
- 那麼：同一版本不並行重複執行；等待互斥鎖逾時須非零退出。SQL 失敗或程序中斷後，runner 留下可稽核的 dirty 狀態，重跑須先明示核對／修復；checksum 不符拒絕執行，不自動回退。即使 SQL 使用 `IF NOT EXISTS`，實際表、欄位、索引或約束不符也不能標記成功。

### R5：逐版日誌與部署門檻

- 場景：runner 成功或失敗，接著由 Compose 啟動 app。
- 測試：待建立 `TestMigrationRunnerLogsAndStartupGate`，加上 Compose 設定檢查。
- 假設：runner 與 app 使用同一版 migration 檔案；runner 使用部署 DDL 角色，常駐 Bot 使用受限 DML 角色；日誌不含 DSN 或憑證。
- 當：執行部署流程。
- 那麼：每版記錄 version、stream、status、duration、checksum 摘要；失敗非零退出且 app 不啟動。即使繞過 Compose 直接啟動 Bot，只要必要 stream 有 dirty、失敗、缺版或 checksum 不符，都須拒絕啟動；通過時記錄已驗證版本。

### R6：既有行為回歸

- 場景：完成 schema 切換後處理原有 Telegram 更新與管理指令。
- 測試：`go test -race -count=1 ./...` 及待建立的資料庫整合測試。
- 假設：資料庫結構與模型相容。
- 當：啟動新版 Bot 並執行既有測試。
- 那麼：偵測、處置與人工回饋結果不因 runner 改變。

### R7：已部署 Runner 的增量升級

- 場景：資料庫已由舊版 Runner 套用到 `11800`，另有舊訊息事件資料。
- 測試：`TestMigrationRunnerMessageMetadataUpgrade`、`TestMigrationRunnerAdoptAndRerun`、`TestMigrationManifestStreams`。
- 假設：新版本 `133800` 的 SQL 與三個可為 `NULL` 的欄位契約同版發布。
- 當：新版 Bot 在 `133800` 尚未套用時驗證，再執行新版 Runner `up`。
- 那麼：Bot 先拒絕啟動；runner 只向前套用缺少版本，不改舊版 SQL 或舊事件；原有事件保留且新欄位為 `NULL`，重跑零 DDL。若既有 GORM 已建立這三欄且結構相符，明示接管時標為 `adopted`。

## 驗收條件與驗證需求

- R1～R5 與 R7 必須在一次性隔離 PostgreSQL 實跑，涵蓋空庫、既有庫、重跑、增量升級、並行、失敗與缺 pgvector；專用 CI 不得因缺少 `TEST_DATABASE_URL` 而靜默略過。
- R5 必須確認容器映像含同版 SQL、runner 退出碼確實控制 app 啟動，並能從日誌辨識已完成版本；另以兩組 DB 憑證實測 runner 可做 DDL、Bot 可正常讀寫但無 DDL 權限。
- R6 須通過既有 Go 回歸；README 與部署設定不得再宣稱 SQL 不會自動執行而忽略 pre-deploy job 的實際行為。

## 風險與下一步

- 既有 `down.sql` 含刪表／刪欄位，不能當作安全自動回退。
- 基線不完整、既有 schema 漂移或 pgvector 缺失會阻止部署；應先在備份還原環境驗證，再安排正式切換。
- 已在一次性 PostgreSQL 16、18 與 pgvector 容器驗證 runner；正式部署前仍須完成備份還原演練、確認既有 schema 與切換窗口。本次不執行正式資料庫變更。
