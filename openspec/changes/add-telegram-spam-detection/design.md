## Context

儲存庫目前只有 OpenSpec，需從零建立 Go 1.25+ Telegram 群組管理服務。Bot 將以管理員身分接收一般群組訊息，僅授予刪除訊息與限制成員權限。系統不只判斷垃圾訊息，還需在 Webhook 重送、Telegram API 部分失敗及程序重啟下，可靠完成刪除、30 天違規累計、分級處置與稽核。

樣本包含繁體中文、簡體中文、英文、混合文字、Telegram mention、邀請連結、轉傳內容及刻意插入符號的廣告。繁簡轉換可能產生歧義，因此不能覆寫或只比對轉換後文字。規則需由 YAML 自訂，訊息熱路徑不得逐則查詢資料庫。

## Goals / Non-Goals

**Goals:**

- 以安全 Webhook 接收並冪等處理 Telegram 更新。
- 以可解釋的內容與行為訊號支援多語言垃圾訊息偵測。
- 讓管理者以 YAML 定義一般與嚴重違規，不修改核心程式。
- 可靠執行刪除、警告、禁言、封鎖及 30 天滾動違規政策。
- 保存可追蹤且不洩漏完整訊息與秘密值的管理紀錄。
- 讓規則、Telegram API、狀態儲存與 delivery adapter 可獨立測試及替換。

**Non-Goals:**

- 建立規則管理後台或未經審核的線上規則編輯。
- 訓練或呼叫機器學習與大型語言模型。
- 保存完整訊息內容或建立人工標註資料庫。
- 自動提升 Bot 權限或修改群組 Privacy Mode。
- 可靠推算 Telegram 帳號建立日期；系統只追蹤可觀測的入群時間。

## Decisions

### 採用 Gin、GORM AutoMigrate 與 zlogger

HTTP delivery 統一使用 Gin 註冊 Webhook 與健康檢查；資料存取使用 GORM PostgreSQL driver，啟動時以明確 model 清單執行 `AutoMigrate`；結構化日誌統一使用 `github.com/vincent119/zlogger`。這是使用者明確指定，覆蓋專案原先偏好 `net/http` 組裝、版本化 migration 執行及避免 `AutoMigrate` 的規範。既有 SQL migration 保留作 schema 參考與既有環境相容，但新環境以 GORM model 為 schema 來源。

為避免 `AutoMigrate` 意外刪除資料，model 只允許向前新增或調整 GORM 支援的非破壞性結構；破壞性欄位變更仍需另行審查。資料表與欄位透過 GORM tag 的 `comment` 建立繁體中文註解。

### 採 DDD 分層與可替換邊界

`cmd/tg-spam-bot` 負責設定與生命週期組裝。delivery 層驗證 Webhook 並將 Telegram DTO 轉為 application command；application 層協調冪等、豁免、偵測、違規與處置；domain 層定義訊息、規則、判定、違規政策與處置計畫；infra 層實作 Telegram Client、PostgreSQL、Redis 或記憶體 adapter。

替代方案是由 HTTP handler 直接查詢及呼叫 Telegram API，但會把傳輸、規則與狀態耦合，難以模擬部分失敗及保證冪等，因此不採用。

### Webhook 先可靠登記再執行處理

Webhook 必須以固定時間比較驗證 `X-Telegram-Bot-Api-Secret-Token`，設定 body 上限並拒絕未知 JSON 欄位。更新以 `update_id` 原子占用；處理可同步完成後回傳 2xx，或先可靠寫入工作紀錄再由 worker 執行。只有可安全重試的內部錯誤才回傳非 2xx，重送仍由相同冪等鍵收斂。

Webhook 解析後先以 `allowed_chat_ids` 驗證群組，再確認 `chat.type` 是 `group` 或 `supergroup`。未授權群組、私人聊天與頻道統一回傳成功並忽略，避免 Bot 被加入其他聊天後誤執行管理動作，也不透過回應差異揭露允許清單。

直接依隱藏 URL 判斷來源不足以抵抗秘密路徑洩漏，因此使用 Telegram `secret_token` Header。Webhook 與 long polling 互斥，本變更只實作 Webhook。

### 原文與繁體轉換副本雙軌偵測

正規化流程先限制長度並做 Unicode 正規化、英文小寫化、全半形與可控干擾字元處理，再保留 `original`、`normalized` 與 `traditional_variant`。偵測器同時比對原文正規化結果與繁體轉換副本，命中結果記錄來源；地區用語、黑話與不可靠轉換放入 `aliases`。

只把簡體轉為繁體後比對雖能縮短詞庫，但 `发`、`后`、`干`、`面` 等字存在語意歧義，且地區詞彙不是字形轉換，因此不能將轉換副本視為原文。中文轉換實作採介面封裝，選定外部套件前需確認授權、Go 1.25 相容性與字典品質。

### YAML 規則編譯為不可變記憶體快照

規則依類型檔案拆分，核心欄位為 `id`、`name`、`severity`、`action`、`threshold`、`terms`、`aliases`、`require_any`、`weight` 與 `enabled`。啟動時驗證完整規則集合，再編譯成字串搜尋及少量有界正則索引；訊息處理不查詢規則資料庫。大量詞彙時使用 Trie 或 Aho-Corasick 類型索引，正則只處理 URL、邀請連結及結構化規避模式。

`ban` 只允許搭配 `critical`，且必須具備必要組合訊號，避免單一模糊詞造成封鎖。首版修改 YAML 後重新啟動；未來動態重載必須先建立完整有效快照，再原子替換並保留上一版。

逐訊息查詢資料庫會增加延遲、故障耦合與連線負載，因此資料庫只保存狀態及稽核，不作詞庫熱路徑。

### 結合內容、身分與時間窗訊號

內容訊號涵蓋 URL、Telegram invite、mention、關鍵詞、網域黑白名單及正規化內容雜湊。行為訊號以 `chat_id + user_id` 計算頻率及重複發文，以 `chat_id + content_fingerprint` 計算跨帳號相似內容；入群事件只記錄 Bot 實際觀測到的時間。

管理員身分由 Telegram 管理員資料以短 TTL 快取，可信任成員由每群設定或儲存介面提供。豁免發生在偵測前，但仍輸出不含內容的豁免紀錄。白名單網域只取消該網域訊號，不得使整則訊息無條件通過。

### PostgreSQL 保存真實狀態，Redis 保存短期訊號

PostgreSQL 保存 `processed_updates`、`detection_events`、`violations`、`enforcement_actions` 與可信任成員。違規以聊天與成員分區，查詢最近 30 天有效紀錄計算階梯。訊息只保存有金鑰的內容雜湊或不可逆指紋，不保存完整文字。

Redis 保存頻率、短期重複、跨帳號指紋與快取；單機開發可使用有容量及 TTL 上限的記憶體實作。多副本正式環境不得使用程序內記憶體作共享冪等與頻率狀態。

### 處置計畫與 Telegram API 呼叫分離

application 層先在 PostgreSQL transaction 中原子建立違規、計算違規後次數並建立具唯一冪等鍵的處置計畫，再逐項呼叫 `deleteMessage`、警告訊息、`restrictChatMember` 或 `banChatMember`，每項結果分開落盤。重試只挑選未完成且可重試動作，不重複累計違規。

一般違規依最近 30 天次數執行警告、禁言 10 分鐘、禁言 24 小時與封鎖。嚴重違規只有在 `critical` 門檻及必要組合訊號同時成立時首次直接封鎖。Telegram API 無法加入資料庫 transaction，因此不能宣稱整體 exactly-once；以冪等計畫與可觀測的逐步結果達成 at-least-once 下的業務收斂。

### 三種執行模式共用相同判定

`observe` 只記錄預計結果，不建立有效違規；`delete-only` 只刪除且不推進階梯；`enforce` 建立有效違規並執行完整處置。三種模式使用同一規則快照及判定格式，使觀測結果可用於上線前校準。

### Docker Compose 提供可重現的本機整合環境

根目錄提供多階段 Dockerfile 與 `docker-compose.yaml`。Compose 定義 `app`、`postgres`、`redis` 三個服務、隔離 network、PostgreSQL named volume，以及 PostgreSQL 與 Redis 健康檢查；`app` 只在相依服務健康後啟動，並以服務名稱連線。規則 YAML 以唯讀 volume 掛載，應用程式使用非 root 使用者並提供 HTTP 健康端點。

Compose 只從 `.env` 或執行環境取得 Bot Token、Webhook secret 與資料庫密碼，儲存庫只提交 `.env.example`。本機 Webhook 若需 Telegram 從網際網路連入，另由開發者提供反向代理或 tunnel 的公開 HTTPS URL；Compose 不內建第三方 tunnel 或憑證。

只在主機直接執行 Go 與個別基礎設施的替代方案較難重現版本與健康順序，因此以 Compose 作為標準本機整合入口；正式部署仍使用目標平台的 Secret Manager 與部署定義，不直接沿用開發用 Compose。

## Risks / Trade-offs

- [規則誤判造成刪除或封鎖] → 新規則先用 `observe` 校準，直接封鎖要求嚴重類型與組合訊號，管理員及可信任成員強制豁免。
- [繁簡轉換誤解語意] → 保留原文、雙軌比對、記錄命中來源，轉換詞單獨命中不得觸發直接封鎖。
- [Telegram API 部分成功] → 處置拆成具唯一鍵的動作，分別落盤並只重試未完成項目。
- [Webhook 重送造成重複違規] → `update_id` 原子占用，違規與處置計畫另設資料庫唯一約束。
- [Redis 不可用使行為訊號缺失] → 內容規則繼續運作並記錄降級；正式環境不得無提示改用各副本記憶體。
- [惡意超長輸入或複雜模式耗盡資源] → 限制 body 與文字長度，使用線性或有界演算法，加入 fuzz、benchmark 與逾時監控。
- [Bot 權限不足導致處置失敗] → 啟動及定期檢查必要權限，個別處置失敗進入稽核與重試，不擴張權限。
- [Bot 被加入未授權聊天後誤執行處置] → 啟動設定強制要求群組允許清單，delivery 層同時驗證 `chat_id` 與 `chat.type`。
- [稽核資料可被用來重建敏感內容] → 使用有金鑰內容指紋、限制保存欄位與存取權，秘密值及完整文字永不落盤。

## Migration Plan

1. 建立 Go 模組、設定、Webhook、領域規則與 PostgreSQL migrations。
2. 建立 Docker image 與 Compose 整合環境，驗證 PostgreSQL migration、Redis、健康檢查及持久化 volume。
3. 在測試群設定 BotFather Privacy Mode、Webhook secret，並只授予刪除訊息及限制成員權限。
4. 以 `observe` 模式部署，使用匿名化樣本驗證繁簡英文規則、門檻、豁免及管理紀錄。
5. 切換 `delete-only`，確認冪等刪除與 Telegram 權限錯誤處理。
6. 切換 `enforce` 啟用 30 天違規、階梯處置及嚴重違規直接封鎖。
7. 多副本前啟用 PostgreSQL 共享冪等與 Redis 時間窗狀態，完成併發與故障測試。

回滾時先切回 `observe` 停止新處置，再回復前一版本；保留 migrations 與稽核紀錄以避免破壞追蹤鏈。需要停用服務時移除或改指 Telegram Webhook，Bot Token 若疑似洩漏則立即由 BotFather 撤銷重發。

## Open Questions

- 初始規則權重、相似度與垃圾訊息門檻仍需以更多正負樣本校準。
- 警告訊息內容與自動刪除警告的時間尚未指定，首版可採可設定模板與 TTL。
- 正式部署的 Secret Manager、PostgreSQL 與 Redis 供應方式需由執行環境決定。
