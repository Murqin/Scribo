package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"scribo/i18n"
)

type GooglePartInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type GooglePart struct {
	Text       string                `json:"text,omitempty"`
	InlineData *GooglePartInlineData `json:"inline_data,omitempty"`
	// Thinking models may return reasoning parts alongside the answer; those must not
	// be concatenated into the user-visible text.
	Thought bool `json:"thought,omitempty"`
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
	Contents          []GoogleContent         `json:"contents"`
}

type GoogleCandidate struct {
	Content      GoogleContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type GooglePromptFeedback struct {
	BlockReason string `json:"blockReason"`
}

type GoogleUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
}

// PromptFeedback and UsageMetadata are pointers so an absent field stays nil and the
// provider behaves exactly as before rather than reporting a phantom failure.
type GoogleResponse struct {
	Candidates     []GoogleCandidate     `json:"candidates"`
	PromptFeedback *GooglePromptFeedback `json:"promptFeedback,omitempty"`
	UsageMetadata  *GoogleUsageMetadata  `json:"usageMetadata,omitempty"`
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
		return nil, fmt.Errorf("no Google API key configured")
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
					{Text: i18n.T("prompt.user_turn")},
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

		if resData.PromptFeedback != nil && resData.PromptFeedback.BlockReason != "" {
			return nil, fmt.Errorf("the request was blocked by a safety filter (%s)", resData.PromptFeedback.BlockReason)
		}

		if len(resData.Candidates) == 0 {
			return nil, fmt.Errorf("the response carried no candidate")
		}

		candidate := resData.Candidates[0]

		var sb strings.Builder
		for _, part := range candidate.Content.Parts {
			if part.Thought || part.Text == "" {
				continue
			}
			sb.WriteString(part.Text)
		}
		text := sb.String()

		if text == "" {
			if candidate.FinishReason != "" && candidate.FinishReason != "STOP" {
				return nil, fmt.Errorf("the model stopped without producing an answer (finishReason: %s)", candidate.FinishReason)
			}
			return nil, fmt.Errorf("the response carried no usable content part")
		}

		// A truncated answer is still worth delivering, but the user must know it is one.
		if candidate.FinishReason == "MAX_TOKENS" {
			text += i18n.T("prompt.truncated")
		}

		result := &AIResult{Text: text, TotalCost: 0.0}
		if resData.UsageMetadata != nil {
			result.PromptTokens = resData.UsageMetadata.PromptTokenCount
			result.CompletionTokens = resData.UsageMetadata.CandidatesTokenCount
		}
		return result, nil
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
