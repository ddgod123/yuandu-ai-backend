package models

import "time"

type SmsCode struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	Phone     string    `gorm:"size:32;index"`
	Code      string    `gorm:"size:16"`
	ExpiresAt time.Time `gorm:"index"`
	Used      bool      `gorm:"default:false"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (SmsCode) TableName() string {
	return "user.sms_codes"
}
