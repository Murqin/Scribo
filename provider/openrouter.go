package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Pricing struct {
	Prompt     float64 `json:"prompt"`
	Completion float64 `json:"completion"`
}

type cachedPricing struct {
	pricing Pricing
	expiry  time.Time
}

var (
	pricesCache  = make(map[string]cachedPricing)
	cacheMutex   sync.RWMutex
	cacheTTL     = time.Hour
	sharedClient = &http.Client{Timeout: 15 * time.Second}
)

func ResetPricingCache() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	pricesCache = make(map[string]cachedPricing)
}

func (p *OpenRouterProvider) GetDynamicPricing(ctx context.Context, modelID string) Pricing {
	cacheMutex.RLock()
	if c, ok := pricesCache[modelID]; ok && time.Now().Before(c.expiry) {
		cacheMutex.RUnlock()
		return c.pricing
	}
	cacheMutex.RUnlock()

	baseURL := "https://openrouter.ai/api/v1"
	client := sharedClient
	if p != nil {
		if p.BaseURL != "" {
			baseURL = p.BaseURL
		}
		if p.client != nil {
			client = p.client
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/models", nil)
	if err == nil {
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
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
							pricesCache[modelID] = cachedPricing{pricing: m.Pricing, expiry: time.Now().Add(cacheTTL)}
							cacheMutex.Unlock()
							return m.Pricing
						}
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
	APIKey  string
	Model   string
	BaseURL string
	client  *http.Client
}

func NewOpenRouterProvider(apiKey, model string) *OpenRouterProvider {
	return &OpenRouterProvider{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://openrouter.ai/api/v1",
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *OpenRouterProvider) SetHTTPClient(c *http.Client) {
	p.client = c
}

func (p *OpenRouterProvider) Name() string {
	return "OpenRouter API"
}

func (p *OpenRouterProvider) Generate(ctx context.Context, systemPrompt, audioBase64, mimeType string) (*AIResult, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("OpenRouter API key bulunamadı")
	}

	audioFormat := "ogg"
	parts := strings.SplitN(mimeType, "/", 2)
	if len(parts) == 2 && parts[1] != "" {
		audioFormat = parts[1]
	}

	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	url := baseURL + "/chat/completions"

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
							Format: audioFormat,
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

	maxRetries := 2
	backoff := 500 * time.Millisecond

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+p.APIKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				continue
			}
			return nil, lastErr
		}

		var resData OpenRouterResponse
		if err := json.Unmarshal(body, &resData); err != nil {
			return nil, err
		}

		if len(resData.Choices) == 0 {
			return nil, fmt.Errorf("OpenRouter yanıtında geçerli seçim bulunamadı")
		}

		pricing := p.GetDynamicPricing(ctx, p.Model)
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

	return nil, lastErr
}

type OpenRouterResult = AIResult

func CallOpenRouterAPI(apiKey, model, systemPrompt, base64Audio, mimeType string) (*AIResult, error) {
	p := NewOpenRouterProvider(apiKey, model)
	return p.Generate(context.Background(), systemPrompt, base64Audio, mimeType)
}

