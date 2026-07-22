package provider

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

	if gp.Name() != "Google Free Tier" {
		t.Errorf("expected name 'Google Free Tier', got %s", gp.Name())
	}

	res, err := gp.Generate(context.Background(), "test prompt", "base64audio", "audio/ogg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Text != "mocked gemini summary" {
		t.Errorf("expected 'mocked gemini summary', got %s", res.Text)
	}

	if res.TotalCost != 0.0 {
		t.Errorf("expected zero cost for Google Free Tier, got %f", res.TotalCost)
	}
}

func TestGoogleProvider_Generate_RateLimitRetry(t *testing.T) {
	var attempts int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "rate limit"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"success after retry"}]}}]}`))
	}))
	defer ts.Close()

	gp := NewGoogleProvider("test_key", "gemini-3.6-flash")
	gp.BaseURL = ts.URL
	gp.SetHTTPClient(ts.Client())

	res, err := gp.Generate(context.Background(), "prompt", "audio", "audio/ogg")
	if err != nil {
		t.Fatalf("expected success on retry, got error: %v", err)
	}

	if res.Text != "success after retry" {
		t.Errorf("expected 'success after retry', got %s", res.Text)
	}

	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestGoogleProvider_Generate_NonRetryableError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer ts.Close()

	gp := NewGoogleProvider("test_key", "gemini-3.6-flash")
	gp.BaseURL = ts.URL
	gp.SetHTTPClient(ts.Client())

	_, err := gp.Generate(context.Background(), "prompt", "audio", "audio/ogg")
	if err == nil {
		t.Error("expected error on 400 Bad Request, got nil")
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
	mockModelsResp := `{
		"data": [
			{
				"id": "google/gemini-3.6-flash",
				"pricing": {
					"prompt": 0.000001,
					"completion": 0.000002
				}
			}
		]
	}`

	mockCompletionResp := `{
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
		if r.URL.Path == "/models" {
			w.Write([]byte(mockModelsResp))
		} else {
			w.Write([]byte(mockCompletionResp))
		}
	}))
	defer ts.Close()

	op := NewOpenRouterProvider("test_key", "google/gemini-3.6-flash")
	op.BaseURL = ts.URL
	op.SetHTTPClient(ts.Client())

	if op.Name() != "OpenRouter API" {
		t.Errorf("expected name 'OpenRouter API', got %s", op.Name())
	}

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

	expectedCost := (100.0 * 0.000001) + (50.0 * 0.000002)
	if math.Abs(res.TotalCost-expectedCost) > 1e-7 {
		t.Errorf("expected TotalCost %f, got %f", expectedCost, res.TotalCost)
	}
}

func TestOpenRouterProvider_Generate_EmptyKey(t *testing.T) {
	op := NewOpenRouterProvider("", "model")
	_, err := op.Generate(context.Background(), "prompt", "audio", "audio/ogg")
	if err == nil {
		t.Error("expected error for empty API key, got nil")
	}
}

func TestOpenRouterProvider_GetDynamicPricing_Cache(t *testing.T) {
	t.Cleanup(func() {
		ResetPricingCache()
	})

	var callCount int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[{"id":"cached-model","pricing":{"prompt":0.005,"completion":0.01}}]}`))
	}))
	defer ts.Close()

	op := NewOpenRouterProvider("key", "cached-model")
	op.BaseURL = ts.URL
	op.SetHTTPClient(ts.Client())

	// Call 1: Fetch pricing
	p1 := op.GetDynamicPricing(context.Background(), "cached-model")
	if p1.Prompt != 0.005 || p1.Completion != 0.01 {
		t.Errorf("unexpected pricing 1: %+v", p1)
	}

	// Call 2: Cache hit (should not call server again)
	p2 := op.GetDynamicPricing(context.Background(), "cached-model")
	if p2 != p1 {
		t.Errorf("pricing 2 != pricing 1: %+v vs %+v", p2, p1)
	}

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 HTTP call due to cache, got %d", callCount)
	}
}

func TestGoogleProvider_Generate_LargeErrorBodyCapped(t *testing.T) {
	largeErrBody := make([]byte, 10000)
	for i := range largeErrBody {
		largeErrBody[i] = 'A'
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(largeErrBody)
	}))
	defer ts.Close()

	gp := NewGoogleProvider("test_key", "gemini-3.6-flash")
	gp.BaseURL = ts.URL
	gp.SetHTTPClient(ts.Client())

	_, err := gp.Generate(context.Background(), "prompt", "audio", "audio/ogg")
	if err == nil {
		t.Fatal("expected error on 400 Bad Request, got nil")
	}

	// Length of "HTTP 400: " is 10 chars, capped body should be 4096, total max length 4106
	if len(err.Error()) > 4106 {
		t.Errorf("expected error message length <= 4106 bytes (capped to 4096 body), got %d bytes", len(err.Error()))
	}
}

func TestOpenRouterProvider_Generate_LargeErrorBodyCapped(t *testing.T) {
	largeErrBody := make([]byte, 10000)
	for i := range largeErrBody {
		largeErrBody[i] = 'B'
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(largeErrBody)
	}))
	defer ts.Close()

	op := NewOpenRouterProvider("test_key", "google/gemini-3.6-flash")
	op.BaseURL = ts.URL
	op.SetHTTPClient(ts.Client())

	_, err := op.Generate(context.Background(), "prompt", "audio", "audio/ogg")
	if err == nil {
		t.Fatal("expected error on 400 Bad Request, got nil")
	}

	if len(err.Error()) > 4106 {
		t.Errorf("expected error message length <= 4106 bytes (capped to 4096 body), got %d bytes", len(err.Error()))
	}
}
