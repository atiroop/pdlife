package models

import "time"

type AdminAction string

const (
	AdminActionManualVerifyEmail AdminAction = "manual_verify_email"
	AdminActionUnlockAccount     AdminAction = "unlock_account"
	AdminActionSuspendAccount    AdminAction = "suspend_account"
	AdminActionUnsuspendAccount  AdminAction = "unsuspend_account"
)

// AdminActionLog is the mandatory audit trail for every account-level
// action an admin performs against another user (see
// internal/handler/admin_users.go). Rows are written in the same
// transaction as the action itself so an action can never happen
// unlogged. Reason is required for suspend_account (an admin must state
// why), optional elsewhere.
type AdminActionLog struct {
	ID           uint64      `gorm:"column:id;primaryKey"`
	AdminID      uint64      `gorm:"column:admin_id;not null"`
	TargetUserID uint64      `gorm:"column:target_user_id;not null"`
	Action       AdminAction `gorm:"column:action;type:enum('manual_verify_email','unlock_account','suspend_account','unsuspend_account');not null"`
	Reason       *string     `gorm:"column:reason"`
	CreatedAt    time.Time   `gorm:"column:created_at"`
}

func (AdminActionLog) TableName() string {
	return "admin_action_logs"
}
