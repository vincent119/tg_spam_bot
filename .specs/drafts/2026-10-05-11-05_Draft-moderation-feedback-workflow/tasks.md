# 任務草稿：重複處置、人工回饋與判定預覽

Status: Planned

## Execution Context

- 意圖：將使用者要求的三項建議納入 SDD，後續依序交付重複處置、人工回饋、判定預覽及中文回歸。
- 本次授權：僅文件；草稿仍為 Draft，不執行下面的程式實作任務。
- 非目標：不掃 Telegram 全歷史、不改語意重複、不建 Web UI、不換模型、不部署。
- 已定範圍：第一項包含第 3 則觸發後處置並清除已追蹤的前兩則；同群同人同原文、30m／3 為基準。
- 設計假設：repeat 預設 observe；`/spam` 預設刪文、ban 明示；`/ham` 不自動解封；預覽先用 `/check`。以上須在 T0 確認，不當成已核准契約。
- 關鍵檔案：behavior adapters、detection application／store、command handler、config、DI 與對應測試；詳細候選見 design。
- 完成條件：未來實作須通過 R1～R8、T0～T6、V1～V2，記錄實際命令及限制；文件新增本身不代表功能完成。

### Protected Behavior

- 不修改前一份 Complete spec 的歷史結論；不重做已完成窗口實作。
- 保留 1 分鐘頻率與協同契約、原文 HMAC、唯一訊息去重、既有豁免與多群隔離。
- `repeated_content` 不參與規則加分／ban gate；新直接處置走明確獨立政策。
- 沒有新設定時保留既有行為；`/feedspam` 不處置，`/purge` 不擴大範圍。
- 全域 observe 不自動刪文／封鎖；AI 失敗不得憑空建立垃圾結果。
- 不常態保存完整原文，不把樣本回饋稱為模型已訓練完成。

### 本輪 Boundary

#### Allowed Changes

- 本草稿目錄的 `brief.md`、`requirements.md`、`design.md`、`tasks.md`。

#### Forbidden

- 程式、sample、真實設定、秘密值、schema、舊規格與 OpenSpec 狀態。
- Git commit／push、發布、正式資料庫、Telegram 或 AI 外部副作用。

未來各 task 的 Allowed Changes 只是升格後的候選邊界，不能取代本輪文件限定。實作前須先確認契約並升格正式 spec；每項仍受 Protected Behavior 與該 task Forbidden 限制。

## 任務依賴

| 任務 | Depends | 狀態 |
|------|---------|------|
| T0 契約確認與草稿升格 | 使用者確認及實作指示 | Planned |
| T1 重複政策與快照 | T0 | Planned |
| T2 自動處置與前文清理 | T1 | Planned |
| T3 可修正人工樣本與範圍 | T0、T2 | Planned |
| T4 管理員回饋指令 | T3 | Planned |
| T5 只讀判定預覽 | T2、T4 | Planned |
| T6 中文回歸資料集 | T0；整合驗證依賴 T2、T4、T5 | Planned |
| V1 驗收、資料與安全回歸 | T1～T6 | Planned |
| V2 品質、文件與交付 | V1 | Planned |

## 實作任務

### T0：確認契約並升格

- [ ] Status: Planned
- Boundary:
  - Allowed Changes：本草稿與升格後明確指定的正式 spec 文件。
  - Forbidden：任何程式實作、資料庫操作或直接啟用封鎖。
- Depends: 使用者確認方案與後續實作範圍。
- Context: 固定 repeat 預設與違規計數、模式來源合併、`/spam`／`/ham`／`/check` 語法、已刪除目標回饋入口、人工樣本 scope／revision／快取與 migration 發布契約；確認後標記草稿 Promoted 並連結正式 spec。
- Verify:
  - 每個未決問題都有明確決策及測試對應；R1～R8 不被刪除或偷換範圍。
  - 對照舊窗口 spec，保留相容預設與 Protected Behavior；`git diff --check`。

### T1：重複政策、設定與原子候選快照

- [ ] Status: Planned
- Boundary:
  - Allowed Changes：config、sample、`.env.example`、detection application 行為 port／新 repeat policy、Redis／Memory adapters、main／DI 與相關測試。
  - Forbidden：改原文 HMAC、短窗口門檻、AI provider、真實設定或直接呼叫 Telegram。
- Depends: T0。
- Context: 結構化 observation 保留 Signals，增加同範圍原子計數與有界 ID 快照；count 與候選使用相同窗口，最多 100 且包含當前訊息。
- Verify:
  - 待建立：設定預設／非法值／ENV 優先、候選窗口／隔離／截斷／重送／並行測試。
  - 回歸 `TestBehaviorStoreRepeatWindow`、`TestBehaviorStoreRepeatIsolation`、`TestBehaviorStoreRepeatBoundaryAndExpiry`、`TestBehaviorStoreRepeatConcurrent` 與 Memory 對等測試；`git diff --check`。

### T2：達門檻直接處置並自動清前文

- [ ] Status: Planned
- Boundary:
  - Allowed Changes：detection processor／policy／處置 ports、新處置計畫服務、PostgreSQL action store、已確認的版本化 migration、Telegram adapter 與測試。
  - Forbidden：未經驗證從 message ID 推算其他訊息、跨群／跨人清除、改 `/purge` 範圍、隱性增加封鎖權限。
- Depends: T1。
- Context: 保存逐項目標與模式決策後執行；同事件計數不因清前兩則增加，已成功項目不重做；重試不擴大候選範圍，部分失敗與結果未知需明示。
- Verify:
  - 待建立：R1～R4 對應 selector，涵蓋三則全部清理、全域模式、豁免、並行、已刪除、權限不足、限流及保存結果失敗。
  - 回歸既有 policy／processor／Telegram adapter 與 `/purge` 測試；新 migration 以隔離資料庫驗證，不使用正式 DB；`git diff --check`。

### T3：人工樣本修正與查詢一致性

- [ ] Status: Planned
- Boundary:
  - Allowed Changes：manual sample／feed service、PostgreSQL 人工樣本與 scoped embedding／AI cache store、必要 application ports／查詢政策、已確認 migration 及測試。
  - Forbidden：直接覆寫他群共享向量、保存完整原文、微調模型、改 AI provider／prompt、將 ham 當永久白名單。
- Depends: T0、T2。
- Context: 支援可稽核 revision、群組範圍、修正後舊樣本失效、快取一致性及向量工作版本檢查；不能用既有 DoNothing 當修正 API。
- Verify:
  - 待建立：R6 標籤修正、並行 revision、舊 embedding 回寫、跨群隔離、矛盾標記、快取失效與缺原文測試。
  - 回歸 `/feedspam` 與既有 embedding provider／model／version／dimensions 過濾、語意訊號及 AI 模式；確認舊資料相容與回滾限制；`git diff --check`。

### T4：管理員 `/spam` 與 `/ham` 入口

- [ ] Status: Planned
- Boundary:
  - Allowed Changes：command domain／handler／ports、Telegram delivery 必要輸入、指令組裝與說明、稽核 store 及測試。
  - Forbidden：略過即時權限查詢、讓一般成員寫標籤或封鎖、變更 `/feedspam` 副作用、`/ham` 自動解封或清全部違規。
- Depends: T3。
- Context: 依 T0 語法處理回覆與已刪除目標；樣本、向量化、處置分別回報。樣本保存失敗不開始新處置；向量化失敗保留標記但不宣稱完成學習。
- Verify:
  - 待建立：R5～R6 指令與 E2E 測試，包含缺文字、無權限、匿名來源、目標豁免、錯誤參數、已刪除目標與重送。
  - 回歸既有 command domain／handler、`command_e2e_test.go`、`/feedspam`／`/purge`／`/ban`／`/unban`；`git diff --check`。

### T5：只讀判定預覽

- [ ] Status: Planned
- Boundary:
  - Allowed Changes：新增 preview service／只讀 ports、必要唯讀 store 查詢、command `/check`、DI、回覆格式與測試。
  - Forbidden：直接呼叫正式 Process／Observe、寫正式樣本／違規／AI 快取、隱式付費 AI、實際 Telegram 處置、新 Web UI。
- Depends: T2、T4。
- Context: 提供分階段原因、當時紀錄與目前重新預覽區別；預設不呼叫 AI，明示選用才呼叫。未知資料須標示不可得，不以零值假裝未命中。
- Verify:
  - 待建立：R7 的重複預覽、目標已觀測／未觀測、AI 停用／跳過／逾時／低信心、輸出逸出／截斷測試。
  - 使用 spy 證明正式寫入與處置呼叫為零，並回歸既有指令稽核、授權、限流及一般訊息處理；`git diff --check`。

### T6：中文回歸資料與序列測試

- [ ] Status: Planned
- Boundary:
  - Allowed Changes：detection／command 測試、規則測試 `testdata/`、測試資料說明與本 spec。
  - Forbidden：為了通過測試擴大關鍵字封鎖、加入真人識別資料、以付費模型即時回答當單元測試斷言。
- Depends: T0；可先整理資料，整合驗證依賴 T2、T4、T5。
- Context: 收錄六種使用者短廣告、繁簡／大小寫／數字變形與正常反例；精確重複不合併變形。分開標註內容結果與選用重複封鎖政策結果。
- Verify:
  - 待建立：R8 table-driven corpus、0／9／18 分鐘序列、規則與 mocked AI 的最終動作對照。
  - 回歸既有 `internal/detection/rules`、domain 正規化與 quote／signal 分離；不能用 corpus 通過率宣稱線上準確率；`git diff --check`。

## 驗證任務與品質檢查清單

### V1：驗收、儲存與安全回歸

- [ ] Status: Planned
- Boundary:
  - Allowed Changes：正式 spec 的驗證證據與上述已核准範圍測試。
  - Forbidden：測試正式群／正式 DB、未授權付費 AI、把跳過測試記為通過。
- Depends: T1～T6。
- Context: 對齊 R1～R8，確認文件中的建議 selector 已替換成真實存在的測試。
- Verify:
  - [ ] 先 `go test -list .` 核對相關 package 的 selector，再執行定向與完整 `go test -race -count=1 ./...`。
  - [ ] PostgreSQL／pgvector 整合測試使用隔離 DB；記錄 `TEST_DATABASE_URL` 是否提供與哪些測試 Skip，禁止輸出其秘密值。
  - [ ] 驗證 migration、部分失敗、跨群、權限變化、多副本與窗口過期。
  - [ ] 實機 Telegram 驗證需另取得測試群授權；未執行時列為剩餘風險。
  - [ ] 新增關鍵熱路徑 benchmark，以及指令解析／候選邊界必要 fuzz；明示量測環境。

### V2：品質、文件與交付

- [ ] Status: Planned
- Boundary:
  - Allowed Changes：已核准變更的格式修正、正式 spec、相關 sample／ENV／指令文件。
  - Forbidden：順手修不相關 lint、修改真實設定、commit／push／部署。
- Depends: V1。
- Context: 完成狀態必須有證據；舊 lint 與未執行實群測試分開列出，不假稱全部通過。
- Verify:
  - [ ] 格式、imports、`go vet ./...`、`golangci-lint run ./...`；確認變更未新增問題。
  - [ ] 量測受影響範圍覆蓋率，目標至少 80%，明示 package 範圍與既有差距。
  - [ ] sample／ENV／指令語法與實作相符，沒有秘密值或完整訊息原文落盤。
  - [ ] R1～R8 均有證據，Protected Behavior 回歸與主要風險已記錄。
  - [ ] `git diff --stat`、`git diff --check`、修改範圍與文件連結檢查。

## Implementation Notes

- 2026-10-05：完成文件草稿；程式基準 `2e16115`，不將三項建議寫入舊 Complete spec。
- 目前 `BehaviorStore` 沒有回傳候選 ID，`EnforcementAction` 沒有歷史目標欄位，`CreateManualSample` 不會覆寫既有標籤；不可假設現有介面已支援新功能。
- 目前共用向量沒有 chat 範圍、Store 使用 AutoMigrate；T0 必須先解決人工修正隔離與版本化 migration 契約。
- 本輪沒有執行 Go 測試或外部處置，因為只新增文件。未來恢復時先讀本文件 Execution Context、當前 task、Protected Behavior 與本節，不重掃全部 specs。

## 本輪文件驗證

- 需求範圍：三項建議均已納入，第一項明確涵蓋當前與先前重複訊息。
- 實作狀態：全部 Planned，未開始。
- 文件連結：4 份文件、10 個本地連結檢查通過；檔尾換行與尾端空白檢查通過。
- 差異格式與修改邊界：只新增本目錄 4 份文件；既有 tracked 檔案沒有變更，`git diff --check` 通過。新增未追蹤檔案另以逐檔 `git diff --no-index --check` 驗證。
