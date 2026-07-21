## Context

目前專案的 logger 設定已拆成兩個概念：`log.format` 控制編碼格式，`log.outputs` 控制輸出目的地。檔案輸出透過 `github.com/vincent119/zlogger v1.0.5` 初始化，該版本目前使用 `os.OpenFile` 寫入固定檔案，沒有公開的大小、備份數、保存天數或壓縮設定。

既有 `log.max_files` 只在服務啟動時掃描 `log.path` 底下的 `.log` 檔案並刪除較舊檔案。這不是持續輪轉，無法避免服務長時間執行時單一檔案無限制成長。

## Goals / Non-Goals

**Goals:**

- 將檔案日誌輪轉設計為明確的 `log.rotate` 設定區塊。
- 支援依單檔大小切檔、保留備份數、保留天數與壓縮。
- 保持 `log.format` 與 `log.outputs` 的語意清楚分離。
- 保留現有 console-only 設定的行為。
- 讓設定驗證在啟動階段失敗，避免啟動後才因 log 寫入失敗造成不明確狀態。

**Non-Goals:**

- 不改變 Telegram 指令、自動回覆、垃圾偵測規則與資料庫 schema。
- 不導入集中式日誌平台。
- 不實作動態 reload log rotate 設定。
- 不支援依不同 log level 分檔。

## Decisions

### 使用 `log.rotate` 作為正式設定入口

設定範例：

```yaml
log:
  level: debug
  format: json
  outputs:
    - console
    - file
  path: ./logs
  file: app.log
  rotate:
    enabled: true
    max_size_mb: 100
    max_backups: 14
    max_age_days: 30
    compress: true
```

理由：

- `format` 表示內容格式，不能承擔輸出目的地語意。
- `outputs` 表示輸出目的地，已符合目前設計。
- `rotate` 是 file output 的子能力，獨立成區塊比較容易驗證與擴充。

替代方案：

- 使用 `format: file`：拒絕。這會混淆格式與輸出目的地。
- 延用 `max_files`：拒絕作為主設計。它只能描述檔案數量，不能描述切檔條件與壓縮。

### 保留 `log.max_files` 作為過渡欄位

`log.max_files` 短期保留，文件標示為 deprecated。`log.rotate.enabled=false` 且 `log.max_files > 0` 時，保留既有啟動時 `pruneLogFiles` 清理行為。`log.rotate.enabled=true` 時，不執行 `pruneLogFiles`，避免與正式輪轉行為重疊。

理由：

- 避免現有部署設定立即失效。
- 讓使用者有明確遷移路徑。
- 正式輪轉應由 `log.rotate` 與 `lumberjack` 控制，不能再混用啟動時清理。

替代方案：

- 直接移除 `max_files`：風險較高，會造成現有 config 驗證或預期行為改變。

### 在本專案封裝 rotate logger 初始化

已確認 `github.com/vincent119/zlogger v1.0.5` 的 `Config` 沒有 writer、core 或 rotate 欄位，`zlogger.Init()` 的 file output 由套件內部以 `os.OpenFile` 建立，不能注入 `lumberjack`。`zlogger` README 也明確建議需要 log rotation 時自行建立 `zapcore`。

因此本專案採用下列策略：

1. 業務程式碼繼續使用 `zlogger.InfoContext`、`zlogger.DebugContext`、`zlogger.ErrorContext` 等 facade。
2. `log.rotate.enabled=false` 時保留現有 `zlogger.Init()` 初始化路徑。
3. `log.rotate.enabled=true` 且 `outputs` 包含 `file` 時，在 `cmd/tg-spam-bot/logger.go` 自行建立 zap logger。
4. rotate file writer 使用 `gopkg.in/natefinch/lumberjack.v2`。
5. 自建 logger 完成後呼叫 `zap.ReplaceGlobals(logger)`，讓既有 zlogger facade 繼續寫到同一個 zap global logger。

理由：

- 不需要等待或修改共用 `zlogger` 套件。
- 封裝集中在 main 組裝層，不污染業務邏輯。
- 既有程式碼不需要改掉 zlogger 呼叫點。

替代方案：

- 修改 `zlogger`：短期成本較高，且需要發新版後再回到本專案升級。
- 使用系統 `logrotate`：部署相依性較強，且 sample config 不能完整表達應用層行為。
- 直接全面改用 zap：改動面過大，會破壞目前 context logging facade。

### 採用保守預設值與零值語意

`log.rotate` 採用下列預設與零值規則：

```yaml
rotate:
  enabled: false
  max_size_mb: 100
  max_backups: 14
  max_age_days: 30
  compress: true
```

- `enabled` 預設 `false`，避免既有 file output 部署在升級後突然改變行為。
- `max_size_mb=0` 表示使用預設值 `100`，因為大小是輪轉的主要觸發條件。
- `max_backups=0` 表示不限制備份數，沿用 `lumberjack` 語意。
- `max_age_days=0` 表示不依天數刪除，沿用 `lumberjack` 語意。
- `compress` 預設 `true`，降低長期磁碟使用量。

## Risks / Trade-offs

- 新增 `lumberjack` 會增加依賴面 → 限制使用範圍在 `cmd/tg-spam-bot/logger.go`，並以測試覆蓋初始化行為。
- `compress=true` 會消耗 CPU → 預設可啟用，但需在正式環境觀察資源使用。
- 依大小切檔不等於依日期切檔 → 文件需明確說明切檔條件，避免誤解。
- 過渡支援 `max_files` 會增加設定語意複雜度 → README 與 sample config 明確標示 deprecated。

## Migration Plan

1. 新增 `log.rotate` 設定結構與預設值。
2. 保留 `log.max_files`，但文件標示為 deprecated。
3. 更新 `config.sample.yaml`，讓新部署使用 `log.rotate`。
4. 部署時若要啟用檔案輪轉，將設定改為：

   ```yaml
   log:
     outputs:
       - console
       - file
     rotate:
       enabled: true
   ```

5. 回滾時可停用 `rotate.enabled`，保留原本 file output 行為。
