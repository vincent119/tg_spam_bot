## 1. 設定模型與驗證

- [ ] 1.1 在 `internal/config.Config.Log` 新增 `Rotate` 設定結構，包含 `enabled`、`max_size_mb`、`max_backups`、`max_age_days`、`compress`
- [ ] 1.2 新增 `log.rotate.*` 預設值與環境變數綁定
- [ ] 1.3 更新設定驗證，拒絕負數輪轉設定
- [ ] 1.4 保留 `log.max_files`，並標示為 deprecated
- [ ] 1.5 補上 `log.max_files` 到 `log.rotate.max_backups` 的相容映射規則

## 2. Logger 初始化設計落地

- [ ] 2.1 確認 `github.com/vincent119/zlogger v1.0.5` 是否可直接支援 rotate 設定
- [ ] 2.2 若 `zlogger` 支援 rotate，將專案設定轉成 `zlogger.Config`
- [ ] 2.3 若 `zlogger` 不支援 rotate，決定是否修改 `zlogger` 或在本專案接入 `lumberjack`
- [ ] 2.4 實作 file output 輪轉初始化，支援大小、備份數、保存天數與壓縮
- [ ] 2.5 移除或降級既有 `pruneLogFiles`，避免與正式 rotate 行為重疊

## 3. 文件與範例

- [ ] 3.1 更新 `configs/config.sample.yaml`，加入 `log.rotate` 範例與說明
- [ ] 3.2 更新 README，說明 `log.format`、`log.outputs`、`log.rotate` 的差異
- [ ] 3.3 在 README 標註 `log.max_files` 為 deprecated，並提供遷移方式

## 4. 測試與驗證

- [ ] 4.1 補 config load 測試，確認 `log.rotate` 可由 YAML 載入
- [ ] 4.2 補 config validate 測試，確認負數輪轉設定會失敗
- [ ] 4.3 補相容測試，確認 `max_files` 可映射到有效備份數
- [ ] 4.4 補 logger 初始化測試，確認 console-only 不需要檔案輪轉設定
- [ ] 4.5 補 file output 測試，確認啟用 rotate 時會建立可寫入的 log 檔
- [ ] 4.6 執行 `go test ./...`
