package models

import "time"

// FoodCheckSourceSnapshotSource identifies which upstream source a
// snapshot row was taken from. Deliberately a plain string type (not
// reusing FoodCheckSource) since its enum values ('inmu'/'anamai') are
// shorter than FoodCheckSource's ('thaifcd_inmu'/'thaifcd_anamai') — this
// table only ever talks to cmd/foodcheck_diffcheck, not the rest of Food
// Check, so there's no cross-package value to sharing the type.
type FoodCheckSourceSnapshotSource string

const (
	SnapshotSourceINMU   FoodCheckSourceSnapshotSource = "inmu"
	SnapshotSourceAnamai FoodCheckSourceSnapshotSource = "anamai"
)

// FoodCheckSourceSnapshot is one monthly drift-check run's result for one
// source (see cmd/foodcheck_diffcheck). Only ever written by that
// standalone tool — the web app never reads or writes this table.
type FoodCheckSourceSnapshot struct {
	ID          uint64                        `gorm:"column:id;primaryKey"`
	Source      FoodCheckSourceSnapshotSource `gorm:"column:source;type:enum('inmu','anamai');not null"`
	ItemCount   int                           `gorm:"column:item_count;not null"`
	ContentHash string                        `gorm:"column:content_hash;not null"`
	CheckedAt   time.Time                     `gorm:"column:checked_at;not null"`
	RawSnapshot *string                       `gorm:"column:raw_snapshot"`
	CreatedAt   time.Time                     `gorm:"column:created_at"`
	UpdatedAt   time.Time                     `gorm:"column:updated_at"`
}

func (FoodCheckSourceSnapshot) TableName() string {
	return "foodcheck_source_snapshots"
}
