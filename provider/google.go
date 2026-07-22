package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type GooglePartInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type GooglePart struct {
	Text       string                `json:"text,omitempty"`
	InlineData *GooglePartInlineData `json:"inline_data,omitempty"`
}

type GoogleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GooglePart `json:"parts"`
}

type GoogleSystemInstruction struct {
	Parts []GooglePart `json:"parts"`
}

type GoogleRequest struct {
	SystemInstruction GoogleSystemInstruction `json:"system_instruction"`
	Contents          []GoogleContent          `json:"contents"`
}

type GoogleCandidate struct {
	Content GoogleContent `json:"content"`
}

type GoogleResponse struct {
	Candidates []GoogleCandidate `json:"candidates"`
}

type GoogleProvider struct {
	APIKey  string
	Model   string
	BaseURL string
	client  *http.Client
}

func NewGoogleProvider(apiKey, model string) *GoogleProvider {
	return &GoogleProvider{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://generativelanguage.googleapis.com",
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *GoogleProvider) SetHTTPClient(c *http.Client) {
	p.client = c
}

func (p *GoogleProvider) Name() string {
	return "Google Free Tier"
}

func (p *GoogleProvider) Generate(ctx context.Context, systemPrompt, audioBase64, mimeType string) (*AIResult, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("Google API key bulunamadı")
	}

	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", baseURL, p.Model, p.APIKey)

	reqBody := GoogleRequest{
		SystemInstruction: GoogleSystemInstruction{
			Parts: []GooglePart{
				{Text: systemPrompt},
			},
		},
		Contents: []GoogleContent{
			{
				Parts: []GooglePart{
					{Text: "İşle."},
					{
						InlineData: &GooglePartInlineData{
							MimeType: mimeType,
							Data:     audioBase64,
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
		req.Header.Set("Content-Type", "application/json")

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				continue
			}
			return nil, lastErr
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		var resData GoogleResponse
		if err := json.Unmarshal(body, &resData); err != nil {
			return nil, err
		}

		if len(resData.Candidates) == 0 || len(resData.Candidates[0].Content.Parts) == 0 {
			return nil, fmt.Errorf("Yanıtta geçerli içerik/part bulunamadı")
		}

		return &AIResult{
			Text:      resData.Candidates[0].Content.Parts[0].Text,
			TotalCost: 0.0,
		}, nil
	}

	return nil, lastErr
}

func CallGoogleAPI(apiKey, model, systemPrompt, base64Audio, mimeType string) (string, error) {
	p := NewGoogleProvider(apiKey, model)
	res, err := p.Generate(context.Background(), systemPrompt, base64Audio, mimeType)
	if err != nil {
		return "", err
	}
	return res.Text, nil
}
