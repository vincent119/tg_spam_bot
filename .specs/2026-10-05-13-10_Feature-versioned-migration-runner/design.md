# 設計文件：版本化資料庫 Migration Runner

## 文件定位與已知契約

本設計對應 [requirements.md](requirements.md) R1～R6，接續[既有 SDD](../2026-10-05-11-14_Feature-moderation-feedback-workflow/design.md)已完成的功能。變更前 Bot 在啟動流程直接執行 GORM `AutoMigrate`，SQL 未由 runner 執行；目前本工作樹已切換為[啟動唯讀驗證](../../cmd/tg-spam-bot/main.go)，但未正式部署。

資料契約：變更前有 `11400` 至 `11800` 五組 `up/down.sql`，沒有版本表與 runner。現在另有核心及語意基線、嵌入式 SQL、schema 契約、goose 版本表與 `migration.schema_migration_runs`。`11800` 依賴 `ai_detection_events`；`11500` 依賴 pgvector。映像與 Compose 的修改僅在本工作樹，正式資料庫狀態、已手動套用版本、Bot 權限及備份可用性均未知，不得假造。

`main` 後續合併 `20261005133800_detection_event_message_metadata`。此版只擴充 `detection_events` 的三個可為 `NULL` 欄位，屬核心 stream；Runner 增加該版契約而不改其 SQL，也不改前七版契約或 checksum。

## Bounded Context 與設計原則

包含：核心／語意兩條 migration stream、全新資料庫基線、既有資料庫接管、版本與 checksum、互斥、逐版日誌、部署門檻及文件。

不包含：垃圾訊息判定、人工標籤演算法、Telegram API、業務資料轉換、自動執行 `down.sql`、正式環境實際部署。

設計原則：

- DDL 由獨立部署前程序負責，Bot 啟動只驗證需要的版本；同一發行版不可讓 runner 與 Bot 的 `AutoMigrate` 同時作為 schema 寫入者。
- 預設只執行向前變更；失敗、checksum 漂移或 dirty state 一律停止，不自行猜測修復。
- 語意記憶維持可選，不能因 `11500` 未執行而阻擋不使用 pgvector 的核心服務。

## 目標流程與介面

1. 部署者備份 PostgreSQL，於還原環境執行 `status` 與 `verify`，確認既有 schema 及版本來源；未知或漂移狀態停止並人工處理。
2. 部署前一次性 `up` 程序取得資料庫互斥鎖，校驗檔案版本與 checksum，先處理核心基線，再依版本執行核心待套用 SQL。啟用語意記憶時，另驗證 pgvector 與語意基線，再執行語意 SQL。
3. 每版先在 DDL 交易外持久化 `running/dirty`，再以單一交易執行 SQL、檢查後置 schema invariant 並寫入成功狀態。失敗或程序中斷會保留 dirty，另記錯誤並停止後續版本；重跑前須明示核對或修復，不可自動略過。
4. runner 成功退出後才啟動新版 Bot。Bot 在連接資料庫後確認核心及必要語意 stream 的版本、clean 狀態與本發行版 checksum，記錄已驗證版本；任一條件不符即拒絕啟動，不補跑 DDL。直接執行 binary 也須套用此門檻。
5. 回退須先停用新版、核對資料相容性，再依備份及明示人工程序處理；runner 不提供自動 `down`。

對外命令為獨立 `cmd/tg-spam-migrate` 的 `status`、`verify`、`up`；`up --adopt-existing` 才會接管完整符合 schema 契約的既有版本。連線採 `DATABASE_URL` 或 `DB_*` 環境變數，不輸出 DSN、密碼或 SQL 內容。選用 `pressly/goose/v3` v3.27.0，利用 `migration` schema 內分開的版本表維護雙 stream；checksum、dirty 與來源存於 `migration.schema_migration_runs`。goose 原生版本表不能單獨作為 Bot 啟動驗證依據。

## 版本、基線與語意依賴

- 核心 stream：基線 `20261005110000` 已由 `ad6b259` 於隔離 PostgreSQL 18 重建，固定為 `11400` 前的核心 schema，不含後續版本的新增結構；其後依序執行 `11400`、`11600`、`11700`、`11800`、`133800`。前七版在 PostgreSQL 16、18 的隔離空庫已實測可套用；`133800` 須在本次整合重新驗證。
- 語意 stream：基線 `20261005110100` 已由 `ad6b259` 在隔離 pgvector 0.8.5 資料庫重建，固定為 `11500` 前的語意 schema，需預先安裝 pgvector extension；其後才執行 `11500`。隔離庫已實測可套用。語意功能關閉時此 stream 留待執行，不標記成功。
- 已由 `AutoMigrate` 建立的既有環境不能直接「假定已套用」。接管前比對表、欄位、型別、索引、約束與必要資料；符合時以 `adopted` 保存來源，不能標成 runner `applied`。T0 實測發現 GORM 在部分表使用序列預設值與唯一索引，SQL 使用 identity 與唯一約束；接管允許經明列檢查的語意等價，不以 `pg_dump` 文字完全相同為必要。GORM 目前不建立 `idx_ai_feedback_cache`，因此不得把缺該索引的 `11800` 標為 `adopted`。checksum 指向接管時核對的檔案，不證明該 SQL 曾執行。
- 每版 SQL 後在提交成功紀錄前，再驗證預期表、欄位、型別、索引定義及約束。`IF NOT EXISTS` 只避免重複建立，不能代替結構相容檢查。
- 已套用 SQL 的檔名／內容視為不可變；版本、stream 與 checksum 需持久化，檔案改動須報錯。檔案重新分組時須保留原有版本識別，先確認是否已有外部手動套用紀錄。
- 選用 `pressly/goose/v3` v3.27.0。T0 已實測兩張版本表、PostgreSQL 鎖及交易回滾；其原生狀態不含 checksum、dirty 與 `adopted`，須在同一 runner 中補強且失敗時保持 fail-closed，不能讓兩套 migration 工具並存。

## 日誌與安全契約

每次 `up` 記錄操作摘要與最終版本；每版至少有 `stream`、`version`、`name`、`status`、`provenance`、`duration_ms`、checksum 摘要及穩定錯誤代碼。Bot 啟動時另記錄 schema 版本檢查結果；`status` 應明確分辨已完成、待執行、dirty、checksum 不符及接管來源。結構化日誌不得包含 DB 憑證、DSN、完整 SQL 或業務訊息原文。

執行前使用 PostgreSQL advisory lock，避免多副本並行；鎖等待 30 秒且受呼叫 context 截止約束，逾時非零退出。每版由 goose 的 Go migration 在同一交易內執行原始 SQL、逐項核對 `schema_contract.json` 中的表、欄位、索引、主鍵與序列，再寫入成功狀態；goose 版本紀錄亦在交易內。不支援交易的未來 SQL 必須另開設計審查，不可靜默跳過原子性保證。runner 使用具 DDL 權限的部署角色，常駐 Bot 改用僅有必要業務 DML、序列與 migration 狀態讀取權限的角色；Compose 的兩組憑證與權限已在隔離環境實測，`migration` schema 不授予 Bot 寫入權限。

## 受影響檔案計畫

| 範圍 | 預期變更 | 風險 |
|---|---|---|
| `cmd/tg-spam-migrate/` 與 runner package | 一次性命令、版本追蹤、鎖與日誌 | 並行重跑、dirty 恢復 |
| `migrations/` | 核心／語意基線與既有 SQL 的 manifest／分組 | 已套用版本辨識、pgvector 依賴 |
| `cmd/tg-spam-bot/main.go` | 啟動版本檢查，移除未追蹤版本的 DDL | 舊環境部署失敗 |
| `Dockerfile`、`docker-compose.yaml`、`Makefile` | 同版 runner/SQL 封裝與部署前 gate | app 早於 migration 啟動 |
| `internal/config/`、`README.md`、`.env.example`、CI 與整合測試 | 雙角色設定、操作文件、隔離 DB 驗證 | 權限或文件與實作不符 |

## 需求對應與驗證

| 驗收 | 設計落點 | 驗證 |
|---|---|---|
| R1、R2 | 基線、接管與 checksum | 空庫／既有庫／重跑整合測試 |
| R3 | 核心及語意雙 stream | 無 pgvector 核心啟動、語意缺依賴失敗 |
| R4 | 資料庫鎖、交易與 dirty state | 並行、失敗、程序中斷與檔案漂移 |
| R5 | 逐版日誌、非零退出、部署 gate | 日誌 spy、容器／Compose 啟動順序 |
| R6 | Bot 驗版本、clean 與 checksum，業務流程不變 | 全量 Go 回歸與 DB 整合 |
| R7 | `133800` 獨立版本契約與增量升級 | 舊版 runner 狀態、新版 gate、套用及 GORM 接管測試 |

## 替代方案與風險

| 方案 | 結論 | 原因 |
|---|---|---|
| Bot `main` 每次直接跑 SQL | 不採 | 多副本競爭、DDL 權限常駐、部署失敗界線不清 |
| 單一線性 stream 跳過 `11500` | 不採 | pgvector 未啟用時版本會產生缺口或假成功 |
| 只增加 `AutoMigrate` 逐表日誌 | 不採 | 無版本與 checksum，不能證明 SQL 已套用 |
| 失敗時自動執行 `down.sql` | 不採 | 刪表／刪欄位可能清除人工標籤及稽核資料 |

主要風險是既有環境的 schema 漂移、基線 SQL 與 GORM 模型不一致、pgvector 可用性及部署切換期的新舊 app 併行。先在備份還原環境與隔離 PostgreSQL 驗證，未通過時不進入正式部署。
