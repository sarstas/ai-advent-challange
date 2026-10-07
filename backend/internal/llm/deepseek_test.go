package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient поднимает фейковый DeepSeek и возвращает клиент, смотрящий на него.
func newTestClient(t *testing.T, handler http.HandlerFunc) *DeepSeek {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewDeepSeek(srv.URL, "sk-test-key", "deepseek-chat", 5*time.Second)
}

func TestChatSuccess(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody map[string]any

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "deepseek-chat",
			"choices": [{"message": {"role": "assistant", "content": "Привет! Я DeepSeek."}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 9, "completion_tokens": 12, "total_tokens": 21}
		}`))
	})

	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "Привет"}},
	})
	if err != nil {
		t.Fatalf("Chat вернул ошибку: %v", err)
	}

	if gotPath != "/chat/completions" {
		t.Errorf("путь = %q, ожидался /chat/completions", gotPath)
	}
	if gotAuth != "Bearer sk-test-key" {
		t.Errorf("Authorization = %q, ожидался Bearer sk-test-key", gotAuth)
	}
	if model, _ := gotBody["model"].(string); model != "deepseek-chat" {
		t.Errorf("model в запросе = %v, ожидался deepseek-chat (дефолт клиента)", gotBody["model"])
	}
	if stream, _ := gotBody["stream"].(bool); stream {
		t.Error("stream должен быть false: MVP работает без стриминга")
	}
	if resp.Content != "Привет! Я DeepSeek." {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 21 {
		t.Errorf("total_tokens = %d, ожидалось 21", resp.Usage.TotalTokens)
	}
}

func TestChatUsesRequestedModel(t *testing.T) {
	var gotModel string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		gotModel, _ = body["model"].(string)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})

	if _, err := client.Chat(context.Background(), ChatRequest{
		Model:    "deepseek-reasoner",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}); err != nil {
		t.Fatalf("Chat вернул ошибку: %v", err)
	}
	if gotModel != "deepseek-reasoner" {
		t.Errorf("model = %q, ожидался deepseek-reasoner", gotModel)
	}
}

func TestChatMapsProviderErrors(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantCode   string
		wantStatus int
	}{
		{"неверный ключ", http.StatusUnauthorized, `{"error":{"message":"Authentication Fails","code":"invalid_api_key"}}`, "invalid_api_key", http.StatusUnauthorized},
		{"нет баланса", http.StatusPaymentRequired, `{"error":{"message":"Insufficient Balance"}}`, "insufficient_balance", http.StatusPaymentRequired},
		{"лимит запросов", http.StatusTooManyRequests, `{"error":{"message":"Rate limit reached"}}`, "rate_limited", http.StatusTooManyRequests},
		{"сбой провайдера", http.StatusInternalServerError, `oops`, "provider_error", http.StatusBadGateway},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			_, err := client.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}})
			if err == nil {
				t.Fatal("ожидалась ошибка, получили nil")
			}
			var apiErr *Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("ожидался *Error, получили %T", err)
			}
			if apiErr.Code != tc.wantCode {
				t.Errorf("code = %q, ожидался %q", apiErr.Code, tc.wantCode)
			}
			if apiErr.Status != tc.wantStatus {
				t.Errorf("status = %d, ожидался %d", apiErr.Status, tc.wantStatus)
			}
			if strings.TrimSpace(apiErr.Message) == "" {
				t.Error("сообщение об ошибке пустое — пользователь не поймёт, что случилось")
			}
		})
	}
}

func TestChatRejectsEmptyMessages(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("при пустом запросе поход в сеть не нужен")
	})

	_, err := client.Chat(context.Background(), ChatRequest{})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "empty_messages" {
		t.Fatalf("ожидалась ошибка empty_messages, получили %v", err)
	}
}
