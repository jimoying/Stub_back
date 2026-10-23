package app

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yourGaomu/short_links/back/internal/config"
	"github.com/yourGaomu/short_links/back/internal/modules/auth"
)

func ensureInitialAdmin(ctx context.Context, db *sql.DB, cfg config.Config) error {
	// 1. 串行化首次初始化，避免多实例同时创建多个管理员。
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("启动管理员初始化事务失败: %w", err)
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `SELECT pg_advisory_xact_lock(8347192201)`).Scan(new(any)); err != nil {
		return fmt.Errorf("获取管理员初始化锁失败: %w", err)
	}

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE role = 'admin')`).Scan(&exists); err != nil {
		return fmt.Errorf("查询管理员账号失败: %w", err)
	}
	if exists {
		return tx.Commit()
	}

	// 2. 首位管理员只从部署环境引导；未配置时拒绝启动，不开放匿名提权入口。
	email := strings.ToLower(strings.TrimSpace(cfg.InitialAdminEmail))
	if email == "" || cfg.InitialAdminPassword == "" {
		return fmt.Errorf("数据库尚无管理员，请配置 INITIAL_ADMIN_EMAIL 和 INITIAL_ADMIN_PASSWORD 后重新启动")
	}
	if len(email) > 320 || !strings.Contains(email, "@") {
		return fmt.Errorf("INITIAL_ADMIN_EMAIL 不是有效邮箱")
	}
	passwordHash, err := auth.HashPassword(cfg.InitialAdminPassword)
	if err != nil {
		return err
	}

	// 3. 在同一事务创建管理员、默认工作空间和初始关闭状态的注册设置。
	userID := uuid.New()
	workspaceID := uuid.New()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users (user_id, email, password_hash, role, email_verified_at, default_workspace_id)
		VALUES ($1, $2, $3, 'admin', $4, $5)`, userID, email, passwordHash, time.Now().UTC(), workspaceID); err != nil {
		return fmt.Errorf("创建首位管理员失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO workspaces (workspace_id, owner_user_id, name) VALUES ($1, $2, '默认空间')`, workspaceID, userID); err != nil {
		return fmt.Errorf("创建管理员默认工作空间失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE system_settings SET allow_public_registration = false, allow_invite_registration = false,
		updated_at = now(), updated_by = $1 WHERE settings_id = 1`, userID); err != nil {
		return fmt.Errorf("初始化系统注册设置失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交管理员初始化失败: %w", err)
	}
	return nil
}
