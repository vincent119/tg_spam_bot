## ADDED Requirements

### Requirement: 載入可驗證的自動回覆規則
系統 SHALL 從主設定指定的獨立 YAML 規則檔載入自動回覆規則，並在啟動時驗證每條規則的唯一識別碼、啟用狀態、觸發詞、回覆內容與比對選項。

#### Scenario: 載入有效規則
- **WHEN** 獨立規則檔包含唯一 `id`、至少一個非空白觸發詞與非空白回覆內容
- **THEN** 系統建立不可變規則快照並可使用該快照處理後續一般訊息

#### Scenario: 主設定指定規則檔路徑
- **WHEN** 主設定啟用自動回覆並指定 `rules_file`
- **THEN** 系統從該路徑讀取獨立 YAML 規則檔，不得要求規則內容直接寫入主設定檔

#### Scenario: 啟用但缺少規則檔路徑
- **WHEN** 主設定啟用自動回覆但未提供規則檔路徑
- **THEN** 系統拒絕啟動並回報設定錯誤

#### Scenario: 拒絕無效規則
- **WHEN** 規則缺少 `id`、`id` 重複、觸發詞為空或回覆內容為空
- **THEN** 系統拒絕啟動並回報設定錯誤，不得載入部分自動回覆規則

#### Scenario: 停用規則
- **WHEN** 規則標記為停用
- **THEN** 系統不得讓該規則觸發回覆或寫入命中紀錄

### Requirement: 只在支援的 Telegram 群組訊息觸發
系統 MUST 只在 `telegram.allowed_chat_ids` 允許的 `group` 或 `supergroup` 一般使用者訊息中評估自動回覆。

#### Scenario: 允許群組一般訊息
- **WHEN** 允許群組收到非 Bot 使用者的一般文字或媒體說明文字
- **THEN** 系統可以依自動回覆規則評估是否回覆

#### Scenario: 忽略不支援聊天
- **WHEN** 訊息來自私人聊天、頻道、未允許群組或缺少一般使用者資訊
- **THEN** 系統不得評估自動回覆、不得送出回覆，也不得揭露允許清單狀態

#### Scenario: 忽略 Bot 訊息
- **WHEN** 訊息發送者是 Bot
- **THEN** 系統不得觸發自動回覆，避免 Bot 之間互相回覆造成循環

### Requirement: 不干擾管理指令
系統 MUST 在 Telegram 管理指令分流完成後才評估自動回覆，且不得將目前 Bot 或其他 Bot 的指令送入自動回覆。

#### Scenario: 目前 Bot 指令
- **WHEN** 訊息是 `/ping` 或 `/ping@liyu_spam_bot` 等屬於目前 Bot 的指令
- **THEN** 系統只交由管理指令流程處理，不得觸發自動回覆

#### Scenario: 其他 Bot 指令
- **WHEN** 訊息是 `/command@other_bot` 且 Telegram entity 類型為 `bot_command`
- **THEN** 系統靜默忽略該訊息，不得觸發自動回覆或垃圾訊息偵測

### Requirement: 使用正規化文字比對觸發詞
系統 SHALL 對一般訊息與觸發詞使用一致的文字正規化，支援繁體中文、簡體中文、英文大小寫與混合文字比對。

#### Scenario: 命中繁簡變體
- **WHEN** 規則觸發詞為 `下載頁` 且使用者訊息包含 `下载页在哪`
- **THEN** 系統辨識為同一語意變體並命中該規則

#### Scenario: 命中英文大小寫變體
- **WHEN** 規則觸發詞為 `app download` 且使用者訊息包含 `APP Download`
- **THEN** 系統以大小寫正規化後結果命中該規則

#### Scenario: 未命中不相關文字
- **WHEN** 使用者訊息未包含任何啟用規則的觸發詞或別名
- **THEN** 系統不得送出自動回覆

### Requirement: 與垃圾訊息偵測協調
系統 MUST 確保被判定為垃圾訊息或已執行自動處置的訊息不得觸發自動回覆。

#### Scenario: 非垃圾訊息觸發回覆
- **WHEN** 一般訊息未被判定為垃圾訊息且命中啟用自動回覆規則
- **THEN** 系統回覆該規則設定的固定文字

#### Scenario: 垃圾訊息命中回覆關鍵字
- **WHEN** 訊息同時命中垃圾訊息規則與自動回覆觸發詞
- **THEN** 系統依垃圾訊息流程處理該訊息，不得送出自動回覆

#### Scenario: 觀測模式非垃圾訊息
- **WHEN** 系統在 `observe` 模式處理未達垃圾門檻且命中自動回覆規則的訊息
- **THEN** 系統仍可送出自動回覆，因為該訊息未被判定為垃圾訊息

### Requirement: 回覆行為可預測且不洗版
系統 SHALL 對單一訊息最多送出一則自動回覆，並在多條規則命中時依設定順序選擇第一條啟用規則。

#### Scenario: 多條規則命中
- **WHEN** 同一訊息同時命中兩條或以上啟用自動回覆規則
- **THEN** 系統只回覆設定順序最前面的規則內容

#### Scenario: 重送相同更新
- **WHEN** Telegram 重送已完成且已回覆的相同 `update_id`
- **THEN** 系統不得再次送出自動回覆

#### Scenario: 自動回覆暫時失敗
- **WHEN** Telegram API 因暫時錯誤導致回覆失敗
- **THEN** 系統保存可重試狀態，並讓後續同一 `update_id` 重送可安全重試未完成回覆

### Requirement: 保存安全稽核紀錄
系統 MUST 保存自動回覆執行結果、命中規則、聊天、更新、訊息、目標回覆訊息與 UTC 時間，且不得保存完整使用者原文或秘密值。

#### Scenario: 保存成功回覆
- **WHEN** 自動回覆成功送出
- **THEN** 系統保存 `chat_id`、`update_id`、`message_id`、`rule_id`、狀態與 UTC 完成時間

#### Scenario: 保護使用者文字
- **WHEN** 系統記錄自動回覆命中或失敗
- **THEN** 日誌與資料庫紀錄不得包含完整使用者訊息原文、Bot Token、Webhook secret 或 Telegram API credential

### Requirement: 文件化自動回覆設定與限制
系統 SHALL 在設定範例與 README 中描述自動回覆規則格式、觸發順序、限制及與垃圾訊息偵測的優先順序。

#### Scenario: 管理者查閱設定方式
- **WHEN** 部署者閱讀 README、`configs/config.sample.yaml` 或 `configs/auto_replies.sample.yaml`
- **THEN** 文件清楚說明如何新增「下載頁」、「app 去哪裡下載」等固定回覆規則

#### Scenario: 管理者查閱限制
- **WHEN** 部署者閱讀自動回覆文件
- **THEN** 文件清楚說明首版不提供 AI 語意問答、不支援 Telegram 指令動態新增規則，且垃圾訊息處置優先於自動回覆
