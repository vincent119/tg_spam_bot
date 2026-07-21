# Telegram 垃圾訊息管理機器人

以 Go 實作的 Telegram supergroup 管理服務，支援繁體中文、簡體中文、英文及混合文字偵測。系統透過 Webhook 接收訊息，檢查網址、邀請連結、關鍵詞、發送頻率、重複內容與跨帳號協同行為，並依政策刪除訊息、警告、禁言或封鎖成員。

HTTP 路由使用 Gin，PostgreSQL 使用 GORM 與 `AutoMigrate`，短期行為狀態使用 Redis，結構化日誌使用 `github.com/vincent119/zlogger`。

## 功能

- 支援繁體中文、簡體中文、英文及混合文字。
- 保留原始文字，以正規化原文及繁體轉換副本雙軌比對。
- 偵測 URL、`t.me`、Telegram 邀請連結、mention、關鍵詞及網域名單。
- 偵測短時間高頻發文、同帳號重複內容及跨帳號相同內容。
- Telegram 管理員與 `trusted_members` 可信任成員略過偵測。
- 以 `update_id`、事件及處置鍵避免 Webhook 重送造成重複處置。
- 保存偵測結果、違規、處置結果與規則版本，不保存完整訊息內容。

## 執行模式

| 模式 | 行為 |
|---|---|
| `observe` | 只記錄判定，不刪除、不累計有效違規、不限制成員 |
| `delete-only` | 只刪除垃圾訊息，不推進違規階梯 |
| `enforce` | 刪除訊息並執行完整違規階梯或嚴重違規處置 |

一般違規以同一群組及成員最近 30 天紀錄計算：

1. 第一次：刪除訊息並警告。
2. 第二次：刪除訊息並禁言 10 分鐘。
3. 第三次：刪除訊息並禁言 24 小時。
4. 第四次起：刪除訊息並封鎖。

符合 `critical` 門檻及必要組合訊號的嚴重違規，第一次即刪除訊息並封鎖。

## 系統需求

- Go 1.25 以上版本，僅直接執行程式時需要。
- PostgreSQL 17，其他相容版本需自行驗證。
- Redis 8，其他相容版本需自行驗證。
- 可供 Telegram 連入的公開 HTTPS 網址。
- Docker 與 Docker Compose，使用容器部署時需要。

## 快速啟動 Docker Compose

### 1. 建立環境檔

```sh
cp .env.example .env
```

至少替換以下值：

```env
POSTGRES_PASSWORD=請設定安全的資料庫密碼
TELEGRAM_BOT_TOKEN=由BotFather取得的Token
TELEGRAM_WEBHOOK_SECRET=請使用下方命令產生
CONTENT_HASH_KEY=請使用下方命令產生
```

產生 Webhook secret 與內容雜湊金鑰：

```sh
openssl rand -hex 32
openssl rand -hex 32
```

兩個值必須分開產生，不得共用。`CONTENT_HASH_KEY` 至少 32 字元，正式環境啟用後應固定保存；任意更換會使相同訊息產生不同內容指紋。

### 2. 檢查並啟動

```sh
docker compose config
docker compose up --build -d
docker compose ps
docker compose logs -f app
```

`log.level: debug` 時，每個通過安全驗證的 Telegram Update 會記錄 Webhook 接收事件；管理指令另記錄接收、完成、重送、限流或失敗結果。日誌固定使用 `request_id=tg:<update_id>` 串接 Webhook、command 與垃圾訊息偵測流程，並包含 `subsystem`、`update_id`、`chat_id`、`command`、`status` 等結構化欄位。基於安全要求，不記錄 Bot Token、Webhook secret、指令參數、原因或 Telegram 訊息原文。

啟動時出現 `資料庫結構同步完成` 代表 GORM AutoMigrate 已成功；失敗時應用程式會在 HTTP Server 啟動前結束。查詢最近五分鐘日誌：

```sh
docker compose logs app --since 5m
```

PostgreSQL 與 Redis 健康後，應用程式才會啟動。PostgreSQL 資料與 Redis 狀態分別保存於 named volume。

停止服務但保留資料：

```sh
docker compose down
```

除非確定要刪除全部本機資料，否則不要執行 `docker compose down -v`。

## 建立 Telegram Bot

### 1. 取得 Bot Token

1. 在 Telegram 搜尋官方帳號 `@BotFather`。
2. 傳送 `/newbot`。
3. 依提示輸入 Bot 顯示名稱與以 `bot` 結尾的 username。
4. 保存 BotFather 回傳的 Bot Token，設定至 `TELEGRAM_BOT_TOKEN` 或 Secret Manager。

Bot Token 等同帳號密碼，不得寫入程式碼、YAML、日誌、Git commit 或 issue。若疑似洩漏，立即在 BotFather 撤銷並重新產生。

### 2. 關閉 Privacy Mode

1. 對 `@BotFather` 傳送 `/setprivacy`。
2. 選擇本服務使用的 Bot。
3. 選擇 `Disable`。
4. 確認 BotFather 回覆 Privacy Mode 已停用。

關閉後，Bot 才能接收群組的一般訊息。若 Bot 已在群組內，建議移出後重新加入，或重新設定管理員權限，避免舊設定尚未生效。

### 3. 加入 supergroup 並授予最小權限

將 Bot 加入目標 supergroup 並設為管理員，只授予：

- 刪除訊息，對應 `can_delete_messages`。
- 限制或封鎖成員，對應 `can_restrict_members`。

不要授予新增管理員、修改群組資料、管理語音聊天或其他不需要的權限。

### 4. 查詢群組 ID 與類型

服務只處理 `telegram.allowed_chat_ids` 清單內，且 `chat.type` 為 `group` 或 `supergroup` 的訊息。私人聊天與頻道不會進入偵測、違規累計或處置流程。

公開群組已有 username 時，可直接呼叫 `getChat`：

```sh
export TELEGRAM_BOT_TOKEN='由BotFather取得的Token'
curl -fsS "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getChat" \
  --data-urlencode 'chat_id=@群組username'
```

回應中的 `result.id` 是要填入的值，`result.type` 用來確認聊天類型。例如：

```json
{"ok":true,"result":{"id":-1001234567890,"type":"supergroup","title":"測試群組"}}
```

私人群組沒有 username 時，可暫時移除 Webhook，再由群組成員發送一則測試訊息：

```sh
curl -fsS "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/deleteWebhook" \
  --data-urlencode 'drop_pending_updates=false'
curl -fsS "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getUpdates"
```

從 `result[].message.chat.id` 取得 ID，並以 `result[].message.chat.type` 確認類型。取得後必須重新執行下方 `setWebhook`；Webhook 啟用期間不能使用 `getUpdates`。

Telegram 常見聊天類型如下：

| `chat.type` | 說明 | 本服務是否處理 |
|---|---|---|
| `private` | Bot 與使用者的私人聊天 | 否 |
| `group` | 基本群組 | 是 |
| `supergroup` | 超級群組，建議使用 | 是 |
| `channel` | 頻道；貼文通常由 `channel_post` 傳送 | 否 |

超級群組與頻道的 ID 通常以 `-100` 開頭，基本群組 ID 通常是其他負數；前綴不是可靠的類型判斷方式，必須查看 `chat.type`。

設定一個或多個允許群組：

```yaml
telegram:
  allowed_chat_ids:
    - -1001234567890
    - -1009876543210
```

環境變數使用逗號分隔且不要加空格：

```sh
export TELEGRAM_ALLOWED_CHAT_IDS='-1001234567890,-1009876543210'
```

## 設定 Telegram Webhook

Webhook 必須是 Telegram 可連入的公開 HTTPS URL，完整路徑固定為：

```text
https://你的網域/telegram/webhook
```

先在目前 shell 設定值：

```sh
export TELEGRAM_BOT_TOKEN='由BotFather取得的Token'
export TELEGRAM_WEBHOOK_SECRET='與服務設定相同的Secret'
export PUBLIC_BASE_URL='https://你的網域'
```

註冊 Webhook：

```sh
curl -fsS "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
  --data-urlencode "url=${PUBLIC_BASE_URL}/telegram/webhook" \
  --data-urlencode "secret_token=${TELEGRAM_WEBHOOK_SECRET}" \
  --data-urlencode 'allowed_updates=["message"]'
```

查詢 Bot 身分與 Webhook 狀態：

```sh
curl -fsS "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getMe"
curl -fsS "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getWebhookInfo"
```

`getWebhookInfo` 的 `url` 必須正確，`pending_update_count` 應持續下降，`last_error_message` 應為空。Webhook 與 long polling 不能同時使用。

移除 Webhook：

```sh
curl -fsS "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/deleteWebhook" \
  --data-urlencode "drop_pending_updates=false"
```

## Telegram 管理指令

服務啟動時會透過 `getMe` 取得 Bot ID 與 username；若 Bot Token 無效或 Telegram 無法回應，服務會拒絕啟動。指令只在 `telegram.allowed_chat_ids` 內的 `group` 或 `supergroup` 生效。

| 指令 | 權限 | 用法 |
|---|---|---|
| `/help` | 全部成員 | 顯示目前身分可用的指令 |
| `/ping` | 全部成員 | 確認 Bot 正常運作 |
| `/id` | 全部成員 | 顯示群組與自己 ID；回覆訊息時顯示目標 ID |
| `/warnings` | 管理員 | 回覆成員訊息，查詢最近 30 天有效警告 |
| `/warn [原因]` | 管理員 | 回覆成員訊息，人工增加一次警告 |
| `/clearwarn [原因]` | 管理員 | 回覆成員訊息，將目前有效警告標記失效 |
| `/del` | 管理員 | 回覆要刪除的訊息 |
| `/mute <時間> [原因]` | 管理員 | 回覆成員訊息；時間支援 `m`、`h`、`d`，範圍一分鐘至七天 |
| `/unmute` | 管理員 | 回覆成員訊息，解除禁言 |
| `/ban [原因]` | 管理員 | 回覆成員訊息，封鎖成員 |
| `/unban <user_id>` | 管理員 | 回覆舊訊息或提供十進位 user ID，解除封鎖 |

群組中 Telegram 可能送出 `/command@liyu_spam_bot`，服務會自動驗證 suffix；其他 Bot 的指令會靜默忽略。具副作用指令每次都向 Telegram 即時確認操作者仍是管理員，且禁止處置管理員、Bot 與可信任成員。

匿名管理員或以群組身份發出的管理指令不會執行。這類訊息通常無法提供真實操作者的 user ID，服務無法用 `getChatMember` 驗證該真人仍是管理員，也無法在 `command_executions` 中保存可稽核的 `operator_id`。需要執行 `/del`、`/ban`、`/mute`、`/warn` 等管理指令時，請管理員改用個人身份送出指令。

實際在群組輸入時，可使用 `/ping` 或 `/ping@liyu_spam_bot`。若同一群組有多個 Bot，建議使用帶 username 的格式，例如：

```text
/ping@liyu_spam_bot
```

需要指定目標的指令必須「回覆」目標訊息後再送出指令。以刪除垃圾訊息為例：

1. 在 Telegram 長按或右鍵點選要刪除的垃圾訊息。
2. 選擇「回覆」。
3. 輸入 `/del` 或 `/del@liyu_spam_bot`。
4. 送出指令。

`/del` 不接受把垃圾訊息內容接在指令後方，以下寫法不會刪除該訊息：

```text
/del@liyu_spam_bot 這裡放垃圾訊息內容
```

人工 `/warn` 會計入同群組成員最近 30 天的違規總數，影響後續自動違規階梯，但不會在該指令當下自動升級為禁言或封鎖。`/clearwarn` 不刪除資料，只保存失效時間、操作者與原因。人工指令代表管理員明確意圖，因此不受 `observe`、`delete-only`、`enforce` 自動偵測模式限制。

### 設定 BotFather 指令清單

對 `@BotFather` 執行 `/setcommands`，選擇 Bot 後貼上：

```text
help - 查看指令說明
ping - 檢查機器人是否在線
id - 查看群組與使用者 ID
warnings - 查看最近 30 天警告
warn - 人工增加警告
clearwarn - 失效目前警告
del - 刪除被回覆訊息
mute - 限時禁言成員
unmute - 解除成員禁言
ban - 封鎖成員
unban - 解除成員封鎖
```

BotFather 清單只影響 Telegram 選單顯示，真正權限仍由服務端即時驗證。

## 自動回覆規則

自動回覆用來處理固定問答，例如成員詢問「下載頁在哪」或「app 去哪裡下載」。首版只做明確關鍵字比對，不使用 AI 語意問答，也不支援 Telegram 指令動態新增規則。

主設定只放開關與獨立規則檔路徑：

```yaml
auto_replies:
  enabled: true
  rules_file: configs/auto_replies.yaml
```

規則檔可由範例複製：

```sh
cp configs/auto_replies.sample.yaml configs/auto_replies.yaml
```

規則格式：

```yaml
version: "2026-07-20.1"
rules:
  - id: download_page
    enabled: true
    keywords:
      - 下載頁
      - app 去哪裡下載
      - app 在哪下載
      - 下載 app
    reply: "下載頁在：https://example.com/download"
```

同一則訊息命中多條規則時，只回覆設定順序最前面的啟用規則。系統會先跑垃圾訊息偵測；若訊息被判定為垃圾或已執行自動處置，不會再觸發自動回覆。

## 應用設定

設定範例位於 `configs/config.sample.yaml`。實際執行可透過 `CONFIG_FILE` 指定 YAML；環境變數優先於 YAML，適合用來注入秘密值。

主要環境變數：

| 環境變數 | 用途 | 必要性 |
|---|---|---|
| `CONFIG_FILE` | YAML 設定檔路徑 | 直接執行時建議設定 |
| `APP_MODE` | `observe`、`delete-only` 或 `enforce` | 否，預設 `observe` |
| `HTTP_ADDR` | 完整監聽位址，例如 `:8080` | 否 |
| `TELEGRAM_BOT_TOKEN` | Telegram Bot Token | 是 |
| `TELEGRAM_WEBHOOK_SECRET` | Webhook Secret Header 驗證值 | 是 |
| `TELEGRAM_ALLOWED_CHAT_IDS` | 允許處理的群組 ID，包含多筆時以逗號分隔 | 是 |
| `DATABASE_URL` | 完整 PostgreSQL DSN，設定後覆蓋 DB 分項 | 擇一 |
| `DB_NAME`、`DB_HOST`、`DB_PORT` | PostgreSQL 位置 | 擇一 |
| `DB_USER`、`DB_PASSWORD` | PostgreSQL 憑證 | 擇一 |
| `REDIS_ADDR` | Redis `host:port` | 是 |
| `REDIS_USERNAME` | Redis 6+ ACL username | 否 |
| `REDIS_PASSWORD` | Redis Client 密碼 | 否 |
| `REDIS_REQUIREPASS` | `password` 為空時的相容密碼 | 否 |
| `REDIS_DB` | Redis logical database | 否，預設 `0` |
| `CONTENT_HASH_KEY` | 產生不可逆內容指紋的金鑰 | 是，至少 32 字元 |
| `RULES_DIR` | 規則 YAML 目錄 | 否 |
| `LOG_LEVEL` | 日誌等級，例如 `debug`、`info` | 否，預設 `info` |
| `LOG_FORMAT` | 日誌編碼格式，支援 `json` 或 `console` | 否，預設 `json` |
| `LOG_OUTPUTS` | 日誌輸出目的地，支援 `console`、`file` | 否，預設 `console` |
| `LOG_PATH` | file output 使用的日誌目錄 | 否，預設 `./logs` |
| `LOG_FILE` | file output 使用的日誌檔名 | 否，預設由 logger 決定 |
| `LOG_MAX_FILES` | Deprecated；未啟用 rotate 時，啟動時保留的 `.log` 檔案數量 | 否，預設 `0` |
| `LOG_ROTATE_ENABLED` | 是否啟用應用層檔案日誌輪轉 | 否，預設 `false` |
| `LOG_ROTATE_MAX_SIZE_MB` | 單一日誌檔案大小上限；`0` 表示使用預設 `100` | 否，預設 `100` |
| `LOG_ROTATE_MAX_BACKUPS` | 保留輪轉備份數；`0` 表示不限制備份數 | 否，預設 `14` |
| `LOG_ROTATE_MAX_AGE_DAYS` | 保留輪轉日誌天數；`0` 表示不依天數刪除 | 否，預設 `30` |
| `LOG_ROTATE_COMPRESS` | 是否壓縮輪轉後的舊日誌 | 否，預設 `true` |

### 日誌設定

`log.format` 只控制日誌內容格式，支援 `json` 與 `console`。`log.outputs` 控制輸出目的地，支援 `console` 與 `file`，兩者可以同時啟用。

```yaml
log:
  level: info
  format: json
  outputs:
    - console
    - file
  path: ./logs
  file: app.log
```

檔案日誌輪轉由 `log.rotate` 控制，只有 `outputs` 包含 `file` 且 `rotate.enabled=true` 時才生效：

```yaml
log:
  outputs:
    - console
    - file
  rotate:
    enabled: true
    max_size_mb: 100
    max_backups: 14
    max_age_days: 30
    compress: true
```

`log.max_files` 是 deprecated 相容欄位。`rotate.enabled=false` 且 `max_files>0` 時，服務啟動時仍會保留最新的 `.log` 檔案並清理較舊檔案；`rotate.enabled=true` 時不會執行這個舊清理流程，避免與正式輪轉重疊。

Redis Client 優先使用 `REDIS_PASSWORD`，只有其為空時才使用 `REDIS_REQUIREPASS`。`requirepass` 原本是 Redis Server 設定名稱，保留此欄位只為相容既有部署命名。

## PostgreSQL 初始化

外部 PostgreSQL 只需先建立應用程式使用者與 database，不需要手動建立資料表：

```sql
CREATE USER tg_spam WITH PASSWORD '請替換為安全密碼';
CREATE DATABASE tg_spam OWNER tg_spam;
```

應用程式帳號需要 database 連線權限，以及目標 schema 的 `USAGE`、`CREATE` 和其建立物件的讀寫權限。建議讓應用程式帳號成為 database 與 schema owner。

服務啟動時，GORM `AutoMigrate` 會建立：

- `processed_updates`
- `detection_events`
- `violations`
- `enforcement_actions`
- `trusted_members`
- `command_executions`

`violations` 會新增人工來源、管理員、原因及失效稽核欄位；`command_executions` 保存 `chat_id + update_id` 冪等鍵與人工操作結果。同時建立索引、約束及資料表與欄位註解。專案目前以 GORM model 作為 schema 來源，不再提供或執行獨立 SQL migration。

正式環境升級前應先備份 PostgreSQL，並在備份還原環境執行新版 GORM AutoMigrate。回滾應先回復舊版 app；新增欄位與 `command_executions` 保留，不需刪除，也不影響舊版既有查詢。

Docker Compose 會自動建立 `tg_spam` 使用者與 database，不需要額外執行上述 SQL。

## Redis

Redis 保存發送頻率、短期重複內容與跨帳號內容指紋，資料具有 TTL，不是永久稽核來源。永久偵測、違規與處置紀錄保存於 PostgreSQL。

專案內建的 Docker Compose Redis 預設未啟用密碼。連線外部 Redis 或啟用 ACL 時，使用 `REDIS_USERNAME` 與 `REDIS_PASSWORD`；若沿用 `requirepass` 命名，可改用 `REDIS_REQUIREPASS`。

## 規則設定

規則位於 `configs/rules/*.yaml`。服務只在啟動時載入，修改後必須重新啟動。所有檔案會合併為完整快照，任一規則無效時服務拒絕啟動。

規則主要欄位：

```yaml
version: "2026-07-17.1"
categories:
  - id: generic_ad
    name: 一般廣告
    severity: normal
    action: progressive
    threshold: 60
    weight: 40
    enabled: true
    terms: [免費領取, guaranteed profit]
    aliases: [免费领取]
    require_any: []
```

注意事項：

- `id` 必須唯一。
- `ban` 只允許搭配 `critical`。
- `critical + ban` 必須設定 `require_any`，避免單一模糊詞直接封鎖。
- 大量詞彙放在 YAML 並於啟動時編譯，訊息處理時不查詢資料庫。
- 新規則應先使用 `observe` 模式校準，再逐步切換模式。

## 可信任成員

Telegram 管理員會自動略過偵測。其他可信任成員可寫入 PostgreSQL：

```sql
INSERT INTO trusted_members (chat_id, user_id, reason, enabled, created_at)
VALUES (-1001234567890, 123456789, '群組可信任成員', true, now())
ON CONFLICT (chat_id, user_id)
DO UPDATE SET reason = EXCLUDED.reason, enabled = EXCLUDED.enabled;
```

`chat_id` 與 `user_id` 必須使用 Telegram 實際識別碼，可信任關係以群組為範圍。

## HTTP 端點與健康檢查

| 方法 | 路徑 | 用途 | 成功狀態 |
|---|---|---|---|
| `POST` | `/telegram/webhook` | 接收 Telegram Update | `204` |
| `GET` | `/health/live` | 確認程序仍在運作 | `204` |
| `GET` | `/health/ready` | 確認 PostgreSQL 與 Redis 可用 | `204` |

檢查本機健康狀態：

```sh
curl -i http://127.0.0.1:8080/health/live
curl -i http://127.0.0.1:8080/health/ready
```

服務啟動時會以 `getMe` 驗證 Telegram Bot 身分；目前 `/health/ready` 尚未驗證 Webhook URL、`can_delete_messages` 或 `can_restrict_members`。部署時仍需使用 `getWebhookInfo` 及群組管理員設定人工確認。

目前服務只有 Telegram Webhook 與健康端點，尚未提供管理 REST API，因此沒有 Swagger UI 或 OpenAPI 文件端點。

## 直接執行

`make run` 預設讀取 `configs/config.yaml`：

```sh
export TELEGRAM_BOT_TOKEN='由BotFather取得的Token'
export TELEGRAM_WEBHOOK_SECRET='WebhookSecret'
export DB_USER='tg_spam'
export DB_PASSWORD='資料庫密碼'
export REDIS_ADDR='127.0.0.1:6379'
export CONTENT_HASH_KEY="$(openssl rand -hex 32)"
make run
```

指定其他設定檔：

```sh
CONFIG_FILE=/path/to/config.yaml make run
```

不要在正式環境每次啟動時重新產生 `CONTENT_HASH_KEY`；上述命令只用於首次建立本機開發秘密值。

## Docker 映像

預設映像為 `docker.io/vincent119/tg_spam_bot:latest`：

```sh
make docker-login
make docker-build
make docker-push
```

指定版本並建置、推送：

```sh
DOCKER_TAG=v1.0.0 make docker-publish
```

Docker 登入採互動方式，密碼不得放入 Makefile、命令列、Git 或 CI 明文變數。

## 開發驗證

```sh
make fmt
make vet
make lint
make test
make cover
make bench
```

PostgreSQL Repository 整合測試只有在設定 `TEST_DATABASE_URL` 時執行。正式 CI 應提供隔離的 PostgreSQL 與 Redis 服務。

## 常見問題

### `security.content_hash_key: must contain at least 32 characters`

`CONTENT_HASH_KEY` 未設定或長度不足。執行 `openssl rand -hex 32` 產生固定金鑰，保存於 `.env` 或 Secret Manager。

### Bot 收不到一般群組訊息

確認 `/setprivacy` 已選擇 `Disable`，Bot 已重新加入群組、具有管理員身分、群組 ID 已列於 `TELEGRAM_ALLOWED_CHAT_IDS`，而且 Webhook 的 `allowed_updates` 包含 `message`。

### 群組訊息收到 `204` 但未執行偵測

確認更新內的 `message.chat.id` 已列於 `telegram.allowed_chat_ids`，且 `message.chat.type` 是 `group` 或 `supergroup`。未授權群組、私人聊天與頻道會刻意回傳 `204` 並忽略，避免 Telegram 重送及洩漏允許清單狀態。

### Webhook 持續失敗

依序確認公開網址使用有效 HTTPS、反向代理轉送至 `/telegram/webhook`、服務與 `setWebhook` 使用相同 secret，並查看 `getWebhookInfo` 的 `last_error_message`。

### Bot 無法刪除、禁言或封鎖

確認 Bot 在目標 supergroup 具有 `can_delete_messages` 與 `can_restrict_members`。不要以 Privacy Mode 設定取代管理員權限。

### 管理指令沒有反應

確認指令具有 Telegram `bot_command` entity、群組位於允許清單、指令 suffix 是目前 Bot username，而且操作者仍是群組管理員。`/warnings`、`/warn`、`/clearwarn`、`/del`、`/mute`、`/unmute`、`/ban` 必須回覆目標訊息；其他 Bot 的指令與超過公開指令頻率限制的請求會靜默忽略。

### PostgreSQL 啟動時 AutoMigrate 失敗

確認 database 已存在、帳號是 database 或 schema owner，並具有 `USAGE` 與 `CREATE`。若既有環境曾使用舊版 SQL 腳本建立資料表，需先處理約束名稱相容性，不要直接刪除正式資料。

### Redis 驗證失敗

確認 `REDIS_USERNAME`、`REDIS_PASSWORD`、`REDIS_REQUIREPASS` 與 `REDIS_DB` 符合 Redis Server 設定。ACL 環境通常同時需要 username 與 password。

## 安全注意事項

- Bot Token、Webhook secret、資料庫密碼、Redis 密碼及內容雜湊金鑰不得提交 Git。
- Webhook 必須使用 HTTPS，並設定 `secret_token`。
- Bot 僅授予刪除訊息及限制成員權限。
- 管理紀錄只保存必要識別碼、內容指紋與判定摘要，不保存完整訊息。
- 正式環境先使用 `observe` 校準規則，確認誤判率後再切換模式。
