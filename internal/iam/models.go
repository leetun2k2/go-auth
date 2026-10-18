package iam

import (
	"time"

	"github.com/google/uuid"
)

type Pagination struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

type ResponseEnvelope[T any] struct {
	Data       T           `json:"data"`
	Pagination *Pagination `json:"pagination,omitempty"`
	RequestID  string      `json:"request_id,omitempty"`
}

type ErrorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type ErrorEnvelope struct {
	Error     ErrorDetail `json:"error"`
	RequestID string      `json:"request_id,omitempty"`
}

type UserAccess struct {
	UserID      uuid.UUID `json:"user_id"`
	Email       *string   `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	Roles       []string  `json:"roles"`
	IsAdmin     bool      `json:"is_admin"`
}

type UserSummary struct {
	ID          uuid.UUID `json:"id"`
	Email       *string   `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	Roles       []string  `json:"roles"`
}

type UserDetail struct {
	UserSummary
	Identities []IdentitySummary `json:"identities"`
}

type UpdateUserRolesRequest struct {
	RoleIDs []uuid.UUID `json:"role_ids" binding:"required"`
}

type IdentitySummary struct {
	ID              uuid.UUID `json:"id"`
	UserID          uuid.UUID `json:"user_id"`
	UserEmail       *string   `json:"user_email,omitempty"`
	Provider        string    `json:"provider"`
	ProviderSubject string    `json:"provider_subject"`
	Email           *string   `json:"email"`
	EmailVerified   bool      `json:"email_verified"`
	DisplayName     string    `json:"display_name"`
	AvatarURL       string    `json:"avatar_url"`
	CreatedAt       time.Time `json:"created_at"`
}

type RoleItem struct {
	ID        uuid.UUID `json:"id"`
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	UserCount int       `json:"user_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateRoleRequest struct {
	Key  string `json:"key" binding:"required"`
	Name string `json:"name" binding:"required"`
}

type UpdateRoleRequest struct {
	Name string `json:"name" binding:"required"`
}

type SessionItem struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	UserEmail  *string    `json:"user_email"`
	UserName   string     `json:"user_name"`
	ClientID   uuid.UUID  `json:"client_id"`
	ClientName string     `json:"client_name"`
	Provider   string     `json:"provider"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type ClientRegistrationItem struct {
	ID           uuid.UUID `json:"id"`
	Provider     string    `json:"provider"`
	ClientID     string    `json:"client_id"`
	Name         string    `json:"name"`
	RedirectURL  string    `json:"redirect_url"`
	RedirectURIs []string  `json:"redirect_uris"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type UpdateClientEnabledRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}
