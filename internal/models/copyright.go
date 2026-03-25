package models

import (
	"time"

	"gorm.io/datatypes"
)

type CollectionCopyrightTask struct {
	ID                   uint64     `gorm:"primaryKey;autoIncrement"`
	TaskNo               string     `gorm:"column:task_no;size:64;uniqueIndex"`
	CollectionID         uint64     `gorm:"column:collection_id;index"`
	RunMode              string     `gorm:"column:run_mode;size:16"`
	SampleStrategy       string     `gorm:"column:sample_strategy;size:16"`
	SampleCount          int        `gorm:"column:sample_count"`
	ActualSampleCount    int        `gorm:"column:actual_sample_count"`
	EnableTagging        bool       `gorm:"column:enable_tagging"`
	OverwriteMachineTags bool       `gorm:"column:overwrite_machine_tags"`
	Status               string     `gorm:"column:status;size:16;index"`
	Progress             int        `gorm:"column:progress"`
	HighRiskCount        int        `gorm:"column:high_risk_count"`
	UnknownSourceCount   int        `gorm:"column:unknown_source_count"`
	IPHitCount           int        `gorm:"column:ip_hit_count"`
	MachineConclusion    string     `gorm:"column:machine_conclusion;size:64"`
	ResultSummary        string     `gorm:"column:result_summary;type:text"`
	CreatedBy            *uint64    `gorm:"column:created_by;index"`
	StartedAt            *time.Time `gorm:"column:started_at"`
	FinishedAt           *time.Time `gorm:"column:finished_at"`
	CreatedAt            time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt            time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (CollectionCopyrightTask) TableName() string {
	return "ops.collection_copyright_tasks"
}

type CollectionCopyrightTaskImage struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	TaskID       uint64    `gorm:"column:task_id;index"`
	CollectionID uint64    `gorm:"column:collection_id;index"`
	EmojiID      uint64    `gorm:"column:emoji_id;index"`
	SampleOrder  int       `gorm:"column:sample_order"`
	Status       string    `gorm:"column:status;size:16;index"`
	ErrorMsg     string    `gorm:"column:error_msg;type:text"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (CollectionCopyrightTaskImage) TableName() string {
	return "ops.collection_copyright_task_images"
}

type ImageCopyrightResult struct {
	ID                  uint64         `gorm:"primaryKey;autoIncrement"`
	TaskID              uint64         `gorm:"column:task_id;index"`
	CollectionID        uint64         `gorm:"column:collection_id;index"`
	EmojiID             uint64         `gorm:"column:emoji_id;index"`
	OCRText             string         `gorm:"column:ocr_text;type:text"`
	ContentType         string         `gorm:"column:content_type;size:64"`
	CopyrightOwnerGuess string         `gorm:"column:copyright_owner_guess;size:255"`
	OwnerType           string         `gorm:"column:owner_type;size:64"`
	IsCommercialIP      bool           `gorm:"column:is_commercial_ip"`
	IPName              string         `gorm:"column:ip_name;size:255;index"`
	IsBrandRelated      bool           `gorm:"column:is_brand_related"`
	BrandName           string         `gorm:"column:brand_name;size:255;index"`
	IsCelebrityRelated  bool           `gorm:"column:is_celebrity_related"`
	CelebrityName       string         `gorm:"column:celebrity_name;size:255"`
	IsScreenshot        bool           `gorm:"column:is_screenshot"`
	IsSourceUnknown     bool           `gorm:"column:is_source_unknown"`
	RightsStatus        string         `gorm:"column:rights_status;size:64"`
	CommercialUseAdvice string         `gorm:"column:commercial_use_advice;size:64"`
	RiskLevel           string         `gorm:"column:risk_level;size:16;index"`
	RiskScore           float64        `gorm:"column:risk_score"`
	ModelConfidence     float64        `gorm:"column:model_confidence"`
	EvidenceJSON        datatypes.JSON `gorm:"column:evidence_json;type:jsonb"`
	MachineSummary      string         `gorm:"column:machine_summary;type:text"`
	CreatedAt           time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt           time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (ImageCopyrightResult) TableName() string {
	return "ops.image_copyright_results"
}

type CollectionCopyrightResult struct {
	ID                 uint64     `gorm:"primaryKey;autoIncrement"`
	CollectionID       uint64     `gorm:"column:collection_id;uniqueIndex"`
	LatestTaskID       uint64     `gorm:"column:latest_task_id;index"`
	RunMode            string     `gorm:"column:run_mode;size:16"`
	SampleCoverage     float64    `gorm:"column:sample_coverage"`
	MachineConclusion  string     `gorm:"column:machine_conclusion;size:64"`
	MachineConfidence  float64    `gorm:"column:machine_confidence"`
	RiskLevel          string     `gorm:"column:risk_level;size:16;index"`
	SampledImageCount  int        `gorm:"column:sampled_image_count"`
	HighRiskCount      int        `gorm:"column:high_risk_count"`
	UnknownSourceCount int        `gorm:"column:unknown_source_count"`
	IPHitCount         int        `gorm:"column:ip_hit_count"`
	BrandHitCount      int        `gorm:"column:brand_hit_count"`
	RecommendedAction  string     `gorm:"column:recommended_action;size:64"`
	ReviewStatus       string     `gorm:"column:review_status;size:32;index"`
	FinalDecision      string     `gorm:"column:final_decision;size:64"`
	FinalReviewerID    *uint64    `gorm:"column:final_reviewer_id"`
	FinalReviewedAt    *time.Time `gorm:"column:final_reviewed_at"`
	Summary            string     `gorm:"column:summary;type:text"`
	CreatedAt          time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (CollectionCopyrightResult) TableName() string {
	return "ops.collection_copyright_results"
}

type CopyrightReviewRecord struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement"`
	CollectionID  uint64    `gorm:"column:collection_id;index"`
	EmojiID       *uint64   `gorm:"column:emoji_id;index"`
	TaskID        *uint64   `gorm:"column:task_id;index"`
	ReviewType    string    `gorm:"column:review_type;size:16"`
	ReviewStatus  string    `gorm:"column:review_status;size:32"`
	ReviewResult  string    `gorm:"column:review_result;size:64"`
	ReviewComment string    `gorm:"column:review_comment;type:text"`
	ReviewerID    uint64    `gorm:"column:reviewer_id;index"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (CopyrightReviewRecord) TableName() string {
	return "ops.copyright_review_records"
}

type TagDimension struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement"`
	DimensionCode string    `gorm:"column:dimension_code;size:64;uniqueIndex"`
	DimensionName string    `gorm:"column:dimension_name;size:64"`
	SortNo        int       `gorm:"column:sort_no"`
	Status        int16     `gorm:"column:status"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (TagDimension) TableName() string {
	return "taxonomy.tag_dimensions"
}

type TagDefinition struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement"`
	TagCode       string    `gorm:"column:tag_code;size:128;uniqueIndex"`
	TagName       string    `gorm:"column:tag_name;size:128"`
	DimensionCode string    `gorm:"column:dimension_code;size:64;index"`
	TagLevel      string    `gorm:"column:tag_level;size:16"`
	IsSystem      bool      `gorm:"column:is_system"`
	SortNo        int       `gorm:"column:sort_no"`
	Status        int16     `gorm:"column:status"`
	Remark        string    `gorm:"column:remark;size:255"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (TagDefinition) TableName() string {
	return "taxonomy.tag_definitions"
}

type EmojiAutoTag struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	EmojiID      uint64    `gorm:"column:emoji_id;index"`
	CollectionID uint64    `gorm:"column:collection_id;index"`
	TaskID       *uint64   `gorm:"column:task_id;index"`
	TagID        uint64    `gorm:"column:tag_id;index"`
	Source       string    `gorm:"column:source;size:16"`
	Confidence   float64   `gorm:"column:confidence"`
	ModelVersion string    `gorm:"column:model_version;size:64"`
	Status       int16     `gorm:"column:status"`
	CreatedBy    *uint64   `gorm:"column:created_by;index"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (EmojiAutoTag) TableName() string {
	return "taxonomy.emoji_auto_tags"
}

type CollectionAutoTag struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	CollectionID uint64    `gorm:"column:collection_id;index"`
	TaskID       *uint64   `gorm:"column:task_id;index"`
	TagID        uint64    `gorm:"column:tag_id;index"`
	Source       string    `gorm:"column:source;size:16"`
	Confidence   float64   `gorm:"column:confidence"`
	ModelVersion string    `gorm:"column:model_version;size:64"`
	Status       int16     `gorm:"column:status"`
	CreatedBy    *uint64   `gorm:"column:created_by;index"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (CollectionAutoTag) TableName() string {
	return "taxonomy.collection_auto_tags"
}

type CopyrightEvidence struct {
	ID            uint64         `gorm:"primaryKey;autoIncrement"`
	CollectionID  uint64         `gorm:"column:collection_id;index"`
	EmojiID       *uint64        `gorm:"column:emoji_id;index"`
	TaskID        *uint64        `gorm:"column:task_id;index"`
	EvidenceType  string         `gorm:"column:evidence_type;size:32"`
	EvidenceTitle string         `gorm:"column:evidence_title;size:255"`
	EvidenceValue string         `gorm:"column:evidence_value;type:text"`
	EvidenceURL   string         `gorm:"column:evidence_url;size:1024"`
	ExtraJSON     datatypes.JSON `gorm:"column:extra_json;type:jsonb"`
	CreatedAt     time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (CopyrightEvidence) TableName() string {
	return "ops.copyright_evidences"
}

type CopyrightTaskLog struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement"`
	TaskID     uint64         `gorm:"column:task_id;index"`
	EmojiID    *uint64        `gorm:"column:emoji_id;index"`
	Stage      string         `gorm:"column:stage;size:32"`
	Status     string         `gorm:"column:status;size:16"`
	Message    string         `gorm:"column:message;size:1024"`
	DetailJSON datatypes.JSON `gorm:"column:detail_json;type:jsonb"`
	CreatedAt  time.Time      `gorm:"column:created_at;autoCreateTime"`
}

func (CopyrightTaskLog) TableName() string {
	return "ops.copyright_task_logs"
}
