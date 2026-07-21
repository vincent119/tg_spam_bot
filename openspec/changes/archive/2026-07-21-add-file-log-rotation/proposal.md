## Why

目前檔案日誌只有啟動時清理舊 `.log` 檔案，不能依大小或天數自動輪轉。當服務長時間執行且 `outputs` 包含 `file` 時，單一日誌檔可能持續變大，增加磁碟耗盡與排查困難的風險。

## What Changes

- 新增檔案日誌輪轉設定，使用 `log.rotate` 表達是否啟用、大小上限、保留份數、保留天數與壓縮。
- 保留 `log.format` 僅表示輸出內容格式，支援 `json` 與 `console`。
- 保留 `log.outputs` 表示輸出目的地，支援 `console` 與 `file`。
- 當 `log.outputs` 不包含 `file` 時，檔案路徑與輪轉設定不影響啟動。
- 將既有 `log.max_files` 視為過渡設定，不作為新設計主入口。
- 更新 sample config 與 README，明確說明檔案日誌與輪轉設定方式。

## Capabilities

### New Capabilities

- `file-log-rotation`: 定義檔案日誌輸出與輪轉設定的行為、驗證規則與相容策略。

### Modified Capabilities

無。

## Impact

- 影響設定載入與驗證：`internal/config`。
- 影響 logger 初始化流程：`cmd/tg-spam-bot`。
- 可能新增日誌輪轉依賴，需先確認 `github.com/vincent119/zlogger` 是否支援；若不支援，評估導入 `lumberjack`。
- 影響文件：`configs/config.sample.yaml`、`README.md`。
- 不改變既有 Telegram webhook、偵測規則、自動回覆與資料庫 schema。
