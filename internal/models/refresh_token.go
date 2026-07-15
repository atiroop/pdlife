package models

import "time"

type RefreshToken struct {
	ID        uint64    `gorm:"column:id;primaryKey"`
	UserID    uint64    `gorm:"column:user_id;not null;index"`
	TokenHash string    `gorm:"column:token_hash;unique;not null"`
	ExpiresAt time.Time `gorm:"column:expires_at;not null"`
	// RevokedAt is set both when a token is rotated away and when a session
	// is deliberately ended (logout, password change/reset, account
	// deletion, admin suspension).
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	// RotatedAt is set only by the rotation path in
	// internal/handler/session.go, in the same UPDATE as RevokedAt. It is
	// what lets that path tell "just superseded by a sibling request" from
	// "killed on purpose" — the latter must never be honoured again, even
	// for a moment. Never set this anywhere a session is being ended.
	RotatedAt *time.Time `gorm:"column:rotated_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

func (RefreshToken) TableName() string {
	return "refresh_tokens"
}
