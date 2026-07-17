## 1. 專案與設定基礎

- [x] 1.1 初始化 Go 1.25+ 模組及 `cmd/tg-spam-bot`、domain、application、delivery、infra 分層，加入格式化、lint、race test 與 coverage 指令
- [x] 1.2 定義 Viper 應用設定、`observe`、`delete-only`、`enforce` 模式與環境變數對應，提供不含秘密值的範例設定
- [x] 1.3 定義分類 YAML schema 與分檔詞庫，實作 `id`、嚴重度、處置、權重、門檻、組合訊號、別名及版本的完整快照驗證
- [x] 1.4 加入 Bot Token、Webhook secret、PostgreSQL 與 Redis 憑證缺失驗證，確保秘密值不出現在設定輸出或日誌

## 2. 多語言規則引擎

- [x] 2.1 定義標準化訊息、內容版本、規則、訊號、判定與命中來源等領域型別，並在 slice/map 邊界複製資料
- [x] 2.2 實作 Unicode、大小寫、全半形及有界干擾符號正規化，保留原文並產生繁體轉換副本
- [x] 2.3 封裝中文轉換介面，評估並釘選相容 Go 1.25 且授權可接受的繁簡轉換套件，加入歧義與地區詞彙測試
- [x] 2.4 將有效 YAML 規則編譯為不可變記憶體搜尋索引，實作原文與轉換副本雙軌比對、aliases 及穩定命中識別碼
- [x] 2.5 實作 URL、Telegram 邀請與 mention、網域黑白名單、關鍵詞、重複字元或符號及交易導流訊號
- [x] 2.6 實作加權分類與門檻判定，強制 `critical + ban` 必須符合必要組合訊號，並輸出可解釋結果
- [x] 2.7 使用提供的繁簡廣告樣本與正常對照建立 table-driven tests，並加入混合英文、Unicode 規避、fuzz test 與 benchmark

## 3. 行為偵測與豁免

- [x] 3.1 定義頻率、同帳號重複、跨帳號內容指紋與入群時間的狀態介面及 TTL 設定
- [x] 3.2 實作有界記憶體狀態 adapter，確保併發安全、容量受限且清理 goroutine 可由 context 結束
- [x] 3.3 實作 Redis 狀態 adapter，以原子操作計算時間窗頻率、重複內容及跨帳號協同訊號
- [x] 3.4 實作 Telegram 管理員短期快取與每群可信任成員查詢，在偵測前完成豁免
- [x] 3.5 撰寫併發及時間控制測試，涵蓋頻率臨界值、TTL 到期、多帳號相似內容、未知入群時間與豁免

## 4. PostgreSQL 違規與稽核

- [x] 4.1 建立版本化 up/down migrations，定義 processed updates、detection events、violations、enforcement actions 與 trusted members 表及唯一約束
- [x] 4.2 實作 PostgreSQL repositories，在 transaction 中原子占用更新、建立有效違規、計算最近 30 天次數及建立冪等處置計畫
- [x] 4.3 實作有金鑰內容指紋與資料最小化，確保完整訊息、Bot Token、Webhook secret 與資料庫憑證不落盤
- [x] 4.4 撰寫 repository 整合測試，涵蓋 30 天邊界、並發違規、重複 update、唯一處置鍵、rollback 與部分狀態恢復

## 5. Telegram Webhook 與 API Client

- [x] 5.1 定義必要 Telegram Update、Message、Entity、ChatMember 與 API response DTO，使用 `DisallowUnknownFields` 的範圍需兼顧 Telegram 向下相容新增欄位
- [x] 5.2 實作 HTTPS Webhook handler，以固定時間比較驗證 secret Header、限制 body、解析文字或 caption，並映射成功、拒絕及可重試錯誤
- [x] 5.3 實作 Telegram HTTP Client 與重用 Transport、逾時及受限重試，支援 `deleteMessage`、警告、`restrictChatMember`、`banChatMember`、管理員及 Bot 權限查詢
- [x] 5.4 以 `httptest` 驗證 Webhook 秘密值、過大請求、重送 update、Telegram 4xx、5xx、429、逾時及回應內容遮罩

## 6. 處置流程

- [x] 6.1 實作 application 用例，依序完成更新占用、豁免、偵測、模式政策、違規建立、處置計畫與完成標記
- [x] 6.2 實作一般違規第 1 次警告、第 2 次禁言 10 分鐘、第 3 次禁言 24 小時、第 4 次起封鎖的 30 天政策
- [x] 6.3 實作嚴重違規首次直接刪除並封鎖，缺少必要組合訊號時降回一般判定
- [x] 6.4 實作處置 worker，逐項執行並保存 Telegram API 結果，只重試未完成且可重試的冪等動作
- [x] 6.5 以 mock 與整合測試覆蓋三種模式、一般四階梯、嚴重直接封鎖、管理員豁免、部分失敗、重送及重試收斂

## 7. 組裝、部署與驗證

- [x] 7.1 以依賴注入組裝設定、規則快照、repositories、Redis 或記憶體狀態、Telegram Client、Webhook server 與 worker
- [x] 7.2 使用 `signal.NotifyContext` 或 `commons/graceful` 實作 SIGINT、SIGTERM 優雅關機，確保 HTTP、worker、資料庫、Redis 及清理 goroutine 依序收斂
- [ ] 7.3 新增啟動與定期健康檢查，驗證 Bot 身分、Webhook 設定及 `can_delete_messages`、`can_restrict_members` 最小權限
- [x] 7.4 建立非 root 多階段 Dockerfile、`.dockerignore` 與容器健康檢查，確保 image 不包含秘密值或開發產物
- [x] 7.5 建立 `docker-compose.yaml`，定義 app、PostgreSQL、Redis、健康相依、隔離 network、PostgreSQL named volume、唯讀規則掛載及環境變數注入
- [x] 7.6 驗證 `docker compose config`、缺少秘密值拒絕啟動、服務健康順序、migration、重新啟動資料保存及優雅停止
- [x] 7.7 撰寫合成 Telegram 更新的端到端測試，驗證 Webhook 至偵測、違規、處置及稽核的完整流程
- [x] 7.8 更新繁體中文 README 與部署文件，說明 BotFather、Privacy Mode、supergroup 權限、Webhook、Secret、規則檔、三種模式與 Docker Compose 啟動方式
- [x] 7.9 執行 migrations 測試、`gofmt -s`、`go vet ./...`、專案 lint、`go test -race -count=1 ./...`、coverage 與關鍵 benchmark，修正所有失敗

## 8. 指定技術調整

- [x] 8.1 將 HTTP 路由改用 Gin，註冊 Telegram Webhook 與健康檢查並維持既有安全限制
- [x] 8.2 將 PostgreSQL repository 改用 GORM，定義含繁體中文 table/column comments 的 models 並於啟動執行 `AutoMigrate`
- [x] 8.3 將應用程式結構化日誌改用 `github.com/vincent119/zlogger`，支援設定、Context 欄位與關機 Sync
- [x] 8.4 更新測試、Docker Compose、README 與 OpenSpec，執行 race test、lint、vet 及容器啟動驗證
- [x] 8.5 將 Viper 設定改為具結構的 app、log、db 分層，支援連線池、TLS、環境變數覆寫及設定載入測試
- [x] 8.6 在 Makefile 新增本機 run、Docker Hub 安全登入、映像建置與推送目標，並補充使用文件
- [x] 8.7 在 README 說明外部 PostgreSQL 只需建立使用者與 database，資料表由 GORM AutoMigrate 建立
- [x] 8.8 補齊主要 Go 程式公開 API 與安全、冪等、生命週期關鍵流程的繁體中文註解
- [x] 8.9 擴充 Redis 結構化設定，支援 ACL username、password、requirepass 相容欄位、logical database 與環境變數覆寫
- [x] 8.10 全面補強 README，涵蓋 Telegram BotFather、Privacy Mode、Webhook、設定、資料庫、Redis、規則、健康檢查、部署驗證與故障排查
- [x] 8.11 新增 Telegram 群組允許清單，限制群組類型並補充查詢 `chat.id`、`chat.type` 的操作文件
