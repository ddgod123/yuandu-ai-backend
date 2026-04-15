package videojobs

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

const (
	videoAIPromptTemplateCacheTTLEnv        = "VIDEO_AI_PROMPT_TEMPLATE_CACHE_TTL_SECONDS"
	videoAIPromptTemplateCacheDebounceMsEnv = "VIDEO_AI_PROMPT_TEMPLATE_CACHE_DEBOUNCE_MS"
)

type videoAIPromptTemplateCacheEntry struct {
	Snapshot aiPromptTemplateSnapshot
	ExpireAt time.Time
}

var videoAIPromptTemplateRuntimeCache = struct {
	mu            sync.RWMutex
	entries       map[string]videoAIPromptTemplateCacheEntry
	lastProbeAt   time.Time
	lastProbeSign string
}{
	entries: make(map[string]videoAIPromptTemplateCacheEntry),
}

var (
	videoAIPromptTemplateCacheTTLOne        sync.Once
	videoAIPromptTemplateCacheTTLValue      time.Duration
	videoAIPromptTemplateCacheDebounceOne   sync.Once
	videoAIPromptTemplateCacheDebounceValue time.Duration
)

func loadVideoAIPromptTemplateCacheTTL() time.Duration {
	videoAIPromptTemplateCacheTTLOne.Do(func() {
		value := 20 * time.Second
		raw := strings.TrimSpace(os.Getenv(videoAIPromptTemplateCacheTTLEnv))
		if raw != "" {
			if sec, err := strconv.Atoi(raw); err == nil && sec > 0 && sec <= 3600 {
				value = time.Duration(sec) * time.Second
			}
		}
		videoAIPromptTemplateCacheTTLValue = value
	})
	if videoAIPromptTemplateCacheTTLValue <= 0 {
		return 20 * time.Second
	}
	return videoAIPromptTemplateCacheTTLValue
}

func loadVideoAIPromptTemplateCacheDebounce() time.Duration {
	videoAIPromptTemplateCacheDebounceOne.Do(func() {
		value := 1500 * time.Millisecond
		raw := strings.TrimSpace(os.Getenv(videoAIPromptTemplateCacheDebounceMsEnv))
		if raw != "" {
			if ms, err := strconv.Atoi(raw); err == nil && ms > 0 && ms <= 60000 {
				value = time.Duration(ms) * time.Millisecond
			}
		}
		videoAIPromptTemplateCacheDebounceValue = value
	})
	if videoAIPromptTemplateCacheDebounceValue <= 0 {
		return 1500 * time.Millisecond
	}
	return videoAIPromptTemplateCacheDebounceValue
}

func videoAIPromptTemplateCacheKey(format, stage, layer string) string {
	format = normalizeAIPromptTemplateFormat(format)
	stage = strings.ToLower(strings.TrimSpace(stage))
	layer = strings.ToLower(strings.TrimSpace(layer))
	if stage == "" || layer == "" {
		return ""
	}
	return format + "|" + stage + "|" + layer
}

func getCachedVideoAIPromptTemplateSnapshot(format, stage, layer string) (aiPromptTemplateSnapshot, bool) {
	key := videoAIPromptTemplateCacheKey(format, stage, layer)
	if key == "" {
		return aiPromptTemplateSnapshot{}, false
	}
	now := time.Now()
	videoAIPromptTemplateRuntimeCache.mu.RLock()
	entry, ok := videoAIPromptTemplateRuntimeCache.entries[key]
	videoAIPromptTemplateRuntimeCache.mu.RUnlock()
	if !ok {
		return aiPromptTemplateSnapshot{}, false
	}
	if !entry.ExpireAt.IsZero() && now.After(entry.ExpireAt) {
		videoAIPromptTemplateRuntimeCache.mu.Lock()
		current, exists := videoAIPromptTemplateRuntimeCache.entries[key]
		if exists && current.ExpireAt.Equal(entry.ExpireAt) {
			delete(videoAIPromptTemplateRuntimeCache.entries, key)
		}
		videoAIPromptTemplateRuntimeCache.mu.Unlock()
		return aiPromptTemplateSnapshot{}, false
	}
	return entry.Snapshot, true
}

func putCachedVideoAIPromptTemplateSnapshot(format, stage, layer string, snapshot aiPromptTemplateSnapshot) {
	key := videoAIPromptTemplateCacheKey(format, stage, layer)
	if key == "" {
		return
	}
	ttl := loadVideoAIPromptTemplateCacheTTL()
	videoAIPromptTemplateRuntimeCache.mu.Lock()
	videoAIPromptTemplateRuntimeCache.entries[key] = videoAIPromptTemplateCacheEntry{
		Snapshot: snapshot,
		ExpireAt: time.Now().Add(ttl),
	}
	videoAIPromptTemplateRuntimeCache.mu.Unlock()
}

func InvalidateVideoAIPromptTemplateCache() {
	videoAIPromptTemplateRuntimeCache.mu.Lock()
	videoAIPromptTemplateRuntimeCache.entries = make(map[string]videoAIPromptTemplateCacheEntry)
	videoAIPromptTemplateRuntimeCache.lastProbeSign = ""
	videoAIPromptTemplateRuntimeCache.lastProbeAt = time.Time{}
	videoAIPromptTemplateRuntimeCache.mu.Unlock()
}

func InvalidateVideoAIPromptTemplateCacheBySlot(format, stage, layer string) {
	key := videoAIPromptTemplateCacheKey(format, stage, layer)
	if key == "" {
		InvalidateVideoAIPromptTemplateCache()
		return
	}
	videoAIPromptTemplateRuntimeCache.mu.Lock()
	delete(videoAIPromptTemplateRuntimeCache.entries, key)
	videoAIPromptTemplateRuntimeCache.mu.Unlock()
}

func detectVideoAIPromptTemplateChange(db *gorm.DB) {
	if db == nil {
		return
	}
	now := time.Now()
	debounce := loadVideoAIPromptTemplateCacheDebounce()
	videoAIPromptTemplateRuntimeCache.mu.RLock()
	lastProbeAt := videoAIPromptTemplateRuntimeCache.lastProbeAt
	videoAIPromptTemplateRuntimeCache.mu.RUnlock()
	if !lastProbeAt.IsZero() && now.Sub(lastProbeAt) < debounce {
		return
	}

	videoAIPromptTemplateRuntimeCache.mu.Lock()
	if !videoAIPromptTemplateRuntimeCache.lastProbeAt.IsZero() && now.Sub(videoAIPromptTemplateRuntimeCache.lastProbeAt) < debounce {
		videoAIPromptTemplateRuntimeCache.mu.Unlock()
		return
	}
	videoAIPromptTemplateRuntimeCache.lastProbeAt = now
	prevSign := videoAIPromptTemplateRuntimeCache.lastProbeSign
	videoAIPromptTemplateRuntimeCache.mu.Unlock()

	var row struct {
		Count         int64       `gorm:"column:count"`
		MaxUpdatedRaw interface{} `gorm:"column:max_updated_at"`
	}
	err := db.Raw(`
SELECT
  COUNT(*) AS count,
  MAX(updated_at) AS max_updated_at
FROM ops.video_ai_prompt_templates
`).Scan(&row).Error
	if err != nil {
		if isMissingTableError(err, "video_ai_prompt_templates") {
			return
		}
		return
	}
	sign := fmt.Sprintf("%d|", row.Count)
	if row.MaxUpdatedRaw != nil {
		sign += strings.TrimSpace(fmt.Sprint(row.MaxUpdatedRaw))
	}

	videoAIPromptTemplateRuntimeCache.mu.Lock()
	defer videoAIPromptTemplateRuntimeCache.mu.Unlock()
	if prevSign != "" && sign != prevSign {
		videoAIPromptTemplateRuntimeCache.entries = make(map[string]videoAIPromptTemplateCacheEntry)
	}
	videoAIPromptTemplateRuntimeCache.lastProbeSign = sign
}
