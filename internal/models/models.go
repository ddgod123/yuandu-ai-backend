package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type User struct {
	ID                    uint64     `gorm:"primaryKey;autoIncrement"`
	Phone                 string     `gorm:"size:32;uniqueIndex"`
	Email                 string     `gorm:"size:255;uniqueIndex"`
	Username              string     `gorm:"size:64;uniqueIndex"`
	PasswordHash          string     `gorm:"size:255"`
	DisplayName           string     `gorm:"size:64"`
	AvatarURL             string     `gorm:"size:512"`
	Bio                   string     `gorm:"type:text"`
	Role                  string     `gorm:"size:32;index"`
	Status                string     `gorm:"size:32;index"`
	SubscriptionStatus    string     `gorm:"size:32;index"`
	SubscriptionPlan      string     `gorm:"size:32"`
	SubscriptionStartedAt *time.Time `gorm:"index"`
	SubscriptionExpiresAt *time.Time `gorm:"index"`
	// Legacy virtual field kept for API compatibility.
	// DB does not have user.users.is_admin; admin state is derived from role.
	IsAdmin     bool           `gorm:"-:all"`
	LastLoginAt *time.Time     `gorm:"index"`
	LastLoginIP string         `gorm:"size:64"`
	CreatedAt   time.Time      `gorm:"autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (User) TableName() string {
	return "user.users"
}

type Collection struct {
	ID               uint64         `gorm:"primaryKey;autoIncrement"`
	Title            string         `gorm:"size:255;index"`
	Slug             string         `gorm:"size:128;index"`
	Description      string         `gorm:"type:text"`
	CoverURL         string         `gorm:"size:512"`
	OwnerID          uint64         `gorm:"index"`
	CreatorProfileID *uint64        `gorm:"column:creator_profile_id;index"`
	CategoryID       *uint64        `gorm:"index"`
	IPID             *uint64        `gorm:"index"`
	ThemeID          *uint64        `gorm:"index"`
	Source           string         `gorm:"size:64;index"`
	QiniuPrefix      string         `gorm:"size:512;index"`
	FileCount        int            `gorm:"index"`
	IsFeatured       bool           `gorm:"index"`
	IsPinned         bool           `gorm:"index"`
	IsSample         bool           `gorm:"index"`
	PinnedAt         *time.Time     `gorm:"index"`
	LatestZipKey     string         `gorm:"size:512"`
	LatestZipName    string         `gorm:"size:255"`
	LatestZipSize    int64          `gorm:""`
	LatestZipAt      *time.Time     `gorm:"index"`
	DownloadCode     string         `gorm:"size:16;uniqueIndex"`
	Visibility       string         `gorm:"size:32;index"`
	Status           string         `gorm:"size:32;index"`
	CreatedAt        time.Time      `gorm:"autoCreateTime"`
	UpdatedAt        time.Time      `gorm:"autoUpdateTime"`
	DeletedAt        gorm.DeletedAt `gorm:"index"`
}

func (Collection) TableName() string {
	return "archive.collections"
}

// VideoAssetCollection is the isolated collection table for video-generated outputs.
type VideoAssetCollection struct {
	ID            uint64 `gorm:"primaryKey;autoIncrement"`
	Title         string `gorm:"size:255;index"`
	Slug          string `gorm:"size:128;index"`
	Description   string `gorm:"type:text"`
	CoverURL      string `gorm:"size:512"`
	OwnerID       uint64 `gorm:"index"`
	Source        string `gorm:"size:64;index"`
	StorageBucket string `gorm:"column:storage_bucket;size:128"`
	QiniuPrefix   string `gorm:"size:512;index"`
	FileCount     int    `gorm:"index"`
	LatestZipKey  string `gorm:"size:512"`
	LatestZipName string `gorm:"size:255"`
	LatestZipSize int64
	LatestZipAt   *time.Time     `gorm:"index"`
	Visibility    string         `gorm:"size:32;index"`
	Status        string         `gorm:"size:32;index"`
	CreatedAt     time.Time      `gorm:"autoCreateTime"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime"`
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

func (VideoAssetCollection) TableName() string {
	return "video_asset.collections"
}

type CollectionZip struct {
	ID           uint64     `gorm:"primaryKey;autoIncrement"`
	CollectionID uint64     `gorm:"index"`
	ZipKey       string     `gorm:"column:zip_key;size:512;index"`
	ZipHash      string     `gorm:"column:zip_hash;size:128;index"`
	ZipName      string     `gorm:"column:zip_name;size:255"`
	SizeBytes    int64      `gorm:"column:size_bytes"`
	UploadedAt   *time.Time `gorm:"column:uploaded_at;index"`
	CreatedAt    time.Time  `gorm:"autoCreateTime"`
}

func (CollectionZip) TableName() string {
	return "archive.collection_zips"
}

type VideoAssetCollectionZip struct {
	ID           uint64     `gorm:"primaryKey;autoIncrement"`
	CollectionID uint64     `gorm:"index"`
	ZipKey       string     `gorm:"column:zip_key;size:512;index"`
	ZipHash      string     `gorm:"column:zip_hash;size:128;index"`
	ZipName      string     `gorm:"column:zip_name;size:255"`
	SizeBytes    int64      `gorm:"column:size_bytes"`
	UploadedAt   *time.Time `gorm:"column:uploaded_at;index"`
	CreatedAt    time.Time  `gorm:"autoCreateTime"`
}

func (VideoAssetCollectionZip) TableName() string {
	return "video_asset.collection_zips"
}

type CollectionDownload struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	CollectionID uint64    `gorm:"index"`
	UserID       *uint64   `gorm:"index"`
	IP           string    `gorm:"size:64"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}

func (CollectionDownload) TableName() string {
	return "action.collection_downloads"
}

type CollectionFavorite struct {
	UserID       uint64    `gorm:"primaryKey"`
	CollectionID uint64    `gorm:"primaryKey"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}

func (CollectionFavorite) TableName() string {
	return "action.collection_favorites"
}

type CollectionLike struct {
	UserID       uint64    `gorm:"primaryKey"`
	CollectionID uint64    `gorm:"primaryKey"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}

func (CollectionLike) TableName() string {
	return "action.collection_likes"
}

type CreatorProfile struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	NameZh    string    `gorm:"size:64;column:name_zh"`
	NameEn    string    `gorm:"size:64;column:name_en"`
	AvatarURL string    `gorm:"size:512"`
	Status    string    `gorm:"size:32;index"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (CreatorProfile) TableName() string {
	return "ops.creator_profiles"
}

type IP struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	Name        string    `gorm:"size:128"`
	Slug        string    `gorm:"size:128;uniqueIndex"`
	CoverURL    string    `gorm:"size:512"`
	CategoryID  *uint64   `gorm:"index"`
	Description string    `gorm:"type:text"`
	Sort        int       `gorm:"index"`
	Status      string    `gorm:"size:32;index"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

func (IP) TableName() string {
	return "taxonomy.ips"
}

type Emoji struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	CollectionID uint64 `gorm:"index"`
	Title        string `gorm:"size:255;index"`
	FileURL      string `gorm:"size:512"`
	ThumbURL     string `gorm:"size:512"`
	Format       string `gorm:"size:32"`
	Width        int    `gorm:"index"`
	Height       int    `gorm:"index"`
	SizeBytes    int64
	DisplayOrder int            `gorm:"index"`
	Status       string         `gorm:"size:32;index"`
	CreatedAt    time.Time      `gorm:"autoCreateTime"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

func (Emoji) TableName() string {
	return "archive.emojis"
}

type VideoAssetEmoji struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	CollectionID uint64 `gorm:"index"`
	Title        string `gorm:"size:255;index"`
	FileURL      string `gorm:"size:512"`
	ThumbURL     string `gorm:"size:512"`
	Format       string `gorm:"size:32"`
	Width        int    `gorm:"index"`
	Height       int    `gorm:"index"`
	SizeBytes    int64
	DisplayOrder int            `gorm:"index"`
	Status       string         `gorm:"size:32;index"`
	CreatedAt    time.Time      `gorm:"autoCreateTime"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

func (VideoAssetEmoji) TableName() string {
	return "video_asset.emojis"
}

type Tag struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	Name      string    `gorm:"size:64;uniqueIndex"`
	Slug      string    `gorm:"size:64;uniqueIndex"`
	GroupID   *uint64   `gorm:"column:tag_group_id;index"`
	Sort      int       `gorm:"index"`
	Status    string    `gorm:"size:32;index"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (Tag) TableName() string {
	return "taxonomy.tags"
}

type TagGroup struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	Name        string    `gorm:"size:64"`
	Slug        string    `gorm:"size:64;uniqueIndex"`
	Description string    `gorm:"type:text"`
	Sort        int       `gorm:"index"`
	Status      string    `gorm:"size:32;index"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

func (TagGroup) TableName() string {
	return "taxonomy.tag_groups"
}

type Category struct {
	ID          uint64         `gorm:"primaryKey;autoIncrement"`
	Name        string         `gorm:"size:128"`
	Slug        string         `gorm:"size:128;index"`
	ParentID    *uint64        `gorm:"index"`
	Prefix      string         `gorm:"size:256;uniqueIndex"`
	Description string         `gorm:"type:text"`
	CoverURL    string         `gorm:"size:512"`
	Icon        string         `gorm:"size:128"`
	Sort        int            `gorm:"index"`
	Status      string         `gorm:"size:32;index"`
	CreatedAt   time.Time      `gorm:"autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (Category) TableName() string {
	return "taxonomy.categories"
}

type Theme struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	Name        string    `gorm:"size:64"`
	Slug        string    `gorm:"size:64;uniqueIndex"`
	Description string    `gorm:"type:text"`
	Sort        int       `gorm:"index"`
	Status      string    `gorm:"size:32;index"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

func (Theme) TableName() string {
	return "taxonomy.themes"
}

type CardTheme struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string         `gorm:"size:64" json:"name"`
	Slug      string         `gorm:"size:64;uniqueIndex" json:"slug"`
	Config    datatypes.JSON `gorm:"type:jsonb" json:"config"`
	IsSystem  bool           `json:"is_system"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (CardTheme) TableName() string {
	return "archive.card_themes"
}

type EmojiTag struct {
	EmojiID uint64 `gorm:"primaryKey"`
	TagID   uint64 `gorm:"primaryKey"`
}

func (EmojiTag) TableName() string {
	return "taxonomy.emoji_tags"
}

type CollectionTag struct {
	CollectionID uint64 `gorm:"primaryKey"`
	TagID        uint64 `gorm:"primaryKey"`
}

func (CollectionTag) TableName() string {
	return "taxonomy.collection_tags"
}

type Favorite struct {
	UserID    uint64    `gorm:"primaryKey"`
	EmojiID   uint64    `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	Emoji     *Emoji    `gorm:"foreignKey:EmojiID"`
}

func (Favorite) TableName() string {
	return "action.favorites"
}

type Like struct {
	UserID    uint64    `gorm:"primaryKey"`
	EmojiID   uint64    `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (Like) TableName() string {
	return "action.likes"
}

type Download struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	UserID    *uint64   `gorm:"index"`
	EmojiID   uint64    `gorm:"index"`
	IP        string    `gorm:"size:64"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (Download) TableName() string {
	return "action.downloads"
}

type UserBehaviorEvent struct {
	ID                 uint64         `gorm:"primaryKey;autoIncrement"`
	UserID             *uint64        `gorm:"index"`
	DeviceID           string         `gorm:"size:128;index"`
	SessionID          string         `gorm:"size:128;index"`
	EventName          string         `gorm:"size:64;index"`
	Route              string         `gorm:"size:512"`
	Referrer           string         `gorm:"size:512"`
	CollectionID       *uint64        `gorm:"index"`
	EmojiID            *uint64        `gorm:"index"`
	IPID               *uint64        `gorm:"index"`
	SubscriptionStatus string         `gorm:"size:32;index"`
	Success            *bool          `gorm:"index"`
	ErrorCode          string         `gorm:"size:64"`
	RequestID          string         `gorm:"size:128"`
	Metadata           datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt          time.Time      `gorm:"autoCreateTime"`
}

func (UserBehaviorEvent) TableName() string {
	return "action.user_behavior_events"
}

type DataAuditRun struct {
	ID                      uint64         `gorm:"primaryKey;autoIncrement"`
	RunAt                   time.Time      `gorm:"column:run_at;index"`
	Status                  string         `gorm:"column:status;size:32;index"`
	Apply                   bool           `gorm:"column:apply"`
	FixOrphans              bool           `gorm:"column:fix_orphans"`
	DurationMs              int64          `gorm:"column:duration_ms"`
	ReportPath              string         `gorm:"column:report_path;type:text"`
	ErrorMessage            string         `gorm:"column:error_message;type:text"`
	DBEmojiTotal            int            `gorm:"column:db_emoji_total"`
	DBZipTotal              int            `gorm:"column:db_zip_total"`
	QiniuObjectTotal        int            `gorm:"column:qiniu_object_total"`
	MissingEmojiObjectCount int            `gorm:"column:missing_emoji_object_count"`
	MissingZipObjectCount   int            `gorm:"column:missing_zip_object_count"`
	QiniuOrphanRawCount     int            `gorm:"column:qiniu_orphan_raw_count"`
	QiniuOrphanZipCount     int            `gorm:"column:qiniu_orphan_zip_count"`
	FileCountMismatchCount  int            `gorm:"column:file_count_mismatch_count"`
	ReportJSON              datatypes.JSON `gorm:"column:report_json;type:jsonb"`
	CreatedAt               time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt               time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (DataAuditRun) TableName() string {
	return "ops.data_audit_runs"
}

type Report struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	UserID    uint64    `gorm:"index"`
	EmojiID   uint64    `gorm:"index"`
	Reason    string    `gorm:"type:text"`
	Status    string    `gorm:"size:32;index"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (Report) TableName() string {
	return "audit.reports"
}

type AuditLog struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement"`
	AdminID    uint64         `gorm:"index"`
	TargetType string         `gorm:"size:32;index"`
	TargetID   uint64         `gorm:"index"`
	Action     string         `gorm:"size:64"`
	Meta       datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt  time.Time      `gorm:"autoCreateTime"`
}

func (AuditLog) TableName() string {
	return "audit.audit_logs"
}

type JoinApplication struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	Name       string    `gorm:"size:64;index"`
	Phone      string    `gorm:"size:32;index"`
	Gender     string    `gorm:"size:16;index"`
	Age        int       `gorm:"index"`
	Email      string    `gorm:"size:255;index"`
	Occupation string    `gorm:"size:64;index"`
	CreatedAt  time.Time `gorm:"autoCreateTime;index"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

func (JoinApplication) TableName() string {
	return "audit.join_applications"
}

type UploadTask struct {
	ID           uint64  `gorm:"primaryKey;autoIncrement"`
	Kind         string  `gorm:"size:32;index"`
	Status       string  `gorm:"size:32;index"`
	Stage        string  `gorm:"size:32;index"`
	UserID       uint64  `gorm:"index"`
	CollectionID *uint64 `gorm:"index"`
	CategoryID   *uint64 `gorm:"index"`
	FileName     string  `gorm:"size:255"`
	FileSize     int64
	Input        datatypes.JSON `gorm:"type:jsonb"`
	Result       datatypes.JSON `gorm:"type:jsonb"`
	ErrorMessage string         `gorm:"column:error_message;type:text"`
	StartedAt    time.Time      `gorm:"column:started_at;autoCreateTime"`
	FinishedAt   *time.Time     `gorm:"column:finished_at;index"`
	CreatedAt    time.Time      `gorm:"autoCreateTime"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime"`
}

func (UploadTask) TableName() string {
	return "ops.upload_tasks"
}

type HomeDailyStats struct {
	StatDate         time.Time `gorm:"type:date;primaryKey"`
	TotalCollections int64     `gorm:"column:total_collections"`
	TotalEmojis      int64     `gorm:"column:total_emojis"`
	TodayNewEmojis   int64     `gorm:"column:today_new_emojis"`
	UpdatedAt        time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (HomeDailyStats) TableName() string {
	return "audit.home_daily_stats"
}

type SiteFooterSetting struct {
	ID                   int16     `gorm:"primaryKey;default:1"`
	SiteName             string    `gorm:"column:site_name;size:128"`
	SiteDescription      string    `gorm:"column:site_description;type:text"`
	ContactEmail         string    `gorm:"column:contact_email;size:255"`
	ComplaintEmail       string    `gorm:"column:complaint_email;size:255"`
	SelfMediaLogo        string    `gorm:"column:self_media_logo;size:512"`
	SelfMediaQRCode      string    `gorm:"column:self_media_qr_code;size:512"`
	ICPNumber            string    `gorm:"column:icp_number;size:128"`
	ICPLink              string    `gorm:"column:icp_link;size:512"`
	PublicSecurityNumber string    `gorm:"column:public_security_number;size:128"`
	PublicSecurityLink   string    `gorm:"column:public_security_link;size:512"`
	CopyrightText        string    `gorm:"column:copyright_text;size:255"`
	CreatedAt            time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt            time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (SiteFooterSetting) TableName() string {
	return "ops.site_footer_settings"
}

type RedeemCode struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement"`
	CodeHash      string     `gorm:"column:code_hash;size:128;uniqueIndex"`
	CodePlain     string     `gorm:"column:code_plain;size:64"`
	CodeMask      string     `gorm:"column:code_mask;size:64;index"`
	BatchNo       string     `gorm:"column:batch_no;size:64;index"`
	Plan          string     `gorm:"column:plan;size:32;index"`
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

func (RedeemCode) TableName() string {
	return "ops.redeem_codes"
}

type RedeemCodeRedemption struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement"`
	CodeID           uint64    `gorm:"column:code_id;index"`
	UserID           uint64    `gorm:"column:user_id;index"`
	GrantedPlan      string    `gorm:"column:granted_plan;size:32"`
	GrantedStatus    string    `gorm:"column:granted_status;size:32"`
	GrantedStartsAt  time.Time `gorm:"column:granted_starts_at"`
	GrantedExpiresAt time.Time `gorm:"column:granted_expires_at"`
	IP               string    `gorm:"column:ip;size:64"`
	UserAgent        string    `gorm:"column:user_agent;size:255"`
	CreatedAt        time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (RedeemCodeRedemption) TableName() string {
	return "ops.redeem_code_redemptions"
}

type CollectionDownloadCode struct {
	ID                   uint64     `gorm:"primaryKey;autoIncrement"`
	CodeHash             string     `gorm:"column:code_hash;size:128;uniqueIndex"`
	CodePlain            string     `gorm:"column:code_plain;size:64"`
	CodeMask             string     `gorm:"column:code_mask;size:64;index"`
	BatchNo              string     `gorm:"column:batch_no;size:64;index"`
	CollectionID         uint64     `gorm:"column:collection_id;index"`
	GrantedDownloadTimes int        `gorm:"column:granted_download_times"`
	MaxRedeemUsers       int        `gorm:"column:max_redeem_users"`
	UsedRedeemUsers      int        `gorm:"column:used_redeem_users;index"`
	Status               string     `gorm:"column:status;size:32;index"`
	StartsAt             *time.Time `gorm:"column:starts_at;index"`
	EndsAt               *time.Time `gorm:"column:ends_at;index"`
	CreatedBy            *uint64    `gorm:"column:created_by;index"`
	Note                 string     `gorm:"column:note;type:text"`
	CreatedAt            time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt            time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (CollectionDownloadCode) TableName() string {
	return "ops.collection_download_codes"
}

type CollectionDownloadEntitlement struct {
	ID                     uint64     `gorm:"primaryKey;autoIncrement"`
	UserID                 uint64     `gorm:"column:user_id;index"`
	CollectionID           uint64     `gorm:"column:collection_id;index"`
	CodeID                 *uint64    `gorm:"column:code_id;index"`
	GrantedDownloadTimes   int        `gorm:"column:granted_download_times"`
	UsedDownloadTimes      int        `gorm:"column:used_download_times"`
	RemainingDownloadTimes int        `gorm:"column:remaining_download_times;index"`
	Status                 string     `gorm:"column:status;size:32;index"`
	ExpiresAt              *time.Time `gorm:"column:expires_at;index"`
	LastConsumedAt         *time.Time `gorm:"column:last_consumed_at;index"`
	CreatedAt              time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt              time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (CollectionDownloadEntitlement) TableName() string {
	return "ops.collection_download_entitlements"
}

type CollectionDownloadRedemption struct {
	ID                   uint64     `gorm:"primaryKey;autoIncrement"`
	CodeID               uint64     `gorm:"column:code_id;index"`
	UserID               uint64     `gorm:"column:user_id;index"`
	CollectionID         uint64     `gorm:"column:collection_id;index"`
	GrantedDownloadTimes int        `gorm:"column:granted_download_times"`
	ExpiresAt            *time.Time `gorm:"column:expires_at;index"`
	IP                   string     `gorm:"column:ip;size:64"`
	UserAgent            string     `gorm:"column:user_agent;size:255"`
	CreatedAt            time.Time  `gorm:"column:created_at;autoCreateTime"`
}

func (CollectionDownloadRedemption) TableName() string {
	return "ops.collection_download_redemptions"
}

type CollectionDownloadConsumption struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement"`
	EntitlementID uint64    `gorm:"column:entitlement_id;index"`
	UserID        uint64    `gorm:"column:user_id;index"`
	CollectionID  uint64    `gorm:"column:collection_id;index"`
	CodeID        *uint64   `gorm:"column:code_id;index"`
	DownloadMode  string    `gorm:"column:download_mode;size:32;index"`
	ConsumedTimes int       `gorm:"column:consumed_times"`
	IP            string    `gorm:"column:ip;size:64"`
	UserAgent     string    `gorm:"column:user_agent;size:255"`
	RequestID     string    `gorm:"column:request_id;size:128;index"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime;index"`
}

func (CollectionDownloadConsumption) TableName() string {
	return "action.collection_download_consumptions"
}

type RiskBlacklist struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement"`
	Scope     string         `gorm:"column:scope;size:16;index"`
	Target    string         `gorm:"column:target;size:191;index"`
	Action    string         `gorm:"column:action;size:32;index"`
	Status    string         `gorm:"column:status;size:16;index"`
	Reason    string         `gorm:"column:reason;type:text"`
	ExpiresAt *time.Time     `gorm:"column:expires_at;index"`
	CreatedBy *uint64        `gorm:"column:created_by;index"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (RiskBlacklist) TableName() string {
	return "ops.risk_blacklists"
}

type RiskEvent struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement"`
	EventType string         `gorm:"column:event_type;size:64;index"`
	Action    string         `gorm:"column:action;size:32;index"`
	Scope     string         `gorm:"column:scope;size:16;index"`
	Target    string         `gorm:"column:target;size:191;index"`
	Severity  string         `gorm:"column:severity;size:16;index"`
	Message   string         `gorm:"column:message;type:text"`
	Metadata  datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime;index"`
}

func (RiskEvent) TableName() string {
	return "ops.risk_events"
}
