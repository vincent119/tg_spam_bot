## ADDED Requirements

### Requirement: 安全辨識 Telegram 管理指令
系統 SHALL 只在允許的 `group` 或 `supergroup` 中，依位於文字開頭的 Telegram `bot_command` entity 辨識已註冊指令，並支援 `/command` 與 `/command@bot_username` 格式。

#### Scenario: 辨識群組內指定 Bot 的指令
- **WHEN** 允許群組收到文字開頭為 `/ping@liyu_spam_bot` 且 entity 類型為 `bot_command` 的訊息
- **THEN** 系統將其解析為 `/ping` 並交由指令處理器一次

#### Scenario: 忽略其他 Bot 的指令
- **WHEN** 指令包含的 username 不是目前 Bot username
- **THEN** 系統不得執行、回覆或送入垃圾訊息偵測

#### Scenario: 處理未知指令
- **WHEN** 允許群組成員送出未註冊的 bot command
- **THEN** 系統回覆簡短的未知指令說明且不得執行任何副作用

### Requirement: 提供基礎查詢指令
系統 SHALL 允許群組成員使用 `/help`、`/ping` 與 `/id`，並對公開指令套用短時間頻率限制。

#### Scenario: 顯示指令說明
- **WHEN** 成員執行 `/help`
- **THEN** 系統依操作者權限顯示可用指令、必要參數與回覆目標要求，且不得揭露內部設定或秘密值

#### Scenario: 回報 Bot 存活
- **WHEN** 成員在允許群組執行 `/ping`
- **THEN** 系統回覆 Bot 正常及可用的最小狀態資訊，不查詢或揭露敏感基礎設施資訊

#### Scenario: 查詢群組及使用者 ID
- **WHEN** 成員執行 `/id` 且未回覆其他訊息
- **THEN** 系統回覆目前 `chat_id` 與操作者 `user_id`

#### Scenario: 查詢被回覆成員 ID
- **WHEN** 成員以 `/id` 回覆具有一般使用者發送者的訊息
- **THEN** 系統回覆目前 `chat_id` 與目標 `user_id`

#### Scenario: 公開指令超過頻率限制
- **WHEN** 同一成員在同一群組的公開指令超過設定時間窗門檻
- **THEN** 系統靜默忽略超額指令且不得影響垃圾訊息違規次數

### Requirement: 管理指令必須通過即時授權
系統 MUST 只允許目前 Telegram 身分為 `creator` 或 `administrator` 的操作者執行 `/warnings`、`/warn`、`/clearwarn`、`/del`、`/mute`、`/unmute`、`/ban` 與 `/unban`。

#### Scenario: 管理員執行管理指令
- **WHEN** 操作者在執行當下仍是該群組管理員
- **THEN** 系統繼續驗證目標與參數並執行適用指令

#### Scenario: 一般成員嘗試管理指令
- **WHEN** 非管理員執行具管理權限要求的指令
- **THEN** 系統拒絕操作、不得呼叫處置 API，並留下不含訊息內容的拒絕稽核

#### Scenario: 管理員權限已被撤銷
- **WHEN** 快取仍曾記錄操作者為管理員，但 Telegram 即時查詢顯示權限已撤銷
- **THEN** 系統以即時結果拒絕操作

### Requirement: 管理指令保護特殊目標
系統 MUST 禁止人工處置目前的 Telegram 管理員、Bot 自身、其他 Bot、匿名 sender chat 及該群組啟用的可信任成員。

#### Scenario: 嘗試封鎖管理員
- **WHEN** 管理員以 `/ban` 回覆另一名目前管理員的訊息
- **THEN** 系統拒絕操作且不得呼叫 `banChatMember`

#### Scenario: 嘗試處置可信任成員
- **WHEN** 管理員對該群組啟用的可信任成員執行 `/warn`、`/mute` 或 `/ban`
- **THEN** 系統拒絕增加違規或執行 Telegram 處置

#### Scenario: 回覆目標缺少一般使用者
- **WHEN** 管理指令回覆匿名管理員或 sender chat 發送的訊息而無法取得一般 `user_id`
- **THEN** 系統拒絕操作並說明目標不受支援

### Requirement: 查詢與調整警告紀錄
系統 SHALL 提供 `/warnings`、`/warn` 與 `/clearwarn` 管理同群組成員最近 30 天的有效違規，並區分人工與自動來源。

#### Scenario: 查詢警告次數
- **WHEN** 管理員以 `/warnings` 回覆目標成員訊息
- **THEN** 系統回覆最近 30 天有效違規總數及人工／自動來源摘要

#### Scenario: 人工增加警告
- **WHEN** 管理員以 `/warn [reason]` 回覆可處置成員且 reason 符合長度限制
- **THEN** 系統建立一筆 `manual` 有效違規、寫入操作者與原因、發送警告並顯示更新後次數，但不得在該指令內自動禁言或封鎖

#### Scenario: 人工警告影響後續自動階梯
- **WHEN** 具有人工有效違規的成員之後命中自動違規
- **THEN** 系統將最近 30 天人工及自動有效違規一併計入既有階梯次數

#### Scenario: 清除警告紀錄
- **WHEN** 管理員以 `/clearwarn [reason]` 回覆目標成員訊息
- **THEN** 系統在同一 transaction 將該成員目前有效違規標記失效並保存操作者與原因，不得實體刪除原始紀錄

### Requirement: 執行人工訊息與成員處置
系統 SHALL 提供 `/del`、`/mute`、`/unmute`、`/ban` 與 `/unban`，且人工指令在 `observe`、`delete-only` 與 `enforce` 模式均依管理員明確意圖執行。

#### Scenario: 刪除被回覆訊息
- **WHEN** 管理員以 `/del` 回覆群組內可刪除的訊息
- **THEN** 系統呼叫 `deleteMessage` 刪除目標訊息並保存人工處置結果

#### Scenario: 限時禁言成員
- **WHEN** 管理員以 `/mute 10m reason` 回覆可處置成員
- **THEN** 系統將成員限制至目前 UTC 時間加十分鐘並保存原因與結果

#### Scenario: 拒絕無效禁言時間
- **WHEN** `/mute` 缺少 duration、格式錯誤或時間不在一分鐘至七天範圍
- **THEN** 系統回覆正確語法且不得呼叫 `restrictChatMember`

#### Scenario: 解除禁言
- **WHEN** 管理員以 `/unmute` 回覆成員訊息
- **THEN** 系統透過 `restrictChatMember` 恢復該群組預設成員權限並保存結果

#### Scenario: 封鎖成員
- **WHEN** 管理員以 `/ban [reason]` 回覆可處置成員
- **THEN** 系統呼叫 `banChatMember` 並保存人工封鎖結果

#### Scenario: 解除封鎖
- **WHEN** 管理員以 `/unban` 回覆舊訊息，或提供同群組成員的有效十進位 user ID
- **THEN** 系統呼叫 `unbanChatMember` 且不得自動將該使用者重新加入群組

### Requirement: 管理指令具備冪等與稽核
系統 MUST 以 `chat_id + update_id` 收斂指令重送，並保存命令、操作者、目標、參數摘要、來源、狀態、Telegram API 結果及 UTC 時間，不保存完整目標訊息。

#### Scenario: Telegram 重送人工警告
- **WHEN** Telegram 重送已完成的相同 `/warn` update
- **THEN** 系統回傳既有結果且不得再次建立違規或發送警告

#### Scenario: Telegram 重送封鎖指令
- **WHEN** 相同 `/ban` update 在前次 Telegram API 成功後再次送達
- **THEN** 系統不得再次呼叫 `banChatMember`，並保留第一次操作結果

#### Scenario: 保存失敗操作
- **WHEN** Telegram API 因權限不足或暫時錯誤使人工處置失敗
- **THEN** 系統保存失敗類型與可重試狀態、回覆安全錯誤訊息，且不得記錄 Bot Token 或完整 Telegram response

### Requirement: 限制指令輸入與回應內容
系統 MUST 驗證指令參數數量、duration、十進位 user ID 及 reason 長度，並以繁體中文回覆穩定結果，不得將秘密值、DSN 或完整歷史訊息寫入回應或管理紀錄。

#### Scenario: 原因文字過長
- **WHEN** 管理員提供超過 200 個 Unicode code points 的 reason
- **THEN** 系統拒絕指令並不得建立違規或執行處置

#### Scenario: 指令包含未知參數
- **WHEN** 指令參數不符合該指令固定語法
- **THEN** 系統回覆該指令的正確用法且不得部分執行

#### Scenario: Telegram API 回傳敏感內容
- **WHEN** 外部 API 錯誤包含 Bot Token、URL credential 或未預期 response body
- **THEN** 系統在日誌、回應及稽核前遮蔽敏感內容
