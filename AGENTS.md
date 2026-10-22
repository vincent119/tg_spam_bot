# Repository 與 Go Server 規則範本

## 適用範圍與優先順序

- 本規範適用於此 repository 及其所有子目錄。
- 更深層目錄的 `AGENTS.md` 可補充技術規則；規則衝突時，距離目標檔案最近的規則優先。
- 不確定既有慣例時，先查相鄰程式碼、測試、Makefile、README、CONTRIBUTING 與開發文件；不得以記憶或猜測取代驗證。
- 專案文件與已確認的需求優先於通用建議；使用 skill 時，skill 的流程規則與本檔共同適用。

## 語言與溝通

- 預設使用繁體中文（台灣）回應、撰寫文件、commit message、PR title 與 PR description。
- 程式碼保留既有語言；註解與 docstring 使用繁體中文，說明「為什麼」而非逐行複述行為。
- 英文僅用於程式碼、路徑、URL、API 欄位、技術名稱與必要專有名詞。
- 交付時先說明結果，再說明根本原因、驗證證據、未驗證範圍、部署或回滾影響。
- 明確標示未知資訊與假設；不得把未驗證工作說成完成。

## 規格、需求與工作追蹤

- 新功能、需求變更、範圍外 bug 修正，或使用者要求先規劃時，先使用 SDD workflow 建立或更新需求、設計與未完成 task。
- 規格工作區由專案定義；私有規格可使用 `$PRIVATE_SPEC_WORKSPACE/<repository>/.specs/<space>/`。不得將私有需求、營運資料、內部識別碼或商業決策複製到公開 repository。
- 既有需求的範圍內修正，先確認對應 task；若沒有未完成 task，新增工作項目並等待使用者排程後再實作。
- requirements、design、task、程式碼與驗證結果必須可追溯。不得在已完成 task 追加新工作；應建立新的未完成 task。
- 僅在所有驗收條件都有實際證據時，才能將 task 標示為 `Complete`。

## 變更邊界與跨層契約

- 修改前先檢查相鄰程式碼、型別、設定與測試，沿用既有模式；不假設 library、服務或資料欄位存在。
- 改動必須留在使用者請求與 task 邊界內；不得夾帶無關重構或順帶修正。
- 共用 API 契約變更時，同步更新後端行為、OpenAPI、前端 schema／types、文件與規格。
- 前端不得重算後端擁有的業務指標、報表口徑或授權結果。
- 不得未經確認變更既有 API 語意、資料定義、授權行為、保留政策或相容性承諾。

## 安全性與資料保護

- 不得硬編碼或輸出密碼、Token、DSN、API key、個資與其他敏感資料。
- 所有外部輸入均須驗證格式、長度、範圍、列舉值與權限。
- 使用參數化查詢；不得將使用者輸入拼接為 SQL、shell command、檔案路徑或 URL。
- 錯誤 response 與日誌不得洩漏 SQL、堆疊、設定值或內部拓撲。
- 僅使用標準 `crypto/*` 實作密碼學；評估正則 ReDoS、路徑穿越、Zip Slip、SSRF 與不受控解壓風險。

## Git、PR 與合併

- 功能與修正使用 feature branch。
- 未取得本次動作的明確授權前，不得 commit、push、建立或編輯 PR、rebase、force-push、merge、切換分支或 pull。
- commit 與 PR 內容必須如實描述已驗證範圍；不得以「完成」掩蓋未實作或未驗證項目。
- 合併前檢查目標分支、相依 PR 順序、migration 與檔案衝突、測試狀態、部署與回滾影響。
- 不得使用 `git reset --hard` 或其他覆寫、刪除工作內容的指令，除非取得明確授權。
- 交付前執行 `git diff --check`；合併後同步本機預設分支並回報 merge commit 與待處理部署事項。

## Go Server：修改前

- 先閱讀同一 package 的兩到三個相鄰檔案與測試，沿用既有目錄、檔名、型別與錯誤處理模式。
- 使用 `make help`、README、CONTRIBUTING 或開發文件確認 format、test、lint、build 與 generate 命令。
- 優先修正根本原因；不得以關閉 lint、弱化測試、忽略錯誤或大量型別斷言掩蓋問題。

## Go 程式碼風格與錯誤處理

- 新檔案沿用資料夾既有 package；package 名稱全小寫、單字、無底線，避免 `util` 與 `common`。
- imports 依標準函式庫、第三方套件、專案內部套件分組；不得保留未使用 import 或引入循環依賴。
- 使用 Tab 縮排、UTF-8 無 BOM 與單一檔尾換行；使用專案指定 formatter、lint 與 `go vet`。
- 導出的型別、函數與常數提供符合既有慣例的 godoc；註解只記錄約束、相容性、原因或例外。
- 函式維持單一責任，使用 early return，避免不必要的巢狀 `else`。
- `context.Context` 是 I/O、HTTP、資料庫、queue 與外部服務邊界函式的第一個參數，且必須向下傳遞。
- 不得把 `context.Context` 存進 struct，也不得在 request path 任意使用 `context.Background()`。
- 立即處理錯誤；需補上下文時使用 `fmt.Errorf("情境: %w", err)`，跨層判斷使用 `errors.Is` 或 `errors.As`。
- 正常錯誤流程以 `error` 回傳；library、handler、worker 不得使用 `panic`、`log.Fatal` 或 `os.Exit`。
- error string 以小寫開頭、不加結尾標點，除非專案既有規範不同。
- 縮寫遵循 Go 慣例，例如 `URL`、`HTTP`、`ID`、`API`。
- 避免 `any`、寬鬆型別斷言、全域可變狀態與未受控 goroutine；確有必要時將理由限制在最小範圍。

## 並行、資源與 HTTP Client

- 每個 goroutine 都要有結束機制，例如 `context.Context`、`WaitGroup` 或關閉 channel；關機時必須回應 `ctx.Done()`。
- 共享可變狀態使用 `sync.Mutex`、`sync.RWMutex` 或經量測確認的無鎖設計保護。
- `defer Close()` 放在取得資源後的適當 scope；不得在大型迴圈累積 defer，必要時以匿名函式縮小 scope。
- slice 或 map 跨越可變所有權邊界時，依需要複製，避免意外共享可變資料。
- HTTP Client 僅保存設定與可重用的 `*http.Client`，不得保存單一請求狀態。
- HTTP 呼叫使用 `http.NewRequestWithContext`、合理 timeout，讀取後關閉 `resp.Body`。
- 重用 Transport；重試只適用於冪等操作，並具備退避、上限與明確的可重試錯誤範圍。
- 重讀 `req.Body` 前建立安全副本並設定 `GetBody`；`io.Pipe` 與 multipart 寫入需具明確同步與關閉責任。

## JSON、時間與 API

- 對外輸入／輸出型別使用一致的 `json` tag；可選欄位依契約使用 `omitempty`，不得改變既有 response 語意。
- 相容性允許時，外部 JSON 輸入拒絕未知欄位，避免拼字錯誤被靜默忽略。
- 內部儲存與運算使用 UTC；JSON 時間採 RFC3339，呈現層再依使用者時區格式化。
- 使用既有 DTO、validator、錯誤處理與授權 middleware；不得在 handler 中自行建立不一致的契約。
- API 修改同步更新 OpenAPI、前端 schema／types、規格與測試；破壞性變更必須先取得明確確認。
- 檔案讀取必須設定大小與格式限制；壓縮檔處理須防止 Zip Slip 與不受控解壓。

## Configuration、DI 與 API 文件

- 設定管理強制使用 `spf13/viper`；優先順序至少明確定義為環境變數高於設定檔，設定檔高於預設值。
- 先 unmarshal 至 typed Config，再集中驗證；不得在業務程式碼分散讀取 Viper。
- 選填設定必須有明確預設值；必填設定必須在 startup 驗證。環境變數映射、巢狀 key 與空值行為要有測試。
- 敏感資訊由環境變數、Secret 或受控設定服務注入，不得寫入 repository、範例設定或日誌。
- 必要設定缺失或無效時，application startup 必須明確失敗並回傳至統一生命週期管理入口。
- Infrastructure 與 application 層依賴由 composition root 組裝；採用 `uber-go/fx` 時集中於 `cmd/` 或明確 DI composition root。
- Repository 與 service 以小型 interface 表達真正需要的能力；具體實作維持非公開，測試替身依專案慣例建立。
- HTTP API 以 OpenAPI 作為可追溯契約；採用 `swag` 時同步更新 Swagger 註解，以專案指定命令重新產生文件，不手動修改產生檔。

## 採用 gRPC／Domain Events 時

- Proto 集中於專案指定位置，產生碼不得手動修改；RPC 改動後重新產生並提交。
- gRPC server 尊重 deadline 與 `context.Context` 取消；維持 recovery、observability、logging、authentication／authorization 的 interceptor 順序，並提供 health checking。
- 將 domain error 映射為正確 gRPC status；停止服務使用 `GracefulStop` 或具 timeout 的流程。
- Domain Event 為不可變 struct，至少包含事件識別碼、發生時間、Aggregate 識別碼與事件類型，並以過去式命名。
- 跨 Bounded Context 的非同步事件採明確訊息邊界；Outbox 的業務操作與事件寫入必須在同一 transaction。
- Consumer 必須可冪等處理重複事件，以事件識別碼或等效去重鍵去重。

## 生命週期、日誌與可觀測性

- HTTP server、worker、consumer 與 background goroutine 的生命週期統一使用 `github.com/vincent119/commons/graceful`。
- 關機時先停止接收新工作，等待進行中工作，最後關閉資料庫、cache、queue、trace 等外部資源。
- 不得在 server goroutine 或 library 呼叫 `os.Exit`、`log.Fatal`；錯誤回傳統一生命週期管理入口。
- 應用程式日誌強制使用 `github.com/vincent119/zlogger`，不得混用標準 `log`、`slog`、`zap`、`logrus` 或其他 logger。
- 日誌使用結構化欄位；保留既有 `trace_id`、`span_id`、`req_id`、`subsystem` 等關聯欄位。
- 跨邊界呼叫傳遞 `context.Context`；metrics 使用一致的蛇形與單位後綴，例如 `_total`、`_seconds`、`_bytes`。
- metrics 不得使用 `user_id`、email、`trace_id` 等高基數 label，也不得含敏感資料。

## 測試、資料庫與 Migration

- 新增可觀察行為或修正 bug 時，優先先寫會失敗的測試，再實作最小修正，最後重構。
- bug 修正必須有 regression test；測試名稱描述可觀察行為與結果，不描述私有實作細節。
- 優先使用 table-driven tests；輔助函式使用 `t.Helper()`，清理使用 `t.Cleanup()`；需要並行驗證時執行 `-race`。
- 優先隔離 domain、service、parser、validator、轉換與 query planner；repository、HTTP、資料庫與外部服務整合測試驗證邊界契約。
- 關鍵演算法或解析器依風險新增 benchmark 與 fuzz test，不作為所有小型修改的固定門檻。
- 不得為了通過測試而放寬 assertion、刪除或跳過測試，或把核心邏輯搬到測試外。
- 文件、純格式化與已完整覆蓋的機械調整不強制先寫測試，但仍要執行相稱驗證。
- 不得在 application startup 執行 schema migration。Production 禁止 ORM 自動 schema migration，例如 GORM `AutoMigrate`。
- 結構異動使用版本化 migration；不得修改已在任何環境套用的 migration。
- 新增索引、View、Materialized View、彙總表或資料回填前，量測查詢計畫、資料量、鎖定風險與預期效益。
- migration、資料回填、排程與回滾必須有可追溯部署程序；未授權前不得自行套用 production SQL。

## 驗證與交付

- 執行與改動相稱的 format、test、lint、build、generate 與安全檢查；跨層改動必須驗證兩側契約。
- 若測試或驗證無法執行，明確說明原因、未驗證範圍與風險。
- 面向使用者的功能、API、設定、部署或維運行為變更時，更新既有對應文件，不建立內容重複的平行文件。
- 新增依賴、架構、共用設定或部署行為前，先說明必要性與影響並取得確認。
- 交付時列出變更檔案、行為影響、驗證結果，以及 migration、設定或部署注意事項。
