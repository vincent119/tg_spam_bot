# 任務文件：獨立重複文案偵測窗口

Status: Complete

## Execution Context

- 意圖：抓取每隔 4～9 分鐘的同帳號精確重複文案，送入既有 AI 候選，不由計數直接處罰。
- 本次交付：使用者以「task go」授權依本 spec 完成實作與驗證。
- 非目標：不改相似文案指紋、AI 政策、協同窗口、歷史清除、秘密值、部署與既有 lint 問題。
- 設計基準：獨立 repeatWindow=30m、repeatThreshold=3，包含當前唯一訊息；短窗口仍 time.Minute。
- 邊界：只修改下列 Allowed Changes，保留既有詞彙修正。
- 關鍵檔案：config、main、Redis／Memory behavior、domain rules、application 回歸測試。
- 完成條件：R1～R9 有實際測試，T1～T6 與 V1～V3 完成並記錄證據；不能用「selector 找不到測試」的成功結果驗收。

### Protected Behavior

- 短窗口頻率維持第 5 則契約；跨帳號協同與入群訊號不延長或重新設計。
- 既有 `BehaviorStore` port、原始文字 HMAC、AI 快取／prompt／provider／mode 不變。
- 既有豁免、明確規則命中、Webhook update 冪等與儲存／AI 錯誤各自契約不變。
- 不變更 `configs/rules/money_laundering_recruitment.yaml` 與 `internal/detection/rules/loader_test.go` 的既有未提交修正。
- 不將重複訊號當作 ban 證據；不把所有重複訊息強制降為 observe；保留既有合法處置。

### 全域 Boundary

#### Allowed Changes

- `internal/config/config.go`、`internal/config/config_test.go`。
- `configs/config.sample.yaml`、`.env.example`。
- `cmd/tg-spam-bot/main.go`。
- `internal/detection/infra/redis/behavior.go`、新增 `behavior_test.go`。
- `internal/detection/infra/memory/store.go`、`store_test.go`。
- `internal/detection/domain/rules.go`、`detector_test.go`。
- `internal/detection/application/ai_policy_test.go`、`ai_processor_test.go`、`processor_test.go`。
- 本目錄的 `requirements.md`、`design.md`、`tasks.md`。

#### Forbidden

- 真實 `configs/config.yaml`、憑證、金鑰、部署設定、Git 提交／推送／發布。
- `configs/rules/**`、既有詞彙補強測試、AI adapter／prompt／政策實作、快取與 schema、違規階梯、Telegram adapter、管理員指令。
- `go.mod`、`go.sum`、資料庫 migration、OpenSpec 歷史與任務狀態。
- 修改 Boundary 外檔案前，先記錄必要性並更新本文件，不能為了讓測試通過擴大功能。

## 任務依賴

| 任務 | Depends | 狀態 |
|------|---------|------|
| T1 設定契約 | 設計確認 | Complete |
| T2 Redis 重複窗口 | T1 | Complete |
| T3 Memory 重複窗口 | 設計確認 | Complete |
| T4 domain 觀察訊號隔離 | 設計確認 | Complete |
| T5 既有 AI 與重試整合測試 | T2、T3、T4 | Complete |
| T6 main 組裝 | T1、T2 | Complete |
| V1 驗收與回歸 | T1～T6 | Complete |
| V2 品質、資源與風險檢查 | V1 | Complete |
| V3 文件與交付 | V1、V2 | Complete |

## 實作任務

### T1：設定契約

- [x] Status: Complete
- Boundary:
  - Allowed Changes：config 與 config_test、config.sample.yaml、.env.example、本 spec。
  - Forbidden：真實設定、AI 設定政策、短窗口設定及其他功能。
- Depends: 設計確認。
- Context: 增加 repeat_window／repeat_threshold 的 Viper 預設、ENV、驗證；未提供 behavior 的舊設定仍可載入。範圍為 1ms～24h、2～100，ENV 優先。同步手動 Config fixture。
- Verify:
  - 新增 `TestLoadBehaviorDefaults`、`TestLoadBehaviorEnvironmentOverrides`、`TestValidateBehavior`，涵蓋預設、優先順序、零／負／越界值。
  - `go test -count=1 ./internal/config -run '^(TestLoadBehaviorDefaults|TestLoadBehaviorEnvironmentOverrides|TestValidateBehavior|TestValidate|TestLoadStructuredSample)$'`。
  - 檢查 sample 與 ENV 名稱一致且未加入秘密值；`git diff --check`。

### T2：Redis 獨立窗口與重送去重

- [x] Status: Complete
- Boundary:
  - Allowed Changes：Redis behavior.go、新增 behavior_test.go、本 spec。
  - Forbidden：更改短窗口頻率／協同政策、整個 Redis namespace 清除、Redis Cluster／Lua 改造、新依賴。
- Depends: T1。
- Context: 相容 constructor option、新 repeat:v2 ZSET、穩定 MessageID、NX 與插入後 count；全部仍在既有 TxPipeline 原子交易內。TTL 不等於 score 生命，NX 去重只涵蓋仍保留的 member；保留 client now，計數依交易序列，不承諾嚴格右界。
- Verify:
  - 新增 R1～R4、R8～R9 對應測試：`TestBehaviorStoreRepeatWindow`、`TestBehaviorStoreRepeatIsolation`、`TestBehaviorStoreRepeatBoundaryAndExpiry`、`TestBehaviorStoreRepeatRetryDoesNotRecount`、`TestBehaviorStoreRepeatConcurrent`、`TestBehaviorStoreRepeatLateRetryAndRequestOrdering`、`TestNewBehaviorStoreRepeatPolicy`、`TestBehaviorStoreObserveError`。
  - 明確測試原始窗口清除／TTL 後的未完成延遲重試可成為新觀測，並驗證不同 client 取樣與交易順序時的 ZCARD 契約，不能把它寫成永久去重／嚴格右界。
  - 新增 `TestBehaviorStoreShortWindowRegression`，涵蓋第 5 則頻率、1 分鐘外不累積頻率與現有協同語意。
  - `go test -race -count=1 ./internal/detection/infra/redis`；測試用既有 miniredis，不連正式 Redis。
  - 檢查舊 key 不刪除、新 namespace 冷啟動、無原文儲存；`git diff --check`。

### T3：Memory 獨立重複狀態

- [x] Status: Complete
- Boundary:
  - Allowed Changes：Memory store.go、store_test.go、本 spec。
  - Forbidden：修改短窗口或其他 UpdateStore／ExemptionStore／ViolationStore 職責，新增背景 goroutine 或對外設定入口。
- Depends: 設計確認。
- Context: 既有 constructor 不變，預設 30m／3 則；使用 Mutex 保護獨立歷史、穩定 ID、保留期內首次時間及左開窗口。每群容量超限採固定到 overflowTime + repeatWindow 的冷卻，不因持續觀測續期；只降級重複線索，短窗口仍按原契約運作。
- Verify:
  - 新增 `TestStoreRepeatWindow`、`TestStoreRepeatIsolation`、`TestStoreRepeatBoundaryAndExpiry`、`TestStoreRepeatRetryDoesNotRecount`、`TestStoreRepeatLateRetry`、`TestStoreRepeatCapacityDegradesSafely`。
  - 既有 `TestStoreObserve` 改用不同 MessageID 模擬多則訊息，不以同一 ID 的重送增加重複計數。
  - `go test -race -count=1 ./internal/detection/infra/memory`；回歸協同、豁免與 update claim。
  - 以注入時鐘驗證容量溢位、持續流量不續冷卻、截止精確邊界、冷卻後重新累計與過期後重送；不以實際睡眠等待 30 分鐘；`git diff --check`。

### T4：domain 觀察訊號隔離

- [x] Status: Complete
- Boundary:
  - Allowed Changes：domain rules.go、detector_test.go、本 spec。
  - Forbidden：改詞彙／別名計分、規則 YAML schema、其他訊號權重、reference 來源政策或既有規則檔。
- Depends: 設計確認。
- Context: 保留 repeated_content 於 Result.Signals，但從可加分／滿足 ban 必要條件的訊號集合排除。直接呼叫 Detector 也須安全，不能只依目前 YAML 未引用該訊號。
- Verify:
  - 新增 `TestDetectorRepeatedContentObservationOnly`，同時驗證 progressive 低分不提升、ban gate 不成立、訊號保留、獨立規則垃圾不降級及其他訊號正常加分。
  - `go test -race -count=1 ./internal/detection/domain -run '^(TestDetectorRepeatedContentObservationOnly|TestDetector|TestDetectorSeparatesReferenceTermsFromSenderSignals)$'`。
  - `go test -count=1 ./internal/detection/rules`，保護既有詞彙與分類；`git diff --check`。

### T5：AI、處置與重試整合驗證

- [x] Status: Complete
- Boundary:
  - Allowed Changes：application 的 ai_policy_test.go、ai_processor_test.go、processor_test.go、本 spec。
  - Forbidden：AI policy／processor／prompt 實作、模式優先序、信心／快取邏輯、Telegram 與違規階梯實作。
- Depends: T2、T3、T4。
- Context: repeated_content 已是弱可疑訊號，不需新增入口。驗證候選到既有模式的完整路徑，含 Observe 成功後紀錄或處置失敗、Release 後再 Observe 的情境。
- Verify:
  - 新增 `TestAITriggerPolicyRepeatedContent`、`TestProcessorRepeatedContentDoesNotAffectRules`、`TestProcessorRepeatedContentWithoutAI`、`TestProcessorRepeatedContentAIModes`、`TestProcessorRepeatedContentRetryDoesNotRecount`、`TestProcessorRepeatedContentExemption`、`TestProcessorRepeatedContentDuplicate`。
  - 重試測試使用具有保留期內穩定訊息去重的實際 behavior adapter 或可觀察該契約的測試替身，不能只計算 fake 呼叫次數。
  - 豁免與已完成重送需以 behavior／classifier spy 明確斷言 Observe／AI 零呼叫；既有只驗 Telegram actions 的斷言不充分。
  - `go test -race -count=1 ./internal/detection/application`，回歸既有 AI modes／安全降級、Processor modes／豁免／duplicate、PlanActions。
  - 確認 AI delete-only 與 app observe 的既有有效模式不被改寫，且高信心 AI 一般階梯不等於首次 critical ban；`git diff --check`。

### T6：啟動組裝

- [x] Status: Complete
- Boundary:
  - Allowed Changes：cmd/tg-spam-bot/main.go、本 spec。
  - Forbidden：啟用 AI、切換 app.mode、修改 secrets、HTTP／DB／Telegram／命令組裝或 graceful shutdown。
- Depends: T1、T2。
- Context: 呼叫既有 Redis constructor，保留 time.Minute，只將已驗證的 config repeat 政策注入新 option。
- Verify:
  - `go test -count=1 ./cmd/tg-spam-bot ./internal/config ./internal/detection/infra/redis` 確認組裝可編譯且相關測試通過。
  - 人工檢查短窗口參數仍是 time.Minute、預設與 ENV 政策正確連到 adapter，無其他模式變更；`git diff --check`。

## 驗證任務

### V1：驗收覆蓋與完整回歸

- [x] Status: Complete
- Boundary:
  - Allowed Changes：全域 Allowed Changes 內的測試與本 spec 驗證紀錄。
  - Forbidden：為消除無關回歸而修其他模組，或把未跑測試記為完成。
- Depends: T1～T6。
- Context: R1～R9 都要實際執行；新增 selector 尚不存在時不得用「沒有測試」當通過。
- Verify:
  - 先用 `go test -list 'Test.*(Behavior|Repeat)' ./internal/config ./internal/detection/infra/redis ./internal/detection/infra/memory ./internal/detection/domain ./internal/detection/application`，核對本文件列出的新增名稱。
  - `go test -race -count=1 ./...`，包含 Protected Behavior 與原始截圖重複序列。
  - 跨副本並行與重送測試需重複執行以確認原子性；要求的是處理結果一致，不是固定哪個 goroutine 第三個完成。

### V2：品質、資源與風險檢查

- [x] Status: Complete
- Boundary:
  - Allowed Changes：本 spec、任務邊界內的格式與測試調整。
  - Forbidden：修復全專案既有 lint 問題、連線或更動正式環境、發布映像。
- Depends: V1。
- Context: 上一輪完整 lint 有 14 個位於未修改檔案的既有問題；本輪需重新記錄結果，不能假設已修好，也不得把它當本功能免驗證理由。
- Verify:
  - [x] 僅對本次 Go 修改執行 gofmt／goimports／gofumpt，無格式 diff。
  - [x] `go vet ./...`；`golangci-lint run ./...`，區分新增問題與既有基準，若仍有既有問題明確記錄為限制，不宣稱完整 lint 通過。
  - [x] 重複狀態不保存原文；TTL、滑動清除、Memory 容量冷卻有測試，使用合成流量記錄資源觀測方法與結果，不假造線上用量。
  - [x] 設定與多副本冷啟動限制已記錄，不讀秘密值；AI 候選增加仍沿用既有 cache。
  - [x] 受影響核心測試覆蓋率依專案門檻檢查；未達時記錄缺口，不擴大修改其他模組。
  - [x] `git diff --stat` 與 `git diff --check`，確認無 Boundary 外修改。

### V3：文件一致性與交付

- [x] Status: Complete
- Boundary:
  - Allowed Changes：本 spec 三文件。
  - Forbidden：將本功能標為已部署、替其他 OpenSpec 任務打勾、擅自提交／推送。
- Depends: V1、V2。
- Context: 任務、測試名稱、實際 constructor option 與設定名稱需一致；Complete 只表示程式交付與必要驗證完成，不表示部署。
- Verify:
  - requirements 的各情境都有對應測試與執行證據；狀態如實更新，未完成工作不得打勾。
  - 實際介面、TTL、去重、設定及模式行為與 design 一致。
  - 交付摘要包含變更、驗證、既有 lint 限制、冷啟動與待部署狀態；`git diff --check`。

## 實作中斷恢復

恢復時先讀本文件的 Execution Context、目前未完成任務、Protected Behavior 與 Implementation Notes，再按需要讀 requirements／design。不掃描其他無關 spec，不因目前工作樹已有詞彙變更而重寫它。

## Implementation Notes

- 2026-10-05：先完成 SDD 文件；使用者以「task go」確認後開始實作。
- T1～T6 與 V1～V3 已完成；Complete 表示程式與驗證交付，未提交、推送或部署。完整 lint 仍有下列既有問題，並非全專案零警告。
- 原規則分類未引用 repeated_content，但變更前 domain 允許 RequireAny 對匹配行為加分；T4 已排除 repeated_content 加分與 ban gate，其他訊號仍照舊。
- Redis Observe 已使用原子交易；主要新增是分離窗口、穩定 ID 與 NX 後計數，不是聲稱舊流程完全沒有原子性。
- Processor 下游失敗會 Release update claim，不能只依 Claim 解決行為重送計數。
- 指紋使用原始文字；AI cache、稽核、批次清除共享此身份，不能在本功能順便正規化。
- 文件審查已釐清有限去重保留期、client 取樣與交易順序差異、Memory 固定冷卻期限；豁免／duplicate 的零呼叫驗收需要明確 spy 斷言。
- 後續需要修改 Boundary 外程式才能驗證時，先更新本文件並說明必要性。

## 驗證結果摘要

驗證日期：2026-10-05；僅本機測試與 miniredis，未連正式服務。

- `go test -list 'Test.*(Behavior|Repeat)'`：在五個受影響套件確認 28 個測試名稱，R1～R9 都有對應實測。
- `go test -race -count=1 ./...`：完整通過；main 編譯通過，該 package 沒有測試檔案。
- `go test -race -count=50 ./internal/detection/infra/redis -run '^(TestBehaviorStoreRepeatConcurrent|TestBehaviorStoreRepeatRetryDoesNotRecount)$'`：通過。兩個獨立 Redis client 測試唯一訊息第三則命中與 12 次並行同 ID 重送只保留一則。
- `TestProcessorRepeatedContentRetryDoesNotRecount`：實際 Redis adapter 搭配失敗注入，驗證紀錄／Telegram 失敗後 Release 與重試，ZCARD 不因重送增加。
- `TestProcessorRepeatedContentAIModes`：11 個案例，含 ham、uncertain、低信心、不可用信心、provider 失敗與既有模式優先序；只有重複訊號時，前兩則不呼叫 classifier，第三則才成為候選。
- `TestDetectorRepeatedContentObservationOnly`：7 個案例，含停用分類不因重複被啟用、其他訊號仍可加分／封鎖，以及獨立規則垃圾不降級。
- `go vet ./...`：通過。僅本次 12 個 Go 檔執行格式化，gofmt／goimports／gofumpt 清單檢查無輸出；`git diff --check` 通過。
- 變更邊界：只有 Allowed Changes 內的 17 個檔案有本輪修改；另保留既有詞彙兩檔的 77 行新增、2 行刪除，沒有追加修改。
- requirements／design／tasks 已同步為實際設定名稱、constructor option、模式與完成狀態；文件另檢查 UTF-8、單一尾端換行、空白與程式碼區塊配對。

### 覆蓋率

先跑各套件自身測試；Memory、domain、application 的該項統計低於 80%，未包含其他套件對它們的整合測試。因此另用完整測試加 `-coverpkg`，量測相同五個受影響套件的聯集，並非改變覆蓋率分母或聲稱全專案覆蓋率。補上範圍內的停用分類安全反例後，最終命令如下：

```sh
go test -race -count=1 -coverpkg=./internal/config,./internal/detection/infra/redis,./internal/detection/infra/memory,./internal/detection/domain,./internal/detection/application -coverprofile=/private/tmp/tg-repeat-coverage.gQWXbQ/final.out ./...
go tool cover -func=/private/tmp/tg-repeat-coverage.gQWXbQ/final.out
```

完整測試通過；以相同來源區塊合併跨測試執行結果後，五套件總覆蓋率 **84.2%**，各套件均達 80%：

| 套件 | 已覆蓋／總敘述 | 覆蓋率 |
|------|---------------|--------|
| config | 168／206 | 81.6% |
| detection/application | 283／342 | 82.7% |
| detection/domain | 162／202 | 80.2% |
| detection/infra/memory | 109／116 | 94.0% |
| detection/infra/redis | 44／44 | 100.0% |

新增 `observeRepeat`、Redis constructor／option／Observe、domain `Detect` 均為 100%；Memory `Observe` 為 97.1%。覆蓋率檔位於本機暫存目錄，不納入 Git，清理暫存後需重跑命令產生。

### 合成資源觀測

- `go test -count=1 -v ./internal/detection/infra/redis -run '^TestBehaviorStoreRepeatRetention$'`：模擬每分鐘一則、共 120 則；30 分鐘窗口最後保留 30 個穩定 ID，TTL 為 30 分鐘，member 不含原文，舊 namespace 未被刪除。
- Memory 容量設 5，插入第 6 則後清除重複狀態；連續 30 分鐘重送不延長冷卻、也不新增重複紀錄，短窗口 high_frequency 照常。截止後從第一則重新累計。
- 這些是資料保留量與邊界驗證，不是正式 Redis 的位元組用量、P99 延遲或線上 AI 候選量測。

### Lint 既有限制

`golangci-lint run ./...` 仍回傳 14 項既有問題；本輪變更檔案沒有新增 lint 問題：

- gocritic 1 項：`internal/detection/delivery/telegram/types.go:191`。
- gofumpt 1 項：`internal/detection/application/manual_feed_service_test.go:134`。
- gosec 4 項：`internal/detection/infra/ai/openai_compatible_test.go:54`、`internal/infra/logging/logger.go:169`、`internal/infra/logging/logger_test.go:56`、`:313`。
- revive 8 項：application 的 `ai_policy.go:10`、`ai_processor.go:15`、`manual_sample.go:10`，`internal/detection/di.go:1`，domain 的 `ai.go:14`、`:23`、`:32`、`ai_test.go:114`。

建議另開修復範圍處理上述問題，本輪未修改這些檔案，也不宣稱完整 lint 通過。

### 發布與剩餘風險

- 程式尚未提交、推送或部署；未讀取真實設定，未啟用 AI 或更改 app／AI 模式。
- 新 repeat:v2 狀態在部署後冷啟動，不回補歷史；所有副本需使用相同政策、HMAC 金鑰與同步時鐘。
- 精確文案仍不合併繁簡、大小寫、空白或金額替換；長窗口增加的資源與 AI 候選量需要部署後觀察。
- 已過保留期的未完成訊息重試不保證永久去重；並行計數不承諾相對每次 client 取樣 now 的嚴格右界。

## 後續改善

- 相似文案、交替文案或數字替換的指紋策略需另行規劃，不包含在本次交付。
- 短窗口重送計數、協同 Set 逐成員滑動過期及 Redis／Memory 既有差異另案處理。
