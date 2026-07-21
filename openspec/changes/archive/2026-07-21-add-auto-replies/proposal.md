## Why

群組成員常用自然語句詢問固定資訊，例如「下載頁在哪」、「app 去哪裡下載」，目前只能靠管理員人工回覆，容易重複消耗管理時間。

新增可設定的自動回覆能力，可在不引入 AI 語意判斷的前提下，對明確關鍵字問題提供穩定、可稽核且不干擾垃圾訊息處置的固定答案。

## What Changes

- 新增一般訊息的自動回覆流程，支援依獨立 YAML 規則檔設定的關鍵字或短語命中後回覆固定文字。
- 自動回覆只在已允許的 `group` 或 `supergroup` 中生效，且不得處理管理指令、其他 Bot 的訊息或不支援的聊天。
- 自動回覆須沿用現有文字正規化策略，支援繁體、簡體、英文大小寫與混合文字的穩定比對。
- 自動回覆須與垃圾訊息偵測協調；已判定為垃圾或已執行自動處置的訊息不得觸發回覆。
- 新增自動回覆命中紀錄與結構化日誌欄位，但不得保存或輸出完整使用者原文。
- 新增設定範例、README 說明與測試覆蓋。

## Capabilities

### New Capabilities

- `telegram-auto-replies`: 定義 Telegram 群組一般訊息如何依可設定規則觸發固定自動回覆，並與管理指令、垃圾訊息偵測、冪等及隱私保護協調。

### Modified Capabilities

- 無。

## Impact

- 影響 Webhook 一般訊息流程，需要在管理指令分流後、垃圾訊息處置結果明確後執行自動回覆。
- 新增自動回覆 domain/application/rules 元件、主設定中的規則檔路徑設定與獨立 YAML 載入驗證。
- Telegram client 既有 `SendMessage` 能力可重用，實作時需確認回覆目標訊息 ID 與錯誤遮蔽語意。
- PostgreSQL 需新增非破壞性資料表或欄位保存自動回覆執行結果，以避免 Telegram 重送造成重複回覆。
- README、`configs/config.sample.yaml`、`configs/auto_replies.sample.yaml`、Docker 掛載或規則目錄需補上自動回覆設定說明。
