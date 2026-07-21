# AWS Bedrock CLI 驗證指令

本目錄放置 `tg_spam_bot` 使用 AWS Bedrock 的最小權限 policy 與 CLI 驗證指令。

注意：Bedrock `Converse` API 的 IAM action 由 `bedrock:InvokeModel` 授權。專案目前不使用串流回應，因此 policy 不需要 `bedrock:InvokeModelWithResponseStream`。

預設模型：

- AI classifier：`amazon.nova-micro-v1:0`
- Embedding：`amazon.titan-embed-text-v2:0`
- Region：`us-east-1`
- Auth mode：`iam_role`

## 1. 設定變數

```sh
export AWS_REGION=us-east-1
export BEDROCK_CLASSIFIER_MODEL_ID=amazon.nova-micro-v1:0
export BEDROCK_EMBEDDING_MODEL_ID=amazon.titan-embed-text-v2:0
```

## 2. 確認目前 AWS 身分

```sh
aws sts get-caller-identity
```

## 3. 確認可列出 Bedrock foundation models

```sh
aws bedrock list-foundation-models \
  --region "$AWS_REGION" \
  --by-output-modality TEXT
```

## 4. 測試 classifier 模型

此指令使用 Bedrock Runtime `converse`，對應專案內 `BedrockClassifier`。

```sh
aws bedrock-runtime converse \
  --region "$AWS_REGION" \
  --model-id "$BEDROCK_CLASSIFIER_MODEL_ID" \
  --system '[{"text":"你只回傳 JSON，不要回傳 Markdown。"}]' \
  --messages '[{"role":"user","content":[{"text":"請判斷這句是否是 Telegram 垃圾廣告，並只回傳 {\"label\":\"spam\"}：抖音礼物项目｜无需经验｜有号就能做｜趁早上车 @test"}]}]' \
  --inference-config '{"temperature":0,"maxTokens":128}'
```

## 5. 測試 embedding 模型

此指令使用 Bedrock Runtime `invoke-model`，對應專案內 `BedrockEmbedding`。

```sh
aws bedrock-runtime invoke-model \
  --region "$AWS_REGION" \
  --model-id "$BEDROCK_EMBEDDING_MODEL_ID" \
  --content-type application/json \
  --accept application/json \
  --cli-binary-format raw-in-base64-out \
  --body '{"inputText":"抖音礼物项目｜无需经验｜有号就能做｜趁早上车","dimensions":1024,"normalize":true}' \
  /tmp/tg_spam_bot_bedrock_embedding.json
```

查看 embedding 維度：

```sh
jq '.embedding | length' /tmp/tg_spam_bot_bedrock_embedding.json
```

預期結果是 `1024`。

## 6. 專案 `.env` 對應設定

```env
AI_DETECTION_ENABLED=true
AI_DETECTION_MODE=observe
AI_DETECTION_PROVIDER=bedrock
AI_DETECTION_BEDROCK_REGION=us-east-1
AI_DETECTION_BEDROCK_MODEL_ID=amazon.nova-micro-v1:0
AI_DETECTION_BEDROCK_AUTH_MODE=iam_role

SEMANTIC_MEMORY_ENABLED=true
SEMANTIC_MEMORY_EMBEDDING_PROVIDER=bedrock
SEMANTIC_MEMORY_EMBEDDING_VERSION=v1
SEMANTIC_MEMORY_EMBEDDING_DIMENSIONS=1024
SEMANTIC_MEMORY_BEDROCK_REGION=us-east-1
SEMANTIC_MEMORY_BEDROCK_MODEL_ID=amazon.titan-embed-text-v2:0
SEMANTIC_MEMORY_BEDROCK_AUTH_MODE=iam_role
```

## 7. 常見錯誤檢查

`AccessDeniedException`：

- IAM role 未掛到 EC2 instance profile。
- IAM policy 缺少 `bedrock:InvokeModel`。
- Model access 未在 Bedrock console 啟用。
- Region 與 model id 不匹配。

`UnrecognizedClientException` 或 credential 錯誤：

- 容器無法讀取 EC2 instance metadata。
- EC2 IMDSv2 hop limit 太低。
- 使用 static keys 時，access key id 或 secret access key 設錯。

`ValidationException`：

- `model-id` 錯誤。
- classifier 模型不支援 `converse`。
- embedding body 格式與模型不相容。
