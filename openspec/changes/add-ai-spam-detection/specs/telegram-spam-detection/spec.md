## ADDED Requirements

### Requirement: 與 AI 輔助判定協調

系統 SHALL 在既有 YAML 規則與行為訊號判定後，依 AI 設定決定是否對模糊訊息執行 AI 輔助判定。

#### Scenario: 規則優先

- **WHEN** YAML 規則與行為訊號已達明確垃圾門檻
- **THEN** 系統依既有規則結果建立偵測、違規與處置計畫，不得等待或依賴 AI 判定

#### Scenario: AI 補充模糊訊息

- **WHEN** 訊息未達 YAML 規則處置門檻但符合 AI 模糊觸發條件
- **THEN** 系統可執行 AI 輔助判定並將 AI 結果寫入偵測摘要

#### Scenario: AI 不降低既有判定

- **WHEN** YAML 規則已判定訊息為垃圾，但 AI 判定為 ham 或 uncertain
- **THEN** 系統不得用 AI 結果覆寫既有 YAML 垃圾判定

#### Scenario: AI 不單獨觸發嚴重封鎖

- **WHEN** AI 高信心判定為 spam 但 YAML critical 規則與必要組合訊號未成立
- **THEN** 系統不得執行首次嚴重違規直接封鎖

#### Scenario: AI 失敗安全忽略

- **WHEN** AI 判定失敗或逾時
- **THEN** 系統維持既有 YAML 規則偵測結果，並只保存 AI 失敗稽核紀錄
