## 1. 規則模型與載入

- [x] 1.1 新增 `internal/autoreply` domain 型別，定義規則、觸發詞、回覆內容、命中結果與驗證錯誤。
- [x] 1.2 實作自動回覆獨立 YAML 規則檔載入器，支援版本、規則順序、停用規則與啟動時驗證。
- [x] 1.3 對規則載入器撰寫 table-driven tests，涵蓋有效規則、重複 ID、空觸發詞、空回覆與停用規則。
- [x] 1.4 將 auto-reply 開關與 `rules_file` 路徑加入 `internal/config`，並更新設定驗證與對應測試。

## 2. 比對與應用流程

- [x] 2.1 實作自動回覆 matcher，使用既有 normalizer 對訊息與 keyword 做繁簡、大小寫與空白正規化。
- [x] 2.2 實作 application processor，對單一訊息只選擇第一條命中規則並產生固定回覆結果。
- [x] 2.3 撰寫 matcher 與 processor 單元測試，涵蓋繁簡變體、英文大小寫、多規則命中、未命中與 Bot 訊息忽略。
- [x] 2.4 確認自動回覆不讀取或保存完整使用者原文，只在記憶體中用於本次比對。

## 3. 冪等與稽核儲存

- [x] 3.1 新增 PostgreSQL GORM model 保存自動回覆執行紀錄，包含 `chat_id`、`update_id`、`message_id`、`rule_id`、狀態、錯誤摘要與 UTC 時間。
- [x] 3.2 建立 `chat_id + update_id` 唯一鍵與必要查詢索引，並加入 AutoMigrate 清單。
- [x] 3.3 實作 auto-reply store port 與 PostgreSQL adapter，支援 claim、complete、fail 與重送查詢。
- [x] 3.4 撰寫 store 測試或整合測試，驗證重送不會重複 claim，且失敗狀態可安全重試。

## 4. Webhook 與垃圾偵測整合

- [x] 4.1 調整 spam processor 回傳處理結果，讓呼叫端能判斷訊息是否已被判定為垃圾或已執行處置。
- [x] 4.2 在 Webhook 流程中維持管理指令優先分流，其他 Bot 指令仍靜默忽略。
- [x] 4.3 在非垃圾的一般訊息後串接 auto-reply processor，並用 Telegram `SendMessage` 回覆原訊息。
- [x] 4.4 補上 Webhook 測試，涵蓋目前 Bot 指令不觸發、其他 Bot 指令不觸發、垃圾訊息不觸發、非垃圾命中成功回覆與 Telegram 重送不重複回覆。

## 5. 設定、文件與部署

- [x] 5.1 更新 `configs/config.sample.yaml`，加入 `auto_replies.enabled` 與 `auto_replies.rules_file` 範例。
- [x] 5.2 新增 `configs/auto_replies.sample.yaml`，放入「下載頁」與「app 去哪裡下載」範例規則。
- [x] 5.3 更新 `README.md`，說明自動回覆用途、獨立規則檔格式、觸發順序、垃圾訊息優先與首版限制。
- [x] 5.4 確認 Docker Compose 掛載可讀取獨立自動回覆規則檔，必要時更新掛載路徑。
- [x] 5.5 確認日誌欄位包含 `subsystem=auto_reply`、`rule_id`、`update_id`、`chat_id` 與狀態，且不包含完整原文。

## 6. 驗證

- [x] 6.1 執行 `gofmt -s` 或專案既有格式化流程。
- [x] 6.2 執行 `go test ./internal/autoreply/... ./internal/config ./internal/detection/...`。
- [x] 6.3 執行 `go test ./...`，確認管理指令與垃圾偵測既有行為未回歸。
- [ ] 6.4 在測試群組驗證「下載頁」與「app 去哪裡下載」會回覆，垃圾訊息包含相同字詞時不回覆。
