---
filePatterns:
  - '**/*.go'
  - '**/go.mod'
  - '**/go.sum'
---


# Go 開發規範（Extended）

> **參照**：[Uber Go 風格指南](https://github.com/ianchen0119/uber_go_guide_tw) | [Effective Go](https://go.dev/doc/effective_go)
> **Go 版本目標**：Go 1.25+（升版需完成 CI、-race、關鍵路徑壓測與相容性驗證）

---

## Copilot / Agent 產生守則

### 檔案與 Package 規範
- 每檔僅一行 `package <name>` 宣告（置頂）
  - 編輯檔案：保留原 package
  - 新檔案：與資料夾既有 `.go` 同名 package
- 可執行程式置於 `cmd/<app>/main.go`，library 不得含 `main()`
- Package 名稱：**全小寫、單字、無底線**（避免 `util`、`common`）

### Imports 與工具
- 產出前必可通過 `gofmt -s`（建議 `gofumpt`）、`goimports`、`go vet`
- 自動清除未用 imports，避免循環依賴
- 變更 `go.mod` 後提示 `go mod tidy`
- 縮排：Tab；檔尾留單一換行；UTF-8（無 BOM）
- Imports 排序：**標準庫 → 第三方 → 專案內部**；群組以空行分隔

### 錯誤處理與流程
- 呼叫後**立即**檢查 `err`，採 **early return**
- 包裝錯誤：`fmt.Errorf("context: %w", err)`；跨層使用 `errors.Is/As`
- 多重錯誤聚合使用 `errors.Join`（如：驗證列表或 defer Close）
- 訊息小寫開頭，尾端**不加標點**
- 僅在**不可恢復初始化**時用 `panic`；避免在 library 使用
- 禁止「只記錄不回傳」導致錯誤吞沒；**記錄與回傳擇一**

### 函式設計
- 簡短且專注於單一任務（建議不超過 50 行）
- 參數數量控制在 3-4 個以內
- `context.Context` 作為第一個參數；`error` 作為最後一個回傳值
- 避免 `bool` 參數（改用具名 Option 或拆分函式）
- 多參數時採用 **Functional Options Pattern**（`opts ...Option`）

```go
// Functional Options Pattern
type Option func(*config)

func WithTimeout(d time.Duration) Option {
    return func(c *config) { c.timeout = d }
}

func ProcessData(ctx context.Context, data []byte, opts ...Option) (Result, error)
```

### 並行與 I/O 安全
- 每個 goroutine 需有退出機制（`context`、`WaitGroup` 或關閉 channel）
- Channel 緩衝預設 0 或 1（除非有量測證據）
- 嚴禁 goroutine 泄漏；資源關閉落在呼叫點 `defer Close()`
- 不可重用已讀取的 `req.Body`；需 clone：
  ```go
  buf := bytes.Clone(src)
  req.Body = io.NopCloser(bytes.NewReader(buf))
  req.GetBody = func() (io.ReadCloser, error) {
      return io.NopCloser(bytes.NewReader(buf)), nil
  }
  ```
- `io.Pipe`/multipart 必須單執行緒順序寫入
- 底層 slice/map 在**邊界（入/出）**時一律複製，避免別名共享

### HTTP Client 設計
- `Client` 僅存設定（BaseURL、`*http.Client`、headers）；不得保存請求狀態
- 方法皆接收 `context.Context`；內部建 `http.Request` → `Do(req)` → `defer resp.Body.Close()`
- 重用 Transport，設定逾時：
  ```go
  tr := &http.Transport{
      MaxIdleConns:        100,
      IdleConnTimeout:     90 * time.Second,
      TLSHandshakeTimeout: 10 * time.Second,
  }
  c := &http.Client{Transport: tr, Timeout: 15 * time.Second}
  ```
- 重試策略：僅冪等方法，具退避與上限；對 5xx/網路錯誤重試，對 4xx 不重試

### JSON / Struct Tag
- 對外型別欄位加上 `json,yaml,mapstructure` tags；選填欄位 `omitempty`
- 輸入端預設拒絕未知欄位：`dec.DisallowUnknownFields()`
- 使用 `any` 取代 `interface{}`；但優先具體型別
- 時間欄位採 RFC3339（UTC 優先）

### 測試與範例
- 採 **table-driven tests**；子測試用 `t.Run`
- 使用 `t.Context()` 獲取自動管理的 context（Go 1.24+）
- 輔助函式 `t.Helper()`；清理用 `t.Cleanup()`
- 匯出 API 提供 `example_test.go`
- Mocking：使用 `uber-go/mock` 針對 interface 生成 mock，置於 `internal/mocks/`
- 需通過：`-race`、單元涵蓋率門檻（預設 80%）
- 關鍵路徑提供基準與模糊測試（fuzz）

### 產出內容要求
- 輸出完整可編譯檔案或明確 diff
- 多檔變更列出：檔名 / 變更摘要 / 風險
- 新增外部套件需附：`go get <module>@<version>` 與風險評估

---

## Go 一般開發規範

### 通用原則
- 清晰優於巧妙；主流程靠左排列；讓**零值可用**
- 結構自我說明；註解描述「為何」而非「做什麼」

### 命名慣例
- Package：全小寫、單字、無底線；避免 `util`、`common`
- 變數/函式：小駝峰；匯出名稱首字母大寫
- 介面以 `-er` 結尾（Reader/Writer）；小介面優先
- 縮略詞大小寫一致：`HTTPServer`、`URLParser`
- 建構子：`NewType(...)`；常數駝峰式，禁用全大寫底線

### 常數與列舉
- 群組 `const (...)`；型別化常數避免魔數
- Enum 起始值考慮零值可用性，必要時保留 `Unknown`

### 接收者與方法
- 以量測決定指標/值接收者（大型結構/需變異 → 指標；小值/不變 → 值）
- 避免 `init()` 副作用與全域可變狀態
- 大量數據列表優先使用 **Iterators** (`iter.Seq[T]`) 取代 Slice 回傳

### Context 規範
- 對外 API 第一個參數為 `ctx context.Context`
- 禁用 `context.Background()` 直傳至深層；由呼叫者注入
- 設定逾時/截止於呼叫邊界；尊重 `ctx.Done()`
- 不將 `ctx` 保存於結構體

### 並行進階
- 以 `errgroup`/`WaitGroup` + `ctx` 收斂；提供背壓與取消
- 共享狀態以 `sync.Mutex/RWMutex` 或無鎖結構（經量測）保護

---

## Domain Events（領域事件）

- Event 為不可變 struct，包含：`EventID`（UUID）、`OccurredAt`（UTC RFC3339）、`AggregateID`、`EventType`
- 事件命名使用過去式動詞：`OrderCreated`、`PaymentCompleted`
- 發布模式：同步（同一 BC 內，Aggregate Root 回傳 `[]DomainEvent`）/ 非同步（跨 BC，Message Queue）
- **Outbox Pattern**：業務操作與事件寫入同一 DB Transaction → Worker 輪詢發佈 → 標記完成
- Consumer 必須處理重複事件（冪等），使用 `EventID` 去重

---

## 優雅關機（Graceful Shutdown）

- 所有 server、worker、consumer 必須實作優雅關機
- 監聽 `SIGINT`、`SIGTERM`，轉為 `context.Context` 取消
- **推薦使用 `github.com/vincent119/commons/graceful`** 統一管理生命週期
- 關機順序：
  1. `signal.NotifyContext` 接收訊號
  2. 停止接受新請求（HTTP `Shutdown` / gRPC `GracefulStop`）
  3. 等待進行中請求完成
  4. Timeout 後強制結束
  5. 關閉外部資源（DB、Cache、Queue、Tracer）
- 所有 goroutine 必須回應 `ctx.Done()` 並自行結束
- 禁止 `os.Exit()`；禁止 server goroutine 使用 `log.Fatal`
- Kubernetes：`terminationGracePeriodSeconds >= shutdownTimeout + 5~10s buffer`
- 關機路徑必須可測（可注入 cancel 的測試入口）

### 使用 commons/graceful（推薦）

```go
import (
    "github.com/vincent119/commons/graceful"
)

func main() {
    srv := &http.Server{Addr: ":8080"}

    err := graceful.Run(
        graceful.HTTPTask(srv),
        graceful.WithLogger(logger),
        graceful.WithTimeout(10*time.Second),
        graceful.WithCleanup(func(ctx context.Context) error {
            return srv.Shutdown(ctx)
        }),
        graceful.WithCloser(db),
    )
    if err != nil {
        logger.Error("application exited", "error", err)
        os.Exit(1)
    }
}
```

Options：
- `WithTimeout(d)` - Shutdown 超時（預設 30s）
- `WithLogger(l)` - 設定 `*slog.Logger`
- `WithCleanup(f)` - 註冊清理函式（LIFO 順序）
- `WithCloser(c)` / `WithClosers(c...)` - 註冊 `io.Closer` 資源

注意：Cleanup 採 LIFO 順序，先註冊底層資源（DB），再註冊上層服務（HTTP）。

```yaml
# Kubernetes preStop hook
lifecycle:
  preStop:
    exec:
      command: ["sleep", "5"]
```

---

## gRPC 規範

- Proto 檔案統一放置於 `api/proto/<service>/`；使用 buf 管理
- 產生的程式碼放入 `api/gen/go/`（不手動編輯）
- Interceptor 順序：Recovery → OTel → Logging → Auth
- 必須實作 gRPC Health Checking Protocol
- Server 端必須尊重 client deadline；長時間操作檢查 `ctx.Done()`

| Domain Error | gRPC Status |
|--------------|-------------|
| NotFound | `codes.NotFound` |
| ValidationError | `codes.InvalidArgument` |
| Unauthorized | `codes.Unauthenticated` |
| Forbidden | `codes.PermissionDenied` |
| Conflict | `codes.AlreadyExists` |
| Internal | `codes.Internal` |

---

## 日誌與可觀測性

- 使用結構化日誌（zap）；固定欄位：`trace_id`, `span_id`, `req_id`, `subsystem`
- **推薦使用 `github.com/vincent119/zlogger`**
- 指標/追蹤採 OpenTelemetry；所有跨邊界呼叫必須傳遞 `ctx`
- **Prometheus Metrics**：
  - Counter（僅增）、Gauge（可增減）、Histogram（分佈）
  - 命名：蛇形 + 單位後綴（`_seconds`, `_bytes`, `_total`）
  - 禁止高基數 Label（`user_id`, `email`, `trace_id`）
  - 必備 Label：`service`, `env`, `code`

---

## Database Migration

- 工具：`golang-migrate/migrate` 或 `pressly/goose`（全專案統一）
- 命名：`YYYYMMDDHHMMSS_<description>.<up|down>.sql`
- Migration 檔案必須納入 Git；禁止修改已執行的 migration
- 在應用啟動前執行（init container / pre-deploy hook）；禁止在 `main()` 中執行
- 大型表變更使用 pt-online-schema-change 或 gh-ost

---

## 依賴注入 (DI)

- Infrastructure 與 Application 層依賴透過 DI 容器組裝
- 推薦 `uber-go/fx`；統一管理於 `cmd/` 或 `internal/<service>/di.go`
- 禁止在業務邏輯層手動 `New` 具體 Infrastructure 實作
- Repository / Service 皆以 interface 暴露；實作為 private struct
- Mock 使用 `uber-go/mock`，統一置於 `internal/mocks/`

---

## Configuration（設定管理）

- 使用 `spf13/viper`；優先級：ENV > 設定檔 > 預設值
- 敏感資訊禁止放入設定檔（使用 Vault / K8s Secrets）
- 必要設定缺失時啟動階段 `log.Fatal` 終止
- 提供 `config.sample.yaml` 與 `.env.example`

---

## API 設計

### 統一回應結構
```go
type APIResponse[T any] struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    T      `json:"data,omitempty"`
    TraceID string `json:"trace_id,omitempty"`
}
```

### 版本管理
- URL Path 優先：`/v1/`, `/v2/`
- 新增欄位（向下相容）無需升版；移除/修改欄位需 Major bump
- 舊版本至少維護 6 個月；棄用加 `Deprecation: true` Header

### Swagger
- `main.go` 定義全域資訊；Handler 附 Swagger 註解
- 使用 `swag init -g cmd/main.go` 產生

---

## 安全性

- 僅用標準 `crypto/*`；禁自製密碼學
- 外部輸入需驗證與長度限制；避免正則 ReDoS
- 檔案 I/O 使用 `fs.FS` 與限制型讀取；防 Zip Slip
- 納入 `gosec` 於 CI；敏感資訊不得進日誌

---

## 時間與時區

- 內部以 UTC 儲存與運算；輸出呈現再格式化
- JSON 時間使用 RFC3339（必要時 `time.RFC3339Nano`）

---

## 依賴與模組

- 模組遵循 SemVer；破壞性改動於 major path（`/v2`）
- 嚴格釘版；移除依賴需跑 `go mod tidy` 並附影響說明
- CGO 預設關閉；開啟需 PR 說明

---

## 目錄結構

```
.
├── cmd/<app>/main.go          # 進入點
├── api/                       # Proto / OpenAPI 定義與產生碼
├── configs/                   # config.yaml, env.example
├── internal/
│   ├── <service>/             # Bounded Context
│   │   ├── domain/            # Entity, VO, Repository Interface
│   │   ├── application/       # Use Case
│   │   ├── infra/             # Repository Impl, 外部 API
│   │   ├── delivery/          # HTTP/gRPC Handlers, DTO
│   │   └── di.go              # DI 組裝
│   ├── infra/                 # 全域基礎設施（DB, Cache, Logger）
│   └── pkg/                   # Shared Kernel（跨 BC 通用抽象）
├── pkg/                       # 通用工具（無業務邏輯）
├── migrations/                # DB Migration
├── scripts/                   # Makefile 輔助腳本
├── deployments/               # K8s, Helm
├── docs/                      # Swagger, 架構文件
├── test/                      # 整合測試
├── Makefile
├── .golangci.yml
└── go.mod
```

### 架構原則（DDD）
- 每項業務能力為獨立 Bounded Context（`internal/<service>/`）
- 業務規則集中於 Domain Layer，與框架完全解耦
- MVC 僅用於 Delivery Layer
- 基礎設施透過 DI 解耦

### Shared Kernel（`internal/pkg/`）
- 適合：Value Objects（Money, Email）、Domain Error、通用介面（Clock, UUIDGenerator）
- 禁止：特定 BC 的 Entity、Use Case、框架耦合實作、可變 Singleton
- 變更需所有相依 BC 負責人同意；向下相容可直接合併

---

## 常用套件堆疊

| 類別 | 套件 |
|------|------|
| Web | `net/http`, `gin-gonic/gin` |
| DB | `database/sql`, `jmoiron/sqlx`, `gorm.io/gorm` |
| CLI | `spf13/cobra` |
| Config | `spf13/viper` |
| Logger | `vincent119/zlogger` |
| Validator | `go-playground/validator` |
| DI | `uber-go/fx` |
| Metrics | `prometheus/client_golang` |
| Redis | `redis/go-redis/v9` |
| Utils | `vincent119/commons` |
| Graceful | `vincent119/commons/graceful` |
| Swagger | `swaggo/swag` |
| Mock | `uber-go/mock` |

---

## CI 與工具

### Makefile（節選）
```makefile
tidy:
	go mod tidy
lint:
	golangci-lint run ./...
test:
	go test -race -count=1 ./...
bench:
	go test -run=NONE -bench=. -benchmem ./...
cover:
	go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
swagger:
	swag init -g cmd/main.go
```

### .golangci.yml（建議 linters）
errcheck, gocritic, gofumpt, govet, ineffassign, staticcheck, unparam, prealloc, revive, gosec

---

## Review Checklist

- [ ] 僅一個 `package` 宣告；通過 `gofmt -s` / `goimports` / `go vet`
- [ ] `err` 立即檢查並以 `%w` 包裝；跨層以 `errors.Is/As`
- [ ] Goroutine / channel 正確收斂；無泄漏
- [ ] I/O 操作安全（Close、Pipe、Body clone）
- [ ] JSON tag 一致、解碼拒絕未知欄位、零值可用
- [ ] 測試含 table-driven、-race、必要 fuzz/bench
- [ ] Server/Worker 實作 Graceful Shutdown
- [ ] 使用 DI，無業務層手動 `New` 實體
- [ ] 跨邊界呼叫傳遞 `context`（Trace ID）
- [ ] DB Migration 透過版本化腳本管理
- [ ] Domain Event 為不可變 struct，含 EventID 與 OccurredAt
- [ ] 與 Uber / Effective Go 一致或於 PR 註明偏離理由

---

## Do / Don't

**Do**
- 僅產生一個 `package` 宣告；imports 分群
- 立即檢查 `err`，使用 `%w` 包裝
- 所有公開 API 第一參數 `context.Context`
- 在邊界複製 slice/map；為 struct 加 `json`/`yaml` tags
- 撰寫 table-driven 測試 + `t.Helper()`

**Don't**
- 不要保存 `context.Context` 或 `*http.Request` 於 struct
- 不要在 library 使用 `panic`；不要忽略 `Close()`
- 不要以 `interface{}` 取代具體型別；不要暴露可變 slice/map
- 不要在長迴圈內直接 `defer`；必要時以匿名函式縮小 scope
