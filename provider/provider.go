package provider

import "context"

type AIResult struct {
	Text             string
	PromptTokens     int
	CompletionTokens int
	TotalCost        float64
}

type AIProvider interface {
	Name() string
	Generate(ctx context.Context, systemPrompt, audioBase64, mimeType string) (*AIResult, error)
}
