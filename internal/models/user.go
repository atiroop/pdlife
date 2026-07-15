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
	SecurityStamp   string     `gorm:"column:security_stamp;not null;default:''"`
	Nickname        string     `gorm:"column:nickname;not null"`
	Role            UserRole   `gorm:"column:role;type:enum('Admin','Member','Unverified');not null;default:Unverified"`
	IsActive        bool       `gorm:"column:is_active;not null;default:1"`
	EmailVerifiedAt *time.Time `gorm:"column:email_verified_at"`
	LastLoginAt     *time.Time `gorm:"column:last_login_at"`
	// AccountDeletionRequestedAt is set once when the user confirms
	// account deletion from /profile (password + typed confirmation
	// required — see internal/handler/profile.go). Non-nil blocks login
	// (see internal/handler/login.go) and marks the account for
	// cmd/purge_deleted_accounts to hard-delete/anonymize 90 days later.
	AccountDeletionRequestedAt *time.Time `gorm:"column:account_deletion_requested_at"`
	// TermsAcceptedAt/Version are stamped once at registration, via the
	// scroll-to-accept modal on /register (see internal/handler/auth.go's
	// Register handler). Version is handler.LegalContentUpdatedDate at
	// acceptance time — same rationale as
	// PatientProfile.HealthDataConsentVersion: which dated revision of
	// /terms the user accepted, not just that they did.
	TermsAcceptedAt      *time.Time     `gorm:"column:terms_accepted_at"`
	TermsAcceptedVersion *string        `gorm:"column:terms_accepted_version"`
	// SuspendedAt/SuspendedReason are set by an admin's "ระงับบัญชี" action
	// (internal/handler/admin_users.go, always paired with an
	// admin_action_logs row). Non-nil SuspendedAt blocks login — same
	// enforcement point as AccountDeletionRequestedAt above.
	SuspendedAt     *time.Time `gorm:"column:suspended_at"`
	SuspendedReason *string    `gorm:"column:suspended_reason"`
	CreatedAt            time.Time      `gorm:"column:created_at"`
	UpdatedAt            time.Time      `gorm:"column:updated_at"`
	DeletedAt            gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (User) TableName() string {
	return "users"
}
