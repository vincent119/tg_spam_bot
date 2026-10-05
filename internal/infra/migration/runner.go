package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
)

const lockKey int64 = 741909125

// State 是一個 migration 在資料庫中的可稽核狀態。
type State struct {
	File       File
	Status     string
	Provenance string
	FinishedAt *time.Time
	ErrorCode  string
}

// Runner 只在部署前寫入 DDL；常駐 Bot 僅呼叫 Verify。
type Runner struct {
	db       *sql.DB
	files    []File
	semantic bool
}

// New 建立同一發行版的 runner，semantic 決定啟動所需的 migration stream。
func New(db *sql.DB, semantic bool) (*Runner, error) {
	if db == nil {
		return nil, errors.New("資料庫連線不可為空")
	}
	files, err := Manifest()
	if err != nil {
		return nil, err
	}
	return &Runner{db: db, files: files, semantic: semantic}, nil
}

func (r *Runner) activeFiles() []File {
	active := make([]File, 0, len(r.files))
	for _, file := range r.files {
		if file.Stream == "core" {
			active = append(active, file)
		}
	}
	if r.semantic {
		for _, file := range r.files {
			if file.Stream == "semantic" {
				active = append(active, file)
			}
		}
	}
	return active
}

// Status 不建立表；空庫回報 pending，已套用版本回報來源與漂移。
func (r *Runner) Status(ctx context.Context) ([]State, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx, "SELECT to_regclass('migration.schema_migration_runs') IS NOT NULL").Scan(&exists); err != nil {
		return nil, fmt.Errorf("查詢 migration 狀態表: %w", err)
	}
	if exists {
		rows, err := r.db.QueryContext(ctx, "SELECT stream,version FROM migration.schema_migration_runs")
		if err != nil {
			return nil, fmt.Errorf("查詢 migration 版本清單: %w", err)
		}
		known := make(map[string]bool, len(r.files))
		for _, file := range r.files {
			known[fmt.Sprintf("%s/%d", file.Stream, file.Version)] = true
		}
		for rows.Next() {
			var stream string
			var version int64
			if err := rows.Scan(&stream, &version); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("讀取 migration 版本清單: %w", err)
			}
			if !known[fmt.Sprintf("%s/%d", stream, version)] {
				_ = rows.Close()
				return nil, fmt.Errorf("資料庫有本發行版未知的 migration %s/%d", stream, version)
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("讀取 migration 版本清單: %w", err)
		}
		_ = rows.Close()
	}
	states := make([]State, 0, len(r.files))
	for _, file := range r.files {
		state := State{File: file, Status: "pending"}
		if exists {
			var checksum string
			err := r.db.QueryRowContext(ctx,
				"SELECT checksum,status,provenance,finished_at,error_code FROM migration.schema_migration_runs WHERE stream=$1 AND version=$2",
				file.Stream, file.Version,
			).Scan(&checksum, &state.Status, &state.Provenance, &state.FinishedAt, &state.ErrorCode)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("查詢 migration %d: %w", file.Version, err)
			}
			if errors.Is(err, sql.ErrNoRows) {
				state.Status = "pending"
			} else if checksum != file.Checksum {
				state.Status = "checksum_mismatch"
			}
		}
		states = append(states, state)
	}
	return states, nil
}

// Verify 是 Bot 啟動門檻，從不建立或修正資料庫結構。
func (r *Runner) Verify(ctx context.Context) error {
	states, err := r.Status(ctx)
	if err != nil {
		return err
	}
	for _, state := range states {
		if state.File.Stream == "semantic" && !r.semantic {
			continue
		}
		if state.Status != "applied" && state.Status != "adopted" {
			return fmt.Errorf("migration %s/%d 狀態 %s，不允許啟動", state.File.Stream, state.File.Version, state.Status)
		}
		if err := verifyFile(ctx, r.db, state.File); err != nil {
			return fmt.Errorf("migration %s/%d 驗證失敗: %w", state.File.Stream, state.File.Version, err)
		}
		if err := r.verifyGooseRecord(ctx, state.File); err != nil {
			return err
		}
	}
	return nil
}

// Up 只向前執行。adoptExisting 須由部署者明示，且每版都先檢查完整 schema 契約。
func (r *Runner) Up(ctx context.Context, adoptExisting bool, logf func(State, time.Duration)) (runErr error) {
	conn, err := r.acquireLock(ctx, 30*time.Second)
	if err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, unlockErr := conn.ExecContext(cleanupCtx, "SELECT pg_advisory_unlock($1)", lockKey)
		closeErr := conn.Close()
		runErr = errors.Join(runErr, unlockErr, closeErr)
	}()
	if err := r.ensureMetadata(ctx); err != nil {
		return err
	}
	states, err := r.Status(ctx)
	if err != nil {
		return err
	}
	for _, state := range states {
		if state.File.Stream == "semantic" && !r.semantic {
			continue
		}
		if state.Status == "checksum_mismatch" || state.Status == "running" || state.Status == "failed" {
			return fmt.Errorf("migration %s/%d 狀態 %s，須先人工核對修復", state.File.Stream, state.File.Version, state.Status)
		}
	}
	for _, file := range r.activeFiles() {
		if file.Stream == "semantic" {
			if err := r.verifyVectorExtension(ctx); err != nil {
				return err
			}
		}
		state := findState(states, file.Version)
		if state.Status == "applied" || state.Status == "adopted" {
			if err := verifyFile(ctx, r.db, file); err != nil {
				return fmt.Errorf("已完成 migration %d 發生 schema 漂移: %w", file.Version, err)
			}
			if err := r.verifyGooseRecord(ctx, file); err != nil {
				return err
			}
			continue
		}
		provenance := "applied"
		if adoptExisting && verifyFile(ctx, r.db, file) == nil {
			provenance = "adopted"
		}
		start := time.Now()
		if _, err := r.db.ExecContext(ctx,
			"INSERT INTO migration.schema_migration_runs(stream,version,name,checksum,status,provenance,started_at) VALUES($1,$2,$3,$4,'running',$5,now())",
			file.Stream, file.Version, file.Name, file.Checksum, provenance,
		); err != nil {
			return fmt.Errorf("記錄 migration %d 開始狀態: %w", file.Version, err)
		}
		if logf != nil {
			logf(State{File: file, Status: "running", Provenance: provenance}, 0)
		}
		err := r.applyVersion(ctx, file, provenance)
		if err != nil {
			recordCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, recordErr := r.db.ExecContext(recordCtx,
				"UPDATE migration.schema_migration_runs SET status='failed',error_code='migration_failed',finished_at=now() WHERE stream=$1 AND version=$2 AND status='running'",
				file.Stream, file.Version,
			)
			cancel()
			if recordErr != nil {
				if logf != nil {
					logf(State{File: file, Status: "running", Provenance: provenance, ErrorCode: "failure_record_failed"}, time.Since(start))
				}
				return errors.Join(fmt.Errorf("套用 migration %s/%d 失敗: %w", file.Stream, file.Version, err), fmt.Errorf("寫入失敗狀態: %w", recordErr))
			}
			if logf != nil {
				logf(State{File: file, Status: "failed", Provenance: provenance, ErrorCode: "migration_failed"}, time.Since(start))
			}
			return fmt.Errorf("套用 migration %s/%d 失敗: %w", file.Stream, file.Version, err)
		}
		if logf != nil {
			logf(State{File: file, Status: provenance, Provenance: provenance}, time.Since(start))
		}
	}
	return r.Verify(ctx)
}

func (r *Runner) verifyVectorExtension(ctx context.Context) error {
	var exists bool
	if err := r.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname='vector')").Scan(&exists); err != nil {
		return fmt.Errorf("檢查 pgvector extension: %w", err)
	}
	if !exists {
		return errors.New("語意 migration 需要預先安裝 pgvector extension")
	}
	return nil
}

func findState(states []State, version int64) State {
	for _, state := range states {
		if state.File.Version == version {
			return state
		}
	}
	return State{Status: "pending"}
}

func (r *Runner) ensureMetadata(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS migration"); err != nil {
		return fmt.Errorf("建立 migration schema: %w", err)
	}
	const statement = `CREATE TABLE IF NOT EXISTS migration.schema_migration_runs (
		stream text NOT NULL,
		version bigint NOT NULL,
		name text NOT NULL,
		checksum char(64) NOT NULL,
		status text NOT NULL CHECK (status IN ('running','failed','applied','adopted')),
		provenance text NOT NULL CHECK (provenance IN ('applied','adopted')),
		started_at timestamptz NOT NULL,
		finished_at timestamptz,
		error_code text NOT NULL DEFAULT '',
		PRIMARY KEY (stream,version)
	)`
	if _, err := r.db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("建立 migration 狀態表: %w", err)
	}
	return nil
}

func (r *Runner) acquireLock(ctx context.Context, timeout time.Duration) (*sql.Conn, error) {
	lockCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := r.db.Conn(lockCtx)
	if err != nil {
		return nil, fmt.Errorf("取得 migration 鎖連線: %w", err)
	}
	for {
		var locked bool
		if err := conn.QueryRowContext(lockCtx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&locked); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("取得 migration 資料庫鎖: %w", err)
		}
		if locked {
			return conn, nil
		}
		select {
		case <-lockCtx.Done():
			_ = conn.Close()
			return nil, fmt.Errorf("等待 migration 資料庫鎖逾時: %w", lockCtx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func gooseTable(stream string) string {
	if stream == "semantic" {
		return "migration.semantic_goose_versions"
	}
	return "migration.core_goose_versions"
}

func (r *Runner) verifyGooseRecord(ctx context.Context, file File) error {
	query := "SELECT EXISTS (SELECT 1 FROM " + gooseTable(file.Stream) + " WHERE version_id=$1 AND is_applied)"
	var exists bool
	if err := r.db.QueryRowContext(ctx, query, file.Version).Scan(&exists); err != nil {
		return fmt.Errorf("查詢 goose 版本 %d: %w", file.Version, err)
	}
	if !exists {
		return fmt.Errorf("goose 版本 %s/%d 缺少已套用紀錄", file.Stream, file.Version)
	}
	return nil
}

func (r *Runner) applyVersion(ctx context.Context, file File, provenance string) error {
	up := &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		if provenance == "applied" {
			if _, err := tx.ExecContext(ctx, file.SQL); err != nil {
				return fmt.Errorf("執行版本 SQL: %w", err)
			}
		}
		if err := verifyFile(ctx, tx, file); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx,
			"UPDATE migration.schema_migration_runs SET status=$1,finished_at=now(),error_code='' WHERE stream=$2 AND version=$3 AND status='running'",
			provenance, file.Stream, file.Version,
		)
		if err != nil {
			return err
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if updated != 1 {
			return fmt.Errorf("migration %d 的 running 狀態已變更", file.Version)
		}
		return nil
	}}
	provider, err := goose.NewProvider(goose.DialectPostgres, r.db, nil,
		goose.WithTableName(gooseTable(file.Stream)),
		goose.WithDisableGlobalRegistry(true),
		goose.WithGoMigrations(goose.NewGoMigration(file.Version, up, nil)),
	)
	if err != nil {
		return fmt.Errorf("建立 goose provider: %w", err)
	}
	_, err = provider.ApplyVersion(ctx, file.Version, true)
	if err != nil {
		return err
	}
	return nil
}

// Versions 輸出符合設定的預期最高版本，供啟動日誌記錄。
func (r *Runner) Versions() string {
	versions := make(map[string]int64)
	for _, file := range r.activeFiles() {
		versions[file.Stream] = file.Version
	}
	return fmt.Sprintf("core=%d semantic=%d", versions["core"], versions["semantic"])
}

// ChecksumPrefix 只輸出足以辨識版本的摘要，不記錄整份 SQL。
func ChecksumPrefix(checksum string) string {
	return strings.Clone(checksum[:min(12, len(checksum))])
}
