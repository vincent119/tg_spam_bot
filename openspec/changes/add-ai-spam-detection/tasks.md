## 1. 設定模型與啟動驗證

- [x] 1.1 在 `internal/config` 新增 `AI` 或 `AIDetection` 設定結構，包含 `enabled`、`mode`、`provider`、provider-specific config、`timeout`、`max_text_chars`、`min_confidence`、`only_when_ambiguous`、`cache_ttl`
- [x] 1.2 新增 AI 設定預設值：`enabled=false`、`mode=observe`、`timeout=3s`、`max_text_chars=800`、`min_confidence=0.85`、`only_when_ambiguous=true`、`cache_ttl=24h`
- [x] 1.3 綁定 `AI_DETECTION_*` 環境變數，確保 API key、AWS access key、AWS secret access key 與 session token 只由環境變數或 Secret Manager 注入
- [x] 1.4 實作設定驗證，啟用 AI 時要求 provider 對應的必要設定、正數 timeout、合理 confidence 與文字長度上限
- [x] 1.5 更新 `configs/config.sample.yaml`、`.env.example` 與 README，說明首版預設停用與 observe 校準流程
- [x] 1.6 補設定載入與驗證測試，涵蓋預設停用、缺少 provider 欄位、OpenAI-compatible API key、Bedrock IAM role、Bedrock static keys、環境變數覆寫與無效 confidence
- [x] 1.7 新增 `semantic_memory` 設定結構，包含 `enabled`、`embedding_provider`、provider-specific embedding config、`embedding_version`、`embedding_dimensions`、`similarity_threshold`、`spam_similarity_threshold`、`ham_similarity_threshold`、`max_neighbors`、`cache_ttl`
- [x] 1.8 新增語意記憶設定預設值：`enabled=false`、`max_neighbors=5`、`cache_ttl=168h`，並驗證相似度門檻介於 0 到 1
- [x] 1.9 新增 provider-specific config 型別：`openai_compatible` 使用 endpoint、model、api key；`bedrock` 使用 region、model id、auth mode、access key id、secret access key、session token

## 2. AI 判定 domain 與 application port

- [x] 2.1 新增 AI 判定 domain 型別，定義 input、result、label、category、confidence source、reason code、evidence、safe action、prompt version 與穩定錯誤
- [x] 2.2 定義 `AIClassifier` application port，讓 provider adapter 可替換
- [x] 2.3 實作 AI provider 輸出 JSON 驗證，拒絕無效 JSON、缺少欄位、未知 label、未知 safe action、未知 confidence source 與超出範圍 confidence
- [x] 2.4 實作 AI 輸入資料最小化與文字截斷，禁止傳送秘密值、完整個資與未截斷長訊息
- [x] 2.5 補 domain 與 schema 驗證 table-driven tests，涵蓋合法輸出、invalid JSON、未知 enum、低 confidence 與文字截斷
- [x] 2.6 新增 embedding domain 型別與 `EmbeddingProvider` port，定義 vector、provider、model、version、dimensions 與穩定錯誤
- [x] 2.7 補 embedding 型別測試，驗證 provider/model/version/dimensions、空 vector 與維度不一致會被拒絕

## 3. 模糊訊息觸發策略

- [x] 3.1 在 detection application 中定義模糊區間判斷，明確垃圾與明確正常不得呼叫 AI
- [x] 3.2 將弱可疑訊號納入 AI 觸發條件，例如 URL、Telegram mention、交易導流、下載註冊、低權重規則命中、短期相似內容
- [x] 3.3 確保管理員、可信任成員、Bot 訊息、未授權聊天與不支援訊息不呼叫 AI
- [x] 3.4 補 application tests，驗證明確垃圾不呼叫 AI、明確正常不呼叫 AI、模糊訊息呼叫 AI、豁免對象不呼叫 AI
- [x] 3.5 在 AI classifier 前加入語意相似案例查詢，命中高相似 spam 或 ham 時產生輔助訊號
- [x] 3.6 補 application tests，驗證相似 spam 只能提升 observe 輔助訊號，不得單獨 ban

## 4. OpenAI-compatible provider adapter

- [x] 4.1 新增 OpenAI-compatible HTTP adapter，支援 endpoint、model、api key、timeout 與 context cancellation
- [x] 4.2 建立固定 system prompt 與 JSON-only response 要求，並加入 prompt version
- [x] 4.3 實作 HTTP 錯誤分類，區分 provider 權限錯誤、限流、server error、timeout 與無效 response
- [x] 4.4 遮蔽 provider 錯誤中的 API key、URL credential 與敏感 header
- [x] 4.5 補 `httptest` 測試，涵蓋成功、401/403、429、5xx、timeout、invalid JSON、敏感錯誤遮蔽
- [x] 4.6 新增 OpenAI-compatible embedding adapter，支援 endpoint、model、api key、timeout、context cancellation 與 vector 維度驗證
- [x] 4.7 補 embedding adapter 測試，涵蓋成功、空 vector、維度不一致、401/403、429、5xx、timeout 與敏感錯誤遮蔽

## 4A. AWS Bedrock provider adapter

- [x] 4A.1 新增 Bedrock classifier adapter factory，支援 region、model id、`auth_mode=iam_role` 與 `auth_mode=static_keys`
- [x] 4A.2 新增 Bedrock embedding adapter factory，支援 region、model id、auth mode、timeout、context cancellation 與 vector 維度驗證
- [x] 4A.3 實作 AWS credential 載入與驗證，IAM role 模式不得要求 static keys，static keys 模式必須要求 access key id 與 secret access key
- [x] 4A.4 將 Bedrock 錯誤轉成穩定錯誤碼，區分權限錯誤、限流、server error、timeout、invalid response
- [x] 4A.5 遮蔽 Bedrock 錯誤中的 access key、secret access key、session token、URL credential 與敏感 header
- [x] 4A.6 補 Bedrock adapter factory 測試，涵蓋 IAM role、static keys、缺少 region、缺少 model id、缺少 static keys、權限錯誤與敏感錯誤遮蔽

## 5. AI 判定、embedding 稽核與快取

- [x] 5.1 新增 GORM `ai_detection_events` model，含繁體中文 table/column comments、必要索引與唯一鍵，保存 confidence source、provider、model 與 prompt/schema version
- [x] 5.2 實作 AI 判定 store，支援依 `chat_id + update_id` 冪等建立、完成、失敗與既有結果讀取
- [x] 5.3 實作以 content fingerprint、provider、model、prompt version、規則版本為 key 的快取查詢與 TTL 判斷
- [x] 5.4 確保資料庫與日誌不保存完整訊息、provider 原始完整 response、API key、Bot Token 或 Webhook secret
- [x] 5.5 補 PostgreSQL/GORM 整合測試，驗證 AutoMigrate、唯一鍵、重送不重複、快取命中與失敗紀錄
- [x] 5.6 新增 pgvector extension 檢查與 `message_embeddings` model，記錄 content fingerprint、embedding provider、model、version、dimensions、vector、label、category、reason code、created_at、expires_at
- [x] 5.7 實作 embedding store，支援建立、依 fingerprint 查詢、相似度搜尋、TTL 過濾與 provider/model/version/dimensions 隔離
- [x] 5.8 補 pgvector 整合測試，驗證 extension 可用、向量寫入、相似查詢、不同 provider/model/version/dimensions 不混查
- [x] 5.9 新增 `semantic_manual_samples` 或等價 model，保存 `/feedspam` 提交的 chat、message、target user、operator、content fingerprint、label、category、source、狀態與 UTC 時間，不保存完整原文
- [x] 5.10 實作 manual sample store，支援冪等建立、查詢待向量化樣本、標記 embedding 完成與失敗
- [x] 5.11 補 manual sample store 測試，驗證重複提交收斂、受保護欄位不落盤、狀態轉換與 AutoMigrate

## 6. 語意黑名單與批次聚類

- [x] 6.1 新增 `semantic_blacklist_categories` 與 `semantic_blacklist_examples` models，支援分類、範例 embedding、啟用狀態與建立來源
- [x] 6.2 實作語意黑名單查詢，命中後產生 `semantic_blacklist_match` 訊號與分類摘要
- [x] 6.3 確保語意黑名單只作輔助訊號，不得單獨刪除、禁言或封鎖
- [x] 6.4 實作批次聚類 service，針對指定時間範圍的可疑訊息輸出 cluster id、樣本數、建議分類、reason code、signals 與候選 YAML 詞彙摘要
- [x] 6.5 確保聚類報表不包含完整原文或秘密值，且不得自動修改 `configs/rules/*.yaml`
- [x] 6.6 補語意黑名單與聚類測試，涵蓋啟用、停用、相似度門檻、輸出遮蔽與不自動寫 YAML
- [x] 6.7 實作 manual feed 樣本向量化流程，將待處理 `/feedspam` 樣本產生 embedding 後寫入語意記憶庫，並記錄 embedding provider、model、version 與 dimensions

## 7. `/feedspam` 管理指令

- [x] 7.1 擴充 command registry 與 BotFather 指令文件，加入 `/feedspam [分類]`，說明必須回覆漏網垃圾訊息
- [x] 7.2 實作 `/feedspam` command parser，限制分類長度與允許字元，未提供分類時使用 `uncategorized_spam`
- [x] 7.3 在 command application 中實作 `/feedspam`，即時驗證操作者管理員身分，並沿用受保護目標檢查
- [x] 7.4 `/feedspam` 成功時只保存 manual feed 樣本並回覆「已加入待訓練樣本」，不得刪除、警告、禁言、封鎖或修改 YAML
- [x] 7.5 補 command tests，涵蓋成功提交、非管理員拒絕、未回覆訊息、受保護目標、匿名 sender chat、分類格式錯誤與重送冪等
- [x] 7.6 補 Webhook 端到端測試，確認 `/feedspam` 由 command handler 處理且不落入垃圾偵測或自動回覆

## 8. 偵測流程整合與模式政策

- [x] 8.1 將語意相似查詢與 AI 判定插入既有規則偵測之後、違規與處置計畫之前
- [x] 8.2 `ai_detection.mode=observe` 時只寫 AI 判定、語意相似結果與偵測摘要，不建立處置副作用
- [x] 8.3 `ai_detection.mode=delete-only` 時高信心 spam 最多建立刪除候選，不推進警告階梯
- [x] 8.4 `ai_detection.mode=enforce` 時 AI 與語意相似結果只作為一般違規輔助訊號，不得單獨觸發 critical ban
- [x] 8.5 AI provider、embedding provider 或 pgvector 查詢失敗時安全降級，維持既有 YAML 規則判定與處置
- [x] 8.6 補 application tests，涵蓋 observe、delete-only、enforce、低 confidence、AI ham 不覆寫規則 spam、相似 spam 不單獨 ban、AI 或 pgvector 失敗不阻斷規則處置

## 9. 組裝、觀測與文件

- [x] 9.1 在 `cmd/tg-spam-bot` DI 組裝 AI config、classifier、embedding provider、pgvector store、semantic memory、manual sample store 與 detection processor
- [x] 9.2 新增結構化日誌欄位，記錄 `subsystem=ai_detection`、`subsystem=semantic_memory`、provider、model、label、confidence、similarity、status、error_code，不記錄完整原文
- [x] 9.3 更新 README，說明 AI 判定、pgvector、embedding、`/feedspam`、語意黑名單、批次聚類、風險、成本、隱私、observe 校準、provider 設定與故障排查
- [x] 9.4 更新 Docker Compose 環境變數與 PostgreSQL pgvector 說明，保留 AI 與語意記憶預設停用
- [x] 9.5 補充查詢 AI 判定、相似案例、manual feed 樣本、語意黑名單命中與 cluster 報表的 PostgreSQL 範例 SQL

## 10. 驗證與上線校準

- [x] 10.1 執行 `gofmt -s`、`go test ./internal/config ./internal/detection/... ./internal/command/... ./cmd/tg-spam-bot`
- [x] 10.2 執行 `go test ./...`
- [ ] 10.3 在測試群組以 `/feedspam [分類]` 回覆漏網訊息，確認只保存 manual feed 樣本，不刪除、不禁言、不封鎖
- [ ] 10.4 在測試群組以 `ai_detection.enabled=true`、`ai_detection.mode=observe` 驗證模糊廣告會產生 AI 判定與 embedding 紀錄但不刪除
- [ ] 10.5 驗證語意相似垃圾樣本會產生 `semantic_similar_spam` 或 `semantic_blacklist_match`，但不單獨 ban
- [ ] 10.6 驗證明確 YAML 垃圾訊息不呼叫 AI classifier，仍依現有規則處置
- [ ] 10.7 驗證 AI provider、embedding provider timeout、401/403、429、invalid JSON 與 pgvector 查詢失敗只留下錯誤紀錄且不中斷 Webhook
- [ ] 10.8 收集 observe 結果、manual feed 樣本與 cluster 報表後，再決定是否另開 change 啟用 `delete-only`、`enforce` 或自動規則建議流程
