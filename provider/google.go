package provider

import (
	"bytes"
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

func CallGoogleAPI(apiKey, model, systemPrompt, base64Audio string) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("Google API key bulunamadı")
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)

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
							MimeType: "audio/ogg",
							Data:     base64Audio,
						},
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 35 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var resData GoogleResponse
	if err := json.Unmarshal(body, &resData); err != nil {
		return "", err
	}

	if len(resData.Candidates) == 0 || len(resData.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Yanıtta geçerli içerik/part bulunamadı")
	}

	return resData.Candidates[0].Content.Parts[0].Text, nil
}
