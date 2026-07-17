# Telegram 垃圾訊息管理機器人

以 Go 實作的 Telegram supergroup 管理服務，支援繁體中文、簡體中文、英文及混合文字偵測，並提供刪除、30 天違規累計、警告、禁言與封鎖。

HTTP 路由使用 Gin，PostgreSQL 存取使用 GORM 並在啟動時執行 `AutoMigrate`，結構化日誌使用 `github.com/vincent119/zlogger`。

## Telegram 設定

1. 在 `@BotFather` 執行 `/newbot` 取得 Bot Token。
2. 執行 `/setprivacy` 關閉 Privacy Mode。
3. 將 Bot 加入 supergroup 並只授予刪除訊息與限制成員權限。
4. 使用 Telegram `setWebhook` 設定公開 HTTPS URL 與 `secret_token`，路徑為 `/telegram/webhook`。

Bot Token 與 Webhook secret 只能放在環境變數或 Secret Manager，不得提交 Git。

## 應用設定

設定範例位於 `configs/config.sample.yaml`，採 `app`、`log`、`db`、`telegram`、`redis`、`security` 與 `rules` 分層。可透過 `CONFIG_FILE` 指定設定檔；環境變數優先於 YAML。

資料庫可使用 `DB_NAME`、`DB_HOST`、`DB_PORT`、`DB_USER`、`DB_PASSWORD` 分別覆寫，也可用 `DATABASE_URL` 整體覆寫。Bot Token、Webhook secret、資料庫密碼及內容雜湊金鑰不得寫入 YAML。

## 執行模式

- `observe`：只記錄判定，不累計或處置。
- `delete-only`：只刪除垃圾訊息，不推進違規階梯。
- `enforce`：一般違規依序警告、禁言 10 分鐘、禁言 24 小時及封鎖；嚴重違規首次直接封鎖。

## Docker Compose

複製 `.env.example` 為 `.env`，替換所有範例秘密值，再執行：

```sh
docker compose config
docker compose up --build
```

Compose 會啟動 app、PostgreSQL 與 Redis。PostgreSQL 使用 named volume 保存資料；規則目錄以唯讀方式掛載。本機仍需另行提供可讓 Telegram 連入的公開 HTTPS 反向代理或 tunnel。

PostgreSQL 預設只映射到 `127.0.0.1:55432` 供整合測試使用，可用 `POSTGRES_PORT` 覆寫，容器內連線仍使用 `postgres:5432`。

## 規則

規則位於 `configs/rules/*.yaml`。修改時必須更新版本；`ban` 只允許 `critical` 且必須設定 `require_any` 組合訊號。訊息處理使用啟動時建立的記憶體快照，不逐則查詢資料庫。

## 開發驗證

```sh
make fmt
make vet
make test
make cover
```

服務提供 `/health/live` 與 `/health/ready` 健康端點。管理紀錄只保存內容指紋與判定摘要，不保存完整訊息或秘密值。

## 測試覆蓋率

目前核心規則、Webhook、application policy 與記憶體狀態已有單元測試；整體 coverage 尚未達 80%，主要缺口為 PostgreSQL、Redis、程序啟動及 Telegram 外部 API 的整合路徑。Repository 整合測試可透過 `TEST_DATABASE_URL` 啟用，後續應在 CI 提供隔離的 PostgreSQL 與 Redis 服務後再提高門檻。
