package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"emoji/internal/config"
	"emoji/internal/models"

	"gorm.io/gorm"
)

// AIService handles meme phrase matching via any LLM provider.
type AIService struct {
	provider LLMProvider
	db       *gorm.DB
	mu       sync.RWMutex
	phrases  []models.MemePhrase
	lastLoad time.Time
}

func NewAIService(cfg config.Config, db *gorm.DB) *AIService {
	provider := NewLLMProvider(cfg.LLMProvider, cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMEndpoint)
	s := &AIService{provider: provider, db: db}
	s.refreshPhrases()
	return s
}

func (s *AIService) ProviderName() string {
	return s.provider.Name()
}

func (s *AIService) refreshPhrases() {
	var phrases []models.MemePhrase
	s.db.Where("status = ?", "active").Order("hot_score DESC").Find(&phrases)
	s.mu.Lock()
	s.phrases = phrases
	s.lastLoad = time.Now()
	s.mu.Unlock()
}

func (s *AIService) getPhrases() []models.MemePhrase {
	s.mu.RLock()
	age := time.Since(s.lastLoad)
	s.mu.RUnlock()
	if age > 5*time.Minute {
		s.refreshPhrases()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.phrases
}

// PLACEHOLDER_AI_1

type MatchResult struct {
	PhraseID         uint64 `json:"phrase_id"`
	Phrase           string `json:"phrase"`
	TemplateCategory string `json:"template_category"`
}

func (s *AIService) MatchPhrase(ctx context.Context, userInput string) (*MatchResult, error) {
	phrases := s.getPhrases()
	if len(phrases) == 0 {
		return nil, fmt.Errorf("no phrases available")
	}

	var sb strings.Builder
	for _, p := range phrases {
		sb.WriteString(fmt.Sprintf("- id:%d phrase:\"%s\" category:%s emotion:%s\n", p.ID, p.Phrase, p.Category, p.Emotion))
	}

	systemPrompt := fmt.Sprintf(`你是一个梗文案匹配专家。用户会输入一段文字描述他们的心情或场景，你需要从以下梗文案库中选择最匹配的一条。

可用梗文案列表：
%s

请返回JSON格式（不要包含markdown代码块标记）：
{"phrase_id": <id>, "phrase": "<匹配的文案>", "template_category": "<推荐的模板分类>"}

只返回JSON，不要其他文字。`, sb.String())

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	result, err := s.provider.Chat(ctx, systemPrompt, userInput, 200)
	if err != nil {
		return nil, fmt.Errorf("LLM (%s): %w", s.provider.Name(), err)
	}

	// Strip possible markdown code fences
	result = strings.TrimSpace(result)
	result = strings.TrimPrefix(result, "```json")
	result = strings.TrimPrefix(result, "```")
	result = strings.TrimSuffix(result, "```")
	result = strings.TrimSpace(result)

	var match MatchResult
	if err := json.Unmarshal([]byte(result), &match); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w (raw: %s)", err, result)
	}
	return &match, nil
}
