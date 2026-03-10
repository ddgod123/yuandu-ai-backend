package models

import (
	"time"

	"gorm.io/gorm"
)

type MemeTemplate struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Name       string         `gorm:"size:128" json:"name"`
	ImageURL   string         `gorm:"size:512" json:"image_url"`
	Category   string         `gorm:"size:64;index" json:"category"`
	Tags       string         `gorm:"size:255" json:"tags"`
	TextX      int            `json:"text_x"`
	TextY      int            `json:"text_y"`
	TextWidth  int            `json:"text_width"`
	TextHeight int            `json:"text_height"`
	TextColor  string         `gorm:"size:32;default:#FFFFFF" json:"text_color"`
	FontSize   int            `gorm:"default:32" json:"font_size"`
	Status     string         `gorm:"size:32;index;default:active" json:"status"`
	CreatedAt  time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (MemeTemplate) TableName() string {
	return "meme.templates"
}
