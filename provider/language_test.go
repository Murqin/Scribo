package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"scribo/i18n"
)

// captureBody serves a minimal successful response and hands back whatever the
// provider posted, which is where the user turn lives.
func captureBody(t *testing.T, response string) (*httptest.Server, func() string) {
	t.Helper()

	var body string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// OpenRouter's pricing lookup hits the same server with a GET; only the
		// generation POST carries the request we are inspecting.
		if r.Method == http.MethodPost {
			raw, _ := io.ReadAll(r.Body)
			body = string(raw)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	}))
	t.Cleanup(ts.Close)

	return ts, func() string { return body }
}

func useLanguage(t *testing.T, lang string) {
	t.Helper()
	t.Cleanup(func() { i18n.SetLanguage(i18n.DefaultLanguage) })
	i18n.SetLanguage(lang)
}

// The user turn sits next to the audio in the request. A Turkish "İşle." in an
// otherwise English conversation is enough to pull the model's answer back into
// Turkish, so it has to follow the language like everything else.
func TestGoogleProvider_UserTurnFollowsTheLanguage(t *testing.T) {
	ts, body := captureBody(t, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)

	gp := NewGoogleProvider("test_key", "gemini-3.6-flash")
	gp.BaseURL = ts.URL
	gp.SetHTTPClient(ts.Client())

	useLanguage(t, "en")
	if _, err := gp.Generate(context.Background(), "prompt", "audio", "audio/ogg"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(body(), "Process.") {
		t.Errorf("the English user turn was not sent:\n%s", body())
	}
	if strings.Contains(body(), `İşle.`) {
		t.Errorf("the Turkish user turn leaked into an English run:\n%s", body())
	}
}

func TestOpenRouterProvider_UserTurnFollowsTheLanguage(t *testing.T) {
	ts, body := captureBody(t, `{"choices":[{"message":{"content":"ok"}}],"usage":{}}`)

	op := NewOpenRouterProvider("test_key", "test/model")
	op.BaseURL = ts.URL
	op.SetHTTPClient(ts.Client())

	useLanguage(t, "en")
	if _, err := op.Generate(context.Background(), "prompt", "audio", "audio/ogg"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(body(), "Process.") {
		t.Errorf("the English user turn was not sent:\n%s", body())
	}
}

func TestGoogleProvider_UserTurnStaysTurkishByDefault(t *testing.T) {
	ts, body := captureBody(t, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`)

	gp := NewGoogleProvider("test_key", "gemini-3.6-flash")
	gp.BaseURL = ts.URL
	gp.SetHTTPClient(ts.Client())

	if _, err := gp.Generate(context.Background(), "prompt", "audio", "audio/ogg"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(body(), `İşle.`) {
		t.Errorf("an unconfigured provider did not send the Turkish user turn:\n%s", body())
	}
}

func TestGoogleProvider_TruncationMarkerFollowsTheLanguage(t *testing.T) {
	ts, _ := captureBody(t,
		`{"candidates":[{"content":{"parts":[{"text":"half an answer"}]},"finishReason":"MAX_TOKENS"}]}`)

	gp := NewGoogleProvider("test_key", "gemini-3.6-flash")
	gp.BaseURL = ts.URL
	gp.SetHTTPClient(ts.Client())

	useLanguage(t, "en")
	res, err := gp.Generate(context.Background(), "prompt", "audio", "audio/ogg")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(res.Text, "hit the length limit") {
		t.Errorf("the truncation marker was not translated: %q", res.Text)
	}
	if !strings.HasPrefix(res.Text, "half an answer") {
		t.Errorf("the truncated answer itself was dropped: %q", res.Text)
	}
}
