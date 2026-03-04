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

	QiniuAccessKey string
	QiniuSecretKey string
	QiniuBucket    string
	QiniuDomain    string
	QiniuZone      string
	QiniuUseHTTPS  bool
	QiniuUseCDN    bool
	QiniuPrivate   bool
	QiniuSignTTL   int

	AliyunAccessKeyId      string
	AliyunAccessKeySecret  string
	AliyunSmsSignName      string
	AliyunSmsTemplateCode  string
	AliyunSmsTemplateParam string
	AliyunSmsValidTime     int
	AliyunSmsInterval      int
	AliyunSmsReturnCode    bool
	AliyunSmsRegionId      string
	AliyunSmsEndpoint      string
	AliyunSmsCountryCode   string
	AliyunSmsDailyMaxPhone int
	AliyunSmsDailyMaxIP    int
	LoginDailyMaxPhone     int
	LoginDailyMaxIP        int
	DevAuthEnabled         bool
	DevAuthPhone           string
	DevAuthCode            string

	TelegramBotToken    string
	TelegramDownloadDir string
	TelegramProxy       string

	AuthAccessCookieName  string
	AuthRefreshCookieName string
	AuthCookieDomain      string
	AuthCookieSecure      bool
	AuthCookieSameSite    string

	CORSAllowOrigins []string
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
	cfg.RefreshTokenTTL = getEnvAsDuration("JWT_REFRESH_TTL", 168*time.Hour)

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

	cfg.QiniuAccessKey = getEnv("QINIU_ACCESS_KEY", "")
	cfg.QiniuSecretKey = getEnv("QINIU_SECRET_KEY", "")
	cfg.QiniuBucket = getEnv("QINIU_BUCKET", "")
	cfg.QiniuDomain = getEnv("QINIU_DOMAIN", "")
	cfg.QiniuZone = getEnv("QINIU_ZONE", "")
	cfg.QiniuUseHTTPS = getEnvAsBool("QINIU_USE_HTTPS", true)
	cfg.QiniuUseCDN = getEnvAsBool("QINIU_USE_CDN", false)
	cfg.QiniuPrivate = getEnvAsBool("QINIU_PRIVATE", false)
	cfg.QiniuSignTTL = getEnvAsInt("QINIU_SIGN_TTL", 3600)

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
	cfg.LoginDailyMaxPhone = getEnvAsInt("LOGIN_DAILY_MAX_PHONE", 50)
	cfg.LoginDailyMaxIP = getEnvAsInt("LOGIN_DAILY_MAX_IP", 300)
	cfg.DevAuthEnabled = getEnvAsBool("DEV_AUTH_ENABLED", false)
	cfg.DevAuthPhone = getEnv("DEV_AUTH_PHONE", "")
	cfg.DevAuthCode = getEnv("DEV_AUTH_CODE", "")

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
