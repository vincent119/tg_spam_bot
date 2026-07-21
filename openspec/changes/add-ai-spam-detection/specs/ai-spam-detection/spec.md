## ADDED Requirements

### Requirement: 可設定 AI 輔助判定

系統 SHALL 透過 `ai_detection` 設定區塊控制 AI 垃圾訊息判定，並在預設狀態停用。

#### Scenario: 預設停用

- **WHEN** `ai_detection.enabled` 未設定
- **THEN** 系統不得呼叫任何 AI provider，且既有 YAML 規則偵測行為不變

#### Scenario: 啟用但缺少必要 provider 設定

- **WHEN** `ai_detection.enabled=true` 且缺少 provider 或該 provider 對應的必要設定
- **THEN** 系統啟動驗證失敗並回報設定錯誤

#### Scenario: 設定環境變數覆寫

- **WHEN** 部署者提供 `AI_DETECTION_*` 環境變數
- **THEN** 系統以環境變數覆寫 YAML 中對應的 AI 設定

### Requirement: 支援 provider-specific credential

系統 MUST 依 AI provider 類型使用對應 credential 設定，不得假設所有 provider 都使用單一 API key。

#### Scenario: OpenAI-compatible credential

- **WHEN** `ai_detection.provider=openai_compatible`
- **THEN** 系統要求 endpoint、model 與 API key，且 API key 只能由環境變數或 Secret Manager 注入

#### Scenario: Bedrock IAM Role credential

- **WHEN** `ai_detection.provider=bedrock` 且 `bedrock.auth_mode=iam_role`
- **THEN** 系統要求 AWS region 與 model id，但不得要求 static access key

#### Scenario: Bedrock static key credential

- **WHEN** `ai_detection.provider=bedrock` 且 `bedrock.auth_mode=static_keys`
- **THEN** 系統要求 AWS region、model id、access key id 與 secret access key，session token 可選

#### Scenario: 隔離 application 與 credential

- **WHEN** detection application 呼叫 AI 判定
- **THEN** application 只依賴 provider-neutral `AIClassifier` 介面，不得分支處理 API key、AWS access key 或 IAM role

### Requirement: 只在模糊訊息呼叫 AI

系統 MUST 只在訊息通過既有聊天、身分與豁免檢查，且規則引擎結果落入模糊區間時呼叫 AI。

#### Scenario: 明確垃圾不呼叫 AI

- **WHEN** YAML 規則與行為訊號已達處置門檻
- **THEN** 系統依既有規則結果處理，不得額外呼叫 AI

#### Scenario: 明確正常不呼叫 AI

- **WHEN** 訊息沒有規則分數且沒有弱可疑訊號
- **THEN** 系統不得呼叫 AI

#### Scenario: 模糊訊息呼叫 AI

- **WHEN** 訊息具備弱可疑訊號但未達 YAML 規則處置門檻
- **THEN** 系統可呼叫 AI provider 取得輔助判定

#### Scenario: 語意相似案例優先

- **WHEN** 語意記憶庫已找到高相似歷史案例且足以產生 observe 輔助訊號
- **THEN** 系統可先保存相似案例結果，並依設定決定是否仍呼叫 AI classifier

#### Scenario: 豁免對象不呼叫 AI

- **WHEN** 訊息來自 Telegram 管理員、可信任成員、Bot、未授權聊天或不支援聊天類型
- **THEN** 系統不得呼叫 AI

### Requirement: 最小化 AI 輸入資料

系統 MUST 只傳送 AI 判定必要資料，並限制文字長度與敏感欄位。

#### Scenario: 截斷輸入文字

- **WHEN** 訊息文字超過 `ai_detection.max_text_chars`
- **THEN** 系統只傳送截斷後文字給 AI provider

#### Scenario: 不傳送秘密值

- **WHEN** 系統建立 AI provider request
- **THEN** request 不得包含 Bot Token、Webhook secret、資料庫憑證、AI API key、完整 Telegram 個資或未截斷完整長訊息

#### Scenario: 傳送可疑訊號摘要

- **WHEN** 系統呼叫 AI provider
- **THEN** request SHALL 包含規則分數、門檻摘要、弱可疑訊號列表與截斷後文字

### Requirement: AI provider 回傳固定 JSON

系統 MUST 要求 AI provider 回傳固定 JSON schema，並拒絕無效或超出允許值的輸出。

#### Scenario: 有效 AI 判定

- **WHEN** provider 回傳 `label`、`category`、`confidence`、`confidence_source`、`reason_code`、`evidence` 與 `safe_action`
- **THEN** 系統驗證欄位型別與允許值後建立 AI 判定結果

#### Scenario: 無效 JSON

- **WHEN** provider 回傳無法解析的 JSON 或缺少必要欄位
- **THEN** 系統記錄 AI 判定失敗且不得依該回應處置訊息

#### Scenario: 不允許的標籤

- **WHEN** provider 回傳的 `label` 不是 `spam`、`ham` 或 `uncertain`
- **THEN** 系統拒絕該結果並記錄穩定錯誤碼

#### Scenario: 信心分數超出範圍

- **WHEN** provider 回傳的 `confidence` 小於 0 或大於 1
- **THEN** 系統拒絕該結果並記錄穩定錯誤碼

#### Scenario: 不允許的信心來源

- **WHEN** provider 回傳的 `confidence_source` 不是 `model_reported`、`heuristic` 或 `unavailable`
- **THEN** 系統拒絕該結果並記錄穩定錯誤碼

### Requirement: AI 結果受模式與信心門檻限制

系統 MUST 依 `ai_detection.mode` 與 `ai_detection.min_confidence` 限制 AI 判定對處置流程的影響。

#### Scenario: Observe 模式只記錄

- **WHEN** `ai_detection.mode=observe` 且 AI 判定為高信心 spam
- **THEN** 系統只保存 AI 判定紀錄，不得刪除、警告、禁言、封鎖或推進違規階梯

#### Scenario: 信心不足視為 uncertain

- **WHEN** AI 回傳 `label=spam` 但 `confidence` 低於 `ai_detection.min_confidence`
- **THEN** 系統將該結果視為 `uncertain` 並不得處置

#### Scenario: 無可信信心分數

- **WHEN** AI 回傳 `confidence_source=unavailable`
- **THEN** 系統不得將該結果提升為可處置 spam，只能保存 observe 稽核或視為 `uncertain`

#### Scenario: Delete-only 模式最多刪除

- **WHEN** `ai_detection.mode=delete-only` 且 AI 高信心判定為 spam
- **THEN** 系統最多建立刪除候選，不得警告、禁言、封鎖或推進 30 天違規階梯

#### Scenario: Enforce 模式不得單獨封鎖

- **WHEN** `ai_detection.mode=enforce` 且 AI 高信心判定為 spam
- **THEN** 系統可將 AI 結果作為一般違規輔助訊號，但不得只依 AI 結果觸發直接封鎖

### Requirement: 保存 AI 判定稽核紀錄

系統 MUST 保存 AI 判定結果與錯誤摘要，且不得保存完整原文或 provider 原始完整 response。

#### Scenario: 保存成功 AI 判定

- **WHEN** AI provider 成功回傳有效判定
- **THEN** 系統保存 update、chat、message、user、content fingerprint、provider、model、prompt version、label、confidence、confidence source、reason code、safe action 與 UTC 時間

#### Scenario: 保存 AI 失敗摘要

- **WHEN** AI provider 呼叫逾時、失敗或回傳無效 JSON
- **THEN** 系統保存穩定錯誤碼、retryable、provider、model 與 UTC 時間

#### Scenario: 不保存完整原文

- **WHEN** 系統寫入 AI 判定紀錄或日誌
- **THEN** 紀錄不得包含完整使用者訊息、AI API key、Bot Token、Webhook secret 或 provider 原始完整 response

### Requirement: AI 判定支援快取與冪等

系統 SHALL 使用內容指紋、provider、model、prompt version 與規則版本建立 AI 判定快取，並避免 Webhook 重送造成重複呼叫。

#### Scenario: 相同 update 重送

- **WHEN** Telegram 重送相同 `update_id`
- **THEN** 系統不得重複呼叫 AI 或建立重複 AI 判定紀錄

#### Scenario: 相同內容命中快取

- **WHEN** 相同內容指紋、provider、model、prompt version 與規則版本在快取 TTL 內已有判定
- **THEN** 系統可重用既有 AI 判定結果，不重新呼叫 provider

#### Scenario: 快取過期

- **WHEN** AI 判定快取超過 `ai_detection.cache_ttl`
- **THEN** 系統可重新呼叫 AI provider 取得新判定

### Requirement: AI provider 失敗安全降級

系統 MUST 在 AI provider 不可用、逾時、限流或回傳錯誤時安全降級。

#### Scenario: Provider 逾時

- **WHEN** AI provider 在 `ai_detection.timeout` 內未回應
- **THEN** 系統記錄可重試錯誤，並維持既有規則判定結果

#### Scenario: Provider 權限錯誤

- **WHEN** AI provider 回傳 API key 無效或權限不足
- **THEN** 系統記錄不可重試錯誤，不得輸出 API key、AWS access key、session token 或完整 provider response

#### Scenario: AI 失敗不阻斷規則處置

- **WHEN** YAML 規則已判定明確垃圾且 AI provider 同時不可用
- **THEN** 系統仍依既有 YAML 規則處置，不得因 AI 失敗阻斷
