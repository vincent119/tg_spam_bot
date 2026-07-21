## Why

Telegram 討論群持續遭受跨語言廣告、邀請導流、重複洗版及違法內容侵擾，人工巡查無法即時完成刪除、累計違規與一致處置。需要一套可解釋、可設定且具冪等性的自動管理流程，在最小權限下辨識垃圾訊息並留下可稽核紀錄。

## What Changes

- 以受秘密值保護的 HTTPS Webhook 接收 Telegram 更新，驗證來源並避免重複處理同一 `update_id`。
- 檢查 URL、Telegram 邀請連結、關鍵字、網域黑白名單、單一帳號發送頻率、短期重複內容、跨帳號相似內容及入群後立即貼連結。
- 支援繁體中文、簡體中文、英文及混合文字；保留原文，並以 Unicode 正規化、英文小寫化與繁體轉換副本進行雙軌比對。
- 以 YAML 自訂違規類型、詞彙、別名、嚴重度、組合訊號、權重、門檻與處置；啟動時載入記憶體索引。
- 允許 Telegram 管理員與設定的可信任成員略過自動偵測及處置。
- 垃圾訊息命中後呼叫 Telegram Bot API 刪除訊息，並以 30 天滾動期限累計成員違規。
- 一般違規採四階梯政策：警告、禁言 10 分鐘、禁言 24 小時、封鎖；符合組合條件的嚴重違規首次命中即刪除並封鎖。
- 提供 `observe`、`delete-only`、`enforce` 三種執行模式，並保存偵測、違規與每項處置結果的管理紀錄。
- 提供 `docker-compose.yaml`，可在本機以容器啟動應用程式、PostgreSQL 與 Redis 整合環境。

## Capabilities

### New Capabilities

- `telegram-spam-detection`: 定義安全接收更新、多語言規則偵測、可信任成員豁免、違規累計、分級處置、嚴重違規封鎖與管理稽核行為。

### Modified Capabilities

無。

## Impact

- 新增 Go Webhook 服務、Telegram Bot API Client、文字正規化與規則引擎。
- 新增 PostgreSQL 違規與稽核儲存；短期頻率、內容相似度及更新去重以可替換介面支援 Redis，單機開發可用有界記憶體實作。
- Bot Token、Webhook secret 與資料庫憑證必須來自環境變數或 Secret Manager，不得寫入程式碼、設定檔或 Git。
- Bot 在 supergroup 僅需 `can_delete_messages` 與 `can_restrict_members`；群組需允許 Bot 接收一般訊息。
- 新增多階段 Dockerfile、`docker-compose.yaml`、健康檢查、持久化 volume 與不含秘密值的 `.env.example`。
- 不包含管理後台、機器學習模型訓練、完整訊息長期保存或自動修改 Telegram 群組權限。
