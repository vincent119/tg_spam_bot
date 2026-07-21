## Why

現有 YAML 規則能處理明確關鍵字、連結與行為訊號，但新型廣告與詐騙文案會頻繁變形，例如「抖音禮物項目」、「有號就能做」、「趁早上車」這類低關鍵字密度內容。需要加入 AI 輔助判斷模糊訊息，降低人工補詞頻率，同時避免 AI 誤判直接造成刪除或封鎖。

## What Changes

- 新增 AI 垃圾訊息判定能力，僅在規則引擎結果落入模糊區間時呼叫。
- 新增 `ai_detection` 設定區塊，支援啟用開關、模式、provider-specific credential、timeout、信心門檻、文字長度上限與快取 TTL。
- 首版預設 `enabled=false`、`mode=observe`、`only_when_ambiguous=true`，AI 結果只記錄，不直接擴權。
- 支援 OpenAI-compatible HTTP provider 與 AWS Bedrock credential 設計邊界；OpenAI-compatible 使用 API key，Bedrock 優先使用 IAM Role，必要時才使用 static access key。
- 新增 AI 判定稽核資料，只保存必要識別碼、內容指紋、provider、model、label、confidence、confidence source、reason code 與錯誤摘要，不保存完整訊息。
- 新增語意垃圾記憶庫，使用 embedding 與 pgvector 比對語意相似垃圾樣本。
- 新增 `/feedspam [分類]` 管理員指令，讓管理員以回覆訊息方式提交漏網垃圾樣本，作為後續 embedding 與語意相似判斷的人工入口。
- 在 AI 判定前先查詢相似歷史案例；高相似 spam 樣本可提高可疑分數或進入 observe 記錄，但首版不得直接封鎖。
- 支援語意黑名單分類，例如兼職拉人、代理招募、投資導流、色情引流、VPN 客服冒充、交易所轉 U。
- 支援批次聚類分析，將一段時間內的可疑訊息分群，供人工轉成 YAML 規則。
- 將 AI 判定結果納入既有 detection event 的可解釋摘要，但不得取代 YAML 規則版本與既有處置冪等流程。
- 文件補充部署設定、風險、成本控制、隱私限制與 observe 校準流程。

## Capabilities

### New Capabilities

- `ai-spam-detection`: 定義 AI 輔助垃圾訊息判定、模糊訊息觸發條件、provider 介面、結構化輸出、稽核紀錄、模式限制與安全降級行為。
- `semantic-spam-memory`: 定義 embedding 產生、pgvector 儲存、管理員漏網樣本提交、相似案例查詢、語意黑名單、批次聚類與人工規則生成輔助流程。

### Modified Capabilities

- `telegram-spam-detection`: 補充規則引擎與 AI 判定的協調行為，包含模糊區間、AI 高信心結果如何進入既有模式政策，以及 AI 失敗不得阻斷既有規則處置。

## Impact

- 影響 `internal/config`，新增 AI 設定模型、provider-specific credential、預設值、環境變數綁定與驗證。
- 影響 detection domain/application，新增 AI 判定 port、模糊區間判斷、AI 結果合併與安全模式政策。
- 影響 command domain/application，新增 `/feedspam` 指令、管理員授權、受保護目標檢查與樣本提交稽核。
- 影響 infra，新增 OpenAI-compatible 與 Bedrock provider adapter/factory 設計、逾時、錯誤分類、輸出 JSON 驗證與內容最小化。
- 影響 PostgreSQL GORM model 與 AutoMigrate，新增 AI 判定稽核表。
- 影響 PostgreSQL 部署，正式使用語意記憶時需要啟用 `pgvector` extension，並新增向量欄位、索引與查詢。
- 影響 infra，新增 embedding provider client 與向量查詢 repository。
- 影響 README、`configs/config.sample.yaml`、`.env.example` 與部署說明。
- 新增外部 AI 與 embedding 服務呼叫風險：成本、延遲、可用性、隱私、誤判、向量模型版本漂移與 provider 政策差異。
