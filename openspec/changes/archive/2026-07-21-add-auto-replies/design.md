## Context

現有 Webhook 先驗證 Telegram secret、聊天類型與允許清單，再將開頭的 Telegram `bot_command` 分流到管理指令處理器；非指令的一般訊息會轉成偵測 domain message，交由 spam processor 做豁免、行為觀察、規則判定、違規保存與處置。

自動回覆要讀取一般使用者文字並回覆固定答案，例如「下載頁在哪」或「app 去哪裡下載」。它會跨越 Telegram delivery、文字正規化、設定載入、Telegram 回覆 API、冪等紀錄與文件，因此應作為獨立能力加入，而不是塞進管理指令或垃圾規則。

## Goals / Non-Goals

**Goals:**

- 讓部署者用獨立 YAML 規則檔設定固定問答，例如下載頁、客服、App 下載位置。
- 在允許群組中對一般文字或媒體說明文字自動回覆。
- 沿用既有繁簡與大小寫正規化策略，提升固定關鍵字命中率。
- 確保垃圾訊息處置優先於自動回覆，避免幫垃圾訊息增加曝光。
- 以 `update_id` 收斂 Telegram 重送，避免重複回覆。
- 保存必要稽核資料，但不保存完整使用者原文。

**Non-Goals:**

- 不導入 AI、向量搜尋、模糊語意問答或外部 LLM。
- 不在首版提供 Telegram 指令新增、刪除或修改自動回覆規則。
- 不支援私人聊天、頻道或未列入允許清單的群組。
- 不讓自動回覆規則修改垃圾訊息規則、可信任名單或管理員權限。
- 不提供多輪對話狀態或使用者個人化回覆。

## Decisions

### 新增獨立 auto-reply application 邊界

新增 `internal/autoreply` 能力區塊，至少包含 domain 規則模型、application processor、rules YAML loader 與 persistence port。Webhook 不直接比對字串，而是把已通過聊天與指令分流的一般訊息交給應用層協調。

替代方案是把自動回覆塞進既有 detection processor。這會讓垃圾規則、違規處置與一般問答耦合，後續很難清楚表達「垃圾優先」與「不保存完整原文」的邊界，因此不採用。

### 垃圾訊息偵測先於自動回覆

一般訊息流程採：

```text
Webhook
  │
  ├─ command entity → 管理指令
  │
  └─ domain message
       │
       ├─ spam processor
       │    └─ spam 或已處置 → 結束
       │
       └─ auto-reply processor
            └─ 命中規則 → 回覆
```

這個順序避免廣告訊息因包含「下載 app」而觸發機器人回覆。為了讓 auto-reply 知道是否可繼續，spam processor 需要回傳處理結果，至少能表達 `spam`、`exempt`、`processed` 或等價狀態；若現有 `Process` 只有 `error`，實作時需小幅調整回傳型別。

### 規則以獨立 YAML 檔載入，啟動時驗證

主設定只保存開關與規則檔路徑，規則內容放在獨立檔案，例如 `configs/auto_replies.yaml`。這樣可避免主設定被大量業務文案污染，也方便只審查回覆規則變更。

```yaml
auto_replies:
  enabled: true
  rules_file: configs/auto_replies.yaml
```

獨立規則檔格式如下：

```yaml
version: "2026-07-20.1"
rules:
  - id: download_page
    enabled: true
    keywords:
      - 下載頁
      - app 去哪裡下載
      - app 在哪下載
      - 下載 app
    reply: "下載頁在：https://example.com/download"
```

啟動時先讀主設定，再依 `rules_file` 載入規則檔，驗證 `id` 唯一、至少一個 keyword、reply 非空。規則載入失敗時拒絕啟動，不載入部分規則，保持部署結果可預測。

選擇 YAML 而不是 DB，是因為首版規則屬部署設定，變更頻率低，且可與 Git review、部署流程一起管理。DB 管理可作為後續 change。

### 使用既有正規化策略做 contains 比對

自動回覆比對使用與垃圾偵測相同的 normalizer，對使用者訊息與 keyword 都產生正規化文字與繁體轉換副本。首版比對採 `contains`，規則順序決定優先級。

不採用正則作為首版預設，原因是一般部署者較容易寫出過寬或高成本正則，且這類固定問答不需要複雜模式。若日後需要，應新增明確 `match_type` 並限制正則長度與編譯成本。

### 單一訊息只回覆第一個命中規則

當同一訊息命中多條規則，系統只回覆設定順序最前面的規則。這讓結果可預測，也避免群組中一則問題觸發多則機器人回覆。

可選替代方案是合併多條回覆，但會讓訊息過長且難以控制格式，因此首版不採用。

### 以 PostgreSQL 保存自動回覆執行紀錄

新增非破壞性資料表保存 `chat_id`、`update_id`、`message_id`、`rule_id`、`status`、`retryable`、`error_code`、遮蔽後錯誤摘要、建立與完成 UTC 時間。`chat_id + update_id` 建唯一鍵，避免 Telegram 重送重複回覆。

Redis 適合短期頻率狀態，但自動回覆屬外部副作用，應落 PostgreSQL 方便稽核與重送收斂。完整使用者原文不入庫；必要時只保存內容 HMAC 指紋，沿用既有 `CONTENT_HASH_KEY`。

### 重用 Telegram SendMessage 並回覆原訊息

自動回覆使用既有 Telegram client 的 `SendMessage(ctx, chatID, replyToMessageID, text)`，回覆到觸發訊息，讓群組成員能看出答案對應哪個問題。Telegram API 錯誤沿用現有遮蔽策略，不把 token、credential 或完整 response 寫入回應或日誌。

### 文件先給實際範例

README 需要補「自動回覆規則」段落，直接示範「下載頁」與「app 去哪裡下載」。`configs/config.sample.yaml` 只示範 `enabled` 與 `rules_file`，`configs/auto_replies.sample.yaml` 提供完整範例規則，避免部署者不知道 YAML 結構。

## Risks / Trade-offs

- [自動回覆幫垃圾訊息增加曝光] → 垃圾訊息偵測與處置優先，spam 命中後不得執行自動回覆。
- [規則過寬造成洗版] → 單訊息最多一則回覆，規則順序優先，文件提醒 keyword 需具體。
- [Telegram 重送造成重複回覆] → PostgreSQL 以 `chat_id + update_id` 做冪等鍵，完成後重送直接略過。
- [使用者原文進入資料庫或日誌] → 稽核只保存識別碼、rule_id、狀態與遮蔽錯誤，不保存完整原文。
- [調整 spam processor 回傳型別影響既有測試] → 以最小型別新增結果資訊，維持錯誤語意不變，補齊 processor 與 webhook 測試。
- [YAML 規則需重新部署才生效] → 首版接受此限制，換取可 review、可回滾與低風險。

## Migration Plan

1. 新增 auto-reply spec 對應的 domain、rules loader 與純單元測試，尚不組裝 Webhook。
2. 新增設定結構與獨立 sample YAML，驗證 `enabled=false` 時不讀取規則檔，`enabled=true` 時必須提供有效 `rules_file`。
3. 新增 PostgreSQL model、store 與 AutoMigrate 清單，建立非破壞性唯一鍵與索引。
4. 調整 spam processor 回傳處理結果，讓 Webhook 能在非垃圾訊息時接續 auto-reply。
5. 在 Webhook 組裝 auto-reply processor，補上指令、不支援聊天、垃圾命中、成功回覆與重送測試。
6. 更新 README，部署到測試群組驗證「下載頁」與「app 去哪裡下載」命中。

回滾時移除 Webhook 組裝或將 `auto_replies.enabled=false`；獨立規則檔與新增資料表可保留，不影響既有垃圾訊息偵測與管理指令。

## Open Questions

- 是否需要自動回覆也做短時間頻率限制，例如同一使用者每分鐘最多觸發一次；首版可以先依 `update_id` 冪等，不做跨訊息限流。
- 是否需要支援 Markdown 或 HTML parse mode；首版建議純文字，避免格式錯誤導致回覆失敗。
- 是否需要每個群組不同規則；首版可以先全域規則，若多群組需求明確，再加 `chat_ids` 條件。
