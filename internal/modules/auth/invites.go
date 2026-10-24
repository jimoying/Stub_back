package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

type InviteCreateRequest struct {
	AssignedRole string    `json:"assignedRole"`
	BoundEmail   string    `json:"boundEmail,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type InviteDTO struct {
	InviteID        string     `json:"inviteId"`
	CreatedByUserID string     `json:"createdByUserId"`
	AssignedRole    string     `json:"assignedRole"`
	BoundEmail      *string    `json:"boundEmail"`
	ExpiresAt       time.Time  `json:"expiresAt"`
	RedeemedAt      *time.Time `json:"redeemedAt"`
	RevokedAt       *time.Time `json:"revokedAt"`
	CreatedAt       time.Time  `json:"createdAt"`
	Status          string     `json:"status"`
}

type InviteList struct {
	Items      []InviteDTO `json:"items"`
	Pagination struct {
		Page     int   `json:"page"`
		PageSize int   `json:"pageSize"`
		Total    int64 `json:"total"`
	} `json:"pagination"`
}

type InviteCreateResponse struct {
	InviteID     string    `json:"inviteId"`
	Code         string    `json:"code"`
	InviteURL    string    `json:"inviteURL"`
	AssignedRole string    `json:"assignedRole"`
	BoundEmail   *string   `json:"boundEmail"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

func (s *Service) CreateInvite(ctx context.Context, actor User, input InviteCreateRequest, meta AuditMeta) (InviteCreateResponse, error) {
	// 1. 普通用户只能邀请普通用户；邀请码最长七天且管理员邀请码必须绑定邮箱。
	if input.AssignedRole == "" {
		input.AssignedRole = "user"
	}
	if input.AssignedRole != "user" && input.AssignedRole != "admin" {
		return InviteCreateResponse{}, problem(422, "invalid_invite_role", "邀请码角色无效")
	}
	if actor.Role != "admin" && input.AssignedRole != "user" {
		return InviteCreateResponse{}, problem(403, "invite_role_forbidden", "普通用户只能邀请普通用户")
	}
	var boundEmail *string
	if strings.TrimSpace(input.BoundEmail) != "" {
		normalized, err := normalizeEmail(input.BoundEmail)
		if err != nil {
			return InviteCreateResponse{}, problem(422, "invalid_bound_email", "绑定邮箱格式无效")
		}
		boundEmail = &normalized
	}
	if input.AssignedRole == "admin" && boundEmail == nil {
		return InviteCreateResponse{}, problem(422, "admin_invite_email_required", "管理员邀请码必须绑定邮箱")
	}
	now := time.Now().UTC()
	expiry := input.ExpiresAt.UTC()
	if !expiry.After(now) || expiry.After(now.Add(7*24*time.Hour)) {
		return InviteCreateResponse{}, problem(422, "invalid_expiration", "邀请码有效期必须在当前时间之后且不超过七天")
	}
	var allowInvite bool
	if err := s.db.QueryRowContext(ctx, `SELECT allow_invite_registration FROM system_settings WHERE settings_id=1`).Scan(&allowInvite); err != nil {
		return InviteCreateResponse{}, err
	}
	if !allowInvite {
		return InviteCreateResponse{}, problem(409, "invite_registration_disabled", "当前未开放邀请码注册")
	}
	code, err := randomToken(24)
	if err != nil {
		return InviteCreateResponse{}, err
	}
	id := uuid.New()
	// 2. 行锁串行化普通用户的邀请码数量检查并创建记录。
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return InviteCreateResponse{}, err
	}
	defer tx.Rollback()
	var role string
	if err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE user_id=$1 FOR UPDATE`, actor.ID).Scan(&role); err != nil {
		return InviteCreateResponse{}, err
	}
	if role != "admin" {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM invite_codes WHERE created_by_user_id=$1 AND redeemed_at IS NULL AND revoked_at IS NULL AND expires_at>now()`, actor.ID).Scan(&count); err != nil {
			return InviteCreateResponse{}, err
		}
		if count >= 10 {
			return InviteCreateResponse{}, problem(409, "invite_quota_exceeded", "最多同时持有十个有效未使用的邀请码")
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO invite_codes(invite_code_id,code_hash,created_by_user_id,assigned_role,bound_email,expires_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, s.secretDigest(s.cfg.ShortCodeHMACKey, code), actor.ID, input.AssignedRole, boundEmail, expiry, now); err != nil {
		return InviteCreateResponse{}, err
	}
	workspace := actor.ID
	if err := tx.QueryRowContext(ctx, `SELECT default_workspace_id FROM users WHERE user_id=$1`, actor.ID).Scan(&workspace); err != nil {
		return InviteCreateResponse{}, err
	}
	if err := writeAuditMeta(ctx, tx, &actor.ID, &workspace, "invite.create", "invite", id.String(), "success", meta, map[string]any{
		"assignedRole": input.AssignedRole,
		"boundEmail":   boundEmail,
		"expiresAt":    expiry,
	}); err != nil {
		return InviteCreateResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return InviteCreateResponse{}, err
	}
	inviteURL := s.cfg.PublicBaseURL + "/register?inviteCode=" + url.QueryEscape(code)
	return InviteCreateResponse{InviteID: publicID("inv", id), Code: code, InviteURL: inviteURL, AssignedRole: input.AssignedRole, BoundEmail: boundEmail, ExpiresAt: expiry}, nil
}

func (s *Service) ListInvites(ctx context.Context, actor User, status, creator string, page, pageSize int) (InviteList, error) {
	// 1. 普通用户查询范围固定为本人创建；管理员才可按创建者筛选全站数据。
	if page < 1 || pageSize < 1 || pageSize > 100 || (status != "" && status != "active" && status != "redeemed" && status != "expired" && status != "revoked") {
		return InviteList{}, problem(422, "invalid_filter", "邀请码筛选参数无效")
	}
	where := []string{}
	args := []any{}
	if actor.Role != "admin" {
		where = append(where, "i.created_by_user_id=$1")
		args = append(args, actor.ID)
	} else if creator != "" {
		id, err := parseID("usr", creator)
		if err != nil {
			return InviteList{}, problem(422, "invalid_user_id", "创建者参数无效")
		}
		where = append(where, fmt.Sprintf("i.created_by_user_id=$%d", len(args)+1))
		args = append(args, id)
	}
	statusExpr := "CASE WHEN i.redeemed_at IS NOT NULL THEN 'redeemed' WHEN i.revoked_at IS NOT NULL THEN 'revoked' WHEN i.expires_at<=now() THEN 'expired' ELSE 'active' END"
	if status != "" {
		where = append(where, fmt.Sprintf("%s=$%d", statusExpr, len(args)+1))
		args = append(args, status)
	}
	filter := "TRUE"
	if len(where) > 0 {
		filter = strings.Join(where, " AND ")
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM invite_codes i WHERE `+filter, args...).Scan(&total); err != nil {
		return InviteList{}, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, `SELECT i.invite_code_id,i.created_by_user_id,i.assigned_role,i.bound_email,i.expires_at,i.redeemed_at,i.revoked_at,i.created_at,`+statusExpr+` FROM invite_codes i WHERE `+filter+fmt.Sprintf(" ORDER BY i.created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return InviteList{}, err
	}
	defer rows.Close()
	items := make([]InviteDTO, 0)
	for rows.Next() {
		var id, creatorID uuid.UUID
		var item InviteDTO
		var email sql.NullString
		var redeemed, revoked sql.NullTime
		if err := rows.Scan(&id, &creatorID, &item.AssignedRole, &email, &item.ExpiresAt, &redeemed, &revoked, &item.CreatedAt, &item.Status); err != nil {
			return InviteList{}, err
		}
		item.InviteID = publicID("inv", id)
		item.CreatedByUserID = publicID("usr", creatorID)
		if email.Valid {
			value := email.String
			item.BoundEmail = &value
		}
		if redeemed.Valid {
			value := redeemed.Time.UTC()
			item.RedeemedAt = &value
		}
		if revoked.Valid {
			value := revoked.Time.UTC()
			item.RevokedAt = &value
		}
		item.ExpiresAt = item.ExpiresAt.UTC()
		item.CreatedAt = item.CreatedAt.UTC()
		items = append(items, item)
	}
	result := InviteList{Items: items}
	result.Pagination.Page = page
	result.Pagination.PageSize = pageSize
	result.Pagination.Total = total
	return result, rows.Err()
}

func (s *Service) RevokeInvite(ctx context.Context, actor User, rawID string, meta AuditMeta) (map[string]any, error) {
	// 1. 撤销只允许作用于未核销的邀请码；重复撤销保持幂等。
	id, err := parseID("inv", rawID)
	if err != nil {
		return nil, problem(404, "invite_not_found", "邀请码不存在或无权访问")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var creator uuid.UUID
	var redeemed, revoked sql.NullTime
	var workspace uuid.UUID
	err = tx.QueryRowContext(ctx, `SELECT i.created_by_user_id,i.redeemed_at,i.revoked_at,u.default_workspace_id FROM invite_codes i JOIN users u ON u.user_id=i.created_by_user_id WHERE i.invite_code_id=$1 FOR UPDATE OF i`, id).Scan(&creator, &redeemed, &revoked, &workspace)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && actor.Role != "admin" && creator != actor.ID) {
		return nil, problem(404, "invite_not_found", "邀请码不存在或无权访问")
	}
	if err != nil {
		return nil, err
	}
	if redeemed.Valid {
		return nil, problem(409, "invite_already_redeemed", "已使用的邀请码不能撤销")
	}
	now := time.Now().UTC()
	if !revoked.Valid {
		if _, err := tx.ExecContext(ctx, `UPDATE invite_codes SET revoked_at=$2 WHERE invite_code_id=$1`, id, now); err != nil {
			return nil, err
		}
		if err := writeAuditMeta(ctx, tx, &actor.ID, &workspace, "invite.revoke", "invite", id.String(), "success", meta, map[string]any{"status": "revoked"}); err != nil {
			return nil, err
		}
	} else {
		now = revoked.Time.UTC()
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"inviteId": publicID("inv", id), "status": "revoked", "revokedAt": now}, nil
}

func (s *Service) RevokeUnusedInvites(ctx context.Context, actorID uuid.UUID, userID *uuid.UUID, confirmAll bool, meta AuditMeta) (map[string]any, error) {
	// 1. 全站批量撤销必须由管理员明确 confirmAll，避免空参数误操作。
	if userID == nil && !confirmAll {
		return nil, problem(422, "confirm_all_required", "全站撤销必须显式确认")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var rows *sql.Rows
	if userID != nil {
		rows, err = tx.QueryContext(ctx, `UPDATE invite_codes SET revoked_at=now() WHERE created_by_user_id=$1 AND redeemed_at IS NULL AND revoked_at IS NULL RETURNING invite_code_id,created_by_user_id`, *userID)
	} else {
		rows, err = tx.QueryContext(ctx, `UPDATE invite_codes SET revoked_at=now() WHERE redeemed_at IS NULL AND revoked_at IS NULL RETURNING invite_code_id,created_by_user_id`)
	}
	if err != nil {
		return nil, err
	}
	type item struct{ id, creator uuid.UUID }
	items := make([]item, 0)
	for rows.Next() {
		var value item
		if err := rows.Scan(&value.id, &value.creator); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, value := range items {
		var workspace uuid.UUID
		if err := tx.QueryRowContext(ctx, `SELECT default_workspace_id FROM users WHERE user_id=$1`, value.creator).Scan(&workspace); err != nil {
			return nil, err
		}
		if err := writeAuditMeta(ctx, tx, &actorID, &workspace, "admin.invite.revoke_unused", "invite", value.id.String(), "success", meta, map[string]any{
			"createdByUserId": publicID("usr", value.creator),
			"status":          "revoked",
		}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"revokedCount": len(items), "revokedAt": time.Now().UTC()}, nil
}

// 1. 解析带资源前缀的公开 ID，并校验其 UUID 部分。
func parseID(prefix, value string) (uuid.UUID, error) {
	trimmed, ok := strings.CutPrefix(value, prefix+"_")
	if !ok {
		return uuid.Nil, fmt.Errorf("invalid %s id", prefix)
	}
	return uuid.Parse(trimmed)
}
