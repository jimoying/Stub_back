package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Open 建立 PostgreSQL 连接并执行应用内版本化迁移。
func Open(ctx context.Context, databaseURL string) (*sql.DB, error) {
	// 1. 创建连接池并验证数据库可达。
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("创建 PostgreSQL 连接失败: %w", err)
	}
	db.SetMaxOpenConns(30)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接 PostgreSQL 失败: %w", err)
	}

	// 2. 使用 Goose 管理嵌入式 SQL 迁移，确保部署镜像无需额外挂载迁移文件。
	goose.SetBaseFS(migrationFS)
	if err := goose.SetDialect("postgres"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("初始化数据库迁移器失败: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("执行数据库迁移失败: %w", err)
	}
	return db, nil
}
