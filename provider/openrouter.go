package provider

import (
	"bytes"
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

func GetDynamicPricing(modelID string) Pricing {
	cacheMutex.RLock()
	if p, ok := pricesCache[modelID]; ok {
		cacheMutex.RUnlock()
		return p
	}
	cacheMutex.RUnlock()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://openrouter.ai/api/v1/models")
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

	fallback := Pricing{Prompt: 0.0000015, Completion: 0.000009}
	return fallback
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

type OpenRouterResult struct {
	Text             string
	PromptTokens     int
	CompletionTokens int
	TotalCost        float64
}

func CallOpenRouterAPI(apiKey, model, systemPrompt, base64Audio string) (*OpenRouterResult, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OpenRouter API key bulunamadı")
	}

	priceChan := make(chan Pricing, 1)
	go func() {
		priceChan <- GetDynamicPricing(model)
	}()

	url := "https://openrouter.ai/api/v1/chat/completions"

	reqBody := OpenRouterRequest{
		Model: model,
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
							Data:   base64Audio,
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

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 35 * time.Second}
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

	return &OpenRouterResult{
		Text:             resData.Choices[0].Message.Content,
		PromptTokens:     pTokens,
		CompletionTokens: cTokens,
		TotalCost:        cost,
	}, nil
}
