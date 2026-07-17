## ADDED Requirements

### Requirement: 安全接收 Telegram Webhook
系統 MUST 只透過設定的 HTTPS Webhook 接收 Telegram 更新，驗證 `X-Telegram-Bot-Api-Secret-Token`，限制請求大小，並以 `update_id` 提供冪等處理。

#### Scenario: 接收有效 Webhook
- **WHEN** 請求包含正確秘密值、合法大小及可解析的 Telegram 更新
- **THEN** 系統接受更新並只執行一次處理流程

#### Scenario: 拒絕無效來源
- **WHEN** Webhook 秘密值缺失或不正確，或請求超過大小限制
- **THEN** 系統拒絕請求且不解析、偵測或處置訊息

#### Scenario: 重送相同更新
- **WHEN** Telegram 再次送達已完成或正在處理的 `update_id`
- **THEN** 系統不得重複評分、累計違規、刪除、警告、禁言或封鎖

#### Scenario: 忽略未授權或不支援的聊天
- **WHEN** 更新的 `chat_id` 不在允許清單，或 `chat.type` 不是 `group` 或 `supergroup`
- **THEN** 系統回傳成功且不得進行偵測、違規累計、處置或揭露允許清單狀態

### Requirement: 正規化可偵測訊息
系統 SHALL 從訊息文字、媒體說明文字及可用的轉傳或引用文字建立有界標準化輸入，並保留更新、聊天、訊息、發送者及 Telegram entities 識別資訊。

#### Scenario: 處理文字與媒體說明
- **WHEN** 更新包含非空白文字或媒體說明文字
- **THEN** 系統建立標準化訊息並保留 URL、mention 及 text mention entities

#### Scenario: 忽略不支援更新
- **WHEN** 更新不含可偵測內容或必要識別資訊
- **THEN** 系統記錄已忽略結果且不進行違規累計或處置

### Requirement: 支援多語言雙軌比對
系統 SHALL 支援繁體中文、簡體中文、英文及混合文字，MUST 保留原始文字，並以原文正規化結果及繁體轉換副本同時比對規則。

#### Scenario: 比對繁簡變體
- **WHEN** 簡體訊息的原文或繁體轉換副本命中設定詞彙或別名
- **THEN** 系統記錄實際命中來源且不得覆寫原始文字

#### Scenario: 比對英文與混合文字
- **WHEN** 英文大小寫變體或中英混合訊息經 Unicode 與大小寫正規化後命中規則
- **THEN** 系統回報相同的穩定規則識別碼

#### Scenario: 避免轉換詞單獨造成封鎖
- **WHEN** 只有繁體轉換副本命中單一普通詞彙且沒有必要組合訊號
- **THEN** 系統不得將該訊息判定為可直接封鎖的嚴重違規

### Requirement: 以 YAML 定義可驗證規則
系統 SHALL 從 YAML 載入具版本的違規類型、詞彙、別名、嚴重度、權重、門檻、組合條件與處置，驗證成功後建立不可變記憶體規則快照。

#### Scenario: 載入有效規則
- **WHEN** 規則具有唯一 `id`、有效嚴重度與處置、非負權重、有效門檻及規則版本
- **THEN** 系統建立完整記憶體索引並以該版本處理後續訊息

#### Scenario: 拒絕危險封鎖規則
- **WHEN** `ban` 規則不是 `critical`、未設定必要組合訊號或只依賴單一模糊詞彙
- **THEN** 系統拒絕該規則快照且不得載入部分規則

#### Scenario: 規則停用
- **WHEN** 訊息只命中已停用規則
- **THEN** 該規則不影響分數、判定或處置

### Requirement: 偵測內容與行為訊號
系統 SHALL 評估 URL、Telegram 邀請連結、關鍵字、網域黑白名單、單一成員發送頻率、短期重複內容、跨帳號相似內容及入群後貼連結等可設定訊號。

#### Scenario: 黑白名單網域判斷
- **WHEN** 訊息包含 URL
- **THEN** 系統解析正規化網域，套用精確或子網域政策，且白名單只豁免網域訊號而不豁免其他惡意訊號

#### Scenario: 單一帳號短期洗版
- **WHEN** 同一聊天中的同一成員在設定時間窗超過發送或重複內容門檻
- **THEN** 系統加入對應頻率或重複內容訊號

#### Scenario: 多帳號發送相似內容
- **WHEN** 同一聊天中多個帳號在設定時間窗發送正規化後相同或達相似門檻的內容
- **THEN** 系統加入跨帳號協同行為訊號

#### Scenario: 入群後立即貼連結
- **WHEN** 系統已知成員加入時間且該成員在設定時間內發布連結
- **THEN** 系統加入新進成員貼連結訊號

### Requirement: 豁免管理員與可信任成員
系統 MUST 在偵測前辨識 Telegram 管理員及設定的可信任成員，並略過其自動判定、違規累計與處置。

#### Scenario: 管理員發送符合規則的訊息
- **WHEN** Telegram 管理員發送會命中廣告規則的訊息
- **THEN** 系統回傳豁免結果且不刪除、不累計違規也不限制該管理員

#### Scenario: 可信任成員豁免
- **WHEN** 成員存在該聊天的啟用可信任名單
- **THEN** 系統略過自動處置並記錄豁免來源

### Requirement: 產生可解釋判定
系統 SHALL 依啟用規則累加非負分數，於分數達門檻時回傳違規類型、嚴重度、命中規則、命中來源、分數、門檻及規則版本。

#### Scenario: 多項規則命中
- **WHEN** 訊息同時命中多項規則及行為訊號
- **THEN** 系統回傳總分及不重複的完整命中依據

#### Scenario: 分數低於門檻
- **WHEN** 訊息總分低於所有適用類型門檻
- **THEN** 系統判定為非違規且不得執行刪除或限制成員

### Requirement: 支援安全執行模式
系統 MUST 支援 `observe`、`delete-only` 與 `enforce` 模式，且模式切換不得改變偵測與管理紀錄內容。

#### Scenario: 觀測模式
- **WHEN** 違規訊息在 `observe` 模式命中
- **THEN** 系統記錄預計處置但不得呼叫刪除、禁言或封鎖 API，也不得累計有效違規

#### Scenario: 僅刪除模式
- **WHEN** 違規訊息在 `delete-only` 模式命中
- **THEN** 系統只刪除訊息並記錄結果，不警告、不禁言、不封鎖且不推進階梯次數

#### Scenario: 強制執行模式
- **WHEN** 違規訊息在 `enforce` 模式命中
- **THEN** 系統依嚴重度執行刪除、累計與適用處置

### Requirement: 執行一般違規階梯處置
系統 MUST 在 `enforce` 模式成功建立一般違規紀錄後刪除訊息，並依同一聊天與成員最近 30 天的有效違規次數執行第 1 次警告、第 2 次禁言 10 分鐘、第 3 次禁言 24 小時、第 4 次及後續封鎖。

#### Scenario: 第一次一般違規
- **WHEN** 一般違規為該成員最近 30 天內第 1 次有效違規
- **THEN** 系統刪除訊息並發送警告

#### Scenario: 第二及第三次一般違規
- **WHEN** 一般違規分別為最近 30 天內第 2 次或第 3 次
- **THEN** 系統刪除訊息並分別禁言 10 分鐘或 24 小時

#### Scenario: 第四次一般違規
- **WHEN** 一般違規為最近 30 天內第 4 次或更多
- **THEN** 系統刪除訊息並封鎖該成員

#### Scenario: 舊違規超過期限
- **WHEN** 計算階梯次數時違規發生時間早於目前時間 30 天以上
- **THEN** 系統不得將該紀錄計入有效違規次數

### Requirement: 首次處置嚴重違規
系統 MUST 在 `enforce` 模式對達到 `critical` 類型門檻且符合必要組合訊號的嚴重違規，首次命中即刪除訊息並封鎖成員，不套用一般階梯。

#### Scenario: 嚴重違規符合組合條件
- **WHEN** 訊息命中嚴重詞組並同時具備設定的交易、聯絡、黑名單連結或跨帳號協同訊號
- **THEN** 系統記錄嚴重違規、刪除訊息並封鎖成員

#### Scenario: 嚴重詞彙缺少組合訊號
- **WHEN** 訊息只命中單一嚴重類型詞彙但未符合必要組合訊號
- **THEN** 系統不得直接封鎖，並依其餘規則決定是否走一般違規流程

### Requirement: 保存管理稽核紀錄
系統 MUST 保存偵測結果、有效違規、違規前後次數、預計處置、每次 Telegram API 呼叫結果、規則版本及 UTC 時間，並確保重試不重複產生有效違規或處置。

#### Scenario: 部分處置失敗
- **WHEN** 刪除成功但警告、禁言或封鎖失敗
- **THEN** 系統分別記錄每項結果與 Telegram 錯誤資訊，並允許只重試未完成且可重試的動作

#### Scenario: 查閱管理紀錄
- **WHEN** 管理流程完成或停止於錯誤
- **THEN** 紀錄可由事件識別碼、聊天、訊息及成員識別碼追蹤完整判定與處置狀態

### Requirement: 保護憑證與訊息資料
系統 MUST 從環境變數或 Secret Manager 取得 Bot Token、Webhook secret 及資料庫憑證，且一般日誌與管理紀錄不得包含任何秘密值或完整訊息內容。

#### Scenario: 記錄處理結果
- **WHEN** 系統記錄 Webhook、偵測或處置事件
- **THEN** 紀錄只包含必要識別碼、內容雜湊、判定摘要與遮罩後錯誤，不包含秘密值或完整原文

### Requirement: 提供 Docker Compose 整合環境
系統 MUST 提供 `docker-compose.yaml`，以容器啟動應用程式、PostgreSQL 與 Redis，並以健康檢查及服務相依條件避免應用程式在必要服務尚未就緒時啟動。

#### Scenario: 啟動本機整合環境
- **WHEN** 開發者提供必要環境變數並執行 `docker compose up`
- **THEN** PostgreSQL、Redis 與應用程式依健康狀態啟動，且應用程式可使用 Compose 內部服務名稱連線

#### Scenario: 缺少必要秘密值
- **WHEN** Bot Token、Webhook secret 或必要資料庫憑證未提供
- **THEN** 應用程式容器拒絕啟動且 `docker-compose.yaml` 不提供任何真實預設秘密值

#### Scenario: 保存本機資料
- **WHEN** 開發者停止並重新啟動 Compose 環境但未刪除 volumes
- **THEN** PostgreSQL 資料仍然存在，Redis 短期偵測狀態可依設定決定是否持久化
