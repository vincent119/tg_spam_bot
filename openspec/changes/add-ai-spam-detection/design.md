## Context

系統目前以 Telegram Webhook 接收訊息，經過 allowlist、chat type、管理員豁免、文字正規化、YAML 規則與行為訊號後，依 `observe`、`delete-only`、`enforce` 模式保存偵測紀錄與執行處置。現有設計刻意不保存完整訊息，只保存內容指紋與必要稽核欄位。

AI 判定要補足「低規則命中但語意明顯可疑」的缺口，但不能破壞現有可解釋、可回溯、可控成本與低誤殺原則。

## Goals / Non-Goals

**Goals:**

- 只在模糊訊息上呼叫 AI，避免每則訊息都增加成本與延遲。
- 支援語意相似垃圾訊息比對，讓字面不同但話術相近的廣告可被歷史案例輔助識別。
- 在 AI 判定前查詢相似歷史案例，降低不必要的 AI classifier 呼叫。
- 建立可人工維護的語意黑名單分類，支援兼職拉人、代理招募、投資導流、色情引流、VPN 客服冒充與交易所轉 U 等話術。
- 支援批次聚類分析，將一段時間內的可疑訊息分群，提供人工轉成 YAML 規則的依據。
- 首版以 `observe` 為預設，先收集 AI 判定品質，不直接擴大自動處置。
- 強制 AI provider 回傳固定 JSON schema，禁止自然語言自由解讀。
- 保存可稽核但不含完整原文的 AI 判定紀錄。
- AI 呼叫失敗時安全降級，不影響既有高信心規則判定與處置。
- provider 介面可替換，首版落地 OpenAI-compatible HTTP API。

**Non-Goals:**

- 不訓練模型。
- 不自動把 AI 判定寫回 YAML 規則。
- 不保存完整訊息或建立標註資料庫。
- 不讓 AI 單獨觸發封鎖。
- 不另行部署 Pinecone、Qdrant 或其他獨立向量資料庫；首版使用既有 PostgreSQL 搭配 pgvector。
- 不讓相似案例查詢結果單獨觸發 ban。
- 不把 Bot Token、Webhook secret、DB 密碼、AI API key、完整 Telegram 使用者資訊送給 AI。
- 不承諾所有 provider 的輸出一致；provider 差異需要以觀測資料校準。

## Decisions

### AI 只處理模糊區間

規則引擎仍是第一層判定。AI 只在訊息有可疑訊號但未達明確垃圾門檻時執行。

建議首版模糊條件：

- 規則分數大於 0 但低於處置門檻。
- 或沒有明確規則分數，但存在弱可疑訊號，例如 URL、Telegram mention、交易導流詞、下載註冊詞、短期相似內容或新入群貼連結。
- 管理員、可信任成員、未授權聊天、Bot 訊息與不支援訊息不呼叫 AI。

明確垃圾與明確正常都不呼叫 AI。這可以控制成本、降低延遲，並避免把普通聊天大量送到外部服務。

### 使用 pgvector 作為語意記憶庫

既有系統已使用 PostgreSQL 保存稽核資料，首版語意記憶庫使用 `pgvector`，不新增獨立向量資料庫服務。

原因：

- 少一個服務要部署、監控與備份。
- 向量資料、AI 判定、偵測紀錄與稽核資料在同一交易與備份邊界內。
- 目前 Telegram 群組規模不需要獨立向量 DB 的水平擴展能力。
- PostgreSQL 可以用 metadata 條件縮小查詢範圍，再做向量相似度排序。

資料表建議：

- `message_embeddings`
  - `content_fingerprint`
  - `embedding_model`
  - `embedding_version`
  - `embedding vector`
  - `source_event_id`
  - `label`
  - `category`
  - `reason_code`
  - `created_at`
  - `expires_at`

索引策略：

- 先以 B-tree 索引 `content_fingerprint`、`embedding_model`、`label`、`category`、`created_at`。
- 向量索引使用 pgvector 支援的 HNSW 或 IVFFlat，實際選型依 PostgreSQL 與 pgvector 版本確認。
- 初期資料量小時可先不建立 approximate index，待資料量成長後再加。

### AI 判定前先查相似歷史案例

模糊訊息進入 AI classifier 前，先產生或讀取 embedding，查詢語意相近的歷史樣本。

流程：

```text
模糊訊息
  │
  ├─ 產生 content fingerprint
  ├─ 產生或讀取 embedding
  ├─ 查 pgvector 相似歷史案例
  │
  ├─ 高相似 spam 案例
  │    └─ 提高可疑分數或寫入 observe 輔助訊號
  │
  ├─ 高相似 ham 案例
  │    └─ 降低 AI classifier 優先級，但不覆寫明確規則
  │
  └─ 無足夠相似案例
       └─ 呼叫 AI classifier
```

相似案例首版只影響 observe 與輔助分數，不得單獨刪除或封鎖。若高相似 spam 與 YAML 弱訊號同時存在，才可在後續 delete-only 階段考慮刪除候選。

### 建立語意黑名單

語意黑名單不是關鍵字，而是帶有分類與範例向量的話術集合。

建議分類：

- `part_time_recruiting`: 兼職拉人。
- `agent_recruiting`: 代理招募。
- `investment_redirect`: 投資導流。
- `adult_redirect`: 色情引流。
- `vpn_support_impersonation`: VPN 客服冒充。
- `crypto_exchange_redirect`: 交易所轉 U。

每個分類保存多個範例 embedding、描述、啟用狀態與建立來源。命中語意黑名單只能產生 `semantic_blacklist_match` 輔助訊號，不得單獨觸發 ban。

### 以 `/feedspam` 收集漏網垃圾樣本

管理員需要能把未被 YAML 或 AI 命中的漏網訊息提交成語意樣本。首版新增 `/feedspam [分類]` 指令，必須以回覆目標訊息方式使用。

流程：

```text
管理員回覆漏網訊息執行 /feedspam [分類]
  │
  ├─ 驗證操作者仍是 Telegram creator／administrator
  ├─ 驗證目標是一般成員，且不是管理員、Bot、可信任成員、匿名 sender chat 或缺少 user ID
  ├─ 擷取目標文字或 caption，套用既有長度限制與正規化
  ├─ 產生 content fingerprint，不保存完整原文
  ├─ 以目標文字同步產生 embedding
  ├─ 寫入 `message_embeddings`
  ├─ 建立 manual feed 樣本紀錄，label 固定為 spam，category 使用參數或預設分類，並標示 embedding 完成
  └─ 回覆管理員：已加入待訓練樣本
```

`/feedspam` 不得立即修改 YAML、不得立即建立違規、不得刪除、禁言或封鎖。為了維持「資料庫不保存完整原文」原則，首版在指令處理當下以記憶體中的回覆訊息文字同步產生 embedding，成功後只保存 fingerprint、向量與稽核欄位；若 embedding provider 失敗，保存 manual sample 的失敗摘要與可重試狀態，但不落完整原文。

分類參數應限制為短字串或既有分類 ID，避免把任意長文字寫入稽核欄位。若未提供分類，使用 `uncategorized_spam`。未來可新增 `/feedham` 收集正常樣本，但首版不納入，避免擴大指令面。

### 批次聚類產生規則建議

系統可對最近一段時間的模糊訊息、AI spam、AI uncertain 或低分可疑訊息做批次聚類，輸出人工審查用報表。因資料庫不保存完整原文，首版聚類不得輸出原文片段；候選 YAML 摘要由 category、reason code、signals 與 AI evidence 摘要推導，只能作為人工審查線索。

輸出內容：

- cluster id。
- category 建議。
- 樣本數。
- 代表 reason code 與 signals。
- 代表性短摘要。
- 建議加入 YAML 的候選詞或 aliases。

首版不得自動修改 YAML。管理者人工審查後，才把候選詞加入 `configs/rules/*.yaml`。

### 首版預設 observe

`ai_detection.mode` 支援 `observe`、`delete-only`、`enforce`，但預設必須是 `observe`。即使應用程式 `app.mode=enforce`，AI 自身模式仍可限制 AI 結果只能觀測。

首版建議政策：

- `observe`：只寫入 AI 判定紀錄，不改變原偵測結果。
- `delete-only`：AI 高信心 spam 最多建立刪除候選，不能推進 30 天違規階梯。
- `enforce`：AI 高信心 spam 可作為一般違規輔助訊號，但不得單獨觸發 `critical + ban`。

實作時應先完成 observe，後續是否開啟 delete-only 或 enforce 必須另外經過實測校準。

### Provider adapter 隔離不同 credential 型態

第一版定義 application port：

```go
type AIClassifier interface {
    Classify(ctx context.Context, input AIClassifyInput) (AIClassifyResult, error)
}
```

application 層不得知道 provider credential 型態，只依賴 `AIClassifier` 與 `EmbeddingProvider` 介面。provider 差異集中在 config、validation、adapter factory 與 infra adapter。

設定採 provider-specific 結構，不使用單一 `api_key` 硬塞所有 provider：

```yaml
ai_detection:
  enabled: false
  mode: observe
  provider: openai_compatible
  timeout: 3s
  max_text_chars: 800
  min_confidence: 0.85

  openai_compatible:
    endpoint: ""
    model: ""
    api_key: "" # ENV 或 Secret Manager 注入

  bedrock:
    region: "us-east-1"
    model_id: ""
    auth_mode: iam_role # iam_role 或 static_keys
    access_key_id: "" # static_keys 時只允許 ENV 或 Secret Manager 注入
    secret_access_key: "" # static_keys 時只允許 ENV 或 Secret Manager 注入
    session_token: "" # 可選，只允許 ENV 或 Secret Manager 注入
```

embedding provider 同樣分開設定，因為 classifier 與 embedding 不一定使用同一 provider：

```yaml
semantic_memory:
  enabled: false
  embedding_provider: openai_compatible

  openai_compatible:
    endpoint: ""
    model: ""
    api_key: ""

  bedrock:
    region: "us-east-1"
    model_id: ""
    auth_mode: iam_role
    access_key_id: ""
    secret_access_key: ""
    session_token: ""
```

驗證規則：

- `provider=openai_compatible` 時必須有 endpoint、model、api key。
- `provider=bedrock` 時必須有 region、model id、auth mode。
- `bedrock.auth_mode=iam_role` 時不要求 access key，正式環境優先使用此模式。
- `bedrock.auth_mode=static_keys` 時必須有 access key id 與 secret access key，session token 可選。
- 所有 secret 欄位只能由環境變數、Secret Manager 或部署平台注入，sample config 不得提供真實值。

OpenAI-compatible、AWS Bedrock、Ollama 或其他 provider 都必須透過 adapter factory 建立，不得讓 detection application 分支處理 credential。

### 固定輸入與輸出 schema

送給 AI 的資料只包含必要上下文：

- 截斷後文字。
- 正規化後的弱訊號列表。
- 規則分數與門檻摘要。
- 是否包含 URL、Telegram mention、交易導流訊號等布林或枚舉。

不得送出：

- Bot Token。
- Webhook secret。
- AI API key。
- DB 憑證。
- 完整 chat title、username 或使用者個資。
- 未截斷的完整長訊息。

AI 必須回傳固定 JSON：

```json
{
  "label": "spam",
  "category": "ad",
  "confidence": 0.92,
  "confidence_source": "model_reported",
  "reason_code": "commercial_solicitation",
  "evidence": ["promises_income", "contact_or_redirect"],
  "safe_action": "delete"
}
```

允許值：

- `label`: `spam`、`ham`、`uncertain`
- `confidence_source`: `model_reported`、`heuristic`、`unavailable`
- `safe_action`: `none`、`delete`、`restrict_candidate`

若 JSON 無法解析、欄位超出允許值、confidence 超出 0 到 1，結果視為失敗並記錄，不得處置。若 provider 沒有可信 confidence，adapter 必須標記 `confidence_source=unavailable`，結果預設視為 `uncertain` 或受更高門檻限制。

### AI 判定不可直接造成封鎖

AI 結果只能作為輔助訊號。嚴重違規直接封鎖仍必須由現有 YAML critical 規則與必要組合訊號共同成立。

原因是 AI 判定不可完全可解釋，且 provider 輸出可能因模型版本或政策改變而漂移。刪除訊息仍可恢復成本較低，封鎖使用者成本高，必須維持規則主導。

### 快取與重送冪等

以既有 content fingerprint 作為 AI 判定與 embedding 快取 key，搭配 provider、model、prompt version、embedding model 與規則版本。TTL 預設 24 小時。

Webhook 重送相同 `update_id` 時不得重複呼叫 AI 或建立重複 AI 判定紀錄。若同內容在短時間多次出現，可重用快取結果降低成本。

### 稽核資料最小化

新增 `ai_detection_events`，欄位建議：

- `id`
- `update_id`
- `chat_id`
- `message_id`
- `user_id`
- `content_fingerprint`
- `provider`
- `model`
- `prompt_version`
- `label`
- `category`
- `confidence`
- `confidence_source`
- `reason_code`
- `evidence`
- `safe_action`
- `status`
- `error_code`
- `retryable`
- `created_at`
- `completed_at`

不保存完整訊息、不保存原始 provider response。錯誤文字需遮蔽憑證與 URL credential。

語意記憶相關資料同樣不得保存完整原文。若需要人工審查代表樣本，應保存截斷摘要或由既有安全日誌窗口臨時查詢，不得把完整訊息長期落盤。

## Risks / Trade-offs

- AI 誤判：預設 observe，封鎖仍需規則主導。
- 成本失控：只處理模糊訊息、設定 timeout、快取、文字長度上限與開關。
- 延遲增加：AI 呼叫需短 timeout，失敗安全降級。
- provider 不可用或 credential 錯誤：轉成穩定錯誤碼，不阻斷既有規則引擎。
- 隱私風險：最小化輸入、截斷文字、不落盤完整原文、不送秘密值。
- 模型輸出漂移：記錄 provider、model、prompt version、schema version 與 confidence source，先以 observe 校準。
- 向量模型漂移：embedding 必須記錄 provider、model、version 與 dimensions，不同 provider/model/version/dimensions 不得混查。
- pgvector 部署成本：正式使用前需確認 PostgreSQL image 或外部資料庫支援 extension。
- 相似案例誤導：相似 spam 只能作輔助訊號，不能單獨 ban。

## Rollout Plan

1. 新增設定、provider port、provider-specific credential、OpenAI-compatible adapter、Bedrock adapter factory 與 AI 判定 domain 型別。
2. 新增 AI 判定 JSON schema 驗證與 prompt version。
3. 新增 AI 判定稽核表與 store。
4. 新增 pgvector schema、embedding provider port、message embedding store 與相似查詢。
5. 新增 `/feedspam` 管理員指令與 manual feed 樣本 store。
6. 在 detection application 中接入 ambiguous-only observe 流程，先查相似案例，再視需要呼叫 AI classifier。
7. 更新 README、sample config、pgvector 部署說明、`/feedspam` 使用方式與查詢 SQL。
8. 以測試資料與測試群組執行 observe，查詢 AI 判定、相似案例、manual feed 樣本與人工判斷差異。
9. 若誤判率可接受，再另開 change 啟用 delete-only 或 enforce 的實際處置策略。

## Open Questions

- 第一個實作 provider 是否先做 OpenAI-compatible，Bedrock adapter 是否同步落地或僅先完成 config/factory 介面。
- embedding provider 是否與 classifier 共用同一 provider，或獨立設定；目前設計採獨立設定。
- pgvector extension 在目前 production PostgreSQL 是否可啟用。
- AI 模糊區間的初始分數上下限需用現有 DB 樣本校準。
- AI 結果是否要在管理指令中查詢，例如 `/ai_last`，首版暫不納入。
- 是否需要把 provider 回傳的簡短 reason 顯示給管理員，首版建議只進 DB 稽核。
