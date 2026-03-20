package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppName string
	Env     string
	Port    int

	DBHost       string
	DBPort       int
	DBUser       string
	DBPassword   string
	DBName       string
	DBSSLMode    string
	DBTimezone   string
	DBSearchPath string

	JWTSecret       string
	JWTIssuer       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	MinIOUseSSL    bool

	AsynqRedisAddr     string
	AsynqRedisPassword string
	AsynqRedisDB       int
	WorkerStartCommand string
	WorkerStartTimeout int
	GIFSICLEBin        string
	// Enable legacy feedback_v1 fallback/mirror path.
	// Default false: strict output_id/candidate_id learning only.
	EnableLegacyFeedbackFallback bool

	QiniuAccessKey string
	QiniuSecretKey string
	QiniuBucket    string
	QiniuDomain    string
	QiniuZone      string
	QiniuUseHTTPS  bool
	QiniuUseCDN    bool
	QiniuPrivate   bool
	QiniuSignTTL   int
	// Allow degraded create path when source preflight probe fails (dev/staging helper).
	VideoSourceProbeAllowDegraded bool

	AliyunAccessKeyId                     string
	AliyunAccessKeySecret                 string
	AliyunSmsSignName                     string
	AliyunSmsTemplateCode                 string
	AliyunSmsTemplateParam                string
	AliyunSmsValidTime                    int
	AliyunSmsInterval                     int
	AliyunSmsReturnCode                   bool
	AliyunSmsRegionId                     string
	AliyunSmsEndpoint                     string
	AliyunSmsCountryCode                  string
	AliyunSmsDailyMaxPhone                int
	AliyunSmsDailyMaxIP                   int
	AliyunSmsDailyMaxDevice               int
	AliyunSmsDailyMaxUniquePhonePerIP     int
	AliyunSmsDailyMaxUniquePhonePerDevice int
	LoginDailyMaxPhone                    int
	LoginDailyMaxIP                       int
	LoginDailyMaxDevice                   int
	DevAuthEnabled                        bool
	DevAuthPhone                          string
	DevAuthCode                           string
	CaptchaTTLSeconds                     int
	CaptchaLength                         int
	RegisterDailyMaxDevice                int
	RegisterDailyMaxIP                    int
	AuthFailWindowSeconds                 int
	AuthFailLockLevel1                    int
	AuthFailLockLevel2                    int
	AuthFailLockTTL1                      int
	AuthFailLockTTL2                      int

	DownloadEmojiPerMinuteUser int
	DownloadEmojiPerMinuteIP   int
	DownloadEmojiPerHourUser   int
	DownloadEmojiPerHourIP     int

	DownloadCollectionPerHourUser int
	DownloadCollectionPerHourIP   int
	DownloadCollectionPerDayUser  int
	DownloadCollectionPerDayIP    int

	RedeemValidatePer10MinUser int
	RedeemValidatePer10MinIP   int
	RedeemSubmitPer10MinUser   int
	RedeemSubmitPer10MinIP     int
	DownloadTicketTTL          int
	DownloadTicketSignTTL      int
	DownloadTicketBindIP       bool
	DownloadTicketBindUA       bool
	RiskAutoBlockEnabled       bool
	RiskAutoBlockThreshold     int
	RiskAutoBlockWindowSeconds int
	RiskAutoBlockDurationSec   int

	TelegramBotToken    string
	TelegramDownloadDir string
	TelegramProxy       string

	AuthAccessCookieName  string
	AuthRefreshCookieName string
	AuthCookieDomain      string
	AuthCookieSecure      bool
	AuthCookieSameSite    string

	CORSAllowOrigins []string

	// Aliyun OSS
	OSSEndpoint        string
	OSSAccessKeyID     string
	OSSAccessKeySecret string
	OSSBucket          string
	OSSRegion          string
	OSSBaseURL         string

	// LLM API (supports: claude, deepseek, qwen, moonshot, or any OpenAI-compatible)
	LLMProvider string // "claude" | "deepseek" | "qwen" | "moonshot" | ...
	LLMAPIKey   string
	LLMModel    string // leave empty for provider default
	LLMEndpoint string // leave empty for provider default

	// GIF AI Planner (Stage1) / Judge (Stage3)
	AIPlannerEnabled       bool
	AIPlannerProvider      string
	AIPlannerAPIKey        string
	AIPlannerModel         string
	AIPlannerEndpoint      string
	AIPlannerPromptVersion string
	AIPlannerTimeoutSec    int
	AIPlannerMaxTokens     int

	AIDirectorEnabled       bool
	AIDirectorProvider      string
	AIDirectorAPIKey        string
	AIDirectorModel         string
	AIDirectorEndpoint      string
	AIDirectorPromptVersion string
	AIDirectorTimeoutSec    int
	AIDirectorMaxTokens     int

	AIJudgeEnabled       bool
	AIJudgeProvider      string
	AIJudgeAPIKey        string
	AIJudgeModel         string
	AIJudgeEndpoint      string
	AIJudgePromptVersion string
	AIJudgeTimeoutSec    int
	AIJudgeMaxTokens     int

	// Legacy aliases (still read, used as fallback)
	ClaudeAPIKey   string
	ClaudeModel    string
	ClaudeEndpoint string

	// Font path for meme composition
	FontPath string
}

func Load() Config {
	cfg := Config{}
	cfg.AppName = getEnv("APP_NAME", "emoji")
	cfg.Env = getEnv("APP_ENV", "dev")
	cfg.Port = getEnvAsInt("APP_PORT", 5050)

	cfg.DBHost = getEnv("DB_HOST", "localhost")
	cfg.DBPort = getEnvAsInt("DB_PORT", 5432)
	cfg.DBUser = getEnv("DB_USER", os.Getenv("USER"))
	cfg.DBPassword = getEnv("DB_PASSWORD", "")
	cfg.DBName = getEnv("DB_NAME", "emojiDB")
	cfg.DBSSLMode = getEnv("DB_SSLMODE", "disable")
	cfg.DBTimezone = getEnv("DB_TIMEZONE", "UTC")
	cfg.DBSearchPath = getEnv("DB_SEARCH_PATH", "\"user\",archive,taxonomy,action,audit,public")

	cfg.JWTSecret = getEnv("JWT_SECRET", "change-me")
	cfg.JWTIssuer = getEnv("JWT_ISSUER", "emoji")
	cfg.AccessTokenTTL = getEnvAsDuration("JWT_ACCESS_TTL", 2*time.Hour)
	cfg.RefreshTokenTTL = getEnvAsDuration("JWT_REFRESH_TTL", 720*time.Hour)

	cfg.RedisAddr = getEnv("REDIS_ADDR", "127.0.0.1:6379")
	cfg.RedisPassword = getEnv("REDIS_PASSWORD", "")
	cfg.RedisDB = getEnvAsInt("REDIS_DB", 0)

	cfg.MinIOEndpoint = getEnv("MINIO_ENDPOINT", "127.0.0.1:9000")
	cfg.MinIOAccessKey = getEnv("MINIO_ACCESS_KEY", "minioadmin")
	cfg.MinIOSecretKey = getEnv("MINIO_SECRET_KEY", "minioadmin")
	cfg.MinIOBucket = getEnv("MINIO_BUCKET", "emoji")
	cfg.MinIOUseSSL = getEnvAsBool("MINIO_USE_SSL", false)

	cfg.AsynqRedisAddr = getEnv("ASYNQ_REDIS_ADDR", cfg.RedisAddr)
	cfg.AsynqRedisPassword = getEnv("ASYNQ_REDIS_PASSWORD", cfg.RedisPassword)
	cfg.AsynqRedisDB = getEnvAsInt("ASYNQ_REDIS_DB", cfg.RedisDB)
	cfg.WorkerStartCommand = getEnv("WORKER_START_COMMAND", "")
	cfg.WorkerStartTimeout = getEnvAsInt("WORKER_START_TIMEOUT_SECONDS", 20)
	cfg.GIFSICLEBin = getEnv("GIFSICLE_BIN", "")
	cfg.EnableLegacyFeedbackFallback = getEnvAsBool("ENABLE_LEGACY_FEEDBACK_FALLBACK", false)

	cfg.QiniuAccessKey = getEnv("QINIU_ACCESS_KEY", "")
	cfg.QiniuSecretKey = getEnv("QINIU_SECRET_KEY", "")
	cfg.QiniuBucket = getEnv("QINIU_BUCKET", "")
	cfg.QiniuDomain = getEnv("QINIU_DOMAIN", "")
	cfg.QiniuZone = getEnv("QINIU_ZONE", "")
	cfg.QiniuUseHTTPS = getEnvAsBool("QINIU_USE_HTTPS", true)
	cfg.QiniuUseCDN = getEnvAsBool("QINIU_USE_CDN", false)
	cfg.QiniuPrivate = getEnvAsBool("QINIU_PRIVATE", false)
	cfg.QiniuSignTTL = getEnvAsInt("QINIU_SIGN_TTL", 3600)
	cfg.VideoSourceProbeAllowDegraded = getEnvAsBool("VIDEO_SOURCE_PROBE_ALLOW_DEGRADED", strings.ToLower(strings.TrimSpace(cfg.Env)) != "prod")

	cfg.AliyunAccessKeyId = getEnv("ALIYUN_ACCESS_KEY_ID", "")
	cfg.AliyunAccessKeySecret = getEnv("ALIYUN_ACCESS_KEY_SECRET", "")
	cfg.AliyunSmsSignName = getEnv("ALIYUN_SMS_SIGN_NAME", "")
	cfg.AliyunSmsTemplateCode = getEnv("ALIYUN_SMS_TEMPLATE_CODE", "")
	cfg.AliyunSmsTemplateParam = getEnv("ALIYUN_SMS_TEMPLATE_PARAM", "")
	cfg.AliyunSmsValidTime = getEnvAsInt("ALIYUN_SMS_VALID_TIME", 300)
	cfg.AliyunSmsInterval = getEnvAsInt("ALIYUN_SMS_INTERVAL", 60)
	cfg.AliyunSmsReturnCode = getEnvAsBool("ALIYUN_SMS_RETURN_CODE", false)
	cfg.AliyunSmsRegionId = getEnv("ALIYUN_SMS_REGION_ID", "cn-hangzhou")
	cfg.AliyunSmsEndpoint = getEnv("ALIYUN_SMS_ENDPOINT", "dypnsapi.aliyuncs.com")
	cfg.AliyunSmsCountryCode = getEnv("ALIYUN_SMS_COUNTRY_CODE", "86")
	cfg.AliyunSmsDailyMaxPhone = getEnvAsInt("ALIYUN_SMS_DAILY_MAX_PHONE", 20)
	cfg.AliyunSmsDailyMaxIP = getEnvAsInt("ALIYUN_SMS_DAILY_MAX_IP", 200)
	cfg.AliyunSmsDailyMaxDevice = getEnvAsInt("ALIYUN_SMS_DAILY_MAX_DEVICE", 60)
	cfg.AliyunSmsDailyMaxUniquePhonePerIP = getEnvAsInt("ALIYUN_SMS_DAILY_MAX_UNIQUE_PHONE_PER_IP", 30)
	cfg.AliyunSmsDailyMaxUniquePhonePerDevice = getEnvAsInt("ALIYUN_SMS_DAILY_MAX_UNIQUE_PHONE_PER_DEVICE", 20)
	cfg.LoginDailyMaxPhone = getEnvAsInt("LOGIN_DAILY_MAX_PHONE", 50)
	cfg.LoginDailyMaxIP = getEnvAsInt("LOGIN_DAILY_MAX_IP", 300)
	cfg.LoginDailyMaxDevice = getEnvAsInt("LOGIN_DAILY_MAX_DEVICE", 120)
	cfg.DevAuthEnabled = getEnvAsBool("DEV_AUTH_ENABLED", false)
	cfg.DevAuthPhone = getEnv("DEV_AUTH_PHONE", "")
	cfg.DevAuthCode = getEnv("DEV_AUTH_CODE", "")
	cfg.CaptchaTTLSeconds = getEnvAsInt("CAPTCHA_TTL_SECONDS", 300)
	cfg.CaptchaLength = getEnvAsInt("CAPTCHA_LENGTH", 4)
	cfg.RegisterDailyMaxDevice = getEnvAsInt("REGISTER_DAILY_MAX_DEVICE", 3)
	cfg.RegisterDailyMaxIP = getEnvAsInt("REGISTER_DAILY_MAX_IP", 20)
	cfg.AuthFailWindowSeconds = getEnvAsInt("AUTH_FAIL_WINDOW_SECONDS", 600)
	cfg.AuthFailLockLevel1 = getEnvAsInt("AUTH_FAIL_LOCK_LEVEL1", 5)
	cfg.AuthFailLockLevel2 = getEnvAsInt("AUTH_FAIL_LOCK_LEVEL2", 12)
	cfg.AuthFailLockTTL1 = getEnvAsInt("AUTH_FAIL_LOCK_TTL1", 600)
	cfg.AuthFailLockTTL2 = getEnvAsInt("AUTH_FAIL_LOCK_TTL2", 3600)

	cfg.DownloadEmojiPerMinuteUser = getEnvAsInt("DOWNLOAD_EMOJI_PER_MINUTE_USER", 120)
	cfg.DownloadEmojiPerMinuteIP = getEnvAsInt("DOWNLOAD_EMOJI_PER_MINUTE_IP", 300)
	cfg.DownloadEmojiPerHourUser = getEnvAsInt("DOWNLOAD_EMOJI_PER_HOUR_USER", 1200)
	cfg.DownloadEmojiPerHourIP = getEnvAsInt("DOWNLOAD_EMOJI_PER_HOUR_IP", 3000)

	cfg.DownloadCollectionPerHourUser = getEnvAsInt("DOWNLOAD_COLLECTION_PER_HOUR_USER", 20)
	cfg.DownloadCollectionPerHourIP = getEnvAsInt("DOWNLOAD_COLLECTION_PER_HOUR_IP", 60)
	cfg.DownloadCollectionPerDayUser = getEnvAsInt("DOWNLOAD_COLLECTION_PER_DAY_USER", 120)
	cfg.DownloadCollectionPerDayIP = getEnvAsInt("DOWNLOAD_COLLECTION_PER_DAY_IP", 400)

	cfg.RedeemValidatePer10MinUser = getEnvAsInt("REDEEM_VALIDATE_PER_10MIN_USER", 30)
	cfg.RedeemValidatePer10MinIP = getEnvAsInt("REDEEM_VALIDATE_PER_10MIN_IP", 80)
	cfg.RedeemSubmitPer10MinUser = getEnvAsInt("REDEEM_SUBMIT_PER_10MIN_USER", 10)
	cfg.RedeemSubmitPer10MinIP = getEnvAsInt("REDEEM_SUBMIT_PER_10MIN_IP", 30)
	cfg.DownloadTicketTTL = getEnvAsInt("DOWNLOAD_TICKET_TTL", 120)
	cfg.DownloadTicketSignTTL = getEnvAsInt("DOWNLOAD_TICKET_SIGN_TTL", 180)
	cfg.DownloadTicketBindIP = getEnvAsBool("DOWNLOAD_TICKET_BIND_IP", true)
	cfg.DownloadTicketBindUA = getEnvAsBool("DOWNLOAD_TICKET_BIND_UA", true)
	cfg.RiskAutoBlockEnabled = getEnvAsBool("RISK_AUTO_BLOCK_ENABLED", true)
	cfg.RiskAutoBlockThreshold = getEnvAsInt("RISK_AUTO_BLOCK_THRESHOLD", 10)
	cfg.RiskAutoBlockWindowSeconds = getEnvAsInt("RISK_AUTO_BLOCK_WINDOW_SECONDS", 600)
	cfg.RiskAutoBlockDurationSec = getEnvAsInt("RISK_AUTO_BLOCK_DURATION_SECONDS", 86400)

	cfg.TelegramBotToken = getEnv("TELEGRAM_BOT_TOKEN", "")
	cfg.TelegramDownloadDir = getEnv("TELEGRAM_DOWNLOAD_DIR", "/Users/mac/go/src/emoji/telegram_to_wechat")
	cfg.TelegramProxy = getEnv("TELEGRAM_PROXY", "")

	cfg.AuthAccessCookieName = getEnv("AUTH_ACCESS_COOKIE_NAME", "emoji_access_token")
	cfg.AuthRefreshCookieName = getEnv("AUTH_REFRESH_COOKIE_NAME", "emoji_refresh_token")
	cfg.AuthCookieDomain = getEnv("AUTH_COOKIE_DOMAIN", "")
	cfg.AuthCookieSecure = getEnvAsBool("AUTH_COOKIE_SECURE", cfg.Env == "prod")
	cfg.AuthCookieSameSite = getEnv("AUTH_COOKIE_SAMESITE", "lax")

	cfg.CORSAllowOrigins = getEnvAsSlice("CORS_ALLOW_ORIGINS", []string{
		"http://localhost:5818",
		"http://127.0.0.1:5818",
		"http://localhost:5918",
		"http://127.0.0.1:5918",
	})

	// Aliyun OSS
	cfg.OSSEndpoint = getEnv("OSS_ENDPOINT", "")
	cfg.OSSAccessKeyID = getEnv("OSS_ACCESS_KEY_ID", cfg.AliyunAccessKeyId)
	cfg.OSSAccessKeySecret = getEnv("OSS_ACCESS_KEY_SECRET", cfg.AliyunAccessKeySecret)
	cfg.OSSBucket = getEnv("OSS_BUCKET", "")
	cfg.OSSRegion = getEnv("OSS_REGION", "cn-hangzhou")
	cfg.OSSBaseURL = getEnv("OSS_BASE_URL", "")

	// Claude API (legacy, used as fallback for LLM_*)
	cfg.ClaudeAPIKey = getEnv("CLAUDE_API_KEY", "")
	cfg.ClaudeModel = getEnv("CLAUDE_MODEL", "claude-sonnet-4-20250514")
	cfg.ClaudeEndpoint = getEnv("CLAUDE_ENDPOINT", "https://api.anthropic.com")

	// LLM provider (preferred — falls back to Claude* if not set)
	cfg.LLMProvider = getEnv("LLM_PROVIDER", "claude")
	cfg.LLMAPIKey = getEnv("LLM_API_KEY", cfg.ClaudeAPIKey)
	cfg.LLMModel = getEnv("LLM_MODEL", cfg.ClaudeModel)
	cfg.LLMEndpoint = getEnv("LLM_ENDPOINT", cfg.ClaudeEndpoint)

	cfg.AIPlannerEnabled = getEnvAsBool("AI_PLANNER_ENABLED", true)
	cfg.AIPlannerProvider = getEnv("AI_PLANNER_PROVIDER", "qwen")
	cfg.AIPlannerAPIKey = getEnv("AI_PLANNER_API_KEY", cfg.LLMAPIKey)
	cfg.AIPlannerModel = getEnv("AI_PLANNER_MODEL", "qwen3-vl-flash")
	cfg.AIPlannerEndpoint = getEnv("AI_PLANNER_ENDPOINT", "https://dashscope.aliyuncs.com/compatible-mode")
	cfg.AIPlannerPromptVersion = getEnv("AI_PLANNER_PROMPT_VERSION", "gif_planner_v1")
	cfg.AIPlannerTimeoutSec = getEnvAsInt("AI_PLANNER_TIMEOUT_SECONDS", 20)
	cfg.AIPlannerMaxTokens = getEnvAsInt("AI_PLANNER_MAX_TOKENS", 1200)

	cfg.AIDirectorEnabled = getEnvAsBool("AI_DIRECTOR_ENABLED", true)
	cfg.AIDirectorProvider = getEnv("AI_DIRECTOR_PROVIDER", "qwen")
	cfg.AIDirectorAPIKey = getEnv("AI_DIRECTOR_API_KEY", cfg.AIPlannerAPIKey)
	cfg.AIDirectorModel = getEnv("AI_DIRECTOR_MODEL", "qwen3-vl-flash")
	cfg.AIDirectorEndpoint = getEnv("AI_DIRECTOR_ENDPOINT", cfg.AIPlannerEndpoint)
	cfg.AIDirectorPromptVersion = getEnv("AI_DIRECTOR_PROMPT_VERSION", "gif_director_v1")
	cfg.AIDirectorTimeoutSec = getEnvAsInt("AI_DIRECTOR_TIMEOUT_SECONDS", 16)
	cfg.AIDirectorMaxTokens = getEnvAsInt("AI_DIRECTOR_MAX_TOKENS", 1000)

	cfg.AIJudgeEnabled = getEnvAsBool("AI_JUDGE_ENABLED", true)
	cfg.AIJudgeProvider = getEnv("AI_JUDGE_PROVIDER", "deepseek")
	cfg.AIJudgeAPIKey = getEnv("AI_JUDGE_API_KEY", cfg.LLMAPIKey)
	cfg.AIJudgeModel = getEnv("AI_JUDGE_MODEL", "deepseek-chat")
	cfg.AIJudgeEndpoint = getEnv("AI_JUDGE_ENDPOINT", "https://api.deepseek.com")
	cfg.AIJudgePromptVersion = getEnv("AI_JUDGE_PROMPT_VERSION", "gif_judge_v1")
	cfg.AIJudgeTimeoutSec = getEnvAsInt("AI_JUDGE_TIMEOUT_SECONDS", 45)
	cfg.AIJudgeMaxTokens = getEnvAsInt("AI_JUDGE_MAX_TOKENS", 1400)

	// Font
	cfg.FontPath = getEnv("FONT_PATH", "assets/fonts/NotoSansSC-Bold.ttf")

	return cfg
}

func getEnv(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}

func getEnvAsInt(key string, def int) int {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return i
}

func getEnvAsBool(key string, def bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return def
	}
	return b
}

func getEnvAsDuration(key string, def time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return def
	}
	return d
}

func getEnvAsSlice(key string, def []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		val := strings.TrimSpace(part)
		if val != "" {
			out = append(out, val)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}
