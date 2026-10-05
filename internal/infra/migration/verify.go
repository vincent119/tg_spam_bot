package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// verifyFile 逐項比對預期欄位與索引，額外的未來欄位不影響舊版本接管。
func verifyFile(ctx context.Context, db rowQuerier, file File) error {
	for table, spec := range file.Tables {
		var exists bool
		err := db.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname=$1 AND c.relkind IN ('r','p'))",
			table,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("檢查資料表 %s: %w", table, err)
		}
		if !exists {
			return fmt.Errorf("schema 不相容: 缺少資料表 %s", table)
		}
		if _, hasPrimaryKey := spec.Indexes[table+"_pkey"]; hasPrimaryKey {
			var hasPK bool
			err := db.QueryRowContext(ctx,
				"SELECT EXISTS (SELECT 1 FROM pg_constraint p JOIN pg_class c ON c.oid=p.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname=$1 AND p.contype='p')",
				table,
			).Scan(&hasPK)
			if err != nil {
				return fmt.Errorf("檢查主鍵 %s: %w", table, err)
			}
			if !hasPK {
				return fmt.Errorf("schema 不相容: %s 缺少主鍵約束", table)
			}
		}
		if id, hasID := spec.Columns["id"]; hasID && id.Type == "bigint" {
			var hasSequence bool
			err := db.QueryRowContext(ctx, "SELECT pg_get_serial_sequence($1,'id') IS NOT NULL", "public."+table).Scan(&hasSequence)
			if err != nil {
				return fmt.Errorf("檢查序列 %s.id: %w", table, err)
			}
			if !hasSequence {
				return fmt.Errorf("schema 不相容: %s.id 缺少自動遞增序列", table)
			}
		}
		for column, expected := range spec.Columns {
			var actualType string
			var actualNotNull bool
			err := db.QueryRowContext(ctx,
				"SELECT format_type(a.atttypid,a.atttypmod),a.attnotnull FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname=$1 AND a.attname=$2 AND a.attnum>0 AND NOT a.attisdropped",
				table, column,
			).Scan(&actualType, &actualNotNull)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("schema 不相容: %s.%s 欄位不存在", table, column)
			}
			if err != nil {
				return fmt.Errorf("檢查欄位 %s.%s: %w", table, column, err)
			}
			if actualType != expected.Type || actualNotNull != expected.NotNull {
				return fmt.Errorf("schema 不相容: %s.%s 型別或空值約束不符", table, column)
			}
		}
		for index, expected := range spec.Indexes {
			var actual string
			err := db.QueryRowContext(ctx,
				"SELECT pg_get_indexdef(i.indexrelid) FROM pg_index i JOIN pg_class c ON c.oid=i.indrelid JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_class ic ON ic.oid=i.indexrelid WHERE n.nspname='public' AND c.relname=$1 AND ic.relname=$2 AND i.indisvalid",
				table, index,
			).Scan(&actual)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("schema 不相容: %s 缺少有效索引 %s", table, index)
			}
			if err != nil {
				return fmt.Errorf("檢查索引 %s: %w", index, err)
			}
			if normalizeIndex(actual) != normalizeIndex(expected) {
				return fmt.Errorf("schema 不相容: %s 索引定義不符", index)
			}
		}
	}
	return nil
}

func normalizeIndex(definition string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(definition, "public.", "")), " ")
}
