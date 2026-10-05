# 設計文件：重複處置、人工回饋與判定預覽

## 文件定位與已知契約

本設計對應 [requirements.md](requirements.md) R1～R8，源自 [設計草稿](../drafts/2026-10-05-11-05_Draft-moderation-feedback-workflow/design.md)。只增量擴充 detection、command 與必要資料儲存，不改已完成的 30m／3 規則、原文 HMAC、1 分鐘頻率與協同訊號。

| 契約 | 現況 | 實作方向 |
|------|------|----------|
| `BehaviorStore.Observe` | `[]string`，無歷史訊息 ID | 保留舊方法相容，新增原子 observation／只讀快照介面 |
| `EnforcementAction` | Kind／Key；刪除只用當前 ID | 為歷史清理提供持久化的目標 ID 與結果 |
| `/feedspam` | 只新增 spam，原文只在同步向量化時使用 | 保留舊指令語意，另建可修正的 spam／ham 回饋 |
| 現有向量 | fingerprint＋provider/model/version/dim；無 chat scope | 新人工標記不能直接覆寫他群共享資料 |
| AI 模式 | 獨立 mode 與 app mode 既有優先序 | 僅為新增 repeat policy 定義 app mode 上限；回歸 AI 舊測試 |
| 預覽 | 正式 `Processor.Process` 會寫狀態 | 建立只讀服務，不呼叫正式處理流程 |

不讀取線上秘密值，不假造實際 Bot 權限、已部署模式、Telegram 成功率或模型命中率。

## Bounded Context 與設計原則

包含：repeat 候選快照與處置來源、逐項清理、可修正人工樣本、管理員命令、只讀判定預覽及中文案例。

不包含：網頁後台、貝氏模型、新 AI provider、語意近似重複、全群歷史掃描、自動解封與其他業務模組。

設計原則：先保存可稽核的固定處置目標再執行 Telegram；各來源依自身政策規劃，不能靠偽造 critical 規則來封鎖；外部操作部分失敗逐項回報；正常回饋不覆寫他群證據；預覽不碰正式寫入端口。

## 重複窗口與處置資料流

1. 允許群、更新去重與管理員／Bot／信任成員豁免使用既有流程；新增處置前再次確認保護狀態。
2. Redis 在同一交易內移除窗口外訊息、以穩定 MessageID NX 插入、計數及取得有界候選；Memory 使用等價互斥快照。最多 100 則，包含當前訊息，截斷明示。訊號仍保留於 `Result.Signals` 且不參與規則分數。
3. 新增 `repeat_action`，預設 `observe`，可明示 `delete`／`ban`。`app.mode=observe` 不因 repeat 產生外部處置；`delete-only` 最多刪文；`enforce` 允許設定的 ban。AI 既有模式邏輯不變。
4. 觸發時把 chat、user、fingerprint、窗口、候選 ID、政策版本與逐項動作持久化；刪除動作唯一鍵包含 chat＋目標 message ID，使後續觸發不重複刪同一則。封鎖目標為同群同人，單一觸發只記一次原因，單純 repeat delete 不推進一般違規階梯。
5. 執行前再檢查目標豁免與 Telegram 權限；已確定完成項目跳過，可重試錯誤沿既有重送接續。外部成功而回寫失敗時標為結果未知，不宣稱嚴格 exactly-once。快照持久化後的重送不得再擴大 ID 清單。

現有 `/purge` 48 小時查詢不適合作為 30 分鐘原子快照。首次觀測紀錄因窗口／TTL／容量過期後，不能保證舊 update 永久去重。Redis 計數的並行語意依交易順序，不宣稱每個 client 取樣時間的嚴格右界。

## 人工回饋

- Telegram 指令：`/spam [分類] [delete|ban]` 要求管理員回覆本群有文字或媒體說明的目標；無參數時採未分類垃圾＋刪文，`ban` 必須明示。`/ham [原因]` 可回覆本群訊息，或用 `/ham event:tg:<update_id> [原因]` 修正本群已保存、可查證的偵測事件。找不到事件時明確拒絕；無原文時不得推算文字或建立向量。
- 回饋資料以 chat＋message 及 revision 識別；保留舊標籤與管理員操作紀錄，衝突更新不能讓舊工作反覆寫回。舊 `CreateManualSample` 的 DoNothing 不能用於修正。
- spam／ham 標籤與人工向量以群組生效。現有不帶群組的語意向量不可被此功能局部修正；查詢需檢查來源與有效 revision，AI 快取須在回饋變更後失效或用新版本鍵隔離。
- 樣本保存、向量化與可選處置分開記錄。樣本保存失敗時不開始新處置；向量化失敗可保留已提交標籤並明示未完成。`/ham` 不自動解封、清警告或忽略規則。
- 對無原文目標只能回饋稽核 metadata；沒有可重用向量時明示語意功能未更新。原文不常態存入樣本表。

## 只讀預覽與中文資料

- 管理員回覆 `/check` 以目前規則、mode 與只讀狀態建立摘要：規則版本／分數／門檻／匹配、重複計數與候選可知程度、AI 是否啟用且此次是否執行、處置預估。當時紀錄與目前重算分開標明。
- 預覽不呼叫正式 `Process`、`Observe`、處置或正式 AI cache／audit 寫入；本次不提供即時 AI 呼叫選項。指令仍會驗證管理員權限、記錄執行稽核並回覆。
- 包含六組使用者短廣告、正常煮飯／收款／引用／重複短句；規則、重複政策與 AI stub 分開比對。測試資料不含真人資訊。

## 受影響檔案計畫與風險

| 範圍 | 檔案 | 主要風險 |
|------|------|----------|
| 設定 | `internal/config/config.go`、sample、`.env.example`、main | 升級後意外直接封鎖 |
| 行為狀態 | `internal/detection/application/ports.go`、Redis／Memory behavior | 快照混入窗口外或其他群訊息 |
| 處置 | detection processor／policy、PostgreSQL store、Telegram client | 回溯誤刪、部分失敗重試 |
| 人工回饋 | command domain／handler、manual sample service／store、embedding 查詢 | 重複標註、跨群標籤污染 |
| 預覽 | 新增 preview service、只讀 ports、command、必要 DI | 不小心寫入行為計數 |
| 驗證 | 對應 `*_test.go`、中文測試資料、正式 spec | 空 selector 被當成功 |

若需新增表或欄位，提供可審核、可回退的版本化 SQL 與隔離資料庫驗證；現有啟動仍使用 AutoMigrate，部署前 migration 的採行順序必須在交付說明寫明。任何外部 API 權限與刪除期限由實際測試環境驗證，未驗證時記為限制。

## 需求對應與驗證

| 驗收 | 設計落點 | 驗證 |
|------|----------|------|
| R1～R4 | 原子快照、來源獨立政策、持久化逐項清理 | 重複序列、模式矩陣、並行與重試測試 |
| R5～R6 | 可修正、群組範圍人工樣本與指令 | 命令 E2E、儲存交易、跨群隔離測試 |
| R7 | 只讀預覽服務與管理員入口 | 寫入／Telegram spy 零呼叫測試 |
| R8 | 中文固定資料集 | table-driven 與時間序列測試 |

替代方案：直接用 `/purge` 查詢 48 小時資料會丟失 30 分鐘原子窗口契約，故不採。直接用原有共享向量儲存 ham 會影響其他群，故不採。將重複訊號偽裝成 critical 規則會破壞既有安全邊界，故不採。

## 實作後契約補記

- 重複處置使用 `RepeatSnapshot`、`RepeatPlan`、`repeat_action_executions` 與 `repeat_action_steps`。步驟租約有隨機 fencing token；Telegram 成功但資料庫結果未確認時記為未知，不宣稱一次且僅一次。
- `/ham` 支援回覆現存文字，或 `event:tg:<update_id>` 指向本群已保存事件。後者沒有原文時只保存標籤與稽核，不能重建向量；人工命令的原始參數不進新的稽核摘要。
- `manual_feedback_currents` 與 revision／epoch 保留群組範圍的有效標記；人工向量以 chat、message、revision 限定。AI cache 查詢亦限 chat 與 feedback epoch，修正後不重用舊判定。舊 `/feedspam` 全域向量維持舊範圍，但在本群有相反有效標記時排除其垃圾相似訊號。
- `/check` 由 `PreviewService` 的只讀端口提供；只回純文字固定摘要，沒有即時 AI 呼叫與正式行為狀態寫入。
- 版本化 SQL 依檔名前綴 `11400` 至 `11800` 順序套用；既有啟動 `AutoMigrate` 仍保留。隔離 PostgreSQL 已驗證正反套用及 schema 相容；正式環境尚未執行 migration 或 Bot 權限測試。
