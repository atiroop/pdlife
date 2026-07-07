package models

import "time"

type PasswordResetToken struct {
	ID        uint64     `gorm:"column:id;primaryKey"`
	UserID    uint64     `gorm:"column:user_id;not null;index"`
	TokenHash string     `gorm:"column:token_hash;unique;not null"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null"`
	UsedAt    *time.Time `gorm:"column:used_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

func (PasswordResetToken) TableName() string {
	return "password_reset_tokens"
}
