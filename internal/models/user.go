package models

import (
	"time"

	"gorm.io/gorm"
)

type UserRole string

const (
	RoleAdmin      UserRole = "Admin"
	RoleMember     UserRole = "Member"
	RoleUnverified UserRole = "Unverified"
)

type User struct {
	ID           uint64 `gorm:"column:id;primaryKey"`
	Email        string `gorm:"column:email;unique;not null"`
	PasswordHash string `gorm:"column:password_hash;not null"`
	// SecurityStamp is embedded into every issued JWT and re-checked on
	// every request; changing it (e.g. on password reset) invalidates
	// all previously issued access tokens at once.
	SecurityStamp   string         `gorm:"column:security_stamp;not null;default:''"`
	Nickname        string         `gorm:"column:nickname;not null"`
	Role            UserRole       `gorm:"column:role;type:enum('Admin','Member','Unverified');not null;default:Unverified"`
	IsActive        bool           `gorm:"column:is_active;not null;default:1"`
	EmailVerifiedAt *time.Time     `gorm:"column:email_verified_at"`
	LastLoginAt     *time.Time     `gorm:"column:last_login_at"`
	CreatedAt       time.Time      `gorm:"column:created_at"`
	UpdatedAt       time.Time      `gorm:"column:updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (User) TableName() string {
	return "users"
}
