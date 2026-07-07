package models

import "time"

type RefreshToken struct {
	ID        uint64     `gorm:"column:id;primaryKey"`
	UserID    uint64     `gorm:"column:user_id;not null;index"`
	TokenHash string     `gorm:"column:token_hash;unique;not null"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

func (RefreshToken) TableName() string {
	return "refresh_tokens"
}
