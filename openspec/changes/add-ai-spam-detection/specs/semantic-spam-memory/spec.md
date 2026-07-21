## ADDED Requirements

### Requirement: 使用 pgvector 保存語意垃圾記憶

系統 SHALL 使用 PostgreSQL pgvector 保存訊息 embedding 與語意標籤，作為相似垃圾訊息比對的資料來源。

#### Scenario: pgvector 未啟用

- **WHEN** `semantic_memory.enabled=true` 但 PostgreSQL 未啟用 pgvector extension
- **THEN** 系統啟動或健康檢查 MUST 回報設定錯誤，不得靜默降級為無向量查詢

#### Scenario: 保存 embedding

- **WHEN** 模糊訊息需要進入語意記憶流程
- **THEN** 系統保存 content fingerprint、embedding provider、embedding model、embedding version、embedding dimensions、vector、label、category、reason code 與 UTC 時間

#### Scenario: 不保存完整原文

- **WHEN** 系統保存 message embedding 或語意黑名單樣本
- **THEN** 資料庫不得保存完整使用者訊息、Bot Token、Webhook secret、AI API key 或 provider 原始完整 response

### Requirement: AI 判定前查詢相似歷史案例

系統 MUST 在呼叫 AI classifier 前，查詢語意相似的歷史樣本，並將結果作為輔助訊號。

#### Scenario: 命中高相似 spam

- **WHEN** 新訊息與歷史 spam 樣本的相似度達到 `semantic_memory.spam_similarity_threshold`
- **THEN** 系統加入 `semantic_similar_spam` 輔助訊號並保存命中摘要

#### Scenario: 命中高相似 ham

- **WHEN** 新訊息與歷史 ham 樣本的相似度達到 `semantic_memory.ham_similarity_threshold`
- **THEN** 系統加入 `semantic_similar_ham` 輔助訊號，但不得覆寫明確 YAML 垃圾判定

#### Scenario: 無相似案例

- **WHEN** 查詢不到達門檻的相似歷史案例
- **THEN** 系統可繼續呼叫 AI classifier 或只保存 observe 紀錄

#### Scenario: 相似案例不得單獨封鎖

- **WHEN** 僅命中高相似 spam 但未命中 YAML critical 規則與必要組合訊號
- **THEN** 系統不得執行直接封鎖

### Requirement: 管理員提交漏網垃圾樣本

系統 SHALL 提供 `/feedspam [分類]` 管理員指令，讓管理員以回覆訊息方式提交未命中的垃圾樣本，保存最小化樣本資料供後續 embedding 與語意相似判斷使用。

#### Scenario: 管理員提交漏網訊息

- **WHEN** Telegram 管理員回覆一般成員訊息並執行 `/feedspam [分類]`
- **THEN** 系統保存 chat、message、target user、operator、content fingerprint、label、category、source 與 UTC 時間，且不保存完整原文

#### Scenario: 未提供分類

- **WHEN** 管理員執行 `/feedspam` 但未提供分類
- **THEN** 系統使用預設分類 `uncategorized_spam` 保存樣本

#### Scenario: 拒絕受保護目標

- **WHEN** 目標是管理員、Bot、可信任成員、匿名 sender chat 或缺少 user ID
- **THEN** 系統拒絕提交樣本且不得保存 embedding 或 manual feed 紀錄

#### Scenario: 不立即處置

- **WHEN** `/feedspam` 成功保存漏網樣本
- **THEN** 系統不得立即刪除、警告、禁言、封鎖、修改 YAML 或改變既有違規階梯

#### Scenario: 樣本後續向量化

- **WHEN** manual feed 樣本尚未產生 embedding
- **THEN** 系統可由後續 worker 或離線流程產生 embedding 並寫入語意記憶庫，且必須維持模型與版本隔離

### Requirement: 語意黑名單分類

系統 SHALL 支援以語意範例建立黑名單分類，讓相近話術可產生可解釋輔助訊號。

#### Scenario: 命中語意黑名單

- **WHEN** 訊息 embedding 與啟用的語意黑名單分類範例達到相似度門檻
- **THEN** 系統加入 `semantic_blacklist_match` 訊號，並記錄分類 ID、名稱與相似度

#### Scenario: 停用分類

- **WHEN** 語意黑名單分類被停用
- **THEN** 系統不得讓該分類影響 AI 判定、規則分數或處置候選

#### Scenario: 分類只作輔助訊號

- **WHEN** 訊息只命中語意黑名單但沒有其他垃圾訊號
- **THEN** 系統不得單獨刪除、禁言或封鎖該訊息發送者

### Requirement: 批次聚類產生規則建議

系統 SHALL 支援對一段時間內的可疑訊息 embedding 做批次聚類，供人工產生 YAML 規則。

#### Scenario: 聚類可疑訊息

- **WHEN** 管理者針對指定時間範圍執行語意聚類
- **THEN** 系統輸出 cluster id、樣本數、建議分類、代表 reason code、signals 與候選 YAML 詞彙摘要

#### Scenario: 不自動修改 YAML

- **WHEN** 批次聚類產生候選規則建議
- **THEN** 系統不得自動寫入或修改 `configs/rules/*.yaml`

#### Scenario: 排除敏感內容

- **WHEN** 系統輸出聚類報表
- **THEN** 報表不得包含完整使用者訊息、秘密值或 provider 原始完整 response

### Requirement: 向量模型版本隔離

系統 MUST 記錄 embedding provider、model、version 與 dimensions，且不同向量模型或維度不得混合查詢。

#### Scenario: 相同模型查詢

- **WHEN** 系統查詢相似訊息
- **THEN** 查詢 SHALL 限定相同 embedding provider、model、version 與 dimensions 的向量資料

#### Scenario: 模型版本更換

- **WHEN** 部署者變更 embedding model 或 version
- **THEN** 系統不得將新向量與舊模型向量直接比較，並需允許後續批次重建索引

#### Scenario: 向量維度不一致

- **WHEN** embedding provider 回傳的 vector dimensions 與設定或既有索引 dimensions 不一致
- **THEN** 系統拒絕寫入該 embedding 並記錄穩定錯誤碼，不得混入相似查詢索引

### Requirement: 語意記憶安全降級

系統 MUST 在 embedding provider 或 pgvector 查詢失敗時安全降級，不阻斷既有 YAML 規則。

#### Scenario: Embedding provider 失敗

- **WHEN** embedding provider 逾時、限流或回傳錯誤
- **THEN** 系統記錄錯誤摘要並跳過語意相似查詢，不得阻斷既有規則判定

#### Scenario: pgvector 查詢失敗

- **WHEN** pgvector 查詢失敗
- **THEN** 系統記錄錯誤摘要並維持既有 YAML 規則與 AI classifier 流程

### Requirement: 支援 embedding provider-specific credential

系統 MUST 依 embedding provider 類型使用對應 credential 設定，不得假設所有 embedding provider 都使用單一 API key。

#### Scenario: OpenAI-compatible embedding credential

- **WHEN** `semantic_memory.embedding_provider=openai_compatible`
- **THEN** 系統要求 endpoint、model 與 API key，且 API key 只能由環境變數或 Secret Manager 注入

#### Scenario: Bedrock embedding IAM Role credential

- **WHEN** `semantic_memory.embedding_provider=bedrock` 且 `bedrock.auth_mode=iam_role`
- **THEN** 系統要求 AWS region 與 model id，但不得要求 static access key

#### Scenario: Bedrock embedding static key credential

- **WHEN** `semantic_memory.embedding_provider=bedrock` 且 `bedrock.auth_mode=static_keys`
- **THEN** 系統要求 AWS region、model id、access key id 與 secret access key，session token 可選

#### Scenario: 隔離 application 與 embedding credential

- **WHEN** detection application 或 manual feed worker 產生 embedding
- **THEN** application 只依賴 provider-neutral `EmbeddingProvider` 介面，不得分支處理 API key、AWS access key 或 IAM role
