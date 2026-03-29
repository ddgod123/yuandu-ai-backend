package models

import "time"

// ComputeRedeemCode stores redeemable code batches used to grant compute points.
type ComputeRedeemCode struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement"`
	CodeHash      string     `gorm:"column:code_hash;size:128;uniqueIndex"`
	CodePlain     string     `gorm:"column:code_plain;size:64"`
	CodeMask      string     `gorm:"column:code_mask;size:64;index"`
	BatchNo       string     `gorm:"column:batch_no;size:64;index"`
	GrantedPoints int64      `gorm:"column:granted_points"`
	DurationDays  int        `gorm:"column:duration_days"`
	MaxUses       int        `gorm:"column:max_uses"`
	UsedCount     int        `gorm:"column:used_count;index"`
	Status        string     `gorm:"column:status;size:32;index"`
	StartsAt      *time.Time `gorm:"column:starts_at;index"`
	EndsAt        *time.Time `gorm:"column:ends_at;index"`
	CreatedBy     *uint64    `gorm:"column:created_by;index"`
	Note          string     `gorm:"column:note;type:text"`
	LastIssuedAt  *time.Time `gorm:"column:last_issued_at;index"`
	LastIssuedUID *uint64    `gorm:"column:last_issued_uid;index"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (ComputeRedeemCode) TableName() string {
	return "ops.compute_redeem_codes"
}

// ComputeRedeemRedemption stores each successful user redemption against a compute code.
type ComputeRedeemRedemption struct {
	ID               uint64     `gorm:"primaryKey;autoIncrement"`
	CodeID           uint64     `gorm:"column:code_id;index"`
	UserID           uint64     `gorm:"column:user_id;index"`
	GrantedPoints    int64      `gorm:"column:granted_points"`
	GrantedStartsAt  time.Time  `gorm:"column:granted_starts_at"`
	GrantedExpiresAt *time.Time `gorm:"column:granted_expires_at;index"`
	ClearStatus      string     `gorm:"column:clear_status;size:32;index;default:pending"`
	ClearedPoints    int64      `gorm:"column:cleared_points;default:0"`
	ClearedAt        *time.Time `gorm:"column:cleared_at;index"`
	IP               string     `gorm:"column:ip;size:64"`
	UserAgent        string     `gorm:"column:user_agent;size:255"`
	CreatedAt        time.Time  `gorm:"column:created_at;autoCreateTime"`
}

func (ComputeRedeemRedemption) TableName() string {
	return "ops.compute_redeem_redemptions"
}
