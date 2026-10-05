// Package main 提供部署前一次性資料庫 migration 命令。
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/vincent119/tg_spam_bot/internal/infra/migration"
)

func main() {
	os.Exit(execute(os.Args[1:]))
}

func execute(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "請指定 status、verify 或 up")
		return 2
	}
	command := args[0]
	if command != "status" && command != "verify" && command != "up" {
		_, _ = fmt.Fprintln(os.Stderr, "不支援的 migration 命令")
		return 2
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	adopt := flags.Bool("adopt-existing", false, "明示接管已驗證的既有 schema")
	semantic := flags.Bool("semantic", strings.EqualFold(os.Getenv("SEMANTIC_MEMORY_ENABLED"), "true"), "啟用語意 stream")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if *adopt && command != "up" {
		_, _ = fmt.Fprintln(os.Stderr, "--adopt-existing 只適用於 up")
		return 2
	}
	dsn, err := databaseURL()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 2
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "無法建立資料庫連線")
		return 1
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "資料庫連線失敗")
		return 1
	}
	runner, err := migration.New(db, *semantic)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}
	switch command {
	case "status":
		states, err := runner.Status(ctx)
		if err == nil {
			for _, state := range states {
				logState(state, 0)
			}
		}
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			return 1
		}
	case "verify":
		if err := runner.Verify(ctx); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			return 1
		}
		logSummary("verified", runner.Versions())
	case "up":
		if err := runner.Up(ctx, *adopt, logState); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			return 1
		}
		logSummary("completed", runner.Versions())
	}
	return 0
}

func databaseURL() (string, error) {
	if value := os.Getenv("DATABASE_URL"); value != "" {
		return value, nil
	}
	host := os.Getenv("DB_HOST")
	name := os.Getenv("DB_NAME")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	if host == "" || name == "" || user == "" || password == "" {
		return "", errors.New("請設定 DATABASE_URL 或完整 DB_HOST、DB_NAME、DB_USER、DB_PASSWORD")
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", errors.New("DB_PORT 必須介於 1 到 65535")
	}
	u := &url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, port), Path: "/" + name}
	u.User = url.UserPassword(user, password)
	query := u.Query()
	if strings.EqualFold(os.Getenv("DB_TLS_ENABLED"), "true") {
		mode := os.Getenv("DB_TLS_MODE")
		if mode == "" || mode == "disable" {
			return "", errors.New("啟用 DB_TLS_ENABLED 時須設定有效 DB_TLS_MODE")
		}
		query.Set("sslmode", mode)
		for key, environment := range map[string]string{
			"sslrootcert": "DB_TLS_CA_CERT", "sslcert": "DB_TLS_CLIENT_CERT", "sslkey": "DB_TLS_CLIENT_KEY",
		} {
			if value := os.Getenv(environment); value != "" {
				query.Set(key, value)
			}
		}
	} else {
		query.Set("sslmode", "disable")
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func logState(state migration.State, duration time.Duration) {
	record := map[string]any{
		"stream": state.File.Stream, "version": state.File.Version,
		"name": state.File.Name, "status": state.Status,
		"provenance":  state.Provenance,
		"checksum":    migration.ChecksumPrefix(state.File.Checksum),
		"duration_ms": duration.Milliseconds(), "error_code": state.ErrorCode,
		"finished_at": state.FinishedAt,
	}
	_ = json.NewEncoder(os.Stdout).Encode(record)
}

func logSummary(status, versions string) {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{
		"subsystem": "migration", "status": status, "versions": versions,
	})
}
