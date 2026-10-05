// Package migrations 將版本化 SQL 編入 runner 與 Bot 的同版執行檔。
package migrations

import "embed"

// Files 是經版本控制的 migration 來源；啟動驗證與 runner 使用同一份內容。
//
//go:embed *.up.sql *.down.sql schema_contract.json
var Files embed.FS
