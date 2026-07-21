## 1. 設定模型與驗證

- [x] 1.1 在 `internal/config.Config.Log` 新增 `Rotate` 設定結構，包含 `enabled`、`max_size_mb`、`max_backups`、`max_age_days`、`compress`
- [x] 1.2 新增 `log.rotate.*` 預設值與環境變數綁定
- [x] 1.3 更新設定驗證，拒絕負數輪轉設定
- [x] 1.4 保留 `log.max_files`，並標示為 deprecated
- [x] 1.5 實作 `max_size_mb=0` 使用預設值 `100` 的有效值轉換
- [x] 1.6 保留 `max_backups=0` 為不限制備份數，不映射成預設值
- [x] 1.7 保留 `max_age_days=0` 為不依天數刪除，不映射成預設值

## 2. Logger 初始化設計落地

- [x] 2.1 確認並記錄 `github.com/vincent119/zlogger v1.0.5` 不支援 writer 注入或 rotate config
- [x] 2.2 新增 `gopkg.in/natefinch/lumberjack.v2` 依賴
- [x] 2.3 在 `cmd/tg-spam-bot/logger.go` 封裝 logger 初始化，保留非 rotate 模式使用 `zlogger.Init()`
- [x] 2.4 在 rotate 模式自行建立 zap console core 與 lumberjack file core，並呼叫 `zap.ReplaceGlobals(logger)`
- [x] 2.5 實作 file output 輪轉初始化，支援大小、備份數、保存天數與壓縮
- [x] 2.6 `rotate.enabled=true` 時不執行既有 `pruneLogFiles`
- [x] 2.7 `rotate.enabled=false` 且 `max_files>0` 時保留既有 `pruneLogFiles` 作為 deprecated 相容行為

## 3. 文件與範例

- [x] 3.1 更新 `configs/config.sample.yaml`，加入 `log.rotate` 範例與說明
- [x] 3.2 更新 README，說明 `log.format`、`log.outputs`、`log.rotate` 的差異
- [x] 3.3 在 README 標註 `log.max_files` 為 deprecated，並提供遷移方式

## 4. 測試與驗證

- [x] 4.1 補 config load 測試，確認 `log.rotate` 可由 YAML 載入
- [x] 4.2 補 config validate 測試，確認負數輪轉設定會失敗
- [x] 4.3 補有效值測試，確認 `max_size_mb=0` 轉為 `100`
- [x] 4.4 補 logger 初始化測試，確認 console-only 不需要檔案輪轉設定
- [x] 4.5 補 file output 測試，確認啟用 rotate 時會建立可寫入的 log 檔
- [x] 4.6 補相容測試，確認 `rotate.enabled=false && max_files>0` 仍執行 legacy prune
- [x] 4.7 補相容測試，確認 `rotate.enabled=true` 不執行 legacy prune
- [x] 4.8 執行 `go test ./...`
