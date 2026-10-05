// Package migration 以版本化 SQL 管理 PostgreSQL 結構與啟動驗證。
package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"

	"github.com/vincent119/tg_spam_bot/migrations"
)

type columnContract struct {
	Type    string `json:"type"`
	NotNull bool   `json:"not_null"`
}

type tableContract struct {
	Columns map[string]columnContract `json:"columns"`
	Indexes map[string]string         `json:"indexes"`
}

// File 記錄不可變 migration 內容及其結構驗證契約。
type File struct {
	Version  int64
	Name     string
	Stream   string
	Checksum string
	SQL      string
	Tables   map[string]tableContract
}

var migrationName = regexp.MustCompile(`^(\d{14})_([a-z0-9_]+)\.up\.sql$`)

// Manifest 讀取編入執行檔的 SQL；未知檔名或缺少結構契約時直接失敗。
func Manifest() ([]File, error) {
	contractData, err := migrations.Files.ReadFile("schema_contract.json")
	if err != nil {
		return nil, fmt.Errorf("讀取 schema 契約: %w", err)
	}
	var contracts map[string]map[string]tableContract
	if err := json.Unmarshal(contractData, &contracts); err != nil {
		return nil, fmt.Errorf("解析 schema 契約: %w", err)
	}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		return nil, fmt.Errorf("讀取 migration 清單: %w", err)
	}
	files := make([]File, 0, len(contracts))
	seen := make(map[int64]bool)
	for _, entry := range entries {
		matches := migrationName.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		version, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil || seen[version] {
			return nil, fmt.Errorf("migration 版本無效或重複: %s", entry.Name())
		}
		seen[version] = true
		sqlBytes, err := migrations.Files.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("讀取 migration %s: %w", entry.Name(), err)
		}
		tables, ok := contracts[matches[1]]
		if !ok || len(tables) == 0 {
			return nil, fmt.Errorf("migration %s 缺少 schema 契約", entry.Name())
		}
		stream := "core"
		if version == 20261005110100 || version == 20261005111500 {
			stream = "semantic"
		}
		downName := matches[1] + "_" + matches[2] + ".down.sql"
		downBytes, err := migrations.Files.ReadFile(downName)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("讀取 down migration %s: %w", downName, err)
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("migration %s 缺少 down SQL", entry.Name())
		}
		contractBytes, err := json.Marshal(tables)
		if err != nil {
			return nil, fmt.Errorf("序列化 schema 契約: %w", err)
		}
		hash := sha256.New()
		_, _ = hash.Write([]byte(entry.Name()))
		_, _ = hash.Write(sqlBytes)
		_, _ = hash.Write(downBytes)
		_, _ = hash.Write(contractBytes)
		files = append(files, File{
			Version: version, Name: matches[2], Stream: stream,
			Checksum: hex.EncodeToString(hash.Sum(nil)), SQL: string(sqlBytes), Tables: tables,
		})
	}
	if len(files) != len(contracts) {
		return nil, fmt.Errorf("migration 檔案與 schema 契約數量不一致")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Version < files[j].Version })
	return files, nil
}
