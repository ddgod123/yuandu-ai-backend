package models

import (
	"time"

	"gorm.io/datatypes"
)

type VideoImageSplitBackfillRun struct {
	ID             uint64         `gorm:"primaryKey;autoIncrement"`
	RunID          string         `gorm:"column:run_id;size:128;uniqueIndex"`
	Status         string         `gorm:"column:status;size:32;index"`
	RequestedBy    uint64         `gorm:"column:requested_by;index"`
	Apply          bool           `gorm:"column:apply;index"`
	BatchSize      int            `gorm:"column:batch_size"`
	FormatFilter   string         `gorm:"column:format_filter;size:16;index"`
	FallbackFormat string         `gorm:"column:fallback_format;size:16"`
	Tables         string         `gorm:"column:tables;size:255"`
	Stopped        bool           `gorm:"column:stopped;index"`
	LastError      string         `gorm:"column:last_error;type:text"`
	OptionsJSON    datatypes.JSON `gorm:"column:options_json;type:jsonb"`
	ReportJSON     datatypes.JSON `gorm:"column:report_json;type:jsonb"`
	StartedAt      *time.Time     `gorm:"column:started_at;index"`
	FinishedAt     *time.Time     `gorm:"column:finished_at;index"`
	HeartbeatAt    *time.Time     `gorm:"column:heartbeat_at;index"`
	CreatedAt      time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoImageSplitBackfillRun) TableName() string {
	return "ops.video_image_split_backfill_runs"
}
