# 任務文件：版本化資料庫 Migration Runner

Status: Implemented Locally

## Execution Context

- 意圖：以部署前 runner 套用並稽核版本化 SQL，提供逐版日誌與失敗阻擋。
- 非目標：不執行正式 migration、不自動 `down`，也不改垃圾偵測規則。
- 已定決策：核心／語意雙 stream；只自動 `up`；Bot 啟動驗版本、clean 狀態與 checksum；不符即停止。
- 工具：選用 `pressly/goose/v3` v3.27.0；其缺少 checksum、dirty 與接管來源欄位，runner 須另加不可略過的稽核層。
- 關鍵檔案：`cmd/tg-spam-bot/main.go`、`migrations/`、`Dockerfile`、`docker-compose.yaml`、`README.md`。
- 完成條件：[requirements.md](requirements.md) R1～R6 有隔離 DB 與部署編排驗證，正式上線前另完成備份還原演練。

### Protected Behavior

- `semantic_memory.enabled=false` 不需 pgvector，現有群組偵測與管理指令行為不變。
- 既有資料及 `11400`～`11800` 的已套用歷史不得因接管而遺失或默默重寫。
- 失敗不啟動新版 Bot，不自動執行具破壞性的 `down.sql`。

### 邊界

#### Allowed Changes

- `cmd/tg-spam-migrate/`、必要的 runner package、`migrations/`、`go.mod`、`go.sum` 與相關測試。
- `cmd/tg-spam-bot/main.go` 的 schema 版本檢查及必要 DI。
- `Dockerfile`、`docker-compose.yaml`、`Makefile`、`.env.example`、`deployments/`、`scripts/`、README、設定範例及 `.github/workflows/` 中直接相關的內容。

#### Forbidden

- 修改 Telegram 處置、規則分數或人工標籤語意。
- 在實作任務中連接正式 DB、執行正式 `up/down`、刪除既有資料或憑成功日誌推論資料庫無漂移。
- 未先更新需求與設計，就讓 Bot 啟動程序繼續寫入無版本 DDL。

## 任務依賴

| 任務 | Depends | 狀態 |
|---|---|---|
| T0 工具與現況驗證 | 無 | Done |
| T1 核心／語意基線與版本契約 | T0 | Done |
| T2 runner 執行與日誌 | T1 | Done |
| T3 啟動及部署 gate | T2 | Done |
| T4 整合與回歸驗證 | T3 | Done Locally |
| T5 文件與交付 | T4 | Done Locally |

## 實作任務

- [x] T0：驗證 runner 工具、schema 基線與遷移檔現況
  - Status: Done
  - Boundary：唯讀現況調查、隔離 DB 實驗、必要的設計文件更新；不得改正式 DB。
  - Depends：無。
  - Context：`11500` 需要 pgvector，`11800` 需要既有 `ai_detection_events`；目前 `go.mod` 沒有 migration 套件。基線須固定在 `11400`／`11500` 之前，不能預含新版本結構。
  - Verify：記錄候選工具對雙 stream、鎖、交易、checksum、dirty 的實測結果；核對五組 SQL 在空庫與既有庫的相依順序。若舊基線不可重現，先修訂 SDD 再往下實作。

- [x] T1：建立可審核的基線及接管契約
  - Status: Done
  - Boundary：`migrations/`、runner metadata 與資料庫整合測試；不得變更既有業務資料。
  - Depends：T0。
  - Context：全新 DB 必須先有核心表；既有 `AutoMigrate` DB 要先驗 schema 才能以 `adopted` 標記基線，不能冒稱 `applied`。語意 stream 不得假成功。
  - Verify：隔離 DB 執行核心／語意基線 SQL 並核對必要表、型別、索引與約束；建立 `TestMigrationBaselineSchema` 與 `TestMigrationManifestStreams`。

- [x] T2：實作一次性 runner 與逐版日誌
  - Status: Done
  - Boundary：`cmd/tg-spam-migrate/`、runner package、必要設定與測試；不得改偵測處置邏輯。
  - Depends：T1。
  - Context：提供 `status`、`verify`、`up`；交易外先記 running/dirty，單版 DDL、後置 schema invariant 與成功狀態同交易。失敗、崩潰或逾時不可自動跳過，已接管版本要保留 `adopted` 來源。
  - Verify：`TestMigrationRunnerFreshDatabase`、`TestMigrationRunnerAdoptAndRerun`、`TestMigrationRunnerSemanticOptional`、`TestMigrationRunnerLockDirtyAndChecksum`、`TestMigrationRunnerLogsAndStartupGate` 中 runner 部分；檢查崩潰恢復、同名異構 `IF NOT EXISTS`、鎖逾時、日誌無憑證與完整 SQL。

- [x] T3：切換 Bot 啟動與部署順序
  - Status: Done
  - Boundary：`cmd/tg-spam-bot/main.go`、`Dockerfile`、`docker-compose.yaml`、`Makefile` 與必要啟動測試。
  - Depends：T2。
  - Context：runner 與 Bot 使用同版 SQL；runner 成功才啟動 Bot，Bot 只檢查 schema 版本、clean 與 checksum，不再做無版本 DDL。Compose 的 runner/app 採分離 DB 憑證。
  - Verify：部署設定檢查、runner 失敗時 app 不啟動、直接執行 Bot 且 dirty／缺版／checksum 不符時拒絕啟動、成功時記錄已驗證版本、語意關閉時無 pgvector 可啟動。

- [x] T4：隔離資料庫與既有行為回歸
  - Status: Done Locally
  - Boundary：runner／啟動整合測試與既有相關測試；不得連線正式 DB。
  - Depends：T3。
  - Context：專用 CI 必須提供 `TEST_DATABASE_URL`，不能把 PostgreSQL 測試 Skip 視為通過；runner 與常駐 Bot 使用分離角色。
  - Verify：R1～R6 的空庫、既有庫、重跑、並行、失敗、中斷、缺 pgvector 與兩角色最小權限實測；`go test -race -count=1 ./...`、`go vet ./...`。

- [x] T5：更新操作文件與交付限制
  - Status: Done Locally
  - Boundary：README、設定範例、本 SDD 的結果與風險；不得宣稱正式部署完成。
  - Depends：T4。
  - Context：說明 runner 與 Bot 的責任、逐版日誌、備份／回退、最小權限及手動接管步驟。
  - Verify：文件中的命令、日誌欄位、版本與實作一致；`git diff --stat`、`git diff --check`。

## 驗證任務與品質檢查清單

- [x] R1～R5 均有隔離 PostgreSQL 的成功與失敗案例；專用 CI 明確設定 `TEST_DATABASE_URL`，不把 Skip 當成功。
- [x] R6 既有偵測、管理指令、語意關閉情境回歸通過。
- [x] runner 成功、失敗、checksum 漂移與 dirty 狀態的輸出可供維運判讀。
- [x] Docker/Compose 啟動順序與同版 SQL 封裝經實際測試。
- [x] Go 格式、`go vet`、適用 lint、測試及文件連結檢查完成；全量 lint 只剩 11 項既有檔案警告。
- [x] `git diff --stat` 與 `git diff --check` 通過；變更均位於 Allowed Changes。

## Implementation Notes

- 2026-10-05：依使用者要求先補 migration runner SDD；現有 Bot 只有 `AutoMigrate` 摘要日誌，版本化 SQL 不會自動執行。本次未實作、未操作資料庫、未部署。
- 2026-10-05：使用者授權開始實作。於獨立工作樹與一次性 PostgreSQL 16、18、pgvector 0.8.5 容器進行 T0，不接觸正式資料庫。
- T0：以 `ad6b259` 的舊版 GORM 在隔離資料庫重建 `11400`／`11500` 前基線；核心基線在 PostgreSQL 16、18 成功，後續 `11400`、`11600`、`11700`、`11800` 成功；語意基線加 `11500` 在 pgvector 0.8.5 成功。
- T0：goose v3.27.0 以兩張版本表完成雙 stream，PostgreSQL 鎖與單版交易測試通過；失敗 SQL 的建表已回滾。goose 原生版本表不記錄 checksum、dirty 或 `adopted`，須由 runner 額外實作。
- T0：現有 GORM 與 SQL migration 在 identity／sequence、唯一索引與約束表示法有差異；GORM 既有庫亦缺 `11800` 的 `idx_ai_feedback_cache`。接管檢查需比對語意等價，缺少結構不可標記 `adopted`，要由待執行版本補齊或停止。
- T1～T3：建立兩條基線、`schema_contract.json`、Goose 單版交易、`migration` schema 中的 checksum／dirty／provenance 表與分流版本表；Bot 啟動僅驗證，不執行 DDL。Compose 的 `migrate` 成功後才啟動 app，兩者使用不同 DB 憑證。
- T4：`TestMigrationBaselineSchema`、`TestMigrationManifestStreams`、`TestMigrationRunnerFreshDatabase`、`TestMigrationRunnerAdoptAndRerun`、`TestMigrationRunnerSemanticOptional`、`TestMigrationRunnerLockDirtyAndChecksum`、`TestMigrationRunnerFailedDDLLeavesDirty`、`TestMigrationRunnerLogsAndStartupGate` 在一次性 pgvector PostgreSQL 容器通過；核心 runner 另於 PostgreSQL 16 通過。
- T4：隔離容器中，Bot 角色可讀 `migration` schema 版本表且可讀寫業務表、使用序列；`CREATE` schema 權限及 migration 表 `INSERT, UPDATE, DELETE` 均為 false。Compose 相同依賴條件在 runner 退出碼 7 時令 app 容器保持未啟動。
- T4：`go test -p 1 -race -count=1 ./...`、`go vet ./...`、`docker build`、映像檔案檢查、`docker compose config --quiet` 通過。首次 Docker 建置在本機約 2 GiB 記憶體下失敗，限制 Go 編譯器 `GOMEMLIMIT=1200MiB` 後成功。
- T4：最終 Docker 映像內的 `tg-spam-migrate verify` 以唯讀 Bot 角色連接隔離測試資料庫，回報 `core=20261005111800 semantic=0`；一次性測試容器已停止並自動移除。
- T5：README、`.env.example`、Makefile、部署角色腳本與 migration 專用 CI 已更新；本機沒有執行正式 migration 或推送映像。
- 正式 SDD 原有任務已完成並合併；本規格不改寫其歷史驗收結果。
- 既有 `down.sql` 可能移除人工標籤、向量、重複處置稽核或 `feedback_epoch` 欄位，禁止自動回退。

## 驗證結果摘要

- 新行為驗證：隔離 PostgreSQL 16、18 與 pgvector 測試、Bot 共用唯讀 gate、Compose gate、雙角色權限與映像封裝均通過。
- 現況查證：已核對啟動流程、五組 SQL、Dockerfile、Compose 與 README，並以實際舊版及新版 GORM schema 比對。
- 文件一致性：README、設定範例、Compose、Dockerfile 及專用 CI 已依實作更新。
- 剩餘風險：正式 DB 的既有 schema、已手動套用版本、備份可還原性及 pgvector 可用性仍未知；正式切換前必須在備份還原環境演練。GitHub CI 尚待遠端執行。原始工作樹另有未提交的 `20261005133800_detection_event_message_metadata` 變更，未包含於本分支；若未來合併該版，必須同步增加 schema 契約並重新驗證 runner，否則 manifest 會安全地拒絕啟動。
