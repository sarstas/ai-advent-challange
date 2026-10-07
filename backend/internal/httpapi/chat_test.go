package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aiadvent/backend/internal/config"
	"aiadvent/backend/internal/llm"
	"aiadvent/backend/internal/store"
)

// ---- тестовые двойники ----

type fakeStore struct {
	rows     []store.Request
	pingErr  error
	saveErrs bool
}

func (f *fakeStore) Ping(context.Context) error { return f.pingErr }

func (f *fakeStore) Save(_ context.Context, r store.Request) (store.Request, error) {
	r.ID = int64(len(f.rows) + 1)
	r.CreatedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f.rows = append(f.rows, r)
	return r, nil
}

func (f *fakeStore) List(context.Context, int) ([]store.Request, error) { return f.rows, nil }

func (f *fakeStore) Stats(context.Context) (int, int, error) {
	errs := 0
	for _, r := range f.rows {
		if r.Status == "error" {
			errs++
		}
	}
	return len(f.rows), errs, nil
}

type mockLLM struct {
	resp   llm.ChatResponse
	err    error
	gotReq llm.ChatRequest
	calls  int
}

func (m *mockLLM) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.calls++
	m.gotReq = req
	return m.resp, m.err
}

func testConfig(apiKey string) config.Config {
	return config.Config{
		Port:           "8080",
		DefaultModel:   "deepseek-chat",
		DeepSeekAPIKey: apiKey,
		LLMTimeout:     5 * time.Second,
	}
}

func newTestServer(cfg config.Config, client llm.Client, st RequestStore) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(cfg, client, st, log)
}

func postChat(t *testing.T, h http.Handler, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("ответ не JSON (%q): %v", rec.Body.String(), err)
	}
	return rec, payload
}

// ---- тесты ----

func TestChatSuccess(t *testing.T) {
	st := &fakeStore{}
	mock := &mockLLM{resp: llm.ChatResponse{
		Content: "Привет! Я DeepSeek.",
		Model:   "deepseek-chat",
		Usage:   llm.Usage{PromptTokens: 9, CompletionTokens: 12, TotalTokens: 21},
	}}
	h := newTestServer(testConfig("sk-test"), mock, st)

	rec, payload := postChat(t, h, `{"prompt":"  Привет  "}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("статус = %d, тело = %s", rec.Code, rec.Body.String())
	}
	if payload["content"] != "Привет! Я DeepSeek." {
		t.Errorf("content = %v", payload["content"])
	}
	if payload["id"] != float64(1) {
		t.Errorf("id = %v, ожидался 1 (id из БД)", payload["id"])
	}
	if mock.calls != 1 {
		t.Fatalf("LLM вызван %d раз, ожидался 1", mock.calls)
	}
	if got := mock.gotReq.Messages[0].Content; got != "Привет" {
		t.Errorf("в LLM ушёл промпт %q — должен быть обрезан по краям", got)
	}
	if got := mock.gotReq.Messages[0].Role; got != "user" {
		t.Errorf("роль = %q, ожидалась user", got)
	}

	if len(st.rows) != 1 {
		t.Fatalf("в БД записано %d строк, ожидалась 1", len(st.rows))
	}
	saved := st.rows[0]
	if saved.Status != "ok" || saved.Response != "Привет! Я DeepSeek." || saved.TotalTokens != 21 {
		t.Errorf("строка журнала записана неверно: %+v", saved)
	}
}

func TestChatEmptyPrompt(t *testing.T) {
	mock := &mockLLM{}
	st := &fakeStore{}
	h := newTestServer(testConfig("sk-test"), mock, st)

	for _, body := range []string{`{"prompt":"   "}`, `{"prompt":""}`, `{}`} {
		rec, payload := postChat(t, h, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body=%s: статус = %d, ожидался 400", body, rec.Code)
		}
		if code := payload["error"].(map[string]any)["code"]; code != "empty_prompt" {
			t.Errorf("body=%s: code = %v, ожидался empty_prompt", body, code)
		}
	}
	if mock.calls != 0 {
		t.Errorf("LLM вызван %d раз — при пустом промпте вызова быть не должно", mock.calls)
	}
}

func TestChatInvalidJSON(t *testing.T) {
	h := newTestServer(testConfig("sk-test"), &mockLLM{}, &fakeStore{})

	rec, payload := postChat(t, h, `{не json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("статус = %d, ожидался 400", rec.Code)
	}
	if code := payload["error"].(map[string]any)["code"]; code != "invalid_json" {
		t.Errorf("code = %v", code)
	}
}

func TestChatMissingAPIKey(t *testing.T) {
	mock := &mockLLM{}
	h := newTestServer(testConfig(""), mock, &fakeStore{})

	rec, payload := postChat(t, h, `{"prompt":"привет"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d, ожидался 401", rec.Code)
	}
	if code := payload["error"].(map[string]any)["code"]; code != "missing_api_key" {
		t.Errorf("code = %v, ожидался missing_api_key", code)
	}
	if mock.calls != 0 {
		t.Error("без ключа в LLM ходить нельзя")
	}
}

func TestChatUnknownModel(t *testing.T) {
	mock := &mockLLM{}
	h := newTestServer(testConfig("sk-test"), mock, &fakeStore{})

	rec, payload := postChat(t, h, `{"prompt":"привет","model":"gpt-5"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("статус = %d, ожидался 400", rec.Code)
	}
	if code := payload["error"].(map[string]any)["code"]; code != "unknown_model" {
		t.Errorf("code = %v, ожидался unknown_model", code)
	}
	if mock.calls != 0 {
		t.Error("неизвестную модель в провайдер отправлять нельзя")
	}
}

func TestChatLogsProviderError(t *testing.T) {
	st := &fakeStore{}
	mock := &mockLLM{err: &llm.Error{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "слишком много запросов"}}
	h := newTestServer(testConfig("sk-test"), mock, st)

	rec, payload := postChat(t, h, `{"prompt":"привет"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("статус = %d, ожидался 429", rec.Code)
	}
	if code := payload["error"].(map[string]any)["code"]; code != "rate_limited" {
		t.Errorf("code = %v", code)
	}

	if len(st.rows) != 1 {
		t.Fatalf("ошибка должна попадать в журнал, строк = %d", len(st.rows))
	}
	if st.rows[0].Status != "error" || st.rows[0].ErrorMessage == "" {
		t.Errorf("строка ошибки записана неверно: %+v", st.rows[0])
	}
}

func TestHealth(t *testing.T) {
	h := newTestServer(testConfig("sk-test"), &mockLLM{}, &fakeStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("статус = %d", rec.Code)
	}
	var payload map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	if payload["db"] != "ok" || payload["api_key"] != "set" {
		t.Errorf("health = %v", payload)
	}
}

func TestHealthDegradedWhenDBDown(t *testing.T) {
	st := &fakeStore{pingErr: context.DeadlineExceeded}
	h := newTestServer(testConfig("sk-test"), &mockLLM{}, st)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("статус = %d, ожидался 503", rec.Code)
	}
}

func TestHistory(t *testing.T) {
	st := &fakeStore{rows: []store.Request{
		{ID: 2, Prompt: "второй", Response: "ответ 2", Model: "deepseek-chat", Status: "ok", TotalTokens: 5},
		{ID: 1, Prompt: "первый", Response: "", Model: "deepseek-chat", Status: "error", ErrorMessage: "rate_limited"},
	}}
	h := newTestServer(testConfig("sk-test"), &mockLLM{}, st)

	req := httptest.NewRequest(http.MethodGet, "/api/history?limit=10", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("статус = %d, тело = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []struct {
			ID     int64  `json:"id"`
			Prompt string `json:"prompt"`
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"items"`
		Count  int `json:"count"`
		Total  int `json:"total"`
		Errors int `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count != 2 || payload.Total != 2 || payload.Errors != 1 {
		t.Errorf("сводка неверна: %+v", payload)
	}
	if payload.Items[1].Status != "error" || payload.Items[1].Error != "rate_limited" {
		t.Errorf("ошибка не попала в историю: %+v", payload.Items[1])
	}
}

func TestHistoryInvalidLimit(t *testing.T) {
	h := newTestServer(testConfig("sk-test"), &mockLLM{}, &fakeStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/history?limit=abc", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("статус = %d, ожидался 400", rec.Code)
	}
}
