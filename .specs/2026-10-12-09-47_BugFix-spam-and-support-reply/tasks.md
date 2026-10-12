# 任務：漏判文案與錯誤客服回覆

Status: Complete

## Execution Context

- 意圖：補足七種漏判文案，移除客服回覆，保留官網下載連結。
- 非目標：AI、DB、客服服務、exclude_keywords、部署。
- 已定決策：具體詞組與 progressive；移除整個 support 規則。
- 邊界與關鍵檔案：design 所列規則、sample、corpus、matcher_test、README。
- 完成條件：正負回歸與品質檢查完成。

## Protected Behavior

官網與下載詢問正常回覆；引用不產生發送者聯絡訊號；普通 ATM、收款碼、面交與國家名稱不直接判為垃圾。

## 任務

- [x] T1 漏判文案
  - Boundary: Allowed Changes 為 configs/rules、detection/rules corpus；Forbidden 為引擎及處置政策。
  - Depends: 無
  - Context: 七種截圖及正常反例。
  - Verify: `TestChineseModerationRegressionCorpus` 與 rules 回歸。
- [x] T2 移除錯誤客服回覆
  - Boundary: Allowed Changes 為 sample、matcher_test 及撤掉本次未提交的 autoreply 排除實作；Forbidden 為改動既有 matcher、loader、domain 行為。
  - Depends: 無
  - Context: 使用者確認沒有客服服務；排除招聘並非需求。
  - Verify: `TestMatcherOfficialLinksOnly`、autoreply 回歸。
- [x] T3 文件與驗證
  - Boundary: Allowed Changes 為 README、本 spec、上述檔案格式；Forbidden 為無關修改與部署。
  - Depends: T1、T2
  - Context: 自訂檔需移除 support 規則並重啟。
  - Verify: 全量 race、vet、差異 lint、git diff --stat、git diff --check。

## 品質檢查

- [x] 正負案例與 Protected Behavior 通過。
- [x] 測試、vet、lint 結果記錄。
- [x] 文件一致與差異檢查。

## Implementation Notes

- 使用者確認自動回覆只有官網與下載用途；先前客服排除方案理解錯誤，已撤掉尚未提交的排除功能及其專用測試，未影響垃圾規則補充。
- 尚未提交、推送或部署。
- 更正後全量 `go test -race -count=1 ./...` 與 `go vet ./...` 通過；`golangci-lint run --new-from-rev=HEAD ./...` 為 0 項。官網及下載詢問仍回覆，客服與招聘客服不回覆。
