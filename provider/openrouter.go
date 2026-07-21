package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type Pricing struct {
	Prompt     float64 `json:"prompt"`
	Completion float64 `json:"completion"`
}

var (
	pricesCache = make(map[string]Pricing)
	cacheMutex  sync.RWMutex
)

func GetDynamicPricing(ctx context.Context, modelID string) Pricing {
	cacheMutex.RLock()
	if p, ok := pricesCache[modelID]; ok {
		cacheMutex.RUnlock()
		return p
	}
	cacheMutex.RUnlock()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/models", nil)
	if err == nil {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var result struct {
				Data []struct {
					ID      string  `json:"id"`
					Pricing Pricing `json:"pricing"`
				} `json:"data"`
			}
			if json.Unmarshal(body, &result) == nil {
				for _, m := range result.Data {
					if m.ID == modelID {
						cacheMutex.Lock()
						pricesCache[modelID] = m.Pricing
						cacheMutex.Unlock()
						return m.Pricing
					}
				}
			}
		}
	}

	return Pricing{Prompt: 0.0000015, Completion: 0.000009}
}

type OpenRouterContentAudioItem struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

type OpenRouterContentItem struct {
	Type       string                      `json:"type"`
	Text       string                      `json:"text,omitempty"`
	InputAudio *OpenRouterContentAudioItem `json:"input_audio,omitempty"`
}

type OpenRouterMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type OpenRouterRequest struct {
	Model    string              `json:"model"`
	Messages []OpenRouterMessage `json:"messages"`
}

type OpenRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type OpenRouterProvider struct {
	APIKey string
	Model  string
}

func NewOpenRouterProvider(apiKey, model string) *OpenRouterProvider {
	return &OpenRouterProvider{APIKey: apiKey, Model: model}
}

func (p *OpenRouterProvider) Name() string {
	return "OpenRouter API"
}

func (p *OpenRouterProvider) Generate(ctx context.Context, systemPrompt, audioBase64 string) (*AIResult, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("OpenRouter API key bulunamadı")
	}

	priceChan := make(chan Pricing, 1)
	go func() {
		priceChan <- GetDynamicPricing(ctx, p.Model)
	}()

	url := "https://openrouter.ai/api/v1/chat/completions"

	reqBody := OpenRouterRequest{
		Model: p.Model,
		Messages: []OpenRouterMessage{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role: "user",
				Content: []OpenRouterContentItem{
					{Type: "text", Text: "İşle."},
					{
						Type: "input_audio",
						InputAudio: &OpenRouterContentAudioItem{
							Data:   audioBase64,
							Format: "ogg",
						},
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var resData OpenRouterResponse
	if err := json.Unmarshal(body, &resData); err != nil {
		return nil, err
	}

	if len(resData.Choices) == 0 {
		return nil, fmt.Errorf("OpenRouter yanıtında geçerli seçim bulunamadı")
	}

	pricing := <-priceChan
	pTokens := resData.Usage.PromptTokens
	cTokens := resData.Usage.CompletionTokens
	cost := (float64(pTokens) * pricing.Prompt) + (float64(cTokens) * pricing.Completion)

	return &AIResult{
		Text:             resData.Choices[0].Message.Content,
		PromptTokens:     pTokens,
		CompletionTokens: cTokens,
		TotalCost:        cost,
	}, nil
}

type OpenRouterResult = AIResult

func CallOpenRouterAPI(apiKey, model, systemPrompt, base64Audio string) (*AIResult, error) {
	p := NewOpenRouterProvider(apiKey, model)
	return p.Generate(context.Background(), systemPrompt, base64Audio)
}
