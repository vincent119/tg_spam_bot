## Why

目前服務只有自動偵測與處置，群組管理員無法在 Telegram 內查詢狀態、人工修正違規紀錄或執行臨時處置。新增受權限與稽核保護的管理指令，可縮短事故處理時間，並補足自動規則誤判或漏判時的人工介入能力。

## What Changes

- 新增 `/help`、`/ping` 與 `/id` 基礎指令，提供操作說明、存活確認及群組／使用者識別資訊。
- 新增 `/warnings`、`/warn` 與 `/clearwarn`，供管理員查詢、增加及失效指定成員最近 30 天的違規紀錄。
- 新增 `/del`、`/mute`、`/unmute`、`/ban` 與 `/unban`，讓管理員以回覆訊息的方式執行人工處置。
- 指令同時支援 `/command` 與群組內的 `/command@bot_username` 格式，參數採嚴格解析與長度限制。
- 所有具副作用指令限制於允許群組及 Telegram 管理員，禁止處置管理員、Bot 自身與可信任成員。
- 人工違規與處置使用獨立來源標記、冪等鍵及管理紀錄，不與自動偵測結果混淆。
- 指令回應不揭露秘密值或完整歷史訊息，錯誤內容採使用者可理解但不暴露內部細節的繁體中文。

## Capabilities

### New Capabilities

- `telegram-admin-commands`: 定義 Telegram 群組管理指令的解析、授權、目標選取、違規管理、人工處置、回應與稽核行為。

### Modified Capabilities

無。

## Impact

- Telegram Webhook DTO 需保留 bot command entities、回覆訊息、操作者與目標成員資訊，並在垃圾訊息偵測前分流指令。
- application 層新增 command use case、管理員授權、違規查詢／調整及人工處置介面。
- Telegram Client 增加訊息回覆、解除禁言與解除封鎖能力；既有刪除、禁言及封鎖能力可重用。
- PostgreSQL model 與 repository 增加人工來源、操作者、原因及冪等資料；由既有 GORM AutoMigrate 建立非破壞性欄位或新表。
- BotFather 指令清單、README、測試與管理紀錄格式需同步更新。
