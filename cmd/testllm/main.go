package main

import (
	"context"
	"fmt"
	"log"

	"emoji/internal/config"
	"emoji/internal/service"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Overload("/Users/mac/go/src/emoji/backend/.env")

	cfg := config.Load()
	fmt.Printf("Provider : %s\n", cfg.LLMProvider)
	fmt.Printf("Model    : %s\n", cfg.LLMModel)
	fmt.Printf("Endpoint : %s\n", cfg.LLMEndpoint)
	if len(cfg.LLMAPIKey) > 8 {
		fmt.Printf("APIKey   : %s...\n\n", cfg.LLMAPIKey[:8])
	}

	provider := service.NewLLMProvider(cfg.LLMProvider, cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMEndpoint)
	fmt.Printf("Using    : %s\n\n", provider.Name())

	resp, err := provider.Chat(
		context.Background(),
		"你是一个梗文案专家，只返回JSON，不要任何多余文字。",
		`我今天被老板骂了，帮我匹配一条梗文案。返回格式：{"phrase_id":1,"phrase":"文案内容","template_category":"愤怒"}`,
		300,
	)
	if err != nil {
		log.Fatalf("FAIL: %v", err)
	}
	fmt.Printf("Response:\n%s\n", resp)
}
