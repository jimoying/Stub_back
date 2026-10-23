package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Problem struct {
	Status  int
	Code    string
	Message string
}

func (p *Problem) Error() string { return p.Message }

type User struct {
	ID                 uuid.UUID `json:"-"`
	UserID             string    `json:"userId"`
	Email              string    `json:"email"`
	Role               string    `json:"role"`
	EmailVerifiedAt    time.Time `json:"emailVerifiedAt"`
	InvitedByUserID    *string   `json:"invitedByUserId"`
	DefaultWorkspaceID string    `json:"defaultWorkspaceId"`
	CreatedAt          time.Time `json:"createdAt"`
}

type Session struct {
	SessionID        string     `json:"sessionId"`
	ClientAppID      string     `json:"clientAppId"`
	DeviceInstanceID *string    `json:"deviceInstanceId,omitempty"`
	DeviceName       *string    `json:"deviceName,omitempty"`
	Platform         *string    `json:"platform,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	LastSeenAt       time.Time  `json:"lastSeenAt"`
	ExpiresAt        time.Time  `json:"expiresAt"`
	RevokedAt        *time.Time `json:"revokedAt"`
	IsCurrent        bool       `json:"isCurrent"`
	Online           bool       `json:"online"`
}

type EmailCodeRequest struct {
	Email      string `json:"email"`
	Purpose    string `json:"purpose"`
	InviteCode string `json:"inviteCode,omitempty"`
}

type EmailCodeResponse struct {
	Accepted          bool      `json:"accepted"`
	ExpiresInSeconds  int       `json:"expiresInSeconds"`
	ResendAvailableAt time.Time `json:"resendAvailableAt"`
}

type RegisterRequest struct {
	Email            string `json:"email"`
	Password         string `json:"password"`
	VerificationCode string `json:"verificationCode"`
	InviteCode       string `json:"inviteCode,omitempty"`
}

type LoginRequest struct {
	Email            string  `json:"email"`
	Password         string  `json:"password"`
	DeviceInstanceID *string `json:"deviceInstanceId,omitempty"`
	DeviceName       *string `json:"deviceName,omitempty"`
	Platform         *string `json:"platform,omitempty"`
}

type LoginResponse struct {
	AccessToken string    `json:"accessToken"`
	TokenType   string    `json:"tokenType"`
	ExpiresAt   time.Time `json:"expiresAt"`
	User        User      `json:"user"`
	Session     Session   `json:"session"`
}

type PasswordResetRequest struct {
	Email            string `json:"email"`
	VerificationCode string `json:"verificationCode"`
	NewPassword      string `json:"newPassword"`
}

type EmailSender interface {
	SendVerificationCode(ctx context.Context, email, purpose, code string) error
}
