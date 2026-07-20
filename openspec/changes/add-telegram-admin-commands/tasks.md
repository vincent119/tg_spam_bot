## 1. Telegram DTO 與指令解析

- [x] 1.1 擴充 Telegram Update DTO，保留 `bot_command` entity、`reply_to_message`、sender chat、目標使用者與必要 username 欄位
- [x] 1.2 實作 UTF-16 entity offset 安全解析器，支援 `/command` 與 `/command@bot_username` 並拒絕其他 Bot 指令
- [x] 1.3 建立固定指令註冊表、權限矩陣、回覆目標要求與繁體中文用法說明
- [x] 1.4 以 table-driven、Unicode 邊界及 fuzz tests 覆蓋合法、未知、錯誤 suffix、超長參數與非開頭 command entity

## 2. Command domain 與 application 邊界

- [x] 2.1 定義 Command、Target、Actor、Duration、Reason、Result 與穩定錯誤型別，邊界複製所有 slice 與可變資料
- [x] 2.2 定義管理員即時查詢、可信任成員、警告 repository、command execution store、Telegram command client 與時鐘介面
- [x] 2.3 實作目標解析與保護規則，拒絕管理員、Bot、可信任成員、匿名 sender chat 及缺少 user ID 的回覆
- [x] 2.4 實作 reason 最多 200 Unicode code points、十進位 user ID 及一分鐘至七天 duration 解析驗證

## 3. 公開指令

- [x] 3.1 實作 `/help`，依操作者權限輸出可用指令、語法及回覆要求
- [x] 3.2 實作 `/ping`，只回傳 Bot 存活所需最小資訊且不揭露基礎設施設定
- [x] 3.3 實作 `/id`，未回覆時顯示 chat／actor ID，回覆一般成員時顯示 chat／target ID
- [x] 3.4 使用 Redis 對公開指令套用 `chat_id + user_id` 短時間頻率限制，超額時靜默忽略
- [x] 3.5 撰寫公開指令、未知指令、其他 Bot 指令及頻率限制單元測試

## 4. 管理員授權與警告指令

- [x] 4.1 實作每次管理指令都向 Telegram 即時驗證 `creator`／`administrator`，不得沿用管理員豁免快取
- [x] 4.2 實作 `/warnings`，查詢同群組目標最近 30 天有效違規並回傳人工／自動摘要
- [x] 4.3 實作 `/warn [reason]`，transaction 內新增 manual 違規與稽核、回傳更新後次數但不立即自動升級
- [x] 4.4 調整既有自動階梯計數，明確包含最近 30 天仍有效的 manual 違規
- [x] 4.5 實作 `/clearwarn [reason]`，transaction 內失效目前有效違規並保留原紀錄、操作者與原因
- [x] 4.6 撰寫一般成員拒絕、權限撤銷、受保護目標、人工警告計數及清除警告的 application tests

## 5. 人工 Telegram 處置

- [x] 5.1 擴充 Telegram Client 的 sendMessage、unmute 與 unban API，沿用 token 遮蔽、context timeout 與錯誤分類
- [x] 5.2 實作 `/del`，刪除被回覆訊息並保存人工處置結果
- [x] 5.3 實作 `/mute <duration> [reason]` 與 `/unmute`，正確建立 UTC 到期時間及恢復群組預設權限
- [x] 5.4 實作 `/ban [reason]` 與 `/unban`，後者支援回覆訊息或明確十進位 user ID，且不自動重新加入
- [x] 5.5 確認人工處置在 observe、delete-only、enforce 三種模式均執行，並以 `manual_command` 與自動處置區隔
- [x] 5.6 以 mock Telegram Server 覆蓋成功、權限不足、暫時錯誤、受保護目標及敏感錯誤遮蔽

## 6. Persistence、冪等與稽核

- [x] 6.1 建立含繁體中文 table／column comments 的 GORM command execution 與人工稽核 models，加入必要唯一鍵及索引
- [x] 6.2 擴充 violations model 支援 manual source、operator、reason、失效時間與失效操作者，維持既有資料相容
- [x] 6.3 實作 `chat_id + update_id` 原子占用、完成、失敗及既有結果讀取，避免重送重複副作用
- [x] 6.4 將人工違規調整與管理紀錄包在同一 PostgreSQL transaction，Telegram API 結果分開落盤
- [x] 6.5 撰寫 GORM AutoMigrate、唯一鍵、30 天查詢、soft invalidation、重送與部分失敗整合測試

## 7. Webhook 組裝與生命週期

- [x] 7.1 在 Webhook 完成 secret、body、chat allowlist 及 chat type 驗證後，於垃圾訊息偵測前分流 command handler
- [x] 7.2 確保命令 update 只由 command handler 或一般 processor 其中之一處理，未知與其他 Bot 指令不得落入偵測
- [x] 7.3 將 command handler、repositories、Redis limiter、Telegram Client 與既有 Gin server 透過 DI 組裝
- [x] 7.4 確保所有新增 goroutine、HTTP 呼叫及 persistence 操作回應根 context，沿用既有 graceful shutdown
- [x] 7.5 建立合成 Update 端到端測試，覆蓋公開指令、管理指令、重送、未授權群組與一般垃圾訊息不回歸

## 8. 文件與部署驗證

- [x] 8.1 在 README 補充所有指令語法、權限、回覆操作、manual 警告語意、模式關係與故障排查
- [x] 8.2 提供 BotFather `/setcommands` 的繁體中文指令清單及更新步驟
- [x] 8.3 補充 PostgreSQL schema 變更、AutoMigrate 部署前備份驗證與回滾說明
- [x] 8.4 執行 `gofmt -s`、`go vet ./...`、lint、`go test -race -count=1 ./...`、coverage、關鍵 parser fuzz／benchmark
- [ ] 8.5 在測試 supergroup 逐項驗證 11 個指令、權限撤銷、受保護目標、Webhook 重送及 Telegram API 權限錯誤

## 9. 結構化日誌修正

- [x] 9.1 在 Webhook 邊界注入穩定 request ID，並傳遞至 command 與一般訊息處理 context
- [x] 9.2 記錄 command 接收、完成、重送、限流及失敗結果，禁止輸出參數、原因與訊息原文
- [x] 9.3 記錄 AutoMigrate 成功事件與 Webhook 處理錯誤，統一補上 subsystem 與安全錯誤欄位
- [x] 9.4 補充 context 傳遞測試並執行格式化、vet、lint 與 race tests
