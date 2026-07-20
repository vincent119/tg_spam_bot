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

`log.max_files` 短期保留，文件標示為 deprecated。若未設定 `log.rotate.max_backups` 且 `log.max_files > 0`，實作可將其映射為 `rotate.max_backups`。

理由：

- 避免現有部署設定立即失效。
- 讓使用者有明確遷移路徑。

替代方案：

- 直接移除 `max_files`：風險較高，會造成現有 config 驗證或預期行為改變。

### 優先在 `zlogger` 支援輪轉，必要時才在本專案封裝

優先順序：

1. 若 `zlogger` 已有或可新增 rotate config，使用 `zlogger` 原生能力。
2. 若短期不改 `zlogger`，在本專案 logger 初始化層接入 `lumberjack` 或等價 writer。

理由：

- 輪轉是 logger 基礎設施能力，放在共用 logger 套件更一致。
- 本專案可以先規格化 config，再決定共用套件實作位置。

替代方案：

- 使用系統 `logrotate`：部署相依性較強，且 sample config 不能完整表達應用層行為。

## Risks / Trade-offs

- 新增第三方輪轉套件會增加依賴面 → 先確認 `zlogger` 是否支援或可調整，只有必要時才新增依賴。
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

## Open Questions

- 是否要優先修改 `github.com/vincent119/zlogger`，還是在本專案先接 `lumberjack`？
- 預設 `rotate.enabled` 要維持 `false` 以保守相容，還是當 `outputs` 包含 `file` 時預設啟用？
