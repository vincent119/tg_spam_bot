# 設計：漏判文案與錯誤客服回覆

## 文件定位與已知契約

需求來自 requirements。偵測以詞組與既有訊號評分；引用只參與詞彙。Webhook 先偵測垃圾，再執行自動回覆。自動回覆規則來自部署指定的獨立 YAML 檔。

## Bounded Context 與原則

包含具體垃圾詞組、範例自動回覆與測試文件。不包含 AI、DB、客服服務或通用排除詞功能。以範例實際載入的回覆範圍驗證使用者意圖。

## 目標流程與關鍵行為

1. 補錢款、成人招攬及新增海外客服招聘、交易廣告分類；所有新增分類為 progressive。
2. 移除 auto_replies.sample.yaml 的 support 規則；保留 download_page 與 official_page。
3. 原 autoreply loader、domain 與 matcher 維持既有實作；撤掉本次尚未提交的 exclude_keywords 修改。
4. 範例回歸測試同時覆蓋客服不回覆與官網下載正常回覆。

## 受影響檔案計畫

- configs/rules 的 money_laundering_recruitment、adult_service_promo、新 recruitment 及 trade 檔案。
- detection/rules 的中文 corpus 與測試。
- configs/auto_replies.sample.yaml、autoreply/application/matcher_test.go。
- README 與本 spec。

## 風險與處理方式

Substring 無法理解所有引用與否定語境，以具體詞組及正常反例限制誤判。無法由截圖確認 prod 快照及白名單；本地只驗證文案。實際掛載檔須移除 support 區塊，範例更新不會改寫自訂檔。
