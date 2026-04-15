package queue

import (
	"emoji/internal/config"
	"emoji/internal/videojobs"
	"os"
	"strconv"
	"strings"

	"github.com/hibiken/asynq"
)

func NewClient(cfg config.Config) *asynq.Client {
	return asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.AsynqRedisAddr,
		Password: cfg.AsynqRedisPassword,
		DB:       cfg.AsynqRedisDB,
	})
}

func NewServer(cfg config.Config) *asynq.Server {
	concurrency := parseAsynqConcurrency(10)
	queues := ResolveAsynqQueueWeightsFromEnv()

	return asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.AsynqRedisAddr,
			Password: cfg.AsynqRedisPassword,
			DB:       cfg.AsynqRedisDB,
		},
		asynq.Config{
			Concurrency: concurrency,
			Queues:      queues,
		},
	)
}

func NewInspector(cfg config.Config) *asynq.Inspector {
	return asynq.NewInspector(asynq.RedisClientOpt{
		Addr:     cfg.AsynqRedisAddr,
		Password: cfg.AsynqRedisPassword,
		DB:       cfg.AsynqRedisDB,
	})
}

func ResolveAsynqQueueWeightsFromEnv() map[string]int {
	queues := defaultAsynqQueueWeights()
	if override := parseAsynqQueueWeightsEnv(strings.TrimSpace(os.Getenv("ASYNQ_QUEUE_WEIGHTS"))); len(override) > 0 {
		queues = override
	}
	role := resolveVideoWorkerRole(strings.TrimSpace(os.Getenv("VIDEO_WORKER_ROLE")))
	return applyVideoWorkerRoleFilter(queues, role)
}

func defaultAsynqQueueWeights() map[string]int {
	return map[string]int{
		"default":                    4,
		videojobs.QueueVideoJobGIF:   4,
		videojobs.QueueVideoJobPNG:   3,
		videojobs.QueueVideoJobJPG:   3,
		videojobs.QueueVideoJobWEBP:  3,
		videojobs.QueueVideoJobLIVE:  2,
		videojobs.QueueVideoJobMP4:   2,
		videojobs.QueueVideoJobMedia: 2,
		videojobs.QueueVideoJobAI:    2,
	}
}

func parseAsynqConcurrency(fallback int) int {
	raw := strings.TrimSpace(os.Getenv("ASYNQ_CONCURRENCY"))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func parseAsynqQueueWeightsEnv(raw string) map[string]int {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := map[string]int{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		kv := strings.SplitN(item, "=", 2)
		if len(kv) != 2 {
			continue
		}
		name := strings.TrimSpace(kv[0])
		if name == "" {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(kv[1]))
		if err != nil || value <= 0 {
			continue
		}
		out[name] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func resolveVideoWorkerRole(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "all", "default":
		return "all"
	case "gif":
		return "gif"
	case "png":
		return "png"
	case "jpg":
		return "jpg"
	case "webp":
		return "webp"
	case "live":
		return "live"
	case "mp4":
		return "mp4"
	case "ai":
		return "ai"
	case "image":
		return "image"
	case "media":
		return "media"
	default:
		return "all"
	}
}

func applyVideoWorkerRoleFilter(queues map[string]int, role string) map[string]int {
	role = resolveVideoWorkerRole(role)
	if role == "all" {
		return queues
	}

	pickMany := func(names ...string) map[string]int {
		out := make(map[string]int, len(names))
		for _, name := range names {
			if strings.TrimSpace(name) == "" {
				continue
			}
			weight := 1
			if queues != nil {
				if value, ok := queues[name]; ok && value > 0 {
					weight = value
				}
			}
			out[name] = weight
		}
		return out
	}

	switch role {
	case "gif":
		return pickMany(videojobs.QueueVideoJobGIF)
	case "png":
		return pickMany(videojobs.QueueVideoJobPNG)
	case "jpg":
		return pickMany(videojobs.QueueVideoJobJPG)
	case "webp":
		return pickMany(videojobs.QueueVideoJobWEBP)
	case "live":
		return pickMany(videojobs.QueueVideoJobLIVE)
	case "mp4":
		return pickMany(videojobs.QueueVideoJobMP4)
	case "ai":
		return pickMany(videojobs.QueueVideoJobAI)
	case "image":
		return pickMany(
			videojobs.QueueVideoJobPNG,
			videojobs.QueueVideoJobJPG,
			videojobs.QueueVideoJobWEBP,
			videojobs.QueueVideoJobLIVE,
			videojobs.QueueVideoJobMP4,
		)
	case "media":
		return pickMany(videojobs.QueueVideoJobMedia)
	default:
		return queues
	}
}
