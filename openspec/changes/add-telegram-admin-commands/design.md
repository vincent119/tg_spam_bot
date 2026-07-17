## Context

現有 Webhook 將群組文字直接轉成偵測訊息，application 層再進行豁免、規則判定與自動處置；Telegram Client 已支援刪除、禁言及封鎖，但尚未支援指令回覆、解除禁言與解除封鎖。PostgreSQL 已保存違規與處置紀錄，可信任成員及管理員也有既有查詢邊界。

管理指令跨越 Telegram DTO、delivery 分流、管理員授權、違規資料、Telegram API 及稽核，因此必須避免把字串解析或 Telegram 細節放進既有垃圾訊息 domain。所有指令只在 `allowed_chat_ids` 內的 `group` 或 `supergroup` 生效，並延續 Webhook secret、body 上限及 `update_id` 冪等保護。

## Goals / Non-Goals

**Goals:**

- 提供 `/help`、`/ping`、`/id`、`/warnings`、`/warn`、`/clearwarn`、`/del`、`/mute`、`/unmute`、`/ban`、`/unban`。
- 讓管理員以回覆訊息為主要目標選取方式，降低處置錯誤。
- 對副作用指令執行即時管理員授權、受保護目標檢查、輸入驗證、冪等與完整稽核。
- 保持人工操作與自動偵測來源可區分，並讓清除警告採可追蹤的失效操作。
- 讓指令功能可獨立測試，不影響一般訊息的既有偵測流程。

**Non-Goals:**

- 不建立 `/welcome`、自訂歡迎文案、規則管理後台或任意 Telegram Bot 指令框架。
- 不支援私人聊天、頻道管理、跨群組處置或一般成員自行申訴。
- 不以指令修改 YAML 偵測規則、可信任名單、Bot 權限或 `app.mode`。
- 不永久刪除違規及稽核資料。

## Decisions

### 在 delivery 層辨識指令並交由獨立 application use case

Telegram DTO 增加 `entities` 中的 `bot_command`、`reply_to_message` 與必要的發送者欄位。delivery 只在 command entity 位於文字開頭時解析命令名稱，移除可選的 `@bot_username` 後交給 `CommandHandler`；一般文字仍送入既有 `MessageProcessor`。

只用字串前綴判斷 `/` 容易把一般內容誤當指令，也無法正確處理 Telegram entity 的 UTF-16 offset，因此採 Telegram entity 作為主要依據。未知指令回傳簡短說明，不送入垃圾訊息偵測。

### 採固定指令註冊表與明確權限矩陣

`/help`、`/ping`、`/id` 允許群組成員使用；其餘指令只允許目前仍是 `creator` 或 `administrator` 的操作者。具副作用指令每次都即時呼叫 Telegram 查詢操作者身分，不使用既有五分鐘管理員快取，避免權限撤銷後仍可操作。

固定註冊表保存名稱、別名、權限、是否要求回覆目標及參數解析器。相較反射或動態註冊，固定表更容易稽核公開面與產生一致的 `/help`。

### 以回覆訊息作為主要目標，僅 `/unban` 支援明確 user ID

`/warnings`、`/warn`、`/clearwarn`、`/del`、`/mute`、`/unmute` 與 `/ban` 必須回覆目標訊息；`/unban` 可回覆舊訊息，或在訊息已不存在時接受十進位 Telegram user ID。`/id` 回覆訊息時顯示目標 user ID，未回覆時顯示操作者與 chat ID。

不在首版讓所有指令接受任意 user ID，避免複製錯誤或跨群誤用。所有目標必須是一般使用者，禁止處置 Bot 自身、Telegram 管理員及該群啟用的可信任成員。

### 人工警告會影響後續階梯，但不在 `/warn` 當下自動升級

`/warn [reason]` 建立一筆來源為 `manual` 的有效違規並發送警告，計入同群組、同成員最近 30 天總數；它本身不自動轉成禁言或封鎖，避免管理員發出警告時產生未預期處置。後續自動違規仍依包含人工警告的有效總數推進既有階梯。

`/warnings` 顯示有效總數及人工／自動來源摘要。`/clearwarn [reason]` 在 transaction 中將該成員目前有效違規標記失效並寫入操作者與原因，不刪除原始資料。替代方案是直接刪除資料，但會破壞稽核鏈，因此不採用。

### 人工處置不受自動偵測模式限制

`observe`、`delete-only`、`enforce` 只控制自動判定後的行為；已通過即時管理員授權的 `/del`、`/mute`、`/unmute`、`/ban`、`/unban` 與 `/warn` 是明確人工操作，在三種模式均執行。每筆紀錄標記 `source=manual_command`，避免與預計處置混淆。

### 嚴格定義參數與 Telegram API 語意

`/mute` 語法為 `/mute <duration> [reason]`，支援 `m`、`h`、`d`，範圍一分鐘至七天；reason 去除前後空白後最多 200 個 Unicode code points。`/warn`、`/clearwarn`、`/ban` 的 reason 選填且使用相同長度限制。`/unmute` 透過 `restrictChatMember` 恢復預設成員權限，`/unban` 使用 `unbanChatMember` 並避免自動重新加入。

所有 Telegram API 錯誤轉為穩定的繁體中文回應；原始回應只以遮蔽秘密值的結構化日誌與管理紀錄保存。

### 副作用以 command execution 冪等鍵收斂

新增 command execution persistence，以 `chat_id + update_id` 為唯一鍵保存命令、操作者、目標、狀態與結果。處理器先占用再執行；Telegram 重送時回傳既有完成結果，不重複增加警告或呼叫處置 API。違規調整與管理紀錄在同一 PostgreSQL transaction 完成，外部 Telegram API 結果則分開標記，維持既有 at-least-once 收斂策略。

### 控制公開指令濫用與回應內容

`/help`、`/ping`、`/id` 使用 Redis 依 `chat_id + user_id` 做短時間頻率限制；超過門檻時靜默忽略，避免回覆洗版。指令回應不包含 Bot Token、Webhook Secret、內部 DSN、完整訊息內容或未遮蔽的 Telegram API response。

## Risks / Trade-offs

- [管理員帳號遭入侵後可直接處置成員] → 每次即時驗證管理員、限制允許群組、保護管理員與可信任成員並完整稽核。
- [回覆訊息目標與實際成員不一致] → 僅使用 Telegram `reply_to_message.from.id`，拒絕匿名管理員、sender chat 或缺少使用者資訊的目標。
- [重送造成重複警告或處置] → command execution 唯一鍵、transaction 與逐項結果狀態收斂。
- [人工警告推進後續自動階梯] → 回應明確顯示更新後次數，`/clearwarn` 可失效且保留稽核。
- [公開 `/ping`、`/id` 造成洗版] → Redis 頻率限制、簡短回應及可選的指令訊息清理。
- [Telegram API 成功但資料庫結果標記失敗] → 保存執行中狀態並以相同冪等鍵重查；不盲目重複高風險 ban 操作。
- [GORM AutoMigrate 變更正式 schema] → 僅新增非破壞性欄位／資料表與索引，部署前先在備份還原環境驗證。

## Migration Plan

1. 新增 DTO、parser、command domain/application 邊界及純單元測試，功能保持未組裝。
2. 新增 GORM models、repository 與 AutoMigrate 清單，在測試 PostgreSQL 驗證註解、唯一鍵及回滾相容性。
3. 擴充 Telegram Client，使用 mock server 驗證 sendMessage、unmute、unban 及錯誤遮蔽。
4. 在測試群組組裝 command handler，先驗證公開唯讀指令，再驗證管理員指令與受保護目標。
5. 透過 BotFather `/setcommands` 發布指令清單，觀察稽核、錯誤率及頻率限制。
6. 發生異常時先從 Webhook 組裝移除 command handler；保留新增資料表與紀錄，不破壞既有自動偵測。

## Open Questions

- 指令成功或失敗回應是否需要設定自動刪除秒數，首版預設保留回應供管理員確認。
- `/clearwarn` 首版清除該成員全部有效違規；未來若需要逐筆清除，需另增違規 ID 查詢介面。
- BotFather 指令描述採繁體中文；是否需要依 Telegram language code 提供簡體中文與英文描述可於後續變更處理。
