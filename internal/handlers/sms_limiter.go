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
	IncrWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error)
	SetValue(ctx context.Context, key string, value string, ttl time.Duration) error
	GetValue(ctx context.Context, key string) (string, error)
	ConsumeValue(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
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

func (l *smsLimiter) IncrWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if l.redis == nil {
		return 0, errors.New("redis not configured")
	}
	val, err := l.redis.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if val == 1 && ttl > 0 {
		_, _ = l.redis.Expire(ctx, key, ttl).Result()
	}
	return val, nil
}

func (l *smsLimiter) SetValue(ctx context.Context, key string, value string, ttl time.Duration) error {
	if l.redis == nil {
		return errors.New("redis not configured")
	}
	return l.redis.Set(ctx, key, value, ttl).Err()
}

func (l *smsLimiter) GetValue(ctx context.Context, key string) (string, error) {
	if l.redis == nil {
		return "", errors.New("redis not configured")
	}
	val, err := l.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (l *smsLimiter) Delete(ctx context.Context, key string) error {
	if l.redis == nil {
		return errors.New("redis not configured")
	}
	return l.redis.Del(ctx, key).Err()
}

func (l *smsLimiter) ConsumeValue(ctx context.Context, key string) (string, error) {
	if l.redis == nil {
		return "", errors.New("redis not configured")
	}
	val, err := l.redis.GetDel(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}
