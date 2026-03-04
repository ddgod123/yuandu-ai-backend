package handlers

import (
	"context"
	"errors"
	"time"

	"emoji/internal/config"

	"github.com/redis/go-redis/v9"
)

type SMSLimiter interface {
	AllowInterval(ctx context.Context, key string, interval time.Duration) (bool, error)
	AllowDaily(ctx context.Context, key string, limit int, ttl time.Duration) (bool, error)
}

type smsLimiter struct {
	redis *redis.Client
}

func newSMSLimiter(cfg config.Config) SMSLimiter {
	if cfg.RedisAddr == "" {
		return &smsLimiter{redis: nil}
	}
	return &smsLimiter{
		redis: redis.NewClient(&redis.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		}),
	}
}

func (l *smsLimiter) AllowInterval(ctx context.Context, key string, interval time.Duration) (bool, error) {
	if interval <= 0 {
		return true, nil
	}
	if l.redis == nil {
		return false, errors.New("redis not configured")
	}
	return l.redis.SetNX(ctx, key, "1", interval).Result()
}

func (l *smsLimiter) AllowDaily(ctx context.Context, key string, limit int, ttl time.Duration) (bool, error) {
	if limit <= 0 {
		return true, nil
	}
	if l.redis == nil {
		return false, errors.New("redis not configured")
	}
	val, err := l.redis.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if val == 1 && ttl > 0 {
		_, _ = l.redis.Expire(ctx, key, ttl).Result()
	}
	return val <= int64(limit), nil
}
