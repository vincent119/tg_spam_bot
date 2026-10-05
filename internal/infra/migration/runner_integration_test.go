package migration

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	pgstore "github.com/vincent119/tg_spam_bot/internal/detection/infra/postgres"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func testDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("未設定 TEST_DATABASE_URL；專用 CI 必須提供隔離 PostgreSQL")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "tg_migration_it_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if _, err := admin.ExecContext(t.Context(), "CREATE DATABASE \""+name+"\""); err != nil {
		_ = admin.Close()
		t.Fatalf("建立隔離測試資料庫: %v", err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE \""+name+"\" WITH (FORCE)")
		_ = admin.Close()
	})
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMigrationBaselineSchema(t *testing.T) {
	db := testDatabase(t)
	files, err := Manifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files[:2] {
		if file.Stream == "semantic" {
			if _, err := db.ExecContext(t.Context(), "CREATE EXTENSION vector"); err != nil {
				t.Fatalf("專用 CI 必須提供 pgvector: %v", err)
			}
		}
		if _, err := db.ExecContext(t.Context(), file.SQL); err != nil {
			t.Fatalf("執行基線 %d: %v", file.Version, err)
		}
		if err := verifyFile(t.Context(), db, file); err != nil {
			t.Fatalf("基線 %d 結構不符: %v", file.Version, err)
		}
	}
	var laterTableExists bool
	if err := db.QueryRowContext(t.Context(), "SELECT to_regclass('public.manual_feedback_currents') IS NOT NULL").Scan(&laterTableExists); err != nil {
		t.Fatal(err)
	}
	if laterTableExists {
		t.Fatal("11400 前基線不得包含後續人工回饋資料表")
	}
}

func TestMigrationRunnerFreshDatabase(t *testing.T) {
	db := testDatabase(t)
	runner, err := New(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Verify(t.Context()); err == nil {
		t.Fatal("空庫不得通過啟動門檻")
	}
	var transitions []string
	logf := func(state State, _ time.Duration) { transitions = append(transitions, state.Status) }
	if err := runner.Up(t.Context(), false, logf); err != nil {
		t.Fatal(err)
	}
	if err := runner.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 10 {
		t.Fatalf("預期五組核心 migration 各有開始及完成日誌，實際 %d", len(transitions))
	}
	states, err := runner.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.File.Stream == "semantic" && state.Status != "pending" {
			t.Fatalf("語意關閉仍標記 %d 為 %s", state.File.Version, state.Status)
		}
	}
	transitions = nil
	if err := runner.Up(t.Context(), false, logf); err != nil || len(transitions) != 0 {
		t.Fatalf("重跑應為零 DDL，err=%v transitions=%v", err, transitions)
	}
}

func TestMigrationRunnerLogsAndStartupGate(t *testing.T) {
	db := testDatabase(t)
	runner, err := New(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Verify(t.Context()); err == nil {
		t.Fatal("缺版時直接啟動 Bot 必須失敗")
	}
	var started, completed int
	logf := func(state State, elapsed time.Duration) {
		if state.Status == "running" && elapsed == 0 {
			started++
		}
		if state.Status == "applied" && elapsed >= 0 && len(state.File.Checksum) == 64 {
			completed++
		}
	}
	if err := runner.Up(t.Context(), false, logf); err != nil {
		t.Fatal(err)
	}
	if started != 5 || completed != 5 {
		t.Fatalf("逐版開始與完成日誌不完整: started=%d completed=%d", started, completed)
	}
	if err := runner.Verify(t.Context()); err != nil {
		t.Fatalf("runner 成功後啟動門檻應通過: %v", err)
	}
}

func TestMigrationRunnerAdoptAndRerun(t *testing.T) {
	db := testDatabase(t)
	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, migrate := range []func(context.Context, *gorm.DB) error{
		pgstore.AutoMigrate, pgstore.AutoMigrateRepeatActions,
		pgstore.AutoMigrateManualFeedback, pgstore.AutoMigrateManualFeedbackActions,
	} {
		if err := migrate(t.Context(), gormDB); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(t.Context(), "INSERT INTO processed_updates(update_id,status,claimed_at) VALUES(42,'completed',now())"); err != nil {
		t.Fatal(err)
	}
	runner, err := New(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(t.Context(), true, nil); err != nil {
		t.Fatal(err)
	}
	states, err := runner.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.File.Stream == "semantic" {
			continue
		}
		want := "adopted"
		if state.File.Version == 20261005111800 {
			want = "applied"
		}
		if state.Status != want || state.Provenance != want {
			t.Fatalf("版本 %d 狀態=%s 來源=%s，預期 %s", state.File.Version, state.Status, state.Provenance, want)
		}
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM processed_updates WHERE update_id=42").Scan(&count); err != nil || count != 1 {
		t.Fatalf("接管不得移除既有資料，count=%d err=%v", count, err)
	}
	if err := runner.Up(t.Context(), true, nil); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationRunnerSemanticOptional(t *testing.T) {
	db := testDatabase(t)
	core, err := New(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.Up(t.Context(), false, nil); err != nil {
		t.Fatal(err)
	}
	semantic, err := New(db, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := semantic.Up(t.Context(), false, nil); err == nil {
		t.Fatal("未安裝 pgvector 時語意 stream 應失敗")
	}
	if err := core.Verify(t.Context()); err != nil {
		t.Fatalf("語意失敗不得阻擋核心: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "CREATE EXTENSION vector"); err != nil {
		t.Fatalf("專用 CI 必須提供 pgvector: %v", err)
	}
	if err := semantic.Up(t.Context(), false, nil); err != nil {
		t.Fatal(err)
	}
	if err := semantic.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationRunnerLockDirtyAndChecksum(t *testing.T) {
	db := testDatabase(t)
	runner, err := New(db, false)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(t.Context(), "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		t.Fatal(err)
	}
	lockCtx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	if err := runner.Up(lockCtx, false, nil); err == nil {
		t.Fatal("並行 runner 應等待鎖並逾時，且不得開始 DDL")
	}
	_, _ = conn.ExecContext(t.Context(), "SELECT pg_advisory_unlock($1)", lockKey)
	_ = conn.Close()
	if err := runner.Up(t.Context(), false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE migration.schema_migration_runs SET checksum=repeat('0',64) WHERE version=20261005111400"); err != nil {
		t.Fatal(err)
	}
	if err := runner.Verify(t.Context()); err == nil {
		t.Fatal("checksum 漂移不得通過啟動門檻")
	}
	if err := runner.Up(t.Context(), false, nil); err == nil {
		t.Fatal("checksum 漂移不得繼續升級")
	}
	files, _ := Manifest()
	for _, file := range files {
		if file.Version == 20261005111400 {
			_, _ = db.ExecContext(t.Context(), "UPDATE migration.schema_migration_runs SET checksum=$1,status='running' WHERE version=$2", file.Checksum, file.Version)
		}
	}
	if err := runner.Verify(t.Context()); err == nil {
		t.Fatal("dirty 狀態不得通過啟動門檻")
	}
	if err := runner.Up(t.Context(), false, nil); err == nil {
		t.Fatal("dirty 狀態不得自動重試")
	}
}

func TestMigrationRunnerFailedDDLLeavesDirty(t *testing.T) {
	db := testDatabase(t)
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE processed_updates (update_id text)"); err != nil {
		t.Fatal(err)
	}
	runner, err := New(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Up(t.Context(), false, nil); err == nil {
		t.Fatal("同名異構資料表不得被 IF NOT EXISTS 略過")
	}
	states, err := runner.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if states[0].Status != "failed" {
		t.Fatalf("失敗後應保留 dirty，實際 %s", states[0].Status)
	}
	if err := runner.Up(t.Context(), false, nil); err == nil {
		t.Fatal("dirty 狀態不得自動重跑")
	}
}
