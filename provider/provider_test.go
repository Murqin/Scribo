package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleProvider_Generate_Success(t *testing.T) {
	mockResp := `{
		"candidates": [
			{
				"content": {
					"parts": [
						{"text": "mocked gemini summary"}
					]
				}
			}
		]
	}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResp))
	}))
	defer ts.Close()

	gp := NewGoogleProvider("test_key", "gemini-3.6-flash")
	gp.BaseURL = ts.URL
	gp.SetHTTPClient(ts.Client())

	res, err := gp.Generate(context.Background(), "test prompt", "base64audio", "audio/ogg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Text != "mocked gemini summary" {
		t.Errorf("expected 'mocked gemini summary', got %s", res.Text)
	}
}

func TestGoogleProvider_Generate_EmptyKey(t *testing.T) {
	gp := NewGoogleProvider("", "gemini-3.6-flash")
	_, err := gp.Generate(context.Background(), "prompt", "audio", "audio/ogg")
	if err == nil {
		t.Error("expected error for empty API key, got nil")
	}
}

func TestOpenRouterProvider_Generate_Success(t *testing.T) {
	mockResp := `{
		"choices": [
			{
				"message": {
					"content": "mocked openrouter summary"
				}
			}
		],
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 50,
			"total_tokens": 150
		}
	}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResp))
	}))
	defer ts.Close()

	op := NewOpenRouterProvider("test_key", "google/gemini-3.6-flash")
	op.BaseURL = ts.URL
	op.SetHTTPClient(ts.Client())

	res, err := op.Generate(context.Background(), "test prompt", "base64audio", "audio/ogg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Text != "mocked openrouter summary" {
		t.Errorf("expected 'mocked openrouter summary', got %s", res.Text)
	}

	if res.PromptTokens != 100 || res.CompletionTokens != 50 {
		t.Errorf("expected 100/50 tokens, got %d/%d", res.PromptTokens, res.CompletionTokens)
	}
}

func TestOpenRouterProvider_Generate_SafeMIME(t *testing.T) {
	mockResp := `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResp))
	}))
	defer ts.Close()

	op := NewOpenRouterProvider("test_key", "model")
	op.BaseURL = ts.URL
	op.SetHTTPClient(ts.Client())

	// Test MIME type without slash (should not panic)
	res, err := op.Generate(context.Background(), "prompt", "audio", "ogg")
	if err != nil {
		t.Fatalf("expected safe MIME split without panic, got error: %v", err)
	}
	if res.Text != "ok" {
		t.Errorf("expected ok, got %s", res.Text)
	}
}
